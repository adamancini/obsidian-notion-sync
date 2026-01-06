// Package state provides SQLite-based state management for tracking sync
// status between Obsidian and Notion.
package state

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// RemotePageInfo contains information about a remote Notion page.
type RemotePageInfo struct {
	PageID         string
	LastEditedTime time.Time
	Archived       bool
	Err            error // Non-nil if page could not be fetched (deleted, permission denied)
}

// RemoteChecker provides an interface for checking remote page status.
// This abstraction allows for different implementations (live API, cached, mock).
type RemoteChecker interface {
	// GetPageInfo retrieves metadata for a single page.
	GetPageInfo(ctx context.Context, pageID string) (*RemotePageInfo, error)

	// GetPagesInfoBatch retrieves metadata for multiple pages efficiently.
	// Returns a map of pageID -> info. Missing pages are included with Err set.
	GetPagesInfoBatch(ctx context.Context, pageIDs []string) (map[string]*RemotePageInfo, error)
}

// RemoteChangeDetector extends ChangeDetector with remote change detection capabilities.
type RemoteChangeDetector struct {
	*ChangeDetector
	remote RemoteChecker
	mu     sync.Mutex // Protects concurrent access during detection
}

// NewRemoteChangeDetector creates a ChangeDetector with remote checking capabilities.
func NewRemoteChangeDetector(db *DB, vaultPath string, remote RemoteChecker) *RemoteChangeDetector {
	return &RemoteChangeDetector{
		ChangeDetector: NewChangeDetector(db, vaultPath),
		remote:         remote,
	}
}

// DetectAllChanges performs comprehensive change detection including remote changes.
// It combines local change detection with remote timestamp comparison to identify:
// - Local-only changes (push)
// - Remote-only changes (pull)
// - Conflicts (both modified)
func (d *RemoteChangeDetector) DetectAllChanges(ctx context.Context) ([]Change, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Step 1: Detect local changes.
	localChanges, err := d.ChangeDetector.DetectChanges(ctx)
	if err != nil {
		return nil, fmt.Errorf("detect local changes: %w", err)
	}

	// Step 2: If no remote checker, return local changes only.
	if d.remote == nil {
		return localChanges, nil
	}

	// Step 3: Get all synced states with Notion page IDs for remote checking.
	states, err := d.db.ListStates("synced")
	if err != nil {
		return nil, fmt.Errorf("list synced states: %w", err)
	}

	// Collect page IDs to check.
	pageIDs := make([]string, 0, len(states))
	stateByPageID := make(map[string]*SyncState)
	for _, state := range states {
		if state.NotionPageID != "" {
			pageIDs = append(pageIDs, state.NotionPageID)
			stateByPageID[state.NotionPageID] = state
		}
	}

	if len(pageIDs) == 0 {
		return localChanges, nil
	}

	// Step 4: Batch fetch remote page info.
	remoteInfo, err := d.remote.GetPagesInfoBatch(ctx, pageIDs)
	if err != nil {
		// On remote fetch failure, return local changes with warning.
		// This provides graceful degradation when Notion is unavailable.
		return localChanges, nil
	}

	// Step 5: Build map of local changes by path for conflict detection.
	localChangesByPath := make(map[string]*Change)
	for i := range localChanges {
		localChangesByPath[localChanges[i].Path] = &localChanges[i]
	}

	// Step 6: Process remote info and merge with local changes.
	var additionalChanges []Change
	for pageID, info := range remoteInfo {
		state, ok := stateByPageID[pageID]
		if !ok {
			continue
		}

		// Skip if remote fetch failed for this page.
		if info.Err != nil {
			continue
		}

		// Check if page was archived remotely.
		if info.Archived {
			// Remote page was deleted/archived.
			if localChange, exists := localChangesByPath[state.ObsidianPath]; exists {
				// Local also has changes - conflict.
				localChange.Type = ChangeConflict
				localChange.Direction = DirectionBoth
				localChange.RemoteMtime = info.LastEditedTime
			} else {
				// Only remote deleted - this is a pull (delete local).
				additionalChanges = append(additionalChanges, Change{
					Path:        state.ObsidianPath,
					Type:        ChangeDeleted,
					Direction:   DirectionPull,
					RemoteMtime: info.LastEditedTime,
					State:       state,
				})
			}
			continue
		}

		// Check if remote was modified since last sync.
		// Note: We truncate to seconds because SQLite stores timestamps as Unix seconds.
		// Without truncation, sub-second differences would cause false positives.
		remoteMtimeSec := info.LastEditedTime.Truncate(time.Second)
		stateMtimeSec := state.NotionMtime.Truncate(time.Second)
		if remoteMtimeSec.After(stateMtimeSec) {
			// Remote has been modified.
			if localChange, exists := localChangesByPath[state.ObsidianPath]; exists {
				// Both local and remote modified - conflict!
				localChange.Type = ChangeConflict
				localChange.Direction = DirectionBoth
				localChange.RemoteMtime = info.LastEditedTime
			} else {
				// Check if local file still exists and is unchanged.
				localPath := filepath.Join(d.vaultPath, state.ObsidianPath)
				localContent, err := os.ReadFile(localPath)
				if err != nil {
					// Local file doesn't exist - remote only.
					additionalChanges = append(additionalChanges, Change{
						Path:        state.ObsidianPath,
						Type:        ChangeModified,
						Direction:   DirectionPull,
						RemoteMtime: info.LastEditedTime,
						State:       state,
					})
					continue
				}

				// Check if local matches stored state.
				localHashes := HashContent(localContent)
				stateHashes := HashesFromState(state)

				if HasContentChanged(stateHashes, localHashes) {
					// Local also changed (but wasn't in localChanges - edge case).
					additionalChanges = append(additionalChanges, Change{
						Path:        state.ObsidianPath,
						Type:        ChangeConflict,
						Direction:   DirectionBoth,
						LocalHash:   localHashes.FullHash,
						RemoteMtime: info.LastEditedTime,
						State:       state,
						LocalHashes: localHashes,
					})
				} else {
					// Only remote modified - pull.
					additionalChanges = append(additionalChanges, Change{
						Path:        state.ObsidianPath,
						Type:        ChangeModified,
						Direction:   DirectionPull,
						RemoteMtime: info.LastEditedTime,
						State:       state,
					})
				}
			}
		}
	}

	// Step 7: Combine local and additional changes.
	allChanges := make([]Change, 0, len(localChanges)+len(additionalChanges))
	allChanges = append(allChanges, localChanges...)
	allChanges = append(allChanges, additionalChanges...)

	return allChanges, nil
}

// FilterByDirection returns changes matching the specified direction.
func FilterByDirection(changes []Change, direction Direction) []Change {
	var filtered []Change
	for _, c := range changes {
		if c.Direction == direction {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

// FilterByType returns changes matching the specified type.
func FilterByType(changes []Change, changeType ChangeType) []Change {
	var filtered []Change
	for _, c := range changes {
		if c.Type == changeType {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

// HasConflicts returns true if any changes are conflicts.
func HasConflicts(changes []Change) bool {
	for _, c := range changes {
		if c.Type == ChangeConflict {
			return true
		}
	}
	return false
}

// CountByType returns the count of changes for each type.
func CountByType(changes []Change) map[ChangeType]int {
	counts := make(map[ChangeType]int)
	for _, c := range changes {
		counts[c.Type]++
	}
	return counts
}

// CountByDirection returns the count of changes for each direction.
func CountByDirection(changes []Change) map[Direction]int {
	counts := make(map[Direction]int)
	for _, c := range changes {
		counts[c.Direction]++
	}
	return counts
}
