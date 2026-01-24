package jsonldb

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
)

var errZeroID = errors.New("row has zero ID")

// Row is implemented by types that can be stored in a [Table].
type Row[T any] interface {
	// Clone returns a deep copy of the row.
	//
	// Used to prevent mutations to cached data.
	Clone() T

	// GetID returns the unique identifier for this row.
	//
	// Must be non-zero.
	GetID() ID

	// Validate checks data integrity.
	//
	// Called on load and before every write. Return an error to reject invalid data.
	Validate() error
}

// TableObserver receives notifications about table mutations.
//
// Observers are called synchronously while the table lock is held.
// Implementations must not call back into the table or acquire locks
// that could cause deadlock.
type TableObserver[T Row[T]] interface {
	OnAppend(row T)
	OnUpdate(prev, curr T)
	OnDelete(row T)
}

// Table is a concurrent-safe, generic JSONL-backed data store with in-memory caching.
//
// All read and write operations are protected by a read-write mutex, making Table
// safe for concurrent use by multiple goroutines. Write operations (Append, Update,
// Delete) are atomic and immediately persisted to disk.
//
// The JSONL file format uses the first line as a schema header containing version
// and column definitions. Subsequent lines are JSON-encoded rows.
//
// Rows are stored in insertion order and indexed by ID for O(1) lookups.
// All returned rows are clones to prevent accidental mutation of cached data.
type Table[T Row[T]] struct {
	path         string
	mu           sync.RWMutex
	schema       schemaHeader
	rows         []T
	byID         map[ID]int // maps ID to index in rows
	blobRefCount map[BlobRef]int
	observers    []TableObserver[T]
	blobStore    blobStore // lazily initialized for tables with blob fields
}

// AddObserver registers an observer to receive mutation notifications.
//
// The observer is immediately called with OnAppend for each existing row,
// allowing indexes to be built from current table state.
// Observers are called while the table lock is held; see [TableObserver].
func (t *Table[T]) AddObserver(obs TableObserver[T]) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, row := range t.rows {
		obs.OnAppend(row)
	}
	t.observers = append(t.observers, obs)
}

// NewBlob creates a writer for streaming blob creation.
//
// Data is written to a temp file; Close() finalizes and returns a Blob
// that can be assigned to row fields before Append().
func (t *Table[T]) NewBlob() (*BlobWriter, error) {
	return t.blobStore.newBlob()
}

// deriveBlobDir returns the blob directory path for a table file.
// Example: mytable.jsonl → mytable.blobs/.
func deriveBlobDir(tablePath string) string {
	ext := filepath.Ext(tablePath)
	if ext != "" {
		return strings.TrimSuffix(tablePath, ext) + blobDirSuffix
	}
	return tablePath + blobDirSuffix
}

// Len returns the number of rows in the table.
func (t *Table[T]) Len() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.rows)
}

// Get returns a clone of the row with the given ID, or nil if not found.
func (t *Table[T]) Get(id ID) T {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if idx, ok := t.byID[id]; ok {
		return t.rows[idx].Clone()
	}
	var zero T
	return zero
}

// Delete removes a row by ID and persists the change.
//
// Returns the deleted row, or nil if no row with that ID exists.
// The entire table is rewritten to disk on success.
// Blobs are only deleted if no other rows reference them.
func (t *Table[T]) Delete(id ID) (T, error) {
	var zero T
	t.mu.Lock()
	defer t.mu.Unlock()

	idx, ok := t.byID[id]
	if !ok {
		return zero, nil
	}

	deleted := t.rows[idx]

	// Remove from slice
	t.rows = append(t.rows[:idx], t.rows[idx+1:]...)

	// Rebuild index (indices shifted after removal)
	t.byID = make(map[ID]int, len(t.rows))
	for i, row := range t.rows {
		t.byID[row.GetID()] = i
	}

	if err := t.saveLocked(); err != nil {
		return zero, err
	}

	// Decrement blob refcounts, delete blobs with refcount 0.
	if err := t.untrackBlobRefsLocked(deleted); err != nil {
		return zero, fmt.Errorf("failed to untrack blobs: %w", err)
	}

	for _, obs := range t.observers {
		obs.OnDelete(deleted)
	}
	return deleted, nil
}

// trackBlobRefsLocked increments the refcount for all blobs in the row. Caller must hold t.mu.
func (t *Table[T]) trackBlobRefsLocked(row T) {
	for _, blob := range blobFields(row) {
		if !blob.IsZero() {
			t.blobRefCount[blob.Ref]++
		}
	}
}

// untrackBlobRefsLocked decrements the refcount for all blobs in the row
// and removes blobs whose refcount reaches zero. Caller must hold t.mu.
func (t *Table[T]) untrackBlobRefsLocked(row T) error {
	var errs []error
	for _, blob := range blobFields(row) {
		if !blob.IsZero() {
			t.blobRefCount[blob.Ref]--
			if t.blobRefCount[blob.Ref] <= 0 {
				delete(t.blobRefCount, blob.Ref)
				if err := t.blobStore.remove(blob.Ref); err != nil {
					errs = append(errs, err)
				}
			}
		}
	}
	return errors.Join(errs...)
}

// Update replaces an existing row (matched by ID) and persists the change.
//
// Returns the previous row value, or nil if no row with that ID exists.
// Returns an error if validation fails. The entire table is rewritten to disk on success.
func (t *Table[T]) Update(row T) (T, error) {
	var zero T
	if err := row.Validate(); err != nil {
		return zero, fmt.Errorf("invalid row: %w", err)
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	idx, ok := t.byID[row.GetID()]
	if !ok {
		return zero, nil
	}

	prev := t.rows[idx]
	t.rows[idx] = row
	if err := t.saveLocked(); err != nil {
		return zero, err
	}

	// Update blob refcounts: track new first to avoid deleting shared blobs.
	t.trackBlobRefsLocked(row)
	if err := t.untrackBlobRefsLocked(prev); err != nil {
		return zero, fmt.Errorf("failed to untrack old blobs: %w", err)
	}

	for _, obs := range t.observers {
		obs.OnUpdate(prev, row)
	}
	return prev, nil
}

// Modify atomically reads, modifies, and writes a row.
//
// The callback fn receives a clone of the current row and should modify it.
// Returns the modified row on success, or an error if the row doesn't exist
// or validation fails.
//
// Modify uses pessimistic locking: the write lock is held for the entire
// operation (read, callback, validate, write). This guarantees the operation
// succeeds on the first attempt without retry loops, unlike optimistic CAS
// which may require retries under contention. The tradeoff is that fn should
// complete quickly to avoid blocking other operations.
//
// If fn returns an error, the row is not modified. If validation fails after
// fn returns, the row is not modified. If the disk write fails, the in-memory
// state is rolled back.
func (t *Table[T]) Modify(id ID, fn func(row T) error) (T, error) {
	var zero T
	t.mu.Lock()
	defer t.mu.Unlock()

	idx, ok := t.byID[id]
	if !ok {
		return zero, fmt.Errorf("row %s not found", id)
	}

	prev := t.rows[idx]
	row := prev.Clone()

	if err := fn(row); err != nil {
		return zero, err
	}
	if err := row.Validate(); err != nil {
		return zero, fmt.Errorf("invalid row after modify: %w", err)
	}

	t.rows[idx] = row
	if err := t.saveLocked(); err != nil {
		t.rows[idx] = prev // Rollback on save failure
		return zero, err
	}

	// Update blob refcounts: track new first to avoid deleting shared blobs.
	t.trackBlobRefsLocked(row)
	if err := t.untrackBlobRefsLocked(prev); err != nil {
		return zero, fmt.Errorf("failed to untrack old blobs: %w", err)
	}

	for _, obs := range t.observers {
		obs.OnUpdate(prev, row)
	}
	return row.Clone(), nil
}

// NewTable creates a Table and loads existing data from the JSONL file at path.
//
// If the file doesn't exist, an empty table is created and the schema is
// auto-discovered from type T via reflection.
// Returns an error if the file exists but cannot be read or contains invalid data.
func NewTable[T Row[T]](path string) (*Table[T], error) {
	table := &Table[T]{path: path}
	if err := table.load(); err != nil {
		return nil, err
	}
	// Initialize schema if not loaded (new table)
	if table.schema.Version == "" {
		columns, err := schemaFromType[T]()
		if err != nil {
			return nil, fmt.Errorf("failed to discover schema from type: %w", err)
		}
		table.schema = schemaHeader{
			Version: currentVersion,
			Columns: columns,
		}
	}
	return table, nil
}

func (t *Table[T]) load() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.blobStore.dir = deriveBlobDir(t.path)

	data, err := os.ReadFile(t.path)
	if err != nil {
		if os.IsNotExist(err) {
			t.byID = make(map[ID]int)
			t.blobRefCount = make(map[BlobRef]int)
			return nil
		}
		return fmt.Errorf("failed to read table file %s: %w", t.path, err)
	}

	n := bytes.Count(data, []byte{'\n'})
	t.rows = make([]T, 0, n)
	t.byID = make(map[ID]int, n)
	t.blobRefCount = make(map[BlobRef]int)
	lineNum := 0
	var prevID ID
	needsSort := false
	for line := range bytes.SplitSeq(data, []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		lineNum++

		// First line is the schema header
		if lineNum == 1 {
			if err := json.Unmarshal(line, &t.schema); err != nil {
				return fmt.Errorf("failed to unmarshal schema header in %s: %w", t.path, err)
			}
			if err := t.schema.Validate(); err != nil {
				return fmt.Errorf("invalid schema header in %s: %w", t.path, err)
			}
			continue
		}

		// Subsequent lines are rows
		var row T
		if err := json.Unmarshal(line, &row); err != nil {
			return fmt.Errorf("failed to unmarshal row in %s: %w", t.path, err)
		}
		// Inject blob store reference if row has blob fields
		t.injectBlobStoreLocked(row)
		// Track blob references for refcount
		t.trackBlobRefsLocked(row)
		if err := row.Validate(); err != nil {
			return fmt.Errorf("invalid row in %s line %d: %w", t.path, lineNum, err)
		}
		id := row.GetID()
		if id.IsZero() {
			return fmt.Errorf("row in %s line %d has zero ID", t.path, lineNum)
		}
		if _, exists := t.byID[id]; exists {
			return fmt.Errorf("duplicate ID %s in %s line %d", id, t.path, lineNum)
		}
		// Check if rows are in sorted order (needed for Iter with startID)
		if id < prevID {
			needsSort = true
		}
		prevID = id
		t.byID[id] = len(t.rows)
		t.rows = append(t.rows, row)
	}

	// Sort by ID if rows were out of order (e.g., clock drift, manual editing)
	if needsSort {
		slices.SortFunc(t.rows, func(a, b T) int {
			return a.GetID().Compare(b.GetID())
		})
		// Rebuild index after sorting
		for i, row := range t.rows {
			t.byID[row.GetID()] = i
		}
	}

	// Clean up orphaned blob files.
	if err := t.blobStore.gc(t.blobRefCount); err != nil {
		return fmt.Errorf("failed to run blob GC: %w", err)
	}
	return nil
}

// injectBlobStoreLocked sets the store reference on all blob fields in the row. Caller must hold t.mu.
func (t *Table[T]) injectBlobStoreLocked(row T) {
	for _, blob := range blobFields(row) {
		blob.store = &t.blobStore
	}
}

// Iter returns an iterator over clones of rows with ID strictly greater than startID.
//
// Pass 0 to iterate over all rows from the beginning.
// The reader lock is held for the duration of iteration; avoid long-running
// operations inside the loop to prevent blocking writers.
func (t *Table[T]) Iter(startID ID) iter.Seq[T] {
	return func(yield func(T) bool) {
		t.mu.RLock()
		defer t.mu.RUnlock()

		startIdx := 0
		if !startID.IsZero() {
			// Find the first row with ID > startID.
			// This assumes t.rows is sorted by ID.
			startIdx = sort.Search(len(t.rows), func(i int) bool {
				return t.rows[i].GetID().Compare(startID) > 0
			})
		}

		for _, row := range t.rows[startIdx:] {
			if !yield(row.Clone()) {
				return
			}
		}
	}
}

// Append adds a new row to the table and persists it.
//
// Returns an error if the row fails validation, has a zero ID, or has a duplicate ID.
// If the new row's ID is less than the last row's ID (e.g., clock drift), the row is
// inserted at the correct position and the entire file is rewritten to maintain sorted order.
func (t *Table[T]) Append(row T) (err error) {
	if err := row.Validate(); err != nil {
		return fmt.Errorf("invalid row: %w", err)
	}
	id := row.GetID()
	if id.IsZero() {
		return errZeroID
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if _, exists := t.byID[id]; exists {
		return fmt.Errorf("duplicate ID %s", id)
	}

	// Check if new row breaks sorted order (e.g., clock drift)
	if len(t.rows) > 0 && id < t.rows[len(t.rows)-1].GetID() {
		// Find insertion point via binary search
		idx := sort.Search(len(t.rows), func(i int) bool {
			return t.rows[i].GetID() >= id
		})
		// Insert at correct position
		t.rows = slices.Insert(t.rows, idx, row)
		// Update indices for shifted rows
		for i := idx; i < len(t.rows); i++ {
			t.byID[t.rows[i].GetID()] = i
		}
		// Rewrite entire file in sorted order
		if err := t.saveLocked(); err != nil {
			return fmt.Errorf("failed to save table: %w", err)
		}
	} else {
		// Normal case: append to end of file
		if _, err := os.Stat(t.path); os.IsNotExist(err) {
			if err := t.saveSchemaHeaderLocked(); err != nil {
				return fmt.Errorf("failed to write schema header: %w", err)
			}
		}

		data, err := json.Marshal(row)
		if err != nil {
			return fmt.Errorf("failed to marshal row: %w", err)
		}

		f, err := os.OpenFile(t.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) //nolint:gosec // G302: 0o644 is intentional for user data files
		if err != nil {
			return fmt.Errorf("failed to open table file for append: %w", err)
		}
		defer func() {
			if cerr := f.Close(); cerr != nil && err == nil {
				err = fmt.Errorf("failed to close table file: %w", cerr)
			}
		}()
		if _, err := f.Write(append(data, '\n')); err != nil {
			return fmt.Errorf("failed to write row: %w", err)
		}

		t.byID[id] = len(t.rows)
		t.rows = append(t.rows, row)
	}

	// Track blob references.
	t.trackBlobRefsLocked(row)

	for _, obs := range t.observers {
		obs.OnAppend(row)
	}
	return nil
}

// saveSchemaHeaderLocked writes just the schema header as the first line. Caller must hold t.mu.
func (t *Table[T]) saveSchemaHeaderLocked() (err error) {
	f, err := os.Create(t.path)
	if err != nil {
		return fmt.Errorf("failed to create table file: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("failed to close table file: %w", cerr)
		}
	}()

	writer := bufio.NewWriter(f)
	headerData, err := json.Marshal(t.schema)
	if err != nil {
		return fmt.Errorf("failed to marshal schema header: %w", err)
	}
	if _, err := writer.Write(headerData); err != nil {
		return fmt.Errorf("failed to write schema header: %w", err)
	}
	if err := writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush writer: %w", err)
	}
	return nil
}

// saveLocked writes the schema header and all rows to the file. Caller must hold t.mu.
func (t *Table[T]) saveLocked() (err error) {
	f, err := os.Create(t.path)
	if err != nil {
		return fmt.Errorf("failed to create table file: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("failed to close table file: %w", cerr)
		}
	}()

	writer := bufio.NewWriter(f)

	// Write schema header as first line
	headerData, err := json.Marshal(t.schema)
	if err != nil {
		return fmt.Errorf("failed to marshal schema header: %w", err)
	}
	if _, err := writer.Write(headerData); err != nil {
		return fmt.Errorf("failed to write schema header: %w", err)
	}
	if err := writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}

	// Write rows
	for _, row := range t.rows {
		data, err := json.Marshal(row)
		if err != nil {
			return fmt.Errorf("failed to marshal row: %w", err)
		}
		if _, err := writer.Write(data); err != nil {
			return fmt.Errorf("failed to write row: %w", err)
		}
		if err := writer.WriteByte('\n'); err != nil {
			return fmt.Errorf("failed to write newline: %w", err)
		}
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush writer: %w", err)
	}
	return nil
}
