package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jomei/notionapi"
	"github.com/spf13/cobra"

	"github.com/adamancini/obsidian-notion-sync/internal/config"
	"github.com/adamancini/obsidian-notion-sync/internal/notion"
	"github.com/adamancini/obsidian-notion-sync/internal/state"
	osync "github.com/adamancini/obsidian-notion-sync/internal/sync"
	"github.com/adamancini/obsidian-notion-sync/internal/transformer"
)

var (
	pullAll    bool
	pullPath   string
	pullDryRun bool
	pullForce  bool
)

// pullCmd represents the pull command.
var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Pull changes from Notion to local vault",
	Long: `Pull changes from Notion to the local Obsidian vault.

By default, only pulls pages that have changed since the last sync.
Use --all to pull all tracked pages regardless of change detection.

Examples:
  obsidian-notion pull                    # Pull all changed pages
  obsidian-notion pull --all              # Pull all tracked pages
  obsidian-notion pull --path "work/**"   # Pull pages matching pattern
  obsidian-notion pull --dry-run          # Show what would be pulled`,
	RunE: runPull,
}

func init() {
	pullCmd.Flags().BoolVar(&pullAll, "all", false, "pull all tracked pages, not just changed ones")
	pullCmd.Flags().StringVar(&pullPath, "path", "", "glob pattern to filter files")
	pullCmd.Flags().BoolVar(&pullDryRun, "dry-run", false, "show what would be pulled without making changes")
	pullCmd.Flags().BoolVar(&pullForce, "force", false, "force pull even if there are conflicts")
}

func runPull(cmd *cobra.Command, args []string) error {
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

	// 3. Get pages to pull.
	pagesToPull, err := getPagesToPull(ctx, cfg, db, client)
	if err != nil {
		return fmt.Errorf("get pages to pull: %w", err)
	}

	// Filter by path pattern if specified.
	if pullPath != "" {
		pagesToPull = filterPullByPath(pagesToPull, pullPath)
	}

	if len(pagesToPull) == 0 {
		fmt.Println("No pages to pull.")
		return nil
	}

	// 4. Check for conflicts.
	linkRegistry := state.NewLinkRegistry(db)
	conflicts := checkPullConflicts(pagesToPull)
	if len(conflicts) > 0 && !pullForce {
		fmt.Printf("Found %d conflict(s). Use --force to pull anyway, or resolve with 'obsidian-notion conflicts'.\n", len(conflicts))
		for _, c := range conflicts {
			fmt.Printf("  ! %s\n", c)
		}
		return fmt.Errorf("aborting due to conflicts")
	}

	fmt.Printf("Pulling %d change(s) from Notion...\n", len(pagesToPull))
	if pullDryRun {
		fmt.Println("(dry-run mode - no changes will be made)")
		for _, p := range pagesToPull {
			switch p.changeType {
			case pullChangeNew:
				fmt.Printf("  + would create: %s\n", p.localPath)
			case pullChangeModified:
				fmt.Printf("  M would update: %s\n", p.localPath)
			case pullChangeDeleted:
				fmt.Printf("  D would %s: %s\n", cfg.Sync.DeletionStrategy, p.localPath)
			}
		}
		return nil
	}

	// 5. Separate pages by change type for processing.
	var deletions, fetchModify []pullPage
	for _, p := range pagesToPull {
		switch p.changeType {
		case pullChangeDeleted:
			deletions = append(deletions, p)
		case pullChangeNew, pullChangeModified:
			fetchModify = append(fetchModify, p)
		}
	}

	// 6. Process deletions sequentially (state-dependent).
	var deleted int
	var failed int32
	for _, p := range deletions {
		if err := handlePullDeletion(cfg, db, linkRegistry, p); err != nil {
			fmt.Fprintf(os.Stderr, "  Error deleting %s: %v\n", p.localPath, err)
			atomic.AddInt32(&failed, 1)
			continue
		}
		deleted++
		if verbose {
			fmt.Printf("  D %s (%s)\n", p.localPath, cfg.Sync.DeletionStrategy)
		}
	}

	// 7. Process new/modified pages in parallel.
	var created, updated int32
	if len(fetchModify) > 0 {
		// Initialize worker pool.
		workers := cfg.RateLimit.Workers
		if workers < 1 {
			workers = 4
		}
		pool := osync.NewWorkerPool(workers)

		// Initialize progress reporter.
		progress := osync.NewProgress(len(fetchModify), os.Stdout)
		progress.SetEnabled(!verbose) // Use progress bar only when not verbose

		// Create processing context.
		procCtx := &pullContext{
			cfg:          cfg,
			db:           db,
			client:       client,
			linkRegistry: linkRegistry,
		}

		// Process pages in parallel.
		results := osync.ProcessWithProgress(ctx, pool, fetchModify, procCtx.processPage, progress.SimpleCallback())
		progress.Finish()

		// Collect results.
		for _, result := range results {
			if result.Err != nil {
				fmt.Fprintf(os.Stderr, "  Error processing %s: %v\n", result.Input.localPath, result.Err)
				atomic.AddInt32(&failed, 1)
			} else if result.Result.isNew {
				atomic.AddInt32(&created, 1)
				if verbose {
					fmt.Printf("  + %s\n", result.Input.localPath)
				}
			} else {
				atomic.AddInt32(&updated, 1)
				if verbose {
					fmt.Printf("  M %s\n", result.Input.localPath)
				}
			}
		}
	}

	// Print summary.
	fmt.Println()
	fmt.Printf("Pull complete:\n")
	fmt.Printf("  Created: %d\n", created)
	fmt.Printf("  Updated: %d\n", updated)
	if deleted > 0 {
		fmt.Printf("  Deleted: %d\n", deleted)
	}
	if failed > 0 {
		fmt.Printf("  Failed:  %d\n", failed)
	}

	return nil
}

// pullChangeType represents the type of change for a pull operation.
type pullChangeType int

const (
	pullChangeNew pullChangeType = iota
	pullChangeModified
	pullChangeDeleted
)

// pullPage represents a page to be pulled.
type pullPage struct {
	notionPageID string
	localPath    string
	state        *state.SyncState
	notionMtime  time.Time
	changeType   pullChangeType
}

// getPagesToPull returns the list of pages that need to be pulled.
func getPagesToPull(ctx context.Context, cfg *config.Config, db *state.DB, client *notion.Client) ([]pullPage, error) {
	var pages []pullPage

	// Get all synced states.
	states, err := db.ListStates("synced")
	if err != nil {
		return nil, fmt.Errorf("list synced states: %w", err)
	}

	// Check each tracked page for changes.
	for _, s := range states {
		if s.NotionPageID == "" {
			continue
		}

		// Get current page metadata from Notion.
		notionPage, err := client.GetPage(ctx, s.NotionPageID)
		if err != nil {
			// Page may have been deleted in Notion.
			if isNotFoundError(err) {
				pages = append(pages, pullPage{
					notionPageID: s.NotionPageID,
					localPath:    s.ObsidianPath,
					state:        s,
					changeType:   pullChangeDeleted,
				})
				continue
			}
			// Skip pages with other errors.
			fmt.Fprintf(os.Stderr, "  Warning: could not check %s: %v\n", s.ObsidianPath, err)
			continue
		}

		// Check if page was modified.
		notionMtime := notionPage.LastEditedTime
		if pullAll || notionMtime.After(s.NotionMtime) {
			pages = append(pages, pullPage{
				notionPageID: s.NotionPageID,
				localPath:    s.ObsidianPath,
				state:        s,
				notionMtime:  notionMtime,
				changeType:   pullChangeModified,
			})
		}
	}

	// Also check for new pages in the database.
	if cfg.Notion.DefaultDatabase != "" {
		newPages, err := discoverNewPages(ctx, cfg, db, client)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: could not discover new pages: %v\n", err)
		} else {
			pages = append(pages, newPages...)
		}
	}

	return pages, nil
}

// discoverNewPages finds pages in Notion that don't exist locally.
func discoverNewPages(ctx context.Context, cfg *config.Config, db *state.DB, client *notion.Client) ([]pullPage, error) {
	var pages []pullPage

	// Query the default database.
	resp, err := client.QueryDatabase(ctx, cfg.Notion.DefaultDatabase, nil)
	if err != nil {
		return nil, err
	}

	for _, result := range resp.Results {
		pageID := string(result.ID)

		// Check if we already track this page.
		existing, _ := db.GetStateByNotionID(pageID)
		if existing != nil {
			continue // Already tracked.
		}

		// Extract title from properties.
		title := extractTitle(result.Properties)
		if title == "" {
			title = "Untitled"
		}

		// Generate local path.
		localPath := sanitizeFilename(title) + ".md"

		pages = append(pages, pullPage{
			notionPageID: pageID,
			localPath:    localPath,
			notionMtime:  result.LastEditedTime,
			changeType:   pullChangeNew,
		})
	}

	return pages, nil
}

// extractTitle extracts the title from Notion page properties.
func extractTitle(props notionapi.Properties) string {
	for _, prop := range props {
		if titleProp, ok := prop.(*notionapi.TitleProperty); ok {
			if len(titleProp.Title) > 0 {
				return titleProp.Title[0].PlainText
			}
		}
	}
	return ""
}

// sanitizeFilename converts a title to a valid filename.
func sanitizeFilename(title string) string {
	// Replace invalid characters.
	invalid := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	result := title
	for _, char := range invalid {
		result = strings.ReplaceAll(result, char, "-")
	}
	// Trim spaces and dots from ends.
	result = strings.TrimSpace(result)
	result = strings.Trim(result, ".")
	if result == "" {
		result = "Untitled"
	}
	return result
}

// filterPullByPath filters pages by a glob pattern.
func filterPullByPath(pages []pullPage, pattern string) []pullPage {
	var filtered []pullPage
	for _, p := range pages {
		matched, _ := filepath.Match(pattern, p.localPath)
		if matched {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

// checkPullConflicts returns paths with conflict status.
func checkPullConflicts(pages []pullPage) []string {
	var conflicts []string
	for _, p := range pages {
		if p.state != nil && p.state.Status == "conflict" {
			conflicts = append(conflicts, p.localPath)
		}
	}
	return conflicts
}

// handlePullDeletion processes a deletion from Notion based on the configured strategy.
func handlePullDeletion(cfg *config.Config, db *state.DB, linkRegistry *state.LinkRegistry, p pullPage) error {
	strategy := cfg.Sync.DeletionStrategy
	if strategy == "" {
		strategy = "archive" // Default to archive.
	}

	switch strategy {
	case "archive":
		// Move local file to .obsidian-notion-archive folder.
		fullPath := filepath.Join(cfg.Vault, p.localPath)
		archivePath := filepath.Join(cfg.Vault, ".obsidian-notion-archive", p.localPath)

		if err := os.MkdirAll(filepath.Dir(archivePath), 0755); err != nil {
			return fmt.Errorf("create archive directory: %w", err)
		}

		if err := os.Rename(fullPath, archivePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("archive file: %w", err)
		}

	case "delete":
		// Delete the local file.
		fullPath := filepath.Join(cfg.Vault, p.localPath)
		if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("delete file: %w", err)
		}

	case "ignore":
		// Do nothing to local file, just remove from tracking.
	default:
		return fmt.Errorf("unknown deletion strategy: %s", strategy)
	}

	// Remove from sync state.
	if err := db.DeleteState(p.localPath); err != nil {
		return fmt.Errorf("delete state: %w", err)
	}

	// Clear links from this file.
	_ = linkRegistry.ClearLinksFrom(p.localPath)

	return nil
}

// isNotFoundError checks if an error is a "not found" error from Notion.
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "not found") ||
		strings.Contains(errStr, "Could not find") ||
		strings.Contains(errStr, "404")
}

// pullContext holds shared dependencies for parallel page processing.
type pullContext struct {
	cfg          *config.Config
	db           *state.DB
	client       *notion.Client
	linkRegistry *state.LinkRegistry
}

// pullResult holds the result of processing a single page.
type pullResult struct {
	isNew bool
}

// processPage processes a single page for pull (fetch, transform, write).
func (pc *pullContext) processPage(ctx context.Context, p pullPage) (pullResult, error) {
	// Fetch full page content from Notion.
	notionPage, err := pc.client.FetchPage(ctx, p.notionPageID)
	if err != nil {
		return pullResult{}, fmt.Errorf("fetch page: %w", err)
	}

	// Create reverse transformer with path-specific property mappings.
	rt := transformer.NewReverse(pc.linkRegistry, buildTransformerConfig(pc.cfg, p.localPath))

	// Transform to markdown.
	markdown, err := rt.NotionToMarkdown(notionPage)
	if err != nil {
		return pullResult{}, fmt.Errorf("transform to markdown: %w", err)
	}

	// Ensure directory exists.
	fullPath := filepath.Join(pc.cfg.Vault, p.localPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return pullResult{}, fmt.Errorf("create directory: %w", err)
	}

	// Write file.
	if err := os.WriteFile(fullPath, markdown, 0644); err != nil {
		return pullResult{}, fmt.Errorf("write file: %w", err)
	}

	// Update sync state.
	contentHash, _ := state.HashFile(fullPath)
	fileInfo, _ := os.Stat(fullPath)
	var mtime time.Time
	if fileInfo != nil {
		mtime = fileInfo.ModTime()
	}

	syncState := p.state
	if syncState == nil {
		syncState = &state.SyncState{
			ObsidianPath: p.localPath,
			NotionPageID: p.notionPageID,
		}
	}
	syncState.ContentHash = contentHash
	syncState.ObsidianMtime = mtime
	syncState.NotionMtime = p.notionMtime
	syncState.LastSync = time.Now()
	syncState.Status = "synced"
	syncState.SyncDirection = "pull"

	if err := pc.db.SetState(syncState); err != nil {
		return pullResult{}, fmt.Errorf("update state: %w", err)
	}

	return pullResult{isNew: p.changeType == pullChangeNew}, nil
}
