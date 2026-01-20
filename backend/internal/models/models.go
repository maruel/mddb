// Package models defines the core domain types and API contracts.
//
// It includes domain entities (Node, User, Organization, Membership),
// API request/response types, and structured error handling with APIError.
package models

import (
	"context"
	"fmt"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
)

// GetOrgID extracts the organization ID from the context.
// Note: This is now a convenience helper that should be used cautiously as we
// transition to explicit path-based organization IDs.
func GetOrgID(ctx context.Context) jsonldb.ID {
	val := ctx.Value(OrgKey)
	if id, ok := val.(jsonldb.ID); ok {
		return id
	}
	return 0
}

// Node represents the unified content entity (can be a Page, a Database, or both)
type Node struct {
	ID         jsonldb.ID `json:"id"`
	ParentID   jsonldb.ID `json:"parent_id,omitempty"` // For hierarchical structure
	Title      string     `json:"title"`
	Content    string     `json:"content,omitempty"`    // Markdown content (Page part)
	Properties []Property `json:"properties,omitempty"` // Schema (Database part)
	Created    time.Time  `json:"created"`
	Modified   time.Time  `json:"modified"`
	Tags       []string   `json:"tags,omitempty"`
	FaviconURL string     `json:"favicon_url,omitempty"`
	Type       NodeType   `json:"type"`               // document, database, or both
	Children   []*Node    `json:"children,omitempty"` // Nested nodes
}

// NodeType defines what features are enabled for a node
type NodeType string

const (
	// NodeTypeDocument represents a markdown document
	NodeTypeDocument NodeType = "document"
	// NodeTypeDatabase represents a structured database
	NodeTypeDatabase NodeType = "database"
	// NodeTypeHybrid represents an entity that is both a document and a database
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
		return fmt.Errorf("id is required")
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
		return fmt.Errorf("id is required")
	}
	if m.UserID.IsZero() {
		return fmt.Errorf("user_id is required")
	}
	if m.OrganizationID.IsZero() {
		return fmt.Errorf("organization_id is required")
	}
	if m.Role == "" {
		return fmt.Errorf("role is required")
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
		return fmt.Errorf("id is required")
	}
	if o.Name == "" {
		return fmt.Errorf("name is required")
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

// GitRemote represents a remote repository for an organization.
type GitRemote struct {
	ID             jsonldb.ID `json:"id" jsonschema:"description=Unique git remote identifier"`
	OrganizationID jsonldb.ID `json:"organization_id" jsonschema:"description=Organization this remote belongs to"`
	Name           string     `json:"name" jsonschema:"description=Remote name (e.g. origin)"`
	URL            string     `json:"url" jsonschema:"description=Git repository URL"`
	Type           string     `json:"type" jsonschema:"description=Remote type (github/gitlab/custom)"`
	AuthType       string     `json:"auth_type" jsonschema:"description=Authentication method (token/ssh)"`
	Created        time.Time  `json:"created" jsonschema:"description=Remote creation timestamp"`
	LastSync       time.Time  `json:"last_sync,omitempty" jsonschema:"description=Last synchronization timestamp"`
}

// Clone returns a copy of the GitRemote.
func (g *GitRemote) Clone() *GitRemote {
	c := *g
	return &c
}

// GetID returns the GitRemote's ID.
func (g *GitRemote) GetID() jsonldb.ID {
	return g.ID
}

// Validate checks that the GitRemote is valid.
func (g *GitRemote) Validate() error {
	if g.ID.IsZero() {
		return fmt.Errorf("id is required")
	}
	if g.OrganizationID.IsZero() {
		return fmt.Errorf("organization_id is required")
	}
	if g.URL == "" {
		return fmt.Errorf("url is required")
	}
	return nil
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
		return fmt.Errorf("id is required")
	}
	if i.Email == "" {
		return fmt.Errorf("email is required")
	}
	if i.OrganizationID.IsZero() {
		return fmt.Errorf("organization_id is required")
	}
	if i.Token == "" {
		return fmt.Errorf("token is required")
	}
	return nil
}

// Session represents an active user session.
type Session struct {
	ID        jsonldb.ID `json:"id"`
	UserID    jsonldb.ID `json:"user_id"`
	ExpiresAt time.Time  `json:"expires_at"`
}

// ContextKey is a custom type for context keys to avoid collisions.
type ContextKey string

const (
	// UserKey is the context key for the authenticated user.
	UserKey ContextKey = "user"
	// OrgKey is the context key for the active organization ID.
	OrgKey ContextKey = "org"
)

// Asset represents an uploaded file/image associated with a node.
// Asset IDs are filenames, not generated IDs, hence the string type.
type Asset struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	MimeType string    `json:"mime_type"`
	Size     int64     `json:"size"`
	Created  time.Time `json:"created"`
	Path     string    `json:"path"`
}

// Legacy types for compatibility during migration (optional to keep or remove)
// For now, we refactor them to use the new Node concept or keep them if needed by existing code.

// Page is kept for backward compatibility with existing storage methods
type Page struct {
	ID         jsonldb.ID `json:"id"`
	Title      string     `json:"title"`
	Content    string     `json:"content"`
	Created    time.Time  `json:"created"`
	Modified   time.Time  `json:"modified"`
	Tags       []string   `json:"tags,omitempty"`
	Path       string     `json:"path"`
	FaviconURL string     `json:"favicon_url,omitempty"`
}

// Database represents a structured database with schema and metadata.
type Database struct {
	ID         jsonldb.ID `json:"id"`
	Title      string     `json:"title"`
	Properties []Property `json:"properties"`
	Created    time.Time  `json:"created"`
	Modified   time.Time  `json:"modified"`
	Version    string     `json:"version"` // JSONL format version (e.g., "1.0")
}

// Commit represents a commit in git history.
type Commit struct {
	Hash      string    `json:"hash"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// CommitDetail contains full commit information.
type CommitDetail struct {
	Hash      string    `json:"hash"`
	Timestamp time.Time `json:"timestamp"`
	Author    string    `json:"author"`
	Email     string    `json:"email"`
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
}

// SearchResult represents a single search result
type SearchResult struct {
	Type     string            `json:"type"` // "page" or "record"
	NodeID   jsonldb.ID        `json:"node_id"`
	RecordID jsonldb.ID        `json:"record_id,omitempty"`
	Title    string            `json:"title"`
	Snippet  string            `json:"snippet"`
	Score    float64           `json:"score"`
	Matches  map[string]string `json:"matches"`
	Modified time.Time         `json:"modified"`
}

// SearchOptions defines parameters for a search
type SearchOptions struct {
	Query       string `json:"query"`
	Limit       int    `json:"limit,omitempty"`
	MatchTitle  bool   `json:"match_title,omitempty"`
	MatchBody   bool   `json:"match_body,omitempty"`
	MatchFields bool   `json:"match_fields,omitempty"`
}
