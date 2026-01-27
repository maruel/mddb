package content

import (
	"testing"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage"
)

func TestDataRecord(t *testing.T) {
	t.Run("Clone", func(t *testing.T) {
		t.Run("copies values", func(t *testing.T) {
			original := &DataRecord{
				ID:       jsonldb.ID(1),
				Data:     map[string]any{"name": "test", "count": 42},
				Created:  storage.Now(),
				Modified: storage.Now(),
			}
			clone := original.Clone()
			if clone.ID != original.ID {
				t.Errorf("Clone ID = %v, want %v", clone.ID, original.ID)
			}
			if clone.Data["name"] != original.Data["name"] {
				t.Error("Clone Data not properly copied")
			}
			clone.Data["name"] = "modified"
			if original.Data["name"] == "modified" {
				t.Error("Clone Data should not share reference with original")
			}
		})
		t.Run("deep copies slices", func(t *testing.T) {
			original := &DataRecord{
				ID:   jsonldb.ID(1),
				Data: map[string]any{"tags": []string{"a", "b", "c"}},
			}
			clone := original.Clone()
			// Modify the cloned slice
			clone.Data["tags"].([]string)[0] = "modified"
			// Original should be unchanged
			if original.Data["tags"].([]string)[0] != "a" {
				t.Error("Clone should deep copy slices")
			}
		})
		t.Run("deep copies any slices", func(t *testing.T) {
			original := &DataRecord{
				ID:   jsonldb.ID(1),
				Data: map[string]any{"values": []any{1, "two", 3.0}},
			}
			clone := original.Clone()
			clone.Data["values"].([]any)[1] = "modified"
			if original.Data["values"].([]any)[1] != "two" {
				t.Error("Clone should deep copy []any slices")
			}
		})
		t.Run("deep copies nested maps", func(t *testing.T) {
			original := &DataRecord{
				ID:   jsonldb.ID(1),
				Data: map[string]any{"nested": map[string]any{"key": "value"}},
			}
			clone := original.Clone()
			clone.Data["nested"].(map[string]any)["key"] = "modified"
			if original.Data["nested"].(map[string]any)["key"] != "value" {
				t.Error("Clone should deep copy nested maps")
			}
		})
		t.Run("nil data", func(t *testing.T) {
			original := &DataRecord{ID: jsonldb.ID(1), Data: nil}
			if original.Clone().Data != nil {
				t.Error("Clone of nil Data should be nil")
			}
		})
	})
	t.Run("GetID", func(t *testing.T) {
		if got := (&DataRecord{ID: jsonldb.ID(42)}).GetID(); got != jsonldb.ID(42) {
			t.Errorf("GetID() = %v, want %v", got, jsonldb.ID(42))
		}
	})
	t.Run("Validate", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			if err := (&DataRecord{ID: jsonldb.ID(1)}).Validate(); err != nil {
				t.Errorf("Validate() unexpected error: %v", err)
			}
		})
		t.Run("zero ID", func(t *testing.T) {
			if err := (&DataRecord{ID: jsonldb.ID(0)}).Validate(); err == nil {
				t.Error("Validate() expected error for zero ID")
			}
		})
	})
}
