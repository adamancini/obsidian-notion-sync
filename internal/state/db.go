// Package state provides SQLite-based state management for tracking sync
// status between Obsidian and Notion.
package state

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// DB wraps the SQLite database connection for sync state management.
type DB struct {
	conn *sql.DB
	path string
}

// SyncState represents the sync state for a single note.
type SyncState struct {
	ID              int64
	ObsidianPath    string
	NotionPageID    string
	NotionParentID  string
	ContentHash     string
	FrontmatterHash string
	ObsidianMtime   time.Time
	NotionMtime     time.Time
	LastSync        time.Time
	SyncDirection   string
	Status          string // "synced", "pending", "conflict", "error"
}

// Open opens or creates a sync state database at the given path.
func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite3", path+"?_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	db := &DB{
		conn: conn,
		path: path,
	}

	if err := db.initSchema(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}

	return db, nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.conn.Close()
}

// initSchema creates the database schema if it doesn't exist.
func (db *DB) initSchema() error {
	schema := `
	-- Core sync state
	CREATE TABLE IF NOT EXISTS sync_state (
		id INTEGER PRIMARY KEY,
		obsidian_path TEXT UNIQUE NOT NULL,
		notion_page_id TEXT,
		notion_parent_id TEXT,
		content_hash TEXT NOT NULL,
		frontmatter_hash TEXT,
		obsidian_mtime INTEGER,
		notion_mtime INTEGER,
		last_sync INTEGER,
		sync_direction TEXT,
		status TEXT DEFAULT 'pending'
	);

	-- Link resolution cache
	CREATE TABLE IF NOT EXISTS links (
		id INTEGER PRIMARY KEY,
		source_path TEXT NOT NULL,
		target_name TEXT NOT NULL,
		target_path TEXT,
		notion_page_id TEXT,
		resolved INTEGER DEFAULT 0
	);

	-- Sync history for conflict resolution
	CREATE TABLE IF NOT EXISTS sync_history (
		id INTEGER PRIMARY KEY,
		obsidian_path TEXT NOT NULL,
		action TEXT NOT NULL,
		timestamp INTEGER NOT NULL,
		content_hash TEXT,
		details TEXT
	);

	-- Configuration
	CREATE TABLE IF NOT EXISTS config (
		key TEXT PRIMARY KEY,
		value TEXT
	);

	-- Indexes
	CREATE INDEX IF NOT EXISTS idx_sync_state_status ON sync_state(status);
	CREATE INDEX IF NOT EXISTS idx_sync_state_notion_page ON sync_state(notion_page_id);
	CREATE INDEX IF NOT EXISTS idx_links_source ON links(source_path);
	CREATE INDEX IF NOT EXISTS idx_links_target ON links(target_name);
	CREATE INDEX IF NOT EXISTS idx_history_path ON sync_history(obsidian_path);
	`

	_, err := db.conn.Exec(schema)
	return err
}

// GetState retrieves the sync state for a given Obsidian path.
func (db *DB) GetState(path string) (*SyncState, error) {
	row := db.conn.QueryRow(`
		SELECT id, obsidian_path, notion_page_id, notion_parent_id,
		       content_hash, frontmatter_hash, obsidian_mtime, notion_mtime,
		       last_sync, sync_direction, status
		FROM sync_state
		WHERE obsidian_path = ?
	`, path)

	state := &SyncState{}
	var obsidianMtime, notionMtime, lastSync sql.NullInt64
	var notionPageID, notionParentID, frontmatterHash, syncDirection sql.NullString

	err := row.Scan(
		&state.ID, &state.ObsidianPath, &notionPageID, &notionParentID,
		&state.ContentHash, &frontmatterHash, &obsidianMtime, &notionMtime,
		&lastSync, &syncDirection, &state.Status,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan state: %w", err)
	}

	// Convert nullable fields.
	if notionPageID.Valid {
		state.NotionPageID = notionPageID.String
	}
	if notionParentID.Valid {
		state.NotionParentID = notionParentID.String
	}
	if frontmatterHash.Valid {
		state.FrontmatterHash = frontmatterHash.String
	}
	if syncDirection.Valid {
		state.SyncDirection = syncDirection.String
	}
	if obsidianMtime.Valid {
		state.ObsidianMtime = time.Unix(obsidianMtime.Int64, 0)
	}
	if notionMtime.Valid {
		state.NotionMtime = time.Unix(notionMtime.Int64, 0)
	}
	if lastSync.Valid {
		state.LastSync = time.Unix(lastSync.Int64, 0)
	}

	return state, nil
}

// SetState creates or updates the sync state for a path.
func (db *DB) SetState(state *SyncState) error {
	_, err := db.conn.Exec(`
		INSERT INTO sync_state (
			obsidian_path, notion_page_id, notion_parent_id,
			content_hash, frontmatter_hash, obsidian_mtime, notion_mtime,
			last_sync, sync_direction, status
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(obsidian_path) DO UPDATE SET
			notion_page_id = excluded.notion_page_id,
			notion_parent_id = excluded.notion_parent_id,
			content_hash = excluded.content_hash,
			frontmatter_hash = excluded.frontmatter_hash,
			obsidian_mtime = excluded.obsidian_mtime,
			notion_mtime = excluded.notion_mtime,
			last_sync = excluded.last_sync,
			sync_direction = excluded.sync_direction,
			status = excluded.status
	`,
		state.ObsidianPath, nullString(state.NotionPageID), nullString(state.NotionParentID),
		state.ContentHash, nullString(state.FrontmatterHash),
		nullTime(state.ObsidianMtime), nullTime(state.NotionMtime),
		nullTime(state.LastSync), nullString(state.SyncDirection), state.Status,
	)
	return err
}

// DeleteState removes the sync state for a path.
func (db *DB) DeleteState(path string) error {
	_, err := db.conn.Exec(`DELETE FROM sync_state WHERE obsidian_path = ?`, path)
	return err
}

// ListStates returns all sync states matching the given status filter.
// If status is empty, returns all states.
func (db *DB) ListStates(status string) ([]*SyncState, error) {
	var rows *sql.Rows
	var err error

	if status == "" {
		rows, err = db.conn.Query(`
			SELECT id, obsidian_path, notion_page_id, notion_parent_id,
			       content_hash, frontmatter_hash, obsidian_mtime, notion_mtime,
			       last_sync, sync_direction, status
			FROM sync_state
			ORDER BY obsidian_path
		`)
	} else {
		rows, err = db.conn.Query(`
			SELECT id, obsidian_path, notion_page_id, notion_parent_id,
			       content_hash, frontmatter_hash, obsidian_mtime, notion_mtime,
			       last_sync, sync_direction, status
			FROM sync_state
			WHERE status = ?
			ORDER BY obsidian_path
		`, status)
	}
	if err != nil {
		return nil, fmt.Errorf("query states: %w", err)
	}
	defer rows.Close()

	var states []*SyncState
	for rows.Next() {
		state := &SyncState{}
		var obsidianMtime, notionMtime, lastSync sql.NullInt64
		var notionPageID, notionParentID, frontmatterHash, syncDirection sql.NullString

		err := rows.Scan(
			&state.ID, &state.ObsidianPath, &notionPageID, &notionParentID,
			&state.ContentHash, &frontmatterHash, &obsidianMtime, &notionMtime,
			&lastSync, &syncDirection, &state.Status,
		)
		if err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		// Convert nullable fields.
		if notionPageID.Valid {
			state.NotionPageID = notionPageID.String
		}
		if notionParentID.Valid {
			state.NotionParentID = notionParentID.String
		}
		if frontmatterHash.Valid {
			state.FrontmatterHash = frontmatterHash.String
		}
		if syncDirection.Valid {
			state.SyncDirection = syncDirection.String
		}
		if obsidianMtime.Valid {
			state.ObsidianMtime = time.Unix(obsidianMtime.Int64, 0)
		}
		if notionMtime.Valid {
			state.NotionMtime = time.Unix(notionMtime.Int64, 0)
		}
		if lastSync.Valid {
			state.LastSync = time.Unix(lastSync.Int64, 0)
		}

		states = append(states, state)
	}

	return states, rows.Err()
}

// GetConfig retrieves a configuration value.
func (db *DB) GetConfig(key string) (string, error) {
	var value string
	err := db.conn.QueryRow(`SELECT value FROM config WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// SetConfig sets a configuration value.
func (db *DB) SetConfig(key, value string) error {
	_, err := db.conn.Exec(`
		INSERT INTO config (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, key, value)
	return err
}

// Helper functions.

func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nullTime(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return t.Unix()
}
