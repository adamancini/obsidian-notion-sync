package obsidian

import (
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// wikiLinkRegex matches [[target]] and [[target|alias]] patterns.
	wikiLinkRegex = regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)

	// embedRegex matches ![[target]] embedded content patterns.
	embedRegex = regexp.MustCompile(`!\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)
)

// WikiLink represents a parsed Obsidian wiki-link.
type WikiLink struct {
	// Raw is the original link text including brackets.
	Raw string

	// Target is the link destination (note name or path).
	Target string

	// Alias is the optional display text (after |).
	Alias string

	// Heading is the optional heading anchor (after #).
	Heading string

	// Block is the optional block reference (after ^).
	Block string

	// IsEmbed indicates if this is an embed (![[...]]).
	IsEmbed bool
}

// ParseWikiLink parses a single wiki-link string.
func ParseWikiLink(s string) *WikiLink {
	link := &WikiLink{Raw: s}

	// Check if embed.
	if strings.HasPrefix(s, "![[") {
		link.IsEmbed = true
		s = s[3 : len(s)-2] // Remove ![[...]]
	} else if strings.HasPrefix(s, "[[") {
		s = s[2 : len(s)-2] // Remove [[...]]
	} else {
		return nil
	}

	// Extract alias.
	if idx := strings.LastIndex(s, "|"); idx != -1 {
		link.Alias = s[idx+1:]
		s = s[:idx]
	}

	// Extract block reference.
	if idx := strings.LastIndex(s, "^"); idx != -1 {
		link.Block = s[idx+1:]
		s = s[:idx]
	}

	// Extract heading.
	if idx := strings.LastIndex(s, "#"); idx != -1 {
		link.Heading = s[idx+1:]
		s = s[:idx]
	}

	link.Target = s

	return link
}

// ExtractWikiLinks finds all wiki-links in content.
func ExtractWikiLinks(content []byte) []WikiLink {
	var links []WikiLink

	matches := wikiLinkRegex.FindAllSubmatch(content, -1)
	for _, match := range matches {
		link := WikiLink{
			Raw:    string(match[0]),
			Target: string(match[1]),
		}
		if len(match) > 2 && len(match[2]) > 0 {
			link.Alias = string(match[2])
		}
		// Parse heading/block from target.
		parseTargetParts(&link)
		links = append(links, link)
	}

	return links
}

// ExtractEmbeds finds all embedded content references in content.
func ExtractEmbeds(content []byte) []WikiLink {
	var embeds []WikiLink

	matches := embedRegex.FindAllSubmatch(content, -1)
	for _, match := range matches {
		embed := WikiLink{
			Raw:     string(match[0]),
			Target:  string(match[1]),
			IsEmbed: true,
		}
		if len(match) > 2 && len(match[2]) > 0 {
			embed.Alias = string(match[2])
		}
		parseTargetParts(&embed)
		embeds = append(embeds, embed)
	}

	return embeds
}

// parseTargetParts extracts heading and block from target.
func parseTargetParts(link *WikiLink) {
	target := link.Target

	// Extract block reference.
	if idx := strings.LastIndex(target, "^"); idx != -1 {
		link.Block = target[idx+1:]
		target = target[:idx]
	}

	// Extract heading.
	if idx := strings.LastIndex(target, "#"); idx != -1 {
		link.Heading = target[idx+1:]
		target = target[:idx]
	}

	link.Target = target
}

// Display returns the display text for the link.
func (w WikiLink) Display() string {
	if w.Alias != "" {
		return w.Alias
	}
	// Use file name without extension.
	return strings.TrimSuffix(filepath.Base(w.Target), ".md")
}

// NormalizedTarget returns the target normalized for matching.
func (w WikiLink) NormalizedTarget() string {
	// Remove .md extension if present.
	target := strings.TrimSuffix(w.Target, ".md")
	// Normalize path separators.
	target = filepath.ToSlash(target)
	return target
}

// String returns the wiki-link in Obsidian format.
func (w WikiLink) String() string {
	var sb strings.Builder

	if w.IsEmbed {
		sb.WriteString("!")
	}
	sb.WriteString("[[")
	sb.WriteString(w.Target)

	if w.Heading != "" {
		sb.WriteString("#")
		sb.WriteString(w.Heading)
	}
	if w.Block != "" {
		sb.WriteString("^")
		sb.WriteString(w.Block)
	}
	if w.Alias != "" {
		sb.WriteString("|")
		sb.WriteString(w.Alias)
	}

	sb.WriteString("]]")

	return sb.String()
}

// ReplaceWikiLinks replaces wiki-links in content using a replacer function.
func ReplaceWikiLinks(content []byte, replacer func(WikiLink) string) []byte {
	return wikiLinkRegex.ReplaceAllFunc(content, func(match []byte) []byte {
		link := ParseWikiLink(string(match))
		if link == nil {
			return match
		}
		return []byte(replacer(*link))
	})
}

// ReplaceEmbeds replaces embeds in content using a replacer function.
func ReplaceEmbeds(content []byte, replacer func(WikiLink) string) []byte {
	return embedRegex.ReplaceAllFunc(content, func(match []byte) []byte {
		link := ParseWikiLink(string(match))
		if link == nil {
			return match
		}
		return []byte(replacer(*link))
	})
}

// IsImageEmbed checks if a wiki-link embed is an image.
func (w WikiLink) IsImageEmbed() bool {
	if !w.IsEmbed {
		return false
	}

	ext := strings.ToLower(filepath.Ext(w.Target))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".svg", ".webp":
		return true
	}
	return false
}

// IsFileEmbed checks if a wiki-link embed is a non-image file.
func (w WikiLink) IsFileEmbed() bool {
	if !w.IsEmbed {
		return false
	}
	return !w.IsImageEmbed() && filepath.Ext(w.Target) != ""
}

// IsNoteEmbed checks if a wiki-link embed is a note (no extension or .md).
func (w WikiLink) IsNoteEmbed() bool {
	if !w.IsEmbed {
		return false
	}
	ext := filepath.Ext(w.Target)
	return ext == "" || ext == ".md"
}
