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
)

var (
	resolveKeep   string
	conflictsJson bool
)

// conflictsCmd represents the conflicts command.
var conflictsCmd = &cobra.Command{
	Use:   "conflicts",
	Short: "List and resolve sync conflicts",
	Long: `List all current sync conflicts and optionally resolve them.

A conflict occurs when both the local Obsidian file and the remote
Notion page have been modified since the last sync.

Examples:
  obsidian-notion conflicts                              # List all conflicts
  obsidian-notion conflicts resolve path/to/note.md --keep local
  obsidian-notion conflicts resolve path/to/note.md --keep remote
  obsidian-notion conflicts resolve path/to/note.md --keep both`,
	RunE: runConflicts,
}

// resolveCmd represents the resolve subcommand.
var resolveCmd = &cobra.Command{
	Use:   "resolve <path>",
	Short: "Resolve a specific conflict",
	Long: `Resolve a specific sync conflict by keeping one version.

Options for --keep:
  local   - Keep the Obsidian version, overwrite Notion
  remote  - Keep the Notion version, overwrite Obsidian
  both    - Keep both versions (create .conflict file)`,
	Args: cobra.ExactArgs(1),
	RunE: runResolve,
}

func init() {
	resolveCmd.Flags().StringVar(&resolveKeep, "keep", "", "which version to keep (local|remote|both)")
	_ = resolveCmd.MarkFlagRequired("keep")

	conflictsCmd.Flags().BoolVar(&conflictsJson, "json", false, "output in JSON format")
	conflictsCmd.AddCommand(resolveCmd)
}

func runConflicts(cmd *cobra.Command, args []string) error {
	cfg, err := getConfig()
	if err != nil {
		return err
	}

	// Open state database.
	dbPath := filepath.Join(cfg.Vault, ".obsidian-notion.db")
	db, err := state.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w (run 'obsidian-notion init' first)", err)
	}
	defer db.Close()

	// Get all conflicts.
	tracker := state.NewConflictTracker(db)
	conflicts, err := tracker.GetConflicts()
	if err != nil {
		return fmt.Errorf("get conflicts: %w", err)
	}

	if len(conflicts) == 0 {
		fmt.Println("No conflicts found.")
		return nil
	}

	fmt.Printf("Found %d conflict(s):\n\n", len(conflicts))

	for _, c := range conflicts {
		// Try to get detailed conflict info from history.
		info, _ := tracker.GetConflictInfo(c.ObsidianPath)

		fmt.Printf("  %s\n", c.ObsidianPath)
		fmt.Printf("    Notion page: %s\n", c.NotionPageID)

		if info != nil {
			fmt.Printf("    Local modified:  %s\n", info.LocalMtime.Format(time.RFC3339))
			fmt.Printf("    Remote modified: %s\n", info.RemoteMtime.Format(time.RFC3339))
			fmt.Printf("    Detected:        %s\n", info.DetectedAt.Format(time.RFC3339))
		} else {
			fmt.Printf("    Last sync:       %s\n", c.LastSync.Format(time.RFC3339))
		}
		fmt.Println()
	}

	fmt.Println("To resolve a conflict:")
	fmt.Println("  obsidian-notion conflicts resolve <path> --keep local   # Keep Obsidian version")
	fmt.Println("  obsidian-notion conflicts resolve <path> --keep remote  # Keep Notion version")
	fmt.Println("  obsidian-notion conflicts resolve <path> --keep both    # Keep both (creates .conflict file)")

	return nil
}

func runResolve(cmd *cobra.Command, args []string) error {
	cfg, err := getConfig()
	if err != nil {
		return err
	}
	path := args[0]

	switch resolveKeep {
	case "local", "remote", "both":
		// Valid option
	default:
		return fmt.Errorf("invalid --keep value: %s (must be local, remote, or both)", resolveKeep)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Open state database.
	dbPath := filepath.Join(cfg.Vault, ".obsidian-notion.db")
	db, err := state.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w (run 'obsidian-notion init' first)", err)
	}
	defer db.Close()

	// Verify file is in conflict state.
	syncState, err := db.GetState(path)
	if err != nil {
		return fmt.Errorf("get state: %w", err)
	}
	if syncState == nil {
		return fmt.Errorf("no sync state for path: %s", path)
	}
	if syncState.Status != "conflict" {
		return fmt.Errorf("file is not in conflict state: %s (status: %s)", path, syncState.Status)
	}

	// Initialize Notion client.
	client := notion.New(cfg.Notion.Token,
		notion.WithRateLimit(cfg.RateLimit.RequestsPerSecond),
		notion.WithBatchSize(cfg.RateLimit.BatchSize),
	)

	tracker := state.NewConflictTracker(db)
	linkRegistry := state.NewLinkRegistry(db)

	var newHash string

	switch resolveKeep {
	case "local":
		// Push local version to Notion.
		newHash, err = resolveKeepLocal(ctx, cfg, db, client, linkRegistry, path, syncState)
		if err != nil {
			return fmt.Errorf("resolve keep local: %w", err)
		}
		fmt.Printf("Resolved conflict for %s: kept local version (pushed to Notion)\n", path)

	case "remote":
		// Pull remote version to local.
		newHash, err = resolveKeepRemote(ctx, cfg, db, client, linkRegistry, path, syncState)
		if err != nil {
			return fmt.Errorf("resolve keep remote: %w", err)
		}
		fmt.Printf("Resolved conflict for %s: kept remote version (pulled from Notion)\n", path)

	case "both":
		// Keep both versions.
		newHash, err = resolveKeepBoth(ctx, cfg, db, client, linkRegistry, path, syncState)
		if err != nil {
			return fmt.Errorf("resolve keep both: %w", err)
		}
		conflictPath := strings.TrimSuffix(path, ".md") + ".conflict.md"
		fmt.Printf("Resolved conflict for %s: kept both versions\n", path)
		fmt.Printf("  Local version: %s\n", path)
		fmt.Printf("  Remote version: %s\n", conflictPath)
	}

	// Mark conflict as resolved.
	if err := tracker.ResolveConflict(path, resolveKeep, newHash); err != nil {
		return fmt.Errorf("mark resolved: %w", err)
	}

	return nil
}

// resolveKeepLocal pushes the local version to Notion.
func resolveKeepLocal(ctx context.Context, cfg *config.Config, db *state.DB, client *notion.Client, linkRegistry *state.LinkRegistry, path string, syncState *state.SyncState) (string, error) {
	// Read local file.
	fullPath := filepath.Join(cfg.Vault, path)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}

	// Parse and transform.
	p := parser.New()
	note, err := p.Parse(path, content)
	if err != nil {
		return "", fmt.Errorf("parse markdown: %w", err)
	}

	t := transformer.New(linkRegistry, &transformer.Config{
		UnresolvedLinkStyle: cfg.Transform.UnresolvedLinks,
		CalloutIcons:        cfg.Transform.Callouts,
		DataviewHandling:    cfg.Transform.Dataview,
		FlattenHeadings:     true,
	})

	notionPage, err := t.Transform(note)
	if err != nil {
		return "", fmt.Errorf("transform: %w", err)
	}

	// Update Notion page.
	if err := client.UpdatePage(ctx, syncState.NotionPageID, notionPage); err != nil {
		return "", fmt.Errorf("update page: %w", err)
	}

	// Compute new hash.
	hashes, err := state.HashFileDetailed(fullPath)
	if err != nil {
		return "", fmt.Errorf("hash file: %w", err)
	}

	return hashes.FullHash, nil
}

// resolveKeepRemote pulls the remote version from Notion.
func resolveKeepRemote(ctx context.Context, cfg *config.Config, db *state.DB, client *notion.Client, linkRegistry *state.LinkRegistry, path string, syncState *state.SyncState) (string, error) {
	// Fetch page from Notion.
	notionPage, err := client.FetchPage(ctx, syncState.NotionPageID)
	if err != nil {
		return "", fmt.Errorf("fetch page: %w", err)
	}

	// Transform to markdown.
	rt := transformer.NewReverse(linkRegistry, &transformer.Config{
		UnresolvedLinkStyle: cfg.Transform.UnresolvedLinks,
		CalloutIcons:        cfg.Transform.Callouts,
		DataviewHandling:    cfg.Transform.Dataview,
		FlattenHeadings:     true,
	})

	markdown, err := rt.NotionToMarkdown(notionPage)
	if err != nil {
		return "", fmt.Errorf("transform to markdown: %w", err)
	}

	// Write to local file.
	fullPath := filepath.Join(cfg.Vault, path)
	if err := os.WriteFile(fullPath, markdown, 0644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	// Compute new hash.
	hashes, err := state.HashFileDetailed(fullPath)
	if err != nil {
		return "", fmt.Errorf("hash file: %w", err)
	}

	return hashes.FullHash, nil
}

// resolveKeepBoth keeps local version and saves remote to a .conflict file.
func resolveKeepBoth(ctx context.Context, cfg *config.Config, db *state.DB, client *notion.Client, linkRegistry *state.LinkRegistry, path string, syncState *state.SyncState) (string, error) {
	// Fetch page from Notion.
	notionPage, err := client.FetchPage(ctx, syncState.NotionPageID)
	if err != nil {
		return "", fmt.Errorf("fetch page: %w", err)
	}

	// Transform to markdown.
	rt := transformer.NewReverse(linkRegistry, &transformer.Config{
		UnresolvedLinkStyle: cfg.Transform.UnresolvedLinks,
		CalloutIcons:        cfg.Transform.Callouts,
		DataviewHandling:    cfg.Transform.Dataview,
		FlattenHeadings:     true,
	})

	markdown, err := rt.NotionToMarkdown(notionPage)
	if err != nil {
		return "", fmt.Errorf("transform to markdown: %w", err)
	}

	// Write remote version to .conflict file.
	conflictPath := strings.TrimSuffix(path, ".md") + ".conflict.md"
	fullConflictPath := filepath.Join(cfg.Vault, conflictPath)

	// Add header to conflict file explaining its origin.
	header := fmt.Sprintf("---\nconflict_source: notion\nconflict_page_id: %s\nconflict_date: %s\noriginal_file: %s\n---\n\n",
		syncState.NotionPageID,
		time.Now().Format(time.RFC3339),
		path,
	)

	conflictContent := []byte(header + string(markdown))
	if err := os.WriteFile(fullConflictPath, conflictContent, 0644); err != nil {
		return "", fmt.Errorf("write conflict file: %w", err)
	}

	// Keep local version, compute its hash.
	fullPath := filepath.Join(cfg.Vault, path)
	hashes, err := state.HashFileDetailed(fullPath)
	if err != nil {
		return "", fmt.Errorf("hash file: %w", err)
	}

	return hashes.FullHash, nil
}
