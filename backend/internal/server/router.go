// Package server implements HTTP routing, middleware, and request handling.
//
// It provides handler composition utilities (Wrap, WrapAuth) for type-safe routes
// with JWT authentication, role-based access control, and automatic JSON marshaling.
// It also serves the embedded SolidJS frontend.
package server

//go:generate go run ../apiroutes -q
//go:generate go run ../apiclient -q

import (
	"embed"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/maruel/mddb/backend/frontend"
	"github.com/maruel/mddb/backend/internal/email"
	"github.com/maruel/mddb/backend/internal/server/handlers"
	"github.com/maruel/mddb/backend/internal/server/ratelimit"
	"github.com/maruel/mddb/backend/internal/server/reqctx"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// NewRouter creates and configures the HTTP router.
// Serves API endpoints at /api/* and static SolidJS frontend at /.
// baseURL is used for constructing OAuth callback URLs (e.g., "http://localhost:8080" or "https://example.com").
// emailService may be nil if SMTP is not configured.
func NewRouter(
	fileStore *content.FileStoreService,
	userService *identity.UserService,
	orgService *identity.OrganizationService,
	wsService *identity.WorkspaceService,
	orgInvService *identity.OrganizationInvitationService,
	wsInvService *identity.WorkspaceInvitationService,
	orgMemService *identity.OrganizationMembershipService,
	wsMemService *identity.WorkspaceMembershipService,
	sessionService *identity.SessionService,
	emailService *email.Service,
	jwtSecret, baseURL, googleClientID, googleClientSecret, msClientID, msClientSecret, githubClientID, githubClientSecret string,
) http.Handler {
	mux := &http.ServeMux{}
	jwtSecretBytes := []byte(jwtSecret)
	_ = emailService // Available for handlers; nil if SMTP not configured

	// Create rate limit config
	rlConfig := ratelimit.DefaultConfig()

	// Content handlers (workspace-scoped)
	nh := handlers.NewNodeHandler(fileStore)
	ah := handlers.NewAssetHandler(fileStore)
	sh := handlers.NewSearchHandler(fileStore)

	// Auth handler
	authh := handlers.NewAuthHandler(userService, orgMemService, wsMemService, orgService, wsService, sessionService, fileStore, jwtSecret)

	// Other handlers
	uh := handlers.NewUserHandler(userService, orgMemService, wsMemService, orgService, wsService)
	ih := handlers.NewInvitationHandler(orgInvService, wsInvService, userService, orgService, wsService, orgMemService, wsMemService, authh)
	mh := handlers.NewMembershipHandler(orgMemService, wsMemService, userService, orgService, wsService, authh)
	orgh := handlers.NewOrganizationHandler(orgService, orgMemService, wsService, wsMemService, fileStore)
	grh := handlers.NewGitRemoteHandler(wsService, fileStore)

	// Role constants
	member := identity.OrgRoleMember
	orgAdmin := identity.OrgRoleAdmin
	wsViewer := identity.WSRoleViewer
	wsEditor := identity.WSRoleEditor
	wsAdmin := identity.WSRoleAdmin

	// Health check (public)
	hh := handlers.NewHealthHandler("1.0.0")
	mux.Handle("/api/health", Wrap(hh.GetHealth, rlConfig))

	// Global admin endpoints (requires IsGlobalAdmin)
	adminh := handlers.NewAdminHandler(userService, orgService, wsService, orgMemService)
	mux.Handle("GET /api/admin/stats", WrapGlobalAdmin(userService, sessionService, jwtSecretBytes, adminh.GetAdminStats, rlConfig))
	mux.Handle("GET /api/admin/users", WrapGlobalAdmin(userService, sessionService, jwtSecretBytes, adminh.ListAllUsers, rlConfig))
	mux.Handle("GET /api/admin/organizations", WrapGlobalAdmin(userService, sessionService, jwtSecretBytes, adminh.ListAllOrgs, rlConfig))

	// Auth endpoints (public)
	mux.Handle("POST /api/auth/login", Wrap(authh.Login, rlConfig))
	mux.Handle("POST /api/auth/register", Wrap(authh.Register, rlConfig))
	mux.Handle("POST /api/auth/invitations/org/accept", Wrap(ih.AcceptOrgInvitation, rlConfig))
	mux.Handle("POST /api/auth/invitations/ws/accept", Wrap(ih.AcceptWSInvitation, rlConfig))

	// Auth endpoints (authenticated, no org context)
	mux.Handle("GET /api/auth/me", WrapAuth(userService, orgMemService, sessionService, jwtSecretBytes, member, authh.GetMe, rlConfig))
	mux.Handle("POST /api/auth/switch-org", WrapAuth(userService, orgMemService, sessionService, jwtSecretBytes, member, mh.SwitchOrg, rlConfig))
	mux.Handle("POST /api/auth/switch-workspace", WrapAuth(userService, orgMemService, sessionService, jwtSecretBytes, member, mh.SwitchWorkspace, rlConfig))
	mux.Handle("POST /api/auth/settings", WrapAuth(userService, orgMemService, sessionService, jwtSecretBytes, member, uh.UpdateUserSettings, rlConfig))
	mux.Handle("POST /api/organizations", WrapAuth(userService, orgMemService, sessionService, jwtSecretBytes, member, authh.CreateOrganization, rlConfig))

	// Session management endpoints (authenticated, no org)
	mux.Handle("POST /api/auth/logout", WrapAuth(userService, orgMemService, sessionService, jwtSecretBytes, member, authh.Logout, rlConfig))
	mux.Handle("GET /api/auth/sessions", WrapAuth(userService, orgMemService, sessionService, jwtSecretBytes, member, authh.ListSessions, rlConfig))
	mux.Handle("POST /api/auth/sessions/revoke", WrapAuth(userService, orgMemService, sessionService, jwtSecretBytes, member, authh.RevokeSession, rlConfig))
	mux.Handle("POST /api/auth/sessions/revoke-all", WrapAuth(userService, orgMemService, sessionService, jwtSecretBytes, member, authh.RevokeAllSessions, rlConfig))

	// Email management (authenticated)
	mux.Handle("POST /api/auth/email", WrapAuth(userService, orgMemService, sessionService, jwtSecretBytes, member, authh.ChangeEmail, rlConfig))

	// Organization settings (org-scoped)
	mux.Handle("GET /api/organizations/{orgID}", WrapAuth(userService, orgMemService, sessionService, jwtSecretBytes, member, orgh.GetOrganization, rlConfig))
	mux.Handle("POST /api/organizations/{orgID}", WrapAuth(userService, orgMemService, sessionService, jwtSecretBytes, orgAdmin, orgh.UpdateOrganization, rlConfig))
	mux.Handle("POST /api/organizations/{orgID}/settings", WrapAuth(userService, orgMemService, sessionService, jwtSecretBytes, orgAdmin, orgh.UpdateOrgPreferences, rlConfig))

	// Organization user management (org-scoped)
	mux.Handle("GET /api/organizations/{orgID}/users", WrapAuth(userService, orgMemService, sessionService, jwtSecretBytes, orgAdmin, uh.ListUsers, rlConfig))
	mux.Handle("POST /api/organizations/{orgID}/users/role", WrapAuth(userService, orgMemService, sessionService, jwtSecretBytes, orgAdmin, uh.UpdateOrgMemberRole, rlConfig))

	// Organization invitations (org-scoped)
	mux.Handle("GET /api/organizations/{orgID}/invitations", WrapAuth(userService, orgMemService, sessionService, jwtSecretBytes, orgAdmin, ih.ListOrgInvitations, rlConfig))
	mux.Handle("POST /api/organizations/{orgID}/invitations", WrapAuth(userService, orgMemService, sessionService, jwtSecretBytes, orgAdmin, ih.CreateOrgInvitation, rlConfig))

	// Workspace creation (org-scoped)
	mux.Handle("POST /api/organizations/{orgID}/workspaces", WrapAuth(userService, orgMemService, sessionService, jwtSecretBytes, orgAdmin, orgh.CreateWorkspace, rlConfig))

	// Workspace details (workspace-scoped)
	mux.Handle("GET /api/workspaces/{wsID}", WrapWSAuth(userService, orgMemService, wsMemService, wsService, sessionService, jwtSecretBytes, wsViewer, orgh.GetWorkspace, rlConfig))
	mux.Handle("POST /api/workspaces/{wsID}", WrapWSAuth(userService, orgMemService, wsMemService, wsService, sessionService, jwtSecretBytes, wsAdmin, orgh.UpdateWorkspace, rlConfig))

	// Workspace settings (workspace-scoped)
	mux.Handle("POST /api/workspaces/{wsID}/settings/membership", WrapWSAuth(userService, orgMemService, wsMemService, wsService, sessionService, jwtSecretBytes, wsViewer, mh.UpdateWSMembershipSettings, rlConfig))

	// Workspace user management (workspace-scoped)
	mux.Handle("POST /api/workspaces/{wsID}/users/role", WrapWSAuth(userService, orgMemService, wsMemService, wsService, sessionService, jwtSecretBytes, wsAdmin, uh.UpdateWSMemberRole, rlConfig))

	// Workspace invitations (workspace-scoped)
	mux.Handle("GET /api/workspaces/{wsID}/invitations", WrapWSAuth(userService, orgMemService, wsMemService, wsService, sessionService, jwtSecretBytes, wsAdmin, ih.ListWSInvitations, rlConfig))
	mux.Handle("POST /api/workspaces/{wsID}/invitations", WrapWSAuth(userService, orgMemService, wsMemService, wsService, sessionService, jwtSecretBytes, wsAdmin, ih.CreateWSInvitation, rlConfig))

	// Git Remote endpoints (workspace-scoped)
	mux.Handle("GET /api/workspaces/{wsID}/settings/git", WrapWSAuth(userService, orgMemService, wsMemService, wsService, sessionService, jwtSecretBytes, wsAdmin, grh.GetGitRemote, rlConfig))
	mux.Handle("POST /api/workspaces/{wsID}/settings/git", WrapWSAuth(userService, orgMemService, wsMemService, wsService, sessionService, jwtSecretBytes, wsAdmin, grh.UpdateGitRemote, rlConfig))
	mux.Handle("POST /api/workspaces/{wsID}/settings/git/push", WrapWSAuth(userService, orgMemService, wsMemService, wsService, sessionService, jwtSecretBytes, wsAdmin, grh.PushGit, rlConfig))
	mux.Handle("POST /api/workspaces/{wsID}/settings/git/delete", WrapWSAuth(userService, orgMemService, wsMemService, wsService, sessionService, jwtSecretBytes, wsAdmin, grh.DeleteGitRemote, rlConfig))

	// OAuth endpoints (public) - always registered, returns error if provider not configured
	oh := handlers.NewOAuthHandler(userService, authh)
	base := strings.TrimRight(baseURL, "/")
	var providers []identity.OAuthProvider
	if googleClientID != "" && googleClientSecret != "" {
		oh.AddProvider(identity.OAuthProviderGoogle, googleClientID, googleClientSecret, base+"/api/auth/oauth/google/callback")
		providers = append(providers, identity.OAuthProviderGoogle)
	}
	if msClientID != "" && msClientSecret != "" {
		oh.AddProvider(identity.OAuthProviderMicrosoft, msClientID, msClientSecret, base+"/api/auth/oauth/microsoft/callback")
		providers = append(providers, identity.OAuthProviderMicrosoft)
	}
	if githubClientID != "" && githubClientSecret != "" {
		oh.AddProvider(identity.OAuthProviderGitHub, githubClientID, githubClientSecret, base+"/api/auth/oauth/github/callback")
		providers = append(providers, identity.OAuthProviderGitHub)
	}
	if len(providers) > 0 {
		slog.Info("OAuth providers initialized", "providers", providers)
	} else {
		slog.Info("No OAuth providers configured")
	}
	mux.Handle("GET /api/auth/providers", Wrap(oh.ListProviders, rlConfig))
	mux.HandleFunc("GET /api/auth/oauth/{provider}", oh.LoginRedirect)
	mux.HandleFunc("GET /api/auth/oauth/{provider}/callback", oh.Callback)

	// OAuth linking endpoints (authenticated)
	mux.Handle("POST /api/auth/oauth/link", WrapAuth(userService, orgMemService, sessionService, jwtSecretBytes, member, oh.LinkOAuth, rlConfig))
	mux.Handle("POST /api/auth/oauth/unlink", WrapAuth(userService, orgMemService, sessionService, jwtSecretBytes, member, oh.UnlinkOAuth, rlConfig))

	// Nodes endpoints (workspace-scoped)
	// id=0 is valid for root node
	mux.Handle("GET /api/workspaces/{wsID}/nodes", WrapWSAuth(userService, orgMemService, wsMemService, wsService, sessionService, jwtSecretBytes, wsViewer, nh.ListNodes, rlConfig))
	mux.Handle("GET /api/workspaces/{wsID}/nodes/{id}", WrapWSAuth(userService, orgMemService, wsMemService, wsService, sessionService, jwtSecretBytes, wsViewer, nh.GetNode, rlConfig))
	mux.Handle("GET /api/workspaces/{wsID}/nodes/{id}/children", WrapWSAuth(userService, orgMemService, wsMemService, wsService, sessionService, jwtSecretBytes, wsViewer, nh.ListNodeChildren, rlConfig))
	mux.Handle("POST /api/workspaces/{wsID}/nodes/{id}/delete", WrapWSAuth(userService, orgMemService, wsMemService, wsService, sessionService, jwtSecretBytes, wsEditor, nh.DeleteNode, rlConfig))

	// Page endpoints (workspace-scoped, under nodes)
	mux.Handle("POST /api/workspaces/{wsID}/nodes/{id}/page/create", WrapWSAuth(userService, orgMemService, wsMemService, wsService, sessionService, jwtSecretBytes, wsEditor, nh.CreatePage, rlConfig))
	mux.Handle("GET /api/workspaces/{wsID}/nodes/{id}/page", WrapWSAuth(userService, orgMemService, wsMemService, wsService, sessionService, jwtSecretBytes, wsViewer, nh.GetPage, rlConfig))
	mux.Handle("POST /api/workspaces/{wsID}/nodes/{id}/page", WrapWSAuth(userService, orgMemService, wsMemService, wsService, sessionService, jwtSecretBytes, wsEditor, nh.UpdatePage, rlConfig))
	mux.Handle("POST /api/workspaces/{wsID}/nodes/{id}/page/delete", WrapWSAuth(userService, orgMemService, wsMemService, wsService, sessionService, jwtSecretBytes, wsEditor, nh.DeletePage, rlConfig))

	// Table endpoints (workspace-scoped, under nodes)
	mux.Handle("POST /api/workspaces/{wsID}/nodes/{id}/table/create", WrapWSAuth(userService, orgMemService, wsMemService, wsService, sessionService, jwtSecretBytes, wsEditor, nh.CreateTable, rlConfig))
	mux.Handle("GET /api/workspaces/{wsID}/nodes/{id}/table", WrapWSAuth(userService, orgMemService, wsMemService, wsService, sessionService, jwtSecretBytes, wsViewer, nh.GetTable, rlConfig))
	mux.Handle("POST /api/workspaces/{wsID}/nodes/{id}/table", WrapWSAuth(userService, orgMemService, wsMemService, wsService, sessionService, jwtSecretBytes, wsEditor, nh.UpdateTable, rlConfig))
	mux.Handle("POST /api/workspaces/{wsID}/nodes/{id}/table/delete", WrapWSAuth(userService, orgMemService, wsMemService, wsService, sessionService, jwtSecretBytes, wsEditor, nh.DeleteTable, rlConfig))

	// Records endpoints (workspace-scoped, under nodes/table)
	mux.Handle("GET /api/workspaces/{wsID}/nodes/{id}/table/records", WrapWSAuth(userService, orgMemService, wsMemService, wsService, sessionService, jwtSecretBytes, wsViewer, nh.ListRecords, rlConfig))
	mux.Handle("GET /api/workspaces/{wsID}/nodes/{id}/table/records/{rid}", WrapWSAuth(userService, orgMemService, wsMemService, wsService, sessionService, jwtSecretBytes, wsViewer, nh.GetRecord, rlConfig))
	mux.Handle("POST /api/workspaces/{wsID}/nodes/{id}/table/records/create", WrapWSAuth(userService, orgMemService, wsMemService, wsService, sessionService, jwtSecretBytes, wsEditor, nh.CreateRecord, rlConfig))
	mux.Handle("POST /api/workspaces/{wsID}/nodes/{id}/table/records/{rid}", WrapWSAuth(userService, orgMemService, wsMemService, wsService, sessionService, jwtSecretBytes, wsEditor, nh.UpdateRecord, rlConfig))
	mux.Handle("POST /api/workspaces/{wsID}/nodes/{id}/table/records/{rid}/delete", WrapWSAuth(userService, orgMemService, wsMemService, wsService, sessionService, jwtSecretBytes, wsEditor, nh.DeleteRecord, rlConfig))

	// History endpoints (workspace-scoped, under nodes)
	mux.Handle("GET /api/workspaces/{wsID}/nodes/{id}/history", WrapWSAuth(userService, orgMemService, wsMemService, wsService, sessionService, jwtSecretBytes, wsViewer, nh.ListNodeVersions, rlConfig))
	mux.Handle("GET /api/workspaces/{wsID}/nodes/{id}/history/{hash}", WrapWSAuth(userService, orgMemService, wsMemService, wsService, sessionService, jwtSecretBytes, wsViewer, nh.GetNodeVersion, rlConfig))

	// Assets endpoints (workspace-scoped, node-based)
	mux.Handle("GET /api/workspaces/{wsID}/nodes/{id}/assets", WrapWSAuth(userService, orgMemService, wsMemService, wsService, sessionService, jwtSecretBytes, wsViewer, nh.ListNodeAssets, rlConfig))
	mux.Handle("POST /api/workspaces/{wsID}/nodes/{id}/assets", WrapAuthRaw(userService, orgMemService, wsMemService, wsService, sessionService, jwtSecretBytes, wsEditor, ah.UploadNodeAssetHandler, rlConfig))
	mux.Handle("POST /api/workspaces/{wsID}/nodes/{id}/assets/{name}/delete", WrapWSAuth(userService, orgMemService, wsMemService, wsService, sessionService, jwtSecretBytes, wsEditor, nh.DeleteNodeAsset, rlConfig))

	// Search endpoint (workspace-scoped)
	mux.Handle("POST /api/workspaces/{wsID}/search", WrapWSAuth(userService, orgMemService, wsMemService, wsService, sessionService, jwtSecretBytes, wsViewer, sh.Search, rlConfig))

	// File serving (raw asset files) - public for now
	mux.HandleFunc("GET /assets/{wsID}/{id}/{name}", ah.ServeAssetFile)

	// API catch-all - return 404 for any unmatched /api/ routes (never fall through to SPA)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Not found", http.StatusNotFound)
	})

	// Serve embedded SolidJS frontend with SPA fallback
	mux.Handle("/", NewEmbeddedSPAHandler(frontend.Files))

	f := func(w http.ResponseWriter, r *http.Request) {
		clientIP := reqctx.GetClientIP(r)
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		mux.ServeHTTP(rw, r)
		slog.InfoContext(r.Context(), "http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"size", rw.size,
			"ip", clientIP,
		)
	}
	return http.HandlerFunc(f)
}

// responseWriter wraps http.ResponseWriter to capture status code and response size.
type responseWriter struct {
	http.ResponseWriter
	status int
	size   int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.size += n
	return n, err
}

// EmbeddedSPAHandler serves an embedded single-page application with fallback to index.html.
type EmbeddedSPAHandler struct {
	fs embed.FS
}

// NewEmbeddedSPAHandler creates a handler for the embedded frontend.
func NewEmbeddedSPAHandler(f embed.FS) *EmbeddedSPAHandler {
	return &EmbeddedSPAHandler{fs: f}
}

// ServeHTTP implements http.Handler for embedded SPA routing.
func (h *EmbeddedSPAHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Try to serve the exact file from dist/
	path := "dist" + r.URL.Path
	f, err := h.fs.Open(path)
	if err == nil {
		if err := f.Close(); err != nil {
			slog.Error("Failed to close embedded file", "path", path, "error", err)
		}
		// File exists, serve it from embedded FS
		fsys, err := fs.Sub(h.fs, "dist")
		if err != nil {
			slog.Error("Failed to create sub-filesystem", "error", err)
			http.NotFound(w, r)
			return
		}
		fileServer := http.FileServer(http.FS(fsys))
		// Set cache headers for static assets with extensions
		if containsDot(r.URL.Path) {
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}
		fileServer.ServeHTTP(w, r)
		return
	}

	// File not found - fall back to index.html for SPA routing
	indexFile, err := h.fs.Open("dist/index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() {
		if err := indexFile.Close(); err != nil {
			slog.Error("Failed to close index.html", "error", err)
		}
	}()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	// Serve index.html
	if _, err := io.Copy(w, indexFile); err != nil {
		slog.Error("Failed to serve index.html", "error", err)
	}
}

// containsDot checks if a path contains a dot (file extension).
func containsDot(path string) bool {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return false
		}
		if path[i] == '.' {
			return true
		}
	}
	return false
}
