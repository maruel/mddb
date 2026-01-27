// Defines API response payloads.

package dto

import (
	"github.com/maruel/mddb/backend/internal/jsonldb"
)

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
	Created    Time       `json:"created" jsonschema:"description=Session creation Unix timestamp"`
	LastUsed   Time       `json:"last_used" jsonschema:"description=Last activity Unix timestamp"`
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

// --- Node Content Responses ---

// UpdateNodeResponse is a response from updating a node.
type UpdateNodeResponse struct {
	ID jsonldb.ID `json:"id"`
}

// DeleteNodeResponse is a response from deleting a node.
type DeleteNodeResponse = OkResponse

// ListNodeVersionsResponse is a response containing node version history.
type ListNodeVersionsResponse struct {
	History []*Commit `json:"history"`
}

// GetNodeVersionResponse is a response containing node content at a version.
type GetNodeVersionResponse struct {
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
	Created  Time       `json:"created"`
	Modified Time       `json:"modified"`
}

// GetTableResponse is a response containing a table.
type GetTableResponse struct {
	ID         jsonldb.ID `json:"id"`
	Title      string     `json:"title"`
	Properties []Property `json:"properties"`
	Created    Time       `json:"created"`
	Modified   Time       `json:"modified"`
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
	Created  Time           `json:"created"`
	Modified Time           `json:"modified"`
}

// DeleteRecordResponse is a response from deleting a record.
type DeleteRecordResponse = OkResponse

// --- Node Responses ---

// ListNodesResponse is a response containing a list of nodes.
type ListNodesResponse struct {
	Nodes []NodeResponse `json:"nodes"`
}

// --- Asset Responses ---

// ListNodeAssetsResponse is a response containing a list of assets.
type ListNodeAssetsResponse struct {
	Assets []AssetSummary `json:"assets"`
}

// AssetSummary is a brief representation of an asset for list responses.
type AssetSummary struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	MimeType string `json:"mime_type"`
	Created  Time   `json:"created"`
	URL      string `json:"url"`
}

// UploadNodeAssetResponse is a response from uploading an asset.
type UploadNodeAssetResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	MimeType string `json:"mime_type"`
}

// DeleteNodeAssetResponse is a response from deleting an asset.
type DeleteNodeAssetResponse = OkResponse

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

// --- Email Change Responses ---

// ChangeEmailResponse is a response from changing email.
type ChangeEmailResponse struct {
	Ok            bool   `json:"ok"`
	EmailVerified bool   `json:"email_verified"`
	Message       string `json:"message,omitempty"`
}

// --- Email Verification Responses ---

// SendVerificationEmailResponse is a response from sending a verification email.
type SendVerificationEmailResponse struct {
	Ok      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

// --- OAuth Linking Responses ---

// LinkOAuthAccountResponse is a response from initiating OAuth linking.
type LinkOAuthAccountResponse struct {
	RedirectURL string `json:"redirect_url"`
}

// UnlinkOAuthAccountResponse is a response from unlinking an OAuth provider.
type UnlinkOAuthAccountResponse = OkResponse

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
	EmailVerified   bool            `json:"email_verified,omitempty" jsonschema:"description=Whether the email has been verified"`
	Name            string          `json:"name" jsonschema:"description=User display name"`
	IsGlobalAdmin   bool            `json:"is_global_admin,omitempty" jsonschema:"description=Whether user has server-wide administrative access"`
	OAuthIdentities []OAuthIdentity `json:"oauth_identities,omitempty" jsonschema:"description=Linked OAuth provider accounts"`
	Settings        UserSettings    `json:"settings" jsonschema:"description=Global user preferences"`
	Created         Time            `json:"created" jsonschema:"description=Account creation Unix timestamp"`
	Modified        Time            `json:"modified" jsonschema:"description=Last modification Unix timestamp"`

	// Current context
	OrganizationID jsonldb.ID       `json:"organization_id,omitempty" jsonschema:"description=Active organization ID"`
	OrgRole        OrganizationRole `json:"org_role,omitempty" jsonschema:"description=Role in active organization"`
	WorkspaceID    jsonldb.ID       `json:"workspace_id,omitempty" jsonschema:"description=Active workspace ID"`
	WorkspaceName  string           `json:"workspace_name,omitempty" jsonschema:"description=Active workspace name"`
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
	Created          Time             `json:"created" jsonschema:"description=Membership creation Unix timestamp"`
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
	Created        Time                        `json:"created" jsonschema:"description=Membership creation Unix timestamp"`
}

// OrgInvitationResponse is the API representation of an organization invitation (excludes Token).
type OrgInvitationResponse struct {
	ID             jsonldb.ID       `json:"id" jsonschema:"description=Unique invitation identifier"`
	Email          string           `json:"email" jsonschema:"description=Email address of the invitee"`
	OrganizationID jsonldb.ID       `json:"organization_id" jsonschema:"description=Organization the user is invited to"`
	Role           OrganizationRole `json:"role" jsonschema:"description=Role assigned upon acceptance"`
	InvitedBy      jsonldb.ID       `json:"invited_by" jsonschema:"description=User ID who created the invitation"`
	ExpiresAt      Time             `json:"expires_at" jsonschema:"description=Invitation expiration Unix timestamp"`
	Created        Time             `json:"created" jsonschema:"description=Invitation creation Unix timestamp"`
}

// WSInvitationResponse is the API representation of a workspace invitation (excludes Token).
type WSInvitationResponse struct {
	ID          jsonldb.ID    `json:"id" jsonschema:"description=Unique invitation identifier"`
	Email       string        `json:"email" jsonschema:"description=Email address of the invitee"`
	WorkspaceID jsonldb.ID    `json:"workspace_id" jsonschema:"description=Workspace the user is invited to"`
	Role        WorkspaceRole `json:"role" jsonschema:"description=Role assigned upon acceptance"`
	InvitedBy   jsonldb.ID    `json:"invited_by" jsonschema:"description=User ID who created the invitation"`
	ExpiresAt   Time          `json:"expires_at" jsonschema:"description=Invitation expiration Unix timestamp"`
	Created     Time          `json:"created" jsonschema:"description=Invitation creation Unix timestamp"`
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
	Created        Time                 `json:"created" jsonschema:"description=Organization creation Unix timestamp"`
}

// WorkspaceResponse is the API representation of a workspace.
type WorkspaceResponse struct {
	ID             jsonldb.ID         `json:"id" jsonschema:"description=Unique workspace identifier"`
	OrganizationID jsonldb.ID         `json:"organization_id" jsonschema:"description=Parent organization ID"`
	Name           string             `json:"name" jsonschema:"description=Display name of the workspace"`
	Quotas         WorkspaceQuotas    `json:"quotas" jsonschema:"description=Resource limits for the workspace"`
	Settings       WorkspaceSettings  `json:"settings" jsonschema:"description=Workspace-wide configuration"`
	GitRemote      *GitRemoteResponse `json:"git_remote,omitempty" jsonschema:"description=Git remote configuration"`
	MemberCount    int                `json:"member_count" jsonschema:"description=Number of members"`
	Created        Time               `json:"created" jsonschema:"description=Workspace creation Unix timestamp"`
}

// GitRemoteResponse is the API representation of a git remote.
type GitRemoteResponse struct {
	WorkspaceID jsonldb.ID `json:"workspace_id" jsonschema:"description=Workspace this remote belongs to"`
	URL         string     `json:"url" jsonschema:"description=Git repository URL"`
	Type        string     `json:"type" jsonschema:"description=Remote type (github/gitlab/custom)"`
	AuthType    string     `json:"auth_type" jsonschema:"description=Authentication method (token/ssh)"`
	Created     Time       `json:"created" jsonschema:"description=Remote creation Unix timestamp"`
	LastSync    Time       `json:"last_sync,omitempty" jsonschema:"description=Last synchronization Unix timestamp"`
}

// NodeResponse is the API representation of a node.
type NodeResponse struct {
	ID          jsonldb.ID     `json:"id" jsonschema:"description=Unique node identifier"`
	ParentID    jsonldb.ID     `json:"parent_id,omitempty" jsonschema:"description=Parent node ID for hierarchical structure"`
	Title       string         `json:"title" jsonschema:"description=Node title"`
	Content     string         `json:"content,omitempty" jsonschema:"description=Markdown content (Page part)"`
	Properties  []Property     `json:"properties,omitempty" jsonschema:"description=Schema (Table part)"`
	Created     Time           `json:"created" jsonschema:"description=Node creation Unix timestamp"`
	Modified    Time           `json:"modified" jsonschema:"description=Last modification Unix timestamp"`
	Tags        []string       `json:"tags,omitempty" jsonschema:"description=Node tags"`
	FaviconURL  string         `json:"favicon_url,omitempty" jsonschema:"description=Favicon URL"`
	HasPage     bool           `json:"has_page" jsonschema:"description=Whether node has page content (index.md exists)"`
	HasTable    bool           `json:"has_table" jsonschema:"description=Whether node has table content (metadata.json exists)"`
	HasChildren bool           `json:"has_children,omitempty" jsonschema:"description=Whether node has child nodes"`
	Children    []NodeResponse `json:"children,omitempty" jsonschema:"description=Nested nodes"`
}

// GetPageResponse is a response containing page content.
type GetPageResponse struct {
	ID       jsonldb.ID `json:"id" jsonschema:"description=Node identifier"`
	Title    string     `json:"title" jsonschema:"description=Page title"`
	Content  string     `json:"content" jsonschema:"description=Markdown content"`
	Created  Time       `json:"created" jsonschema:"description=Page creation Unix timestamp"`
	Modified Time       `json:"modified" jsonschema:"description=Last modification Unix timestamp"`
}

// CreatePageResponse is a response from creating a page.
type CreatePageResponse struct {
	ID jsonldb.ID `json:"id" jsonschema:"description=New node identifier"`
}

// UpdatePageResponse is a response from updating a page.
type UpdatePageResponse struct {
	ID jsonldb.ID `json:"id" jsonschema:"description=Node identifier"`
}

// DeletePageResponse is a response from deleting a page.
type DeletePageResponse = OkResponse

// GetTableSchemaResponse is a response containing table schema.
type GetTableSchemaResponse struct {
	ID         jsonldb.ID `json:"id" jsonschema:"description=Node identifier"`
	Title      string     `json:"title" jsonschema:"description=Table title"`
	Properties []Property `json:"properties" jsonschema:"description=Table schema"`
	Created    Time       `json:"created" jsonschema:"description=Table creation Unix timestamp"`
	Modified   Time       `json:"modified" jsonschema:"description=Last modification Unix timestamp"`
}

// CreateTableUnderParentResponse is a response from creating a table under a parent.
type CreateTableUnderParentResponse struct {
	ID jsonldb.ID `json:"id" jsonschema:"description=New node identifier"`
}

// ListNodeChildrenResponse is a response containing children of a node.
type ListNodeChildrenResponse struct {
	Nodes []NodeResponse `json:"nodes" jsonschema:"description=Child nodes"`
}

// DataRecordResponse is the API representation of a data record.
type DataRecordResponse struct {
	ID       jsonldb.ID     `json:"id" jsonschema:"description=Unique record identifier"`
	Data     map[string]any `json:"data" jsonschema:"description=Record field values keyed by property name"`
	Created  Time           `json:"created" jsonschema:"description=Record creation Unix timestamp"`
	Modified Time           `json:"modified" jsonschema:"description=Last modification Unix timestamp"`
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
