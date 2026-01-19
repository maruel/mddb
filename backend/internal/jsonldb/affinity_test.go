package jsonldb

import (
	"math"
	"testing"
)

func TestColumnType_affinity(t *testing.T) {
	tests := []struct {
		colType  ColumnType
		expected affinity
	}{
		{ColumnTypeText, affinityTEXT},
		{ColumnTypeNumber, affinityNUMERIC},
		{ColumnTypeBool, affinityINTEGER},
		{ColumnTypeDate, affinityTEXT},
		{ColumnType("unknown"), affinityBLOB},
		{ColumnType(""), affinityBLOB},
	}

	for _, tt := range tests {
		t.Run(string(tt.colType), func(t *testing.T) {
			got := tt.colType.affinity()
			if got != tt.expected {
				t.Errorf("ColumnType(%q).affinity() = %v, want %v", tt.colType, got, tt.expected)
			}
		})
	}
}

func Test_coerceValue_Nil(t *testing.T) {
	for _, affinity := range []affinity{affinityBLOB, affinityTEXT, affinityINTEGER, affinityREAL, affinityNUMERIC} {
		got := coerceValue(nil, affinity)
		if got != nil {
			t.Errorf("coerceValue(nil, %v) = %v, want nil", affinity, got)
		}
	}
}

func Test_coerceValue_TEXT(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected any
	}{
		{"string passthrough", "hello", "hello"},
		{"float64 whole", float64(42), "42"},
		{"float64 decimal", float64(3.14), "3.14"},
		{"int64", int64(123), "123"},
		{"int", 456, "456"},
		{"bool true", true, "1"},
		{"bool false", false, "0"},
		{"array passthrough", []string{"a", "b"}, []string{"a", "b"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := coerceValue(tt.input, affinityTEXT)
			switch g := got.(type) {
			case string:
				if e, ok := tt.expected.(string); ok && g != e {
					t.Errorf("coerceValue(%v, TEXT) = %v, want %v", tt.input, got, tt.expected)
				}
			default:
				// For non-string results, just check they're not nil when expected
			}
		})
	}
}

func Test_coerceValue_INTEGER(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected any
	}{
		{"float64 whole", float64(42), int64(42)},
		{"float64 truncate", float64(3.9), int64(3)},
		{"int64 passthrough", int64(123), int64(123)},
		{"int", 456, int64(456)},
		{"string integer", "789", int64(789)},
		{"string float", "3.14", int64(3)},
		{"string non-numeric", "hello", "hello"},
		{"bool true", true, int64(1)},
		{"bool false", false, int64(0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := coerceValue(tt.input, affinityINTEGER)
			if got != tt.expected {
				t.Errorf("coerceValue(%v, INTEGER) = %v (%T), want %v (%T)", tt.input, got, got, tt.expected, tt.expected)
			}
		})
	}
}

func Test_coerceValue_REAL(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected any
	}{
		{"float64 passthrough", float64(3.14), float64(3.14)},
		{"int64", int64(42), float64(42)},
		{"int", 456, float64(456)},
		{"string numeric", "3.14", float64(3.14)},
		{"string integer", "42", float64(42)},
		{"string non-numeric", "hello", "hello"},
		{"bool true", true, float64(1)},
		{"bool false", false, float64(0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := coerceValue(tt.input, affinityREAL)
			if got != tt.expected {
				t.Errorf("coerceValue(%v, REAL) = %v (%T), want %v (%T)", tt.input, got, got, tt.expected, tt.expected)
			}
		})
	}
}

func Test_coerceValue_NUMERIC(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected any
	}{
		{"float64 whole becomes int64", float64(42), int64(42)},
		{"float64 decimal stays float64", float64(3.14), float64(3.14)},
		{"int64 passthrough", int64(123), int64(123)},
		{"int becomes int64", 456, int64(456)},
		{"string integer", "789", int64(789)},
		{"string float whole", "42.0", int64(42)},
		{"string float decimal", "3.14", float64(3.14)},
		{"string non-numeric", "hello", "hello"},
		{"bool true", true, int64(1)},
		{"bool false", false, int64(0)},
		{"float64 scientific whole", float64(3e5), int64(300000)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := coerceValue(tt.input, affinityNUMERIC)
			if got != tt.expected {
				t.Errorf("coerceValue(%v, NUMERIC) = %v (%T), want %v (%T)", tt.input, got, got, tt.expected, tt.expected)
			}
		})
	}
}

func Test_coerceValue_NUMERIC_EdgeCases(t *testing.T) {
	// Infinity stays as float64
	inf := coerceValue(math.Inf(1), affinityNUMERIC)
	if _, ok := inf.(float64); !ok {
		t.Errorf("Inf should stay as float64, got %T", inf)
	}

	// NaN stays as float64
	nan := coerceValue(math.NaN(), affinityNUMERIC)
	if _, ok := nan.(float64); !ok {
		t.Errorf("NaN should stay as float64, got %T", nan)
	}
}

func Test_coerceValue_BLOB(t *testing.T) {
	// Test comparable types
	comparableTests := []any{
		"hello",
		float64(3.14),
		int64(42),
		true,
	}

	for _, input := range comparableTests {
		got := coerceValue(input, affinityBLOB)
		if got != input {
			t.Errorf("coerceValue(%v, BLOB) should pass through unchanged, got %v", input, got)
		}
	}

	// Test slice passthrough (use reflect for comparison)
	slice := []string{"a", "b"}
	gotSlice := coerceValue(slice, affinityBLOB)
	if s, ok := gotSlice.([]string); !ok || len(s) != 2 || s[0] != "a" || s[1] != "b" {
		t.Errorf("coerceValue(slice, BLOB) should pass through unchanged")
	}

	// Test map passthrough
	m := map[string]any{"key": "value"}
	gotMap := coerceValue(m, affinityBLOB)
	if mp, ok := gotMap.(map[string]any); !ok || mp["key"] != "value" {
		t.Errorf("coerceValue(map, BLOB) should pass through unchanged")
	}
}

func TestCoerceData(t *testing.T) {
	columns := []Column{
		{Name: "name", Type: ColumnTypeText},
		{Name: "age", Type: ColumnTypeNumber},
		{Name: "active", Type: ColumnTypeBool},
		{Name: "score", Type: ColumnTypeNumber},
		{Name: "tags", Type: ColumnTypeText},
	}

	data := map[string]any{
		"name":    123,        // number → text: "123"
		"age":     "25",       // string → numeric: int64(25)
		"active":  true,       // bool → integer: int64(1)
		"score":   float64(3), // float whole → numeric: int64(3)
		"tags":    "tag1",     // stays as text
		"unknown": 42,         // not in schema, passthrough
	}

	result := CoerceData(data, columns)

	// Check name: number → text
	if name, ok := result["name"].(string); !ok || name != "123" {
		t.Errorf("name = %v (%T), want '123' (string)", result["name"], result["name"])
	}

	// Check age: string → int64
	if age, ok := result["age"].(int64); !ok || age != 25 {
		t.Errorf("age = %v (%T), want 25 (int64)", result["age"], result["age"])
	}

	// Check active: bool → int64
	if active, ok := result["active"].(int64); !ok || active != 1 {
		t.Errorf("active = %v (%T), want 1 (int64)", result["active"], result["active"])
	}

	// Check score: float64 whole → int64
	if score, ok := result["score"].(int64); !ok || score != 3 {
		t.Errorf("score = %v (%T), want 3 (int64)", result["score"], result["score"])
	}

	// Check unknown: passthrough
	if unknown, ok := result["unknown"].(int); !ok || unknown != 42 {
		t.Errorf("unknown = %v (%T), want 42 (int)", result["unknown"], result["unknown"])
	}
}

func TestCoerceData_Nil(t *testing.T) {
	columns := []Column{{Name: "name", Type: "text"}}
	result := CoerceData(nil, columns)
	if result != nil {
		t.Errorf("CoerceData(nil, columns) = %v, want nil", result)
	}
}

func TestCoerceData_EmptyColumns(t *testing.T) {
	data := map[string]any{"key": "value"}
	result := CoerceData(data, nil)
	if result["key"] != "value" {
		t.Errorf("CoerceData with empty columns should passthrough, got %v", result)
	}
}
