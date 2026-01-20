package jsonldb

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// ID structure (64 bits):
//   - Bit 63: sign (always 0, keeps int64 positive)
//   - Bits 62-15: 10µs intervals since epoch (48 bits)
//   - Bits 14-4: slice (11 bits = 2048 values per 10µs)
//   - Bits 3-0: version (4 bits)
//
// Time span: ~89 years from epoch (until ~2115).
// Max throughput: ~204.8M IDs/sec (2048 per 10µs × 100,000 intervals/sec).
const (
	// epoch is 2026-01-01 00:00:00 UTC in 10µs intervals.
	epoch int64 = 176722560000000

	// idVersion is the current ID schema version.
	// Bump this when the ID format changes in a breaking way.
	idVersion uint64 = 1

	// idEncodedLen is the fixed length of encoded IDs.
	// 64 bits / 6 bits per char = 10.67, rounded up to 11.
	idEncodedLen = 11

	// sliceMask is the bitmask for extracting the 11-bit slice value.
	sliceMask = 0x7FF
)

// sortableAlphabet is a base64 alphabet in ASCII order for lexicographic sorting.
// Characters: - (0x2D), 0-9 (0x30-39), A-Z (0x41-5A), _ (0x5F), a-z (0x61-7A)
const sortableAlphabet = "-0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ_abcdefghijklmnopqrstuvwxyz"

// decodeMap maps ASCII characters back to their 6-bit values.
var decodeMap [128]byte

func init() {
	for i := range decodeMap {
		decodeMap[i] = 0xFF // invalid
	}
	for i, c := range sortableAlphabet {
		decodeMap[c] = byte(i)
	}
}

// ID is a time-sortable 64-bit identifier inspired by LUCI IDs.
//
// IDs encode a 10µs timestamp, a monotonically increasing slice for collision
// avoidance, and a version number. They are lexicographically sortable when
// encoded as strings, making them suitable for use as database keys and filenames.
// The zero value (0) represents an invalid/unset ID.
type ID uint64

var (
	idMu        sync.Mutex
	idLastT10us int64
	idSlice     uint16
)

// NewID generates a new time-based ID.
//
// IDs are guaranteed to be unique and monotonically increasing within a process.
// Multiple calls in the same 10µs interval use an incrementing slice counter.
// If the slice overflows (>2047 IDs in one 10µs interval), it spins until the
// next interval to maintain uniqueness.
func NewID() ID {
	idMu.Lock()
	for {
		t10us := max(0, time.Now().UnixMicro()/10-epoch)

		if t10us != idLastT10us {
			// New interval: reset slice to 0
			idLastT10us = t10us
			idSlice = 0
			id := newIDFromParts(uint64(t10us), uint64(idSlice), idVersion)
			idMu.Unlock()
			return id
		}

		// Same 10µs interval: increment slice
		idSlice++
		if idSlice <= sliceMask {
			id := newIDFromParts(uint64(t10us), uint64(idSlice), idVersion)
			idMu.Unlock()
			return id
		}

		// Overflow: release lock, wait, and retry
		idMu.Unlock()
		time.Sleep(time.Microsecond)
		idMu.Lock()
	}
}

func newIDFromParts(t10us, slice, version uint64) ID {
	// t10us: 48 bits, shifted to bits 62-15
	// slice: 11 bits, shifted to bits 14-4
	// version: 4 bits, in bits 3-0
	return ID((t10us << 15) | ((slice & sliceMask) << 4) | (version & 0xF))
}

// String returns a big-endian base64 encoding using a sortable alphabet.
//
// Big-endian ensures lexicographic string order matches numeric order,
// making IDs sortable as strings in databases and file systems.
// Leading zeros are stripped for compactness. Zero IDs return "-".
func (id ID) String() string {
	if id == 0 {
		return "-"
	}
	// Encode 64 bits into up to 11 characters (6 bits each)
	var buf [idEncodedLen]byte
	v := uint64(id)
	// Process from right to left, 6 bits at a time
	for i := idEncodedLen - 1; i >= 0; i-- {
		buf[i] = sortableAlphabet[v&0x3F]
		v >>= 6
	}
	// Strip leading '-' (zeros)
	for i := range idEncodedLen {
		if buf[i] != '-' {
			return string(buf[i:])
		}
	}
	// Unreachable: any non-zero ID has at least one non-'-' character.
	// Kept as safety fallback.
	return "-"
}

// MarshalJSON implements json.Marshaler.
func (id ID) MarshalJSON() ([]byte, error) {
	return json.Marshal(id.String())
}

// UnmarshalJSON implements json.Unmarshaler.
func (id *ID) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	parsed, err := DecodeID(s)
	if err != nil {
		return err
	}
	*id = parsed
	return nil
}

// IsZero returns true if the ID is the zero value.
func (id ID) IsZero() bool {
	return id == 0
}

// DecodeID parses an encoded string back to an ID.
//
// Empty string or "-" decode to zero ID. Returns an error for invalid input.
func DecodeID(s string) (ID, error) {
	if s == "-" || s == "" {
		return 0, nil
	}
	if len(s) > idEncodedLen {
		return 0, fmt.Errorf("invalid ID length: got %d, max %d", len(s), idEncodedLen)
	}
	// Left-pad with '-' (zero char) to full length
	for len(s) < idEncodedLen {
		s = "-" + s
	}
	var v uint64
	for i := range idEncodedLen {
		c := s[i]
		if c >= 128 {
			return 0, fmt.Errorf("invalid ID character at position %d: %c", i, c)
		}
		val := decodeMap[c]
		if val == 0xFF {
			return 0, fmt.Errorf("invalid ID character at position %d: %c", i, c)
		}
		v = (v << 6) | uint64(val)
	}
	return ID(v), nil
}

// Time extracts the timestamp from an ID.
func (id ID) Time() time.Time {
	t10us := int64(id>>15) + epoch
	return time.UnixMicro(t10us * 10)
}

// Version extracts the version bits from an ID.
func (id ID) Version() int {
	return int(id & 0xF)
}

// Slice extracts the slice counter from an ID.
func (id ID) Slice() uint16 {
	return uint16((id >> 4) & sliceMask)
}

// Compare returns -1 if id < other, 0 if equal, 1 if id > other.
func (id ID) Compare(other ID) int {
	if id < other {
		return -1
	}
	if id > other {
		return 1
	}
	return 0
}
