package storage

import (
	"sync"

	"github.com/maruel/mddb/internal/models"
)

// Cache handles in-memory caching of metadata, hot pages, and records.
type Cache struct {
	mu sync.RWMutex

	// Node tree metadata (full tree for sidebar)
	nodeTree []*models.Node

	// Hot pages (map of page ID to page)
	pages map[string]*models.Page

	// Hot records (map of database ID to list of records)
	records map[string][]*models.Record

	// Max size for LRU-like behavior (simplified for now)
	maxPages   int
	maxRecords int
}

// NewCache initializes a new cache.
func NewCache() *Cache {
	return &Cache{
		pages:      make(map[string]*models.Page),
		records:    make(map[string][]*models.Record),
		maxPages:   100,
		maxRecords: 100,
	}
}

// GetNodeTree returns the cached node tree.
func (c *Cache) GetNodeTree() []*models.Node {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.nodeTree
}

// SetNodeTree updates the cached node tree.
func (c *Cache) SetNodeTree(nodes []*models.Node) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nodeTree = nodes
}

// InvalidateNodeTree clears the cached node tree.
func (c *Cache) InvalidateNodeTree() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nodeTree = nil
}

// GetPage returns a cached page by ID.
func (c *Cache) GetPage(id string) (*models.Page, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	page, ok := c.pages[id]
	return page, ok
}

// SetPage caches a page.
func (c *Cache) SetPage(page *models.Page) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Simple size limiting: clear if it grows too large
	if len(c.pages) >= c.maxPages {
		c.pages = make(map[string]*models.Page)
	}
	c.pages[page.ID] = page
}

// InvalidatePage removes a page from cache.
func (c *Cache) InvalidatePage(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.pages, id)
}

// GetRecords returns cached records for a database.
func (c *Cache) GetRecords(databaseID string) ([]*models.Record, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	records, ok := c.records[databaseID]
	return records, ok
}

// SetRecords caches records for a database.
func (c *Cache) SetRecords(databaseID string, records []*models.Record) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.records) >= c.maxRecords {
		c.records = make(map[string][]*models.Record)
	}
	c.records[databaseID] = records
}

// InvalidateRecords removes records for a database from cache.
func (c *Cache) InvalidateRecords(databaseID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.records, databaseID)
}

// InvalidateAll clears the entire cache.
func (c *Cache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nodeTree = nil
	c.pages = make(map[string]*models.Page)
	c.records = make(map[string][]*models.Record)
}
