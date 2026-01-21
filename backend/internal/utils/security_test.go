package utils

import (
	"testing"
)

func TestGenerateToken(t *testing.T) {
	t.Run("Length", func(t *testing.T) {
		tests := []struct {
			name   string
			length int
		}{
			{"Short", 8},
			{"Medium", 16},
			{"Long", 32},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				token, err := GenerateToken(tt.length)
				if err != nil {
					t.Fatalf("GenerateToken(%d) returned error: %v", tt.length, err)
				}

				// Hex string length is 2 * byte length
				expectedLen := tt.length * 2
				if len(token) != expectedLen {
					t.Errorf("GenerateToken(%d) length = %d; want %d", tt.length, len(token), expectedLen)
				}
			})
		}
	})

	t.Run("Randomness", func(t *testing.T) {
		length := 16
		token1, err := GenerateToken(length)
		if err != nil {
			t.Fatalf("GenerateToken failed: %v", err)
		}

		token2, err := GenerateToken(length)
		if err != nil {
			t.Fatalf("GenerateToken failed: %v", err)
		}

		if token1 == token2 {
			t.Error("GenerateToken produced duplicate tokens")
		}
	})
}
