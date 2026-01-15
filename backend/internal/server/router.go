// Package server implements HTTP routing, middleware, and request handling.
//
// It provides handler composition utilities (Wrap, WrapAuth) for type-safe routes
// with JWT authentication, role-based access control, and automatic JSON marshaling.
// It also serves the embedded SolidJS frontend.
package server

import (
	"embed"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	"github.com/maruel/mddb/backend/frontend"
	"github.com/maruel/mddb/backend/internal/server/handlers"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// NewRouter creates and configures the HTTP router.
// Serves API endpoints at /api/* and static SolidJS frontend at /.
// baseURL is used for constructing OAuth callback URLs (e.g., "http://localhost:8080" or "https://example.com").
func NewRouter(fileStore *content.FileStore, gitService *git.Client, userService *identity.UserService, orgService *identity.OrganizationService, invService *identity.InvitationService, memService *identity.MembershipService, jwtSecret, baseURL, googleClientID, googleClientSecret, msClientID, msClientSecret string) http.Handler {
	mux := &http.ServeMux{}
	jwtSecretBytes := []byte(jwtSecret)

	pageService := content.NewPageService(fileStore, gitService, orgService)
	ph := handlers.NewPageHandler(pageService)
	dh := handlers.NewDatabaseHandler(fileStore, gitService, orgService)
	nh := handlers.NewNodeHandler(fileStore, gitService, orgService)
	ah := handlers.NewAssetHandler(fileStore, gitService, orgService)
	sh := handlers.NewSearchHandler(fileStore)
	authh := handlers.NewAuthHandler(userService, memService, orgService, pageService, jwtSecret)
	uh := handlers.NewUserHandler(userService, memService, orgService)
	ih := handlers.NewInvitationHandler(invService, userService, orgService, memService)
	mh := handlers.NewMembershipHandler(memService, userService, orgService, authh)
	orgh := handlers.NewOrganizationHandler(orgService)
	grh := handlers.NewGitRemoteHandler(orgService, gitService)

	// Helper to create WrapAuth with common deps
	viewer := identity.UserRoleViewer
	editor := identity.UserRoleEditor
	admin := identity.UserRoleAdmin

	// Health check (public)
	hh := handlers.NewHealthHandler("1.0.0")
	mux.Handle("/api/health", Wrap(hh.Health))

	// Auth endpoints (public)
	mux.Handle("POST /api/auth/login", Wrap(authh.Login))
	mux.Handle("POST /api/auth/register", Wrap(authh.Register))
	mux.Handle("POST /api/auth/invitations/accept", Wrap(ih.AcceptInvitation))

	// Auth endpoints (authenticated, no org)
	mux.Handle("GET /api/auth/me", WrapAuth(userService, memService, jwtSecretBytes, viewer, authh.Me))
	mux.Handle("POST /api/auth/switch-org", WrapAuth(userService, memService, jwtSecretBytes, viewer, mh.SwitchOrg))
	mux.Handle("PUT /api/auth/settings", WrapAuth(userService, memService, jwtSecretBytes, viewer, uh.UpdateUserSettings))

	// Settings endpoints (authenticated with org)
	mux.Handle("PUT /api/{orgID}/settings/membership", WrapAuth(userService, memService, jwtSecretBytes, viewer, mh.UpdateMembershipSettings))
	mux.Handle("GET /api/{orgID}/settings/organization", WrapAuth(userService, memService, jwtSecretBytes, viewer, orgh.GetOrganization))
	mux.Handle("PUT /api/{orgID}/settings/organization", WrapAuth(userService, memService, jwtSecretBytes, admin, orgh.UpdateSettings))
	mux.Handle("GET /api/{orgID}/onboarding", WrapAuth(userService, memService, jwtSecretBytes, viewer, orgh.GetOnboarding))
	mux.Handle("PUT /api/{orgID}/onboarding", WrapAuth(userService, memService, jwtSecretBytes, admin, orgh.UpdateOnboarding))

	// Git Remote endpoints (one remote per org)
	mux.Handle("GET /api/{orgID}/settings/git/remote", WrapAuth(userService, memService, jwtSecretBytes, admin, grh.GetRemote))
	mux.Handle("PUT /api/{orgID}/settings/git/remote", WrapAuth(userService, memService, jwtSecretBytes, admin, grh.SetRemote))
	mux.Handle("POST /api/{orgID}/settings/git/remote/push", WrapAuth(userService, memService, jwtSecretBytes, admin, grh.Push))
	mux.Handle("DELETE /api/{orgID}/settings/git/remote", WrapAuth(userService, memService, jwtSecretBytes, admin, grh.DeleteRemote))

	// OAuth endpoints (public)
	if (googleClientID != "" && googleClientSecret != "") || (msClientID != "" && msClientSecret != "") {
		oh := handlers.NewOAuthHandler(userService, memService, orgService, pageService, authh)
		base := strings.TrimRight(baseURL, "/")
		if googleClientID != "" && googleClientSecret != "" {
			oh.AddProvider("google", googleClientID, googleClientSecret, base+"/api/auth/oauth/google/callback")
		}
		if msClientID != "" && msClientSecret != "" {
			oh.AddProvider("microsoft", msClientID, msClientSecret, base+"/api/auth/oauth/microsoft/callback")
		}
		mux.HandleFunc("GET /api/auth/oauth/{provider}", oh.LoginRedirect)
		mux.HandleFunc("GET /api/auth/oauth/{provider}/callback", oh.Callback)
	}

	// User management endpoints
	mux.Handle("GET /api/{orgID}/users", WrapAuth(userService, memService, jwtSecretBytes, admin, uh.ListUsers))
	mux.Handle("PUT /api/{orgID}/users/role", WrapAuth(userService, memService, jwtSecretBytes, admin, uh.UpdateUserRole))

	// Invitation endpoints
	mux.Handle("GET /api/{orgID}/invitations", WrapAuth(userService, memService, jwtSecretBytes, admin, ih.ListInvitations))
	mux.Handle("POST /api/{orgID}/invitations", WrapAuth(userService, memService, jwtSecretBytes, admin, ih.CreateInvitation))

	// Unified Nodes endpoints
	mux.Handle("GET /api/{orgID}/nodes", WrapAuth(userService, memService, jwtSecretBytes, viewer, nh.ListNodes))
	mux.Handle("GET /api/{orgID}/nodes/{id}", WrapAuth(userService, memService, jwtSecretBytes, viewer, nh.GetNode))
	mux.Handle("POST /api/{orgID}/nodes", WrapAuth(userService, memService, jwtSecretBytes, editor, nh.CreateNode))

	// Pages endpoints
	mux.Handle("GET /api/{orgID}/pages", WrapAuth(userService, memService, jwtSecretBytes, viewer, ph.ListPages))
	mux.Handle("GET /api/{orgID}/pages/{id}", WrapAuth(userService, memService, jwtSecretBytes, viewer, ph.GetPage))
	mux.Handle("GET /api/{orgID}/pages/{id}/history", WrapAuth(userService, memService, jwtSecretBytes, viewer, ph.GetPageHistory))
	mux.Handle("GET /api/{orgID}/pages/{id}/history/{hash}", WrapAuth(userService, memService, jwtSecretBytes, viewer, ph.GetPageVersion))
	mux.Handle("POST /api/{orgID}/pages", WrapAuth(userService, memService, jwtSecretBytes, editor, ph.CreatePage))
	mux.Handle("PUT /api/{orgID}/pages/{id}", WrapAuth(userService, memService, jwtSecretBytes, editor, ph.UpdatePage))
	mux.Handle("DELETE /api/{orgID}/pages/{id}", WrapAuth(userService, memService, jwtSecretBytes, editor, ph.DeletePage))

	// Database endpoints
	mux.Handle("GET /api/{orgID}/databases", WrapAuth(userService, memService, jwtSecretBytes, viewer, dh.ListDatabases))
	mux.Handle("GET /api/{orgID}/databases/{id}", WrapAuth(userService, memService, jwtSecretBytes, viewer, dh.GetDatabase))
	mux.Handle("POST /api/{orgID}/databases", WrapAuth(userService, memService, jwtSecretBytes, editor, dh.CreateDatabase))
	mux.Handle("PUT /api/{orgID}/databases/{id}", WrapAuth(userService, memService, jwtSecretBytes, editor, dh.UpdateDatabase))
	mux.Handle("DELETE /api/{orgID}/databases/{id}", WrapAuth(userService, memService, jwtSecretBytes, editor, dh.DeleteDatabase))

	// Records endpoints
	mux.Handle("GET /api/{orgID}/databases/{id}/records", WrapAuth(userService, memService, jwtSecretBytes, viewer, dh.ListRecords))
	mux.Handle("GET /api/{orgID}/databases/{id}/records/{rid}", WrapAuth(userService, memService, jwtSecretBytes, viewer, dh.GetRecord))
	mux.Handle("POST /api/{orgID}/databases/{id}/records", WrapAuth(userService, memService, jwtSecretBytes, editor, dh.CreateRecord))
	mux.Handle("PUT /api/{orgID}/databases/{id}/records/{rid}", WrapAuth(userService, memService, jwtSecretBytes, editor, dh.UpdateRecord))
	mux.Handle("DELETE /api/{orgID}/databases/{id}/records/{rid}", WrapAuth(userService, memService, jwtSecretBytes, editor, dh.DeleteRecord))

	// Assets endpoints (page-based)
	mux.Handle("GET /api/{orgID}/pages/{id}/assets", WrapAuth(userService, memService, jwtSecretBytes, viewer, ah.ListPageAssets))
	mux.Handle("POST /api/{orgID}/pages/{id}/assets", WrapAuthRaw(userService, memService, jwtSecretBytes, editor, ah.UploadPageAssetHandler))
	mux.Handle("DELETE /api/{orgID}/pages/{id}/assets/{name}", WrapAuth(userService, memService, jwtSecretBytes, editor, ah.DeletePageAsset))

	// Search endpoint
	mux.Handle("POST /api/{orgID}/search", WrapAuth(userService, memService, jwtSecretBytes, viewer, sh.Search))

	// File serving (raw asset files) - public for now
	mux.HandleFunc("GET /assets/{orgID}/{id}/{name}", ah.ServeAssetFile)

	// Serve embedded SolidJS frontend with SPA fallback
	mux.Handle("/", NewEmbeddedSPAHandler(frontend.Files))

	return mux
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
