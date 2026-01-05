package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	initVaultPath   string
	initNotionToken string
	initDatabase    string
	initPage        string
)

// initCmd represents the init command.
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize sync configuration",
	Long: `Initialize a new obsidian-notion sync configuration.

This command creates the necessary configuration file and SQLite database
for tracking sync state between your Obsidian vault and Notion.

Example:
  obsidian-notion init \
    --vault ~/notes \
    --notion-token $NOTION_TOKEN \
    --database "Obsidian Notes"`,
	RunE: runInit,
}

func init() {
	initCmd.Flags().StringVar(&initVaultPath, "vault", "", "path to Obsidian vault (required)")
	initCmd.Flags().StringVar(&initNotionToken, "notion-token", "", "Notion API token (required)")
	initCmd.Flags().StringVar(&initDatabase, "database", "", "Notion database name or ID")
	initCmd.Flags().StringVar(&initPage, "page", "", "Notion parent page name or ID (alternative to database)")

	_ = initCmd.MarkFlagRequired("vault")
	_ = initCmd.MarkFlagRequired("notion-token")
}

func runInit(cmd *cobra.Command, args []string) error {
	// TODO: Implement init command
	// 1. Validate vault path exists
	// 2. Validate Notion token by making a test API call
	// 3. Resolve database or page ID
	// 4. Create config directory and file
	// 5. Initialize SQLite database with schema
	// 6. Perform initial vault scan to populate link registry

	fmt.Println("Initializing obsidian-notion sync...")
	fmt.Printf("  Vault: %s\n", initVaultPath)
	fmt.Printf("  Database: %s\n", initDatabase)

	return fmt.Errorf("init command not yet implemented")
}
