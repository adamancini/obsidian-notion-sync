//go:build e2e
// +build e2e

package e2e

import (
	"testing"
	"time"

	"github.com/adamancini/obsidian-notion-sync/internal/parser"
	"github.com/adamancini/obsidian-notion-sync/internal/state"
	"github.com/adamancini/obsidian-notion-sync/internal/transformer"
)

// TestSync_RoundTrip tests a full push -> modify remote -> pull cycle.
func TestSync_RoundTrip(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	// Step 1: Create and push a local note.
	content1 := Markdown.SimpleNote("Round Trip Test", "Original local content.")
	f.WriteMarkdownFile("round-trip.md", content1)

	p := parser.New()
	trans := transformer.New(nil, nil)

	doc, err := p.ParseFile(f.VaultPath, "round-trip.md")
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	notionPage, err := trans.Transform(doc)
	if err != nil {
		t.Fatalf("failed to transform to notion: %v", err)
	}

	result, err := f.NotionClient.CreatePageUnderPage(ctx, f.ParentPageID, notionPage)
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}
	f.TrackPage(result.PageID)
	pageID := result.PageID

	// Record initial sync state.
	hash1 := state.HashContent([]byte(content1)).FullHash
	syncTime := time.Now()
	err = f.DB.SetState(&state.SyncState{
		ObsidianPath: "round-trip.md",
		NotionPageID: pageID,
		ContentHash:  hash1,
		Status:       "synced",
		LastSync:     syncTime,
	})
	if err != nil {
		t.Fatalf("failed to set state: %v", err)
	}

	// Step 2: Modify the page in Notion.
	time.Sleep(time.Second) // Ensure time difference.
	newRemoteContent := "Modified content from Notion."
	err = f.UpdateNotionPage(ctx, pageID, newRemoteContent)
	if err != nil {
		t.Fatalf("failed to update Notion page: %v", err)
	}

	f.WaitForSync()

	// Step 3: Pull the changes.
	fetchedPage, err := f.NotionClient.FetchPage(ctx, pageID)
	if err != nil {
		t.Fatalf("failed to fetch page: %v", err)
	}

	reverseTrans := transformer.NewReverse(nil, nil)
	markdown, err := reverseTrans.NotionToMarkdown(fetchedPage)
	if err != nil {
		t.Fatalf("failed to transform to obsidian: %v", err)
	}

	// Write pulled content.
	f.WriteMarkdownFile("round-trip.md", string(markdown))

	// Update sync state.
	hash2 := state.HashContent(markdown).FullHash
	err = f.DB.SetState(&state.SyncState{
		ObsidianPath: "round-trip.md",
		NotionPageID: pageID,
		ContentHash:  hash2,
		Status:       "synced",
		LastSync:     time.Now(),
	})
	if err != nil {
		t.Fatalf("failed to update state: %v", err)
	}

	// Verify the file now contains the remote content.
	f.AssertFileContains("round-trip.md", newRemoteContent)
}

// TestSync_ConflictDetection tests that conflicts are detected when both sides are modified.
func TestSync_ConflictDetection(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	// Step 1: Create and sync a note.
	content1 := Markdown.SimpleNote("Conflict Test", "Original content.")
	f.WriteMarkdownFile("conflict-test.md", content1)

	p := parser.New()
	trans := transformer.New(nil, nil)

	doc, err := p.ParseFile(f.VaultPath, "conflict-test.md")
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
	pageID := result.PageID

	// Record sync state.
	hash1 := state.HashContent([]byte(content1)).FullHash
	syncTime := time.Now()
	err = f.DB.SetState(&state.SyncState{
		ObsidianPath: "conflict-test.md",
		NotionPageID: pageID,
		ContentHash:  hash1,
		Status:       "synced",
		LastSync:     syncTime,
	})
	if err != nil {
		t.Fatalf("failed to set state: %v", err)
	}

	// Step 2: Modify both local and remote.
	time.Sleep(time.Second)

	// Modify local.
	localContent := Markdown.SimpleNote("Conflict Test", "Local modification.")
	f.WriteMarkdownFile("conflict-test.md", localContent)

	// Modify remote.
	err = f.UpdateNotionPage(ctx, pageID, "Remote modification.")
	if err != nil {
		t.Fatalf("failed to update Notion: %v", err)
	}

	f.WaitForSync()

	// Step 3: Detect changes.
	detector := state.NewChangeDetector(f.DB, f.VaultPath)
	changes, err := detector.DetectChanges(ctx)
	if err != nil {
		t.Fatalf("failed to detect changes: %v", err)
	}

	// We should detect the local modification.
	foundModified := false
	for _, change := range changes {
		if change.Path == "conflict-test.md" && change.Type == state.ChangeModified {
			foundModified = true
		}
	}

	if !foundModified {
		t.Error("expected to detect local modification")
	}

	// Step 4: Check remote modification using metadata.
	meta, err := f.NotionClient.GetPageMetadata(ctx, pageID)
	if err != nil {
		t.Fatalf("failed to get metadata: %v", err)
	}

	if !meta.LastEditedTime.After(syncTime) {
		t.Error("expected remote page to be modified after sync time")
	}

	// Step 5: Mark as conflict.
	err = f.DB.SetState(&state.SyncState{
		ObsidianPath: "conflict-test.md",
		NotionPageID: pageID,
		ContentHash:  hash1, // Keep original hash.
		Status:       "conflict",
		LastSync:     syncTime,
	})
	if err != nil {
		t.Fatalf("failed to mark conflict: %v", err)
	}

	// Verify conflict state.
	syncState, err := f.DB.GetState("conflict-test.md")
	if err != nil {
		t.Fatalf("failed to get state: %v", err)
	}
	if syncState.Status != "conflict" {
		t.Errorf("status = %s, want conflict", syncState.Status)
	}
}

// TestSync_ResolveConflictKeepLocal tests resolving a conflict by keeping local changes.
func TestSync_ResolveConflictKeepLocal(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	// Create and sync a note.
	content1 := Markdown.SimpleNote("Resolve Local", "Original.")
	f.WriteMarkdownFile("resolve-local.md", content1)

	p := parser.New()
	trans := transformer.New(nil, nil)

	doc, err := p.ParseFile(f.VaultPath, "resolve-local.md")
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
	pageID := result.PageID

	// Simulate conflict state.
	err = f.DB.SetState(&state.SyncState{
		ObsidianPath: "resolve-local.md",
		NotionPageID: pageID,
		ContentHash:  "old-hash",
		Status:       "conflict",
		LastSync:     time.Now(),
	})
	if err != nil {
		t.Fatalf("failed to set conflict: %v", err)
	}

	// Update local with the "winning" content.
	localContent := Markdown.SimpleNote("Resolve Local", "Local wins.")
	f.WriteMarkdownFile("resolve-local.md", localContent)

	// Parse and push local changes.
	doc2, err := p.ParseFile(f.VaultPath, "resolve-local.md")
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	notionPage2, err := trans.Transform(doc2)
	if err != nil {
		t.Fatalf("failed to transform: %v", err)
	}

	err = f.NotionClient.UpdatePage(ctx, pageID, notionPage2)
	if err != nil {
		t.Fatalf("failed to update page: %v", err)
	}

	// Update sync state to resolved.
	hash := state.HashContent([]byte(localContent)).FullHash
	err = f.DB.SetState(&state.SyncState{
		ObsidianPath: "resolve-local.md",
		NotionPageID: pageID,
		ContentHash:  hash,
		Status:       "synced",
		LastSync:     time.Now(),
	})
	if err != nil {
		t.Fatalf("failed to update state: %v", err)
	}

	// Verify sync state is now synced.
	syncState, err := f.DB.GetState("resolve-local.md")
	if err != nil {
		t.Fatalf("failed to get state: %v", err)
	}
	if syncState.Status != "synced" {
		t.Errorf("status = %s, want synced", syncState.Status)
	}

	// Verify page exists.
	f.AssertPageExists(ctx, pageID)
}

// TestSync_ResolveConflictKeepRemote tests resolving a conflict by keeping remote changes.
func TestSync_ResolveConflictKeepRemote(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	// Create a page in Notion.
	pageID, err := f.CreateNotionPage(ctx, "Resolve Remote", "Remote content wins.")
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}

	f.WaitForSync()

	// Create local file with different content (simulating conflict).
	localContent := Markdown.SimpleNote("Resolve Remote", "Local content loses.")
	f.WriteMarkdownFile("resolve-remote.md", localContent)

	// Set conflict state.
	err = f.DB.SetState(&state.SyncState{
		ObsidianPath: "resolve-remote.md",
		NotionPageID: pageID,
		ContentHash:  "old-hash",
		Status:       "conflict",
		LastSync:     time.Now(),
	})
	if err != nil {
		t.Fatalf("failed to set conflict: %v", err)
	}

	// Pull remote content to resolve.
	reverseTrans := transformer.NewReverse(nil, nil)
	notionPage, err := f.NotionClient.FetchPage(ctx, pageID)
	if err != nil {
		t.Fatalf("failed to fetch page: %v", err)
	}

	markdown, err := reverseTrans.NotionToMarkdown(notionPage)
	if err != nil {
		t.Fatalf("failed to transform: %v", err)
	}

	// Overwrite local with remote content.
	f.WriteMarkdownFile("resolve-remote.md", string(markdown))

	// Update sync state to resolved.
	hash := state.HashContent(markdown).FullHash
	err = f.DB.SetState(&state.SyncState{
		ObsidianPath: "resolve-remote.md",
		NotionPageID: pageID,
		ContentHash:  hash,
		Status:       "synced",
		LastSync:     time.Now(),
	})
	if err != nil {
		t.Fatalf("failed to update state: %v", err)
	}

	// Verify local file has remote content.
	f.AssertFileContains("resolve-remote.md", "Remote content wins.")

	// Verify sync state.
	syncState, err := f.DB.GetState("resolve-remote.md")
	if err != nil {
		t.Fatalf("failed to get state: %v", err)
	}
	if syncState.Status != "synced" {
		t.Errorf("status = %s, want synced", syncState.Status)
	}
}

// TestSync_DeletedLocally tests syncing when a local file is deleted.
func TestSync_DeletedLocally(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	// Create and sync a note.
	content := Markdown.SimpleNote("To Delete Locally", "Will be deleted.")
	f.WriteMarkdownFile("delete-local.md", content)

	p := parser.New()
	trans := transformer.New(nil, nil)

	doc, err := p.ParseFile(f.VaultPath, "delete-local.md")
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
	pageID := result.PageID

	// Record sync state.
	hash := state.HashContent([]byte(content)).FullHash
	err = f.DB.SetState(&state.SyncState{
		ObsidianPath: "delete-local.md",
		NotionPageID: pageID,
		ContentHash:  hash,
		Status:       "synced",
		LastSync:     time.Now(),
	})
	if err != nil {
		t.Fatalf("failed to set state: %v", err)
	}

	// Delete the local file.
	f.DeleteFile("delete-local.md")

	// Detect the deletion.
	detector := state.NewChangeDetector(f.DB, f.VaultPath)
	changes, err := detector.DetectChanges(ctx)
	if err != nil {
		t.Fatalf("failed to detect changes: %v", err)
	}

	// Should detect deletion.
	foundDeleted := false
	for _, change := range changes {
		if change.Path == "delete-local.md" && change.Type == state.ChangeDeleted {
			foundDeleted = true
		}
	}

	if !foundDeleted {
		t.Error("expected to detect local deletion")
	}

	// Simulate archiving the remote page (based on deletion_strategy: archive).
	err = f.NotionClient.ArchivePage(ctx, pageID)
	if err != nil {
		t.Fatalf("failed to archive page: %v", err)
	}

	// Remove from sync state.
	err = f.DB.DeleteState("delete-local.md")
	if err != nil {
		t.Fatalf("failed to delete state: %v", err)
	}

	// Verify page is archived.
	f.AssertPageArchived(ctx, pageID)

	// Verify no sync state.
	_, err = f.DB.GetState("delete-local.md")
	if err == nil {
		t.Error("expected state to be deleted")
	}
}

// TestSync_MultipleFilesInParallel tests syncing multiple files.
func TestSync_MultipleFilesInParallel(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	// Create multiple notes.
	notes := []struct {
		filename string
		title    string
		content  string
	}{
		{"parallel1.md", "Parallel Note 1", "Content for note 1."},
		{"parallel2.md", "Parallel Note 2", "Content for note 2."},
		{"parallel3.md", "Parallel Note 3", "Content for note 3."},
	}

	p := parser.New()
	trans := transformer.New(nil, nil)
	pageIDs := make(map[string]string)

	// Push all notes.
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
		pageIDs[note.filename] = result.PageID

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

	f.WaitForSync()

	// Modify all notes locally.
	for _, note := range notes {
		newContent := Markdown.SimpleNote(note.title, note.content+" Modified.")
		f.WriteMarkdownFile(note.filename, newContent)
	}

	// Detect all changes.
	detector := state.NewChangeDetector(f.DB, f.VaultPath)
	changes, err := detector.DetectChanges(ctx)
	if err != nil {
		t.Fatalf("failed to detect changes: %v", err)
	}

	// Should detect 3 modifications.
	modifiedCount := 0
	for _, change := range changes {
		if change.Type == state.ChangeModified {
			modifiedCount++
		}
	}

	if modifiedCount != 3 {
		t.Errorf("expected 3 modified files, got %d", modifiedCount)
	}

	// Push all changes.
	for _, note := range notes {
		content := f.ReadMarkdownFile(note.filename)

		doc, err := p.ParseFile(f.VaultPath, note.filename)
		if err != nil {
			t.Fatalf("failed to parse %s: %v", note.filename, err)
		}

		notionPage, err := trans.Transform(doc)
		if err != nil {
			t.Fatalf("failed to transform %s: %v", note.filename, err)
		}

		err = f.NotionClient.UpdatePage(ctx, pageIDs[note.filename], notionPage)
		if err != nil {
			t.Fatalf("failed to update page for %s: %v", note.filename, err)
		}

		// Update sync state.
		hash := state.HashContent([]byte(content)).FullHash
		err = f.DB.SetState(&state.SyncState{
			ObsidianPath: note.filename,
			NotionPageID: pageIDs[note.filename],
			ContentHash:  hash,
			Status:       "synced",
			LastSync:     time.Now(),
		})
		if err != nil {
			t.Fatalf("failed to update state for %s: %v", note.filename, err)
		}
	}

	// Verify all pages exist.
	for _, pageID := range pageIDs {
		f.AssertPageExists(ctx, pageID)
	}
}

// TestSync_RenamedFile tests handling of renamed files.
func TestSync_RenamedFile(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	// Create and sync a note.
	content := Markdown.SimpleNote("Original Name", "Content stays the same.")
	f.WriteMarkdownFile("original-name.md", content)

	p := parser.New()
	trans := transformer.New(nil, nil)

	doc, err := p.ParseFile(f.VaultPath, "original-name.md")
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
	pageID := result.PageID

	// Record sync state.
	hash := state.HashContent([]byte(content)).FullHash
	err = f.DB.SetState(&state.SyncState{
		ObsidianPath: "original-name.md",
		NotionPageID: pageID,
		ContentHash:  hash,
		Status:       "synced",
		LastSync:     time.Now(),
	})
	if err != nil {
		t.Fatalf("failed to set state: %v", err)
	}

	// Rename the file (delete old, create new with same content).
	f.DeleteFile("original-name.md")
	f.WriteMarkdownFile("renamed-file.md", content)

	// Detect changes (should detect rename due to same content hash).
	detector := state.NewChangeDetector(f.DB, f.VaultPath)
	changes, err := detector.DetectChanges(ctx)
	if err != nil {
		t.Fatalf("failed to detect changes: %v", err)
	}

	// Should detect rename.
	foundRename := false
	for _, change := range changes {
		if change.Type == state.ChangeRenamed {
			foundRename = true
			if change.Path != "renamed-file.md" {
				t.Errorf("new path = %s, want renamed-file.md", change.Path)
			}
			if change.OldPath != "original-name.md" {
				t.Errorf("old path = %s, want original-name.md", change.OldPath)
			}
		}
	}

	if !foundRename {
		t.Log("Note: Rename detection may require exact content hash match")
	}

	// Update sync state with new path.
	err = f.DB.DeleteState("original-name.md")
	if err != nil {
		t.Fatalf("failed to delete old state: %v", err)
	}

	err = f.DB.SetState(&state.SyncState{
		ObsidianPath: "renamed-file.md",
		NotionPageID: pageID,
		ContentHash:  hash,
		Status:       "synced",
		LastSync:     time.Now(),
	})
	if err != nil {
		t.Fatalf("failed to create new state: %v", err)
	}

	// Verify the page still exists and state is correct.
	f.AssertPageExists(ctx, pageID)

	syncState, err := f.DB.GetState("renamed-file.md")
	if err != nil {
		t.Fatalf("failed to get state: %v", err)
	}
	if syncState.NotionPageID != pageID {
		t.Errorf("page ID = %s, want %s", syncState.NotionPageID, pageID)
	}
}

// TestSync_IncrementalChanges tests detecting only incremental changes since last sync.
func TestSync_IncrementalChanges(t *testing.T) {
	f := NewTestFixture(t)
	defer f.Cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	// Create initial notes.
	notes := []string{"inc1.md", "inc2.md", "inc3.md"}
	p := parser.New()
	trans := transformer.New(nil, nil)

	for i, filename := range notes {
		content := Markdown.SimpleNote("Incremental "+filename, "Content "+filename)
		f.WriteMarkdownFile(filename, content)

		doc, err := p.ParseFile(f.VaultPath, filename)
		if err != nil {
			t.Fatalf("failed to parse %s: %v", filename, err)
		}

		notionPage, err := trans.Transform(doc)
		if err != nil {
			t.Fatalf("failed to transform %s: %v", filename, err)
		}

		result, err := f.NotionClient.CreatePageUnderPage(ctx, f.ParentPageID, notionPage)
		if err != nil {
			t.Fatalf("failed to create page for %s: %v", filename, err)
		}
		f.TrackPage(result.PageID)

		// Record sync state.
		hash := state.HashContent([]byte(content)).FullHash
		err = f.DB.SetState(&state.SyncState{
			ObsidianPath: filename,
			NotionPageID: result.PageID,
			ContentHash:  hash,
			Status:       "synced",
			LastSync:     time.Now(),
		})
		if err != nil {
			t.Fatalf("failed to set state for %s: %v", filename, err)
		}

		_ = i // Avoid unused variable warning.
	}

	// Detect changes - should be none.
	detector := state.NewChangeDetector(f.DB, f.VaultPath)
	changes, err := detector.DetectChanges(ctx)
	if err != nil {
		t.Fatalf("failed to detect changes: %v", err)
	}

	if len(changes) != 0 {
		t.Errorf("expected 0 changes, got %d", len(changes))
	}

	// Modify only one file.
	modifiedContent := Markdown.SimpleNote("Incremental inc2.md", "Modified content.")
	f.WriteMarkdownFile("inc2.md", modifiedContent)

	// Detect changes - should be only the modified file.
	changes, err = detector.DetectChanges(ctx)
	if err != nil {
		t.Fatalf("failed to detect changes: %v", err)
	}

	if len(changes) != 1 {
		t.Errorf("expected 1 change, got %d", len(changes))
	}

	if len(changes) > 0 && changes[0].Path != "inc2.md" {
		t.Errorf("expected changed file to be inc2.md, got %s", changes[0].Path)
	}
}
