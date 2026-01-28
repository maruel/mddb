// Defines shared service dependencies for handlers.

package handlers

import (
	"github.com/maruel/mddb/backend/internal/email"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// Services holds all service dependencies for handlers.
type Services struct {
	FileStore     *content.FileStoreService
	Search        *content.SearchService
	User          *identity.UserService
	Organization  *identity.OrganizationService
	Workspace     *identity.WorkspaceService
	OrgInvitation *identity.OrganizationInvitationService
	WSInvitation  *identity.WorkspaceInvitationService
	OrgMembership *identity.OrganizationMembershipService
	WSMembership  *identity.WorkspaceMembershipService
	Session       *identity.SessionService
	EmailVerif    *identity.EmailVerificationService // may be nil
	Email         *email.Service                     // may be nil
}

// Config holds configuration values needed by handlers.
type Config struct {
	JWTSecret    string
	BaseURL      string
	Version      string
	ServerQuotas identity.ServerQuotas
}
