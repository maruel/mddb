package dto

// --- Common Responses ---

// OkResponse is a simple success response.
type OkResponse struct {
	Ok bool `json:"ok"`
}

// --- Auth Responses ---

// LoginResponse is a response from logging in.
type LoginResponse struct {
	Token string        `json:"token"`
	User  *UserResponse `json:"user"`
}

// --- Page Responses ---

// ListPagesResponse is a response containing a list of pages.
type ListPagesResponse struct {
	Pages []PageSummary `json:"pages"`
}

// PageSummary is a brief representation of a page for list responses.
type PageSummary struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Created  string `json:"created"`
	Modified string `json:"modified"`
}

// GetPageResponse is a response containing a page.
type GetPageResponse struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

// CreatePageResponse is a response from creating a page.
type CreatePageResponse struct {
	ID string `json:"id"`
}

// UpdatePageResponse is a response from updating a page.
type UpdatePageResponse struct {
	ID string `json:"id"`
}

// DeletePageResponse is a response from deleting a page.
type DeletePageResponse = OkResponse

// GetPageHistoryResponse is a response containing page history.
type GetPageHistoryResponse struct {
	History []*Commit `json:"history"`
}

// GetPageVersionResponse is a response containing page content at a version.
type GetPageVersionResponse struct {
	Content string `json:"content"`
}

// --- Table Responses ---

// ListTablesResponse is a response containing a list of tables.
type ListTablesResponse struct {
	Tables []TableSummary `json:"tables"`
}

// TableSummary is a brief representation of a table for list responses.
type TableSummary struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Created  string `json:"created"`
	Modified string `json:"modified"`
}

// GetTableResponse is a response containing a table.
type GetTableResponse struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	Properties []Property `json:"properties"`
	Created    string     `json:"created"`
	Modified   string     `json:"modified"`
}

// CreateTableResponse is a response from creating a table.
type CreateTableResponse struct {
	ID string `json:"id"`
}

// UpdateTableResponse is a response from updating a table.
type UpdateTableResponse struct {
	ID string `json:"id"`
}

// DeleteTableResponse is a response from deleting a table.
type DeleteTableResponse = OkResponse

// ListRecordsResponse is a response containing a list of records.
type ListRecordsResponse struct {
	Records []DataRecordResponse `json:"records"`
}

// CreateRecordResponse is a response from creating a record.
type CreateRecordResponse struct {
	ID string `json:"id"`
}

// UpdateRecordResponse is a response from updating a record.
type UpdateRecordResponse struct {
	ID string `json:"id"`
}

// GetRecordResponse is a response containing a record.
type GetRecordResponse struct {
	ID       string         `json:"id"`
	Data     map[string]any `json:"data"`
	Created  string         `json:"created"`
	Modified string         `json:"modified"`
}

// DeleteRecordResponse is a response from deleting a record.
type DeleteRecordResponse = OkResponse

// --- Node Responses ---

// ListNodesResponse is a response containing a list of nodes.
type ListNodesResponse struct {
	Nodes []NodeResponse `json:"nodes"`
}

// --- Asset Responses ---

// ListPageAssetsResponse is a response containing a list of assets.
type ListPageAssetsResponse struct {
	Assets []AssetSummary `json:"assets"`
}

// AssetSummary is a brief representation of an asset for list responses.
type AssetSummary struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	MimeType string `json:"mime_type"`
	Created  string `json:"created"`
	URL      string `json:"url"`
}

// UploadPageAssetResponse is a response from uploading an asset.
type UploadPageAssetResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	MimeType string `json:"mime_type"`
}

// DeletePageAssetResponse is a response from deleting an asset.
type DeletePageAssetResponse = OkResponse

// ServeAssetResponse wraps the binary asset data.
type ServeAssetResponse struct {
	Data     string `json:"data"`
	MimeType string `json:"mime_type"`
}

// --- Search Responses ---

// SearchResponse is the response to a search request.
type SearchResponse struct {
	Results []SearchResult `json:"results"`
}

// --- Invitation Responses ---

// ListInvitationsResponse is a response containing a list of invitations.
type ListInvitationsResponse struct {
	Invitations []InvitationResponse `json:"invitations"`
}

// --- Membership Responses ---

// SwitchOrgResponse is a response from switching organization.
type SwitchOrgResponse struct {
	Token string        `json:"token"`
	User  *UserResponse `json:"user"`
}

// --- Git Remote Responses ---

// --- Health Responses ---

// HealthResponse is a response from a health check.
type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// --- User Responses ---

// ListUsersResponse is a response containing a list of users.
type ListUsersResponse struct {
	Users []UserResponse `json:"users"`
}

// --- API Response Types ---

// UserResponse is the API representation of a user.
type UserResponse struct {
	ID              string               `json:"id" jsonschema:"description=Unique user identifier"`
	Email           string               `json:"email" jsonschema:"description=User email address"`
	Name            string               `json:"name" jsonschema:"description=User display name"`
	IsGlobalAdmin   bool                 `json:"is_global_admin,omitempty" jsonschema:"description=Whether user has server-wide administrative access"`
	OAuthIdentities []OAuthIdentity      `json:"oauth_identities,omitempty" jsonschema:"description=Linked OAuth provider accounts"`
	Settings        UserSettings         `json:"settings" jsonschema:"description=Global user preferences"`
	Created         string               `json:"created" jsonschema:"description=Account creation timestamp (RFC3339)"`
	Modified        string               `json:"modified" jsonschema:"description=Last modification timestamp (RFC3339)"`
	Memberships     []MembershipResponse `json:"memberships,omitempty" jsonschema:"description=Organization memberships"`
	OrganizationID  string               `json:"organization_id,omitempty" jsonschema:"description=Current organization context"`
	Role            UserRole             `json:"role,omitempty" jsonschema:"description=Role in current organization"`
	Onboarding      *OnboardingState     `json:"onboarding,omitempty" jsonschema:"description=Onboarding state for current org"`
}

// MembershipResponse is the API representation of a membership.
type MembershipResponse struct {
	ID               string             `json:"id" jsonschema:"description=Unique membership identifier"`
	UserID           string             `json:"user_id" jsonschema:"description=User ID this membership belongs to"`
	OrganizationID   string             `json:"organization_id" jsonschema:"description=Organization ID the user is a member of"`
	OrganizationName string             `json:"organization_name,omitempty" jsonschema:"description=Organization name"`
	Role             UserRole           `json:"role" jsonschema:"description=User role within the organization"`
	Settings         MembershipSettings `json:"settings" jsonschema:"description=User preferences within this organization"`
	Created          string             `json:"created" jsonschema:"description=Membership creation timestamp (RFC3339)"`
}

// InvitationResponse is the API representation of an invitation (excludes Token).
type InvitationResponse struct {
	ID             string   `json:"id" jsonschema:"description=Unique invitation identifier"`
	Email          string   `json:"email" jsonschema:"description=Email address of the invitee"`
	OrganizationID string   `json:"organization_id" jsonschema:"description=Organization the user is invited to"`
	Role           UserRole `json:"role" jsonschema:"description=Role assigned upon acceptance"`
	ExpiresAt      string   `json:"expires_at" jsonschema:"description=Invitation expiration timestamp (RFC3339)"`
	Created        string   `json:"created" jsonschema:"description=Invitation creation timestamp (RFC3339)"`
	// Token intentionally excluded for security
}

// OrganizationResponse is the API representation of an organization.
type OrganizationResponse struct {
	ID         string               `json:"id" jsonschema:"description=Unique organization identifier"`
	Name       string               `json:"name" jsonschema:"description=Display name of the organization"`
	Quotas     OrganizationQuota    `json:"quotas" jsonschema:"description=Resource limits for the organization"`
	Settings   OrganizationSettings `json:"settings" jsonschema:"description=Organization-wide configuration"`
	Onboarding OnboardingState      `json:"onboarding" jsonschema:"description=Initial setup progress tracking"`
	Created    string               `json:"created" jsonschema:"description=Organization creation timestamp (RFC3339)"`
}

// GitRemoteResponse is the API representation of a git remote.
// Each organization has at most one remote, identified by OrganizationID.
type GitRemoteResponse struct {
	OrganizationID string `json:"organization_id" jsonschema:"description=Organization this remote belongs to"`
	URL            string `json:"url" jsonschema:"description=Git repository URL"`
	Type           string `json:"type" jsonschema:"description=Remote type (github/gitlab/custom)"`
	AuthType       string `json:"auth_type" jsonschema:"description=Authentication method (token/ssh)"`
	Created        string `json:"created" jsonschema:"description=Remote creation timestamp (RFC3339)"`
	LastSync       string `json:"last_sync,omitempty" jsonschema:"description=Last synchronization timestamp (RFC3339)"`
}

// NodeResponse is the API representation of a node.
type NodeResponse struct {
	ID         string         `json:"id" jsonschema:"description=Unique node identifier"`
	ParentID   string         `json:"parent_id,omitempty" jsonschema:"description=Parent node ID for hierarchical structure"`
	Title      string         `json:"title" jsonschema:"description=Node title"`
	Content    string         `json:"content,omitempty" jsonschema:"description=Markdown content (Page part)"`
	Properties []Property     `json:"properties,omitempty" jsonschema:"description=Schema (Database part)"`
	Created    string         `json:"created" jsonschema:"description=Node creation timestamp (RFC3339)"`
	Modified   string         `json:"modified" jsonschema:"description=Last modification timestamp (RFC3339)"`
	Tags       []string       `json:"tags,omitempty" jsonschema:"description=Node tags"`
	FaviconURL string         `json:"favicon_url,omitempty" jsonschema:"description=Favicon URL"`
	Type       NodeType       `json:"type" jsonschema:"description=Node type (document/database/hybrid)"`
	Children   []NodeResponse `json:"children,omitempty" jsonschema:"description=Nested nodes"`
}

// DataRecordResponse is the API representation of a data record.
type DataRecordResponse struct {
	ID       string         `json:"id" jsonschema:"description=Unique record identifier"`
	Data     map[string]any `json:"data" jsonschema:"description=Record field values keyed by property name"`
	Created  string         `json:"created" jsonschema:"description=Record creation timestamp (RFC3339)"`
	Modified string         `json:"modified" jsonschema:"description=Last modification timestamp (RFC3339)"`
}

// --- Global Admin Responses ---

// AdminStatsResponse contains server-wide statistics.
type AdminStatsResponse struct {
	UserCount int `json:"user_count" jsonschema:"description=Total number of users"`
	OrgCount  int `json:"org_count" jsonschema:"description=Total number of organizations"`
}

// AdminUsersResponse contains all users in the system.
type AdminUsersResponse struct {
	Users []UserResponse `json:"users" jsonschema:"description=All users in the system"`
}

// AdminOrgsResponse contains all organizations in the system.
type AdminOrgsResponse struct {
	Organizations []OrganizationResponse `json:"organizations" jsonschema:"description=All organizations in the system"`
}
