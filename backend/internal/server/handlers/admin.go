// Handles global system administration endpoints.

package handlers

import (
	"context"
	"log/slog"
	"time"

	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// AdminHandler handles global admin endpoints.
type AdminHandler struct {
	Svc             *Services
	RateLimitCounts func() (auth, write, readAuth, readUnauth int64)
	ServerStartTime func() time.Time
}

// GetServerDetail returns detailed server-wide dashboard data.
func (h *AdminHandler) GetServerDetail(ctx context.Context, _ *identity.User, _ *dto.AdminServerDetailRequest) (*dto.AdminServerDetail, error) {
	var userCount, orgCount, wsCount int
	for range h.Svc.User.Iter(0) {
		userCount++
	}

	// Build org/workspace details.
	orgs := make([]dto.AdminOrgDetail, 0) //nolint:prealloc // size unknown from iterator
	for org := range h.Svc.Organization.Iter(0) {
		orgCount++
		memberCount := h.Svc.OrgMembership.CountOrgMemberships(org.ID)

		workspaces := make([]dto.AdminWorkspaceDetail, 0) //nolint:prealloc // size unknown from iterator
		for ws := range h.Svc.Workspace.IterByOrg(org.ID) {
			wsCount++
			wsMemberCount := h.Svc.WSMembership.CountWSMemberships(ws.ID)

			var pageCount int
			var storageBytes int64
			var gitCommits int
			store, err := h.Svc.FileStore.GetWorkspaceStore(ctx, ws.ID)
			if err == nil {
				pageCount, storageBytes, _ = store.GetWorkspaceUsage()
				gitCommits, _ = store.CommitCount(ctx)
			} else {
				slog.Warn("admin dashboard: failed to get workspace store", "ws_id", ws.ID, "error", err)
			}

			workspaces = append(workspaces, dto.AdminWorkspaceDetail{
				ID:           ws.ID,
				OrgID:        ws.OrganizationID,
				Name:         ws.Name,
				MemberCount:  wsMemberCount,
				PageCount:    pageCount,
				StorageBytes: storageBytes,
				GitCommits:   gitCommits,
				Created:      ws.Created,
			})
		}

		orgs = append(orgs, dto.AdminOrgDetail{
			ID:             org.ID,
			Name:           org.Name,
			MemberCount:    memberCount,
			WorkspaceCount: len(workspaces),
			Created:        org.Created,
			Workspaces:     workspaces,
		})
	}

	// Total storage.
	totalStorage, _ := h.Svc.FileStore.GetServerUsage()

	// Active sessions.
	activeSessions := h.Svc.Session.CountActive()

	// Request metrics.
	var metrics dto.AdminRequestMetrics
	if h.RateLimitCounts != nil && h.ServerStartTime != nil {
		startTime := h.ServerStartTime()
		auth, write, readAuth, readUnauth := h.RateLimitCounts()
		metrics = dto.AdminRequestMetrics{
			ServerStartTime: float64(startTime.UnixMilli()) / 1000,
			UptimeSeconds:   time.Since(startTime).Seconds(),
			AuthCount:       auth,
			WriteCount:      write,
			ReadAuthCount:   readAuth,
			ReadUnauthCount: readUnauth,
		}
	}

	return &dto.AdminServerDetail{
		UserCount:      userCount,
		OrgCount:       orgCount,
		WorkspaceCount: wsCount,
		TotalStorage:   totalStorage,
		ActiveSessions: activeSessions,
		Organizations:  orgs,
		RequestMetrics: metrics,
	}, nil
}

// ListAllUsers returns all users in the system.
func (h *AdminHandler) ListAllUsers(ctx context.Context, _ *identity.User, _ *dto.AdminUsersRequest) (*dto.AdminUsersResponse, error) {
	users := make([]dto.UserResponse, 0) //nolint:prealloc // size unknown from iterator
	for user := range h.Svc.User.Iter(0) {
		users = append(users, *userToResponse(user))
	}
	return &dto.AdminUsersResponse{Users: users}, nil
}

// ListAllOrgs returns all organizations in the system.
func (h *AdminHandler) ListAllOrgs(ctx context.Context, _ *identity.User, _ *dto.AdminOrgsRequest) (*dto.AdminOrgsResponse, error) {
	orgs := make([]dto.OrganizationResponse, 0) //nolint:prealloc // size unknown from iterator
	for org := range h.Svc.Organization.Iter(0) {
		memberCount := h.Svc.OrgMembership.CountOrgMemberships(org.ID)
		workspaceCount := h.Svc.Workspace.CountByOrg(org.ID)
		orgs = append(orgs, *organizationToResponse(org, memberCount, workspaceCount))
	}
	return &dto.AdminOrgsResponse{Organizations: orgs}, nil
}
