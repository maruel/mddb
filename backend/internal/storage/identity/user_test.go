package identity

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/maruel/mddb/backend/internal/jsonldb"
)

func TestUserStorage(t *testing.T) {
	t.Run("Validate", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			valid := &userStorage{
				User:         User{ID: jsonldb.ID(1), Email: "test@example.com"},
				PasswordHash: "hash",
			}
			if err := valid.Validate(); err != nil {
				t.Errorf("Expected valid userStorage, got error: %v", err)
			}
		})

		t.Run("zero ID", func(t *testing.T) {
			zeroID := &userStorage{
				User:         User{ID: jsonldb.ID(0), Email: "test@example.com"},
				PasswordHash: "hash",
			}
			if err := zeroID.Validate(); err == nil {
				t.Error("Expected error for zero ID")
			}
		})

		t.Run("empty email", func(t *testing.T) {
			emptyEmail := &userStorage{
				User:         User{ID: jsonldb.ID(1), Email: ""},
				PasswordHash: "hash",
			}
			if err := emptyEmail.Validate(); err == nil {
				t.Error("Expected error for empty email")
			}
		})
	})

	t.Run("Clone", func(t *testing.T) {
		t.Run("with OAuthIdentities", func(t *testing.T) {
			original := &userStorage{
				User: User{
					ID:    jsonldb.ID(1),
					Email: "test@example.com",
					OAuthIdentities: []OAuthIdentity{
						{Provider: "google", ProviderID: "123"},
					},
				},
				PasswordHash: "hash",
			}

			clone := original.Clone()

			clone.OAuthIdentities[0].Provider = "modified"
			if original.OAuthIdentities[0].Provider == "modified" {
				t.Error("Clone should not share OAuthIdentities slice")
			}
		})

		t.Run("nil OAuthIdentities", func(t *testing.T) {
			noOAuth := &userStorage{
				User:         User{ID: jsonldb.ID(1), Email: "test@example.com"},
				PasswordHash: "hash",
			}
			cloneNoOAuth := noOAuth.Clone()
			if cloneNoOAuth.OAuthIdentities != nil {
				t.Error("Clone of nil OAuthIdentities should be nil")
			}
		})
	})
}

func TestUserService(t *testing.T) {
	service, err := NewUserService(filepath.Join(t.TempDir(), "users.jsonl"))
	if err != nil {
		t.Fatal(err)
	}

	var user, user2 *User

	t.Run("Create", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			var createErr error
			user, createErr = service.Create("test@example.com", "password123", "Test User")
			if createErr != nil {
				t.Fatalf("Failed to create user: %v", createErr)
			}
			if user.Email != "test@example.com" {
				t.Errorf("Expected email test@example.com, got %s", user.Email)
			}
			if user.GetID().IsZero() {
				t.Error("Expected non-zero user ID")
			}
		})

		t.Run("empty email", func(t *testing.T) {
			_, createErr := service.Create("", "password123", "Test")
			if createErr == nil {
				t.Error("Expected error for empty email")
			}
		})

		t.Run("empty password", func(t *testing.T) {
			_, createErr := service.Create("test2@example.com", "", "Test")
			if createErr == nil {
				t.Error("Expected error for empty password")
			}
		})

		t.Run("duplicate", func(t *testing.T) {
			_, createErr := service.Create("test@example.com", "password456", "Another User")
			if createErr == nil {
				t.Error("Expected error when creating duplicate user")
			}
		})
	})

	t.Run("Get", func(t *testing.T) {
		t.Run("existing", func(t *testing.T) {
			retrieved, getErr := service.Get(user.ID)
			if getErr != nil {
				t.Fatalf("Failed to get user: %v", getErr)
			}
			if retrieved.ID != user.ID {
				t.Errorf("Expected user ID %s, got %s", user.ID, retrieved.ID)
			}
		})

		t.Run("zero ID", func(t *testing.T) {
			_, getErr := service.Get(jsonldb.ID(0))
			if getErr == nil {
				t.Error("Expected error for zero ID")
			}
		})

		t.Run("non-existent", func(t *testing.T) {
			_, getErr := service.Get(jsonldb.ID(99999))
			if getErr == nil {
				t.Error("Expected error for non-existent user")
			}
		})
	})

	t.Run("GetByEmail", func(t *testing.T) {
		t.Run("existing", func(t *testing.T) {
			byEmail, getErr := service.GetByEmail("test@example.com")
			if getErr != nil {
				t.Fatalf("Failed to get user by email: %v", getErr)
			}
			if byEmail.ID != user.ID {
				t.Errorf("Expected user ID %s, got %s", user.ID, byEmail.ID)
			}
		})

		t.Run("non-existent", func(t *testing.T) {
			_, getErr := service.GetByEmail("nonexistent@example.com")
			if getErr == nil {
				t.Error("Expected error for non-existent email")
			}
		})
	})

	t.Run("Authenticate", func(t *testing.T) {
		t.Run("valid credentials", func(t *testing.T) {
			authenticatedUser, authErr := service.Authenticate("test@example.com", "password123")
			if authErr != nil {
				t.Fatalf("Authentication failed: %v", authErr)
			}
			if authenticatedUser.ID != user.ID {
				t.Errorf("Expected user ID %s, got %s", user.ID, authenticatedUser.ID)
			}
		})

		t.Run("wrong password", func(t *testing.T) {
			_, authErr := service.Authenticate("test@example.com", "wrongpassword")
			if authErr == nil {
				t.Error("Expected authentication to fail with wrong password")
			}
		})

		t.Run("non-existent user", func(t *testing.T) {
			_, authErr := service.Authenticate("nonexistent@example.com", "password")
			if authErr == nil {
				t.Error("Expected authentication to fail for non-existent user")
			}
		})
	})

	t.Run("Modify", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			modified, modErr := service.Modify(user.ID, func(u *User) error {
				u.Name = "Modified Name"
				return nil
			})
			if modErr != nil {
				t.Fatalf("Modify failed: %v", modErr)
			}
			if modified.Name != "Modified Name" {
				t.Errorf("Expected name 'Modified Name', got %s", modified.Name)
			}
		})

		t.Run("zero ID", func(t *testing.T) {
			_, modErr := service.Modify(jsonldb.ID(0), func(u *User) error {
				return nil
			})
			if modErr == nil {
				t.Error("Expected error for Modify with zero ID")
			}
		})

		t.Run("non-existent", func(t *testing.T) {
			_, modErr := service.Modify(jsonldb.ID(99999), func(u *User) error {
				return nil
			})
			if modErr == nil {
				t.Error("Expected error for Modify with non-existent ID")
			}
		})
	})

	t.Run("Iter", func(t *testing.T) {
		// Create another user first
		user2, _ = service.Create("user2@example.com", "password", "User 2")

		t.Run("all users", func(t *testing.T) {
			count := 0
			for u := range service.Iter(0) {
				count++
				if u.ID != user.ID && u.ID != user2.ID {
					t.Errorf("Unexpected user ID: %s", u.ID)
				}
			}
			if count != 2 {
				t.Errorf("Expected 2 users, got %d", count)
			}
		})

		t.Run("with startID", func(t *testing.T) {
			count := 0
			for range service.Iter(user.ID) {
				count++
			}
			if count != 1 {
				t.Errorf("Expected 1 user after startID, got %d", count)
			}
		})

		t.Run("early termination", func(t *testing.T) {
			count := 0
			for range service.Iter(0) {
				count++
				if count >= 1 {
					break
				}
			}
			if count != 1 {
				t.Errorf("Expected 1 user with early break, got %d", count)
			}
		})
	})

	t.Run("OAuth", func(t *testing.T) {
		oauthService, oauthErr := NewUserService(filepath.Join(t.TempDir(), "users.jsonl"))
		if oauthErr != nil {
			t.Fatal(oauthErr)
		}

		oauthUser, oauthErr := oauthService.Create("oauth@example.com", "password123", "OAuth User")
		if oauthErr != nil {
			t.Fatal(oauthErr)
		}

		_, oauthErr = oauthService.Modify(oauthUser.ID, func(u *User) error {
			u.OAuthIdentities = append(u.OAuthIdentities, OAuthIdentity{
				Provider:   "google",
				ProviderID: "google-123",
				Email:      "oauth@gmail.com",
			})
			return nil
		})
		if oauthErr != nil {
			t.Fatalf("Failed to add OAuth identity: %v", oauthErr)
		}

		t.Run("GetByOAuth existing", func(t *testing.T) {
			found, getErr := oauthService.GetByOAuth("google", "google-123")
			if getErr != nil {
				t.Fatalf("GetByOAuth failed: %v", getErr)
			}
			if found.ID != oauthUser.ID {
				t.Errorf("Expected user ID %s, got %s", oauthUser.ID, found.ID)
			}
		})

		t.Run("GetByOAuth non-existent provider", func(t *testing.T) {
			_, getErr := oauthService.GetByOAuth("github", "github-123")
			if getErr == nil {
				t.Error("Expected error for non-existent OAuth identity")
			}
		})

		t.Run("GetByOAuth wrong provider ID", func(t *testing.T) {
			_, getErr := oauthService.GetByOAuth("google", "wrong-id")
			if getErr == nil {
				t.Error("Expected error for wrong provider ID")
			}
		})

		t.Run("update OAuth identity", func(t *testing.T) {
			_, updateErr := oauthService.Modify(oauthUser.ID, func(u *User) error {
				u.OAuthIdentities = []OAuthIdentity{
					{Provider: "microsoft", ProviderID: "ms-456", Email: "oauth@outlook.com"},
				}
				return nil
			})
			if updateErr != nil {
				t.Fatalf("Failed to update OAuth identity: %v", updateErr)
			}

			// Old identity should not be found
			_, getErr := oauthService.GetByOAuth("google", "google-123")
			if getErr == nil {
				t.Error("Expected old OAuth identity to be removed")
			}

			// New identity should be found
			found, getErr := oauthService.GetByOAuth("microsoft", "ms-456")
			if getErr != nil {
				t.Fatalf("GetByOAuth failed for new identity: %v", getErr)
			}
			if found.ID != oauthUser.ID {
				t.Errorf("Expected user ID %s, got %s", oauthUser.ID, found.ID)
			}
		})
	})

	t.Run("Persistence", func(t *testing.T) {
		tablePath := filepath.Join(t.TempDir(), "users.jsonl")

		service1, svcErr := NewUserService(tablePath)
		if svcErr != nil {
			t.Fatal(svcErr)
		}

		persistUser, createErr := service1.Create("persist@example.com", "password123", "Persist User")
		if createErr != nil {
			t.Fatal(createErr)
		}

		_, modErr := service1.Modify(persistUser.ID, func(u *User) error {
			u.OAuthIdentities = []OAuthIdentity{
				{Provider: "google", ProviderID: "google-persist-123"},
				{Provider: "github", ProviderID: "github-persist-456"},
			}
			return nil
		})
		if modErr != nil {
			t.Fatal(modErr)
		}

		// Create new service instance (simulating restart)
		service2, svcErr := NewUserService(tablePath)
		if svcErr != nil {
			t.Fatal(svcErr)
		}

		// Verify OAuth index is populated from loaded data
		found, getErr := service2.GetByOAuth("google", "google-persist-123")
		if getErr != nil {
			t.Fatalf("GetByOAuth failed for persisted OAuth identity: %v", getErr)
		}
		if found.ID != persistUser.ID {
			t.Errorf("Expected user ID %v, got %v", persistUser.ID, found.ID)
		}

		found2, getErr := service2.GetByOAuth("github", "github-persist-456")
		if getErr != nil {
			t.Fatalf("GetByOAuth failed for second persisted OAuth identity: %v", getErr)
		}
		if found2.ID != persistUser.ID {
			t.Errorf("Expected user ID %v, got %v", persistUser.ID, found2.ID)
		}
	})

	t.Run("InvalidJSONL", func(t *testing.T) {
		t.Run("malformed JSON", func(t *testing.T) {
			tempDir := t.TempDir()
			jsonlPath := filepath.Join(tempDir, "invalid_users.jsonl")

			// Write invalid JSON to the file (malformed JSON)
			err := os.WriteFile(jsonlPath, []byte(`{"version":"1.0","columns":[]}
{"user":{"id":1,"email":"test@example.com"},"password_hash":"hash"}
{"user":{"id":2,"email":"test2@example.com"},"password_hash":"hash2"
`), 0o600)
			if err != nil {
				t.Fatal(err)
			}

			_, err = NewUserService(jsonlPath)
			if err == nil {
				t.Error("Expected error when loading invalid JSONL file")
			}
		})

		t.Run("malformed row with empty email", func(t *testing.T) {
			tempDir := t.TempDir()
			jsonlPath := filepath.Join(tempDir, "malformed_users.jsonl")

			// Write JSON with malformed row (missing required fields)
			err := os.WriteFile(jsonlPath, []byte(`{"version":"1.0","columns":[]}
{"user":{"id":1,"email":""},"password_hash":"hash"}
`), 0o600)
			if err != nil {
				t.Fatal(err)
			}

			_, err = NewUserService(jsonlPath)
			if err == nil {
				t.Error("Expected error when loading JSONL with invalid row (empty email)")
			}
		})

		t.Run("row with zero ID", func(t *testing.T) {
			tempDir := t.TempDir()
			jsonlPath := filepath.Join(tempDir, "zero_id_users.jsonl")

			// Write JSON with zero ID
			err := os.WriteFile(jsonlPath, []byte(`{"version":"1.0","columns":[]}
{"user":{"id":0,"email":"test@example.com"},"password_hash":"hash"}
`), 0o600)
			if err != nil {
				t.Fatal(err)
			}

			_, err = NewUserService(jsonlPath)
			if err == nil {
				t.Error("Expected error when loading JSONL with zero ID")
			}
		})
	})
}
