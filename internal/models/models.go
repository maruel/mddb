// Package models defines the core data structures used throughout the application.
package models

import (
	"context"
	"time"
)

// GetOrgID extracts the organization ID from the context.
// Note: This is now a convenience helper that should be used cautiously as we
// transition to explicit path-based organization IDs.
func GetOrgID(ctx context.Context) string {
	val := ctx.Value(OrgKey)
	if id, ok := val.(string); ok {
		return id
	}
	return ""
}

// Node represents the unified content entity (can be a Page, a Database, or both)
type Node struct {
	ID         string    `json:"id"`
	ParentID   string    `json:"parent_id,omitempty"` // For hierarchical structure
	Title      string    `json:"title"`
	Content    string    `json:"content,omitempty"` // Markdown content (Page part)
	Columns    []Column  `json:"columns,omitempty"` // Schema (Database part)
	Created    time.Time `json:"created"`
	Modified   time.Time `json:"modified"`
	Tags       []string  `json:"tags,omitempty"`
	FaviconURL string    `json:"favicon_url,omitempty"`
	Type       NodeType  `json:"type"`               // document, database, or both
	Children   []*Node   `json:"children,omitempty"` // Nested nodes
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

// Column represents a database column
type Column struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Type     string   `json:"type"` // text, number, select, multi_select, checkbox, date
	Options  []string `json:"options,omitempty"`
	Required bool     `json:"required,omitempty"`
}

// DataRecord represents a database record associated with a node
type DataRecord struct {
	ID       string                 `json:"id"`
	Data     map[string]interface{} `json:"data"`
	Created  time.Time              `json:"created"`
	Modified time.Time              `json:"modified"`
}

// User represents a system user.
type User struct {
	ID              string          `json:"id"`
	Email           string          `json:"email"`
	Name            string          `json:"name"`
	Memberships     []Membership    `json:"memberships,omitempty"`
	OAuthIdentities []OAuthIdentity `json:"oauth_identities,omitempty"`
	Settings        UserSettings    `json:"settings"`
	Created         time.Time       `json:"created"`
	Modified        time.Time       `json:"modified"`
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
	UserID           string             `json:"user_id"`
	OrganizationID   string             `json:"organization_id"`
	OrganizationName string             `json:"organization_name,omitempty"`
	Role             UserRole           `json:"role"`
	Settings         MembershipSettings `json:"settings"`
	Created          time.Time          `json:"created"`
}

// MembershipSettings represents user preferences within a specific organization.
type MembershipSettings struct {
	Notifications bool `json:"notifications"`
}

// UserRole defines the permissions for a user.
type UserRole string

const (
	// RoleAdmin has full access to all resources and settings.
	RoleAdmin UserRole = "admin"
	// RoleEditor can create and modify content but cannot manage users.
	RoleEditor UserRole = "editor"
	// RoleViewer can only read content.
	RoleViewer UserRole = "viewer"
)

// Organization represents a workspace or group of users.
type Organization struct {
	ID       string               `json:"id"`
	Name     string               `json:"name"`
	Quotas   Quota                `json:"quotas"`
	Settings OrganizationSettings `json:"settings"`
	Created  time.Time            `json:"created"`
}

// OrganizationSettings represents organization-wide settings.
type OrganizationSettings struct {
	AllowedDomains []string `json:"allowed_domains,omitempty"`
	PublicAccess   bool     `json:"public_access"`
}

// Quota defines limits for an organization.
type Quota struct {
	MaxPages   int   `json:"max_pages"`
	MaxStorage int64 `json:"max_storage"` // in bytes
	MaxUsers   int   `json:"max_users"`
}

// Invitation represents a request for a user to join an organization.
type Invitation struct {
	ID             string    `json:"id"`
	Email          string    `json:"email"`
	OrganizationID string    `json:"organization_id"`
	Role           UserRole  `json:"role"`
	Token          string    `json:"token"`
	ExpiresAt      time.Time `json:"expires_at"`
	Created        time.Time `json:"created"`
}

// Session represents an active user session.
type Session struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

// ContextKey is a custom type for context keys to avoid collisions.
type ContextKey string

const (
	// UserKey is the context key for the authenticated user.
	UserKey ContextKey = "user"
	// OrgKey is the context key for the active organization ID.
	OrgKey ContextKey = "org"
)

// Asset represents an uploaded file/image associated with a node
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
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	Content    string    `json:"content"`
	Created    time.Time `json:"created"`
	Modified   time.Time `json:"modified"`
	Tags       []string  `json:"tags,omitempty"`
	Path       string    `json:"path"`
	FaviconURL string    `json:"favicon_url,omitempty"`
}

// Database is kept for backward compatibility
type Database struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Columns  []Column  `json:"columns"`
	Created  time.Time `json:"created"`
	Modified time.Time `json:"modified"`
	Path     string    `json:"path"`
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
	NodeID   string            `json:"node_id"`
	RecordID string            `json:"record_id,omitempty"`
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
