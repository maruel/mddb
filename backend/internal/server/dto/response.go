package dto

import "github.com/maruel/mddb/backend/internal/jsonldb"

// --- Common Responses ---

// OkResponse is a simple success response.
type OkResponse struct {
	Ok bool `json:"ok"`
}

// --- Auth Responses ---

// AuthResponse is a response containing authentication token and user info.
// Used by login, register, and invitation acceptance endpoints.
type AuthResponse struct {
	Token string        `json:"token"`
	User  *UserResponse `json:"user"`
}

// ProvidersResponse is a response containing the list of configured OAuth providers.
type ProvidersResponse struct {
	Providers []OAuthProvider `json:"providers"`
}

// --- Session Responses ---

// SessionResponse is the API representation of a session.
type SessionResponse struct {
	ID         jsonldb.ID `json:"id" jsonschema:"description=Unique session identifier"`
	DeviceInfo string     `json:"device_info" jsonschema:"description=Browser/OS info"`
	IPAddress  string     `json:"ip_address" jsonschema:"description=IP address"`
	Created    string     `json:"created" jsonschema:"description=Session creation timestamp (RFC3339)"`
	LastUsed   string     `json:"last_used" jsonschema:"description=Last activity timestamp (RFC3339)"`
	IsCurrent  bool       `json:"is_current" jsonschema:"description=Whether this is the current session"`
}

// ListSessionsResponse is a response containing user's sessions.
type ListSessionsResponse struct {
	Sessions []SessionResponse `json:"sessions"`
}

// LogoutResponse is a response from logout.
type LogoutResponse = OkResponse

// RevokeSessionResponse is a response from revoking a session.
type RevokeSessionResponse = OkResponse

// RevokeAllSessionsResponse is a response from revoking all sessions.
type RevokeAllSessionsResponse struct {
	RevokedCount int `json:"revoked_count" jsonschema:"description=Number of sessions revoked"`
}

// --- Page Responses ---

// ListPagesResponse is a response containing a list of pages.
type ListPagesResponse struct {
	Pages []PageSummary `json:"pages"`
}

// PageSummary is a brief representation of a page for list responses.
type PageSummary struct {
	ID       jsonldb.ID `json:"id"`
	Title    string     `json:"title"`
	Created  string     `json:"created"`
	Modified string     `json:"modified"`
}

// GetPageResponse is a response containing a page.
type GetPageResponse struct {
	ID      jsonldb.ID `json:"id"`
	Title   string     `json:"title"`
	Content string     `json:"content"`
}

// CreatePageResponse is a response from creating a page.
type CreatePageResponse struct {
	ID jsonldb.ID `json:"id"`
}

// UpdatePageResponse is a response from updating a page.
type UpdatePageResponse struct {
	ID jsonldb.ID `json:"id"`
}

// DeletePageResponse is a response from deleting a page.
type DeletePageResponse = OkResponse

// ListPageVersionsResponse is a response containing page version history.
type ListPageVersionsResponse struct {
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
	ID       jsonldb.ID `json:"id"`
	Title    string     `json:"title"`
	Created  string     `json:"created"`
	Modified string     `json:"modified"`
}

// GetTableResponse is a response containing a table.
type GetTableResponse struct {
	ID         jsonldb.ID `json:"id"`
	Title      string     `json:"title"`
	Properties []Property `json:"properties"`
	Created    string     `json:"created"`
	Modified   string     `json:"modified"`
}

// CreateTableResponse is a response from creating a table.
type CreateTableResponse struct {
	ID jsonldb.ID `json:"id"`
}

// UpdateTableResponse is a response from updating a table.
type UpdateTableResponse struct {
	ID jsonldb.ID `json:"id"`
}

// DeleteTableResponse is a response from deleting a table.
type DeleteTableResponse = OkResponse

// ListRecordsResponse is a response containing a list of records.
type ListRecordsResponse struct {
	Records []DataRecordResponse `json:"records"`
}

// CreateRecordResponse is a response from creating a record.
type CreateRecordResponse struct {
	ID jsonldb.ID `json:"id"`
}

// UpdateRecordResponse is a response from updating a record.
type UpdateRecordResponse struct {
	ID jsonldb.ID `json:"id"`
}

// GetRecordResponse is a response containing a record.
type GetRecordResponse struct {
	ID       jsonldb.ID     `json:"id"`
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

// ListOrgInvitationsResponse is a response containing a list of organization invitations.
type ListOrgInvitationsResponse struct {
	Invitations []OrgInvitationResponse `json:"invitations"`
}

// ListWSInvitationsResponse is a response containing a list of workspace invitations.
type ListWSInvitationsResponse struct {
	Invitations []WSInvitationResponse `json:"invitations"`
}

// --- Membership Responses ---

// SwitchOrgResponse is a response from switching organization.
type SwitchOrgResponse struct {
	Token string        `json:"token"`
	User  *UserResponse `json:"user"`
}

// SwitchWorkspaceResponse is a response from switching workspace.
type SwitchWorkspaceResponse struct {
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
	ID              jsonldb.ID      `json:"id" jsonschema:"description=Unique user identifier"`
	Email           string          `json:"email" jsonschema:"description=User email address"`
	Name            string          `json:"name" jsonschema:"description=User display name"`
	IsGlobalAdmin   bool            `json:"is_global_admin,omitempty" jsonschema:"description=Whether user has server-wide administrative access"`
	OAuthIdentities []OAuthIdentity `json:"oauth_identities,omitempty" jsonschema:"description=Linked OAuth provider accounts"`
	Settings        UserSettings    `json:"settings" jsonschema:"description=Global user preferences"`
	Created         string          `json:"created" jsonschema:"description=Account creation timestamp (RFC3339)"`
	Modified        string          `json:"modified" jsonschema:"description=Last modification timestamp (RFC3339)"`

	// Current context
	OrganizationID jsonldb.ID       `json:"organization_id,omitempty" jsonschema:"description=Active organization ID"`
	OrgRole        OrganizationRole `json:"org_role,omitempty" jsonschema:"description=Role in active organization"`
	WorkspaceID    jsonldb.ID       `json:"workspace_id,omitempty" jsonschema:"description=Active workspace ID"`
	WorkspaceRole  WorkspaceRole    `json:"workspace_role,omitempty" jsonschema:"description=Role in active workspace"`

	// All memberships
	Organizations []OrgMembershipResponse `json:"organizations,omitempty" jsonschema:"description=Organization memberships"`
	Workspaces    []WSMembershipResponse  `json:"workspaces,omitempty" jsonschema:"description=Workspace memberships"`
}

// OrgMembershipResponse is the API representation of an organization membership.
type OrgMembershipResponse struct {
	ID               jsonldb.ID       `json:"id" jsonschema:"description=Unique membership identifier"`
	UserID           jsonldb.ID       `json:"user_id" jsonschema:"description=User ID this membership belongs to"`
	OrganizationID   jsonldb.ID       `json:"organization_id" jsonschema:"description=Organization ID the user is a member of"`
	OrganizationName string           `json:"organization_name,omitempty" jsonschema:"description=Organization name"`
	Role             OrganizationRole `json:"role" jsonschema:"description=User role within the organization"`
	Created          string           `json:"created" jsonschema:"description=Membership creation timestamp (RFC3339)"`
}

// WSMembershipResponse is the API representation of a workspace membership.
type WSMembershipResponse struct {
	ID             jsonldb.ID                  `json:"id" jsonschema:"description=Unique membership identifier"`
	UserID         jsonldb.ID                  `json:"user_id" jsonschema:"description=User ID this membership belongs to"`
	WorkspaceID    jsonldb.ID                  `json:"workspace_id" jsonschema:"description=Workspace ID the user is a member of"`
	WorkspaceName  string                      `json:"workspace_name,omitempty" jsonschema:"description=Workspace name"`
	OrganizationID jsonldb.ID                  `json:"organization_id" jsonschema:"description=Parent organization ID"`
	Role           WorkspaceRole               `json:"role" jsonschema:"description=User role within the workspace"`
	Settings       WorkspaceMembershipSettings `json:"settings" jsonschema:"description=User preferences within this workspace"`
	Created        string                      `json:"created" jsonschema:"description=Membership creation timestamp (RFC3339)"`
}

// OrgInvitationResponse is the API representation of an organization invitation (excludes Token).
type OrgInvitationResponse struct {
	ID             jsonldb.ID       `json:"id" jsonschema:"description=Unique invitation identifier"`
	Email          string           `json:"email" jsonschema:"description=Email address of the invitee"`
	OrganizationID jsonldb.ID       `json:"organization_id" jsonschema:"description=Organization the user is invited to"`
	Role           OrganizationRole `json:"role" jsonschema:"description=Role assigned upon acceptance"`
	InvitedBy      jsonldb.ID       `json:"invited_by" jsonschema:"description=User ID who created the invitation"`
	ExpiresAt      string           `json:"expires_at" jsonschema:"description=Invitation expiration timestamp (RFC3339)"`
	Created        string           `json:"created" jsonschema:"description=Invitation creation timestamp (RFC3339)"`
}

// WSInvitationResponse is the API representation of a workspace invitation (excludes Token).
type WSInvitationResponse struct {
	ID          jsonldb.ID    `json:"id" jsonschema:"description=Unique invitation identifier"`
	Email       string        `json:"email" jsonschema:"description=Email address of the invitee"`
	WorkspaceID jsonldb.ID    `json:"workspace_id" jsonschema:"description=Workspace the user is invited to"`
	Role        WorkspaceRole `json:"role" jsonschema:"description=Role assigned upon acceptance"`
	InvitedBy   jsonldb.ID    `json:"invited_by" jsonschema:"description=User ID who created the invitation"`
	ExpiresAt   string        `json:"expires_at" jsonschema:"description=Invitation expiration timestamp (RFC3339)"`
	Created     string        `json:"created" jsonschema:"description=Invitation creation timestamp (RFC3339)"`
}

// OrganizationResponse is the API representation of an organization.
type OrganizationResponse struct {
	ID             jsonldb.ID           `json:"id" jsonschema:"description=Unique organization identifier"`
	Name           string               `json:"name" jsonschema:"description=Display name of the organization"`
	BillingEmail   string               `json:"billing_email,omitempty" jsonschema:"description=Primary billing contact (owner only)"`
	Quotas         OrganizationQuotas   `json:"quotas" jsonschema:"description=Resource limits for the organization"`
	Settings       OrganizationSettings `json:"settings" jsonschema:"description=Organization-wide configuration"`
	MemberCount    int                  `json:"member_count" jsonschema:"description=Number of members"`
	WorkspaceCount int                  `json:"workspace_count" jsonschema:"description=Number of workspaces"`
	Created        string               `json:"created" jsonschema:"description=Organization creation timestamp (RFC3339)"`
}

// WorkspaceResponse is the API representation of a workspace.
type WorkspaceResponse struct {
	ID             jsonldb.ID         `json:"id" jsonschema:"description=Unique workspace identifier"`
	OrganizationID jsonldb.ID         `json:"organization_id" jsonschema:"description=Parent organization ID"`
	Name           string             `json:"name" jsonschema:"description=Display name of the workspace"`
	Slug           string             `json:"slug" jsonschema:"description=URL-friendly identifier"`
	Quotas         WorkspaceQuotas    `json:"quotas" jsonschema:"description=Resource limits for the workspace"`
	Settings       WorkspaceSettings  `json:"settings" jsonschema:"description=Workspace-wide configuration"`
	GitRemote      *GitRemoteResponse `json:"git_remote,omitempty" jsonschema:"description=Git remote configuration"`
	MemberCount    int                `json:"member_count" jsonschema:"description=Number of members"`
	Created        string             `json:"created" jsonschema:"description=Workspace creation timestamp (RFC3339)"`
}

// GitRemoteResponse is the API representation of a git remote.
type GitRemoteResponse struct {
	WorkspaceID jsonldb.ID `json:"workspace_id" jsonschema:"description=Workspace this remote belongs to"`
	URL         string     `json:"url" jsonschema:"description=Git repository URL"`
	Type        string     `json:"type" jsonschema:"description=Remote type (github/gitlab/custom)"`
	AuthType    string     `json:"auth_type" jsonschema:"description=Authentication method (token/ssh)"`
	Created     string     `json:"created" jsonschema:"description=Remote creation timestamp (RFC3339)"`
	LastSync    string     `json:"last_sync,omitempty" jsonschema:"description=Last synchronization timestamp (RFC3339)"`
}

// NodeResponse is the API representation of a node.
type NodeResponse struct {
	ID         jsonldb.ID     `json:"id" jsonschema:"description=Unique node identifier"`
	ParentID   jsonldb.ID     `json:"parent_id,omitempty" jsonschema:"description=Parent node ID for hierarchical structure"`
	Title      string         `json:"title" jsonschema:"description=Node title"`
	Content    string         `json:"content,omitempty" jsonschema:"description=Markdown content (Page part)"`
	Properties []Property     `json:"properties,omitempty" jsonschema:"description=Schema (Table part)"`
	Created    string         `json:"created" jsonschema:"description=Node creation timestamp (RFC3339)"`
	Modified   string         `json:"modified" jsonschema:"description=Last modification timestamp (RFC3339)"`
	Tags       []string       `json:"tags,omitempty" jsonschema:"description=Node tags"`
	FaviconURL string         `json:"favicon_url,omitempty" jsonschema:"description=Favicon URL"`
	Type       NodeType       `json:"type" jsonschema:"description=Node type (document/table/hybrid)"`
	Children   []NodeResponse `json:"children,omitempty" jsonschema:"description=Nested nodes"`
}

// DataRecordResponse is the API representation of a data record.
type DataRecordResponse struct {
	ID       jsonldb.ID     `json:"id" jsonschema:"description=Unique record identifier"`
	Data     map[string]any `json:"data" jsonschema:"description=Record field values keyed by property name"`
	Created  string         `json:"created" jsonschema:"description=Record creation timestamp (RFC3339)"`
	Modified string         `json:"modified" jsonschema:"description=Last modification timestamp (RFC3339)"`
}

// --- Global Admin Responses ---

// AdminStatsResponse contains server-wide statistics.
type AdminStatsResponse struct {
	UserCount      int `json:"user_count" jsonschema:"description=Total number of users"`
	OrgCount       int `json:"org_count" jsonschema:"description=Total number of organizations"`
	WorkspaceCount int `json:"workspace_count" jsonschema:"description=Total number of workspaces"`
}

// AdminUsersResponse contains all users in the system.
type AdminUsersResponse struct {
	Users []UserResponse `json:"users" jsonschema:"description=All users in the system"`
}

// AdminOrgsResponse contains all organizations in the system.
type AdminOrgsResponse struct {
	Organizations []OrganizationResponse `json:"organizations" jsonschema:"description=All organizations in the system"`
}

// AdminWorkspacesResponse contains all workspaces in the system.
type AdminWorkspacesResponse struct {
	Workspaces []WorkspaceResponse `json:"workspaces" jsonschema:"description=All workspaces in the system"`
}

// --- List Workspaces Response ---

// ListWorkspacesResponse is a response containing a list of workspaces.
type ListWorkspacesResponse struct {
	Workspaces []WorkspaceResponse `json:"workspaces"`
}
