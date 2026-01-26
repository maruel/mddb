// Defines the Blob type and content-addressed reference format.

package jsonldb

import (
	"encoding/json"
	"errors"
	"io"
	"reflect"
)

// BlobRef is a content-addressed blob reference in format "sha256:<BASE32>-<size>".
type BlobRef string

const blobRefPrefix = "sha256:"

// Validate checks if the blob reference is valid.
// Format: "sha256:<hash>-<size>" where hash is 52 uppercase base32 hex chars (0-9, A-V) and size is decimal digits.
func (r BlobRef) Validate() error {
	if r == "" {
		return nil // Empty ref is valid (unset).
	}
	// "sha256:" (7) + 52 base32 + "-" + at least 1 digit = 61 minimum
	if len(r) < 61 || r[:7] != blobRefPrefix || r[59] != '-' {
		return errInvalidBlobRef
	}
	for i := 7; i < 59; i++ {
		c := r[i]
		// Base32 hex alphabet: 0-9, A-V (uppercase only)
		if (c < '0' || c > '9') && (c < 'A' || c > 'V') {
			return errInvalidBlobRef
		}
	}
	// Validate size portion (digits only, at least one digit).
	for i := 60; i < len(r); i++ {
		if r[i] < '0' || r[i] > '9' {
			return errInvalidBlobRef
		}
	}
	return nil
}

// IsZero returns true if the blob reference is unset.
func (r BlobRef) IsZero() bool {
	return r == ""
}

// Blob represents a reference to content-addressed binary data stored externally.
//
// Blob fields store only a reference in the JSONL table; actual data lives in
// a sibling blobs directory. Use [Table.NewBlob] to create blobs, then
// [Blob.Reader] to stream data back. Blob fields in row structs are automatically
// discovered via reflection, including nested structs and slices.
type Blob struct {
	Ref   BlobRef    `json:"ref,omitzero"` // "sha256:<BASE32>-<size>" format
	store *blobStore // set by Table after unmarshal, not serialized
}

// IsZero returns true if the blob is unset (null reference, no ref assigned).
// Implements the interface for json omitzero.
func (b *Blob) IsZero() bool {
	return b.Ref.IsZero()
}

// Reader opens the blob file for streaming read.
//
// Returns an error if the blob is unset or the file cannot be opened.
// The caller must close the returned ReadCloser.
func (b *Blob) Reader() (io.ReadCloser, error) {
	if b.IsZero() {
		return nil, errUnsetBlob
	}
	if b.store == nil {
		return nil, errNoBlobStore
	}
	return b.store.open(b.Ref)
}

// Clone returns a shallow copy of the blob with the same ref and store reference.
func (b *Blob) Clone() Blob {
	return Blob{Ref: b.Ref, store: b.store}
}

// MarshalJSON implements json.Marshaler. Only the ref is serialized.
func (b *Blob) MarshalJSON() ([]byte, error) {
	if b.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(b.Ref)
}

// UnmarshalJSON implements json.Unmarshaler. Only the ref is deserialized.
func (b *Blob) UnmarshalJSON(data []byte) error {
	// Handle null.
	if string(data) == "null" {
		b.Ref = ""
		return nil
	}
	return json.Unmarshal(data, &b.Ref)
}

//

var (
	blobType          = reflect.TypeFor[Blob]()
	errUnsetBlob      = errors.New("blob is unset")
	errNoBlobStore    = errors.New("blob has no store reference")
	errInvalidBlobRef = errors.New("invalid blob ref")
)

// blobFields returns pointers to all Blob fields in a struct using reflection.
// Handles nested structs and embedded fields. Returns nil if v is not a pointer to struct.
func blobFields(v any) []*Blob {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return nil
	}
	rv = rv.Elem()
	if rv.Kind() != reflect.Struct {
		return nil
	}
	return collectBlobFields(rv)
}

func collectBlobFields(v reflect.Value) []*Blob {
	var blobs []*Blob
	t := v.Type()
	for i := range v.NumField() {
		field := v.Field(i)
		fieldType := t.Field(i)

		// Skip unexported fields.
		if !fieldType.IsExported() {
			continue
		}

		blobs = append(blobs, collectBlobValues(field)...)
	}
	return blobs
}

func collectBlobValues(v reflect.Value) []*Blob {
	switch v.Kind() { //nolint:exhaustive // Only handle types that can contain Blob fields
	case reflect.Struct:
		if v.Type() == blobType {
			return []*Blob{v.Addr().Interface().(*Blob)}
		}
		return collectBlobFields(v)
	case reflect.Pointer:
		if !v.IsNil() {
			return collectBlobValues(v.Elem())
		}
	case reflect.Array, reflect.Slice:
		var blobs []*Blob
		for i := range v.Len() {
			blobs = append(blobs, collectBlobValues(v.Index(i))...)
		}
		return blobs
	}
	return nil
}
