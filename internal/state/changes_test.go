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
	contentHash := hashContent(content)

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
