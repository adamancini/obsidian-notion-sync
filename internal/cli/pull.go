package cli

import (
	"fmt"

	"github.com/spf13/cobra"
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
	Short: "Pull remote changes from Notion",
	Long: `Pull changes from Notion back to Obsidian.

This command fetches pages that have been modified in Notion and
converts them back to Obsidian-flavored markdown.

Examples:
  obsidian-notion pull                    # Pull all changed files
  obsidian-notion pull --all              # Pull all files
  obsidian-notion pull --path "work/**"   # Pull files matching pattern
  obsidian-notion pull --dry-run          # Show what would be pulled`,
	RunE: runPull,
}

func init() {
	pullCmd.Flags().BoolVar(&pullAll, "all", false, "pull all files, not just changed ones")
	pullCmd.Flags().StringVar(&pullPath, "path", "", "glob pattern to filter files")
	pullCmd.Flags().BoolVar(&pullDryRun, "dry-run", false, "show what would be pulled without making changes")
	pullCmd.Flags().BoolVar(&pullForce, "force", false, "force pull even if there are conflicts")
}

func runPull(cmd *cobra.Command, args []string) error {
	cfg := getConfig()

	// TODO: Implement pull command
	// 1. Initialize state database
	// 2. Query Notion for pages modified since last sync
	// 3. Filter by --path if specified
	// 4. Check for conflicts (abort if any unless --force)
	// 5. Fetch each page with notion client
	// 6. Transform blocks to markdown with reverse transformer
	// 7. Write files to vault
	// 8. Update sync state

	fmt.Printf("Pulling changes to vault: %s\n", cfg.Vault)

	if pullDryRun {
		fmt.Println("(dry-run mode - no changes will be made)")
	}

	return fmt.Errorf("pull command not yet implemented")
}
