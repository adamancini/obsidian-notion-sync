package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	resolvePath string
	resolveKeep string
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

	conflictsCmd.AddCommand(resolveCmd)
}

func runConflicts(cmd *cobra.Command, args []string) error {
	cfg := getConfig()

	// TODO: Implement conflicts command
	// 1. Query state database for files with conflict status
	// 2. Display list with paths, local mtime, remote mtime
	// 3. Show diff preview if requested

	fmt.Printf("Checking conflicts in vault: %s\n", cfg.Vault)
	fmt.Println()
	fmt.Println("No conflicts found.")

	return fmt.Errorf("conflicts command not yet implemented")
}

func runResolve(cmd *cobra.Command, args []string) error {
	cfg := getConfig()
	path := args[0]

	switch resolveKeep {
	case "local", "remote", "both":
		// Valid option
	default:
		return fmt.Errorf("invalid --keep value: %s (must be local, remote, or both)", resolveKeep)
	}

	// TODO: Implement resolve command
	// 1. Verify file is in conflict state
	// 2. Based on --keep:
	//    - local: push to Notion, update sync state
	//    - remote: pull from Notion, update sync state
	//    - both: create .conflict file with remote version
	// 3. Clear conflict status

	fmt.Printf("Resolving conflict for: %s\n", path)
	fmt.Printf("Keeping: %s version\n", resolveKeep)
	fmt.Printf("Vault: %s\n", cfg.Vault)

	return fmt.Errorf("resolve command not yet implemented")
}
