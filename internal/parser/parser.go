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

	// IsPDF indicates if this is a PDF embed.
	IsPDF bool

	// IsAudio indicates if this is an audio embed.
	IsAudio bool

	// IsVideo indicates if this is a video embed.
	IsVideo bool

	// Heading is the optional heading reference (after #).
	Heading string

	// Block is the optional block reference (after ^).
	Block string

	// Width is the optional resize width.
	Width int

	// Height is the optional resize height.
	Height int

	// Line is the source line number (1-indexed).
	Line int

	// Depth tracks nesting level for recursive embeds.
	Depth int
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

			// Parse heading and block references.
			// Note: The wikilink library puts # fragments in Fragment field,
			// but ^ block references stay in Target. We need to check both.
			var heading, block string
			if fragment != "" {
				if strings.HasPrefix(fragment, "^") {
					block = fragment[1:]
				} else {
					heading = fragment
				}
			}
			// Check for block reference in target (wikilink lib doesn't parse ^ as fragment).
			if idx := strings.Index(target, "^"); idx != -1 {
				block = target[idx+1:]
				target = target[:idx]
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
				// This is an embed: ![[target]] or ![[target|dimensions]]
				// The wikilink library puts dimensions in the alias (child text nodes),
				// not in the target. Parse dimensions from alias for image embeds.
				var width, height int
				if alias != "" && isImageEmbed(target) {
					_, width, height = parseEmbedDimensions(target + "|" + alias)
				}

				embeds = append(embeds, Embed{
					Target:  target,
					Heading: heading,
					Block:   block,
					IsImage: isImageEmbed(target),
					IsPDF:   isPDFEmbed(target),
					IsAudio: isAudioEmbed(target),
					IsVideo: isVideoEmbed(target),
					Width:   width,
					Height:  height,
					Line:    line,
					Depth:   0, // Will be populated during resolution.
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
	// Walk up to find a block node with line info.
	// Inline nodes have Lines() but it panics - we must check Kind() first.
	for node := n; node != nil; node = node.Parent() {
		// Only block nodes have valid Lines() - inline nodes panic.
		if node.Kind().String() != "" && node.Type() == ast.TypeBlock {
			lines := node.Lines()
			if lines != nil && lines.Len() > 0 {
				line := lines.At(0)
				// Count newlines to determine line number.
				return countNewlines(source[:line.Start]) + 1
			}
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
	imageExts := []string{".png", ".jpg", ".jpeg", ".gif", ".bmp", ".svg", ".webp", ".avif", ".tiff", ".ico"}
	for _, ext := range imageExts {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// isPDFEmbed checks if an embed target is a PDF.
func isPDFEmbed(target string) bool {
	return strings.HasSuffix(strings.ToLower(target), ".pdf")
}

// isAudioEmbed checks if an embed target is an audio file.
func isAudioEmbed(target string) bool {
	lower := strings.ToLower(target)
	audioExts := []string{".mp3", ".wav", ".ogg", ".m4a", ".flac", ".aac", ".wma", ".webm"}
	for _, ext := range audioExts {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// isVideoEmbed checks if an embed target is a video file.
func isVideoEmbed(target string) bool {
	lower := strings.ToLower(target)
	videoExts := []string{".mp4", ".mkv", ".avi", ".mov", ".wmv", ".webm", ".ogv"}
	for _, ext := range videoExts {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// parseEmbedDimensions extracts dimensions from an embed target.
// Obsidian supports formats like: image.png|100 or image.png|100x200
// Returns: target (without dimensions), width, height.
func parseEmbedDimensions(target string) (string, int, int) {
	// Check for pipe separator indicating dimensions.
	idx := strings.LastIndex(target, "|")
	if idx == -1 {
		return target, 0, 0
	}

	cleanTarget := target[:idx]
	dimStr := target[idx+1:]

	// Parse dimensions: either "width" or "widthxheight".
	if strings.Contains(dimStr, "x") {
		parts := strings.SplitN(dimStr, "x", 2)
		width := parseInt(parts[0])
		height := parseInt(parts[1])
		return cleanTarget, width, height
	}

	// Single value means width only.
	width := parseInt(dimStr)
	return cleanTarget, width, 0
}

// parseInt converts a string to int, returning 0 on error.
func parseInt(s string) int {
	s = strings.TrimSpace(s)
	var n int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		} else {
			break // Stop at first non-digit.
		}
	}
	return n
}

// MaxEmbedDepth is the maximum recursion depth for resolving nested embeds.
const MaxEmbedDepth = 10

// EmbedResolver resolves embed targets to their content for nested embed support.
type EmbedResolver interface {
	// ResolveEmbed returns the content of an embedded note.
	// Returns the content and true if found, or nil and false if not found.
	ResolveEmbed(target string) ([]byte, bool)
}

// ResolveNestedEmbeds recursively resolves embeds in a parsed note.
// This detects circular references and enforces maximum depth.
func (p *Parser) ResolveNestedEmbeds(note *ParsedNote, resolver EmbedResolver) error {
	if resolver == nil {
		return nil
	}

	visited := make(map[string]bool)
	visited[note.Path] = true

	for i := range note.Embeds {
		if err := p.resolveEmbedRecursive(&note.Embeds[i], resolver, visited, 0); err != nil {
			return err
		}
	}

	return nil
}

// resolveEmbedRecursive resolves a single embed and its nested embeds.
func (p *Parser) resolveEmbedRecursive(embed *Embed, resolver EmbedResolver, visited map[string]bool, depth int) error {
	if depth > MaxEmbedDepth {
		return &EmbedError{
			Target:  embed.Target,
			Message: "maximum embed depth exceeded",
		}
	}

	// Check for circular reference.
	if visited[embed.Target] {
		return &EmbedError{
			Target:    embed.Target,
			Message:   "circular embed reference detected",
			IsCircular: true,
		}
	}

	embed.Depth = depth
	visited[embed.Target] = true
	defer func() {
		delete(visited, embed.Target)
	}()

	// Media embeds don't have nested content.
	if embed.IsImage || embed.IsPDF || embed.IsAudio || embed.IsVideo {
		return nil
	}

	// Try to resolve the embed target.
	content, found := resolver.ResolveEmbed(embed.Target)
	if !found {
		// Not found is not an error, just means we can't resolve nested embeds.
		return nil
	}

	// Parse the embedded content to find nested embeds.
	nestedNote, err := p.Parse(embed.Target, content)
	if err != nil {
		// Parse errors in embedded content are noted but not fatal.
		return nil
	}

	// Recursively resolve nested embeds.
	for i := range nestedNote.Embeds {
		if err := p.resolveEmbedRecursive(&nestedNote.Embeds[i], resolver, visited, depth+1); err != nil {
			return err
		}
	}

	return nil
}

// EmbedError represents an error in embed resolution.
type EmbedError struct {
	// Target is the embed target that caused the error.
	Target string

	// Message describes the error.
	Message string

	// IsCircular indicates if this is a circular reference error.
	IsCircular bool
}

func (e *EmbedError) Error() string {
	return "embed error for '" + e.Target + "': " + e.Message
}
