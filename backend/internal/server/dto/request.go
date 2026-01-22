package dto

// --- Auth ---

// LoginRequest is a request to log in.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Validate validates the login request fields.
func (r *LoginRequest) Validate() error {
	if r.Email == "" {
		return MissingField("email")
	}
	if r.Password == "" {
		return MissingField("password")
	}
	return nil
}

// RegisterRequest is a request to register a new user.
type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

// Validate validates the register request fields.
func (r *RegisterRequest) Validate() error {
	if r.Email == "" {
		return MissingField("email")
	}
	if r.Password == "" {
		return MissingField("password")
	}
	if r.Name == "" {
		return MissingField("name")
	}
	return nil
}

// MeRequest is a request to get current user info.
type MeRequest struct{}

// Validate is a no-op for MeRequest.
func (r *MeRequest) Validate() error {
	return nil
}

// --- Pages ---

// ListPagesRequest is a request to list all pages.
type ListPagesRequest struct {
	OrgID string `path:"orgID"`
}

// Validate validates the list pages request fields.
func (r *ListPagesRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("orgID")
	}
	return nil
}

// GetPageRequest is a request to get a page.
type GetPageRequest struct {
	OrgID string `path:"orgID"`
	ID    string `path:"id"`
}

// Validate validates the get page request fields.
func (r *GetPageRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("orgID")
	}
	if r.ID == "" {
		return MissingField("id")
	}
	return nil
}

// CreatePageRequest is a request to create a page.
type CreatePageRequest struct {
	OrgID   string `path:"orgID"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

// Validate validates the create page request fields.
func (r *CreatePageRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("orgID")
	}
	if r.Title == "" {
		return MissingField("title")
	}
	return nil
}

// UpdatePageRequest is a request to update a page.
type UpdatePageRequest struct {
	OrgID   string `path:"orgID"`
	ID      string `path:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

// Validate validates the update page request fields.
func (r *UpdatePageRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("orgID")
	}
	if r.ID == "" {
		return MissingField("id")
	}
	if r.Title == "" {
		return MissingField("title")
	}
	return nil
}

// DeletePageRequest is a request to delete a page.
type DeletePageRequest struct {
	OrgID string `path:"orgID"`
	ID    string `path:"id"`
}

// Validate validates the delete page request fields.
func (r *DeletePageRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("orgID")
	}
	if r.ID == "" {
		return MissingField("id")
	}
	return nil
}

// GetPageHistoryRequest is a request to get page history.
type GetPageHistoryRequest struct {
	OrgID string `path:"orgID"`
	ID    string `path:"id"`
	Limit int    `query:"limit"` // Max commits to return (1-1000, default 1000).
}

// Validate validates the get page history request fields.
func (r *GetPageHistoryRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("orgID")
	}
	if r.ID == "" {
		return MissingField("id")
	}
	return nil
}

// GetPageVersionRequest is a request to get a specific page version.
type GetPageVersionRequest struct {
	OrgID string `path:"orgID"`
	ID    string `path:"id"`
	Hash  string `path:"hash"`
}

// Validate validates the get page version request fields.
func (r *GetPageVersionRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("orgID")
	}
	if r.ID == "" {
		return MissingField("id")
	}
	if r.Hash == "" {
		return MissingField("hash")
	}
	return nil
}

// --- Tables ---

// ListTablesRequest is a request to list tables.
type ListTablesRequest struct {
	OrgID string `path:"orgID"`
}

// Validate validates the list tables request fields.
func (r *ListTablesRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("orgID")
	}
	return nil
}

// GetTableRequest is a request to get a table.
type GetTableRequest struct {
	OrgID string `path:"orgID"`
	ID    string `path:"id"`
}

// Validate validates the get table request fields.
func (r *GetTableRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("orgID")
	}
	if r.ID == "" {
		return MissingField("id")
	}
	return nil
}

// CreateTableRequest is a request to create a table.
type CreateTableRequest struct {
	OrgID      string     `path:"orgID"`
	Title      string     `json:"title"`
	Properties []Property `json:"properties"`
}

// Validate validates the create table request fields.
func (r *CreateTableRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("orgID")
	}
	if r.Title == "" {
		return MissingField("title")
	}
	return nil
}

// UpdateTableRequest is a request to update a table.
type UpdateTableRequest struct {
	OrgID      string     `path:"orgID"`
	ID         string     `path:"id"`
	Title      string     `json:"title"`
	Properties []Property `json:"properties"`
}

// Validate validates the update table request fields.
func (r *UpdateTableRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("orgID")
	}
	if r.ID == "" {
		return MissingField("id")
	}
	if r.Title == "" {
		return MissingField("title")
	}
	return nil
}

// DeleteTableRequest is a request to delete a table.
type DeleteTableRequest struct {
	OrgID string `path:"orgID"`
	ID    string `path:"id"`
}

// Validate validates the delete table request fields.
func (r *DeleteTableRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("orgID")
	}
	if r.ID == "" {
		return MissingField("id")
	}
	return nil
}

// ListRecordsRequest is a request to list records in a table.
type ListRecordsRequest struct {
	OrgID  string `path:"orgID"`
	ID     string `path:"id"`
	Offset int    `query:"offset"`
	Limit  int    `query:"limit"`
}

// Validate validates the list records request fields.
func (r *ListRecordsRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("orgID")
	}
	if r.ID == "" {
		return MissingField("id")
	}
	return nil
}

// CreateRecordRequest is a request to create a record.
type CreateRecordRequest struct {
	OrgID string         `path:"orgID"`
	ID    string         `path:"id"`
	Data  map[string]any `json:"data"`
}

// Validate validates the create record request fields.
func (r *CreateRecordRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("orgID")
	}
	if r.ID == "" {
		return MissingField("id")
	}
	return nil
}

// UpdateRecordRequest is a request to update a record.
type UpdateRecordRequest struct {
	OrgID string         `path:"orgID"`
	ID    string         `path:"id"`
	RID   string         `path:"rid"`
	Data  map[string]any `json:"data"`
}

// Validate validates the update record request fields.
func (r *UpdateRecordRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("orgID")
	}
	if r.ID == "" {
		return MissingField("id")
	}
	if r.RID == "" {
		return MissingField("rid")
	}
	return nil
}

// GetRecordRequest is a request to get a record.
type GetRecordRequest struct {
	OrgID string `path:"orgID"`
	ID    string `path:"id"`
	RID   string `path:"rid"`
}

// Validate validates the get record request fields.
func (r *GetRecordRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("orgID")
	}
	if r.ID == "" {
		return MissingField("id")
	}
	if r.RID == "" {
		return MissingField("rid")
	}
	return nil
}

// DeleteRecordRequest is a request to delete a record.
type DeleteRecordRequest struct {
	OrgID string `path:"orgID"`
	ID    string `path:"id"`
	RID   string `path:"rid"`
}

// Validate validates the delete record request fields.
func (r *DeleteRecordRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("orgID")
	}
	if r.ID == "" {
		return MissingField("id")
	}
	if r.RID == "" {
		return MissingField("rid")
	}
	return nil
}

// --- Nodes ---

// ListNodesRequest is a request to list nodes.
type ListNodesRequest struct {
	OrgID string `path:"orgID"`
}

// Validate validates the list nodes request fields.
func (r *ListNodesRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("orgID")
	}
	return nil
}

// GetNodeRequest is a request to get a node.
type GetNodeRequest struct {
	OrgID string `path:"orgID"`
	ID    string `path:"id"`
}

// Validate validates the get node request fields.
func (r *GetNodeRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("orgID")
	}
	if r.ID == "" {
		return MissingField("id")
	}
	return nil
}

// CreateNodeRequest is a request to create a node.
type CreateNodeRequest struct {
	OrgID    string   `path:"orgID"`
	ParentID string   `json:"parent_id,omitempty"`
	Title    string   `json:"title"`
	Type     NodeType `json:"type"`
}

// Validate validates the create node request fields.
func (r *CreateNodeRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("orgID")
	}
	if r.Title == "" {
		return MissingField("title")
	}
	if r.Type == "" {
		return MissingField("type")
	}
	return nil
}

// --- Assets ---

// ListPageAssetsRequest is a request to list assets in a page.
type ListPageAssetsRequest struct {
	OrgID  string `path:"orgID"`
	PageID string `path:"id"`
}

// Validate validates the list page assets request fields.
func (r *ListPageAssetsRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("orgID")
	}
	if r.PageID == "" {
		return MissingField("id")
	}
	return nil
}

// UploadPageAssetRequest is a request to upload an asset to a page.
type UploadPageAssetRequest struct {
	OrgID  string `path:"orgID"`
	PageID string `path:"id"`
}

// Validate validates the upload page asset request fields.
func (r *UploadPageAssetRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("orgID")
	}
	if r.PageID == "" {
		return MissingField("id")
	}
	return nil
}

// DeletePageAssetRequest is a request to delete an asset from a page.
type DeletePageAssetRequest struct {
	OrgID     string `path:"orgID"`
	PageID    string `path:"id"`
	AssetName string `path:"assetName"`
}

// Validate validates the delete page asset request fields.
func (r *DeletePageAssetRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("orgID")
	}
	if r.PageID == "" {
		return MissingField("id")
	}
	if r.AssetName == "" {
		return MissingField("assetName")
	}
	return nil
}

// ServeAssetRequest is a request to serve an asset file directly.
type ServeAssetRequest struct {
	OrgID     string `path:"path"`
	PageID    string `path:"id"`
	AssetName string `path:"assetName"`
}

// Validate validates the serve asset request fields.
func (r *ServeAssetRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("orgID")
	}
	if r.PageID == "" {
		return MissingField("id")
	}
	if r.AssetName == "" {
		return MissingField("assetName")
	}
	return nil
}

// --- Search ---

// SearchRequest is a request to search pages and tables.
type SearchRequest struct {
	OrgID       string `path:"orgID"`
	Query       string `json:"query"`
	Limit       int    `json:"limit,omitempty"`
	MatchTitle  bool   `json:"match_title,omitempty"`
	MatchBody   bool   `json:"match_body,omitempty"`
	MatchFields bool   `json:"match_fields,omitempty"`
}

// Validate validates the search request fields.
func (r *SearchRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("orgID")
	}
	if r.Query == "" {
		return MissingField("query")
	}
	return nil
}

// --- Invitations ---

// CreateInvitationRequest is a request to create an invitation.
type CreateInvitationRequest struct {
	OrgID string   `path:"orgID"`
	Email string   `json:"email"`
	Role  UserRole `json:"role"`
}

// Validate validates the create invitation request fields.
func (r *CreateInvitationRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("orgID")
	}
	if r.Email == "" {
		return MissingField("email")
	}
	if r.Role == "" {
		return MissingField("role")
	}
	return nil
}

// ListInvitationsRequest is a request to list invitations for an organization.
type ListInvitationsRequest struct {
	OrgID string `path:"orgID"`
}

// Validate validates the list invitations request fields.
func (r *ListInvitationsRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("orgID")
	}
	return nil
}

// AcceptInvitationRequest is a request to accept an invitation.
type AcceptInvitationRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

// Validate validates the accept invitation request fields.
func (r *AcceptInvitationRequest) Validate() error {
	if r.Token == "" {
		return MissingField("token")
	}
	if r.Password == "" {
		return MissingField("password")
	}
	if r.Name == "" {
		return MissingField("name")
	}
	return nil
}

// --- Memberships ---

// SwitchOrgRequest is a request to switch active organization.
type SwitchOrgRequest struct {
	OrgID string `json:"org_id"`
}

// Validate validates the switch org request fields.
func (r *SwitchOrgRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("org_id")
	}
	return nil
}

// UpdateMembershipSettingsRequest is a request to update user preferences within an organization.
type UpdateMembershipSettingsRequest struct {
	OrgID    string             `path:"orgID"`
	Settings MembershipSettings `json:"settings"`
}

// Validate validates the update membership settings request fields.
func (r *UpdateMembershipSettingsRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("orgID")
	}
	return nil
}

// --- Organizations ---

// UpdateOrgSettingsRequest is a request to update organization-wide settings.
type UpdateOrgSettingsRequest struct {
	OrgID    string               `path:"orgID"`
	Settings OrganizationSettings `json:"settings"`
}

// Validate validates the update org settings request fields.
func (r *UpdateOrgSettingsRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("orgID")
	}
	return nil
}

// UpdateOrganizationRequest is a request to update organization details.
type UpdateOrganizationRequest struct {
	OrgID string `path:"orgID"`
	Name  string `json:"name"`
}

// Validate validates the update organization request fields.
func (r *UpdateOrganizationRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("orgID")
	}
	if r.Name == "" {
		return MissingField("name")
	}
	return nil
}

// CreateOrganizationRequest is a request to create a new organization.
type CreateOrganizationRequest struct {
	Name               string `json:"name"`
	WelcomePageTitle   string `json:"welcome_page_title,omitempty"`
	WelcomePageContent string `json:"welcome_page_content,omitempty"`
}

// Validate validates the create organization request fields.
func (r *CreateOrganizationRequest) Validate() error {
	if r.Name == "" {
		return MissingField("name")
	}
	return nil
}

// GetOnboardingRequest is a request to get onboarding status.
type GetOnboardingRequest struct {
	OrgID string `path:"orgID"`
}

// Validate validates the get onboarding request fields.
func (r *GetOnboardingRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("orgID")
	}
	return nil
}

// UpdateOnboardingRequest is a request to update onboarding status.
type UpdateOnboardingRequest struct {
	OrgID string          `path:"orgID"`
	State OnboardingState `json:"state"`
}

// Validate validates the update onboarding request fields.
func (r *UpdateOnboardingRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("orgID")
	}
	return nil
}

// --- Git Remotes ---

// GetGitRemoteRequest is a request to get the git remote for an organization.
type GetGitRemoteRequest struct {
	OrgID string `path:"orgID"`
}

// Validate validates the get git remote request fields.
func (r *GetGitRemoteRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("orgID")
	}
	return nil
}

// SetGitRemoteRequest is a request to set (create or update) the git remote for an organization.
type SetGitRemoteRequest struct {
	OrgID    string `path:"orgID"`
	URL      string `json:"url"`
	Type     string `json:"type"`      // github, gitlab, custom
	AuthType string `json:"auth_type"` // token, ssh
	Token    string `json:"token,omitempty"`
}

// Validate validates the set git remote request fields.
func (r *SetGitRemoteRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("orgID")
	}
	if r.URL == "" {
		return MissingField("url")
	}
	if r.Type == "" {
		return MissingField("type")
	}
	if r.AuthType == "" {
		return MissingField("auth_type")
	}
	return nil
}

// DeleteGitRemoteRequest is a request to delete the git remote for an organization.
type DeleteGitRemoteRequest struct {
	OrgID string `path:"orgID"`
}

// Validate validates the delete git remote request fields.
func (r *DeleteGitRemoteRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("orgID")
	}
	return nil
}

// PushGitRemoteRequest is a request to push to the git remote.
type PushGitRemoteRequest struct {
	OrgID string `path:"orgID"`
}

// Validate validates the push git remote request fields.
func (r *PushGitRemoteRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("orgID")
	}
	return nil
}

// --- Health ---

// HealthRequest is a request to check system health.
type HealthRequest struct{}

// Validate is a no-op for HealthRequest.
func (r *HealthRequest) Validate() error {
	return nil
}

// --- Users ---

// ListUsersRequest is a request to list users.
type ListUsersRequest struct {
	OrgID string `path:"orgID"`
}

// Validate validates the list users request fields.
func (r *ListUsersRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("orgID")
	}
	return nil
}

// UpdateRoleRequest is a request to update a user's role.
type UpdateRoleRequest struct {
	OrgID  string   `path:"orgID"`
	UserID string   `json:"user_id"`
	Role   UserRole `json:"role"`
}

// Validate validates the update role request fields.
func (r *UpdateRoleRequest) Validate() error {
	if r.OrgID == "" {
		return MissingField("orgID")
	}
	if r.UserID == "" {
		return MissingField("user_id")
	}
	if r.Role == "" {
		return MissingField("role")
	}
	return nil
}

// UpdateUserSettingsRequest is a request to update user global settings.
type UpdateUserSettingsRequest struct {
	Settings UserSettings `json:"settings"`
}

// Validate is a no-op for UpdateUserSettingsRequest.
func (r *UpdateUserSettingsRequest) Validate() error {
	return nil
}

// --- Admin ---

// AdminStatsRequest is a request to get admin stats.
type AdminStatsRequest struct{}

// Validate is a no-op for AdminStatsRequest.
func (r *AdminStatsRequest) Validate() error {
	return nil
}

// AdminUsersRequest is a request to list all users (admin only).
type AdminUsersRequest struct{}

// Validate is a no-op for AdminUsersRequest.
func (r *AdminUsersRequest) Validate() error {
	return nil
}

// AdminOrgsRequest is a request to list all organizations (admin only).
type AdminOrgsRequest struct{}

// Validate is a no-op for AdminOrgsRequest.
func (r *AdminOrgsRequest) Validate() error {
	return nil
}
