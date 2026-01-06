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

// Helper function to create RichText for tests.
func testRichText(content string) []notionapi.RichText {
	return []notionapi.RichText{
		{
			Type: notionapi.ObjectTypeText,
			Text: &notionapi.Text{Content: content},
		},
	}
}

func TestBuildBlockUpdateRequest_Paragraph(t *testing.T) {
	block := &notionapi.ParagraphBlock{
		BasicBlock: notionapi.BasicBlock{ID: "para-123"},
		Paragraph: notionapi.Paragraph{
			RichText: testRichText("Hello, world!"),
			Color:    "blue",
		},
	}

	req, err := buildBlockUpdateRequest(block)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.Paragraph == nil {
		t.Fatal("expected Paragraph to be set")
	}
	if len(req.Paragraph.RichText) != 1 {
		t.Errorf("expected 1 RichText, got %d", len(req.Paragraph.RichText))
	}
	if req.Paragraph.Color != "blue" {
		t.Errorf("expected color blue, got %s", req.Paragraph.Color)
	}
}

func TestBuildBlockUpdateRequest_Headings(t *testing.T) {
	tests := []struct {
		name  string
		block notionapi.Block
		check func(*notionapi.BlockUpdateRequest) bool
	}{
		{
			name: "heading1",
			block: &notionapi.Heading1Block{
				BasicBlock: notionapi.BasicBlock{ID: "h1-123"},
				Heading1: notionapi.Heading{
					RichText:     testRichText("Heading 1"),
					Color:        "red",
					IsToggleable: true,
				},
			},
			check: func(req *notionapi.BlockUpdateRequest) bool {
				return req.Heading1 != nil &&
					len(req.Heading1.RichText) == 1 &&
					req.Heading1.Color == "red" &&
					req.Heading1.IsToggleable == true
			},
		},
		{
			name: "heading2",
			block: &notionapi.Heading2Block{
				BasicBlock: notionapi.BasicBlock{ID: "h2-123"},
				Heading2: notionapi.Heading{
					RichText: testRichText("Heading 2"),
					Color:    "green",
				},
			},
			check: func(req *notionapi.BlockUpdateRequest) bool {
				return req.Heading2 != nil &&
					len(req.Heading2.RichText) == 1 &&
					req.Heading2.Color == "green"
			},
		},
		{
			name: "heading3",
			block: &notionapi.Heading3Block{
				BasicBlock: notionapi.BasicBlock{ID: "h3-123"},
				Heading3: notionapi.Heading{
					RichText: testRichText("Heading 3"),
				},
			},
			check: func(req *notionapi.BlockUpdateRequest) bool {
				return req.Heading3 != nil && len(req.Heading3.RichText) == 1
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := buildBlockUpdateRequest(tt.block)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.check(req) {
				t.Errorf("request validation failed")
			}
		})
	}
}

func TestBuildBlockUpdateRequest_ListItems(t *testing.T) {
	tests := []struct {
		name  string
		block notionapi.Block
		check func(*notionapi.BlockUpdateRequest) bool
	}{
		{
			name: "bulleted list item",
			block: &notionapi.BulletedListItemBlock{
				BasicBlock: notionapi.BasicBlock{ID: "bullet-123"},
				BulletedListItem: notionapi.ListItem{
					RichText: testRichText("Bullet point"),
					Color:    "yellow",
				},
			},
			check: func(req *notionapi.BlockUpdateRequest) bool {
				return req.BulletedListItem != nil &&
					len(req.BulletedListItem.RichText) == 1 &&
					req.BulletedListItem.Color == "yellow"
			},
		},
		{
			name: "numbered list item",
			block: &notionapi.NumberedListItemBlock{
				BasicBlock: notionapi.BasicBlock{ID: "num-123"},
				NumberedListItem: notionapi.ListItem{
					RichText: testRichText("Numbered item"),
					Color:    "purple",
				},
			},
			check: func(req *notionapi.BlockUpdateRequest) bool {
				return req.NumberedListItem != nil &&
					len(req.NumberedListItem.RichText) == 1 &&
					req.NumberedListItem.Color == "purple"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := buildBlockUpdateRequest(tt.block)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.check(req) {
				t.Errorf("request validation failed")
			}
		})
	}
}

func TestBuildBlockUpdateRequest_ToDo(t *testing.T) {
	tests := []struct {
		name    string
		checked bool
	}{
		{name: "unchecked todo", checked: false},
		{name: "checked todo", checked: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			block := &notionapi.ToDoBlock{
				BasicBlock: notionapi.BasicBlock{ID: "todo-123"},
				ToDo: notionapi.ToDo{
					RichText: testRichText("Task item"),
					Checked:  tt.checked,
					Color:    "default",
				},
			}

			req, err := buildBlockUpdateRequest(block)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if req.ToDo == nil {
				t.Fatal("expected ToDo to be set")
			}
			if req.ToDo.Checked != tt.checked {
				t.Errorf("expected checked=%v, got %v", tt.checked, req.ToDo.Checked)
			}
			if len(req.ToDo.RichText) != 1 {
				t.Errorf("expected 1 RichText, got %d", len(req.ToDo.RichText))
			}
		})
	}
}

func TestBuildBlockUpdateRequest_Toggle(t *testing.T) {
	block := &notionapi.ToggleBlock{
		BasicBlock: notionapi.BasicBlock{ID: "toggle-123"},
		Toggle: notionapi.Toggle{
			RichText: testRichText("Toggle header"),
			Color:    "gray",
		},
	}

	req, err := buildBlockUpdateRequest(block)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.Toggle == nil {
		t.Fatal("expected Toggle to be set")
	}
	if len(req.Toggle.RichText) != 1 {
		t.Errorf("expected 1 RichText, got %d", len(req.Toggle.RichText))
	}
	if req.Toggle.Color != "gray" {
		t.Errorf("expected color gray, got %s", req.Toggle.Color)
	}
}

func TestBuildBlockUpdateRequest_Quote(t *testing.T) {
	block := &notionapi.QuoteBlock{
		BasicBlock: notionapi.BasicBlock{ID: "quote-123"},
		Quote: notionapi.Quote{
			RichText: testRichText("Famous quote"),
			Color:    "orange",
		},
	}

	req, err := buildBlockUpdateRequest(block)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.Quote == nil {
		t.Fatal("expected Quote to be set")
	}
	if len(req.Quote.RichText) != 1 {
		t.Errorf("expected 1 RichText, got %d", len(req.Quote.RichText))
	}
	if req.Quote.Color != "orange" {
		t.Errorf("expected color orange, got %s", req.Quote.Color)
	}
}

func TestBuildBlockUpdateRequest_Callout(t *testing.T) {
	emoji := notionapi.Emoji("warning")
	block := &notionapi.CalloutBlock{
		BasicBlock: notionapi.BasicBlock{ID: "callout-123"},
		Callout: notionapi.Callout{
			RichText: testRichText("Important note"),
			Icon: &notionapi.Icon{
				Type:  "emoji",
				Emoji: &emoji,
			},
			Color: "yellow_background",
		},
	}

	req, err := buildBlockUpdateRequest(block)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.Callout == nil {
		t.Fatal("expected Callout to be set")
	}
	if len(req.Callout.RichText) != 1 {
		t.Errorf("expected 1 RichText, got %d", len(req.Callout.RichText))
	}
	if req.Callout.Icon == nil {
		t.Error("expected Icon to be set")
	}
	if req.Callout.Color != "yellow_background" {
		t.Errorf("expected color yellow_background, got %s", req.Callout.Color)
	}
}

func TestBuildBlockUpdateRequest_Code(t *testing.T) {
	block := &notionapi.CodeBlock{
		BasicBlock: notionapi.BasicBlock{ID: "code-123"},
		Code: notionapi.Code{
			RichText: testRichText("func main() {}"),
			Caption:  testRichText("Go code example"),
			Language: "go",
		},
	}

	req, err := buildBlockUpdateRequest(block)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.Code == nil {
		t.Fatal("expected Code to be set")
	}
	if len(req.Code.RichText) != 1 {
		t.Errorf("expected 1 RichText for code, got %d", len(req.Code.RichText))
	}
	if len(req.Code.Caption) != 1 {
		t.Errorf("expected 1 RichText for caption, got %d", len(req.Code.Caption))
	}
	if req.Code.Language != "go" {
		t.Errorf("expected language go, got %s", req.Code.Language)
	}
}

func TestBuildBlockUpdateRequest_Divider(t *testing.T) {
	block := &notionapi.DividerBlock{
		BasicBlock: notionapi.BasicBlock{ID: "div-123"},
	}

	req, err := buildBlockUpdateRequest(block)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Divider blocks have no content to update.
	// The request should be valid but empty.
	if req == nil {
		t.Fatal("expected non-nil request")
	}
}

func TestBuildBlockUpdateRequest_Equation(t *testing.T) {
	block := &notionapi.EquationBlock{
		BasicBlock: notionapi.BasicBlock{ID: "eq-123"},
		Equation: notionapi.Equation{
			Expression: "E = mc^2",
		},
	}

	req, err := buildBlockUpdateRequest(block)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.Equation == nil {
		t.Fatal("expected Equation to be set")
	}
	if req.Equation.Expression != "E = mc^2" {
		t.Errorf("expected expression 'E = mc^2', got %s", req.Equation.Expression)
	}
}

func TestBuildBlockUpdateRequest_Bookmark(t *testing.T) {
	block := &notionapi.BookmarkBlock{
		BasicBlock: notionapi.BasicBlock{ID: "bookmark-123"},
		Bookmark: notionapi.Bookmark{
			URL:     "https://example.com",
			Caption: testRichText("Example website"),
		},
	}

	req, err := buildBlockUpdateRequest(block)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.Bookmark == nil {
		t.Fatal("expected Bookmark to be set")
	}
	if req.Bookmark.URL != "https://example.com" {
		t.Errorf("expected URL 'https://example.com', got %s", req.Bookmark.URL)
	}
	if len(req.Bookmark.Caption) != 1 {
		t.Errorf("expected 1 RichText for caption, got %d", len(req.Bookmark.Caption))
	}
}

func TestBuildBlockUpdateRequest_TableRow(t *testing.T) {
	block := &notionapi.TableRowBlock{
		BasicBlock: notionapi.BasicBlock{ID: "row-123"},
		TableRow: notionapi.TableRow{
			Cells: [][]notionapi.RichText{
				testRichText("Cell 1"),
				testRichText("Cell 2"),
				testRichText("Cell 3"),
			},
		},
	}

	req, err := buildBlockUpdateRequest(block)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.TableRow == nil {
		t.Fatal("expected TableRow to be set")
	}
	if len(req.TableRow.Cells) != 3 {
		t.Errorf("expected 3 cells, got %d", len(req.TableRow.Cells))
	}
}

func TestBuildBlockUpdateRequest_UnsupportedType(t *testing.T) {
	// Test with a block type that's not supported for updates.
	block := &notionapi.ChildDatabaseBlock{
		BasicBlock: notionapi.BasicBlock{ID: "child-db-123"},
	}

	_, err := buildBlockUpdateRequest(block)
	if err == nil {
		t.Fatal("expected error for unsupported block type")
	}
	if err.Error() != "unsupported block type for update: *notionapi.ChildDatabaseBlock" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestBuildBlockUpdateRequest_PreservesRichTextFormatting(t *testing.T) {
	// Test that rich text with formatting is preserved.
	richText := []notionapi.RichText{
		{
			Type: notionapi.ObjectTypeText,
			Text: &notionapi.Text{Content: "Bold text"},
			Annotations: &notionapi.Annotations{
				Bold:  true,
				Color: "blue",
			},
		},
		{
			Type: notionapi.ObjectTypeText,
			Text: &notionapi.Text{Content: " and "},
		},
		{
			Type: notionapi.ObjectTypeText,
			Text: &notionapi.Text{Content: "italic text"},
			Annotations: &notionapi.Annotations{
				Italic: true,
			},
		},
	}

	block := &notionapi.ParagraphBlock{
		BasicBlock: notionapi.BasicBlock{ID: "para-123"},
		Paragraph: notionapi.Paragraph{
			RichText: richText,
		},
	}

	req, err := buildBlockUpdateRequest(block)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(req.Paragraph.RichText) != 3 {
		t.Fatalf("expected 3 RichText items, got %d", len(req.Paragraph.RichText))
	}

	// Verify first item has bold annotation.
	if req.Paragraph.RichText[0].Annotations == nil ||
		!req.Paragraph.RichText[0].Annotations.Bold {
		t.Error("expected first item to have bold annotation")
	}

	// Verify third item has italic annotation.
	if req.Paragraph.RichText[2].Annotations == nil ||
		!req.Paragraph.RichText[2].Annotations.Italic {
		t.Error("expected third item to have italic annotation")
	}
}
