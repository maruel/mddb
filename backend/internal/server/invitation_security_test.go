// Tests that invitation links never authenticate users or grant permissions to unproven addresses.

package server

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

func TestInvitationEmailOwnership(t *testing.T) {
	for _, kind := range []string{"org", "ws"} {
		t.Run(kind, func(t *testing.T) {
			e := setupTestEnv(t)
			register := func(email string) dto.AuthResponse {
				var out dto.AuthResponse
				if got := e.doJSON(t, http.MethodPost, "/api/v1/auth/register", dto.RegisterRequest{Email: email, Password: "password123", Name: email}, &out, ""); got != http.StatusOK {
					t.Fatalf("register: %d", got)
				}
				return out
			}
			owner := register("owner@example.com")
			recipient := register("recipient@example.com")
			org, err := e.orgService.Create(t.Context(), "Organization", "owner@example.com")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := e.orgMemService.Create(owner.User.ID, org.ID, identity.OrgRoleOwner); err != nil {
				t.Fatal(err)
			}
			ws, err := e.wsService.Create(t.Context(), org.ID, "Workspace")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := e.wsMemService.Create(owner.User.ID, ws.ID, identity.WSRoleAdmin); err != nil {
				t.Fatal(err)
			}
			notifications, err := identity.NewNotificationService(filepath.Join(t.TempDir(), "notifications.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			e.services.Notification = notifications
			var token string
			if kind == "org" {
				if got := e.doJSON(t, http.MethodPost, "/api/v1/organizations/"+org.ID.String()+"/invitations", dto.CreateOrgInvitationRequest{Email: recipient.User.Email, Role: dto.OrgRoleMember}, nil, owner.Token); got != http.StatusOK {
					t.Fatalf("invite: %d", got)
				}
				for inv := range e.orgInvService.IterByOrg(org.ID) {
					token = inv.Token
				}
			} else {
				if got := e.doJSON(t, http.MethodPost, "/api/v1/workspaces/"+ws.ID.String()+"/invitations", dto.CreateWSInvitationRequest{Email: recipient.User.Email, Role: dto.WSRoleEditor}, nil, owner.Token); got != http.StatusOK {
					t.Fatalf("invite: %d", got)
				}
				for inv := range e.wsInvService.IterByWorkspace(ws.ID) {
					token = inv.Token
				}
			}
			if token == "" {
				t.Fatal("missing invitation")
			}
			if got := notifications.CountByUser(recipient.User.ID); got != 0 {
				t.Fatalf("unverified account received %d invitation notifications", got)
			}
			path := "/api/v1/auth/invitations/" + kind + "/accept"
			body := dto.AcceptInvitationRequest{Token: token}
			before := e.services.Session.CountActive()
			for _, tc := range []struct {
				name, auth string
				status     int
			}{
				{"token alone", "", http.StatusUnauthorized},
				{"inviter reads own link", owner.Token, http.StatusForbidden},
				{"unverified squatter", recipient.Token, http.StatusForbidden},
			} {
				t.Run(tc.name, func(t *testing.T) {
					if got := e.doJSON(t, http.MethodPost, path, body, nil, tc.auth); got != tc.status {
						t.Fatalf("accept: %d, want %d", got, tc.status)
					}
					if got := e.services.Session.CountActive(); got != before {
						t.Fatalf("accept created a session: %d -> %d", before, got)
					}
					if _, err := e.orgMemService.Get(recipient.User.ID, org.ID); err == nil {
						t.Fatal("unproven recipient gained organization membership")
					}
					if _, err := e.wsMemService.Get(recipient.User.ID, ws.ID); err == nil {
						t.Fatal("unproven recipient gained workspace membership")
					}
				})
			}
			if _, err := e.userService.Modify(owner.User.ID, func(u *identity.User) error { u.EmailVerified = true; return nil }); err != nil {
				t.Fatal(err)
			}
			if got := e.doJSON(t, http.MethodPost, path, body, nil, owner.Token); got != http.StatusForbidden {
				t.Fatalf("verified wrong recipient: %d", got)
			}
			// Independent email/provider proof is modeled at the user service boundary.
			if _, err := e.userService.Modify(recipient.User.ID, func(u *identity.User) error { u.EmailVerified = true; return nil }); err != nil {
				t.Fatal(err)
			}
			var out map[string]any
			if got := e.doJSON(t, http.MethodPost, path, body, &out, recipient.Token); got != http.StatusOK {
				t.Fatalf("verified recipient: %d", got)
			}
			if out["id"] != recipient.User.ID.String() {
				t.Fatalf("response user: %v", out)
			}
			if _, ok := out["token"]; ok {
				t.Fatal("invitation issued credential")
			}
			if got := e.services.Session.CountActive(); got != before {
				t.Fatalf("accept created session: %d -> %d", before, got)
			}
			if _, err := e.orgMemService.Get(recipient.User.ID, org.ID); err != nil {
				t.Fatal("missing accepted org membership", err)
			}
			if kind == "ws" {
				if _, err := e.wsMemService.Get(recipient.User.ID, ws.ID); err != nil {
					t.Fatal("missing accepted workspace membership", err)
				}
			}
		})
	}
}
