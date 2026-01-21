package dto

// --- Auth ---

// LoginRequest is a request to log in.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// RegisterRequest is a request to register a new user.
type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

// MeRequest is a request to get current user info.
type MeRequest struct{}

// --- Pages ---

// ListPagesRequest is a request to list all pages.
type ListPagesRequest struct {
	OrgID string `path:"orgID"`
}

// GetPageRequest is a request to get a page.
type GetPageRequest struct {
	OrgID string `path:"orgID"`
	ID    string `path:"id"`
}

// CreatePageRequest is a request to create a page.
type CreatePageRequest struct {
	OrgID   string `path:"orgID"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

// UpdatePageRequest is a request to update a page.
type UpdatePageRequest struct {
	OrgID   string `path:"orgID"`
	ID      string `path:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

// DeletePageRequest is a request to delete a page.
type DeletePageRequest struct {
	OrgID string `path:"orgID"`
	ID    string `path:"id"`
}

// GetPageHistoryRequest is a request to get page history.
type GetPageHistoryRequest struct {
	OrgID string `path:"orgID"`
	ID    string `path:"id"`
	Limit int    `query:"limit"` // Max commits to return (1-1000, default 1000).
}

// GetPageVersionRequest is a request to get a specific page version.
type GetPageVersionRequest struct {
	OrgID string `path:"orgID"`
	ID    string `path:"id"`
	Hash  string `path:"hash"`
}

// --- Tables ---

// ListTablesRequest is a request to list tables.
type ListTablesRequest struct {
	OrgID string `path:"orgID"`
}

// GetTableRequest is a request to get a table.
type GetTableRequest struct {
	OrgID string `path:"orgID"`
	ID    string `path:"id"`
}

// CreateTableRequest is a request to create a table.
type CreateTableRequest struct {
	OrgID      string     `path:"orgID"`
	Title      string     `json:"title"`
	Properties []Property `json:"properties"`
}

// UpdateTableRequest is a request to update a table.
type UpdateTableRequest struct {
	OrgID      string     `path:"orgID"`
	ID         string     `path:"id"`
	Title      string     `json:"title"`
	Properties []Property `json:"properties"`
}

// DeleteTableRequest is a request to delete a table.
type DeleteTableRequest struct {
	OrgID string `path:"orgID"`
	ID    string `path:"id"`
}

// ListRecordsRequest is a request to list records in a database.
type ListRecordsRequest struct {
	OrgID  string `path:"orgID"`
	ID     string `path:"id"`
	Offset int    `query:"offset"`
	Limit  int    `query:"limit"`
}

// CreateRecordRequest is a request to create a record.
type CreateRecordRequest struct {
	OrgID string         `path:"orgID"`
	ID    string         `path:"id"`
	Data  map[string]any `json:"data"`
}

// UpdateRecordRequest is a request to update a record.
type UpdateRecordRequest struct {
	OrgID string         `path:"orgID"`
	ID    string         `path:"id"`
	RID   string         `path:"rid"`
	Data  map[string]any `json:"data"`
}

// GetRecordRequest is a request to get a record.
type GetRecordRequest struct {
	OrgID string `path:"orgID"`
	ID    string `path:"id"`
	RID   string `path:"rid"`
}

// DeleteRecordRequest is a request to delete a record.
type DeleteRecordRequest struct {
	OrgID string `path:"orgID"`
	ID    string `path:"id"`
	RID   string `path:"rid"`
}

// --- Nodes ---

// ListNodesRequest is a request to list nodes.
type ListNodesRequest struct {
	OrgID string `path:"orgID"`
}

// GetNodeRequest is a request to get a node.
type GetNodeRequest struct {
	OrgID string `path:"orgID"`
	ID    string `path:"id"`
}

// CreateNodeRequest is a request to create a node.
type CreateNodeRequest struct {
	OrgID    string   `path:"orgID"`
	ParentID string   `json:"parent_id,omitempty"`
	Title    string   `json:"title"`
	Type     NodeType `json:"type"`
}

// --- Assets ---

// ListPageAssetsRequest is a request to list assets in a page.
type ListPageAssetsRequest struct {
	OrgID  string `path:"orgID"`
	PageID string `path:"id"`
}

// UploadPageAssetRequest is a request to upload an asset to a page.
type UploadPageAssetRequest struct {
	OrgID  string `path:"orgID"`
	PageID string `path:"id"`
}

// DeletePageAssetRequest is a request to delete an asset from a page.
type DeletePageAssetRequest struct {
	OrgID     string `path:"orgID"`
	PageID    string `path:"id"`
	AssetName string `path:"assetName"`
}

// ServeAssetRequest is a request to serve an asset file directly.
type ServeAssetRequest struct {
	OrgID     string `path:"path"`
	PageID    string `path:"id"`
	AssetName string `path:"assetName"`
}

// --- Search ---

// SearchRequest is a request to search pages and databases.
type SearchRequest struct {
	OrgID       string `path:"orgID"`
	Query       string `json:"query"`
	Limit       int    `json:"limit,omitempty"`
	MatchTitle  bool   `json:"match_title,omitempty"`
	MatchBody   bool   `json:"match_body,omitempty"`
	MatchFields bool   `json:"match_fields,omitempty"`
}

// --- Invitations ---

// CreateInvitationRequest is a request to create an invitation.
type CreateInvitationRequest struct {
	OrgID string   `path:"orgID"`
	Email string   `json:"email"`
	Role  UserRole `json:"role"`
}

// ListInvitationsRequest is a request to list invitations for an organization.
type ListInvitationsRequest struct {
	OrgID string `path:"orgID"`
}

// AcceptInvitationRequest is a request to accept an invitation.
type AcceptInvitationRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

// --- Memberships ---

// SwitchOrgRequest is a request to switch active organization.
type SwitchOrgRequest struct {
	OrgID string `json:"org_id"`
}

// UpdateMembershipSettingsRequest is a request to update user preferences within an organization.
type UpdateMembershipSettingsRequest struct {
	OrgID    string             `path:"orgID"`
	Settings MembershipSettings `json:"settings"`
}

// --- Organizations ---

// UpdateOrgSettingsRequest is a request to update organization-wide settings.
type UpdateOrgSettingsRequest struct {
	OrgID    string               `path:"orgID"`
	Settings OrganizationSettings `json:"settings"`
}

// GetOnboardingRequest is a request to get onboarding status.
type GetOnboardingRequest struct {
	OrgID string `path:"orgID"`
}

// UpdateOnboardingRequest is a request to update onboarding status.
type UpdateOnboardingRequest struct {
	OrgID string          `path:"orgID"`
	State OnboardingState `json:"state"`
}

// --- Git Remotes ---

// GetGitRemoteRequest is a request to get the git remote for an organization.
type GetGitRemoteRequest struct {
	OrgID string `path:"orgID"`
}

// SetGitRemoteRequest is a request to set (create or update) the git remote for an organization.
type SetGitRemoteRequest struct {
	OrgID    string `path:"orgID"`
	URL      string `json:"url"`
	Type     string `json:"type"`      // github, gitlab, custom
	AuthType string `json:"auth_type"` // token, ssh
	Token    string `json:"token,omitempty"`
}

// DeleteGitRemoteRequest is a request to delete the git remote for an organization.
type DeleteGitRemoteRequest struct {
	OrgID string `path:"orgID"`
}

// PushGitRemoteRequest is a request to push to the git remote.
type PushGitRemoteRequest struct {
	OrgID string `path:"orgID"`
}

// --- Health ---

// HealthRequest is a request to check system health.
type HealthRequest struct{}

// --- Users ---

// ListUsersRequest is a request to list users.
type ListUsersRequest struct {
	OrgID string `path:"orgID"`
}

// UpdateRoleRequest is a request to update a user's role.
type UpdateRoleRequest struct {
	OrgID  string   `path:"orgID"`
	UserID string   `json:"user_id"`
	Role   UserRole `json:"role"`
}

// UpdateUserSettingsRequest is a request to update user global settings.
type UpdateUserSettingsRequest struct {
	Settings UserSettings `json:"settings"`
}
