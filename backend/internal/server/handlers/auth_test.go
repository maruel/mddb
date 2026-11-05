// Tests for authentication handlers.

package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
	"golang.org/x/crypto/bcrypt"
)

func TestAuthHandler(t *testing.T) {
	t.Run("VerifyEmailRedirect", func(t *testing.T) {
		e := newOAuthTestEnv(t, "http://localhost:8080")
		var err error
		e.svc.EmailVerif, err = identity.NewEmailVerificationService(filepath.Join(e.dir, "db", "email_verifications.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		verification, err := e.svc.EmailVerif.Create(e.caller.ID, e.caller.Email)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(e.dir, ".git", "index.lock"), []byte("test owns lock"), 0o600); err != nil {
			t.Fatal(err)
		}
		h := NewAuthHandler(e.svc, e.h.cfg)
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/auth/verify-email?token="+verification.Token, http.NoBody)
		w := httptest.NewRecorder()
		h.VerifyEmailRedirect(w, r)
		if w.Code != http.StatusFound || w.Header().Get("Location") != "http://localhost:8080/settings?email_verified=true" {
			t.Fatalf("completed verification: %d %s", w.Code, w.Header().Get("Location"))
		}
		if u := e.get(t, e.caller); !u.EmailVerified {
			t.Fatal("email was not persisted as verified")
		}
		if _, err := e.svc.EmailVerif.GetByToken(verification.Token); err == nil {
			t.Fatal("verification token remains usable")
		}
	})
	t.Run("Register", func(t *testing.T) {
		ctx := t.Context()
		tempDir := t.TempDir()

		userService, err := identity.NewUserService(filepath.Join(tempDir, "users.jsonl"))
		if err != nil {
			t.Fatalf("NewUserService failed: %v", err)
		}
		userService.SetBcryptCostForTesting(bcrypt.MinCost)

		orgService, err := identity.NewOrganizationService(filepath.Join(tempDir, "organizations.jsonl"))
		if err != nil {
			t.Fatalf("NewOrganizationService failed: %v", err)
		}

		wsService, err := identity.NewWorkspaceService(filepath.Join(tempDir, "workspaces.jsonl"))
		if err != nil {
			t.Fatalf("NewWorkspaceService failed: %v", err)
		}

		orgMemService, err := identity.NewOrganizationMembershipService(filepath.Join(tempDir, "org_memberships.jsonl"), userService, orgService)
		if err != nil {
			t.Fatalf("NewOrganizationMembershipService failed: %v", err)
		}

		wsMemService, err := identity.NewWorkspaceMembershipService(filepath.Join(tempDir, "ws_memberships.jsonl"), wsService, orgService)
		if err != nil {
			t.Fatalf("NewWorkspaceMembershipService failed: %v", err)
		}

		sessionService, err := identity.NewSessionService(filepath.Join(tempDir, "sessions.jsonl"))
		if err != nil {
			t.Fatalf("NewSessionService failed: %v", err)
		}

		gitMgr := git.NewManager(tempDir, "test", "test@test.com")

		serverQuotas := storage.DefaultResourceQuotas()
		fileStore, err := content.NewFileStoreService(tempDir, gitMgr, wsService, orgService, &serverQuotas)
		if err != nil {
			t.Fatalf("NewFileStore failed: %v", err)
		}

		svc := &Services{
			FileStore:     fileStore,
			User:          userService,
			Organization:  orgService,
			Workspace:     wsService,
			OrgMembership: orgMemService,
			WSMembership:  wsMemService,
			Session:       sessionService,
			EmailVerif:    nil,
			Email:         nil,
		}

		cfg := &Config{
			ServerConfig: storage.ServerConfig{
				JWTSecret: []byte("test-secret-key-32-bytes-long!!!"),
			},
			BaseURL: "http://localhost:8080",
		}
		authHandler := NewAuthHandler(svc, cfg)

		// Register Joe - should not create organization (frontend handles that)
		req1 := &dto.RegisterRequest{
			Email:    "joe@example.com",
			Password: "password",
			Name:     "Joe",
		}
		resp1, err := authHandler.Register(ctx, req1)
		if err != nil {
			t.Fatalf("Failed to register Joe: %v", err)
		}

		if resp1.User.Name != "Joe" {
			t.Errorf("Expected name Joe, got %s", resp1.User.Name)
		}

		// New users should have no org/workspace memberships
		if len(resp1.User.Organizations) != 0 {
			t.Errorf("Expected Joe to have no org memberships after registration, got %d", len(resp1.User.Organizations))
		}
		if len(resp1.User.Workspaces) != 0 {
			t.Errorf("Expected Joe to have no workspace memberships after registration, got %d", len(resp1.User.Workspaces))
		}

		// Register Alice
		req2 := &dto.RegisterRequest{
			Email:    "alice@example.com",
			Password: "password",
			Name:     "Alice",
		}
		resp2, err := authHandler.Register(ctx, req2)
		if err != nil {
			t.Fatalf("Failed to register Alice: %v", err)
		}

		if resp2.User.Name != "Alice" {
			t.Errorf("Expected name Alice, got %s", resp2.User.Name)
		}

		// Alice should also have no memberships
		if len(resp2.User.Organizations) != 0 {
			t.Errorf("Expected Alice to have no org memberships after registration, got %d", len(resp2.User.Organizations))
		}

		// Verify token is returned
		if resp1.Token == "" {
			t.Error("Expected Joe to receive a token")
		}
		if resp2.Token == "" {
			t.Error("Expected Alice to receive a token")
		}
	})
}
