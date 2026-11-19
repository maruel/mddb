// Tests for JSONLDB column serialization.

package jsonldb

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	jsonldbv1 "github.com/maruel/mddb/backend/internal/jsonldb/data/v1"

	"github.com/maruel/ksid"
)

// TestSchemaHeader tests the schemaHeader type and its methods.
func TestSchemaHeader(t *testing.T) {
	t.Run("Validate", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			tests := []struct {
				name   string
				header jsonldbv1.SchemaHeader
			}{
				{
					"minimal valid header",
					jsonldbv1.SchemaHeader{Version: "1.0", Columns: []jsonldbv1.Column{}},
				},
				{
					"version 1.1",
					jsonldbv1.SchemaHeader{Version: "1.1", Columns: []jsonldbv1.Column{}},
				},
				{
					"version 1.99",
					jsonldbv1.SchemaHeader{Version: "1.99", Columns: []jsonldbv1.Column{}},
				},
				{
					"header with columns",
					jsonldbv1.SchemaHeader{
						Version: "1.0",
						Columns: []jsonldbv1.Column{
							{Name: "id", Type: jsonldbv1.ColumnTypeNumber},
							{Name: "name", Type: jsonldbv1.ColumnTypeText},
						},
					},
				},
				{
					"header with description",
					jsonldbv1.SchemaHeader{
						Version: "1.0",
						Columns: []jsonldbv1.Column{
							{Name: "id", Type: jsonldbv1.ColumnTypeNumber, Required: true, Description: "Primary key"},
						},
					},
				},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					if err := tt.header.Validate(); err != nil {
						t.Errorf("Validate() error = %v, want nil", err)
					}
				})
			}
		})

		t.Run("errors", func(t *testing.T) {
			tests := []struct {
				name   string
				header jsonldbv1.SchemaHeader
			}{
				{
					"empty version",
					jsonldbv1.SchemaHeader{Version: "", Columns: []jsonldbv1.Column{}},
				},
				{
					"unsupported version 2.0",
					jsonldbv1.SchemaHeader{Version: "2.0", Columns: []jsonldbv1.Column{}},
				},
				{
					"unsupported version 0.9",
					jsonldbv1.SchemaHeader{Version: "0.9", Columns: []jsonldbv1.Column{}},
				},
				{
					"column with empty name",
					jsonldbv1.SchemaHeader{
						Version: "1.0",
						Columns: []jsonldbv1.Column{{Name: "", Type: jsonldbv1.ColumnTypeText}},
					},
				},
				{
					"column with empty type",
					jsonldbv1.SchemaHeader{
						Version: "1.0",
						Columns: []jsonldbv1.Column{{Name: "id", Type: ""}},
					},
				},
				{
					"multiple columns one invalid",
					jsonldbv1.SchemaHeader{
						Version: "1.0",
						Columns: []jsonldbv1.Column{
							{Name: "id", Type: jsonldbv1.ColumnTypeNumber},
							{Name: "", Type: jsonldbv1.ColumnTypeText}, // invalid
						},
					},
				},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					if err := tt.header.Validate(); err == nil {
						t.Error("Validate() expected error, got nil")
					}
				})
			}
		})
	})
}

// schemaTestStruct is a struct for schema testing (value receiver Row implementation).
type schemaTestStruct struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func (s schemaTestStruct) Clone() schemaTestStruct { return s }
func (s schemaTestStruct) GetID() ksid.ID          { return ksid.ID(s.ID) } //nolint:gosec // test code with small integers
func (s schemaTestStruct) Validate() error         { return nil }

// TestSchemaFromType tests schemaFromType indirectly through Table creation.
// Direct testing of schemaFromType is limited due to jsonschema library behavior.
func TestSchemaFromType(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		// Test that NewTable successfully creates schema for testRow type
		t.Run("creates schema through table with pointer type", func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "schema.jsonl")
			table, err := NewTable[*testRow](path)
			if err != nil {
				t.Fatalf("NewTable error: %v", err)
			}

			// Table should be created successfully with auto-discovered schema
			if table == nil {
				t.Error("NewTable returned nil table")
			}

			// Write a row to create the file
			err = table.Append(&testRow{ID: 1, Name: "Test"})
			if err != nil {
				t.Fatalf("Append error: %v", err)
			}

			// Reload and verify schema was persisted
			table2, err := NewTable[*testRow](path)
			if err != nil {
				t.Fatalf("NewTable reload error: %v", err)
			}
			if table2.Len() != 1 {
				t.Errorf("Reloaded table Len() = %d, want 1", table2.Len())
			}
		})

		// Test with value type (non-pointer struct)
		t.Run("creates schema for value type", func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "value.jsonl")
			table, err := NewTable[schemaTestStruct](path)
			if err != nil {
				t.Fatalf("NewTable error: %v", err)
			}

			if table == nil {
				t.Error("NewTable returned nil table")
			}
		})

		// Test with validatingRow type
		t.Run("creates schema for validating row type", func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "validating.jsonl")
			table, err := NewTable[*validatingRow](path)
			if err != nil {
				t.Fatalf("NewTable error: %v", err)
			}

			if table == nil {
				t.Error("NewTable returned nil table")
			}
		})

		// Direct test of schemaFromType with struct type (non-pointer)
		t.Run("schemaFromType with struct type", func(t *testing.T) {
			columns, err := schemaFromType[schemaTestStruct]()
			if err != nil {
				t.Fatalf("schemaFromType error: %v", err)
			}
			// Just verify it succeeds (jsonschema may return empty columns for test types)
			_ = columns
		})

		// Direct test of schemaFromType with pointer to struct
		t.Run("schemaFromType with pointer to struct", func(t *testing.T) {
			columns, err := schemaFromType[*testRow]()
			if err != nil {
				t.Fatalf("schemaFromType error: %v", err)
			}
			// Just verify it succeeds
			_ = columns
		})
	})

	t.Run("errors", func(t *testing.T) {
		t.Run("non-struct type", func(t *testing.T) {
			_, err := schemaFromType[int]()
			if err == nil {
				t.Error("schemaFromType(int) expected error, got nil")
			}
		})

		t.Run("pointer to non-struct", func(t *testing.T) {
			_, err := schemaFromType[*int]()
			if err == nil {
				t.Error("schemaFromType(*int) expected error, got nil")
			}
		})

		t.Run("slice type", func(t *testing.T) {
			_, err := schemaFromType[[]int]()
			if err == nil {
				t.Error("schemaFromType([]int) expected error, got nil")
			}
		})

		t.Run("map type", func(t *testing.T) {
			_, err := schemaFromType[map[string]int]()
			if err == nil {
				t.Error("schemaFromType(map) expected error, got nil")
			}
		})
	})
}

// TestJsonFieldName tests the jsonFieldName helper function.
func TestJsonFieldName(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		type testStruct struct {
			NoTag      string
			WithTag    string `json:"custom_name"`
			WithOmit   string `json:"with_omit,omitempty"`
			OnlyOmit   string `json:",omitempty"`
			DashTag    string `json:"-"`
			EmptyTag   string `json:""`
			ComplexTag string `json:"complex,omitempty,string"`
		}

		tests := []struct {
			fieldName string
			want      string
		}{
			{"NoTag", "NoTag"},
			{"WithTag", "custom_name"},
			{"WithOmit", "with_omit"},
			{"OnlyOmit", "OnlyOmit"}, // ",omitempty" returns Go field name
			{"DashTag", "DashTag"},   // "-" returns field name
			{"EmptyTag", "EmptyTag"}, // empty tag returns field name
			{"ComplexTag", "complex"},
		}

		typ := reflect.TypeFor[testStruct]()
		for _, tt := range tests {
			t.Run(tt.fieldName, func(t *testing.T) {
				field, _ := typ.FieldByName(tt.fieldName)
				got := jsonFieldName(&field)
				if got != tt.want {
					t.Errorf("jsonFieldName(%q) = %q, want %q", tt.fieldName, got, tt.want)
				}
			})
		}
	})
}

// TestGoTypeToColumnType tests the goTypeToColumnType helper function.
func TestGoTypeToColumnType(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		tests := []struct {
			name string
			typ  reflect.Type
			want jsonldbv1.ColumnType
		}{
			// Basic types
			{"string", reflect.TypeFor[string](), jsonldbv1.ColumnTypeText},
			{"bool", reflect.TypeFor[bool](), jsonldbv1.ColumnTypeBool},
			{"int", reflect.TypeFor[int](), jsonldbv1.ColumnTypeNumber},
			{"int8", reflect.TypeFor[int8](), jsonldbv1.ColumnTypeNumber},
			{"int16", reflect.TypeFor[int16](), jsonldbv1.ColumnTypeNumber},
			{"int32", reflect.TypeFor[int32](), jsonldbv1.ColumnTypeNumber},
			{"int64", reflect.TypeFor[int64](), jsonldbv1.ColumnTypeNumber},
			{"uint", reflect.TypeFor[uint](), jsonldbv1.ColumnTypeNumber},
			{"uint8", reflect.TypeFor[uint8](), jsonldbv1.ColumnTypeNumber},
			{"uint16", reflect.TypeFor[uint16](), jsonldbv1.ColumnTypeNumber},
			{"uint32", reflect.TypeFor[uint32](), jsonldbv1.ColumnTypeNumber},
			{"uint64", reflect.TypeFor[uint64](), jsonldbv1.ColumnTypeNumber},
			{"float32", reflect.TypeFor[float32](), jsonldbv1.ColumnTypeNumber},
			{"float64", reflect.TypeFor[float64](), jsonldbv1.ColumnTypeNumber},

			// Special types
			{"time.Time", reflect.TypeFor[time.Time](), jsonldbv1.ColumnTypeDate},
			{"[]byte", reflect.TypeFor[[]byte](), jsonldbv1.ColumnTypeBlob},

			// Complex types -> JSONB
			{"struct", reflect.TypeFor[struct{}](), jsonldbv1.ColumnTypeJSONB},
			{"slice", reflect.TypeFor[[]string](), jsonldbv1.ColumnTypeJSONB},
			{"array", reflect.TypeFor[[5]int](), jsonldbv1.ColumnTypeJSONB},
			{"map", reflect.TypeFor[map[string]int](), jsonldbv1.ColumnTypeJSONB},
			{"complex64", reflect.TypeFor[complex64](), jsonldbv1.ColumnTypeJSONB},
			{"complex128", reflect.TypeFor[complex128](), jsonldbv1.ColumnTypeJSONB},

			// Pointer types (should dereference)
			{"*string", reflect.TypeFor[*string](), jsonldbv1.ColumnTypeText},
			{"*int", reflect.TypeFor[*int](), jsonldbv1.ColumnTypeNumber},
			{"*bool", reflect.TypeFor[*bool](), jsonldbv1.ColumnTypeBool},
			{"*time.Time", reflect.TypeFor[*time.Time](), jsonldbv1.ColumnTypeDate},

			// Unsupported types -> text fallback
			{"chan", reflect.TypeFor[chan int](), jsonldbv1.ColumnTypeText},
			{"func", reflect.TypeFor[func()](), jsonldbv1.ColumnTypeText},
			{"interface", reflect.TypeFor[any](), jsonldbv1.ColumnTypeText},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := goTypeToColumnType(tt.typ)
				if got != tt.want {
					t.Errorf("goTypeToColumnType(%v) = %q, want %q", tt.typ, got, tt.want)
				}
			})
		}
	})
}
