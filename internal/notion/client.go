// Package notion provides a Notion API client wrapper with rate limiting
// and batch operations for the obsidian-notion sync tool.
package notion

import (
	"context"
	"fmt"
	"time"

	"github.com/jomei/notionapi"
	"golang.org/x/time/rate"
)

const (
	// DefaultRateLimit is the default requests per second (Notion's limit is 3/sec).
	DefaultRateLimit = 3

	// DefaultBatchSize is the max blocks per append request.
	DefaultBatchSize = 100
)

// Client wraps the Notion API client with rate limiting and helper methods.
type Client struct {
	api       *notionapi.Client
	limiter   *rate.Limiter
	batchSize int
}

// ClientOption configures the Client.
type ClientOption func(*Client)

// WithRateLimit sets a custom rate limit.
func WithRateLimit(requestsPerSecond float64) ClientOption {
	return func(c *Client) {
		c.limiter = rate.NewLimiter(rate.Limit(requestsPerSecond), 1)
	}
}

// WithBatchSize sets a custom batch size for block operations.
func WithBatchSize(size int) ClientOption {
	return func(c *Client) {
		c.batchSize = size
	}
}

// New creates a new Notion API client with rate limiting.
func New(token string, opts ...ClientOption) *Client {
	c := &Client{
		api:       notionapi.NewClient(notionapi.Token(token)),
		limiter:   rate.NewLimiter(rate.Every(time.Second/DefaultRateLimit), 1),
		batchSize: DefaultBatchSize,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// wait blocks until the rate limiter allows a request.
func (c *Client) wait(ctx context.Context) error {
	return c.limiter.Wait(ctx)
}

// GetDatabase retrieves a database by ID.
func (c *Client) GetDatabase(ctx context.Context, databaseID string) (*notionapi.Database, error) {
	if err := c.wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}

	db, err := c.api.Database.Get(ctx, notionapi.DatabaseID(databaseID))
	if err != nil {
		return nil, fmt.Errorf("get database: %w", err)
	}

	return db, nil
}

// QueryDatabase queries a database with filters and pagination.
func (c *Client) QueryDatabase(ctx context.Context, databaseID string, filter *notionapi.DatabaseQueryRequest) (*notionapi.DatabaseQueryResponse, error) {
	if err := c.wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}

	if filter == nil {
		filter = &notionapi.DatabaseQueryRequest{}
	}

	resp, err := c.api.Database.Query(ctx, notionapi.DatabaseID(databaseID), filter)
	if err != nil {
		return nil, fmt.Errorf("query database: %w", err)
	}

	return resp, nil
}

// SearchPages searches for pages matching a query.
func (c *Client) SearchPages(ctx context.Context, query string) (*notionapi.SearchResponse, error) {
	if err := c.wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}

	resp, err := c.api.Search.Do(ctx, &notionapi.SearchRequest{
		Query: query,
		Filter: notionapi.SearchFilter{
			Property: "object",
			Value:    "page",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("search pages: %w", err)
	}

	return resp, nil
}

// API returns the underlying notionapi.Client for advanced operations.
func (c *Client) API() *notionapi.Client {
	return c.api
}
