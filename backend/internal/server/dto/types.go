// Defines shared data types and enums for the API.

package dto

import (
	"errors"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/storage"
)

// Time is a type alias for storage.Time to ensure it generates as 'number' in TypeScript.
type Time = storage.Time

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
	OrgRoleOwner OrganizationRole = "org:owner"
	// OrgRoleAdmin can manage workspaces and members.
	OrgRoleAdmin OrganizationRole = "org:admin"
	// OrgRoleMember can only access granted workspaces.
	OrgRoleMember OrganizationRole = "org:member"
)

// WorkspaceRole defines the permissions for a user within a workspace.
type WorkspaceRole string

const (
	// WSRoleAdmin has full workspace control.
	WSRoleAdmin WorkspaceRole = "ws:admin"
	// WSRoleEditor can create and modify content.
	WSRoleEditor WorkspaceRole = "ws:editor"
	// WSRoleViewer can only read content.
	WSRoleViewer WorkspaceRole = "ws:viewer"
)

// UserSettings represents global user preferences.
type UserSettings struct {
	Theme                string   `json:"theme" jsonschema:"description=UI theme preference (light/dark/system)"`
	Language             string   `json:"language" jsonschema:"description=Preferred language code (en/fr/etc)"`
	LastActiveWorkspaces []string `json:"last_active_workspaces,omitempty" jsonschema:"description=Recently used workspace IDs in LRU order (most recent first)"`
}

// OAuthProvider represents a supported OAuth2 provider.
type OAuthProvider string

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
	Provider   OAuthProvider `json:"provider" jsonschema:"description=OAuth provider name (google/microsoft/github)"`
	ProviderID string        `json:"provider_id" jsonschema:"description=User ID at the OAuth provider"`
	Email      string        `json:"email" jsonschema:"description=Email address from OAuth provider"`
	AvatarURL  string        `json:"avatar_url,omitempty" jsonschema:"description=Profile picture URL from OAuth provider"`
	LastLogin  Time          `json:"last_login" jsonschema:"description=Last login Unix timestamp via this provider"`
}

// WorkspaceMembershipSettings represents user preferences within a specific workspace.
type WorkspaceMembershipSettings struct {
	Notifications bool `json:"notifications" jsonschema:"description=Whether notifications are enabled"`
}

// ResourceQuotas defines per-workspace content limits shared by server, org, and workspace layers.
// A zero value means "no limit at this layer" (inherit from other layers).
type ResourceQuotas struct {
	MaxPages              int   `json:"max_pages" jsonschema:"description=Maximum pages per workspace (0=no limit at this layer)"`
	MaxStorageBytes       int64 `json:"max_storage_bytes" jsonschema:"description=Maximum storage per workspace in bytes (0=no limit at this layer)"`
	MaxRecordsPerTable    int   `json:"max_records_per_table" jsonschema:"description=Maximum records per table (0=no limit at this layer)"`
	MaxAssetSizeBytes     int64 `json:"max_asset_size_bytes" jsonschema:"description=Maximum single asset file size in bytes (0=no limit at this layer)"`
	MaxTablesPerWorkspace int   `json:"max_tables_per_workspace" jsonschema:"description=Maximum tables per workspace (0=no limit at this layer)"`
	MaxColumnsPerTable    int   `json:"max_columns_per_table" jsonschema:"description=Maximum columns per table (0=no limit at this layer)"`
}

// Validate checks that all resource quota values are non-negative.
// The prefix is prepended to field names in error messages (e.g., "quotas").
func (q *ResourceQuotas) Validate(prefix string) error {
	if prefix != "" {
		prefix += "."
	}
	if q.MaxPages < 0 {
		return InvalidField(prefix+"max_pages", "must be non-negative")
	}
	if q.MaxStorageBytes < 0 {
		return InvalidField(prefix+"max_storage_bytes", "must be non-negative")
	}
	if q.MaxRecordsPerTable < 0 {
		return InvalidField(prefix+"max_records_per_table", "must be non-negative")
	}
	if q.MaxAssetSizeBytes < 0 {
		return InvalidField(prefix+"max_asset_size_bytes", "must be non-negative")
	}
	if q.MaxTablesPerWorkspace < 0 {
		return InvalidField(prefix+"max_tables_per_workspace", "must be non-negative")
	}
	if q.MaxColumnsPerTable < 0 {
		return InvalidField(prefix+"max_columns_per_table", "must be non-negative")
	}
	return nil
}

// OrganizationQuotas defines limits for an organization.
type OrganizationQuotas struct {
	ResourceQuotas `tstype:",extends"`

	MaxWorkspacesPerOrg    int   `json:"max_workspaces_per_org" jsonschema:"description=Maximum number of workspaces in this org"`
	MaxMembersPerOrg       int   `json:"max_members_per_org" jsonschema:"description=Maximum members at org level"`
	MaxMembersPerWorkspace int   `json:"max_members_per_workspace" jsonschema:"description=Maximum members per workspace"`
	MaxTotalStorageBytes   int64 `json:"max_total_storage_bytes" jsonschema:"description=Total storage across all workspaces in bytes"`
}

// WorkspaceQuotas is a type alias for ResourceQuotas.
// Zero values mean "inherit from server/org layer".
type WorkspaceQuotas = ResourceQuotas

// UserQuota defines limits for a user.
type UserQuota struct {
	MaxOrganizations int `json:"max_organizations" jsonschema:"description=Maximum number of organizations the user can create"`
}

// OrganizationSettings represents organization-wide settings.
type OrganizationSettings struct {
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
	Hash        string `json:"hash"`
	Message     string `json:"message"`
	AuthorName  string `json:"author_name"`
	AuthorEmail string `json:"author_email"`
	Timestamp   Time   `json:"timestamp"`
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
	Modified Time              `json:"modified"`
}

// --- View Types ---

// View represents a saved table view configuration.
type View struct {
	ID      ksid.ID      `json:"id" jsonschema:"description=Unique view identifier"`
	Name    string       `json:"name" jsonschema:"description=View display name"`
	Type    ViewType     `json:"type" jsonschema:"description=View layout type (table/board/gallery/list/calendar)"`
	Default bool         `json:"default,omitempty" jsonschema:"description=Whether this is the default view"`
	Columns []ViewColumn `json:"columns,omitempty" jsonschema:"description=Column visibility and ordering"`
	Filters []Filter     `json:"filters,omitempty" jsonschema:"description=Filter conditions"`
	Sorts   []Sort       `json:"sorts,omitempty" jsonschema:"description=Sort order"`
	Groups  []Group      `json:"groups,omitempty" jsonschema:"description=Grouping configuration"`
}

// ViewType defines the layout type for a view.
type ViewType string

const (
	// ViewTypeTable displays records in a spreadsheet-like table.
	ViewTypeTable ViewType = "table"
	// ViewTypeBoard displays records in a kanban board grouped by a property.
	ViewTypeBoard ViewType = "board"
	// ViewTypeGallery displays records as cards in a grid.
	ViewTypeGallery ViewType = "gallery"
	// ViewTypeList displays records in a simple list.
	ViewTypeList ViewType = "list"
	// ViewTypeCalendar displays records on a calendar by date property.
	ViewTypeCalendar ViewType = "calendar"
)

// Validate returns an error if the view type is not a known valid value.
func (v ViewType) Validate() error {
	switch v {
	case ViewTypeTable, ViewTypeBoard, ViewTypeGallery, ViewTypeList, ViewTypeCalendar:
		return nil
	}
	return errors.New("must be one of: table, board, gallery, list, calendar")
}

// ViewColumn defines the visibility and width of a property column.
type ViewColumn struct {
	Property string `json:"property" jsonschema:"description=Property name to display"`
	Width    int    `json:"width,omitempty" jsonschema:"description=Column width in pixels"`
	Visible  bool   `json:"visible" jsonschema:"description=Whether the column is visible"`
}

// Filter defines a condition for filtering records.
type Filter struct {
	Property string   `json:"property,omitempty" jsonschema:"description=Property name to filter on"`
	Operator FilterOp `json:"operator,omitempty" jsonschema:"description=Filter operator"`
	Value    any      `json:"value,omitempty" jsonschema:"description=Value to compare against"`
	And      []Filter `json:"and,omitempty" jsonschema:"description=All conditions must match (AND)"`
	Or       []Filter `json:"or,omitempty" jsonschema:"description=Any condition must match (OR)"`
}

// FilterOp defines the comparison operator for a filter.
type FilterOp string

const (
	// FilterOpEquals matches if value equals the filter value.
	FilterOpEquals FilterOp = "equals"
	// FilterOpNotEquals matches if value does not equal the filter value.
	FilterOpNotEquals FilterOp = "not_equals"
	// FilterOpContains matches if value contains the filter value (text).
	FilterOpContains FilterOp = "contains"
	// FilterOpNotContains matches if value does not contain the filter value.
	FilterOpNotContains FilterOp = "not_contains"
	// FilterOpStartsWith matches if value starts with the filter value.
	FilterOpStartsWith FilterOp = "starts_with"
	// FilterOpEndsWith matches if value ends with the filter value.
	FilterOpEndsWith FilterOp = "ends_with"
	// FilterOpGreaterThan matches if value is greater than the filter value.
	FilterOpGreaterThan FilterOp = "gt"
	// FilterOpLessThan matches if value is less than the filter value.
	FilterOpLessThan FilterOp = "lt"
	// FilterOpGreaterEqual matches if value is greater than or equal to the filter value.
	FilterOpGreaterEqual FilterOp = "gte"
	// FilterOpLessEqual matches if value is less than or equal to the filter value.
	FilterOpLessEqual FilterOp = "lte"
	// FilterOpIsEmpty matches if value is empty/null.
	FilterOpIsEmpty FilterOp = "is_empty"
	// FilterOpIsNotEmpty matches if value is not empty/null.
	FilterOpIsNotEmpty FilterOp = "is_not_empty"
)

// Validate returns an error if the filter operator is not a known valid value.
func (f FilterOp) Validate() error {
	switch f {
	case FilterOpEquals, FilterOpNotEquals, FilterOpContains, FilterOpNotContains,
		FilterOpStartsWith, FilterOpEndsWith, FilterOpGreaterThan, FilterOpLessThan,
		FilterOpGreaterEqual, FilterOpLessEqual, FilterOpIsEmpty, FilterOpIsNotEmpty:
		return nil
	}
	return errors.New("invalid operator: " + string(f))
}

// Sort defines the sort order for a property.
type Sort struct {
	Property  string  `json:"property" jsonschema:"description=Property name to sort by"`
	Direction SortDir `json:"direction" jsonschema:"description=Sort direction (asc/desc)"`
}

// SortDir defines the sort direction.
type SortDir string

const (
	// SortAsc sorts in ascending order (A-Z, 0-9, oldest-newest).
	SortAsc SortDir = "asc"
	// SortDesc sorts in descending order (Z-A, 9-0, newest-oldest).
	SortDesc SortDir = "desc"
)

// Group defines how to group records by a property.
type Group struct {
	Property string `json:"property" jsonschema:"description=Property name to group by"`
	Hidden   []any  `json:"hidden,omitempty" jsonschema:"description=Group values to hide"`
}

// --- Notification Types ---

// NotificationDTO is the API representation of a notification.
type NotificationDTO struct {
	ID         ksid.ID `json:"id" jsonschema:"description=Unique notification identifier"`
	Type       string  `json:"type" jsonschema:"description=Notification type (org_invite, ws_invite, member_joined, member_removed, page_mention, page_edited)"`
	Title      string  `json:"title" jsonschema:"description=Human-readable summary"`
	Body       string  `json:"body,omitempty" jsonschema:"description=Optional detail text"`
	ResourceID string  `json:"resource_id,omitempty" jsonschema:"description=Related resource identifier"`
	ActorID    ksid.ID `json:"actor_id,omitempty" jsonschema:"description=User who triggered the notification"`
	ActorName  string  `json:"actor_name,omitempty" jsonschema:"description=Display name of actor"`
	Read       bool    `json:"read" jsonschema:"description=Whether the notification has been read"`
	CreatedAt  Time    `json:"created_at" jsonschema:"description=Creation timestamp"`
}

// ChannelSetDTO indicates which delivery channels are enabled.
type ChannelSetDTO struct {
	Email bool `json:"email" jsonschema:"description=Email delivery enabled"`
	Web   bool `json:"web" jsonschema:"description=Web push delivery enabled"`
}

// NotificationPrefsDTO holds user notification preferences.
type NotificationPrefsDTO struct {
	Defaults  map[string]ChannelSetDTO `json:"defaults" jsonschema:"description=Default channels per notification type"`
	Overrides map[string]ChannelSetDTO `json:"overrides" jsonschema:"description=User overrides per notification type"`
}
