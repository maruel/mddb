package jsonldb

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"iter"
	"os"
	"sync"
)

// Cloner is implemented by types that can clone themselves.
type Cloner[T any] interface {
	Clone() T
}

// Table handles storage and in-memory caching for a single table in JSONL format.
type Table[T Cloner[T]] struct {
	path string
	mu   sync.RWMutex

	rows []T
}

// Len returns the number of rows.
func (t *Table[T]) Len() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.rows)
}

// Last returns a clone of the last row, or false if empty.
func (t *Table[T]) Last() (T, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if len(t.rows) == 0 {
		var zero T
		return zero, false
	}
	return t.rows[len(t.rows)-1].Clone(), true
}

// NewTable creates a new Table and loads all data from the file.
func NewTable[T Cloner[T]](path string) (*Table[T], error) {
	table := &Table[T]{path: path}
	if err := table.load(); err != nil {
		return nil, err
	}
	return table, nil
}

func (t *Table[T]) load() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	data, err := os.ReadFile(t.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read table file %s: %w", t.path, err)
	}

	n := bytes.Count(data, []byte{'\n'})
	t.rows = make([]T, 0, n)
	for line := range bytes.SplitSeq(data, []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		var row T
		if err := json.Unmarshal(line, &row); err != nil {
			return fmt.Errorf("failed to unmarshal row in %s: %w", t.path, err)
		}
		t.rows = append(t.rows, row)
	}
	return nil
}

// All returns an iterator over clones of all rows.
func (t *Table[T]) All() iter.Seq[T] {
	return func(yield func(T) bool) {
		t.mu.RLock()
		defer t.mu.RUnlock()
		for _, row := range t.rows {
			if !yield(row.Clone()) {
				return
			}
		}
	}
}

// Append adds a new row to the table and persists it.
func (t *Table[T]) Append(row T) error {
	t.mu.Lock()
	defer t.mu.Unlock()
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
	t.rows = append(t.rows, row)
	return nil
}

// Replace replaces all rows with the provided slice and persists it.
func (t *Table[T]) Replace(rows []T) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.rows = rows
	return t.save()
}

// save writes all rows to the file. Caller must hold t.mu.
func (t *Table[T]) save() error {
	f, err := os.Create(t.path)
	if err != nil {
		return fmt.Errorf("failed to create table file: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	writer := bufio.NewWriter(f)
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
