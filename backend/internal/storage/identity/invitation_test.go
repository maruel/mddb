// Tests recipient and bearer invitation claims, persistence failures, restart, and revocation.

package identity

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	identityv1 "github.com/maruel/mddb/backend/internal/storage/identity/data/v1"

	"github.com/maruel/ksid"
)

func TestOrganizationInvitation(t *testing.T) {
	t.Run("Validate", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			valid := &identityv1.OrganizationInvitation{
				ID:             ksid.ID(1),
				Email:          "test@example.com",
				OrganizationID: ksid.ID(100),
				Role:           identityv1.OrgRoleMember,
				Token:          "token123",
			}
			if err := valid.Validate(); err != nil {
				t.Errorf("Expected valid invitation, got error: %v", err)
			}
		})

		t.Run("zero ID", func(t *testing.T) {
			zeroID := &identityv1.OrganizationInvitation{
				ID:             ksid.ID(0),
				Email:          "test@example.com",
				OrganizationID: ksid.ID(100),
				Role:           identityv1.OrgRoleMember,
				Token:          "token123",
			}
			if err := zeroID.Validate(); err == nil {
				t.Error("Expected error for zero ID")
			}
		})

		t.Run("empty email", func(t *testing.T) {
			emptyEmail := &identityv1.OrganizationInvitation{
				ID:             ksid.ID(1),
				Email:          "",
				OrganizationID: ksid.ID(100),
				Role:           identityv1.OrgRoleMember,
				Token:          "token123",
			}
			if err := emptyEmail.Validate(); err != nil {
				t.Errorf("bearer invitation rejected: %v", err)
			}
		})

		t.Run("zero org ID", func(t *testing.T) {
			zeroOrg := &identityv1.OrganizationInvitation{
				ID:             ksid.ID(1),
				Email:          "test@example.com",
				OrganizationID: ksid.ID(0),
				Role:           identityv1.OrgRoleMember,
				Token:          "token123",
			}
			if err := zeroOrg.Validate(); err == nil {
				t.Error("Expected error for zero org ID")
			}
		})

		t.Run("empty token", func(t *testing.T) {
			emptyToken := &identityv1.OrganizationInvitation{
				ID:             ksid.ID(1),
				Email:          "test@example.com",
				OrganizationID: ksid.ID(100),
				Role:           identityv1.OrgRoleMember,
				Token:          "",
			}
			if err := emptyToken.Validate(); err == nil {
				t.Error("Expected error for empty token")
			}
		})
	})

	t.Run("Clone", func(t *testing.T) {
		original := &identityv1.OrganizationInvitation{
			ID:             ksid.ID(1),
			Email:          "test@example.com",
			OrganizationID: ksid.ID(100),
			Role:           identityv1.OrgRoleAdmin,
			Token:          "token123",
		}

		clone := original.Clone()

		if clone.ID != original.ID {
			t.Error("Clone ID should match")
		}
		if clone.Email != original.Email {
			t.Error("Clone Email should match")
		}

		clone.Email = "modified@example.com"
		if original.Email == "modified@example.com" {
			t.Error("Clone should be independent of original")
		}
	})

	t.Run("GetID", func(t *testing.T) {
		inv := &identityv1.OrganizationInvitation{ID: ksid.ID(42)}
		if inv.GetID() != ksid.ID(42) {
			t.Errorf("GetID() = %v, want %v", inv.GetID(), ksid.ID(42))
		}
	})
}

func TestOrganizationInvitationService(t *testing.T) {
	t.Run("Accept", func(t *testing.T) { testInvitationClaims(t, "org") })

	service, err := NewOrganizationInvitationService(filepath.Join(t.TempDir(), "org_invitations.jsonl"))
	if err != nil {
		t.Fatal(err)
	}

	orgID := ksid.ID(100)
	inviterID := ksid.ID(1)
	email := "invitee@example.com"
	role := identityv1.OrgRoleMember
	var inv, inv2 *identityv1.OrganizationInvitation

	t.Run("Create", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			var createErr error
			inv, createErr = service.Create(email, orgID, role, inviterID)
			if createErr != nil {
				t.Fatalf("Failed to create invitation: %v", createErr)
			}

			if inv.Email != email {
				t.Errorf("Expected email %s, got %s", email, inv.Email)
			}
			if inv.OrganizationID != orgID {
				t.Errorf("Expected orgID %v, got %v", orgID, inv.OrganizationID)
			}
			if inv.Role != role {
				t.Errorf("Expected role %s, got %s", role, inv.Role)
			}
			if inv.Token == "" {
				t.Error("Expected token to be generated")
			}
			if inv.ExpiresAt.IsZero() {
				t.Error("Expected ExpiresAt to be set")
			}
		})

		t.Run("empty email", func(t *testing.T) {
			inv, createErr := service.Create("", orgID, role, inviterID)
			if createErr != nil {
				t.Fatal(createErr)
			}
			if inv.Email != "" {
				t.Fatal("bearer invitation gained a recipient")
			}
			if err := service.Delete(inv.ID); err != nil {
				t.Fatal(err)
			}
		})

		t.Run("zero orgID", func(t *testing.T) {
			_, createErr := service.Create(email, ksid.ID(0), role, inviterID)
			if createErr == nil {
				t.Error("Expected error for zero orgID")
			}
		})

		t.Run("second invitation", func(t *testing.T) {
			var createErr error
			inv2, createErr = service.Create("second@example.com", orgID, identityv1.OrgRoleAdmin, inviterID)
			if createErr != nil {
				t.Fatalf("Failed to create second invitation: %v", createErr)
			}
		})
	})

	t.Run("GetByToken", func(t *testing.T) {
		t.Run("existing", func(t *testing.T) {
			found, getErr := service.GetByToken(inv.Token)
			if getErr != nil {
				t.Fatalf("Failed to find invitation by token: %v", getErr)
			}
			if found.ID != inv.ID {
				t.Errorf("Expected ID %v, got %v", inv.ID, found.ID)
			}
		})

		t.Run("non-existent", func(t *testing.T) {
			_, getErr := service.GetByToken("nonexistent-token")
			if getErr == nil {
				t.Error("Expected error for non-existent token")
			}
		})
	})

	t.Run("IterByOrg", func(t *testing.T) {
		t.Run("by org", func(t *testing.T) {
			count := 0
			for range service.IterByOrg(orgID) {
				count++
			}
			if count != 2 {
				t.Errorf("Expected 2 invitations, got %d", count)
			}
		})

		t.Run("different org", func(t *testing.T) {
			count := 0
			for range service.IterByOrg(ksid.ID(999)) {
				count++
			}
			if count != 0 {
				t.Errorf("Expected 0 invitations for different org, got %d", count)
			}
		})
	})

	t.Run("Delete", func(t *testing.T) {
		t.Run("zero ID", func(t *testing.T) {
			delErr := service.Delete(ksid.ID(0))
			if delErr == nil {
				t.Error("Expected error for Delete with zero ID")
			}
		})

		t.Run("non-existent", func(t *testing.T) {
			delErr := service.Delete(ksid.ID(99999))
			if delErr == nil {
				t.Error("Expected error for Delete with non-existent ID")
			}
		})

		t.Run("valid", func(t *testing.T) {
			delErr := service.Delete(inv.ID)
			if delErr != nil {
				t.Fatalf("Failed to delete invitation: %v", delErr)
			}

			_, getErr := service.GetByToken(inv.Token)
			if getErr == nil {
				t.Error("Expected invitation to be deleted")
			}

			// Second invitation should still exist
			found2, getErr := service.GetByToken(inv2.Token)
			if getErr != nil {
				t.Fatalf("Second invitation should still exist: %v", getErr)
			}
			if found2.ID != inv2.ID {
				t.Error("Wrong invitation returned")
			}
		})
	})
}

func TestWorkspaceInvitation(t *testing.T) {
	t.Run("Validate", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			valid := &identityv1.WorkspaceInvitation{
				ID:          ksid.ID(1),
				Email:       "test@example.com",
				WorkspaceID: ksid.ID(100),
				Role:        identityv1.WSRoleEditor,
				Token:       "token123",
			}
			if err := valid.Validate(); err != nil {
				t.Errorf("Expected valid invitation, got error: %v", err)
			}
		})

		t.Run("zero ID", func(t *testing.T) {
			zeroID := &identityv1.WorkspaceInvitation{
				ID:          ksid.ID(0),
				Email:       "test@example.com",
				WorkspaceID: ksid.ID(100),
				Role:        identityv1.WSRoleEditor,
				Token:       "token123",
			}
			if err := zeroID.Validate(); err == nil {
				t.Error("Expected error for zero ID")
			}
		})

		t.Run("empty email", func(t *testing.T) {
			emptyEmail := &identityv1.WorkspaceInvitation{
				ID:          ksid.ID(1),
				Email:       "",
				WorkspaceID: ksid.ID(100),
				Role:        identityv1.WSRoleEditor,
				Token:       "token123",
			}
			if err := emptyEmail.Validate(); err != nil {
				t.Fatalf("bearer invitation rejected: %v", err)
			}
		})

		t.Run("zero workspace ID", func(t *testing.T) {
			zeroWS := &identityv1.WorkspaceInvitation{
				ID:          ksid.ID(1),
				Email:       "test@example.com",
				WorkspaceID: ksid.ID(0),
				Role:        identityv1.WSRoleEditor,
				Token:       "token123",
			}
			if err := zeroWS.Validate(); err == nil {
				t.Error("Expected error for zero workspace ID")
			}
		})

		t.Run("empty token", func(t *testing.T) {
			emptyToken := &identityv1.WorkspaceInvitation{
				ID:          ksid.ID(1),
				Email:       "test@example.com",
				WorkspaceID: ksid.ID(100),
				Role:        identityv1.WSRoleEditor,
				Token:       "",
			}
			if err := emptyToken.Validate(); err == nil {
				t.Error("Expected error for empty token")
			}
		})
	})

	t.Run("Clone", func(t *testing.T) {
		original := &identityv1.WorkspaceInvitation{
			ID:          ksid.ID(1),
			Email:       "test@example.com",
			WorkspaceID: ksid.ID(100),
			Role:        identityv1.WSRoleAdmin,
			Token:       "token123",
		}

		clone := original.Clone()

		if clone.ID != original.ID {
			t.Error("Clone ID should match")
		}
		if clone.Email != original.Email {
			t.Error("Clone Email should match")
		}

		clone.Email = "modified@example.com"
		if original.Email == "modified@example.com" {
			t.Error("Clone should be independent of original")
		}
	})

	t.Run("GetID", func(t *testing.T) {
		inv := &identityv1.WorkspaceInvitation{ID: ksid.ID(42)}
		if inv.GetID() != ksid.ID(42) {
			t.Errorf("GetID() = %v, want %v", inv.GetID(), ksid.ID(42))
		}
	})
}

func TestWorkspaceInvitationService(t *testing.T) {
	t.Run("Accept", func(t *testing.T) { testInvitationClaims(t, "ws") })

	service, err := NewWorkspaceInvitationService(filepath.Join(t.TempDir(), "ws_invitations.jsonl"))
	if err != nil {
		t.Fatal(err)
	}

	wsID := ksid.ID(100)
	inviterID := ksid.ID(1)
	email := "invitee@example.com"
	role := identityv1.WSRoleEditor
	var inv, inv2 *identityv1.WorkspaceInvitation

	t.Run("Create", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			var createErr error
			inv, createErr = service.Create(email, wsID, role, inviterID)
			if createErr != nil {
				t.Fatalf("Failed to create invitation: %v", createErr)
			}

			if inv.Email != email {
				t.Errorf("Expected email %s, got %s", email, inv.Email)
			}
			if inv.WorkspaceID != wsID {
				t.Errorf("Expected wsID %v, got %v", wsID, inv.WorkspaceID)
			}
			if inv.Role != role {
				t.Errorf("Expected role %s, got %s", role, inv.Role)
			}
			if inv.Token == "" {
				t.Error("Expected token to be generated")
			}
			if inv.ExpiresAt.IsZero() {
				t.Error("Expected ExpiresAt to be set")
			}
		})

		t.Run("empty email", func(t *testing.T) {
			inv, createErr := service.Create("", wsID, role, inviterID)
			if createErr != nil {
				t.Fatal(createErr)
			}
			if inv.Email != "" {
				t.Fatal("bearer invitation gained a recipient")
			}
			if err := service.Delete(inv.ID); err != nil {
				t.Fatal(err)
			}
		})

		t.Run("zero wsID", func(t *testing.T) {
			_, createErr := service.Create(email, ksid.ID(0), role, inviterID)
			if createErr == nil {
				t.Error("Expected error for zero wsID")
			}
		})

		t.Run("second invitation", func(t *testing.T) {
			var createErr error
			inv2, createErr = service.Create("second@example.com", wsID, identityv1.WSRoleViewer, inviterID)
			if createErr != nil {
				t.Fatalf("Failed to create second invitation: %v", createErr)
			}
		})
	})

	t.Run("GetByToken", func(t *testing.T) {
		t.Run("existing", func(t *testing.T) {
			found, getErr := service.GetByToken(inv.Token)
			if getErr != nil {
				t.Fatalf("Failed to find invitation by token: %v", getErr)
			}
			if found.ID != inv.ID {
				t.Errorf("Expected ID %v, got %v", inv.ID, found.ID)
			}
		})

		t.Run("non-existent", func(t *testing.T) {
			_, getErr := service.GetByToken("nonexistent-token")
			if getErr == nil {
				t.Error("Expected error for non-existent token")
			}
		})
	})

	t.Run("IterByWorkspace", func(t *testing.T) {
		t.Run("by workspace", func(t *testing.T) {
			count := 0
			for range service.IterByWorkspace(wsID) {
				count++
			}
			if count != 2 {
				t.Errorf("Expected 2 invitations, got %d", count)
			}
		})

		t.Run("different workspace", func(t *testing.T) {
			count := 0
			for range service.IterByWorkspace(ksid.ID(999)) {
				count++
			}
			if count != 0 {
				t.Errorf("Expected 0 invitations for different workspace, got %d", count)
			}
		})
	})

	t.Run("Delete", func(t *testing.T) {
		t.Run("zero ID", func(t *testing.T) {
			delErr := service.Delete(ksid.ID(0))
			if delErr == nil {
				t.Error("Expected error for Delete with zero ID")
			}
		})

		t.Run("non-existent", func(t *testing.T) {
			delErr := service.Delete(ksid.ID(99999))
			if delErr == nil {
				t.Error("Expected error for Delete with non-existent ID")
			}
		})

		t.Run("valid", func(t *testing.T) {
			delErr := service.Delete(inv.ID)
			if delErr != nil {
				t.Fatalf("Failed to delete invitation: %v", delErr)
			}

			_, getErr := service.GetByToken(inv.Token)
			if getErr == nil {
				t.Error("Expected invitation to be deleted")
			}

			// Second invitation should still exist
			found2, getErr := service.GetByToken(inv2.Token)
			if getErr != nil {
				t.Fatalf("Second invitation should still exist: %v", getErr)
			}
			if found2.ID != inv2.ID {
				t.Error("Wrong invitation returned")
			}
		})
	})
}

// invitationClaimsStore drives the same claim lifecycle through both concrete services.
type invitationClaimsStore struct {
	create    func() (string, ksid.ID, error)
	accept    func(string, *identityv1.User, func() error) error
	get       func(string) (ksid.ID, storagev1.Time, error)
	revoke    func(ksid.ID) error
	completed func(func())
}

type invitationClaimObserver[T jsonldb.Row[T]] struct{ update func(T) }

func (o *invitationClaimObserver[T]) OnAppend(T)         {}
func (o *invitationClaimObserver[T]) OnDelete(T)         {}
func (o *invitationClaimObserver[T]) OnUpdate(_, next T) { o.update(next) }

func newInvitationClaimsStore(t *testing.T, kind, path string) invitationClaimsStore {
	if kind == "org" {
		svc, err := NewOrganizationInvitationService(path)
		if err != nil {
			t.Fatal(err)
		}
		return invitationClaimsStore{
			create: func() (string, ksid.ID, error) {
				inv, err := svc.Create("", ksid.ID(1), identityv1.OrgRoleMember, ksid.ID(2))
				if err != nil {
					return "", 0, err
				}
				return inv.Token, inv.ID, nil
			},
			accept: func(token string, user *identityv1.User, grant func() error) error {
				_, err := svc.Accept(token, user, func(*identityv1.OrganizationInvitation) error { return grant() })
				return err
			},
			get: func(token string) (ksid.ID, storagev1.Time, error) {
				inv, err := svc.GetByToken(token)
				if err != nil {
					return 0, 0, err
				}
				return inv.ClaimedBy, inv.AcceptedAt, nil
			},
			revoke: svc.Delete,
			completed: func(fn func()) {
				svc.table.AddObserver(&invitationClaimObserver[*identityv1.OrganizationInvitation]{update: func(inv *identityv1.OrganizationInvitation) {
					if !inv.AcceptedAt.IsZero() {
						fn()
					}
				}})
			},
		}
	}
	svc, err := NewWorkspaceInvitationService(path)
	if err != nil {
		t.Fatal(err)
	}
	return invitationClaimsStore{
		create: func() (string, ksid.ID, error) {
			inv, err := svc.Create("", ksid.ID(1), identityv1.WSRoleViewer, ksid.ID(2))
			if err != nil {
				return "", 0, err
			}
			return inv.Token, inv.ID, nil
		},
		accept: func(token string, user *identityv1.User, grant func() error) error {
			_, err := svc.Accept(token, user, func(*identityv1.WorkspaceInvitation) error { return grant() })
			return err
		},
		get: func(token string) (ksid.ID, storagev1.Time, error) {
			inv, err := svc.GetByToken(token)
			if err != nil {
				return 0, 0, err
			}
			return inv.ClaimedBy, inv.AcceptedAt, nil
		},
		revoke: svc.Delete,
		completed: func(fn func()) {
			svc.table.AddObserver(&invitationClaimObserver[*identityv1.WorkspaceInvitation]{update: func(inv *identityv1.WorkspaceInvitation) {
				if !inv.AcceptedAt.IsZero() {
					fn()
				}
			}})
		},
	}
}

// blockInvitationDirectory makes writes fail without altering the published table.
func blockInvitationDirectory(t *testing.T, dir string) func() {
	parked := dir + "-parked"
	if err := os.Rename(dir, parked); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, []byte("write obstacle"), 0o600); err != nil {
		t.Fatal(err)
	}
	restored := false
	restore := func() {
		if restored {
			return
		}
		restored = true
		if err := os.Remove(dir); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(parked, dir); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(restore)
	return restore
}

func testInvitationClaims(t *testing.T, kind string) {
	t.Run("race for one grant", func(t *testing.T) {
		t.Parallel()
		svc := newInvitationClaimsStore(t, kind, filepath.Join(t.TempDir(), "invitations.jsonl"))
		token, _, err := svc.create()
		if err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		results := make(chan error, 2)
		var grants atomic.Int32
		var wg sync.WaitGroup
		for _, id := range []ksid.ID{3, 4} {
			wg.Go(func() {
				<-start
				results <- svc.accept(token, &identityv1.User{ID: id}, func() error { grants.Add(1); return nil })
			})
		}
		close(start)
		wg.Wait()
		close(results)
		successes := 0
		for err := range results {
			if err == nil {
				successes++
			}
		}
		if successes != 1 || grants.Load() != 1 {
			t.Fatalf("successful claims=%d grants=%d", successes, grants.Load())
		}
	})
	for _, failure := range []string{"claim persistence", "grant", "completion persistence", "cleanup"} {
		t.Run(failure, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			path := filepath.Join(dir, "invitations.jsonl")
			svc := newInvitationClaimsStore(t, kind, path)
			token, _, err := svc.create()
			if err != nil {
				t.Fatal(err)
			}
			first := &identityv1.User{ID: 3}
			other := &identityv1.User{ID: 4}
			var restore func()
			if failure == "claim persistence" {
				restore = blockInvitationDirectory(t, dir)
			}
			if failure == "cleanup" {
				svc.completed(func() { restore = blockInvitationDirectory(t, dir) })
			}
			grants := 0
			err = svc.accept(token, first, func() error {
				grants++
				if failure == "grant" {
					return errors.New("membership write failed")
				}
				if failure == "completion persistence" {
					restore = blockInvitationDirectory(t, dir)
				}
				return nil
			})
			if err == nil {
				t.Fatal("injected failure succeeded")
			}
			if restore != nil {
				restore()
			}
			restarted := newInvitationClaimsStore(t, kind, path)
			claimant, accepted, err := restarted.get(token)
			if err != nil {
				t.Fatal(err)
			}
			if failure == "claim persistence" {
				if grants != 0 || !claimant.IsZero() {
					t.Fatalf("failed claim granted=%d claimant=%v", grants, claimant)
				}
				if err := restarted.accept(token, other, func() error { return nil }); err != nil {
					t.Fatal(err)
				}
				return
			}
			if claimant != first.ID {
				t.Fatalf("restart lost claimant: %v", claimant)
			}
			if err := restarted.accept(token, other, func() error { t.Error("second account received grant"); return nil }); !errors.Is(err, ErrInvitationClaimed) {
				t.Fatalf("second claim: %v", err)
			}
			retryGrants := 0
			if err := restarted.accept(token, first, func() error { retryGrants++; return nil }); err != nil {
				t.Fatal(err)
			}
			if failure == "cleanup" {
				if accepted.IsZero() || retryGrants != 0 {
					t.Fatalf("completed claim replayed grant=%d accepted=%v", retryGrants, accepted)
				}
			} else if !accepted.IsZero() || retryGrants != 1 {
				t.Fatalf("incomplete claim retry grants=%d accepted=%v", retryGrants, accepted)
			}
		})
	}
	t.Run("failed save retries persist the claimant", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "invitations.jsonl")
		svc := newInvitationClaimsStore(t, kind, path)
		token, _, err := svc.create()
		if err != nil {
			t.Fatal(err)
		}
		restore := blockInvitationDirectory(t, dir)
		first := &identityv1.User{ID: 3}
		if err := svc.accept(token, first, func() error { t.Error("failed save granted membership"); return nil }); err == nil {
			t.Fatal("claim save succeeded unexpectedly")
		}
		restore()
		if err := svc.accept(token, first, func() error { return errors.New("interrupted after partial membership grant") }); err == nil {
			t.Fatal("grant failure succeeded unexpectedly")
		}
		restarted := newInvitationClaimsStore(t, kind, path)
		if err := restarted.accept(token, &identityv1.User{ID: 4}, func() error { t.Error("other account reused undurable claimant"); return nil }); !errors.Is(err, ErrInvitationClaimed) {
			t.Fatalf("restart allowed a different claimant: %v", err)
		}
	})
	t.Run("revocation wins before claim", func(t *testing.T) {
		t.Parallel()
		svc := newInvitationClaimsStore(t, kind, filepath.Join(t.TempDir(), "invitations.jsonl"))
		token, id, err := svc.create()
		if err != nil {
			t.Fatal(err)
		}
		if err := svc.revoke(id); err != nil {
			t.Fatal(err)
		}
		if err := svc.accept(token, &identityv1.User{ID: 3}, func() error { t.Error("revoked invitation granted"); return nil }); err == nil {
			t.Fatal("revoked token accepted")
		}
	})
	t.Run("revocation races with acceptance", func(t *testing.T) {
		t.Parallel()
		svc := newInvitationClaimsStore(t, kind, filepath.Join(t.TempDir(), "invitations.jsonl"))
		token, id, err := svc.create()
		if err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		accepted := make(chan error, 1)
		revoked := make(chan error, 1)
		var grants atomic.Int32
		var wg sync.WaitGroup
		wg.Go(func() {
			<-start
			accepted <- svc.accept(token, &identityv1.User{ID: 3}, func() error { grants.Add(1); return nil })
		})
		wg.Go(func() { <-start; revoked <- svc.revoke(id) })
		close(start)
		wg.Wait()
		ae, re := <-accepted, <-revoked
		if ae == nil {
			if re == nil || grants.Load() != 1 {
				t.Fatalf("accepted=%v revoked=%v grants=%d", ae, re, grants.Load())
			}
		} else if re != nil || grants.Load() != 0 {
			t.Fatalf("accepted=%v revoked=%v grants=%d", ae, re, grants.Load())
		}
	})
}
