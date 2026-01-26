// Defines API request payloads and validation logic.

package dto

import (
	"net/mail"
	"unicode"

	"github.com/maruel/mddb/backend/internal/jsonldb"
)

// validateEmail checks if the email has valid format.
func validateEmail(email string) error {
	if email == "" {
		return MissingField("email")
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return InvalidField("email", "invalid email format")
	}
	return nil
}

// validatePassword checks password meets requirements.
// Requires 8-1024 characters with at least one letter and one digit.
func validatePassword(password string) error {
	if password == "" {
		return MissingField("password")
	}
	if len(password) < 8 {
		return InvalidField("password", "must be at least 8 characters")
	}
	if len(password) > 1024 {
		return InvalidField("password", "must be at most 1024 characters")
	}
	var hasLetter, hasDigit bool
	for _, r := range password {
		if unicode.IsLetter(r) {
			hasLetter = true
		}
		if unicode.IsDigit(r) {
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return InvalidField("password", "must contain at least one letter and one digit")
	}
	return nil
}

// --- Auth ---

// LoginRequest is a request to log in.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Validate validates the login request fields.
func (r *LoginRequest) Validate() error {
	if err := validateEmail(r.Email); err != nil {
		return err
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
	if err := validateEmail(r.Email); err != nil {
		return err
	}
	if err := validatePassword(r.Password); err != nil {
		return err
	}
	if r.Name == "" {
		return MissingField("name")
	}
	return nil
}

// GetMeRequest is a request to get current user info.
type GetMeRequest struct{}

// Validate is a no-op for GetMeRequest.
func (r *GetMeRequest) Validate() error {
	return nil
}

// --- Pages ---

// ListPagesRequest is a request to list all pages.
type ListPagesRequest struct {
	WsID jsonldb.ID `path:"wsID" tstype:"-"`
}

// Validate validates the list pages request fields.
func (r *ListPagesRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	return nil
}

// GetPageRequest is a request to get a page.
type GetPageRequest struct {
	WsID jsonldb.ID `path:"wsID" tstype:"-"`
	ID   jsonldb.ID `path:"id" tstype:"-"`
}

// Validate validates the get page request fields.
func (r *GetPageRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	if r.ID.IsZero() {
		return MissingField("id")
	}
	return nil
}

// CreatePageRequest is a request to create a page.
type CreatePageRequest struct {
	WsID    jsonldb.ID `path:"wsID" tstype:"-"`
	Title   string     `json:"title"`
	Content string     `json:"content"`
}

// Validate validates the create page request fields.
func (r *CreatePageRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	if r.Title == "" {
		return MissingField("title")
	}
	return nil
}

// UpdatePageRequest is a request to update a page.
type UpdatePageRequest struct {
	WsID    jsonldb.ID `path:"wsID" tstype:"-"`
	ID      jsonldb.ID `path:"id" tstype:"-"`
	Title   string     `json:"title"`
	Content string     `json:"content"`
}

// Validate validates the update page request fields.
func (r *UpdatePageRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	if r.ID.IsZero() {
		return MissingField("id")
	}
	if r.Title == "" {
		return MissingField("title")
	}
	return nil
}

// DeletePageRequest is a request to delete a page.
type DeletePageRequest struct {
	WsID jsonldb.ID `path:"wsID" tstype:"-"`
	ID   jsonldb.ID `path:"id" tstype:"-"`
}

// Validate validates the delete page request fields.
func (r *DeletePageRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	if r.ID.IsZero() {
		return MissingField("id")
	}
	return nil
}

// ListPageVersionsRequest is a request to list page version history.
type ListPageVersionsRequest struct {
	WsID  jsonldb.ID `path:"wsID" tstype:"-"`
	ID    jsonldb.ID `path:"id" tstype:"-"`
	Limit int        `query:"limit"` // Max commits to return (1-1000, default 1000).
}

// Validate validates the list page versions request fields.
func (r *ListPageVersionsRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	if r.ID.IsZero() {
		return MissingField("id")
	}
	return nil
}

// GetPageVersionRequest is a request to get a specific page version.
type GetPageVersionRequest struct {
	WsID jsonldb.ID `path:"wsID" tstype:"-"`
	ID   jsonldb.ID `path:"id" tstype:"-"`
	Hash string     `path:"hash" tstype:"-"`
}

// Validate validates the get page version request fields.
func (r *GetPageVersionRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	if r.ID.IsZero() {
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
	WsID jsonldb.ID `path:"wsID" tstype:"-"`
}

// Validate validates the list tables request fields.
func (r *ListTablesRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	return nil
}

// GetTableRequest is a request to get a table.
type GetTableRequest struct {
	WsID jsonldb.ID `path:"wsID" tstype:"-"`
	ID   jsonldb.ID `path:"id" tstype:"-"`
}

// Validate validates the get table request fields.
func (r *GetTableRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	if r.ID.IsZero() {
		return MissingField("id")
	}
	return nil
}

// CreateTableRequest is a request to create a table.
type CreateTableRequest struct {
	WsID       jsonldb.ID `path:"wsID" tstype:"-"`
	Title      string     `json:"title"`
	Properties []Property `json:"properties"`
}

// Validate validates the create table request fields.
func (r *CreateTableRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	if r.Title == "" {
		return MissingField("title")
	}
	return nil
}

// UpdateTableRequest is a request to update a table.
type UpdateTableRequest struct {
	WsID       jsonldb.ID `path:"wsID" tstype:"-"`
	ID         jsonldb.ID `path:"id" tstype:"-"`
	Title      string     `json:"title"`
	Properties []Property `json:"properties"`
}

// Validate validates the update table request fields.
func (r *UpdateTableRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	if r.ID.IsZero() {
		return MissingField("id")
	}
	if r.Title == "" {
		return MissingField("title")
	}
	return nil
}

// DeleteTableRequest is a request to delete a table.
type DeleteTableRequest struct {
	WsID jsonldb.ID `path:"wsID" tstype:"-"`
	ID   jsonldb.ID `path:"id" tstype:"-"`
}

// Validate validates the delete table request fields.
func (r *DeleteTableRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	if r.ID.IsZero() {
		return MissingField("id")
	}
	return nil
}

// ListRecordsRequest is a request to list records in a table.
type ListRecordsRequest struct {
	WsID   jsonldb.ID `path:"wsID" tstype:"-"`
	ID     jsonldb.ID `path:"id" tstype:"-"`
	Offset int        `query:"offset"`
	Limit  int        `query:"limit"`
}

// Validate validates the list records request fields.
func (r *ListRecordsRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	if r.ID.IsZero() {
		return MissingField("id")
	}
	return nil
}

// CreateRecordRequest is a request to create a record.
type CreateRecordRequest struct {
	WsID jsonldb.ID     `path:"wsID" tstype:"-"`
	ID   jsonldb.ID     `path:"id" tstype:"-"`
	Data map[string]any `json:"data"`
}

// Validate validates the create record request fields.
func (r *CreateRecordRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	if r.ID.IsZero() {
		return MissingField("id")
	}
	return nil
}

// UpdateRecordRequest is a request to update a record.
type UpdateRecordRequest struct {
	WsID jsonldb.ID     `path:"wsID" tstype:"-"`
	ID   jsonldb.ID     `path:"id" tstype:"-"`
	RID  jsonldb.ID     `path:"rid" tstype:"-"`
	Data map[string]any `json:"data"`
}

// Validate validates the update record request fields.
func (r *UpdateRecordRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	if r.ID.IsZero() {
		return MissingField("id")
	}
	if r.RID.IsZero() {
		return MissingField("rid")
	}
	return nil
}

// GetRecordRequest is a request to get a record.
type GetRecordRequest struct {
	WsID jsonldb.ID `path:"wsID" tstype:"-"`
	ID   jsonldb.ID `path:"id" tstype:"-"`
	RID  jsonldb.ID `path:"rid" tstype:"-"`
}

// Validate validates the get record request fields.
func (r *GetRecordRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	if r.ID.IsZero() {
		return MissingField("id")
	}
	if r.RID.IsZero() {
		return MissingField("rid")
	}
	return nil
}

// DeleteRecordRequest is a request to delete a record.
type DeleteRecordRequest struct {
	WsID jsonldb.ID `path:"wsID" tstype:"-"`
	ID   jsonldb.ID `path:"id" tstype:"-"`
	RID  jsonldb.ID `path:"rid" tstype:"-"`
}

// Validate validates the delete record request fields.
func (r *DeleteRecordRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	if r.ID.IsZero() {
		return MissingField("id")
	}
	if r.RID.IsZero() {
		return MissingField("rid")
	}
	return nil
}

// --- Nodes ---

// ListNodesRequest is a request to list nodes.
type ListNodesRequest struct {
	WsID jsonldb.ID `path:"wsID" tstype:"-"`
}

// Validate validates the list nodes request fields.
func (r *ListNodesRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	return nil
}

// GetNodeRequest is a request to get a node.
type GetNodeRequest struct {
	WsID jsonldb.ID `path:"wsID" tstype:"-"`
	ID   jsonldb.ID `path:"id" tstype:"-"`
}

// Validate validates the get node request fields.
func (r *GetNodeRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	if r.ID.IsZero() {
		return MissingField("id")
	}
	return nil
}

// CreateNodeRequest is a request to create a node.
type CreateNodeRequest struct {
	WsID     jsonldb.ID `path:"wsID" tstype:"-"`
	ParentID jsonldb.ID `json:"parent_id,omitempty"`
	Title    string     `json:"title"`
	Type     NodeType   `json:"type"`
}

// Validate validates the create node request fields.
func (r *CreateNodeRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
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
	WsID   jsonldb.ID `path:"wsID" tstype:"-"`
	PageID jsonldb.ID `path:"id" tstype:"-"`
}

// Validate validates the list page assets request fields.
func (r *ListPageAssetsRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	if r.PageID.IsZero() {
		return MissingField("id")
	}
	return nil
}

// UploadPageAssetRequest is a request to upload an asset to a page.
type UploadPageAssetRequest struct {
	WsID   jsonldb.ID `path:"wsID" tstype:"-"`
	PageID jsonldb.ID `path:"id" tstype:"-"`
}

// Validate validates the upload page asset request fields.
func (r *UploadPageAssetRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	if r.PageID.IsZero() {
		return MissingField("id")
	}
	return nil
}

// DeletePageAssetRequest is a request to delete an asset from a page.
type DeletePageAssetRequest struct {
	WsID      jsonldb.ID `path:"wsID" tstype:"-"`
	PageID    jsonldb.ID `path:"id" tstype:"-"`
	AssetName string     `path:"name" tstype:"-"`
}

// Validate validates the delete page asset request fields.
func (r *DeletePageAssetRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	if r.PageID.IsZero() {
		return MissingField("id")
	}
	if r.AssetName == "" {
		return MissingField("assetName")
	}
	return nil
}

// ServeAssetRequest is a request to serve an asset file directly.
type ServeAssetRequest struct {
	WsID      jsonldb.ID `path:"wsID" tstype:"-"`
	PageID    jsonldb.ID `path:"id" tstype:"-"`
	AssetName string     `path:"name" tstype:"-"`
}

// Validate validates the serve asset request fields.
func (r *ServeAssetRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	if r.PageID.IsZero() {
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
	WsID        jsonldb.ID `path:"wsID" tstype:"-"`
	Query       string     `json:"query"`
	Limit       int        `json:"limit,omitempty"`
	MatchTitle  bool       `json:"match_title,omitempty"`
	MatchBody   bool       `json:"match_body,omitempty"`
	MatchFields bool       `json:"match_fields,omitempty"`
}

// Validate validates the search request fields.
func (r *SearchRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	if r.Query == "" {
		return MissingField("query")
	}
	return nil
}

// --- Invitations ---

// CreateOrgInvitationRequest is a request to create an organization invitation.
type CreateOrgInvitationRequest struct {
	OrgID jsonldb.ID       `path:"orgID" tstype:"-"`
	Email string           `json:"email"`
	Role  OrganizationRole `json:"role"`
}

// CreateWSInvitationRequest is a request to create a workspace invitation.
type CreateWSInvitationRequest struct {
	WsID  jsonldb.ID    `path:"wsID" tstype:"-"`
	Email string        `json:"email"`
	Role  WorkspaceRole `json:"role"`
}

// Validate validates the create organization invitation request fields.
func (r *CreateOrgInvitationRequest) Validate() error {
	if r.OrgID.IsZero() {
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

// Validate validates the create workspace invitation request fields.
func (r *CreateWSInvitationRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	if r.Email == "" {
		return MissingField("email")
	}
	if r.Role == "" {
		return MissingField("role")
	}
	return nil
}

// ListOrgInvitationsRequest is a request to list invitations for an organization.
type ListOrgInvitationsRequest struct {
	OrgID jsonldb.ID `path:"orgID" tstype:"-"`
}

// Validate validates the list org invitations request fields.
func (r *ListOrgInvitationsRequest) Validate() error {
	if r.OrgID.IsZero() {
		return MissingField("orgID")
	}
	return nil
}

// ListWSInvitationsRequest is a request to list invitations for a workspace.
type ListWSInvitationsRequest struct {
	WsID jsonldb.ID `path:"wsID" tstype:"-"`
}

// Validate validates the list workspace invitations request fields.
func (r *ListWSInvitationsRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
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
	OrgID jsonldb.ID `json:"org_id"`
}

// Validate validates the switch org request fields.
func (r *SwitchOrgRequest) Validate() error {
	if r.OrgID.IsZero() {
		return MissingField("org_id")
	}
	return nil
}

// SwitchWorkspaceRequest is a request to switch active workspace.
type SwitchWorkspaceRequest struct {
	WsID jsonldb.ID `json:"ws_id"`
}

// Validate validates the switch workspace request fields.
func (r *SwitchWorkspaceRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("ws_id")
	}
	return nil
}

// UpdateWSMembershipSettingsRequest is a request to update user preferences within a workspace.
type UpdateWSMembershipSettingsRequest struct {
	WsID     jsonldb.ID                  `path:"wsID" tstype:"-"`
	Settings WorkspaceMembershipSettings `json:"settings"`
}

// Validate validates the update workspace membership settings request fields.
func (r *UpdateWSMembershipSettingsRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	return nil
}

// --- Organizations ---

// UpdateOrgPreferencesRequest is a request to update organization-wide preferences.
type UpdateOrgPreferencesRequest struct {
	OrgID    jsonldb.ID           `path:"orgID" tstype:"-"`
	Settings OrganizationSettings `json:"settings"`
}

// Validate validates the update org preferences request fields.
func (r *UpdateOrgPreferencesRequest) Validate() error {
	if r.OrgID.IsZero() {
		return MissingField("orgID")
	}
	return nil
}

// GetOrganizationRequest is a request to get organization details.
type GetOrganizationRequest struct {
	OrgID jsonldb.ID `path:"orgID" tstype:"-"`
}

// Validate validates the get organization request fields.
func (r *GetOrganizationRequest) Validate() error {
	if r.OrgID.IsZero() {
		return MissingField("orgID")
	}
	return nil
}

// UpdateOrganizationRequest is a request to update organization details.
type UpdateOrganizationRequest struct {
	OrgID jsonldb.ID `path:"orgID" tstype:"-"`
	Name  string     `json:"name,omitempty"`
}

// Validate validates the update organization request fields.
func (r *UpdateOrganizationRequest) Validate() error {
	if r.OrgID.IsZero() {
		return MissingField("orgID")
	}
	if r.Name == "" {
		return MissingField("name")
	}
	return nil
}

// CreateOrganizationRequest is a request to create a new organization.
type CreateOrganizationRequest struct {
	Name string `json:"name"`
}

// Validate validates the create organization request fields.
func (r *CreateOrganizationRequest) Validate() error {
	if r.Name == "" {
		return MissingField("name")
	}
	return nil
}

// CreateWorkspaceRequest is a request to create a new workspace within an organization.
type CreateWorkspaceRequest struct {
	OrgID jsonldb.ID `path:"orgID" tstype:"-"`
	Name  string     `json:"name"`
}

// Validate validates the create workspace request fields.
func (r *CreateWorkspaceRequest) Validate() error {
	if r.OrgID.IsZero() {
		return MissingField("orgID")
	}
	if r.Name == "" {
		return MissingField("name")
	}
	return nil
}

// --- Git Remotes ---

// GetGitRemoteRequest is a request to get the git remote for a workspace.
type GetGitRemoteRequest struct {
	WsID jsonldb.ID `path:"wsID" tstype:"-"`
}

// Validate validates the get git remote request fields.
func (r *GetGitRemoteRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	return nil
}

// UpdateGitRemoteRequest is a request to set (create or update) the git remote for a workspace.
type UpdateGitRemoteRequest struct {
	WsID     jsonldb.ID `path:"wsID" tstype:"-"`
	URL      string     `json:"url"`
	Type     string     `json:"type"`      // github, gitlab, custom
	AuthType string     `json:"auth_type"` // token, ssh
	Token    string     `json:"token,omitempty"`
}

// Validate validates the set git remote request fields.
func (r *UpdateGitRemoteRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
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

// DeleteGitRequest is a request to delete the git remote for a workspace.
type DeleteGitRequest struct {
	WsID jsonldb.ID `path:"wsID" tstype:"-"`
}

// Validate validates the delete git remote request fields.
func (r *DeleteGitRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	return nil
}

// PushGitRequest is a request to push to the git remote.
type PushGitRequest struct {
	WsID jsonldb.ID `path:"wsID" tstype:"-"`
}

// Validate validates the push git remote request fields.
func (r *PushGitRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
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

// --- OAuth Providers ---

// ProvidersRequest is a request to list configured OAuth providers.
type ProvidersRequest struct{}

// Validate is a no-op for ProvidersRequest.
func (r *ProvidersRequest) Validate() error {
	return nil
}

// --- Sessions ---

// LogoutRequest is a request to logout (revoke current session).
type LogoutRequest struct{}

// Validate is a no-op for LogoutRequest.
func (r *LogoutRequest) Validate() error {
	return nil
}

// ListSessionsRequest is a request to list user's active sessions.
type ListSessionsRequest struct{}

// Validate is a no-op for ListSessionsRequest.
func (r *ListSessionsRequest) Validate() error {
	return nil
}

// RevokeSessionRequest is a request to revoke a specific session.
type RevokeSessionRequest struct {
	SessionID jsonldb.ID `json:"session_id"`
}

// Validate validates the revoke session request fields.
func (r *RevokeSessionRequest) Validate() error {
	if r.SessionID.IsZero() {
		return MissingField("session_id")
	}
	return nil
}

// RevokeAllSessionsRequest is a request to revoke all sessions (logout everywhere).
type RevokeAllSessionsRequest struct{}

// Validate is a no-op for RevokeAllSessionsRequest.
func (r *RevokeAllSessionsRequest) Validate() error {
	return nil
}

// --- Users ---

// ListUsersRequest is a request to list users.
type ListUsersRequest struct {
	OrgID jsonldb.ID `path:"orgID" tstype:"-"`
}

// Validate validates the list users request fields.
func (r *ListUsersRequest) Validate() error {
	if r.OrgID.IsZero() {
		return MissingField("orgID")
	}
	return nil
}

// UpdateOrgMemberRoleRequest is a request to update a user's organization role.
type UpdateOrgMemberRoleRequest struct {
	OrgID  jsonldb.ID       `path:"orgID" tstype:"-"`
	UserID jsonldb.ID       `json:"user_id"`
	Role   OrganizationRole `json:"role"`
}

// Validate validates the update org member role request fields.
func (r *UpdateOrgMemberRoleRequest) Validate() error {
	if r.OrgID.IsZero() {
		return MissingField("orgID")
	}
	if r.UserID.IsZero() {
		return MissingField("user_id")
	}
	if r.Role == "" {
		return MissingField("role")
	}
	return nil
}

// UpdateWSMemberRoleRequest is a request to update a user's workspace role.
type UpdateWSMemberRoleRequest struct {
	WsID   jsonldb.ID    `path:"wsID" tstype:"-"`
	UserID jsonldb.ID    `json:"user_id"`
	Role   WorkspaceRole `json:"role"`
}

// Validate validates the update workspace member role request fields.
func (r *UpdateWSMemberRoleRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	if r.UserID.IsZero() {
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
