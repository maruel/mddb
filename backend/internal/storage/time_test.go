package storage

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTimePrecision(t *testing.T) {
	// Target time: 1.234 seconds (1234 ms)
	target := time.UnixMilli(1234).UTC()

	// Convert to storage.Time
	st := ToTime(target)

	// Convert back to time.Time
	back := st.AsTime()

	// Check if precision is preserved
	if !back.Equal(target) {
		t.Errorf("Precision lost: expected %v, got %v", target, back)
	}

	// JSON Marshal
	b, err := json.Marshal(st)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	expectedJSON := "1.234"
	if string(b) != expectedJSON {
		t.Errorf("Marshal unexpected: expected %s, got %s", expectedJSON, string(b))
	}

	// JSON Unmarshal 1.234
	jsonInput := []byte("1.234")
	var st2 Time
	if err := json.Unmarshal(jsonInput, &st2); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	back2 := st2.AsTime()

	// Check unmarshal precision
	if back2.UnixMilli() != 1234 {
		t.Errorf("Unmarshal precision lost: expected 1234ms, got %v", back2.UnixMilli())
	}
}

func TestTime_JSON_IntegerCompatibility(t *testing.T) {
	// Test backward compatibility with integer seconds (e.g. 123)
	// 123 in JSON -> 123 seconds -> 12300 units.
	jsonInput := []byte("123")
	var st Time
	if err := json.Unmarshal(jsonInput, &st); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	expected := time.Unix(123, 0).UTC()
	if !st.AsTime().Equal(expected) {
		t.Errorf("Integer unmarshal failed: expected %v, got %v", expected, st.AsTime())
	}
}
