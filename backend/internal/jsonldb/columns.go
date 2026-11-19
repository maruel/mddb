// Derives table header columns from Go types through JSON Schema reflection.

package jsonldb

import (
	"fmt"
	"reflect"
	"time"

	jsonldbv1 "github.com/maruel/mddb/backend/internal/jsonldb/data/v1"

	"github.com/invopop/jsonschema"
)

// schemaFromType extracts column definitions using JSON Schema reflection.
//
// It uses github.com/invopop/jsonschema to extract field descriptions from
// `jsonschema:"description=..."` tags and required fields from the schema.
func schemaFromType[T any]() ([]jsonldbv1.Column, error) {
	t := reflect.TypeFor[T]()

	// Validate type
	switch t.Kind() {
	case reflect.Pointer:
		if t.Elem().Kind() != reflect.Struct {
			return nil, fmt.Errorf("type must be a struct or pointer to struct, got %s", t.Kind())
		}
	case reflect.Struct:
		// ok
	default:
		return nil, fmt.Errorf("type must be a struct or pointer to struct, got %s", t.Kind())
	}

	// Get the struct type for Go type mapping
	structType := t
	if t.Kind() == reflect.Pointer {
		structType = t.Elem()
	}

	// Generate JSON Schema from type with inline properties (no $ref).
	r := jsonschema.Reflector{Anonymous: true, DoNotReference: true}
	schema := r.ReflectFromType(structType)

	// Build required set for quick lookup
	required := make(map[string]bool)
	for _, name := range schema.Required {
		required[name] = true
	}

	// Build columns from schema properties
	var columns []jsonldbv1.Column
	for pair := schema.Properties.Oldest(); pair != nil; pair = pair.Next() {
		name := pair.Key
		prop := pair.Value

		// Find the Go field for type inference
		colType := jsonldbv1.ColumnTypeText
		for i := range structType.NumField() {
			field := structType.Field(i)
			if jsonFieldName(&field) == name {
				colType = goTypeToColumnType(field.Type)
				break
			}
		}

		columns = append(columns, jsonldbv1.Column{
			Name:        name,
			Type:        colType,
			Required:    required[name],
			Description: prop.Description,
		})
	}

	return columns, nil
}

// jsonFieldName returns the JSON field name for a struct field.
func jsonFieldName(field *reflect.StructField) string {
	tag := field.Tag.Get("json")
	if tag == "" || tag == "-" {
		return field.Name
	}
	// Handle "name,omitempty" format
	for i, c := range tag {
		if c == ',' {
			if i == 0 {
				return field.Name // ",omitempty" - no name specified, use Go field name
			}
			return tag[:i]
		}
	}
	return tag
}

// goTypeToColumnType maps Go types to JSONL column types.
func goTypeToColumnType(t reflect.Type) jsonldbv1.ColumnType {
	// Dereference pointers
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	// Check for Blob type (content-addressed external blob)
	if t == reflect.TypeFor[Blob]() {
		return jsonldbv1.ColumnTypeBlobRef
	}

	// Check for time.Time first (before switch)
	if t == reflect.TypeFor[time.Time]() {
		return jsonldbv1.ColumnTypeDate
	}

	// Check for []byte (blob)
	if t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8 {
		return jsonldbv1.ColumnTypeBlob
	}

	switch t.Kind() {
	case reflect.String:
		return jsonldbv1.ColumnTypeText
	case reflect.Bool:
		return jsonldbv1.ColumnTypeBool
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return jsonldbv1.ColumnTypeNumber
	case reflect.Struct, reflect.Slice, reflect.Array, reflect.Map:
		return jsonldbv1.ColumnTypeJSONB
	case reflect.Complex64, reflect.Complex128:
		// Complex numbers stored as JSON array [real, imag]
		return jsonldbv1.ColumnTypeJSONB
	case reflect.Invalid, reflect.Uintptr, reflect.Chan, reflect.Func,
		reflect.Interface, reflect.Pointer, reflect.UnsafePointer:
		// Unsupported types default to text
		return jsonldbv1.ColumnTypeText
	}
	// Unreachable: switch exhaustively handles all reflect.Kind values.
	// Kept as safety fallback.
	return jsonldbv1.ColumnTypeText
}
