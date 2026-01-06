// Package cli implements the Cobra-based command-line interface for obsidian-notion.
//
// The CLI provides commands for initializing sync configuration, pushing changes
// to Notion, pulling changes from Notion, performing bidirectional sync, and
// managing conflicts.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/adamancini/obsidian-notion-sync/internal/config"
	"github.com/adamancini/obsidian-notion-sync/internal/transformer"
)

var (
	// Version information set at build time.
	version = "dev"
	commit  = "none"
	date    = "unknown"

	// Global flags.
	cfgFile string
	verbose bool

	// Loaded configuration.
	cfg *config.Config
)

// SetVersion sets the version information for the CLI.
func SetVersion(v, c, d string) {
	version = v
	commit = c
	date = d
}

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "obsidian-notion",
	Short: "Bidirectional sync between Obsidian and Notion",
	Long: `obsidian-notion is a CLI tool for bidirectional synchronization
between an Obsidian vault and Notion databases.

It preserves semantic meaning of Obsidian-specific features:
  - Wiki-links become Notion page mentions
  - Frontmatter becomes page properties
  - Callouts map to callout blocks
  - And more...

Use 'obsidian-notion init' to set up a new sync configuration,
then 'obsidian-notion push' to export your notes to Notion.`,
	Version: version,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip config loading for init command.
		if cmd.Name() == "init" {
			return nil
		}

		var err error
		cfg, err = config.Load(cfgFile)
		if err != nil {
			// Config not required for all commands.
			if verbose {
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			}
		}
		return nil
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Persistent flags available to all subcommands.
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.config/obsidian-notion/config.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")

	// Set version template.
	rootCmd.SetVersionTemplate(fmt.Sprintf("obsidian-notion %s (commit: %s, built: %s)\n", version, commit, date))

	// Add subcommands.
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(pushCmd)
	rootCmd.AddCommand(pullCmd)
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(conflictsCmd)
	rootCmd.AddCommand(linksCmd)
}

// ErrNoConfig is returned when no configuration is available.
var ErrNoConfig = fmt.Errorf("no configuration found - run 'obsidian-notion init' first")

// getConfig returns the loaded configuration or an error if not available.
func getConfig() (*config.Config, error) {
	if cfg == nil {
		return nil, ErrNoConfig
	}
	return cfg, nil
}

// buildTransformerConfig creates a transformer.Config from the app config.
// If path is provided, it merges global and path-specific property mappings.
func buildTransformerConfig(cfg *config.Config, path string) *transformer.Config {
	transformerCfg := &transformer.Config{
		UnresolvedLinkStyle: cfg.Transform.UnresolvedLinks,
		CalloutIcons:        cfg.Transform.Callouts,
		DataviewHandling:    cfg.Transform.Dataview,
		FlattenHeadings:     true,
	}

	// Convert config property mappings to transformer property mappings.
	configMappings := cfg.GetPropertyMappingsForPath(path)
	if len(configMappings) > 0 {
		transformerCfg.PropertyMappings = make([]transformer.PropertyMapping, len(configMappings))
		for i, m := range configMappings {
			transformerCfg.PropertyMappings[i] = transformer.PropertyMappingFromConfig(m.Obsidian, m.Notion, m.Type)
		}
	}

	return transformerCfg
}
