package identity

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage"
)

func TestEmailVerificationService(t *testing.T) {
	tempDir := t.TempDir()
	tablePath := filepath.Join(tempDir, "email_verifications.jsonl")

	service, err := NewEmailVerificationService(tablePath)
	if err != nil {
		t.Fatalf("NewEmailVerificationService failed: %v", err)
	}

	userID := jsonldb.NewID()
	email := "test@example.com"

	t.Run("Create", func(t *testing.T) {
		verification, err := service.Create(userID, email)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		if verification.UserID != userID {
			t.Errorf("UserID mismatch: got %v, want %v", verification.UserID, userID)
		}
		if verification.Email != email {
			t.Errorf("Email mismatch: got %v, want %v", verification.Email, email)
		}
		if verification.Token == "" {
			t.Error("Token should not be empty")
		}
		if len(verification.Token) != 64 {
			t.Errorf("Token should be 64 chars, got %d", len(verification.Token))
		}
	})

	t.Run("GetByToken", func(t *testing.T) {
		verification, err := service.Create(userID, email)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		found, err := service.GetByToken(verification.Token)
		if err != nil {
			t.Fatalf("GetByToken failed: %v", err)
		}

		if found.ID != verification.ID {
			t.Errorf("ID mismatch: got %v, want %v", found.ID, verification.ID)
		}
	})

	t.Run("GetByToken_NotFound", func(t *testing.T) {
		_, err := service.GetByToken("nonexistent")
		if err == nil {
			t.Error("Expected error for nonexistent token")
		}
	})

	t.Run("Delete", func(t *testing.T) {
		verification, err := service.Create(userID, email)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		err = service.Delete(verification.ID)
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		_, err = service.GetByToken(verification.Token)
		if err == nil {
			t.Error("Expected error after deletion")
		}
	})

	t.Run("DeleteByUserID", func(t *testing.T) {
		user2ID := jsonldb.NewID()

		v1, err := service.Create(user2ID, "test1@example.com")
		if err != nil {
			t.Fatalf("Create v1 failed: %v", err)
		}

		v2, err := service.Create(user2ID, "test2@example.com")
		if err != nil {
			t.Fatalf("Create v2 failed: %v", err)
		}

		// v1 should be deleted by Create of v2 (same user)
		_, err = service.GetByToken(v1.Token)
		if err == nil {
			t.Error("v1 should have been deleted when v2 was created")
		}

		// v2 should exist
		_, err = service.GetByToken(v2.Token)
		if err != nil {
			t.Errorf("v2 should exist: %v", err)
		}
	})

	t.Run("IsExpired", func(t *testing.T) {
		verification, err := service.Create(userID, email)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		// Fresh verification should not be expired
		if service.IsExpired(verification) {
			t.Error("Fresh verification should not be expired")
		}

		// Manually create an expired verification
		expired := &EmailVerification{
			ID:        verification.ID,
			UserID:    userID,
			Email:     email,
			Token:     verification.Token,
			ExpiresAt: storage.ToTime(time.Now().Add(-1 * time.Hour)),
			Created:   verification.Created,
		}

		if !service.IsExpired(expired) {
			t.Error("Expired verification should be reported as expired")
		}
	})

	t.Run("Validate", func(t *testing.T) {
		// Test valid verification
		v := &EmailVerification{
			ID:     jsonldb.NewID(),
			UserID: jsonldb.NewID(),
			Email:  "test@example.com",
			Token:  "abc123",
		}
		if err := v.Validate(); err != nil {
			t.Errorf("Valid verification should pass validation: %v", err)
		}

		// Test missing ID
		v.ID = jsonldb.ID(0)
		if err := v.Validate(); err == nil {
			t.Error("Missing ID should fail validation")
		}
		v.ID = jsonldb.NewID()

		// Test missing UserID
		v.UserID = jsonldb.ID(0)
		if err := v.Validate(); err == nil {
			t.Error("Missing UserID should fail validation")
		}
		v.UserID = jsonldb.NewID()

		// Test missing Email
		v.Email = ""
		if err := v.Validate(); err == nil {
			t.Error("Missing Email should fail validation")
		}
		v.Email = "test@example.com"

		// Test missing Token
		v.Token = ""
		if err := v.Validate(); err == nil {
			t.Error("Missing Token should fail validation")
		}
	})
}
