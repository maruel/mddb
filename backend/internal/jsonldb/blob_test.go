package jsonldb

import (
	"errors"
	"strings"
	"testing"
)

func TestBlob(t *testing.T) {
	t.Run("IsZero", func(t *testing.T) {
		tests := []struct {
			name string
			blob Blob
			want bool
		}{
			{"unset", Blob{}, true},
			{"with hash", Blob{Ref: "abc123"}, false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if got := tt.blob.IsZero(); got != tt.want {
					t.Errorf("IsZero() = %v, want %v", got, tt.want)
				}
			})
		}
	})

	t.Run("MarshalJSON", func(t *testing.T) {
		tests := []struct {
			name string
			blob Blob
			want string
		}{
			{"unset", Blob{}, "null"},
			{"with hash", Blob{Ref: "abc123"}, `"abc123"`},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := tt.blob.MarshalJSON()
				if err != nil {
					t.Fatalf("MarshalJSON() error = %v", err)
				}
				if string(got) != tt.want {
					t.Errorf("MarshalJSON() = %s, want %s", got, tt.want)
				}
			})
		}
	})

	t.Run("UnmarshalJSON", func(t *testing.T) {
		tests := []struct {
			name    string
			data    string
			want    string
			wantErr bool
		}{
			{"null", "null", "", false},
			{"hash", `"abc123"`, "abc123", false},
			{"invalid", `123`, "", true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var b Blob
				err := b.UnmarshalJSON([]byte(tt.data))
				if (err != nil) != tt.wantErr {
					t.Errorf("UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if string(b.Ref) != tt.want {
					t.Errorf("UnmarshalJSON() hash = %q, want %q", b.Ref, tt.want)
				}
			})
		}
	})

	t.Run("Reader", func(t *testing.T) {
		t.Run("unset blob", func(t *testing.T) {
			b := Blob{}
			_, err := b.Reader()
			if !errors.Is(err, errUnsetBlob) {
				t.Errorf("Reader() error = %v, want errUnsetBlob", err)
			}
		})

		t.Run("no store", func(t *testing.T) {
			b := Blob{Ref: "abc123"}
			_, err := b.Reader()
			if !errors.Is(err, errNoBlobStore) {
				t.Errorf("Reader() error = %v, want errNoBlobStore", err)
			}
		})
	})

	t.Run("Clone", func(t *testing.T) {
		store := &blobStore{dir: "/tmp"}
		b := Blob{Ref: "abc123", store: store}
		cloned := b.Clone()

		if cloned.Ref != b.Ref {
			t.Errorf("Clone().Hash = %q, want %q", cloned.Ref, b.Ref)
		}
		if cloned.store != b.store {
			t.Error("Clone().store differs from original")
		}
	})
}

func TestBlobRef(t *testing.T) {
	t.Run("Validate", func(t *testing.T) {
		// Base32 hex alphabet: 0-9, A-V (uppercase only)
		tests := []struct {
			name    string
			ref     string
			wantErr bool
		}{
			{"empty is valid", "", false},
			{"valid uppercase A-V", "sha256:" + strings.Repeat("A", 52) + "-0", false},
			{"valid with digits", "sha256:" + strings.Repeat("5", 52) + "-123", false},
			{"valid V boundary", "sha256:" + strings.Repeat("V", 52) + "-999999", false},
			{"valid mixed", "sha256:" + strings.Repeat("0", 26) + strings.Repeat("V", 26) + "-1", false},
			{"missing prefix", strings.Repeat("A", 52) + "-0", true},
			{"wrong prefix", "sha512:" + strings.Repeat("A", 52) + "-0", true},
			{"no size", "sha256:" + strings.Repeat("A", 52), true},
			{"no dash", "sha256:" + strings.Repeat("A", 52) + "0", true},
			{"short hash", "sha256:" + strings.Repeat("A", 51) + "-0", true},
			{"long hash", "sha256:" + strings.Repeat("A", 53) + "-0", true},
			{"lowercase rejected", "sha256:" + strings.Repeat("a", 52) + "-1", true},
			{"mixed case rejected", "sha256:" + strings.Repeat("A", 26) + strings.Repeat("a", 26) + "-1", true},
			{"invalid char W", "sha256:" + strings.Repeat("W", 52) + "-0", true},
			{"invalid char X", "sha256:" + strings.Repeat("X", 52) + "-0", true},
			{"invalid char Z", "sha256:" + strings.Repeat("Z", 52) + "-0", true},
			{"invalid char", "sha256:" + strings.Repeat("!", 52) + "-0", true},
			{"empty size", "sha256:" + strings.Repeat("A", 52) + "-", true},
			{"non-digit size", "sha256:" + strings.Repeat("A", 52) + "-abc", true},
			{"size with letter", "sha256:" + strings.Repeat("A", 52) + "-12a", true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := BlobRef(tt.ref).Validate()
				if (err != nil) != tt.wantErr {
					t.Errorf("BlobRef(%q).Validate() error = %v, wantErr %v", tt.ref, err, tt.wantErr)
				}
			})
		}
	})
}

func TestBlobFields(t *testing.T) {
	t.Run("single blob", func(t *testing.T) {
		type row struct {
			Content Blob
		}
		r := &row{Content: Blob{Ref: "abc"}}
		blobs := blobFields(r)
		if len(blobs) != 1 {
			t.Fatalf("got %d blobs, want 1", len(blobs))
		}
		if blobs[0].Ref != "abc" {
			t.Errorf("got ref %q, want %q", blobs[0].Ref, "abc")
		}
	})

	t.Run("slice of blobs", func(t *testing.T) {
		type row struct {
			Attachments []Blob
		}
		r := &row{Attachments: []Blob{{Ref: "a"}, {Ref: "b"}, {Ref: "c"}}}
		blobs := blobFields(r)
		if len(blobs) != 3 {
			t.Fatalf("got %d blobs, want 3", len(blobs))
		}
		for i, want := range []BlobRef{"a", "b", "c"} {
			if blobs[i].Ref != want {
				t.Errorf("blob[%d].Ref = %q, want %q", i, blobs[i].Ref, want)
			}
		}
	})

	t.Run("array of blobs", func(t *testing.T) {
		type row struct {
			Files [2]Blob
		}
		r := &row{Files: [2]Blob{{Ref: "x"}, {Ref: "y"}}}
		blobs := blobFields(r)
		if len(blobs) != 2 {
			t.Fatalf("got %d blobs, want 2", len(blobs))
		}
	})

	t.Run("nested struct", func(t *testing.T) {
		type inner struct {
			Data Blob
		}
		type row struct {
			Nested inner
		}
		r := &row{Nested: inner{Data: Blob{Ref: "nested"}}}
		blobs := blobFields(r)
		if len(blobs) != 1 {
			t.Fatalf("got %d blobs, want 1", len(blobs))
		}
		if blobs[0].Ref != "nested" {
			t.Errorf("got ref %q, want %q", blobs[0].Ref, "nested")
		}
	})

	t.Run("slice of structs with blobs", func(t *testing.T) {
		type attachment struct {
			File Blob
		}
		type row struct {
			Attachments []attachment
		}
		r := &row{Attachments: []attachment{{File: Blob{Ref: "f1"}}, {File: Blob{Ref: "f2"}}}}
		blobs := blobFields(r)
		if len(blobs) != 2 {
			t.Fatalf("got %d blobs, want 2", len(blobs))
		}
	})

	t.Run("nil slice", func(t *testing.T) {
		type row struct {
			Attachments []Blob
		}
		r := &row{}
		blobs := blobFields(r)
		if len(blobs) != 0 {
			t.Fatalf("got %d blobs, want 0", len(blobs))
		}
	})

	t.Run("non-pointer returns nil", func(t *testing.T) {
		type row struct {
			Content Blob
		}
		r := row{Content: Blob{Ref: "abc"}}
		blobs := blobFields(r)
		if blobs != nil {
			t.Fatalf("got %v, want nil", blobs)
		}
	})
}
