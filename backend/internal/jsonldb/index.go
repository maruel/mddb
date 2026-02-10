// Provides concurrent-safe, in-memory secondary indexes for tables.

package jsonldb

import (
	"iter"
	"sync"

	"github.com/maruel/ksid"
)

// UniqueIndex provides O(1) lookup by a unique secondary key.
//
// The index is built from existing table data when created and kept
// synchronized via the [TableObserver] interface. All operations are
// concurrent-safe.
type UniqueIndex[K comparable, T Row[T]] struct {
	table   *Table[T]
	keyFunc func(T) K
	mu      sync.Mutex
	byKey   map[K]ksid.ID
}

// NewUniqueIndex creates a unique index on the given table.
//
// The keyFunc extracts the index key from each row. Keys must be unique;
// if duplicates exist in the table, the last row with each key wins.
func NewUniqueIndex[K comparable, T Row[T]](table *Table[T], keyFunc func(T) K) *UniqueIndex[K, T] {
	idx := &UniqueIndex[K, T]{
		table:   table,
		keyFunc: keyFunc,
		byKey:   make(map[K]ksid.ID),
	}
	table.AddObserver(idx)
	return idx
}

// Get returns the row with the given key, or nil if not found.
func (idx *UniqueIndex[K, T]) Get(key K) T {
	idx.mu.Lock()
	id, ok := idx.byKey[key]
	idx.mu.Unlock()
	if !ok {
		var zero T
		return zero
	}
	return idx.table.Get(id)
}

// OnAppend implements [TableObserver].
func (idx *UniqueIndex[K, T]) OnAppend(row T) {
	idx.mu.Lock()
	idx.byKey[idx.keyFunc(row)] = row.GetID()
	idx.mu.Unlock()
}

// OnUpdate implements [TableObserver].
func (idx *UniqueIndex[K, T]) OnUpdate(prev, curr T) {
	oldKey := idx.keyFunc(prev)
	newKey := idx.keyFunc(curr)
	idx.mu.Lock()
	if oldKey != newKey {
		delete(idx.byKey, oldKey)
	}
	idx.byKey[newKey] = curr.GetID()
	idx.mu.Unlock()
}

// OnDelete implements [TableObserver].
func (idx *UniqueIndex[K, T]) OnDelete(row T) {
	idx.mu.Lock()
	delete(idx.byKey, idx.keyFunc(row))
	idx.mu.Unlock()
}

// Index provides O(1) lookup by a non-unique secondary key.
//
// The index is built from existing table data when created and kept
// synchronized via the [TableObserver] interface. All operations are
// concurrent-safe.
type Index[K comparable, T Row[T]] struct {
	table   *Table[T]
	keyFunc func(T) K
	mu      sync.Mutex
	byKey   map[K]map[ksid.ID]struct{}
}

// NewIndex creates a non-unique index on the given table.
//
// The keyFunc extracts the index key from each row. Multiple rows
// may share the same key.
func NewIndex[K comparable, T Row[T]](table *Table[T], keyFunc func(T) K) *Index[K, T] {
	idx := &Index[K, T]{
		table:   table,
		keyFunc: keyFunc,
		byKey:   make(map[K]map[ksid.ID]struct{}),
	}
	table.AddObserver(idx)
	return idx
}

// Iter returns an iterator over all rows matching the given key.
func (idx *Index[K, T]) Iter(key K) iter.Seq[T] {
	return func(yield func(T) bool) {
		// Copy IDs under lock to avoid holding lock during iteration.
		idx.mu.Lock()
		ids := make([]ksid.ID, 0, len(idx.byKey[key]))
		for id := range idx.byKey[key] {
			ids = append(ids, id)
		}
		idx.mu.Unlock()

		for _, id := range ids {
			row := idx.table.Get(id)
			var zero T
			if any(row) == any(zero) {
				continue // Row was deleted between snapshot and lookup
			}
			if !yield(row) {
				return
			}
		}
	}
}

// OnAppend implements [TableObserver].
func (idx *Index[K, T]) OnAppend(row T) {
	key := idx.keyFunc(row)
	idx.mu.Lock()
	if idx.byKey[key] == nil {
		idx.byKey[key] = make(map[ksid.ID]struct{})
	}
	idx.byKey[key][row.GetID()] = struct{}{}
	idx.mu.Unlock()
}

// OnUpdate implements [TableObserver].
func (idx *Index[K, T]) OnUpdate(prev, curr T) {
	oldKey := idx.keyFunc(prev)
	newKey := idx.keyFunc(curr)
	id := curr.GetID()
	idx.mu.Lock()
	if oldKey != newKey {
		delete(idx.byKey[oldKey], id)
		if len(idx.byKey[oldKey]) == 0 {
			delete(idx.byKey, oldKey)
		}
	}
	if idx.byKey[newKey] == nil {
		idx.byKey[newKey] = make(map[ksid.ID]struct{})
	}
	idx.byKey[newKey][id] = struct{}{}
	idx.mu.Unlock()
}

// OnDelete implements [TableObserver].
func (idx *Index[K, T]) OnDelete(row T) {
	key := idx.keyFunc(row)
	idx.mu.Lock()
	delete(idx.byKey[key], row.GetID())
	if len(idx.byKey[key]) == 0 {
		delete(idx.byKey, key)
	}
	idx.mu.Unlock()
}
