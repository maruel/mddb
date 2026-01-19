package jsonldb

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// testRow is a simple row type for testing.
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

// validatingRow is a row type that can fail validation programmatically.
type validatingRow struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	FailValidate bool   `json:"-"` // If true, Validate() returns error (not serialized)
}

func (r *validatingRow) Clone() *validatingRow {
	c := *r
	return &c
}

func (r *validatingRow) GetID() ID {
	return ID(r.ID)
}

func (r *validatingRow) Validate() error {
	if r.FailValidate {
		return errors.New("validation failed")
	}
	return nil
}

// alwaysInvalidRow is a row type that always fails validation.
type alwaysInvalidRow struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func (r *alwaysInvalidRow) Clone() *alwaysInvalidRow {
	c := *r
	return &c
}

func (r *alwaysInvalidRow) GetID() ID {
	return ID(r.ID)
}

func (r *alwaysInvalidRow) Validate() error {
	return errors.New("always invalid")
}

// setupTable creates a table in the test's temp directory.
func setupTable(t *testing.T) (*Table[*testRow], string) {
	path := filepath.Join(t.TempDir(), "test.jsonl")
	table, err := NewTable[*testRow](path)
	if err != nil {
		t.Fatalf("NewTable failed: %v", err)
	}
	return table, path
}

// TestTable tests all Table methods using table-driven tests.
func TestTable(t *testing.T) {
	t.Run("Len", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			table, _ := setupTable(t)

			tests := []struct {
				name    string
				setup   func()
				wantLen int
			}{
				{"empty table", func() {}, 0},
				{"one row", func() {
					table.Append(&testRow{ID: 1, Name: "One"})
				}, 1},
				{"two rows", func() {
					table.Append(&testRow{ID: 2, Name: "Two"})
				}, 2},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					tt.setup()
					if got := table.Len(); got != tt.wantLen {
						t.Errorf("Len() = %d, want %d", got, tt.wantLen)
					}
				})
			}
		})
	})

	t.Run("Last", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			table, _ := setupTable(t)

			// Test empty table returns zero value
			t.Run("empty table", func(t *testing.T) {
				last := table.Last()
				if last != nil {
					t.Errorf("Last() on empty table = %v, want nil", last)
				}
			})

			// Add rows and test Last returns correct row
			table.Append(&testRow{ID: 1, Name: "First"})
			t.Run("single row", func(t *testing.T) {
				last := table.Last()
				if last == nil || last.ID != 1 || last.Name != "First" {
					t.Errorf("Last() = %+v, want {ID:1, Name:First}", last)
				}
			})

			table.Append(&testRow{ID: 2, Name: "Second"})
			t.Run("multiple rows", func(t *testing.T) {
				last := table.Last()
				if last == nil || last.ID != 2 || last.Name != "Second" {
					t.Errorf("Last() = %+v, want {ID:2, Name:Second}", last)
				}
			})

			// Verify Last returns a clone (mutation doesn't affect table)
			t.Run("returns clone", func(t *testing.T) {
				last := table.Last()
				last.Name = "Modified"
				lastAgain := table.Last()
				if lastAgain.Name == "Modified" {
					t.Error("Last() returned reference instead of clone")
				}
			})
		})
	})

	t.Run("Get", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			table, _ := setupTable(t)

			// Add test data
			table.Append(&testRow{ID: 10, Name: "Ten"})
			table.Append(&testRow{ID: 20, Name: "Twenty"})

			tests := []struct {
				name   string
				id     ID
				wantID int
				found  bool
			}{
				{"existing ID", ID(10), 10, true},
				{"existing ID 2", ID(20), 20, true},
				{"non-existing ID", ID(999), 0, false},
				{"zero ID", ID(0), 0, false},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					got := table.Get(tt.id)
					if tt.found {
						if got == nil || got.ID != tt.wantID {
							t.Errorf("Get(%d) = %+v, want ID=%d", tt.id, got, tt.wantID)
						}
					} else {
						if got != nil {
							t.Errorf("Get(%d) = %+v, want nil", tt.id, got)
						}
					}
				})
			}
		})

		t.Run("returns clone", func(t *testing.T) {
			table, _ := setupTable(t)

			table.Append(&testRow{ID: 1, Name: "Original"})
			got := table.Get(ID(1))
			got.Name = "Modified"

			gotAgain := table.Get(ID(1))
			if gotAgain.Name == "Modified" {
				t.Error("Get() returned reference instead of clone")
			}
		})
	})

	t.Run("Delete", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			table, path := setupTable(t)

			// Add test data
			table.Append(&testRow{ID: 1, Name: "One"})
			table.Append(&testRow{ID: 2, Name: "Two"})
			table.Append(&testRow{ID: 3, Name: "Three"})

			t.Run("delete existing row", func(t *testing.T) {
				deleted, err := table.Delete(ID(2))
				if err != nil {
					t.Fatalf("Delete error: %v", err)
				}
				if !deleted {
					t.Error("Delete() = false, want true for existing ID")
				}
				if table.Len() != 2 {
					t.Errorf("Len() = %d, want 2 after delete", table.Len())
				}
				if table.Get(ID(2)) != nil {
					t.Error("Deleted row still accessible via Get")
				}
			})

			t.Run("delete non-existing row", func(t *testing.T) {
				deleted, err := table.Delete(ID(999))
				if err != nil {
					t.Fatalf("Delete error: %v", err)
				}
				if deleted {
					t.Error("Delete() = true, want false for non-existing ID")
				}
			})

			t.Run("persistence after delete", func(t *testing.T) {
				// Reload table and verify persistence
				table2, err := NewTable[*testRow](path)
				if err != nil {
					t.Fatalf("NewTable error: %v", err)
				}
				if table2.Len() != 2 {
					t.Errorf("Reloaded table Len() = %d, want 2", table2.Len())
				}
				if table2.Get(ID(2)) != nil {
					t.Error("Deleted row still present after reload")
				}
			})
		})

		t.Run("delete first row", func(t *testing.T) {
			table, _ := setupTable(t)

			table.Append(&testRow{ID: 1, Name: "One"})
			table.Append(&testRow{ID: 2, Name: "Two"})

			deleted, err := table.Delete(ID(1))
			if err != nil {
				t.Fatalf("Delete error: %v", err)
			}
			if !deleted {
				t.Error("Delete() = false, want true")
			}

			// Verify index was rebuilt correctly
			got := table.Get(ID(2))
			if got == nil || got.ID != 2 {
				t.Error("Get(2) failed after deleting first row")
			}
		})

		t.Run("delete last row", func(t *testing.T) {
			table, _ := setupTable(t)

			table.Append(&testRow{ID: 1, Name: "One"})
			table.Append(&testRow{ID: 2, Name: "Two"})

			deleted, err := table.Delete(ID(2))
			if err != nil {
				t.Fatalf("Delete error: %v", err)
			}
			if !deleted {
				t.Error("Delete() = false, want true")
			}

			// Verify first row still accessible
			got := table.Get(ID(1))
			if got == nil || got.ID != 1 {
				t.Error("Get(1) failed after deleting last row")
			}
		})
	})

	t.Run("Update", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			table, path := setupTable(t)

			// Add test data
			table.Append(&testRow{ID: 1, Name: "Original"})

			t.Run("update existing row", func(t *testing.T) {
				prev, err := table.Update(&testRow{ID: 1, Name: "Updated"})
				if err != nil {
					t.Fatalf("Update error: %v", err)
				}
				if prev == nil || prev.Name != "Original" {
					t.Errorf("Update() returned prev = %+v, want Name=Original", prev)
				}

				got := table.Get(ID(1))
				if got == nil || got.Name != "Updated" {
					t.Errorf("Get() after Update = %+v, want Name=Updated", got)
				}
			})

			t.Run("update non-existing row", func(t *testing.T) {
				prev, err := table.Update(&testRow{ID: 999, Name: "New"})
				if err != nil {
					t.Fatalf("Update error: %v", err)
				}
				if prev != nil {
					t.Errorf("Update() for non-existing returned %+v, want nil", prev)
				}
			})

			t.Run("persistence after update", func(t *testing.T) {
				table2, err := NewTable[*testRow](path)
				if err != nil {
					t.Fatalf("NewTable error: %v", err)
				}
				got := table2.Get(ID(1))
				if got == nil || got.Name != "Updated" {
					t.Errorf("Reloaded row = %+v, want Name=Updated", got)
				}
			})
		})

		t.Run("errors", func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "test.jsonl")
			table, err := NewTable[*validatingRow](path)
			if err != nil {
				t.Fatalf("NewTable failed: %v", err)
			}

			table.Append(&validatingRow{ID: 1, Name: "Valid"})

			t.Run("validation error", func(t *testing.T) {
				_, err := table.Update(&validatingRow{ID: 1, Name: "Invalid", FailValidate: true})
				if err == nil {
					t.Error("Update() expected validation error, got nil")
				}
			})
		})
	})

	t.Run("NewTable", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			t.Run("creates new table", func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "new.jsonl")
				table, err := NewTable[*testRow](path)
				if err != nil {
					t.Fatalf("NewTable error: %v", err)
				}
				if table.Len() != 0 {
					t.Errorf("New table Len() = %d, want 0", table.Len())
				}
			})

			t.Run("loads existing table", func(t *testing.T) {
				table, path := setupTable(t)

				table.Append(&testRow{ID: 1, Name: "One"})
				table.Append(&testRow{ID: 2, Name: "Two"})

				table2, err := NewTable[*testRow](path)
				if err != nil {
					t.Fatalf("NewTable error: %v", err)
				}
				if table2.Len() != 2 {
					t.Errorf("Reloaded table Len() = %d, want 2", table2.Len())
				}
			})
		})

		t.Run("errors", func(t *testing.T) {
			t.Run("unreadable file", func(t *testing.T) {
				// Create a directory where we expect a file
				path := filepath.Join(t.TempDir(), "not-a-file")
				os.Mkdir(path, 0o755)

				_, err := NewTable[*testRow](path)
				if err == nil {
					t.Error("NewTable() expected error for directory, got nil")
				}
			})

			t.Run("invalid schema header", func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "bad-schema.jsonl")
				os.WriteFile(path, []byte("not valid json\n"), 0o644)

				_, err := NewTable[*testRow](path)
				if err == nil {
					t.Error("NewTable() expected error for invalid schema, got nil")
				}
			})

			t.Run("invalid row data", func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "bad-row.jsonl")
				// Valid schema header, invalid row
				os.WriteFile(path, []byte(`{"version":"1.0","columns":[]}
not valid json
`), 0o644)

				_, err := NewTable[*testRow](path)
				if err == nil {
					t.Error("NewTable() expected error for invalid row, got nil")
				}
			})

			t.Run("row with zero ID", func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "zero-id.jsonl")
				os.WriteFile(path, []byte(`{"version":"1.0","columns":[]}
{"id":0,"name":"Zero"}
`), 0o644)

				_, err := NewTable[*testRow](path)
				if err == nil {
					t.Error("NewTable() expected error for zero ID row, got nil")
				}
			})

			t.Run("duplicate ID", func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "dup-id.jsonl")
				os.WriteFile(path, []byte(`{"version":"1.0","columns":[]}
{"id":1,"name":"First"}
{"id":1,"name":"Duplicate"}
`), 0o644)

				_, err := NewTable[*testRow](path)
				if err == nil {
					t.Error("NewTable() expected error for duplicate ID, got nil")
				}
			})

			t.Run("invalid schema version", func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "bad-version.jsonl")
				os.WriteFile(path, []byte(`{"version":"","columns":[]}
`), 0o644)

				_, err := NewTable[*testRow](path)
				if err == nil {
					t.Error("NewTable() expected error for empty version, got nil")
				}
			})

			t.Run("row fails validation on load", func(t *testing.T) {
				// Use alwaysInvalidRow which always fails validation
				path := filepath.Join(t.TempDir(), "invalid-row.jsonl")
				os.WriteFile(path, []byte(`{"version":"1.0","columns":[]}
{"id":1,"name":"Test"}
`), 0o644)

				_, err := NewTable[*alwaysInvalidRow](path)
				if err == nil {
					t.Error("NewTable() expected error for invalid row, got nil")
				}
			})
		})
	})

	t.Run("Iter", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			table, _ := setupTable(t)

			iterRows := []*testRow{
				{ID: 10, Name: "Ten"},
				{ID: 20, Name: "Twenty"},
				{ID: 30, Name: "Thirty"},
				{ID: 40, Name: "Forty"},
			}
			for _, r := range iterRows {
				table.Append(r)
			}

			tests := []struct {
				name      string
				startID   ID
				wantCount int
				wantFirst int
			}{
				{"all rows", 0, 4, 10},
				{"from ID 10", ID(10), 3, 20},
				{"from ID 25", ID(25), 2, 30},
				{"from ID 40", ID(40), 0, 0},
				{"from ID beyond max", ID(100), 0, 0},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					result := slices.Collect(table.Iter(tt.startID))
					if len(result) != tt.wantCount {
						t.Errorf("Iter(%d) returned %d rows, want %d", tt.startID, len(result), tt.wantCount)
					}
					if tt.wantCount > 0 && result[0].ID != tt.wantFirst {
						t.Errorf("Iter(%d) first ID = %d, want %d", tt.startID, result[0].ID, tt.wantFirst)
					}
				})
			}
		})

		t.Run("early termination", func(t *testing.T) {
			table, _ := setupTable(t)

			for i := 1; i <= 10; i++ {
				table.Append(&testRow{ID: i, Name: "Row"})
			}

			count := 0
			for range table.Iter(0) {
				count++
				if count >= 3 {
					break
				}
			}

			if count != 3 {
				t.Errorf("Early termination count = %d, want 3", count)
			}
		})

		t.Run("returns clones", func(t *testing.T) {
			table, _ := setupTable(t)

			table.Append(&testRow{ID: 1, Name: "Original"})

			for row := range table.Iter(0) {
				row.Name = "Modified"
			}

			got := table.Get(ID(1))
			if got.Name == "Modified" {
				t.Error("Iter returned reference instead of clone")
			}
		})
	})

	t.Run("Append", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			table, path := setupTable(t)

			t.Run("append to empty table", func(t *testing.T) {
				err := table.Append(&testRow{ID: 1, Name: "First"})
				if err != nil {
					t.Fatalf("Append error: %v", err)
				}
				if table.Len() != 1 {
					t.Errorf("Len() = %d, want 1", table.Len())
				}
			})

			t.Run("append multiple rows", func(t *testing.T) {
				err := table.Append(&testRow{ID: 2, Name: "Second"})
				if err != nil {
					t.Fatalf("Append error: %v", err)
				}
				if table.Len() != 2 {
					t.Errorf("Len() = %d, want 2", table.Len())
				}
			})

			t.Run("persistence after append", func(t *testing.T) {
				table2, err := NewTable[*testRow](path)
				if err != nil {
					t.Fatalf("NewTable error: %v", err)
				}
				if table2.Len() != 2 {
					t.Errorf("Reloaded table Len() = %d, want 2", table2.Len())
				}
			})
		})

		t.Run("errors", func(t *testing.T) {
			t.Run("zero ID", func(t *testing.T) {
				table, _ := setupTable(t)

				err := table.Append(&testRow{ID: 0, Name: "Zero"})
				if err == nil {
					t.Error("Append() expected error for zero ID, got nil")
				}
			})

			t.Run("duplicate ID", func(t *testing.T) {
				table, _ := setupTable(t)

				table.Append(&testRow{ID: 1, Name: "First"})
				err := table.Append(&testRow{ID: 1, Name: "Duplicate"})
				if err == nil {
					t.Error("Append() expected error for duplicate ID, got nil")
				}
			})

			t.Run("validation error", func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "test.jsonl")
				table, err := NewTable[*validatingRow](path)
				if err != nil {
					t.Fatalf("NewTable failed: %v", err)
				}

				err = table.Append(&validatingRow{ID: 1, Name: "Invalid", FailValidate: true})
				if err == nil {
					t.Error("Append() expected validation error, got nil")
				}
			})
		})
	})

	t.Run("Replace", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			table, path := setupTable(t)

			// Add initial data
			table.Append(&testRow{ID: 1, Name: "One"})
			table.Append(&testRow{ID: 2, Name: "Two"})

			t.Run("replace all rows", func(t *testing.T) {
				newRows := []*testRow{
					{ID: 10, Name: "Ten"},
					{ID: 20, Name: "Twenty"},
					{ID: 30, Name: "Thirty"},
				}
				err := table.Replace(newRows)
				if err != nil {
					t.Fatalf("Replace error: %v", err)
				}

				if table.Len() != 3 {
					t.Errorf("Len() = %d, want 3", table.Len())
				}

				// Old rows should be gone
				if table.Get(ID(1)) != nil {
					t.Error("Old row 1 still present after Replace")
				}

				// New rows should be present
				if table.Get(ID(10)) == nil {
					t.Error("New row 10 not present after Replace")
				}
			})

			t.Run("replace with empty slice", func(t *testing.T) {
				err := table.Replace([]*testRow{})
				if err != nil {
					t.Fatalf("Replace error: %v", err)
				}
				if table.Len() != 0 {
					t.Errorf("Len() = %d, want 0", table.Len())
				}
			})

			t.Run("persistence after replace", func(t *testing.T) {
				table.Replace([]*testRow{{ID: 100, Name: "Hundred"}})

				table2, err := NewTable[*testRow](path)
				if err != nil {
					t.Fatalf("NewTable error: %v", err)
				}
				if table2.Len() != 1 {
					t.Errorf("Reloaded table Len() = %d, want 1", table2.Len())
				}
				got := table2.Get(ID(100))
				if got == nil || got.Name != "Hundred" {
					t.Error("Replaced row not persisted correctly")
				}
			})
		})

		t.Run("errors", func(t *testing.T) {
			t.Run("duplicate ID in replacement", func(t *testing.T) {
				table, _ := setupTable(t)

				err := table.Replace([]*testRow{
					{ID: 1, Name: "First"},
					{ID: 1, Name: "Duplicate"},
				})
				if err == nil {
					t.Error("Replace() expected error for duplicate ID, got nil")
				}
			})
		})
	})
}

// TestRow tests the Row interface through Table operations.
// The Row interface is tested indirectly through the testRow and validatingRow types.
func TestRow(t *testing.T) {
	t.Run("Clone", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			original := &testRow{ID: 1, Name: "Original"}
			cloned := original.Clone()

			if cloned == original {
				t.Error("Clone returned same pointer")
			}
			if cloned.ID != original.ID || cloned.Name != original.Name {
				t.Error("Clone has different values")
			}

			// Modify clone and verify original unchanged
			cloned.Name = "Modified"
			if original.Name == "Modified" {
				t.Error("Modifying clone affected original")
			}
		})
	})

	t.Run("GetID", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			tests := []struct {
				name string
				row  *testRow
				want ID
			}{
				{"zero ID", &testRow{ID: 0}, 0},
				{"positive ID", &testRow{ID: 42}, 42},
				{"large ID", &testRow{ID: 1000000}, 1000000},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					if got := tt.row.GetID(); got != tt.want {
						t.Errorf("GetID() = %d, want %d", got, tt.want)
					}
				})
			}
		})
	})

	t.Run("Validate", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			row := &testRow{ID: 1, Name: "Test"}
			if err := row.Validate(); err != nil {
				t.Errorf("Validate() error = %v, want nil", err)
			}
		})

		t.Run("errors", func(t *testing.T) {
			row := &validatingRow{ID: 1, Name: "Test", FailValidate: true}
			if err := row.Validate(); err == nil {
				t.Error("Validate() expected error, got nil")
			}
		})
	})
}
