package vault

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// setupTestVault creates a temporary vault structure for testing.
func setupTestVault(t *testing.T) string {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "vault-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}

	// Create vault structure:
	// tmpDir/
	// ├── root.md
	// ├── notes/
	// │   ├── note1.md
	// │   └── note2.md
	// ├── projects/
	// │   └── project.md
	// ├── .hidden/
	// │   └── secret.md
	// └── other.txt

	files := map[string]string{
		"root.md":               "# Root note",
		"notes/note1.md":        "# Note 1",
		"notes/note2.md":        "# Note 2",
		"projects/project.md":   "# Project note",
		".hidden/secret.md":     "# Secret note",
		"other.txt":             "Not a markdown file",
		"ignored/skip.md":       "# Should be ignored",
		"deep/nested/file.md":   "# Deep nested",
	}

	for path, content := range files {
		fullPath := filepath.Join(tmpDir, path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("create dir %s: %v", dir, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("write file %s: %v", path, err)
		}
	}

	return tmpDir
}

func TestNewScanner(t *testing.T) {
	scanner := NewScanner("/path/to/vault", []string{"*.ignore"})

	if scanner.root != "/path/to/vault" {
		t.Errorf("expected root '/path/to/vault', got %q", scanner.root)
	}
	if len(scanner.ignore) != 1 || scanner.ignore[0] != "*.ignore" {
		t.Errorf("unexpected ignore patterns: %v", scanner.ignore)
	}
}

func TestScanner_Scan(t *testing.T) {
	tmpDir := setupTestVault(t)
	defer os.RemoveAll(tmpDir)

	scanner := NewScanner(tmpDir, nil)
	ctx := context.Background()

	files, err := scanner.Scan(ctx)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	// Should find markdown files but not:
	// - .txt files
	// - files in hidden directories
	expectedPaths := map[string]bool{
		"root.md":             true,
		"notes/note1.md":      true,
		"notes/note2.md":      true,
		"projects/project.md": true,
		"ignored/skip.md":     true,
		"deep/nested/file.md": true,
	}

	if len(files) != len(expectedPaths) {
		t.Errorf("expected %d files, got %d", len(expectedPaths), len(files))
		for _, f := range files {
			t.Logf("  found: %s", f.Path)
		}
	}

	for _, f := range files {
		if !expectedPaths[f.Path] {
			t.Errorf("unexpected file: %s", f.Path)
		}
		delete(expectedPaths, f.Path)
	}

	for path := range expectedPaths {
		t.Errorf("missing expected file: %s", path)
	}
}

func TestScanner_Scan_WithIgnorePatterns(t *testing.T) {
	tmpDir := setupTestVault(t)
	defer os.RemoveAll(tmpDir)

	// Ignore 'ignored/' directory files
	scanner := NewScanner(tmpDir, []string{"ignored/*", "skip.md"})
	ctx := context.Background()

	files, err := scanner.Scan(ctx)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	for _, f := range files {
		if f.Path == "ignored/skip.md" {
			t.Error("should have ignored ignored/skip.md")
		}
	}
}

func TestScanner_Scan_ContextCancellation(t *testing.T) {
	tmpDir := setupTestVault(t)
	defer os.RemoveAll(tmpDir)

	scanner := NewScanner(tmpDir, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := scanner.Scan(ctx)
	if err == nil {
		t.Error("expected context cancellation error")
	}
}

func TestScanner_ScanDir(t *testing.T) {
	tmpDir := setupTestVault(t)
	defer os.RemoveAll(tmpDir)

	scanner := NewScanner(tmpDir, nil)
	ctx := context.Background()

	// Scan only the 'notes' directory
	files, err := scanner.ScanDir(ctx, "notes")
	if err != nil {
		t.Fatalf("scan dir: %v", err)
	}

	if len(files) != 2 {
		t.Errorf("expected 2 files in notes/, got %d", len(files))
	}

	for _, f := range files {
		if filepath.Dir(f.Path) != "notes" {
			t.Errorf("unexpected file outside notes/: %s", f.Path)
		}
	}
}

func TestScanner_ScanDir_NonExistent(t *testing.T) {
	tmpDir := setupTestVault(t)
	defer os.RemoveAll(tmpDir)

	scanner := NewScanner(tmpDir, nil)
	ctx := context.Background()

	files, err := scanner.ScanDir(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("scan dir: %v", err)
	}

	if files != nil {
		t.Errorf("expected nil for non-existent directory, got %v", files)
	}
}

func TestScanner_ScanGlob(t *testing.T) {
	tmpDir := setupTestVault(t)
	defer os.RemoveAll(tmpDir)

	scanner := NewScanner(tmpDir, nil)
	ctx := context.Background()

	// Match files directly in notes/
	files, err := scanner.ScanGlob(ctx, "notes/*.md")
	if err != nil {
		t.Fatalf("scan glob: %v", err)
	}

	if len(files) != 2 {
		t.Errorf("expected 2 files matching notes/*.md, got %d", len(files))
	}
}

func TestScanner_GetFile(t *testing.T) {
	tmpDir := setupTestVault(t)
	defer os.RemoveAll(tmpDir)

	scanner := NewScanner(tmpDir, nil)

	file, err := scanner.GetFile("notes/note1.md")
	if err != nil {
		t.Fatalf("get file: %v", err)
	}

	if file.Path != "notes/note1.md" {
		t.Errorf("expected path 'notes/note1.md', got %q", file.Path)
	}
	if file.AbsPath != filepath.Join(tmpDir, "notes/note1.md") {
		t.Errorf("unexpected abs path: %s", file.AbsPath)
	}
	if file.Info == nil {
		t.Error("expected file info")
	}
}

func TestScanner_GetFile_NotFound(t *testing.T) {
	tmpDir := setupTestVault(t)
	defer os.RemoveAll(tmpDir)

	scanner := NewScanner(tmpDir, nil)

	_, err := scanner.GetFile("nonexistent.md")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestScanner_ReadFile(t *testing.T) {
	tmpDir := setupTestVault(t)
	defer os.RemoveAll(tmpDir)

	scanner := NewScanner(tmpDir, nil)

	content, err := scanner.ReadFile("root.md")
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	if string(content) != "# Root note" {
		t.Errorf("expected '# Root note', got %q", string(content))
	}
}

func TestScanner_WriteFile(t *testing.T) {
	tmpDir := setupTestVault(t)
	defer os.RemoveAll(tmpDir)

	scanner := NewScanner(tmpDir, nil)

	// Write to new nested path
	err := scanner.WriteFile("new/nested/file.md", []byte("# New file"))
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Verify it was written
	content, err := scanner.ReadFile("new/nested/file.md")
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}

	if string(content) != "# New file" {
		t.Errorf("expected '# New file', got %q", string(content))
	}
}

func TestScanner_DeleteFile(t *testing.T) {
	tmpDir := setupTestVault(t)
	defer os.RemoveAll(tmpDir)

	scanner := NewScanner(tmpDir, nil)

	// Verify file exists
	if !scanner.Exists("root.md") {
		t.Fatal("root.md should exist before deletion")
	}

	// Delete the file
	err := scanner.DeleteFile("root.md")
	if err != nil {
		t.Fatalf("delete file: %v", err)
	}

	// Verify it's gone
	if scanner.Exists("root.md") {
		t.Error("root.md should not exist after deletion")
	}
}

func TestScanner_Exists(t *testing.T) {
	tmpDir := setupTestVault(t)
	defer os.RemoveAll(tmpDir)

	scanner := NewScanner(tmpDir, nil)

	if !scanner.Exists("root.md") {
		t.Error("expected root.md to exist")
	}

	if scanner.Exists("nonexistent.md") {
		t.Error("expected nonexistent.md to not exist")
	}
}

func TestScanner_Root(t *testing.T) {
	scanner := NewScanner("/path/to/vault", nil)

	if scanner.Root() != "/path/to/vault" {
		t.Errorf("expected root '/path/to/vault', got %q", scanner.Root())
	}
}

func TestScanner_ListDirectories(t *testing.T) {
	tmpDir := setupTestVault(t)
	defer os.RemoveAll(tmpDir)

	scanner := NewScanner(tmpDir, nil)
	ctx := context.Background()

	dirs, err := scanner.ListDirectories(ctx)
	if err != nil {
		t.Fatalf("list directories: %v", err)
	}

	// Expected directories (not including hidden .hidden)
	expectedDirs := map[string]bool{
		"notes":       true,
		"projects":    true,
		"ignored":     true,
		"deep":        true,
		"deep/nested": true,
	}

	if len(dirs) != len(expectedDirs) {
		t.Errorf("expected %d directories, got %d", len(expectedDirs), len(dirs))
		for _, d := range dirs {
			t.Logf("  found: %s", d)
		}
	}

	for _, d := range dirs {
		if !expectedDirs[d] {
			t.Errorf("unexpected directory: %s", d)
		}
		delete(expectedDirs, d)
	}

	for d := range expectedDirs {
		t.Errorf("missing expected directory: %s", d)
	}
}

func TestScanner_shouldIgnore(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		path     string
		expected bool
	}{
		{
			name:     "no patterns",
			patterns: nil,
			path:     "any/path.md",
			expected: false,
		},
		{
			name:     "exact file match",
			patterns: []string{"ignore.md"},
			path:     "ignore.md",
			expected: true,
		},
		{
			name:     "wildcard match",
			patterns: []string{"*.tmp"},
			path:     "file.tmp",
			expected: true,
		},
		{
			name:     "directory pattern",
			patterns: []string{"templates/*"},
			path:     "templates/note.md",
			expected: true,
		},
		{
			name:     "basename match",
			patterns: []string{"skip.md"},
			path:     "deep/nested/skip.md",
			expected: true,
		},
		{
			name:     "no match",
			patterns: []string{"*.tmp"},
			path:     "file.md",
			expected: false,
		},
		{
			name:     "multiple patterns one match",
			patterns: []string{"*.tmp", "*.bak", "skip.md"},
			path:     "file.bak",
			expected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scanner := NewScanner("/vault", tc.patterns)
			result := scanner.shouldIgnore(tc.path)
			if result != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestScanner_FileInfo(t *testing.T) {
	tmpDir := setupTestVault(t)
	defer os.RemoveAll(tmpDir)

	scanner := NewScanner(tmpDir, nil)
	ctx := context.Background()

	files, err := scanner.Scan(ctx)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	for _, f := range files {
		// Verify Info is populated
		if f.Info == nil {
			t.Errorf("file %s: Info should not be nil", f.Path)
			continue
		}

		// Verify it's not a directory
		if f.Info.IsDir() {
			t.Errorf("file %s: should not be a directory", f.Path)
		}

		// Verify size > 0 (our test files have content)
		if f.Info.Size() <= 0 {
			t.Errorf("file %s: expected positive size", f.Path)
		}

		// Verify mod time is reasonable (not zero)
		if f.Info.ModTime().IsZero() {
			t.Errorf("file %s: mod time should not be zero", f.Path)
		}

		// Verify mod time is recent (within last minute)
		if time.Since(f.Info.ModTime()) > time.Minute {
			t.Errorf("file %s: mod time too old", f.Path)
		}
	}
}

func TestScanner_HiddenDirectoriesSkipped(t *testing.T) {
	tmpDir := setupTestVault(t)
	defer os.RemoveAll(tmpDir)

	scanner := NewScanner(tmpDir, nil)
	ctx := context.Background()

	files, err := scanner.Scan(ctx)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	for _, f := range files {
		// Should not find any files in .hidden/
		if filepath.Dir(f.Path) == ".hidden" || filepath.Base(filepath.Dir(f.Path)) == ".hidden" {
			t.Errorf("should not find files in hidden directory: %s", f.Path)
		}
	}
}

func TestScanner_ListDirectories_ContextCancellation(t *testing.T) {
	tmpDir := setupTestVault(t)
	defer os.RemoveAll(tmpDir)

	scanner := NewScanner(tmpDir, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := scanner.ListDirectories(ctx)
	if err == nil {
		t.Error("expected context cancellation error")
	}
}
