//go:build e2e
// +build e2e

package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/adamancini/obsidian-notion-sync/internal/parser"
	"github.com/adamancini/obsidian-notion-sync/internal/state"
	"github.com/adamancini/obsidian-notion-sync/internal/transformer"
)

// =============================================================================
// Wiki-Link Tests
// =============================================================================

// TestWikiLinks_BasicResolution tests basic wiki-link resolution across notes.
func TestWikiLinks_BasicResolution(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	p := parser.New()
	trans := transformer.New(nil, nil) // No link resolver, default config

	// Step 1: Create target note first.
	targetContent := Markdown.SimpleNote("Target Note", "This is the target of a wiki-link.")
	f.WriteMarkdownFile("target-note.md", targetContent)

	doc, err := p.ParseFile(f.VaultPath, "target-note.md")
	if err != nil {
		t.Fatalf("failed to parse target: %v", err)
	}

	notionPage, err := trans.Transform(doc)
	if err != nil {
		t.Fatalf("failed to transform target: %v", err)
	}

	result, err := f.NotionClient.CreatePageUnderPage(ctx, f.ParentPageID, notionPage)
	if err != nil {
		t.Fatalf("failed to create target page: %v", err)
	}
	f.TrackPage(result.PageID)
	targetPageID := result.PageID

	// Record sync state for target.
	hash := state.HashContent([]byte(targetContent)).FullHash
	err = f.DB.SetState(&state.SyncState{
		ObsidianPath: "target-note.md",
		NotionPageID: targetPageID,
		ContentHash:  hash,
		Status:       "synced",
		LastSync:     time.Now(),
	})
	if err != nil {
		t.Fatalf("failed to set target state: %v", err)
	}

	// Step 2: Create source note with wiki-link.
	sourceContent := Markdown.NoteWithWikiLinks(
		"Source Note",
		"This note links to another note.",
		[]string{"Target Note"},
	)
	f.WriteMarkdownFile("source-note.md", sourceContent)

	doc2, err := p.ParseFile(f.VaultPath, "source-note.md")
	if err != nil {
		t.Fatalf("failed to parse source: %v", err)
	}

	// Create link registry and resolver.
	linkRegistry := state.NewLinkRegistry(f.DB)

	// Register the link.
	err = linkRegistry.RegisterLink("source-note.md", "Target Note")
	if err != nil {
		t.Fatalf("failed to register link: %v", err)
	}

	// Resolve the link.
	resolved, found := linkRegistry.Resolve("Target Note")
	if !found {
		t.Error("expected to resolve wiki-link to Target Note")
	}
	if resolved != targetPageID {
		t.Errorf("resolved page ID = %s, want %s", resolved, targetPageID)
	}

	// Transform with link resolver.
	trans2 := transformer.New(linkRegistry, nil)
	notionPage2, err := trans2.Transform(doc2)
	if err != nil {
		t.Fatalf("failed to transform source: %v", err)
	}

	// Create source page.
	result2, err := f.NotionClient.CreatePageUnderPage(ctx, f.ParentPageID, notionPage2)
	if err != nil {
		t.Fatalf("failed to create source page: %v", err)
	}
	f.TrackPage(result2.PageID)

	// Verify both pages exist.
	f.AssertPageExists(ctx, targetPageID)
	f.AssertPageExists(ctx, result2.PageID)
}

// TestWikiLinks_UnresolvedLink tests handling of unresolved wiki-links.
func TestWikiLinks_UnresolvedLink(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	// Create note with link to non-existent page.
	content := Markdown.NoteWithWikiLinks(
		"Unresolved Link Note",
		"This note has an unresolved link.",
		[]string{"NonExistent Page"},
	)
	f.WriteMarkdownFile("unresolved-link.md", content)

	p := parser.New()
	doc, err := p.ParseFile(f.VaultPath, "unresolved-link.md")
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	// Register the unresolved link.
	linkRegistry := state.NewLinkRegistry(f.DB)
	err = linkRegistry.RegisterLink("unresolved-link.md", "NonExistent Page")
	if err != nil {
		t.Fatalf("failed to register link: %v", err)
	}

	// Verify link is unresolved.
	unresolved, err := linkRegistry.GetUnresolvedLinks()
	if err != nil {
		t.Fatalf("failed to get unresolved: %v", err)
	}

	found := false
	for _, link := range unresolved {
		if link.TargetName == "NonExistent Page" {
			found = true
		}
	}
	if !found {
		t.Error("expected to find unresolved link to NonExistent Page")
	}

	// Transform (should handle unresolved gracefully).
	cfg := transformer.DefaultConfig()
	cfg.UnresolvedLinkStyle = "placeholder"
	trans := transformer.New(linkRegistry, cfg)
	notionPage, err := trans.Transform(doc)
	if err != nil {
		t.Fatalf("failed to transform: %v", err)
	}

	// Create page (should succeed despite unresolved link).
	result, err := f.NotionClient.CreatePageUnderPage(ctx, f.ParentPageID, notionPage)
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}
	f.TrackPage(result.PageID)

	f.AssertPageExists(ctx, result.PageID)
}

// TestWikiLinks_WithHeadingAnchor tests wiki-links with heading anchors.
func TestWikiLinks_WithHeadingAnchor(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	p := parser.New()
	trans := transformer.New(nil, nil)

	// Create target note with headings.
	targetContent := `---
title: Note With Headings
---

# Section One

Content in section one.

## Subsection

More content.
`
	f.WriteMarkdownFile("note-with-headings.md", targetContent)

	doc, err := p.ParseFile(f.VaultPath, "note-with-headings.md")
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	notionPage, err := trans.Transform(doc)
	if err != nil {
		t.Fatalf("failed to transform: %v", err)
	}

	result, err := f.NotionClient.CreatePageUnderPage(ctx, f.ParentPageID, notionPage)
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}
	f.TrackPage(result.PageID)
	targetPageID := result.PageID

	// Record sync state.
	hash := state.HashContent([]byte(targetContent)).FullHash
	err = f.DB.SetState(&state.SyncState{
		ObsidianPath: "note-with-headings.md",
		NotionPageID: targetPageID,
		ContentHash:  hash,
		Status:       "synced",
		LastSync:     time.Now(),
	})
	if err != nil {
		t.Fatalf("failed to set state: %v", err)
	}

	// Create source with anchor link.
	sourceContent := `---
title: Anchor Link Test
---

See [[Note With Headings#Section One]] for details.
`
	f.WriteMarkdownFile("anchor-link.md", sourceContent)

	linkRegistry := state.NewLinkRegistry(f.DB)

	// Extended resolution should handle anchors.
	result2 := linkRegistry.ResolveExtended("Note With Headings#Section One", false)
	if !result2.Found {
		t.Error("expected to resolve link with anchor")
	}
	if result2.Heading != "Section One" {
		t.Errorf("heading = %s, want 'Section One'", result2.Heading)
	}
	if result2.PageID != targetPageID {
		t.Errorf("page ID = %s, want %s", result2.PageID, targetPageID)
	}
}

// =============================================================================
// Frontmatter Tests
// =============================================================================

// TestFrontmatter_ComplexTypes tests various frontmatter value types.
func TestFrontmatter_ComplexTypes(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	// Create note with complex frontmatter.
	content := `---
title: Complex Frontmatter
tags: [tag1, tag2, tag3]
status: draft
priority: 5
date: 2024-01-15
completed: false
aliases:
  - CFM Test
  - Frontmatter Example
---

Content with complex frontmatter.
`
	f.WriteMarkdownFile("complex-frontmatter.md", content)

	p := parser.New()
	doc, err := p.ParseFile(f.VaultPath, "complex-frontmatter.md")
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	// Verify frontmatter was parsed correctly.
	if doc.Frontmatter == nil {
		t.Fatal("frontmatter should not be nil")
	}

	// Check title.
	if title, ok := doc.Frontmatter["title"].(string); !ok || title != "Complex Frontmatter" {
		t.Errorf("title = %v, want 'Complex Frontmatter'", doc.Frontmatter["title"])
	}

	// Check tags (should be a slice).
	tags, ok := doc.Frontmatter["tags"].([]interface{})
	if !ok {
		t.Errorf("tags should be a slice, got %T", doc.Frontmatter["tags"])
	} else if len(tags) != 3 {
		t.Errorf("tags length = %d, want 3", len(tags))
	}

	// Check priority (should be numeric).
	priority, ok := doc.Frontmatter["priority"].(int)
	if !ok {
		t.Logf("priority type = %T", doc.Frontmatter["priority"])
	} else if priority != 5 {
		t.Errorf("priority = %d, want 5", priority)
	}

	// Transform and create.
	trans := transformer.New(nil, nil)
	notionPage, err := trans.Transform(doc)
	if err != nil {
		t.Fatalf("failed to transform: %v", err)
	}

	result, err := f.NotionClient.CreatePageUnderPage(ctx, f.ParentPageID, notionPage)
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}
	f.TrackPage(result.PageID)

	f.AssertPageExists(ctx, result.PageID)
}

// TestFrontmatter_EmptyValues tests handling of empty frontmatter values.
func TestFrontmatter_EmptyValues(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	// Create note with some empty values.
	content := `---
title: Empty Values Test
tags: []
description:
---

Content with empty frontmatter values.
`
	f.WriteMarkdownFile("empty-values.md", content)

	p := parser.New()
	doc, err := p.ParseFile(f.VaultPath, "empty-values.md")
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	trans := transformer.New(nil, nil)
	notionPage, err := trans.Transform(doc)
	if err != nil {
		t.Fatalf("failed to transform: %v", err)
	}

	result, err := f.NotionClient.CreatePageUnderPage(ctx, f.ParentPageID, notionPage)
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}
	f.TrackPage(result.PageID)

	f.AssertPageExists(ctx, result.PageID)
}

// TestFrontmatter_SpecialCharacters tests frontmatter with special characters.
func TestFrontmatter_SpecialCharacters(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	// Create note with special characters in frontmatter.
	content := `---
title: "Title with: colons and \"quotes\""
description: "Line 1\nLine 2"
tags: ["tag:with:colons", "tag with spaces"]
---

Content with special frontmatter.
`
	f.WriteMarkdownFile("special-chars.md", content)

	p := parser.New()
	doc, err := p.ParseFile(f.VaultPath, "special-chars.md")
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	trans := transformer.New(nil, nil)
	notionPage, err := trans.Transform(doc)
	if err != nil {
		t.Fatalf("failed to transform: %v", err)
	}

	result, err := f.NotionClient.CreatePageUnderPage(ctx, f.ParentPageID, notionPage)
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}
	f.TrackPage(result.PageID)

	f.AssertPageExists(ctx, result.PageID)
}

// =============================================================================
// Formatting Tests
// =============================================================================

// TestFormatting_NestedQuotes tests nested blockquotes.
func TestFormatting_NestedQuotes(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	content := `---
title: Nested Quotes
---

> Level 1 quote
> > Nested level 2 quote
> Back to level 1
`
	f.WriteMarkdownFile("nested-quotes.md", content)

	p := parser.New()
	doc, err := p.ParseFile(f.VaultPath, "nested-quotes.md")
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	trans := transformer.New(nil, nil)
	notionPage, err := trans.Transform(doc)
	if err != nil {
		t.Fatalf("failed to transform: %v", err)
	}

	result, err := f.NotionClient.CreatePageUnderPage(ctx, f.ParentPageID, notionPage)
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}
	f.TrackPage(result.PageID)

	f.AssertPageExists(ctx, result.PageID)
}

// TestFormatting_Tables tests markdown tables.
func TestFormatting_Tables(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	content := `---
title: Table Test
---

| Header 1 | Header 2 | Header 3 |
|----------|----------|----------|
| Cell 1   | Cell 2   | Cell 3   |
| Cell 4   | Cell 5   | Cell 6   |
`
	f.WriteMarkdownFile("table-test.md", content)

	p := parser.New()
	doc, err := p.ParseFile(f.VaultPath, "table-test.md")
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	trans := transformer.New(nil, nil)
	notionPage, err := trans.Transform(doc)
	if err != nil {
		t.Fatalf("failed to transform: %v", err)
	}

	result, err := f.NotionClient.CreatePageUnderPage(ctx, f.ParentPageID, notionPage)
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}
	f.TrackPage(result.PageID)

	f.AssertPageExists(ctx, result.PageID)
}

// TestFormatting_HorizontalRule tests horizontal rules.
func TestFormatting_HorizontalRule(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	content := `---
title: Horizontal Rule Test
---

Content before rule.

---

Content after rule.

***

More content.
`
	f.WriteMarkdownFile("hr-test.md", content)

	p := parser.New()
	doc, err := p.ParseFile(f.VaultPath, "hr-test.md")
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	trans := transformer.New(nil, nil)
	notionPage, err := trans.Transform(doc)
	if err != nil {
		t.Fatalf("failed to transform: %v", err)
	}

	result, err := f.NotionClient.CreatePageUnderPage(ctx, f.ParentPageID, notionPage)
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}
	f.TrackPage(result.PageID)

	f.AssertPageExists(ctx, result.PageID)
}

// TestFormatting_TaskLists tests task lists (todo items).
func TestFormatting_TaskLists(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	content := `---
title: Task List Test
---

- [ ] Uncompleted task
- [x] Completed task
- [ ] Another task
  - [ ] Nested uncompleted
  - [x] Nested completed
`
	f.WriteMarkdownFile("task-list.md", content)

	p := parser.New()
	doc, err := p.ParseFile(f.VaultPath, "task-list.md")
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	trans := transformer.New(nil, nil)
	notionPage, err := trans.Transform(doc)
	if err != nil {
		t.Fatalf("failed to transform: %v", err)
	}

	result, err := f.NotionClient.CreatePageUnderPage(ctx, f.ParentPageID, notionPage)
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}
	f.TrackPage(result.PageID)

	f.AssertPageExists(ctx, result.PageID)
}

// TestFormatting_MixedContent tests a document with mixed content types.
func TestFormatting_MixedContent(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	content := `---
title: Mixed Content Test
tags: [test, comprehensive]
---

# Main Heading

This is a **bold** and *italic* paragraph with ` + "`inline code`" + `.

## Second Level Heading

- Bullet item 1
- Bullet item 2
  - Nested bullet

1. Numbered item 1
2. Numbered item 2

> A blockquote with important information.

` + "```" + `go
func example() {
    fmt.Println("Code block")
}
` + "```" + `

### Third Level

| Col 1 | Col 2 |
|-------|-------|
| A     | B     |

---

Final paragraph with [a link](https://example.com).
`
	f.WriteMarkdownFile("mixed-content.md", content)

	p := parser.New()
	doc, err := p.ParseFile(f.VaultPath, "mixed-content.md")
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	trans := transformer.New(nil, nil)
	notionPage, err := trans.Transform(doc)
	if err != nil {
		t.Fatalf("failed to transform: %v", err)
	}

	result, err := f.NotionClient.CreatePageUnderPage(ctx, f.ParentPageID, notionPage)
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}
	f.TrackPage(result.PageID)

	f.AssertPageExists(ctx, result.PageID)

	// Verify blocks were created.
	blocks, err := f.GetNotionPageBlocks(ctx, result.PageID)
	if err != nil {
		t.Fatalf("failed to get blocks: %v", err)
	}

	// Should have multiple blocks.
	if len(blocks) < 5 {
		t.Errorf("expected at least 5 blocks, got %d", len(blocks))
	}
}

// TestFormatting_DeepNesting tests deeply nested content.
func TestFormatting_DeepNesting(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	content := `---
title: Deep Nesting Test
---

- Level 1
  - Level 2
    - Level 3
      - Level 4
        - Level 5
`
	f.WriteMarkdownFile("deep-nesting.md", content)

	p := parser.New()
	doc, err := p.ParseFile(f.VaultPath, "deep-nesting.md")
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	trans := transformer.New(nil, nil)
	notionPage, err := trans.Transform(doc)
	if err != nil {
		t.Fatalf("failed to transform: %v", err)
	}

	result, err := f.NotionClient.CreatePageUnderPage(ctx, f.ParentPageID, notionPage)
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}
	f.TrackPage(result.PageID)

	f.AssertPageExists(ctx, result.PageID)
}

// TestFormatting_LongDocument tests a document with many blocks.
func TestFormatting_LongDocument(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	// Create a document with many sections.
	var sb strings.Builder
	sb.WriteString("---\ntitle: Long Document Test\n---\n\n")

	for i := 1; i <= 20; i++ {
		sb.WriteString("## Section ")
		sb.WriteString(strings.Repeat("X", i%10))
		sb.WriteString("\n\n")
		sb.WriteString("Paragraph content for this section. ")
		sb.WriteString(strings.Repeat("Content ", 10))
		sb.WriteString("\n\n")
		sb.WriteString("- Item 1\n- Item 2\n- Item 3\n\n")
	}

	content := sb.String()
	f.WriteMarkdownFile("long-document.md", content)

	p := parser.New()
	doc, err := p.ParseFile(f.VaultPath, "long-document.md")
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	trans := transformer.New(nil, nil)
	notionPage, err := trans.Transform(doc)
	if err != nil {
		t.Fatalf("failed to transform: %v", err)
	}

	// This tests batch block creation.
	result, err := f.NotionClient.CreatePageUnderPage(ctx, f.ParentPageID, notionPage)
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}
	f.TrackPage(result.PageID)

	f.AssertPageExists(ctx, result.PageID)
}

// TestFormatting_SpecialCharactersInContent tests special characters in content.
func TestFormatting_SpecialCharactersInContent(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	content := `---
title: Special Characters Content
---

Ampersand: &
Less than: <
Greater than: >
Double quote: "
Single quote: '
Backslash: \
Asterisks: * ** ***
Underscores: _ __ ___
Brackets: [] {} ()
Pipe: |
Tilde: ~
Backtick: ` + "\\`" + `
`
	f.WriteMarkdownFile("special-chars-content.md", content)

	p := parser.New()
	doc, err := p.ParseFile(f.VaultPath, "special-chars-content.md")
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	trans := transformer.New(nil, nil)
	notionPage, err := trans.Transform(doc)
	if err != nil {
		t.Fatalf("failed to transform: %v", err)
	}

	result, err := f.NotionClient.CreatePageUnderPage(ctx, f.ParentPageID, notionPage)
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}
	f.TrackPage(result.PageID)

	f.AssertPageExists(ctx, result.PageID)
}

// TestFormatting_EmptyDocument tests handling of minimal/empty documents.
func TestFormatting_EmptyDocument(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	// Minimal document with just frontmatter.
	content := `---
title: Minimal Document
---
`
	f.WriteMarkdownFile("minimal.md", content)

	p := parser.New()
	doc, err := p.ParseFile(f.VaultPath, "minimal.md")
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	trans := transformer.New(nil, nil)
	notionPage, err := trans.Transform(doc)
	if err != nil {
		t.Fatalf("failed to transform: %v", err)
	}

	result, err := f.NotionClient.CreatePageUnderPage(ctx, f.ParentPageID, notionPage)
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}
	f.TrackPage(result.PageID)

	f.AssertPageExists(ctx, result.PageID)
}

// TestFormatting_MultipleCallouts tests multiple callout types.
func TestFormatting_MultipleCallouts(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	content := `---
title: Multiple Callouts
---

> [!note]
> This is a note callout.

> [!warning]
> This is a warning callout.

> [!tip]
> This is a tip callout.

> [!info]
> This is an info callout.

> [!danger]
> This is a danger callout.
`
	f.WriteMarkdownFile("multiple-callouts.md", content)

	p := parser.New()
	doc, err := p.ParseFile(f.VaultPath, "multiple-callouts.md")
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	trans := transformer.New(nil, nil)
	notionPage, err := trans.Transform(doc)
	if err != nil {
		t.Fatalf("failed to transform: %v", err)
	}

	result, err := f.NotionClient.CreatePageUnderPage(ctx, f.ParentPageID, notionPage)
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}
	f.TrackPage(result.PageID)

	f.AssertPageExists(ctx, result.PageID)
}
