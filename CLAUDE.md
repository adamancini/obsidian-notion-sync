# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A Go CLI tool for bidirectional synchronization between Obsidian vaults and Notion databases. Converts Obsidian-specific features (wiki-links, frontmatter, callouts) to Notion's block-based format while preserving semantic meaning.

**Current Status:** Design phase complete, implementation pending. See `docs/plans/2025-01-05-obsidian-notion-sync-design.md` for the full specification.

## Build Commands

```bash
# Build the binary
go build -o obsidian-notion ./cmd/obsidian-notion

# Run tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run a single test
go test -run TestFunctionName ./internal/parser

# Lint (once configured)
golangci-lint run

# Format
gofmt -w .
```

## Architecture

```
                   ┌──────────────────────────────────────────────┐
                   │            cmd/obsidian-notion               │
                   │   (Cobra CLI: init, push, pull, sync, status)│
                   └─────────────────┬────────────────────────────┘
                                     │
       ┌─────────────────────────────┼─────────────────────────────┐
       │                             │                             │
       ▼                             ▼                             ▼
┌─────────────┐             ┌─────────────────┐           ┌──────────────┐
│internal/parser│            │internal/transformer│         │internal/notion│
│              │            │                    │         │              │
│goldmark-obsidian│ ──AST──▶ │ Obsidian → Notion │ ──────▶ │ API client   │
│+ wikilink ext   │          │ Notion → Obsidian │         │ Rate limiting│
└──────┬───────┘            └────────────────────┘         └──────────────┘
       │
       │                    ┌─────────────────────────────────────────┐
       └───────────────────▶│              internal/state             │
                            │  SQLite: sync_state, links, history     │
                            │  Content hashing, link registry         │
                            └─────────────────────────────────────────┘
```

### Package Responsibilities

| Package | Purpose |
|---------|---------|
| `cmd/obsidian-notion` | Entry point, Cobra CLI setup |
| `internal/cli` | Command implementations (init, push, pull, sync, status, conflicts) |
| `internal/parser` | goldmark-obsidian wrapper, AST extraction, frontmatter parsing |
| `internal/transformer` | Bidirectional AST ↔ Notion block conversion |
| `internal/notion` | Notion API operations with rate limiting and batch handling |
| `internal/state` | SQLite sync state, content hashing, wiki-link registry |
| `internal/config` | YAML configuration parsing and validation |
| `internal/vault` | Obsidian vault scanning and file operations |
| `pkg/obsidian` | Reusable Obsidian utilities (frontmatter, wiki-links) |

### Key Data Flow

1. **Push (Obsidian → Notion):**
   - Parser extracts AST + frontmatter + wiki-links from markdown
   - Transformer converts AST nodes to Notion blocks
   - Wiki-links resolved via link registry (two-pass: create pages first, resolve links second)
   - Notion client creates/updates pages with rate limiting

2. **Pull (Notion → Obsidian):**
   - Fetch Notion page and blocks
   - Reverse transformer converts blocks to Obsidian-flavored markdown
   - Page mentions converted back to wiki-links using link registry

### State Management

SQLite database (`sync.db`) tracks:
- `sync_state`: path ↔ Notion page ID mapping, content hashes, sync timestamps
- `links`: wiki-link resolution cache
- `sync_history`: conflict resolution audit trail

## Key Dependencies

- **goldmark + goldmark-obsidian + wikilink**: Markdown parsing with Obsidian flavor
- **jomei/notionapi**: Notion API client
- **spf13/cobra**: CLI framework
- **mattn/go-sqlite3**: State persistence
- **golang.org/x/time/rate**: API rate limiting

## Implementation Phases

1. **Phase 1 (MVP):** One-way export with `init`, `push`, `status` commands
2. **Phase 2:** Incremental sync with change detection
3. **Phase 3:** Bidirectional sync with conflict handling
4. **Phase 4:** Watch mode, dataview snapshots, parallel processing

## Element Mapping Reference

Key Obsidian → Notion conversions:
- `[[wiki-link]]` → page mention (via link registry)
- `> [!callout]` → callout block with mapped icon
- Frontmatter YAML → page properties (type-aware)
- `==highlight==` → yellow background annotation
- H4-H6 → flattened to H3 (Notion limitation)
- Dataview queries → static snapshots with placeholder
