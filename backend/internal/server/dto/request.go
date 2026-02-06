// Defines API request payloads and validation logic.

package dto

import (
	"net/mail"
	"strconv"
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

// validateFilters validates a slice of filters recursively.
func validateFilters(filters []Filter) error {
	seen := make(map[string]struct{})
	for i, f := range filters {
		if err := validateFilter(&f, i); err != nil {
			return err
		}
		// Check for duplicate properties at the same level
		if f.Property != "" {
			if _, ok := seen[f.Property]; ok {
				return InvalidField("filters", "duplicate property: "+f.Property)
			}
			seen[f.Property] = struct{}{}
		}
	}
	return nil
}

// validateFilter validates a single filter and its nested filters.
func validateFilter(f *Filter, index int) error {
	// A filter must have either a property+operator or nested And/Or conditions
	hasLeaf := f.Property != "" || f.Operator != ""
	hasNested := len(f.And) > 0 || len(f.Or) > 0

	if hasLeaf {
		if f.Property == "" {
			return InvalidField("filters", "filter at index "+strconv.Itoa(index)+" has operator but no property")
		}
		if f.Operator == "" {
			return InvalidField("filters", "filter at index "+strconv.Itoa(index)+" has property but no operator")
		}
		if err := f.Operator.Validate(); err != nil {
			return InvalidField("filters["+strconv.Itoa(index)+"].operator", err.Error())
		}
	}

	// Validate nested filters
	if err := validateFilters(f.And); err != nil {
		return err
	}
	if err := validateFilters(f.Or); err != nil {
		return err
	}

	// Must have at least one condition
	if !hasLeaf && !hasNested {
		return InvalidField("filters", "filter at index "+strconv.Itoa(index)+" has no condition")
	}

	return nil
}

// validateSorts validates a slice of sorts.
func validateSorts(sorts []Sort) error {
	seen := make(map[string]struct{})
	for i, s := range sorts {
		if s.Property == "" {
			return InvalidField("sorts", "sort at index "+strconv.Itoa(i)+" missing property")
		}
		if s.Direction != SortAsc && s.Direction != SortDesc {
			return InvalidField("sorts", "sort at index "+strconv.Itoa(i)+" has invalid direction: "+string(s.Direction))
		}
		if _, ok := seen[s.Property]; ok {
			return InvalidField("sorts", "duplicate property: "+s.Property)
		}
		seen[s.Property] = struct{}{}
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

// --- Node Content (Page) ---

// CreatePageRequest is a request to create a page under a parent node.
// The parent ID is in the path ({id}); use "0" for root.
type CreatePageRequest struct {
	WsID     jsonldb.ID `path:"wsID" tstype:"-"`
	ParentID jsonldb.ID `path:"id" tstype:"-"` // Parent node ID; 0 = root
	Title    string     `json:"title"`
	Content  string     `json:"content,omitempty"`
}

// Validate validates the create page request fields.
func (r *CreatePageRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	// ParentID can be zero (root)
	if r.Title == "" {
		return MissingField("title")
	}
	return nil
}

// GetPageRequest is a request to get a page's content.
type GetPageRequest struct {
	WsID jsonldb.ID `path:"wsID" tstype:"-"`
	ID   jsonldb.ID `path:"id" tstype:"-"`
}

// Validate validates the get page request fields.
func (r *GetPageRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	// ID can be zero (root)
	return nil
}

// UpdatePageRequest is a request to update a page's content.
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
	// ID can be zero (root)
	if r.Title == "" {
		return MissingField("title")
	}
	return nil
}

// DeletePageRequest is a request to delete a page from a node.
// This removes the index.md but keeps the node directory if table data exists.
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
		return InvalidField("id", "cannot delete root page")
	}
	return nil
}

// DeleteNodeRequest is a request to delete a node.
type DeleteNodeRequest struct {
	WsID jsonldb.ID `path:"wsID" tstype:"-"`
	ID   jsonldb.ID `path:"id" tstype:"-"`
}

// Validate validates the delete node request fields.
func (r *DeleteNodeRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	if r.ID.IsZero() {
		return InvalidField("id", "cannot delete root node")
	}
	return nil
}

// ListNodeVersionsRequest is a request to list node version history.
type ListNodeVersionsRequest struct {
	WsID  jsonldb.ID `path:"wsID" tstype:"-"`
	ID    jsonldb.ID `path:"id" tstype:"-"` // Node ID; 0 = root
	Limit int        `query:"limit"`        // Max commits to return (1-1000, default 1000).
}

// Validate validates the list node versions request fields.
func (r *ListNodeVersionsRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	// ID can be zero (root)
	return nil
}

// GetNodeVersionRequest is a request to get a specific node version.
type GetNodeVersionRequest struct {
	WsID jsonldb.ID `path:"wsID" tstype:"-"`
	ID   jsonldb.ID `path:"id" tstype:"-"` // Node ID; 0 = root
	Hash string     `path:"hash" tstype:"-"`
}

// Validate validates the get node version request fields.
func (r *GetNodeVersionRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	// ID can be zero (root)
	if r.Hash == "" {
		return MissingField("hash")
	}
	return nil
}

// --- Tables ---

// GetTableRequest is a request to get a table.
// Now used for /nodes/{id}/table endpoint.
type GetTableRequest struct {
	WsID jsonldb.ID `path:"wsID" tstype:"-"`
	ID   jsonldb.ID `path:"id" tstype:"-"`
}

// Validate validates the get table request fields.
func (r *GetTableRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	// ID can be zero (root)
	return nil
}

// CreateTableRequest is a request to create a table under a parent node.
// The parent ID is in the path ({id}); use "0" for root.
type CreateTableRequest struct {
	WsID       jsonldb.ID `path:"wsID" tstype:"-"`
	ParentID   jsonldb.ID `path:"id" tstype:"-"` // Parent node ID; 0 = root
	Title      string     `json:"title"`
	Properties []Property `json:"properties"`
}

// Validate validates the create table request fields.
func (r *CreateTableRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	// ParentID can be zero (root)
	if r.Title == "" {
		return MissingField("title")
	}
	return nil
}

// UpdateTableRequest is a request to update a table.
// Now used for /nodes/{id}/table endpoint.
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
	// ID can be zero (root)
	if r.Title == "" {
		return MissingField("title")
	}
	return nil
}

// DeleteTableRequest is a request to delete a table from a node.
// This removes the metadata.json and data.jsonl but keeps the node directory if page exists.
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
		return InvalidField("id", "cannot delete root table")
	}
	return nil
}

// CreateViewRequest is a request to create a new view for a table.
type CreateViewRequest struct {
	WsID   jsonldb.ID `path:"wsID" tstype:"-"`
	NodeID jsonldb.ID `path:"id" tstype:"-"`
	Name   string     `json:"name"`
	Type   ViewType   `json:"type"`
}

// Validate validates the create view request fields.
func (r *CreateViewRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	if r.NodeID.IsZero() {
		return MissingField("id")
	}
	if r.Name == "" {
		return MissingField("name")
	}
	if r.Type == "" {
		return MissingField("type")
	}
	if err := r.Type.Validate(); err != nil {
		return InvalidField("type", err.Error())
	}
	return nil
}

// UpdateViewRequest is a request to update an existing view.
type UpdateViewRequest struct {
	WsID    jsonldb.ID   `path:"wsID" tstype:"-"`
	NodeID  jsonldb.ID   `path:"id" tstype:"-"`
	ViewID  jsonldb.ID   `path:"viewID" tstype:"-"`
	Name    string       `json:"name,omitempty"`
	Type    ViewType     `json:"type,omitempty"`
	Columns []ViewColumn `json:"columns,omitempty"`
	Filters []Filter     `json:"filters,omitempty"`
	Sorts   []Sort       `json:"sorts,omitempty"`
	Groups  []Group      `json:"groups,omitempty"`
}

// Validate validates the update view request fields.
func (r *UpdateViewRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	if r.NodeID.IsZero() {
		return MissingField("id")
	}
	if r.ViewID.IsZero() {
		return MissingField("viewID")
	}
	if r.Type != "" {
		if err := r.Type.Validate(); err != nil {
			return InvalidField("type", err.Error())
		}
	}
	if err := validateFilters(r.Filters); err != nil {
		return err
	}
	if err := validateSorts(r.Sorts); err != nil {
		return err
	}
	return nil
}

// DeleteViewRequest is a request to delete a view.
type DeleteViewRequest struct {
	WsID   jsonldb.ID `path:"wsID" tstype:"-"`
	NodeID jsonldb.ID `path:"id" tstype:"-"`
	ViewID jsonldb.ID `path:"viewID" tstype:"-"`
}

// Validate validates the delete view request fields.
func (r *DeleteViewRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	if r.NodeID.IsZero() {
		return MissingField("id")
	}
	if r.ViewID.IsZero() {
		return MissingField("viewID")
	}
	return nil
}

// ListRecordsRequest is a request to list records in a table.
// Now used for /nodes/{id}/table/records endpoint.
type ListRecordsRequest struct {
	WsID    jsonldb.ID `path:"wsID" tstype:"-"`
	ID      jsonldb.ID `path:"id" tstype:"-"` // Node ID; 0 = root
	ViewID  jsonldb.ID `query:"view_id"`      // Optional: apply saved view configuration
	Filters string     `query:"filters"`      // Optional: JSON-encoded ad-hoc filters
	Sorts   string     `query:"sorts"`        // Optional: JSON-encoded ad-hoc sorts
	Offset  int        `query:"offset"`
	Limit   int        `query:"limit"`
}

// Validate validates the list records request fields.
func (r *ListRecordsRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	// ID can be zero (root)
	return nil
}

// CreateRecordRequest is a request to create a record.
// Now used for /nodes/{id}/table/records/create endpoint.
type CreateRecordRequest struct {
	WsID jsonldb.ID     `path:"wsID" tstype:"-"`
	ID   jsonldb.ID     `path:"id" tstype:"-"` // Node ID; 0 = root
	Data map[string]any `json:"data"`
}

// Validate validates the create record request fields.
func (r *CreateRecordRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	// ID can be zero (root)
	return nil
}

// UpdateRecordRequest is a request to update a record.
// Now used for /nodes/{id}/table/records/{rid} endpoint.
type UpdateRecordRequest struct {
	WsID jsonldb.ID     `path:"wsID" tstype:"-"`
	ID   jsonldb.ID     `path:"id" tstype:"-"` // Node ID; 0 = root
	RID  jsonldb.ID     `path:"rid" tstype:"-"`
	Data map[string]any `json:"data"`
}

// Validate validates the update record request fields.
func (r *UpdateRecordRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	// ID can be zero (root)
	if r.RID.IsZero() {
		return MissingField("rid")
	}
	return nil
}

// GetRecordRequest is a request to get a record.
// Now used for /nodes/{id}/table/records/{rid} endpoint.
type GetRecordRequest struct {
	WsID jsonldb.ID `path:"wsID" tstype:"-"`
	ID   jsonldb.ID `path:"id" tstype:"-"` // Node ID; 0 = root
	RID  jsonldb.ID `path:"rid" tstype:"-"`
}

// Validate validates the get record request fields.
func (r *GetRecordRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	// ID can be zero (root)
	if r.RID.IsZero() {
		return MissingField("rid")
	}
	return nil
}

// DeleteRecordRequest is a request to delete a record.
// Now used for /nodes/{id}/table/records/{rid}/delete endpoint.
type DeleteRecordRequest struct {
	WsID jsonldb.ID `path:"wsID" tstype:"-"`
	ID   jsonldb.ID `path:"id" tstype:"-"` // Node ID; 0 = root
	RID  jsonldb.ID `path:"rid" tstype:"-"`
}

// Validate validates the delete record request fields.
func (r *DeleteRecordRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	// ID can be zero (root)
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

// GetNodeTitlesRequest is a request to get titles for multiple nodes.
type GetNodeTitlesRequest struct {
	WsID jsonldb.ID     `path:"wsID" tstype:"-"`
	IDs  jsonldb.IDList `query:"ids" tstype:"string"` // Comma-separated node IDs
}

// Validate validates the get node titles request fields.
func (r *GetNodeTitlesRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	if len(r.IDs) == 0 {
		return MissingField("ids")
	}
	return nil
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
	// ID can be zero (root node)
	return nil
}

// ListNodeChildrenRequest is a request to list children of a node.
type ListNodeChildrenRequest struct {
	WsID     jsonldb.ID `path:"wsID" tstype:"-"`
	ParentID jsonldb.ID `path:"id" tstype:"-"` // Parent node ID; 0 = root
}

// Validate validates the list children request fields.
func (r *ListNodeChildrenRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	// ParentID can be zero (list root children)
	return nil
}

// --- Assets ---

// ListNodeAssetsRequest is a request to list assets in a node.
type ListNodeAssetsRequest struct {
	WsID   jsonldb.ID `path:"wsID" tstype:"-"`
	NodeID jsonldb.ID `path:"id" tstype:"-"` // Node ID; 0 = root
}

// Validate validates the list node assets request fields.
func (r *ListNodeAssetsRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	// NodeID can be zero (root)
	return nil
}

// UploadNodeAssetRequest is a request to upload an asset to a node.
type UploadNodeAssetRequest struct {
	WsID   jsonldb.ID `path:"wsID" tstype:"-"`
	NodeID jsonldb.ID `path:"id" tstype:"-"` // Node ID; 0 = root
}

// Validate validates the upload node asset request fields.
func (r *UploadNodeAssetRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	// NodeID can be zero (root)
	return nil
}

// DeleteNodeAssetRequest is a request to delete an asset from a node.
type DeleteNodeAssetRequest struct {
	WsID      jsonldb.ID `path:"wsID" tstype:"-"`
	NodeID    jsonldb.ID `path:"id" tstype:"-"` // Node ID; 0 = root
	AssetName string     `path:"name" tstype:"-"`
}

// Validate validates the delete node asset request fields.
func (r *DeleteNodeAssetRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	// NodeID can be zero (root)
	if r.AssetName == "" {
		return MissingField("assetName")
	}
	return nil
}

// ServeAssetRequest is a request to serve an asset file directly.
type ServeAssetRequest struct {
	WsID      jsonldb.ID `path:"wsID" tstype:"-"`
	NodeID    jsonldb.ID `path:"id" tstype:"-"` // Node ID; 0 = root
	AssetName string     `path:"name" tstype:"-"`
}

// Validate validates the serve asset request fields.
func (r *ServeAssetRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	// NodeID can be zero (root)
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
	OrgID  jsonldb.ID       `path:"orgID" tstype:"-"`
	Email  string           `json:"email"`
	Role   OrganizationRole `json:"role"`
	Locale string           `json:"locale,omitempty"` // Optional: language for invitation email (en, fr, de, es)
}

// CreateWSInvitationRequest is a request to create a workspace invitation.
type CreateWSInvitationRequest struct {
	WsID   jsonldb.ID    `path:"wsID" tstype:"-"`
	Email  string        `json:"email"`
	Role   WorkspaceRole `json:"role"`
	Locale string        `json:"locale,omitempty"` // Optional: language for invitation email (en, fr, de, es)
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
	OrgID    jsonldb.ID            `path:"orgID" tstype:"-"`
	Settings *OrganizationSettings `json:"settings,omitempty"`
	Quotas   *OrganizationQuotas   `json:"quotas,omitempty"`
}

// Validate validates the update org preferences request fields.
func (r *UpdateOrgPreferencesRequest) Validate() error {
	if r.OrgID.IsZero() {
		return MissingField("orgID")
	}
	if r.Quotas != nil {
		if err := r.Quotas.Validate("quotas"); err != nil {
			return err
		}
		if r.Quotas.MaxWorkspacesPerOrg <= 0 {
			return InvalidField("quotas.max_workspaces_per_org", "must be positive")
		}
		if r.Quotas.MaxMembersPerOrg <= 0 {
			return InvalidField("quotas.max_members_per_org", "must be positive")
		}
		if r.Quotas.MaxMembersPerWorkspace <= 0 {
			return InvalidField("quotas.max_members_per_workspace", "must be positive")
		}
		if r.Quotas.MaxTotalStorageBytes <= 0 {
			return InvalidField("quotas.max_total_storage_bytes", "must be positive")
		}
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

// GetWorkspaceRequest is a request to get workspace details.
type GetWorkspaceRequest struct {
	WsID jsonldb.ID `path:"wsID" tstype:"-"`
}

// Validate validates the get workspace request fields.
func (r *GetWorkspaceRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	return nil
}

// UpdateWorkspaceRequest is a request to update workspace details.
type UpdateWorkspaceRequest struct {
	WsID   jsonldb.ID       `path:"wsID" tstype:"-"`
	Name   string           `json:"name,omitempty"`
	Quotas *WorkspaceQuotas `json:"quotas,omitempty"`
}

// Validate validates the update workspace request fields.
func (r *UpdateWorkspaceRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	// At least one field must be set
	if r.Name == "" && r.Quotas == nil {
		return MissingField("name or quotas")
	}
	if r.Quotas != nil {
		if err := r.Quotas.Validate("quotas"); err != nil {
			return err
		}
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

// --- Email Change ---

// ChangeEmailRequest is a request to change the user's email address.
type ChangeEmailRequest struct {
	NewEmail string `json:"new_email"`
	Password string `json:"password"` // Required for security verification
}

// --- Email Verification ---

// SendVerificationEmailRequest is a request to send a verification email.
type SendVerificationEmailRequest struct{}

// Validate is a no-op for SendVerificationEmailRequest.
func (r *SendVerificationEmailRequest) Validate() error {
	return nil
}

// VerifyEmailRequest is a request to verify an email via magic link token.
type VerifyEmailRequest struct {
	Token string `query:"token" tstype:"-"`
}

// Validate validates the verify email request fields.
func (r *VerifyEmailRequest) Validate() error {
	if r.Token == "" {
		return MissingField("token")
	}
	return nil
}

// Validate validates the change email request fields.
func (r *ChangeEmailRequest) Validate() error {
	if err := validateEmail(r.NewEmail); err != nil {
		return InvalidField("new_email", "invalid email format")
	}
	if r.Password == "" {
		return MissingField("password")
	}
	return nil
}

// --- OAuth Linking ---

// LinkOAuthAccountRequest is a request to initiate linking an OAuth provider.
type LinkOAuthAccountRequest struct {
	Provider OAuthProvider `json:"provider"`
}

// Validate validates the link OAuth account request fields.
func (r *LinkOAuthAccountRequest) Validate() error {
	if r.Provider == "" {
		return MissingField("provider")
	}
	return nil
}

// UnlinkOAuthAccountRequest is a request to unlink an OAuth provider.
type UnlinkOAuthAccountRequest struct {
	Provider OAuthProvider `json:"provider"`
}

// Validate validates the unlink OAuth account request fields.
func (r *UnlinkOAuthAccountRequest) Validate() error {
	if r.Provider == "" {
		return MissingField("provider")
	}
	return nil
}

// SetPasswordRequest is a request to set or change the user's password.
type SetPasswordRequest struct {
	CurrentPassword string `json:"current_password,omitempty"` // Required if user has a password
	NewPassword     string `json:"new_password"`
}

// Validate validates the set password request fields.
func (r *SetPasswordRequest) Validate() error {
	return validatePassword(r.NewPassword)
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

// AdminServerDetailRequest is a request to get admin dashboard data.
type AdminServerDetailRequest struct{}

// Validate is a no-op for AdminServerDetailRequest.
func (r *AdminServerDetailRequest) Validate() error {
	return nil
}

// --- Server Config ---

// ServerConfigRequest is a request to get server configuration.
type ServerConfigRequest struct{}

// Validate is a no-op for ServerConfigRequest.
func (r *ServerConfigRequest) Validate() error {
	return nil
}

// SMTPConfigUpdate contains SMTP configuration fields for updates.
type SMTPConfigUpdate struct {
	Host     string `json:"host"`
	Port     int32  `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"` // Empty preserves existing
	From     string `json:"from"`
}

// QuotasConfigUpdate contains quota configuration fields for updates.
type QuotasConfigUpdate struct {
	ResourceQuotas `tstype:",extends"`

	MaxRequestBodyBytes   int64 `json:"max_request_body_bytes"`
	MaxSessionsPerUser    int   `json:"max_sessions_per_user"`
	MaxOrganizations      int   `json:"max_organizations"`
	MaxWorkspaces         int   `json:"max_workspaces"`
	MaxUsers              int   `json:"max_users"`
	MaxTotalStorageBytes  int64 `json:"max_total_storage_bytes"`
	MaxEgressBandwidthBps int64 `json:"max_egress_bandwidth_bps"`
}

// RateLimitsConfigUpdate contains rate limit configuration fields for updates.
type RateLimitsConfigUpdate struct {
	AuthRatePerMin       int `json:"auth_rate_per_min"`
	WriteRatePerMin      int `json:"write_rate_per_min"`
	ReadAuthRatePerMin   int `json:"read_auth_rate_per_min"`
	ReadUnauthRatePerMin int `json:"read_unauth_rate_per_min"`
}

// UpdateServerConfigRequest is a request to update server configuration.
type UpdateServerConfigRequest struct {
	SMTP       *SMTPConfigUpdate       `json:"smtp,omitempty"`
	Quotas     *QuotasConfigUpdate     `json:"quotas,omitempty"`
	RateLimits *RateLimitsConfigUpdate `json:"rate_limits,omitempty"`
}

// Validate validates the update server config request fields.
func (r *UpdateServerConfigRequest) Validate() error {
	return nil
}

// --- Notion Import ---

// NotionImportRequest is a request to start a Notion import into a new workspace.
type NotionImportRequest struct {
	OrgID       jsonldb.ID `path:"orgID" tstype:"-"`
	NotionToken string     `json:"notion_token"`
}

// Validate validates the notion import request fields.
func (r *NotionImportRequest) Validate() error {
	if r.OrgID.IsZero() {
		return MissingField("orgID")
	}
	// workspace_name is optional - will be derived from Notion API if empty
	if r.NotionToken == "" {
		return MissingField("notion_token")
	}
	return nil
}

// NotionImportStatusRequest is a request to get the status of a Notion import.
type NotionImportStatusRequest struct {
	OrgID      jsonldb.ID `path:"orgID" tstype:"-"`
	ImportWsID jsonldb.ID `path:"importWsID" json:"-"`
}

// Validate validates the notion import status request fields.
func (r *NotionImportStatusRequest) Validate() error {
	if r.OrgID.IsZero() {
		return MissingField("orgID")
	}
	if r.ImportWsID.IsZero() {
		return MissingField("importWsID")
	}
	return nil
}

// NotionImportCancelRequest is a request to cancel a running Notion import.
type NotionImportCancelRequest struct {
	WsID jsonldb.ID `path:"wsID" tstype:"-"`
}

// Validate validates the notion import cancel request fields.
func (r *NotionImportCancelRequest) Validate() error {
	if r.WsID.IsZero() {
		return MissingField("wsID")
	}
	return nil
}
