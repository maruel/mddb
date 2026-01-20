package infra

import (
	"sync"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/entity"
)

// Cache handles in-memory caching of the node tree.
// Per the simplification plan, this cache only stores the expensive node tree operation.
type Cache struct {
	mu sync.RWMutex

	// Node tree metadata (full tree for sidebar)
	nodeTree []*entity.Node

	// Hot records (map of database ID to list of records)
	records    map[jsonldb.ID][]*entity.DataRecord
	maxRecords int
}

// NewCache initializes a new cache.
func NewCache() *Cache {
	return &Cache{
		records:    make(map[jsonldb.ID][]*entity.DataRecord),
		maxRecords: 100,
	}
}

// GetNodeTree returns the cached node tree.
func (c *Cache) GetNodeTree() []*entity.Node {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.nodeTree
}

// SetNodeTree updates the cached node tree.
func (c *Cache) SetNodeTree(nodes []*entity.Node) {
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

// GetRecords returns cached records for a database.
func (c *Cache) GetRecords(databaseID jsonldb.ID) ([]*entity.DataRecord, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	records, ok := c.records[databaseID]
	return records, ok
}

// SetRecords caches records for a database.
func (c *Cache) SetRecords(databaseID jsonldb.ID, records []*entity.DataRecord) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.records) >= c.maxRecords {
		c.records = make(map[jsonldb.ID][]*entity.DataRecord)
	}
	c.records[databaseID] = records
}

// InvalidateRecords removes records for a database from cache.
func (c *Cache) InvalidateRecords(databaseID jsonldb.ID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.records, databaseID)
}

// InvalidateAll clears the entire cache.
func (c *Cache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nodeTree = nil
	c.records = make(map[jsonldb.ID][]*entity.DataRecord)
}
