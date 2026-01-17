package storage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// JSONLTable handles storage and in-memory caching for a single table in JSONL format.
type JSONLTable[T any] struct {
	path string
	mu   sync.RWMutex

rows []T
}

// NewJSONLTable creates a new JSONLTable and loads all data from the file.
func NewJSONLTable[T any](path string) (*JSONLTable[T], error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create directory for %s: %w", path, err)
	}

	table := &JSONLTable[T]{
		path: path,
	}

	if err := table.load(); err != nil {
		return nil, err
	}

	return table, nil
}

func (t *JSONLTable[T]) load() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	f, err := os.Open(t.path)
	if err != nil {
		if os.IsNotExist(err) {
			t.rows = []T{}
			return nil
		}
		return fmt.Errorf("failed to open table file %s: %w", t.path, err)
	}
	defer f.Close()

	var rows []T
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var row T
		if err := json.Unmarshal(line, &row); err != nil {
			return fmt.Errorf("failed to unmarshal row in %s: %w", t.path, err)
		}
		rows = append(rows, row)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to read table file %s: %w", t.path, err)
	}

	t.rows = rows
	return nil
}

// All returns a copy of all rows.
func (t *JSONLTable[T]) All() []T {
	t.mu.RLock()
	defer t.mu.RUnlock()
	rows := make([]T, len(t.rows))
	copy(rows, t.rows)
	return rows
}

// Append adds a new row to the table and persists it.
func (t *JSONLTable[T]) Append(row T) error {
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
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("failed to write row: %w", err)
	}
	if _, err := f.WriteString("\n"); err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}

	t.rows = append(t.rows, row)
	return nil
}

// Replace replaces all rows with the provided slice and persists it.
func (t *JSONLTable[T]) Replace(rows []T) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	f, err := os.Create(t.path)
	if err != nil {
		return fmt.Errorf("failed to create table file: %w", err)
	}
	defer f.Close()

	writer := bufio.NewWriter(f)
	for _, row := range rows {
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

	t.rows = rows
	return nil
}
