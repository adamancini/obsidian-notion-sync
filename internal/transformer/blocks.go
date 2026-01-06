package transformer

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/jomei/notionapi"
	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
)

// transformHeading converts a goldmark heading to a Notion heading block.
func (t *Transformer) transformHeading(h *ast.Heading, source []byte) notionapi.Block {
	richText := t.transformInlineContent(h, source)

	switch h.Level {
	case 1:
		return &notionapi.Heading1Block{
			BasicBlock: notionapi.BasicBlock{
				Object: notionapi.ObjectTypeBlock,
				Type:   notionapi.BlockTypeHeading1,
			},
			Heading1: notionapi.Heading{
				RichText: richText,
			},
		}
	case 2:
		return &notionapi.Heading2Block{
			BasicBlock: notionapi.BasicBlock{
				Object: notionapi.ObjectTypeBlock,
				Type:   notionapi.BlockTypeHeading2,
			},
			Heading2: notionapi.Heading{
				RichText: richText,
			},
		}
	default: // 3+ flattened to H3
		return &notionapi.Heading3Block{
			BasicBlock: notionapi.BasicBlock{
				Object: notionapi.ObjectTypeBlock,
				Type:   notionapi.BlockTypeHeading3,
			},
			Heading3: notionapi.Heading{
				RichText: richText,
			},
		}
	}
}

// transformParagraph converts a goldmark paragraph to a Notion paragraph block.
func (t *Transformer) transformParagraph(p *ast.Paragraph, source []byte) notionapi.Block {
	richText := t.transformInlineContent(p, source)

	return &notionapi.ParagraphBlock{
		BasicBlock: notionapi.BasicBlock{
			Object: notionapi.ObjectTypeBlock,
			Type:   notionapi.BlockTypeParagraph,
		},
		Paragraph: notionapi.Paragraph{
			RichText: richText,
		},
	}
}

// tryImageBlock checks if a paragraph contains only an image and returns an ImageBlock.
// Returns nil if the paragraph is not a standalone image.
func (t *Transformer) tryImageBlock(p *ast.Paragraph, source []byte) notionapi.Block {
	// Count children - we want exactly one image.
	var imageNode *ast.Image
	childCount := 0

	for child := p.FirstChild(); child != nil; child = child.NextSibling() {
		childCount++
		if img, ok := child.(*ast.Image); ok {
			imageNode = img
		} else if txt, ok := child.(*ast.Text); ok {
			// Allow whitespace-only text nodes.
			content := strings.TrimSpace(string(txt.Segment.Value(source)))
			if content != "" {
				return nil // Non-whitespace text, not a standalone image.
			}
			childCount-- // Don't count whitespace.
		}
	}

	if imageNode == nil || childCount != 1 {
		return nil
	}

	return t.transformImage(imageNode, source)
}

// transformImage converts an ast.Image to a Notion image block.
func (t *Transformer) transformImage(img *ast.Image, source []byte) notionapi.Block {
	url := string(img.Destination)
	alt := string(img.Text(source))

	// Check if this is a local file path.
	if isLocalPath(url) {
		// For local images without upload support, create a placeholder callout.
		return t.createImagePlaceholder(url, alt, "local")
	}

	// External image - create an ImageBlock.
	var caption []notionapi.RichText
	if alt != "" {
		caption = []notionapi.RichText{
			{
				Type: notionapi.ObjectTypeText,
				Text: &notionapi.Text{Content: alt},
			},
		}
	}

	return &notionapi.ImageBlock{
		BasicBlock: notionapi.BasicBlock{
			Object: notionapi.ObjectTypeBlock,
			Type:   notionapi.BlockTypeImage,
		},
		Image: notionapi.Image{
			Type: "external",
			External: &notionapi.FileObject{
				URL: url,
			},
			Caption: caption,
		},
	}
}

// isLocalPath checks if a URL is a local file path.
func isLocalPath(url string) bool {
	// Data URLs are inline embedded content, not local files.
	if strings.HasPrefix(url, "data:") {
		return false
	}
	// file:// protocol.
	if strings.HasPrefix(url, "file://") {
		return true
	}
	// Relative paths (no protocol).
	if !strings.Contains(url, "://") {
		return true
	}
	return false
}

// createImagePlaceholder creates a callout placeholder for images that can't be embedded.
func (t *Transformer) createImagePlaceholder(path, alt, reason string) notionapi.Block {
	var title, body string
	switch reason {
	case "local":
		title = "Local Image"
		body = "Local image cannot be embedded without upload support."
	case "wikilink":
		title = "Wiki-link Image"
		body = "Wiki-link image reference."
	default:
		title = "Image"
		body = "Image cannot be embedded."
	}

	displayText := path
	if alt != "" {
		displayText = fmt.Sprintf("%s (%s)", alt, path)
	}

	richText := []notionapi.RichText{
		{
			Type: notionapi.ObjectTypeText,
			Text: &notionapi.Text{Content: title},
			Annotations: &notionapi.Annotations{Bold: true},
		},
		{
			Type: notionapi.ObjectTypeText,
			Text: &notionapi.Text{Content: "\n" + body + "\n"},
		},
		{
			Type: notionapi.ObjectTypeText,
			Text: &notionapi.Text{Content: displayText},
			Annotations: &notionapi.Annotations{Code: true},
		},
	}

	emoji := notionapi.Emoji("🖼️")
	return &notionapi.CalloutBlock{
		BasicBlock: notionapi.BasicBlock{
			Object: notionapi.ObjectTypeBlock,
			Type:   "callout",
		},
		Callout: notionapi.Callout{
			RichText: richText,
			Icon:     &notionapi.Icon{Type: "emoji", Emoji: &emoji},
			Color:    "gray_background",
		},
	}
}

// transformListItem converts a goldmark list item to the appropriate Notion block.
func (t *Transformer) transformListItem(li *ast.ListItem, source []byte) notionapi.Block {
	// Get the parent list to determine type.
	parent := li.Parent()
	list, ok := parent.(*ast.List)
	if !ok {
		// Fallback to bullet.
		return t.transformBulletItem(li, source)
	}

	// Check if this is a task list item.
	if isTaskItem(li) {
		return t.transformTaskItem(li, source)
	}

	// Check if ordered or unordered.
	if list.IsOrdered() {
		return t.transformNumberedItem(li, source)
	}

	return t.transformBulletItem(li, source)
}

// transformBulletItem creates a bulleted list item block.
func (t *Transformer) transformBulletItem(li *ast.ListItem, source []byte) notionapi.Block {
	richText := t.transformListItemContent(li, source)
	children := t.extractNestedChildren(li, source)

	return &notionapi.BulletedListItemBlock{
		BasicBlock: notionapi.BasicBlock{
			Object: notionapi.ObjectTypeBlock,
			Type:   notionapi.BlockTypeBulletedListItem,
		},
		BulletedListItem: notionapi.ListItem{
			RichText: richText,
			Children: children,
		},
	}
}

// transformNumberedItem creates a numbered list item block.
func (t *Transformer) transformNumberedItem(li *ast.ListItem, source []byte) notionapi.Block {
	richText := t.transformListItemContent(li, source)
	children := t.extractNestedChildren(li, source)

	return &notionapi.NumberedListItemBlock{
		BasicBlock: notionapi.BasicBlock{
			Object: notionapi.ObjectTypeBlock,
			Type:   notionapi.BlockTypeNumberedListItem,
		},
		NumberedListItem: notionapi.ListItem{
			RichText: richText,
			Children: children,
		},
	}
}

// transformTaskItem creates a to-do block.
func (t *Transformer) transformTaskItem(li *ast.ListItem, source []byte) notionapi.Block {
	richText := t.transformListItemContent(li, source)
	checked := isTaskChecked(li, source)
	children := t.extractNestedChildren(li, source)

	return &notionapi.ToDoBlock{
		BasicBlock: notionapi.BasicBlock{
			Object: notionapi.ObjectTypeBlock,
			Type:   notionapi.BlockTypeToDo,
		},
		ToDo: notionapi.ToDo{
			RichText: richText,
			Checked:  checked,
			Children: children,
		},
	}
}

// notionRichTextMaxLength is the maximum length allowed for a single rich_text content.
// Notion API limits this to 2000 characters.
const notionRichTextMaxLength = 2000

// transformCodeBlock converts a fenced code block to a Notion code block.
func (t *Transformer) transformCodeBlock(cb *ast.FencedCodeBlock, source []byte) notionapi.Block {
	// Get language.
	lang := string(cb.Language(source))
	if lang == "" {
		lang = "plain text"
	}

	// Get code content.
	var content strings.Builder
	lines := cb.Lines()
	for i := 0; i < lines.Len(); i++ {
		line := lines.At(i)
		content.Write(line.Value(source))
	}

	// Trim trailing newline.
	code := strings.TrimSuffix(content.String(), "\n")

	// Split code into chunks if it exceeds the Notion rich_text limit.
	richTextSegments := splitCodeContent(code, notionRichTextMaxLength)

	return &notionapi.CodeBlock{
		BasicBlock: notionapi.BasicBlock{
			Object: notionapi.ObjectTypeBlock,
			Type:   notionapi.BlockTypeCode,
		},
		Code: notionapi.Code{
			Language: lang,
			RichText: richTextSegments,
		},
	}
}

// splitCodeContent splits code content into multiple rich_text segments,
// each not exceeding maxLen characters. Prefers splitting at newline boundaries.
func splitCodeContent(code string, maxLen int) []notionapi.RichText {
	if len(code) <= maxLen {
		return []notionapi.RichText{
			{
				Type: notionapi.ObjectTypeText,
				Text: &notionapi.Text{Content: code},
			},
		}
	}

	var segments []notionapi.RichText
	remaining := code

	for len(remaining) > 0 {
		if len(remaining) <= maxLen {
			// Last segment fits entirely.
			segments = append(segments, notionapi.RichText{
				Type: notionapi.ObjectTypeText,
				Text: &notionapi.Text{Content: remaining},
			})
			break
		}

		// Find a good split point - prefer newline boundaries.
		splitPoint := findSplitPoint(remaining, maxLen)
		chunk := remaining[:splitPoint]
		remaining = remaining[splitPoint:]

		segments = append(segments, notionapi.RichText{
			Type: notionapi.ObjectTypeText,
			Text: &notionapi.Text{Content: chunk},
		})
	}

	return segments
}

// findSplitPoint finds the best position to split the text, preferring newlines.
// Returns a position <= maxLen.
func findSplitPoint(text string, maxLen int) int {
	if len(text) <= maxLen {
		return len(text)
	}

	// Look for the last newline within the maxLen range.
	lastNewline := strings.LastIndex(text[:maxLen], "\n")
	if lastNewline > 0 {
		// Include the newline in this chunk.
		return lastNewline + 1
	}

	// No newline found, split at maxLen.
	return maxLen
}

// transformQuote converts a blockquote to a Notion quote block.
func (t *Transformer) transformQuote(bq *ast.Blockquote, source []byte) notionapi.Block {
	richText := t.transformBlockquoteContent(bq, source)

	return &notionapi.QuoteBlock{
		BasicBlock: notionapi.BasicBlock{
			Object: notionapi.ObjectTypeBlock,
			Type:   "quote",
		},
		Quote: notionapi.Quote{
			RichText: richText,
		},
	}
}

// calloutRegex matches Obsidian callout syntax: [!type] or [!type]+ or [!type]- with optional title.
var calloutRegex = regexp.MustCompile(`^\[!(\w+)\]([+-])?(.*)$`)

// tryCallout attempts to parse a blockquote as an Obsidian callout.
// Returns nil if it's not a callout.
func (t *Transformer) tryCallout(bq *ast.Blockquote, source []byte) notionapi.Block {
	// Get first line of blockquote content.
	firstLine := getBlockquoteFirstLine(bq, source)

	// Check for callout syntax: > [!type] or > [!type] title
	matches := calloutRegex.FindStringSubmatch(firstLine)
	if matches == nil {
		return nil
	}

	calloutType := strings.ToLower(matches[1])
	// matches[2] is the fold indicator (+ or -), we ignore it for Notion
	title := strings.TrimSpace(matches[3])

	// Get icon for this callout type.
	icon := t.config.CalloutIcons[calloutType]
	if icon == "" {
		icon = "💡" // Default icon
	}

	// Get remaining content, skipping the first line (callout marker).
	content := t.transformCalloutContent(bq, source)

	// Build callout block.
	var richText []notionapi.RichText

	// Add title with bold formatting if present.
	if title != "" {
		richText = append(richText, notionapi.RichText{
			Type: notionapi.ObjectTypeText,
			Text: &notionapi.Text{Content: title},
			Annotations: &notionapi.Annotations{
				Bold: true,
			},
		})
		// Add newline separator if there's content after the title.
		if len(content) > 0 {
			richText = append(richText, notionapi.RichText{
				Type: notionapi.ObjectTypeText,
				Text: &notionapi.Text{Content: "\n"},
			})
		}
	}

	richText = append(richText, content...)

	emoji := notionapi.Emoji(icon)
	return &notionapi.CalloutBlock{
		BasicBlock: notionapi.BasicBlock{
			Object: notionapi.ObjectTypeBlock,
			Type:   "callout",
		},
		Callout: notionapi.Callout{
			RichText: richText,
			Icon: &notionapi.Icon{
				Type:  "emoji",
				Emoji: &emoji,
			},
		},
	}
}

// transformDivider creates a divider block.
func (t *Transformer) transformDivider() notionapi.Block {
	return &notionapi.DividerBlock{
		BasicBlock: notionapi.BasicBlock{
			Object: notionapi.ObjectTypeBlock,
			Type:   notionapi.BlockTypeDivider,
		},
	}
}

// transformTable converts a goldmark table to a Notion table block.
func (t *Transformer) transformTable(table *extast.Table, source []byte) notionapi.Block {
	// Count columns by looking at the header row.
	columnCount := 0
	hasHeader := false

	// First pass: count columns from header.
	for row := table.FirstChild(); row != nil; row = row.NextSibling() {
		if header, ok := row.(*extast.TableHeader); ok {
			hasHeader = true
			for cell := header.FirstChild(); cell != nil; cell = cell.NextSibling() {
				columnCount++
			}
			break
		}
	}

	// If no header, count from first row.
	if columnCount == 0 {
		for row := table.FirstChild(); row != nil; row = row.NextSibling() {
			if tableRow, ok := row.(*extast.TableRow); ok {
				for cell := tableRow.FirstChild(); cell != nil; cell = cell.NextSibling() {
					columnCount++
				}
				break
			}
		}
	}

	// Ensure at least 1 column.
	if columnCount == 0 {
		columnCount = 1
	}

	// Build table rows.
	var rows []notionapi.TableRow

	for row := table.FirstChild(); row != nil; row = row.NextSibling() {
		switch r := row.(type) {
		case *extast.TableHeader:
			tableRow := t.transformTableRow(r, source, columnCount)
			rows = append(rows, tableRow)

		case *extast.TableRow:
			tableRow := t.transformTableRow(r, source, columnCount)
			rows = append(rows, tableRow)
		}
	}

	return &notionapi.TableBlock{
		BasicBlock: notionapi.BasicBlock{
			Object: notionapi.ObjectTypeBlock,
			Type:   notionapi.BlockTypeTableBlock,
		},
		Table: notionapi.Table{
			TableWidth:      columnCount,
			HasColumnHeader: hasHeader,
			HasRowHeader:    false,
			Children:        buildTableRowBlocks(rows),
		},
	}
}

// transformTableRow converts a table row (header or body) to Notion table row data.
func (t *Transformer) transformTableRow(row ast.Node, source []byte, expectedColumns int) notionapi.TableRow {
	var cells [][]notionapi.RichText

	for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
		if tableCell, ok := cell.(*extast.TableCell); ok {
			cellContent := t.transformInlineContent(tableCell, source)
			cells = append(cells, cellContent)
		}
	}

	// Pad with empty cells if needed.
	for len(cells) < expectedColumns {
		cells = append(cells, []notionapi.RichText{})
	}

	return notionapi.TableRow{
		Cells: cells,
	}
}

// buildTableRowBlocks converts TableRow data to TableRowBlock slices.
func buildTableRowBlocks(rows []notionapi.TableRow) []notionapi.Block {
	blocks := make([]notionapi.Block, len(rows))
	for i, row := range rows {
		blocks[i] = &notionapi.TableRowBlock{
			BasicBlock: notionapi.BasicBlock{
				Object: notionapi.ObjectTypeBlock,
				Type:   notionapi.BlockTypeTableRowBlock,
			},
			TableRow: row,
		}
	}
	return blocks
}

// transformEquation converts a math block to a Notion equation block.
func (t *Transformer) transformEquation(expression string) notionapi.Block {
	return &notionapi.EquationBlock{
		BasicBlock: notionapi.BasicBlock{
			Object: notionapi.ObjectTypeBlock,
			Type:   notionapi.BlockTypeEquation,
		},
		Equation: notionapi.Equation{
			Expression: expression,
		},
	}
}

// transformDataviewBlock converts a dataview code block to a Notion callout placeholder.
// Since Notion cannot execute dataview queries, we create an informational callout
// that preserves the original query for reference.
func (t *Transformer) transformDataviewBlock(cb *ast.FencedCodeBlock, source []byte) notionapi.Block {
	// Get language to distinguish dataview vs dataviewjs.
	lang := string(cb.Language(source))
	isJS := lang == "dataviewjs"

	// Get query content.
	var content strings.Builder
	lines := cb.Lines()
	for i := 0; i < lines.Len(); i++ {
		line := lines.At(i)
		content.Write(line.Value(source))
	}
	query := strings.TrimSuffix(content.String(), "\n")

	// Build the callout content.
	var titleText, bodyText string
	if isJS {
		titleText = "Dataview JS Query"
		bodyText = "This JavaScript query requires Obsidian to execute:"
	} else {
		titleText = "Dataview Query"
		bodyText = "This query requires Obsidian to execute:"
	}

	// Create rich text content: title (bold) + newline + description + newline + query (code).
	richText := []notionapi.RichText{
		{
			Type: notionapi.ObjectTypeText,
			Text: &notionapi.Text{Content: titleText},
			Annotations: &notionapi.Annotations{
				Bold: true,
			},
		},
		{
			Type: notionapi.ObjectTypeText,
			Text: &notionapi.Text{Content: "\n" + bodyText + "\n\n"},
		},
		{
			Type: notionapi.ObjectTypeText,
			Text: &notionapi.Text{Content: query},
			Annotations: &notionapi.Annotations{
				Code: true,
			},
		},
	}

	// Use info icon for dataview placeholders.
	emoji := notionapi.Emoji("📊")

	return &notionapi.CalloutBlock{
		BasicBlock: notionapi.BasicBlock{
			Object: notionapi.ObjectTypeBlock,
			Type:   "callout",
		},
		Callout: notionapi.Callout{
			RichText: richText,
			Icon: &notionapi.Icon{
				Type:  "emoji",
				Emoji: &emoji,
			},
			Color: "blue_background",
		},
	}
}

// Helper functions

// extractNestedChildren extracts nested list items from a list item's children.
// In goldmark's AST, a list item can have nested lists as children (after the TextBlock/Paragraph).
// This function finds those nested lists and recursively transforms their items into Notion blocks.
func (t *Transformer) extractNestedChildren(li *ast.ListItem, source []byte) notionapi.Blocks {
	var children notionapi.Blocks

	// Walk through children of the list item looking for nested lists.
	for child := li.FirstChild(); child != nil; child = child.NextSibling() {
		if nestedList, ok := child.(*ast.List); ok {
			// Transform each item in the nested list.
			for item := nestedList.FirstChild(); item != nil; item = item.NextSibling() {
				if nestedItem, ok := item.(*ast.ListItem); ok {
					block := t.transformListItem(nestedItem, source)
					if block != nil {
						children = append(children, block)
					}
				}
			}
		}
	}

	return children
}

// isTaskItem checks if a list item is a task (checkbox) item.
// Task items have a TaskCheckBox node as the first child of their content.
func isTaskItem(li *ast.ListItem) bool {
	// Walk through children to find TaskCheckBox.
	for child := li.FirstChild(); child != nil; child = child.NextSibling() {
		// Check the first inline-level child of block-level content.
		for inner := child.FirstChild(); inner != nil; inner = inner.NextSibling() {
			if _, ok := inner.(*extast.TaskCheckBox); ok {
				return true
			}
		}
	}
	return false
}

// isTaskChecked checks if a task item is checked.
func isTaskChecked(li *ast.ListItem, source []byte) bool {
	// Walk through children to find TaskCheckBox.
	for child := li.FirstChild(); child != nil; child = child.NextSibling() {
		for inner := child.FirstChild(); inner != nil; inner = inner.NextSibling() {
			if cb, ok := inner.(*extast.TaskCheckBox); ok {
				return cb.IsChecked
			}
		}
	}
	return false
}

// transformListItemContent extracts rich text from a list item's first paragraph.
// Returns an empty slice (not nil) if the list item has no content.
func (t *Transformer) transformListItemContent(li *ast.ListItem, source []byte) []notionapi.RichText {
	// List items typically contain a TextBlock or Paragraph as first child.
	for child := li.FirstChild(); child != nil; child = child.NextSibling() {
		switch c := child.(type) {
		case *ast.TextBlock:
			result := t.transformInlineContent(c, source)
			if result == nil {
				return []notionapi.RichText{}
			}
			return result
		case *ast.Paragraph:
			result := t.transformInlineContent(c, source)
			if result == nil {
				return []notionapi.RichText{}
			}
			return result
		}
	}
	// Return empty slice, not nil - Notion API requires array, not null.
	return []notionapi.RichText{}
}

// transformBlockquoteContent extracts content from a blockquote.
func (t *Transformer) transformBlockquoteContent(bq *ast.Blockquote, source []byte) []notionapi.RichText {
	var result []notionapi.RichText

	for child := bq.FirstChild(); child != nil; child = child.NextSibling() {
		if p, ok := child.(*ast.Paragraph); ok {
			result = append(result, t.transformInlineContent(p, source)...)
		}
	}

	return result
}

// transformCalloutContent extracts content from a callout blockquote,
// skipping the first line which contains the callout marker.
func (t *Transformer) transformCalloutContent(bq *ast.Blockquote, source []byte) []notionapi.RichText {
	var result []notionapi.RichText
	isFirst := true

	for child := bq.FirstChild(); child != nil; child = child.NextSibling() {
		if p, ok := child.(*ast.Paragraph); ok {
			if isFirst {
				// Skip content before the first newline in the first paragraph.
				// The callout marker is on the first line.
				isFirst = false
				result = append(result, t.transformCalloutParagraph(p, source)...)
			} else {
				result = append(result, t.transformInlineContent(p, source)...)
			}
		}
	}

	return result
}

// transformCalloutParagraph transforms a paragraph, skipping the first line.
func (t *Transformer) transformCalloutParagraph(p *ast.Paragraph, source []byte) []notionapi.RichText {
	var result []notionapi.RichText
	skippedFirstLine := false

	for child := p.FirstChild(); child != nil; child = child.NextSibling() {
		if !skippedFirstLine {
			// Skip until we hit a soft/hard line break or run out of text nodes.
			if txt, ok := child.(*ast.Text); ok {
				if txt.SoftLineBreak() || txt.HardLineBreak() {
					skippedFirstLine = true
					continue
				}
				// Skip this text node as it's part of the first line.
				continue
			}
		}

		// Process remaining content.
		result = append(result, t.transformInline(child, source, nil)...)
	}

	return result
}

// getBlockquoteFirstLine extracts the first line of text from a blockquote.
// It concatenates all text segments until a line break is encountered.
func getBlockquoteFirstLine(bq *ast.Blockquote, source []byte) string {
	var result strings.Builder
	for child := bq.FirstChild(); child != nil; child = child.NextSibling() {
		if p, ok := child.(*ast.Paragraph); ok {
			for inline := p.FirstChild(); inline != nil; inline = inline.NextSibling() {
				if t, ok := inline.(*ast.Text); ok {
					result.Write(t.Segment.Value(source))
					// Stop at line break - we only want the first line.
					if t.SoftLineBreak() || t.HardLineBreak() {
						return result.String()
					}
				}
			}
			// If no line break found, return what we have (single line blockquote).
			if result.Len() > 0 {
				return result.String()
			}
		}
	}
	return result.String()
}
