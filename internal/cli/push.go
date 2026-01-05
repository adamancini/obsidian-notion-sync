package cli

import (
	"fmt"

	"github.com/spf13/cobra"
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

	// TODO: Implement push command
	// 1. Initialize state database
	// 2. Scan vault for changes (or all files if --all)
	// 3. Filter by --path if specified
	// 4. Check for conflicts (abort if any unless --force)
	// 5. Parse each note with parser package
	// 6. Transform to Notion blocks with transformer package
	// 7. Create or update pages in Notion
	// 8. Update sync state
	// 9. Second pass: resolve wiki-links to page mentions

	fmt.Printf("Pushing changes from vault: %s\n", cfg.Vault)

	if pushDryRun {
		fmt.Println("(dry-run mode - no changes will be made)")
	}

	return fmt.Errorf("push command not yet implemented")
}
