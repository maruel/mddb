// In-memory bidirectional link index for backlink queries.

package content

import (
	"iter"
	"slices"
	"sync"

	"github.com/maruel/mddb/backend/internal/rid"
)

// linkCache maintains a bidirectional index of internal page links.
//
// It is lazily built on first access by scanning all pages, then kept
// up-to-date incrementally as pages are created, updated, or deleted.
//
// The forward map tracks source→targets so we can diff on update.
// The backward map tracks target→sources for O(1) backlink lookups.
type linkCache struct {
	mu       sync.RWMutex
	built    bool
	forward  map[rid.ID][]rid.ID // source → target IDs
	backward map[rid.ID][]rid.ID // target → source IDs
}

// buildLocked populates both maps by scanning all pages. Caller must hold mu for writing.
func (c *linkCache) buildLocked(iterPages func() (iter.Seq[*Node], error)) error {
	pages, err := iterPages()
	if err != nil {
		return err
	}
	c.forward = make(map[rid.ID][]rid.ID)
	c.backward = make(map[rid.ID][]rid.ID)
	for page := range pages {
		targets := ExtractLinkedNodeIDs(page.Content)
		if len(targets) == 0 {
			continue
		}
		c.forward[page.ID] = targets
		for _, t := range targets {
			c.backward[t] = append(c.backward[t], page.ID)
		}
	}
	c.built = true
	return nil
}

// ensureBuilt lazily initializes the cache on first access.
func (c *linkCache) ensureBuilt(iterPages func() (iter.Seq[*Node], error)) error {
	c.mu.RLock()
	if c.built {
		c.mu.RUnlock()
		return nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.built {
		return nil
	}
	return c.buildLocked(iterPages)
}

// update recomputes entries for sourceID based on its current content.
// Call after a page is created or updated.
func (c *linkCache) update(sourceID rid.ID, content string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.built {
		return // will be built lazily with current data
	}

	newTargets := ExtractLinkedNodeIDs(content)

	// Remove old backward entries.
	for _, old := range c.forward[sourceID] {
		c.removeBackwardLocked(old, sourceID)
	}

	// Update forward map.
	if len(newTargets) == 0 {
		delete(c.forward, sourceID)
	} else {
		c.forward[sourceID] = newTargets
	}

	// Add new backward entries.
	for _, t := range newTargets {
		c.backward[t] = append(c.backward[t], sourceID)
	}
}

// remove deletes all entries for a deleted page.
func (c *linkCache) remove(sourceID rid.ID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.built {
		return
	}
	for _, t := range c.forward[sourceID] {
		c.removeBackwardLocked(t, sourceID)
	}
	delete(c.forward, sourceID)
}

// backlinks returns source IDs that link to targetID.
// Must be called after ensureBuilt.
func (c *linkCache) backlinks(targetID rid.ID) []rid.ID {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.backward[targetID]
}

// forwardAll returns a snapshot of all forward links (source → targets).
func (c *linkCache) forwardAll() map[rid.ID][]rid.ID {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[rid.ID][]rid.ID, len(c.forward))
	for src, targets := range c.forward {
		out[src] = slices.Clone(targets)
	}
	return out
}

// removeBackwardLocked removes sourceID from the backward entry for targetID. Caller must hold mu for writing.
func (c *linkCache) removeBackwardLocked(targetID, sourceID rid.ID) {
	srcs := c.backward[targetID]
	for i, s := range srcs {
		if s == sourceID {
			c.backward[targetID] = append(srcs[:i], srcs[i+1:]...)
			if len(c.backward[targetID]) == 0 {
				delete(c.backward, targetID)
			}
			return
		}
	}
}
