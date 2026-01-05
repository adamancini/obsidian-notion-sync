package notion

import (
	"context"
	"fmt"

	"github.com/jomei/notionapi"
)

// GetAllBlocks retrieves all blocks from a page, handling pagination.
func (c *Client) GetAllBlocks(ctx context.Context, pageID string) ([]notionapi.Block, error) {
	var allBlocks []notionapi.Block
	var cursor notionapi.Cursor

	for {
		if err := c.wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limit: %w", err)
		}

		resp, err := c.api.Block.GetChildren(ctx, notionapi.BlockID(pageID), &notionapi.Pagination{
			StartCursor: cursor,
			PageSize:    100,
		})
		if err != nil {
			return nil, fmt.Errorf("get children: %w", err)
		}

		allBlocks = append(allBlocks, resp.Results...)

		if !resp.HasMore {
			break
		}
		cursor = notionapi.Cursor(resp.NextCursor)
	}

	// Recursively fetch nested blocks.
	for i, block := range allBlocks {
		if hasChildren(block) {
			blockID := extractBlockID(block)
			if blockID != "" {
				children, err := c.GetAllBlocks(ctx, blockID)
				if err != nil {
					return nil, fmt.Errorf("get nested blocks: %w", err)
				}
				allBlocks[i] = setBlockChildren(block, children)
			}
		}
	}

	return allBlocks, nil
}

// GetBlock retrieves a single block by ID.
func (c *Client) GetBlock(ctx context.Context, blockID string) (notionapi.Block, error) {
	if err := c.wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}

	block, err := c.api.Block.Get(ctx, notionapi.BlockID(blockID))
	if err != nil {
		return nil, fmt.Errorf("get block: %w", err)
	}

	return block, nil
}

// AppendBlocks appends blocks to a parent (page or block).
func (c *Client) AppendBlocks(ctx context.Context, parentID string, blocks []notionapi.Block) error {
	return c.appendBlocks(ctx, parentID, blocks)
}

// DeleteBlock deletes a single block.
func (c *Client) DeleteBlock(ctx context.Context, blockID string) error {
	if err := c.wait(ctx); err != nil {
		return fmt.Errorf("rate limit: %w", err)
	}

	_, err := c.api.Block.Delete(ctx, notionapi.BlockID(blockID))
	if err != nil {
		return fmt.Errorf("delete block: %w", err)
	}

	return nil
}

// UpdateBlock updates a block's content.
func (c *Client) UpdateBlock(ctx context.Context, blockID string, block notionapi.Block) (notionapi.Block, error) {
	if err := c.wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}

	// Note: Notion API update is complex - each block type needs specific handling.
	// This is a simplified stub.

	// TODO: Implement block-type-specific update requests
	return nil, fmt.Errorf("block update not yet implemented")
}

// hasChildren checks if a block has child blocks.
func hasChildren(block notionapi.Block) bool {
	// TODO: Check HasChildren field on each block type.
	// This requires type assertions for each block type.

	switch b := block.(type) {
	case *notionapi.ParagraphBlock:
		return b.HasChildren
	case *notionapi.BulletedListItemBlock:
		return b.HasChildren
	case *notionapi.NumberedListItemBlock:
		return b.HasChildren
	case *notionapi.ToDoBlock:
		return b.HasChildren
	case *notionapi.ToggleBlock:
		return b.HasChildren
	case *notionapi.QuoteBlock:
		return b.HasChildren
	case *notionapi.CalloutBlock:
		return b.HasChildren
	case *notionapi.ColumnListBlock:
		return b.HasChildren
	case *notionapi.ColumnBlock:
		return b.HasChildren
	case *notionapi.SyncedBlock:
		return b.HasChildren
	default:
		return false
	}
}

// extractBlockID gets the ID from a block interface.
func extractBlockID(block notionapi.Block) string {
	// Access the ID through the block's GetID method if available,
	// or use type assertion to get BasicBlock.ID

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
	case *notionapi.QuoteBlock:
		return string(b.ID)
	case *notionapi.CalloutBlock:
		return string(b.ID)
	case *notionapi.CodeBlock:
		return string(b.ID)
	case *notionapi.DividerBlock:
		return string(b.ID)
	case *notionapi.ImageBlock:
		return string(b.ID)
	case *notionapi.EquationBlock:
		return string(b.ID)
	case *notionapi.TableBlock:
		return string(b.ID)
	case *notionapi.TableRowBlock:
		return string(b.ID)
	default:
		return ""
	}
}

// setBlockChildren sets children on a block that supports them.
// Note: This modifies the block's Children field based on block type.
func setBlockChildren(block notionapi.Block, children []notionapi.Block) notionapi.Block {
	switch b := block.(type) {
	case *notionapi.ParagraphBlock:
		b.Paragraph.Children = children
		return b
	case *notionapi.BulletedListItemBlock:
		b.BulletedListItem.Children = children
		return b
	case *notionapi.NumberedListItemBlock:
		b.NumberedListItem.Children = children
		return b
	case *notionapi.ToDoBlock:
		b.ToDo.Children = children
		return b
	case *notionapi.ToggleBlock:
		b.Toggle.Children = children
		return b
	case *notionapi.QuoteBlock:
		b.Quote.Children = children
		return b
	case *notionapi.CalloutBlock:
		b.Callout.Children = children
		return b
	case *notionapi.ColumnListBlock:
		b.ColumnList.Children = children
		return b
	case *notionapi.ColumnBlock:
		b.Column.Children = children
		return b
	case *notionapi.SyncedBlock:
		b.SyncedBlock.Children = children
		return b
	default:
		// Block type doesn't support children, return unchanged.
		return block
	}
}
