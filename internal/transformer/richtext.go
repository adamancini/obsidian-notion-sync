package transformer

import (
	"regexp"
	"strings"

	"github.com/jomei/notionapi"
	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
	"go.abhg.dev/goldmark/wikilink"
)

// transformInlineContent converts all inline children of a node to rich text.
func (t *Transformer) transformInlineContent(n ast.Node, source []byte) []notionapi.RichText {
	var result []notionapi.RichText

	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		result = append(result, t.transformInline(child, source, nil)...)
	}

	return result
}

// transformInline converts a single inline node to rich text.
// Annotations are inherited from parent formatting contexts.
func (t *Transformer) transformInline(n ast.Node, source []byte, inherited *notionapi.Annotations) []notionapi.RichText {
	// Ensure we have annotations to work with.
	if inherited == nil {
		inherited = &notionapi.Annotations{}
	}

	switch node := n.(type) {
	case *ast.Text:
		return t.transformText(node, source, inherited)

	case *ast.String:
		content := string(node.Value)
		return []notionapi.RichText{
			{
				Type:        notionapi.ObjectTypeText,
				Text:        &notionapi.Text{Content: content},
				Annotations: copyAnnotations(inherited),
			},
		}

	case *ast.Emphasis:
		// Level 1 = italic, Level 2 = bold
		newAnnotations := copyAnnotations(inherited)
		if node.Level == 1 {
			newAnnotations.Italic = true
		} else {
			newAnnotations.Bold = true
		}
		return t.transformInlineChildren(node, source, newAnnotations)

	case *ast.CodeSpan:
		content := string(node.Text(source))
		// Check for inline dataview query: `=expression`
		if strings.HasPrefix(content, "=") {
			return t.transformInlineDataview(content[1:], inherited)
		}
		newAnnotations := copyAnnotations(inherited)
		newAnnotations.Code = true
		return []notionapi.RichText{
			{
				Type:        notionapi.ObjectTypeText,
				Text:        &notionapi.Text{Content: content},
				Annotations: newAnnotations,
			},
		}

	case *ast.Link:
		// Get link text.
		var textContent string
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			if t, ok := child.(*ast.Text); ok {
				textContent += string(t.Segment.Value(source))
			}
		}
		if textContent == "" {
			textContent = string(node.Destination)
		}

		return []notionapi.RichText{
			{
				Type: notionapi.ObjectTypeText,
				Text: &notionapi.Text{
					Content: textContent,
					Link:    &notionapi.Link{Url: string(node.Destination)},
				},
				Annotations: copyAnnotations(inherited),
			},
		}

	case *ast.AutoLink:
		url := string(node.URL(source))
		return []notionapi.RichText{
			{
				Type: notionapi.ObjectTypeText,
				Text: &notionapi.Text{
					Content: url,
					Link:    &notionapi.Link{Url: url},
				},
				Annotations: copyAnnotations(inherited),
			},
		}

	case *ast.Image:
		// Images in inline context become links to the image.
		// TODO: Handle image embeds differently?
		alt := string(node.Text(source))
		if alt == "" {
			alt = string(node.Destination)
		}
		return []notionapi.RichText{
			{
				Type: notionapi.ObjectTypeText,
				Text: &notionapi.Text{
					Content: alt,
					Link:    &notionapi.Link{Url: string(node.Destination)},
				},
				Annotations: copyAnnotations(inherited),
			},
		}

	case *ast.RawHTML:
		// Pass through raw HTML as plain text.
		content := ""
		for i := 0; i < node.Segments.Len(); i++ {
			seg := node.Segments.At(i)
			content += string(seg.Value(source))
		}
		return []notionapi.RichText{
			{
				Type:        notionapi.ObjectTypeText,
				Text:        &notionapi.Text{Content: content},
				Annotations: copyAnnotations(inherited),
			},
		}

	case *wikilink.Node:
		target := string(node.Target)
		alias := extractWikilinkAliasFromNode(node, source)

		// Check if this is an embed (![[...]]).
		if node.Embed {
			// Check if it's an image embed.
			if isImageFile(target) {
				return t.transformWikiLinkImage(target, alias, inherited)
			}
			// Other embeds (notes, PDFs, etc.) - for now, render as text placeholder.
			return t.transformWikiLinkEmbed(target, alias, inherited)
		}

		// Regular wiki-link: [[target]] or [[target|alias]]
		return t.transformWikiLink(target, alias, inherited)

	case *extast.Strikethrough:
		// Strikethrough: ~~text~~
		newAnnotations := copyAnnotations(inherited)
		newAnnotations.Strikethrough = true
		return t.transformInlineChildren(node, source, newAnnotations)

	case *extast.TaskCheckBox:
		// Task checkbox: skip, it's handled at the list item level.
		return nil

	default:
		// For unknown inline types, try to process children.
		// Also handle highlight pattern (==text==) in text since goldmark-obsidian
		// doesn't parse highlights as separate nodes.
		return t.transformInlineChildrenWithHighlight(n, source, inherited)
	}
}

// transformText converts an ast.Text node to rich text.
func (t *Transformer) transformText(text *ast.Text, source []byte, annotations *notionapi.Annotations) []notionapi.RichText {
	content := string(text.Segment.Value(source))

	// Handle soft line breaks.
	if text.SoftLineBreak() {
		content += " "
	}

	// Handle hard line breaks.
	if text.HardLineBreak() {
		content += "\n"
	}

	return []notionapi.RichText{
		{
			Type:        notionapi.ObjectTypeText,
			Text:        &notionapi.Text{Content: content},
			Annotations: annotations,
		},
	}
}

// transformInlineChildren processes all children of a node with given annotations.
func (t *Transformer) transformInlineChildren(n ast.Node, source []byte, annotations *notionapi.Annotations) []notionapi.RichText {
	var result []notionapi.RichText

	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		result = append(result, t.transformInline(child, source, annotations)...)
	}

	return result
}

// highlightRegex matches Obsidian highlight syntax: ==text==
var highlightRegex = regexp.MustCompile(`==([^=]+)==`)

// transformInlineChildrenWithHighlight processes children, also handling raw highlight patterns.
// This is a fallback for when goldmark-obsidian doesn't parse highlights as nodes.
func (t *Transformer) transformInlineChildrenWithHighlight(n ast.Node, source []byte, annotations *notionapi.Annotations) []notionapi.RichText {
	var result []notionapi.RichText

	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		// For text nodes, check for highlight patterns.
		if txt, ok := child.(*ast.Text); ok {
			content := string(txt.Segment.Value(source))
			result = append(result, t.parseTextWithHighlights(content, annotations)...)

			// Handle line breaks.
			if txt.SoftLineBreak() {
				result = append(result, notionapi.RichText{
					Type:        notionapi.ObjectTypeText,
					Text:        &notionapi.Text{Content: " "},
					Annotations: copyAnnotations(annotations),
				})
			}
			if txt.HardLineBreak() {
				result = append(result, notionapi.RichText{
					Type:        notionapi.ObjectTypeText,
					Text:        &notionapi.Text{Content: "\n"},
					Annotations: copyAnnotations(annotations),
				})
			}
		} else {
			result = append(result, t.transformInline(child, source, annotations)...)
		}
	}

	return result
}

// parseTextWithHighlights parses text content for ==highlight== patterns.
func (t *Transformer) parseTextWithHighlights(content string, annotations *notionapi.Annotations) []notionapi.RichText {
	var result []notionapi.RichText

	matches := highlightRegex.FindAllStringSubmatchIndex(content, -1)
	if len(matches) == 0 {
		// No highlights, return plain text.
		if content != "" {
			result = append(result, notionapi.RichText{
				Type:        notionapi.ObjectTypeText,
				Text:        &notionapi.Text{Content: content},
				Annotations: copyAnnotations(annotations),
			})
		}
		return result
	}

	lastEnd := 0
	for _, match := range matches {
		// match[0] and match[1] are the full match (==text==)
		// match[2] and match[3] are the capture group (text)
		fullStart, fullEnd := match[0], match[1]
		captureStart, captureEnd := match[2], match[3]

		// Add text before the highlight.
		if fullStart > lastEnd {
			before := content[lastEnd:fullStart]
			result = append(result, notionapi.RichText{
				Type:        notionapi.ObjectTypeText,
				Text:        &notionapi.Text{Content: before},
				Annotations: copyAnnotations(annotations),
			})
		}

		// Add highlighted text.
		highlighted := content[captureStart:captureEnd]
		highlightAnnotations := copyAnnotations(annotations)
		highlightAnnotations.Color = notionapi.ColorYellowBackground
		result = append(result, notionapi.RichText{
			Type:        notionapi.ObjectTypeText,
			Text:        &notionapi.Text{Content: highlighted},
			Annotations: highlightAnnotations,
		})

		lastEnd = fullEnd
	}

	// Add text after the last highlight.
	if lastEnd < len(content) {
		after := content[lastEnd:]
		result = append(result, notionapi.RichText{
			Type:        notionapi.ObjectTypeText,
			Text:        &notionapi.Text{Content: after},
			Annotations: copyAnnotations(annotations),
		})
	}

	return result
}

// transformWikiLink converts an Obsidian wiki-link to a Notion mention or text.
func (t *Transformer) transformWikiLink(target, alias string, annotations *notionapi.Annotations) []notionapi.RichText {
	// Try to resolve the link.
	if t.linkResolver != nil {
		if pageID, found := t.linkResolver.Resolve(target); found {
			// Create page mention.
			return []notionapi.RichText{
				{
					Type: "mention",
					Mention: &notionapi.Mention{
						Type: "page",
						Page: &notionapi.PageMention{
							ID: notionapi.ObjectID(pageID),
						},
					},
					Annotations: copyAnnotations(annotations),
					PlainText:   displayText(target, alias),
				},
			}
		}
	}

	// Unresolved link - handle according to config.
	display := displayText(target, alias)

	switch t.config.UnresolvedLinkStyle {
	case "skip":
		return nil

	case "text":
		return []notionapi.RichText{
			{
				Type:        notionapi.ObjectTypeText,
				Text:        &notionapi.Text{Content: display},
				Annotations: copyAnnotations(annotations),
			},
		}

	default: // "placeholder"
		// Red text to indicate unresolved link, but preserve other formatting.
		placeholderAnnotations := copyAnnotations(annotations)
		placeholderAnnotations.Color = notionapi.ColorRed
		return []notionapi.RichText{
			{
				Type:        notionapi.ObjectTypeText,
				Text:        &notionapi.Text{Content: "[[" + display + "]]"},
				Annotations: placeholderAnnotations,
			},
		}
	}
}

// copyAnnotations creates a copy of annotations to avoid mutation.
func copyAnnotations(a *notionapi.Annotations) *notionapi.Annotations {
	if a == nil {
		return &notionapi.Annotations{}
	}
	return &notionapi.Annotations{
		Bold:          a.Bold,
		Italic:        a.Italic,
		Strikethrough: a.Strikethrough,
		Underline:     a.Underline,
		Code:          a.Code,
		Color:         a.Color,
	}
}

// extractWikilinkAliasFromNode extracts the alias text from a wiki-link node.
// For [[target|alias]], the alias is stored as child text nodes.
func extractWikilinkAliasFromNode(node *wikilink.Node, source []byte) string {
	var alias string
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if text, ok := child.(*ast.Text); ok {
			alias += string(text.Segment.Value(source))
		}
	}
	return alias
}

// displayText returns the display text for a wiki-link.
func displayText(target, alias string) string {
	if alias != "" {
		return alias
	}
	return target
}

// transformInlineDataview converts an inline dataview query to a placeholder.
// Inline dataview queries like `=this.file.name` cannot be executed in Notion,
// so we create a styled placeholder that preserves the original query.
func (t *Transformer) transformInlineDataview(query string, annotations *notionapi.Annotations) []notionapi.RichText {
	// Create a placeholder that shows the query was a dataview expression.
	// Use purple background to distinguish from regular code.
	return []notionapi.RichText{
		{
			Type: notionapi.ObjectTypeText,
			Text: &notionapi.Text{Content: "[dv: " + query + "]"},
			Annotations: &notionapi.Annotations{
				Code:  true,
				Color: notionapi.ColorPurpleBackground,
			},
		},
	}
}

// isImageFile checks if a filename has an image extension.
func isImageFile(filename string) bool {
	lower := strings.ToLower(filename)
	imageExts := []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".bmp", ".ico"}
	for _, ext := range imageExts {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// transformWikiLinkImage converts a wiki-link image embed to rich text.
// Since we can't create block-level images in inline context, we create a placeholder.
func (t *Transformer) transformWikiLinkImage(target, alias string, annotations *notionapi.Annotations) []notionapi.RichText {
	displayText := target
	if alias != "" {
		// Alias might contain dimensions like "300x200", ignore for display.
		if !strings.Contains(alias, "x") && !strings.ContainsAny(alias, "0123456789") {
			displayText = alias
		}
	}

	// Create an inline image placeholder.
	return []notionapi.RichText{
		{
			Type: notionapi.ObjectTypeText,
			Text: &notionapi.Text{Content: "[🖼️ " + displayText + "]"},
			Annotations: &notionapi.Annotations{
				Color: notionapi.ColorGrayBackground,
			},
		},
	}
}

// transformWikiLinkEmbed converts a wiki-link embed (non-image) to rich text.
// This handles ![[note]] or ![[file.pdf]] embeds.
func (t *Transformer) transformWikiLinkEmbed(target, alias string, annotations *notionapi.Annotations) []notionapi.RichText {
	displayText := target
	if alias != "" {
		displayText = alias
	}

	// Determine embed type for icon.
	var icon string
	lower := strings.ToLower(target)
	switch {
	case strings.HasSuffix(lower, ".pdf"):
		icon = "📄"
	case strings.HasSuffix(lower, ".mp3"), strings.HasSuffix(lower, ".wav"), strings.HasSuffix(lower, ".m4a"):
		icon = "🎵"
	case strings.HasSuffix(lower, ".mp4"), strings.HasSuffix(lower, ".webm"), strings.HasSuffix(lower, ".mov"):
		icon = "🎬"
	case strings.HasSuffix(lower, ".excalidraw.md"):
		icon = "✏️"
	default:
		icon = "📎" // Generic embed/note reference.
	}

	return []notionapi.RichText{
		{
			Type: notionapi.ObjectTypeText,
			Text: &notionapi.Text{Content: "[" + icon + " " + displayText + "]"},
			Annotations: &notionapi.Annotations{
				Color: notionapi.ColorBlueBackground,
			},
		},
	}
}
