package jsonldb

import (
	"sort"
	"testing"
	"time"
)

func TestNewID(t *testing.T) {
	id := NewID()
	if id == 0 {
		t.Error("NewID returned zero")
	}
	if id.Version() != int(IDVersion) {
		t.Errorf("Version = %d, want %d", id.Version(), IDVersion)
	}
}

func TestIDEncodeDecode(t *testing.T) {
	id := NewID()
	encoded := id.String()

	if len(encoded) > idEncodedLen {
		t.Errorf("Encode length = %d, want <= %d", len(encoded), idEncodedLen)
	}

	decoded, err := DecodeID(encoded)
	if err != nil {
		t.Fatalf("DecodeID error: %v", err)
	}
	if decoded != id {
		t.Errorf("DecodeID = %d, want %d", decoded, id)
	}

	// Test zero ID
	if ID(0).String() != "-" {
		t.Errorf("Zero ID string = %q, want %q", ID(0).String(), "-")
	}
	zeroDecoded, err := DecodeID("-")
	if err != nil || zeroDecoded != 0 {
		t.Errorf("DecodeID(-) = %d, %v; want 0, nil", zeroDecoded, err)
	}

	// Test small ID (variable length)
	smallID := ID(1)
	smallEncoded := smallID.String()
	if len(smallEncoded) >= idEncodedLen {
		t.Errorf("Small ID length = %d, want < %d", len(smallEncoded), idEncodedLen)
	}
	smallDecoded, err := DecodeID(smallEncoded)
	if err != nil || smallDecoded != smallID {
		t.Errorf("DecodeID(%s) = %d, %v; want %d, nil", smallEncoded, smallDecoded, err, smallID)
	}
}

func TestIDMonotonic(t *testing.T) {
	ids := make([]ID, 1000)
	for i := range ids {
		ids[i] = NewID()
	}

	for i := 1; i < len(ids); i++ {
		if ids[i] <= ids[i-1] {
			t.Errorf("ID[%d] = %d <= ID[%d] = %d (not monotonic)", i, ids[i], i-1, ids[i-1])
		}
	}
}

func TestIDLexicographicOrder(t *testing.T) {
	ids := make([]ID, 100)
	for i := range ids {
		ids[i] = NewID()
	}

	encoded := make([]string, len(ids))
	for i, id := range ids {
		encoded[i] = id.String()
	}

	// Check that string sorting preserves ID order
	sortedEncoded := make([]string, len(encoded))
	copy(sortedEncoded, encoded)
	sort.Strings(sortedEncoded)

	for i := range encoded {
		if encoded[i] != sortedEncoded[i] {
			t.Errorf("Lexicographic order mismatch at %d: got %s, want %s", i, sortedEncoded[i], encoded[i])
		}
	}
}

func TestIDTime(t *testing.T) {
	before := time.Now()
	id := NewID()
	after := time.Now()

	idTime := id.Time()
	if idTime.Before(before.Truncate(time.Millisecond)) {
		t.Errorf("ID time %v is before creation time %v", idTime, before)
	}
	if idTime.After(after.Add(time.Millisecond)) {
		t.Errorf("ID time %v is after creation time %v", idTime, after)
	}
}

func TestDecodeIDErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"too long", "abcdefghijklm"},
		{"invalid char", "!!!"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeID(tt.input)
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestIDCompare(t *testing.T) {
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
