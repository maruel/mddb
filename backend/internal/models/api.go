package models

// --- Auth ---

// LoginRequest is a request to log in.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginResponse is a response from logging in.
type LoginResponse struct {
	Token string `json:"token"`
	User  *User  `json:"user"`
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

// ListPagesResponse is a response containing a list of pages.
type ListPagesResponse struct {
	Pages []any `json:"pages"`
}

// GetPageRequest is a request to get a page.
type GetPageRequest struct {
	OrgID string `path:"orgID"`
	ID    string `path:"id"`
}

// GetPageResponse is a response containing a page.
type GetPageResponse struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

// CreatePageRequest is a request to create a page.
type CreatePageRequest struct {
	OrgID   string `path:"orgID"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

// CreatePageResponse is a response from creating a page.
type CreatePageResponse struct {
	ID string `json:"id"`
}

// UpdatePageRequest is a request to update a page.
type UpdatePageRequest struct {
	OrgID   string `path:"orgID"`
	ID      string `path:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

// UpdatePageResponse is a response from updating a page.
type UpdatePageResponse struct {
	ID string `json:"id"`
}

// DeletePageRequest is a request to delete a page.
type DeletePageRequest struct {
	OrgID string `path:"orgID"`
	ID    string `path:"id"`
}

// DeletePageResponse is a response from deleting a page.
type DeletePageResponse struct{}

// GetPageHistoryRequest is a request to get page history.
type GetPageHistoryRequest struct {
	OrgID string `path:"orgID"`
	ID    string `path:"id"`
}

// GetPageHistoryResponse is a response containing page history.
type GetPageHistoryResponse struct {
	History []*Commit `json:"history"`
}

// GetPageVersionRequest is a request to get a specific page version.
type GetPageVersionRequest struct {
	OrgID string `path:"orgID"`
	ID    string `path:"id"`
	Hash  string `path:"hash"`
}

// GetPageVersionResponse is a response containing page content at a version.
type GetPageVersionResponse struct {
	Content string `json:"content"`
}

// --- Databases ---

// ListDatabasesRequest is a request to list databases.
type ListDatabasesRequest struct {
	OrgID string `path:"orgID"`
}

// ListDatabasesResponse is a response containing a list of databases.
type ListDatabasesResponse struct {
	Databases []any `json:"databases"`
}

// GetDatabaseRequest is a request to get a database.
type GetDatabaseRequest struct {
	OrgID string `path:"orgID"`
	ID    string `path:"id"`
}

// GetDatabaseResponse is a response containing a database.
type GetDatabaseResponse struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	Properties []Property `json:"properties"`
	Created    string     `json:"created"`
	Modified   string     `json:"modified"`
}

// CreateDatabaseRequest is a request to create a database.
type CreateDatabaseRequest struct {
	OrgID      string     `path:"orgID"`
	Title      string     `json:"title"`
	Properties []Property `json:"properties"`
}

// CreateDatabaseResponse is a response from creating a database.
type CreateDatabaseResponse struct {
	ID string `json:"id"`
}

// UpdateDatabaseRequest is a request to update a database.
type UpdateDatabaseRequest struct {
	OrgID      string     `path:"orgID"`
	ID         string     `path:"id"`
	Title      string     `json:"title"`
	Properties []Property `json:"properties"`
}

// UpdateDatabaseResponse is a response from updating a database.
type UpdateDatabaseResponse struct {
	ID string `json:"id"`
}

// DeleteDatabaseRequest is a request to delete a database.
type DeleteDatabaseRequest struct {
	OrgID string `path:"orgID"`
	ID    string `path:"id"`
}

// DeleteDatabaseResponse is a response from deleting a database.
type DeleteDatabaseResponse struct{}

// ListRecordsRequest is a request to list records in a database.
type ListRecordsRequest struct {
	OrgID  string `path:"orgID"`
	ID     string `path:"id"`
	Offset int    `query:"offset"`
	Limit  int    `query:"limit"`
}

// ListRecordsResponse is a response containing a list of records.
type ListRecordsResponse struct {
	Records []DataRecord `json:"records"`
}

// CreateRecordRequest is a request to create a record.
type CreateRecordRequest struct {
	OrgID string         `path:"orgID"`
	ID    string         `path:"id"`
	Data  map[string]any `json:"data"`
}

// CreateRecordResponse is a response from creating a record.
type CreateRecordResponse struct {
	ID string `json:"id"`
}

// UpdateRecordRequest is a request to update a record.
type UpdateRecordRequest struct {
	OrgID string         `path:"orgID"`
	ID    string         `path:"id"`
	RID   string         `path:"rid"`
	Data  map[string]any `json:"data"`
}

// UpdateRecordResponse is a response from updating a record.
type UpdateRecordResponse struct {
	ID string `json:"id"`
}

// GetRecordRequest is a request to get a record.
type GetRecordRequest struct {
	OrgID string `path:"orgID"`
	ID    string `path:"id"`
	RID   string `path:"rid"`
}

// GetRecordResponse is a response containing a record.
type GetRecordResponse struct {
	ID       string         `json:"id"`
	Data     map[string]any `json:"data"`
	Created  string         `json:"created"`
	Modified string         `json:"modified"`
}

// DeleteRecordRequest is a request to delete a record.
type DeleteRecordRequest struct {
	OrgID string `path:"orgID"`
	ID    string `path:"id"`
	RID   string `path:"rid"`
}

// DeleteRecordResponse is a response from deleting a record.
type DeleteRecordResponse struct{}

// --- Nodes ---

// ListNodesRequest is a request to list nodes.
type ListNodesRequest struct {
	OrgID string `path:"orgID"`
}

// ListNodesResponse is a response containing a list of nodes.
type ListNodesResponse struct {
	Nodes []*Node `json:"nodes"`
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

// ListPageAssetsResponse is a response containing a list of assets.
type ListPageAssetsResponse struct {
	Assets []any `json:"assets"`
}

// UploadPageAssetRequest is a request to upload an asset to a page.
type UploadPageAssetRequest struct {
	OrgID  string `path:"orgID"`
	PageID string `path:"id"`
}

// UploadPageAssetResponse is a response from uploading an asset.
type UploadPageAssetResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	MimeType string `json:"mime_type"`
}

// DeletePageAssetRequest is a request to delete an asset from a page.
type DeletePageAssetRequest struct {
	OrgID     string `path:"orgID"`
	PageID    string `path:"id"`
	AssetName string `path:"assetName"`
}

// DeletePageAssetResponse is a response from deleting an asset.
type DeletePageAssetResponse struct{}

// ServeAssetRequest is a request to serve an asset file directly.
type ServeAssetRequest struct {
	OrgID     string `path:"path"`
	PageID    string `path:"id"`
	AssetName string `path:"assetName"`
}

// ServeAssetResponse wraps the binary asset data.
type ServeAssetResponse struct {
	Data     string `json:"data"`
	MimeType string `json:"mime_type"`
}

// --- Search ---

// SearchRequest is a request to search pages and databases
type SearchRequest struct {
	OrgID       string `path:"orgID"`
	Query       string `json:"query"`
	Limit       int    `json:"limit,omitempty"`
	MatchTitle  bool   `json:"match_title,omitempty"`
	MatchBody   bool   `json:"match_body,omitempty"`
	MatchFields bool   `json:"match_fields,omitempty"`
}

// SearchResponse is the response to a search request
type SearchResponse struct {
	Results []SearchResult `json:"results"`
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

// ListInvitationsResponse is a response containing a list of invitations.
type ListInvitationsResponse struct {
	Invitations []*Invitation `json:"invitations"`
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

// SwitchOrgResponse is a response from switching organization.
type SwitchOrgResponse struct {
	Token string `json:"token"`
	User  *User  `json:"user"`
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

// ListGitRemotesRequest is a request to list git remotes.
type ListGitRemotesRequest struct {
	OrgID string `path:"orgID"`
}

// ListGitRemotesResponse is a response containing a list of git remotes.
type ListGitRemotesResponse struct {
	Remotes []*GitRemote `json:"remotes"`
}

// CreateGitRemoteRequest is a request to create a git remote.
type CreateGitRemoteRequest struct {
	OrgID    string `path:"orgID"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Type     string `json:"type"`      // github, gitlab, custom
	AuthType string `json:"auth_type"` // token, ssh
	Token    string `json:"token,omitempty"`
}

// UpdateGitRemoteRequest is a request to update a git remote.
type UpdateGitRemoteRequest struct {
	OrgID    string `path:"orgID"`
	RemoteID string `path:"remoteID"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Token    string `json:"token,omitempty"`
}

// DeleteGitRemoteRequest is a request to delete a git remote.
type DeleteGitRemoteRequest struct {
	OrgID    string `path:"orgID"`
	RemoteID string `path:"remoteID"`
}

// PushGitRemoteRequest is a request to push to a git remote.
type PushGitRemoteRequest struct {
	OrgID    string `path:"orgID"`
	RemoteID string `path:"remoteID"`
}

// --- Health ---

// HealthRequest is a request to check system health.
type HealthRequest struct{}

// HealthResponse is a response from a health check.
type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

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

// ListUsersResponse is a response containing a list of users.
type ListUsersResponse struct {
	Users []*User `json:"users"`
}

// UpdateUserSettingsRequest is a request to update user global settings.
type UpdateUserSettingsRequest struct {
	Settings UserSettings `json:"settings"`
}
