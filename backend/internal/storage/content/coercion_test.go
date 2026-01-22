package content

import (
	"math"
	"testing"
)

func Test_coerceValue(t *testing.T) {
	t.Run("Text", func(t *testing.T) {
		tests := []struct {
			name  string
			input any
			want  any
		}{
			{"string passthrough", "hello", "hello"},
			{"float64 whole", float64(42), "42"},
			{"float64 decimal", float64(3.14), "3.14"},
			{"int64", int64(123), "123"},
			{"int", 456, "456"},
			{"bool true", true, "1"},
			{"bool false", false, "0"},
			{"nil", nil, nil},
			{"array passthrough", []any{1, 2}, []any{1, 2}},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := coerceValue(tt.input, affinityTEXT)
				if !equalAny(got, tt.want) {
					t.Errorf("coerceValue(%v, TEXT) = %v (%T), want %v (%T)", tt.input, got, got, tt.want, tt.want)
				}
			})
		}
	})

	t.Run("Integer", func(t *testing.T) {
		tests := []struct {
			name  string
			input any
			want  any
		}{
			{"float64 whole", float64(42), int64(42)},
			{"float64 decimal truncates", float64(3.9), int64(3)},
			{"int64 passthrough", int64(123), int64(123)},
			{"int converts", 456, int64(456)},
			{"string int", "789", int64(789)},
			{"string float", "3.14", int64(3)},
			{"string non-numeric", "hello", "hello"},
			{"bool true", true, int64(1)},
			{"bool false", false, int64(0)},
			{"nil", nil, nil},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := coerceValue(tt.input, affinityINTEGER)
				if !equalAny(got, tt.want) {
					t.Errorf("coerceValue(%v, INTEGER) = %v (%T), want %v (%T)", tt.input, got, got, tt.want, tt.want)
				}
			})
		}
	})

	t.Run("Real", func(t *testing.T) {
		tests := []struct {
			name  string
			input any
			want  any
		}{
			{"float64 passthrough", float64(3.14), float64(3.14)},
			{"int64 converts", int64(42), float64(42)},
			{"int converts", 123, float64(123)},
			{"string float", "3.14", float64(3.14)},
			{"string int", "42", float64(42)},
			{"string non-numeric", "hello", "hello"},
			{"bool true", true, float64(1)},
			{"bool false", false, float64(0)},
			{"nil", nil, nil},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := coerceValue(tt.input, affinityREAL)
				if !equalAny(got, tt.want) {
					t.Errorf("coerceValue(%v, REAL) = %v (%T), want %v (%T)", tt.input, got, got, tt.want, tt.want)
				}
			})
		}
	})

	t.Run("Numeric", func(t *testing.T) {
		tests := []struct {
			name  string
			input any
			want  any
		}{
			{"float64 whole becomes int64", float64(42), int64(42)},
			{"float64 decimal stays", float64(3.14), float64(3.14)},
			{"int64 passthrough", int64(123), int64(123)},
			{"int converts", 456, int64(456)},
			{"string int", "789", int64(789)},
			{"string float whole", "42.0", int64(42)},
			{"string float decimal", "3.14", float64(3.14)},
			{"string non-numeric", "hello", "hello"},
			{"bool true", true, int64(1)},
			{"bool false", false, int64(0)},
			{"nil", nil, nil},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := coerceValue(tt.input, affinityNUMERIC)
				if !equalAny(got, tt.want) {
					t.Errorf("coerceValue(%v, NUMERIC) = %v (%T), want %v (%T)", tt.input, got, got, tt.want, tt.want)
				}
			})
		}
	})

	t.Run("Blob", func(t *testing.T) {
		tests := []struct {
			name  string
			input any
		}{
			{"string", "hello"},
			{"float64", float64(3.14)},
			{"int64", int64(42)},
			{"bool", true},
			{"nil", nil},
			{"array", []any{1, 2, 3}},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := coerceValue(tt.input, affinityBLOB)
				if !equalAny(got, tt.input) {
					t.Errorf("coerceValue(%v, BLOB) = %v, want %v (passthrough)", tt.input, got, tt.input)
				}
			})
		}
	})

	t.Run("EdgeCases", func(t *testing.T) {
		t.Run("infinity to text", func(t *testing.T) {
			got := coerceValue(math.Inf(1), affinityTEXT)
			if got != "+Inf" {
				t.Errorf("coerceValue(+Inf, TEXT) = %v, want +Inf", got)
			}
		})

		t.Run("NaN to text", func(t *testing.T) {
			got := coerceValue(math.NaN(), affinityTEXT)
			if got != "NaN" {
				t.Errorf("coerceValue(NaN, TEXT) = %v, want NaN", got)
			}
		})

		t.Run("large float stays float64", func(t *testing.T) {
			large := float64(1e20)
			got := coerceValue(large, affinityNUMERIC)
			if _, ok := got.(float64); !ok {
				t.Errorf("coerceValue(1e20, NUMERIC) = %T, want float64", got)
			}
		})
	})
}

func Test_propertyAffinity(t *testing.T) {
	tests := []struct {
		propType PropertyType
		want     affinity
	}{
		{PropertyTypeText, affinityTEXT},
		{PropertyTypeNumber, affinityNUMERIC},
		{PropertyTypeCheckbox, affinityINTEGER},
		{PropertyTypeDate, affinityTEXT},
		{PropertyTypeSelect, affinityTEXT},
		{PropertyTypeMultiSelect, affinityBLOB},
		{PropertyTypeURL, affinityTEXT},
		{PropertyTypeEmail, affinityTEXT},
		{PropertyTypePhone, affinityTEXT},
		{PropertyType("unknown"), affinityBLOB},
	}

	for _, tt := range tests {
		t.Run(string(tt.propType), func(t *testing.T) {
			got := propertyAffinity(tt.propType)
			if got != tt.want {
				t.Errorf("propertyAffinity(%s) = %v, want %v", tt.propType, got, tt.want)
			}
		})
	}
}

func Test_CoerceRecordData(t *testing.T) {
	t.Run("Basic", func(t *testing.T) {
		properties := []Property{
			{Name: "name", Type: PropertyTypeText},
			{Name: "age", Type: PropertyTypeNumber},
			{Name: "active", Type: PropertyTypeCheckbox},
		}

		data := map[string]any{
			"name":    123,         // number → text: "123"
			"age":     float64(25), // float64 → numeric: int64(25)
			"active":  true,        // bool → integer: int64(1)
			"unknown": "value",     // unknown property, pass through
		}

		result := CoerceRecordData(data, properties)

		if result["name"] != "123" {
			t.Errorf("name: got %v (%T), want '123' (string)", result["name"], result["name"])
		}
		if result["age"] != int64(25) {
			t.Errorf("age: got %v (%T), want 25 (int64)", result["age"], result["age"])
		}
		if result["active"] != int64(1) {
			t.Errorf("active: got %v (%T), want 1 (int64)", result["active"], result["active"])
		}
		if result["unknown"] != "value" {
			t.Errorf("unknown: got %v, want 'value' (passthrough)", result["unknown"])
		}
	})

	t.Run("Nil", func(t *testing.T) {
		properties := []Property{{Name: "col", Type: PropertyTypeText}}
		result := CoerceRecordData(nil, properties)
		if result != nil {
			t.Errorf("CoerceRecordData(nil, properties) = %v, want nil", result)
		}
	})

	t.Run("EmptyProperties", func(t *testing.T) {
		data := map[string]any{"key": "value"}
		result := CoerceRecordData(data, nil)
		if result["key"] != "value" {
			t.Errorf("CoerceRecordData with empty properties should passthrough, got %v", result)
		}
	})
}

// equalAny compares two any values for equality.
func equalAny(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	switch av := a.(type) {
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !equalAny(av[i], bv[i]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}
