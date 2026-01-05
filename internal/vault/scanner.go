// Package vault provides operations for scanning and managing Obsidian vaults.
package vault

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Scanner walks an Obsidian vault and discovers markdown files.
type Scanner struct {
	root    string
	ignore  []string
}

// File represents a markdown file in the vault.
type File struct {
	// Path is the relative path from vault root.
	Path string

	// AbsPath is the absolute filesystem path.
	AbsPath string

	// Info contains file metadata.
	Info fs.FileInfo
}

// NewScanner creates a new vault Scanner.
func NewScanner(root string, ignore []string) *Scanner {
	return &Scanner{
		root:   root,
		ignore: ignore,
	}
}

// Scan walks the vault and returns all markdown files.
func (s *Scanner) Scan(ctx context.Context) ([]File, error) {
	var files []File

	err := filepath.WalkDir(s.root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Check for context cancellation.
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Skip hidden directories.
		if entry.IsDir() && strings.HasPrefix(entry.Name(), ".") {
			return filepath.SkipDir
		}

		// Skip non-markdown files.
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			return nil
		}

		// Get relative path.
		relPath, err := filepath.Rel(s.root, path)
		if err != nil {
			return err
		}

		// Check ignore patterns.
		if s.shouldIgnore(relPath) {
			return nil
		}

		// Get file info.
		info, err := entry.Info()
		if err != nil {
			return err
		}

		files = append(files, File{
			Path:    relPath,
			AbsPath: path,
			Info:    info,
		})

		return nil
	})

	return files, err
}

// ScanDir scans a specific directory within the vault.
func (s *Scanner) ScanDir(ctx context.Context, dir string) ([]File, error) {
	fullPath := filepath.Join(s.root, dir)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return nil, nil
	}

	var files []File

	err := filepath.WalkDir(fullPath, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if entry.IsDir() && strings.HasPrefix(entry.Name(), ".") {
			return filepath.SkipDir
		}

		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			return nil
		}

		relPath, err := filepath.Rel(s.root, path)
		if err != nil {
			return err
		}

		if s.shouldIgnore(relPath) {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}

		files = append(files, File{
			Path:    relPath,
			AbsPath: path,
			Info:    info,
		})

		return nil
	})

	return files, err
}

// ScanGlob returns files matching a glob pattern.
func (s *Scanner) ScanGlob(ctx context.Context, pattern string) ([]File, error) {
	// First scan all files.
	allFiles, err := s.Scan(ctx)
	if err != nil {
		return nil, err
	}

	// Filter by pattern.
	var matched []File
	for _, f := range allFiles {
		ok, err := filepath.Match(pattern, f.Path)
		if err != nil {
			return nil, err
		}
		if ok {
			matched = append(matched, f)
		}
	}

	return matched, nil
}

// GetFile retrieves a single file by relative path.
func (s *Scanner) GetFile(relPath string) (*File, error) {
	absPath := filepath.Join(s.root, relPath)

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}

	return &File{
		Path:    relPath,
		AbsPath: absPath,
		Info:    info,
	}, nil
}

// ReadFile reads the content of a file.
func (s *Scanner) ReadFile(relPath string) ([]byte, error) {
	absPath := filepath.Join(s.root, relPath)
	return os.ReadFile(absPath)
}

// WriteFile writes content to a file, creating directories as needed.
func (s *Scanner) WriteFile(relPath string, content []byte) error {
	absPath := filepath.Join(s.root, relPath)

	// Ensure directory exists.
	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(absPath, content, 0644)
}

// DeleteFile removes a file from the vault.
func (s *Scanner) DeleteFile(relPath string) error {
	absPath := filepath.Join(s.root, relPath)
	return os.Remove(absPath)
}

// Exists checks if a file exists in the vault.
func (s *Scanner) Exists(relPath string) bool {
	absPath := filepath.Join(s.root, relPath)
	_, err := os.Stat(absPath)
	return err == nil
}

// Root returns the vault root path.
func (s *Scanner) Root() string {
	return s.root
}

// shouldIgnore checks if a path matches any ignore pattern.
func (s *Scanner) shouldIgnore(path string) bool {
	for _, pattern := range s.ignore {
		matched, _ := filepath.Match(pattern, path)
		if matched {
			return true
		}

		// Also try matching just the file name.
		matched, _ = filepath.Match(pattern, filepath.Base(path))
		if matched {
			return true
		}

		// Handle ** patterns (recursive).
		if strings.Contains(pattern, "**") {
			// Simple handling: check if pattern without ** matches.
			simplePattern := strings.ReplaceAll(pattern, "**", "*")
			matched, _ = filepath.Match(simplePattern, path)
			if matched {
				return true
			}
		}
	}
	return false
}

// ListDirectories returns all directories in the vault.
func (s *Scanner) ListDirectories(ctx context.Context) ([]string, error) {
	var dirs []string

	err := filepath.WalkDir(s.root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if !entry.IsDir() {
			return nil
		}

		// Skip hidden directories.
		if strings.HasPrefix(entry.Name(), ".") {
			return filepath.SkipDir
		}

		relPath, err := filepath.Rel(s.root, path)
		if err != nil {
			return err
		}

		if relPath != "." {
			dirs = append(dirs, relPath)
		}

		return nil
	})

	return dirs, err
}
