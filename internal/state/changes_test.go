package state

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectRenames(t *testing.T) {
	// Create temporary directory for test vault.
	tmpDir, err := os.MkdirTemp("", "obsidian-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test database.
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	// Create a file with known content.
	content := []byte("# Test Note\n\nThis is a test note.")
	contentHash := HashContent(content).FullHash

	// Set up sync state for "old-name.md" with known content hash.
	err = db.SetState(&SyncState{
		ObsidianPath: "old-name.md",
		NotionPageID: "page-123",
		ContentHash:  contentHash,
		Status:       "synced",
	})
	if err != nil {
		t.Fatalf("set state: %v", err)
	}

	// Create a "new-name.md" file with the same content.
	err = os.WriteFile(filepath.Join(tmpDir, "new-name.md"), content, 0644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Detect changes - should find a rename.
	detector := NewChangeDetector(db, tmpDir)
	changes, err := detector.DetectChanges(context.Background())
	if err != nil {
		t.Fatalf("detect changes: %v", err)
	}

	// Should have exactly one change: a rename.
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}

	change := changes[0]
	if change.Type != ChangeRenamed {
		t.Errorf("expected ChangeRenamed, got %v", change.Type)
	}
	if change.OldPath != "old-name.md" {
		t.Errorf("expected old path 'old-name.md', got '%s'", change.OldPath)
	}
	if change.Path != "new-name.md" {
		t.Errorf("expected new path 'new-name.md', got '%s'", change.Path)
	}
	if change.State == nil {
		t.Error("expected state to be set")
	} else if change.State.NotionPageID != "page-123" {
		t.Errorf("expected page ID 'page-123', got '%s'", change.State.NotionPageID)
	}
}

func TestDetectDeletions(t *testing.T) {
	// Create temporary directory for test vault.
	tmpDir, err := os.MkdirTemp("", "obsidian-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test database.
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	// Set up sync state for a file that doesn't exist on disk.
	err = db.SetState(&SyncState{
		ObsidianPath: "deleted.md",
		NotionPageID: "page-456",
		ContentHash:  "somehash",
		Status:       "synced",
	})
	if err != nil {
		t.Fatalf("set state: %v", err)
	}

	// Detect changes - should find a deletion.
	detector := NewChangeDetector(db, tmpDir)
	changes, err := detector.DetectChanges(context.Background())
	if err != nil {
		t.Fatalf("detect changes: %v", err)
	}

	// Should have exactly one change: a deletion.
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}

	change := changes[0]
	if change.Type != ChangeDeleted {
		t.Errorf("expected ChangeDeleted, got %v", change.Type)
	}
	if change.Path != "deleted.md" {
		t.Errorf("expected path 'deleted.md', got '%s'", change.Path)
	}
	if change.State == nil {
		t.Error("expected state to be set")
	} else if change.State.NotionPageID != "page-456" {
		t.Errorf("expected page ID 'page-456', got '%s'", change.State.NotionPageID)
	}
}

func TestDetectCreations(t *testing.T) {
	// Create temporary directory for test vault.
	tmpDir, err := os.MkdirTemp("", "obsidian-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test database.
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	// Create a new file with no sync state.
	content := []byte("# New Note\n\nThis is a new note.")
	err = os.WriteFile(filepath.Join(tmpDir, "new-note.md"), content, 0644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Detect changes - should find a creation.
	detector := NewChangeDetector(db, tmpDir)
	changes, err := detector.DetectChanges(context.Background())
	if err != nil {
		t.Fatalf("detect changes: %v", err)
	}

	// Should have exactly one change: a creation.
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}

	change := changes[0]
	if change.Type != ChangeCreated {
		t.Errorf("expected ChangeCreated, got %v", change.Type)
	}
	if change.Path != "new-note.md" {
		t.Errorf("expected path 'new-note.md', got '%s'", change.Path)
	}
}

func TestDetectModifications(t *testing.T) {
	// Create temporary directory for test vault.
	tmpDir, err := os.MkdirTemp("", "obsidian-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test database.
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	// Create a file.
	newContent := []byte("# Modified Note\n\nThis content is different.")
	err = os.WriteFile(filepath.Join(tmpDir, "existing.md"), newContent, 0644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Set up sync state with a different content hash.
	err = db.SetState(&SyncState{
		ObsidianPath: "existing.md",
		NotionPageID: "page-789",
		ContentHash:  "old-hash-different-from-current",
		Status:       "synced",
	})
	if err != nil {
		t.Fatalf("set state: %v", err)
	}

	// Detect changes - should find a modification.
	detector := NewChangeDetector(db, tmpDir)
	changes, err := detector.DetectChanges(context.Background())
	if err != nil {
		t.Fatalf("detect changes: %v", err)
	}

	// Should have exactly one change: a modification.
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}

	change := changes[0]
	if change.Type != ChangeModified {
		t.Errorf("expected ChangeModified, got %v", change.Type)
	}
	if change.Path != "existing.md" {
		t.Errorf("expected path 'existing.md', got '%s'", change.Path)
	}
	if change.Direction != DirectionPush {
		t.Errorf("expected DirectionPush, got %v", change.Direction)
	}
}

func TestDetectFrontmatterOnlyModification(t *testing.T) {
	// Create temporary directory for test vault.
	tmpDir, err := os.MkdirTemp("", "obsidian-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test database.
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	// Original content with frontmatter.
	originalContent := []byte(`---
title: Original Title
tags:
  - tag1
---
# Body Content

This body stays the same.
`)

	// Modified content - only frontmatter changed.
	modifiedContent := []byte(`---
title: Changed Title
tags:
  - tag1
  - tag2
---
# Body Content

This body stays the same.
`)

	// Compute hashes for the original content.
	originalHashes := HashContent(originalContent)

	// Write the modified file to disk.
	err = os.WriteFile(filepath.Join(tmpDir, "note-with-fm.md"), modifiedContent, 0644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Set up sync state with original hashes.
	err = db.SetState(&SyncState{
		ObsidianPath:    "note-with-fm.md",
		NotionPageID:    "page-fm-test",
		ContentHash:     originalHashes.ContentHash,     // Body hash
		FrontmatterHash: originalHashes.FrontmatterHash, // Frontmatter hash
		Status:          "synced",
	})
	if err != nil {
		t.Fatalf("set state: %v", err)
	}

	// Detect changes.
	detector := NewChangeDetector(db, tmpDir)
	changes, err := detector.DetectChanges(context.Background())
	if err != nil {
		t.Fatalf("detect changes: %v", err)
	}

	// Should have exactly one change.
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}

	change := changes[0]
	if change.Type != ChangeModified {
		t.Errorf("expected ChangeModified, got %v", change.Type)
	}

	// Verify FrontmatterOnly flag is set.
	if !change.FrontmatterOnly {
		t.Error("expected FrontmatterOnly to be true for frontmatter-only change")
	}
}

func TestDetectBodyOnlyModification(t *testing.T) {
	// Create temporary directory for test vault.
	tmpDir, err := os.MkdirTemp("", "obsidian-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test database.
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	// Original content with frontmatter.
	originalContent := []byte(`---
title: Same Title
---
# Original Body

This is the original body.
`)

	// Modified content - only body changed.
	modifiedContent := []byte(`---
title: Same Title
---
# Modified Body

This body has been changed.
`)

	// Compute hashes for the original content.
	originalHashes := HashContent(originalContent)

	// Write the modified file to disk.
	err = os.WriteFile(filepath.Join(tmpDir, "note-body-change.md"), modifiedContent, 0644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Set up sync state with original hashes.
	err = db.SetState(&SyncState{
		ObsidianPath:    "note-body-change.md",
		NotionPageID:    "page-body-test",
		ContentHash:     originalHashes.ContentHash,
		FrontmatterHash: originalHashes.FrontmatterHash,
		Status:          "synced",
	})
	if err != nil {
		t.Fatalf("set state: %v", err)
	}

	// Detect changes.
	detector := NewChangeDetector(db, tmpDir)
	changes, err := detector.DetectChanges(context.Background())
	if err != nil {
		t.Fatalf("detect changes: %v", err)
	}

	// Should have exactly one change.
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}

	change := changes[0]
	if change.Type != ChangeModified {
		t.Errorf("expected ChangeModified, got %v", change.Type)
	}

	// Verify FrontmatterOnly flag is NOT set (body changed).
	if change.FrontmatterOnly {
		t.Error("expected FrontmatterOnly to be false when body content changed")
	}
}

func TestDetectNoChangeWhenHashesMatch(t *testing.T) {
	// Create temporary directory for test vault.
	tmpDir, err := os.MkdirTemp("", "obsidian-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test database.
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	// Content for the file.
	content := []byte(`---
title: Unchanged Note
---
# Body Content

This content has not changed.
`)

	// Compute hashes.
	hashes := HashContent(content)

	// Write file to disk.
	err = os.WriteFile(filepath.Join(tmpDir, "unchanged.md"), content, 0644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Set up sync state with matching hashes.
	err = db.SetState(&SyncState{
		ObsidianPath:    "unchanged.md",
		NotionPageID:    "page-unchanged",
		ContentHash:     hashes.ContentHash,
		FrontmatterHash: hashes.FrontmatterHash,
		Status:          "synced",
	})
	if err != nil {
		t.Fatalf("set state: %v", err)
	}

	// Detect changes.
	detector := NewChangeDetector(db, tmpDir)
	changes, err := detector.DetectChanges(context.Background())
	if err != nil {
		t.Fatalf("detect changes: %v", err)
	}

	// Should have no changes when content matches stored hashes.
	if len(changes) != 0 {
		t.Errorf("expected 0 changes for unchanged file, got %d", len(changes))
		for _, c := range changes {
			t.Logf("  unexpected change: type=%v path=%s", c.Type, c.Path)
		}
	}
}

func TestDetectFrontmatterOnlyFile(t *testing.T) {
	// Create temporary directory for test vault.
	tmpDir, err := os.MkdirTemp("", "obsidian-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test database.
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	// File with only frontmatter, no body.
	content := []byte(`---
title: Metadata Only
type: config
---
`)

	// Write file to disk.
	err = os.WriteFile(filepath.Join(tmpDir, "metadata-only.md"), content, 0644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Detect changes - should find a creation.
	detector := NewChangeDetector(db, tmpDir)
	changes, err := detector.DetectChanges(context.Background())
	if err != nil {
		t.Fatalf("detect changes: %v", err)
	}

	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}

	change := changes[0]
	if change.Type != ChangeCreated {
		t.Errorf("expected ChangeCreated, got %v", change.Type)
	}

	// Verify LocalHashes are properly set.
	if change.LocalHashes.FrontmatterHash == "" {
		t.Error("expected non-empty FrontmatterHash for frontmatter-only file")
	}
	if change.LocalHashes.ContentHash != "" {
		t.Error("expected empty ContentHash for frontmatter-only file (no body)")
	}
}

func TestDetectEmptyFile(t *testing.T) {
	// Create temporary directory for test vault.
	tmpDir, err := os.MkdirTemp("", "obsidian-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test database.
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	// Empty file.
	err = os.WriteFile(filepath.Join(tmpDir, "empty.md"), []byte{}, 0644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Detect changes - should find a creation even for empty file.
	detector := NewChangeDetector(db, tmpDir)
	changes, err := detector.DetectChanges(context.Background())
	if err != nil {
		t.Fatalf("detect changes: %v", err)
	}

	if len(changes) != 1 {
		t.Fatalf("expected 1 change for empty file, got %d", len(changes))
	}

	change := changes[0]
	if change.Type != ChangeCreated {
		t.Errorf("expected ChangeCreated, got %v", change.Type)
	}

	// Verify all hashes are empty for empty file.
	if change.LocalHashes.ContentHash != "" {
		t.Error("expected empty ContentHash for empty file")
	}
	if change.LocalHashes.FrontmatterHash != "" {
		t.Error("expected empty FrontmatterHash for empty file")
	}
	if change.LocalHashes.FullHash != "" {
		t.Error("expected empty FullHash for empty file")
	}
}

func TestDetectWhitespaceNormalizationNoChange(t *testing.T) {
	// Create temporary directory for test vault.
	tmpDir, err := os.MkdirTemp("", "obsidian-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test database.
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	// Original content (normalized).
	originalContent := []byte("# Title\n\nParagraph one.\n\nParagraph two.")

	// "Modified" content with trailing whitespace and extra blank lines.
	// Should normalize to the same hash.
	modifiedContent := []byte("# Title  \n\n\n\nParagraph one.   \n\nParagraph two.  ")

	// Compute hashes for the original content.
	originalHashes := HashContent(originalContent)

	// Write the "modified" file to disk.
	err = os.WriteFile(filepath.Join(tmpDir, "whitespace-test.md"), modifiedContent, 0644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Set up sync state with original hashes.
	err = db.SetState(&SyncState{
		ObsidianPath:    "whitespace-test.md",
		NotionPageID:    "page-ws-test",
		ContentHash:     originalHashes.ContentHash,
		FrontmatterHash: originalHashes.FrontmatterHash,
		Status:          "synced",
	})
	if err != nil {
		t.Fatalf("set state: %v", err)
	}

	// Detect changes.
	detector := NewChangeDetector(db, tmpDir)
	changes, err := detector.DetectChanges(context.Background())
	if err != nil {
		t.Fatalf("detect changes: %v", err)
	}

	// Should have no changes - whitespace normalization makes them equivalent.
	if len(changes) != 0 {
		t.Errorf("expected 0 changes after whitespace normalization, got %d", len(changes))
	}
}
