package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/adamancini/obsidian-notion-sync/internal/state"
)

var (
	linksRepair     bool
	linksDryRun     bool
	linksSuggestions bool
)

// linksCmd represents the links command.
var linksCmd = &cobra.Command{
	Use:   "links",
	Short: "Manage wiki-link resolution",
	Long: `Manage wiki-link resolution between Obsidian and Notion.

Shows statistics about wiki-link resolution, unresolved links, and provides
tools to repair broken links using fuzzy matching.

Examples:
  # Show link statistics
  obsidian-notion links

  # Show suggestions for unresolved links
  obsidian-notion links --suggestions

  # Preview what would be repaired (dry run)
  obsidian-notion links --repair --dry-run

  # Repair unresolved links using fuzzy matching
  obsidian-notion links --repair`,
	RunE: runLinks,
}

func init() {
	linksCmd.Flags().BoolVarP(&linksRepair, "repair", "r", false, "repair unresolved links using fuzzy matching")
	linksCmd.Flags().BoolVarP(&linksDryRun, "dry-run", "n", false, "show what would be repaired without making changes")
	linksCmd.Flags().BoolVarP(&linksSuggestions, "suggestions", "s", false, "show fuzzy match suggestions for unresolved links")
}

func runLinks(cmd *cobra.Command, args []string) error {
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

	registry := state.NewLinkRegistry(db)

	// Handle repair mode.
	if linksRepair {
		return runLinksRepair(registry, linksDryRun)
	}

	// Handle suggestions mode.
	if linksSuggestions {
		return runLinksSuggestions(registry)
	}

	// Default: show statistics.
	return runLinksStats(registry)
}

// runLinksStats shows link resolution statistics.
func runLinksStats(registry *state.LinkRegistry) error {
	stats, err := registry.GetStats()
	if err != nil {
		return fmt.Errorf("get stats: %w", err)
	}

	fmt.Println("Wiki-link statistics:")
	fmt.Println()
	fmt.Printf("  Total links:     %d\n", stats.Total)
	fmt.Printf("  Resolved:        %d\n", stats.Resolved)
	fmt.Printf("  Unresolved:      %d\n", stats.Unresolved)

	if stats.Unresolved > 0 {
		fmt.Println()
		fmt.Println("Files with unresolved links:")
		for path, count := range stats.BySource {
			fmt.Printf("  %s: %d unresolved\n", path, count)
		}
		fmt.Println()
		fmt.Println("Use --suggestions to see fuzzy match suggestions")
		fmt.Println("Use --repair to fix with fuzzy matching")
	}

	return nil
}

// runLinksSuggestions shows fuzzy match suggestions for unresolved links.
func runLinksSuggestions(registry *state.LinkRegistry) error {
	suggestions, err := registry.GetSuggestionsForUnresolved(3)
	if err != nil {
		return fmt.Errorf("get suggestions: %w", err)
	}

	if len(suggestions) == 0 {
		fmt.Println("No suggestions available (no unresolved links with potential matches)")
		return nil
	}

	fmt.Println("Suggestions for unresolved links:")
	fmt.Println()

	for _, s := range suggestions {
		fmt.Printf("  [[%s]]:\n", s.Target)
		for i, match := range s.Suggestions {
			scoreLabel := scoreToLabel(match.Score)
			fmt.Printf("    %d. %s (%s, distance: %d)\n",
				i+1, match.Name, scoreLabel, match.Distance)
		}
		fmt.Println()
	}

	return nil
}

// runLinksRepair repairs unresolved links using fuzzy matching.
func runLinksRepair(registry *state.LinkRegistry, dryRun bool) error {
	results, err := registry.RepairLinks(dryRun)
	if err != nil {
		return fmt.Errorf("repair links: %w", err)
	}

	if len(results) == 0 {
		fmt.Println("No links to repair (no unresolved links with fuzzy matches)")
		return nil
	}

	if dryRun {
		fmt.Println("Dry run - the following links would be repaired:")
	} else {
		fmt.Println("Repaired links:")
	}
	fmt.Println()

	repairedCount := 0
	for _, r := range results {
		scoreLabel := scoreToLabel(r.Score)
		status := "would repair"
		if !dryRun {
			if r.WasRepaired {
				status = "repaired"
				repairedCount++
			} else {
				status = "failed: " + r.Error
			}
		}

		fmt.Printf("  [[%s]] -> %s (%s)\n", r.TargetName, r.MatchedPath, scoreLabel)
		fmt.Printf("    in %s [%s]\n", r.SourcePath, status)
		fmt.Println()
	}

	if dryRun {
		fmt.Printf("Would repair %d links. Run without --dry-run to apply.\n", len(results))
	} else {
		fmt.Printf("Repaired %d of %d links.\n", repairedCount, len(results))
	}

	return nil
}

// scoreToLabel converts a MatchScore to a human-readable label.
func scoreToLabel(score state.MatchScore) string {
	switch score {
	case state.MatchExact:
		return "exact"
	case state.MatchCaseInsensitive:
		return "case-insensitive"
	case state.MatchPrefix:
		return "prefix"
	case state.MatchFuzzy:
		return "fuzzy"
	default:
		return "none"
	}
}
