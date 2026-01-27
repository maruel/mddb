// Provides typed access to record data values based on property schema.

package content

import (
	"time"
)

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
