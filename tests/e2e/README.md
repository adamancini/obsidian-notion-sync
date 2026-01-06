# End-to-End Integration Tests

This directory contains end-to-end (E2E) integration tests for the obsidian-notion-sync project. These tests run against a real Notion workspace to verify the full sync lifecycle.

## Prerequisites

### 1. Notion Integration Token

Create a Notion integration:

1. Go to https://www.notion.so/my-integrations
2. Click "New integration"
3. Name it (e.g., "obsidian-notion-sync-tests")
4. Select the workspace where tests will run
5. Copy the "Internal Integration Token"

### 2. Test Parent Page

Create a dedicated page in your Notion workspace for test pages:

1. Create a new page (e.g., "E2E Test Pages")
2. Share the page with your integration (click "..." > "Add connections")
3. Copy the page ID from the URL (the 32-character hex string after the page name)

**Important:** All test pages will be created under this parent page and cleaned up after tests.

## Running E2E Tests

### Using Make

```bash
# Set environment variables and run
export NOTION_TOKEN="secret_xxxxx..."
export NOTION_TEST_PAGE_ID="xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
make test-e2e

# Or inline:
NOTION_TOKEN="..." NOTION_TEST_PAGE_ID="..." make test-e2e
```

### Using Go Test Directly

```bash
export NOTION_TOKEN="secret_xxxxx..."
export NOTION_TEST_PAGE_ID="xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
go test -v -tags=e2e -timeout=10m ./tests/e2e/...
```

### Running Specific Tests

```bash
# Run only push tests
go test -v -tags=e2e -timeout=10m -run TestPush ./tests/e2e/...

# Run only sync tests
go test -v -tags=e2e -timeout=10m -run TestSync ./tests/e2e/...

# Run a specific test
go test -v -tags=e2e -timeout=10m -run TestPush_SimpleNote ./tests/e2e/...
```

## Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `NOTION_TOKEN` | Yes | Notion integration token (starts with `secret_`) |
| `NOTION_TEST_PAGE_ID` | Yes | Parent page ID for test pages (32-char hex) |

## Test Categories

### Push Tests (`push_test.go`)

Tests for pushing content from Obsidian to Notion:

- `TestPush_SimpleNote` - Basic markdown note
- `TestPush_NoteWithFrontmatter` - Frontmatter properties
- `TestPush_NoteWithCallout` - Obsidian callouts
- `TestPush_NoteWithNestedLists` - Nested bullet lists
- `TestPush_NoteWithCodeBlock` - Code blocks
- `TestPush_UpdateExistingPage` - Updating existing pages
- `TestPush_MultipleNotes` - Multiple files
- `TestPush_NoteInSubdirectory` - Subdirectory handling
- `TestPush_NoteWithUnicode` - Unicode characters
- `TestPush_ChangeDetection` - Change detection
- `TestPush_DeletedFileDetection` - Deletion detection

### Pull Tests (`pull_test.go`)

Tests for pulling content from Notion to Obsidian:

- `TestPull_NewPageFromNotion` - Pulling new pages
- `TestPull_ModifiedPageFromNotion` - Pulling updates
- `TestPull_PageWithMultipleBlocks` - Various block types
- `TestPull_PageWithFormattedText` - Text formatting
- `TestPull_ArchivedPage` - Archived page handling
- `TestPull_DiscoverNewPages` - Discovering new remote pages
- `TestPull_SyncStateTracking` - State tracking

### Sync Tests (`sync_test.go`)

Tests for bidirectional synchronization:

- `TestSync_RoundTrip` - Push -> modify remote -> pull cycle
- `TestSync_ConflictDetection` - Detecting conflicts
- `TestSync_ResolveConflictKeepLocal` - Resolve keeping local
- `TestSync_ResolveConflictKeepRemote` - Resolve keeping remote
- `TestSync_DeletedLocally` - Local deletion handling
- `TestSync_MultipleFilesInParallel` - Parallel sync
- `TestSync_RenamedFile` - File rename detection
- `TestSync_IncrementalChanges` - Incremental change detection

### Edge Case Tests (`edge_cases_test.go`)

Tests for edge cases and complex scenarios:

#### Wiki-Links
- `TestWikiLinks_BasicResolution` - Link resolution
- `TestWikiLinks_UnresolvedLink` - Unresolved links
- `TestWikiLinks_WithHeadingAnchor` - Heading anchors

#### Frontmatter
- `TestFrontmatter_ComplexTypes` - Complex value types
- `TestFrontmatter_EmptyValues` - Empty values
- `TestFrontmatter_SpecialCharacters` - Special characters

#### Formatting
- `TestFormatting_NestedQuotes` - Nested blockquotes
- `TestFormatting_Tables` - Markdown tables
- `TestFormatting_HorizontalRule` - Horizontal rules
- `TestFormatting_TaskLists` - Task lists
- `TestFormatting_MixedContent` - Mixed content types
- `TestFormatting_DeepNesting` - Deep nesting
- `TestFormatting_LongDocument` - Large documents
- `TestFormatting_SpecialCharactersInContent` - Special characters
- `TestFormatting_EmptyDocument` - Minimal documents
- `TestFormatting_MultipleCallouts` - Multiple callout types

## Test Architecture

### Test Fixture

The `TestFixture` struct provides:

- Temporary vault directory creation
- Configuration file generation
- Notion API client initialization
- SQLite database setup
- Page tracking for cleanup
- Helper methods for file operations

### Cleanup

All tests use `defer f.Cleanup()` to ensure:

1. All created Notion pages are archived
2. Temporary directories are removed
3. Database connections are closed

### Rate Limiting

Tests respect Notion's rate limit (3 requests/second) using the same rate limiter as the production code.

## Writing New Tests

### Basic Test Structure

```go
//go:build e2e
// +build e2e

package e2e

import (
    "testing"
)

func TestMyFeature(t *testing.T) {
    f := NewTestFixture(t)
    defer f.Cleanup()

    ctx, cancel := testContext(t)
    defer cancel()

    // Create test files
    content := Markdown.SimpleNote("Test", "Content")
    f.WriteMarkdownFile("test.md", content)

    // Perform operations...

    // Assert results
    f.AssertPageExists(ctx, pageID)
    f.AssertFileContains("test.md", "expected content")
}
```

### Using Markdown Helpers

```go
// Simple note
Markdown.SimpleNote("Title", "Content")

// Note with frontmatter
Markdown.NoteWithFrontmatter("Title", []string{"tag1", "tag2"}, "status", "Content")

// Note with wiki-links
Markdown.NoteWithWikiLinks("Title", "Content", []string{"Other Note"})

// Note with callout
Markdown.NoteWithCallout("Title", "warning", "Warning message", "Body")

// Note with nested lists
Markdown.NoteWithNestedLists("Title")

// Note with code block
Markdown.NoteWithCodeBlock("Title", "go", "fmt.Println()")
```

## Troubleshooting

### Tests Skipped

If tests are skipped silently, check:
- `NOTION_TOKEN` is set correctly
- `NOTION_TEST_PAGE_ID` is set correctly
- Integration has access to the test page

### Rate Limit Errors

The test framework includes rate limiting, but if you run many tests quickly:
- Add `f.WaitForSync()` between operations
- Run fewer tests in parallel
- Increase timeout with `-timeout=15m`

### Cleanup Failures

If test cleanup fails:
- Manually archive/delete test pages in Notion
- Pages are created under the test parent page
- Look for pages with test-related titles

### Timeout Errors

Default timeout is 5 minutes per test. For slow connections:
```bash
go test -v -tags=e2e -timeout=20m ./tests/e2e/...
```

## CI/CD Integration

For CI/CD pipelines, set secrets as environment variables:

```yaml
# GitHub Actions example
env:
  NOTION_TOKEN: ${{ secrets.NOTION_TOKEN }}
  NOTION_TEST_PAGE_ID: ${{ secrets.NOTION_TEST_PAGE_ID }}

steps:
  - name: Run E2E Tests
    run: make test-e2e
```

**Note:** E2E tests modify a real Notion workspace. Consider using a dedicated test workspace for CI/CD.
