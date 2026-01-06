package state

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
)

// LinkRegistry manages the mapping between Obsidian wiki-links and Notion page IDs.
type LinkRegistry struct {
	db *DB
}

// LinkEntry represents a link from one note to another.
type LinkEntry struct {
	ID           int64
	SourcePath   string // Path of the note containing the link.
	TargetName   string // The [[target]] text.
	TargetPath   string // Resolved Obsidian path (if found).
	NotionPageID string // Resolved Notion page ID (if synced).
	Resolved     bool
}

// NewLinkRegistry creates a new LinkRegistry backed by the given database.
func NewLinkRegistry(db *DB) *LinkRegistry {
	return &LinkRegistry{db: db}
}

// RegisterLink records a wiki-link from a source note.
func (r *LinkRegistry) RegisterLink(sourcePath, targetName string) error {
	_, err := r.db.conn.Exec(`
		INSERT INTO links (source_path, target_name, resolved)
		VALUES (?, ?, 0)
		ON CONFLICT DO NOTHING
	`, sourcePath, targetName)
	return err
}

// RegisterLinks records multiple wiki-links from a source note.
func (r *LinkRegistry) RegisterLinks(sourcePath string, targetNames []string) error {
	tx, err := r.db.conn.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO links (source_path, target_name, resolved)
		VALUES (?, ?, 0)
		ON CONFLICT DO NOTHING
	`)
	if err != nil {
		return fmt.Errorf("prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, target := range targetNames {
		if _, err := stmt.Exec(sourcePath, target); err != nil {
			return fmt.Errorf("insert link: %w", err)
		}
	}

	return tx.Commit()
}

// ClearLinksFrom removes all links originating from a source path.
func (r *LinkRegistry) ClearLinksFrom(sourcePath string) error {
	_, err := r.db.conn.Exec(`DELETE FROM links WHERE source_path = ?`, sourcePath)
	return err
}

// UpdateSourcePath updates all links from an old source path to a new source path.
// Used when handling file renames.
func (r *LinkRegistry) UpdateSourcePath(oldPath, newPath string) error {
	_, err := r.db.conn.Exec(`UPDATE links SET source_path = ? WHERE source_path = ?`, newPath, oldPath)
	return err
}

// Resolve looks up a wiki-link target and returns the Notion page ID.
// This implements the transformer.LinkResolver interface.
func (r *LinkRegistry) Resolve(target string) (notionPageID string, found bool) {
	// Resolution order:
	// 1. Exact path match (if target contains /)
	// 2. Name match in sync_state
	// 3. Alias match (from frontmatter)

	// Try exact path match first.
	if strings.Contains(target, "/") {
		var pageID sql.NullString
		err := r.db.conn.QueryRow(`
			SELECT notion_page_id FROM sync_state
			WHERE obsidian_path = ? AND notion_page_id IS NOT NULL
		`, normalizeTarget(target)).Scan(&pageID)
		if err == nil && pageID.Valid {
			return pageID.String, true
		}
	}

	// Try name match (file name without extension).
	var pageID sql.NullString
	normalizedTarget := normalizeTarget(target)

	// Match by file name (last component of path, without .md).
	err := r.db.conn.QueryRow(`
		SELECT notion_page_id FROM sync_state
		WHERE (
			obsidian_path = ?
			OR obsidian_path LIKE '%/' || ? || '.md'
			OR obsidian_path = ? || '.md'
		)
		AND notion_page_id IS NOT NULL
		LIMIT 1
	`, normalizedTarget, normalizedTarget, normalizedTarget).Scan(&pageID)
	if err == nil && pageID.Valid {
		return pageID.String, true
	}

	return "", false
}

// LookupPath returns the Obsidian path for a Notion page ID.
// This implements the transformer.PathLookup interface.
func (r *LinkRegistry) LookupPath(notionPageID string) (obsidianPath string, found bool) {
	var path string
	err := r.db.conn.QueryRow(`
		SELECT obsidian_path FROM sync_state
		WHERE notion_page_id = ?
	`, notionPageID).Scan(&path)
	if err != nil {
		return "", false
	}
	return path, true
}

// GetUnresolvedLinks returns all links that haven't been resolved yet.
func (r *LinkRegistry) GetUnresolvedLinks() ([]*LinkEntry, error) {
	rows, err := r.db.conn.Query(`
		SELECT id, source_path, target_name, target_path, notion_page_id, resolved
		FROM links
		WHERE resolved = 0
	`)
	if err != nil {
		return nil, fmt.Errorf("query unresolved: %w", err)
	}
	defer rows.Close()

	return scanLinks(rows)
}

// GetLinksFrom returns all links originating from a source path.
func (r *LinkRegistry) GetLinksFrom(sourcePath string) ([]*LinkEntry, error) {
	rows, err := r.db.conn.Query(`
		SELECT id, source_path, target_name, target_path, notion_page_id, resolved
		FROM links
		WHERE source_path = ?
	`, sourcePath)
	if err != nil {
		return nil, fmt.Errorf("query links: %w", err)
	}
	defer rows.Close()

	return scanLinks(rows)
}

// GetBacklinks returns all links pointing to a target.
func (r *LinkRegistry) GetBacklinks(targetPath string) ([]*LinkEntry, error) {
	// Match by exact path or by file name.
	targetName := filepath.Base(strings.TrimSuffix(targetPath, ".md"))

	rows, err := r.db.conn.Query(`
		SELECT id, source_path, target_name, target_path, notion_page_id, resolved
		FROM links
		WHERE target_path = ? OR target_name = ?
	`, targetPath, targetName)
	if err != nil {
		return nil, fmt.Errorf("query backlinks: %w", err)
	}
	defer rows.Close()

	return scanLinks(rows)
}

// ResolveAll attempts to resolve all unresolved links.
// Returns the number of newly resolved links.
func (r *LinkRegistry) ResolveAll() (int, error) {
	// Get all unresolved links.
	unresolved, err := r.GetUnresolvedLinks()
	if err != nil {
		return 0, err
	}

	resolved := 0
	for _, link := range unresolved {
		pageID, found := r.Resolve(link.TargetName)
		if found {
			// Update the link entry.
			_, err := r.db.conn.Exec(`
				UPDATE links
				SET notion_page_id = ?, resolved = 1
				WHERE id = ?
			`, pageID, link.ID)
			if err != nil {
				return resolved, fmt.Errorf("update link: %w", err)
			}
			resolved++
		}
	}

	return resolved, nil
}

// scanLinks reads LinkEntry records from rows.
func scanLinks(rows *sql.Rows) ([]*LinkEntry, error) {
	var links []*LinkEntry
	for rows.Next() {
		link := &LinkEntry{}
		var targetPath, notionPageID sql.NullString

		err := rows.Scan(
			&link.ID, &link.SourcePath, &link.TargetName,
			&targetPath, &notionPageID, &link.Resolved,
		)
		if err != nil {
			return nil, fmt.Errorf("scan link: %w", err)
		}

		if targetPath.Valid {
			link.TargetPath = targetPath.String
		}
		if notionPageID.Valid {
			link.NotionPageID = notionPageID.String
		}

		links = append(links, link)
	}

	return links, rows.Err()
}

// normalizeTarget normalizes a wiki-link target for comparison.
func normalizeTarget(target string) string {
	// Remove .md extension if present.
	target = strings.TrimSuffix(target, ".md")

	// Remove heading/block references.
	if idx := strings.Index(target, "#"); idx != -1 {
		target = target[:idx]
	}
	if idx := strings.Index(target, "^"); idx != -1 {
		target = target[:idx]
	}

	return target
}
