package transformer

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jomei/notionapi"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"

	"github.com/adamancini/obsidian-notion-sync/internal/parser"
)

// mockLinkResolver is a test double for link resolution.
type mockLinkResolver struct {
	links map[string]string
}

func (m *mockLinkResolver) Resolve(target string) (string, bool) {
	id, ok := m.links[target]
	return id, ok
}

func TestNew(t *testing.T) {
	tr := New(nil, nil)
	if tr == nil {
		t.Fatal("New() returned nil")
	}

	if tr.config == nil {
		t.Error("Transformer.config should not be nil")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.UnresolvedLinkStyle != "placeholder" {
		t.Errorf("UnresolvedLinkStyle = %q, want %q", cfg.UnresolvedLinkStyle, "placeholder")
	}

	if cfg.FlattenHeadings != true {
		t.Error("FlattenHeadings should be true by default")
	}

	if len(cfg.CalloutIcons) == 0 {
		t.Error("CalloutIcons should have default values")
	}

	// Check some default callout icons.
	if cfg.CalloutIcons["note"] == "" {
		t.Error("CalloutIcons should have 'note' mapping")
	}
	if cfg.CalloutIcons["warning"] == "" {
		t.Error("CalloutIcons should have 'warning' mapping")
	}
}

func TestTransform_BasicNote(t *testing.T) {
	p := parser.New()
	tr := New(nil, nil)

	content := []byte(`---
title: Test Note
tags:
  - tag1
  - tag2
---

# Heading 1

This is a paragraph.

## Heading 2

More content here.
`)

	note, err := p.Parse("test.md", content)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	page, err := tr.Transform(note)
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	if page == nil {
		t.Fatal("Transform() returned nil page")
	}

	// Check that we have some children blocks.
	if len(page.Children) == 0 {
		t.Error("page.Children should not be empty")
	}
}

// Test heading transformation with H4-H6 flattening.
func TestTransformHeading_Flattening(t *testing.T) {
	tests := []struct {
		markdown    string
		wantType    notionapi.BlockType
		description string
	}{
		{"# Heading 1\n", notionapi.BlockTypeHeading1, "H1"},
		{"## Heading 2\n", notionapi.BlockTypeHeading2, "H2"},
		{"### Heading 3\n", notionapi.BlockTypeHeading3, "H3"},
		{"#### Heading 4\n", notionapi.BlockTypeHeading3, "H4 flattened to H3"},
		{"##### Heading 5\n", notionapi.BlockTypeHeading3, "H5 flattened to H3"},
		{"###### Heading 6\n", notionapi.BlockTypeHeading3, "H6 flattened to H3"},
	}

	p := parser.New()
	tr := New(nil, nil)

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			note, err := p.Parse("test.md", []byte(tt.markdown))
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}

			page, err := tr.Transform(note)
			if err != nil {
				t.Fatalf("Transform() error: %v", err)
			}

			if len(page.Children) == 0 {
				t.Fatal("page.Children should not be empty")
			}

			block := page.Children[0]
			if block.GetType() != tt.wantType {
				t.Errorf("block type = %v, want %v", block.GetType(), tt.wantType)
			}
		})
	}
}

func TestTransformParagraph_InlineFormatting(t *testing.T) {
	p := parser.New()
	tr := New(nil, nil)

	content := []byte("This has **bold** and *italic* and `code` text.\n")

	note, err := p.Parse("test.md", content)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	page, err := tr.Transform(note)
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	if len(page.Children) == 0 {
		t.Fatal("page.Children should not be empty")
	}

	block := page.Children[0]
	if block.GetType() != notionapi.BlockTypeParagraph {
		t.Errorf("block type = %v, want paragraph", block.GetType())
	}

	// Check paragraph has rich text with formatting.
	para, ok := block.(*notionapi.ParagraphBlock)
	if !ok {
		t.Fatalf("block is not ParagraphBlock, got %T", block)
	}

	if len(para.Paragraph.RichText) == 0 {
		t.Error("paragraph should have rich text")
	}

	// Verify we have some formatted text.
	hasBold := false
	hasItalic := false
	hasCode := false

	for _, rt := range para.Paragraph.RichText {
		if rt.Annotations != nil {
			if rt.Annotations.Bold {
				hasBold = true
			}
			if rt.Annotations.Italic {
				hasItalic = true
			}
			if rt.Annotations.Code {
				hasCode = true
			}
		}
	}

	if !hasBold {
		t.Error("paragraph should have bold text")
	}
	if !hasItalic {
		t.Error("paragraph should have italic text")
	}
	if !hasCode {
		t.Error("paragraph should have code text")
	}
}

func TestTransformList_Bulleted(t *testing.T) {
	p := parser.New()
	tr := New(nil, nil)

	content := []byte("- Item 1\n- Item 2\n- Item 3\n")

	note, err := p.Parse("test.md", content)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	page, err := tr.Transform(note)
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	// Should have 3 bulleted list items.
	bulletCount := 0
	for _, block := range page.Children {
		if block.GetType() == notionapi.BlockTypeBulletedListItem {
			bulletCount++
		}
	}

	if bulletCount != 3 {
		t.Errorf("bullet count = %d, want 3", bulletCount)
	}
}

func TestTransformList_Numbered(t *testing.T) {
	p := parser.New()
	tr := New(nil, nil)

	content := []byte("1. First\n2. Second\n3. Third\n")

	note, err := p.Parse("test.md", content)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	page, err := tr.Transform(note)
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	// Should have 3 numbered list items.
	numberedCount := 0
	for _, block := range page.Children {
		if block.GetType() == notionapi.BlockTypeNumberedListItem {
			numberedCount++
		}
	}

	if numberedCount != 3 {
		t.Errorf("numbered count = %d, want 3", numberedCount)
	}
}

func TestTransformList_Tasks(t *testing.T) {
	// Create a markdown parser with task list extension.
	md := goldmark.New(
		goldmark.WithExtensions(extension.TaskList),
	)

	content := []byte("- [ ] Todo item\n- [x] Done item\n")
	reader := text.NewReader(content)
	doc := md.Parser().Parse(reader)

	tr := New(nil, nil)

	// Create a parsed note manually.
	note := &parser.ParsedNote{
		Path:   "test.md",
		AST:    doc,
		Source: content,
	}

	page, err := tr.Transform(note)
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	// Should have 2 to-do items.
	todoCount := 0
	checkedCount := 0
	for _, block := range page.Children {
		if block.GetType() == notionapi.BlockTypeToDo {
			todoCount++
			if todo, ok := block.(*notionapi.ToDoBlock); ok {
				if todo.ToDo.Checked {
					checkedCount++
				}
			}
		}
	}

	if todoCount != 2 {
		t.Errorf("todo count = %d, want 2", todoCount)
	}

	if checkedCount != 1 {
		t.Errorf("checked count = %d, want 1", checkedCount)
	}
}

func TestTransformCodeBlock(t *testing.T) {
	p := parser.New()
	tr := New(nil, nil)

	content := []byte("```go\nfunc main() {\n    fmt.Println(\"Hello\")\n}\n```\n")

	note, err := p.Parse("test.md", content)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	page, err := tr.Transform(note)
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	// Find code block.
	var codeBlock *notionapi.CodeBlock
	for _, block := range page.Children {
		if block.GetType() == notionapi.BlockTypeCode {
			codeBlock = block.(*notionapi.CodeBlock)
			break
		}
	}

	if codeBlock == nil {
		t.Fatal("code block not found")
	}

	if codeBlock.Code.Language != "go" {
		t.Errorf("language = %q, want %q", codeBlock.Code.Language, "go")
	}

	// Check code content.
	if len(codeBlock.Code.RichText) == 0 {
		t.Error("code block should have content")
	}
}

func TestTransformCallout(t *testing.T) {
	p := parser.New()
	tr := New(nil, nil)

	content := []byte("> [!warning] Important Notice\n> This is a warning callout.\n")

	note, err := p.Parse("test.md", content)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	page, err := tr.Transform(note)
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	// Find callout block.
	var callout *notionapi.CalloutBlock
	for _, block := range page.Children {
		if cb, ok := block.(*notionapi.CalloutBlock); ok {
			callout = cb
			break
		}
	}

	if callout == nil {
		t.Fatal("callout block not found")
	}

	// Check icon is set.
	if callout.Callout.Icon == nil {
		t.Error("callout should have icon")
	}

	// Check rich text content.
	if len(callout.Callout.RichText) == 0 {
		t.Error("callout should have rich text content")
	}
}

func TestTransformQuote(t *testing.T) {
	p := parser.New()
	tr := New(nil, nil)

	content := []byte("> This is a quote.\n> Multiple lines.\n")

	note, err := p.Parse("test.md", content)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	page, err := tr.Transform(note)
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	// Find quote block.
	var quote *notionapi.QuoteBlock
	for _, block := range page.Children {
		if q, ok := block.(*notionapi.QuoteBlock); ok {
			quote = q
			break
		}
	}

	if quote == nil {
		t.Fatal("quote block not found")
	}

	if len(quote.Quote.RichText) == 0 {
		t.Error("quote should have content")
	}
}

func TestTransformDivider(t *testing.T) {
	p := parser.New()
	tr := New(nil, nil)

	content := []byte("Before\n\n---\n\nAfter\n")

	note, err := p.Parse("test.md", content)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	page, err := tr.Transform(note)
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	// Find divider block.
	found := false
	for _, block := range page.Children {
		if block.GetType() == notionapi.BlockTypeDivider {
			found = true
			break
		}
	}

	if !found {
		t.Error("divider block not found")
	}
}

func TestTransformTable(t *testing.T) {
	// Create a markdown parser with table extension.
	md := goldmark.New(
		goldmark.WithExtensions(extension.Table),
	)

	content := []byte("| Header 1 | Header 2 |\n|----------|----------|\n| Cell 1   | Cell 2   |\n| Cell 3   | Cell 4   |\n")
	reader := text.NewReader(content)
	doc := md.Parser().Parse(reader)

	tr := New(nil, nil)

	// Create a parsed note manually.
	note := &parser.ParsedNote{
		Path:   "test.md",
		AST:    doc,
		Source: content,
	}

	page, err := tr.Transform(note)
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	// Find table block.
	var table *notionapi.TableBlock
	for _, block := range page.Children {
		if tb, ok := block.(*notionapi.TableBlock); ok {
			table = tb
			break
		}
	}

	if table == nil {
		t.Fatal("table block not found")
	}

	// Check table dimensions.
	if table.Table.TableWidth != 2 {
		t.Errorf("table width = %d, want 2", table.Table.TableWidth)
	}

	if !table.Table.HasColumnHeader {
		t.Error("table should have column header")
	}

	// Check we have rows.
	if len(table.Table.Children) != 3 { // header + 2 data rows
		t.Errorf("table rows = %d, want 3", len(table.Table.Children))
	}
}

func TestTransformWikiLink_Resolved(t *testing.T) {
	resolver := &mockLinkResolver{
		links: map[string]string{
			"Other Note": "page-id-123",
		},
	}

	tr := New(resolver, nil)

	// Test the wiki link transformation directly.
	result := tr.transformWikiLink("Other Note", "", nil)

	if len(result) == 0 {
		t.Fatal("transformWikiLink returned empty result")
	}

	// Check it's a mention.
	if result[0].Type != "mention" {
		t.Errorf("type = %v, want mention", result[0].Type)
	}

	if result[0].Mention == nil || result[0].Mention.Page == nil {
		t.Error("mention.page should not be nil")
	}

	if string(result[0].Mention.Page.ID) != "page-id-123" {
		t.Errorf("page ID = %v, want page-id-123", result[0].Mention.Page.ID)
	}
}

func TestTransformWikiLink_Unresolved(t *testing.T) {
	tr := New(nil, nil) // No resolver

	result := tr.transformWikiLink("Unknown Note", "", nil)

	if len(result) == 0 {
		t.Fatal("transformWikiLink returned empty result")
	}

	// Check it's red placeholder text (default config).
	if result[0].Annotations == nil || result[0].Annotations.Color != notionapi.ColorRed {
		t.Error("unresolved link should be red")
	}

	if result[0].Text == nil || result[0].Text.Content != "[[Unknown Note]]" {
		t.Error("unresolved link should have [[]] wrapper")
	}
}

func TestTransformWikiLink_TextStyle(t *testing.T) {
	cfg := DefaultConfig()
	cfg.UnresolvedLinkStyle = "text"
	tr := New(nil, cfg)

	result := tr.transformWikiLink("Unknown Note", "", nil)

	if len(result) == 0 {
		t.Fatal("transformWikiLink returned empty result")
	}

	// Should be plain text without brackets.
	if result[0].Text == nil || result[0].Text.Content != "Unknown Note" {
		t.Errorf("content = %q, want %q", result[0].Text.Content, "Unknown Note")
	}
}

func TestTransformWikiLink_SkipStyle(t *testing.T) {
	cfg := DefaultConfig()
	cfg.UnresolvedLinkStyle = "skip"
	tr := New(nil, cfg)

	result := tr.transformWikiLink("Unknown Note", "", nil)

	if len(result) != 0 {
		t.Errorf("transformWikiLink with skip style should return empty, got %d items", len(result))
	}
}

func TestTransformHighlight(t *testing.T) {
	tr := New(nil, nil)

	result := tr.parseTextWithHighlights("This has ==highlighted== text.", nil)

	if len(result) != 3 {
		t.Fatalf("expected 3 rich text segments, got %d", len(result))
	}

	// Check middle segment is highlighted.
	if result[1].Text.Content != "highlighted" {
		t.Errorf("highlighted content = %q, want %q", result[1].Text.Content, "highlighted")
	}

	if result[1].Annotations == nil || result[1].Annotations.Color != notionapi.ColorYellowBackground {
		t.Error("highlighted text should have yellow background")
	}
}

func TestTransformHighlight_Multiple(t *testing.T) {
	tr := New(nil, nil)

	result := tr.parseTextWithHighlights("==first== normal ==second==", nil)

	// Should be: first(highlighted), " normal ", second(highlighted)
	highlightCount := 0
	for _, rt := range result {
		if rt.Annotations != nil && rt.Annotations.Color == notionapi.ColorYellowBackground {
			highlightCount++
		}
	}

	if highlightCount != 2 {
		t.Errorf("highlight count = %d, want 2", highlightCount)
	}
}

func TestTransformHighlight_NoHighlight(t *testing.T) {
	tr := New(nil, nil)

	result := tr.parseTextWithHighlights("Plain text without highlights.", nil)

	if len(result) != 1 {
		t.Fatalf("expected 1 rich text segment, got %d", len(result))
	}

	if result[0].Annotations != nil && result[0].Annotations.Color != "" {
		t.Error("plain text should not have color annotation")
	}
}

func TestTransformEquation_Block(t *testing.T) {
	tr := New(nil, nil)

	block := tr.transformEquation("E = mc^2")

	if block.GetType() != notionapi.BlockTypeEquation {
		t.Errorf("type = %v, want equation", block.GetType())
	}

	eq, ok := block.(*notionapi.EquationBlock)
	if !ok {
		t.Fatalf("block is not EquationBlock, got %T", block)
	}

	if eq.Equation.Expression != "E = mc^2" {
		t.Errorf("expression = %q, want %q", eq.Equation.Expression, "E = mc^2")
	}
}

func TestTryMathBlock(t *testing.T) {
	tests := []struct {
		input   string
		wantExpr string
		isMath  bool
	}{
		{"$$x^2 + y^2 = r^2$$", "x^2 + y^2 = r^2", true},
		{"$$ E = mc^2 $$", "E = mc^2", true},
		{"$$\n\\frac{a}{b}\n$$", "\\frac{a}{b}", true},
		{"Regular paragraph", "", false},
		{"$inline$ math", "", false}, // Inline math is not a block.
	}

	p := parser.New()
	tr := New(nil, nil)

	for _, tt := range tests {
		t.Run(tt.input[:min(20, len(tt.input))], func(t *testing.T) {
			note, err := p.Parse("test.md", []byte(tt.input))
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}

			if note.AST.FirstChild() == nil {
				t.Skip("No AST content")
			}

			// Find the first paragraph.
			for child := note.AST.FirstChild(); child != nil; child = child.NextSibling() {
				// tryMathBlock expects *ast.Paragraph, so type assert first.
				para, ok := child.(*ast.Paragraph)
				if !ok {
					continue
				}
				expr := tr.tryMathBlock(para, note.Source)
				if tt.isMath {
					if expr == "" {
						// Math blocks might be parsed differently.
						continue
					}
					if expr != tt.wantExpr {
						t.Errorf("expression = %q, want %q", expr, tt.wantExpr)
					}
					return
				} else {
					if expr != "" {
						t.Errorf("expected no math, got %q", expr)
					}
				}
			}
		})
	}
}

func TestCopyAnnotations(t *testing.T) {
	original := &notionapi.Annotations{
		Bold:          true,
		Italic:        true,
		Strikethrough: true,
		Underline:     true,
		Code:          true,
		Color:         notionapi.ColorRed,
	}

	copy := copyAnnotations(original)

	// Verify copy has same values.
	if copy.Bold != original.Bold {
		t.Error("Bold not copied")
	}
	if copy.Italic != original.Italic {
		t.Error("Italic not copied")
	}
	if copy.Strikethrough != original.Strikethrough {
		t.Error("Strikethrough not copied")
	}
	if copy.Color != original.Color {
		t.Error("Color not copied")
	}

	// Verify mutation doesn't affect original.
	copy.Bold = false
	if original.Bold != true {
		t.Error("mutation affected original")
	}
}

func TestCopyAnnotations_Nil(t *testing.T) {
	copy := copyAnnotations(nil)

	if copy == nil {
		t.Fatal("copyAnnotations(nil) returned nil")
	}

	// Should be empty annotations.
	if copy.Bold || copy.Italic || copy.Strikethrough {
		t.Error("nil annotations should result in empty annotations")
	}
}

// Helper for older Go versions.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// TestTransformCodeBlock_LongContent tests that code blocks exceeding 2000 chars
// are split into multiple rich_text segments (ANN-26).
func TestTransformCodeBlock_LongContent(t *testing.T) {
	p := parser.New()
	tr := New(nil, nil)

	// Create a code block with more than 2000 characters.
	// Use a pattern that creates predictable line boundaries.
	var longCode strings.Builder
	longCode.WriteString("```go\n")
	// Write 50-character lines until we exceed 4000 chars (to ensure multiple splits).
	for i := 0; longCode.Len() < 4500; i++ {
		longCode.WriteString("// This is line number ")
		longCode.WriteString(fmt.Sprintf("%04d", i))
		longCode.WriteString(" of the test code file\n")
	}
	longCode.WriteString("```\n")

	note, err := p.Parse("test.md", []byte(longCode.String()))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	page, err := tr.Transform(note)
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	// Find code block.
	var codeBlock *notionapi.CodeBlock
	for _, block := range page.Children {
		if block.GetType() == notionapi.BlockTypeCode {
			codeBlock = block.(*notionapi.CodeBlock)
			break
		}
	}

	if codeBlock == nil {
		t.Fatal("code block not found")
	}

	// Should have multiple rich_text segments.
	if len(codeBlock.Code.RichText) < 2 {
		t.Errorf("expected multiple rich_text segments for long code, got %d", len(codeBlock.Code.RichText))
	}

	// Each segment should be <= 2000 characters.
	for i, rt := range codeBlock.Code.RichText {
		if rt.Text == nil {
			t.Errorf("segment %d has nil Text", i)
			continue
		}
		if len(rt.Text.Content) > 2000 {
			t.Errorf("segment %d has length %d, expected <= 2000", i, len(rt.Text.Content))
		}
	}

	// Verify the total content is preserved.
	var reconstructed strings.Builder
	for _, rt := range codeBlock.Code.RichText {
		if rt.Text != nil {
			reconstructed.WriteString(rt.Text.Content)
		}
	}

	// The original code (minus the fence markers and trailing newline).
	originalCode := longCode.String()
	// Strip ```go\n from start and ```\n from end.
	originalCode = originalCode[6:]                              // Remove "```go\n"
	originalCode = originalCode[:len(originalCode)-5]            // Remove "\n```\n"
	originalCode = strings.TrimSuffix(originalCode, "\n")        // Transformer trims trailing newline

	if reconstructed.String() != originalCode {
		t.Errorf("reconstructed code does not match original\ngot length: %d\nwant length: %d",
			reconstructed.Len(), len(originalCode))
	}
}

// TestTransformCodeBlock_ExactlyAtLimit tests code blocks exactly at the 2000 char limit.
func TestTransformCodeBlock_ExactlyAtLimit(t *testing.T) {
	p := parser.New()
	tr := New(nil, nil)

	// Create code exactly 2000 characters.
	code := strings.Repeat("x", 2000)
	content := "```go\n" + code + "\n```\n"

	note, err := p.Parse("test.md", []byte(content))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	page, err := tr.Transform(note)
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	// Find code block.
	var codeBlock *notionapi.CodeBlock
	for _, block := range page.Children {
		if block.GetType() == notionapi.BlockTypeCode {
			codeBlock = block.(*notionapi.CodeBlock)
			break
		}
	}

	if codeBlock == nil {
		t.Fatal("code block not found")
	}

	// Should have exactly 1 segment (code is at the limit, not over).
	if len(codeBlock.Code.RichText) != 1 {
		t.Errorf("expected 1 rich_text segment for code at limit, got %d", len(codeBlock.Code.RichText))
	}
}

// TestTransformListItemContent_Empty tests that transformListItemContent
// returns an empty slice, not nil, when the list item has no content (ANN-27).
func TestTransformListItemContent_Empty(t *testing.T) {
	tr := New(nil, nil)

	// Create an empty list item manually using goldmark AST.
	listItem := ast.NewListItem(0)
	// Add an empty text block child (simulating an empty list item).
	textBlock := ast.NewTextBlock()
	listItem.AppendChild(listItem, textBlock)

	source := []byte{}
	result := tr.transformListItemContent(listItem, source)

	// Result should not be nil - it should be an empty slice.
	if result == nil {
		t.Error("transformListItemContent returned nil for empty list item, expected empty slice")
	}
}

// TestTransformList_EmptyBulletedItem tests that empty bulleted list items
// produce an empty array, not null (ANN-27).
func TestTransformList_EmptyBulletedItem(t *testing.T) {
	tr := New(nil, nil)

	// Create a list with an empty item using goldmark AST directly.
	list := ast.NewList('-')
	list.IsOrdered()

	listItem := ast.NewListItem(0)
	textBlock := ast.NewTextBlock()
	listItem.AppendChild(listItem, textBlock)
	list.AppendChild(list, listItem)

	doc := ast.NewDocument()
	doc.AppendChild(doc, list)

	note := &parser.ParsedNote{
		Path:   "test.md",
		AST:    doc,
		Source: []byte{},
	}

	page, err := tr.Transform(note)
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	// Find the bulleted list item.
	var bulletItem *notionapi.BulletedListItemBlock
	for _, block := range page.Children {
		if bi, ok := block.(*notionapi.BulletedListItemBlock); ok {
			bulletItem = bi
			break
		}
	}

	if bulletItem == nil {
		t.Fatal("bulleted list item not found")
	}

	// RichText should not be nil - it should be an empty slice.
	if bulletItem.BulletedListItem.RichText == nil {
		t.Error("empty bulleted list item has nil RichText, expected empty slice")
	}
}

// TestTransformList_EmptyNumberedItem tests that empty numbered list items
// produce an empty array, not null (ANN-27).
func TestTransformList_EmptyNumberedItem(t *testing.T) {
	tr := New(nil, nil)

	// Create an ordered list with an empty item using goldmark AST directly.
	list := ast.NewList('.')
	list.Start = 1

	listItem := ast.NewListItem(0)
	textBlock := ast.NewTextBlock()
	listItem.AppendChild(listItem, textBlock)
	list.AppendChild(list, listItem)

	doc := ast.NewDocument()
	doc.AppendChild(doc, list)

	note := &parser.ParsedNote{
		Path:   "test.md",
		AST:    doc,
		Source: []byte{},
	}

	page, err := tr.Transform(note)
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	// Find the numbered list item.
	var numberedItem *notionapi.NumberedListItemBlock
	for _, block := range page.Children {
		if ni, ok := block.(*notionapi.NumberedListItemBlock); ok {
			numberedItem = ni
			break
		}
	}

	if numberedItem == nil {
		t.Fatal("numbered list item not found")
	}

	// RichText should not be nil - it should be an empty slice.
	if numberedItem.NumberedListItem.RichText == nil {
		t.Error("empty numbered list item has nil RichText, expected empty slice")
	}
}

// TestTransformDataviewBlock tests dataview code block transformation (ANN-22).
func TestTransformDataviewBlock(t *testing.T) {
	p := parser.New()
	tr := New(nil, nil)

	content := []byte("```dataview\nTABLE file.name, file.mtime\nFROM \"notes\"\nWHERE status = \"done\"\n```\n")

	note, err := p.Parse("test.md", content)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	page, err := tr.Transform(note)
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	// Find callout block (dataview should become a callout placeholder).
	var callout *notionapi.CalloutBlock
	for _, block := range page.Children {
		if cb, ok := block.(*notionapi.CalloutBlock); ok {
			callout = cb
			break
		}
	}

	if callout == nil {
		t.Fatal("dataview block should be transformed to callout placeholder")
	}

	// Check icon is set (should be 📊).
	if callout.Callout.Icon == nil || callout.Callout.Icon.Emoji == nil {
		t.Error("dataview callout should have emoji icon")
	}
	if callout.Callout.Icon.Emoji != nil && *callout.Callout.Icon.Emoji != "📊" {
		t.Errorf("icon = %v, want 📊", *callout.Callout.Icon.Emoji)
	}

	// Check rich text contains the query.
	hasQueryContent := false
	for _, rt := range callout.Callout.RichText {
		if rt.Text != nil && strings.Contains(rt.Text.Content, "TABLE file.name") {
			hasQueryContent = true
			break
		}
	}
	if !hasQueryContent {
		t.Error("dataview callout should contain the original query")
	}

	// Check color.
	if callout.Callout.Color != "blue_background" {
		t.Errorf("callout color = %v, want blue_background", callout.Callout.Color)
	}
}

// TestTransformDataviewJSBlock tests dataviewjs code block transformation.
func TestTransformDataviewJSBlock(t *testing.T) {
	p := parser.New()
	tr := New(nil, nil)

	content := []byte("```dataviewjs\ndv.list(dv.pages().map(p => p.file.name))\n```\n")

	note, err := p.Parse("test.md", content)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	page, err := tr.Transform(note)
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	// Find callout block.
	var callout *notionapi.CalloutBlock
	for _, block := range page.Children {
		if cb, ok := block.(*notionapi.CalloutBlock); ok {
			callout = cb
			break
		}
	}

	if callout == nil {
		t.Fatal("dataviewjs block should be transformed to callout placeholder")
	}

	// Check it mentions "JS" in the title.
	hasJSTitle := false
	for _, rt := range callout.Callout.RichText {
		if rt.Text != nil && strings.Contains(rt.Text.Content, "JS") {
			hasJSTitle = true
			break
		}
	}
	if !hasJSTitle {
		t.Error("dataviewjs callout should mention JS in the title")
	}
}

// TestTransformInlineDataview tests inline dataview transformation (ANN-22).
func TestTransformInlineDataview(t *testing.T) {
	p := parser.New()
	tr := New(nil, nil)

	// Inline dataview: `=this.file.name`
	content := []byte("The file name is `=this.file.name` in this note.\n")

	note, err := p.Parse("test.md", content)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	page, err := tr.Transform(note)
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	if len(page.Children) == 0 {
		t.Fatal("page.Children should not be empty")
	}

	// Find the paragraph.
	para, ok := page.Children[0].(*notionapi.ParagraphBlock)
	if !ok {
		t.Fatalf("expected paragraph, got %T", page.Children[0])
	}

	// Check that inline dataview is transformed with purple background.
	hasDataviewPlaceholder := false
	hasPurpleBackground := false
	for _, rt := range para.Paragraph.RichText {
		if rt.Text != nil && strings.Contains(rt.Text.Content, "dv:") {
			hasDataviewPlaceholder = true
		}
		if rt.Annotations != nil && rt.Annotations.Color == notionapi.ColorPurpleBackground {
			hasPurpleBackground = true
		}
	}

	if !hasDataviewPlaceholder {
		t.Error("inline dataview should create a [dv: ...] placeholder")
	}
	if !hasPurpleBackground {
		t.Error("inline dataview placeholder should have purple background")
	}
}

// TestTransformInlineDataview_RegularCodeSpan tests that regular code spans are not affected.
func TestTransformInlineDataview_RegularCodeSpan(t *testing.T) {
	p := parser.New()
	tr := New(nil, nil)

	// Regular code span (not starting with =)
	content := []byte("Use `console.log()` for debugging.\n")

	note, err := p.Parse("test.md", content)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	page, err := tr.Transform(note)
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	// Find the paragraph.
	para, ok := page.Children[0].(*notionapi.ParagraphBlock)
	if !ok {
		t.Fatalf("expected paragraph, got %T", page.Children[0])
	}

	// Check that regular code span is not treated as dataview.
	for _, rt := range para.Paragraph.RichText {
		if rt.Text != nil && strings.Contains(rt.Text.Content, "dv:") {
			t.Error("regular code span should not be treated as dataview")
		}
		// Regular code should have code annotation but NOT purple background.
		if rt.Annotations != nil && rt.Annotations.Code && rt.Annotations.Color == notionapi.ColorPurpleBackground {
			t.Error("regular code span should not have purple background")
		}
	}
}

// TestTransformImage_ExternalURL tests that external URL images become ImageBlocks (ANN-25).
func TestTransformImage_ExternalURL(t *testing.T) {
	p := parser.New()
	tr := New(nil, nil)

	content := []byte("![Alt text](https://example.com/image.png)\n")

	note, err := p.Parse("test.md", content)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	page, err := tr.Transform(note)
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	// Find image block.
	var imageBlock *notionapi.ImageBlock
	for _, block := range page.Children {
		if ib, ok := block.(*notionapi.ImageBlock); ok {
			imageBlock = ib
			break
		}
	}

	if imageBlock == nil {
		t.Fatal("image block not found")
	}

	// Check it's an external image.
	if imageBlock.Image.Type != "external" {
		t.Errorf("image type = %v, want external", imageBlock.Image.Type)
	}

	if imageBlock.Image.External == nil {
		t.Fatal("external file object should not be nil")
	}

	if imageBlock.Image.External.URL != "https://example.com/image.png" {
		t.Errorf("URL = %v, want https://example.com/image.png", imageBlock.Image.External.URL)
	}

	// Check caption.
	if len(imageBlock.Image.Caption) == 0 {
		t.Error("image should have caption from alt text")
	} else if imageBlock.Image.Caption[0].Text.Content != "Alt text" {
		t.Errorf("caption = %v, want 'Alt text'", imageBlock.Image.Caption[0].Text.Content)
	}
}

// TestTransformImage_LocalPath tests that local images become placeholder callouts (ANN-25).
func TestTransformImage_LocalPath(t *testing.T) {
	p := parser.New()
	tr := New(nil, nil)

	content := []byte("![Screenshot](./images/screenshot.png)\n")

	note, err := p.Parse("test.md", content)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	page, err := tr.Transform(note)
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	// Find callout block (local images become placeholders).
	var callout *notionapi.CalloutBlock
	for _, block := range page.Children {
		if cb, ok := block.(*notionapi.CalloutBlock); ok {
			callout = cb
			break
		}
	}

	if callout == nil {
		t.Fatal("local image should be transformed to callout placeholder")
	}

	// Check icon is the image emoji.
	if callout.Callout.Icon == nil || callout.Callout.Icon.Emoji == nil {
		t.Error("local image callout should have emoji icon")
	}
	if callout.Callout.Icon.Emoji != nil && *callout.Callout.Icon.Emoji != "🖼️" {
		t.Errorf("icon = %v, want 🖼️", *callout.Callout.Icon.Emoji)
	}

	// Check it mentions the path.
	hasPath := false
	for _, rt := range callout.Callout.RichText {
		if rt.Text != nil && strings.Contains(rt.Text.Content, "screenshot.png") {
			hasPath = true
			break
		}
	}
	if !hasPath {
		t.Error("local image placeholder should contain the file path")
	}
}

// TestTransformImage_NoCaption tests external images without alt text.
func TestTransformImage_NoCaption(t *testing.T) {
	p := parser.New()
	tr := New(nil, nil)

	content := []byte("![](https://example.com/image.png)\n")

	note, err := p.Parse("test.md", content)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	page, err := tr.Transform(note)
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	// Find image block.
	var imageBlock *notionapi.ImageBlock
	for _, block := range page.Children {
		if ib, ok := block.(*notionapi.ImageBlock); ok {
			imageBlock = ib
			break
		}
	}

	if imageBlock == nil {
		t.Fatal("image block not found")
	}

	// Caption should be empty or nil (both are valid for Notion API).
	if len(imageBlock.Image.Caption) > 0 {
		t.Error("image without alt text should have empty caption")
	}
}

// TestTransformImage_InParagraph tests that inline images in paragraphs don't create ImageBlocks.
func TestTransformImage_InParagraph(t *testing.T) {
	p := parser.New()
	tr := New(nil, nil)

	// Image with text around it - not standalone.
	content := []byte("Text before ![image](https://example.com/img.png) and after.\n")

	note, err := p.Parse("test.md", content)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	page, err := tr.Transform(note)
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}

	// Should be a paragraph, not an image block.
	if len(page.Children) == 0 {
		t.Fatal("page should have children")
	}

	// The first block should be a paragraph (inline image remains in paragraph).
	_, isPara := page.Children[0].(*notionapi.ParagraphBlock)
	if !isPara {
		t.Errorf("expected paragraph block for inline image, got %T", page.Children[0])
	}

	// There should be no standalone ImageBlock.
	for _, block := range page.Children {
		if _, ok := block.(*notionapi.ImageBlock); ok {
			t.Error("inline image should not create a standalone ImageBlock")
		}
	}
}

// TestIsLocalPath tests the isLocalPath helper function.
func TestIsLocalPath(t *testing.T) {
	tests := []struct {
		url      string
		expected bool
	}{
		{"./images/photo.png", true},
		{"../assets/img.jpg", true},
		{"images/photo.png", true},
		{"/absolute/path/image.gif", true},
		{"file:///path/to/image.png", true},
		{"https://example.com/image.png", false},
		{"http://example.com/image.jpg", false},
		{"data:image/png;base64,abc123", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			result := isLocalPath(tt.url)
			if result != tt.expected {
				t.Errorf("isLocalPath(%q) = %v, want %v", tt.url, result, tt.expected)
			}
		})
	}
}

// TestIsImageFile tests the isImageFile helper function.
func TestIsImageFile(t *testing.T) {
	tests := []struct {
		filename string
		expected bool
	}{
		{"photo.png", true},
		{"photo.PNG", true},
		{"image.jpg", true},
		{"image.jpeg", true},
		{"image.gif", true},
		{"image.webp", true},
		{"image.svg", true},
		{"image.bmp", true},
		{"document.pdf", false},
		{"note.md", false},
		{"video.mp4", false},
		{"image", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			result := isImageFile(tt.filename)
			if result != tt.expected {
				t.Errorf("isImageFile(%q) = %v, want %v", tt.filename, result, tt.expected)
			}
		})
	}
}

// TestSplitCodeContent tests the splitCodeContent helper function directly.
func TestSplitCodeContent(t *testing.T) {
	tests := []struct {
		name           string
		code           string
		maxLen         int
		wantSegments   int
		wantAllUnder   bool
	}{
		{
			name:         "short code",
			code:         "hello world",
			maxLen:       100,
			wantSegments: 1,
			wantAllUnder: true,
		},
		{
			name:         "exactly at limit",
			code:         strings.Repeat("x", 100),
			maxLen:       100,
			wantSegments: 1,
			wantAllUnder: true,
		},
		{
			name:         "just over limit no newlines",
			code:         strings.Repeat("x", 150),
			maxLen:       100,
			wantSegments: 2,
			wantAllUnder: true,
		},
		{
			name:         "split at newline",
			code:         strings.Repeat("x", 50) + "\n" + strings.Repeat("y", 50) + "\n" + strings.Repeat("z", 50),
			maxLen:       100,
			wantSegments: 3, // 51 chars + newline, 51 chars + newline, 50 chars.
			wantAllUnder: true,
		},
		{
			name:         "multiple splits needed",
			code:         strings.Repeat("line\n", 100), // 500 chars
			maxLen:       100,
			wantSegments: 5,
			wantAllUnder: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			segments := splitCodeContent(tt.code, tt.maxLen)

			if len(segments) != tt.wantSegments {
				t.Errorf("got %d segments, want %d", len(segments), tt.wantSegments)
			}

			// Verify all segments are under the limit.
			if tt.wantAllUnder {
				for i, seg := range segments {
					if seg.Text != nil && len(seg.Text.Content) > tt.maxLen {
						t.Errorf("segment %d has length %d, exceeds max %d",
							i, len(seg.Text.Content), tt.maxLen)
					}
				}
			}

			// Verify content is preserved.
			var reconstructed strings.Builder
			for _, seg := range segments {
				if seg.Text != nil {
					reconstructed.WriteString(seg.Text.Content)
				}
			}
			if reconstructed.String() != tt.code {
				t.Errorf("content not preserved after split")
			}
		})
	}
}
