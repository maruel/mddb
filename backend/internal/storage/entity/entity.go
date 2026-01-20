// Package entity defines persistent domain models for storage in jsonldb.
//
// This package contains the core domain types that are persisted to disk via
// jsonldb. These types implement the jsonldb.Row interface (Clone, GetID,
// Validate) and use jsonldb.ID for unique identifiers.
//
// The entity package is the foundation of the data model:
//   - Node: Unified content entity (document, database, or hybrid)
//   - User: System users with OAuth identities
//   - Organization: Workspaces with quotas and settings
//   - Membership: User-organization relationships with roles
//   - Invitation: Pending invitations to join organizations
//   - DataRecord: Individual records within database nodes
//
// Types in this package are storage-oriented and use jsonldb.ID (uint64) for
// identifiers. For API representations with string IDs and RFC3339 timestamps,
// see the dto package.
//
// Architecture note: The entity package has no dependencies on HTTP or API
// concerns. It only depends on jsonldb for ID types and is designed to be
// imported by both storage services and the dto package for conversions.
package entity

import (
	"errors"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
)

var (
	errIDRequired     = errors.New("id is required")
	errUserIDRequired = errors.New("user_id is required")
	errOrgIDRequired  = errors.New("organization_id is required")
	errRoleRequired   = errors.New("role is required")
	errNameRequired   = errors.New("name is required")
	errEmailRequired  = errors.New("email is required")
	errTokenRequired  = errors.New("token is required")
)

// Node represents the unified content entity (can be a Page, a Database, or both).
type Node struct {
	ID         jsonldb.ID `json:"id" jsonschema:"description=Unique node identifier"`
	ParentID   jsonldb.ID `json:"parent_id,omitempty" jsonschema:"description=Parent node ID for hierarchical structure"`
	Title      string     `json:"title" jsonschema:"description=Node title"`
	Content    string     `json:"content,omitempty" jsonschema:"description=Markdown content (Page part)"`
	Properties []Property `json:"properties,omitempty" jsonschema:"description=Schema definition (Database part)"`
	Created    time.Time  `json:"created" jsonschema:"description=Node creation timestamp"`
	Modified   time.Time  `json:"modified" jsonschema:"description=Last modification timestamp"`
	Tags       []string   `json:"tags,omitempty" jsonschema:"description=Node tags for categorization"`
	FaviconURL string     `json:"favicon_url,omitempty" jsonschema:"description=Custom favicon URL"`
	Type       NodeType   `json:"type" jsonschema:"description=Node type (document/database/hybrid)"`
	Children   []*Node    `json:"children,omitempty" jsonschema:"description=Nested child nodes"`
}

// NodeType defines what features are enabled for a node.
type NodeType string

const (
	// NodeTypeDocument represents a markdown document.
	NodeTypeDocument NodeType = "document"
	// NodeTypeDatabase represents a structured database.
	NodeTypeDatabase NodeType = "database"
	// NodeTypeHybrid represents an entity that is both a document and a database.
	NodeTypeHybrid NodeType = "hybrid"
)

// DataRecord represents a record in a database.
type DataRecord struct {
	ID       jsonldb.ID     `json:"id" jsonschema:"description=Unique record identifier"`
	Data     map[string]any `json:"data" jsonschema:"description=Record field values keyed by property name"`
	Created  time.Time      `json:"created" jsonschema:"description=Record creation timestamp"`
	Modified time.Time      `json:"modified" jsonschema:"description=Last modification timestamp"`
}

// Clone returns a deep copy of the DataRecord.
func (r *DataRecord) Clone() *DataRecord {
	c := *r
	if r.Data != nil {
		c.Data = make(map[string]any, len(r.Data))
		for k, v := range r.Data {
			c.Data[k] = v
		}
	}
	return &c
}

// GetID returns the DataRecord's ID.
func (r *DataRecord) GetID() jsonldb.ID {
	return r.ID
}

// Validate checks that the DataRecord is valid.
func (r *DataRecord) Validate() error {
	if r.ID.IsZero() {
		return errIDRequired
	}
	return nil
}

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

// Membership represents a user's relationship with an organization (persistent fields only).
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
		return errUserIDRequired
	}
	if m.OrganizationID.IsZero() {
		return errOrgIDRequired
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

// Organization represents a workspace or group of users.
type Organization struct {
	ID         jsonldb.ID           `json:"id" jsonschema:"description=Unique organization identifier"`
	Name       string               `json:"name" jsonschema:"description=Display name of the organization"`
	Quotas     Quota                `json:"quotas" jsonschema:"description=Resource limits for the organization"`
	Settings   OrganizationSettings `json:"settings" jsonschema:"description=Organization-wide configuration"`
	Onboarding OnboardingState      `json:"onboarding" jsonschema:"description=Initial setup progress tracking"`
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

// Quota defines limits for an organization.
type Quota struct {
	MaxPages   int   `json:"max_pages" jsonschema:"description=Maximum number of pages allowed"`
	MaxStorage int64 `json:"max_storage" jsonschema:"description=Maximum storage in bytes"`
	MaxUsers   int   `json:"max_users" jsonschema:"description=Maximum number of users allowed"`
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
		return errEmailRequired
	}
	if i.OrganizationID.IsZero() {
		return errOrgIDRequired
	}
	if i.Token == "" {
		return errTokenRequired
	}
	return nil
}

// Session represents an active user session.
type Session struct {
	ID        jsonldb.ID `json:"id" jsonschema:"description=Unique session identifier"`
	UserID    jsonldb.ID `json:"user_id" jsonschema:"description=User this session belongs to"`
	ExpiresAt time.Time  `json:"expires_at" jsonschema:"description=Session expiration timestamp"`
}

// Asset represents an uploaded file/image associated with a node.
// Asset IDs are filenames, not generated IDs, hence the string type.
type Asset struct {
	ID       string    `json:"id" jsonschema:"description=Asset identifier (filename)"`
	Name     string    `json:"name" jsonschema:"description=Original filename"`
	MimeType string    `json:"mime_type" jsonschema:"description=MIME type of the asset"`
	Size     int64     `json:"size" jsonschema:"description=File size in bytes"`
	Created  time.Time `json:"created" jsonschema:"description=Upload timestamp"`
	Path     string    `json:"path" jsonschema:"description=Storage path on disk"`
}

// Commit represents a commit in git history.
type Commit struct {
	Hash      string    `json:"hash" jsonschema:"description=Git commit hash"`
	Message   string    `json:"message" jsonschema:"description=Commit message"`
	Timestamp time.Time `json:"timestamp" jsonschema:"description=Commit timestamp"`
}

// CommitDetail contains full commit information.
type CommitDetail struct {
	Hash      string    `json:"hash" jsonschema:"description=Git commit hash"`
	Timestamp time.Time `json:"timestamp" jsonschema:"description=Commit timestamp"`
	Author    string    `json:"author" jsonschema:"description=Commit author name"`
	Email     string    `json:"email" jsonschema:"description=Commit author email"`
	Subject   string    `json:"subject" jsonschema:"description=Commit subject line"`
	Body      string    `json:"body" jsonschema:"description=Commit body message"`
}

// SearchResult represents a single search result.
type SearchResult struct {
	Type     string            `json:"type" jsonschema:"description=Result type (page or record)"`
	NodeID   jsonldb.ID        `json:"node_id" jsonschema:"description=Node containing the result"`
	RecordID jsonldb.ID        `json:"record_id,omitempty" jsonschema:"description=Record ID if result is a database record"`
	Title    string            `json:"title" jsonschema:"description=Title of the matched item"`
	Snippet  string            `json:"snippet" jsonschema:"description=Text snippet with match context"`
	Score    float64           `json:"score" jsonschema:"description=Relevance score"`
	Matches  map[string]string `json:"matches" jsonschema:"description=Matched fields and their values"`
	Modified time.Time         `json:"modified" jsonschema:"description=Last modification timestamp"`
}

// SearchOptions defines parameters for a search.
type SearchOptions struct {
	Query       string `json:"query" jsonschema:"description=Search query string"`
	Limit       int    `json:"limit,omitempty" jsonschema:"description=Maximum number of results"`
	MatchTitle  bool   `json:"match_title,omitempty" jsonschema:"description=Search in titles"`
	MatchBody   bool   `json:"match_body,omitempty" jsonschema:"description=Search in body content"`
	MatchFields bool   `json:"match_fields,omitempty" jsonschema:"description=Search in database fields"`
}
