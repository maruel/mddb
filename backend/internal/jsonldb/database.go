// Package jsonldb provides thread-safe JSONL file storage with schema management.
//
// It offers Table[T] for generic type-safe row storage with schema support.
// All data types stored in Table[T] must implement the Row interface
// (implementing Clone and GetID methods). Table uses read-write locks for
// concurrent access and atomic file operations.
package jsonldb

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"
)

// currentVersion is the current version of the JSONL database format.
const currentVersion = "1.0"

// ColumnType represents the type of a database column.
type ColumnType string

const (
	// ColumnTypeText stores text values.
	ColumnTypeText ColumnType = "text"
	// ColumnTypeNumber stores numeric values (integer or float).
	ColumnTypeNumber ColumnType = "number"
	// ColumnTypeBool stores boolean values as true/false.
	ColumnTypeBool ColumnType = "bool"
	// ColumnTypeDate stores ISO8601 date strings.
	ColumnTypeDate ColumnType = "date"
	// ColumnTypeBlob stores binary data as base64-encoded string.
	ColumnTypeBlob ColumnType = "blob"
	// ColumnTypeJSONB stores structured data (struct, slice, map) as JSON.
	ColumnTypeJSONB ColumnType = "jsonb"
)

// column represents a database column in storage.
type column struct {
	Name     string     `json:"name"`
	Type     ColumnType `json:"type"`
	Required bool       `json:"required,omitempty"`
}

// schemaHeader is the first row of a JSONL data file containing schema and metadata.
// Used by Table[T] for generic schema storage.
type schemaHeader struct {
	Version string   `json:"version"`
	Columns []column `json:"columns"`
}

// Validate checks that the schema header is well-formed.
func (h *schemaHeader) Validate() error {
	if h.Version == "" {
		return fmt.Errorf("schema version is required")
	}
	// Validate each column
	for i, col := range h.Columns {
		if col.Name == "" {
			return fmt.Errorf("column %d: name is required", i)
		}
		if col.Type == "" {
			return fmt.Errorf("column %d: type is required", i)
		}
	}
	return nil
}

// schemaFromType[T any] extracts column definitions by marshaling a zero instance to JSON.
// This ensures the schema matches what is actually written to disk.
func schemaFromType[T any]() ([]column, error) {
	t := reflect.TypeFor[T]()
	var val any

	switch t.Kind() { //nolint:exhaustive // Only Ptr and Struct are valid; default handles the rest
	case reflect.Ptr:
		if t.Elem().Kind() != reflect.Struct {
			return nil, fmt.Errorf("type must be a struct or pointer to struct, got %s", t.Kind())
		}
		// Create a new instance of the underlying struct
		val = reflect.New(t.Elem()).Interface()
	case reflect.Struct:
		var zero T
		val = zero
	default:
		return nil, fmt.Errorf("type must be a struct or pointer to struct, got %s", t.Kind())
	}

	data, err := json.Marshal(val)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal zero instance: %w", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("failed to unmarshal to map: %w", err)
	}

	// Get the actual struct type for fallback type inference
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	// Build field lookup by JSON name
	fieldByJSONName := make(map[string]reflect.StructField)
	for i := range t.NumField() {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		jsonTag := field.Tag.Get("json")
		if jsonTag == "-" {
			continue
		}
		jsonName := field.Name
		if jsonTag != "" {
			jsonName = strings.Split(jsonTag, ",")[0]
		}
		fieldByJSONName[jsonName] = field
	}

	// Create columns from JSON keys in deterministic order
	var columns []column
	for jsonName := range m {
		field, ok := fieldByJSONName[jsonName]
		if !ok {
			// Field in JSON but not found in struct, infer type from value
			colType := inferTypeFromValue(m[jsonName])
			columns = append(columns, column{
				Name: jsonName,
				Type: colType,
			})
			continue
		}

		// Use struct field info for type inference
		colType := goTypeToColumnType(field.Type)
		columns = append(columns, column{
			Name: jsonName,
			Type: colType,
		})
	}

	return columns, nil
}

// inferTypeFromValue infers a column type from a JSON value.
func inferTypeFromValue(v any) ColumnType {
	if v == nil {
		return ColumnTypeText
	}
	switch v.(type) {
	case bool:
		return ColumnTypeBool
	case float64:
		return ColumnTypeNumber
	case string:
		return ColumnTypeText
	case []byte:
		return ColumnTypeBlob
	case []any, map[string]any:
		return ColumnTypeJSONB
	default:
		return ColumnTypeText
	}
}

// goTypeToColumnType maps Go types to JSONL column types.
func goTypeToColumnType(t reflect.Type) ColumnType {
	// Dereference pointers
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	// Check for time.Time first (before switch)
	if t == reflect.TypeOf(time.Time{}) {
		return ColumnTypeDate
	}

	// Check for []byte (blob)
	if t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8 {
		return ColumnTypeBlob
	}

	switch t.Kind() {
	case reflect.String:
		return ColumnTypeText
	case reflect.Bool:
		return ColumnTypeBool
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return ColumnTypeNumber
	case reflect.Struct, reflect.Slice, reflect.Array, reflect.Map:
		return ColumnTypeJSONB
	case reflect.Complex64, reflect.Complex128:
		// Complex numbers stored as JSON array [real, imag]
		return ColumnTypeJSONB
	case reflect.Invalid, reflect.Uintptr, reflect.Chan, reflect.Func,
		reflect.Interface, reflect.Pointer, reflect.UnsafePointer:
		// Unsupported types default to text
		return ColumnTypeText
	}
	return ColumnTypeText
}
