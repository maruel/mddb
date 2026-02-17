// Package server implements HTTP routing, middleware, and request handling.
//
// It provides handler composition utilities (Wrap, WrapAuth) for type-safe routes
// with JWT authentication, role-based access control, and automatic JSON marshaling.
// It also serves the embedded SolidJS frontend.
package server

//go:generate go run ../apiroutes -q
//go:generate go run ../apiclient -q

import (
	"crypto/rsa"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/maruel/mddb/backend/frontend"
	"github.com/maruel/mddb/backend/internal/githubapp"
	"github.com/maruel/mddb/backend/internal/server/bandwidth"
	"github.com/maruel/mddb/backend/internal/server/handlers"
	"github.com/maruel/mddb/backend/internal/server/ipgeo"
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
	GitHubApp GitHubAppConfig
	IPGeo     *ipgeo.Checker
}

// GitHubAppConfig holds GitHub App credentials for installation-based auth.
type GitHubAppConfig struct {
	AppID         int64
	PrivateKey    *rsa.PrivateKey // parsed PEM at startup
	WebhookSecret string
}

// OAuthConfig holds OAuth provider credentials.
type OAuthConfig struct {
	GoogleClientID     string
	GoogleClientSecret string
	MSClientID         string
	MSClientSecret     string
	GitHubClientID     string
	GitHubClientSecret string
	TestOAuth          bool // When true, OAuth login bypasses real providers and uses fake accounts.
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
	// GitHub App client for git remote handler.
	var ghAppClient *githubapp.Client
	if cfg.GitHubApp.PrivateKey != nil {
		ghAppClient = githubapp.NewClient(cfg.GitHubApp.AppID, cfg.GitHubApp.PrivateKey)
	}
	grh := &handlers.GitRemoteHandler{Svc: svc, GitHubApp: ghAppClient}

	// Health check (public)
	hh := &handlers.HealthHandler{Cfg: hcfg}
	mux.Handle("/api/v1/health", Wrap(hh.GetHealth, hcfg, limiters))

	// Admin endpoints (requires IsGlobalAdmin)
	adminh := &handlers.AdminHandler{Svc: svc, RateLimitCounts: limiters.Counts, ServerStartTime: limiters.StartTime}
	mux.Handle("GET /api/v1/admin/server", WrapGlobalAdmin(adminh.GetServerDetail, svc, hcfg, limiters))

	// Server config endpoints (requires IsGlobalAdmin)
	serverh := &handlers.ServerHandler{Cfg: cfg.ServerConfig, DataDir: cfg.DataDir, FileStore: svc.FileStore, BandwidthLimiter: bandwidthLim, RateLimiters: limiters}
	mux.Handle("GET /api/v1/server/config", WrapGlobalAdmin(serverh.GetConfig, svc, hcfg, limiters))
	mux.Handle("POST /api/v1/server/config", WrapGlobalAdmin(serverh.UpdateConfig, svc, hcfg, limiters))

	// OAuth handler setup (needed before auth routes)
	oh := handlers.NewOAuthHandler(svc, hcfg)
	base := strings.TrimRight(cfg.BaseURL, "/")
	var providers []identity.OAuthProvider
	if cfg.OAuth.GoogleClientID != "" && cfg.OAuth.GoogleClientSecret != "" {
		oh.AddProvider(identity.OAuthProviderGoogle, cfg.OAuth.GoogleClientID, cfg.OAuth.GoogleClientSecret, base+"/api/v1/auth/oauth/google/callback")
		providers = append(providers, identity.OAuthProviderGoogle)
	}
	if cfg.OAuth.MSClientID != "" && cfg.OAuth.MSClientSecret != "" {
		oh.AddProvider(identity.OAuthProviderMicrosoft, cfg.OAuth.MSClientID, cfg.OAuth.MSClientSecret, base+"/api/v1/auth/oauth/microsoft/callback")
		providers = append(providers, identity.OAuthProviderMicrosoft)
	}
	if cfg.OAuth.GitHubClientID != "" && cfg.OAuth.GitHubClientSecret != "" {
		oh.AddProvider(identity.OAuthProviderGitHub, cfg.OAuth.GitHubClientID, cfg.OAuth.GitHubClientSecret, base+"/api/v1/auth/oauth/github/callback")
		providers = append(providers, identity.OAuthProviderGitHub)
	}
	if len(providers) > 0 {
		slog.Info("OAuth providers initialized", "providers", providers)
	} else {
		slog.Info("No OAuth providers configured")
	}

	// In test mode, use a fake handler that bypasses real OAuth providers.
	loginRedirect := oh.LoginRedirect
	if cfg.OAuth.TestOAuth {
		toh := handlers.NewTestOAuthHandler(svc, hcfg, providers)
		loginRedirect = toh.LoginRedirect
	}

	// Auth endpoints - /api/v1/auth/*
	// Public
	mux.Handle("POST /api/v1/auth/login", WrapWithSvc(authh.Login, svc, hcfg, limiters))
	mux.Handle("POST /api/v1/auth/register", WrapWithSvc(authh.Register, svc, hcfg, limiters))
	mux.Handle("POST /api/v1/auth/invitations/org/accept", WrapWithSvc(ih.AcceptOrgInvitation, svc, hcfg, limiters))
	mux.Handle("POST /api/v1/auth/invitations/ws/accept", WrapWithSvc(ih.AcceptWSInvitation, svc, hcfg, limiters))
	mux.HandleFunc("GET /api/v1/auth/email/verify", authh.VerifyEmailRedirect)
	mux.Handle("GET /api/v1/auth/providers", Wrap(oh.ListProviders, hcfg, limiters))
	mux.HandleFunc("GET /api/v1/auth/oauth/{provider}", loginRedirect)
	mux.HandleFunc("GET /api/v1/auth/oauth/{provider}/callback", oh.Callback)
	// Authenticated
	mux.Handle("GET /api/v1/auth/me", WrapAuth(authh.GetMe, svc, hcfg, limiters))
	mux.Handle("POST /api/v1/auth/logout", WrapAuth(authh.Logout, svc, hcfg, limiters))
	mux.Handle("GET /api/v1/auth/sessions", WrapAuth(authh.ListSessions, svc, hcfg, limiters))
	mux.Handle("POST /api/v1/auth/sessions/revoke", WrapAuth(authh.RevokeSession, svc, hcfg, limiters))
	mux.Handle("POST /api/v1/auth/sessions/revoke-all", WrapAuth(authh.RevokeAllSessions, svc, hcfg, limiters))
	mux.Handle("POST /api/v1/auth/email", WrapAuth(authh.ChangeEmail, svc, hcfg, limiters))
	mux.Handle("POST /api/v1/auth/email/send-verification", WrapAuth(authh.SendVerificationEmail, svc, hcfg, limiters))
	mux.Handle("POST /api/v1/auth/switch-workspace", WrapAuth(mh.SwitchWorkspace, svc, hcfg, limiters))
	mux.Handle("POST /api/v1/auth/settings", WrapAuth(uh.UpdateUserSettings, svc, hcfg, limiters))
	mux.Handle("POST /api/v1/auth/oauth/link", WrapAuth(oh.LinkOAuth, svc, hcfg, limiters))
	mux.Handle("POST /api/v1/auth/oauth/unlink", WrapAuth(oh.UnlinkOAuth, svc, hcfg, limiters))
	mux.Handle("POST /api/v1/auth/password", WrapAuth(authh.SetPassword, svc, hcfg, limiters))

	// Organization endpoints - /api/v1/organizations/*
	mux.Handle("POST /api/v1/organizations", WrapAuth(authh.CreateOrganization, svc, hcfg, limiters))
	mux.Handle("GET /api/v1/organizations/{orgID}", WrapOrgAuth(orgh.GetOrganization, svc, hcfg, identity.OrgRoleMember, limiters))
	mux.Handle("POST /api/v1/organizations/{orgID}", WrapOrgAuth(orgh.UpdateOrganization, svc, hcfg, identity.OrgRoleAdmin, limiters))
	mux.Handle("POST /api/v1/organizations/{orgID}/settings", WrapOrgAuth(orgh.UpdateOrgPreferences, svc, hcfg, identity.OrgRoleAdmin, limiters))
	mux.Handle("GET /api/v1/organizations/{orgID}/users", WrapOrgAuth(uh.ListUsers, svc, hcfg, identity.OrgRoleAdmin, limiters))
	mux.Handle("POST /api/v1/organizations/{orgID}/users/role", WrapOrgAuth(uh.UpdateOrgMemberRole, svc, hcfg, identity.OrgRoleAdmin, limiters))
	mux.Handle("POST /api/v1/organizations/{orgID}/users/remove", WrapOrgAuth(uh.RemoveOrgMember, svc, hcfg, identity.OrgRoleAdmin, limiters))
	mux.Handle("GET /api/v1/organizations/{orgID}/invitations", WrapOrgAuth(ih.ListOrgInvitations, svc, hcfg, identity.OrgRoleAdmin, limiters))
	mux.Handle("POST /api/v1/organizations/{orgID}/invitations", WrapOrgAuth(ih.CreateOrgInvitation, svc, hcfg, identity.OrgRoleAdmin, limiters))
	mux.Handle("POST /api/v1/organizations/{orgID}/workspaces", WrapOrgAuth(orgh.CreateWorkspace, svc, hcfg, identity.OrgRoleAdmin, limiters))

	// Notion import endpoints
	nih := handlers.NewNotionImportHandler(svc, hcfg)
	mux.Handle("POST /api/v1/organizations/{orgID}/notion/import", WrapOrgAuth(nih.StartImport, svc, hcfg, identity.OrgRoleAdmin, limiters))
	mux.Handle("GET /api/v1/organizations/{orgID}/notion/import/{importWsID}/status", WrapOrgAuth(nih.GetStatus, svc, hcfg, identity.OrgRoleMember, limiters))

	// Workspace endpoints - /api/v1/workspaces/*
	// Details and settings
	mux.Handle("GET /api/v1/workspaces/{wsID}", WrapWSAuth(orgh.GetWorkspace, svc, hcfg, identity.WSRoleViewer, limiters))
	mux.Handle("POST /api/v1/workspaces/{wsID}", WrapWSAuth(orgh.UpdateWorkspace, svc, hcfg, identity.WSRoleAdmin, limiters))
	mux.Handle("POST /api/v1/workspaces/{wsID}/settings/membership", WrapWSAuth(mh.UpdateWSMembershipSettings, svc, hcfg, identity.WSRoleViewer, limiters))
	mux.Handle("GET /api/v1/workspaces/{wsID}/settings/git", WrapWSAuth(grh.GetGitRemote, svc, hcfg, identity.WSRoleAdmin, limiters))
	mux.Handle("POST /api/v1/workspaces/{wsID}/settings/git", WrapWSAuth(grh.UpdateGitRemote, svc, hcfg, identity.WSRoleAdmin, limiters))
	mux.Handle("POST /api/v1/workspaces/{wsID}/settings/git/push", WrapWSAuth(grh.PushGit, svc, hcfg, identity.WSRoleAdmin, limiters))
	mux.Handle("POST /api/v1/workspaces/{wsID}/settings/git/pull", WrapWSAuth(grh.PullGit, svc, hcfg, identity.WSRoleAdmin, limiters))
	mux.Handle("GET /api/v1/workspaces/{wsID}/settings/git/status", WrapWSAuth(grh.GetSyncStatus, svc, hcfg, identity.WSRoleViewer, limiters))
	mux.Handle("POST /api/v1/workspaces/{wsID}/settings/git/github-app", WrapWSAuth(grh.SetupGitHubAppRemote, svc, hcfg, identity.WSRoleAdmin, limiters))
	mux.Handle("POST /api/v1/workspaces/{wsID}/settings/git/delete", WrapWSAuth(grh.DeleteGitRemote, svc, hcfg, identity.WSRoleAdmin, limiters))

	// GitHub App routes
	mux.Handle("POST /api/v1/github-app/repos", WrapAuth(grh.ListGitHubAppRepos, svc, hcfg, limiters))
	mux.Handle("GET /api/v1/github-app/installations", WrapAuth(grh.ListGitHubAppInstallations, svc, hcfg, limiters))
	mux.Handle("GET /api/v1/github-app/available", Wrap(grh.IsGitHubAppAvailable, hcfg, limiters))
	// Notion import cancel
	mux.Handle("POST /api/v1/workspaces/{wsID}/notion/import/cancel", WrapWSAuth(nih.CancelImport, svc, hcfg, identity.WSRoleAdmin, limiters))
	// Users and invitations
	mux.Handle("POST /api/v1/workspaces/{wsID}/users/role", WrapWSAuth(uh.UpdateWSMemberRole, svc, hcfg, identity.WSRoleAdmin, limiters))
	mux.Handle("GET /api/v1/workspaces/{wsID}/invitations", WrapWSAuth(ih.ListWSInvitations, svc, hcfg, identity.WSRoleAdmin, limiters))
	mux.Handle("POST /api/v1/workspaces/{wsID}/invitations", WrapWSAuth(ih.CreateWSInvitation, svc, hcfg, identity.WSRoleAdmin, limiters))
	// Nodes (id=0 is valid for root node)
	mux.Handle("GET /api/v1/workspaces/{wsID}/nodes/titles", WrapWSAuth(nh.GetNodeTitles, svc, hcfg, identity.WSRoleViewer, limiters))
	mux.Handle("GET /api/v1/workspaces/{wsID}/nodes/{id}", WrapWSAuth(nh.GetNode, svc, hcfg, identity.WSRoleViewer, limiters))
	mux.Handle("GET /api/v1/workspaces/{wsID}/nodes/{id}/children", WrapWSAuth(nh.ListNodeChildren, svc, hcfg, identity.WSRoleViewer, limiters))
	mux.Handle("POST /api/v1/workspaces/{wsID}/nodes/{id}/move", WrapWSAuth(nh.MoveNode, svc, hcfg, identity.WSRoleEditor, limiters))
	mux.Handle("POST /api/v1/workspaces/{wsID}/nodes/{id}/delete", WrapWSAuth(nh.DeleteNode, svc, hcfg, identity.WSRoleEditor, limiters))
	// Pages (under nodes)
	mux.Handle("POST /api/v1/workspaces/{wsID}/nodes/{id}/page/create", WrapWSAuth(nh.CreatePage, svc, hcfg, identity.WSRoleEditor, limiters))
	mux.Handle("GET /api/v1/workspaces/{wsID}/nodes/{id}/page", WrapWSAuth(nh.GetPage, svc, hcfg, identity.WSRoleViewer, limiters))
	mux.Handle("POST /api/v1/workspaces/{wsID}/nodes/{id}/page", WrapWSAuth(nh.UpdatePage, svc, hcfg, identity.WSRoleEditor, limiters))
	mux.Handle("POST /api/v1/workspaces/{wsID}/nodes/{id}/page/delete", WrapWSAuth(nh.DeletePage, svc, hcfg, identity.WSRoleEditor, limiters))
	// Tables (under nodes)
	mux.Handle("POST /api/v1/workspaces/{wsID}/nodes/{id}/table/create", WrapWSAuth(nh.CreateTable, svc, hcfg, identity.WSRoleEditor, limiters))
	mux.Handle("GET /api/v1/workspaces/{wsID}/nodes/{id}/table", WrapWSAuth(nh.GetTable, svc, hcfg, identity.WSRoleViewer, limiters))
	mux.Handle("POST /api/v1/workspaces/{wsID}/nodes/{id}/table", WrapWSAuth(nh.UpdateTable, svc, hcfg, identity.WSRoleEditor, limiters))
	mux.Handle("POST /api/v1/workspaces/{wsID}/nodes/{id}/table/delete", WrapWSAuth(nh.DeleteTable, svc, hcfg, identity.WSRoleEditor, limiters))

	// Views (under nodes/table)
	vh := &handlers.ViewHandler{Svc: svc, Cfg: hcfg}
	mux.Handle("POST /api/v1/workspaces/{wsID}/nodes/{id}/views/create", WrapWSAuth(vh.CreateView, svc, hcfg, identity.WSRoleEditor, limiters))
	mux.Handle("POST /api/v1/workspaces/{wsID}/nodes/{id}/views/{viewID}", WrapWSAuth(vh.UpdateView, svc, hcfg, identity.WSRoleEditor, limiters))
	mux.Handle("POST /api/v1/workspaces/{wsID}/nodes/{id}/views/{viewID}/delete", WrapWSAuth(vh.DeleteView, svc, hcfg, identity.WSRoleEditor, limiters))

	// Records (under nodes/table)
	mux.Handle("GET /api/v1/workspaces/{wsID}/nodes/{id}/table/records", WrapWSAuth(nh.ListRecords, svc, hcfg, identity.WSRoleViewer, limiters))
	mux.Handle("GET /api/v1/workspaces/{wsID}/nodes/{id}/table/records/{rid}", WrapWSAuth(nh.GetRecord, svc, hcfg, identity.WSRoleViewer, limiters))
	mux.Handle("POST /api/v1/workspaces/{wsID}/nodes/{id}/table/records/create", WrapWSAuth(nh.CreateRecord, svc, hcfg, identity.WSRoleEditor, limiters))
	mux.Handle("POST /api/v1/workspaces/{wsID}/nodes/{id}/table/records/{rid}", WrapWSAuth(nh.UpdateRecord, svc, hcfg, identity.WSRoleEditor, limiters))
	mux.Handle("POST /api/v1/workspaces/{wsID}/nodes/{id}/table/records/{rid}/delete", WrapWSAuth(nh.DeleteRecord, svc, hcfg, identity.WSRoleEditor, limiters))
	// History (under nodes)
	mux.Handle("GET /api/v1/workspaces/{wsID}/nodes/{id}/history", WrapWSAuth(nh.ListNodeVersions, svc, hcfg, identity.WSRoleViewer, limiters))
	mux.Handle("GET /api/v1/workspaces/{wsID}/nodes/{id}/history/{hash}", WrapWSAuth(nh.GetNodeVersion, svc, hcfg, identity.WSRoleViewer, limiters))
	// Assets (under nodes)
	mux.Handle("GET /api/v1/workspaces/{wsID}/nodes/{id}/assets", WrapWSAuth(nh.ListNodeAssets, svc, hcfg, identity.WSRoleViewer, limiters))
	mux.Handle("POST /api/v1/workspaces/{wsID}/nodes/{id}/assets", WrapAuthRaw(ah.UploadNodeAssetHandler, svc, hcfg, identity.WSRoleEditor, limiters))
	mux.Handle("POST /api/v1/workspaces/{wsID}/nodes/{id}/assets/{name}/delete", WrapWSAuth(nh.DeleteNodeAsset, svc, hcfg, identity.WSRoleEditor, limiters))
	// Search
	mux.Handle("POST /api/v1/workspaces/{wsID}/search", WrapWSAuth(sh.Search, svc, hcfg, identity.WSRoleViewer, limiters))

	// SSE events (EventSource can't set headers; token accepted as query param)
	sseH := &handlers.SSEHandler{Svc: svc, Cfg: hcfg}
	sseAuth := WrapAuthRaw(sseH.ServeHTTP, svc, hcfg, identity.WSRoleViewer, limiters)
	mux.HandleFunc("GET /api/v1/workspaces/{wsID}/events", handlers.InjectTokenFromQuery(sseAuth).ServeHTTP)

	// Notification endpoints - /api/v1/notifications/*
	notifh := &handlers.NotificationHandler{Svc: svc, Cfg: hcfg}
	mux.Handle("GET /api/v1/notifications", WrapAuth(notifh.ListNotifications, svc, hcfg, limiters))
	mux.Handle("GET /api/v1/notifications/unread-count", WrapAuth(notifh.GetUnreadCount, svc, hcfg, limiters))
	mux.Handle("POST /api/v1/notifications/{id}/read", WrapAuth(notifh.MarkNotificationRead, svc, hcfg, limiters))
	mux.Handle("POST /api/v1/notifications/read-all", WrapAuth(notifh.MarkAllNotificationsRead, svc, hcfg, limiters))
	mux.Handle("POST /api/v1/notifications/{id}/delete", WrapAuth(notifh.DeleteNotification, svc, hcfg, limiters))
	mux.Handle("GET /api/v1/notifications/preferences", WrapAuth(notifh.GetNotificationPrefs, svc, hcfg, limiters))
	mux.Handle("POST /api/v1/notifications/preferences", WrapAuth(notifh.UpdateNotificationPrefs, svc, hcfg, limiters))
	mux.Handle("GET /api/v1/notifications/vapid-key", WrapAuth(notifh.GetVAPIDPublicKey, svc, hcfg, limiters))
	mux.Handle("POST /api/v1/notifications/subscribe", WrapAuth(notifh.SubscribePush, svc, hcfg, limiters))
	mux.Handle("POST /api/v1/notifications/unsubscribe", WrapAuth(notifh.UnsubscribePush, svc, hcfg, limiters))

	// GitHub webhook (unauthenticated, signature-verified)
	if cfg.GitHubApp.WebhookSecret != "" || cfg.GitHubApp.PrivateKey != nil {
		wh := &handlers.GitHubWebhookHandler{
			WebhookSecret: cfg.GitHubApp.WebhookSecret,
			SyncService:   svc.SyncService,
			WsSvc:         svc.Workspace,
		}
		mux.HandleFunc("POST /api/v1/webhooks/github", wh.HandleWebhook)
	}

	// File serving (raw asset files) - requires signed URL (sig + exp query params)
	mux.HandleFunc("GET /assets/{wsID}/{id}/{name}", ah.ServeAssetFile)

	// API catch-all - return 404 for any unmatched /api/ routes (never fall through to SPA)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Not found", http.StatusNotFound)
	})

	// Serve embedded SolidJS frontend with SPA fallback
	dist, _ := fs.Sub(frontend.Files, "dist")
	mux.HandleFunc("/", newStaticHandler(dist))

	// Wrap mux with compression middleware chain.
	var inner http.Handler = mux
	inner = compressMiddleware(inner)
	inner = decompressMiddleware(inner)

	f := func(w http.ResponseWriter, r *http.Request) {
		clientIP := reqctx.GetClientIP(r)
		var cc string
		if cfg.IPGeo != nil {
			cc = cfg.IPGeo.CountryCode(clientIP)
			r = r.WithContext(reqctx.WithCountryCode(r.Context(), cc))
		}
		start := time.Now()
		rw := &responseWriter{
			ResponseWriter:   w,
			status:           http.StatusOK,
			bandwidthLimiter: bandwidthLim,
		}
		inner.ServeHTTP(rw, r)
		slog.InfoContext(r.Context(), "http",
			"m", r.Method,
			"p", r.URL.Path,
			"s", rw.status,
			"d", roundDuration(time.Since(start)),
			"s", rw.size,
			"ip", clientIP,
			"cc", cc,
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

// Flush implements http.Flusher for SSE support through the middleware chain.
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap returns the underlying ResponseWriter for http.ResponseController.
func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
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

// roundDuration rounds d to 3 significant digits with minimum 1µs precision.
func roundDuration(d time.Duration) time.Duration {
	for t := 100 * time.Second; t >= 100*time.Microsecond; t /= 10 {
		if d >= t {
			return d.Round(t / 100)
		}
	}
	return d.Round(time.Microsecond)
}
