package transformer

import (
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

	return &notionapi.BulletedListItemBlock{
		BasicBlock: notionapi.BasicBlock{
			Object: notionapi.ObjectTypeBlock,
			Type:   notionapi.BlockTypeBulletedListItem,
		},
		BulletedListItem: notionapi.ListItem{
			RichText: richText,
			// TODO: Handle nested children
		},
	}
}

// transformNumberedItem creates a numbered list item block.
func (t *Transformer) transformNumberedItem(li *ast.ListItem, source []byte) notionapi.Block {
	richText := t.transformListItemContent(li, source)

	return &notionapi.NumberedListItemBlock{
		BasicBlock: notionapi.BasicBlock{
			Object: notionapi.ObjectTypeBlock,
			Type:   notionapi.BlockTypeNumberedListItem,
		},
		NumberedListItem: notionapi.ListItem{
			RichText: richText,
			// TODO: Handle nested children
		},
	}
}

// transformTaskItem creates a to-do block.
func (t *Transformer) transformTaskItem(li *ast.ListItem, source []byte) notionapi.Block {
	richText := t.transformListItemContent(li, source)
	checked := isTaskChecked(li, source)

	return &notionapi.ToDoBlock{
		BasicBlock: notionapi.BasicBlock{
			Object: notionapi.ObjectTypeBlock,
			Type:   notionapi.BlockTypeToDo,
		},
		ToDo: notionapi.ToDo{
			RichText: richText,
			Checked:  checked,
		},
	}
}

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

	return &notionapi.CodeBlock{
		BasicBlock: notionapi.BasicBlock{
			Object: notionapi.ObjectTypeBlock,
			Type:   notionapi.BlockTypeCode,
		},
		Code: notionapi.Code{
			Language: lang,
			RichText: []notionapi.RichText{
				{
					Type: notionapi.ObjectTypeText,
					Text: &notionapi.Text{Content: code},
				},
			},
		},
	}
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

// Helper functions

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
func (t *Transformer) transformListItemContent(li *ast.ListItem, source []byte) []notionapi.RichText {
	// List items typically contain a TextBlock or Paragraph as first child.
	for child := li.FirstChild(); child != nil; child = child.NextSibling() {
		switch c := child.(type) {
		case *ast.TextBlock:
			return t.transformInlineContent(c, source)
		case *ast.Paragraph:
			return t.transformInlineContent(c, source)
		}
	}
	return nil
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
