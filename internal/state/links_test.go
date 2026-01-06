package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLinkRegistry_RegisterAndResolve(t *testing.T) {
	// Create temporary directory for test database.
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

	registry := NewLinkRegistry(db)

	// Register links from a source file.
	err = registry.RegisterLinks("source.md", []string{"Target Note", "Another Note"})
	if err != nil {
		t.Fatalf("register links: %v", err)
	}

	// Verify links are unresolved initially.
	unresolved, err := registry.GetUnresolvedLinks()
	if err != nil {
		t.Fatalf("get unresolved: %v", err)
	}
	if len(unresolved) != 2 {
		t.Errorf("expected 2 unresolved links, got %d", len(unresolved))
	}

	// Create sync state for target note.
	err = db.SetState(&SyncState{
		ObsidianPath: "Target Note.md",
		NotionPageID: "notion-page-123",
		Status:       "synced",
	})
	if err != nil {
		t.Fatalf("set state: %v", err)
	}

	// Now Resolve should find the target.
	pageID, found := registry.Resolve("Target Note")
	if !found {
		t.Error("expected to resolve 'Target Note'")
	}
	if pageID != "notion-page-123" {
		t.Errorf("expected page ID 'notion-page-123', got '%s'", pageID)
	}

	// ResolveAll should resolve the first link.
	count, err := registry.ResolveAll()
	if err != nil {
		t.Fatalf("resolve all: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 resolved link, got %d", count)
	}

	// Now only one link should be unresolved.
	unresolved, err = registry.GetUnresolvedLinks()
	if err != nil {
		t.Fatalf("get unresolved: %v", err)
	}
	if len(unresolved) != 1 {
		t.Errorf("expected 1 unresolved link, got %d", len(unresolved))
	}
	if unresolved[0].TargetName != "Another Note" {
		t.Errorf("expected unresolved target 'Another Note', got '%s'", unresolved[0].TargetName)
	}
}

func TestLinkRegistry_ResolveByPath(t *testing.T) {
	// Create temporary directory for test database.
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

	registry := NewLinkRegistry(db)

	// Create sync state for nested path.
	err = db.SetState(&SyncState{
		ObsidianPath: "work/projects/Architecture.md",
		NotionPageID: "notion-arch-456",
		Status:       "synced",
	})
	if err != nil {
		t.Fatalf("set state: %v", err)
	}

	// Should resolve by exact path.
	pageID, found := registry.Resolve("work/projects/Architecture")
	if !found {
		t.Error("expected to resolve exact path")
	}
	if pageID != "notion-arch-456" {
		t.Errorf("expected page ID 'notion-arch-456', got '%s'", pageID)
	}

	// Should resolve by file name only.
	pageID, found = registry.Resolve("Architecture")
	if !found {
		t.Error("expected to resolve by name")
	}
	if pageID != "notion-arch-456" {
		t.Errorf("expected page ID 'notion-arch-456', got '%s'", pageID)
	}
}

func TestLinkRegistry_ClearLinksFrom(t *testing.T) {
	// Create temporary directory for test database.
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

	registry := NewLinkRegistry(db)

	// Register links from two sources.
	err = registry.RegisterLinks("source1.md", []string{"Target A", "Target B"})
	if err != nil {
		t.Fatalf("register links: %v", err)
	}
	err = registry.RegisterLinks("source2.md", []string{"Target C"})
	if err != nil {
		t.Fatalf("register links: %v", err)
	}

	// Clear links from source1.
	err = registry.ClearLinksFrom("source1.md")
	if err != nil {
		t.Fatalf("clear links: %v", err)
	}

	// Should only have links from source2.
	links, err := registry.GetLinksFrom("source1.md")
	if err != nil {
		t.Fatalf("get links: %v", err)
	}
	if len(links) != 0 {
		t.Errorf("expected 0 links from source1, got %d", len(links))
	}

	links, err = registry.GetLinksFrom("source2.md")
	if err != nil {
		t.Fatalf("get links: %v", err)
	}
	if len(links) != 1 {
		t.Errorf("expected 1 link from source2, got %d", len(links))
	}
}

func TestLinkRegistry_LookupPath(t *testing.T) {
	// Create temporary directory for test database.
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

	registry := NewLinkRegistry(db)

	// Create sync state.
	err = db.SetState(&SyncState{
		ObsidianPath: "notes/my-note.md",
		NotionPageID: "notion-abc-789",
		Status:       "synced",
	})
	if err != nil {
		t.Fatalf("set state: %v", err)
	}

	// LookupPath (reverse lookup) should work.
	path, found := registry.LookupPath("notion-abc-789")
	if !found {
		t.Error("expected to find path by page ID")
	}
	if path != "notes/my-note.md" {
		t.Errorf("expected path 'notes/my-note.md', got '%s'", path)
	}

	// Unknown page ID should return not found.
	_, found = registry.LookupPath("unknown-page-id")
	if found {
		t.Error("expected not to find unknown page ID")
	}
}

func TestLinkRegistry_UpdateSourcePath(t *testing.T) {
	// Create temporary directory for test database.
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

	registry := NewLinkRegistry(db)

	// Register links from old path.
	err = registry.RegisterLinks("old-path.md", []string{"Target Note"})
	if err != nil {
		t.Fatalf("register links: %v", err)
	}

	// Update source path (simulating rename).
	err = registry.UpdateSourcePath("old-path.md", "new-path.md")
	if err != nil {
		t.Fatalf("update source path: %v", err)
	}

	// Links should now be under new path.
	links, err := registry.GetLinksFrom("old-path.md")
	if err != nil {
		t.Fatalf("get links: %v", err)
	}
	if len(links) != 0 {
		t.Errorf("expected 0 links from old path, got %d", len(links))
	}

	links, err = registry.GetLinksFrom("new-path.md")
	if err != nil {
		t.Fatalf("get links: %v", err)
	}
	if len(links) != 1 {
		t.Errorf("expected 1 link from new path, got %d", len(links))
	}
}

func TestLinkRegistry_GetBacklinks(t *testing.T) {
	// Create temporary directory for test database.
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

	registry := NewLinkRegistry(db)

	// Register links from multiple sources pointing to the same target.
	err = registry.RegisterLinks("note-a.md", []string{"Shared Target"})
	if err != nil {
		t.Fatalf("register links: %v", err)
	}
	err = registry.RegisterLinks("note-b.md", []string{"Shared Target", "Other Note"})
	if err != nil {
		t.Fatalf("register links: %v", err)
	}
	err = registry.RegisterLinks("note-c.md", []string{"Shared Target"})
	if err != nil {
		t.Fatalf("register links: %v", err)
	}

	// Get backlinks to "Shared Target".
	backlinks, err := registry.GetBacklinks("Shared Target.md")
	if err != nil {
		t.Fatalf("get backlinks: %v", err)
	}

	if len(backlinks) != 3 {
		t.Errorf("expected 3 backlinks, got %d", len(backlinks))
	}

	// Verify sources.
	sources := make(map[string]bool)
	for _, link := range backlinks {
		sources[link.SourcePath] = true
	}
	if !sources["note-a.md"] || !sources["note-b.md"] || !sources["note-c.md"] {
		t.Errorf("expected backlinks from note-a, note-b, note-c; got sources: %v", sources)
	}
}
