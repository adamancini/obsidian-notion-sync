package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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

// ConflictStrategy defines how to handle sync conflicts.
type ConflictStrategy string

const (
	// StrategyOurs keeps the local (Obsidian) version.
	StrategyOurs ConflictStrategy = "ours"
	// StrategyTheirs keeps the remote (Notion) version.
	StrategyTheirs ConflictStrategy = "theirs"
	// StrategyManual requires manual resolution.
	StrategyManual ConflictStrategy = "manual"
	// StrategyNewer keeps whichever version is newer.
	StrategyNewer ConflictStrategy = "newer"
)

var (
	syncStrategy string
	syncDryRun   bool
)

// syncCmd represents the sync command.
var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Bidirectional sync between Obsidian and Notion",
	Long: `Perform a bidirectional sync between Obsidian and Notion.

This command first pushes local changes, then pulls remote changes,
handling conflicts according to the specified strategy.

Conflict strategies:
  ours    - Keep local (Obsidian) version
  theirs  - Keep remote (Notion) version
  manual  - Stop and require manual resolution
  newer   - Keep whichever version is newer

Examples:
  obsidian-notion sync                     # Sync with manual conflict resolution
  obsidian-notion sync --strategy ours     # Always keep local version
  obsidian-notion sync --strategy newer    # Keep newer version`,
	RunE: runSync,
}

func init() {
	syncCmd.Flags().StringVar(&syncStrategy, "strategy", "manual", "conflict resolution strategy (ours|theirs|manual|newer)")
	syncCmd.Flags().BoolVar(&syncDryRun, "dry-run", false, "show what would be synced without making changes")
}

// syncResult holds the results of a sync operation.
type syncResult struct {
	Pushed        int
	Pulled        int
	Conflicts     int
	ConflictPaths []string
	Failed        int
}

func runSync(cmd *cobra.Command, args []string) error {
	cfg, err := getConfig()
	if err != nil {
		return err
	}

	strategy := ConflictStrategy(syncStrategy)
	switch strategy {
	case StrategyOurs, StrategyTheirs, StrategyManual, StrategyNewer:
		// Valid strategy
	default:
		return fmt.Errorf("invalid conflict strategy: %s", syncStrategy)
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

	linkRegistry := state.NewLinkRegistry(db)
	conflictTracker := state.NewConflictTracker(db)

	fmt.Printf("Syncing vault: %s\n", cfg.Vault)
	fmt.Printf("Conflict strategy: %s\n", strategy)

	if syncDryRun {
		fmt.Println("(dry-run mode - no changes will be made)")
	}
	fmt.Println()

	// 3. Detect local changes.
	detector := state.NewChangeDetector(db, cfg.Vault)
	localChanges, err := detector.DetectChanges(ctx)
	if err != nil {
		return fmt.Errorf("detect local changes: %w", err)
	}

	// 4. Detect remote changes.
	remoteChanges, err := detector.DetectRemoteChanges(ctx, func(pageID string) (string, time.Time, error) {
		page, err := client.GetPage(ctx, pageID)
		if err != nil {
			return "", time.Time{}, err
		}
		// Use last_edited_time as a proxy for remote change detection.
		// A proper implementation would compare content hashes.
		return "", page.LastEditedTime, nil
	})
	if err != nil {
		return fmt.Errorf("detect remote changes: %w", err)
	}

	// 5. Categorize changes.
	var (
		pushChanges   []state.Change
		pullChanges   []state.Change
		conflicts     []state.Change
	)

	// Process local changes.
	for _, c := range localChanges {
		if c.Type == state.ChangeConflict {
			conflicts = append(conflicts, c)
		} else if c.Direction == state.DirectionPush {
			pushChanges = append(pushChanges, c)
		}
	}

	// Process remote changes.
	for _, c := range remoteChanges {
		if c.Type == state.ChangeConflict {
			// Check if already in conflicts list.
			found := false
			for _, existing := range conflicts {
				if existing.Path == c.Path {
					found = true
					break
				}
			}
			if !found {
				conflicts = append(conflicts, c)
			}
		} else if c.Direction == state.DirectionPull {
			pullChanges = append(pullChanges, c)
		}
	}

	// 6. Handle conflicts based on strategy.
	if len(conflicts) > 0 {
		switch strategy {
		case StrategyManual:
			// Record conflicts and stop.
			fmt.Printf("Found %d conflict(s). Resolve manually with 'obsidian-notion conflicts'.\n\n", len(conflicts))
			for _, c := range conflicts {
				fmt.Printf("  ! %s\n", c.Path)
				// Record conflict in database.
				info := &state.ConflictInfo{
					Path:        c.Path,
					LocalHash:   c.LocalHash,
					RemoteHash:  c.RemoteHash,
					LocalMtime:  c.LocalMtime,
					RemoteMtime: c.RemoteMtime,
					DetectedAt:  time.Now(),
				}
				_ = conflictTracker.RecordConflict(info)
			}
			return fmt.Errorf("sync aborted: %d unresolved conflict(s)", len(conflicts))

		case StrategyOurs:
			// Convert conflicts to push operations.
			for _, c := range conflicts {
				c.Direction = state.DirectionPush
				c.Type = state.ChangeModified
				pushChanges = append(pushChanges, c)
			}
			fmt.Printf("Resolving %d conflict(s) with 'ours' strategy (keeping local)\n\n", len(conflicts))

		case StrategyTheirs:
			// Convert conflicts to pull operations.
			for _, c := range conflicts {
				c.Direction = state.DirectionPull
				c.Type = state.ChangeModified
				pullChanges = append(pullChanges, c)
			}
			fmt.Printf("Resolving %d conflict(s) with 'theirs' strategy (keeping remote)\n\n", len(conflicts))

		case StrategyNewer:
			// Compare timestamps and choose newer.
			for _, c := range conflicts {
				if c.LocalMtime.After(c.RemoteMtime) {
					c.Direction = state.DirectionPush
					c.Type = state.ChangeModified
					pushChanges = append(pushChanges, c)
				} else {
					c.Direction = state.DirectionPull
					c.Type = state.ChangeModified
					pullChanges = append(pullChanges, c)
				}
			}
			fmt.Printf("Resolving %d conflict(s) with 'newer' strategy\n\n", len(conflicts))
		}
	}

	// 7. Show dry-run summary and exit if dry-run.
	if syncDryRun {
		fmt.Printf("Would push: %d change(s)\n", len(pushChanges))
		for _, c := range pushChanges {
			fmt.Printf("  -> %s (%s)\n", c.Path, c.Type)
		}
		fmt.Printf("\nWould pull: %d change(s)\n", len(pullChanges))
		for _, c := range pullChanges {
			fmt.Printf("  <- %s (%s)\n", c.Path, c.Type)
		}
		return nil
	}

	// 8. Execute push operations.
	var pushed, failed int32
	if len(pushChanges) > 0 {
		fmt.Printf("Pushing %d change(s)...\n", len(pushChanges))

		workers := cfg.RateLimit.Workers
		if workers < 1 {
			workers = 4
		}
		pool := osync.NewWorkerPool(workers)
		progress := osync.NewProgress(len(pushChanges), os.Stdout)
		progress.SetEnabled(!verbose)

		pushCtx := &syncPushContext{
			cfg:          cfg,
			db:           db,
			client:       client,
			linkRegistry: linkRegistry,
			parser:       parser.New(),
			scanner:      vault.NewScanner(cfg.Vault, cfg.Sync.Ignore),
		}

		results := osync.ProcessWithProgress(ctx, pool, pushChanges, pushCtx.processChange, progress.SimpleCallback())
		progress.Finish()

		for _, result := range results {
			if result.Err != nil {
				fmt.Fprintf(os.Stderr, "  Error pushing %s: %v\n", result.Input.Path, result.Err)
				atomic.AddInt32(&failed, 1)
			} else {
				atomic.AddInt32(&pushed, 1)
				if verbose {
					fmt.Printf("  -> %s\n", result.Input.Path)
				}
			}
		}
	}

	// 9. Execute pull operations.
	var pulled int32
	if len(pullChanges) > 0 {
		fmt.Printf("Pulling %d change(s)...\n", len(pullChanges))

		workers := cfg.RateLimit.Workers
		if workers < 1 {
			workers = 4
		}
		pool := osync.NewWorkerPool(workers)
		progress := osync.NewProgress(len(pullChanges), os.Stdout)
		progress.SetEnabled(!verbose)

		pullCtx := &syncPullContext{
			cfg:          cfg,
			db:           db,
			client:       client,
			linkRegistry: linkRegistry,
		}

		results := osync.ProcessWithProgress(ctx, pool, pullChanges, pullCtx.processChange, progress.SimpleCallback())
		progress.Finish()

		for _, result := range results {
			if result.Err != nil {
				fmt.Fprintf(os.Stderr, "  Error pulling %s: %v\n", result.Input.Path, result.Err)
				atomic.AddInt32(&failed, 1)
			} else {
				atomic.AddInt32(&pulled, 1)
				if verbose {
					fmt.Printf("  <- %s\n", result.Input.Path)
				}
			}
		}
	}

	// 10. Print summary.
	fmt.Println()
	fmt.Println("Sync complete:")
	fmt.Printf("  Pushed:    %d\n", pushed)
	fmt.Printf("  Pulled:    %d\n", pulled)
	if strategy == StrategyManual && len(conflicts) > 0 {
		fmt.Printf("  Conflicts: %d (manual resolution required)\n", len(conflicts))
	}
	if failed > 0 {
		fmt.Printf("  Failed:    %d\n", failed)
	}

	return nil
}

// syncPushContext holds shared dependencies for push operations.
type syncPushContext struct {
	cfg          *config.Config
	db           *state.DB
	client       *notion.Client
	linkRegistry *state.LinkRegistry
	parser       *parser.Parser
	scanner      *vault.Scanner
}

// processChange processes a single change for push.
func (pc *syncPushContext) processChange(ctx context.Context, c state.Change) (struct{}, error) {
	// Handle deletions.
	if c.Type == state.ChangeDeleted {
		if c.State != nil && c.State.NotionPageID != "" {
			strategy := pc.cfg.Sync.DeletionStrategy
			if strategy == "" {
				strategy = "archive"
			}
			switch strategy {
			case "archive":
				if err := pc.client.ArchivePage(ctx, c.State.NotionPageID); err != nil {
					return struct{}{}, fmt.Errorf("archive page: %w", err)
				}
			case "delete":
				if err := pc.client.DeletePage(ctx, c.State.NotionPageID); err != nil {
					return struct{}{}, fmt.Errorf("delete page: %w", err)
				}
			}
		}
		_ = pc.db.DeleteState(c.Path)
		_ = pc.linkRegistry.ClearLinksFrom(c.Path)
		return struct{}{}, nil
	}

	// Handle renames.
	if c.Type == state.ChangeRenamed {
		if c.State != nil && c.State.NotionPageID != "" {
			basename := filepath.Base(c.Path)
			newTitle := basename[:len(basename)-len(filepath.Ext(basename))]
			if err := pc.client.UpdatePageTitle(ctx, c.State.NotionPageID, newTitle); err != nil {
				return struct{}{}, fmt.Errorf("update page title: %w", err)
			}
		}
		_ = pc.db.UpdatePath(c.OldPath, c.Path)
		_ = pc.linkRegistry.UpdateSourcePath(c.OldPath, c.Path)
		return struct{}{}, nil
	}

	// Handle creates and modifications.
	fullPath := filepath.Join(pc.cfg.Vault, c.Path)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return struct{}{}, fmt.Errorf("read file: %w", err)
	}

	note, err := pc.parser.Parse(c.Path, content)
	if err != nil {
		return struct{}{}, fmt.Errorf("parse markdown: %w", err)
	}

	// Register wiki-links.
	_ = pc.linkRegistry.ClearLinksFrom(c.Path)
	if len(note.WikiLinks) > 0 {
		targets := make([]string, len(note.WikiLinks))
		for i, link := range note.WikiLinks {
			targets[i] = link.Target
		}
		_ = pc.linkRegistry.RegisterLinks(c.Path, targets)
	}

	// Create transformer with path-specific property mappings.
	t := transformer.New(pc.linkRegistry, buildTransformerConfig(pc.cfg, c.Path))
	notionPage, err := t.Transform(note)
	if err != nil {
		return struct{}{}, fmt.Errorf("transform to Notion: %w", err)
	}

	var pageID string
	if c.State == nil || c.State.NotionPageID == "" {
		// Create new page.
		parentID := pc.cfg.GetDatabaseForPath(c.Path)
		if parentID == "" {
			parentID = pc.cfg.Notion.DefaultPage
		}
		result, err := pc.client.CreatePage(ctx, parentID, notionPage)
		if err != nil {
			return struct{}{}, fmt.Errorf("create page: %w", err)
		}
		pageID = result.PageID
	} else {
		// Update existing page.
		pageID = c.State.NotionPageID
		if err := pc.client.UpdatePage(ctx, pageID, notionPage); err != nil {
			return struct{}{}, fmt.Errorf("update page: %w", err)
		}
	}

	// Update sync state.
	hashes, _ := state.HashFileDetailed(fullPath)
	fileInfo, _ := os.Stat(fullPath)
	var mtime time.Time
	if fileInfo != nil {
		mtime = fileInfo.ModTime()
	}

	syncState := &state.SyncState{
		ObsidianPath:    c.Path,
		NotionPageID:    pageID,
		ObsidianMtime:   mtime,
		NotionMtime:     time.Now(),
		ContentHash:     hashes.ContentHash,
		FrontmatterHash: hashes.FrontmatterHash,
		LastSync:        time.Now(),
		SyncDirection:   "push",
		Status:          "synced",
	}
	if err := pc.db.SetState(syncState); err != nil {
		return struct{}{}, fmt.Errorf("update state: %w", err)
	}

	return struct{}{}, nil
}

// syncPullContext holds shared dependencies for pull operations.
type syncPullContext struct {
	cfg          *config.Config
	db           *state.DB
	client       *notion.Client
	linkRegistry *state.LinkRegistry
}

// processChange processes a single change for pull.
func (pc *syncPullContext) processChange(ctx context.Context, c state.Change) (struct{}, error) {
	if c.State == nil || c.State.NotionPageID == "" {
		return struct{}{}, fmt.Errorf("no notion page ID for path: %s", c.Path)
	}

	// Fetch page from Notion.
	notionPage, err := pc.client.FetchPage(ctx, c.State.NotionPageID)
	if err != nil {
		return struct{}{}, fmt.Errorf("fetch page: %w", err)
	}

	// Create reverse transformer with path-specific property mappings.
	rt := transformer.NewReverse(pc.linkRegistry, buildTransformerConfig(pc.cfg, c.Path))

	// Transform to markdown.
	markdown, err := rt.NotionToMarkdown(notionPage)
	if err != nil {
		return struct{}{}, fmt.Errorf("transform to markdown: %w", err)
	}

	// Ensure directory exists.
	fullPath := filepath.Join(pc.cfg.Vault, c.Path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return struct{}{}, fmt.Errorf("create directory: %w", err)
	}

	// Write file.
	if err := os.WriteFile(fullPath, markdown, 0644); err != nil {
		return struct{}{}, fmt.Errorf("write file: %w", err)
	}

	// Update sync state.
	hashes, _ := state.HashFileDetailed(fullPath)
	fileInfo, _ := os.Stat(fullPath)
	var mtime time.Time
	if fileInfo != nil {
		mtime = fileInfo.ModTime()
	}

	syncState := c.State
	syncState.ContentHash = hashes.ContentHash
	syncState.FrontmatterHash = hashes.FrontmatterHash
	syncState.ObsidianMtime = mtime
	syncState.NotionMtime = c.RemoteMtime
	syncState.LastSync = time.Now()
	syncState.Status = "synced"
	syncState.SyncDirection = "pull"

	if err := pc.db.SetState(syncState); err != nil {
		return struct{}{}, fmt.Errorf("update state: %w", err)
	}

	return struct{}{}, nil
}
