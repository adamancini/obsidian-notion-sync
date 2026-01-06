//go:build e2e
// +build e2e

// Package e2e provides end-to-end integration tests for obsidian-notion-sync.
// These tests require a real Notion workspace and API token.
//
// To run E2E tests:
//
//	export NOTION_TOKEN="your-notion-integration-token"
//	export NOTION_TEST_PAGE_ID="parent-page-id-for-test-pages"
//	go test -tags=e2e ./tests/e2e/...
//
// The tests will create test pages under NOTION_TEST_PAGE_ID and clean them up
// after completion.
package e2e

import (
	"context"
	"os"
	"testing"
	"time"
)

// testTimeout is the default timeout for E2E tests.
const testTimeout = 5 * time.Minute

// TestMain sets up the test environment.
func TestMain(m *testing.M) {
	// Verify required environment variables.
	if os.Getenv("NOTION_TOKEN") == "" {
		// Skip silently if no token - tests will be skipped individually.
		os.Exit(0)
	}
	if os.Getenv("NOTION_TEST_PAGE_ID") == "" {
		// Skip silently if no test page ID.
		os.Exit(0)
	}

	os.Exit(m.Run())
}

// requireEnv skips the test if required environment variables are not set.
func requireEnv(t *testing.T) {
	t.Helper()
	if os.Getenv("NOTION_TOKEN") == "" {
		t.Skip("NOTION_TOKEN not set, skipping E2E test")
	}
	if os.Getenv("NOTION_TEST_PAGE_ID") == "" {
		t.Skip("NOTION_TEST_PAGE_ID not set, skipping E2E test")
	}
}

// testContext returns a context with the test timeout.
func testContext(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), testTimeout)
}
