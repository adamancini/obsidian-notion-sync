package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jomei/notionapi"
	"github.com/spf13/cobra"

	"github.com/adamancini/obsidian-notion-sync/internal/config"
	"github.com/adamancini/obsidian-notion-sync/internal/notion"
	"github.com/adamancini/obsidian-notion-sync/internal/state"
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

	// 5. Initialize reverse transformer.
	xform := transformer.NewReverse(linkRegistry, &transformer.Config{
		UnresolvedLinkStyle: cfg.Transform.UnresolvedLinks,
		CalloutIcons:        cfg.Transform.Callouts,
		DataviewHandling:    cfg.Transform.Dataview,
		FlattenHeadings:     true,
	})

	// 6. Process each page.
	var created, updated, deleted, failed int

	for _, p := range pagesToPull {
		switch p.changeType {
		case pullChangeDeleted:
			// Handle deletion based on strategy.
			if err := handlePullDeletion(cfg, db, linkRegistry, p); err != nil {
				fmt.Fprintf(os.Stderr, "  Error deleting %s: %v\n", p.localPath, err)
				failed++
				continue
			}
			deleted++
			if verbose {
				fmt.Printf("  D %s (%s)\n", p.localPath, cfg.Sync.DeletionStrategy)
			}

		case pullChangeNew, pullChangeModified:
			// Fetch full page content from Notion.
			notionPage, err := client.FetchPage(ctx, p.notionPageID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  Error fetching %s: %v\n", p.localPath, err)
				failed++
				continue
			}

			// Transform to markdown.
			markdown, err := xform.NotionToMarkdown(notionPage)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  Error transforming %s: %v\n", p.localPath, err)
				failed++
				continue
			}

			// Ensure directory exists.
			fullPath := filepath.Join(cfg.Vault, p.localPath)
			if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
				fmt.Fprintf(os.Stderr, "  Error creating directory for %s: %v\n", p.localPath, err)
				failed++
				continue
			}

			// Write file.
			if err := os.WriteFile(fullPath, markdown, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "  Error writing %s: %v\n", p.localPath, err)
				failed++
				continue
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

			if err := db.SetState(syncState); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: failed to update state for %s: %v\n", p.localPath, err)
			}

			if p.changeType == pullChangeNew {
				created++
				if verbose {
					fmt.Printf("  + %s\n", p.localPath)
				}
			} else {
				updated++
				if verbose {
					fmt.Printf("  M %s\n", p.localPath)
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
