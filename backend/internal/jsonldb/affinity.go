package jsonldb

import (
	"math"
	"strconv"
	"strings"
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

// Affinity represents SQLite-compatible type affinity for columns.
type Affinity int

const (
	// AffinityBLOB has no type preference; values stored as-is.
	AffinityBLOB Affinity = iota
	// AffinityTEXT converts numeric values to string representation.
	AffinityTEXT
	// AffinityINTEGER forces integer representation.
	AffinityINTEGER
	// AffinityREAL forces floating point representation.
	AffinityREAL
	// AffinityNUMERIC stores as INTEGER if whole number, REAL otherwise.
	AffinityNUMERIC
)

// ColumnTypeAffinity returns the SQLite affinity for a column type.
func ColumnTypeAffinity(colType string) Affinity {
	switch strings.ToLower(colType) {
	case "text", "select":
		return AffinityTEXT
	case "multi_select":
		// multi_select stores as JSON array string
		return AffinityTEXT
	case "number":
		return AffinityNUMERIC
	case "checkbox":
		return AffinityINTEGER
	case "date":
		// ISO8601 string format
		return AffinityTEXT
	default:
		// Unknown types default to BLOB (no coercion)
		return AffinityBLOB
	}
}

// CoerceValue applies SQLite-compatible type coercion to a value based on affinity.
// Returns the coerced value. Nil values pass through unchanged.
func CoerceValue(value any, affinity Affinity) any {
	if value == nil {
		return nil
	}

	switch affinity {
	case AffinityTEXT:
		return coerceToText(value)
	case AffinityINTEGER:
		return coerceToInteger(value)
	case AffinityREAL:
		return coerceToReal(value)
	case AffinityNUMERIC:
		return coerceToNumeric(value)
	case AffinityBLOB:
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
// - Non-numeric text → stays as string
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

// CoerceData applies type coercion to all values in a data map based on column definitions.
// Columns not in the schema are passed through unchanged (BLOB affinity).
func CoerceData(data map[string]any, columns []Column) map[string]any {
	if data == nil {
		return nil
	}

	// Build column type lookup
	colTypes := make(map[string]string, len(columns))
	for _, col := range columns {
		colTypes[col.Name] = col.Type
	}

	return CoerceDataWithTypes(data, colTypes)
}

// CoerceDataWithTypes applies type coercion using a map of column name to type string.
// Columns not in the map are passed through unchanged (BLOB affinity).
func CoerceDataWithTypes(data map[string]any, colTypes map[string]string) map[string]any {
	if data == nil {
		return nil
	}

	result := make(map[string]any, len(data))
	for key, value := range data {
		colType, ok := colTypes[key]
		if !ok {
			// Unknown column, pass through unchanged
			result[key] = value
			continue
		}
		affinity := ColumnTypeAffinity(colType)
		result[key] = CoerceValue(value, affinity)
	}
	return result
}
