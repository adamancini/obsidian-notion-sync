package state

import (
	"encoding/json"
	"fmt"
	"time"
)

// ConflictInfo stores information about a sync conflict.
type ConflictInfo struct {
	Path           string    `json:"path"`
	LocalHash      string    `json:"local_hash"`
	RemoteHash     string    `json:"remote_hash"`
	LocalMtime     time.Time `json:"local_mtime"`
	RemoteMtime    time.Time `json:"remote_mtime"`
	DetectedAt     time.Time `json:"detected_at"`
	LocalContent   string    `json:"local_content,omitempty"`   // Snapshot at detection.
	RemoteContent  string    `json:"remote_content,omitempty"`  // Snapshot at detection.
}

// ConflictTracker manages conflict tracking and resolution.
type ConflictTracker struct {
	db *DB
}

// NewConflictTracker creates a new ConflictTracker.
func NewConflictTracker(db *DB) *ConflictTracker {
	return &ConflictTracker{db: db}
}

// RecordConflict records a new conflict for a path.
func (t *ConflictTracker) RecordConflict(info *ConflictInfo) error {
	// Update sync state to conflict status.
	state, err := t.db.GetState(info.Path)
	if err != nil {
		return fmt.Errorf("get state: %w", err)
	}
	if state == nil {
		return fmt.Errorf("no sync state for path: %s", info.Path)
	}

	state.Status = "conflict"
	if err := t.db.SetState(state); err != nil {
		return fmt.Errorf("set state: %w", err)
	}

	// Record in history.
	details, _ := json.Marshal(info)
	if err := t.recordHistory(info.Path, "conflict", string(details)); err != nil {
		return fmt.Errorf("record history: %w", err)
	}

	return nil
}

// ResolveConflict marks a conflict as resolved.
func (t *ConflictTracker) ResolveConflict(path string, resolution string, contentHash string) error {
	// Update sync state.
	state, err := t.db.GetState(path)
	if err != nil {
		return fmt.Errorf("get state: %w", err)
	}
	if state == nil {
		return fmt.Errorf("no sync state for path: %s", path)
	}

	state.Status = "synced"
	state.ContentHash = contentHash
	state.LastSync = time.Now()
	state.SyncDirection = resolution

	if err := t.db.SetState(state); err != nil {
		return fmt.Errorf("set state: %w", err)
	}

	// Record resolution in history.
	details := map[string]string{
		"resolution":   resolution,
		"content_hash": contentHash,
	}
	detailsJSON, _ := json.Marshal(details)
	if err := t.recordHistory(path, "conflict_resolved", string(detailsJSON)); err != nil {
		return fmt.Errorf("record history: %w", err)
	}

	return nil
}

// GetConflicts returns all paths with conflict status.
func (t *ConflictTracker) GetConflicts() ([]*SyncState, error) {
	return t.db.ListStates("conflict")
}

// GetConflictInfo retrieves detailed conflict information for a path.
func (t *ConflictTracker) GetConflictInfo(path string) (*ConflictInfo, error) {
	// Get the most recent conflict record from history.
	row := t.db.conn.QueryRow(`
		SELECT details FROM sync_history
		WHERE obsidian_path = ? AND action = 'conflict'
		ORDER BY timestamp DESC
		LIMIT 1
	`, path)

	var detailsJSON string
	if err := row.Scan(&detailsJSON); err != nil {
		return nil, fmt.Errorf("query history: %w", err)
	}

	var info ConflictInfo
	if err := json.Unmarshal([]byte(detailsJSON), &info); err != nil {
		return nil, fmt.Errorf("unmarshal conflict info: %w", err)
	}

	return &info, nil
}

// HasConflict checks if a path has an unresolved conflict.
func (t *ConflictTracker) HasConflict(path string) (bool, error) {
	state, err := t.db.GetState(path)
	if err != nil {
		return false, err
	}
	if state == nil {
		return false, nil
	}
	return state.Status == "conflict", nil
}

// recordHistory adds an entry to the sync history.
func (t *ConflictTracker) recordHistory(path, action, details string) error {
	_, err := t.db.conn.Exec(`
		INSERT INTO sync_history (obsidian_path, action, timestamp, details)
		VALUES (?, ?, ?, ?)
	`, path, action, time.Now().Unix(), details)
	return err
}

// GetHistory retrieves sync history for a path.
func (t *ConflictTracker) GetHistory(path string, limit int) ([]HistoryEntry, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := t.db.conn.Query(`
		SELECT id, obsidian_path, action, timestamp, content_hash, details
		FROM sync_history
		WHERE obsidian_path = ?
		ORDER BY timestamp DESC
		LIMIT ?
	`, path, limit)
	if err != nil {
		return nil, fmt.Errorf("query history: %w", err)
	}
	defer rows.Close()

	var entries []HistoryEntry
	for rows.Next() {
		var entry HistoryEntry
		var timestamp int64
		var contentHash, details *string

		err := rows.Scan(&entry.ID, &entry.Path, &entry.Action, &timestamp, &contentHash, &details)
		if err != nil {
			return nil, fmt.Errorf("scan history: %w", err)
		}

		entry.Timestamp = time.Unix(timestamp, 0)
		if contentHash != nil {
			entry.ContentHash = *contentHash
		}
		if details != nil {
			entry.Details = *details
		}

		entries = append(entries, entry)
	}

	return entries, rows.Err()
}

// HistoryEntry represents a sync history record.
type HistoryEntry struct {
	ID          int64
	Path        string
	Action      string
	Timestamp   time.Time
	ContentHash string
	Details     string
}

// ClearHistory removes old history entries.
func (t *ConflictTracker) ClearHistory(olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan).Unix()

	result, err := t.db.conn.Exec(`
		DELETE FROM sync_history WHERE timestamp < ?
	`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete history: %w", err)
	}

	return result.RowsAffected()
}
