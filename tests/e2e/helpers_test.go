//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jomei/notionapi"

	"github.com/adamancini/obsidian-notion-sync/internal/config"
	"github.com/adamancini/obsidian-notion-sync/internal/notion"
	"github.com/adamancini/obsidian-notion-sync/internal/state"
)

// TestFixture provides setup and teardown for E2E tests.
type TestFixture struct {
	t *testing.T

	// VaultPath is the path to the temporary test vault.
	VaultPath string

	// ConfigPath is the path to the temporary config file.
	ConfigPath string

	// DBPath is the path to the SQLite database.
	DBPath string

	// NotionClient is the Notion API client.
	NotionClient *notion.Client

	// DB is the sync state database.
	DB *state.DB

	// Config is the test configuration.
	Config *config.Config

	// CreatedPageIDs tracks pages created during the test for cleanup.
	CreatedPageIDs []string
	mu             sync.Mutex

	// ParentPageID is the Notion page under which test pages are created.
	ParentPageID string

	// Token is the Notion API token.
	Token string
}

// NewTestFixture creates a new test fixture with temporary directories.
func NewTestFixture(t *testing.T) *TestFixture {
	t.Helper()
	requireEnv(t)

	// Create temporary vault directory.
	vaultPath, err := os.MkdirTemp("", "e2e-vault-*")
	if err != nil {
		t.Fatalf("failed to create temp vault: %v", err)
	}

	// Create config path.
	configPath := filepath.Join(vaultPath, ".obsidian-notion.yaml")

	// Create DB path.
	dbPath := filepath.Join(vaultPath, "sync.db")

	token := os.Getenv("NOTION_TOKEN")
	parentPageID := os.Getenv("NOTION_TEST_PAGE_ID")

	// Create the test configuration.
	cfg := &config.Config{
		Vault: vaultPath,
		Notion: config.NotionConfig{
			Token:       token,
			DefaultPage: parentPageID,
		},
		Transform: config.TransformConfig{
			Dataview:        "placeholder",
			UnresolvedLinks: "placeholder",
			Callouts: map[string]string{
				"note":    "note-icon",
				"warning": "warning-icon",
				"tip":     "tip-icon",
				"info":    "info-icon",
			},
		},
		Sync: config.SyncConfig{
			ConflictStrategy: "manual",
			DeletionStrategy: "archive",
		},
		RateLimit: config.RateLimitConfig{
			RequestsPerSecond: 3.0,
			BatchSize:         100,
			Workers:           1,
		},
	}

	// Save config file.
	if err := cfg.Save(configPath); err != nil {
		os.RemoveAll(vaultPath)
		t.Fatalf("failed to save config: %v", err)
	}

	// Create Notion client.
	client := notion.New(token, notion.WithRateLimit(3.0))

	// Create state DB.
	db, err := state.Open(dbPath)
	if err != nil {
		os.RemoveAll(vaultPath)
		t.Fatalf("failed to open state db: %v", err)
	}

	return &TestFixture{
		t:              t,
		VaultPath:      vaultPath,
		ConfigPath:     configPath,
		DBPath:         dbPath,
		NotionClient:   client,
		DB:             db,
		Config:         cfg,
		CreatedPageIDs: make([]string, 0),
		ParentPageID:   parentPageID,
		Token:          token,
	}
}

// Cleanup removes all created test data.
func (f *TestFixture) Cleanup() {
	f.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Archive all created pages in Notion.
	f.mu.Lock()
	pageIDs := make([]string, len(f.CreatedPageIDs))
	copy(pageIDs, f.CreatedPageIDs)
	f.mu.Unlock()

	for _, pageID := range pageIDs {
		if err := f.NotionClient.ArchivePage(ctx, pageID); err != nil {
			f.t.Logf("warning: failed to archive page %s: %v", pageID, err)
		}
	}

	// Close database.
	if f.DB != nil {
		f.DB.Close()
	}

	// Remove temporary directories.
	if f.VaultPath != "" {
		os.RemoveAll(f.VaultPath)
	}
}

// TrackPage adds a page ID to the cleanup list.
func (f *TestFixture) TrackPage(pageID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.CreatedPageIDs = append(f.CreatedPageIDs, pageID)
}

// WriteMarkdownFile creates a markdown file in the test vault.
func (f *TestFixture) WriteMarkdownFile(relativePath, content string) string {
	f.t.Helper()

	fullPath := filepath.Join(f.VaultPath, relativePath)

	// Ensure parent directory exists.
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		f.t.Fatalf("failed to create directory %s: %v", dir, err)
	}

	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		f.t.Fatalf("failed to write file %s: %v", fullPath, err)
	}

	return fullPath
}

// ReadMarkdownFile reads a markdown file from the test vault.
func (f *TestFixture) ReadMarkdownFile(relativePath string) string {
	f.t.Helper()

	fullPath := filepath.Join(f.VaultPath, relativePath)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		f.t.Fatalf("failed to read file %s: %v", fullPath, err)
	}

	return string(content)
}

// FileExists checks if a file exists in the test vault.
func (f *TestFixture) FileExists(relativePath string) bool {
	fullPath := filepath.Join(f.VaultPath, relativePath)
	_, err := os.Stat(fullPath)
	return err == nil
}

// DeleteFile removes a file from the test vault.
func (f *TestFixture) DeleteFile(relativePath string) {
	f.t.Helper()

	fullPath := filepath.Join(f.VaultPath, relativePath)
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		f.t.Fatalf("failed to delete file %s: %v", fullPath, err)
	}
}

// CreateNotionPage creates a page directly in Notion for testing.
func (f *TestFixture) CreateNotionPage(ctx context.Context, title, content string) (string, error) {
	// Create a page with title property.
	page, err := f.NotionClient.API().Page.Create(ctx, &notionapi.PageCreateRequest{
		Parent: notionapi.Parent{
			Type:   notionapi.ParentTypePageID,
			PageID: notionapi.PageID(f.ParentPageID),
		},
		Properties: notionapi.Properties{
			"title": notionapi.TitleProperty{
				Title: []notionapi.RichText{
					{
						Type: notionapi.ObjectTypeText,
						Text: &notionapi.Text{Content: title},
					},
				},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("create page: %w", err)
	}

	pageID := string(page.ID)
	f.TrackPage(pageID)

	// Add content blocks if provided.
	if content != "" {
		blocks := []notionapi.Block{
			&notionapi.ParagraphBlock{
				BasicBlock: notionapi.BasicBlock{
					Object: notionapi.ObjectTypeBlock,
					Type:   notionapi.BlockTypeParagraph,
				},
				Paragraph: notionapi.Paragraph{
					RichText: []notionapi.RichText{
						{
							Type: notionapi.ObjectTypeText,
							Text: &notionapi.Text{Content: content},
						},
					},
				},
			},
		}

		_, err = f.NotionClient.API().Block.AppendChildren(ctx, notionapi.BlockID(pageID), &notionapi.AppendBlockChildrenRequest{
			Children: blocks,
		})
		if err != nil {
			return pageID, fmt.Errorf("append blocks: %w", err)
		}
	}

	return pageID, nil
}

// GetNotionPage retrieves a page from Notion.
func (f *TestFixture) GetNotionPage(ctx context.Context, pageID string) (*notionapi.Page, error) {
	return f.NotionClient.GetPage(ctx, pageID)
}

// GetNotionPageBlocks retrieves all blocks from a Notion page.
func (f *TestFixture) GetNotionPageBlocks(ctx context.Context, pageID string) ([]notionapi.Block, error) {
	return f.NotionClient.GetAllBlocks(ctx, pageID)
}

// UpdateNotionPage updates a page's content in Notion.
func (f *TestFixture) UpdateNotionPage(ctx context.Context, pageID, newContent string) error {
	// First, delete existing blocks.
	blocks, err := f.GetNotionPageBlocks(ctx, pageID)
	if err != nil {
		return fmt.Errorf("get blocks: %w", err)
	}

	for _, block := range blocks {
		blockID := extractBlockID(block)
		if blockID != "" {
			_, err := f.NotionClient.API().Block.Delete(ctx, notionapi.BlockID(blockID))
			if err != nil {
				return fmt.Errorf("delete block %s: %w", blockID, err)
			}
		}
	}

	// Add new content.
	if newContent != "" {
		newBlocks := []notionapi.Block{
			&notionapi.ParagraphBlock{
				BasicBlock: notionapi.BasicBlock{
					Object: notionapi.ObjectTypeBlock,
					Type:   notionapi.BlockTypeParagraph,
				},
				Paragraph: notionapi.Paragraph{
					RichText: []notionapi.RichText{
						{
							Type: notionapi.ObjectTypeText,
							Text: &notionapi.Text{Content: newContent},
						},
					},
				},
			},
		}

		_, err = f.NotionClient.API().Block.AppendChildren(ctx, notionapi.BlockID(pageID), &notionapi.AppendBlockChildrenRequest{
			Children: newBlocks,
		})
		if err != nil {
			return fmt.Errorf("append blocks: %w", err)
		}
	}

	return nil
}

// extractBlockID extracts the block ID from a Block interface.
func extractBlockID(block notionapi.Block) string {
	switch b := block.(type) {
	case *notionapi.ParagraphBlock:
		return string(b.ID)
	case *notionapi.Heading1Block:
		return string(b.ID)
	case *notionapi.Heading2Block:
		return string(b.ID)
	case *notionapi.Heading3Block:
		return string(b.ID)
	case *notionapi.BulletedListItemBlock:
		return string(b.ID)
	case *notionapi.NumberedListItemBlock:
		return string(b.ID)
	case *notionapi.ToDoBlock:
		return string(b.ID)
	case *notionapi.ToggleBlock:
		return string(b.ID)
	case *notionapi.CodeBlock:
		return string(b.ID)
	case *notionapi.QuoteBlock:
		return string(b.ID)
	case *notionapi.CalloutBlock:
		return string(b.ID)
	case *notionapi.DividerBlock:
		return string(b.ID)
	case *notionapi.ImageBlock:
		return string(b.ID)
	case *notionapi.BookmarkBlock:
		return string(b.ID)
	default:
		return ""
	}
}

// WaitForSync waits a bit to allow Notion API to settle.
// Useful between operations to avoid race conditions.
func (f *TestFixture) WaitForSync() {
	time.Sleep(500 * time.Millisecond)
}

// AssertPageExists verifies a page exists in Notion.
func (f *TestFixture) AssertPageExists(ctx context.Context, pageID string) {
	f.t.Helper()

	page, err := f.GetNotionPage(ctx, pageID)
	if err != nil {
		f.t.Fatalf("page %s does not exist: %v", pageID, err)
	}
	if page.Archived {
		f.t.Fatalf("page %s is archived", pageID)
	}
}

// AssertPageArchived verifies a page is archived in Notion.
func (f *TestFixture) AssertPageArchived(ctx context.Context, pageID string) {
	f.t.Helper()

	page, err := f.GetNotionPage(ctx, pageID)
	if err != nil {
		// Page might be completely deleted or inaccessible.
		return
	}
	if !page.Archived {
		f.t.Fatalf("page %s should be archived but is not", pageID)
	}
}

// AssertFileContent verifies a file's content in the test vault.
func (f *TestFixture) AssertFileContent(relativePath string, expectedContent string) {
	f.t.Helper()

	content := f.ReadMarkdownFile(relativePath)
	if content != expectedContent {
		f.t.Fatalf("file %s content mismatch:\nexpected:\n%s\n\nactual:\n%s", relativePath, expectedContent, content)
	}
}

// AssertFileContains verifies a file contains a substring.
func (f *TestFixture) AssertFileContains(relativePath, substring string) {
	f.t.Helper()

	content := f.ReadMarkdownFile(relativePath)
	if !strings.Contains(content, substring) {
		f.t.Fatalf("file %s does not contain %q:\n%s", relativePath, substring, content)
	}
}

// TestMarkdown provides standard markdown content for tests.
type TestMarkdown struct{}

// SimpleNote returns a simple markdown note.
func (TestMarkdown) SimpleNote(title, content string) string {
	return fmt.Sprintf(`---
title: %s
---

%s
`, title, content)
}

// NoteWithFrontmatter returns a note with various frontmatter fields.
func (TestMarkdown) NoteWithFrontmatter(title string, tags []string, status string, content string) string {
	tagStr := ""
	if len(tags) > 0 {
		tagStr = fmt.Sprintf("\ntags: [%s]", strings.Join(tags, ", "))
	}
	statusStr := ""
	if status != "" {
		statusStr = fmt.Sprintf("\nstatus: %s", status)
	}
	return fmt.Sprintf(`---
title: %s%s%s
---

%s
`, title, tagStr, statusStr, content)
}

// NoteWithWikiLinks returns a note containing wiki-links.
func (TestMarkdown) NoteWithWikiLinks(title, content string, links []string) string {
	linkRefs := ""
	for _, link := range links {
		linkRefs += fmt.Sprintf("\n- [[%s]]", link)
	}
	return fmt.Sprintf(`---
title: %s
---

%s
%s
`, title, content, linkRefs)
}

// NoteWithCallout returns a note with an Obsidian callout.
func (TestMarkdown) NoteWithCallout(title, calloutType, calloutContent, bodyContent string) string {
	return fmt.Sprintf(`---
title: %s
---

> [!%s]
> %s

%s
`, title, calloutType, calloutContent, bodyContent)
}

// NoteWithNestedLists returns a note with nested bullet lists.
func (TestMarkdown) NoteWithNestedLists(title string) string {
	return fmt.Sprintf(`---
title: %s
---

- Item 1
  - Nested 1.1
  - Nested 1.2
    - Deep nested 1.2.1
- Item 2
  - Nested 2.1
- Item 3
`, title)
}

// NoteWithCodeBlock returns a note with a code block.
func (TestMarkdown) NoteWithCodeBlock(title, language, code string) string {
	return fmt.Sprintf("---\ntitle: %s\n---\n\n```%s\n%s\n```\n", title, language, code)
}

// NoteWithUnicode returns a note with Unicode characters.
func (TestMarkdown) NoteWithUnicode(title string) string {
	return fmt.Sprintf(`---
title: %s
---

This note has Unicode: cafe, resume, naive, uber

Emojis: Writing tests

CJK characters: Chinese

Greek: alpha beta gamma
`, title)
}

// MarkdownHelper provides helper methods for creating test markdown.
var Markdown = TestMarkdown{}
