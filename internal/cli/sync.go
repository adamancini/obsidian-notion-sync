package cli

import (
	"fmt"

	"github.com/spf13/cobra"
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

func runSync(cmd *cobra.Command, args []string) error {
	cfg := getConfig()

	strategy := ConflictStrategy(syncStrategy)
	switch strategy {
	case StrategyOurs, StrategyTheirs, StrategyManual, StrategyNewer:
		// Valid strategy
	default:
		return fmt.Errorf("invalid conflict strategy: %s", syncStrategy)
	}

	// TODO: Implement sync command
	// 1. Detect changes on both sides
	// 2. Identify conflicts (both sides modified since last sync)
	// 3. Handle conflicts according to strategy
	// 4. Push local changes
	// 5. Pull remote changes
	// 6. Update sync state

	fmt.Printf("Syncing vault: %s\n", cfg.Vault)
	fmt.Printf("Conflict strategy: %s\n", strategy)

	if syncDryRun {
		fmt.Println("(dry-run mode - no changes will be made)")
	}

	return fmt.Errorf("sync command not yet implemented")
}
