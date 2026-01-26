// Handles schema definition, column types, and reflection-based schema generation.

package jsonldb

import (
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/invopop/jsonschema"
)

var errSchemaVersionRequired = errors.New("schema version is required")

// currentVersion is the current version of the JSONL table format.
const currentVersion = "1.0"

// columnType represents the type of a table column.
type columnType string

const (
	columnTypeText    columnType = "text"
	columnTypeNumber  columnType = "number"
	columnTypeBool    columnType = "bool"
	columnTypeDate    columnType = "date"
	columnTypeBlob    columnType = "blob"
	columnTypeBlobRef columnType = "blob_ref" // content-addressed external blob
	columnTypeJSONB   columnType = "jsonb"
)

// column represents a table column in storage.
type column struct {
	Name        string     `json:"name"`
	Type        columnType `json:"type"`
	Required    bool       `json:"required,omitempty"`
	Description string     `json:"description,omitempty"`
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
		return errSchemaVersionRequired
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

// schemaFromType extracts column definitions using JSON Schema reflection.
//
// It uses github.com/invopop/jsonschema to extract field descriptions from
// `jsonschema:"description=..."` tags and required fields from the schema.
func schemaFromType[T any]() ([]column, error) {
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
	var columns []column
	for pair := schema.Properties.Oldest(); pair != nil; pair = pair.Next() {
		name := pair.Key
		prop := pair.Value

		// Find the Go field for type inference
		colType := columnTypeText
		for i := range structType.NumField() {
			field := structType.Field(i)
			if jsonFieldName(&field) == name {
				colType = goTypeToColumnType(field.Type)
				break
			}
		}

		columns = append(columns, column{
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
func goTypeToColumnType(t reflect.Type) columnType {
	// Dereference pointers
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	// Check for Blob type (content-addressed external blob)
	if t == reflect.TypeFor[Blob]() {
		return columnTypeBlobRef
	}

	// Check for time.Time first (before switch)
	if t == reflect.TypeFor[time.Time]() {
		return columnTypeDate
	}

	// Check for []byte (blob)
	if t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8 {
		return columnTypeBlob
	}

	switch t.Kind() {
	case reflect.String:
		return columnTypeText
	case reflect.Bool:
		return columnTypeBool
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return columnTypeNumber
	case reflect.Struct, reflect.Slice, reflect.Array, reflect.Map:
		return columnTypeJSONB
	case reflect.Complex64, reflect.Complex128:
		// Complex numbers stored as JSON array [real, imag]
		return columnTypeJSONB
	case reflect.Invalid, reflect.Uintptr, reflect.Chan, reflect.Func,
		reflect.Interface, reflect.Pointer, reflect.UnsafePointer:
		// Unsupported types default to text
		return columnTypeText
	}
	// Unreachable: switch exhaustively handles all reflect.Kind values.
	// Kept as safety fallback.
	return columnTypeText
}
