package jsonldb

import (
	"encoding/json"
	"sort"
	"testing"
	"time"
)

// TestID tests all ID type methods using table-driven tests.
func TestID(t *testing.T) {
	t.Run("NewID", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			tests := []struct {
				name string
			}{
				{"generates non-zero ID"},
				{"generates correct version"},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					id := NewID()
					if id == 0 {
						t.Error("NewID returned zero")
					}
					if id.Version() != int(idVersion) {
						t.Errorf("Version = %d, want %d", id.Version(), idVersion)
					}
				})
			}
		})

		t.Run("monotonic ordering", func(t *testing.T) {
			ids := make([]ID, 1000)
			for i := range ids {
				ids[i] = NewID()
			}

			for i := 1; i < len(ids); i++ {
				if ids[i] <= ids[i-1] {
					t.Errorf("ID[%d] = %d <= ID[%d] = %d (not monotonic)", i, ids[i], i-1, ids[i-1])
				}
			}
		})

		t.Run("slice increment within same interval", func(t *testing.T) {
			// Generate multiple IDs quickly to trigger same-interval logic
			id1 := NewID()
			id2 := NewID()
			id3 := NewID()

			// All should be unique and ordered
			if id1 >= id2 || id2 >= id3 {
				t.Error("IDs not properly ordered within same 10µs interval")
			}
		})
	})

	t.Run("NewIDSlice", func(t *testing.T) {
		t.Run("instance 0 of 3", func(t *testing.T) {
			// Reset state to ensure we start fresh for this test
			idMu.Lock()
			idLastT10us = 0
			idMu.Unlock()

			id1 := NewIDSlice(0, 3)
			if id1.Slice()%3 != 0 {
				t.Errorf("ID1 slice %d not matching instance 0 (mod 3)", id1.Slice())
			}

			// Force same interval logic if possible, or just check modulo
			// It's hard to guarantee same interval, but if we call fast enough:
			id2 := NewIDSlice(0, 3)
			if id2.Slice()%3 != 0 {
				t.Errorf("ID2 slice %d not matching instance 0 (mod 3)", id2.Slice())
			}

			// Verify increment if in same interval
			if id1.Time().Equal(id2.Time()) {
				if id2.Slice() != id1.Slice()+3 {
					t.Errorf("Slice did not increment by stride 3: %d -> %d", id1.Slice(), id2.Slice())
				}
			}
		})

		t.Run("instance 1 of 3", func(t *testing.T) {
			// Reset state
			idMu.Lock()
			idLastT10us = 0
			idMu.Unlock()

			id1 := NewIDSlice(1, 3)
			if id1.Slice()%3 != 1 {
				t.Errorf("ID1 slice %d not matching instance 1 (mod 3)", id1.Slice())
			}

			id2 := NewIDSlice(1, 3)
			if id2.Slice()%3 != 1 {
				t.Errorf("ID2 slice %d not matching instance 1 (mod 3)", id2.Slice())
			}
		})

		t.Run("instance 2 of 3", func(t *testing.T) {
			// Reset state
			idMu.Lock()
			idLastT10us = 0
			idMu.Unlock()

			id1 := NewIDSlice(2, 3)
			if id1.Slice()%3 != 2 {
				t.Errorf("ID1 slice %d not matching instance 2 (mod 3)", id1.Slice())
			}
		})

		t.Run("invalid inputs", func(t *testing.T) {
			id := NewIDSlice(0, 0) // Should default to 1
			// It behaves like NewID()
			if id == 0 {
				t.Error("NewIDSlice(0, 0) returned zero ID")
			}
		})
	})

	t.Run("String", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			tests := []struct {
				name      string
				id        ID
				wantLen   int    // -1 means check <= idEncodedLen
				wantExact string // empty means check is non-empty
			}{
				{"zero ID returns dash", 0, 1, "-"},
				{"small ID has compact encoding", 1, -1, ""},
				{"max ID has full length", ID(^uint64(0) >> 1), idEncodedLen, ""}, // Max positive value
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					got := tt.id.String()
					if tt.wantExact != "" && got != tt.wantExact {
						t.Errorf("String() = %q, want %q", got, tt.wantExact)
					}
					if tt.wantLen > 0 && len(got) != tt.wantLen {
						t.Errorf("String() length = %d, want %d", len(got), tt.wantLen)
					}
					if tt.wantLen == -1 && len(got) > idEncodedLen {
						t.Errorf("String() length = %d, want <= %d", len(got), idEncodedLen)
					}
				})
			}
		})

		t.Run("lexicographic order", func(t *testing.T) {
			ids := make([]ID, 100)
			for i := range ids {
				ids[i] = NewID()
			}

			encoded := make([]string, len(ids))
			for i, id := range ids {
				encoded[i] = id.String()
			}

			sortedEncoded := make([]string, len(encoded))
			copy(sortedEncoded, encoded)
			sort.Strings(sortedEncoded)

			for i := range encoded {
				if encoded[i] != sortedEncoded[i] {
					t.Errorf("Lexicographic order mismatch at %d: got %s, want %s", i, sortedEncoded[i], encoded[i])
				}
			}
		})
	})

	t.Run("MarshalJSON", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			tests := []struct {
				name string
				id   ID
				want string
			}{
				{"zero ID", 0, `"-"`},
				{"small ID", 1, `"0"`},        // ID 1 encodes to "0" in sortable alphabet
				{"generated ID", NewID(), ""}, // Will verify round-trip
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					got, err := tt.id.MarshalJSON()
					if err != nil {
						t.Fatalf("MarshalJSON error: %v", err)
					}
					if tt.want != "" && string(got) != tt.want {
						t.Errorf("MarshalJSON() = %s, want %s", got, tt.want)
					}
					// Verify it's valid JSON string
					var s string
					if err := json.Unmarshal(got, &s); err != nil {
						t.Errorf("MarshalJSON produced invalid JSON: %v", err)
					}
				})
			}
		})

		t.Run("round trip", func(t *testing.T) {
			original := NewID()
			data, err := json.Marshal(original)
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}
			var decoded ID
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}
			if decoded != original {
				t.Errorf("Round trip failed: got %d, want %d", decoded, original)
			}
		})
	})

	t.Run("UnmarshalJSON", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			tests := []struct {
				name  string
				input string
				want  ID
			}{
				{"zero ID dash", `"-"`, 0},
				{"zero ID empty", `""`, 0},
				{"small ID", `"0"`, 1}, // "0" decodes to ID 1 in sortable alphabet
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					var got ID
					if err := json.Unmarshal([]byte(tt.input), &got); err != nil {
						t.Fatalf("UnmarshalJSON error: %v", err)
					}
					if got != tt.want {
						t.Errorf("UnmarshalJSON() = %d, want %d", got, tt.want)
					}
				})
			}
		})

		t.Run("errors", func(t *testing.T) {
			tests := []struct {
				name  string
				input string
			}{
				{"JSON number instead of string", `123`},     // Valid JSON but wrong type
				{"JSON object instead of string", `{"a":1}`}, // Valid JSON but wrong type
				{"invalid ID too long", `"abcdefghijklm"`},
				{"invalid ID character", `"!!!"`},
				{"invalid high byte character", `"` + string([]byte{200}) + `"`},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					var id ID
					if err := json.Unmarshal([]byte(tt.input), &id); err == nil {
						t.Error("expected error, got nil")
					}
				})
			}
		})
	})

	t.Run("IsZero", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			tests := []struct {
				name string
				id   ID
				want bool
			}{
				{"zero ID", 0, true},
				{"non-zero ID", 1, false},
				{"generated ID", NewID(), false},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					if got := tt.id.IsZero(); got != tt.want {
						t.Errorf("IsZero() = %v, want %v", got, tt.want)
					}
				})
			}
		})
	})

	t.Run("DecodeID", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			tests := []struct {
				name  string
				input string
				want  ID
			}{
				{"empty string", "", 0},
				{"dash", "-", 0},
				{"small ID encoded", "0", 1}, // "0" decodes to ID 1 in sortable alphabet
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					got, err := DecodeID(tt.input)
					if err != nil {
						t.Fatalf("DecodeID error: %v", err)
					}
					if got != tt.want {
						t.Errorf("DecodeID() = %d, want %d", got, tt.want)
					}
				})
			}
		})

		t.Run("round trip", func(t *testing.T) {
			// Test round-trip for various IDs
			ids := []ID{0, 1, 100, NewID(), ID(1 << 30), ID(1 << 60)}
			for _, original := range ids {
				encoded := original.String()
				decoded, err := DecodeID(encoded)
				if err != nil {
					t.Fatalf("DecodeID(%q) error: %v", encoded, err)
				}
				if decoded != original {
					t.Errorf("Round trip failed for %d: encoded=%q, decoded=%d", original, encoded, decoded)
				}
			}
		})

		t.Run("errors", func(t *testing.T) {
			tests := []struct {
				name  string
				input string
			}{
				{"too long", "abcdefghijklm"},
				{"invalid char low", "!!!"},
				{"invalid char high", string([]byte{200})},
				{"mixed valid invalid", "abc!def"},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					_, err := DecodeID(tt.input)
					if err == nil {
						t.Error("expected error, got nil")
					}
				})
			}
		})
	})

	t.Run("Time", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			before := time.Now()
			id := NewID()
			after := time.Now()

			idTime := id.Time()
			// Truncate to 10µs resolution
			if idTime.Before(before.Truncate(10 * time.Microsecond)) {
				t.Errorf("ID time %v is before creation time %v", idTime, before)
			}
			if idTime.After(after.Add(10 * time.Microsecond)) {
				t.Errorf("ID time %v is after creation time %v", idTime, after)
			}
		})

		t.Run("zero ID time", func(t *testing.T) {
			id := ID(0)
			idTime := id.Time()
			// Zero ID should return epoch time (2026-01-01 00:00:00 UTC)
			expectedEpoch := time.UnixMicro(epoch * 10)
			if !idTime.Equal(expectedEpoch) {
				t.Errorf("Zero ID time = %v, want epoch %v", idTime, expectedEpoch)
			}
		})
	})

	t.Run("Version", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			tests := []struct {
				name string
				id   ID
				want int
			}{
				{"zero ID", 0, 0},
				{"generated ID", NewID(), int(idVersion)},
				{"constructed ID version 1", newIDFromParts(0, 0, 1), 1},
				{"constructed ID version 15", newIDFromParts(0, 0, 15), 15},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					if got := tt.id.Version(); got != tt.want {
						t.Errorf("Version() = %d, want %d", got, tt.want)
					}
				})
			}
		})
	})

	t.Run("Slice", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			tests := []struct {
				name string
				id   ID
				want uint16
			}{
				{"zero ID", 0, 0},
				{"constructed ID slice 0", newIDFromParts(0, 0, 0), 0},
				{"constructed ID slice 1234", newIDFromParts(0, 1234, 0), 1234},
				{"constructed ID slice max", newIDFromParts(0, sliceMask, 0), sliceMask},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					if got := tt.id.Slice(); got != tt.want {
						t.Errorf("Slice() = %d, want %d", got, tt.want)
					}
				})
			}
		})

		t.Run("slice increments within interval", func(t *testing.T) {
			// Generate several IDs quickly and verify slice increments
			id1 := NewID()
			id2 := NewID()
			// If in same interval, slice should increment
			if id1.Time().Equal(id2.Time()) && id2.Slice() != id1.Slice()+1 {
				t.Errorf("Slice not incrementing: id1.Slice()=%d, id2.Slice()=%d", id1.Slice(), id2.Slice())
			}
		})
	})

	t.Run("Compare", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			id1 := ID(100)
			id2 := ID(200)
			id3 := ID(100)

			tests := []struct {
				name string
				a, b ID
				want int
			}{
				{"equal", id1, id3, 0},
				{"less than", id1, id2, -1},
				{"greater than", id2, id1, 1},
				{"zero compare", ID(0), ID(0), 0},
				{"zero less than", ID(0), ID(1), -1},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					if got := tt.a.Compare(tt.b); got != tt.want {
						t.Errorf("Compare() = %d, want %d", got, tt.want)
					}
				})
			}
		})

		t.Run("ordering with generated IDs", func(t *testing.T) {
			id1 := NewID()
			id2 := NewID()

			if id1.Compare(id1) != 0 { //nolint:gocritic // Intentional self-comparison test
				t.Error("Compare(self) != 0")
			}
			if id1.Compare(id2) >= 0 {
				t.Error("Earlier ID should be less than later ID")
			}
			if id2.Compare(id1) <= 0 {
				t.Error("Later ID should be greater than earlier ID")
			}
		})
	})
}

func BenchmarkNewID(b *testing.B) {
	for range b.N {
		NewID()
	}
}

func BenchmarkIDEncode(b *testing.B) {
	id := NewID()
	b.ResetTimer()
	for range b.N {
		_ = id.String()
	}
}

func BenchmarkDecodeID(b *testing.B) {
	id := NewID()
	encoded := id.String()
	b.ResetTimer()
	for range b.N {
		_, _ = DecodeID(encoded)
	}
}
