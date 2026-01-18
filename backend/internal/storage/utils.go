package storage

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"time"
)

// EncodeID converts a uint64 to a base64 string without padding.
func EncodeID(n uint64) string {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, n)
	i := 0
	for i < 7 && buf[i] == 0 {
		i++
	}
	return base64.RawURLEncoding.EncodeToString(buf[i:])
}

// DecodeID converts a base64 string without padding back to uint64.
func DecodeID(s string) (uint64, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return 0, err
	}
	if len(b) > 8 {
		return 0, fmt.Errorf("ID too long")
	}
	var n uint64
	for _, x := range b {
		n = (n << 8) | uint64(x)
	}
	return n, nil
}

// generateShortID generates a compact base64 encoded ID based on current time.
func generateShortID() string {
	return EncodeID(uint64(time.Now().UnixNano()))
}

// generateID generates a UUID v4 string.
func generateID() (string, error) {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}

	// Set version to 4
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// GenerateToken generates a secure random token.
func GenerateToken(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", b), nil
}
