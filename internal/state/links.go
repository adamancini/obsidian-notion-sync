package state

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
)

// LinkRegistry manages the mapping between Obsidian wiki-links and Notion page IDs.
type LinkRegistry struct {
	db    *DB
	fuzzy *FuzzyMatcher
}

// ResolveResult contains extended resolution information.
type ResolveResult struct {
	PageID     string // Notion page ID (empty if unresolved)
	Path       string // Obsidian path (empty if unresolved)
	Heading    string // Heading anchor (from [[Page#Heading]])
	BlockRef   string // Block reference (from [[Page^block-id]])
	Found      bool   // Whether the page was resolved
	FuzzyMatch bool   // Whether this was a fuzzy match
	Distance   int    // Edit distance for fuzzy matches
}

// LinkSuggestion represents a possible match for an unresolved link.
type LinkSuggestion struct {
	Target      string       // Original unresolved target
	Suggestions []MatchResult
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
	return &LinkRegistry{
		db:    db,
		fuzzy: NewFuzzyMatcher(),
	}
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

// parseTarget extracts the page name, heading anchor, and block reference from a target.
// Examples:
//   - "Page" -> ("Page", "", "")
//   - "Page#Heading" -> ("Page", "Heading", "")
//   - "Page^block-id" -> ("Page", "", "block-id")
//   - "Page#Heading^block-id" -> ("Page", "Heading", "block-id")
func parseTarget(target string) (page, heading, blockRef string) {
	// Extract block reference first (after ^).
	if idx := strings.Index(target, "^"); idx != -1 {
		blockRef = target[idx+1:]
		target = target[:idx]
	}

	// Extract heading (after #).
	if idx := strings.Index(target, "#"); idx != -1 {
		heading = target[idx+1:]
		target = target[:idx]
	}

	// Remove .md extension if present (after extracting anchors).
	page = strings.TrimSuffix(target, ".md")
	return
}

// ResolveExtended performs extended resolution including heading/block info and fuzzy matching.
func (r *LinkRegistry) ResolveExtended(target string, enableFuzzy bool) *ResolveResult {
	page, heading, blockRef := parseTarget(target)

	result := &ResolveResult{
		Heading:  heading,
		BlockRef: blockRef,
	}

	// First try exact resolution.
	if pageID, found := r.Resolve(page); found {
		result.PageID = pageID
		result.Found = true
		if path, ok := r.LookupPath(pageID); ok {
			result.Path = path
		}
		return result
	}

	// Try fuzzy matching if enabled.
	if enableFuzzy {
		candidates, err := r.getAllSyncedPaths()
		if err == nil && len(candidates) > 0 {
			matches := r.fuzzy.FindBestMatches(page, candidates, 1)
			if len(matches) > 0 && matches[0].Score >= MatchFuzzy {
				result.PageID = matches[0].PageID
				result.Path = matches[0].Path
				result.Found = true
				result.FuzzyMatch = true
				result.Distance = matches[0].Distance
				return result
			}
		}
	}

	return result
}

// getAllSyncedPaths returns all synced paths with their Notion page IDs.
func (r *LinkRegistry) getAllSyncedPaths() ([]MatchResult, error) {
	rows, err := r.db.conn.Query(`
		SELECT obsidian_path, notion_page_id
		FROM sync_state
		WHERE notion_page_id IS NOT NULL AND notion_page_id != ''
	`)
	if err != nil {
		return nil, fmt.Errorf("query sync_state: %w", err)
	}
	defer rows.Close()

	var results []MatchResult
	for rows.Next() {
		var path, pageID string
		if err := rows.Scan(&path, &pageID); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		results = append(results, MatchResult{
			Path:   path,
			PageID: pageID,
		})
	}
	return results, rows.Err()
}

// GetSuggestionsForUnresolved returns fuzzy match suggestions for all unresolved links.
func (r *LinkRegistry) GetSuggestionsForUnresolved(maxSuggestions int) ([]LinkSuggestion, error) {
	unresolved, err := r.GetUnresolvedLinks()
	if err != nil {
		return nil, err
	}

	candidates, err := r.getAllSyncedPaths()
	if err != nil {
		return nil, err
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	var suggestions []LinkSuggestion
	for _, link := range unresolved {
		page, _, _ := parseTarget(link.TargetName)
		matches := r.fuzzy.FindBestMatches(page, candidates, maxSuggestions)

		if len(matches) > 0 {
			suggestions = append(suggestions, LinkSuggestion{
				Target:      link.TargetName,
				Suggestions: matches,
			})
		}
	}

	return suggestions, nil
}

// RepairLinks attempts to resolve unresolved links using fuzzy matching.
// Returns the number of links repaired and any error.
func (r *LinkRegistry) RepairLinks(dryRun bool) ([]RepairResult, error) {
	unresolved, err := r.GetUnresolvedLinks()
	if err != nil {
		return nil, fmt.Errorf("get unresolved: %w", err)
	}

	candidates, err := r.getAllSyncedPaths()
	if err != nil {
		return nil, fmt.Errorf("get candidates: %w", err)
	}

	var results []RepairResult
	for _, link := range unresolved {
		page, _, _ := parseTarget(link.TargetName)
		matches := r.fuzzy.FindBestMatches(page, candidates, 1)

		if len(matches) > 0 && matches[0].Score >= MatchFuzzy {
			match := matches[0]
			result := RepairResult{
				SourcePath:   link.SourcePath,
				TargetName:   link.TargetName,
				MatchedPath:  match.Path,
				MatchedID:    match.PageID,
				Score:        match.Score,
				Distance:     match.Distance,
				WouldRepair:  true,
				WasRepaired:  false,
			}

			if !dryRun {
				// Update the link.
				_, err := r.db.conn.Exec(`
					UPDATE links
					SET target_path = ?, notion_page_id = ?, resolved = 1
					WHERE id = ?
				`, match.Path, match.PageID, link.ID)
				if err != nil {
					result.Error = err.Error()
				} else {
					result.WasRepaired = true
				}
			}

			results = append(results, result)
		}
	}

	return results, nil
}

// RepairResult describes the outcome of repairing a single link.
type RepairResult struct {
	SourcePath  string     // Path containing the link
	TargetName  string     // Original target text
	MatchedPath string     // Path of the matched page
	MatchedID   string     // Notion page ID of the matched page
	Score       MatchScore // Match quality
	Distance    int        // Edit distance
	WouldRepair bool       // Whether this link would be repaired
	WasRepaired bool       // Whether the link was actually repaired
	Error       string     // Error message if repair failed
}

// ResolveAllWithFuzzy attempts to resolve all unresolved links, using fuzzy matching as fallback.
// Returns counts of exact and fuzzy matches.
func (r *LinkRegistry) ResolveAllWithFuzzy(enableFuzzy bool) (exact, fuzzy int, err error) {
	unresolved, err := r.GetUnresolvedLinks()
	if err != nil {
		return 0, 0, err
	}

	var candidates []MatchResult
	if enableFuzzy {
		candidates, err = r.getAllSyncedPaths()
		if err != nil {
			return 0, 0, err
		}
	}

	for _, link := range unresolved {
		page, _, _ := parseTarget(link.TargetName)

		// Try exact match first.
		pageID, found := r.Resolve(page)
		if found {
			if _, err := r.db.conn.Exec(`
				UPDATE links
				SET notion_page_id = ?, resolved = 1
				WHERE id = ?
			`, pageID, link.ID); err != nil {
				return exact, fuzzy, fmt.Errorf("update link: %w", err)
			}
			exact++
			continue
		}

		// Try fuzzy match.
		if enableFuzzy && len(candidates) > 0 {
			matches := r.fuzzy.FindBestMatches(page, candidates, 1)
			if len(matches) > 0 && matches[0].Score >= MatchFuzzy {
				match := matches[0]
				if _, err := r.db.conn.Exec(`
					UPDATE links
					SET target_path = ?, notion_page_id = ?, resolved = 1
					WHERE id = ?
				`, match.Path, match.PageID, link.ID); err != nil {
					return exact, fuzzy, fmt.Errorf("update link: %w", err)
				}
				fuzzy++
			}
		}
	}

	return exact, fuzzy, nil
}

// GetLinkStats returns statistics about link resolution.
type LinkStats struct {
	Total      int
	Resolved   int
	Unresolved int
	BySource   map[string]int // Count of unresolved links per source file
}

// GetStats returns link resolution statistics.
func (r *LinkRegistry) GetStats() (*LinkStats, error) {
	stats := &LinkStats{
		BySource: make(map[string]int),
	}

	// Get total counts.
	err := r.db.conn.QueryRow(`SELECT COUNT(*) FROM links`).Scan(&stats.Total)
	if err != nil {
		return nil, fmt.Errorf("count total: %w", err)
	}

	err = r.db.conn.QueryRow(`SELECT COUNT(*) FROM links WHERE resolved = 1`).Scan(&stats.Resolved)
	if err != nil {
		return nil, fmt.Errorf("count resolved: %w", err)
	}

	stats.Unresolved = stats.Total - stats.Resolved

	// Get unresolved by source.
	rows, err := r.db.conn.Query(`
		SELECT source_path, COUNT(*) as cnt
		FROM links
		WHERE resolved = 0
		GROUP BY source_path
		ORDER BY cnt DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query by source: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var path string
		var count int
		if err := rows.Scan(&path, &count); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		stats.BySource[path] = count
	}

	return stats, rows.Err()
}
