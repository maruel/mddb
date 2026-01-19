package jsonldb

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// ID structure (64 bits):
// - Bit 63: sign (always 0, keeps int64 positive)
// - Bits 62-20: milliseconds since Epoch (43 bits = ~278 years)
// - Bits 19-4: random (16 bits = 65536 values per ms)
// - Bits 3-0: version (4 bits)

const (
	// Epoch is 2024-01-01 00:00:00 UTC in milliseconds.
	Epoch int64 = 1704067200000

	// IDVersion is the current ID schema version.
	// Bump this when the ID format changes in a breaking way.
	IDVersion uint64 = 1

	// idEncodedLen is the fixed length of encoded IDs.
	// 64 bits / 6 bits per char = 10.67, rounded up to 11.
	idEncodedLen = 11
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

// ID is a LUCI-style 64-bit identifier that is time-sortable.
type ID uint64

var (
	idMu      sync.Mutex
	idLastMs  int64
	idCounter uint16
)

// NewID generates a new time-based ID with collision avoidance.
// IDs are monotonically increasing within a process.
func NewID() ID {
	idMu.Lock()
	defer idMu.Unlock()

	ms := time.Now().UnixMilli() - Epoch
	if ms < 0 {
		ms = 0
	}

	var randBits uint16
	if ms == idLastMs {
		// Same millisecond: increment counter
		idCounter++
		randBits = idCounter
	} else {
		// New millisecond: reset with random value
		idLastMs = ms
		var b [2]byte
		_, _ = rand.Read(b[:])
		idCounter = binary.BigEndian.Uint16(b[:])
		randBits = idCounter
	}

	return newIDFromParts(uint64(ms), uint64(randBits), IDVersion)
}

// NewIDAt generates an ID at a specific time. Useful for testing and migration.
func NewIDAt(t time.Time) ID {
	ms := t.UnixMilli() - Epoch
	if ms < 0 {
		ms = 0
	}
	var b [2]byte
	_, _ = rand.Read(b[:])
	randBits := binary.BigEndian.Uint16(b[:])
	return newIDFromParts(uint64(ms), uint64(randBits), IDVersion)
}

func newIDFromParts(ms, randBits, version uint64) ID {
	// ms: 43 bits, shifted to bits 62-20
	// randBits: 16 bits, shifted to bits 19-4
	// version: 4 bits, in bits 3-0
	return ID((ms << 20) | (randBits << 4) | (version & 0xF))
}

// String returns the fixed-width 11-character encoding using a sortable alphabet.
// The encoding is lexicographically sortable: if id1 < id2, then id1.String() < id2.String().
func (id ID) String() string {
	// Encode 64 bits into 11 characters (6 bits each, last char uses 4 bits)
	var buf [idEncodedLen]byte
	v := uint64(id)
	// Process from right to left, 6 bits at a time
	for i := idEncodedLen - 1; i >= 0; i-- {
		buf[i] = sortableAlphabet[v&0x3F]
		v >>= 6
	}
	return string(buf[:])
}

// MarshalJSON implements json.Marshaler.
// Zero IDs are marshaled as empty strings.
func (id ID) MarshalJSON() ([]byte, error) {
	if id == 0 {
		return json.Marshal("")
	}
	return json.Marshal(id.String())
}

// UnmarshalJSON implements json.Unmarshaler.
// Empty strings are unmarshaled as zero IDs.
func (id *ID) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	if s == "" {
		*id = 0
		return nil
	}
	parsed, err := DecodeID(s)
	if err != nil {
		return err
	}
	*id = parsed
	return nil
}

// IsZero returns true if the ID is the zero value.
// This is useful for omitempty JSON tags.
func (id ID) IsZero() bool {
	return id == 0
}

// DecodeID parses an 11-character encoded string back to an ID.
func DecodeID(s string) (ID, error) {
	if len(s) != idEncodedLen {
		return 0, fmt.Errorf("invalid ID length: got %d, want %d", len(s), idEncodedLen)
	}
	var v uint64
	for i := 0; i < idEncodedLen; i++ {
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
	ms := int64(id>>20) + Epoch
	return time.UnixMilli(ms)
}

// Version extracts the version bits from an ID.
func (id ID) Version() int {
	return int(id & 0xF)
}

// RandomBits extracts the random/counter bits from an ID.
func (id ID) RandomBits() uint16 {
	return uint16((id >> 4) & 0xFFFF)
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
