package dto

// PropertyType represents the type of a table property.
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

// Property represents a table column with its configuration.
type Property struct {
	Name     string       `json:"name"`
	Type     PropertyType `json:"type"`
	Required bool         `json:"required,omitempty"`

	// Options contains the allowed values for select and multi_select properties.
	// Each option has an ID (used in storage), name (display), and optional color.
	Options []SelectOption `json:"options,omitempty"`
}

// OrganizationRole defines the role of a user within an organization.
type OrganizationRole string

const (
	// OrgRoleOwner has full control including billing.
	OrgRoleOwner OrganizationRole = "owner"
	// OrgRoleAdmin can manage workspaces and members.
	OrgRoleAdmin OrganizationRole = "admin"
	// OrgRoleMember can only access granted workspaces.
	OrgRoleMember OrganizationRole = "member"
)

// WorkspaceRole defines the permissions for a user within a workspace.
type WorkspaceRole string

const (
	// WSRoleAdmin has full workspace control.
	WSRoleAdmin WorkspaceRole = "admin"
	// WSRoleEditor can create and modify content.
	WSRoleEditor WorkspaceRole = "editor"
	// WSRoleViewer can only read content.
	WSRoleViewer WorkspaceRole = "viewer"
)

// NodeType defines what features are enabled for a node.
type NodeType string

const (
	// NodeTypeDocument represents a markdown document.
	NodeTypeDocument NodeType = "document"
	// NodeTypeTable represents a structured table.
	NodeTypeTable NodeType = "table"
	// NodeTypeHybrid represents an entity that is both a document and a table.
	NodeTypeHybrid NodeType = "hybrid"
)

// UserSettings represents global user preferences.
type UserSettings struct {
	Theme    string `json:"theme" jsonschema:"description=UI theme preference (light/dark/system)"`
	Language string `json:"language" jsonschema:"description=Preferred language code (en/fr/etc)"`
}

// OAuthProvider represents a supported OAuth2 provider.
type OAuthProvider string

const (
	// OAuthProviderGoogle represents Google OAuth.
	OAuthProviderGoogle OAuthProvider = "google"
	// OAuthProviderMicrosoft represents Microsoft OAuth.
	OAuthProviderMicrosoft OAuthProvider = "microsoft"
)

// OAuthIdentity represents a link between a local user and an OAuth2 provider.
type OAuthIdentity struct {
	Provider   OAuthProvider `json:"provider" jsonschema:"description=OAuth provider name (google/microsoft)"`
	ProviderID string        `json:"provider_id" jsonschema:"description=User ID at the OAuth provider"`
	Email      string        `json:"email" jsonschema:"description=Email address from OAuth provider"`
	AvatarURL  string        `json:"avatar_url,omitempty" jsonschema:"description=Profile picture URL from OAuth provider"`
	LastLogin  string        `json:"last_login" jsonschema:"description=Last login timestamp via this provider (RFC3339)"`
}

// WorkspaceMembershipSettings represents user preferences within a specific workspace.
type WorkspaceMembershipSettings struct {
	Notifications bool `json:"notifications" jsonschema:"description=Whether notifications are enabled"`
}

// OrganizationQuotas defines limits for an organization.
type OrganizationQuotas struct {
	MaxWorkspaces          int `json:"max_workspaces" jsonschema:"description=Maximum number of workspaces in this org"`
	MaxMembersPerOrg       int `json:"max_members_per_org" jsonschema:"description=Maximum members at org level"`
	MaxMembersPerWorkspace int `json:"max_members_per_workspace" jsonschema:"description=Maximum members per workspace"`
	MaxTotalStorageGB      int `json:"max_total_storage_gb" jsonschema:"description=Total storage across all workspaces in GB"`
}

// WorkspaceQuotas defines limits for a workspace.
type WorkspaceQuotas struct {
	MaxPages           int `json:"max_pages" jsonschema:"description=Maximum number of pages allowed"`
	MaxStorageMB       int `json:"max_storage_mb" jsonschema:"description=Maximum storage in megabytes"`
	MaxRecordsPerTable int `json:"max_records_per_table" jsonschema:"description=Maximum records per table"`
	MaxAssetSizeMB     int `json:"max_asset_size_mb" jsonschema:"description=Maximum size of a single asset in megabytes"`
}

// UserQuota defines limits for a user.
type UserQuota struct {
	MaxOrganizations int `json:"max_organizations" jsonschema:"description=Maximum number of organizations the user can create"`
}

// OrganizationSettings represents organization-wide settings.
type OrganizationSettings struct {
	AllowedEmailDomains    []string        `json:"allowed_email_domains,omitempty" jsonschema:"description=Restrict membership to these email domains"`
	RequireSSO             bool            `json:"require_sso" jsonschema:"description=Require SSO for all members"`
	DefaultWorkspaceQuotas WorkspaceQuotas `json:"default_workspace_quotas" jsonschema:"description=Default quotas for new workspaces"`
}

// WorkspaceSettings represents workspace-wide settings.
type WorkspaceSettings struct {
	AllowedDomains []string `json:"allowed_domains,omitempty" jsonschema:"description=Additional email domain restrictions"`
	PublicAccess   bool     `json:"public_access" jsonschema:"description=Whether content is publicly accessible"`
	GitAutoPush    bool     `json:"git_auto_push" jsonschema:"description=Automatically push changes to remote"`
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
