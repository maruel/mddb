// Package utils provides shared utility functions across the application.
package utils

import (
	"crypto/rand"
	"fmt"
)

// GenerateToken generates a secure random token.
func GenerateToken(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", b), nil
}
