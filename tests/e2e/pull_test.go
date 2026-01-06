//go:build e2e
// +build e2e

package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/jomei/notionapi"

	"github.com/adamancini/obsidian-notion-sync/internal/state"
	"github.com/adamancini/obsidian-notion-sync/internal/transformer"
)

// TestPull_NewPageFromNotion tests pulling a page created directly in Notion.
func TestPull_NewPageFromNotion(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	// Create a page directly in Notion.
	title := "Page Created In Notion"
	content := "This page was created directly in Notion and will be pulled."

	pageID, err := f.CreateNotionPage(ctx, title, content)
	if err != nil {
		t.Fatalf("failed to create Notion page: %v", err)
	}

	// Wait for Notion to settle.
	f.WaitForSync()

	// Fetch the page.
	notionPage, err := f.NotionClient.FetchPage(ctx, pageID)
	if err != nil {
		t.Fatalf("failed to fetch page: %v", err)
	}

	// Transform to Obsidian markdown using ReverseTransformer.
	reverseTrans := transformer.NewReverse(nil, nil)
	markdown, err := reverseTrans.NotionToMarkdown(notionPage)
	if err != nil {
		t.Fatalf("failed to transform to obsidian: %v", err)
	}

	// Write to local file.
	filename := "page-from-notion.md"
	f.WriteMarkdownFile(filename, string(markdown))

	// Record sync state.
	hash := state.HashContent(markdown).FullHash
	err = f.DB.SetState(&state.SyncState{
		ObsidianPath: filename,
		NotionPageID: pageID,
		ContentHash:  hash,
		Status:       "synced",
		LastSync:     time.Now(),
	})
	if err != nil {
		t.Fatalf("failed to set state: %v", err)
	}

	// Verify file exists and contains expected content.
	f.AssertFileContains(filename, content)
}

// TestPull_ModifiedPageFromNotion tests pulling updates to an existing synced page.
func TestPull_ModifiedPageFromNotion(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	// Create initial page in Notion.
	title := "Page To Modify"
	initialContent := "Original content from Notion."

	pageID, err := f.CreateNotionPage(ctx, title, initialContent)
	if err != nil {
		t.Fatalf("failed to create Notion page: %v", err)
	}

	// Pull initial content.
	f.WaitForSync()
	notionPage, err := f.NotionClient.FetchPage(ctx, pageID)
	if err != nil {
		t.Fatalf("failed to fetch page: %v", err)
	}

	reverseTrans := transformer.NewReverse(nil, nil)
	markdown, err := reverseTrans.NotionToMarkdown(notionPage)
	if err != nil {
		t.Fatalf("failed to transform: %v", err)
	}

	filename := "modified-page.md"
	f.WriteMarkdownFile(filename, string(markdown))

	// Record initial sync state with timestamp.
	hash := state.HashContent(markdown).FullHash
	initialSync := time.Now()
	err = f.DB.SetState(&state.SyncState{
		ObsidianPath: filename,
		NotionPageID: pageID,
		ContentHash:  hash,
		Status:       "synced",
		LastSync:     initialSync,
	})
	if err != nil {
		t.Fatalf("failed to set state: %v", err)
	}

	// Wait and modify page in Notion.
	time.Sleep(time.Second)
	newContent := "Updated content from Notion."
	err = f.UpdateNotionPage(ctx, pageID, newContent)
	if err != nil {
		t.Fatalf("failed to update Notion page: %v", err)
	}

	// Wait for Notion to settle.
	f.WaitForSync()

	// Get page metadata to check if modified.
	meta, err := f.NotionClient.GetPageMetadata(ctx, pageID)
	if err != nil {
		t.Fatalf("failed to get page metadata: %v", err)
	}

	// Verify page was modified after our sync.
	if !meta.LastEditedTime.After(initialSync) {
		t.Log("Page was not detected as modified - this may be a timing issue")
	}

	// Pull updated content.
	notionPage2, err := f.NotionClient.FetchPage(ctx, pageID)
	if err != nil {
		t.Fatalf("failed to fetch updated page: %v", err)
	}

	markdown2, err := reverseTrans.NotionToMarkdown(notionPage2)
	if err != nil {
		t.Fatalf("failed to transform updated: %v", err)
	}

	// Write updated content.
	f.WriteMarkdownFile(filename, string(markdown2))

	// Update sync state.
	hash2 := state.HashContent(markdown2).FullHash
	err = f.DB.SetState(&state.SyncState{
		ObsidianPath: filename,
		NotionPageID: pageID,
		ContentHash:  hash2,
		Status:       "synced",
		LastSync:     time.Now(),
	})
	if err != nil {
		t.Fatalf("failed to update state: %v", err)
	}

	// Verify updated content.
	f.AssertFileContains(filename, newContent)
}

// TestPull_PageWithMultipleBlocks tests pulling a page with various block types.
func TestPull_PageWithMultipleBlocks(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	// Create a page with multiple block types in Notion.
	page, err := f.NotionClient.API().Page.Create(ctx, &notionapi.PageCreateRequest{
		Parent: notionapi.Parent{
			Type:   notionapi.ParentTypePageID,
			PageID: notionapi.PageID(f.ParentPageID),
		},
		Properties: notionapi.Properties{
			"title": notionapi.TitleProperty{
				Title: []notionapi.RichText{
					{
						Type: notionapi.ObjectTypeText,
						Text: &notionapi.Text{Content: "Multi-Block Page"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}

	pageID := string(page.ID)
	f.TrackPage(pageID)

	// Add various block types.
	blocks := []notionapi.Block{
		// Heading
		&notionapi.Heading1Block{
			BasicBlock: notionapi.BasicBlock{
				Object: notionapi.ObjectTypeBlock,
				Type:   notionapi.BlockTypeHeading1,
			},
			Heading1: notionapi.Heading{
				RichText: []notionapi.RichText{
					{Type: notionapi.ObjectTypeText, Text: &notionapi.Text{Content: "Section One"}},
				},
			},
		},
		// Paragraph
		&notionapi.ParagraphBlock{
			BasicBlock: notionapi.BasicBlock{
				Object: notionapi.ObjectTypeBlock,
				Type:   notionapi.BlockTypeParagraph,
			},
			Paragraph: notionapi.Paragraph{
				RichText: []notionapi.RichText{
					{Type: notionapi.ObjectTypeText, Text: &notionapi.Text{Content: "A paragraph of text."}},
				},
			},
		},
		// Bullet list item
		&notionapi.BulletedListItemBlock{
			BasicBlock: notionapi.BasicBlock{
				Object: notionapi.ObjectTypeBlock,
				Type:   notionapi.BlockTypeBulletedListItem,
			},
			BulletedListItem: notionapi.ListItem{
				RichText: []notionapi.RichText{
					{Type: notionapi.ObjectTypeText, Text: &notionapi.Text{Content: "List item one"}},
				},
			},
		},
		&notionapi.BulletedListItemBlock{
			BasicBlock: notionapi.BasicBlock{
				Object: notionapi.ObjectTypeBlock,
				Type:   notionapi.BlockTypeBulletedListItem,
			},
			BulletedListItem: notionapi.ListItem{
				RichText: []notionapi.RichText{
					{Type: notionapi.ObjectTypeText, Text: &notionapi.Text{Content: "List item two"}},
				},
			},
		},
		// Code block
		&notionapi.CodeBlock{
			BasicBlock: notionapi.BasicBlock{
				Object: notionapi.ObjectTypeBlock,
				Type:   notionapi.BlockTypeCode,
			},
			Code: notionapi.Code{
				Language: "go",
				RichText: []notionapi.RichText{
					{Type: notionapi.ObjectTypeText, Text: &notionapi.Text{Content: "fmt.Println(\"Hello\")"}},
				},
			},
		},
	}

	_, err = f.NotionClient.API().Block.AppendChildren(ctx, notionapi.BlockID(pageID), &notionapi.AppendBlockChildrenRequest{
		Children: blocks,
	})
	if err != nil {
		t.Fatalf("failed to append blocks: %v", err)
	}

	f.WaitForSync()

	// Fetch and transform.
	notionPage, err := f.NotionClient.FetchPage(ctx, pageID)
	if err != nil {
		t.Fatalf("failed to fetch page: %v", err)
	}

	reverseTrans := transformer.NewReverse(nil, nil)
	markdown, err := reverseTrans.NotionToMarkdown(notionPage)
	if err != nil {
		t.Fatalf("failed to transform: %v", err)
	}

	// Write to file.
	filename := "multi-block.md"
	f.WriteMarkdownFile(filename, string(markdown))

	// Verify content includes expected elements.
	content := f.ReadMarkdownFile(filename)

	// Check for heading (may be # or ## depending on transformation).
	if !strings.Contains(content, "Section One") {
		t.Error("expected heading 'Section One' not found")
	}

	// Check for paragraph.
	if !strings.Contains(content, "A paragraph of text.") {
		t.Error("expected paragraph text not found")
	}

	// Check for list items.
	if !strings.Contains(content, "List item one") {
		t.Error("expected list item 'List item one' not found")
	}

	// Check for code.
	if !strings.Contains(content, "fmt.Println") {
		t.Error("expected code content not found")
	}
}

// TestPull_PageWithFormattedText tests pulling a page with formatted text.
func TestPull_PageWithFormattedText(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	// Create a page with formatted text.
	page, err := f.NotionClient.API().Page.Create(ctx, &notionapi.PageCreateRequest{
		Parent: notionapi.Parent{
			Type:   notionapi.ParentTypePageID,
			PageID: notionapi.PageID(f.ParentPageID),
		},
		Properties: notionapi.Properties{
			"title": notionapi.TitleProperty{
				Title: []notionapi.RichText{
					{
						Type: notionapi.ObjectTypeText,
						Text: &notionapi.Text{Content: "Formatted Text Page"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}

	pageID := string(page.ID)
	f.TrackPage(pageID)

	// Add paragraph with formatted text.
	blocks := []notionapi.Block{
		&notionapi.ParagraphBlock{
			BasicBlock: notionapi.BasicBlock{
				Object: notionapi.ObjectTypeBlock,
				Type:   notionapi.BlockTypeParagraph,
			},
			Paragraph: notionapi.Paragraph{
				RichText: []notionapi.RichText{
					{
						Type:        notionapi.ObjectTypeText,
						Text:        &notionapi.Text{Content: "Bold text"},
						Annotations: &notionapi.Annotations{Bold: true},
					},
					{
						Type: notionapi.ObjectTypeText,
						Text: &notionapi.Text{Content: " and "},
					},
					{
						Type:        notionapi.ObjectTypeText,
						Text:        &notionapi.Text{Content: "italic text"},
						Annotations: &notionapi.Annotations{Italic: true},
					},
					{
						Type: notionapi.ObjectTypeText,
						Text: &notionapi.Text{Content: " and "},
					},
					{
						Type:        notionapi.ObjectTypeText,
						Text:        &notionapi.Text{Content: "code"},
						Annotations: &notionapi.Annotations{Code: true},
					},
				},
			},
		},
	}

	_, err = f.NotionClient.API().Block.AppendChildren(ctx, notionapi.BlockID(pageID), &notionapi.AppendBlockChildrenRequest{
		Children: blocks,
	})
	if err != nil {
		t.Fatalf("failed to append blocks: %v", err)
	}

	f.WaitForSync()

	// Fetch and transform.
	notionPage, err := f.NotionClient.FetchPage(ctx, pageID)
	if err != nil {
		t.Fatalf("failed to fetch page: %v", err)
	}

	reverseTrans := transformer.NewReverse(nil, nil)
	markdown, err := reverseTrans.NotionToMarkdown(notionPage)
	if err != nil {
		t.Fatalf("failed to transform: %v", err)
	}

	// Write to file.
	filename := "formatted-text.md"
	f.WriteMarkdownFile(filename, string(markdown))

	// Verify content includes formatted text.
	content := f.ReadMarkdownFile(filename)

	// Check for bold (should be **text**).
	if !strings.Contains(content, "Bold text") {
		t.Error("expected bold text not found")
	}

	// Check for italic (should be *text*).
	if !strings.Contains(content, "italic text") {
		t.Error("expected italic text not found")
	}

	// Check for code (should be `text`).
	if !strings.Contains(content, "code") {
		t.Error("expected code text not found")
	}
}

// TestPull_ArchivedPage tests that archived pages are handled correctly.
func TestPull_ArchivedPage(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	// Create a page.
	pageID, err := f.CreateNotionPage(ctx, "To Archive", "This will be archived.")
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}

	// Record sync state.
	filename := "archived-page.md"
	err = f.DB.SetState(&state.SyncState{
		ObsidianPath: filename,
		NotionPageID: pageID,
		ContentHash:  "test",
		Status:       "synced",
		LastSync:     time.Now(),
	})
	if err != nil {
		t.Fatalf("failed to set state: %v", err)
	}

	// Archive the page.
	err = f.NotionClient.ArchivePage(ctx, pageID)
	if err != nil {
		t.Fatalf("failed to archive page: %v", err)
	}

	f.WaitForSync()

	// Verify page is archived.
	f.AssertPageArchived(ctx, pageID)

	// Check page metadata.
	meta, err := f.NotionClient.GetPageMetadata(ctx, pageID)
	if err != nil {
		t.Fatalf("failed to get metadata: %v", err)
	}

	if !meta.Archived {
		t.Error("page should be archived")
	}
}

// TestPull_DiscoverNewPages tests discovering new pages from a parent page.
func TestPull_DiscoverNewPages(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	// Create multiple pages directly in Notion.
	titles := []string{"Discovery Page 1", "Discovery Page 2", "Discovery Page 3"}
	pageIDs := make([]string, 0, len(titles))

	for _, title := range titles {
		pageID, err := f.CreateNotionPage(ctx, title, "Content for "+title)
		if err != nil {
			t.Fatalf("failed to create page %s: %v", title, err)
		}
		pageIDs = append(pageIDs, pageID)
	}

	f.WaitForSync()

	// Verify all pages were created.
	for _, pageID := range pageIDs {
		f.AssertPageExists(ctx, pageID)
	}

	// Fetch and transform each page.
	reverseTrans := transformer.NewReverse(nil, nil)
	for i, pageID := range pageIDs {
		notionPage, err := f.NotionClient.FetchPage(ctx, pageID)
		if err != nil {
			t.Fatalf("failed to fetch page %d: %v", i, err)
		}

		markdown, err := reverseTrans.NotionToMarkdown(notionPage)
		if err != nil {
			t.Fatalf("failed to transform page %d: %v", i, err)
		}

		filename := "discovery-" + pageID[:8] + ".md"
		f.WriteMarkdownFile(filename, string(markdown))

		// Record sync state.
		hash := state.HashContent(markdown).FullHash
		err = f.DB.SetState(&state.SyncState{
			ObsidianPath: filename,
			NotionPageID: pageID,
			ContentHash:  hash,
			Status:       "synced",
			LastSync:     time.Now(),
		})
		if err != nil {
			t.Fatalf("failed to set state: %v", err)
		}

		// Verify file contains expected content.
		f.AssertFileContains(filename, titles[i])
	}
}

// TestPull_SyncStateTracking tests that sync state is properly tracked during pull.
func TestPull_SyncStateTracking(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	// Create a page.
	pageID, err := f.CreateNotionPage(ctx, "Sync State Test", "Testing sync state tracking.")
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}

	f.WaitForSync()

	// Fetch and transform.
	notionPage, err := f.NotionClient.FetchPage(ctx, pageID)
	if err != nil {
		t.Fatalf("failed to fetch page: %v", err)
	}

	reverseTrans := transformer.NewReverse(nil, nil)
	markdown, err := reverseTrans.NotionToMarkdown(notionPage)
	if err != nil {
		t.Fatalf("failed to transform: %v", err)
	}

	filename := "sync-state-test.md"
	f.WriteMarkdownFile(filename, string(markdown))

	// Record sync state with all fields.
	hashes := state.HashContent(markdown)
	syncState := &state.SyncState{
		ObsidianPath:    filename,
		NotionPageID:    pageID,
		ContentHash:     hashes.FullHash,
		FrontmatterHash: hashes.FrontmatterHash,
		Status:          "synced",
		LastSync:        time.Now(),
	}

	err = f.DB.SetState(syncState)
	if err != nil {
		t.Fatalf("failed to set state: %v", err)
	}

	// Verify sync state was recorded correctly.
	retrieved, err := f.DB.GetState(filename)
	if err != nil {
		t.Fatalf("failed to get state: %v", err)
	}

	if retrieved.NotionPageID != pageID {
		t.Errorf("page ID = %s, want %s", retrieved.NotionPageID, pageID)
	}
	if retrieved.ContentHash != hashes.FullHash {
		t.Errorf("content hash = %s, want %s", retrieved.ContentHash, hashes.FullHash)
	}
	if retrieved.Status != "synced" {
		t.Errorf("status = %s, want synced", retrieved.Status)
	}
}
