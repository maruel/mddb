package handlers

import (
	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// --- Entity to DTO conversions ---

func userToResponse(u *identity.User) *dto.UserResponse {
	identities := make([]dto.OAuthIdentity, len(u.OAuthIdentities))
	for i := range u.OAuthIdentities {
		identities[i] = oauthIdentityToDTO(&u.OAuthIdentities[i])
	}
	return &dto.UserResponse{
		ID:              u.ID,
		Email:           u.Email,
		Name:            u.Name,
		IsGlobalAdmin:   u.IsGlobalAdmin,
		OAuthIdentities: identities,
		Settings:        userSettingsToDTO(u.Settings),
		Created:         u.Created,
		Modified:        u.Modified,
	}
}

//nolint:unused // Reserved for future use
func orgMembershipToResponse(m *identity.OrganizationMembership) *dto.OrgMembershipResponse {
	return &dto.OrgMembershipResponse{
		ID:             m.ID,
		UserID:         m.UserID,
		OrganizationID: m.OrganizationID,
		Role:           dto.OrganizationRole(m.Role),
		Created:        m.Created,
	}
}

func wsMembershipToResponse(m *identity.WorkspaceMembership) *dto.WSMembershipResponse {
	return &dto.WSMembershipResponse{
		ID:          m.ID,
		UserID:      m.UserID,
		WorkspaceID: m.WorkspaceID,
		Role:        dto.WorkspaceRole(m.Role),
		Settings:    wsMembershipSettingsToDTO(m.Settings),
		Created:     m.Created,
	}
}

func orgInvitationToResponse(i *identity.OrganizationInvitation) *dto.OrgInvitationResponse {
	return &dto.OrgInvitationResponse{
		ID:             i.ID,
		Email:          i.Email,
		OrganizationID: i.OrganizationID,
		Role:           dto.OrganizationRole(i.Role),
		InvitedBy:      i.InvitedBy,
		ExpiresAt:      i.ExpiresAt,
		Created:        i.Created,
	}
}

func wsInvitationToResponse(i *identity.WorkspaceInvitation) *dto.WSInvitationResponse {
	return &dto.WSInvitationResponse{
		ID:          i.ID,
		Email:       i.Email,
		WorkspaceID: i.WorkspaceID,
		Role:        dto.WorkspaceRole(i.Role),
		InvitedBy:   i.InvitedBy,
		ExpiresAt:   i.ExpiresAt,
		Created:     i.Created,
	}
}

func organizationToResponse(o *identity.Organization, memberCount, workspaceCount int) *dto.OrganizationResponse {
	return &dto.OrganizationResponse{
		ID:             o.ID,
		Name:           o.Name,
		BillingEmail:   o.BillingEmail,
		Quotas:         organizationQuotasToDTO(o.Quotas),
		Settings:       organizationSettingsToDTO(o.Settings),
		MemberCount:    memberCount,
		WorkspaceCount: workspaceCount,
		Created:        o.Created,
	}
}

//nolint:unused // Reserved for future use
func workspaceToResponse(w *identity.Workspace, memberCount int) *dto.WorkspaceResponse {
	resp := &dto.WorkspaceResponse{
		ID:             w.ID,
		OrganizationID: w.OrganizationID,
		Name:           w.Name,
		Slug:           w.Slug,
		Quotas:         workspaceQuotasToDTO(w.Quotas),
		Settings:       workspaceSettingsToDTO(w.Settings),
		MemberCount:    memberCount,
		Created:        w.Created,
	}
	if !w.GitRemote.IsZero() {
		resp.GitRemote = gitRemoteToResponse(w.ID, &w.GitRemote)
	}
	return resp
}

func gitRemoteToResponse(wsID jsonldb.ID, g *identity.GitRemote) *dto.GitRemoteResponse {
	return &dto.GitRemoteResponse{
		WorkspaceID: wsID,
		URL:         g.URL,
		Type:        g.Type,
		AuthType:    g.AuthType,
		Created:     g.Created,
		LastSync:    g.LastSync,
	}
}

func nodeToResponse(n *content.Node) *dto.NodeResponse {
	resp := &dto.NodeResponse{
		ID:         n.ID,
		ParentID:   n.ParentID,
		Title:      n.Title,
		Content:    n.Content,
		Properties: propertiesToDTO(n.Properties),
		Created:    n.Created,
		Modified:   n.Modified,
		Tags:       n.Tags,
		FaviconURL: n.FaviconURL,
		Type:       dto.NodeType(n.Type),
	}
	if len(n.Children) > 0 {
		resp.Children = make([]dto.NodeResponse, 0, len(n.Children))
		for _, child := range n.Children {
			if child != nil {
				resp.Children = append(resp.Children, *nodeToResponse(child))
			}
		}
	}
	return resp
}

func dataRecordToResponse(r *content.DataRecord) *dto.DataRecordResponse {
	return &dto.DataRecordResponse{
		ID:       r.ID,
		Data:     r.Data,
		Created:  r.Created,
		Modified: r.Modified,
	}
}

func commitToDTO(c *git.Commit) *dto.Commit {
	if c == nil {
		return nil
	}
	return &dto.Commit{
		Hash:      c.Hash,
		Message:   c.Message,
		Timestamp: storage.ToTime(c.CommitDate),
	}
}

func commitsToDTO(commits []*git.Commit) []*dto.Commit {
	result := make([]*dto.Commit, len(commits))
	for i, c := range commits {
		result[i] = commitToDTO(c)
	}
	return result
}

func searchResultToDTO(r *content.SearchResult) dto.SearchResult {
	return dto.SearchResult{
		Type:     r.Type,
		NodeID:   r.NodeID.String(),
		RecordID: r.RecordID.String(),
		Title:    r.Title,
		Snippet:  r.Snippet,
		Score:    r.Score,
		Matches:  r.Matches,
		Modified: r.Modified,
	}
}

func searchResultsToDTO(results []content.SearchResult) []dto.SearchResult {
	dtoResults := make([]dto.SearchResult, len(results))
	for i := range results {
		dtoResults[i] = searchResultToDTO(&results[i])
	}
	return dtoResults
}

// --- Nested type conversions (entity -> dto) ---

func propertyToDTO(p content.Property) dto.Property {
	options := make([]dto.SelectOption, len(p.Options))
	for i, o := range p.Options {
		options[i] = dto.SelectOption{
			ID:    o.ID,
			Name:  o.Name,
			Color: o.Color,
		}
	}
	return dto.Property{
		Name:     p.Name,
		Type:     dto.PropertyType(p.Type),
		Required: p.Required,
		Options:  options,
	}
}

func propertiesToDTO(props []content.Property) []dto.Property {
	if props == nil {
		return nil
	}
	result := make([]dto.Property, len(props))
	for i, p := range props {
		result[i] = propertyToDTO(p)
	}
	return result
}

func userSettingsToDTO(s identity.UserSettings) dto.UserSettings {
	return dto.UserSettings{
		Theme:    s.Theme,
		Language: s.Language,
	}
}

func oauthIdentityToDTO(o *identity.OAuthIdentity) dto.OAuthIdentity {
	return dto.OAuthIdentity{
		Provider:   dto.OAuthProvider(o.Provider),
		ProviderID: o.ProviderID,
		Email:      o.Email,
		AvatarURL:  o.AvatarURL,
		LastLogin:  o.LastLogin,
	}
}

func wsMembershipSettingsToDTO(s identity.WorkspaceMembershipSettings) dto.WorkspaceMembershipSettings {
	return dto.WorkspaceMembershipSettings{
		Notifications: s.Notifications,
	}
}

func organizationQuotasToDTO(q identity.OrganizationQuotas) dto.OrganizationQuotas {
	return dto.OrganizationQuotas{
		MaxWorkspaces:          q.MaxWorkspaces,
		MaxMembersPerOrg:       q.MaxMembersPerOrg,
		MaxMembersPerWorkspace: q.MaxMembersPerWorkspace,
		MaxTotalStorageGB:      q.MaxTotalStorageGB,
	}
}

func workspaceQuotasToDTO(q identity.WorkspaceQuotas) dto.WorkspaceQuotas {
	return dto.WorkspaceQuotas{
		MaxPages:           q.MaxPages,
		MaxStorageMB:       q.MaxStorageMB,
		MaxRecordsPerTable: q.MaxRecordsPerTable,
		MaxAssetSizeMB:     q.MaxAssetSizeMB,
	}
}

func organizationSettingsToDTO(s identity.OrganizationSettings) dto.OrganizationSettings {
	return dto.OrganizationSettings{
		AllowedEmailDomains:    s.AllowedEmailDomains,
		RequireSSO:             s.RequireSSO,
		DefaultWorkspaceQuotas: workspaceQuotasToDTO(s.DefaultWorkspaceQuotas),
	}
}

//nolint:unused // Reserved for future use
func workspaceSettingsToDTO(s identity.WorkspaceSettings) dto.WorkspaceSettings {
	return dto.WorkspaceSettings{
		AllowedDomains: s.AllowedDomains,
		PublicAccess:   s.PublicAccess,
		GitAutoPush:    s.GitAutoPush,
	}
}

// --- DTO to Entity conversions (for requests) ---

func propertyToEntity(p dto.Property) content.Property {
	options := make([]content.SelectOption, len(p.Options))
	for i, o := range p.Options {
		options[i] = content.SelectOption{
			ID:    o.ID,
			Name:  o.Name,
			Color: o.Color,
		}
	}
	return content.Property{
		Name:     p.Name,
		Type:     content.PropertyType(p.Type),
		Required: p.Required,
		Options:  options,
	}
}

func propertiesToEntity(props []dto.Property) []content.Property {
	if props == nil {
		return nil
	}
	result := make([]content.Property, len(props))
	for i, p := range props {
		result[i] = propertyToEntity(p)
	}
	return result
}

func orgRoleToEntity(r dto.OrganizationRole) identity.OrganizationRole {
	return identity.OrganizationRole(r)
}

func wsRoleToEntity(r dto.WorkspaceRole) identity.WorkspaceRole {
	return identity.WorkspaceRole(r)
}

func wsMembershipSettingsToEntity(s dto.WorkspaceMembershipSettings) identity.WorkspaceMembershipSettings {
	return identity.WorkspaceMembershipSettings{
		Notifications: s.Notifications,
	}
}

func userSettingsToEntity(s dto.UserSettings) identity.UserSettings {
	return identity.UserSettings{
		Theme:    s.Theme,
		Language: s.Language,
	}
}

func organizationSettingsToEntity(s dto.OrganizationSettings) identity.OrganizationSettings {
	return identity.OrganizationSettings{
		AllowedEmailDomains:    s.AllowedEmailDomains,
		RequireSSO:             s.RequireSSO,
		DefaultWorkspaceQuotas: workspaceQuotasToEntity(s.DefaultWorkspaceQuotas),
	}
}

func workspaceQuotasToEntity(q dto.WorkspaceQuotas) identity.WorkspaceQuotas {
	return identity.WorkspaceQuotas{
		MaxPages:           q.MaxPages,
		MaxStorageMB:       q.MaxStorageMB,
		MaxRecordsPerTable: q.MaxRecordsPerTable,
		MaxAssetSizeMB:     q.MaxAssetSizeMB,
	}
}

//nolint:unused // Reserved for future use
func workspaceSettingsToEntity(s dto.WorkspaceSettings) identity.WorkspaceSettings {
	return identity.WorkspaceSettings{
		AllowedDomains: s.AllowedDomains,
		PublicAccess:   s.PublicAccess,
		GitAutoPush:    s.GitAutoPush,
	}
}

// --- User with memberships aggregation ---

// orgMembershipWithName wraps an organization membership with the org name.
type orgMembershipWithName struct {
	*identity.OrganizationMembership
	OrganizationName string
}

// wsMembershipWithName wraps a workspace membership with the workspace name and org ID.
type wsMembershipWithName struct {
	*identity.WorkspaceMembership
	WorkspaceName  string
	OrganizationID jsonldb.ID
}

// userWithMemberships wraps a user with their org and workspace memberships.
type userWithMemberships struct {
	User           *identity.User
	OrgMemberships []orgMembershipWithName
	WSMemberships  []wsMembershipWithName
	CurrentOrgID   jsonldb.ID
	CurrentOrgRole identity.OrganizationRole
	CurrentWSID    jsonldb.ID
	CurrentWSRole  identity.WorkspaceRole
}

func orgMembershipWithNameToResponse(m *orgMembershipWithName) dto.OrgMembershipResponse {
	return dto.OrgMembershipResponse{
		ID:               m.ID,
		UserID:           m.UserID,
		OrganizationID:   m.OrganizationID,
		OrganizationName: m.OrganizationName,
		Role:             dto.OrganizationRole(m.Role),
		Created:          m.Created,
	}
}

func wsMembershipWithNameToResponse(m *wsMembershipWithName) dto.WSMembershipResponse {
	return dto.WSMembershipResponse{
		ID:             m.ID,
		UserID:         m.UserID,
		WorkspaceID:    m.WorkspaceID,
		WorkspaceName:  m.WorkspaceName,
		OrganizationID: m.OrganizationID,
		Role:           dto.WorkspaceRole(m.Role),
		Settings:       wsMembershipSettingsToDTO(m.Settings),
		Created:        m.Created,
	}
}

func userWithMembershipsToResponse(uwm *userWithMemberships) *dto.UserResponse {
	resp := userToResponse(uwm.User)

	// Add org memberships
	orgMems := make([]dto.OrgMembershipResponse, len(uwm.OrgMemberships))
	for i := range uwm.OrgMemberships {
		orgMems[i] = orgMembershipWithNameToResponse(&uwm.OrgMemberships[i])
	}
	resp.Organizations = orgMems

	// Add workspace memberships
	wsMems := make([]dto.WSMembershipResponse, len(uwm.WSMemberships))
	for i := range uwm.WSMemberships {
		wsMems[i] = wsMembershipWithNameToResponse(&uwm.WSMemberships[i])
	}
	resp.Workspaces = wsMems

	// Add current context
	if !uwm.CurrentOrgID.IsZero() {
		resp.OrganizationID = uwm.CurrentOrgID
		resp.OrgRole = dto.OrganizationRole(uwm.CurrentOrgRole)
	}
	if !uwm.CurrentWSID.IsZero() {
		resp.WorkspaceID = uwm.CurrentWSID
		resp.WorkspaceRole = dto.WorkspaceRole(uwm.CurrentWSRole)
		// Find and set the workspace name from memberships
		for _, wsMem := range uwm.WSMemberships {
			if wsMem.WorkspaceID == uwm.CurrentWSID {
				resp.WorkspaceName = wsMem.WorkspaceName
				break
			}
		}
	}

	return resp
}

// getUserWithMemberships fetches a user and their org/workspace memberships with names.
func getUserWithMemberships(
	userService *identity.UserService,
	orgMemService *identity.OrganizationMembershipService,
	wsMemService *identity.WorkspaceMembershipService,
	orgService *identity.OrganizationService,
	wsService *identity.WorkspaceService,
	userID jsonldb.ID,
) (*userWithMemberships, error) {
	user, err := userService.Get(userID)
	if err != nil {
		return nil, err
	}

	// Get org memberships
	var orgMems []orgMembershipWithName
	for m := range orgMemService.IterByUser(userID) {
		mwon := orgMembershipWithName{OrganizationMembership: m}
		if org, err := orgService.Get(m.OrganizationID); err == nil {
			mwon.OrganizationName = org.Name
		}
		orgMems = append(orgMems, mwon)
	}

	// Get workspace memberships
	var wsMems []wsMembershipWithName
	for m := range wsMemService.IterByUser(userID) {
		mwon := wsMembershipWithName{WorkspaceMembership: m}
		if ws, err := wsService.Get(m.WorkspaceID); err == nil {
			mwon.WorkspaceName = ws.Name
			mwon.OrganizationID = ws.OrganizationID
		}
		wsMems = append(wsMems, mwon)
	}

	return &userWithMemberships{
		User:           user,
		OrgMemberships: orgMems,
		WSMemberships:  wsMems,
	}, nil
}

// --- List summary conversions ---

func pageToSummary(n *content.Node) dto.PageSummary {
	return dto.PageSummary{
		ID:       n.ID,
		Title:    n.Title,
		Created:  n.Created,
		Modified: n.Modified,
	}
}

func pagesToSummaries(nodes []*content.Node) []dto.PageSummary {
	result := make([]dto.PageSummary, len(nodes))
	for i, n := range nodes {
		result[i] = pageToSummary(n)
	}
	return result
}

func tableToSummary(n *content.Node) dto.TableSummary {
	return dto.TableSummary{
		ID:       n.ID,
		Title:    n.Title,
		Created:  n.Created,
		Modified: n.Modified,
	}
}

func tablesToSummaries(nodes []*content.Node) []dto.TableSummary {
	result := make([]dto.TableSummary, len(nodes))
	for i, n := range nodes {
		result[i] = tableToSummary(n)
	}
	return result
}

func assetToSummary(a *content.Asset, wsID, pageID string) dto.AssetSummary {
	return dto.AssetSummary{
		ID:       a.ID,
		Name:     a.Name,
		Size:     a.Size,
		MimeType: a.MimeType,
		Created:  a.Created,
		URL:      "/api/workspaces/" + wsID + "/assets/" + pageID + "/" + a.Name,
	}
}

func assetsToSummaries(assets []*content.Asset, wsID, pageID string) []dto.AssetSummary {
	result := make([]dto.AssetSummary, len(assets))
	for i, a := range assets {
		result[i] = assetToSummary(a, wsID, pageID)
	}
	return result
}
