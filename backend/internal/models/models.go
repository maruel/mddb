// Package models defines the core domain types and API contracts.
//
// It includes domain entities (Node, User, Organization, Membership),
// API request/response types, and structured error handling with APIError.
package models

import (
	"context"
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
	Content    string     `json:"content,omitempty"` // Markdown content (Page part)
	Columns    []Column   `json:"columns,omitempty"` // Schema (Database part)
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

// ColumnType represents the type of a database column at the application level.
// This includes both primitive storage types and higher-level semantic types.
type ColumnType string

const (
	// Primitive types (map directly to jsonldb storage types)

	// ColumnTypeText stores text values.
	ColumnTypeText ColumnType = "text"
	// ColumnTypeNumber stores numeric values (integer or float).
	ColumnTypeNumber ColumnType = "number"
	// ColumnTypeCheckbox stores boolean values as 0/1.
	ColumnTypeCheckbox ColumnType = "checkbox"
	// ColumnTypeDate stores ISO8601 date strings.
	ColumnTypeDate ColumnType = "date"

	// High-level semantic types (stored as text in jsonldb)

	// ColumnTypeSelect stores a single selection from predefined options.
	ColumnTypeSelect ColumnType = "select"
	// ColumnTypeMultiSelect stores multiple selections as a JSON array string.
	ColumnTypeMultiSelect ColumnType = "multi_select"
)

// StorageType returns the underlying jsonldb storage type for this column type.
// High-level types like select and multi_select map to text storage.
func (ct ColumnType) StorageType() jsonldb.ColumnType {
	switch ct {
	case ColumnTypeText, ColumnTypeSelect, ColumnTypeMultiSelect:
		return jsonldb.ColumnTypeText
	case ColumnTypeNumber:
		return jsonldb.ColumnTypeNumber
	case ColumnTypeCheckbox:
		return jsonldb.ColumnTypeBool
	case ColumnTypeDate:
		return jsonldb.ColumnTypeDate
	default:
		return jsonldb.ColumnTypeText
	}
}

// Column represents a database column
type Column struct {
	Name     string     `json:"name"`
	Type     ColumnType `json:"type"`
	Required bool       `json:"required,omitempty"`
	// Options contains the allowed values for select and multi_select column types.
	// Ignored for other column types.
	Options []string `json:"options,omitempty"`
}

// DataRecord represents a record in a database.
type DataRecord struct {
	ID       jsonldb.ID     `json:"id"`
	Data     map[string]any `json:"data"`
	Created  time.Time      `json:"created"`
	Modified time.Time      `json:"modified"`
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

// User represents a system user.
type User struct {
	ID              jsonldb.ID      `json:"id"`
	Email           string          `json:"email"`
	Name            string          `json:"name"`
	Memberships     []Membership    `json:"memberships,omitempty"`
	OAuthIdentities []OAuthIdentity `json:"oauth_identities,omitempty"`
	Settings        UserSettings    `json:"settings"`
	Created         time.Time       `json:"created"`
	Modified        time.Time       `json:"modified"`

	// Active context (populated in API responses)
	OrganizationID jsonldb.ID       `json:"organization_id,omitempty"`
	Role           UserRole         `json:"role,omitempty"`
	Onboarding     *OnboardingState `json:"onboarding,omitempty"`
}

// GetID returns the User's ID.
func (u *User) GetID() jsonldb.ID {
	return u.ID
}

// UserSettings represents global user preferences.
type UserSettings struct {
	Theme    string `json:"theme"`    // light, dark, system
	Language string `json:"language"` // en, fr, etc.
}

// OAuthIdentity represents a link between a local user and an OAuth2 provider.
type OAuthIdentity struct {
	Provider   string    `json:"provider"` // google, microsoft
	ProviderID string    `json:"provider_id"`
	Email      string    `json:"email"`
	LastLogin  time.Time `json:"last_login"`
}

// Membership represents a user's relationship with an organization.
type Membership struct {
	UserID           jsonldb.ID         `json:"user_id"`
	OrganizationID   jsonldb.ID         `json:"organization_id"`
	OrganizationName string             `json:"organization_name,omitempty"`
	Role             UserRole           `json:"role"`
	Settings         MembershipSettings `json:"settings"`
	Created          time.Time          `json:"created"`
}

// Clone returns a copy of the Membership.
func (m *Membership) Clone() *Membership {
	c := *m
	return &c
}

// GetID returns zero for Membership (composite key not ID-based).
// Use All() with filtering for Membership lookups.
func (m *Membership) GetID() jsonldb.ID {
	return 0
}

// MembershipSettings represents user preferences within a specific organization.
type MembershipSettings struct {
	Notifications bool `json:"notifications"`
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
	ID         jsonldb.ID           `json:"id"`
	Name       string               `json:"name"`
	Quotas     Quota                `json:"quotas"`
	Settings   OrganizationSettings `json:"settings"`
	Onboarding OnboardingState      `json:"onboarding"`
	Created    time.Time            `json:"created"`
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

// OnboardingState tracks the progress of an organization's initial setup.
type OnboardingState struct {
	Completed bool      `json:"completed"`
	Step      string    `json:"step"` // e.g., "name", "members", "git", "done"
	UpdatedAt time.Time `json:"updated_at"`
}

// OrganizationSettings represents organization-wide settings.
type OrganizationSettings struct {
	AllowedDomains []string    `json:"allowed_domains,omitempty"`
	PublicAccess   bool        `json:"public_access"`
	Git            GitSettings `json:"git"`
}

// GitSettings contains configuration for Git remotes and synchronization.
type GitSettings struct {
	AutoPush bool `json:"auto_push"`
}

// GitRemote represents a remote repository for an organization.
type GitRemote struct {
	ID             jsonldb.ID `json:"id"`
	OrganizationID jsonldb.ID `json:"organization_id"`
	Name           string     `json:"name"` // e.g., "origin"
	URL            string     `json:"url"`
	Type           string     `json:"type"`      // "github", "gitlab", "custom"
	AuthType       string     `json:"auth_type"` // "token", "ssh"
	Created        time.Time  `json:"created"`
	LastSync       time.Time  `json:"last_sync,omitempty"`
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

// Quota defines limits for an organization.
type Quota struct {
	MaxPages   int   `json:"max_pages"`
	MaxStorage int64 `json:"max_storage"` // in bytes
	MaxUsers   int   `json:"max_users"`
}

// Invitation represents a request for a user to join an organization.
type Invitation struct {
	ID             jsonldb.ID `json:"id"`
	Email          string     `json:"email"`
	OrganizationID jsonldb.ID `json:"organization_id"`
	Role           UserRole   `json:"role"`
	Token          string     `json:"token"`
	ExpiresAt      time.Time  `json:"expires_at"`
	Created        time.Time  `json:"created"`
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
	ID       jsonldb.ID `json:"id"`
	Title    string     `json:"title"`
	Columns  []Column   `json:"columns"`
	Created  time.Time  `json:"created"`
	Modified time.Time  `json:"modified"`
	Version  string     `json:"version"` // JSONL format version (e.g., "1.0")
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
