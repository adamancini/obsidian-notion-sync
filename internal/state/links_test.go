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

func TestParseTarget(t *testing.T) {
	tests := []struct {
		input    string
		wantPage string
		wantHead string
		wantBlk  string
	}{
		{"Page", "Page", "", ""},
		{"Page.md", "Page", "", ""},
		{"Page#Heading", "Page", "Heading", ""},
		{"Page#Heading with spaces", "Page", "Heading with spaces", ""},
		{"Page^block-id", "Page", "", "block-id"},
		{"Page#Heading^block-id", "Page", "Heading", "block-id"},
		{"folder/Page#Section", "folder/Page", "Section", ""},
		{"work/notes/Architecture.md#Overview", "work/notes/Architecture", "Overview", ""},
	}

	for _, tc := range tests {
		page, heading, blockRef := parseTarget(tc.input)
		if page != tc.wantPage {
			t.Errorf("parseTarget(%q) page = %q; want %q", tc.input, page, tc.wantPage)
		}
		if heading != tc.wantHead {
			t.Errorf("parseTarget(%q) heading = %q; want %q", tc.input, heading, tc.wantHead)
		}
		if blockRef != tc.wantBlk {
			t.Errorf("parseTarget(%q) blockRef = %q; want %q", tc.input, blockRef, tc.wantBlk)
		}
	}
}

func TestLinkRegistry_ResolveExtended(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "obsidian-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	registry := NewLinkRegistry(db)

	// Create sync state for test notes.
	err = db.SetState(&SyncState{
		ObsidianPath: "ServiceClass Architecture.md",
		NotionPageID: "page-service-class",
		Status:       "synced",
	})
	if err != nil {
		t.Fatalf("set state: %v", err)
	}

	// Test exact match with heading anchor.
	result := registry.ResolveExtended("ServiceClass Architecture#Overview", false)
	if !result.Found {
		t.Error("expected to resolve exact target")
	}
	if result.PageID != "page-service-class" {
		t.Errorf("page ID = %q; want %q", result.PageID, "page-service-class")
	}
	if result.Heading != "Overview" {
		t.Errorf("heading = %q; want %q", result.Heading, "Overview")
	}
	if result.FuzzyMatch {
		t.Error("expected exact match, not fuzzy")
	}

	// Test fuzzy match with typo.
	result = registry.ResolveExtended("SrviceClass Architecture", true)
	if !result.Found {
		t.Error("expected to resolve fuzzy target")
	}
	if !result.FuzzyMatch {
		t.Error("expected fuzzy match flag to be true")
	}

	// Test unresolved without fuzzy.
	result = registry.ResolveExtended("SrviceClass Architecture", false)
	if result.Found {
		t.Error("expected NOT to resolve without fuzzy matching")
	}
}

func TestLinkRegistry_GetSuggestions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "obsidian-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	registry := NewLinkRegistry(db)

	// Set up some synced notes.
	for _, note := range []struct {
		path, pageID string
	}{
		{"Architecture.md", "page-arch"},
		{"ServiceClass.md", "page-svc"},
		{"Configuration.md", "page-cfg"},
	} {
		err = db.SetState(&SyncState{
			ObsidianPath: note.path,
			NotionPageID: note.pageID,
			Status:       "synced",
		})
		if err != nil {
			t.Fatalf("set state: %v", err)
		}
	}

	// Register an unresolved link with a typo.
	err = registry.RegisterLinks("index.md", []string{"Architcture", "ServiceClas"})
	if err != nil {
		t.Fatalf("register links: %v", err)
	}

	// Get suggestions.
	suggestions, err := registry.GetSuggestionsForUnresolved(3)
	if err != nil {
		t.Fatalf("get suggestions: %v", err)
	}

	if len(suggestions) != 2 {
		t.Fatalf("expected 2 suggestions, got %d", len(suggestions))
	}

	// Verify suggestions are reasonable.
	foundArch := false
	foundSvc := false
	for _, s := range suggestions {
		if s.Target == "Architcture" && len(s.Suggestions) > 0 {
			if s.Suggestions[0].Path == "Architecture.md" {
				foundArch = true
			}
		}
		if s.Target == "ServiceClas" && len(s.Suggestions) > 0 {
			if s.Suggestions[0].Path == "ServiceClass.md" {
				foundSvc = true
			}
		}
	}

	if !foundArch {
		t.Error("expected 'Architecture.md' as suggestion for 'Architcture'")
	}
	if !foundSvc {
		t.Error("expected 'ServiceClass.md' as suggestion for 'ServiceClas'")
	}
}

func TestLinkRegistry_RepairLinks(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "obsidian-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	registry := NewLinkRegistry(db)

	// Set up synced note.
	err = db.SetState(&SyncState{
		ObsidianPath: "MyNote.md",
		NotionPageID: "page-mynote",
		Status:       "synced",
	})
	if err != nil {
		t.Fatalf("set state: %v", err)
	}

	// Register unresolved link with typo.
	err = registry.RegisterLinks("source.md", []string{"MyNot"})
	if err != nil {
		t.Fatalf("register links: %v", err)
	}

	// Dry run should show what would be repaired.
	results, err := registry.RepairLinks(true)
	if err != nil {
		t.Fatalf("repair links dry run: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 repair result, got %d", len(results))
	}
	if !results[0].WouldRepair {
		t.Error("expected WouldRepair to be true")
	}
	if results[0].WasRepaired {
		t.Error("expected WasRepaired to be false in dry run")
	}

	// Verify still unresolved.
	unresolved, _ := registry.GetUnresolvedLinks()
	if len(unresolved) != 1 {
		t.Errorf("expected 1 unresolved after dry run, got %d", len(unresolved))
	}

	// Actual repair.
	results, err = registry.RepairLinks(false)
	if err != nil {
		t.Fatalf("repair links: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 repair result, got %d", len(results))
	}
	if !results[0].WasRepaired {
		t.Error("expected WasRepaired to be true")
	}

	// Now should be resolved.
	unresolved, _ = registry.GetUnresolvedLinks()
	if len(unresolved) != 0 {
		t.Errorf("expected 0 unresolved after repair, got %d", len(unresolved))
	}
}

func TestLinkRegistry_GetStats(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "obsidian-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	registry := NewLinkRegistry(db)

	// Register some links.
	err = registry.RegisterLinks("note-a.md", []string{"Target1", "Target2", "Target3"})
	if err != nil {
		t.Fatalf("register links: %v", err)
	}
	err = registry.RegisterLinks("note-b.md", []string{"Target4"})
	if err != nil {
		t.Fatalf("register links: %v", err)
	}

	stats, err := registry.GetStats()
	if err != nil {
		t.Fatalf("get stats: %v", err)
	}

	if stats.Total != 4 {
		t.Errorf("total = %d; want 4", stats.Total)
	}
	if stats.Resolved != 0 {
		t.Errorf("resolved = %d; want 0", stats.Resolved)
	}
	if stats.Unresolved != 4 {
		t.Errorf("unresolved = %d; want 4", stats.Unresolved)
	}
	if stats.BySource["note-a.md"] != 3 {
		t.Errorf("bySource[note-a.md] = %d; want 3", stats.BySource["note-a.md"])
	}
	if stats.BySource["note-b.md"] != 1 {
		t.Errorf("bySource[note-b.md] = %d; want 1", stats.BySource["note-b.md"])
	}
}

// TestLinkRegistry_ResolveByTitle tests resolving wiki-links by frontmatter title
// when the title differs from the filename. This is the bug from ANN-42.
//
// In Obsidian, [[Target Note]] should resolve to target-note.md if that file
// has `title: Target Note` in its frontmatter.
func TestLinkRegistry_ResolveByTitle(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "obsidian-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	registry := NewLinkRegistry(db)

	// Create sync state with kebab-case filename (how files are often named).
	err = db.SetState(&SyncState{
		ObsidianPath: "target-note.md",
		NotionPageID: "notion-page-123",
		Status:       "synced",
	})
	if err != nil {
		t.Fatalf("set state: %v", err)
	}

	// Register the title as an alias (simulating what push should do).
	err = registry.RegisterAlias("target-note.md", "Target Note", "title")
	if err != nil {
		t.Fatalf("register alias: %v", err)
	}

	// Now [[Target Note]] should resolve to the page, even though the
	// filename is target-note.md (different from the title).
	pageID, found := registry.Resolve("Target Note")
	if !found {
		t.Error("expected to resolve 'Target Note' by title alias")
	}
	if pageID != "notion-page-123" {
		t.Errorf("expected page ID 'notion-page-123', got '%s'", pageID)
	}

	// Resolving by filename should still work.
	pageID, found = registry.Resolve("target-note")
	if !found {
		t.Error("expected to resolve 'target-note' by filename")
	}
	if pageID != "notion-page-123" {
		t.Errorf("expected page ID 'notion-page-123', got '%s'", pageID)
	}
}
