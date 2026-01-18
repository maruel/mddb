package jsonldb

import (
	"os"
	"path/filepath"
	"testing"
)

type testRow struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func TestTable(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "jsonl-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	path := filepath.Join(tmpDir, "test.jsonl")

	// Test NewTable and Append
	table, err := NewTable[testRow](path)
	if err != nil {
		t.Fatalf("NewTable failed: %v", err)
	}

	rows := []testRow{
		{ID: 1, Name: "One"},
		{ID: 2, Name: "Two"},
	}

	for _, r := range rows {
		if err := table.Append(r); err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}

	if len(table.Rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(table.Rows))
	}

	// Test All
	all := table.All()
	if len(all) != 2 {
		t.Errorf("All() expected 2 rows, got %d", len(all))
	}

	// Test persistence (re-load)
	table2, err := NewTable[testRow](path)
	if err != nil {
		t.Fatalf("re-loading table failed: %v", err)
	}

	if len(table2.Rows) != 2 {
		t.Errorf("re-loaded table expected 2 rows, got %d", len(table2.Rows))
	}

	if table2.Rows[0].Name != "One" || table2.Rows[1].Name != "Two" {
		t.Errorf("re-loaded data mismatch: %+v", table2.Rows)
	}

	// Test Replace
	newRows := []testRow{
		{ID: 3, Name: "Three"},
	}
	if err := table.Replace(newRows); err != nil {
		t.Fatalf("Replace failed: %v", err)
	}

	if len(table.Rows) != 1 || table.Rows[0].ID != 3 {
		t.Errorf("Replace failed to update in-memory rows: %+v", table.Rows)
	}

	table3, err := NewTable[testRow](path)
	if err != nil {
		t.Fatalf("re-loading table after replace failed: %v", err)
	}
	if len(table3.Rows) != 1 || table3.Rows[0].ID != 3 {
		t.Errorf("Replace failed to update file: %+v", table3.Rows)
	}
}
