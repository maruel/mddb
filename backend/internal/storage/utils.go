package storage

import (
	"crypto/rand"
	"fmt"

	"github.com/maruel/mddb/backend/internal/jsonldb"
)

// GenerateID generates a new LUCI-style ID and returns its encoded form.
// IDs are time-sortable and lexicographically ordered.
func GenerateID() string {
	return jsonldb.NewID().Encode()
}

// GenerateToken generates a secure random token.
func GenerateToken(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", b), nil
}
