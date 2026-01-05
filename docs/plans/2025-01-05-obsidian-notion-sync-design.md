# Obsidian-Notion Sync: Detailed Design

**Date:** 2025-01-05
**Status:** Draft
**Author:** Design session with Claude

## Overview

A Go CLI tool for bidirectional synchronization between an Obsidian vault and Notion databases. The tool preserves semantic meaning of Obsidian-specific features (wiki-links, frontmatter, callouts) when converting to Notion's block-based format.

## Goals

1. **Initial export:** Push all Obsidian notes to Notion with high fidelity
2. **Incremental sync:** Only sync changed files in either direction
3. **Bidirectional sync:** Full two-way sync with conflict detection and resolution
4. **Semantic preservation:** Wiki-links become page mentions, frontmatter becomes properties, callouts map to callout blocks

## Non-Goals (for MVP)

- Real-time sync / watch mode
- Excalidraw conversion
- Daily notes migration
- Dataview query execution (snapshots only)

## User Requirements

- **Priority content:** Wiki-links & backlinks, frontmatter/YAML properties, dataview queries
- **Secondary content:** Excalidraw drawings, daily note structure
- **Transformation tolerance:** Preserve semantics first, best-effort with markers for untranslatable content
- **Implementation:** Custom Go CLI tool

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         obsidian-notion-sync                            │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌─────────────┐   ┌─────────────┐   ┌─────────────┐   ┌─────────────┐ │
│  │   cmd/      │   │  internal/  │   │  internal/  │   │  internal/  │ │
│  │   cli       │   │   parser    │   │ transformer │   │   notion    │ │
│  └──────┬──────┘   └──────┬──────┘   └──────┬──────┘   └──────┬──────┘ │
│         │                 │                 │                 │         │
│         │    ┌────────────┴────────────┐    │                 │         │
│         │    │   goldmark-obsidian     │    │                 │         │
│         │    │   + wikilink extension  │    │                 │         │
│         │    └─────────────────────────┘    │                 │         │
│         │                                   │                 │         │
│         │         ┌─────────────────────────┴───────┐         │         │
│         │         │  Obsidian AST → Notion Blocks   │         │         │
│         │         │  - Paragraphs, headings, lists  │         │         │
│         │         │  - Wiki-links → page mentions   │         │         │
│         │         │  - Properties → page properties │         │         │
│         │         │  - Callouts → callout blocks    │         │         │
│         │         └─────────────────────────────────┘         │         │
│         │                                           ┌─────────┴───────┐ │
│         │                                           │  jomei/notionapi│ │
│         │                                           └─────────────────┘ │
│  ┌──────┴──────────────────────────────────────────────────────────┐   │
│  │                      internal/state                              │   │
│  │  ┌────────────┐  ┌────────────┐  ┌────────────┐                 │   │
│  │  │  SQLite    │  │  File hash │  │  Link      │                 │   │
│  │  │  sync.db   │  │  tracking  │  │  registry  │                 │   │
│  │  └────────────┘  └────────────┘  └────────────┘                 │   │
│  └──────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────┘
```

### Package Responsibilities

| Package | Responsibility |
|---------|----------------|
| `cmd/cli` | Cobra-based CLI commands (init, push, pull, sync, status) |
| `internal/parser` | Wraps goldmark-obsidian, extracts AST + frontmatter |
| `internal/transformer` | Maps Obsidian AST nodes → Notion block types |
| `internal/notion` | Notion API operations, rate limiting, batch operations |
| `internal/state` | SQLite-based sync state, content hashing, link resolution |

---

## Data Model & Mapping

### Obsidian → Notion Element Mapping

| Obsidian Element | Notion Block Type | Notes |
|------------------|-------------------|-------|
| `# Heading 1` | `heading_1` | Direct map |
| `## Heading 2` | `heading_2` | Direct map |
| `### Heading 3` | `heading_3` | H4-H6 flatten to H3 |
| Paragraph | `paragraph` | Direct map |
| `- bullet` | `bulleted_list_item` | Direct map |
| `1. numbered` | `numbered_list_item` | Direct map |
| `- [ ] task` | `to_do` | With checked state |
| `> quote` | `quote` | Direct map |
| `> [!note]` callout | `callout` | Map icon by type |
| ` ```code``` ` | `code` | With language |
| `[[wiki-link]]` | `mention` (page) | Requires link registry |
| `![[embed]]` | `embed` or `synced_block` | Complex - see below |
| `==highlight==` | Rich text annotation | `highlighted: true` |
| `**bold**` | Rich text annotation | `bold: true` |
| `*italic*` | Rich text annotation | `italic: true` |
| `~~strike~~` | Rich text annotation | `strikethrough: true` |
| `` `code` `` | Rich text annotation | `code: true` |
| `[link](url)` | Rich text with `href` | Direct map |
| `#tag` | Page property (multi-select) | Collected to properties |
| `---` frontmatter | Page properties | Type-aware mapping |
| Tables | `table` + `table_row` | Direct map |
| `$$latex$$` | `equation` | Direct map |
| `---` divider | `divider` | Direct map |
| Images `![]()` | `image` | Upload or external URL |

### Frontmatter → Notion Properties

```yaml
# Obsidian frontmatter
---
title: My Note
tags: [work, kubernetes]
status: in-progress
due: 2025-01-15
priority: high
---
```

Maps to Notion page properties:

```go
type PropertyMapping struct {
    ObsidianKey  string
    NotionName   string
    NotionType   PropertyType // title, multi_select, select, date, checkbox, number, rich_text
    Transform    func(any) any // optional transformation
}

var DefaultMappings = []PropertyMapping{
    {"title", "Name", PropertyTypeTitle, nil},
    {"tags", "Tags", PropertyTypeMultiSelect, nil},
    {"status", "Status", PropertyTypeSelect, nil},
    {"due", "Due Date", PropertyTypeDate, parseDate},
    {"priority", "Priority", PropertyTypeSelect, nil},
    // User can define custom mappings in config
}
```

### Wiki-Link Resolution Strategy

```go
// Link registry tracks Obsidian path → Notion page ID
type LinkRegistry struct {
    db *sql.DB
}

type LinkEntry struct {
    ObsidianPath string    // "work/chef-360/ServiceClass Architecture.md"
    ObsidianName string    // "ServiceClass Architecture" (for [[name]] matching)
    NotionPageID string    // "12345678-abcd-..."
    LastSynced   time.Time
}

// Resolution order:
// 1. Exact path match: [[work/chef-360/ServiceClass Architecture]]
// 2. Name match: [[ServiceClass Architecture]] → finds by name
// 3. Alias match: from frontmatter aliases field
// 4. Unresolved: creates placeholder or marks for later resolution
```

---

## Sync State Management

### SQLite Schema

```sql
-- Core sync state
CREATE TABLE sync_state (
    id INTEGER PRIMARY KEY,
    obsidian_path TEXT UNIQUE NOT NULL,
    notion_page_id TEXT,
    notion_parent_id TEXT,          -- Database or page containing this
    content_hash TEXT NOT NULL,      -- SHA256 of normalized content
    frontmatter_hash TEXT,           -- SHA256 of YAML frontmatter
    obsidian_mtime INTEGER,          -- File modification time
    notion_mtime INTEGER,            -- Last edited time from Notion
    last_sync INTEGER,               -- Unix timestamp of last sync
    sync_direction TEXT,             -- 'push', 'pull', 'conflict'
    status TEXT DEFAULT 'pending'    -- 'synced', 'pending', 'conflict', 'error'
);

-- Link resolution cache
CREATE TABLE links (
    id INTEGER PRIMARY KEY,
    source_path TEXT NOT NULL,       -- Note containing the link
    target_name TEXT NOT NULL,       -- [[target_name]]
    target_path TEXT,                -- Resolved path (if found)
    notion_page_id TEXT,             -- Resolved Notion page ID
    resolved INTEGER DEFAULT 0       -- Boolean
);

-- Sync history for conflict resolution
CREATE TABLE sync_history (
    id INTEGER PRIMARY KEY,
    obsidian_path TEXT NOT NULL,
    action TEXT NOT NULL,            -- 'push', 'pull', 'skip', 'conflict_resolved'
    timestamp INTEGER NOT NULL,
    content_hash TEXT,
    details TEXT                     -- JSON with additional context
);

-- Configuration
CREATE TABLE config (
    key TEXT PRIMARY KEY,
    value TEXT
);
```

### Change Detection Algorithm

```go
type ChangeDetector struct {
    db       *sql.DB
    vault    string
    notionDB string
}

type Change struct {
    Path      string
    Type      ChangeType // Created, Modified, Deleted, Conflict
    Direction Direction  // Push, Pull, Both (conflict)
    LocalHash string
    RemoteHash string
}

func (d *ChangeDetector) DetectChanges(ctx context.Context) ([]Change, error) {
    var changes []Change

    // 1. Scan local vault for changes
    localFiles := d.scanVault()

    for path, file := range localFiles {
        state, exists := d.getState(path)

        if !exists {
            // New local file
            changes = append(changes, Change{
                Path: path,
                Type: Created,
                Direction: Push,
            })
            continue
        }

        localHash := hashContent(file.Content)

        if localHash != state.ContentHash {
            // Local modification - check remote too
            remoteHash, remoteMtime := d.getNotionState(state.NotionPageID)

            if remoteMtime > state.LastSync && remoteHash != state.ContentHash {
                // Both modified = conflict
                changes = append(changes, Change{
                    Path:       path,
                    Type:       Conflict,
                    Direction:  Both,
                    LocalHash:  localHash,
                    RemoteHash: remoteHash,
                })
            } else {
                // Only local modified
                changes = append(changes, Change{
                    Path:      path,
                    Type:      Modified,
                    Direction: Push,
                })
            }
        }
    }

    // 2. Check for remote-only changes (files modified in Notion)
    // 3. Check for deletions (in state but not in vault or Notion)

    return changes, nil
}
```

---

## CLI Interface

### Commands

```bash
# Initialize sync configuration
obsidian-notion init \
  --vault ~/notes \
  --notion-token $NOTION_TOKEN \
  --database "Obsidian Notes"  # or --page "Parent Page"

# Check what would sync
obsidian-notion status
# Output:
#   New (push):     3 notes
#   Modified (push): 5 notes
#   Modified (pull): 2 notes
#   Conflicts:       1 note
#   Synced:         152 notes

# Push local changes to Notion
obsidian-notion push [--all | --path <glob>] [--dry-run] [--force]

# Pull remote changes from Notion
obsidian-notion pull [--all | --path <glob>] [--dry-run] [--force]

# Bidirectional sync (push then pull, with conflict handling)
obsidian-notion sync [--strategy <ours|theirs|manual>]

# Resolve conflicts
obsidian-notion conflicts
obsidian-notion resolve <path> --keep <local|remote|both>

# Link management
obsidian-notion links --unresolved  # Show broken links
obsidian-notion links --repair      # Attempt to resolve unresolved links

# Export (one-time, no state tracking)
obsidian-notion export --path "work/**/*.md" --to-database "Work Notes"
```

### Configuration File

```yaml
# ~/.config/obsidian-notion/config.yaml (or .obsidian-notion.yaml in vault)

vault: ~/notes
notion:
  token: ${NOTION_TOKEN}  # Environment variable reference
  default_database: "12345678-abcd-..."

# Folder → Notion database mapping
mappings:
  - path: "work/**"
    database: "Work Notes"
    properties:
      - obsidian: status
        notion: Status
        type: select
      - obsidian: tags
        notion: Tags
        type: multi_select

  - path: "personal/**"
    database: "Personal Notes"

  - path: "journal/daily/**"
    database: "Daily Journal"
    properties:
      - obsidian: date
        notion: Date
        type: date

# Content transformation rules
transform:
  # Dataview queries: snapshot to static content
  dataview: snapshot

  # Callout type mapping
  callouts:
    note: 💡
    warning: ⚠️
    tip: 💡
    important: ❗

  # What to do with unresolved wiki-links
  unresolved_links: placeholder  # or 'text', 'skip'

# Sync behavior
sync:
  conflict_strategy: manual  # 'local', 'remote', 'manual', 'newer'
  ignore:
    - "templates/**"
    - "**/.excalidraw.md"
    - "**/daily/**"  # Skip daily notes

# Rate limiting (Notion API limits)
rate_limit:
  requests_per_second: 3
  batch_size: 100
```

---

## Core Implementation

### Parser Package

```go
// internal/parser/parser.go
package parser

import (
    "github.com/yuin/goldmark"
    "github.com/yuin/goldmark/ast"
    "github.com/yuin/goldmark/text"
    "github.com/powerman/goldmark-obsidian"
    wikilink "go.abhg.dev/goldmark/wikilink"
)

type Parser struct {
    md goldmark.Markdown
}

type ParsedNote struct {
    Path        string
    Frontmatter map[string]any
    AST         ast.Node
    Source      []byte
    WikiLinks   []WikiLink
    Tags        []string
    Embeds      []Embed
}

type WikiLink struct {
    Target string  // The [[target]]
    Alias  string  // The [[target|alias]] display text
    Line   int
}

func New() *Parser {
    md := goldmark.New(
        goldmark.WithExtensions(
            obsidian.NewObsidian(),
            &wikilink.Extender{},
        ),
    )
    return &Parser{md: md}
}

func (p *Parser) Parse(path string, content []byte) (*ParsedNote, error) {
    // 1. Extract and parse frontmatter
    frontmatter, body := extractFrontmatter(content)

    // 2. Parse markdown to AST
    reader := text.NewReader(body)
    doc := p.md.Parser().Parse(reader)

    // 3. Walk AST to collect wiki-links, tags, embeds
    var links []WikiLink
    var tags []string
    var embeds []Embed

    ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
        if !entering {
            return ast.WalkContinue, nil
        }

        switch node := n.(type) {
        case *wikilink.Node:
            links = append(links, WikiLink{
                Target: string(node.Target),
                // ...
            })
        case *obsidian.Tag:
            tags = append(tags, string(node.Name))
        case *obsidian.Embed:
            embeds = append(embeds, Embed{Target: string(node.Target)})
        }

        return ast.WalkContinue, nil
    })

    return &ParsedNote{
        Path:        path,
        Frontmatter: frontmatter,
        AST:         doc,
        Source:      body,
        WikiLinks:   links,
        Tags:        tags,
        Embeds:      embeds,
    }, nil
}
```

### Transformer Package

```go
// internal/transformer/transformer.go
package transformer

import (
    "github.com/jomei/notionapi"
    "github.com/yuin/goldmark/ast"
)

type Transformer struct {
    linkResolver LinkResolver
    config       *Config
}

type LinkResolver interface {
    Resolve(target string) (notionPageID string, found bool)
}

// Transform converts Obsidian AST to Notion blocks
func (t *Transformer) Transform(note *parser.ParsedNote) (*NotionPage, error) {
    page := &NotionPage{
        Properties: t.transformProperties(note.Frontmatter, note.Tags),
        Children:   []notionapi.Block{},
    }

    // Walk AST and build Notion blocks
    err := ast.Walk(note.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
        if !entering {
            return ast.WalkContinue, nil
        }

        block, skip := t.transformNode(n, note.Source)
        if block != nil {
            page.Children = append(page.Children, block)
        }
        if skip {
            return ast.WalkSkipChildren, nil
        }
        return ast.WalkContinue, nil
    })

    return page, err
}

func (t *Transformer) transformNode(n ast.Node, source []byte) (notionapi.Block, bool) {
    switch node := n.(type) {

    case *ast.Heading:
        return t.transformHeading(node, source), true

    case *ast.Paragraph:
        return t.transformParagraph(node, source), true

    case *ast.List:
        return t.transformList(node, source), true

    case *ast.FencedCodeBlock:
        return t.transformCodeBlock(node, source), true

    case *ast.Blockquote:
        // Check if it's a callout
        if callout := t.tryCallout(node, source); callout != nil {
            return callout, true
        }
        return t.transformQuote(node, source), true

    case *wikilink.Node:
        // Handled at inline level within paragraphs
        return nil, false

    // ... more node types
    }

    return nil, false
}

func (t *Transformer) transformHeading(h *ast.Heading, source []byte) notionapi.Block {
    richText := t.transformInlineContent(h, source)

    switch h.Level {
    case 1:
        return &notionapi.Heading1Block{
            Heading1: notionapi.Heading{RichText: richText},
        }
    case 2:
        return &notionapi.Heading2Block{
            Heading2: notionapi.Heading{RichText: richText},
        }
    default: // 3+
        return &notionapi.Heading3Block{
            Heading3: notionapi.Heading{RichText: richText},
        }
    }
}

// Transform inline content (bold, italic, links, wiki-links, etc.)
func (t *Transformer) transformInlineContent(n ast.Node, source []byte) []notionapi.RichText {
    var result []notionapi.RichText

    for child := n.FirstChild(); child != nil; child = child.NextSibling() {
        switch c := child.(type) {

        case *ast.Text:
            result = append(result, notionapi.RichText{
                Type: notionapi.ObjectTypeText,
                Text: &notionapi.Text{Content: string(c.Segment.Value(source))},
            })

        case *ast.Emphasis:
            text := t.transformInlineContent(c, source)
            for i := range text {
                if c.Level == 1 {
                    text[i].Annotations.Italic = true
                } else {
                    text[i].Annotations.Bold = true
                }
            }
            result = append(result, text...)

        case *wikilink.Node:
            pageID, found := t.linkResolver.Resolve(string(c.Target))
            if found {
                result = append(result, notionapi.RichText{
                    Type: notionapi.ObjectTypeMention,
                    Mention: &notionapi.Mention{
                        Type: notionapi.MentionTypePage,
                        Page: &notionapi.PageMention{ID: notionapi.PageID(pageID)},
                    },
                })
            } else {
                // Unresolved link - render as text with marker
                result = append(result, notionapi.RichText{
                    Type: notionapi.ObjectTypeText,
                    Text: &notionapi.Text{Content: "[[" + string(c.Target) + "]]"},
                    Annotations: &notionapi.Annotations{
                        Color: notionapi.ColorRed, // Visual marker for unresolved
                    },
                })
            }

        case *obsidian.Highlight:
            text := t.transformInlineContent(c, source)
            for i := range text {
                text[i].Annotations.Color = notionapi.ColorYellowBackground
            }
            result = append(result, text...)

        // ... code, strikethrough, links, etc.
        }
    }

    return result
}
```

### Notion API Client Wrapper

```go
// internal/notion/client.go
package notion

import (
    "context"
    "time"

    "github.com/jomei/notionapi"
    "golang.org/x/time/rate"
)

type Client struct {
    api     *notionapi.Client
    limiter *rate.Limiter
}

func New(token string) *Client {
    return &Client{
        api:     notionapi.NewClient(notionapi.Token(token)),
        limiter: rate.NewLimiter(rate.Every(time.Second/3), 1), // 3 req/sec
    }
}

// CreatePage creates a new page in a database with blocks
func (c *Client) CreatePage(ctx context.Context, databaseID string, page *NotionPage) (string, error) {
    if err := c.limiter.Wait(ctx); err != nil {
        return "", err
    }

    // Create page with properties
    created, err := c.api.Page.Create(ctx, &notionapi.PageCreateRequest{
        Parent: notionapi.Parent{
            Type:       notionapi.ParentTypeDatabaseID,
            DatabaseID: notionapi.DatabaseID(databaseID),
        },
        Properties: page.Properties,
    })
    if err != nil {
        return "", fmt.Errorf("create page: %w", err)
    }

    pageID := string(created.ID)

    // Append blocks in batches (Notion limit: 100 blocks per request)
    for i := 0; i < len(page.Children); i += 100 {
        end := min(i+100, len(page.Children))
        batch := page.Children[i:end]

        if err := c.limiter.Wait(ctx); err != nil {
            return pageID, err
        }

        _, err := c.api.Block.AppendChildren(ctx, notionapi.BlockID(pageID), &notionapi.AppendBlockChildrenRequest{
            Children: batch,
        })
        if err != nil {
            return pageID, fmt.Errorf("append blocks: %w", err)
        }
    }

    return pageID, nil
}

// UpdatePage updates an existing page (clear and recreate blocks)
func (c *Client) UpdatePage(ctx context.Context, pageID string, page *NotionPage) error {
    // 1. Update properties
    if err := c.limiter.Wait(ctx); err != nil {
        return err
    }

    _, err := c.api.Page.Update(ctx, notionapi.PageID(pageID), &notionapi.PageUpdateRequest{
        Properties: page.Properties,
    })
    if err != nil {
        return fmt.Errorf("update properties: %w", err)
    }

    // 2. Delete existing blocks
    if err := c.deleteAllBlocks(ctx, pageID); err != nil {
        return fmt.Errorf("delete blocks: %w", err)
    }

    // 3. Append new blocks
    for i := 0; i < len(page.Children); i += 100 {
        end := min(i+100, len(page.Children))
        batch := page.Children[i:end]

        if err := c.limiter.Wait(ctx); err != nil {
            return err
        }

        _, err := c.api.Block.AppendChildren(ctx, notionapi.BlockID(pageID), &notionapi.AppendBlockChildrenRequest{
            Children: batch,
        })
        if err != nil {
            return fmt.Errorf("append blocks: %w", err)
        }
    }

    return nil
}

// FetchPage retrieves a page and its blocks (for pull operations)
func (c *Client) FetchPage(ctx context.Context, pageID string) (*NotionPage, error) {
    // ... implementation for bidirectional sync
}
```

---

## Reverse Transformer (Notion → Obsidian)

For bidirectional sync:

```go
// internal/transformer/reverse.go
package transformer

type ReverseTransformer struct {
    config *Config
}

// NotionToMarkdown converts Notion blocks back to Obsidian-flavored markdown
func (t *ReverseTransformer) NotionToMarkdown(page *NotionPage) ([]byte, error) {
    var buf bytes.Buffer

    // 1. Convert properties to frontmatter
    if len(page.Properties) > 0 {
        buf.WriteString("---\n")
        for key, prop := range page.Properties {
            value := t.propertyToYAML(prop)
            buf.WriteString(fmt.Sprintf("%s: %s\n", key, value))
        }
        buf.WriteString("---\n\n")
    }

    // 2. Convert blocks to markdown
    for _, block := range page.Children {
        md := t.blockToMarkdown(block, 0)
        buf.WriteString(md)
    }

    return buf.Bytes(), nil
}

func (t *ReverseTransformer) blockToMarkdown(block notionapi.Block, depth int) string {
    indent := strings.Repeat("  ", depth)

    switch b := block.(type) {

    case *notionapi.Heading1Block:
        return "# " + t.richTextToMarkdown(b.Heading1.RichText) + "\n\n"

    case *notionapi.Heading2Block:
        return "## " + t.richTextToMarkdown(b.Heading2.RichText) + "\n\n"

    case *notionapi.Heading3Block:
        return "### " + t.richTextToMarkdown(b.Heading3.RichText) + "\n\n"

    case *notionapi.ParagraphBlock:
        return t.richTextToMarkdown(b.Paragraph.RichText) + "\n\n"

    case *notionapi.BulletedListItemBlock:
        text := t.richTextToMarkdown(b.BulletedListItem.RichText)
        result := indent + "- " + text + "\n"
        for _, child := range b.BulletedListItem.Children {
            result += t.blockToMarkdown(child, depth+1)
        }
        return result

    case *notionapi.ToDoBlock:
        checkbox := "[ ]"
        if b.ToDo.Checked {
            checkbox = "[x]"
        }
        return indent + "- " + checkbox + " " + t.richTextToMarkdown(b.ToDo.RichText) + "\n"

    case *notionapi.CalloutBlock:
        // Convert to Obsidian callout
        icon := b.Callout.Icon.Emoji
        calloutType := t.iconToCalloutType(icon)
        text := t.richTextToMarkdown(b.Callout.RichText)
        return fmt.Sprintf("> [!%s]\n> %s\n\n", calloutType, text)

    case *notionapi.CodeBlock:
        lang := b.Code.Language
        code := t.richTextToMarkdown(b.Code.RichText)
        return fmt.Sprintf("```%s\n%s\n```\n\n", lang, code)

    // ... more block types
    }

    return ""
}

func (t *ReverseTransformer) richTextToMarkdown(richText []notionapi.RichText) string {
    var result strings.Builder

    for _, rt := range richText {
        text := rt.PlainText

        // Handle mentions (convert back to wiki-links)
        if rt.Type == notionapi.ObjectTypeMention && rt.Mention != nil {
            if rt.Mention.Type == notionapi.MentionTypePage {
                // Look up the original Obsidian path
                obsidianPath := t.lookupPath(string(rt.Mention.Page.ID))
                if obsidianPath != "" {
                    text = "[[" + obsidianPath + "]]"
                } else {
                    text = "[[" + rt.PlainText + "]]"
                }
            }
        } else {
            // Apply annotations
            if rt.Annotations != nil {
                if rt.Annotations.Bold {
                    text = "**" + text + "**"
                }
                if rt.Annotations.Italic {
                    text = "*" + text + "*"
                }
                if rt.Annotations.Strikethrough {
                    text = "~~" + text + "~~"
                }
                if rt.Annotations.Code {
                    text = "`" + text + "`"
                }
                if rt.Annotations.Color == notionapi.ColorYellowBackground {
                    text = "==" + text + "=="  // Obsidian highlight
                }
            }

            // Handle links
            if rt.Text != nil && rt.Text.Link != nil {
                text = "[" + text + "](" + rt.Text.Link.Url + ")"
            }
        }

        result.WriteString(text)
    }

    return result.String()
}
```

---

## Dataview Query Handling

Since Notion has no equivalent to dataview, we snapshot queries:

```go
// internal/parser/dataview.go
package parser

import (
    "regexp"
)

var dataviewBlockRegex = regexp.MustCompile("(?s)```dataview\n(.+?)\n```")
var dataviewInlineRegex = regexp.MustCompile("`=(.+?)`")

type DataviewQuery struct {
    Raw      string
    Type     string  // "TABLE", "LIST", "TASK"
    Source   string
    StartPos int
    EndPos   int
}

// ExtractDataviewQueries finds all dataview blocks in content
func ExtractDataviewQueries(content []byte) []DataviewQuery {
    var queries []DataviewQuery

    matches := dataviewBlockRegex.FindAllSubmatchIndex(content, -1)
    for _, match := range matches {
        query := string(content[match[2]:match[3]])
        queries = append(queries, DataviewQuery{
            Raw:      query,
            Type:     parseQueryType(query),
            StartPos: match[0],
            EndPos:   match[1],
        })
    }

    return queries
}

// SnapshotDataview executes query and returns static markdown
// This requires dataview CLI or API - we'll shell out to Obsidian's dataview
func (p *Parser) SnapshotDataview(vaultPath string, query DataviewQuery) (string, error) {
    // Option 1: Use obsidian-local-rest-api if running
    // Option 2: Parse and execute simple queries ourselves
    // Option 3: Leave a placeholder with the original query as comment

    // For now, create an informative placeholder:
    return fmt.Sprintf(`> [!info] Dataview Query (Snapshot Required)
> Original query:
> '''
> %s
> '''
>
> Run 'obsidian-notion snapshot-dataview' to populate this with live data.
`, query.Raw), nil
}
```

---

## Project Structure

```
obsidian-notion-sync/
├── cmd/
│   └── obsidian-notion/
│       └── main.go
├── internal/
│   ├── cli/
│   │   ├── root.go
│   │   ├── init.go
│   │   ├── push.go
│   │   ├── pull.go
│   │   ├── sync.go
│   │   ├── status.go
│   │   └── conflicts.go
│   ├── config/
│   │   ├── config.go
│   │   └── mappings.go
│   ├── parser/
│   │   ├── parser.go
│   │   ├── frontmatter.go
│   │   └── dataview.go
│   ├── transformer/
│   │   ├── transformer.go      # Obsidian → Notion
│   │   ├── reverse.go          # Notion → Obsidian
│   │   ├── blocks.go
│   │   ├── properties.go
│   │   └── richtext.go
│   ├── notion/
│   │   ├── client.go
│   │   ├── pages.go
│   │   ├── blocks.go
│   │   └── databases.go
│   ├── state/
│   │   ├── db.go
│   │   ├── links.go
│   │   ├── changes.go
│   │   └── conflicts.go
│   └── vault/
│       ├── scanner.go
│       └── watcher.go          # For daemon mode (future)
├── pkg/
│   └── obsidian/               # Reusable Obsidian utilities
│       ├── frontmatter.go
│       └── wikilink.go
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

---

## Implementation Phases

### Phase 1: One-Way Export (MVP)

**Goal:** Push all notes from Obsidian → Notion database

- [ ] Project scaffolding, CLI setup with Cobra
- [ ] Config file parsing
- [ ] goldmark-obsidian parser integration
- [ ] Basic AST → Notion block transformer
- [ ] Notion API client with rate limiting
- [ ] SQLite state tracking
- [ ] `init`, `push`, `status` commands
- [ ] Wiki-link → page mention resolution (second pass)

**Deliverable:** `obsidian-notion push --all` works

### Phase 2: Incremental Sync

**Goal:** Only push changed files

- [ ] Content hashing for change detection
- [ ] Notion modification time tracking
- [ ] `push` only modified files
- [ ] Handle renames and deletions
- [ ] Improved wiki-link resolution

**Deliverable:** Fast incremental syncs

### Phase 3: Bidirectional Sync

**Goal:** Pull changes from Notion back to Obsidian

- [ ] Reverse transformer (Notion blocks → Markdown)
- [ ] Conflict detection algorithm
- [ ] Conflict resolution UI/commands
- [ ] `pull` and `sync` commands
- [ ] Page mention → wiki-link reverse mapping

**Deliverable:** Full bidirectional sync

### Phase 4: Polish & Advanced Features

- [ ] Dataview snapshot integration
- [ ] Watch mode / daemon
- [ ] Parallel processing for large vaults
- [ ] Image/attachment handling
- [ ] Excalidraw export (as images)

---

## Key Dependencies

| Package | Purpose | Link |
|---------|---------|------|
| `github.com/yuin/goldmark` | Base markdown parser | https://github.com/yuin/goldmark |
| `github.com/powerman/goldmark-obsidian` | Obsidian flavor support | https://pkg.go.dev/github.com/powerman/goldmark-obsidian |
| `go.abhg.dev/goldmark/wikilink` | Wiki-link parsing | https://pkg.go.dev/go.abhg.dev/goldmark/wikilink |
| `github.com/jomei/notionapi` | Notion API client | https://github.com/jomei/notionapi |
| `github.com/spf13/cobra` | CLI framework | https://github.com/spf13/cobra |
| `github.com/mattn/go-sqlite3` | SQLite driver | https://github.com/mattn/go-sqlite3 |
| `gopkg.in/yaml.v3` | YAML parsing | https://gopkg.in/yaml.v3 |
| `golang.org/x/time/rate` | Rate limiting | https://pkg.go.dev/golang.org/x/time/rate |

---

## Key Architectural Decisions

1. **SQLite for state** - Portable, no external dependencies, handles complex queries for change detection
2. **Two-pass wiki-link resolution** - First pass creates all pages, second pass resolves links (chicken-and-egg problem)
3. **goldmark-obsidian** - Handles 90% of Obsidian syntax out of the box, extensible for edge cases
4. **Block-level updates** - Notion's API requires replacing all blocks on update; we accept this trade-off

---

## References

- [Notion API Documentation](https://developers.notion.com/)
- [goldmark-obsidian](https://pkg.go.dev/github.com/powerman/goldmark-obsidian)
- [goldmark/wikilink](https://pkg.go.dev/go.abhg.dev/goldmark/wikilink)
- [jomei/notionapi](https://github.com/jomei/notionapi)
- [Martian (JS Markdown→Notion)](https://github.com/tryfabric/martian)
- [Nobsidion Plugin](https://github.com/quanphan2906/nobsidion)
