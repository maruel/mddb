// Persisted table definitions stored in metadata.json.

package v1

import (
	"bytes"
	"encoding/json"
	"slices"
	"time"

	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"
)

// TableVersion is the metadata.json version the store writes. Readers accept any
// 1.x and treat a missing version as 1.0.
const TableVersion = "1.0"

// Table is metadata.json, which defines a table. The store writes every key, and
// omits Created only when it rewrites a table whose file had none; the Notion
// importer writes only Views. Fields follow key order, so the encoding matches
// the map the store wrote before this type existed, byte for byte.
type Table struct {
	// Created keeps its stored bytes through rewrites: quota admission compares
	// exact sizes, and earlier releases wrote other widths.
	Created    Timestamp  `json:"created,omitempty"`
	Modified   Timestamp  `json:"modified,omitempty"`
	Properties []Property `json:"properties"`
	Title      string     `json:"title"`
	Version    string     `json:"version,omitempty"`
	Views      []View     `json:"views,omitempty"`
}

// Timestamp is a metadata.json time as stored: a storagev1.Time number, or an
// RFC 3339 string in tables written before 2026-01-24. It keeps its bytes.
type Timestamp []byte

// NewTimestamp encodes t the way the store writes it.
func NewTimestamp(t storagev1.Time) Timestamp {
	b, _ := t.MarshalJSON() // Time encoding cannot fail.
	return b
}

// Time decodes either encoding. A missing or unrecognized value is zero, as
// readers never rejected one.
func (t Timestamp) Time() storagev1.Time {
	var v storagev1.Time
	if err := json.Unmarshal(t, &v); err == nil {
		return v
	}
	var s string
	if err := json.Unmarshal(t, &s); err == nil {
		if parsed, err := time.Parse(time.RFC3339, s); err == nil {
			return storagev1.ToTime(parsed)
		}
	}
	return 0
}

// MarshalJSON writes the stored bytes.
func (t Timestamp) MarshalJSON() ([]byte, error) {
	if len(t) == 0 {
		return []byte("null"), nil
	}
	return t, nil
}

// UnmarshalJSON keeps the stored bytes; JSON null is no timestamp.
func (t *Timestamp) UnmarshalJSON(b []byte) error {
	if bytes.Equal(b, []byte("null")) {
		*t = nil
		return nil
	}
	*t = slices.Clone(b)
	return nil
}
