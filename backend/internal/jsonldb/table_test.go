package jsonldb

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

type testRow struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func (r testRow) Clone() testRow {
	return r
}

func (r testRow) GetID() ID {
	return ID(r.ID)
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

	if table.Len() != 2 {
		t.Errorf("expected 2 rows, got %d", table.Len())
	}

	// Test All
	all := slices.Collect(table.All())
	if len(all) != 2 {
		t.Errorf("All() expected 2 rows, got %d", len(all))
	}

	// Test persistence (re-load)
	table2, err := NewTable[testRow](path)
	if err != nil {
		t.Fatalf("re-loading table failed: %v", err)
	}

	if table2.Len() != 2 {
		t.Errorf("re-loaded table expected 2 rows, got %d", table2.Len())
	}

	all2 := slices.Collect(table2.All())
	if all2[0].Name != "One" || all2[1].Name != "Two" {
		t.Errorf("re-loaded data mismatch: %v, %v", all2[0], all2[1])
	}

	// Test Replace
	newRows := []testRow{
		{ID: 3, Name: "Three"},
	}
	if err := table.Replace(newRows); err != nil {
		t.Fatalf("Replace failed: %v", err)
	}

	allAfterReplace := slices.Collect(table.All())
	if table.Len() != 1 || allAfterReplace[0].ID != 3 {
		t.Errorf("Replace failed to update in-memory rows: len=%d", table.Len())
	}

	table3, err := NewTable[testRow](path)
	if err != nil {
		t.Fatalf("re-loading table after replace failed: %v", err)
	}
	all3 := slices.Collect(table3.All())
	if table3.Len() != 1 || all3[0].ID != 3 {
		t.Errorf("Replace failed to update file: len=%d", table3.Len())
	}
}
