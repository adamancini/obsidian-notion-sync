package transformer

import (
	"bytes"
	"fmt"
	"sort"
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

// Transform converts Notion blocks to Obsidian-flavored markdown.
// This is the main entry point for block-level conversion.
func (rt *ReverseTransformer) Transform(blocks []notionapi.Block) (string, error) {
	var buf bytes.Buffer

	for _, block := range blocks {
		md := rt.blockToMarkdown(block, 0)
		buf.WriteString(md)
	}

	return buf.String(), nil
}

// TransformRichText converts Notion rich text to markdown string.
// This is the main entry point for inline text conversion.
func (rt *ReverseTransformer) TransformRichText(richText []notionapi.RichText) string {
	return rt.richTextToMarkdown(richText)
}

// NotionToMarkdown converts a Notion page to Obsidian-flavored markdown.
func (t *ReverseTransformer) NotionToMarkdown(page *NotionPage) ([]byte, error) {
	var buf bytes.Buffer

	// 1. Convert properties to frontmatter.
	frontmatter := t.propertiesToFrontmatter(page.Properties)
	if len(frontmatter) > 0 {
		buf.WriteString("---\n")
		// Sort keys for deterministic output.
		keys := make([]string, 0, len(frontmatter))
		for key := range frontmatter {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			buf.WriteString(fmt.Sprintf("%s: %v\n", key, frontmatter[key]))
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
		result := indent + text + "\n\n"
		// Handle nested children.
		result += t.transformChildren(b.Paragraph.Children, depth+1)
		return result

	case *notionapi.BulletedListItemBlock:
		text := t.richTextToMarkdown(b.BulletedListItem.RichText)
		result := indent + "- " + text + "\n"
		// Handle nested children.
		result += t.transformChildren(b.BulletedListItem.Children, depth+1)
		return result

	case *notionapi.NumberedListItemBlock:
		text := t.richTextToMarkdown(b.NumberedListItem.RichText)
		result := indent + "1. " + text + "\n"
		// Handle nested children.
		result += t.transformChildren(b.NumberedListItem.Children, depth+1)
		return result

	case *notionapi.ToDoBlock:
		checkbox := "[ ]"
		if b.ToDo.Checked {
			checkbox = "[x]"
		}
		text := t.richTextToMarkdown(b.ToDo.RichText)
		result := indent + "- " + checkbox + " " + text + "\n"
		// Handle nested children.
		result += t.transformChildren(b.ToDo.Children, depth+1)
		return result

	case *notionapi.QuoteBlock:
		text := t.richTextToMarkdown(b.Quote.RichText)
		lines := strings.Split(text, "\n")
		var result strings.Builder
		for _, line := range lines {
			result.WriteString(indent + "> " + line + "\n")
		}
		// Handle nested children.
		childMd := t.transformChildren(b.Quote.Children, depth)
		if childMd != "" {
			// Prefix each line with > for quote context.
			childLines := strings.Split(strings.TrimSuffix(childMd, "\n"), "\n")
			for _, line := range childLines {
				result.WriteString(indent + "> " + line + "\n")
			}
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
		// Format callout text: wrap lines with > prefix.
		lines := strings.Split(text, "\n")
		var result strings.Builder
		result.WriteString(fmt.Sprintf("%s> [!%s]\n", indent, calloutType))
		for _, line := range lines {
			if line != "" {
				result.WriteString(fmt.Sprintf("%s> %s\n", indent, line))
			}
		}
		// Handle nested children.
		childMd := t.transformChildren(b.Callout.Children, depth)
		if childMd != "" {
			childLines := strings.Split(strings.TrimSuffix(childMd, "\n"), "\n")
			for _, line := range childLines {
				result.WriteString(indent + "> " + line + "\n")
			}
		}
		result.WriteString("\n")
		return result.String()

	case *notionapi.CodeBlock:
		lang := string(b.Code.Language)
		if lang == "plain text" {
			lang = ""
		}
		code := t.richTextToPlainText(b.Code.RichText)
		return fmt.Sprintf("%s```%s\n%s\n```\n\n", indent, lang, code)

	case *notionapi.DividerBlock:
		return indent + "---\n\n"

	case *notionapi.EquationBlock:
		return fmt.Sprintf("%s$$\n%s\n$$\n\n", indent, b.Equation.Expression)

	case *notionapi.ImageBlock:
		url := ""
		if b.Image.File != nil {
			url = b.Image.File.URL
		} else if b.Image.External != nil {
			url = b.Image.External.URL
		}
		caption := t.richTextToMarkdown(b.Image.Caption)
		if caption != "" {
			return fmt.Sprintf("%s![%s](%s)\n\n", indent, caption, url)
		}
		return fmt.Sprintf("%s![](%s)\n\n", indent, url)

	case *notionapi.TableBlock:
		return t.tableToMarkdown(b, depth)

	case *notionapi.TableRowBlock:
		// Table rows are handled by tableToMarkdown, skip here.
		return ""

	case *notionapi.ToggleBlock:
		text := t.richTextToMarkdown(b.Toggle.RichText)
		result := fmt.Sprintf("%s- %s\n", indent, text)
		// Handle nested children.
		result += t.transformChildren(b.Toggle.Children, depth+1)
		return result

	case *notionapi.BookmarkBlock:
		url := ""
		if b.Bookmark.URL != "" {
			url = b.Bookmark.URL
		}
		caption := t.richTextToMarkdown(b.Bookmark.Caption)
		if caption != "" {
			return fmt.Sprintf("%s[%s](%s)\n\n", indent, caption, url)
		}
		return fmt.Sprintf("%s<%s>\n\n", indent, url)

	case *notionapi.EmbedBlock:
		return fmt.Sprintf("%s<%s>\n\n", indent, b.Embed.URL)

	case *notionapi.VideoBlock:
		url := ""
		if b.Video.File != nil {
			url = b.Video.File.URL
		} else if b.Video.External != nil {
			url = b.Video.External.URL
		}
		return fmt.Sprintf("%s![video](%s)\n\n", indent, url)

	case *notionapi.FileBlock:
		url := ""
		if b.File.File != nil {
			url = b.File.File.URL
		} else if b.File.External != nil {
			url = b.File.External.URL
		}
		caption := t.richTextToMarkdown(b.File.Caption)
		if caption != "" {
			return fmt.Sprintf("%s[%s](%s)\n\n", indent, caption, url)
		}
		return fmt.Sprintf("%s[file](%s)\n\n", indent, url)

	case *notionapi.PdfBlock:
		url := ""
		if b.Pdf.File != nil {
			url = b.Pdf.File.URL
		} else if b.Pdf.External != nil {
			url = b.Pdf.External.URL
		}
		return fmt.Sprintf("%s[PDF](%s)\n\n", indent, url)

	default:
		// Unknown block type, skip.
		return ""
	}
}

// transformChildren recursively transforms child blocks.
func (t *ReverseTransformer) transformChildren(children []notionapi.Block, depth int) string {
	if len(children) == 0 {
		return ""
	}

	var result strings.Builder
	for _, child := range children {
		result.WriteString(t.blockToMarkdown(child, depth))
	}
	return result.String()
}

// tableToMarkdown converts a Notion table block to markdown table format.
func (t *ReverseTransformer) tableToMarkdown(table *notionapi.TableBlock, depth int) string {
	indent := strings.Repeat("  ", depth)
	var result strings.Builder

	if len(table.Table.Children) == 0 {
		return ""
	}

	// Process table rows.
	for i, child := range table.Table.Children {
		row, ok := child.(*notionapi.TableRowBlock)
		if !ok {
			continue
		}

		// Build row content.
		result.WriteString(indent + "|")
		for _, cell := range row.TableRow.Cells {
			cellContent := t.richTextToMarkdown(cell)
			// Escape pipe characters in cell content.
			cellContent = strings.ReplaceAll(cellContent, "|", "\\|")
			result.WriteString(" " + cellContent + " |")
		}
		result.WriteString("\n")

		// Add separator after header row (if table has column header).
		if i == 0 && table.Table.HasColumnHeader {
			result.WriteString(indent + "|")
			for range row.TableRow.Cells {
				result.WriteString(" --- |")
			}
			result.WriteString("\n")
		}
	}

	result.WriteString("\n")
	return result.String()
}

// richTextToPlainText extracts plain text from rich text without markdown formatting.
// Used for code blocks where we don't want to apply formatting.
func (t *ReverseTransformer) richTextToPlainText(richText []notionapi.RichText) string {
	var result strings.Builder

	for _, rt := range richText {
		result.WriteString(rt.PlainText)
	}

	return result.String()
}

// richTextToMarkdown converts Notion rich text to markdown.
func (t *ReverseTransformer) richTextToMarkdown(richText []notionapi.RichText) string {
	var result strings.Builder

	for _, rt := range richText {
		text := rt.PlainText

		// Handle equations (inline math).
		if rt.Type == "equation" && rt.Equation != nil {
			result.WriteString("$" + rt.Equation.Expression + "$")
			continue
		}

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
			// Handle date mentions.
			if rt.Mention.Type == "date" && rt.Mention.Date != nil {
				result.WriteString(rt.PlainText)
				continue
			}
			// Handle user mentions.
			if rt.Mention.Type == "user" && rt.Mention.User != nil {
				result.WriteString("@" + rt.PlainText)
				continue
			}
		}

		// Apply annotations in the correct order.
		// Order matters: innermost first, then outer wrappers.
		if rt.Annotations != nil {
			// Handle highlight (yellow background = Obsidian highlight).
			// This should wrap the content before other formatting.
			if rt.Annotations.Color == notionapi.ColorYellowBackground {
				text = "==" + text + "=="
			}

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
		}

		// Handle links (external URLs).
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
