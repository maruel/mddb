package handlers

import (
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage/entity"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// --- ID decoding helpers ---

func decodeOrgID(s string) (jsonldb.ID, error) {
	id, err := jsonldb.DecodeID(s)
	if err != nil {
		return 0, dto.BadRequest("invalid_org_id")
	}
	return id, nil
}

func decodeID(s, field string) (jsonldb.ID, error) {
	id, err := jsonldb.DecodeID(s)
	if err != nil {
		return 0, dto.BadRequest("invalid_" + field)
	}
	return id, nil
}

// --- Time formatting ---

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

// --- Entity to DTO conversions ---

func userToResponse(u *entity.User) *dto.UserResponse {
	identities := make([]dto.OAuthIdentity, len(u.OAuthIdentities))
	for i, id := range u.OAuthIdentities {
		identities[i] = oauthIdentityToDTO(id)
	}
	return &dto.UserResponse{
		ID:              u.ID.String(),
		Email:           u.Email,
		Name:            u.Name,
		OAuthIdentities: identities,
		Settings:        userSettingsToDTO(u.Settings),
		Created:         formatTime(u.Created),
		Modified:        formatTime(u.Modified),
	}
}

func membershipToResponse(m *entity.Membership) *dto.MembershipResponse {
	return &dto.MembershipResponse{
		ID:             m.ID.String(),
		UserID:         m.UserID.String(),
		OrganizationID: m.OrganizationID.String(),
		Role:           dto.UserRole(m.Role),
		Settings:       membershipSettingsToDTO(m.Settings),
		Created:        formatTime(m.Created),
	}
}

func invitationToResponse(i *entity.Invitation) *dto.InvitationResponse {
	return &dto.InvitationResponse{
		ID:             i.ID.String(),
		Email:          i.Email,
		OrganizationID: i.OrganizationID.String(),
		Role:           dto.UserRole(i.Role),
		ExpiresAt:      formatTime(i.ExpiresAt),
		Created:        formatTime(i.Created),
	}
}

func organizationToResponse(o *entity.Organization) *dto.OrganizationResponse {
	return &dto.OrganizationResponse{
		ID:         o.ID.String(),
		Name:       o.Name,
		Quotas:     quotaToDTO(o.Quotas),
		Settings:   organizationSettingsToDTO(o.Settings),
		Onboarding: onboardingStateToDTO(o.Onboarding),
		Created:    formatTime(o.Created),
	}
}

func nodeToResponse(n *entity.Node) *dto.NodeResponse {
	resp := &dto.NodeResponse{
		ID:         n.ID.String(),
		Title:      n.Title,
		Content:    n.Content,
		Properties: propertiesToDTO(n.Properties),
		Created:    formatTime(n.Created),
		Modified:   formatTime(n.Modified),
		Tags:       n.Tags,
		FaviconURL: n.FaviconURL,
		Type:       dto.NodeType(n.Type),
	}
	if !n.ParentID.IsZero() {
		resp.ParentID = n.ParentID.String()
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

func dataRecordToResponse(r *entity.DataRecord) *dto.DataRecordResponse {
	return &dto.DataRecordResponse{
		ID:       r.ID.String(),
		Data:     r.Data,
		Created:  formatTime(r.Created),
		Modified: formatTime(r.Modified),
	}
}

func commitToDTO(c *entity.Commit) *dto.Commit {
	if c == nil {
		return nil
	}
	return &dto.Commit{
		Hash:      c.Hash,
		Message:   c.Message,
		Timestamp: formatTime(c.Timestamp),
	}
}

func commitsToDTO(commits []*entity.Commit) []*dto.Commit {
	result := make([]*dto.Commit, len(commits))
	for i, c := range commits {
		result[i] = commitToDTO(c)
	}
	return result
}

func searchResultToDTO(r *entity.SearchResult) dto.SearchResult {
	return dto.SearchResult{
		Type:     r.Type,
		NodeID:   r.NodeID.String(),
		RecordID: r.RecordID.String(),
		Title:    r.Title,
		Snippet:  r.Snippet,
		Score:    r.Score,
		Matches:  r.Matches,
		Modified: formatTime(r.Modified),
	}
}

func searchResultsToDTO(results []entity.SearchResult) []dto.SearchResult {
	dtoResults := make([]dto.SearchResult, len(results))
	for i := range results {
		dtoResults[i] = searchResultToDTO(&results[i])
	}
	return dtoResults
}

// --- Nested type conversions (entity -> dto) ---

func propertyToDTO(p entity.Property) dto.Property {
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

func propertiesToDTO(props []entity.Property) []dto.Property {
	if props == nil {
		return nil
	}
	result := make([]dto.Property, len(props))
	for i, p := range props {
		result[i] = propertyToDTO(p)
	}
	return result
}

func userSettingsToDTO(s entity.UserSettings) dto.UserSettings {
	return dto.UserSettings{
		Theme:    s.Theme,
		Language: s.Language,
	}
}

func oauthIdentityToDTO(o entity.OAuthIdentity) dto.OAuthIdentity {
	return dto.OAuthIdentity{
		Provider:   o.Provider,
		ProviderID: o.ProviderID,
		Email:      o.Email,
		LastLogin:  formatTime(o.LastLogin),
	}
}

func membershipSettingsToDTO(s entity.MembershipSettings) dto.MembershipSettings {
	return dto.MembershipSettings{
		Notifications: s.Notifications,
	}
}

func quotaToDTO(q entity.Quota) dto.Quota {
	return dto.Quota{
		MaxPages:   q.MaxPages,
		MaxStorage: q.MaxStorage,
		MaxUsers:   q.MaxUsers,
	}
}

func gitSettingsToDTO(g entity.GitSettings) dto.GitSettings {
	return dto.GitSettings{
		AutoPush: g.AutoPush,
	}
}

func organizationSettingsToDTO(s entity.OrganizationSettings) dto.OrganizationSettings {
	return dto.OrganizationSettings{
		AllowedDomains: s.AllowedDomains,
		PublicAccess:   s.PublicAccess,
		Git:            gitSettingsToDTO(s.Git),
	}
}

func onboardingStateToDTO(o entity.OnboardingState) dto.OnboardingState {
	return dto.OnboardingState{
		Completed: o.Completed,
		Step:      o.Step,
		UpdatedAt: formatTime(o.UpdatedAt),
	}
}

func onboardingStatePtrToDTO(o *entity.OnboardingState) *dto.OnboardingState {
	if o == nil {
		return nil
	}
	result := onboardingStateToDTO(*o)
	return &result
}

// --- DTO to Entity conversions (for requests) ---

func propertyToEntity(p dto.Property) entity.Property {
	options := make([]entity.SelectOption, len(p.Options))
	for i, o := range p.Options {
		options[i] = entity.SelectOption{
			ID:    o.ID,
			Name:  o.Name,
			Color: o.Color,
		}
	}
	return entity.Property{
		Name:     p.Name,
		Type:     entity.PropertyType(p.Type),
		Required: p.Required,
		Options:  options,
	}
}

func propertiesToEntity(props []dto.Property) []entity.Property {
	if props == nil {
		return nil
	}
	result := make([]entity.Property, len(props))
	for i, p := range props {
		result[i] = propertyToEntity(p)
	}
	return result
}

func userRoleToEntity(r dto.UserRole) entity.UserRole {
	return entity.UserRole(r)
}

func membershipSettingsToEntity(s dto.MembershipSettings) entity.MembershipSettings {
	return entity.MembershipSettings{
		Notifications: s.Notifications,
	}
}

func userSettingsToEntity(s dto.UserSettings) entity.UserSettings {
	return entity.UserSettings{
		Theme:    s.Theme,
		Language: s.Language,
	}
}

func gitSettingsToEntity(g dto.GitSettings) entity.GitSettings {
	return entity.GitSettings{
		AutoPush: g.AutoPush,
	}
}

func organizationSettingsToEntity(s dto.OrganizationSettings) entity.OrganizationSettings {
	return entity.OrganizationSettings{
		AllowedDomains: s.AllowedDomains,
		PublicAccess:   s.PublicAccess,
		Git:            gitSettingsToEntity(s.Git),
	}
}

func onboardingStateToEntity(o dto.OnboardingState) entity.OnboardingState {
	var updatedAt time.Time
	if o.UpdatedAt != "" {
		updatedAt, _ = time.Parse(time.RFC3339, o.UpdatedAt)
	}
	return entity.OnboardingState{
		Completed: o.Completed,
		Step:      o.Step,
		UpdatedAt: updatedAt,
	}
}

// --- User with memberships aggregation ---

// membershipWithOrgName wraps a membership with its organization name.
type membershipWithOrgName struct {
	entity.Membership
	OrganizationName string
}

// userWithMemberships wraps a user with their memberships.
type userWithMemberships struct {
	User        *entity.User
	Memberships []membershipWithOrgName
}

// getUserWithMemberships fetches a user and their memberships with org names.
func getUserWithMemberships(userService *identity.UserService, memService *identity.MembershipService, orgService *identity.OrganizationService, userID jsonldb.ID) (*userWithMemberships, error) {
	user, err := userService.GetUser(userID)
	if err != nil {
		return nil, err
	}

	var mems []membershipWithOrgName
	if memIter, err := memService.Iter(userID); err == nil {
		for m := range memIter {
			mwon := membershipWithOrgName{Membership: *m}
			if org, err := orgService.GetOrganization(m.OrganizationID); err == nil {
				mwon.OrganizationName = org.Name
			}
			mems = append(mems, mwon)
		}
	}

	return &userWithMemberships{User: user, Memberships: mems}, nil
}

func membershipWithOrgNameToResponse(m *membershipWithOrgName) dto.MembershipResponse {
	return dto.MembershipResponse{
		ID:               m.ID.String(),
		UserID:           m.UserID.String(),
		OrganizationID:   m.OrganizationID.String(),
		OrganizationName: m.OrganizationName,
		Role:             dto.UserRole(m.Role),
		Settings:         membershipSettingsToDTO(m.Settings),
		Created:          formatTime(m.Created),
	}
}

func membershipsWithOrgNameToResponse(mems []membershipWithOrgName) []dto.MembershipResponse {
	result := make([]dto.MembershipResponse, len(mems))
	for i := range mems {
		result[i] = membershipWithOrgNameToResponse(&mems[i])
	}
	return result
}

func userWithMembershipsToResponse(uwm *userWithMemberships) *dto.UserResponse {
	resp := userToResponse(uwm.User)
	resp.Memberships = membershipsWithOrgNameToResponse(uwm.Memberships)
	return resp
}

// --- List summary conversions ---

func pageToSummary(n *entity.Node) dto.PageSummary {
	return dto.PageSummary{
		ID:       n.ID.String(),
		Title:    n.Title,
		Created:  formatTime(n.Created),
		Modified: formatTime(n.Modified),
	}
}

func pagesToSummaries(nodes []*entity.Node) []dto.PageSummary {
	result := make([]dto.PageSummary, len(nodes))
	for i, n := range nodes {
		result[i] = pageToSummary(n)
	}
	return result
}

func databaseToSummary(n *entity.Node) dto.DatabaseSummary {
	return dto.DatabaseSummary{
		ID:       n.ID.String(),
		Title:    n.Title,
		Created:  formatTime(n.Created),
		Modified: formatTime(n.Modified),
	}
}

func databasesToSummaries(nodes []*entity.Node) []dto.DatabaseSummary {
	result := make([]dto.DatabaseSummary, len(nodes))
	for i, n := range nodes {
		result[i] = databaseToSummary(n)
	}
	return result
}

func assetToSummary(a *entity.Asset, orgID, pageID string) dto.AssetSummary {
	return dto.AssetSummary{
		ID:       a.ID,
		Name:     a.Name,
		Size:     a.Size,
		MimeType: a.MimeType,
		Created:  formatTime(a.Created),
		URL:      "/api/" + orgID + "/assets/" + pageID + "/" + a.Name,
	}
}

func assetsToSummaries(assets []*entity.Asset, orgID, pageID string) []dto.AssetSummary {
	result := make([]dto.AssetSummary, len(assets))
	for i, a := range assets {
		result[i] = assetToSummary(a, orgID, pageID)
	}
	return result
}
