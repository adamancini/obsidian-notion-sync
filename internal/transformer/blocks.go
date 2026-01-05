package transformer

import (
	"strings"

	"github.com/jomei/notionapi"
	"github.com/yuin/goldmark/ast"
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

// tryCallout attempts to parse a blockquote as an Obsidian callout.
// Returns nil if it's not a callout.
func (t *Transformer) tryCallout(bq *ast.Blockquote, source []byte) notionapi.Block {
	// Get first line of blockquote content.
	firstLine := getBlockquoteFirstLine(bq, source)

	// Check for callout syntax: > [!type] or > [!type] title
	if !strings.HasPrefix(firstLine, "[!") {
		return nil
	}

	// Extract callout type.
	endBracket := strings.Index(firstLine, "]")
	if endBracket == -1 {
		return nil
	}

	calloutType := strings.ToLower(firstLine[2:endBracket])
	title := strings.TrimSpace(firstLine[endBracket+1:])

	// Get icon for this callout type.
	icon := t.config.CalloutIcons[calloutType]
	if icon == "" {
		icon = "💡" // Default icon
	}

	// Get remaining content.
	content := t.transformBlockquoteContent(bq, source)
	// TODO: Strip the first line (callout marker) from content

	// Build callout block.
	var titleText []notionapi.RichText
	if title != "" {
		titleText = []notionapi.RichText{
			{
				Type: notionapi.ObjectTypeText,
				Text: &notionapi.Text{Content: title},
				Annotations: &notionapi.Annotations{
					Bold: true,
				},
			},
		}
	}

	emoji := notionapi.Emoji(icon)
	return &notionapi.CalloutBlock{
		BasicBlock: notionapi.BasicBlock{
			Object: notionapi.ObjectTypeBlock,
			Type:   "callout",
		},
		Callout: notionapi.Callout{
			RichText: append(titleText, content...),
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

// Helper functions

// isTaskItem checks if a list item is a task (checkbox) item.
func isTaskItem(li *ast.ListItem) bool {
	// Check for task list item marker.
	if tc := li.FirstChild(); tc != nil {
		if tb, ok := tc.(*ast.TextBlock); ok {
			_ = tb // TODO: Check for [ ] or [x] prefix
		}
	}
	return false
}

// isTaskChecked checks if a task item is checked.
func isTaskChecked(li *ast.ListItem, source []byte) bool {
	// TODO: Implement checkbox state detection
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

// getBlockquoteFirstLine extracts the first line of text from a blockquote.
func getBlockquoteFirstLine(bq *ast.Blockquote, source []byte) string {
	for child := bq.FirstChild(); child != nil; child = child.NextSibling() {
		if p, ok := child.(*ast.Paragraph); ok {
			for inline := p.FirstChild(); inline != nil; inline = inline.NextSibling() {
				if t, ok := inline.(*ast.Text); ok {
					return string(t.Segment.Value(source))
				}
			}
		}
	}
	return ""
}
