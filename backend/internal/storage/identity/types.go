package identity

import (
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
)

// UserRole defines the permissions for a user.
type UserRole string

const (
	// UserRoleAdmin has full access to all resources and settings.
	UserRoleAdmin UserRole = "admin"
	// UserRoleEditor can create and modify content but cannot manage users.
	UserRoleEditor UserRole = "editor"
	// UserRoleViewer can only read content.
	UserRoleViewer UserRole = "viewer"
)

// User represents a system user (persistent fields only).
type User struct {
	ID              jsonldb.ID      `json:"id" jsonschema:"description=Unique user identifier"`
	Email           string          `json:"email" jsonschema:"description=User email address"`
	Name            string          `json:"name" jsonschema:"description=User display name"`
	OAuthIdentities []OAuthIdentity `json:"oauth_identities,omitempty" jsonschema:"description=Linked OAuth provider accounts"`
	Settings        UserSettings    `json:"settings" jsonschema:"description=Global user preferences"`
	Created         time.Time       `json:"created" jsonschema:"description=Account creation timestamp"`
	Modified        time.Time       `json:"modified" jsonschema:"description=Last modification timestamp"`
}

// GetID returns the User's ID.
func (u *User) GetID() jsonldb.ID {
	return u.ID
}

// UserSettings represents global user preferences.
type UserSettings struct {
	Theme    string `json:"theme" jsonschema:"description=UI theme preference (light/dark/system)"`
	Language string `json:"language" jsonschema:"description=Preferred language code (en/fr/etc)"`
}

// OAuthIdentity represents a link between a local user and an OAuth2 provider.
type OAuthIdentity struct {
	Provider   string    `json:"provider" jsonschema:"description=OAuth provider name (google/microsoft)"`
	ProviderID string    `json:"provider_id" jsonschema:"description=User ID at the OAuth provider"`
	Email      string    `json:"email" jsonschema:"description=Email address from OAuth provider"`
	LastLogin  time.Time `json:"last_login" jsonschema:"description=Last login timestamp via this provider"`
}

// Membership represents a user's relationship with an organization.
type Membership struct {
	ID             jsonldb.ID         `json:"id" jsonschema:"description=Unique membership identifier"`
	UserID         jsonldb.ID         `json:"user_id" jsonschema:"description=User ID this membership belongs to"`
	OrganizationID jsonldb.ID         `json:"organization_id" jsonschema:"description=Organization ID the user is a member of"`
	Role           UserRole           `json:"role" jsonschema:"description=User role within the organization (admin/editor/viewer)"`
	Settings       MembershipSettings `json:"settings" jsonschema:"description=User preferences within this organization"`
	Created        time.Time          `json:"created" jsonschema:"description=Membership creation timestamp"`
}

// Clone returns a copy of the Membership.
func (m *Membership) Clone() *Membership {
	c := *m
	return &c
}

// GetID returns the Membership's ID.
func (m *Membership) GetID() jsonldb.ID {
	return m.ID
}

// Validate checks that the Membership is valid.
func (m *Membership) Validate() error {
	if m.ID.IsZero() {
		return errIDRequired
	}
	if m.UserID.IsZero() {
		return errUserIDEmpty
	}
	if m.OrganizationID.IsZero() {
		return errOrgIDEmpty
	}
	if m.Role == "" {
		return errRoleRequired
	}
	return nil
}

// MembershipSettings represents user preferences within a specific organization.
type MembershipSettings struct {
	Notifications bool `json:"notifications" jsonschema:"description=Whether email notifications are enabled"`
}

// Invitation represents a request for a user to join an organization.
type Invitation struct {
	ID             jsonldb.ID `json:"id" jsonschema:"description=Unique invitation identifier"`
	Email          string     `json:"email" jsonschema:"description=Email address of the invitee"`
	OrganizationID jsonldb.ID `json:"organization_id" jsonschema:"description=Organization the user is invited to"`
	Role           UserRole   `json:"role" jsonschema:"description=Role assigned upon acceptance"`
	Token          string     `json:"token" jsonschema:"description=Secret token for invitation verification"`
	ExpiresAt      time.Time  `json:"expires_at" jsonschema:"description=Invitation expiration timestamp"`
	Created        time.Time  `json:"created" jsonschema:"description=Invitation creation timestamp"`
}

// Clone returns a copy of the Invitation.
func (i *Invitation) Clone() *Invitation {
	c := *i
	return &c
}

// GetID returns the Invitation's ID.
func (i *Invitation) GetID() jsonldb.ID {
	return i.ID
}

// Validate checks that the Invitation is valid.
func (i *Invitation) Validate() error {
	if i.ID.IsZero() {
		return errIDRequired
	}
	if i.Email == "" {
		return errEmailEmpty
	}
	if i.OrganizationID.IsZero() {
		return errOrgIDEmpty
	}
	if i.Token == "" {
		return errTokenRequired
	}
	return nil
}

// Organization represents a workspace or group of users.
type Organization struct {
	ID         jsonldb.ID           `json:"id" jsonschema:"description=Unique organization identifier"`
	Name       string               `json:"name" jsonschema:"description=Display name of the organization"`
	Quotas     Quota                `json:"quotas" jsonschema:"description=Resource limits for the organization"`
	Settings   OrganizationSettings `json:"settings" jsonschema:"description=Organization-wide configuration"`
	Onboarding OnboardingState      `json:"onboarding" jsonschema:"description=Initial setup progress tracking"`
	GitRemote  GitRemote            `json:"git_remote,omitzero" jsonschema:"description=Git remote repository configuration"`
	Created    time.Time            `json:"created" jsonschema:"description=Organization creation timestamp"`
}

// Clone returns a deep copy of the Organization.
func (o *Organization) Clone() *Organization {
	c := *o
	if o.Settings.AllowedDomains != nil {
		c.Settings.AllowedDomains = make([]string, len(o.Settings.AllowedDomains))
		copy(c.Settings.AllowedDomains, o.Settings.AllowedDomains)
	}
	return &c
}

// GetID returns the Organization's ID.
func (o *Organization) GetID() jsonldb.ID {
	return o.ID
}

// Validate checks that the Organization is valid.
func (o *Organization) Validate() error {
	if o.ID.IsZero() {
		return errIDRequired
	}
	if o.Name == "" {
		return errNameRequired
	}
	return nil
}

// OnboardingState tracks the progress of an organization's initial setup.
type OnboardingState struct {
	Completed bool      `json:"completed" jsonschema:"description=Whether onboarding is complete"`
	Step      string    `json:"step" jsonschema:"description=Current onboarding step (name/members/git/done)"`
	UpdatedAt time.Time `json:"updated_at" jsonschema:"description=Last progress update timestamp"`
}

// OrganizationSettings represents organization-wide settings.
type OrganizationSettings struct {
	AllowedDomains []string    `json:"allowed_domains,omitempty" jsonschema:"description=Email domains allowed for membership"`
	PublicAccess   bool        `json:"public_access" jsonschema:"description=Whether content is publicly accessible"`
	Git            GitSettings `json:"git" jsonschema:"description=Git synchronization configuration"`
}

// GitSettings contains configuration for Git remotes and synchronization.
type GitSettings struct {
	AutoPush bool `json:"auto_push" jsonschema:"description=Automatically push changes to remote"`
}

// GitRemote represents the single remote repository configuration for an organization.
type GitRemote struct {
	URL      string    `json:"url,omitempty" jsonschema:"description=Git repository URL"`
	Type     string    `json:"type,omitempty" jsonschema:"description=Remote type (github/gitlab/custom)"`
	AuthType string    `json:"auth_type,omitempty" jsonschema:"description=Authentication method (token/ssh)"`
	Token    string    `json:"token,omitempty" jsonschema:"description=Authentication token"`
	Created  time.Time `json:"created,omitzero" jsonschema:"description=Remote creation timestamp"`
	LastSync time.Time `json:"last_sync,omitzero" jsonschema:"description=Last synchronization timestamp"`
}

// IsZero returns true if the GitRemote has no URL configured.
func (g *GitRemote) IsZero() bool {
	return g.URL == ""
}

// Quota defines limits for an organization.
type Quota struct {
	MaxPages   int   `json:"max_pages" jsonschema:"description=Maximum number of pages allowed"`
	MaxStorage int64 `json:"max_storage" jsonschema:"description=Maximum storage in bytes"`
	MaxUsers   int   `json:"max_users" jsonschema:"description=Maximum number of users allowed"`
}
