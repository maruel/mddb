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
	"github.com/maruel/mddb/backend/internal/server/handlers"
	"github.com/maruel/mddb/backend/internal/server/ratelimit"
	"github.com/maruel/mddb/backend/internal/server/reqctx"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// NewRouter creates and configures the HTTP router.
// Serves API endpoints at /api/* and static SolidJS frontend at /.
// baseURL is used for constructing OAuth callback URLs (e.g., "http://localhost:8080" or "https://example.com").
func NewRouter(fileStore *content.FileStore, userService *identity.UserService, orgService *identity.OrganizationService, invService *identity.InvitationService, memService *identity.MembershipService, sessionService *identity.SessionService, jwtSecret, baseURL, googleClientID, googleClientSecret, msClientID, msClientSecret string) http.Handler {
	mux := &http.ServeMux{}
	jwtSecretBytes := []byte(jwtSecret)

	// Create rate limit config
	rlConfig := ratelimit.DefaultConfig()

	ph := handlers.NewPageHandler(fileStore)
	th := handlers.NewTableHandler(fileStore)
	nh := handlers.NewNodeHandler(fileStore)
	ah := handlers.NewAssetHandler(fileStore)
	sh := handlers.NewSearchHandler(fileStore)
	authh := handlers.NewAuthHandler(userService, memService, orgService, sessionService, fileStore, jwtSecret)
	uh := handlers.NewUserHandler(userService, memService, orgService)
	ih := handlers.NewInvitationHandler(invService, userService, orgService, memService)
	mh := handlers.NewMembershipHandler(memService, userService, orgService, authh)
	orgh := handlers.NewOrganizationHandler(orgService)
	grh := handlers.NewGitRemoteHandler(orgService, fileStore)

	// Helper to create WrapAuth with common deps
	viewer := identity.UserRoleViewer
	editor := identity.UserRoleEditor
	admin := identity.UserRoleAdmin

	// Health check (public)
	hh := handlers.NewHealthHandler("1.0.0")
	mux.Handle("/api/health", Wrap(hh.GetHealth, rlConfig))

	// Global admin endpoints (requires IsGlobalAdmin)
	adminh := handlers.NewAdminHandler(userService, orgService, memService)
	mux.Handle("GET /api/admin/stats", WrapGlobalAdmin(userService, sessionService, jwtSecretBytes, adminh.GetAdminStats, rlConfig))
	mux.Handle("GET /api/admin/users", WrapGlobalAdmin(userService, sessionService, jwtSecretBytes, adminh.ListAllUsers, rlConfig))
	mux.Handle("GET /api/admin/organizations", WrapGlobalAdmin(userService, sessionService, jwtSecretBytes, adminh.ListAllOrgs, rlConfig))

	// Auth endpoints (public)
	mux.Handle("POST /api/auth/login", Wrap(authh.Login, rlConfig))
	mux.Handle("POST /api/auth/register", Wrap(authh.Register, rlConfig))
	mux.Handle("POST /api/auth/invitations/accept", Wrap(ih.AcceptInvitation, rlConfig))

	// Auth endpoints (authenticated, no org)
	mux.Handle("GET /api/auth/me", WrapAuth(userService, memService, sessionService, jwtSecretBytes, viewer, authh.GetMe, rlConfig))
	mux.Handle("POST /api/auth/switch-org", WrapAuth(userService, memService, sessionService, jwtSecretBytes, viewer, mh.SwitchOrg, rlConfig))
	mux.Handle("POST /api/auth/settings", WrapAuth(userService, memService, sessionService, jwtSecretBytes, viewer, uh.UpdateUserSettings, rlConfig))
	mux.Handle("POST /api/organizations", WrapAuth(userService, memService, sessionService, jwtSecretBytes, viewer, authh.CreateOrganization, rlConfig))

	// Session management endpoints (authenticated, no org)
	mux.Handle("POST /api/auth/logout", WrapAuth(userService, memService, sessionService, jwtSecretBytes, viewer, authh.Logout, rlConfig))
	mux.Handle("GET /api/auth/sessions", WrapAuth(userService, memService, sessionService, jwtSecretBytes, viewer, authh.ListSessions, rlConfig))
	mux.Handle("POST /api/auth/sessions/revoke", WrapAuth(userService, memService, sessionService, jwtSecretBytes, viewer, authh.RevokeSession, rlConfig))
	mux.Handle("POST /api/auth/sessions/revoke-all", WrapAuth(userService, memService, sessionService, jwtSecretBytes, viewer, authh.RevokeAllSessions, rlConfig))

	// Settings endpoints (authenticated with org)
	mux.Handle("POST /api/{orgID}/settings/membership", WrapAuth(userService, memService, sessionService, jwtSecretBytes, viewer, mh.UpdateMembershipSettings, rlConfig))
	mux.Handle("GET /api/{orgID}/settings/organization", WrapAuth(userService, memService, sessionService, jwtSecretBytes, viewer, orgh.GetOrganization, rlConfig))
	mux.Handle("POST /api/{orgID}/settings/organization", WrapAuth(userService, memService, sessionService, jwtSecretBytes, admin, orgh.UpdateOrganization, rlConfig))
	mux.Handle("POST /api/{orgID}/settings/preferences", WrapAuth(userService, memService, sessionService, jwtSecretBytes, admin, orgh.UpdateOrgPreferences, rlConfig))

	// Git Remote endpoints (one remote per org)
	mux.Handle("GET /api/{orgID}/settings/git", WrapAuth(userService, memService, sessionService, jwtSecretBytes, admin, grh.GetGitRemote, rlConfig))
	mux.Handle("POST /api/{orgID}/settings/git", WrapAuth(userService, memService, sessionService, jwtSecretBytes, admin, grh.UpdateGitRemote, rlConfig))
	mux.Handle("POST /api/{orgID}/settings/git/push", WrapAuth(userService, memService, sessionService, jwtSecretBytes, admin, grh.PushGit, rlConfig))
	mux.Handle("POST /api/{orgID}/settings/git/delete", WrapAuth(userService, memService, sessionService, jwtSecretBytes, admin, grh.DeleteGitRemote, rlConfig))

	// OAuth endpoints (public) - always registered, returns error if provider not configured
	oh := handlers.NewOAuthHandler(userService, memService, orgService, fileStore, authh)
	base := strings.TrimRight(baseURL, "/")
	var providers []string
	if googleClientID != "" && googleClientSecret != "" {
		oh.AddProvider("google", googleClientID, googleClientSecret, base+"/api/auth/oauth/google/callback")
		providers = append(providers, "google")
	}
	if msClientID != "" && msClientSecret != "" {
		oh.AddProvider("microsoft", msClientID, msClientSecret, base+"/api/auth/oauth/microsoft/callback")
		providers = append(providers, "microsoft")
	}
	if len(providers) > 0 {
		slog.Info("OAuth providers initialized", "providers", providers)
	} else {
		slog.Info("No OAuth providers configured")
	}
	mux.Handle("GET /api/auth/providers", Wrap(oh.ListProviders, rlConfig))
	mux.HandleFunc("GET /api/auth/oauth/{provider}", oh.LoginRedirect)
	mux.HandleFunc("GET /api/auth/oauth/{provider}/callback", oh.Callback)

	// User management endpoints
	mux.Handle("GET /api/{orgID}/users", WrapAuth(userService, memService, sessionService, jwtSecretBytes, admin, uh.ListUsers, rlConfig))
	mux.Handle("POST /api/{orgID}/users/role", WrapAuth(userService, memService, sessionService, jwtSecretBytes, admin, uh.UpdateUserRole, rlConfig))

	// Invitation endpoints
	mux.Handle("GET /api/{orgID}/invitations", WrapAuth(userService, memService, sessionService, jwtSecretBytes, admin, ih.ListInvitations, rlConfig))
	mux.Handle("POST /api/{orgID}/invitations", WrapAuth(userService, memService, sessionService, jwtSecretBytes, admin, ih.CreateInvitation, rlConfig))

	// Unified Nodes endpoints
	mux.Handle("GET /api/{orgID}/nodes", WrapAuth(userService, memService, sessionService, jwtSecretBytes, viewer, nh.ListNodes, rlConfig))
	mux.Handle("GET /api/{orgID}/nodes/{id}", WrapAuth(userService, memService, sessionService, jwtSecretBytes, viewer, nh.GetNode, rlConfig))
	mux.Handle("POST /api/{orgID}/nodes", WrapAuth(userService, memService, sessionService, jwtSecretBytes, editor, nh.CreateNode, rlConfig))

	// Pages endpoints
	mux.Handle("GET /api/{orgID}/pages", WrapAuth(userService, memService, sessionService, jwtSecretBytes, viewer, ph.ListPages, rlConfig))
	mux.Handle("GET /api/{orgID}/pages/{id}", WrapAuth(userService, memService, sessionService, jwtSecretBytes, viewer, ph.GetPage, rlConfig))
	mux.Handle("GET /api/{orgID}/pages/{id}/history", WrapAuth(userService, memService, sessionService, jwtSecretBytes, viewer, ph.ListPageVersions, rlConfig))
	mux.Handle("GET /api/{orgID}/pages/{id}/history/{hash}", WrapAuth(userService, memService, sessionService, jwtSecretBytes, viewer, ph.GetPageVersion, rlConfig))
	mux.Handle("POST /api/{orgID}/pages", WrapAuth(userService, memService, sessionService, jwtSecretBytes, editor, ph.CreatePage, rlConfig))
	mux.Handle("POST /api/{orgID}/pages/{id}", WrapAuth(userService, memService, sessionService, jwtSecretBytes, editor, ph.UpdatePage, rlConfig))
	mux.Handle("POST /api/{orgID}/pages/{id}/delete", WrapAuth(userService, memService, sessionService, jwtSecretBytes, editor, ph.DeletePage, rlConfig))

	// Table endpoints
	mux.Handle("GET /api/{orgID}/tables", WrapAuth(userService, memService, sessionService, jwtSecretBytes, viewer, th.ListTables, rlConfig))
	mux.Handle("GET /api/{orgID}/tables/{id}", WrapAuth(userService, memService, sessionService, jwtSecretBytes, viewer, th.GetTable, rlConfig))
	mux.Handle("POST /api/{orgID}/tables", WrapAuth(userService, memService, sessionService, jwtSecretBytes, editor, th.CreateTable, rlConfig))
	mux.Handle("POST /api/{orgID}/tables/{id}", WrapAuth(userService, memService, sessionService, jwtSecretBytes, editor, th.UpdateTable, rlConfig))
	mux.Handle("POST /api/{orgID}/tables/{id}/delete", WrapAuth(userService, memService, sessionService, jwtSecretBytes, editor, th.DeleteTable, rlConfig))

	// Records endpoints
	mux.Handle("GET /api/{orgID}/tables/{id}/records", WrapAuth(userService, memService, sessionService, jwtSecretBytes, viewer, th.ListRecords, rlConfig))
	mux.Handle("GET /api/{orgID}/tables/{id}/records/{rid}", WrapAuth(userService, memService, sessionService, jwtSecretBytes, viewer, th.GetRecord, rlConfig))
	mux.Handle("POST /api/{orgID}/tables/{id}/records", WrapAuth(userService, memService, sessionService, jwtSecretBytes, editor, th.CreateRecord, rlConfig))
	mux.Handle("POST /api/{orgID}/tables/{id}/records/{rid}", WrapAuth(userService, memService, sessionService, jwtSecretBytes, editor, th.UpdateRecord, rlConfig))
	mux.Handle("POST /api/{orgID}/tables/{id}/records/{rid}/delete", WrapAuth(userService, memService, sessionService, jwtSecretBytes, editor, th.DeleteRecord, rlConfig))

	// Assets endpoints (page-based)
	mux.Handle("GET /api/{orgID}/pages/{id}/assets", WrapAuth(userService, memService, sessionService, jwtSecretBytes, viewer, ah.ListPageAssets, rlConfig))
	mux.Handle("POST /api/{orgID}/pages/{id}/assets", WrapAuthRaw(userService, memService, sessionService, jwtSecretBytes, editor, ah.UploadPageAssetHandler, rlConfig))
	mux.Handle("POST /api/{orgID}/pages/{id}/assets/{name}/delete", WrapAuth(userService, memService, sessionService, jwtSecretBytes, editor, ah.DeletePageAsset, rlConfig))

	// Search endpoint
	mux.Handle("POST /api/{orgID}/search", WrapAuth(userService, memService, sessionService, jwtSecretBytes, viewer, sh.Search, rlConfig))

	// File serving (raw asset files) - public for now
	mux.HandleFunc("GET /assets/{orgID}/{id}/{name}", ah.ServeAssetFile)

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
