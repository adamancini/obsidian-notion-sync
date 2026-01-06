package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/adamancini/obsidian-notion-sync/internal/config"
	"github.com/adamancini/obsidian-notion-sync/internal/notion"
	"github.com/adamancini/obsidian-notion-sync/internal/parser"
	"github.com/adamancini/obsidian-notion-sync/internal/state"
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
	cfg := getConfig()

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

	// 5. Initialize parser and transformer.
	p := parser.New()
	xform := transformer.New(linkRegistry, &transformer.Config{
		UnresolvedLinkStyle: cfg.Transform.UnresolvedLinks,
		CalloutIcons:        cfg.Transform.Callouts,
		DataviewHandling:    cfg.Transform.Dataview,
		FlattenHeadings:     true,
	})

	// 6. Process changes by type.
	var created, updated, renamed, deleted, failed int
	scanner := vault.NewScanner(cfg.Vault, cfg.Sync.Ignore)

	for _, f := range filesToPush {
		switch f.changeType {
		case state.ChangeDeleted:
			// Handle deletion based on strategy.
			if err := handleDeletion(ctx, cfg, db, client, linkRegistry, f); err != nil {
				fmt.Fprintf(os.Stderr, "  Error deleting %s: %v\n", f.path, err)
				failed++
				continue
			}
			deleted++
			if verbose {
				fmt.Printf("  D %s (%s)\n", f.path, cfg.Sync.DeletionStrategy)
			}

		case state.ChangeRenamed:
			// Handle rename.
			if err := handleRename(ctx, cfg, db, client, linkRegistry, f); err != nil {
				fmt.Fprintf(os.Stderr, "  Error renaming %s: %v\n", f.oldPath, err)
				failed++
				continue
			}
			renamed++
			if verbose {
				fmt.Printf("  R %s -> %s\n", f.oldPath, f.path)
			}

		case state.ChangeCreated, state.ChangeModified:
			// Handle create or update.
			content, err := scanner.ReadFile(f.path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  Error reading %s: %v\n", f.path, err)
				failed++
				continue
			}

			note, err := p.Parse(f.path, content)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  Error parsing %s: %v\n", f.path, err)
				failed++
				continue
			}

			// Set title from filename if not in frontmatter.
			if _, ok := note.Frontmatter["title"]; !ok {
				basename := filepath.Base(f.path)
				title := strings.TrimSuffix(basename, filepath.Ext(basename))
				note.Frontmatter["title"] = title
			}

			// Transform to Notion page.
			notionPage, err := xform.Transform(note)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  Error transforming %s: %v\n", f.path, err)
				failed++
				continue
			}

			// Get sync state.
			syncState := f.state
			if syncState == nil {
				syncState = &state.SyncState{
					ObsidianPath: f.path,
				}
			}

			// Create or update in Notion.
			var pageID string
			if syncState.NotionPageID == "" {
				// Create new page.
				databaseID := cfg.GetDatabaseForPath(f.path)
				if databaseID == "" && cfg.Notion.DefaultPage != "" {
					result, err := client.CreatePageUnderPage(ctx, cfg.Notion.DefaultPage, notionPage)
					if err != nil {
						fmt.Fprintf(os.Stderr, "  Error creating %s: %v\n", f.path, err)
						failed++
						continue
					}
					pageID = result.PageID
				} else if databaseID != "" {
					result, err := client.CreatePage(ctx, databaseID, notionPage)
					if err != nil {
						fmt.Fprintf(os.Stderr, "  Error creating %s: %v\n", f.path, err)
						failed++
						continue
					}
					pageID = result.PageID
				} else {
					fmt.Fprintf(os.Stderr, "  Error: no database or page configured for %s\n", f.path)
					failed++
					continue
				}
				created++
				if verbose {
					fmt.Printf("  + %s (page: %s)\n", f.path, pageID)
				}
			} else {
				// Update existing page.
				pageID = syncState.NotionPageID
				if err := client.UpdatePage(ctx, pageID, notionPage); err != nil {
					fmt.Fprintf(os.Stderr, "  Error updating %s: %v\n", f.path, err)
					failed++
					continue
				}
				updated++
				if verbose {
					fmt.Printf("  M %s\n", f.path)
				}
			}

			// Update sync state.
			contentHash, _ := state.HashFile(filepath.Join(cfg.Vault, f.path))
			syncState.NotionPageID = pageID
			syncState.ContentHash = contentHash
			syncState.ObsidianMtime = f.mtime
			syncState.LastSync = time.Now()
			syncState.Status = "synced"
			syncState.SyncDirection = "push"

			if err := db.SetState(syncState); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: failed to update state for %s: %v\n", f.path, err)
			}

			// Register wiki-links for resolution.
			if len(note.WikiLinks) > 0 {
				targets := make([]string, len(note.WikiLinks))
				for i, link := range note.WikiLinks {
					targets[i] = link.Target
				}
				_ = linkRegistry.ClearLinksFrom(f.path)
				_ = linkRegistry.RegisterLinks(f.path, targets)
			}
		}
	}

	// 7. Second pass: resolve wiki-links.
	resolvedCount, _ := linkRegistry.ResolveAll()
	if resolvedCount > 0 && verbose {
		fmt.Printf("  Resolved %d wiki-links\n", resolvedCount)
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
