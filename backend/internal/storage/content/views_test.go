// Tests for view types.

package content

import (
	"testing"

	"github.com/maruel/mddb/backend/internal/jsonldb"
)

func TestViewValidate(t *testing.T) {
	t.Run("valid view", func(t *testing.T) {
		v := &View{
			ID:   jsonldb.NewID(),
			Name: "My View",
			Type: ViewTypeTable,
		}
		if err := v.Validate(); err != nil {
			t.Errorf("Validate() unexpected error: %v", err)
		}
	})

	t.Run("missing ID", func(t *testing.T) {
		v := &View{
			Name: "My View",
			Type: ViewTypeTable,
		}
		if err := v.Validate(); err == nil {
			t.Error("Validate() expected error for missing ID")
		}
	})

	t.Run("missing name", func(t *testing.T) {
		v := &View{
			ID:   jsonldb.NewID(),
			Type: ViewTypeTable,
		}
		if err := v.Validate(); err == nil {
			t.Error("Validate() expected error for missing name")
		}
	})
}

func TestViewGetID(t *testing.T) {
	id := jsonldb.NewID()
	v := &View{ID: id}
	if v.GetID() != id {
		t.Errorf("GetID() = %v, want %v", v.GetID(), id)
	}
}

func TestViewTypes(t *testing.T) {
	// Ensure all view types are distinct
	types := []ViewType{
		ViewTypeTable,
		ViewTypeBoard,
		ViewTypeGallery,
		ViewTypeList,
		ViewTypeCalendar,
	}

	seen := make(map[ViewType]bool)
	for _, vt := range types {
		if seen[vt] {
			t.Errorf("duplicate view type: %s", vt)
		}
		seen[vt] = true
	}
}

func TestFilterOps(t *testing.T) {
	// Ensure all filter operators are distinct
	ops := []FilterOp{
		FilterOpEquals,
		FilterOpNotEquals,
		FilterOpContains,
		FilterOpNotContains,
		FilterOpStartsWith,
		FilterOpEndsWith,
		FilterOpGreaterThan,
		FilterOpLessThan,
		FilterOpGreaterEqual,
		FilterOpLessEqual,
		FilterOpIsEmpty,
		FilterOpIsNotEmpty,
	}

	seen := make(map[FilterOp]bool)
	for _, op := range ops {
		if seen[op] {
			t.Errorf("duplicate filter op: %s", op)
		}
		seen[op] = true
	}
}

func TestSortDir(t *testing.T) {
	if SortAsc == SortDesc {
		t.Error("SortAsc and SortDesc should be different")
	}
}
