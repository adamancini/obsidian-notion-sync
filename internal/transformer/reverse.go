package transformer

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/jomei/notionapi"
)

// PathLookup resolves Notion page IDs back to Obsidian paths.
type PathLookup interface {
	// LookupPath returns the Obsidian path for a Notion page ID.
	LookupPath(notionPageID string) (obsidianPath string, found bool)
}

// ReverseTransformer converts Notion pages back to Obsidian-flavored markdown.
type ReverseTransformer struct {
	pathLookup PathLookup
	config     *Config
}

// NewReverse creates a new ReverseTransformer.
func NewReverse(lookup PathLookup, cfg *Config) *ReverseTransformer {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &ReverseTransformer{
		pathLookup: lookup,
		config:     cfg,
	}
}

// NotionToMarkdown converts a Notion page to Obsidian-flavored markdown.
func (t *ReverseTransformer) NotionToMarkdown(page *NotionPage) ([]byte, error) {
	var buf bytes.Buffer

	// 1. Convert properties to frontmatter.
	frontmatter := t.propertiesToFrontmatter(page.Properties)
	if len(frontmatter) > 0 {
		buf.WriteString("---\n")
		for key, value := range frontmatter {
			buf.WriteString(fmt.Sprintf("%s: %v\n", key, value))
		}
		buf.WriteString("---\n\n")
	}

	// 2. Convert blocks to markdown.
	for _, block := range page.Children {
		md := t.blockToMarkdown(block, 0)
		buf.WriteString(md)
	}

	return buf.Bytes(), nil
}

// blockToMarkdown converts a Notion block to markdown with proper indentation.
func (t *ReverseTransformer) blockToMarkdown(block notionapi.Block, depth int) string {
	indent := strings.Repeat("  ", depth)

	// TODO: Implement full block conversion
	// This is a stub that handles the basic cases

	switch b := block.(type) {
	case *notionapi.Heading1Block:
		return "# " + t.richTextToMarkdown(b.Heading1.RichText) + "\n\n"

	case *notionapi.Heading2Block:
		return "## " + t.richTextToMarkdown(b.Heading2.RichText) + "\n\n"

	case *notionapi.Heading3Block:
		return "### " + t.richTextToMarkdown(b.Heading3.RichText) + "\n\n"

	case *notionapi.ParagraphBlock:
		text := t.richTextToMarkdown(b.Paragraph.RichText)
		if text == "" {
			return "\n"
		}
		return text + "\n\n"

	case *notionapi.BulletedListItemBlock:
		text := t.richTextToMarkdown(b.BulletedListItem.RichText)
		result := indent + "- " + text + "\n"
		// TODO: Handle nested children
		return result

	case *notionapi.NumberedListItemBlock:
		text := t.richTextToMarkdown(b.NumberedListItem.RichText)
		result := indent + "1. " + text + "\n"
		// TODO: Handle nested children
		return result

	case *notionapi.ToDoBlock:
		checkbox := "[ ]"
		if b.ToDo.Checked {
			checkbox = "[x]"
		}
		text := t.richTextToMarkdown(b.ToDo.RichText)
		return indent + "- " + checkbox + " " + text + "\n"

	case *notionapi.QuoteBlock:
		text := t.richTextToMarkdown(b.Quote.RichText)
		lines := strings.Split(text, "\n")
		var result strings.Builder
		for _, line := range lines {
			result.WriteString("> " + line + "\n")
		}
		result.WriteString("\n")
		return result.String()

	case *notionapi.CalloutBlock:
		icon := ""
		if b.Callout.Icon != nil && b.Callout.Icon.Emoji != nil {
			icon = string(*b.Callout.Icon.Emoji)
		}
		calloutType := t.iconToCalloutType(icon)
		text := t.richTextToMarkdown(b.Callout.RichText)
		return fmt.Sprintf("> [!%s]\n> %s\n\n", calloutType, text)

	case *notionapi.CodeBlock:
		lang := string(b.Code.Language)
		code := t.richTextToMarkdown(b.Code.RichText)
		return fmt.Sprintf("```%s\n%s\n```\n\n", lang, code)

	case *notionapi.DividerBlock:
		return "---\n\n"

	case *notionapi.EquationBlock:
		return fmt.Sprintf("$$\n%s\n$$\n\n", b.Equation.Expression)

	case *notionapi.ImageBlock:
		url := ""
		if b.Image.File != nil {
			url = b.Image.File.URL
		} else if b.Image.External != nil {
			url = b.Image.External.URL
		}
		caption := t.richTextToMarkdown(b.Image.Caption)
		if caption != "" {
			return fmt.Sprintf("![%s](%s)\n\n", caption, url)
		}
		return fmt.Sprintf("![](%s)\n\n", url)

	default:
		// Unknown block type, skip or add comment
		return ""
	}
}

// richTextToMarkdown converts Notion rich text to markdown.
func (t *ReverseTransformer) richTextToMarkdown(richText []notionapi.RichText) string {
	var result strings.Builder

	for _, rt := range richText {
		text := rt.PlainText

		// Handle mentions (convert back to wiki-links).
		if rt.Type == "mention" && rt.Mention != nil {
			if rt.Mention.Type == "page" && rt.Mention.Page != nil {
				pageID := string(rt.Mention.Page.ID)
				if t.pathLookup != nil {
					if path, found := t.pathLookup.LookupPath(pageID); found {
						text = "[[" + path + "]]"
					} else {
						text = "[[" + rt.PlainText + "]]"
					}
				} else {
					text = "[[" + rt.PlainText + "]]"
				}
				result.WriteString(text)
				continue
			}
		}

		// Apply annotations.
		if rt.Annotations != nil {
			if rt.Annotations.Code {
				text = "`" + text + "`"
			}
			if rt.Annotations.Strikethrough {
				text = "~~" + text + "~~"
			}
			if rt.Annotations.Italic {
				text = "*" + text + "*"
			}
			if rt.Annotations.Bold {
				text = "**" + text + "**"
			}
			// Handle highlight (yellow background = Obsidian highlight)
			if rt.Annotations.Color == notionapi.ColorYellowBackground {
				text = "==" + text + "=="
			}
		}

		// Handle links.
		if rt.Text != nil && rt.Text.Link != nil {
			text = "[" + text + "](" + rt.Text.Link.Url + ")"
		}

		result.WriteString(text)
	}

	return result.String()
}

// iconToCalloutType maps Notion icons back to Obsidian callout types.
func (t *ReverseTransformer) iconToCalloutType(icon string) string {
	// Reverse lookup in callout icons map.
	for calloutType, calloutIcon := range t.config.CalloutIcons {
		if calloutIcon == icon {
			return calloutType
		}
	}
	// Default to "note" if icon not found.
	return "note"
}

// propertiesToFrontmatter converts Notion properties to frontmatter map.
func (t *ReverseTransformer) propertiesToFrontmatter(props notionapi.Properties) map[string]any {
	frontmatter := make(map[string]any)

	// TODO: Implement property-to-frontmatter conversion
	// This should reverse the PropertyMapping to convert Notion properties
	// back to Obsidian frontmatter fields

	for name, prop := range props {
		switch p := prop.(type) {
		case *notionapi.TitleProperty:
			if len(p.Title) > 0 {
				frontmatter["title"] = p.Title[0].PlainText
			}

		case *notionapi.MultiSelectProperty:
			var tags []string
			for _, opt := range p.MultiSelect {
				tags = append(tags, opt.Name)
			}
			if len(tags) > 0 {
				frontmatter[strings.ToLower(name)] = tags
			}

		case *notionapi.SelectProperty:
			if p.Select.Name != "" {
				frontmatter[strings.ToLower(name)] = p.Select.Name
			}

		case *notionapi.DateProperty:
			if p.Date != nil && p.Date.Start != nil {
				frontmatter[strings.ToLower(name)] = p.Date.Start.String()
			}

		case *notionapi.CheckboxProperty:
			frontmatter[strings.ToLower(name)] = p.Checkbox

		case *notionapi.NumberProperty:
			if p.Number != 0 {
				frontmatter[strings.ToLower(name)] = p.Number
			}

		case *notionapi.RichTextProperty:
			if len(p.RichText) > 0 {
				frontmatter[strings.ToLower(name)] = p.RichText[0].PlainText
			}
		}
	}

	return frontmatter
}
