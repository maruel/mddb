// Tests for filtering and sorting logic.

package content

import (
	"testing"

	"github.com/maruel/mddb/backend/internal/jsonldb"
)

func makeRecord(data map[string]any) *DataRecord {
	return &DataRecord{
		ID:   jsonldb.NewID(),
		Data: data,
	}
}

func TestFilterRecords(t *testing.T) {
	records := []*DataRecord{
		makeRecord(map[string]any{"name": "Alice", "age": float64(30), "active": true}),
		makeRecord(map[string]any{"name": "Bob", "age": float64(25), "active": false}),
		makeRecord(map[string]any{"name": "Charlie", "age": float64(35), "active": true}),
	}

	t.Run("equals filter", func(t *testing.T) {
		filters := []Filter{
			{Property: "name", Operator: FilterOpEquals, Value: "Bob"},
		}
		result := FilterRecords(records, filters)
		if len(result) != 1 {
			t.Fatalf("expected 1 record, got %d", len(result))
		}
		if result[0].Data["name"] != "Bob" {
			t.Errorf("expected Bob, got %v", result[0].Data["name"])
		}
	})

	t.Run("not equals filter", func(t *testing.T) {
		filters := []Filter{
			{Property: "name", Operator: FilterOpNotEquals, Value: "Bob"},
		}
		result := FilterRecords(records, filters)
		if len(result) != 2 {
			t.Fatalf("expected 2 records, got %d", len(result))
		}
	})

	t.Run("greater than filter", func(t *testing.T) {
		filters := []Filter{
			{Property: "age", Operator: FilterOpGreaterThan, Value: float64(28)},
		}
		result := FilterRecords(records, filters)
		if len(result) != 2 {
			t.Fatalf("expected 2 records, got %d", len(result))
		}
	})

	t.Run("less than filter", func(t *testing.T) {
		filters := []Filter{
			{Property: "age", Operator: FilterOpLessThan, Value: float64(30)},
		}
		result := FilterRecords(records, filters)
		if len(result) != 1 {
			t.Fatalf("expected 1 record, got %d", len(result))
		}
	})

	t.Run("contains filter", func(t *testing.T) {
		filters := []Filter{
			{Property: "name", Operator: FilterOpContains, Value: "li"},
		}
		result := FilterRecords(records, filters)
		if len(result) != 2 { // Alice, Charlie
			t.Fatalf("expected 2 records, got %d", len(result))
		}
	})

	t.Run("starts with filter", func(t *testing.T) {
		filters := []Filter{
			{Property: "name", Operator: FilterOpStartsWith, Value: "A"},
		}
		result := FilterRecords(records, filters)
		if len(result) != 1 {
			t.Fatalf("expected 1 record, got %d", len(result))
		}
	})

	t.Run("boolean equals filter", func(t *testing.T) {
		filters := []Filter{
			{Property: "active", Operator: FilterOpEquals, Value: true},
		}
		result := FilterRecords(records, filters)
		if len(result) != 2 {
			t.Fatalf("expected 2 records, got %d", len(result))
		}
	})

	t.Run("empty filters returns all", func(t *testing.T) {
		result := FilterRecords(records, nil)
		if len(result) != 3 {
			t.Fatalf("expected 3 records, got %d", len(result))
		}
	})
}

func TestFilterRecordsCompound(t *testing.T) {
	records := []*DataRecord{
		makeRecord(map[string]any{"name": "Alice", "age": float64(30)}),
		makeRecord(map[string]any{"name": "Bob", "age": float64(25)}),
		makeRecord(map[string]any{"name": "Charlie", "age": float64(35)}),
	}

	t.Run("AND filter", func(t *testing.T) {
		filters := []Filter{
			{
				And: []Filter{
					{Property: "age", Operator: FilterOpGreaterEqual, Value: float64(25)},
					{Property: "age", Operator: FilterOpLessEqual, Value: float64(30)},
				},
			},
		}
		result := FilterRecords(records, filters)
		if len(result) != 2 { // Alice (30), Bob (25)
			t.Fatalf("expected 2 records, got %d", len(result))
		}
	})

	t.Run("OR filter", func(t *testing.T) {
		filters := []Filter{
			{
				Or: []Filter{
					{Property: "name", Operator: FilterOpEquals, Value: "Alice"},
					{Property: "name", Operator: FilterOpEquals, Value: "Charlie"},
				},
			},
		}
		result := FilterRecords(records, filters)
		if len(result) != 2 {
			t.Fatalf("expected 2 records, got %d", len(result))
		}
	})
}

func TestFilterRecordsEmpty(t *testing.T) {
	records := []*DataRecord{
		makeRecord(map[string]any{"name": "Alice", "email": "alice@example.com"}),
		makeRecord(map[string]any{"name": "Bob", "email": ""}),
		makeRecord(map[string]any{"name": "Charlie"}), // email not set
	}

	t.Run("is empty", func(t *testing.T) {
		filters := []Filter{
			{Property: "email", Operator: FilterOpIsEmpty},
		}
		result := FilterRecords(records, filters)
		if len(result) != 2 { // Bob (empty string), Charlie (not set)
			t.Fatalf("expected 2 records, got %d", len(result))
		}
	})

	t.Run("is not empty", func(t *testing.T) {
		filters := []Filter{
			{Property: "email", Operator: FilterOpIsNotEmpty},
		}
		result := FilterRecords(records, filters)
		if len(result) != 1 {
			t.Fatalf("expected 1 record, got %d", len(result))
		}
	})
}

func TestSortRecords(t *testing.T) {
	records := []*DataRecord{
		makeRecord(map[string]any{"name": "Charlie", "age": float64(35)}),
		makeRecord(map[string]any{"name": "Alice", "age": float64(30)}),
		makeRecord(map[string]any{"name": "Bob", "age": float64(25)}),
	}

	t.Run("sort ascending by name", func(t *testing.T) {
		r := make([]*DataRecord, len(records))
		copy(r, records)

		SortRecords(r, []Sort{{Property: "name", Direction: SortAsc}})

		if r[0].Data["name"] != "Alice" {
			t.Errorf("expected Alice first, got %v", r[0].Data["name"])
		}
		if r[1].Data["name"] != "Bob" {
			t.Errorf("expected Bob second, got %v", r[1].Data["name"])
		}
		if r[2].Data["name"] != "Charlie" {
			t.Errorf("expected Charlie third, got %v", r[2].Data["name"])
		}
	})

	t.Run("sort descending by age", func(t *testing.T) {
		r := make([]*DataRecord, len(records))
		copy(r, records)

		SortRecords(r, []Sort{{Property: "age", Direction: SortDesc}})

		if r[0].Data["name"] != "Charlie" {
			t.Errorf("expected Charlie first (age 35), got %v", r[0].Data["name"])
		}
		if r[1].Data["name"] != "Alice" {
			t.Errorf("expected Alice second (age 30), got %v", r[1].Data["name"])
		}
		if r[2].Data["name"] != "Bob" {
			t.Errorf("expected Bob third (age 25), got %v", r[2].Data["name"])
		}
	})

	t.Run("multi-sort", func(t *testing.T) {
		records := []*DataRecord{
			makeRecord(map[string]any{"dept": "Engineering", "name": "Charlie"}),
			makeRecord(map[string]any{"dept": "Engineering", "name": "Alice"}),
			makeRecord(map[string]any{"dept": "Sales", "name": "Bob"}),
		}

		SortRecords(records, []Sort{
			{Property: "dept", Direction: SortAsc},
			{Property: "name", Direction: SortAsc},
		})

		if records[0].Data["name"] != "Alice" {
			t.Errorf("expected Alice first, got %v", records[0].Data["name"])
		}
		if records[1].Data["name"] != "Charlie" {
			t.Errorf("expected Charlie second, got %v", records[1].Data["name"])
		}
		if records[2].Data["name"] != "Bob" {
			t.Errorf("expected Bob third, got %v", records[2].Data["name"])
		}
	})

	t.Run("empty sorts returns unchanged", func(t *testing.T) {
		r := make([]*DataRecord, len(records))
		copy(r, records)

		SortRecords(r, nil)

		// Order should be unchanged
		if r[0].Data["name"] != "Charlie" {
			t.Errorf("expected Charlie first, got %v", r[0].Data["name"])
		}
	})
}

func TestQueryRecords(t *testing.T) {
	records := []*DataRecord{
		makeRecord(map[string]any{"name": "Charlie", "age": float64(35)}),
		makeRecord(map[string]any{"name": "Alice", "age": float64(30)}),
		makeRecord(map[string]any{"name": "Bob", "age": float64(25)}),
	}

	t.Run("filter and sort", func(t *testing.T) {
		view := &View{
			Filters: []Filter{
				{Property: "age", Operator: FilterOpGreaterEqual, Value: float64(30)},
			},
			Sorts: []Sort{
				{Property: "name", Direction: SortAsc},
			},
		}

		result := QueryRecords(records, view)

		if len(result) != 2 {
			t.Fatalf("expected 2 records, got %d", len(result))
		}
		if result[0].Data["name"] != "Alice" {
			t.Errorf("expected Alice first, got %v", result[0].Data["name"])
		}
		if result[1].Data["name"] != "Charlie" {
			t.Errorf("expected Charlie second, got %v", result[1].Data["name"])
		}
	})

	t.Run("nil view returns all", func(t *testing.T) {
		result := QueryRecords(records, nil)
		if len(result) != 3 {
			t.Fatalf("expected 3 records, got %d", len(result))
		}
	})
}
