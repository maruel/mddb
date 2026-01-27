// Implements the Notion API client with rate limiting.

package notion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	// BaseURL is the Notion API base URL.
	BaseURL = "https://api.notion.com/v1"
	// APIVersion is the pinned Notion API version.
	APIVersion = "2022-06-28"
	// MinInterval is the minimum time between requests (3 req/sec).
	MinInterval = 334 * time.Millisecond
)

// Client is a rate-limited Notion API client.
type Client struct {
	token       string
	httpClient  *http.Client
	lastRequest time.Time
	mu          sync.Mutex
}

// NewClient creates a new Notion API client.
func NewClient(token string) *Client {
	return &Client{
		token: token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// throttle ensures rate limiting between requests.
func (c *Client) throttle() {
	c.mu.Lock()
	defer c.mu.Unlock()

	elapsed := time.Since(c.lastRequest)
	if elapsed < MinInterval {
		time.Sleep(MinInterval - elapsed)
	}
	c.lastRequest = time.Now()
}

// do performs an HTTP request with rate limiting.
func (c *Client) do(ctx context.Context, method, path string, body any) ([]byte, error) {
	c.throttle()

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, BaseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Notion-Version", APIVersion)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var apiErr Error
		if err := json.Unmarshal(respBody, &apiErr); err != nil {
			return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
		}
		return nil, &apiErr
	}

	return respBody, nil
}

// SearchFilter defines filters for the search endpoint.
type SearchFilter struct {
	Value    string `json:"value"`    // "page" or "database"
	Property string `json:"property"` // "object"
}

// SearchRequest is the request body for the search endpoint.
type SearchRequest struct {
	Query       string        `json:"query,omitempty"`
	Filter      *SearchFilter `json:"filter,omitempty"`
	StartCursor string        `json:"start_cursor,omitempty"`
	PageSize    int           `json:"page_size,omitempty"`
}

// Search searches for pages and databases.
func (c *Client) Search(ctx context.Context, req *SearchRequest) (*SearchResponse, error) {
	if req.PageSize == 0 {
		req.PageSize = 100
	}

	data, err := c.do(ctx, http.MethodPost, "/search", req)
	if err != nil {
		return nil, err
	}

	var resp SearchResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse search response: %w", err)
	}
	return &resp, nil
}

// SearchAll searches for all pages and databases, handling pagination.
func (c *Client) SearchAll(ctx context.Context, query string, filter *SearchFilter) ([]SearchResult, error) {
	var results []SearchResult
	var cursor string

	for {
		req := &SearchRequest{
			Query:       query,
			Filter:      filter,
			StartCursor: cursor,
			PageSize:    100,
		}

		resp, err := c.Search(ctx, req)
		if err != nil {
			return nil, err
		}

		results = append(results, resp.Results...)

		if !resp.HasMore || resp.NextCursor == nil {
			break
		}
		cursor = *resp.NextCursor
	}

	return results, nil
}

// GetDatabase retrieves a database by ID.
func (c *Client) GetDatabase(ctx context.Context, id string) (*Database, error) {
	data, err := c.do(ctx, http.MethodGet, "/databases/"+id, nil)
	if err != nil {
		return nil, err
	}

	var db Database
	if err := json.Unmarshal(data, &db); err != nil {
		return nil, fmt.Errorf("failed to parse database response: %w", err)
	}
	return &db, nil
}

// QueryOptions defines options for querying a database.
type QueryOptions struct {
	Filter      any    `json:"filter,omitempty"`
	Sorts       []Sort `json:"sorts,omitempty"`
	StartCursor string `json:"start_cursor,omitempty"`
	PageSize    int    `json:"page_size,omitempty"`
}

// Sort defines a sort order for database queries.
type Sort struct {
	Property  string `json:"property,omitempty"`
	Timestamp string `json:"timestamp,omitempty"` // "created_time" or "last_edited_time"
	Direction string `json:"direction"`           // "ascending" or "descending"
}

// QueryDatabase queries a database for pages.
func (c *Client) QueryDatabase(ctx context.Context, databaseID string, opts *QueryOptions) (*QueryResponse, error) {
	if opts == nil {
		opts = &QueryOptions{}
	}
	if opts.PageSize == 0 {
		opts.PageSize = 100
	}

	data, err := c.do(ctx, http.MethodPost, "/databases/"+databaseID+"/query", opts)
	if err != nil {
		return nil, err
	}

	var resp QueryResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse query response: %w", err)
	}
	return &resp, nil
}

// QueryDatabaseAll queries all pages in a database, handling pagination.
func (c *Client) QueryDatabaseAll(ctx context.Context, databaseID string, opts *QueryOptions) ([]Page, error) {
	var pages []Page
	var cursor string

	for {
		reqOpts := &QueryOptions{
			PageSize: 100,
		}
		if opts != nil {
			reqOpts.Filter = opts.Filter
			reqOpts.Sorts = opts.Sorts
		}
		reqOpts.StartCursor = cursor

		resp, err := c.QueryDatabase(ctx, databaseID, reqOpts)
		if err != nil {
			return nil, err
		}

		pages = append(pages, resp.Results...)

		if !resp.HasMore || resp.NextCursor == nil {
			break
		}
		cursor = *resp.NextCursor
	}

	return pages, nil
}

// GetPage retrieves a page by ID.
func (c *Client) GetPage(ctx context.Context, id string) (*Page, error) {
	data, err := c.do(ctx, http.MethodGet, "/pages/"+id, nil)
	if err != nil {
		return nil, err
	}

	var page Page
	if err := json.Unmarshal(data, &page); err != nil {
		return nil, fmt.Errorf("failed to parse page response: %w", err)
	}
	return &page, nil
}

// GetBlockChildren retrieves the children of a block.
func (c *Client) GetBlockChildren(ctx context.Context, blockID, cursor string) (*BlocksResponse, error) {
	path := "/blocks/" + blockID + "/children?page_size=100"
	if cursor != "" {
		path += "&start_cursor=" + cursor
	}

	data, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var resp BlocksResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse blocks response: %w", err)
	}
	return &resp, nil
}

// GetBlockChildrenAll retrieves all children of a block, handling pagination.
func (c *Client) GetBlockChildrenAll(ctx context.Context, blockID string) ([]Block, error) {
	var blocks []Block
	var cursor string

	for {
		resp, err := c.GetBlockChildren(ctx, blockID, cursor)
		if err != nil {
			return nil, err
		}

		blocks = append(blocks, resp.Results...)

		if !resp.HasMore || resp.NextCursor == nil {
			break
		}
		cursor = *resp.NextCursor
	}

	return blocks, nil
}

// GetBlockChildrenRecursive retrieves all children of a block recursively.
// Children are stored in each block's Children field, not flattened.
func (c *Client) GetBlockChildrenRecursive(ctx context.Context, blockID string, maxDepth int) ([]Block, error) {
	return c.getBlockChildrenRecursiveImpl(ctx, blockID, maxDepth, 0)
}

func (c *Client) getBlockChildrenRecursiveImpl(ctx context.Context, blockID string, maxDepth, depth int) ([]Block, error) {
	if maxDepth > 0 && depth >= maxDepth {
		return nil, nil
	}

	blocks, err := c.GetBlockChildrenAll(ctx, blockID)
	if err != nil {
		return nil, err
	}

	for i := range blocks {
		if blocks[i].HasChildren {
			children, err := c.getBlockChildrenRecursiveImpl(ctx, blocks[i].ID, maxDepth, depth+1)
			if err != nil {
				return nil, err
			}
			blocks[i].Children = children
		}
	}

	return blocks, nil
}
