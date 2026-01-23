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
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/maruel/mddb/backend/frontend"
	"github.com/maruel/mddb/backend/internal/server/handlers"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// NewRouter creates and configures the HTTP router.
// Serves API endpoints at /api/* and static SolidJS frontend at /.
// baseURL is used for constructing OAuth callback URLs (e.g., "http://localhost:8080" or "https://example.com").
func NewRouter(fileStore *content.FileStore, userService *identity.UserService, orgService *identity.OrganizationService, invService *identity.InvitationService, memService *identity.MembershipService, jwtSecret, baseURL, googleClientID, googleClientSecret, msClientID, msClientSecret string) http.Handler {
	mux := &http.ServeMux{}
	jwtSecretBytes := []byte(jwtSecret)

	ph := handlers.NewPageHandler(fileStore)
	th := handlers.NewTableHandler(fileStore)
	nh := handlers.NewNodeHandler(fileStore)
	ah := handlers.NewAssetHandler(fileStore)
	sh := handlers.NewSearchHandler(fileStore)
	authh := handlers.NewAuthHandler(userService, memService, orgService, fileStore, jwtSecret)
	uh := handlers.NewUserHandler(userService, memService, orgService)
	ih := handlers.NewInvitationHandler(invService, userService, orgService, memService)
	mh := handlers.NewMembershipHandler(memService, userService, orgService, authh)
	orgh := handlers.NewOrganizationHandler(orgService)
	grh := handlers.NewGitRemoteHandler(orgService, fileStore.Git)

	// Helper to create WrapAuth with common deps
	viewer := identity.UserRoleViewer
	editor := identity.UserRoleEditor
	admin := identity.UserRoleAdmin

	// Health check (public)
	hh := handlers.NewHealthHandler("1.0.0")
	mux.Handle("/api/health", Wrap(hh.GetHealth))

	// Global admin endpoints (requires IsGlobalAdmin)
	adminh := handlers.NewAdminHandler(userService, orgService, memService)
	mux.Handle("GET /api/admin/stats", WrapGlobalAdmin(userService, jwtSecretBytes, adminh.GetAdminStats))
	mux.Handle("GET /api/admin/users", WrapGlobalAdmin(userService, jwtSecretBytes, adminh.ListAllUsers))
	mux.Handle("GET /api/admin/organizations", WrapGlobalAdmin(userService, jwtSecretBytes, adminh.ListAllOrgs))

	// Auth endpoints (public)
	mux.Handle("POST /api/auth/login", Wrap(authh.Login))
	mux.Handle("POST /api/auth/register", Wrap(authh.Register))
	mux.Handle("POST /api/auth/invitations/accept", Wrap(ih.AcceptInvitation))

	// Auth endpoints (authenticated, no org)
	mux.Handle("GET /api/auth/me", WrapAuth(userService, memService, jwtSecretBytes, viewer, authh.GetMe))
	mux.Handle("POST /api/auth/switch-org", WrapAuth(userService, memService, jwtSecretBytes, viewer, mh.SwitchOrg))
	mux.Handle("POST /api/auth/settings", WrapAuth(userService, memService, jwtSecretBytes, viewer, uh.UpdateUserSettings))
	mux.Handle("POST /api/organizations", WrapAuth(userService, memService, jwtSecretBytes, viewer, authh.CreateOrganization))

	// Settings endpoints (authenticated with org)
	mux.Handle("POST /api/{orgID}/settings/membership", WrapAuth(userService, memService, jwtSecretBytes, viewer, mh.UpdateMembershipSettings))
	mux.Handle("GET /api/{orgID}/settings/organization", WrapAuth(userService, memService, jwtSecretBytes, viewer, orgh.GetOrganization))
	mux.Handle("POST /api/{orgID}/settings/organization", WrapAuth(userService, memService, jwtSecretBytes, admin, orgh.UpdateOrganization))
	mux.Handle("POST /api/{orgID}/settings/preferences", WrapAuth(userService, memService, jwtSecretBytes, admin, orgh.UpdateOrgPreferences))

	// Git Remote endpoints (one remote per org)
	mux.Handle("GET /api/{orgID}/settings/git", WrapAuth(userService, memService, jwtSecretBytes, admin, grh.GetGitRemote))
	mux.Handle("POST /api/{orgID}/settings/git", WrapAuth(userService, memService, jwtSecretBytes, admin, grh.UpdateGitRemote))
	mux.Handle("POST /api/{orgID}/settings/git/push", WrapAuth(userService, memService, jwtSecretBytes, admin, grh.PushGit))
	mux.Handle("POST /api/{orgID}/settings/git/delete", WrapAuth(userService, memService, jwtSecretBytes, admin, grh.DeleteGitRemote))

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
	mux.HandleFunc("GET /api/auth/oauth/{provider}", oh.LoginRedirect)
	mux.HandleFunc("GET /api/auth/oauth/{provider}/callback", oh.Callback)

	// User management endpoints
	mux.Handle("GET /api/{orgID}/users", WrapAuth(userService, memService, jwtSecretBytes, admin, uh.ListUsers))
	mux.Handle("POST /api/{orgID}/users/role", WrapAuth(userService, memService, jwtSecretBytes, admin, uh.UpdateUserRole))

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
	mux.Handle("GET /api/{orgID}/pages/{id}/history", WrapAuth(userService, memService, jwtSecretBytes, viewer, ph.ListPageVersions))
	mux.Handle("GET /api/{orgID}/pages/{id}/history/{hash}", WrapAuth(userService, memService, jwtSecretBytes, viewer, ph.GetPageVersion))
	mux.Handle("POST /api/{orgID}/pages", WrapAuth(userService, memService, jwtSecretBytes, editor, ph.CreatePage))
	mux.Handle("POST /api/{orgID}/pages/{id}", WrapAuth(userService, memService, jwtSecretBytes, editor, ph.UpdatePage))
	mux.Handle("POST /api/{orgID}/pages/{id}/delete", WrapAuth(userService, memService, jwtSecretBytes, editor, ph.DeletePage))

	// Table endpoints
	mux.Handle("GET /api/{orgID}/tables", WrapAuth(userService, memService, jwtSecretBytes, viewer, th.ListTables))
	mux.Handle("GET /api/{orgID}/tables/{id}", WrapAuth(userService, memService, jwtSecretBytes, viewer, th.GetTable))
	mux.Handle("POST /api/{orgID}/tables", WrapAuth(userService, memService, jwtSecretBytes, editor, th.CreateTable))
	mux.Handle("POST /api/{orgID}/tables/{id}", WrapAuth(userService, memService, jwtSecretBytes, editor, th.UpdateTable))
	mux.Handle("POST /api/{orgID}/tables/{id}/delete", WrapAuth(userService, memService, jwtSecretBytes, editor, th.DeleteTable))

	// Records endpoints
	mux.Handle("GET /api/{orgID}/tables/{id}/records", WrapAuth(userService, memService, jwtSecretBytes, viewer, th.ListRecords))
	mux.Handle("GET /api/{orgID}/tables/{id}/records/{rid}", WrapAuth(userService, memService, jwtSecretBytes, viewer, th.GetRecord))
	mux.Handle("POST /api/{orgID}/tables/{id}/records", WrapAuth(userService, memService, jwtSecretBytes, editor, th.CreateRecord))
	mux.Handle("POST /api/{orgID}/tables/{id}/records/{rid}", WrapAuth(userService, memService, jwtSecretBytes, editor, th.UpdateRecord))
	mux.Handle("POST /api/{orgID}/tables/{id}/records/{rid}/delete", WrapAuth(userService, memService, jwtSecretBytes, editor, th.DeleteRecord))

	// Assets endpoints (page-based)
	mux.Handle("GET /api/{orgID}/pages/{id}/assets", WrapAuth(userService, memService, jwtSecretBytes, viewer, ah.ListPageAssets))
	mux.Handle("POST /api/{orgID}/pages/{id}/assets", WrapAuthRaw(userService, memService, jwtSecretBytes, editor, ah.UploadPageAssetHandler))
	mux.Handle("POST /api/{orgID}/pages/{id}/assets/{name}/delete", WrapAuth(userService, memService, jwtSecretBytes, editor, ah.DeletePageAsset))

	// Search endpoint
	mux.Handle("POST /api/{orgID}/search", WrapAuth(userService, memService, jwtSecretBytes, viewer, sh.Search))

	// File serving (raw asset files) - public for now
	mux.HandleFunc("GET /assets/{orgID}/{id}/{name}", ah.ServeAssetFile)

	// API catch-all - return 404 for any unmatched /api/ routes (never fall through to SPA)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Not found", http.StatusNotFound)
	})

	// Serve embedded SolidJS frontend with SPA fallback
	mux.Handle("/", NewEmbeddedSPAHandler(frontend.Files))

	f := func(w http.ResponseWriter, r *http.Request) {
		clientIP, err := getRealIP(r)
		ctx := r.Context()
		if err != nil {
			slog.ErrorContext(ctx, "Failed to determine client IP", "err", err)
			http.Error(w, "Can't determine IP address", http.StatusPreconditionFailed)
			return
		}
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		mux.ServeHTTP(rw, r)
		slog.InfoContext(ctx, "http",
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

// getRealIP extracts the client's real IP address from an HTTP request,
// taking into account X-Forwarded-For or other proxy headers.
func getRealIP(r *http.Request) (net.IP, error) {
	// Check X-Forwarded-For header (most common proxy header)
	if xForwardedFor := r.Header.Get("X-Forwarded-For"); xForwardedFor != "" {
		// X-Forwarded-For can contain multiple IPs, the client's IP is the first one
		ip := net.ParseIP(strings.TrimSpace(strings.Split(xForwardedFor, ",")[0]))
		if ip != nil {
			return ip, nil
		}
	}

	// Check X-Real-IP header (used by some proxies)
	if xRealIP := r.Header.Get("X-Real-IP"); xRealIP != "" {
		if ip := net.ParseIP(xRealIP); ip != nil {
			return ip, nil
		}
	}

	// If no proxy headers found, get the remote address
	if remoteAddr := r.RemoteAddr; remoteAddr != "" {
		// RemoteAddr might be in the format IP:port
		if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
			if ip := net.ParseIP(host); ip != nil {
				return ip, nil
			}
		} else {
			// If SplitHostPort fails, try parsing the whole RemoteAddr as an IP
			if ip := net.ParseIP(remoteAddr); ip != nil {
				return ip, nil
			}
		}
	}
	return nil, errors.New("could not determine client IP address")
}
