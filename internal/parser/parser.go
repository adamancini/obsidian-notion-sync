// Package parser provides Obsidian-flavored markdown parsing using goldmark.
//
// The parser extracts frontmatter, wiki-links, tags, embeds, and dataview queries
// from Obsidian notes, producing a structured representation suitable for
// transformation to Notion blocks.
package parser

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"

	obsidian "github.com/powerman/goldmark-obsidian"
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

		// TODO: Implement AST node collection
		// switch node := n.(type) {
		// case *wikilink.Node:
		//     // Collect wiki-link
		// case *obsidian.Tag:
		//     // Collect tag
		// case *obsidian.Embed:
		//     // Collect embed
		// }

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
	// TODO: Implement file reading
	// fullPath := filepath.Join(vaultRoot, relativePath)
	// content, err := os.ReadFile(fullPath)
	// if err != nil {
	//     return nil, fmt.Errorf("read file: %w", err)
	// }
	// return p.Parse(relativePath, content)

	return nil, nil
}
