// Persisted JSONL table header: format version, columns, and application properties.

// Package v1 defines version 1.x of the jsonldb table header, the first line of every
// JSONL table file.
package v1

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Header validation errors.
var (
	// ErrSchemaVersionRequired is returned for a header without a version.
	ErrSchemaVersionRequired = errors.New("schema version is required")
	// ErrSchemaVersionUnsupported is returned for a header version other than 1.x.
	ErrSchemaVersionUnsupported = errors.New("unsupported schema version (expected 1.x)")
)

// CurrentVersion is the current version of the JSONL table format.
const CurrentVersion = "1.0"

// ColumnType represents the type of a table column.
type ColumnType string

// Column types stored in the header.
const (
	ColumnTypeText    ColumnType = "text"
	ColumnTypeNumber  ColumnType = "number"
	ColumnTypeBool    ColumnType = "bool"
	ColumnTypeDate    ColumnType = "date"
	ColumnTypeBlob    ColumnType = "blob"
	ColumnTypeBlobRef ColumnType = "blob_ref" // content-addressed external blob
	ColumnTypeJSONB   ColumnType = "jsonb"
)

// Column represents a table column in storage.
type Column struct {
	Name        string     `json:"name"`
	Type        ColumnType `json:"type"`
	Required    bool       `json:"required,omitempty"`
	Description string     `json:"description,omitempty"`
}

// SchemaHeader is the first row of a JSONL data file containing schema and metadata.
// Used by Table[T] for generic schema storage.
type SchemaHeader struct {
	Version string   `json:"version"`
	Columns []Column `json:"columns"`
	// Properties stores application-specific schema for map fields (e.g., DataRecord.Data).
	// jsonldb treats this as opaque; the application interprets it.
	Properties json.RawMessage `json:"properties,omitempty"`
}

// Validate checks that the schema header is well-formed.
func (h *SchemaHeader) Validate() error {
	if h.Version == "" {
		return ErrSchemaVersionRequired
	}
	// Accept any 1.x version
	if !strings.HasPrefix(h.Version, "1.") {
		return ErrSchemaVersionUnsupported
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
