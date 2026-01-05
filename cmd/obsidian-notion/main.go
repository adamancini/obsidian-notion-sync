// Package main provides the entry point for the obsidian-notion CLI tool.
//
// obsidian-notion is a bidirectional synchronization tool between Obsidian
// vaults and Notion databases. It preserves semantic meaning of Obsidian-specific
// features (wiki-links, frontmatter, callouts) when converting to Notion's
// block-based format.
package main

import (
	"os"

	"github.com/adamancini/obsidian-notion-sync/internal/cli"
)

// Version information set by build flags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cli.SetVersion(version, commit, date)
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
