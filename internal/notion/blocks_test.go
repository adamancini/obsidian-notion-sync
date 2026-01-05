package notion

import (
	"testing"

	"github.com/jomei/notionapi"
)

func TestHasChildren(t *testing.T) {
	tests := []struct {
		name     string
		block    notionapi.Block
		expected bool
	}{
		{
			name: "paragraph with children",
			block: &notionapi.ParagraphBlock{
				BasicBlock:  notionapi.BasicBlock{HasChildren: true},
				Paragraph:   notionapi.Paragraph{},
			},
			expected: true,
		},
		{
			name: "paragraph without children",
			block: &notionapi.ParagraphBlock{
				BasicBlock:  notionapi.BasicBlock{HasChildren: false},
				Paragraph:   notionapi.Paragraph{},
			},
			expected: false,
		},
		{
			name: "bulleted list with children",
			block: &notionapi.BulletedListItemBlock{
				BasicBlock:       notionapi.BasicBlock{HasChildren: true},
				BulletedListItem: notionapi.ListItem{},
			},
			expected: true,
		},
		{
			name: "numbered list with children",
			block: &notionapi.NumberedListItemBlock{
				BasicBlock:       notionapi.BasicBlock{HasChildren: true},
				NumberedListItem: notionapi.ListItem{},
			},
			expected: true,
		},
		{
			name: "todo with children",
			block: &notionapi.ToDoBlock{
				BasicBlock: notionapi.BasicBlock{HasChildren: true},
				ToDo:       notionapi.ToDo{},
			},
			expected: true,
		},
		{
			name: "toggle with children",
			block: &notionapi.ToggleBlock{
				BasicBlock: notionapi.BasicBlock{HasChildren: true},
				Toggle:     notionapi.Toggle{},
			},
			expected: true,
		},
		{
			name: "quote with children",
			block: &notionapi.QuoteBlock{
				BasicBlock: notionapi.BasicBlock{HasChildren: true},
				Quote:      notionapi.Quote{},
			},
			expected: true,
		},
		{
			name: "callout with children",
			block: &notionapi.CalloutBlock{
				BasicBlock: notionapi.BasicBlock{HasChildren: true},
				Callout:    notionapi.Callout{},
			},
			expected: true,
		},
		{
			name: "column list with children",
			block: &notionapi.ColumnListBlock{
				BasicBlock: notionapi.BasicBlock{HasChildren: true},
				ColumnList: notionapi.ColumnList{},
			},
			expected: true,
		},
		{
			name: "column with children",
			block: &notionapi.ColumnBlock{
				BasicBlock: notionapi.BasicBlock{HasChildren: true},
				Column:     notionapi.Column{},
			},
			expected: true,
		},
		{
			name: "synced block with children",
			block: &notionapi.SyncedBlock{
				BasicBlock:  notionapi.BasicBlock{HasChildren: true},
				SyncedBlock: notionapi.Synced{},
			},
			expected: true,
		},
		{
			name: "heading1 (no children support)",
			block: &notionapi.Heading1Block{
				BasicBlock: notionapi.BasicBlock{HasChildren: false},
				Heading1:   notionapi.Heading{},
			},
			expected: false,
		},
		{
			name: "divider (no children support)",
			block: &notionapi.DividerBlock{
				BasicBlock: notionapi.BasicBlock{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasChildren(tt.block)
			if result != tt.expected {
				t.Errorf("hasChildren() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestExtractBlockID(t *testing.T) {
	tests := []struct {
		name     string
		block    notionapi.Block
		expected string
	}{
		{
			name: "paragraph block",
			block: &notionapi.ParagraphBlock{
				BasicBlock: notionapi.BasicBlock{ID: "para-123"},
			},
			expected: "para-123",
		},
		{
			name: "heading1 block",
			block: &notionapi.Heading1Block{
				BasicBlock: notionapi.BasicBlock{ID: "h1-123"},
			},
			expected: "h1-123",
		},
		{
			name: "heading2 block",
			block: &notionapi.Heading2Block{
				BasicBlock: notionapi.BasicBlock{ID: "h2-123"},
			},
			expected: "h2-123",
		},
		{
			name: "heading3 block",
			block: &notionapi.Heading3Block{
				BasicBlock: notionapi.BasicBlock{ID: "h3-123"},
			},
			expected: "h3-123",
		},
		{
			name: "bulleted list block",
			block: &notionapi.BulletedListItemBlock{
				BasicBlock: notionapi.BasicBlock{ID: "bullet-123"},
			},
			expected: "bullet-123",
		},
		{
			name: "numbered list block",
			block: &notionapi.NumberedListItemBlock{
				BasicBlock: notionapi.BasicBlock{ID: "num-123"},
			},
			expected: "num-123",
		},
		{
			name: "todo block",
			block: &notionapi.ToDoBlock{
				BasicBlock: notionapi.BasicBlock{ID: "todo-123"},
			},
			expected: "todo-123",
		},
		{
			name: "toggle block",
			block: &notionapi.ToggleBlock{
				BasicBlock: notionapi.BasicBlock{ID: "toggle-123"},
			},
			expected: "toggle-123",
		},
		{
			name: "quote block",
			block: &notionapi.QuoteBlock{
				BasicBlock: notionapi.BasicBlock{ID: "quote-123"},
			},
			expected: "quote-123",
		},
		{
			name: "callout block",
			block: &notionapi.CalloutBlock{
				BasicBlock: notionapi.BasicBlock{ID: "callout-123"},
			},
			expected: "callout-123",
		},
		{
			name: "code block",
			block: &notionapi.CodeBlock{
				BasicBlock: notionapi.BasicBlock{ID: "code-123"},
			},
			expected: "code-123",
		},
		{
			name: "divider block",
			block: &notionapi.DividerBlock{
				BasicBlock: notionapi.BasicBlock{ID: "div-123"},
			},
			expected: "div-123",
		},
		{
			name: "image block",
			block: &notionapi.ImageBlock{
				BasicBlock: notionapi.BasicBlock{ID: "img-123"},
			},
			expected: "img-123",
		},
		{
			name: "equation block",
			block: &notionapi.EquationBlock{
				BasicBlock: notionapi.BasicBlock{ID: "eq-123"},
			},
			expected: "eq-123",
		},
		{
			name: "table block",
			block: &notionapi.TableBlock{
				BasicBlock: notionapi.BasicBlock{ID: "table-123"},
			},
			expected: "table-123",
		},
		{
			name: "table row block",
			block: &notionapi.TableRowBlock{
				BasicBlock: notionapi.BasicBlock{ID: "row-123"},
			},
			expected: "row-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractBlockID(tt.block)
			if result != tt.expected {
				t.Errorf("extractBlockID() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestSetBlockChildren(t *testing.T) {
	children := []notionapi.Block{
		&notionapi.ParagraphBlock{
			BasicBlock: notionapi.BasicBlock{ID: "child-1"},
		},
	}

	tests := []struct {
		name  string
		block notionapi.Block
		check func(notionapi.Block) bool
	}{
		{
			name: "paragraph",
			block: &notionapi.ParagraphBlock{
				BasicBlock: notionapi.BasicBlock{ID: "para-1"},
				Paragraph:  notionapi.Paragraph{},
			},
			check: func(b notionapi.Block) bool {
				p := b.(*notionapi.ParagraphBlock)
				return len(p.Paragraph.Children) == 1
			},
		},
		{
			name: "bulleted list",
			block: &notionapi.BulletedListItemBlock{
				BasicBlock:       notionapi.BasicBlock{ID: "bullet-1"},
				BulletedListItem: notionapi.ListItem{},
			},
			check: func(b notionapi.Block) bool {
				p := b.(*notionapi.BulletedListItemBlock)
				return len(p.BulletedListItem.Children) == 1
			},
		},
		{
			name: "numbered list",
			block: &notionapi.NumberedListItemBlock{
				BasicBlock:       notionapi.BasicBlock{ID: "num-1"},
				NumberedListItem: notionapi.ListItem{},
			},
			check: func(b notionapi.Block) bool {
				p := b.(*notionapi.NumberedListItemBlock)
				return len(p.NumberedListItem.Children) == 1
			},
		},
		{
			name: "todo",
			block: &notionapi.ToDoBlock{
				BasicBlock: notionapi.BasicBlock{ID: "todo-1"},
				ToDo:       notionapi.ToDo{},
			},
			check: func(b notionapi.Block) bool {
				p := b.(*notionapi.ToDoBlock)
				return len(p.ToDo.Children) == 1
			},
		},
		{
			name: "toggle",
			block: &notionapi.ToggleBlock{
				BasicBlock: notionapi.BasicBlock{ID: "toggle-1"},
				Toggle:     notionapi.Toggle{},
			},
			check: func(b notionapi.Block) bool {
				p := b.(*notionapi.ToggleBlock)
				return len(p.Toggle.Children) == 1
			},
		},
		{
			name: "quote",
			block: &notionapi.QuoteBlock{
				BasicBlock: notionapi.BasicBlock{ID: "quote-1"},
				Quote:      notionapi.Quote{},
			},
			check: func(b notionapi.Block) bool {
				p := b.(*notionapi.QuoteBlock)
				return len(p.Quote.Children) == 1
			},
		},
		{
			name: "callout",
			block: &notionapi.CalloutBlock{
				BasicBlock: notionapi.BasicBlock{ID: "callout-1"},
				Callout:    notionapi.Callout{},
			},
			check: func(b notionapi.Block) bool {
				p := b.(*notionapi.CalloutBlock)
				return len(p.Callout.Children) == 1
			},
		},
		{
			name: "column list",
			block: &notionapi.ColumnListBlock{
				BasicBlock: notionapi.BasicBlock{ID: "collist-1"},
				ColumnList: notionapi.ColumnList{},
			},
			check: func(b notionapi.Block) bool {
				p := b.(*notionapi.ColumnListBlock)
				return len(p.ColumnList.Children) == 1
			},
		},
		{
			name: "column",
			block: &notionapi.ColumnBlock{
				BasicBlock: notionapi.BasicBlock{ID: "col-1"},
				Column:     notionapi.Column{},
			},
			check: func(b notionapi.Block) bool {
				p := b.(*notionapi.ColumnBlock)
				return len(p.Column.Children) == 1
			},
		},
		{
			name: "synced block",
			block: &notionapi.SyncedBlock{
				BasicBlock:  notionapi.BasicBlock{ID: "sync-1"},
				SyncedBlock: notionapi.Synced{},
			},
			check: func(b notionapi.Block) bool {
				p := b.(*notionapi.SyncedBlock)
				return len(p.SyncedBlock.Children) == 1
			},
		},
		{
			name: "heading1 (no children support)",
			block: &notionapi.Heading1Block{
				BasicBlock: notionapi.BasicBlock{ID: "h1-1"},
				Heading1:   notionapi.Heading{},
			},
			check: func(b notionapi.Block) bool {
				// Should return unchanged.
				return true
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := setBlockChildren(tt.block, children)
			if !tt.check(result) {
				t.Errorf("setBlockChildren() did not set children correctly")
			}
		})
	}
}
