// Persisted user rows, linked OAuth identities, and user preferences.

package v1

import (
	"github.com/maruel/ksid"
	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"
)

// User represents a system user (persistent fields only).
type User struct {
	ID              ksid.ID         `json:"id" jsonschema:"description=Unique user identifier"`
	Email           string          `json:"email" jsonschema:"description=User email address"`
	EmailVerified   bool            `json:"email_verified,omitempty" jsonschema:"description=Whether the email has been verified"`
	Name            string          `json:"name" jsonschema:"description=User display name"`
	IsGlobalAdmin   bool            `json:"is_global_admin,omitempty" jsonschema:"description=Whether user has server-wide administrative access"`
	OAuthIdentities []OAuthIdentity `json:"oauth_identities,omitempty" jsonschema:"description=Linked OAuth provider accounts"`
	Quotas          UserQuota       `json:"quotas" jsonschema:"description=Resource limits for the user"`
	Settings        UserSettings    `json:"settings" jsonschema:"description=Global user preferences"`
	Created         storagev1.Time  `json:"created" jsonschema:"description=Account creation timestamp"`
	Modified        storagev1.Time  `json:"modified" jsonschema:"description=Last modification timestamp"`
}

// GetID returns the User's ID.
func (u *User) GetID() ksid.ID {
	return u.ID
}

// UserSettings represents global user preferences.
type UserSettings struct {
	Theme                string                  `json:"theme" jsonschema:"description=UI theme preference (light/dark/system)"`
	Language             string                  `json:"language" jsonschema:"description=Preferred language code (en/fr/etc)"`
	LastActiveWorkspaces []ksid.ID               `json:"last_active_workspaces,omitempty" jsonschema:"description=Recently used workspace IDs in LRU order (most recent first)"`
	NotificationPrefs    NotificationPreferences `json:"notification_prefs,omitzero" jsonschema:"description=Per-type notification channel preferences"`
}

// UserQuota defines limits for a user.
type UserQuota struct {
	MaxOrganizations int `json:"max_organizations" jsonschema:"description=Maximum number of organizations the user can create"`
}

// OAuthProvider represents a supported OAuth2 provider.
type OAuthProvider string

// IsValid checks if the provider is a known valid value.
func (p OAuthProvider) IsValid() bool {
	switch p {
	case OAuthProviderGoogle, OAuthProviderMicrosoft, OAuthProviderGitHub:
		return true
	}
	return false
}

const (
	// OAuthProviderGoogle represents Google OAuth.
	OAuthProviderGoogle OAuthProvider = "google"
	// OAuthProviderMicrosoft represents Microsoft OAuth.
	OAuthProviderMicrosoft OAuthProvider = "microsoft"
	// OAuthProviderGitHub represents GitHub OAuth.
	OAuthProviderGitHub OAuthProvider = "github"
)

// OAuthIdentity represents a link between a local user and an OAuth2 provider.
type OAuthIdentity struct {
	Provider   OAuthProvider  `json:"provider" jsonschema:"description=OAuth provider name (google/microsoft/github)"`
	ProviderID string         `json:"provider_id" jsonschema:"description=User ID at the OAuth provider"`
	Email      string         `json:"email" jsonschema:"description=Email address from OAuth provider"`
	AvatarURL  string         `json:"avatar_url,omitempty" jsonschema:"description=Profile picture URL from OAuth provider"`
	LastLogin  storagev1.Time `json:"last_login" jsonschema:"description=Last login timestamp via this provider"`
}

// StoredUser is a users.jsonl row: the user and its password hash.
type StoredUser struct {
	User

	PasswordHash string `json:"password_hash" jsonschema:"description=Bcrypt-hashed password"`
}

// Clone returns a deep copy of the StoredUser.
func (u *StoredUser) Clone() *StoredUser {
	c := *u
	if u.OAuthIdentities != nil {
		c.OAuthIdentities = make([]OAuthIdentity, len(u.OAuthIdentities))
		copy(c.OAuthIdentities, u.OAuthIdentities)
	}
	return &c
}

// GetID returns the StoredUser's ID.
func (u *StoredUser) GetID() ksid.ID {
	return u.ID
}

// Validate checks that the StoredUser is valid.
func (u *StoredUser) Validate() error {
	if u.ID.IsZero() {
		return ErrUserIDRequired
	}
	if u.Email == "" {
		return ErrEmailEmpty
	}
	if u.Quotas.MaxOrganizations <= 0 {
		return ErrInvalidUserQuota
	}
	return nil
}
