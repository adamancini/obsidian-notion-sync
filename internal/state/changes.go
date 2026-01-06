package state

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ChangeType represents the type of change detected.
type ChangeType string

const (
	ChangeCreated  ChangeType = "created"
	ChangeModified ChangeType = "modified"
	ChangeDeleted  ChangeType = "deleted"
	ChangeRenamed  ChangeType = "renamed"
	ChangeConflict ChangeType = "conflict"
)

// Direction indicates whether a change should be pushed or pulled.
type Direction string

const (
	DirectionPush Direction = "push"
	DirectionPull Direction = "pull"
	DirectionBoth Direction = "both" // Conflict
)

// Change represents a detected change in the sync state.
type Change struct {
	Path        string
	OldPath     string // For renames: the previous path
	Type        ChangeType
	Direction   Direction
	LocalHash   string
	RemoteHash  string
	LocalMtime  time.Time
	RemoteMtime time.Time
	State       *SyncState // Associated sync state (for renames/deletions)

	// FrontmatterOnly indicates only metadata changed (not body content).
	// This enables property-only updates in Notion without re-syncing content.
	FrontmatterOnly bool
	LocalHashes     ContentHashes // Full hash breakdown for local content
}

// ChangeDetector detects changes between the local vault and sync state.
type ChangeDetector struct {
	db        *DB
	vaultPath string
}

// NewChangeDetector creates a new ChangeDetector.
func NewChangeDetector(db *DB, vaultPath string) *ChangeDetector {
	return &ChangeDetector{
		db:        db,
		vaultPath: vaultPath,
	}
}

// DetectChanges scans the vault and compares with stored sync state.
func (d *ChangeDetector) DetectChanges(ctx context.Context) ([]Change, error) {
	var changes []Change

	// 1. Scan local vault.
	localFiles, err := d.scanVault()
	if err != nil {
		return nil, fmt.Errorf("scan vault: %w", err)
	}

	// 2. Get all sync states.
	states, err := d.db.ListStates("")
	if err != nil {
		return nil, fmt.Errorf("list states: %w", err)
	}

	// Build map for quick lookup.
	stateMap := make(map[string]*SyncState)
	for _, state := range states {
		stateMap[state.ObsidianPath] = state
	}

	// Track new files and their hashes for rename detection.
	newFiles := make(map[string]fileWithHash) // path -> hash
	deletedStates := make(map[string]*SyncState) // path -> state

	// 3. Check each local file.
	for path, info := range localFiles {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		state, exists := stateMap[path]

		if !exists {
			// New local file - compute hash for rename detection.
			content, err := os.ReadFile(filepath.Join(d.vaultPath, path))
			if err != nil {
				continue // Skip files we can't read.
			}
			localHashes := HashContent(content)
			newFiles[path] = fileWithHash{
				info:   info,
				hash:   localHashes.FullHash,
				hashes: localHashes,
			}
			continue
		}

		// File exists in state - check for modifications.
		content, err := os.ReadFile(filepath.Join(d.vaultPath, path))
		if err != nil {
			continue // Skip files we can't read.
		}

		localHashes := HashContent(content)
		stateHashes := HashesFromState(state)

		// Check if content has changed using normalized comparison.
		if HasContentChanged(stateHashes, localHashes) {
			// Determine if it's a frontmatter-only change.
			frontmatterOnly := HasFrontmatterChanged(stateHashes, localHashes) &&
				!HasBodyChanged(stateHashes, localHashes)

			// Local modification detected.
			change := Change{
				Path:            path,
				Type:            ChangeModified,
				Direction:       DirectionPush,
				LocalHash:       localHashes.FullHash,
				LocalMtime:      info.ModTime(),
				State:           state,
				FrontmatterOnly: frontmatterOnly,
				LocalHashes:     localHashes,
			}

			// Check if remote was also modified (conflict).
			// Note: Full remote change detection is handled by RemoteChangeDetector.
			// This local-only detector uses stored state to detect previously-flagged conflicts.
			// For integrated remote checking, use RemoteChangeDetector.DetectAllChanges().
			if state.Status == "conflict" {
				change.Type = ChangeConflict
				change.Direction = DirectionBoth
			}

			changes = append(changes, change)
		}

		// Mark as seen.
		delete(stateMap, path)
	}

	// 4. Collect deleted files (in state but not in vault).
	for path, state := range stateMap {
		if state.NotionPageID != "" && state.Status == "synced" {
			deletedStates[path] = state
		}
	}

	// 5. Detect renames by matching content hashes.
	// A rename is when a deleted file's hash matches a new file's hash.
	renamedPaths := make(map[string]bool) // Track which new paths are renames

	for deletedPath, deletedState := range deletedStates {
		for newPath, newFile := range newFiles {
			if deletedState.ContentHash == newFile.hash {
				// Found a rename: content hash matches.
				changes = append(changes, Change{
					Path:       newPath,
					OldPath:    deletedPath,
					Type:       ChangeRenamed,
					Direction:  DirectionPush,
					LocalHash:  newFile.hash,
					LocalMtime: newFile.info.ModTime(),
					State:      deletedState,
				})
				// Mark both as handled.
				renamedPaths[newPath] = true
				delete(deletedStates, deletedPath)
				break // One deleted file can only be renamed to one new file.
			}
		}
	}

	// 6. Add remaining new files (not renames) as created.
	for path, fh := range newFiles {
		if !renamedPaths[path] {
			changes = append(changes, Change{
				Path:        path,
				Type:        ChangeCreated,
				Direction:   DirectionPush,
				LocalHash:   fh.hash,
				LocalMtime:  fh.info.ModTime(),
				LocalHashes: fh.hashes,
			})
		}
	}

	// 7. Add remaining deleted files (not renames) as deleted.
	for path, state := range deletedStates {
		changes = append(changes, Change{
			Path:       path,
			Type:       ChangeDeleted,
			Direction:  DirectionPush,
			RemoteHash: state.ContentHash,
			State:      state,
		})
	}

	return changes, nil
}

// fileWithHash holds file info and its content hash.
type fileWithHash struct {
	info   fs.FileInfo
	hash   string        // FullHash for rename detection
	hashes ContentHashes // Complete hash breakdown
}

// DetectRemoteChanges checks Notion for pages modified since last sync.
// This requires the Notion client and is called separately.
func (d *ChangeDetector) DetectRemoteChanges(ctx context.Context, getRemoteInfo func(pageID string) (hash string, mtime time.Time, err error)) ([]Change, error) {
	var changes []Change

	// Get all synced states with Notion page IDs.
	states, err := d.db.ListStates("synced")
	if err != nil {
		return nil, fmt.Errorf("list states: %w", err)
	}

	for _, state := range states {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if state.NotionPageID == "" {
			continue
		}

		// Check remote state.
		remoteHash, remoteMtime, err := getRemoteInfo(state.NotionPageID)
		if err != nil {
			// Page might be deleted or inaccessible.
			continue
		}

		// Check if remote was modified after last sync.
		if remoteMtime.After(state.LastSync) && remoteHash != state.ContentHash {
			// Check if local was also modified.
			localContent, err := os.ReadFile(filepath.Join(d.vaultPath, state.ObsidianPath))
			if err != nil {
				// Local file might be deleted.
				changes = append(changes, Change{
					Path:        state.ObsidianPath,
					Type:        ChangeModified,
					Direction:   DirectionPull,
					RemoteHash:  remoteHash,
					RemoteMtime: remoteMtime,
				})
				continue
			}

			localHashes := HashContent(localContent)
			stateHashes := HashesFromState(state)

			if HasContentChanged(stateHashes, localHashes) {
				// Both modified - conflict!
				changes = append(changes, Change{
					Path:        state.ObsidianPath,
					Type:        ChangeConflict,
					Direction:   DirectionBoth,
					LocalHash:   localHashes.FullHash,
					RemoteHash:  remoteHash,
					RemoteMtime: remoteMtime,
					LocalHashes: localHashes,
				})
			} else {
				// Only remote modified.
				changes = append(changes, Change{
					Path:        state.ObsidianPath,
					Type:        ChangeModified,
					Direction:   DirectionPull,
					RemoteHash:  remoteHash,
					RemoteMtime: remoteMtime,
				})
			}
		}
	}

	return changes, nil
}

// scanVault walks the vault directory and returns all markdown files.
func (d *ChangeDetector) scanVault() (map[string]fs.FileInfo, error) {
	files := make(map[string]fs.FileInfo)

	err := filepath.WalkDir(d.vaultPath, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden directories (like .obsidian).
		if entry.IsDir() && strings.HasPrefix(entry.Name(), ".") {
			return filepath.SkipDir
		}

		// Only process markdown files.
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			relPath, err := filepath.Rel(d.vaultPath, path)
			if err != nil {
				return err
			}

			info, err := entry.Info()
			if err != nil {
				return err
			}

			files[relPath] = info
		}

		return nil
	})

	return files, err
}

// HashFile computes a normalized SHA-256 hash of a file.
// Returns the FullHash which considers both frontmatter and body content.
func HashFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return HashContent(content).FullHash, nil
}

// HashFileDetailed computes all hashes for a file.
// Returns ContentHashes with separate frontmatter and body hashes.
func HashFileDetailed(path string) (ContentHashes, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return ContentHashes{}, err
	}
	return HashContent(content), nil
}
