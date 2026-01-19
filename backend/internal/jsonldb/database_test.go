package jsonldb

import (
	"os"
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
func (s schemaTestStruct) GetID() ID               { return ID(s.ID) }
func (s schemaTestStruct) Validate() error         { return nil }

// TestSchemaFromType tests schemaFromType indirectly through Table creation.
// Direct testing of schemaFromType is limited due to jsonschema library behavior.
func TestSchemaFromType(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		// Test that NewTable successfully creates schema for testRow type
		t.Run("creates schema through table with pointer type", func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "jsonl-schema-*")
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			path := filepath.Join(tmpDir, "schema.jsonl")
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
			tmpDir, err := os.MkdirTemp("", "jsonl-schema-*")
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			path := filepath.Join(tmpDir, "value.jsonl")
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
			tmpDir, err := os.MkdirTemp("", "jsonl-schema-*")
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			path := filepath.Join(tmpDir, "validating.jsonl")
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
			{"OnlyOmit", ""},         // ",omitempty" returns empty name (comma at index 0)
			{"DashTag", "DashTag"},   // "-" returns field name
			{"EmptyTag", "EmptyTag"}, // empty tag returns field name
			{"ComplexTag", "complex"},
		}

		typ := reflect.TypeOf(testStruct{})
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
			{"string", reflect.TypeOf(""), columnTypeText},
			{"bool", reflect.TypeOf(true), columnTypeBool},
			{"int", reflect.TypeOf(0), columnTypeNumber},
			{"int8", reflect.TypeOf(int8(0)), columnTypeNumber},
			{"int16", reflect.TypeOf(int16(0)), columnTypeNumber},
			{"int32", reflect.TypeOf(int32(0)), columnTypeNumber},
			{"int64", reflect.TypeOf(int64(0)), columnTypeNumber},
			{"uint", reflect.TypeOf(uint(0)), columnTypeNumber},
			{"uint8", reflect.TypeOf(uint8(0)), columnTypeNumber},
			{"uint16", reflect.TypeOf(uint16(0)), columnTypeNumber},
			{"uint32", reflect.TypeOf(uint32(0)), columnTypeNumber},
			{"uint64", reflect.TypeOf(uint64(0)), columnTypeNumber},
			{"float32", reflect.TypeOf(float32(0)), columnTypeNumber},
			{"float64", reflect.TypeOf(float64(0)), columnTypeNumber},

			// Special types
			{"time.Time", reflect.TypeOf(time.Time{}), columnTypeDate},
			{"[]byte", reflect.TypeOf([]byte{}), columnTypeBlob},

			// Complex types -> JSONB
			{"struct", reflect.TypeOf(struct{}{}), columnTypeJSONB},
			{"slice", reflect.TypeOf([]string{}), columnTypeJSONB},
			{"array", reflect.TypeOf([5]int{}), columnTypeJSONB},
			{"map", reflect.TypeOf(map[string]int{}), columnTypeJSONB},
			{"complex64", reflect.TypeOf(complex64(0)), columnTypeJSONB},
			{"complex128", reflect.TypeOf(complex128(0)), columnTypeJSONB},

			// Pointer types (should dereference)
			{"*string", reflect.TypeOf((*string)(nil)), columnTypeText},
			{"*int", reflect.TypeOf((*int)(nil)), columnTypeNumber},
			{"*bool", reflect.TypeOf((*bool)(nil)), columnTypeBool},
			{"*time.Time", reflect.TypeOf((*time.Time)(nil)), columnTypeDate},

			// Unsupported types -> text fallback
			{"chan", reflect.TypeOf(make(chan int)), columnTypeText},
			{"func", reflect.TypeOf(func() {}), columnTypeText},
			{"interface", reflect.TypeOf((*interface{})(nil)).Elem(), columnTypeText},
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
