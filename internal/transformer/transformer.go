// Package transformer converts between Obsidian AST and Notion blocks.
//
// The transformer handles the semantic mapping of Obsidian-specific features
// (wiki-links, callouts, frontmatter) to their Notion equivalents, preserving
// as much meaning as possible in the conversion.
package transformer

import (
	"regexp"
	"strings"

	"github.com/jomei/notionapi"
	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"

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
	linkResolver   LinkResolver
	config         *Config
	propertyMapper *PropertyMapper
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

	// PropertyMappings defines how frontmatter fields map to Notion properties.
	// If nil or empty, uses DefaultMappings.
	PropertyMappings []PropertyMapping
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

	// Create property mapper from config or use defaults.
	var mappings []PropertyMapping
	if len(cfg.PropertyMappings) > 0 {
		mappings = cfg.PropertyMappings
	}

	return &Transformer{
		linkResolver:   resolver,
		config:         cfg,
		propertyMapper: NewPropertyMapper(mappings),
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
	switch node := n.(type) {
	case *ast.Document:
		// Document is the root, process children normally.
		return nil, false

	case *ast.Heading:
		return t.transformHeading(node, source), true

	case *ast.Paragraph:
		// Check for math block ($$...$$ on its own line).
		if mathExpr := t.tryMathBlock(node, source); mathExpr != "" {
			return t.transformEquation(mathExpr), true
		}
		// Check for standalone image (paragraph containing only an image).
		if imageBlock := t.tryImageBlock(node, source); imageBlock != nil {
			return imageBlock, true
		}
		return t.transformParagraph(node, source), true

	case *ast.List:
		// Lists are handled specially - each item becomes a block.
		// Return nil here and let list items be processed.
		return nil, false

	case *ast.ListItem:
		return t.transformListItem(node, source), true

	case *ast.FencedCodeBlock:
		// Check for special code block types.
		lang := string(node.Language(source))
		if lang == "math" || lang == "latex" {
			return t.transformMathCodeBlock(node, source), true
		}
		// Check for dataview/dataviewjs code blocks.
		if lang == "dataview" || lang == "dataviewjs" {
			return t.transformDataviewBlock(node, source), true
		}
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
		// Check for extension types.
		return t.transformExtensionNode(n, source)
	}
}

// transformProperties converts frontmatter and tags to Notion properties.
func (t *Transformer) transformProperties(frontmatter map[string]any, tags []string) notionapi.Properties {
	return t.propertyMapper.ToNotionProperties(frontmatter, tags)
}

// transformExtensionNode handles goldmark extension node types.
func (t *Transformer) transformExtensionNode(n ast.Node, source []byte) (notionapi.Block, bool) {
	switch node := n.(type) {
	case *extast.Table:
		return t.transformTable(node, source), true

	case *extast.TableHeader, *extast.TableRow, *extast.TableCell:
		// Table internals are handled by transformTable.
		return nil, false

	default:
		// Unknown extension type, skip.
		return nil, false
	}
}

// mathBlockRegex matches display math: $$ ... $$ (potentially multiline).
var mathBlockRegex = regexp.MustCompile(`^\s*\$\$\s*([\s\S]*?)\s*\$\$\s*$`)

// tryMathBlock checks if a paragraph is a display math block ($$...$$).
// Returns the expression if it is, empty string otherwise.
func (t *Transformer) tryMathBlock(p *ast.Paragraph, source []byte) string {
	// Get the raw text content of the paragraph.
	var content strings.Builder
	for child := p.FirstChild(); child != nil; child = child.NextSibling() {
		if txt, ok := child.(*ast.Text); ok {
			content.Write(txt.Segment.Value(source))
		}
	}

	text := content.String()
	matches := mathBlockRegex.FindStringSubmatch(text)
	if matches == nil {
		return ""
	}

	return strings.TrimSpace(matches[1])
}

// transformMathCodeBlock converts a ```math or ```latex code block to an equation.
func (t *Transformer) transformMathCodeBlock(cb *ast.FencedCodeBlock, source []byte) notionapi.Block {
	// Get the expression from the code block content.
	var content strings.Builder
	lines := cb.Lines()
	for i := 0; i < lines.Len(); i++ {
		line := lines.At(i)
		content.Write(line.Value(source))
	}

	// Trim whitespace.
	expression := strings.TrimSpace(content.String())

	return t.transformEquation(expression)
}
