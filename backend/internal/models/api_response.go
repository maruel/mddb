package models

import (
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
)

// UserResponse is the API representation of a user.
type UserResponse struct {
	ID              string               `json:"id" jsonschema:"description=Unique user identifier"`
	Email           string               `json:"email" jsonschema:"description=User email address"`
	Name            string               `json:"name" jsonschema:"description=User display name"`
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
	Quotas     Quota                `json:"quotas" jsonschema:"description=Resource limits for the organization"`
	Settings   OrganizationSettings `json:"settings" jsonschema:"description=Organization-wide configuration"`
	Onboarding OnboardingState      `json:"onboarding" jsonschema:"description=Initial setup progress tracking"`
	Created    string               `json:"created" jsonschema:"description=Organization creation timestamp (RFC3339)"`
}

// GitRemoteResponse is the API representation of a git remote.
type GitRemoteResponse struct {
	ID             string `json:"id" jsonschema:"description=Unique git remote identifier"`
	OrganizationID string `json:"organization_id" jsonschema:"description=Organization this remote belongs to"`
	Name           string `json:"name" jsonschema:"description=Remote name (e.g. origin)"`
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

// formatTime formats a time.Time to RFC3339 string.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

// ToResponse converts a User to a UserResponse.
func (u *User) ToResponse() *UserResponse {
	return &UserResponse{
		ID:              u.ID.String(),
		Email:           u.Email,
		Name:            u.Name,
		OAuthIdentities: u.OAuthIdentities,
		Settings:        u.Settings,
		Created:         formatTime(u.Created),
		Modified:        formatTime(u.Modified),
	}
}

// ToResponse converts a Membership to a MembershipResponse.
func (m *Membership) ToResponse() *MembershipResponse {
	return &MembershipResponse{
		ID:             m.ID.String(),
		UserID:         m.UserID.String(),
		OrganizationID: m.OrganizationID.String(),
		Role:           m.Role,
		Settings:       m.Settings,
		Created:        formatTime(m.Created),
	}
}

// ToResponseWithOrgName converts a Membership to a MembershipResponse with org name.
func (m *Membership) ToResponseWithOrgName(orgName string) *MembershipResponse {
	resp := m.ToResponse()
	resp.OrganizationName = orgName
	return resp
}

// ToResponse converts an Invitation to an InvitationResponse.
func (i *Invitation) ToResponse() *InvitationResponse {
	return &InvitationResponse{
		ID:             i.ID.String(),
		Email:          i.Email,
		OrganizationID: i.OrganizationID.String(),
		Role:           i.Role,
		ExpiresAt:      formatTime(i.ExpiresAt),
		Created:        formatTime(i.Created),
	}
}

// ToResponse converts an Organization to an OrganizationResponse.
func (o *Organization) ToResponse() *OrganizationResponse {
	return &OrganizationResponse{
		ID:         o.ID.String(),
		Name:       o.Name,
		Quotas:     o.Quotas,
		Settings:   o.Settings,
		Onboarding: o.Onboarding,
		Created:    formatTime(o.Created),
	}
}

// ToResponse converts a GitRemote to a GitRemoteResponse.
func (g *GitRemote) ToResponse() *GitRemoteResponse {
	return &GitRemoteResponse{
		ID:             g.ID.String(),
		OrganizationID: g.OrganizationID.String(),
		Name:           g.Name,
		URL:            g.URL,
		Type:           g.Type,
		AuthType:       g.AuthType,
		Created:        formatTime(g.Created),
		LastSync:       formatTime(g.LastSync),
	}
}

// ToResponse converts a Node to a NodeResponse.
func (n *Node) ToResponse() *NodeResponse {
	resp := &NodeResponse{
		ID:         n.ID.String(),
		Title:      n.Title,
		Content:    n.Content,
		Properties: n.Properties,
		Created:    formatTime(n.Created),
		Modified:   formatTime(n.Modified),
		Tags:       n.Tags,
		FaviconURL: n.FaviconURL,
		Type:       n.Type,
	}
	if !n.ParentID.IsZero() {
		resp.ParentID = n.ParentID.String()
	}
	if len(n.Children) > 0 {
		resp.Children = make([]NodeResponse, 0, len(n.Children))
		for _, child := range n.Children {
			if child != nil {
				resp.Children = append(resp.Children, *child.ToResponse())
			}
		}
	}
	return resp
}

// ToResponse converts a DataRecord to a DataRecordResponse.
func (r *DataRecord) ToResponse() *DataRecordResponse {
	return &DataRecordResponse{
		ID:       r.ID.String(),
		Data:     r.Data,
		Created:  formatTime(r.Created),
		Modified: formatTime(r.Modified),
	}
}

// UserResponseBuilder helps construct UserResponse with context fields.
type UserResponseBuilder struct {
	resp *UserResponse
}

// NewUserResponseBuilder creates a new builder from a User.
func NewUserResponseBuilder(u *User) *UserResponseBuilder {
	return &UserResponseBuilder{resp: u.ToResponse()}
}

// WithMemberships sets the memberships.
func (b *UserResponseBuilder) WithMemberships(memberships []MembershipResponse) *UserResponseBuilder {
	b.resp.Memberships = memberships
	return b
}

// WithActiveContext sets the active organization context.
func (b *UserResponseBuilder) WithActiveContext(orgID jsonldb.ID, role UserRole, onboarding *OnboardingState) *UserResponseBuilder {
	if !orgID.IsZero() {
		b.resp.OrganizationID = orgID.String()
	}
	b.resp.Role = role
	b.resp.Onboarding = onboarding
	return b
}

// Build returns the constructed UserResponse.
func (b *UserResponseBuilder) Build() *UserResponse {
	return b.resp
}
