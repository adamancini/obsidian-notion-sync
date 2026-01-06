package notion

import (
	"context"
	"fmt"

	"github.com/jomei/notionapi"

	"github.com/adamancini/obsidian-notion-sync/internal/transformer"
)

// PageResult contains information about a created or updated page.
type PageResult struct {
	PageID    string
	URL       string
	CreatedAt string
	UpdatedAt string
}

// CreatePage creates a new page in a database with properties and blocks.
func (c *Client) CreatePage(ctx context.Context, databaseID string, page *transformer.NotionPage) (*PageResult, error) {
	if err := c.wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}

	// Create page with properties.
	created, err := c.api.Page.Create(ctx, &notionapi.PageCreateRequest{
		Parent: notionapi.Parent{
			Type:       notionapi.ParentTypeDatabaseID,
			DatabaseID: notionapi.DatabaseID(databaseID),
		},
		Properties: page.Properties,
	})
	if err != nil {
		return nil, fmt.Errorf("create page: %w", err)
	}

	pageID := string(created.ID)

	// Append blocks in batches.
	if err := c.appendBlocks(ctx, pageID, page.Children); err != nil {
		return &PageResult{PageID: pageID}, fmt.Errorf("append blocks: %w", err)
	}

	return &PageResult{
		PageID:    pageID,
		URL:       created.URL,
		CreatedAt: created.CreatedTime.String(),
		UpdatedAt: created.LastEditedTime.String(),
	}, nil
}

// CreatePageUnderPage creates a new page as a child of another page.
func (c *Client) CreatePageUnderPage(ctx context.Context, parentPageID string, page *transformer.NotionPage) (*PageResult, error) {
	if err := c.wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}

	// Create page with properties.
	created, err := c.api.Page.Create(ctx, &notionapi.PageCreateRequest{
		Parent: notionapi.Parent{
			Type:   notionapi.ParentTypePageID,
			PageID: notionapi.PageID(parentPageID),
		},
		Properties: page.Properties,
	})
	if err != nil {
		return nil, fmt.Errorf("create page: %w", err)
	}

	pageID := string(created.ID)

	// Append blocks in batches.
	if err := c.appendBlocks(ctx, pageID, page.Children); err != nil {
		return &PageResult{PageID: pageID}, fmt.Errorf("append blocks: %w", err)
	}

	return &PageResult{
		PageID:    pageID,
		URL:       created.URL,
		CreatedAt: created.CreatedTime.String(),
		UpdatedAt: created.LastEditedTime.String(),
	}, nil
}

// UpdatePage updates an existing page's properties and replaces all blocks.
func (c *Client) UpdatePage(ctx context.Context, pageID string, page *transformer.NotionPage) error {
	// 1. Update properties.
	if err := c.wait(ctx); err != nil {
		return fmt.Errorf("rate limit: %w", err)
	}

	_, err := c.api.Page.Update(ctx, notionapi.PageID(pageID), &notionapi.PageUpdateRequest{
		Properties: page.Properties,
	})
	if err != nil {
		return fmt.Errorf("update properties: %w", err)
	}

	// 2. Delete existing blocks.
	if err := c.deleteAllBlocks(ctx, pageID); err != nil {
		return fmt.Errorf("delete blocks: %w", err)
	}

	// 3. Append new blocks.
	if err := c.appendBlocks(ctx, pageID, page.Children); err != nil {
		return fmt.Errorf("append blocks: %w", err)
	}

	return nil
}

// GetPage retrieves a page by ID.
func (c *Client) GetPage(ctx context.Context, pageID string) (*notionapi.Page, error) {
	if err := c.wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}

	page, err := c.api.Page.Get(ctx, notionapi.PageID(pageID))
	if err != nil {
		return nil, fmt.Errorf("get page: %w", err)
	}

	return page, nil
}

// FetchPage retrieves a complete page with all blocks for pull operations.
func (c *Client) FetchPage(ctx context.Context, pageID string) (*transformer.NotionPage, error) {
	// Get page properties.
	page, err := c.GetPage(ctx, pageID)
	if err != nil {
		return nil, err
	}

	// Get all blocks.
	blocks, err := c.GetAllBlocks(ctx, pageID)
	if err != nil {
		return nil, err
	}

	return &transformer.NotionPage{
		Properties: page.Properties,
		Children:   blocks,
	}, nil
}

// ArchivePage archives (soft deletes) a page.
func (c *Client) ArchivePage(ctx context.Context, pageID string) error {
	if err := c.wait(ctx); err != nil {
		return fmt.Errorf("rate limit: %w", err)
	}

	_, err := c.api.Page.Update(ctx, notionapi.PageID(pageID), &notionapi.PageUpdateRequest{
		Archived: true,
	})
	if err != nil {
		return fmt.Errorf("archive page: %w", err)
	}

	return nil
}

// DeletePage permanently deletes a page by archiving it.
// Note: Notion API does not support permanent deletion via API.
// This archives the page, which can be permanently deleted from Notion's trash.
func (c *Client) DeletePage(ctx context.Context, pageID string) error {
	// Notion API does not support permanent deletion, only archiving.
	// Archived pages go to trash and can be manually deleted from there.
	return c.ArchivePage(ctx, pageID)
}

// UpdatePageTitle updates the title property of a page.
func (c *Client) UpdatePageTitle(ctx context.Context, pageID string, title string) error {
	if err := c.wait(ctx); err != nil {
		return fmt.Errorf("rate limit: %w", err)
	}

	// Create title property update.
	// The title property in Notion is typically named "title" or "Name".
	// We'll update the title property.
	titleProp := notionapi.TitleProperty{
		Title: []notionapi.RichText{
			{
				Type: notionapi.ObjectTypeText,
				Text: &notionapi.Text{
					Content: title,
				},
			},
		},
	}

	_, err := c.api.Page.Update(ctx, notionapi.PageID(pageID), &notionapi.PageUpdateRequest{
		Properties: notionapi.Properties{
			"title": titleProp,
		},
	})
	if err != nil {
		return fmt.Errorf("update page title: %w", err)
	}

	return nil
}

// appendBlocks appends blocks to a page in batches.
func (c *Client) appendBlocks(ctx context.Context, pageID string, blocks []notionapi.Block) error {
	for i := 0; i < len(blocks); i += c.batchSize {
		end := i + c.batchSize
		if end > len(blocks) {
			end = len(blocks)
		}
		batch := blocks[i:end]

		if err := c.wait(ctx); err != nil {
			return fmt.Errorf("rate limit: %w", err)
		}

		_, err := c.api.Block.AppendChildren(ctx, notionapi.BlockID(pageID), &notionapi.AppendBlockChildrenRequest{
			Children: batch,
		})
		if err != nil {
			return fmt.Errorf("append batch %d-%d: %w", i, end, err)
		}
	}

	return nil
}

// deleteAllBlocks deletes all children blocks of a page.
func (c *Client) deleteAllBlocks(ctx context.Context, pageID string) error {
	// Get all block IDs first.
	blocks, err := c.GetAllBlocks(ctx, pageID)
	if err != nil {
		return err
	}

	// Delete each block.
	for _, block := range blocks {
		if err := c.wait(ctx); err != nil {
			return fmt.Errorf("rate limit: %w", err)
		}

		// Get block ID from the block interface.
		blockID := getBlockID(block)
		if blockID == "" {
			continue
		}

		_, err := c.api.Block.Delete(ctx, notionapi.BlockID(blockID))
		if err != nil {
			return fmt.Errorf("delete block %s: %w", blockID, err)
		}
	}

	return nil
}

// getBlockID extracts the block ID from a notionapi.Block interface.
// Uses the extractBlockID function from blocks.go.
func getBlockID(block notionapi.Block) string {
	return extractBlockID(block)
}
