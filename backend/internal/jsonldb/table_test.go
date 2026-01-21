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
	return ID(r.ID) //nolint:gosec // test code with small integers
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
	return ID(r.ID) //nolint:gosec // test code with small integers
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
	return ID(r.ID) //nolint:gosec // test code with small integers
}

func (r *alwaysInvalidRow) Validate() error {
	return errors.New("always invalid")
}

// setupTable creates a table in the test's temp directory.
func setupTable(t *testing.T) (table *Table[*testRow], path string) {
	path = filepath.Join(t.TempDir(), "test.jsonl")
	var err error
	table, err = NewTable[*testRow](path)
	if err != nil {
		t.Fatalf("NewTable failed: %v", err)
	}
	return table, path
}

// mockObserver records observer calls for testing.
type mockObserver struct {
	appends []int
	updates [][2]int // [prev, curr]
	deletes []int
}

func (m *mockObserver) OnAppend(row *testRow) {
	m.appends = append(m.appends, row.ID)
}

func (m *mockObserver) OnUpdate(prev, curr *testRow) {
	m.updates = append(m.updates, [2]int{prev.ID, curr.ID})
}

func (m *mockObserver) OnDelete(row *testRow) {
	m.deletes = append(m.deletes, row.ID)
}

func TestTable_Observers(t *testing.T) {
	table, _ := setupTable(t)

	obs := &mockObserver{}
	table.AddObserver(obs)

	// Test OnAppend
	if err := table.Append(&testRow{ID: 1, Name: "one"}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(obs.appends, []int{1}) {
		t.Errorf("OnAppend calls = %v, want [1]", obs.appends)
	}

	// Test OnUpdate
	if _, err := table.Update(&testRow{ID: 1, Name: "updated"}); err != nil {
		t.Fatal(err)
	}
	if len(obs.updates) != 1 || obs.updates[0] != [2]int{1, 1} {
		t.Errorf("OnUpdate calls = %v, want [[1,1]]", obs.updates)
	}

	// Test OnDelete
	if _, err := table.Delete(ID(1)); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(obs.deletes, []int{1}) {
		t.Errorf("OnDelete calls = %v, want [1]", obs.deletes)
	}
}

func TestTable_MultipleObservers(t *testing.T) {
	table, _ := setupTable(t)

	obs1 := &mockObserver{}
	obs2 := &mockObserver{}
	table.AddObserver(obs1)
	table.AddObserver(obs2)

	if err := table.Append(&testRow{ID: 1, Name: "one"}); err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(obs1.appends, []int{1}) {
		t.Errorf("obs1 OnAppend = %v, want [1]", obs1.appends)
	}
	if !slices.Equal(obs2.appends, []int{1}) {
		t.Errorf("obs2 OnAppend = %v, want [1]", obs2.appends)
	}
}

func TestTable_AddObserverWithExistingData(t *testing.T) {
	table, _ := setupTable(t)

	// Add data before observer
	if err := table.Append(&testRow{ID: 1, Name: "one"}); err != nil {
		t.Fatal(err)
	}
	if err := table.Append(&testRow{ID: 2, Name: "two"}); err != nil {
		t.Fatal(err)
	}

	obs := &mockObserver{}
	table.AddObserver(obs)

	// Observer should receive OnAppend for existing rows
	slices.Sort(obs.appends)
	if !slices.Equal(obs.appends, []int{1, 2}) {
		t.Errorf("OnAppend for existing = %v, want [1, 2]", obs.appends)
	}
}

func TestTable_AppendToReadOnlyDir(t *testing.T) {
	// Create a read-only directory
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "test.jsonl")

	// Don't create subdir - Append should fail when trying to create file
	table := &Table[*testRow]{
		path:   path,
		rows:   nil,
		byID:   make(map[ID]int),
		schema: schemaHeader{Version: "1.0"},
	}

	err := table.Append(&testRow{ID: 1, Name: "test"})
	if err == nil {
		t.Error("Append to non-existent directory should fail")
	}
}

func TestTable_UpdateNonExistentReturnsNil(t *testing.T) {
	table, _ := setupTable(t)

	prev, err := table.Update(&testRow{ID: 999, Name: "ghost"})
	if err != nil {
		t.Fatalf("Update error: %v", err)
	}
	if prev != nil {
		t.Errorf("Update non-existent returned %v, want nil", prev)
	}
}

func TestTable_DeleteNonExistentReturnsNil(t *testing.T) {
	table, _ := setupTable(t)

	deleted, err := table.Delete(ID(999))
	if err != nil {
		t.Fatalf("Delete error: %v", err)
	}
	if deleted != nil {
		t.Errorf("Delete non-existent returned %v, want nil", deleted)
	}
}

func TestTable_Modify(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		table, _ := setupTable(t)
		_ = table.Append(&testRow{ID: 1, Name: "original"})

		result, err := table.Modify(ID(1), func(row *testRow) error {
			row.Name = "modified"
			return nil
		})
		if err != nil {
			t.Fatalf("Modify error: %v", err)
		}
		if result.Name != "modified" {
			t.Errorf("Modify returned Name = %q, want %q", result.Name, "modified")
		}

		// Verify persisted
		got := table.Get(ID(1))
		if got.Name != "modified" {
			t.Errorf("Get after Modify = %q, want %q", got.Name, "modified")
		}
	})

	t.Run("not found", func(t *testing.T) {
		table, _ := setupTable(t)

		_, err := table.Modify(ID(999), func(row *testRow) error {
			return nil
		})
		if err == nil {
			t.Error("Modify non-existent should return error")
		}
	})

	t.Run("callback error", func(t *testing.T) {
		table, _ := setupTable(t)
		_ = table.Append(&testRow{ID: 1, Name: "original"})

		_, err := table.Modify(ID(1), func(row *testRow) error {
			return errors.New("callback failed")
		})
		if err == nil {
			t.Error("Modify with failing callback should return error")
		}

		// Verify unchanged
		got := table.Get(ID(1))
		if got.Name != "original" {
			t.Errorf("Row changed despite callback error: %q", got.Name)
		}
	})

	t.Run("validation error", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "test.jsonl")
		table, _ := NewTable[*validatingRow](path)
		_ = table.Append(&validatingRow{ID: 1, Name: "valid"})

		_, err := table.Modify(ID(1), func(row *validatingRow) error {
			row.FailValidate = true
			return nil
		})
		if err == nil {
			t.Error("Modify with invalid result should return error")
		}
	})

	t.Run("notifies observers", func(t *testing.T) {
		table, _ := setupTable(t)
		_ = table.Append(&testRow{ID: 1, Name: "original"})

		obs := &mockObserver{}
		table.AddObserver(obs)

		_, _ = table.Modify(ID(1), func(row *testRow) error {
			row.Name = "modified"
			return nil
		})

		if len(obs.updates) != 1 {
			t.Errorf("Observer updates = %d, want 1", len(obs.updates))
		}
	})

	t.Run("returns clone", func(t *testing.T) {
		table, _ := setupTable(t)
		_ = table.Append(&testRow{ID: 1, Name: "original"})

		result, _ := table.Modify(ID(1), func(row *testRow) error {
			row.Name = "modified"
			return nil
		})

		// Mutate returned value
		result.Name = "mutated"

		// Verify table unaffected
		got := table.Get(ID(1))
		if got.Name != "modified" {
			t.Errorf("Table affected by mutating returned clone: %q", got.Name)
		}
	})
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
					_ = table.Append(&testRow{ID: 1, Name: "One"})
				}, 1},
				{"two rows", func() {
					_ = table.Append(&testRow{ID: 2, Name: "Two"})
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

	t.Run("Get", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			table, _ := setupTable(t)

			// Add test data
			_ = table.Append(&testRow{ID: 10, Name: "Ten"})
			_ = table.Append(&testRow{ID: 20, Name: "Twenty"})

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

			_ = table.Append(&testRow{ID: 1, Name: "Original"})
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
			_ = table.Append(&testRow{ID: 1, Name: "One"})
			_ = table.Append(&testRow{ID: 2, Name: "Two"})
			_ = table.Append(&testRow{ID: 3, Name: "Three"})

			t.Run("delete existing row", func(t *testing.T) {
				deleted, err := table.Delete(ID(2))
				if err != nil {
					t.Fatalf("Delete error: %v", err)
				}
				if deleted == nil {
					t.Fatal("Delete() = nil, want deleted row for existing ID")
				}
				if deleted.ID != 2 || deleted.Name != "Two" {
					t.Errorf("Delete() returned %+v, want {ID:2, Name:Two}", deleted)
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
				if deleted != nil {
					t.Errorf("Delete() = %+v, want nil for non-existing ID", deleted)
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

			_ = table.Append(&testRow{ID: 1, Name: "One"})
			_ = table.Append(&testRow{ID: 2, Name: "Two"})

			deleted, err := table.Delete(ID(1))
			if err != nil {
				t.Fatalf("Delete error: %v", err)
			}
			if deleted == nil || deleted.ID != 1 {
				t.Errorf("Delete() = %+v, want {ID:1}", deleted)
			}

			// Verify index was rebuilt correctly
			got := table.Get(ID(2))
			if got == nil || got.ID != 2 {
				t.Error("Get(2) failed after deleting first row")
			}
		})

		t.Run("delete last row", func(t *testing.T) {
			table, _ := setupTable(t)

			_ = table.Append(&testRow{ID: 1, Name: "One"})
			_ = table.Append(&testRow{ID: 2, Name: "Two"})

			deleted, err := table.Delete(ID(2))
			if err != nil {
				t.Fatalf("Delete error: %v", err)
			}
			if deleted == nil || deleted.ID != 2 {
				t.Errorf("Delete() = %+v, want {ID:2}", deleted)
			}

			// Verify first row still accessible
			got := table.Get(ID(1))
			if got == nil || got.ID != 1 {
				t.Error("Get(1) failed after deleting last row")
			}
		})

		t.Run("returns clone", func(t *testing.T) {
			table, _ := setupTable(t)

			_ = table.Append(&testRow{ID: 1, Name: "Original"})
			_ = table.Append(&testRow{ID: 2, Name: "Two"})

			deleted, _ := table.Delete(ID(1))
			deleted.Name = "Modified"

			// Re-add and verify it's not affected by mutation
			_ = table.Append(&testRow{ID: 1, Name: "Readded"})
			got := table.Get(ID(1))
			if got.Name != "Readded" {
				t.Error("Delete() returned reference instead of clone")
			}
		})
	})

	t.Run("Update", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			table, path := setupTable(t)

			// Add test data
			_ = table.Append(&testRow{ID: 1, Name: "Original"})

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

			_ = table.Append(&validatingRow{ID: 1, Name: "Valid"})

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

				_ = table.Append(&testRow{ID: 1, Name: "One"})
				_ = table.Append(&testRow{ID: 2, Name: "Two"})

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
				_ = os.Mkdir(path, 0o750)

				_, err := NewTable[*testRow](path)
				if err == nil {
					t.Error("NewTable() expected error for directory, got nil")
				}
			})

			t.Run("invalid schema header", func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "bad-schema.jsonl")
				_ = os.WriteFile(path, []byte("not valid json\n"), 0o600)

				_, err := NewTable[*testRow](path)
				if err == nil {
					t.Error("NewTable() expected error for invalid schema, got nil")
				}
			})

			t.Run("invalid row data", func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "bad-row.jsonl")
				// Valid schema header, invalid row
				_ = os.WriteFile(path, []byte(`{"version":"1.0","columns":[]}
not valid json
`), 0o600)

				_, err := NewTable[*testRow](path)
				if err == nil {
					t.Error("NewTable() expected error for invalid row, got nil")
				}
			})

			t.Run("row with zero ID", func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "zero-id.jsonl")
				_ = os.WriteFile(path, []byte(`{"version":"1.0","columns":[]}
{"id":0,"name":"Zero"}
`), 0o600)

				_, err := NewTable[*testRow](path)
				if err == nil {
					t.Error("NewTable() expected error for zero ID row, got nil")
				}
			})

			t.Run("duplicate ID", func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "dup-id.jsonl")
				_ = os.WriteFile(path, []byte(`{"version":"1.0","columns":[]}
{"id":1,"name":"First"}
{"id":1,"name":"Duplicate"}
`), 0o600)

				_, err := NewTable[*testRow](path)
				if err == nil {
					t.Error("NewTable() expected error for duplicate ID, got nil")
				}
			})

			t.Run("invalid schema version", func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "bad-version.jsonl")
				_ = os.WriteFile(path, []byte(`{"version":"","columns":[]}
`), 0o600)

				_, err := NewTable[*testRow](path)
				if err == nil {
					t.Error("NewTable() expected error for empty version, got nil")
				}
			})

			t.Run("row fails validation on load", func(t *testing.T) {
				// Use alwaysInvalidRow which always fails validation
				path := filepath.Join(t.TempDir(), "invalid-row.jsonl")
				_ = os.WriteFile(path, []byte(`{"version":"1.0","columns":[]}
{"id":1,"name":"Test"}
`), 0o600)

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
				_ = table.Append(r)
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
				_ = table.Append(&testRow{ID: i, Name: "Row"})
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

			_ = table.Append(&testRow{ID: 1, Name: "Original"})

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

				_ = table.Append(&testRow{ID: 1, Name: "First"})
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
