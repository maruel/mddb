package dto

// PropertyType represents the type of a database property.
type PropertyType string

const (
	// Primitive types.

	// PropertyTypeText stores plain text values.
	PropertyTypeText PropertyType = "text"
	// PropertyTypeNumber stores numeric values (integer or float).
	PropertyTypeNumber PropertyType = "number"
	// PropertyTypeCheckbox stores boolean values.
	PropertyTypeCheckbox PropertyType = "checkbox"
	// PropertyTypeDate stores ISO8601 date strings.
	PropertyTypeDate PropertyType = "date"

	// Enumerated types (with predefined options).

	// PropertyTypeSelect stores a single selection from predefined options.
	PropertyTypeSelect PropertyType = "select"
	// PropertyTypeMultiSelect stores multiple selections from predefined options.
	PropertyTypeMultiSelect PropertyType = "multi_select"

	// Validated text types.

	// PropertyTypeURL stores URLs with validation.
	PropertyTypeURL PropertyType = "url"
	// PropertyTypeEmail stores email addresses with validation.
	PropertyTypeEmail PropertyType = "email"
	// PropertyTypePhone stores phone numbers with validation.
	PropertyTypePhone PropertyType = "phone"
)

// SelectOption represents an option for select/multi_select properties.
type SelectOption struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

// Property represents a database property (column) with its configuration.
type Property struct {
	Name     string       `json:"name"`
	Type     PropertyType `json:"type"`
	Required bool         `json:"required,omitempty"`

	// Options contains the allowed values for select and multi_select properties.
	// Each option has an ID (used in storage), name (display), and optional color.
	Options []SelectOption `json:"options,omitempty"`
}

// UserRole defines the permissions for a user.
type UserRole string

const (
	// UserRoleAdmin has full access to all resources and settings within an organization.
	UserRoleAdmin UserRole = "admin"
	// UserRoleEditor can create and modify content but cannot manage users.
	UserRoleEditor UserRole = "editor"
	// UserRoleViewer can only read content.
	UserRoleViewer UserRole = "viewer"
)

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

// UserSettings represents global user preferences.
type UserSettings struct {
	Theme    string `json:"theme" jsonschema:"description=UI theme preference (light/dark/system)"`
	Language string `json:"language" jsonschema:"description=Preferred language code (en/fr/etc)"`
}

// OAuthIdentity represents a link between a local user and an OAuth2 provider.
type OAuthIdentity struct {
	Provider   string `json:"provider" jsonschema:"description=OAuth provider name (google/microsoft)"`
	ProviderID string `json:"provider_id" jsonschema:"description=User ID at the OAuth provider"`
	Email      string `json:"email" jsonschema:"description=Email address from OAuth provider"`
	LastLogin  string `json:"last_login" jsonschema:"description=Last login timestamp via this provider (RFC3339)"`
}

// MembershipSettings represents user preferences within a specific organization.
type MembershipSettings struct {
	Notifications bool `json:"notifications" jsonschema:"description=Whether email notifications are enabled"`
}

// Quota defines limits for an organization.
type Quota struct {
	MaxPages   int   `json:"max_pages" jsonschema:"description=Maximum number of pages allowed"`
	MaxStorage int64 `json:"max_storage" jsonschema:"description=Maximum storage in bytes"`
	MaxUsers   int   `json:"max_users" jsonschema:"description=Maximum number of users allowed"`
}

// OrganizationSettings represents organization-wide settings.
type OrganizationSettings struct {
	AllowedDomains []string `json:"allowed_domains,omitempty" jsonschema:"description=Email domains allowed for membership"`
	PublicAccess   bool     `json:"public_access" jsonschema:"description=Whether content is publicly accessible"`
	GitAutoPush    bool     `json:"git_auto_push" jsonschema:"description=Automatically push changes to remote"`
}

// OnboardingState tracks the progress of an organization's initial setup.
type OnboardingState struct {
	Completed bool   `json:"completed" jsonschema:"description=Whether onboarding is complete"`
	Step      string `json:"step" jsonschema:"description=Current onboarding step (name/members/git/done)"`
	UpdatedAt string `json:"updated_at" jsonschema:"description=Last progress update timestamp (RFC3339)"`
}

// Commit represents a commit in git history.
type Commit struct {
	Hash      string `json:"hash"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

// SearchResult represents a single search result.
type SearchResult struct {
	Type     string            `json:"type"` // "page" or "record"
	NodeID   string            `json:"node_id"`
	RecordID string            `json:"record_id,omitempty"`
	Title    string            `json:"title"`
	Snippet  string            `json:"snippet"`
	Score    float64           `json:"score"`
	Matches  map[string]string `json:"matches"`
	Modified string            `json:"modified"`
}
