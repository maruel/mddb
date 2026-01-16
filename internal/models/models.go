// Package models defines the core data structures used throughout the application.
package models

import "time"

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

// Record represents a database record associated with a node
type Record struct {
	ID       string                 `json:"id"`
	Data     map[string]interface{} `json:"data"`
	Created  time.Time              `json:"created"`
	Modified time.Time              `json:"modified"`
}

// User represents a system user.
type User struct {
	ID             string    `json:"id"`
	Email          string    `json:"email"`
	PasswordHash   string    `json:"-"` // Never export password hash
	Name           string    `json:"name"`
	OrganizationID string    `json:"organization_id"`
	Role           UserRole  `json:"role"`
	Created        time.Time `json:"created"`
	Modified       time.Time `json:"modified"`
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
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Created time.Time `json:"created"`
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
