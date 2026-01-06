package state

import (
	"context"
	"testing"
	"time"
)

func TestCachingRemoteChecker_CachesResults(t *testing.T) {
	// Create a counting mock.
	callCount := 0
	inner := &countingMockChecker{
		pages: map[string]*RemotePageInfo{
			"page-1": {
				PageID:         "page-1",
				LastEditedTime: time.Now(),
			},
		},
		getInfoCalls: &callCount,
	}

	cached := NewCachingRemoteChecker(inner, 5*time.Minute)

	ctx := context.Background()

	// First call should hit inner.
	_, err := cached.GetPageInfo(ctx, "page-1")
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}

	// Second call should use cache.
	_, err = cached.GetPageInfo(ctx, "page-1")
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call (cached), got %d", callCount)
	}
}

func TestCachingRemoteChecker_ExpiresTTL(t *testing.T) {
	callCount := 0
	inner := &countingMockChecker{
		pages: map[string]*RemotePageInfo{
			"page-1": {
				PageID:         "page-1",
				LastEditedTime: time.Now(),
			},
		},
		getInfoCalls: &callCount,
	}

	// Very short TTL.
	cached := NewCachingRemoteChecker(inner, 1*time.Millisecond)

	ctx := context.Background()

	// First call.
	_, err := cached.GetPageInfo(ctx, "page-1")
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}

	// Wait for TTL to expire.
	time.Sleep(5 * time.Millisecond)

	// Second call should hit inner again (cache expired).
	_, err = cached.GetPageInfo(ctx, "page-1")
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls (cache expired), got %d", callCount)
	}
}

func TestCachingRemoteChecker_BatchUsesCachePartially(t *testing.T) {
	batchCallCount := 0
	inner := &countingMockChecker{
		pages: map[string]*RemotePageInfo{
			"page-1": {PageID: "page-1", LastEditedTime: time.Now()},
			"page-2": {PageID: "page-2", LastEditedTime: time.Now()},
			"page-3": {PageID: "page-3", LastEditedTime: time.Now()},
		},
		getBatchCalls: &batchCallCount,
	}

	cached := NewCachingRemoteChecker(inner, 5*time.Minute)

	ctx := context.Background()

	// Pre-populate cache with page-1.
	_, err := cached.GetPageInfo(ctx, "page-1")
	if err != nil {
		t.Fatalf("pre-populate failed: %v", err)
	}

	// Now do a batch request for page-1, page-2, page-3.
	results, err := cached.GetPagesInfoBatch(ctx, []string{"page-1", "page-2", "page-3"})
	if err != nil {
		t.Fatalf("batch call failed: %v", err)
	}

	// Should have all 3 results.
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}

	// Batch call should only have been made for uncached pages (page-2, page-3).
	if batchCallCount != 1 {
		t.Errorf("expected 1 batch call, got %d", batchCallCount)
	}
}

func TestCachingRemoteChecker_ClearCache(t *testing.T) {
	callCount := 0
	inner := &countingMockChecker{
		pages: map[string]*RemotePageInfo{
			"page-1": {PageID: "page-1", LastEditedTime: time.Now()},
		},
		getInfoCalls: &callCount,
	}

	cached := NewCachingRemoteChecker(inner, 5*time.Minute)

	ctx := context.Background()

	// First call.
	_, err := cached.GetPageInfo(ctx, "page-1")
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}

	// Clear cache.
	cached.ClearCache()

	// Second call should hit inner again.
	_, err = cached.GetPageInfo(ctx, "page-1")
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls after clear, got %d", callCount)
	}
}

func TestCachingRemoteChecker_ClearCacheEntry(t *testing.T) {
	callCount := 0
	inner := &countingMockChecker{
		pages: map[string]*RemotePageInfo{
			"page-1": {PageID: "page-1", LastEditedTime: time.Now()},
			"page-2": {PageID: "page-2", LastEditedTime: time.Now()},
		},
		getInfoCalls: &callCount,
	}

	cached := NewCachingRemoteChecker(inner, 5*time.Minute)

	ctx := context.Background()

	// Populate cache with both pages.
	_, _ = cached.GetPageInfo(ctx, "page-1")
	_, _ = cached.GetPageInfo(ctx, "page-2")
	if callCount != 2 {
		t.Errorf("expected 2 calls, got %d", callCount)
	}

	// Clear only page-1.
	cached.ClearCacheEntry("page-1")

	// page-1 should hit inner, page-2 should use cache.
	_, _ = cached.GetPageInfo(ctx, "page-1")
	_, _ = cached.GetPageInfo(ctx, "page-2")
	if callCount != 3 {
		t.Errorf("expected 3 calls (page-1 refetched), got %d", callCount)
	}
}

func TestErrPageNotFound(t *testing.T) {
	err := ErrPageNotFound{PageID: "test-id"}
	expected := "page not found: test-id"
	if err.Error() != expected {
		t.Errorf("expected '%s', got '%s'", expected, err.Error())
	}
}

// countingMockChecker counts how many times its methods are called.
type countingMockChecker struct {
	pages         map[string]*RemotePageInfo
	getInfoCalls  *int
	getBatchCalls *int
}

func (m *countingMockChecker) GetPageInfo(ctx context.Context, pageID string) (*RemotePageInfo, error) {
	if m.getInfoCalls != nil {
		*m.getInfoCalls++
	}
	if info, ok := m.pages[pageID]; ok {
		return info, nil
	}
	return &RemotePageInfo{PageID: pageID, Err: ErrPageNotFound{PageID: pageID}}, ErrPageNotFound{PageID: pageID}
}

func (m *countingMockChecker) GetPagesInfoBatch(ctx context.Context, pageIDs []string) (map[string]*RemotePageInfo, error) {
	if m.getBatchCalls != nil {
		*m.getBatchCalls++
	}
	results := make(map[string]*RemotePageInfo)
	for _, id := range pageIDs {
		if info, ok := m.pages[id]; ok {
			results[id] = info
		} else {
			results[id] = &RemotePageInfo{PageID: id, Err: ErrPageNotFound{PageID: id}}
		}
	}
	return results, nil
}
