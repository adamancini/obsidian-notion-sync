package transformer

import (
	"strings"
	"testing"

	"github.com/jomei/notionapi"
)

// mockPathLookup is a test double for path lookup.
type mockPathLookup struct {
	paths map[string]string
}

func (m *mockPathLookup) LookupPath(notionPageID string) (string, bool) {
	path, ok := m.paths[notionPageID]
	return path, ok
}

func TestNewReverse(t *testing.T) {
	rt := NewReverse(nil, nil)
	if rt == nil {
		t.Fatal("NewReverse() returned nil")
	}

	if rt.config == nil {
		t.Error("ReverseTransformer.config should not be nil")
	}
}

func TestNewReverse_WithConfig(t *testing.T) {
	cfg := &Config{
		UnresolvedLinkStyle: "text",
	}
	rt := NewReverse(nil, cfg)

	if rt.config.UnresolvedLinkStyle != "text" {
		t.Errorf("config not applied, got %q", rt.config.UnresolvedLinkStyle)
	}
}

func TestTransform_Headings(t *testing.T) {
	tests := []struct {
		name     string
		block    notionapi.Block
		expected string
	}{
		{
			name: "heading_1",
			block: &notionapi.Heading1Block{
				Heading1: notionapi.Heading{
					RichText: []notionapi.RichText{
						{PlainText: "Heading 1"},
					},
				},
			},
			expected: "# Heading 1\n\n",
		},
		{
			name: "heading_2",
			block: &notionapi.Heading2Block{
				Heading2: notionapi.Heading{
					RichText: []notionapi.RichText{
						{PlainText: "Heading 2"},
					},
				},
			},
			expected: "## Heading 2\n\n",
		},
		{
			name: "heading_3",
			block: &notionapi.Heading3Block{
				Heading3: notionapi.Heading{
					RichText: []notionapi.RichText{
						{PlainText: "Heading 3"},
					},
				},
			},
			expected: "### Heading 3\n\n",
		},
	}

	rt := NewReverse(nil, nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := rt.Transform([]notionapi.Block{tt.block})
			if err != nil {
				t.Fatalf("Transform() error: %v", err)
			}

			if result != tt.expected {
				t.Errorf("Transform() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestTransform_Paragraph(t *testing.T) {
	rt := NewReverse(nil, nil)

	block := &notionapi.ParagraphBlock{
		Paragraph: notionapi.Paragraph{
			RichText: []notionapi.RichText{
				{PlainText: "This is a paragraph."},
			},
		},
	}

	result, err := rt.Transform([]notionapi.Block{block})
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	expected := "This is a paragraph.\n\n"
	if result != expected {
		t.Errorf("Transform() = %q, want %q", result, expected)
	}
}

func TestTransform_EmptyParagraph(t *testing.T) {
	rt := NewReverse(nil, nil)

	block := &notionapi.ParagraphBlock{
		Paragraph: notionapi.Paragraph{
			RichText: []notionapi.RichText{},
		},
	}

	result, err := rt.Transform([]notionapi.Block{block})
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	expected := "\n"
	if result != expected {
		t.Errorf("Transform() = %q, want %q", result, expected)
	}
}

func TestTransform_BulletedList(t *testing.T) {
	rt := NewReverse(nil, nil)

	blocks := []notionapi.Block{
		&notionapi.BulletedListItemBlock{
			BulletedListItem: notionapi.ListItem{
				RichText: []notionapi.RichText{
					{PlainText: "Item 1"},
				},
			},
		},
		&notionapi.BulletedListItemBlock{
			BulletedListItem: notionapi.ListItem{
				RichText: []notionapi.RichText{
					{PlainText: "Item 2"},
				},
			},
		},
	}

	result, err := rt.Transform(blocks)
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	expected := "- Item 1\n- Item 2\n"
	if result != expected {
		t.Errorf("Transform() = %q, want %q", result, expected)
	}
}

func TestTransform_NumberedList(t *testing.T) {
	rt := NewReverse(nil, nil)

	blocks := []notionapi.Block{
		&notionapi.NumberedListItemBlock{
			NumberedListItem: notionapi.ListItem{
				RichText: []notionapi.RichText{
					{PlainText: "First"},
				},
			},
		},
		&notionapi.NumberedListItemBlock{
			NumberedListItem: notionapi.ListItem{
				RichText: []notionapi.RichText{
					{PlainText: "Second"},
				},
			},
		},
	}

	result, err := rt.Transform(blocks)
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	expected := "1. First\n1. Second\n"
	if result != expected {
		t.Errorf("Transform() = %q, want %q", result, expected)
	}
}

func TestTransform_ToDo(t *testing.T) {
	rt := NewReverse(nil, nil)

	blocks := []notionapi.Block{
		&notionapi.ToDoBlock{
			ToDo: notionapi.ToDo{
				RichText: []notionapi.RichText{
					{PlainText: "Unchecked task"},
				},
				Checked: false,
			},
		},
		&notionapi.ToDoBlock{
			ToDo: notionapi.ToDo{
				RichText: []notionapi.RichText{
					{PlainText: "Checked task"},
				},
				Checked: true,
			},
		},
	}

	result, err := rt.Transform(blocks)
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	expected := "- [ ] Unchecked task\n- [x] Checked task\n"
	if result != expected {
		t.Errorf("Transform() = %q, want %q", result, expected)
	}
}

func TestTransform_Quote(t *testing.T) {
	rt := NewReverse(nil, nil)

	block := &notionapi.QuoteBlock{
		Quote: notionapi.Quote{
			RichText: []notionapi.RichText{
				{PlainText: "This is a quote."},
			},
		},
	}

	result, err := rt.Transform([]notionapi.Block{block})
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	expected := "> This is a quote.\n\n"
	if result != expected {
		t.Errorf("Transform() = %q, want %q", result, expected)
	}
}

func TestTransform_Callout(t *testing.T) {
	rt := NewReverse(nil, nil)

	emoji := notionapi.Emoji("⚠️")
	block := &notionapi.CalloutBlock{
		Callout: notionapi.Callout{
			RichText: []notionapi.RichText{
				{PlainText: "This is a warning."},
			},
			Icon: &notionapi.Icon{
				Type:  "emoji",
				Emoji: &emoji,
			},
		},
	}

	result, err := rt.Transform([]notionapi.Block{block})
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	// Should convert to Obsidian callout syntax.
	// The warning icon maps to warning, caution, or attention depending on map iteration order.
	hasValidCallout := strings.Contains(result, "> [!warning]") ||
		strings.Contains(result, "> [!caution]") ||
		strings.Contains(result, "> [!attention]")
	if !hasValidCallout {
		t.Errorf("Expected warning/caution/attention callout syntax, got %q", result)
	}
	if !strings.Contains(result, "This is a warning.") {
		t.Errorf("Expected callout content, got %q", result)
	}
}

func TestTransform_CodeBlock(t *testing.T) {
	rt := NewReverse(nil, nil)

	block := &notionapi.CodeBlock{
		Code: notionapi.Code{
			Language: "go",
			RichText: []notionapi.RichText{
				{PlainText: "func main() {\n    fmt.Println(\"Hello\")\n}"},
			},
		},
	}

	result, err := rt.Transform([]notionapi.Block{block})
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	expected := "```go\nfunc main() {\n    fmt.Println(\"Hello\")\n}\n```\n\n"
	if result != expected {
		t.Errorf("Transform() = %q, want %q", result, expected)
	}
}

func TestTransform_CodeBlock_PlainText(t *testing.T) {
	rt := NewReverse(nil, nil)

	block := &notionapi.CodeBlock{
		Code: notionapi.Code{
			Language: "plain text",
			RichText: []notionapi.RichText{
				{PlainText: "some code"},
			},
		},
	}

	result, err := rt.Transform([]notionapi.Block{block})
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	// "plain text" should become empty language.
	expected := "```\nsome code\n```\n\n"
	if result != expected {
		t.Errorf("Transform() = %q, want %q", result, expected)
	}
}

func TestTransform_Divider(t *testing.T) {
	rt := NewReverse(nil, nil)

	block := &notionapi.DividerBlock{}

	result, err := rt.Transform([]notionapi.Block{block})
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	expected := "---\n\n"
	if result != expected {
		t.Errorf("Transform() = %q, want %q", result, expected)
	}
}

func TestTransform_Equation(t *testing.T) {
	rt := NewReverse(nil, nil)

	block := &notionapi.EquationBlock{
		Equation: notionapi.Equation{
			Expression: "E = mc^2",
		},
	}

	result, err := rt.Transform([]notionapi.Block{block})
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	expected := "$$\nE = mc^2\n$$\n\n"
	if result != expected {
		t.Errorf("Transform() = %q, want %q", result, expected)
	}
}

func TestTransform_Table(t *testing.T) {
	rt := NewReverse(nil, nil)

	block := &notionapi.TableBlock{
		Table: notionapi.Table{
			TableWidth:      2,
			HasColumnHeader: true,
			Children: []notionapi.Block{
				&notionapi.TableRowBlock{
					TableRow: notionapi.TableRow{
						Cells: [][]notionapi.RichText{
							{{PlainText: "Header 1"}},
							{{PlainText: "Header 2"}},
						},
					},
				},
				&notionapi.TableRowBlock{
					TableRow: notionapi.TableRow{
						Cells: [][]notionapi.RichText{
							{{PlainText: "Cell 1"}},
							{{PlainText: "Cell 2"}},
						},
					},
				},
			},
		},
	}

	result, err := rt.Transform([]notionapi.Block{block})
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	// Check table structure.
	if !strings.Contains(result, "| Header 1 | Header 2 |") {
		t.Errorf("Expected header row, got %q", result)
	}
	if !strings.Contains(result, "| --- | --- |") {
		t.Errorf("Expected separator row, got %q", result)
	}
	if !strings.Contains(result, "| Cell 1 | Cell 2 |") {
		t.Errorf("Expected data row, got %q", result)
	}
}

func TestTransform_Table_NoPipeEscape(t *testing.T) {
	rt := NewReverse(nil, nil)

	block := &notionapi.TableBlock{
		Table: notionapi.Table{
			TableWidth:      1,
			HasColumnHeader: false,
			Children: []notionapi.Block{
				&notionapi.TableRowBlock{
					TableRow: notionapi.TableRow{
						Cells: [][]notionapi.RichText{
							{{PlainText: "A|B"}},
						},
					},
				},
			},
		},
	}

	result, err := rt.Transform([]notionapi.Block{block})
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	// Pipe in content should be escaped.
	if !strings.Contains(result, `A\|B`) {
		t.Errorf("Expected escaped pipe, got %q", result)
	}
}

func TestTransform_Image(t *testing.T) {
	rt := NewReverse(nil, nil)

	tests := []struct {
		name     string
		block    *notionapi.ImageBlock
		expected string
	}{
		{
			name: "external image with caption",
			block: &notionapi.ImageBlock{
				Image: notionapi.Image{
					External: &notionapi.FileObject{
						URL: "https://example.com/image.png",
					},
					Caption: []notionapi.RichText{
						{PlainText: "My Image"},
					},
				},
			},
			expected: "![My Image](https://example.com/image.png)\n\n",
		},
		{
			name: "external image without caption",
			block: &notionapi.ImageBlock{
				Image: notionapi.Image{
					External: &notionapi.FileObject{
						URL: "https://example.com/image.png",
					},
				},
			},
			expected: "![](https://example.com/image.png)\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := rt.Transform([]notionapi.Block{tt.block})
			if err != nil {
				t.Fatalf("Transform() error: %v", err)
			}

			if result != tt.expected {
				t.Errorf("Transform() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// Rich text tests.

func TestTransformRichText_Bold(t *testing.T) {
	rt := NewReverse(nil, nil)

	richText := []notionapi.RichText{
		{
			PlainText: "bold text",
			Annotations: &notionapi.Annotations{
				Bold: true,
			},
		},
	}

	result := rt.TransformRichText(richText)
	expected := "**bold text**"

	if result != expected {
		t.Errorf("TransformRichText() = %q, want %q", result, expected)
	}
}

func TestTransformRichText_Italic(t *testing.T) {
	rt := NewReverse(nil, nil)

	richText := []notionapi.RichText{
		{
			PlainText: "italic text",
			Annotations: &notionapi.Annotations{
				Italic: true,
			},
		},
	}

	result := rt.TransformRichText(richText)
	expected := "*italic text*"

	if result != expected {
		t.Errorf("TransformRichText() = %q, want %q", result, expected)
	}
}

func TestTransformRichText_Strikethrough(t *testing.T) {
	rt := NewReverse(nil, nil)

	richText := []notionapi.RichText{
		{
			PlainText: "struck text",
			Annotations: &notionapi.Annotations{
				Strikethrough: true,
			},
		},
	}

	result := rt.TransformRichText(richText)
	expected := "~~struck text~~"

	if result != expected {
		t.Errorf("TransformRichText() = %q, want %q", result, expected)
	}
}

func TestTransformRichText_Code(t *testing.T) {
	rt := NewReverse(nil, nil)

	richText := []notionapi.RichText{
		{
			PlainText: "code",
			Annotations: &notionapi.Annotations{
				Code: true,
			},
		},
	}

	result := rt.TransformRichText(richText)
	expected := "`code`"

	if result != expected {
		t.Errorf("TransformRichText() = %q, want %q", result, expected)
	}
}

func TestTransformRichText_Highlight(t *testing.T) {
	rt := NewReverse(nil, nil)

	richText := []notionapi.RichText{
		{
			PlainText: "highlighted",
			Annotations: &notionapi.Annotations{
				Color: notionapi.ColorYellowBackground,
			},
		},
	}

	result := rt.TransformRichText(richText)
	expected := "==highlighted=="

	if result != expected {
		t.Errorf("TransformRichText() = %q, want %q", result, expected)
	}
}

func TestTransformRichText_Underline(t *testing.T) {
	rt := NewReverse(nil, nil)

	richText := []notionapi.RichText{
		{
			PlainText: "underlined text",
			Annotations: &notionapi.Annotations{
				Underline: true,
			},
		},
	}

	result := rt.TransformRichText(richText)
	expected := "<u>underlined text</u>"

	if result != expected {
		t.Errorf("TransformRichText() = %q, want %q", result, expected)
	}
}

func TestTransformRichText_UnderlineWithOtherFormats(t *testing.T) {
	rt := NewReverse(nil, nil)

	richText := []notionapi.RichText{
		{
			PlainText: "formatted",
			Annotations: &notionapi.Annotations{
				Bold:      true,
				Underline: true,
			},
		},
	}

	result := rt.TransformRichText(richText)
	// Underline is applied before italic/bold in order, so expect: **<u>text</u>**
	expected := "**<u>formatted</u>**"

	if result != expected {
		t.Errorf("TransformRichText() = %q, want %q", result, expected)
	}
}

func TestTransformRichText_Link(t *testing.T) {
	rt := NewReverse(nil, nil)

	richText := []notionapi.RichText{
		{
			PlainText: "link text",
			Text: &notionapi.Text{
				Content: "link text",
				Link: &notionapi.Link{
					Url: "https://example.com",
				},
			},
		},
	}

	result := rt.TransformRichText(richText)
	expected := "[link text](https://example.com)"

	if result != expected {
		t.Errorf("TransformRichText() = %q, want %q", result, expected)
	}
}

func TestTransformRichText_PageMention_Resolved(t *testing.T) {
	lookup := &mockPathLookup{
		paths: map[string]string{
			"page-id-123": "Notes/Other Note",
		},
	}
	rt := NewReverse(lookup, nil)

	richText := []notionapi.RichText{
		{
			Type:      "mention",
			PlainText: "Other Note",
			Mention: &notionapi.Mention{
				Type: "page",
				Page: &notionapi.PageMention{
					ID: "page-id-123",
				},
			},
		},
	}

	result := rt.TransformRichText(richText)
	expected := "[[Notes/Other Note]]"

	if result != expected {
		t.Errorf("TransformRichText() = %q, want %q", result, expected)
	}
}

func TestTransformRichText_PageMention_Unresolved(t *testing.T) {
	rt := NewReverse(nil, nil)

	richText := []notionapi.RichText{
		{
			Type:      "mention",
			PlainText: "Unknown Page",
			Mention: &notionapi.Mention{
				Type: "page",
				Page: &notionapi.PageMention{
					ID: "unknown-id",
				},
			},
		},
	}

	result := rt.TransformRichText(richText)
	expected := "[[Unknown Page]]"

	if result != expected {
		t.Errorf("TransformRichText() = %q, want %q", result, expected)
	}
}

func TestTransformRichText_InlineEquation(t *testing.T) {
	rt := NewReverse(nil, nil)

	richText := []notionapi.RichText{
		{
			Type:      "equation",
			PlainText: "E = mc^2",
			Equation: &notionapi.Equation{
				Expression: "E = mc^2",
			},
		},
	}

	result := rt.TransformRichText(richText)
	expected := "$E = mc^2$"

	if result != expected {
		t.Errorf("TransformRichText() = %q, want %q", result, expected)
	}
}

func TestTransformRichText_MultipleFormats(t *testing.T) {
	rt := NewReverse(nil, nil)

	richText := []notionapi.RichText{
		{PlainText: "Normal "},
		{
			PlainText: "bold",
			Annotations: &notionapi.Annotations{
				Bold: true,
			},
		},
		{PlainText: " and "},
		{
			PlainText: "italic",
			Annotations: &notionapi.Annotations{
				Italic: true,
			},
		},
		{PlainText: " text."},
	}

	result := rt.TransformRichText(richText)
	expected := "Normal **bold** and *italic* text."

	if result != expected {
		t.Errorf("TransformRichText() = %q, want %q", result, expected)
	}
}

func TestTransformRichText_BoldAndItalic(t *testing.T) {
	rt := NewReverse(nil, nil)

	richText := []notionapi.RichText{
		{
			PlainText: "bold italic",
			Annotations: &notionapi.Annotations{
				Bold:   true,
				Italic: true,
			},
		},
	}

	result := rt.TransformRichText(richText)
	expected := "***bold italic***"

	if result != expected {
		t.Errorf("TransformRichText() = %q, want %q", result, expected)
	}
}

// Nested blocks tests.

func TestTransform_NestedBulletedList(t *testing.T) {
	rt := NewReverse(nil, nil)

	block := &notionapi.BulletedListItemBlock{
		BulletedListItem: notionapi.ListItem{
			RichText: []notionapi.RichText{
				{PlainText: "Parent item"},
			},
			Children: []notionapi.Block{
				&notionapi.BulletedListItemBlock{
					BulletedListItem: notionapi.ListItem{
						RichText: []notionapi.RichText{
							{PlainText: "Child item"},
						},
					},
				},
			},
		},
	}

	result, err := rt.Transform([]notionapi.Block{block})
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	// Check for proper indentation.
	if !strings.Contains(result, "- Parent item") {
		t.Errorf("Expected parent item, got %q", result)
	}
	if !strings.Contains(result, "  - Child item") {
		t.Errorf("Expected indented child item, got %q", result)
	}
}

func TestTransform_NestedNumberedList(t *testing.T) {
	rt := NewReverse(nil, nil)

	block := &notionapi.NumberedListItemBlock{
		NumberedListItem: notionapi.ListItem{
			RichText: []notionapi.RichText{
				{PlainText: "Parent item"},
			},
			Children: []notionapi.Block{
				&notionapi.NumberedListItemBlock{
					NumberedListItem: notionapi.ListItem{
						RichText: []notionapi.RichText{
							{PlainText: "Child item"},
						},
					},
				},
			},
		},
	}

	result, err := rt.Transform([]notionapi.Block{block})
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	// Check for proper indentation.
	if !strings.Contains(result, "1. Parent item") {
		t.Errorf("Expected parent item, got %q", result)
	}
	if !strings.Contains(result, "  1. Child item") {
		t.Errorf("Expected indented child item, got %q", result)
	}
}

// NotionToMarkdown tests.

func TestNotionToMarkdown_WithFrontmatter(t *testing.T) {
	rt := NewReverse(nil, nil)

	page := &NotionPage{
		Properties: notionapi.Properties{
			"Name": &notionapi.TitleProperty{
				Title: []notionapi.RichText{
					{PlainText: "Test Page"},
				},
			},
		},
		Children: []notionapi.Block{
			&notionapi.ParagraphBlock{
				Paragraph: notionapi.Paragraph{
					RichText: []notionapi.RichText{
						{PlainText: "Content here."},
					},
				},
			},
		},
	}

	result, err := rt.NotionToMarkdown(page)
	if err != nil {
		t.Fatalf("NotionToMarkdown() error: %v", err)
	}

	// Check frontmatter.
	if !strings.Contains(string(result), "---") {
		t.Errorf("Expected frontmatter delimiters, got %s", result)
	}
	if !strings.Contains(string(result), "title: Test Page") {
		t.Errorf("Expected title in frontmatter, got %s", result)
	}
	if !strings.Contains(string(result), "Content here.") {
		t.Errorf("Expected content, got %s", result)
	}
}

func TestIconToCalloutType(t *testing.T) {
	rt := NewReverse(nil, nil)

	tests := []struct {
		icon           string
		validTypes     []string // Multiple valid types due to non-deterministic map iteration.
	}{
		{"⚠️", []string{"warning", "caution", "attention"}},
		{"💡", []string{"note", "tip", "hint"}},
		{"❌", []string{"failure", "fail", "missing"}},
		{"✅", []string{"success", "check", "done"}},
		{"❓", []string{"question", "help", "faq"}},
		{"🔴", []string{"danger", "error"}},
		{"📋", []string{"abstract", "summary"}},
		{"🐛", []string{"bug"}},
		{"📖", []string{"example"}},
		{"💬", []string{"quote", "cite"}},
		{"unknown", []string{"note"}}, // Default fallback.
	}

	for _, tt := range tests {
		t.Run(tt.icon, func(t *testing.T) {
			result := rt.iconToCalloutType(tt.icon)
			found := false
			for _, valid := range tt.validTypes {
				if result == valid {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("iconToCalloutType(%q) = %q, want one of %v", tt.icon, result, tt.validTypes)
			}
		})
	}
}

func TestPropertiesToFrontmatter(t *testing.T) {
	rt := NewReverse(nil, nil)

	props := notionapi.Properties{
		"Name": &notionapi.TitleProperty{
			Title: []notionapi.RichText{
				{PlainText: "Test Title"},
			},
		},
		"Tags": &notionapi.MultiSelectProperty{
			MultiSelect: []notionapi.Option{
				{Name: "tag1"},
				{Name: "tag2"},
			},
		},
		"Status": &notionapi.SelectProperty{
			Select: notionapi.Option{
				Name: "Done",
			},
		},
		"Published": &notionapi.CheckboxProperty{
			Checkbox: true,
		},
		"Count": &notionapi.NumberProperty{
			Number: 42,
		},
	}

	frontmatter := rt.propertiesToFrontmatter(props)

	if frontmatter["title"] != "Test Title" {
		t.Errorf("title = %v, want 'Test Title'", frontmatter["title"])
	}

	tags, ok := frontmatter["tags"].([]string)
	if !ok || len(tags) != 2 {
		t.Errorf("tags = %v, want ['tag1', 'tag2']", frontmatter["tags"])
	}

	if frontmatter["status"] != "Done" {
		t.Errorf("status = %v, want 'Done'", frontmatter["status"])
	}

	if frontmatter["published"] != true {
		t.Errorf("published = %v, want true", frontmatter["published"])
	}

	if frontmatter["count"] != float64(42) {
		t.Errorf("count = %v, want 42", frontmatter["count"])
	}
}

func TestTransform_Toggle(t *testing.T) {
	rt := NewReverse(nil, nil)

	block := &notionapi.ToggleBlock{
		Toggle: notionapi.Toggle{
			RichText: []notionapi.RichText{
				{PlainText: "Toggle header"},
			},
			Children: []notionapi.Block{
				&notionapi.ParagraphBlock{
					Paragraph: notionapi.Paragraph{
						RichText: []notionapi.RichText{
							{PlainText: "Hidden content"},
						},
					},
				},
			},
		},
	}

	result, err := rt.Transform([]notionapi.Block{block})
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	// Toggle should render as a list item with indented content.
	if !strings.Contains(result, "- Toggle header") {
		t.Errorf("Expected toggle header, got %q", result)
	}
	if !strings.Contains(result, "  Hidden content") {
		t.Errorf("Expected indented hidden content, got %q", result)
	}
}

func TestTransform_Bookmark(t *testing.T) {
	rt := NewReverse(nil, nil)

	tests := []struct {
		name     string
		block    *notionapi.BookmarkBlock
		expected string
	}{
		{
			name: "with caption",
			block: &notionapi.BookmarkBlock{
				Bookmark: notionapi.Bookmark{
					URL: "https://example.com",
					Caption: []notionapi.RichText{
						{PlainText: "Example Site"},
					},
				},
			},
			expected: "[Example Site](https://example.com)\n\n",
		},
		{
			name: "without caption",
			block: &notionapi.BookmarkBlock{
				Bookmark: notionapi.Bookmark{
					URL: "https://example.com",
				},
			},
			expected: "<https://example.com>\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := rt.Transform([]notionapi.Block{tt.block})
			if err != nil {
				t.Fatalf("Transform() error: %v", err)
			}

			if result != tt.expected {
				t.Errorf("Transform() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestTransform_Embed(t *testing.T) {
	rt := NewReverse(nil, nil)

	block := &notionapi.EmbedBlock{
		Embed: notionapi.Embed{
			URL: "https://youtube.com/watch?v=123",
		},
	}

	result, err := rt.Transform([]notionapi.Block{block})
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	expected := "<https://youtube.com/watch?v=123>\n\n"
	if result != expected {
		t.Errorf("Transform() = %q, want %q", result, expected)
	}
}

func TestTransform_MultipleBlocks(t *testing.T) {
	rt := NewReverse(nil, nil)

	blocks := []notionapi.Block{
		&notionapi.Heading1Block{
			Heading1: notionapi.Heading{
				RichText: []notionapi.RichText{
					{PlainText: "Title"},
				},
			},
		},
		&notionapi.ParagraphBlock{
			Paragraph: notionapi.Paragraph{
				RichText: []notionapi.RichText{
					{PlainText: "First paragraph."},
				},
			},
		},
		&notionapi.DividerBlock{},
		&notionapi.ParagraphBlock{
			Paragraph: notionapi.Paragraph{
				RichText: []notionapi.RichText{
					{PlainText: "Second paragraph."},
				},
			},
		},
	}

	result, err := rt.Transform(blocks)
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	expected := "# Title\n\nFirst paragraph.\n\n---\n\nSecond paragraph.\n\n"
	if result != expected {
		t.Errorf("Transform() = %q, want %q", result, expected)
	}
}

func TestRichTextToPlainText(t *testing.T) {
	rt := NewReverse(nil, nil)

	richText := []notionapi.RichText{
		{PlainText: "Hello "},
		{
			PlainText: "world",
			Annotations: &notionapi.Annotations{
				Bold: true,
			},
		},
		{PlainText: "!"},
	}

	result := rt.richTextToPlainText(richText)
	expected := "Hello world!"

	if result != expected {
		t.Errorf("richTextToPlainText() = %q, want %q", result, expected)
	}
}

// mockUnknownBlock is a test double for an unknown/unsupported block type.
type mockUnknownBlock struct {
	notionapi.BasicBlock
}

func TestTransform_UnknownBlockType(t *testing.T) {
	rt := NewReverse(nil, nil)

	// Use a mock block that doesn't match any known type.
	block := &mockUnknownBlock{}

	result, err := rt.Transform([]notionapi.Block{block})
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	// Should return an HTML comment indicating the unsupported block type.
	if !strings.Contains(result, "<!-- Unsupported Notion block type:") {
		t.Errorf("Expected unsupported block comment, got %q", result)
	}
	if !strings.Contains(result, "mockUnknownBlock") {
		t.Errorf("Expected block type name in comment, got %q", result)
	}
}

func TestTransform_UnknownBlockTypeWithIndent(t *testing.T) {
	rt := NewReverse(nil, nil)

	// Test unknown block as a child (with indentation).
	// Create a paragraph with an unknown child.
	block := &notionapi.ParagraphBlock{
		Paragraph: notionapi.Paragraph{
			RichText: []notionapi.RichText{
				{PlainText: "Parent paragraph"},
			},
			Children: []notionapi.Block{
				&mockUnknownBlock{},
			},
		},
	}

	result, err := rt.Transform([]notionapi.Block{block})
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	// Should contain the parent text and an indented unsupported block comment.
	if !strings.Contains(result, "Parent paragraph") {
		t.Errorf("Expected parent content, got %q", result)
	}
	if !strings.Contains(result, "<!-- Unsupported Notion block type:") {
		t.Errorf("Expected unsupported block comment, got %q", result)
	}
}
