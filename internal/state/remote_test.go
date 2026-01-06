package state

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// MockRemoteChecker is a test implementation of RemoteChecker.
type MockRemoteChecker struct {
	Pages map[string]*RemotePageInfo
	Err   error // Global error to return from batch operations
}

func (m *MockRemoteChecker) GetPageInfo(ctx context.Context, pageID string) (*RemotePageInfo, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	if info, ok := m.Pages[pageID]; ok {
		return info, info.Err
	}
	return &RemotePageInfo{
		PageID: pageID,
		Err:    ErrPageNotFound{PageID: pageID},
	}, ErrPageNotFound{PageID: pageID}
}

func (m *MockRemoteChecker) GetPagesInfoBatch(ctx context.Context, pageIDs []string) (map[string]*RemotePageInfo, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	results := make(map[string]*RemotePageInfo)
	for _, id := range pageIDs {
		if info, ok := m.Pages[id]; ok {
			results[id] = info
		} else {
			results[id] = &RemotePageInfo{
				PageID: id,
				Err:    ErrPageNotFound{PageID: id},
			}
		}
	}
	return results, nil
}

func TestRemoteChangeDetector_LocalOnlyModified(t *testing.T) {
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

	// Set up initial state - file synced with Notion.
	now := time.Now()
	syncTime := now.Add(-1 * time.Hour)
	err = db.SetState(&SyncState{
		ObsidianPath: "test-note.md",
		NotionPageID: "page-123",
		ContentHash:  "old-hash",
		NotionMtime:  syncTime,
		LastSync:     syncTime,
		Status:       "synced",
	})
	if err != nil {
		t.Fatalf("set state: %v", err)
	}

	// Create modified local file.
	content := []byte("# Modified Note\n\nThis content has been changed locally.")
	err = os.WriteFile(filepath.Join(tmpDir, "test-note.md"), content, 0644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Mock remote - page NOT modified since last sync.
	mock := &MockRemoteChecker{
		Pages: map[string]*RemotePageInfo{
			"page-123": {
				PageID:         "page-123",
				LastEditedTime: syncTime, // Same as last sync
				Archived:       false,
			},
		},
	}

	// Detect changes.
	detector := NewRemoteChangeDetector(db, tmpDir, mock)
	changes, err := detector.DetectAllChanges(context.Background())
	if err != nil {
		t.Fatalf("detect changes: %v", err)
	}

	// Should have one change: local modification only (push).
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}

	change := changes[0]
	if change.Type != ChangeModified {
		t.Errorf("expected ChangeModified, got %v", change.Type)
	}
	if change.Direction != DirectionPush {
		t.Errorf("expected DirectionPush, got %v", change.Direction)
	}
	if change.Path != "test-note.md" {
		t.Errorf("expected path 'test-note.md', got '%s'", change.Path)
	}
}

func TestRemoteChangeDetector_RemoteOnlyModified(t *testing.T) {
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

	// Create a file with content matching the stored hash.
	content := []byte("# Test Note\n\nOriginal content.")
	contentHash := HashContent(content).FullHash

	// Set up initial state.
	now := time.Now()
	syncTime := now.Add(-1 * time.Hour)
	err = db.SetState(&SyncState{
		ObsidianPath: "test-note.md",
		NotionPageID: "page-456",
		ContentHash:  contentHash, // Matches local file
		NotionMtime:  syncTime,
		LastSync:     syncTime,
		Status:       "synced",
	})
	if err != nil {
		t.Fatalf("set state: %v", err)
	}

	// Write local file (unchanged).
	err = os.WriteFile(filepath.Join(tmpDir, "test-note.md"), content, 0644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Mock remote - page WAS modified since last sync.
	mock := &MockRemoteChecker{
		Pages: map[string]*RemotePageInfo{
			"page-456": {
				PageID:         "page-456",
				LastEditedTime: now, // Modified after sync
				Archived:       false,
			},
		},
	}

	// Detect changes.
	detector := NewRemoteChangeDetector(db, tmpDir, mock)
	changes, err := detector.DetectAllChanges(context.Background())
	if err != nil {
		t.Fatalf("detect changes: %v", err)
	}

	// Should have one change: remote modification only (pull).
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}

	change := changes[0]
	if change.Type != ChangeModified {
		t.Errorf("expected ChangeModified, got %v", change.Type)
	}
	if change.Direction != DirectionPull {
		t.Errorf("expected DirectionPull, got %v", change.Direction)
	}
}

func TestRemoteChangeDetector_Conflict(t *testing.T) {
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

	// Set up initial state with old content.
	now := time.Now()
	syncTime := now.Add(-1 * time.Hour)
	err = db.SetState(&SyncState{
		ObsidianPath: "test-note.md",
		NotionPageID: "page-789",
		ContentHash:  "original-hash",
		NotionMtime:  syncTime,
		LastSync:     syncTime,
		Status:       "synced",
	})
	if err != nil {
		t.Fatalf("set state: %v", err)
	}

	// Create locally modified file.
	content := []byte("# Locally Modified\n\nThis was changed locally.")
	err = os.WriteFile(filepath.Join(tmpDir, "test-note.md"), content, 0644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Mock remote - page ALSO modified since last sync.
	mock := &MockRemoteChecker{
		Pages: map[string]*RemotePageInfo{
			"page-789": {
				PageID:         "page-789",
				LastEditedTime: now, // Modified after sync
				Archived:       false,
			},
		},
	}

	// Detect changes.
	detector := NewRemoteChangeDetector(db, tmpDir, mock)
	changes, err := detector.DetectAllChanges(context.Background())
	if err != nil {
		t.Fatalf("detect changes: %v", err)
	}

	// Should have one change: conflict (both modified).
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}

	change := changes[0]
	if change.Type != ChangeConflict {
		t.Errorf("expected ChangeConflict, got %v", change.Type)
	}
	if change.Direction != DirectionBoth {
		t.Errorf("expected DirectionBoth, got %v", change.Direction)
	}
}

func TestRemoteChangeDetector_RemoteArchived(t *testing.T) {
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

	// Create a file with content matching the stored hash.
	content := []byte("# Test Note\n\nOriginal content.")
	contentHash := HashContent(content).FullHash

	// Set up initial state.
	now := time.Now()
	syncTime := now.Add(-1 * time.Hour)
	err = db.SetState(&SyncState{
		ObsidianPath: "archived-note.md",
		NotionPageID: "page-archived",
		ContentHash:  contentHash,
		NotionMtime:  syncTime,
		LastSync:     syncTime,
		Status:       "synced",
	})
	if err != nil {
		t.Fatalf("set state: %v", err)
	}

	// Write local file (unchanged from sync state).
	err = os.WriteFile(filepath.Join(tmpDir, "archived-note.md"), content, 0644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Mock remote - page was archived.
	mock := &MockRemoteChecker{
		Pages: map[string]*RemotePageInfo{
			"page-archived": {
				PageID:         "page-archived",
				LastEditedTime: now,
				Archived:       true, // Page was archived
			},
		},
	}

	// Detect changes.
	detector := NewRemoteChangeDetector(db, tmpDir, mock)
	changes, err := detector.DetectAllChanges(context.Background())
	if err != nil {
		t.Fatalf("detect changes: %v", err)
	}

	// Should have one change: remote deletion (pull to delete local).
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}

	change := changes[0]
	if change.Type != ChangeDeleted {
		t.Errorf("expected ChangeDeleted, got %v", change.Type)
	}
	if change.Direction != DirectionPull {
		t.Errorf("expected DirectionPull, got %v", change.Direction)
	}
}

func TestRemoteChangeDetector_NoRemoteChecker(t *testing.T) {
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

	// Create a new file.
	content := []byte("# New Note\n\nThis is new.")
	err = os.WriteFile(filepath.Join(tmpDir, "new-note.md"), content, 0644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Create detector with nil remote checker (graceful degradation).
	detector := NewRemoteChangeDetector(db, tmpDir, nil)
	changes, err := detector.DetectAllChanges(context.Background())
	if err != nil {
		t.Fatalf("detect changes: %v", err)
	}

	// Should still detect local changes.
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Type != ChangeCreated {
		t.Errorf("expected ChangeCreated, got %v", changes[0].Type)
	}
}

func TestRemoteChangeDetector_RemoteAPIFailure(t *testing.T) {
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

	// Set up state.
	now := time.Now()
	syncTime := now.Add(-1 * time.Hour)
	err = db.SetState(&SyncState{
		ObsidianPath: "test-note.md",
		NotionPageID: "page-123",
		ContentHash:  "some-hash",
		NotionMtime:  syncTime,
		LastSync:     syncTime,
		Status:       "synced",
	})
	if err != nil {
		t.Fatalf("set state: %v", err)
	}

	// Create modified local file.
	content := []byte("# Modified Note\n\nChanged locally.")
	err = os.WriteFile(filepath.Join(tmpDir, "test-note.md"), content, 0644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Mock remote that fails.
	mock := &MockRemoteChecker{
		Err: errors.New("API unavailable"),
	}

	// Detect changes - should gracefully return local changes only.
	detector := NewRemoteChangeDetector(db, tmpDir, mock)
	changes, err := detector.DetectAllChanges(context.Background())
	if err != nil {
		t.Fatalf("expected graceful degradation, got error: %v", err)
	}

	// Should still have the local change detected.
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Type != ChangeModified {
		t.Errorf("expected ChangeModified, got %v", changes[0].Type)
	}
	// Direction should be push (local only since remote failed).
	if changes[0].Direction != DirectionPush {
		t.Errorf("expected DirectionPush, got %v", changes[0].Direction)
	}
}

func TestFilterByDirection(t *testing.T) {
	changes := []Change{
		{Path: "push1.md", Direction: DirectionPush},
		{Path: "pull1.md", Direction: DirectionPull},
		{Path: "push2.md", Direction: DirectionPush},
		{Path: "conflict1.md", Direction: DirectionBoth},
	}

	pushChanges := FilterByDirection(changes, DirectionPush)
	if len(pushChanges) != 2 {
		t.Errorf("expected 2 push changes, got %d", len(pushChanges))
	}

	pullChanges := FilterByDirection(changes, DirectionPull)
	if len(pullChanges) != 1 {
		t.Errorf("expected 1 pull change, got %d", len(pullChanges))
	}

	conflicts := FilterByDirection(changes, DirectionBoth)
	if len(conflicts) != 1 {
		t.Errorf("expected 1 conflict, got %d", len(conflicts))
	}
}

func TestFilterByType(t *testing.T) {
	changes := []Change{
		{Path: "created.md", Type: ChangeCreated},
		{Path: "modified1.md", Type: ChangeModified},
		{Path: "modified2.md", Type: ChangeModified},
		{Path: "deleted.md", Type: ChangeDeleted},
		{Path: "conflict.md", Type: ChangeConflict},
	}

	created := FilterByType(changes, ChangeCreated)
	if len(created) != 1 {
		t.Errorf("expected 1 created, got %d", len(created))
	}

	modified := FilterByType(changes, ChangeModified)
	if len(modified) != 2 {
		t.Errorf("expected 2 modified, got %d", len(modified))
	}

	conflicts := FilterByType(changes, ChangeConflict)
	if len(conflicts) != 1 {
		t.Errorf("expected 1 conflict, got %d", len(conflicts))
	}
}

func TestHasConflicts(t *testing.T) {
	noConflicts := []Change{
		{Type: ChangeCreated},
		{Type: ChangeModified},
	}
	if HasConflicts(noConflicts) {
		t.Error("expected no conflicts")
	}

	withConflicts := []Change{
		{Type: ChangeCreated},
		{Type: ChangeConflict},
	}
	if !HasConflicts(withConflicts) {
		t.Error("expected conflicts to be detected")
	}
}

func TestCountByType(t *testing.T) {
	changes := []Change{
		{Type: ChangeCreated},
		{Type: ChangeModified},
		{Type: ChangeModified},
		{Type: ChangeDeleted},
	}

	counts := CountByType(changes)
	if counts[ChangeCreated] != 1 {
		t.Errorf("expected 1 created, got %d", counts[ChangeCreated])
	}
	if counts[ChangeModified] != 2 {
		t.Errorf("expected 2 modified, got %d", counts[ChangeModified])
	}
	if counts[ChangeDeleted] != 1 {
		t.Errorf("expected 1 deleted, got %d", counts[ChangeDeleted])
	}
}

func TestCountByDirection(t *testing.T) {
	changes := []Change{
		{Direction: DirectionPush},
		{Direction: DirectionPush},
		{Direction: DirectionPull},
		{Direction: DirectionBoth},
	}

	counts := CountByDirection(changes)
	if counts[DirectionPush] != 2 {
		t.Errorf("expected 2 push, got %d", counts[DirectionPush])
	}
	if counts[DirectionPull] != 1 {
		t.Errorf("expected 1 pull, got %d", counts[DirectionPull])
	}
	if counts[DirectionBoth] != 1 {
		t.Errorf("expected 1 both, got %d", counts[DirectionBoth])
	}
}
