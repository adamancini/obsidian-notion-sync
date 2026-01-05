package cli

import (
	"fmt"

	"github.com/spf13/cobra"
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
	cfg := getConfig()

	// TODO: Implement status command
	// 1. Initialize state database
	// 2. Scan vault for current state
	// 3. Query Notion for modification times
	// 4. Compare with stored sync state
	// 5. Categorize files: new, modified (push), modified (pull), conflict, synced
	// 6. Display summary or full list

	fmt.Printf("Checking sync status for vault: %s\n", cfg.Vault)
	fmt.Println()

	// Placeholder output
	fmt.Println("  New (push):       ? notes")
	fmt.Println("  Modified (push):  ? notes")
	fmt.Println("  Modified (pull):  ? notes")
	fmt.Println("  Conflicts:        ? notes")
	fmt.Println("  Synced:           ? notes")

	return fmt.Errorf("status command not yet implemented")
}
