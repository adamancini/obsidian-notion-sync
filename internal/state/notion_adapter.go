// Package state provides SQLite-based state management for tracking sync
// status between Obsidian and Notion.
package state

import (
	"context"
	"time"

	"github.com/adamancini/obsidian-notion-sync/internal/notion"
)

// NotionRemoteChecker adapts the Notion client to the RemoteChecker interface.
type NotionRemoteChecker struct {
	client *notion.Client
}

// NewNotionRemoteChecker creates a RemoteChecker backed by the Notion API.
func NewNotionRemoteChecker(client *notion.Client) *NotionRemoteChecker {
	return &NotionRemoteChecker{client: client}
}

// GetPageInfo retrieves metadata for a single Notion page.
func (c *NotionRemoteChecker) GetPageInfo(ctx context.Context, pageID string) (*RemotePageInfo, error) {
	meta, err := c.client.GetPageMetadata(ctx, pageID)
	if err != nil {
		return &RemotePageInfo{
			PageID: pageID,
			Err:    err,
		}, err
	}

	return &RemotePageInfo{
		PageID:         meta.PageID,
		LastEditedTime: meta.LastEditedTime,
		Archived:       meta.Archived,
	}, nil
}

// GetPagesInfoBatch retrieves metadata for multiple Notion pages.
// Returns all pages, including those with errors (Err field set).
func (c *NotionRemoteChecker) GetPagesInfoBatch(ctx context.Context, pageIDs []string) (map[string]*RemotePageInfo, error) {
	results := make(map[string]*RemotePageInfo)

	// Use the batch method which handles rate limiting internally.
	metadata, err := c.client.GetPagesMetadataBatch(ctx, pageIDs)
	if err != nil {
		return nil, err
	}

	// Convert to RemotePageInfo.
	for pageID, meta := range metadata {
		results[pageID] = &RemotePageInfo{
			PageID:         meta.PageID,
			LastEditedTime: meta.LastEditedTime,
			Archived:       meta.Archived,
		}
	}

	// Mark missing pages as having errors.
	for _, pageID := range pageIDs {
		if _, exists := results[pageID]; !exists {
			results[pageID] = &RemotePageInfo{
				PageID: pageID,
				Err:    ErrPageNotFound{PageID: pageID},
			}
		}
	}

	return results, nil
}

// ErrPageNotFound indicates a page could not be found in Notion.
type ErrPageNotFound struct {
	PageID string
}

func (e ErrPageNotFound) Error() string {
	return "page not found: " + e.PageID
}

// CachingRemoteChecker wraps a RemoteChecker with a time-based cache.
// This reduces API calls when checking the same pages multiple times.
type CachingRemoteChecker struct {
	inner    RemoteChecker
	cache    map[string]*cachedPageInfo
	cacheTTL time.Duration
}

type cachedPageInfo struct {
	info      *RemotePageInfo
	fetchedAt time.Time
}

// NewCachingRemoteChecker creates a caching wrapper around a RemoteChecker.
// The cacheTTL controls how long entries are valid.
func NewCachingRemoteChecker(inner RemoteChecker, cacheTTL time.Duration) *CachingRemoteChecker {
	return &CachingRemoteChecker{
		inner:    inner,
		cache:    make(map[string]*cachedPageInfo),
		cacheTTL: cacheTTL,
	}
}

// GetPageInfo retrieves page info, using cache if available and not expired.
func (c *CachingRemoteChecker) GetPageInfo(ctx context.Context, pageID string) (*RemotePageInfo, error) {
	// Check cache.
	if cached, ok := c.cache[pageID]; ok {
		if time.Since(cached.fetchedAt) < c.cacheTTL {
			return cached.info, cached.info.Err
		}
		delete(c.cache, pageID)
	}

	// Fetch from inner.
	info, err := c.inner.GetPageInfo(ctx, pageID)
	if info != nil {
		c.cache[pageID] = &cachedPageInfo{
			info:      info,
			fetchedAt: time.Now(),
		}
	}

	return info, err
}

// GetPagesInfoBatch retrieves multiple pages, using cache where available.
func (c *CachingRemoteChecker) GetPagesInfoBatch(ctx context.Context, pageIDs []string) (map[string]*RemotePageInfo, error) {
	results := make(map[string]*RemotePageInfo)
	var uncached []string

	// Check cache for each page.
	now := time.Now()
	for _, pageID := range pageIDs {
		if cached, ok := c.cache[pageID]; ok {
			if now.Sub(cached.fetchedAt) < c.cacheTTL {
				results[pageID] = cached.info
				continue
			}
			delete(c.cache, pageID)
		}
		uncached = append(uncached, pageID)
	}

	// Fetch uncached pages.
	if len(uncached) > 0 {
		fetched, err := c.inner.GetPagesInfoBatch(ctx, uncached)
		if err != nil {
			return nil, err
		}

		// Add to cache and results.
		for pageID, info := range fetched {
			c.cache[pageID] = &cachedPageInfo{
				info:      info,
				fetchedAt: now,
			}
			results[pageID] = info
		}
	}

	return results, nil
}

// ClearCache removes all cached entries.
func (c *CachingRemoteChecker) ClearCache() {
	c.cache = make(map[string]*cachedPageInfo)
}

// ClearCacheEntry removes a specific page from the cache.
func (c *CachingRemoteChecker) ClearCacheEntry(pageID string) {
	delete(c.cache, pageID)
}
