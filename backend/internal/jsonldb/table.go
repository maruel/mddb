// Implements the concurrent-safe Table[T] for JSONL storage.

package jsonldb

import (
	"bufio"
	"bytes"
	"crypto/rand"
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

	"github.com/maruel/ksid"
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
	GetID() ksid.ID

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
	byID         map[ksid.ID]int // maps ID to index in rows
	blobRefCount map[BlobRef]int
	observers    []TableObserver[T]
	blobStore    blobStore // lazily initialized for tables with blob fields
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

// Properties returns the application-specific schema properties stored in the header.
// Returns nil if no properties are set.
func (t *Table[T]) Properties() json.RawMessage {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return slices.Clone(t.schema.Properties)
}

// SetProperties sets the application-specific schema properties and persists the change.
// The entire table is rewritten to update the header.
func (t *Table[T]) SetProperties(props json.RawMessage) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	prev := t.schema.Properties
	t.schema.Properties = slices.Clone(props)
	if err := t.saveLocked(); err != nil {
		t.schema.Properties = prev
		return err
	}
	return nil
}

// Get returns a clone of the row with the given ID, or nil if not found.
func (t *Table[T]) Get(id ksid.ID) T {
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
// A blob cleanup error returns the deleted row with an error describing the
// already persisted deletion; observers have received the deletion.
func (t *Table[T]) Delete(id ksid.ID) (T, error) {
	var zero T
	t.mu.Lock()
	defer t.mu.Unlock()

	idx, ok := t.byID[id]
	if !ok {
		return zero, nil
	}

	deleted := t.rows[idx]

	// Keep the original backing array intact until publication succeeds.
	prevRows := t.rows
	t.rows = slices.Delete(slices.Clone(t.rows), idx, idx+1)
	if err := t.saveLocked(); err != nil {
		t.rows = prevRows
		return zero, err
	}
	for i := idx; i < len(t.rows); i++ {
		t.byID[t.rows[i].GetID()] = i
	}
	delete(t.byID, id)

	// Cleanup can fail after publication; observers must still see the deletion.
	cleanupErr := t.untrackBlobRefsLocked(deleted)

	for _, obs := range t.observers {
		obs.OnDelete(deleted)
	}
	if cleanupErr != nil {
		return deleted, fmt.Errorf("row deletion persisted, but blob cleanup failed: %w", cleanupErr)
	}
	return deleted, nil
}

// DeleteFunc deletes every row for which del returns true and returns them in
// table order. It rewrites the table once, so bulk cleanup costs one rewrite
// instead of one per row. del runs under the table lock: it must not call the
// table or modify the row. Publication, blob cleanup, and observer semantics
// match Delete.
func (t *Table[T]) DeleteFunc(del func(T) bool) ([]T, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	kept := make([]T, 0, len(t.rows))
	var deleted []T
	for _, row := range t.rows {
		if del(row) {
			deleted = append(deleted, row)
		} else {
			kept = append(kept, row)
		}
	}
	if len(deleted) == 0 {
		return nil, nil
	}

	prevRows := t.rows
	t.rows = kept
	if err := t.saveLocked(); err != nil {
		t.rows = prevRows
		return nil, err
	}
	for _, row := range deleted {
		delete(t.byID, row.GetID())
	}
	for i, row := range t.rows {
		t.byID[row.GetID()] = i
	}

	// Cleanup can fail after publication; observers must still see the deletions.
	var cleanupErr error
	for _, row := range deleted {
		cleanupErr = errors.Join(cleanupErr, t.untrackBlobRefsLocked(row))
		for _, obs := range t.observers {
			obs.OnDelete(row)
		}
	}
	if cleanupErr != nil {
		return deleted, fmt.Errorf("row deletion persisted, but blob cleanup failed: %w", cleanupErr)
	}
	return deleted, nil
}

// UpdateFunc applies update to a clone of every row and persists the rows it
// changed with one rewrite, so bulk maintenance costs one rewrite instead of
// one per row. update reports whether it changed its clone and must keep the
// row's ID; it runs under the table lock and must not call the table. It
// returns how many rows changed. Validation, publication, blob cleanup, and
// observer semantics match Update.
func (t *Table[T]) UpdateFunc(update func(row T) bool) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	next := slices.Clone(t.rows)
	var changed []int
	for i, row := range t.rows {
		c := row.Clone()
		if !update(c) {
			continue
		}
		if c.GetID() != row.GetID() {
			return 0, fmt.Errorf("update changed row ID %s to %s", row.GetID(), c.GetID())
		}
		if err := c.Validate(); err != nil {
			return 0, fmt.Errorf("invalid row after update: %w", err)
		}
		next[i] = c
		changed = append(changed, i)
	}
	if len(changed) == 0 {
		return 0, nil
	}

	prevRows := t.rows
	t.rows = next
	if err := t.saveLocked(); err != nil {
		t.rows = prevRows
		return 0, err
	}

	// Track new references first so a blob shared by the old and new row survives.
	var cleanupErr error
	for _, i := range changed {
		t.trackBlobRefsLocked(next[i])
		cleanupErr = errors.Join(cleanupErr, t.untrackBlobRefsLocked(prevRows[i]))
		for _, obs := range t.observers {
			obs.OnUpdate(prevRows[i], next[i])
		}
	}
	if cleanupErr != nil {
		return len(changed), fmt.Errorf("row update persisted, but blob cleanup failed: %w", cleanupErr)
	}
	return len(changed), nil
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
// A blob cleanup error returns the previous row with an error describing the
// already persisted update; observers have received the update.
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
		t.rows[idx] = prev
		return zero, err
	}

	// Update blob refcounts: track new first to avoid deleting shared blobs.
	t.trackBlobRefsLocked(row)
	cleanupErr := t.untrackBlobRefsLocked(prev)

	for _, obs := range t.observers {
		obs.OnUpdate(prev, row)
	}
	if cleanupErr != nil {
		return prev, fmt.Errorf("row update persisted, but blob cleanup failed: %w", cleanupErr)
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
// state is rolled back. A blob cleanup error returns the modified row with an
// error describing the already persisted modification; observers have received it.
func (t *Table[T]) Modify(id ksid.ID, fn func(row T) error) (T, error) {
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
	cleanupErr := t.untrackBlobRefsLocked(prev)

	for _, obs := range t.observers {
		obs.OnUpdate(prev, row)
	}
	if cleanupErr != nil {
		return row.Clone(), fmt.Errorf("row modification persisted, but blob cleanup failed: %w", cleanupErr)
	}
	return row.Clone(), nil
}

func (t *Table[T]) load() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.blobStore.dir = deriveBlobDir(t.path)

	data, err := os.ReadFile(t.path)
	if err != nil {
		if os.IsNotExist(err) {
			t.byID = make(map[ksid.ID]int)
			t.blobRefCount = make(map[BlobRef]int)
			return nil
		}
		return fmt.Errorf("failed to read table file %s: %w", t.path, err)
	}

	n := bytes.Count(data, []byte{'\n'})
	t.rows = make([]T, 0, n)
	t.byID = make(map[ksid.ID]int, n)
	t.blobRefCount = make(map[BlobRef]int)
	lineNum := 0
	var prevID ksid.ID
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
// Row references are snapshotted under the reader lock, then cloned as yielded
// after unlocking. Writers replace stored rows rather than mutate them in place,
// so callers can read or write this table during iteration without blocking.
func (t *Table[T]) Iter(startID ksid.ID) iter.Seq[T] {
	return func(yield func(T) bool) {
		t.mu.RLock()

		startIdx := 0
		if !startID.IsZero() {
			// Find the first row with ID > startID.
			// This assumes t.rows is sorted by ID.
			startIdx = sort.Search(len(t.rows), func(i int) bool {
				return t.rows[i].GetID().Compare(startID) > 0
			})
		}

		rows := slices.Clone(t.rows[startIdx:])
		t.mu.RUnlock()

		for _, row := range rows {
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
// A close error after a successful append reports that the row was persisted;
// cached rows and observers already include the row. Failed partial writes are
// replaced with the previous snapshot; a failed rollback explicitly reports
// incomplete stored state.
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
		prevRows := t.rows
		t.rows = slices.Insert(slices.Clone(t.rows), idx, row)
		if err := t.saveLocked(); err != nil {
			t.rows = prevRows
			return fmt.Errorf("failed to save table: %w", err)
		}
		// Update indices only after the new snapshot is published.
		for i := idx; i < len(t.rows); i++ {
			t.byID[t.rows[i].GetID()] = i
		}
	} else {
		// Normal case: append to end of file
		if _, err := os.Stat(t.path); os.IsNotExist(err) {
			if err := t.saveLocked(); err != nil {
				return fmt.Errorf("failed to write schema header: %w", err)
			}
		}

		data, marshalErr := json.Marshal(row)
		if marshalErr != nil {
			return fmt.Errorf("failed to marshal row: %w", marshalErr)
		}

		f, openErr := os.OpenFile(t.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) //nolint:gosec // G302: 0o644 is intentional for user data files
		if openErr != nil {
			return fmt.Errorf("failed to open table file for append: %w", openErr)
		}
		published := false
		defer func() {
			if cerr := f.Close(); cerr != nil {
				if published {
					err = errors.Join(err, fmt.Errorf("row append persisted, but closing table file failed: %w", cerr))
				} else {
					err = errors.Join(err, fmt.Errorf("failed to close table file: %w", cerr))
				}
			}
		}()
		if _, writeErr := f.Write(append(data, '\n')); writeErr != nil {
			// A failed Write may have published a prefix of the row. Replace it
			// with the cached snapshot without truncating a concurrent reader.
			err := fmt.Errorf("failed to write row: %w", writeErr)
			if rollbackErr := t.saveLocked(); rollbackErr != nil {
				err = errors.Join(err, fmt.Errorf("failed to restore table after append; stored state may be incomplete: %w", rollbackErr))
			}
			return err
		}
		published = true

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

// saveLocked writes the schema header and all rows to the file. Caller must hold t.mu.
func (t *Table[T]) saveLocked() (err error) {
	// Git reads tables concurrently with request mutations and may mmap them.
	// Replace the file instead of truncating an inode another reader still owns.
	tmp := filepath.Join(filepath.Dir(t.path), ".jsonldb-"+rand.Text())
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644) //nolint:gosec // G302: 0o644 is intentional for user data files
	if err != nil {
		return fmt.Errorf("failed to create table file: %w", err)
	}
	defer func() {
		if tmp != "" {
			err = errors.Join(err, os.Remove(tmp))
		}
	}()
	info, statErr := os.Stat(t.path)
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		err = statErr
	} else if info != nil {
		err = f.Chmod(info.Mode().Perm())
	}
	if err == nil {
		err = t.writeLocked(f)
	}
	if err := errors.Join(err, f.Close()); err != nil {
		return err
	}
	if err := os.Rename(tmp, t.path); err != nil {
		return fmt.Errorf("failed to replace table file: %w", err)
	}
	tmp = ""
	return nil
}

// writeLocked encodes a complete table snapshot. Caller must hold t.mu.
func (t *Table[T]) writeLocked(f *os.File) error {
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
