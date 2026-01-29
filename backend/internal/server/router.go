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
	"github.com/maruel/mddb/backend/internal/server/bandwidth"
	"github.com/maruel/mddb/backend/internal/server/handlers"
	"github.com/maruel/mddb/backend/internal/server/ratelimit"
	"github.com/maruel/mddb/backend/internal/server/reqctx"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// Config holds configuration for the router.
type Config struct {
	*storage.ServerConfig
	DataDir   string
	BaseURL   string
	Version   string
	GoVersion string
	Revision  string
	Dirty     bool
	OAuth     OAuthConfig
}

// OAuthConfig holds OAuth provider credentials.
type OAuthConfig struct {
	GoogleClientID     string
	GoogleClientSecret string
	MSClientID         string
	MSClientSecret     string
	GitHubClientID     string
	GitHubClientSecret string
}

// NewRouter creates and configures the HTTP router.
// Serves API endpoints at /api/* and static SolidJS frontend at /.
// Services.Email and Services.EmailVerif may be nil if SMTP is not configured.
func NewRouter(svc *handlers.Services, cfg *Config) http.Handler {
	mux := &http.ServeMux{}

	// Create rate limiters from storage config
	rlCfg := ratelimit.ConfigFromStorage(
		cfg.RateLimits.AuthRatePerMin,
		cfg.RateLimits.WriteRatePerMin,
		cfg.RateLimits.ReadAuthRatePerMin,
		cfg.RateLimits.ReadUnauthRatePerMin,
	)
	limiters := ratelimit.NewLimiters(rlCfg)

	// Create bandwidth limiter
	bandwidthLim := bandwidth.NewLimiter(cfg.Quotas.MaxEgressBandwidthBps)

	// Create handler config from server config
	hcfg := &handlers.Config{
		ServerConfig: *cfg.ServerConfig,
		BaseURL:      cfg.BaseURL,
		Version:      cfg.Version,
		GoVersion:    cfg.GoVersion,
		Revision:     cfg.Revision,
		Dirty:        cfg.Dirty,
	}

	// Auth handler (needs New* for map initialization)
	authh := handlers.NewAuthHandler(svc, hcfg)

	// Content handlers
	ah := &handlers.AssetHandler{Svc: svc, Cfg: hcfg}
	nh := &handlers.NodeHandler{Svc: svc, Cfg: hcfg}
	sh := &handlers.SearchHandler{Svc: svc}

	// Other handlers
	uh := &handlers.UserHandler{Svc: svc}
	ih := &handlers.InvitationHandler{Svc: svc, Cfg: hcfg}
	mh := &handlers.MembershipHandler{Svc: svc, Cfg: hcfg}
	orgh := &handlers.OrganizationHandler{Svc: svc, Cfg: hcfg}
	grh := &handlers.GitRemoteHandler{Svc: svc}

	// Health check (public)
	hh := &handlers.HealthHandler{Cfg: hcfg}
	mux.Handle("/api/health", Wrap(hh.GetHealth, hcfg, limiters))

	// Admin endpoints (requires IsGlobalAdmin)
	adminh := &handlers.AdminHandler{Svc: svc}
	mux.Handle("GET /api/admin/stats", WrapGlobalAdmin(adminh.GetAdminStats, svc, hcfg, limiters))
	mux.Handle("GET /api/admin/users", WrapGlobalAdmin(adminh.ListAllUsers, svc, hcfg, limiters))
	mux.Handle("GET /api/admin/organizations", WrapGlobalAdmin(adminh.ListAllOrgs, svc, hcfg, limiters))

	// Server config endpoints (requires IsGlobalAdmin)
	serverh := &handlers.ServerHandler{Cfg: cfg.ServerConfig, DataDir: cfg.DataDir, BandwidthLimiter: bandwidthLim, RateLimiters: limiters}
	mux.Handle("GET /api/server/config", WrapGlobalAdmin(serverh.GetConfig, svc, hcfg, limiters))
	mux.Handle("POST /api/server/config", WrapGlobalAdmin(serverh.UpdateConfig, svc, hcfg, limiters))

	// OAuth handler setup (needed before auth routes)
	oh := handlers.NewOAuthHandler(svc, hcfg)
	base := strings.TrimRight(cfg.BaseURL, "/")
	var providers []identity.OAuthProvider
	if cfg.OAuth.GoogleClientID != "" && cfg.OAuth.GoogleClientSecret != "" {
		oh.AddProvider(identity.OAuthProviderGoogle, cfg.OAuth.GoogleClientID, cfg.OAuth.GoogleClientSecret, base+"/api/auth/oauth/google/callback")
		providers = append(providers, identity.OAuthProviderGoogle)
	}
	if cfg.OAuth.MSClientID != "" && cfg.OAuth.MSClientSecret != "" {
		oh.AddProvider(identity.OAuthProviderMicrosoft, cfg.OAuth.MSClientID, cfg.OAuth.MSClientSecret, base+"/api/auth/oauth/microsoft/callback")
		providers = append(providers, identity.OAuthProviderMicrosoft)
	}
	if cfg.OAuth.GitHubClientID != "" && cfg.OAuth.GitHubClientSecret != "" {
		oh.AddProvider(identity.OAuthProviderGitHub, cfg.OAuth.GitHubClientID, cfg.OAuth.GitHubClientSecret, base+"/api/auth/oauth/github/callback")
		providers = append(providers, identity.OAuthProviderGitHub)
	}
	if len(providers) > 0 {
		slog.Info("OAuth providers initialized", "providers", providers)
	} else {
		slog.Info("No OAuth providers configured")
	}

	// Auth endpoints - /api/auth/*
	// Public
	mux.Handle("POST /api/auth/login", Wrap(authh.Login, hcfg, limiters))
	mux.Handle("POST /api/auth/register", Wrap(authh.Register, hcfg, limiters))
	mux.Handle("POST /api/auth/invitations/org/accept", Wrap(ih.AcceptOrgInvitation, hcfg, limiters))
	mux.Handle("POST /api/auth/invitations/ws/accept", Wrap(ih.AcceptWSInvitation, hcfg, limiters))
	mux.HandleFunc("GET /api/auth/email/verify", authh.VerifyEmailRedirect)
	mux.Handle("GET /api/auth/providers", Wrap(oh.ListProviders, hcfg, limiters))
	mux.HandleFunc("GET /api/auth/oauth/{provider}", oh.LoginRedirect)
	mux.HandleFunc("GET /api/auth/oauth/{provider}/callback", oh.Callback)
	// Authenticated
	mux.Handle("GET /api/auth/me", WrapAuth(authh.GetMe, svc, hcfg, limiters))
	mux.Handle("POST /api/auth/logout", WrapAuth(authh.Logout, svc, hcfg, limiters))
	mux.Handle("GET /api/auth/sessions", WrapAuth(authh.ListSessions, svc, hcfg, limiters))
	mux.Handle("POST /api/auth/sessions/revoke", WrapAuth(authh.RevokeSession, svc, hcfg, limiters))
	mux.Handle("POST /api/auth/sessions/revoke-all", WrapAuth(authh.RevokeAllSessions, svc, hcfg, limiters))
	mux.Handle("POST /api/auth/email", WrapAuth(authh.ChangeEmail, svc, hcfg, limiters))
	mux.Handle("POST /api/auth/email/send-verification", WrapAuth(authh.SendVerificationEmail, svc, hcfg, limiters))
	mux.Handle("POST /api/auth/switch-workspace", WrapAuth(mh.SwitchWorkspace, svc, hcfg, limiters))
	mux.Handle("POST /api/auth/settings", WrapAuth(uh.UpdateUserSettings, svc, hcfg, limiters))
	mux.Handle("POST /api/auth/oauth/link", WrapAuth(oh.LinkOAuth, svc, hcfg, limiters))
	mux.Handle("POST /api/auth/oauth/unlink", WrapAuth(oh.UnlinkOAuth, svc, hcfg, limiters))

	// Organization endpoints - /api/organizations/*
	mux.Handle("POST /api/organizations", WrapAuth(authh.CreateOrganization, svc, hcfg, limiters))
	mux.Handle("GET /api/organizations/{orgID}", WrapOrgAuth(orgh.GetOrganization, svc, hcfg, identity.OrgRoleMember, limiters))
	mux.Handle("POST /api/organizations/{orgID}", WrapOrgAuth(orgh.UpdateOrganization, svc, hcfg, identity.OrgRoleAdmin, limiters))
	mux.Handle("POST /api/organizations/{orgID}/settings", WrapOrgAuth(orgh.UpdateOrgPreferences, svc, hcfg, identity.OrgRoleAdmin, limiters))
	mux.Handle("GET /api/organizations/{orgID}/users", WrapOrgAuth(uh.ListUsers, svc, hcfg, identity.OrgRoleAdmin, limiters))
	mux.Handle("POST /api/organizations/{orgID}/users/role", WrapOrgAuth(uh.UpdateOrgMemberRole, svc, hcfg, identity.OrgRoleAdmin, limiters))
	mux.Handle("GET /api/organizations/{orgID}/invitations", WrapOrgAuth(ih.ListOrgInvitations, svc, hcfg, identity.OrgRoleAdmin, limiters))
	mux.Handle("POST /api/organizations/{orgID}/invitations", WrapOrgAuth(ih.CreateOrgInvitation, svc, hcfg, identity.OrgRoleAdmin, limiters))
	mux.Handle("POST /api/organizations/{orgID}/workspaces", WrapOrgAuth(orgh.CreateWorkspace, svc, hcfg, identity.OrgRoleAdmin, limiters))

	// Notion import endpoints
	nih := handlers.NewNotionImportHandler(svc, hcfg)
	mux.Handle("POST /api/organizations/{orgID}/notion/import", WrapOrgAuth(nih.StartImport, svc, hcfg, identity.OrgRoleAdmin, limiters))
	mux.Handle("GET /api/organizations/{orgID}/notion/import/{importWsID}/status", WrapOrgAuth(nih.GetStatus, svc, hcfg, identity.OrgRoleMember, limiters))

	// Workspace endpoints - /api/workspaces/*
	// Details and settings
	mux.Handle("GET /api/workspaces/{wsID}", WrapWSAuth(orgh.GetWorkspace, svc, hcfg, identity.WSRoleViewer, limiters))
	mux.Handle("POST /api/workspaces/{wsID}", WrapWSAuth(orgh.UpdateWorkspace, svc, hcfg, identity.WSRoleAdmin, limiters))
	mux.Handle("POST /api/workspaces/{wsID}/settings/membership", WrapWSAuth(mh.UpdateWSMembershipSettings, svc, hcfg, identity.WSRoleViewer, limiters))
	mux.Handle("GET /api/workspaces/{wsID}/settings/git", WrapWSAuth(grh.GetGitRemote, svc, hcfg, identity.WSRoleAdmin, limiters))
	mux.Handle("POST /api/workspaces/{wsID}/settings/git", WrapWSAuth(grh.UpdateGitRemote, svc, hcfg, identity.WSRoleAdmin, limiters))
	mux.Handle("POST /api/workspaces/{wsID}/settings/git/push", WrapWSAuth(grh.PushGit, svc, hcfg, identity.WSRoleAdmin, limiters))
	mux.Handle("POST /api/workspaces/{wsID}/settings/git/delete", WrapWSAuth(grh.DeleteGitRemote, svc, hcfg, identity.WSRoleAdmin, limiters))
	// Notion import cancel
	mux.Handle("POST /api/workspaces/{wsID}/notion/import/cancel", WrapWSAuth(nih.CancelImport, svc, hcfg, identity.WSRoleAdmin, limiters))
	// Users and invitations
	mux.Handle("POST /api/workspaces/{wsID}/users/role", WrapWSAuth(uh.UpdateWSMemberRole, svc, hcfg, identity.WSRoleAdmin, limiters))
	mux.Handle("GET /api/workspaces/{wsID}/invitations", WrapWSAuth(ih.ListWSInvitations, svc, hcfg, identity.WSRoleAdmin, limiters))
	mux.Handle("POST /api/workspaces/{wsID}/invitations", WrapWSAuth(ih.CreateWSInvitation, svc, hcfg, identity.WSRoleAdmin, limiters))
	// Nodes (id=0 is valid for root node)
	mux.Handle("GET /api/workspaces/{wsID}/nodes/titles", WrapWSAuth(nh.GetNodeTitles, svc, hcfg, identity.WSRoleViewer, limiters))
	mux.Handle("GET /api/workspaces/{wsID}/nodes/{id}", WrapWSAuth(nh.GetNode, svc, hcfg, identity.WSRoleViewer, limiters))
	mux.Handle("GET /api/workspaces/{wsID}/nodes/{id}/children", WrapWSAuth(nh.ListNodeChildren, svc, hcfg, identity.WSRoleViewer, limiters))
	mux.Handle("POST /api/workspaces/{wsID}/nodes/{id}/delete", WrapWSAuth(nh.DeleteNode, svc, hcfg, identity.WSRoleEditor, limiters))
	// Pages (under nodes)
	mux.Handle("POST /api/workspaces/{wsID}/nodes/{id}/page/create", WrapWSAuth(nh.CreatePage, svc, hcfg, identity.WSRoleEditor, limiters))
	mux.Handle("GET /api/workspaces/{wsID}/nodes/{id}/page", WrapWSAuth(nh.GetPage, svc, hcfg, identity.WSRoleViewer, limiters))
	mux.Handle("POST /api/workspaces/{wsID}/nodes/{id}/page", WrapWSAuth(nh.UpdatePage, svc, hcfg, identity.WSRoleEditor, limiters))
	mux.Handle("POST /api/workspaces/{wsID}/nodes/{id}/page/delete", WrapWSAuth(nh.DeletePage, svc, hcfg, identity.WSRoleEditor, limiters))
	// Tables (under nodes)
	mux.Handle("POST /api/workspaces/{wsID}/nodes/{id}/table/create", WrapWSAuth(nh.CreateTable, svc, hcfg, identity.WSRoleEditor, limiters))
	mux.Handle("GET /api/workspaces/{wsID}/nodes/{id}/table", WrapWSAuth(nh.GetTable, svc, hcfg, identity.WSRoleViewer, limiters))
	mux.Handle("POST /api/workspaces/{wsID}/nodes/{id}/table", WrapWSAuth(nh.UpdateTable, svc, hcfg, identity.WSRoleEditor, limiters))
	mux.Handle("POST /api/workspaces/{wsID}/nodes/{id}/table/delete", WrapWSAuth(nh.DeleteTable, svc, hcfg, identity.WSRoleEditor, limiters))
	// Records (under nodes/table)
	mux.Handle("GET /api/workspaces/{wsID}/nodes/{id}/table/records", WrapWSAuth(nh.ListRecords, svc, hcfg, identity.WSRoleViewer, limiters))
	mux.Handle("GET /api/workspaces/{wsID}/nodes/{id}/table/records/{rid}", WrapWSAuth(nh.GetRecord, svc, hcfg, identity.WSRoleViewer, limiters))
	mux.Handle("POST /api/workspaces/{wsID}/nodes/{id}/table/records/create", WrapWSAuth(nh.CreateRecord, svc, hcfg, identity.WSRoleEditor, limiters))
	mux.Handle("POST /api/workspaces/{wsID}/nodes/{id}/table/records/{rid}", WrapWSAuth(nh.UpdateRecord, svc, hcfg, identity.WSRoleEditor, limiters))
	mux.Handle("POST /api/workspaces/{wsID}/nodes/{id}/table/records/{rid}/delete", WrapWSAuth(nh.DeleteRecord, svc, hcfg, identity.WSRoleEditor, limiters))
	// History (under nodes)
	mux.Handle("GET /api/workspaces/{wsID}/nodes/{id}/history", WrapWSAuth(nh.ListNodeVersions, svc, hcfg, identity.WSRoleViewer, limiters))
	mux.Handle("GET /api/workspaces/{wsID}/nodes/{id}/history/{hash}", WrapWSAuth(nh.GetNodeVersion, svc, hcfg, identity.WSRoleViewer, limiters))
	// Assets (under nodes)
	mux.Handle("GET /api/workspaces/{wsID}/nodes/{id}/assets", WrapWSAuth(nh.ListNodeAssets, svc, hcfg, identity.WSRoleViewer, limiters))
	mux.Handle("POST /api/workspaces/{wsID}/nodes/{id}/assets", WrapAuthRaw(ah.UploadNodeAssetHandler, svc, hcfg, identity.WSRoleEditor, limiters))
	mux.Handle("POST /api/workspaces/{wsID}/nodes/{id}/assets/{name}/delete", WrapWSAuth(nh.DeleteNodeAsset, svc, hcfg, identity.WSRoleEditor, limiters))
	// Search
	mux.Handle("POST /api/workspaces/{wsID}/search", WrapWSAuth(sh.Search, svc, hcfg, identity.WSRoleViewer, limiters))

	// File serving (raw asset files) - requires signed URL (sig + exp query params)
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
		rw := &responseWriter{
			ResponseWriter:   w,
			status:           http.StatusOK,
			bandwidthLimiter: bandwidthLim,
		}
		mux.ServeHTTP(rw, r)
		slog.InfoContext(r.Context(), "http",
			"m", r.Method,
			"p", r.URL.Path,
			"s", rw.status,
			"d", roundDuration(time.Since(start)),
			"s", rw.size,
			"ip", clientIP,
		)
	}
	return http.HandlerFunc(f)
}

// responseWriter wraps http.ResponseWriter to capture status code, response size, and apply bandwidth limiting.
type responseWriter struct {
	http.ResponseWriter
	status           int
	size             int
	bandwidthLimiter *bandwidth.Limiter
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	// Apply bandwidth limiting before writing
	if rw.bandwidthLimiter != nil {
		waitDuration := rw.bandwidthLimiter.Allow(int64(len(b)))
		if waitDuration > 0 {
			time.Sleep(waitDuration)
		}
	}

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
	// Set security headers for all responses
	setSecurityHeaders(w)

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
		// Set appropriate cache headers based on file type
		setCacheHeaders(w, r.URL.Path)
		fileServer.ServeHTTP(w, r)
		return
	}

	// File not found - check if this is a static asset path that should 404
	if strings.HasPrefix(r.URL.Path, "/assets/") {
		http.NotFound(w, r)
		return
	}

	// Fall back to index.html for SPA routing
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

// setSecurityHeaders sets security-related HTTP headers.
func setSecurityHeaders(w http.ResponseWriter) {
	// Prevent MIME type sniffing
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Prevent clickjacking
	w.Header().Set("X-Frame-Options", "DENY")
	// Control referrer information
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	// Prevent XSS in older browsers (modern browsers ignore this)
	w.Header().Set("X-XSS-Protection", "1; mode=block")
}

// setCacheHeaders sets Cache-Control headers based on file path.
// Caching strategy:
//   - /assets/* (Vite hashed files): immutable, 1 year
//   - workbox-*.js (hashed): immutable, 1 year
//   - sw.js, registerSW.js: no-cache (service workers must be fresh)
//   - manifest.webmanifest, manifest.json: 1 hour
//   - icons (png, svg, ico): 1 hour
//   - other files with extensions: 1 hour
func setCacheHeaders(w http.ResponseWriter, path string) {
	// Vite-hashed assets under /assets/ - immutable, cache 1 year
	if strings.HasPrefix(path, "/assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}

	// Get the filename from the path
	filename := path
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		filename = path[idx+1:]
	}

	// Service worker files must always be revalidated
	if filename == "sw.js" || filename == "registerSW.js" {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		return
	}

	// Workbox runtime is hashed, can be cached long-term
	if strings.HasPrefix(filename, "workbox-") && strings.HasSuffix(filename, ".js") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}

	// PWA manifests - cache 1 hour (increase later)
	if filename == "manifest.webmanifest" || filename == "manifest.json" {
		w.Header().Set("Cache-Control", "public, max-age=3600")
		return
	}

	// Icons and static images - cache 1 hour (increase later)
	if strings.HasSuffix(filename, ".png") || strings.HasSuffix(filename, ".svg") || strings.HasSuffix(filename, ".ico") {
		w.Header().Set("Cache-Control", "public, max-age=3600")
		return
	}

	// Other files with extensions - default 1 hour cache
	if containsDot(path) {
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}
}

// roundDuration rounds d to 3 significant digits with minimum 1µs precision.
func roundDuration(d time.Duration) time.Duration {
	for t := 100 * time.Second; t >= 100*time.Microsecond; t /= 10 {
		if d >= t {
			return d.Round(t / 100)
		}
	}
	return d.Round(time.Microsecond)
}
