package jsonldb

import (
	"iter"
	"path/filepath"
	"slices"
	"testing"

	"github.com/maruel/ksid"
)

func TestUniqueIndex(t *testing.T) {
	t.Run("Basic", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "test.jsonl")
		table, err := NewTable[*testRow](path)
		if err != nil {
			t.Fatal(err)
		}

		// Create index on Name field
		byName := NewUniqueIndex(table, func(r *testRow) string { return r.Name })

		// Test empty index
		if got := byName.Get("alice"); got != nil {
			t.Errorf("Get on empty index = %v, want nil", got)
		}

		// Append rows
		if err := table.Append(&testRow{ID: 1, Name: "alice"}); err != nil {
			t.Fatal(err)
		}
		if err := table.Append(&testRow{ID: 2, Name: "bob"}); err != nil {
			t.Fatal(err)
		}

		// Test lookup
		if got := byName.Get("alice"); got == nil || got.ID != 1 {
			t.Errorf("Get(alice) = %v, want ID=1", got)
		}
		if got := byName.Get("bob"); got == nil || got.ID != 2 {
			t.Errorf("Get(bob) = %v, want ID=2", got)
		}
		if got := byName.Get("charlie"); got != nil {
			t.Errorf("Get(charlie) = %v, want nil", got)
		}

		// Test update with key change
		if _, err := table.Update(&testRow{ID: 1, Name: "alice-renamed"}); err != nil {
			t.Fatal(err)
		}
		if got := byName.Get("alice"); got != nil {
			t.Errorf("Get(alice) after rename = %v, want nil", got)
		}
		if got := byName.Get("alice-renamed"); got == nil || got.ID != 1 {
			t.Errorf("Get(alice-renamed) = %v, want ID=1", got)
		}

		// Test update without key change
		if _, err := table.Update(&testRow{ID: 2, Name: "bob"}); err != nil {
			t.Fatal(err)
		}
		if got := byName.Get("bob"); got == nil || got.ID != 2 {
			t.Errorf("Get(bob) after no-op update = %v, want ID=2", got)
		}

		// Test delete
		if _, err := table.Delete(ksid.ID(1)); err != nil {
			t.Fatal(err)
		}
		if got := byName.Get("alice-renamed"); got != nil {
			t.Errorf("Get(alice-renamed) after delete = %v, want nil", got)
		}
		if got := byName.Get("bob"); got == nil || got.ID != 2 {
			t.Errorf("Get(bob) after unrelated delete = %v, want ID=2", got)
		}
	})

	t.Run("ExistingData", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "test.jsonl")
		table, err := NewTable[*testRow](path)
		if err != nil {
			t.Fatal(err)
		}

		// Add data before creating index
		if err := table.Append(&testRow{ID: 1, Name: "alice"}); err != nil {
			t.Fatal(err)
		}
		if err := table.Append(&testRow{ID: 2, Name: "bob"}); err != nil {
			t.Fatal(err)
		}

		// Create index - should build from existing data
		byName := NewUniqueIndex(table, func(r *testRow) string { return r.Name })

		if got := byName.Get("alice"); got == nil || got.ID != 1 {
			t.Errorf("Get(alice) = %v, want ID=1", got)
		}
		if got := byName.Get("bob"); got == nil || got.ID != 2 {
			t.Errorf("Get(bob) = %v, want ID=2", got)
		}
	})

	t.Run("UpdateSameKey", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "test.jsonl")
		table, err := NewTable[*testRow](path)
		if err != nil {
			t.Fatal(err)
		}

		// Index by first letter (unique in this test)
		byFirstLetter := NewUniqueIndex(table, func(r *testRow) string {
			return string(r.Name[0])
		})

		if err := table.Append(&testRow{ID: 1, Name: "alice"}); err != nil {
			t.Fatal(err)
		}

		// Update without changing the indexed key
		if _, err := table.Update(&testRow{ID: 1, Name: "amy"}); err != nil {
			t.Fatal(err)
		}

		if got := byFirstLetter.Get("a"); got == nil || got.ID != 1 {
			t.Errorf("Get(a) after same-key update = %v, want ID=1", got)
		}
	})
}

func TestIndex(t *testing.T) {
	t.Run("Basic", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "test.jsonl")
		table, err := NewTable[*testRow](path)
		if err != nil {
			t.Fatal(err)
		}

		// Create non-unique index (simulating grouping by first letter)
		byFirstLetter := NewIndex(table, func(r *testRow) string {
			if r.Name == "" {
				return ""
			}
			return string(r.Name[0])
		})

		// Test empty index
		count := 0
		for range byFirstLetter.Iter("a") {
			count++
		}
		if count != 0 {
			t.Errorf("Iter on empty index returned %d rows, want 0", count)
		}

		// Append rows
		if err := table.Append(&testRow{ID: 1, Name: "alice"}); err != nil {
			t.Fatal(err)
		}
		if err := table.Append(&testRow{ID: 2, Name: "adam"}); err != nil {
			t.Fatal(err)
		}
		if err := table.Append(&testRow{ID: 3, Name: "bob"}); err != nil {
			t.Fatal(err)
		}

		// Test lookup - multiple rows with same key
		ids := collectIDs(byFirstLetter.Iter("a"))
		slices.Sort(ids)
		if !slices.Equal(ids, []int{1, 2}) {
			t.Errorf("Iter(a) = %v, want [1, 2]", ids)
		}

		// Test lookup - single row
		ids = collectIDs(byFirstLetter.Iter("b"))
		if !slices.Equal(ids, []int{3}) {
			t.Errorf("Iter(b) = %v, want [3]", ids)
		}

		// Test update with key change
		if _, err := table.Update(&testRow{ID: 1, Name: "zelda"}); err != nil {
			t.Fatal(err)
		}
		ids = collectIDs(byFirstLetter.Iter("a"))
		if !slices.Equal(ids, []int{2}) {
			t.Errorf("Iter(a) after update = %v, want [2]", ids)
		}
		ids = collectIDs(byFirstLetter.Iter("z"))
		if !slices.Equal(ids, []int{1}) {
			t.Errorf("Iter(z) = %v, want [1]", ids)
		}

		// Test delete
		if _, err := table.Delete(ksid.ID(2)); err != nil {
			t.Fatal(err)
		}
		ids = collectIDs(byFirstLetter.Iter("a"))
		if len(ids) != 0 {
			t.Errorf("Iter(a) after delete = %v, want []", ids)
		}
	})

	t.Run("ExistingData", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "test.jsonl")
		table, err := NewTable[*testRow](path)
		if err != nil {
			t.Fatal(err)
		}

		// Add data before creating index
		if err := table.Append(&testRow{ID: 1, Name: "alice"}); err != nil {
			t.Fatal(err)
		}
		if err := table.Append(&testRow{ID: 2, Name: "adam"}); err != nil {
			t.Fatal(err)
		}

		// Create index - should build from existing data
		byFirstLetter := NewIndex(table, func(r *testRow) string {
			return string(r.Name[0])
		})

		ids := collectIDs(byFirstLetter.Iter("a"))
		slices.Sort(ids)
		if !slices.Equal(ids, []int{1, 2}) {
			t.Errorf("Iter(a) = %v, want [1, 2]", ids)
		}
	})

	t.Run("IterEarlyTermination", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "test.jsonl")
		table, err := NewTable[*testRow](path)
		if err != nil {
			t.Fatal(err)
		}

		byFirstLetter := NewIndex(table, func(r *testRow) string {
			return string(r.Name[0])
		})

		// Add multiple rows with same key
		for i := 1; i <= 5; i++ {
			if err := table.Append(&testRow{ID: i, Name: "a" + string(rune('0'+i))}); err != nil {
				t.Fatal(err)
			}
		}

		// Test early termination - stop after first row
		count := 0
		for range byFirstLetter.Iter("a") {
			count++
			if count >= 2 {
				break
			}
		}
		if count != 2 {
			t.Errorf("Early termination count = %d, want 2", count)
		}
	})

	t.Run("UpdateSameKey", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "test.jsonl")
		table, err := NewTable[*testRow](path)
		if err != nil {
			t.Fatal(err)
		}

		byFirstLetter := NewIndex(table, func(r *testRow) string {
			return string(r.Name[0])
		})

		if err := table.Append(&testRow{ID: 1, Name: "alice"}); err != nil {
			t.Fatal(err)
		}

		// Update without changing the indexed key (still starts with 'a')
		if _, err := table.Update(&testRow{ID: 1, Name: "amy"}); err != nil {
			t.Fatal(err)
		}

		ids := collectIDs(byFirstLetter.Iter("a"))
		if !slices.Equal(ids, []int{1}) {
			t.Errorf("Iter(a) after same-key update = %v, want [1]", ids)
		}
	})

	t.Run("DeleteLastRowWithKey", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "test.jsonl")
		table, err := NewTable[*testRow](path)
		if err != nil {
			t.Fatal(err)
		}

		byFirstLetter := NewIndex(table, func(r *testRow) string {
			return string(r.Name[0])
		})

		// Add single row with key "a"
		if err := table.Append(&testRow{ID: 1, Name: "alice"}); err != nil {
			t.Fatal(err)
		}

		// Delete it - should clean up the key entirely
		if _, err := table.Delete(ksid.ID(1)); err != nil {
			t.Fatal(err)
		}

		// Iter should return nothing
		ids := collectIDs(byFirstLetter.Iter("a"))
		if len(ids) != 0 {
			t.Errorf("Iter(a) after delete last = %v, want []", ids)
		}
	})

	t.Run("UpdateRemovesLastRowFromOldKey", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "test.jsonl")
		table, err := NewTable[*testRow](path)
		if err != nil {
			t.Fatal(err)
		}

		byFirstLetter := NewIndex(table, func(r *testRow) string {
			return string(r.Name[0])
		})

		// Add single row with key "a"
		if err := table.Append(&testRow{ID: 1, Name: "alice"}); err != nil {
			t.Fatal(err)
		}

		// Update to change key from "a" to "z"
		if _, err := table.Update(&testRow{ID: 1, Name: "zelda"}); err != nil {
			t.Fatal(err)
		}

		// "a" should have no rows now
		ids := collectIDs(byFirstLetter.Iter("a"))
		if len(ids) != 0 {
			t.Errorf("Iter(a) after update = %v, want []", ids)
		}

		// "z" should have the row
		ids = collectIDs(byFirstLetter.Iter("z"))
		if !slices.Equal(ids, []int{1}) {
			t.Errorf("Iter(z) after update = %v, want [1]", ids)
		}
	})
}

func collectIDs(seq iter.Seq[*testRow]) []int {
	var ids []int
	for row := range seq {
		ids = append(ids, row.ID)
	}
	return ids
}
