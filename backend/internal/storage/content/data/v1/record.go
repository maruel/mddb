// Persisted table rows stored in data.jsonl.

// Package v1 defines version 1 of the table schema persisted in workspaces: data.jsonl
// rows, and the properties and views stored in metadata.json.
package v1

import (
	"time"

	"github.com/maruel/ksid"
	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"
)

// DataRecord represents a record in a table.
type DataRecord struct {
	ID       ksid.ID        `json:"id" jsonschema:"description=Unique record identifier"`
	Data     map[string]any `json:"data" jsonschema:"description=Record field values keyed by property name"`
	Created  storagev1.Time `json:"created" jsonschema:"description=Record creation timestamp"`
	Modified storagev1.Time `json:"modified" jsonschema:"description=Last modification timestamp"`
}

// Clone returns a deep copy of the DataRecord.
func (r *DataRecord) Clone() *DataRecord {
	c := *r
	if r.Data != nil {
		c.Data = cloneDataMap(r.Data)
	}
	return &c
}

// cloneDataMap performs a deep clone of a record data map.
func cloneDataMap(m map[string]any) map[string]any {
	c := make(map[string]any, len(m))
	for k, v := range m {
		c[k] = cloneValue(v)
	}
	return c
}

// cloneValue deep clones a value that may appear in record data.
func cloneValue(v any) any {
	switch val := v.(type) {
	case []any:
		c := make([]any, len(val))
		for i, item := range val {
			c[i] = cloneValue(item)
		}
		return c
	case []string:
		return append([]string(nil), val...)
	case map[string]any:
		return cloneDataMap(val)
	default:
		// Primitives (string, float64, bool, nil) are immutable
		return v
	}
}

// GetID returns the DataRecord's ID.
func (r *DataRecord) GetID() ksid.ID {
	return r.ID
}

// Validate checks that the DataRecord is valid.
func (r *DataRecord) Validate() error {
	if r.ID.IsZero() {
		return ErrIDRequired
	}
	return nil
}

// GetString returns the string value for a property, or empty string if not found/wrong type.
func (r *DataRecord) GetString(name string) string {
	if v, ok := r.Data[name].(string); ok {
		return v
	}
	return ""
}

// GetNumber returns the numeric value for a property, or 0 if not found/wrong type.
func (r *DataRecord) GetNumber(name string) float64 {
	if v, ok := r.Data[name].(float64); ok {
		return v
	}
	return 0
}

// GetBool returns the boolean value for a property, or false if not found/wrong type.
func (r *DataRecord) GetBool(name string) bool {
	if v, ok := r.Data[name].(bool); ok {
		return v
	}
	return false
}

// GetTime returns the time value for a date property (stored as epoch seconds).
// Returns zero time if not found/wrong type.
func (r *DataRecord) GetTime(name string) time.Time {
	if v, ok := r.Data[name].(float64); ok {
		return time.Unix(int64(v), 0).UTC()
	}
	return time.Time{}
}

// GetStrings returns the string slice value for a multi-select property.
// Returns nil if not found/wrong type.
func (r *DataRecord) GetStrings(name string) []string {
	switch v := r.Data[name].(type) {
	case []string:
		return v
	case []any:
		// JSON unmarshal produces []any, convert to []string
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

// GetAny returns the raw value for a property, or nil if not found.
func (r *DataRecord) GetAny(name string) any {
	return r.Data[name]
}

// SetValue sets a value in the record's Data map.
func (r *DataRecord) SetValue(name string, value any) {
	if r.Data == nil {
		r.Data = make(map[string]any)
	}
	r.Data[name] = value
}

// SetTime sets a time value as epoch seconds in the record's Data map.
func (r *DataRecord) SetTime(name string, t time.Time) {
	r.SetValue(name, float64(t.Unix()))
}
