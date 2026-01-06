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

// TestPush_SimpleNote tests pushing a simple markdown note to Notion.
func TestPush_SimpleNote(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	// Create a simple note in the vault.
	content := Markdown.SimpleNote("Test Note", "This is a test note with simple content.")
	f.WriteMarkdownFile("test-note.md", content)

	// Parse the markdown.
	p := parser.New()
	doc, err := p.ParseFile(f.VaultPath, "test-note.md")
	if err != nil {
		t.Fatalf("failed to parse file: %v", err)
	}

	// Transform to Notion format.
	trans := transformer.New(nil, nil)
	notionPage, err := trans.Transform(doc)
	if err != nil {
		t.Fatalf("failed to transform: %v", err)
	}

	// Create page in Notion under parent page.
	result, err := f.NotionClient.CreatePageUnderPage(ctx, f.ParentPageID, notionPage)
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}
	f.TrackPage(result.PageID)

	// Verify page was created.
	f.AssertPageExists(ctx, result.PageID)

	// Record sync state.
	contentHash := state.HashContent([]byte(content)).FullHash
	err = f.DB.SetState(&state.SyncState{
		ObsidianPath: "test-note.md",
		NotionPageID: result.PageID,
		ContentHash:  contentHash,
		Status:       "synced",
		LastSync:     time.Now(),
	})
	if err != nil {
		t.Fatalf("failed to set state: %v", err)
	}

	// Verify sync state.
	syncState, err := f.DB.GetState("test-note.md")
	if err != nil {
		t.Fatalf("failed to get state: %v", err)
	}
	if syncState.NotionPageID != result.PageID {
		t.Errorf("sync state page ID = %s, want %s", syncState.NotionPageID, result.PageID)
	}
	if syncState.Status != "synced" {
		t.Errorf("sync state status = %s, want synced", syncState.Status)
	}
}

// TestPush_NoteWithFrontmatter tests pushing a note with frontmatter properties.
func TestPush_NoteWithFrontmatter(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	// Create note with frontmatter.
	content := Markdown.NoteWithFrontmatter(
		"Frontmatter Test",
		[]string{"test", "e2e"},
		"draft",
		"This note has frontmatter properties.",
	)
	f.WriteMarkdownFile("frontmatter-test.md", content)

	// Parse the markdown.
	p := parser.New()
	doc, err := p.ParseFile(f.VaultPath, "frontmatter-test.md")
	if err != nil {
		t.Fatalf("failed to parse file: %v", err)
	}

	// Verify frontmatter was parsed.
	if doc.Frontmatter == nil {
		t.Fatal("frontmatter should not be nil")
	}
	if title, ok := doc.Frontmatter["title"].(string); !ok || title != "Frontmatter Test" {
		t.Errorf("frontmatter title = %v, want 'Frontmatter Test'", doc.Frontmatter["title"])
	}

	// Transform to Notion format.
	trans := transformer.New(nil, nil)
	notionPage, err := trans.Transform(doc)
	if err != nil {
		t.Fatalf("failed to transform: %v", err)
	}

	// Create page in Notion.
	result, err := f.NotionClient.CreatePageUnderPage(ctx, f.ParentPageID, notionPage)
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}
	f.TrackPage(result.PageID)

	// Verify page exists.
	f.AssertPageExists(ctx, result.PageID)
}

// TestPush_NoteWithCallout tests pushing a note with Obsidian callouts.
func TestPush_NoteWithCallout(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	// Create note with callout.
	content := Markdown.NoteWithCallout(
		"Callout Test",
		"warning",
		"This is a warning callout",
		"Regular body content after the callout.",
	)
	f.WriteMarkdownFile("callout-test.md", content)

	// Parse and transform.
	p := parser.New()
	doc, err := p.ParseFile(f.VaultPath, "callout-test.md")
	if err != nil {
		t.Fatalf("failed to parse file: %v", err)
	}

	trans := transformer.New(nil, nil)
	notionPage, err := trans.Transform(doc)
	if err != nil {
		t.Fatalf("failed to transform: %v", err)
	}

	// Create page in Notion.
	result, err := f.NotionClient.CreatePageUnderPage(ctx, f.ParentPageID, notionPage)
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}
	f.TrackPage(result.PageID)

	// Verify page exists.
	f.AssertPageExists(ctx, result.PageID)

	// Get page blocks and verify callout was created.
	blocks, err := f.GetNotionPageBlocks(ctx, result.PageID)
	if err != nil {
		t.Fatalf("failed to get blocks: %v", err)
	}

	// We expect at least one block (may include callout and/or paragraph).
	if len(blocks) == 0 {
		t.Error("expected at least one block")
	}
}

// TestPush_NoteWithNestedLists tests pushing a note with nested bullet lists.
func TestPush_NoteWithNestedLists(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	// Create note with nested lists.
	content := Markdown.NoteWithNestedLists("Nested List Test")
	f.WriteMarkdownFile("nested-list-test.md", content)

	// Parse and transform.
	p := parser.New()
	doc, err := p.ParseFile(f.VaultPath, "nested-list-test.md")
	if err != nil {
		t.Fatalf("failed to parse file: %v", err)
	}

	trans := transformer.New(nil, nil)
	notionPage, err := trans.Transform(doc)
	if err != nil {
		t.Fatalf("failed to transform: %v", err)
	}

	// Create page in Notion.
	result, err := f.NotionClient.CreatePageUnderPage(ctx, f.ParentPageID, notionPage)
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}
	f.TrackPage(result.PageID)

	// Verify page exists.
	f.AssertPageExists(ctx, result.PageID)
}

// TestPush_NoteWithCodeBlock tests pushing a note with code blocks.
func TestPush_NoteWithCodeBlock(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	// Create note with code block.
	content := Markdown.NoteWithCodeBlock("Code Test", "go", `func main() {
    fmt.Println("Hello, World!")
}`)
	f.WriteMarkdownFile("code-test.md", content)

	// Parse and transform.
	p := parser.New()
	doc, err := p.ParseFile(f.VaultPath, "code-test.md")
	if err != nil {
		t.Fatalf("failed to parse file: %v", err)
	}

	trans := transformer.New(nil, nil)
	notionPage, err := trans.Transform(doc)
	if err != nil {
		t.Fatalf("failed to transform: %v", err)
	}

	// Create page in Notion.
	result, err := f.NotionClient.CreatePageUnderPage(ctx, f.ParentPageID, notionPage)
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}
	f.TrackPage(result.PageID)

	// Verify page exists.
	f.AssertPageExists(ctx, result.PageID)
}

// TestPush_UpdateExistingPage tests updating an existing page in Notion.
func TestPush_UpdateExistingPage(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	// First, create a note and push it.
	content1 := Markdown.SimpleNote("Update Test", "Original content.")
	f.WriteMarkdownFile("update-test.md", content1)

	p := parser.New()
	doc1, err := p.ParseFile(f.VaultPath, "update-test.md")
	if err != nil {
		t.Fatalf("failed to parse file: %v", err)
	}

	trans := transformer.New(nil, nil)
	notionPage1, err := trans.Transform(doc1)
	if err != nil {
		t.Fatalf("failed to transform: %v", err)
	}

	result, err := f.NotionClient.CreatePageUnderPage(ctx, f.ParentPageID, notionPage1)
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}
	f.TrackPage(result.PageID)
	pageID := result.PageID

	// Record initial sync state.
	hash1 := state.HashContent([]byte(content1)).FullHash
	err = f.DB.SetState(&state.SyncState{
		ObsidianPath: "update-test.md",
		NotionPageID: pageID,
		ContentHash:  hash1,
		Status:       "synced",
		LastSync:     time.Now(),
	})
	if err != nil {
		t.Fatalf("failed to set state: %v", err)
	}

	// Now update the local file.
	content2 := Markdown.SimpleNote("Update Test", "Updated content with new information.")
	f.WriteMarkdownFile("update-test.md", content2)

	// Parse the updated file.
	doc2, err := p.ParseFile(f.VaultPath, "update-test.md")
	if err != nil {
		t.Fatalf("failed to parse updated file: %v", err)
	}

	notionPage2, err := trans.Transform(doc2)
	if err != nil {
		t.Fatalf("failed to transform updated: %v", err)
	}

	// Update the existing Notion page.
	err = f.NotionClient.UpdatePage(ctx, pageID, notionPage2)
	if err != nil {
		t.Fatalf("failed to update page: %v", err)
	}

	// Update sync state.
	hash2 := state.HashContent([]byte(content2)).FullHash
	err = f.DB.SetState(&state.SyncState{
		ObsidianPath: "update-test.md",
		NotionPageID: pageID,
		ContentHash:  hash2,
		Status:       "synced",
		LastSync:     time.Now(),
	})
	if err != nil {
		t.Fatalf("failed to update state: %v", err)
	}

	// Verify the page still exists.
	f.AssertPageExists(ctx, pageID)

	// Verify sync state was updated.
	syncState, err := f.DB.GetState("update-test.md")
	if err != nil {
		t.Fatalf("failed to get state: %v", err)
	}
	if syncState.ContentHash != hash2 {
		t.Errorf("content hash = %s, want %s", syncState.ContentHash, hash2)
	}
}

// TestPush_MultipleNotes tests pushing multiple notes in sequence.
func TestPush_MultipleNotes(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	notes := []struct {
		filename string
		title    string
		content  string
	}{
		{"note1.md", "First Note", "Content of the first note."},
		{"note2.md", "Second Note", "Content of the second note."},
		{"note3.md", "Third Note", "Content of the third note."},
	}

	p := parser.New()
	trans := transformer.New(nil, nil)

	pageIDs := make([]string, 0, len(notes))

	for _, note := range notes {
		content := Markdown.SimpleNote(note.title, note.content)
		f.WriteMarkdownFile(note.filename, content)

		doc, err := p.ParseFile(f.VaultPath, note.filename)
		if err != nil {
			t.Fatalf("failed to parse %s: %v", note.filename, err)
		}

		notionPage, err := trans.Transform(doc)
		if err != nil {
			t.Fatalf("failed to transform %s: %v", note.filename, err)
		}

		result, err := f.NotionClient.CreatePageUnderPage(ctx, f.ParentPageID, notionPage)
		if err != nil {
			t.Fatalf("failed to create page for %s: %v", note.filename, err)
		}
		f.TrackPage(result.PageID)
		pageIDs = append(pageIDs, result.PageID)

		// Record sync state.
		hash := state.HashContent([]byte(content)).FullHash
		err = f.DB.SetState(&state.SyncState{
			ObsidianPath: note.filename,
			NotionPageID: result.PageID,
			ContentHash:  hash,
			Status:       "synced",
			LastSync:     time.Now(),
		})
		if err != nil {
			t.Fatalf("failed to set state for %s: %v", note.filename, err)
		}
	}

	// Verify all pages exist.
	for _, pageID := range pageIDs {
		f.AssertPageExists(ctx, pageID)
	}

	// Verify sync state for all notes.
	states, err := f.DB.ListStates("")
	if err != nil {
		t.Fatalf("failed to list states: %v", err)
	}
	if len(states) != len(notes) {
		t.Errorf("expected %d sync states, got %d", len(notes), len(states))
	}
}

// TestPush_NoteInSubdirectory tests pushing a note from a subdirectory.
func TestPush_NoteInSubdirectory(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	// Create note in subdirectory.
	content := Markdown.SimpleNote("Subdirectory Note", "This note is in a subdirectory.")
	f.WriteMarkdownFile("projects/work/note.md", content)

	// Parse and transform.
	p := parser.New()
	doc, err := p.ParseFile(f.VaultPath, "projects/work/note.md")
	if err != nil {
		t.Fatalf("failed to parse file: %v", err)
	}

	trans := transformer.New(nil, nil)
	notionPage, err := trans.Transform(doc)
	if err != nil {
		t.Fatalf("failed to transform: %v", err)
	}

	// Create page in Notion.
	result, err := f.NotionClient.CreatePageUnderPage(ctx, f.ParentPageID, notionPage)
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}
	f.TrackPage(result.PageID)

	// Record sync state with relative path.
	hash := state.HashContent([]byte(content)).FullHash
	err = f.DB.SetState(&state.SyncState{
		ObsidianPath: "projects/work/note.md",
		NotionPageID: result.PageID,
		ContentHash:  hash,
		Status:       "synced",
		LastSync:     time.Now(),
	})
	if err != nil {
		t.Fatalf("failed to set state: %v", err)
	}

	// Verify page exists.
	f.AssertPageExists(ctx, result.PageID)

	// Verify sync state path.
	syncState, err := f.DB.GetState("projects/work/note.md")
	if err != nil {
		t.Fatalf("failed to get state: %v", err)
	}
	if syncState.NotionPageID != result.PageID {
		t.Errorf("sync state page ID mismatch")
	}
}

// TestPush_NoteWithUnicode tests pushing a note with Unicode characters.
func TestPush_NoteWithUnicode(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	// Create note with Unicode.
	content := Markdown.NoteWithUnicode("Unicode Test")
	f.WriteMarkdownFile("unicode-test.md", content)

	// Parse and transform.
	p := parser.New()
	doc, err := p.ParseFile(f.VaultPath, "unicode-test.md")
	if err != nil {
		t.Fatalf("failed to parse file: %v", err)
	}

	trans := transformer.New(nil, nil)
	notionPage, err := trans.Transform(doc)
	if err != nil {
		t.Fatalf("failed to transform: %v", err)
	}

	// Create page in Notion.
	result, err := f.NotionClient.CreatePageUnderPage(ctx, f.ParentPageID, notionPage)
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}
	f.TrackPage(result.PageID)

	// Verify page exists.
	f.AssertPageExists(ctx, result.PageID)
}

// TestPush_ChangeDetection tests that the change detector identifies new and modified files.
func TestPush_ChangeDetection(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	// Create and push an initial note.
	content1 := Markdown.SimpleNote("Initial Note", "Initial content.")
	f.WriteMarkdownFile("initial.md", content1)

	p := parser.New()
	trans := transformer.New(nil, nil)

	doc, err := p.ParseFile(f.VaultPath, "initial.md")
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

	// Record sync state.
	hash := state.HashContent([]byte(content1)).FullHash
	err = f.DB.SetState(&state.SyncState{
		ObsidianPath: "initial.md",
		NotionPageID: result.PageID,
		ContentHash:  hash,
		Status:       "synced",
		LastSync:     time.Now(),
	})
	if err != nil {
		t.Fatalf("failed to set state: %v", err)
	}

	// Create a new file (should be detected as created).
	content2 := Markdown.SimpleNote("New Note", "New content.")
	f.WriteMarkdownFile("new.md", content2)

	// Modify the existing file (should be detected as modified).
	content3 := Markdown.SimpleNote("Initial Note", "Modified content.")
	f.WriteMarkdownFile("initial.md", content3)

	// Run change detection.
	detector := state.NewChangeDetector(f.DB, f.VaultPath)
	changes, err := detector.DetectChanges(ctx)
	if err != nil {
		t.Fatalf("failed to detect changes: %v", err)
	}

	// We should have 2 changes: 1 created, 1 modified.
	createdCount := 0
	modifiedCount := 0
	for _, change := range changes {
		switch change.Type {
		case state.ChangeCreated:
			createdCount++
			if change.Path != "new.md" {
				t.Errorf("expected created file to be new.md, got %s", change.Path)
			}
		case state.ChangeModified:
			modifiedCount++
			if change.Path != "initial.md" {
				t.Errorf("expected modified file to be initial.md, got %s", change.Path)
			}
		}
	}

	if createdCount != 1 {
		t.Errorf("expected 1 created file, got %d", createdCount)
	}
	if modifiedCount != 1 {
		t.Errorf("expected 1 modified file, got %d", modifiedCount)
	}
}

// TestPush_DeletedFileDetection tests that deleted files are detected.
func TestPush_DeletedFileDetection(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	// Create and push a note.
	content := Markdown.SimpleNote("To Delete", "This will be deleted.")
	f.WriteMarkdownFile("to-delete.md", content)

	p := parser.New()
	trans := transformer.New(nil, nil)

	doc, err := p.ParseFile(f.VaultPath, "to-delete.md")
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

	// Record sync state.
	hash := state.HashContent([]byte(content)).FullHash
	err = f.DB.SetState(&state.SyncState{
		ObsidianPath: "to-delete.md",
		NotionPageID: result.PageID,
		ContentHash:  hash,
		Status:       "synced",
		LastSync:     time.Now(),
	})
	if err != nil {
		t.Fatalf("failed to set state: %v", err)
	}

	// Delete the local file.
	f.DeleteFile("to-delete.md")

	// Run change detection.
	detector := state.NewChangeDetector(f.DB, f.VaultPath)
	changes, err := detector.DetectChanges(ctx)
	if err != nil {
		t.Fatalf("failed to detect changes: %v", err)
	}

	// We should have 1 deleted change.
	deletedCount := 0
	for _, change := range changes {
		if change.Type == state.ChangeDeleted {
			deletedCount++
			if change.Path != "to-delete.md" {
				t.Errorf("expected deleted file to be to-delete.md, got %s", change.Path)
			}
		}
	}

	if deletedCount != 1 {
		t.Errorf("expected 1 deleted file, got %d (changes: %v)", deletedCount, changes)
	}
}

// TestPush_EmptyFrontmatter tests pushing a note without frontmatter.
func TestPush_EmptyFrontmatter(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	// Create note without frontmatter.
	content := `# Plain Heading

This is a plain markdown file without frontmatter.

Some more content.
`
	f.WriteMarkdownFile("no-frontmatter.md", content)

	// Parse and transform.
	p := parser.New()
	doc, err := p.ParseFile(f.VaultPath, "no-frontmatter.md")
	if err != nil {
		t.Fatalf("failed to parse file: %v", err)
	}

	trans := transformer.New(nil, nil)
	notionPage, err := trans.Transform(doc)
	if err != nil {
		t.Fatalf("failed to transform: %v", err)
	}

	// Create page in Notion.
	result, err := f.NotionClient.CreatePageUnderPage(ctx, f.ParentPageID, notionPage)
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}
	f.TrackPage(result.PageID)

	// Verify page exists.
	f.AssertPageExists(ctx, result.PageID)
}

// TestPush_LargeNote tests pushing a note with significant content.
func TestPush_LargeNote(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	// Create a larger note with multiple sections.
	var sb strings.Builder
	sb.WriteString("---\ntitle: Large Note Test\n---\n\n")

	for i := 1; i <= 10; i++ {
		sb.WriteString("## Section ")
		sb.WriteString(strings.Repeat("X", i))
		sb.WriteString("\n\n")
		sb.WriteString("This is paragraph content for section ")
		sb.WriteString(strings.Repeat("content ", 20))
		sb.WriteString("\n\n")
		sb.WriteString("- List item 1\n- List item 2\n- List item 3\n\n")
	}

	content := sb.String()
	f.WriteMarkdownFile("large-note.md", content)

	// Parse and transform.
	p := parser.New()
	doc, err := p.ParseFile(f.VaultPath, "large-note.md")
	if err != nil {
		t.Fatalf("failed to parse file: %v", err)
	}

	trans := transformer.New(nil, nil)
	notionPage, err := trans.Transform(doc)
	if err != nil {
		t.Fatalf("failed to transform: %v", err)
	}

	// Create page in Notion.
	result, err := f.NotionClient.CreatePageUnderPage(ctx, f.ParentPageID, notionPage)
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}
	f.TrackPage(result.PageID)

	// Verify page exists.
	f.AssertPageExists(ctx, result.PageID)
}
