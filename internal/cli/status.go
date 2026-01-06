package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/adamancini/obsidian-notion-sync/internal/state"
)

var (
	statusShowAll bool
)

// statusCmd represents the status command.
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show sync status",
	Long: `Show the current sync status between Obsidian and Notion.

Displays counts of:
  - New files (to push)
  - Modified files (local changes to push)
  - Modified files (remote changes to pull)
  - Conflicts (both sides modified)
  - Synced files (up to date)

Example output:
  New (push):       3 notes
  Modified (push):  5 notes
  Modified (pull):  2 notes
  Conflicts:        1 note
  Synced:         152 notes`,
	RunE: runStatus,
}

func init() {
	statusCmd.Flags().BoolVarP(&statusShowAll, "all", "a", false, "show all files, not just summary")
}

func runStatus(cmd *cobra.Command, args []string) error {
	cfg, err := getConfig()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Open state database.
	dbPath := filepath.Join(cfg.Vault, ".obsidian-notion.db")
	db, err := state.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w (run 'obsidian-notion init' first)", err)
	}
	defer db.Close()

	// Get change detector.
	detector := state.NewChangeDetector(db, cfg.Vault)

	// Detect local changes.
	changes, err := detector.DetectChanges(ctx)
	if err != nil {
		return fmt.Errorf("detect changes: %w", err)
	}

	// Categorize changes.
	var newFiles, modifiedPush, modifiedPull, renamedFiles, deletedFiles, conflicts []state.Change

	for _, c := range changes {
		switch c.Type {
		case state.ChangeCreated:
			newFiles = append(newFiles, c)
		case state.ChangeModified:
			if c.Direction == state.DirectionPush {
				modifiedPush = append(modifiedPush, c)
			} else {
				modifiedPull = append(modifiedPull, c)
			}
		case state.ChangeRenamed:
			renamedFiles = append(renamedFiles, c)
		case state.ChangeDeleted:
			deletedFiles = append(deletedFiles, c)
		case state.ChangeConflict:
			conflicts = append(conflicts, c)
		}
	}

	// Count synced files.
	syncedStates, err := db.ListStates("synced")
	if err != nil {
		return fmt.Errorf("list synced: %w", err)
	}

	// Count pending files (initialized but never synced).
	pendingStates, err := db.ListStates("pending")
	if err != nil {
		return fmt.Errorf("list pending: %w", err)
	}

	// Get link registry stats.
	linkRegistry := state.NewLinkRegistry(db)
	unresolvedLinks, err := linkRegistry.GetUnresolvedLinks()
	if err != nil {
		return fmt.Errorf("get unresolved links: %w", err)
	}

	// Print summary.
	fmt.Printf("Sync status for: %s\n\n", cfg.Vault)

	printStatusLine("New (push)", len(newFiles)+len(pendingStates))
	printStatusLine("Modified (push)", len(modifiedPush))
	printStatusLine("Modified (pull)", len(modifiedPull))
	printStatusLine("Renamed", len(renamedFiles))
	printStatusLine("Deleted", len(deletedFiles))
	printStatusLine("Conflicts", len(conflicts))
	printStatusLine("Synced", len(syncedStates))
	fmt.Println()
	printStatusLine("Unresolved links", len(unresolvedLinks))

	// Show details if requested.
	if statusShowAll {
		if len(newFiles) > 0 || len(pendingStates) > 0 {
			fmt.Println("\nNew files (to push):")
			for _, s := range pendingStates {
				fmt.Printf("  + %s\n", s.ObsidianPath)
			}
			for _, c := range newFiles {
				fmt.Printf("  + %s\n", c.Path)
			}
		}

		if len(modifiedPush) > 0 {
			fmt.Println("\nModified (to push):")
			for _, c := range modifiedPush {
				fmt.Printf("  M %s\n", c.Path)
			}
		}

		if len(modifiedPull) > 0 {
			fmt.Println("\nModified (to pull):")
			for _, c := range modifiedPull {
				fmt.Printf("  M %s\n", c.Path)
			}
		}

		if len(renamedFiles) > 0 {
			fmt.Println("\nRenamed files:")
			for _, c := range renamedFiles {
				fmt.Printf("  R %s -> %s\n", c.OldPath, c.Path)
			}
		}

		if len(deletedFiles) > 0 {
			fmt.Println("\nDeleted files:")
			for _, c := range deletedFiles {
				fmt.Printf("  D %s\n", c.Path)
			}
		}

		if len(conflicts) > 0 {
			fmt.Println("\nConflicts (both modified):")
			for _, c := range conflicts {
				fmt.Printf("  ! %s\n", c.Path)
			}
		}

		if len(unresolvedLinks) > 0 && verbose {
			fmt.Println("\nUnresolved wiki-links:")
			for _, l := range unresolvedLinks {
				fmt.Printf("  ? [[%s]] in %s\n", l.TargetName, l.SourcePath)
			}
		}
	}

	return nil
}

// printStatusLine prints a formatted status line with count.
func printStatusLine(label string, count int) {
	note := "notes"
	if count == 1 {
		note = "note"
	}
	fmt.Printf("  %-18s %4d %s\n", label+":", count, note)
}
