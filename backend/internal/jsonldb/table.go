package jsonldb

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"iter"
	"os"
	"sort"
	"sync"
)

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

// Table is a concurrent-safe, generic JSONL-backed data store with in-memory caching.
//
// All read and write operations are protected by a read-write mutex, making Table
// safe for concurrent use by multiple goroutines. Write operations (Append, Update,
// Delete, Replace) are atomic and immediately persisted to disk.
//
// The JSONL file format uses the first line as a schema header containing version
// and column definitions. Subsequent lines are JSON-encoded rows.
//
// Rows are stored in insertion order and indexed by ID for O(1) lookups.
// All returned rows are clones to prevent accidental mutation of cached data.
type Table[T Row[T]] struct {
	path   string
	mu     sync.RWMutex
	schema schemaHeader
	rows   []T
	byID   map[ID]int // maps ID to index in rows
}

// Len returns the number of rows in the table.
func (t *Table[T]) Len() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.rows)
}

// Last returns a clone of the most recently appended row.
//
// Returns false if the table is empty.
func (t *Table[T]) Last() (T, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if len(t.rows) == 0 {
		var zero T
		return zero, false
	}
	return t.rows[len(t.rows)-1].Clone(), true
}

// Get returns a clone of the row with the given ID.
//
// Returns false if no row with that ID exists.
func (t *Table[T]) Get(id ID) (T, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if idx, ok := t.byID[id]; ok {
		return t.rows[idx].Clone(), true
	}
	var zero T
	return zero, false
}

// Delete removes a row by ID and persists the change.
//
// Returns true if the row existed and was deleted, false if no row with that ID exists.
// The entire table is rewritten to disk on success.
func (t *Table[T]) Delete(id ID) (bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	idx, ok := t.byID[id]
	if !ok {
		return false, nil
	}

	// Remove from slice
	t.rows = append(t.rows[:idx], t.rows[idx+1:]...)

	// Rebuild index (indices shifted after removal)
	t.byID = make(map[ID]int, len(t.rows))
	for i, row := range t.rows {
		t.byID[row.GetID()] = i
	}

	if err := t.save(); err != nil {
		return false, err
	}
	return true, nil
}

// Update replaces an existing row (matched by ID) and persists the change.
//
// Returns true if the row existed and was updated, false if no row with that ID exists.
// Returns an error if validation fails. The entire table is rewritten to disk on success.
func (t *Table[T]) Update(row T) (bool, error) {
	if err := row.Validate(); err != nil {
		return false, fmt.Errorf("invalid row: %w", err)
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	idx, ok := t.byID[row.GetID()]
	if !ok {
		return false, nil
	}

	t.rows[idx] = row
	if err := t.save(); err != nil {
		return false, err
	}
	return true, nil
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

	data, err := os.ReadFile(t.path)
	if err != nil {
		if os.IsNotExist(err) {
			t.byID = make(map[ID]int)
			return nil
		}
		return fmt.Errorf("failed to read table file %s: %w", t.path, err)
	}

	n := bytes.Count(data, []byte{'\n'})
	t.rows = make([]T, 0, n)
	t.byID = make(map[ID]int, n)
	lineNum := 0
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
		t.byID[id] = len(t.rows)
		t.rows = append(t.rows, row)
	}
	return nil
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

// Append adds a new row to the table and persists it by appending to the file.
//
// Returns an error if the row fails validation, has a zero ID, or has a duplicate ID.
// If the file doesn't exist, it is created with a schema header first.
func (t *Table[T]) Append(row T) error {
	if err := row.Validate(); err != nil {
		return fmt.Errorf("invalid row: %w", err)
	}
	id := row.GetID()
	if id.IsZero() {
		return fmt.Errorf("row has zero ID")
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if _, exists := t.byID[id]; exists {
		return fmt.Errorf("duplicate ID %s", id)
	}

	// If file doesn't exist, write schema header first
	if _, err := os.Stat(t.path); os.IsNotExist(err) {
		if err := t.saveSchemaHeaderLocked(); err != nil {
			return fmt.Errorf("failed to write schema header: %w", err)
		}
	}

	data, err := json.Marshal(row)
	if err != nil {
		return fmt.Errorf("failed to marshal row: %w", err)
	}

	f, err := os.OpenFile(t.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open table file for append: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()
	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to write row: %w", err)
	}
	t.byID[id] = len(t.rows)
	t.rows = append(t.rows, row)
	return nil
}

// Replace atomically replaces all rows with the provided slice and rewrites the file.
//
// Returns an error if any row has a duplicate ID.
// Use this for bulk updates or when row order needs to change.
func (t *Table[T]) Replace(rows []T) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Rebuild the index, checking for duplicates
	byID := make(map[ID]int, len(rows))
	for i, row := range rows {
		id := row.GetID()
		if _, exists := byID[id]; exists {
			return fmt.Errorf("duplicate ID %s", id)
		}
		byID[id] = i
	}
	t.byID = byID
	t.rows = rows
	return t.save()
}

// saveSchemaHeaderLocked writes just the schema header as the first line. Caller must hold t.mu.
func (t *Table[T]) saveSchemaHeaderLocked() error {
	f, err := os.Create(t.path)
	if err != nil {
		return fmt.Errorf("failed to create table file: %w", err)
	}
	defer func() {
		_ = f.Close()
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

// save writes the schema header and all rows to the file. Caller must hold t.mu.
func (t *Table[T]) save() error {
	f, err := os.Create(t.path)
	if err != nil {
		return fmt.Errorf("failed to create table file: %w", err)
	}
	defer func() {
		_ = f.Close()
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
