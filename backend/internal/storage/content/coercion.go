package content

import (
	"math"
	"strconv"
)

// Type coercion maps JSON wire types through Go types to SQLite storage classes.
//
// JSON wire type → Go type (json.Unmarshal) → Go type (coerced) → SQLite class:
//
//	null           → nil           → nil        → NULL
//	true/false     → bool          → int64      → INTEGER (0 or 1)
//	123            → float64       → int64      → INTEGER
//	3.14           → float64       → float64    → REAL
//	"text"         → string        → string     → TEXT
//	[...]          → []any         → []any      → TEXT (JSON-encoded)
//	{...}          → map[string]any → map[string]any → TEXT (JSON-encoded)
//
// The coercion depends on column affinity:
//   - TEXT: numbers → string, bool → "0"/"1"
//   - INTEGER: float64 → int64, bool → int64(0/1), numeric strings → int64
//   - REAL: all numbers → float64, numeric strings → float64
//   - NUMERIC: whole floats → int64, fractional → float64, numeric strings parsed
//   - BLOB: no coercion, values pass through unchanged
//
// See https://www.sqlite.org/datatype3.html for the SQLite specification.

// affinity represents SQLite-compatible type affinity for columns.
type affinity int

const (
	// affinityBLOB has no type preference; values stored as-is.
	affinityBLOB affinity = iota
	// affinityTEXT converts numeric values to string representation.
	affinityTEXT
	// affinityINTEGER forces integer representation.
	affinityINTEGER
	// affinityREAL forces floating point representation.
	affinityREAL
	// affinityNUMERIC stores as INTEGER if whole number, REAL otherwise.
	affinityNUMERIC
)

// propertyAffinity returns the SQLite affinity for a property type.
func propertyAffinity(pt PropertyType) affinity {
	switch pt {
	case PropertyTypeText, PropertyTypeURL, PropertyTypeEmail, PropertyTypePhone:
		return affinityTEXT
	case PropertyTypeNumber:
		return affinityNUMERIC
	case PropertyTypeCheckbox:
		return affinityINTEGER
	case PropertyTypeDate:
		// ISO8601 string format
		return affinityTEXT
	case PropertyTypeSelect:
		// Select stores option ID as text
		return affinityTEXT
	case PropertyTypeMultiSelect:
		// Multi-select stores as JSON array, no coercion
		return affinityBLOB
	default:
		// Unknown types default to BLOB (no coercion)
		return affinityBLOB
	}
}

// coerceValue applies SQLite-compatible type coercion to a value based on affinity.
// Returns the coerced value. Nil values pass through unchanged.
func coerceValue(value any, aff affinity) any {
	if value == nil {
		return nil
	}

	switch aff {
	case affinityTEXT:
		return coerceToText(value)
	case affinityINTEGER:
		return coerceToInteger(value)
	case affinityREAL:
		return coerceToReal(value)
	case affinityNUMERIC:
		return coerceToNumeric(value)
	case affinityBLOB:
		// No coercion for BLOB affinity
		return value
	default:
		return value
	}
}

// coerceToText converts numeric values to string representation.
func coerceToText(value any) any {
	switch v := value.(type) {
	case string:
		return v
	case float64:
		// Format without unnecessary decimal places for whole numbers
		if v == math.Trunc(v) && !math.IsInf(v, 0) && !math.IsNaN(v) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(v, 10)
	case int:
		return strconv.Itoa(v)
	case bool:
		if v {
			return "1"
		}
		return "0"
	default:
		// Keep as-is for complex types (arrays, objects)
		return value
	}
}

// coerceToInteger converts values to integer representation.
func coerceToInteger(value any) any {
	switch v := value.(type) {
	case float64:
		// Truncate to integer
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	case string:
		// Try to parse as integer
		if i, err := strconv.ParseInt(v, 10, 64); err == nil {
			return i
		}
		// Try to parse as float and truncate
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return int64(f)
		}
		// Non-numeric string stays as TEXT
		return v
	case bool:
		if v {
			return int64(1)
		}
		return int64(0)
	default:
		return value
	}
}

// coerceToReal converts values to floating point representation.
func coerceToReal(value any) any {
	switch v := value.(type) {
	case float64:
		return v
	case int64:
		return float64(v)
	case int:
		return float64(v)
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
		// Non-numeric string stays as TEXT
		return v
	case bool:
		if v {
			return float64(1)
		}
		return float64(0)
	default:
		return value
	}
}

// coerceToNumeric applies NUMERIC affinity rules:
// - Well-formed integer text → int64
// - Well-formed real text → float64
// - Float equal to integer → int64
// - Non-numeric text → stays as string.
func coerceToNumeric(value any) any {
	switch v := value.(type) {
	case float64:
		// Float that equals integer becomes int64
		if v == math.Trunc(v) && !math.IsInf(v, 0) && !math.IsNaN(v) && v >= math.MinInt64 && v <= math.MaxInt64 {
			return int64(v)
		}
		return v
	case int64:
		return v
	case int:
		return int64(v)
	case string:
		// Try integer first
		if i, err := strconv.ParseInt(v, 10, 64); err == nil {
			return i
		}
		// Try float
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			// Float that equals integer becomes int64
			if f == math.Trunc(f) && !math.IsInf(f, 0) && !math.IsNaN(f) && f >= math.MinInt64 && f <= math.MaxInt64 {
				return int64(f)
			}
			return f
		}
		// Non-numeric string stays as TEXT
		return v
	case bool:
		if v {
			return int64(1)
		}
		return int64(0)
	default:
		return value
	}
}

// coerceRecordData applies type coercion to all values in a data map based on property definitions.
// Properties not in the schema are passed through unchanged (BLOB affinity).
func coerceRecordData(data map[string]any, properties []Property) map[string]any {
	if data == nil {
		return nil
	}

	// Build property type lookup
	propTypes := make(map[string]PropertyType, len(properties))
	for _, prop := range properties {
		propTypes[prop.Name] = prop.Type
	}

	result := make(map[string]any, len(data))
	for key, value := range data {
		propType, ok := propTypes[key]
		if !ok {
			// Unknown property, pass through unchanged
			result[key] = value
			continue
		}
		result[key] = coerceValue(value, propertyAffinity(propType))
	}
	return result
}
