// Package transformer converts between Obsidian AST and Notion blocks.
//
// The transformer handles the semantic mapping of Obsidian-specific features
// (wiki-links, callouts, frontmatter) to their Notion equivalents, preserving
// as much meaning as possible in the conversion.
package transformer

import (
	"github.com/jomei/notionapi"
	"github.com/yuin/goldmark/ast"

	"github.com/adamancini/obsidian-notion-sync/internal/parser"
)

// LinkResolver resolves Obsidian wiki-links to Notion page IDs.
type LinkResolver interface {
	// Resolve looks up a wiki-link target and returns the Notion page ID.
	// Returns empty string and false if the link cannot be resolved.
	Resolve(target string) (notionPageID string, found bool)
}

// Transformer converts Obsidian parsed notes to Notion page structures.
type Transformer struct {
	linkResolver LinkResolver
	config       *Config
}

// Config holds transformer configuration options.
type Config struct {
	// UnresolvedLinkStyle determines how to render unresolved wiki-links.
	// Options: "placeholder" (red text), "text" (plain text), "skip" (omit)
	UnresolvedLinkStyle string

	// CalloutIcons maps Obsidian callout types to Notion icons.
	CalloutIcons map[string]string

	// DataviewHandling determines how to handle dataview queries.
	// Options: "snapshot" (static content), "placeholder" (info block)
	DataviewHandling string

	// FlattenHeadings flattens H4-H6 to H3 (Notion only supports H1-H3).
	FlattenHeadings bool
}

// NotionPage represents a page ready to be created in Notion.
type NotionPage struct {
	// Properties are the page properties (title, tags, etc.).
	Properties notionapi.Properties

	// Children are the content blocks.
	Children []notionapi.Block
}

// New creates a new Transformer with the given link resolver and config.
func New(resolver LinkResolver, cfg *Config) *Transformer {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &Transformer{
		linkResolver: resolver,
		config:       cfg,
	}
}

// DefaultConfig returns the default transformer configuration.
func DefaultConfig() *Config {
	return &Config{
		UnresolvedLinkStyle: "placeholder",
		CalloutIcons: map[string]string{
			"note":      "💡",
			"abstract":  "📋",
			"summary":   "📋",
			"info":      "ℹ️",
			"todo":      "📝",
			"tip":       "💡",
			"hint":      "💡",
			"important": "❗",
			"success":   "✅",
			"check":     "✅",
			"done":      "✅",
			"question":  "❓",
			"help":      "❓",
			"faq":       "❓",
			"warning":   "⚠️",
			"caution":   "⚠️",
			"attention": "⚠️",
			"failure":   "❌",
			"fail":      "❌",
			"missing":   "❌",
			"danger":    "🔴",
			"error":     "🔴",
			"bug":       "🐛",
			"example":   "📖",
			"quote":     "💬",
			"cite":      "💬",
		},
		DataviewHandling: "placeholder",
		FlattenHeadings:  true,
	}
}

// Transform converts an Obsidian parsed note to a Notion page structure.
func (t *Transformer) Transform(note *parser.ParsedNote) (*NotionPage, error) {
	page := &NotionPage{
		Properties: t.transformProperties(note.Frontmatter, note.Tags),
		Children:   []notionapi.Block{},
	}

	// Walk AST and build Notion blocks.
	err := ast.Walk(note.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		block, skipChildren := t.transformNode(n, note.Source)
		if block != nil {
			page.Children = append(page.Children, block)
		}
		if skipChildren {
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return nil, err
	}

	return page, nil
}

// transformNode converts a goldmark AST node to a Notion block.
// Returns the block and whether to skip children (already processed).
func (t *Transformer) transformNode(n ast.Node, source []byte) (notionapi.Block, bool) {
	// TODO: Implement node transformation
	// This is the core transformation logic that maps each AST node type
	// to the corresponding Notion block type.

	switch node := n.(type) {
	case *ast.Document:
		// Document is the root, process children normally.
		return nil, false

	case *ast.Heading:
		return t.transformHeading(node, source), true

	case *ast.Paragraph:
		return t.transformParagraph(node, source), true

	case *ast.List:
		// Lists are handled specially - each item becomes a block.
		// Return nil here and let list items be processed.
		return nil, false

	case *ast.ListItem:
		return t.transformListItem(node, source), true

	case *ast.FencedCodeBlock:
		return t.transformCodeBlock(node, source), true

	case *ast.Blockquote:
		// Check if it's a callout.
		if callout := t.tryCallout(node, source); callout != nil {
			return callout, true
		}
		return t.transformQuote(node, source), true

	case *ast.ThematicBreak:
		return t.transformDivider(), true

	case *ast.CodeSpan, *ast.Text, *ast.Emphasis, *ast.Link, *ast.Image:
		// Inline elements are handled at the paragraph/heading level.
		return nil, false

	default:
		// Unknown node type, skip.
		return nil, false
	}
}

// transformProperties converts frontmatter and tags to Notion properties.
func (t *Transformer) transformProperties(frontmatter map[string]any, tags []string) notionapi.Properties {
	props := make(notionapi.Properties)

	// TODO: Implement property transformation using mappings.
	// This should use the PropertyMapping configuration to convert
	// frontmatter fields to Notion property types.

	// Handle title property.
	if title, ok := frontmatter["title"].(string); ok {
		props["Name"] = notionapi.TitleProperty{
			Title: []notionapi.RichText{
				{
					Type: notionapi.ObjectTypeText,
					Text: &notionapi.Text{Content: title},
				},
			},
		}
	}

	// Handle tags as multi-select.
	if len(tags) > 0 {
		options := make([]notionapi.Option, len(tags))
		for i, tag := range tags {
			options[i] = notionapi.Option{Name: tag}
		}
		props["Tags"] = notionapi.MultiSelectProperty{
			MultiSelect: options,
		}
	}

	return props
}
