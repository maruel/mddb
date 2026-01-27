package jsonldb

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// TestSchemaHeader tests the schemaHeader type and its methods.
func TestSchemaHeader(t *testing.T) {
	t.Run("Validate", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			tests := []struct {
				name   string
				header schemaHeader
			}{
				{
					"minimal valid header",
					schemaHeader{Version: "1.0", Columns: []column{}},
				},
				{
					"version 1.1",
					schemaHeader{Version: "1.1", Columns: []column{}},
				},
				{
					"version 1.99",
					schemaHeader{Version: "1.99", Columns: []column{}},
				},
				{
					"header with columns",
					schemaHeader{
						Version: "1.0",
						Columns: []column{
							{Name: "id", Type: columnTypeNumber},
							{Name: "name", Type: columnTypeText},
						},
					},
				},
				{
					"header with description",
					schemaHeader{
						Version: "1.0",
						Columns: []column{
							{Name: "id", Type: columnTypeNumber, Required: true, Description: "Primary key"},
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
				header schemaHeader
			}{
				{
					"empty version",
					schemaHeader{Version: "", Columns: []column{}},
				},
				{
					"unsupported version 2.0",
					schemaHeader{Version: "2.0", Columns: []column{}},
				},
				{
					"unsupported version 0.9",
					schemaHeader{Version: "0.9", Columns: []column{}},
				},
				{
					"column with empty name",
					schemaHeader{
						Version: "1.0",
						Columns: []column{{Name: "", Type: columnTypeText}},
					},
				},
				{
					"column with empty type",
					schemaHeader{
						Version: "1.0",
						Columns: []column{{Name: "id", Type: ""}},
					},
				},
				{
					"multiple columns one invalid",
					schemaHeader{
						Version: "1.0",
						Columns: []column{
							{Name: "id", Type: columnTypeNumber},
							{Name: "", Type: columnTypeText}, // invalid
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
func (s schemaTestStruct) GetID() ID               { return ID(s.ID) } //nolint:gosec // test code with small integers
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
			want columnType
		}{
			// Basic types
			{"string", reflect.TypeFor[string](), columnTypeText},
			{"bool", reflect.TypeFor[bool](), columnTypeBool},
			{"int", reflect.TypeFor[int](), columnTypeNumber},
			{"int8", reflect.TypeFor[int8](), columnTypeNumber},
			{"int16", reflect.TypeFor[int16](), columnTypeNumber},
			{"int32", reflect.TypeFor[int32](), columnTypeNumber},
			{"int64", reflect.TypeFor[int64](), columnTypeNumber},
			{"uint", reflect.TypeFor[uint](), columnTypeNumber},
			{"uint8", reflect.TypeFor[uint8](), columnTypeNumber},
			{"uint16", reflect.TypeFor[uint16](), columnTypeNumber},
			{"uint32", reflect.TypeFor[uint32](), columnTypeNumber},
			{"uint64", reflect.TypeFor[uint64](), columnTypeNumber},
			{"float32", reflect.TypeFor[float32](), columnTypeNumber},
			{"float64", reflect.TypeFor[float64](), columnTypeNumber},

			// Special types
			{"time.Time", reflect.TypeFor[time.Time](), columnTypeDate},
			{"[]byte", reflect.TypeFor[[]byte](), columnTypeBlob},

			// Complex types -> JSONB
			{"struct", reflect.TypeFor[struct{}](), columnTypeJSONB},
			{"slice", reflect.TypeFor[[]string](), columnTypeJSONB},
			{"array", reflect.TypeFor[[5]int](), columnTypeJSONB},
			{"map", reflect.TypeFor[map[string]int](), columnTypeJSONB},
			{"complex64", reflect.TypeFor[complex64](), columnTypeJSONB},
			{"complex128", reflect.TypeFor[complex128](), columnTypeJSONB},

			// Pointer types (should dereference)
			{"*string", reflect.TypeFor[*string](), columnTypeText},
			{"*int", reflect.TypeFor[*int](), columnTypeNumber},
			{"*bool", reflect.TypeFor[*bool](), columnTypeBool},
			{"*time.Time", reflect.TypeFor[*time.Time](), columnTypeDate},

			// Unsupported types -> text fallback
			{"chan", reflect.TypeFor[chan int](), columnTypeText},
			{"func", reflect.TypeFor[func()](), columnTypeText},
			{"interface", reflect.TypeFor[any](), columnTypeText},
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

// TestColumnTypes tests the column type constants.
func TestColumnTypes(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		tests := []struct {
			name string
			typ  columnType
			want string
		}{
			{"text", columnTypeText, "text"},
			{"number", columnTypeNumber, "number"},
			{"bool", columnTypeBool, "bool"},
			{"date", columnTypeDate, "date"},
			{"blob", columnTypeBlob, "blob"},
			{"jsonb", columnTypeJSONB, "jsonb"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if string(tt.typ) != tt.want {
					t.Errorf("columnType %s = %q, want %q", tt.name, tt.typ, tt.want)
				}
			})
		}
	})
}

// TestColumn tests the column struct.
func TestColumn(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		tests := []struct {
			name string
			col  column
		}{
			{
				"minimal column",
				column{Name: "id", Type: columnTypeNumber},
			},
			{
				"column with all fields",
				column{
					Name:        "created_at",
					Type:        columnTypeDate,
					Required:    true,
					Description: "Creation timestamp",
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// Just verify the struct can be constructed
				if tt.col.Name == "" {
					t.Error("column Name should not be empty")
				}
				if tt.col.Type == "" {
					t.Error("column Type should not be empty")
				}
			})
		}
	})
}

// TestCurrentVersion tests the currentVersion constant.
func TestCurrentVersion(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		if currentVersion == "" {
			t.Error("currentVersion should not be empty")
		}
		if currentVersion != "1.0" {
			t.Errorf("currentVersion = %q, want \"1.0\"", currentVersion)
		}
	})
}
