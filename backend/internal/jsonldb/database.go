// Package jsonldb provides thread-safe JSONL file storage with schema management.
//
// It offers Table[T] for generic type-safe row storage with schema support.
// All data types stored in Table[T] must implement the Row interface
// (combining Cloner and GetID methods). Table uses read-write locks for
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

// Column represents a database column in storage.
type Column struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Options  []string `json:"options,omitempty"`
	Required bool     `json:"required,omitempty"`
}

// schemaHeader is the first row of a JSONL data file containing schema and metadata.
// Used by Table[T] for generic schema storage.
type schemaHeader struct {
	Version  string    `json:"version"`
	Columns  []Column  `json:"columns"`
	Created  time.Time `json:"created"`
	Modified time.Time `json:"modified"`
}

// Validate checks that the schema header is well-formed.
func (h *schemaHeader) Validate() error {
	if h.Version == "" {
		return fmt.Errorf("schema version is required")
	}
	if h.Created.IsZero() {
		return fmt.Errorf("schema created timestamp is required")
	}
	if h.Modified.IsZero() {
		return fmt.Errorf("schema modified timestamp is required")
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
func schemaFromType[T any]() ([]Column, error) {
	t := reflect.TypeFor[T]()
	var val any

	switch t.Kind() {
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
	var columns []Column
	for jsonName := range m {
		field, ok := fieldByJSONName[jsonName]
		if !ok {
			// Field in JSON but not found in struct, infer type from value
			colType := inferTypeFromValue(m[jsonName])
			columns = append(columns, Column{
				Name: jsonName,
				Type: colType,
			})
			continue
		}

		// Use struct field info for type inference
		colType := goTypeToColumnType(field.Type)
		columns = append(columns, Column{
			Name: jsonName,
			Type: colType,
		})
	}

	return columns, nil
}

// inferTypeFromValue infers a column type from a JSON value.
func inferTypeFromValue(v any) string {
	if v == nil {
		return "text"
	}
	switch v.(type) {
	case bool:
		return "checkbox"
	case float64:
		return "number"
	case string:
		return "text"
	default:
		return "text"
	}
}

// goTypeToColumnType maps Go types to JSONL column types.
func goTypeToColumnType(t reflect.Type) string {
	// Dereference pointers
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	// Check for time.Time first (before switch)
	if t == reflect.TypeOf(time.Time{}) {
		return "date"
	}

	switch t.Kind() { //nolint:exhaustive // Other kinds default to "text"
	case reflect.String:
		return "text"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Bool:
		return "checkbox"
	default:
		// Default to text for all other types
		return "text"
	}
}
