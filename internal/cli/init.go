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
	"github.com/adamancini/obsidian-notion-sync/internal/parser"
	"github.com/adamancini/obsidian-notion-sync/internal/state"
	"github.com/adamancini/obsidian-notion-sync/internal/vault"
)

var (
	initVaultPath   string
	initNotionToken string
	initDatabase    string
	initPage        string
	initConfigPath  string
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
	initCmd.Flags().StringVar(&initConfigPath, "config-path", "", "path to write config file (default: vault/.obsidian-notion.yaml)")

	_ = initCmd.MarkFlagRequired("vault")
	_ = initCmd.MarkFlagRequired("notion-token")
}

func runInit(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	fmt.Println("Initializing obsidian-notion sync...")

	// 1. Validate and expand vault path.
	vaultPath, err := expandAndValidateVaultPath(initVaultPath)
	if err != nil {
		return err
	}
	fmt.Printf("  ✓ Vault path: %s\n", vaultPath)

	// 2. Validate Notion token by making a test API call.
	client := notion.New(initNotionToken)
	if err := validateNotionToken(ctx, client); err != nil {
		return fmt.Errorf("invalid Notion token: %w", err)
	}
	fmt.Println("  ✓ Notion token validated")

	// 3. Resolve database or page ID.
	var databaseID, pageID string
	if initDatabase != "" {
		databaseID, err = resolveDatabase(ctx, client, initDatabase)
		if err != nil {
			return fmt.Errorf("resolve database: %w", err)
		}
		fmt.Printf("  ✓ Database: %s\n", databaseID)
	} else if initPage != "" {
		pageID = initPage // For now, accept page ID directly
		fmt.Printf("  ✓ Parent page: %s\n", pageID)
	} else {
		return fmt.Errorf("either --database or --page is required")
	}

	// 4. Determine config file path.
	configPath := initConfigPath
	if configPath == "" {
		configPath = filepath.Join(vaultPath, ".obsidian-notion.yaml")
	}

	// Check if config already exists.
	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("config file already exists: %s (use --config-path to specify different location)", configPath)
	}

	// 5. Create configuration.
	newCfg := config.DefaultConfig()
	newCfg.Vault = vaultPath
	newCfg.Notion.Token = "${NOTION_TOKEN}" // Reference env var instead of storing token
	newCfg.Notion.DefaultDatabase = databaseID
	newCfg.Notion.DefaultPage = pageID

	if err := newCfg.Save(configPath); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	fmt.Printf("  ✓ Config file: %s\n", configPath)

	// 6. Initialize SQLite database.
	dbPath := filepath.Join(vaultPath, ".obsidian-notion.db")
	db, err := state.Open(dbPath)
	if err != nil {
		return fmt.Errorf("init database: %w", err)
	}
	defer db.Close()

	// Store config path in database for reference.
	if err := db.SetConfig("config_path", configPath); err != nil {
		return fmt.Errorf("store config path: %w", err)
	}
	if err := db.SetConfig("vault_path", vaultPath); err != nil {
		return fmt.Errorf("store vault path: %w", err)
	}
	if err := db.SetConfig("initialized_at", time.Now().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("store init time: %w", err)
	}

	fmt.Printf("  ✓ Database: %s\n", dbPath)

	// 7. Scan vault and populate initial state.
	fmt.Println("\nScanning vault...")
	scanner := vault.NewScanner(vaultPath, newCfg.Sync.Ignore)
	files, err := scanner.Scan(ctx)
	if err != nil {
		return fmt.Errorf("scan vault: %w", err)
	}

	// Parse each file to extract wiki-links and register them.
	p := parser.New()
	linkRegistry := state.NewLinkRegistry(db)
	var totalLinks int

	for _, file := range files {
		content, err := scanner.ReadFile(file.Path)
		if err != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "  Warning: could not read %s: %v\n", file.Path, err)
			}
			continue
		}

		note, err := p.Parse(file.Path, content)
		if err != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "  Warning: could not parse %s: %v\n", file.Path, err)
			}
			continue
		}

		// Register wiki-links.
		if len(note.WikiLinks) > 0 {
			targets := make([]string, len(note.WikiLinks))
			for i, link := range note.WikiLinks {
				targets[i] = link.Target
			}
			if err := linkRegistry.RegisterLinks(file.Path, targets); err != nil {
				return fmt.Errorf("register links from %s: %w", file.Path, err)
			}
			totalLinks += len(note.WikiLinks)
		}

		// Create initial state entry (pending sync).
		contentHashStr, err := state.HashFile(file.AbsPath)
		if err != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "  Warning: could not hash %s: %v\n", file.Path, err)
			}
			continue
		}
		syncState := &state.SyncState{
			ObsidianPath:  file.Path,
			ContentHash:   contentHashStr,
			ObsidianMtime: file.Info.ModTime(),
			Status:        "pending",
		}
		if err := db.SetState(syncState); err != nil {
			return fmt.Errorf("set initial state for %s: %w", file.Path, err)
		}
	}

	fmt.Printf("  ✓ Found %d markdown files\n", len(files))
	fmt.Printf("  ✓ Registered %d wiki-links\n", totalLinks)

	fmt.Println("\nInitialization complete!")
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Set NOTION_TOKEN environment variable")
	fmt.Println("  2. Run 'obsidian-notion status' to see pending files")
	fmt.Println("  3. Run 'obsidian-notion push' to sync to Notion")

	return nil
}

// expandAndValidateVaultPath expands ~ and validates the vault path exists.
func expandAndValidateVaultPath(path string) (string, error) {
	// Expand ~ to home directory.
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("get home directory: %w", err)
		}
		path = filepath.Join(home, path[1:])
	}

	// Convert to absolute path.
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("get absolute path: %w", err)
	}

	// Validate path exists and is a directory.
	info, err := os.Stat(absPath)
	if os.IsNotExist(err) {
		return "", fmt.Errorf("vault path does not exist: %s", absPath)
	}
	if err != nil {
		return "", fmt.Errorf("stat vault path: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("vault path is not a directory: %s", absPath)
	}

	return absPath, nil
}

// validateNotionToken tests the Notion API token by making a search request.
func validateNotionToken(ctx context.Context, client *notion.Client) error {
	// Use search with empty query to validate token.
	_, err := client.SearchPages(ctx, "")
	return err
}

// resolveDatabase resolves a database name or ID to a database ID.
func resolveDatabase(ctx context.Context, client *notion.Client, nameOrID string) (string, error) {
	// If it looks like a UUID, try to fetch directly.
	if looksLikeUUID(nameOrID) {
		db, err := client.GetDatabase(ctx, nameOrID)
		if err == nil {
			return string(db.ID), nil
		}
		// Fall through to search if direct fetch fails.
	}

	// Search for database by name.
	resp, err := client.API().Search.Do(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("search: %w", err)
	}

	// Look for matching database.
	for _, result := range resp.Results {
		if db, ok := result.(*notionapi.Database); ok {
			title := extractDatabaseTitle(db)
			if strings.EqualFold(title, nameOrID) {
				return string(db.ID), nil
			}
		}
	}

	return "", fmt.Errorf("database not found: %s", nameOrID)
}

// looksLikeUUID checks if a string looks like a UUID.
func looksLikeUUID(s string) bool {
	// Remove dashes and check length.
	clean := strings.ReplaceAll(s, "-", "")
	if len(clean) != 32 {
		return false
	}
	// Check all characters are hex.
	for _, c := range clean {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// extractDatabaseTitle extracts the title from a database.
func extractDatabaseTitle(db *notionapi.Database) string {
	if db == nil || len(db.Title) == 0 {
		return ""
	}
	var title strings.Builder
	for _, t := range db.Title {
		title.WriteString(t.PlainText)
	}
	return title.String()
}
