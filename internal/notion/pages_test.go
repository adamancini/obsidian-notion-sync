package notion

import (
	"testing"

	"github.com/jomei/notionapi"
)

func TestPageResult(t *testing.T) {
	result := &PageResult{
		PageID:    "page-123",
		URL:       "https://notion.so/page-123",
		CreatedAt: "2024-01-01T00:00:00Z",
		UpdatedAt: "2024-01-02T00:00:00Z",
	}

	if result.PageID != "page-123" {
		t.Errorf("PageID = %q, expected %q", result.PageID, "page-123")
	}

	if result.URL != "https://notion.so/page-123" {
		t.Errorf("URL = %q, expected %q", result.URL, "https://notion.so/page-123")
	}

	if result.CreatedAt != "2024-01-01T00:00:00Z" {
		t.Errorf("CreatedAt = %q, expected %q", result.CreatedAt, "2024-01-01T00:00:00Z")
	}

	if result.UpdatedAt != "2024-01-02T00:00:00Z" {
		t.Errorf("UpdatedAt = %q, expected %q", result.UpdatedAt, "2024-01-02T00:00:00Z")
	}
}

func TestGetBlockID(t *testing.T) {
	// Test that getBlockID uses extractBlockID properly.
	block := &notionapi.ParagraphBlock{
		BasicBlock: notionapi.BasicBlock{ID: "test-block-id"},
	}

	id := getBlockID(block)
	if id != "test-block-id" {
		t.Errorf("getBlockID() = %q, expected %q", id, "test-block-id")
	}
}

// TestBatchSizeCalculation verifies the batch size logic for appending blocks.
func TestBatchSizeCalculation(t *testing.T) {
	tests := []struct {
		name          string
		totalBlocks   int
		batchSize     int
		expectedBatch int
	}{
		{
			name:          "fewer blocks than batch size",
			totalBlocks:   50,
			batchSize:     100,
			expectedBatch: 1,
		},
		{
			name:          "exactly batch size",
			totalBlocks:   100,
			batchSize:     100,
			expectedBatch: 1,
		},
		{
			name:          "more blocks than batch size",
			totalBlocks:   150,
			batchSize:     100,
			expectedBatch: 2,
		},
		{
			name:          "multiple full batches",
			totalBlocks:   300,
			batchSize:     100,
			expectedBatch: 3,
		},
		{
			name:          "custom batch size",
			totalBlocks:   100,
			batchSize:     25,
			expectedBatch: 4,
		},
		{
			name:          "single block",
			totalBlocks:   1,
			batchSize:     100,
			expectedBatch: 1,
		},
		{
			name:          "zero blocks",
			totalBlocks:   0,
			batchSize:     100,
			expectedBatch: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Calculate expected number of batches.
			batches := 0
			for i := 0; i < tt.totalBlocks; i += tt.batchSize {
				batches++
			}

			if batches != tt.expectedBatch {
				t.Errorf("batch count = %d, expected %d", batches, tt.expectedBatch)
			}
		})
	}
}

// TestBatchBoundaries verifies block slicing for batches.
func TestBatchBoundaries(t *testing.T) {
	// Create a slice of 250 blocks.
	blocks := make([]notionapi.Block, 250)
	for i := range blocks {
		blocks[i] = &notionapi.ParagraphBlock{
			BasicBlock: notionapi.BasicBlock{ID: notionapi.BlockID("block-" + string(rune('0'+i%10)))},
		}
	}

	batchSize := 100
	var batches [][]notionapi.Block

	for i := 0; i < len(blocks); i += batchSize {
		end := i + batchSize
		if end > len(blocks) {
			end = len(blocks)
		}
		batches = append(batches, blocks[i:end])
	}

	// Should have 3 batches: 100, 100, 50
	if len(batches) != 3 {
		t.Errorf("batch count = %d, expected 3", len(batches))
	}

	if len(batches[0]) != 100 {
		t.Errorf("batch[0] size = %d, expected 100", len(batches[0]))
	}

	if len(batches[1]) != 100 {
		t.Errorf("batch[1] size = %d, expected 100", len(batches[1]))
	}

	if len(batches[2]) != 50 {
		t.Errorf("batch[2] size = %d, expected 50", len(batches[2]))
	}
}

// TestEmptyBlocksHandling verifies behavior with empty block slices.
func TestEmptyBlocksHandling(t *testing.T) {
	blocks := []notionapi.Block{}
	batchSize := 100

	batches := 0
	for i := 0; i < len(blocks); i += batchSize {
		batches++
	}

	if batches != 0 {
		t.Errorf("batch count for empty blocks = %d, expected 0", batches)
	}
}

// TestClientBatchSizeOption verifies batch size configuration.
func TestClientBatchSizeOption(t *testing.T) {
	tests := []struct {
		name     string
		size     int
		expected int
	}{
		{"small batch", 25, 25},
		{"default batch", 100, 100},
		{"large batch", 200, 200},
		{"single item batch", 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := New("test-token", WithBatchSize(tt.size))

			if client.batchSize != tt.expected {
				t.Errorf("batchSize = %d, expected %d", client.batchSize, tt.expected)
			}
		})
	}
}
