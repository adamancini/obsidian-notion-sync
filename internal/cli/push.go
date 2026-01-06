package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"

	"github.com/adamancini/obsidian-notion-sync/internal/config"
	"github.com/adamancini/obsidian-notion-sync/internal/notion"
	"github.com/adamancini/obsidian-notion-sync/internal/parser"
	"github.com/adamancini/obsidian-notion-sync/internal/state"
	osync "github.com/adamancini/obsidian-notion-sync/internal/sync"
	"github.com/adamancini/obsidian-notion-sync/internal/transformer"
	"github.com/adamancini/obsidian-notion-sync/internal/vault"
)

var (
	pushAll    bool
	pushPath   string
	pushDryRun bool
	pushForce  bool
)

// pushCmd represents the push command.
var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Push local changes to Notion",
	Long: `Push local Obsidian changes to Notion.

By default, only pushes files that have changed since the last sync.
Use --all to push all files regardless of change detection.

Examples:
  obsidian-notion push                    # Push all changed files
  obsidian-notion push --all              # Push all files
  obsidian-notion push --path "work/**"   # Push files matching pattern
  obsidian-notion push --dry-run          # Show what would be pushed`,
	RunE: runPush,
}

func init() {
	pushCmd.Flags().BoolVar(&pushAll, "all", false, "push all files, not just changed ones")
	pushCmd.Flags().StringVar(&pushPath, "path", "", "glob pattern to filter files")
	pushCmd.Flags().BoolVar(&pushDryRun, "dry-run", false, "show what would be pushed without making changes")
	pushCmd.Flags().BoolVar(&pushForce, "force", false, "force push even if there are conflicts")
}

func runPush(cmd *cobra.Command, args []string) error {
	cfg, err := getConfig()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// 1. Open state database.
	dbPath := filepath.Join(cfg.Vault, ".obsidian-notion.db")
	db, err := state.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w (run 'obsidian-notion init' first)", err)
	}
	defer db.Close()

	// 2. Initialize Notion client.
	client := notion.New(cfg.Notion.Token,
		notion.WithRateLimit(cfg.RateLimit.RequestsPerSecond),
		notion.WithBatchSize(cfg.RateLimit.BatchSize),
	)

	// 3. Get files to push.
	filesToPush, err := getFilesToPush(ctx, cfg, db)
	if err != nil {
		return fmt.Errorf("get files to push: %w", err)
	}

	// Filter by path pattern if specified.
	if pushPath != "" {
		filesToPush = filterByPath(filesToPush, pushPath)
	}

	if len(filesToPush) == 0 {
		fmt.Println("No files to push.")
		return nil
	}

	// 4. Check for conflicts.
	linkRegistry := state.NewLinkRegistry(db)
	conflicts := checkConflicts(filesToPush)
	if len(conflicts) > 0 && !pushForce {
		fmt.Printf("Found %d conflict(s). Use --force to push anyway, or resolve with 'obsidian-notion conflicts'.\n", len(conflicts))
		for _, c := range conflicts {
			fmt.Printf("  ! %s\n", c)
		}
		return fmt.Errorf("aborting due to conflicts")
	}

	fmt.Printf("Pushing %d change(s) to Notion...\n", len(filesToPush))
	if pushDryRun {
		fmt.Println("(dry-run mode - no changes will be made)")
		for _, f := range filesToPush {
			switch f.changeType {
			case state.ChangeCreated:
				fmt.Printf("  + would create: %s\n", f.path)
			case state.ChangeModified:
				fmt.Printf("  M would update: %s\n", f.path)
			case state.ChangeRenamed:
				fmt.Printf("  R would rename: %s -> %s\n", f.oldPath, f.path)
			case state.ChangeDeleted:
				fmt.Printf("  D would %s: %s\n", cfg.Sync.DeletionStrategy, f.path)
			}
		}
		return nil
	}

	// 5. Separate files by change type for processing.
	var deletions, renames, createModify []pushFile
	for _, f := range filesToPush {
		switch f.changeType {
		case state.ChangeDeleted:
			deletions = append(deletions, f)
		case state.ChangeRenamed:
			renames = append(renames, f)
		case state.ChangeCreated, state.ChangeModified:
			createModify = append(createModify, f)
		}
	}

	// 6. Process deletions and renames sequentially (state-dependent).
	var renamed, deleted int
	var failed int32
	for _, f := range deletions {
		if err := handleDeletion(ctx, cfg, db, client, linkRegistry, f); err != nil {
			fmt.Fprintf(os.Stderr, "  Error deleting %s: %v\n", f.path, err)
			atomic.AddInt32(&failed, 1)
			continue
		}
		deleted++
		if verbose {
			fmt.Printf("  D %s (%s)\n", f.path, cfg.Sync.DeletionStrategy)
		}
	}

	for _, f := range renames {
		if err := handleRename(ctx, cfg, db, client, linkRegistry, f); err != nil {
			fmt.Fprintf(os.Stderr, "  Error renaming %s: %v\n", f.oldPath, err)
			atomic.AddInt32(&failed, 1)
			continue
		}
		renamed++
		if verbose {
			fmt.Printf("  R %s -> %s\n", f.oldPath, f.path)
		}
	}

	// 7. Process creates/modifies in parallel.
	var created, updated int32
	var results []osync.Task[pushFile, pushResult] // Store results for second pass

	if len(createModify) > 0 {
		// Initialize worker pool.
		workers := cfg.RateLimit.Workers
		if workers < 1 {
			workers = 4
		}
		pool := osync.NewWorkerPool(workers)

		// Initialize progress reporter.
		progress := osync.NewProgress(len(createModify), os.Stdout)
		progress.SetEnabled(!verbose) // Use progress bar only when not verbose

		// Create processing context.
		procCtx := &pushContext{
			cfg:          cfg,
			db:           db,
			client:       client,
			linkRegistry: linkRegistry,
			parser:       parser.New(),
			scanner:      vault.NewScanner(cfg.Vault, cfg.Sync.Ignore),
		}

		// Process files in parallel.
		results = osync.ProcessWithProgress(ctx, pool, createModify, procCtx.processFile, progress.SimpleCallback())
		progress.Finish()

		// Collect results.
		for _, result := range results {
			if result.Err != nil {
				fmt.Fprintf(os.Stderr, "  Error processing %s: %v\n", result.Input.path, result.Err)
				atomic.AddInt32(&failed, 1)
			} else if result.Result.isNew {
				atomic.AddInt32(&created, 1)
				if verbose {
					fmt.Printf("  + %s (page: %s)\n", result.Input.path, result.Result.pageID)
				}
			} else {
				atomic.AddInt32(&updated, 1)
				if verbose {
					fmt.Printf("  M %s\n", result.Input.path)
				}
			}
		}
	}

	// 8. Second pass: resolve wiki-links and update pages.
	// First resolve all links in the database.
	resolvedCount, _ := linkRegistry.ResolveAll()

	// Collect newly created pages with wiki-links for second pass update.
	var pagesNeedingLinkUpdate []pushFile
	for _, result := range results {
		if result.Err == nil && result.Result.hasWikiLinks && result.Result.isNew {
			pagesNeedingLinkUpdate = append(pagesNeedingLinkUpdate, result.Input)
		}
	}

	// Update pages with newly resolved wiki-links.
	var linkUpdates int32
	if len(pagesNeedingLinkUpdate) > 0 && resolvedCount > 0 {
		if verbose {
			fmt.Printf("  Updating %d page(s) with resolved wiki-links...\n", len(pagesNeedingLinkUpdate))
		}

		// Re-process files to update with resolved links.
		for _, f := range pagesNeedingLinkUpdate {
			syncState, err := db.GetState(f.path)
			if err != nil || syncState == nil {
				continue
			}

			// Re-read and re-transform with links now resolvable.
			fullPath := filepath.Join(cfg.Vault, f.path)
			content, err := os.ReadFile(fullPath)
			if err != nil {
				continue
			}

			p := parser.New()
			note, err := p.Parse(f.path, content)
			if err != nil {
				continue
			}

			t := transformer.New(linkRegistry, buildTransformerConfig(cfg, f.path))

			notionPage, err := t.Transform(note)
			if err != nil {
				continue
			}

			// Update the page with resolved wiki-links.
			if err := client.UpdatePage(ctx, syncState.NotionPageID, notionPage); err != nil {
				if verbose {
					fmt.Fprintf(os.Stderr, "  Warning: failed to update links in %s: %v\n", f.path, err)
				}
				continue
			}
			atomic.AddInt32(&linkUpdates, 1)
		}
	}

	if verbose && (resolvedCount > 0 || linkUpdates > 0) {
		fmt.Printf("  Resolved %d wiki-links, updated %d page(s)\n", resolvedCount, linkUpdates)
	}

	// Print summary.
	fmt.Println()
	fmt.Printf("Push complete:\n")
	fmt.Printf("  Created: %d\n", created)
	fmt.Printf("  Updated: %d\n", updated)
	if renamed > 0 {
		fmt.Printf("  Renamed: %d\n", renamed)
	}
	if deleted > 0 {
		fmt.Printf("  Deleted: %d\n", deleted)
	}
	if failed > 0 {
		fmt.Printf("  Failed:  %d\n", failed)
	}

	return nil
}

// handleDeletion processes a file deletion based on the configured strategy.
func handleDeletion(ctx context.Context, cfg *config.Config, db *state.DB, client *notion.Client, linkRegistry *state.LinkRegistry, f pushFile) error {
	if f.state == nil || f.state.NotionPageID == "" {
		// No Notion page to delete, just clean up state.
		return db.DeleteState(f.path)
	}

	strategy := cfg.Sync.DeletionStrategy
	if strategy == "" {
		strategy = "archive" // Default to archive.
	}

	switch strategy {
	case "archive":
		// Archive the Notion page.
		if err := client.ArchivePage(ctx, f.state.NotionPageID); err != nil {
			return fmt.Errorf("archive page: %w", err)
		}
	case "delete":
		// Delete the Notion page (actually archives, as Notion API doesn't support permanent delete).
		if err := client.DeletePage(ctx, f.state.NotionPageID); err != nil {
			return fmt.Errorf("delete page: %w", err)
		}
	case "ignore":
		// Do nothing to Notion, just remove from local tracking.
	default:
		return fmt.Errorf("unknown deletion strategy: %s", strategy)
	}

	// Remove from sync state.
	if err := db.DeleteState(f.path); err != nil {
		return fmt.Errorf("delete state: %w", err)
	}

	// Clear links from this file.
	_ = linkRegistry.ClearLinksFrom(f.path)

	return nil
}

// handleRename processes a file rename.
func handleRename(ctx context.Context, cfg *config.Config, db *state.DB, client *notion.Client, linkRegistry *state.LinkRegistry, f pushFile) error {
	if f.state == nil || f.state.NotionPageID == "" {
		return fmt.Errorf("no sync state for renamed file")
	}

	// 1. Update Notion page title if title comes from filename.
	// Extract new title from new filename.
	basename := filepath.Base(f.path)
	newTitle := strings.TrimSuffix(basename, filepath.Ext(basename))

	if err := client.UpdatePageTitle(ctx, f.state.NotionPageID, newTitle); err != nil {
		return fmt.Errorf("update page title: %w", err)
	}

	// 2. Update sync state with new path.
	if err := db.UpdatePath(f.oldPath, f.path); err != nil {
		return fmt.Errorf("update state path: %w", err)
	}

	// 3. Update link registry with new path.
	if err := linkRegistry.UpdateSourcePath(f.oldPath, f.path); err != nil {
		return fmt.Errorf("update link source path: %w", err)
	}

	// 4. Update last sync time.
	syncState, err := db.GetState(f.path)
	if err != nil {
		return fmt.Errorf("get updated state: %w", err)
	}
	if syncState != nil {
		syncState.LastSync = time.Now()
		syncState.SyncDirection = "push"
		if err := db.SetState(syncState); err != nil {
			return fmt.Errorf("update sync time: %w", err)
		}
	}

	return nil
}

// pushFile represents a file to be pushed.
type pushFile struct {
	path       string
	oldPath    string // For renames: the previous path
	state      *state.SyncState
	mtime      time.Time
	changeType state.ChangeType
}

// getFilesToPush returns the list of files that need to be pushed.
func getFilesToPush(ctx context.Context, cfg *config.Config, db *state.DB) ([]pushFile, error) {
	var files []pushFile

	if pushAll {
		// Push all files.
		scanner := vault.NewScanner(cfg.Vault, cfg.Sync.Ignore)
		vaultFiles, err := scanner.Scan(ctx)
		if err != nil {
			return nil, err
		}

		for _, f := range vaultFiles {
			syncState, _ := db.GetState(f.Path)
			changeType := state.ChangeModified
			if syncState == nil || syncState.NotionPageID == "" {
				changeType = state.ChangeCreated
			}
			files = append(files, pushFile{
				path:       f.Path,
				state:      syncState,
				mtime:      f.Info.ModTime(),
				changeType: changeType,
			})
		}
	} else {
		// Push only changed files.
		detector := state.NewChangeDetector(db, cfg.Vault)
		changes, err := detector.DetectChanges(ctx)
		if err != nil {
			return nil, err
		}

		for _, c := range changes {
			if c.Direction == state.DirectionPull {
				continue // Skip pull-only changes.
			}
			files = append(files, pushFile{
				path:       c.Path,
				oldPath:    c.OldPath,
				state:      c.State,
				mtime:      c.LocalMtime,
				changeType: c.Type,
			})
		}

		// Also include pending files (never synced).
		pendingStates, _ := db.ListStates("pending")
		for _, s := range pendingStates {
			// Check if already in list.
			found := false
			for _, f := range files {
				if f.path == s.ObsidianPath {
					found = true
					break
				}
			}
			if !found {
				files = append(files, pushFile{
					path:       s.ObsidianPath,
					state:      s,
					mtime:      s.ObsidianMtime,
					changeType: state.ChangeCreated,
				})
			}
		}
	}

	return files, nil
}

// filterByPath filters files by a glob pattern.
func filterByPath(files []pushFile, pattern string) []pushFile {
	var filtered []pushFile
	for _, f := range files {
		matched, _ := filepath.Match(pattern, f.path)
		if matched {
			filtered = append(filtered, f)
		}
	}
	return filtered
}

// checkConflicts returns paths with conflict status.
func checkConflicts(files []pushFile) []string {
	var conflicts []string
	for _, f := range files {
		if f.state != nil && f.state.Status == "conflict" {
			conflicts = append(conflicts, f.path)
		}
	}
	return conflicts
}

// pushContext holds shared dependencies for parallel file processing.
type pushContext struct {
	cfg          *config.Config
	db           *state.DB
	client       *notion.Client
	linkRegistry *state.LinkRegistry
	parser       *parser.Parser
	scanner      *vault.Scanner
}

// pushResult holds the result of processing a single file.
type pushResult struct {
	pageID       string
	isNew        bool
	hasWikiLinks bool // Track if file has wiki-links for second pass
}

// processFile processes a single file for push (create or update).
func (pc *pushContext) processFile(ctx context.Context, f pushFile) (pushResult, error) {
	// Read file content.
	fullPath := filepath.Join(pc.cfg.Vault, f.path)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return pushResult{}, fmt.Errorf("read file: %w", err)
	}

	// Parse the markdown.
	note, err := pc.parser.Parse(f.path, content)
	if err != nil {
		return pushResult{}, fmt.Errorf("parse markdown: %w", err)
	}

	// Register title and aliases for wiki-link resolution.
	// This allows [[Title]] to resolve to filename.md when title differs from filename.
	_ = pc.linkRegistry.ClearAliases(f.path) // Clear old aliases first
	if title, ok := note.Frontmatter["title"].(string); ok && title != "" {
		if err := pc.linkRegistry.RegisterAlias(f.path, title, "title"); err != nil {
			// Non-fatal: log but continue
			fmt.Fprintf(os.Stderr, "  Warning: failed to register title alias for %s: %v\n", f.path, err)
		}
	}
	// Also register aliases from frontmatter.
	if aliases, ok := note.Frontmatter["aliases"]; ok {
		var aliasStrings []string
		switch v := aliases.(type) {
		case []any:
			for _, a := range v {
				if s, ok := a.(string); ok && s != "" {
					aliasStrings = append(aliasStrings, s)
				}
			}
		case []string:
			aliasStrings = v
		}
		if len(aliasStrings) > 0 {
			if err := pc.linkRegistry.RegisterAliases(f.path, aliasStrings, "alias"); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: failed to register aliases for %s: %v\n", f.path, err)
			}
		}
	}

	// Register wiki-links for two-pass resolution.
	// Clear existing links first (in case file was modified and links changed).
	_ = pc.linkRegistry.ClearLinksFrom(f.path)
	if len(note.WikiLinks) > 0 {
		targets := make([]string, len(note.WikiLinks))
		for i, link := range note.WikiLinks {
			targets[i] = link.Target
		}
		if err := pc.linkRegistry.RegisterLinks(f.path, targets); err != nil {
			// Non-fatal: log but continue processing
			fmt.Fprintf(os.Stderr, "  Warning: failed to register links from %s: %v\n", f.path, err)
		}
	}

	// Create transformer with path-specific property mappings.
	t := transformer.New(pc.linkRegistry, buildTransformerConfig(pc.cfg, f.path))

	// Transform to Notion page structure.
	notionPage, err := t.Transform(note)
	if err != nil {
		return pushResult{}, fmt.Errorf("transform to Notion: %w", err)
	}

	var pageID string
	var isNew bool

	if f.state == nil || f.state.NotionPageID == "" {
		// Create new page.
		parentID := pc.cfg.GetDatabaseForPath(f.path)
		if parentID == "" {
			parentID = pc.cfg.Notion.DefaultPage
		}

		result, err := pc.client.CreatePage(ctx, parentID, notionPage)
		if err != nil {
			return pushResult{}, fmt.Errorf("create page: %w", err)
		}
		pageID = result.PageID
		isNew = true
	} else {
		// Update existing page.
		pageID = f.state.NotionPageID

		if err := pc.client.UpdatePage(ctx, pageID, notionPage); err != nil {
			return pushResult{}, fmt.Errorf("update page: %w", err)
		}
	}

	// Compute content hashes (normalized, with separate frontmatter hash).
	hashes, err := state.HashFileDetailed(fullPath)
	if err != nil {
		hashes = state.ContentHashes{} // Non-fatal, continue without hashes
	}

	// Update sync state with both content and frontmatter hashes.
	// ContentHash stores the body hash (for detecting content-only changes).
	// FrontmatterHash stores the metadata hash (for property-only updates).
	syncState := &state.SyncState{
		ObsidianPath:    f.path,
		NotionPageID:    pageID,
		ObsidianMtime:   f.mtime,
		NotionMtime:     time.Now(),
		ContentHash:     hashes.ContentHash,
		FrontmatterHash: hashes.FrontmatterHash,
		LastSync:        time.Now(),
		SyncDirection:   "push",
		Status:          "synced",
	}
	if err := pc.db.SetState(syncState); err != nil {
		return pushResult{}, fmt.Errorf("update state: %w", err)
	}

	return pushResult{pageID: pageID, isNew: isNew, hasWikiLinks: len(note.WikiLinks) > 0}, nil
}
