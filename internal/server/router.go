// Package server implements the HTTP server and routing logic.
package server

import (
	"embed"
	"io"
	"io/fs"
	"net/http"

	"github.com/maruel/mddb/frontend"
	"github.com/maruel/mddb/internal/models"
	"github.com/maruel/mddb/internal/server/handlers"
	"github.com/maruel/mddb/internal/storage"
)

// NewRouter creates and configures the HTTP router.
// Serves API endpoints at /api/* and static SolidJS frontend at /.
func NewRouter(fileStore *storage.FileStore, gitService *storage.GitService, userService *storage.UserService, orgService *storage.OrganizationService, invService *storage.InvitationService, memService *storage.MembershipService, remoteService *storage.GitRemoteService, jwtSecret, googleClientID, googleClientSecret, msClientID, msClientSecret string) http.Handler {
	cache := storage.NewCache()
	mux := &http.ServeMux{}
	ph := handlers.NewPageHandler(fileStore, gitService, cache, orgService)
	dh := handlers.NewDatabaseHandler(fileStore, gitService, cache, orgService)
	nh := handlers.NewNodeHandler(fileStore, gitService, cache, orgService)
	ah := handlers.NewAssetHandler(fileStore, gitService, orgService)
	sh := handlers.NewSearchHandler(fileStore)
	authh := handlers.NewAuthHandler(userService, orgService, jwtSecret)
	uh := handlers.NewUserHandler(userService)
	ih := handlers.NewInvitationHandler(invService, userService, orgService, memService)
	mh := handlers.NewMembershipHandler(memService, userService, authh)
	orgh := handlers.NewOrganizationHandler(orgService)
	grh := handlers.NewGitRemoteHandler(remoteService, gitService, orgService.RootDir()) // Added rootDir accessor

	// Health check
	hh := handlers.NewHealthHandler("1.0.0")
	mux.Handle("/api/health", Wrap(hh.Health))

	// Auth endpoints
	mux.Handle("POST /api/auth/login", Wrap(authh.Login))
	mux.Handle("POST /api/auth/register", Wrap(authh.Register))
	mux.Handle("GET /api/auth/me", Wrap(authh.Me))
	mux.Handle("POST /api/auth/invitations/accept", Wrap(ih.AcceptInvitation))
	mux.Handle("POST /api/auth/switch-org", Wrap(mh.SwitchOrg))

	// Settings endpoints
	mux.Handle("PUT /api/auth/settings", Wrap(uh.UpdateUserSettings))
	mux.Handle("PUT /api/{orgID}/settings/membership", RequireRole(memService, models.UserRoleViewer)(Wrap(mh.UpdateMembershipSettings)))
	mux.Handle("GET /api/{orgID}/settings/organization", RequireRole(memService, models.UserRoleViewer)(Wrap(orgh.GetOrganization)))
	mux.Handle("PUT /api/{orgID}/settings/organization", RequireRole(memService, models.UserRoleAdmin)(Wrap(orgh.UpdateSettings)))
	mux.Handle("GET /api/{orgID}/onboarding", RequireRole(memService, models.UserRoleViewer)(Wrap(orgh.GetOnboarding)))
	mux.Handle("PUT /api/{orgID}/onboarding", RequireRole(memService, models.UserRoleAdmin)(Wrap(orgh.UpdateOnboarding)))

	// Git Remote endpoints
	mux.Handle("GET /api/{orgID}/settings/git/remotes", RequireRole(memService, models.UserRoleAdmin)(Wrap(grh.ListRemotes)))
	mux.Handle("POST /api/{orgID}/settings/git/remotes", RequireRole(memService, models.UserRoleAdmin)(Wrap(grh.CreateRemote)))
	mux.Handle("POST /api/{orgID}/settings/git/remotes/{remoteID}/push", RequireRole(memService, models.UserRoleAdmin)(Wrap(grh.Push)))
	mux.Handle("DELETE /api/{orgID}/settings/git/remotes/{remoteID}", RequireRole(memService, models.UserRoleAdmin)(Wrap(grh.DeleteRemote)))

	// OAuth endpoints
	if (googleClientID != "" && googleClientSecret != "") || (msClientID != "" && msClientSecret != "") {
		oh := handlers.NewOAuthHandler(userService, orgService, authh)
		if googleClientID != "" && googleClientSecret != "" {
			oh.AddProvider("google", googleClientID, googleClientSecret, "/api/auth/oauth/google/callback")
		}
		if msClientID != "" && msClientSecret != "" {
			oh.AddProvider("microsoft", msClientID, msClientSecret, "/api/auth/oauth/microsoft/callback")
		}
		mux.HandleFunc("GET /api/auth/oauth/{provider}", oh.LoginRedirect)
		mux.HandleFunc("GET /api/auth/oauth/{provider}/callback", oh.Callback)
	}

	// User management endpoints
	mux.Handle("GET /api/{orgID}/users", RequireRole(memService, models.UserRoleAdmin)(Wrap(uh.ListUsers)))
	mux.Handle("PUT /api/{orgID}/users/role", RequireRole(memService, models.UserRoleAdmin)(Wrap(uh.UpdateUserRole)))

	// Invitation endpoints
	mux.Handle("GET /api/{orgID}/invitations", RequireRole(memService, models.UserRoleAdmin)(Wrap(ih.ListInvitations)))
	mux.Handle("POST /api/{orgID}/invitations", RequireRole(memService, models.UserRoleAdmin)(Wrap(ih.CreateInvitation)))

	// Unified Nodes endpoints
	mux.Handle("GET /api/{orgID}/nodes", RequireRole(memService, models.UserRoleViewer)(Wrap(nh.ListNodes)))
	mux.Handle("GET /api/{orgID}/nodes/{id}", RequireRole(memService, models.UserRoleViewer)(Wrap(nh.GetNode)))
	mux.Handle("POST /api/{orgID}/nodes", RequireRole(memService, models.UserRoleEditor)(Wrap(nh.CreateNode)))

	// Pages endpoints
	mux.Handle("GET /api/{orgID}/pages", RequireRole(memService, models.UserRoleViewer)(Wrap(ph.ListPages)))
	mux.Handle("GET /api/{orgID}/pages/{id}", RequireRole(memService, models.UserRoleViewer)(Wrap(ph.GetPage)))
	mux.Handle("GET /api/{orgID}/pages/{id}/history", RequireRole(memService, models.UserRoleViewer)(Wrap(ph.GetPageHistory)))
	mux.Handle("GET /api/{orgID}/pages/{id}/history/{hash}", RequireRole(memService, models.UserRoleViewer)(Wrap(ph.GetPageVersion)))
	mux.Handle("POST /api/{orgID}/pages", RequireRole(memService, models.UserRoleEditor)(Wrap(ph.CreatePage)))
	mux.Handle("PUT /api/{orgID}/pages/{id}", RequireRole(memService, models.UserRoleEditor)(Wrap(ph.UpdatePage)))
	mux.Handle("DELETE /api/{orgID}/pages/{id}", RequireRole(memService, models.UserRoleEditor)(Wrap(ph.DeletePage)))

	// Database endpoints
	mux.Handle("GET /api/{orgID}/databases", RequireRole(memService, models.UserRoleViewer)(Wrap(dh.ListDatabases)))
	mux.Handle("GET /api/{orgID}/databases/{id}", RequireRole(memService, models.UserRoleViewer)(Wrap(dh.GetDatabase)))
	mux.Handle("POST /api/{orgID}/databases", RequireRole(memService, models.UserRoleEditor)(Wrap(dh.CreateDatabase)))
	mux.Handle("PUT /api/{orgID}/databases/{id}", RequireRole(memService, models.UserRoleEditor)(Wrap(dh.UpdateDatabase)))
	mux.Handle("DELETE /api/{orgID}/databases/{id}", RequireRole(memService, models.UserRoleEditor)(Wrap(dh.DeleteDatabase)))

	// Records endpoints
	mux.Handle("GET /api/{orgID}/databases/{id}/records", RequireRole(memService, models.UserRoleViewer)(Wrap(dh.ListRecords)))
	mux.Handle("GET /api/{orgID}/databases/{id}/records/{rid}", RequireRole(memService, models.UserRoleViewer)(Wrap(dh.GetRecord)))
	mux.Handle("POST /api/{orgID}/databases/{id}/records", RequireRole(memService, models.UserRoleEditor)(Wrap(dh.CreateRecord)))
	mux.Handle("PUT /api/{orgID}/databases/{id}/records/{rid}", RequireRole(memService, models.UserRoleEditor)(Wrap(dh.UpdateRecord)))
	mux.Handle("DELETE /api/{orgID}/databases/{id}/records/{rid}", RequireRole(memService, models.UserRoleEditor)(Wrap(dh.DeleteRecord)))

	// Assets endpoints (page-based)
	mux.Handle("GET /api/{orgID}/pages/{id}/assets", RequireRole(memService, models.UserRoleViewer)(Wrap(ah.ListPageAssets)))
	mux.Handle("POST /api/{orgID}/pages/{id}/assets", RequireRole(memService, models.UserRoleEditor)(http.HandlerFunc(ah.UploadPageAssetHandler)))
	mux.Handle("DELETE /api/{orgID}/pages/{id}/assets/{name}", RequireRole(memService, models.UserRoleEditor)(Wrap(ah.DeletePageAsset)))

	// Search endpoint
	mux.Handle("POST /api/{orgID}/search", RequireRole(memService, models.UserRoleViewer)(Wrap(sh.Search)))

	// File serving (raw asset files)
	mux.HandleFunc("GET /assets/{orgID}/{id}/{name}", ah.ServeAssetFile)

	// Serve embedded SolidJS frontend with SPA fallback
	mux.Handle("/", NewEmbeddedSPAHandler(frontend.Files))

	// Apply Auth Middleware to all API requests
	return AuthMiddleware(userService, []byte(jwtSecret))(mux)
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
		_ = f.Close()
		// File exists, serve it from embedded FS
		fsys, _ := fs.Sub(h.fs, "dist")
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
	defer func() { _ = indexFile.Close() }()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	// Serve index.html
	_, _ = io.Copy(w, indexFile)
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
