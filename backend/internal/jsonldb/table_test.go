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

func (r *testRow) Clone() *testRow {
	c := *r
	return &c
}

func (r *testRow) GetID() ID {
	return ID(r.ID)
}

func (r *testRow) Validate() error {
	return nil
}

func TestTable(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "jsonl-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	path := filepath.Join(tmpDir, "test.jsonl")

	// Test NewTable and Append
	table, err := NewTable[*testRow](path)
	if err != nil {
		t.Fatalf("NewTable failed: %v", err)
	}

	rows := []testRow{
		{ID: 1, Name: "One"},
		{ID: 2, Name: "Two"},
	}

	for _, r := range rows {
		// Pass address of r (creates a copy implicitly if we take address of loop var?
		// No, we should take address of a copy to be safe, though Append clones immediately
		// if implementation stores clones, but here we pass *testRow.
		// Table.Append takes *testRow.
		// We need to be careful with loop variable reuse in Go < 1.22, but we are on 1.25.
		rCopy := r
		if err := table.Append(&rCopy); err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}

	if table.Len() != 2 {
		t.Errorf("expected 2 rows, got %d", table.Len())
	}

	// Test All
	all := slices.Collect(table.Iter(0))
	if len(all) != 2 {
		t.Errorf("All() expected 2 rows, got %d", len(all))
	}

	// Test persistence (re-load)
	table2, err := NewTable[*testRow](path)
	if err != nil {
		t.Fatalf("re-loading table failed: %v", err)
	}

	if table2.Len() != 2 {
		t.Errorf("re-loaded table expected 2 rows, got %d", table2.Len())
	}

	all2 := slices.Collect(table2.Iter(0))
	if all2[0].Name != "One" || all2[1].Name != "Two" {
		t.Errorf("re-loaded data mismatch: %v, %v", all2[0], all2[1])
	}

	// Test Replace
	newRows := []*testRow{
		{ID: 3, Name: "Three"},
	}
	if err := table.Replace(newRows); err != nil {
		t.Fatalf("Replace failed: %v", err)
	}

	allAfterReplace := slices.Collect(table.Iter(0))
	if table.Len() != 1 || allAfterReplace[0].ID != 3 {
		t.Errorf("Replace failed to update in-memory rows: len=%d", table.Len())
	}

	table3, err := NewTable[*testRow](path)
	if err != nil {
		t.Fatalf("re-loading table after replace failed: %v", err)
	}
	all3 := slices.Collect(table3.Iter(0))
	if table3.Len() != 1 || all3[0].ID != 3 {
		t.Errorf("Replace failed to update file: len=%d", table3.Len())
	}

	// Test Iter
	// Reset table with known sorted data
	iterRows := []*testRow{
		{ID: 10, Name: "Ten"},
		{ID: 20, Name: "Twenty"},
		{ID: 30, Name: "Thirty"},
		{ID: 40, Name: "Forty"},
	}
	if err := table.Replace(iterRows); err != nil {
		t.Fatalf("Replace for Iter failed: %v", err)
	}

	// Case 1: Iter(0) -> All
	iterAll := slices.Collect(table.Iter(0))
	if len(iterAll) != 4 {
		t.Errorf("Iter(0) expected 4 rows, got %d", len(iterAll))
	}

	// Case 2: Iter(10) -> 20, 30, 40
	iterFrom10 := slices.Collect(table.Iter(ID(10)))
	if len(iterFrom10) != 3 {
		t.Errorf("Iter(10) expected 3 rows, got %d", len(iterFrom10))
	}
	if iterFrom10[0].ID != 20 {
		t.Errorf("Iter(10) first item expected 20, got %d", iterFrom10[0].ID)
	}

	// Case 3: Iter(25) -> 30, 40
	iterFrom25 := slices.Collect(table.Iter(ID(25)))
	if len(iterFrom25) != 2 {
		t.Errorf("Iter(25) expected 2 rows, got %d", len(iterFrom25))
	}
	if iterFrom25[0].ID != 30 {
		t.Errorf("Iter(25) first item expected 30, got %d", iterFrom25[0].ID)
	}

	// Case 4: Iter(40) -> Empty
	iterFrom40 := slices.Collect(table.Iter(ID(40)))
	if len(iterFrom40) != 0 {
		t.Errorf("Iter(40) expected 0 rows, got %d", len(iterFrom40))
	}
}
