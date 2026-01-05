// Package parser provides Obsidian-flavored markdown parsing using goldmark.
//
// The parser extracts frontmatter, wiki-links, tags, embeds, and dataview queries
// from Obsidian notes, producing a structured representation suitable for
// transformation to Notion blocks.
package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"

	obsidian "github.com/powerman/goldmark-obsidian"
	"go.abhg.dev/goldmark/hashtag"
	"go.abhg.dev/goldmark/wikilink"
)

// Parser wraps goldmark with Obsidian extensions for parsing markdown notes.
type Parser struct {
	md goldmark.Markdown
}

// ParsedNote represents a fully parsed Obsidian note with extracted metadata.
type ParsedNote struct {
	// Path is the relative path within the vault.
	Path string

	// Frontmatter contains the parsed YAML frontmatter key-value pairs.
	Frontmatter map[string]any

	// AST is the goldmark abstract syntax tree of the note body.
	AST ast.Node

	// Source is the raw markdown content (excluding frontmatter).
	Source []byte

	// WikiLinks contains all [[wiki-links]] found in the note.
	WikiLinks []WikiLink

	// Tags contains all #tags found in the note (including from frontmatter).
	Tags []string

	// Embeds contains all ![[embedded]] content references.
	Embeds []Embed

	// DataviewQueries contains all dataview code blocks found.
	DataviewQueries []DataviewQuery
}

// WikiLink represents an Obsidian wiki-link [[target|alias]].
type WikiLink struct {
	// Target is the link destination (note name or path).
	Target string

	// Alias is the optional display text (after |).
	Alias string

	// Heading is the optional heading anchor (after #).
	Heading string

	// Block is the optional block reference (after ^).
	Block string

	// Line is the source line number (1-indexed).
	Line int
}

// Embed represents an Obsidian embed ![[target]].
type Embed struct {
	// Target is the embedded content reference.
	Target string

	// IsImage indicates if this is an image embed.
	IsImage bool

	// Width is the optional resize width.
	Width int

	// Height is the optional resize height.
	Height int

	// Line is the source line number (1-indexed).
	Line int
}

// New creates a new Parser with Obsidian extensions enabled.
func New() *Parser {
	md := goldmark.New(
		goldmark.WithExtensions(
			obsidian.NewObsidian(),
			&wikilink.Extender{},
		),
	)
	return &Parser{md: md}
}

// Parse parses an Obsidian note from the given path and content.
//
// It extracts frontmatter, parses the markdown body to an AST, and collects
// all wiki-links, tags, embeds, and dataview queries found in the note.
func (p *Parser) Parse(path string, content []byte) (*ParsedNote, error) {
	// 1. Extract and parse frontmatter.
	frontmatter, body, err := extractFrontmatter(content)
	if err != nil {
		return nil, err
	}

	// 2. Parse markdown body to AST.
	reader := text.NewReader(body)
	doc := p.md.Parser().Parse(reader)

	// 3. Walk AST to collect wiki-links, tags, embeds.
	var links []WikiLink
	var tags []string
	var embeds []Embed

	err = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch node := n.(type) {
		case *wikilink.Node:
			// Wiki-link node: [[target]] or [[target|alias]] or ![[embed]]
			target := string(node.Target)
			fragment := string(node.Fragment)

			// Parse heading and block references from fragment.
			var heading, block string
			if fragment != "" {
				if strings.HasPrefix(fragment, "^") {
					block = fragment[1:]
				} else {
					heading = fragment
				}
			}

			// Get alias from children if present.
			alias := extractWikilinkAlias(node, body)

			// Determine line number.
			line := 0
			if node.Parent() != nil {
				// Walk up to find a block-level parent with line info.
				line = findNodeLine(node, body)
			}

			if node.Embed {
				// This is an embed: ![[target]]
				embeds = append(embeds, Embed{
					Target:  target,
					IsImage: isImageEmbed(target),
					Line:    line,
				})
			} else {
				// Regular wiki-link.
				links = append(links, WikiLink{
					Target:  target,
					Alias:   alias,
					Heading: heading,
					Block:   block,
					Line:    line,
				})
			}

		case *hashtag.Node:
			// Hashtag node: #tag
			tag := string(node.Tag)
			if tag != "" {
				tags = append(tags, tag)
			}
		}

		return ast.WalkContinue, nil
	})
	if err != nil {
		return nil, err
	}

	// 4. Extract tags from frontmatter if present.
	if fmTags, ok := frontmatter["tags"]; ok {
		switch t := fmTags.(type) {
		case []any:
			for _, tag := range t {
				if s, ok := tag.(string); ok {
					tags = append(tags, s)
				}
			}
		case []string:
			tags = append(tags, t...)
		}
	}

	// 5. Extract dataview queries.
	dataviewQueries := ExtractDataviewQueries(body)

	return &ParsedNote{
		Path:            path,
		Frontmatter:     frontmatter,
		AST:             doc,
		Source:          body,
		WikiLinks:       links,
		Tags:            tags,
		Embeds:          embeds,
		DataviewQueries: dataviewQueries,
	}, nil
}

// ParseFile reads and parses a file from the filesystem.
func (p *Parser) ParseFile(vaultRoot, relativePath string) (*ParsedNote, error) {
	fullPath := filepath.Join(vaultRoot, relativePath)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("read file %s: %w", relativePath, err)
	}
	return p.Parse(relativePath, content)
}

// extractWikilinkAlias extracts the alias text from a wiki-link node.
// For [[target|alias]], the alias is stored as child text nodes.
func extractWikilinkAlias(node *wikilink.Node, source []byte) string {
	var alias strings.Builder
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if text, ok := child.(*ast.Text); ok {
			alias.Write(text.Segment.Value(source))
		}
	}
	return alias.String()
}

// findNodeLine attempts to find the line number for a node.
// Returns 1-indexed line number, or 0 if unknown.
func findNodeLine(n ast.Node, source []byte) int {
	// Walk up to find a node with line info.
	for node := n; node != nil; node = node.Parent() {
		if node.Lines().Len() > 0 {
			line := node.Lines().At(0)
			// Count newlines to determine line number.
			return countNewlines(source[:line.Start]) + 1
		}
	}
	return 0
}

// countNewlines counts the number of newline characters in a byte slice.
func countNewlines(b []byte) int {
	count := 0
	for _, c := range b {
		if c == '\n' {
			count++
		}
	}
	return count
}

// isImageEmbed checks if an embed target is an image.
func isImageEmbed(target string) bool {
	lower := strings.ToLower(target)
	imageExts := []string{".png", ".jpg", ".jpeg", ".gif", ".bmp", ".svg", ".webp"}
	for _, ext := range imageExts {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}
