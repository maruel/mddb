// Tests recipient proof, bearer claim recovery, invitation previews, and scoped revocation.

package server

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	identityv1 "github.com/maruel/mddb/backend/internal/storage/identity/data/v1"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/dto/v1"
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
			if _, err := e.orgMemService.Create(owner.User.ID, org.ID, identityv1.OrgRoleOwner); err != nil {
				t.Fatal(err)
			}
			ws, err := e.wsService.Create(t.Context(), org.ID, "Workspace")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := e.wsMemService.Create(owner.User.ID, ws.ID, identityv1.WSRoleAdmin); err != nil {
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
			if _, err := e.userService.Modify(owner.User.ID, func(u *identityv1.User) error { u.EmailVerified = true; return nil }); err != nil {
				t.Fatal(err)
			}
			if got := e.doJSON(t, http.MethodPost, path, body, nil, owner.Token); got != http.StatusForbidden {
				t.Fatalf("verified wrong recipient: %d", got)
			}
			// Independent email/provider proof is modeled at the user service boundary.
			if _, err := e.userService.Modify(recipient.User.ID, func(u *identityv1.User) error { u.EmailVerified = true; return nil }); err != nil {
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

func TestBearerInvitationLinks(t *testing.T) {
	for _, kind := range []string{"org", "ws"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			e := setupTestEnv(t)
			register := func(address string) dto.AuthResponse {
				var out dto.AuthResponse
				if status := e.doJSON(t, http.MethodPost, "/api/v1/auth/register", dto.RegisterRequest{Email: address, Password: "password123", Name: address}, &out, ""); status != http.StatusOK {
					t.Fatalf("register: %d", status)
				}
				return out
			}
			owner := register("owner@example.com")
			recipient := register("recipient@example.com")
			other := register("other@example.com")
			org, err := e.orgService.Create(t.Context(), "Shared organization", "owner@example.com")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := e.orgMemService.Create(owner.User.ID, org.ID, identityv1.OrgRoleOwner); err != nil {
				t.Fatal(err)
			}
			ws, err := e.wsService.Create(t.Context(), org.ID, "Shared workspace")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := e.wsMemService.Create(owner.User.ID, ws.ID, identityv1.WSRoleAdmin); err != nil {
				t.Fatal(err)
			}
			create := func(admin bool) (string, ksid.ID) {
				var link string
				var id ksid.ID
				if kind == "org" {
					role := dto.OrgRoleMember
					if admin {
						role = dto.OrgRoleAdmin
					}
					var out dto.OrgInvitationResponse
					if status := e.doJSON(t, http.MethodPost, "/api/v1/organizations/"+org.ID.String()+"/invitations", dto.CreateOrgInvitationRequest{Role: role}, &out, owner.Token); status != http.StatusOK {
						t.Fatalf("create org link: %d", status)
					}
					link, id = out.InviteURL, out.ID
				} else {
					role := dto.WSRoleViewer
					if admin {
						role = dto.WSRoleAdmin
					}
					var out dto.WSInvitationResponse
					if status := e.doJSON(t, http.MethodPost, "/api/v1/workspaces/"+ws.ID.String()+"/invitations", dto.CreateWSInvitationRequest{Role: role}, &out, owner.Token); status != http.StatusOK {
						t.Fatalf("create workspace link: %d", status)
					}
					link, id = out.InviteURL, out.ID
				}
				u, err := url.Parse(link)
				if err != nil || u.Host == "" || u.Path != "/accept-invitation/"+kind || u.Query().Get("token") == "" {
					t.Fatalf("invalid copied link: %q (%v)", link, err)
				}
				return u.Query().Get("token"), id
			}
			token, _ := create(false)
			sessions := e.services.Session.CountActive()
			var preview map[string]any
			if status := e.doJSON(t, http.MethodGet, "/api/v1/auth/invitations/"+kind+"?token="+token, nil, &preview, ""); status != http.StatusOK {
				t.Fatalf("preview: %d", status)
			}
			if preview["email"] != "" || preview["organization_id"] != org.ID.String() {
				t.Fatalf("incorrect grant metadata: %v", preview)
			}
			if _, err := e.orgMemService.Get(recipient.User.ID, org.ID); err == nil {
				t.Fatal("preview created membership")
			}
			acceptPath := "/api/v1/auth/invitations/" + kind + "/accept"
			if status := e.doJSON(t, http.MethodPost, acceptPath, dto.AcceptInvitationRequest{Token: token}, nil, ""); status != http.StatusUnauthorized {
				t.Fatalf("anonymous claim: %d", status)
			}
			if kind == "ws" {
				// An actual membership-file write failure must expose the already
				// published organization grant, not imply complete rejection.
				membershipPath := filepath.Join(e.dataDir, "db", "ws_memberships.jsonl")
				backup := membershipPath + ".saved"
				if err := os.Rename(membershipPath, backup); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(membershipPath, 0o700); err != nil {
					t.Fatal(err)
				}
				var writeFailure dto.ErrorResponse
				if status := e.doJSON(t, http.MethodPost, acceptPath, dto.AcceptInvitationRequest{Token: token}, &writeFailure, recipient.Token); status != http.StatusInternalServerError {
					t.Fatalf("failed membership persistence: %d", status)
				}
				if writeFailure.Details["organization_membership_present"] != true || writeFailure.Details["workspace_membership_present"] != false || writeFailure.Details["retryable"] != true || writeFailure.Details["persistence_uncertain"] != true {
					t.Fatalf("hidden partial write: %+v", writeFailure)
				}
				if err := os.Remove(membershipPath); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(backup, membershipPath); err != nil {
					t.Fatal(err)
				}
				// Reload the persisted claim before retry: a different account must
				// remain excluded even when the membership write never completed.
				restarted, err := identity.NewWorkspaceInvitationService(filepath.Join(e.dataDir, "db", "ws_invitations.jsonl"))
				if err != nil {
					t.Fatal(err)
				}
				e.services.WSInvitation = restarted
				e.wsInvService = restarted
				memberships, err := identity.NewOrganizationMembershipService(filepath.Join(e.dataDir, "db", "org_memberships.jsonl"), e.userService, e.orgService)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := memberships.Get(recipient.User.ID, org.ID); err != nil {
					t.Fatalf("organization grant not durable: %v", err)
				}

				// Fail after the organization membership persists. The claimant must stay
				// bound across the partial workspace grant and its retry.
				if _, err := e.orgService.Modify(org.ID, func(o *identityv1.Organization) error { o.Quotas.MaxMembersPerWorkspace = 1; return nil }); err != nil {
					t.Fatal(err)
				}
				var failure dto.ErrorResponse
				if status := e.doJSON(t, http.MethodPost, acceptPath, dto.AcceptInvitationRequest{Token: token}, &failure, recipient.Token); status != http.StatusInternalServerError {
					t.Fatalf("partial grant: %d", status)
				}
				if failure.Details["organization_id"] != org.ID.String() || failure.Details["workspace_id"] != ws.ID.String() || failure.Details["organization_membership_present"] != true || failure.Details["workspace_membership_present"] != false || failure.Details["retryable"] != true {
					t.Fatalf("partial publication not reported: %+v", failure)
				}
				if _, err := e.orgMemService.Get(recipient.User.ID, org.ID); err != nil {
					t.Fatal("partial organization membership was not retained")
				}
				if _, err := e.wsMemService.Get(recipient.User.ID, ws.ID); err == nil {
					t.Fatal("workspace quota was bypassed")
				}
				if status := e.doJSON(t, http.MethodPost, acceptPath, dto.AcceptInvitationRequest{Token: token}, nil, other.Token); status != http.StatusConflict {
					t.Fatalf("other claimant reused partial grant: %d", status)
				}
				if _, err := e.orgService.Modify(org.ID, func(o *identityv1.Organization) error { o.Quotas.MaxMembersPerWorkspace = 10; return nil }); err != nil {
					t.Fatal(err)
				}
			}
			if status := e.doJSON(t, http.MethodPost, acceptPath, dto.AcceptInvitationRequest{Token: token}, nil, recipient.Token); status != http.StatusOK {
				t.Fatalf("bearer acceptance: %d", status)
			}
			if current, err := e.userService.Get(recipient.User.ID); err != nil || current.EmailVerified {
				t.Fatal("invitation verified an unproven address")
			}
			if e.services.Session.CountActive() != sessions {
				t.Fatal("invitation minted a session")
			}
			if status := e.doJSON(t, http.MethodPost, acceptPath, dto.AcceptInvitationRequest{Token: token}, nil, other.Token); status == http.StatusOK {
				t.Fatal("bearer link was reusable by another account")
			}
			// A fresh invitation does not change a pre-existing membership's role.
			token, _ = create(true)
			if status := e.doJSON(t, http.MethodPost, acceptPath, dto.AcceptInvitationRequest{Token: token}, nil, recipient.Token); status != http.StatusOK {
				t.Fatalf("existing member acceptance: %d", status)
			}
			if kind == "org" {
				m, err := e.orgMemService.Get(recipient.User.ID, org.ID)
				if err != nil || m.Role != identityv1.OrgRoleMember {
					t.Fatal("invitation changed an existing organization role")
				}
			} else {
				m, err := e.wsMemService.Get(recipient.User.ID, ws.ID)
				if err != nil || m.Role != identityv1.WSRoleViewer {
					t.Fatal("invitation changed an existing workspace role")
				}
				var members dto.ListWorkspaceMembersResponse
				if status := e.doJSON(t, http.MethodGet, "/api/v1/workspaces/"+ws.ID.String()+"/members", nil, &members, recipient.Token); status != http.StatusOK {
					t.Fatalf("workspace-scoped members: %d", status)
				}
				found := false
				for _, member := range members.Members {
					if member.ID == recipient.User.ID {
						found = true
						if member.Role != dto.WSRoleViewer {
							t.Fatalf("listed workspace role: %s", member.Role)
						}
					}
				}
				if !found {
					t.Fatal("recipient absent from workspace member list")
				}
			}
			token, id := create(false)
			var revokePath string
			if kind == "org" {
				revokePath = "/api/v1/organizations/" + org.ID.String() + "/invitations/" + id.String() + "/delete"
			} else {
				revokePath = "/api/v1/workspaces/" + ws.ID.String() + "/invitations/" + id.String() + "/delete"
			}
			if status := e.doJSON(t, http.MethodPost, revokePath, nil, nil, other.Token); status != http.StatusForbidden {
				t.Fatalf("foreign revocation: %d", status)
			}
			if status := e.doJSON(t, http.MethodPost, revokePath, nil, nil, owner.Token); status != http.StatusOK {
				t.Fatalf("revoke link: %d", status)
			}
			if status := e.doJSON(t, http.MethodGet, "/api/v1/auth/invitations/"+kind+"?token="+token, nil, nil, ""); status != http.StatusNotFound {
				t.Fatalf("revoked preview: %d", status)
			}
			if status := e.doJSON(t, http.MethodPost, acceptPath, dto.AcceptInvitationRequest{Token: token}, nil, other.Token); status != http.StatusNotFound {
				t.Fatalf("revoked claim: %d", status)
			}
		})
	}
}
