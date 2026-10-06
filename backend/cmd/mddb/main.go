// Package main is the entry point for the mddb server.
//
// mddb is a local-first markdown database that stores content as files,
// provides OAuth authentication (Google/Microsoft), and exposes a RESTful
// HTTP API. Startup configuration is read from config.toml and mutable server
// settings from settings.json, both in the config directory. A configured
// voice API key enables the embedded voice gateway.
package main

import (
	"bufio"
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/lmittmann/tint"
	"github.com/maruel/gomode/voicegateway"
	"github.com/maruel/gomode/voicegateway/voicertc"
	"github.com/maruel/mddb/backend/internal/email"
	"github.com/maruel/mddb/backend/internal/githubapp"
	"github.com/maruel/mddb/backend/internal/server"
	"github.com/maruel/mddb/backend/internal/server/handlers"
	"github.com/maruel/mddb/backend/internal/server/ipgeo"
	"github.com/maruel/mddb/backend/internal/server/sse"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
	"github.com/maruel/mddb/backend/internal/syncsvc"
	"github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
)

func main() {
	if err := mainImpl(); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(os.Stderr, "mddb: %v\n", err)
		os.Exit(1)
	}
}

func mainImpl() error {
	flag.Usage = func() {
		w := flag.CommandLine.Output()
		_, _ = fmt.Fprintf(w, `Usage: mddb [flags]

mddb serves the web UI, the HTTP API, and the workspace MCP endpoint.

Configuration is read from config.toml in the config directory
(default: ~/.config/mddb/). See contrib/config.toml for a documented example.

Flags:
`)
		flag.PrintDefaults()
	}
	configDirFlag := flag.String("config-dir", "", "Config directory (default: ~/.config/mddb)")
	version := flag.Bool("version", false, "Print version and exit")
	flag.Parse()
	if len(flag.Args()) > 0 {
		return fmt.Errorf("unknown arguments: %v", flag.Args())
	}

	if *version {
		printVersion()
		return nil
	}

	// config.toml owns every startup setting; -config-dir only selects which
	// directory holds it.
	cfgDir := defaultConfigDir()
	if *configDirFlag != "" {
		cfgDir = *configDirFlag
	}
	tc, err := loadTOMLConfig(cfgDir)
	if err != nil {
		return err
	}
	// Run onboarding when no config file exists yet and stdin is a TTY.
	if _, err := os.Stat(configPath(cfgDir)); os.IsNotExist(err) && isatty.IsTerminal(os.Stdin.Fd()) {
		if err := runOnboarding(cfgDir, &tc.Server); err != nil {
			return fmt.Errorf("onboarding failed: %w", err)
		}
		if tc, err = loadTOMLConfig(cfgDir); err != nil {
			return err
		}
	}
	cfg, err := resolveConfig(&tc, cfgDir)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	defer stop()
	ll := &slog.LevelVar{}
	ll.Set(cfg.LogLevel)
	// Skip timestamps when running under systemd (it adds its own).
	underSystemd := os.Getenv("JOURNAL_STREAM") != ""
	logger := slog.New(tint.NewTextHandler(colorable.NewColorable(os.Stderr), &tint.Options{
		Level:      ll,
		TimeFormat: "15:04:05.000", // Like time.TimeOnly plus milliseconds.
		NoColor:    !isatty.IsTerminal(os.Stderr.Fd()),
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Drop time when running under systemd.
			if underSystemd && a.Key == slog.TimeKey && len(groups) == 0 {
				return slog.Attr{}
			}
			// Drop localhost IPs (not useful in logs).
			if a.Key == "ip" {
				if v := a.Value.String(); v == "127.0.0.1" || v == "::1" {
					return slog.Attr{}
				}
			}
			val := a.Value.Any()
			skip := false
			switch t := val.(type) {
			case string:
				skip = t == ""
			case bool:
				skip = !t
			case uint64:
				skip = t == 0
			case int64:
				skip = t == 0
			case float64:
				skip = t == 0
			case time.Time:
				skip = t.IsZero()
			case time.Duration:
				skip = t == 0
			case nil:
				skip = true
			}
			if skip {
				return slog.Attr{}
			}
			return a
		},
	}))
	slog.SetDefault(logger)

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for data directories
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Load settings.json for JWT secret, SMTP, and quotas (creates with defaults if missing)
	serverCfg, err := storage.LoadServerConfig(cfg.ConfigDir, e2eBuild)
	if err != nil {
		return fmt.Errorf("failed to load settings.json: %w", err)
	}

	// Normalize addr: ":8080" becomes "localhost:8080"
	addr := cfg.HTTP
	if strings.HasPrefix(addr, ":") {
		addr = "localhost" + addr
	}

	// Append port to base URL if localhost and no port specified
	if u, err := url.Parse(cfg.BaseURL); err == nil && u.Port() == "" && u.Hostname() == "localhost" {
		if _, p, err := net.SplitHostPort(addr); err == nil {
			u.Host = net.JoinHostPort(u.Hostname(), p)
			cfg.BaseURL = u.String()
		}
	}

	// Create db directory for identity tables
	dbDir := filepath.Join(cfg.DataDir, "db")
	if err := os.MkdirAll(dbDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for data directories
		return fmt.Errorf("failed to create db directory: %w", err)
	}

	userService, err := identity.NewUserService(filepath.Join(dbDir, "users.jsonl"))
	if err != nil {
		return fmt.Errorf("failed to initialize user service: %w", err)
	}

	orgService, err := identity.NewOrganizationService(filepath.Join(dbDir, "organizations.jsonl"))
	if err != nil {
		return fmt.Errorf("failed to initialize organization service: %w", err)
	}

	wsService, err := identity.NewWorkspaceService(filepath.Join(dbDir, "workspaces.jsonl"))
	if err != nil {
		return fmt.Errorf("failed to initialize workspace service: %w", err)
	}

	orgMemService, err := identity.NewOrganizationMembershipService(filepath.Join(dbDir, "org_memberships.jsonl"), userService, orgService)
	if err != nil {
		return fmt.Errorf("failed to initialize organization membership service: %w", err)
	}

	wsMemService, err := identity.NewWorkspaceMembershipService(filepath.Join(dbDir, "ws_memberships.jsonl"), wsService, orgService)
	if err != nil {
		return fmt.Errorf("failed to initialize workspace membership service: %w", err)
	}

	orgInvService, err := identity.NewOrganizationInvitationService(filepath.Join(dbDir, "org_invitations.jsonl"))
	if err != nil {
		return fmt.Errorf("failed to initialize organization invitation service: %w", err)
	}

	wsInvService, err := identity.NewWorkspaceInvitationService(filepath.Join(dbDir, "ws_invitations.jsonl"))
	if err != nil {
		return fmt.Errorf("failed to initialize workspace invitation service: %w", err)
	}

	gitMgr := git.NewManager(cfg.DataDir, "", "")

	rootRepo, err := git.NewRootRepo(ctx, cfg.DataDir, "", "")
	if err != nil {
		return fmt.Errorf("failed to initialize root repo: %w", err)
	}

	fileStore, err := content.NewFileStoreService(cfg.DataDir, gitMgr, wsService, orgService, &serverCfg.Quotas.ResourceQuotas)
	if err != nil {
		return fmt.Errorf("failed to initialize file store: %w", err)
	}

	sessionService, err := identity.NewSessionService(filepath.Join(dbDir, "sessions.jsonl"))
	if err != nil {
		return fmt.Errorf("failed to initialize session service: %w", err)
	}

	// Cleanup old expired sessions (older than 7 days past expiration)
	if count, err := sessionService.CleanupExpired(7 * 24 * time.Hour); err != nil {
		slog.WarnContext(ctx, "Failed to cleanup expired sessions", "error", err)
	} else if count > 0 {
		slog.InfoContext(ctx, "Cleaned up expired sessions", "count", count)
		if err := rootRepo.CommitDBChanges(ctx, git.Author{}, fmt.Sprintf("cleanup %d expired sessions", count)); err != nil {
			slog.WarnContext(ctx, "Failed to commit session cleanup", "error", err)
		}
	}

	notificationService, err := identity.NewNotificationService(filepath.Join(dbDir, "notifications.jsonl"))
	if err != nil {
		return fmt.Errorf("failed to initialize notification service: %w", err)
	}

	pushSubscriptionService, err := identity.NewPushSubscriptionService(filepath.Join(dbDir, "push_subscriptions.jsonl"))
	if err != nil {
		return fmt.Errorf("failed to initialize push subscription service: %w", err)
	}

	// Initialize email verification service and email service (nil if SMTP not configured)
	var emailVerificationService *identity.EmailVerificationService
	var emailService *email.Service
	if !serverCfg.SMTP.IsZero() {
		emailService = &email.Service{Config: serverCfg.SMTP}
		slog.InfoContext(ctx, "SMTP configured", "host", serverCfg.SMTP.Host, "port", serverCfg.SMTP.Port)

		emailVerificationService, err = identity.NewEmailVerificationService(filepath.Join(dbDir, "email_verifications.jsonl"))
		if err != nil {
			return fmt.Errorf("failed to initialize email verification service: %w", err)
		}
	}

	// Watch own executable for modifications (for development restarts)
	if err := watchExecutable(ctx, stop); err != nil {
		return fmt.Errorf("failed to watch executable: %w", err)
	}

	// Open IP geolocation database if configured
	var geoChecker *ipgeo.Checker
	if cfg.GeoDB != "" {
		var err error
		geoChecker, err = ipgeo.Open(cfg.GeoDB)
		if err != nil {
			return fmt.Errorf("failed to open geo database: %w", err)
		}
		defer func() { _ = geoChecker.Close() }()
		slog.InfoContext(ctx, "IP geolocation enabled", "db", cfg.GeoDB)
	}

	// Parse GitHub App config if provided
	var ghAppConfig server.GitHubAppConfig
	var ghAppClient *githubapp.Client
	if cfg.GitHubApp.ID != 0 {
		pemData, err := os.ReadFile(cfg.GitHubApp.PrivateKeyFile)
		if err != nil {
			return fmt.Errorf("failed to read GitHub App private key file: %w", err)
		}
		block, _ := pem.Decode(pemData)
		if block == nil {
			return errors.New("failed to parse GitHub App private key PEM")
		}
		privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return fmt.Errorf("failed to parse GitHub App private key: %w", err)
		}
		ghAppConfig = server.GitHubAppConfig{
			AppID:         cfg.GitHubApp.ID,
			PrivateKey:    privateKey,
			WebhookSecret: cfg.GitHubApp.WebhookSecret,
		}
		ghAppClient = githubapp.NewClient(cfg.GitHubApp.ID, privateKey)
		slog.InfoContext(ctx, "GitHub App configured", "appID", cfg.GitHubApp.ID)
	}

	// Initialize sync service
	syncService := syncsvc.New(wsService, fileStore, ghAppClient, rootRepo)
	geminiAPIKey := cfg.VoiceAPIKey
	var voiceBridge voicegateway.MediaBridge
	if geminiAPIKey != "" {
		voiceCfg := voicegateway.DefaultConfig()
		if err := voiceCfg.ValidateEmbedded(); err != nil {
			slog.WarnContext(ctx, "embedded voice gateway disabled: invalid configuration", "err", err)
		} else {
			// Activity logs contain transcripts. Keep them outside the git-backed
			// data directory and remove them after the bridge has closed.
			activityDir, dirErr := os.MkdirTemp("", "mddb-voice-")
			if dirErr != nil {
				slog.WarnContext(ctx, "embedded voice gateway disabled: create activity directory", "err", dirErr)
			} else {
				bridge, err := voicertc.NewBridge(ctx, &voiceCfg, geminiAPIKey, voiceCfg.Server.WebRTCUDPPort, activityDir)
				if err != nil {
					slog.WarnContext(ctx, "embedded voice gateway disabled: startup failed", "err", err)
					if removeErr := os.RemoveAll(activityDir); removeErr != nil {
						slog.WarnContext(ctx, "remove voice activity directory", "err", removeErr)
					}
				} else {
					voiceBridge = bridge
					defer func() {
						bridge.CloseAll(context.WithoutCancel(ctx))
						if removeErr := os.RemoveAll(activityDir); removeErr != nil {
							slog.Warn("remove voice activity directory", "err", removeErr)
						}
					}()
				}
			}
		}
	}

	svc := &handlers.Services{
		FileStore:        fileStore,
		Search:           content.NewSearchService(fileStore),
		User:             userService,
		Organization:     orgService,
		Workspace:        wsService,
		OrgInvitation:    orgInvService,
		WSInvitation:     wsInvService,
		OrgMembership:    orgMemService,
		WSMembership:     wsMemService,
		Session:          sessionService,
		EmailVerif:       emailVerificationService,
		Email:            emailService,
		RootRepo:         rootRepo,
		SyncService:      syncService,
		Notification:     notificationService,
		PushSubscription: pushSubscriptionService,
		Broker:           sse.NewBroker(),
	}

	buildVersion, buildGoVersion, buildRevision, buildDirty := getBuildInfo()
	routerCfg := &server.Config{
		ServerConfig:   serverCfg,
		ConfigDir:      cfg.ConfigDir,
		BaseURL:        cfg.BaseURL,
		Version:        buildVersion,
		GoVersion:      buildGoVersion,
		Revision:       buildRevision,
		Dirty:          buildDirty,
		FastRateLimits: e2eBuild,
		IPGeo:          geoChecker,
		TrustedProxies: cfg.TrustedProxies,
		VoiceBridge:    voiceBridge,
		OAuth: server.OAuthConfig{
			GoogleClientID:     cfg.OAuth.Google.ClientID,
			GoogleClientSecret: cfg.OAuth.Google.ClientSecret,
			MSClientID:         cfg.OAuth.Microsoft.ClientID,
			MSClientSecret:     cfg.OAuth.Microsoft.ClientSecret,
			GitHubClientID:     cfg.OAuth.GitHub.ClientID,
			GitHubClientSecret: cfg.OAuth.GitHub.ClientSecret,
			TestOAuth:          cfg.TestOAuth,
		},
		GitHubApp: ghAppConfig,
	}

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           server.NewRouter(svc, routerCfg),
		BaseContext:       func(_ net.Listener) context.Context { return ctx },
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Start notification cleanup goroutine (runs once on startup, then daily).
	go runNotificationCleanup(ctx, notificationService, rootRepo, &serverCfg.Quotas)

	// Run server in goroutine
	serverErr := make(chan error, 1)
	go func() {
		slog.InfoContext(ctx, "Starting server", "addr", addr, "baseURL", cfg.BaseURL, "version", buildVersion)
		serverErr <- httpServer.ListenAndServe()
	}()

	// Wait for either context cancellation or server error
	select {
	case err := <-serverErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("server error: %w", err)
		}
	case <-ctx.Done():
		// Graceful shutdown
		slog.InfoContext(ctx, "Shutting down server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown error: %w", err)
		}
		slog.InfoContext(ctx, "Server stopped")
	}
	return nil
}

func printVersion() {
	version, goVersion, revision, dirty := getBuildInfo()
	fmt.Printf("mddb %s\n", version)
	fmt.Printf("  Go version: %s\n", goVersion)
	fmt.Printf("  Revision:   %s\n", revision)
	if dirty {
		fmt.Printf("  Modified:   true\n")
	}
}

func getBuildInfo() (version, goVersion, revision string, dirty bool) {
	version = "unknown"
	goVersion = "unknown"
	revision = "unknown"
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	version = info.Main.Version
	if version == "" || version == "(devel)" {
		version = "dev"
	}
	goVersion = info.GoVersion
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			dirty = setting.Value == "true"
		}
	}
	return
}

// prompt asks one onboarding question and returns the trimmed answer.
func prompt(reader *bufio.Reader, question string) (string, error) {
	fmt.Print(question)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// promptCredentials asks for one provider's client ID and, when it is given,
// its client secret.
func promptCredentials(reader *bufio.Reader, provider string) (oauthCredentials, error) {
	clientID, err := prompt(reader, provider+" Client ID (optional): ")
	if err != nil {
		return oauthCredentials{}, fmt.Errorf("failed to read %s Client ID: %w", provider, err)
	}
	if clientID == "" {
		return oauthCredentials{}, nil
	}
	secret, err := prompt(reader, provider+" Client Secret: ")
	if err != nil {
		return oauthCredentials{}, fmt.Errorf("failed to read %s Client Secret: %w", provider, err)
	}
	if secret == "" {
		return oauthCredentials{}, fmt.Errorf("%s Client ID requires its client secret", provider)
	}
	return oauthCredentials{ClientID: clientID, ClientSecret: secret}, nil
}

// runOnboarding asks for the OAuth settings and writes them to
// cfgDir/config.toml. The file holds client secrets, so it is created with mode
// 0600. defaults supplies the base URL default and the port shown in the
// callback URLs.
func runOnboarding(cfgDir string, defaults *tomlServer) error {
	fmt.Println("Welcome to mddb! Let's set up your configuration.")
	fmt.Println("This wizard writes " + configPath(cfgDir) + ", which holds client secrets.")
	fmt.Println("")

	reader := bufio.NewReader(os.Stdin)

	// Base URL
	fmt.Println("\n--- Base URL Setup ---")
	fmt.Println("The base URL is used for OAuth callback URLs.")
	fmt.Println("If no port is specified, it will use the server's port automatically.")
	baseURL, err := prompt(reader, "Base URL (default: "+defaults.BaseURL+"): ")
	if err != nil {
		return fmt.Errorf("failed to read base URL: %w", err)
	}
	if baseURL == "" {
		baseURL = defaults.BaseURL
	}
	// For display purposes in onboarding, show with the server's port if localhost.
	displayBaseURL := baseURL
	if u, err := url.Parse(baseURL); err == nil && u.Port() == "" && u.Hostname() == "localhost" {
		if _, p, err := net.SplitHostPort(defaults.HTTP); err == nil {
			u.Host = net.JoinHostPort(u.Hostname(), p)
			displayBaseURL = u.String()
		}
	}

	// Google OAuth
	fmt.Println("\n--- Google OAuth Setup ---")
	fmt.Println("To use Google login, create a project at https://console.cloud.google.com/apis/credentials")
	fmt.Printf("Configure an OAuth 2.0 Client ID with redirect URI: %s/api/v1/auth/oauth/google/callback\n", displayBaseURL)
	google, err := promptCredentials(reader, "Google")
	if err != nil {
		return err
	}

	// Microsoft OAuth
	fmt.Println("\n--- Microsoft OAuth Setup ---")
	fmt.Println("To use Microsoft login, register an app at https://portal.azure.com/")
	fmt.Printf("Configure a redirect URI: %s/api/v1/auth/oauth/microsoft/callback\n", displayBaseURL)
	microsoft, err := promptCredentials(reader, "Microsoft")
	if err != nil {
		return err
	}

	// GitHub OAuth
	fmt.Println("\n--- GitHub OAuth Setup ---")
	fmt.Println("To use GitHub login, create an OAuth App at https://github.com/settings/developers")
	fmt.Printf("Configure a redirect URI: %s/api/v1/auth/oauth/github/callback\n", displayBaseURL)
	github, err := promptCredentials(reader, "GitHub")
	if err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString("# mddb instance configuration written by the onboarding wizard.\n")
	b.WriteString("# This file holds client secrets; keep it readable by the server account only (mode 0600).\n")
	b.WriteString("# See contrib/config.toml for every option and its default.\n\n")
	b.WriteString("[server]\n")
	fmt.Fprintf(&b, "base_url = %q\n", baseURL)
	for _, provider := range []struct {
		section string
		creds   oauthCredentials
	}{
		{"google", google},
		{"microsoft", microsoft},
		{"github", github},
	} {
		if provider.creds.ClientID == "" {
			continue
		}
		fmt.Fprintf(&b, "\n[oauth.%s]\nclient_id = %q\nclient_secret = %q\n", provider.section, provider.creds.ClientID, provider.creds.ClientSecret)
	}

	if err := os.MkdirAll(cfgDir, 0o750); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	path := configPath(cfgDir)
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}

	fmt.Printf("\nConfiguration saved to %s\n", path)
	fmt.Println("You can edit this file later to change your settings; see contrib/config.toml.")
	fmt.Println("")

	return nil
}

// runNotificationCleanup periodically deletes old notifications and caps per-user counts.
func runNotificationCleanup(ctx context.Context, notifSvc *identity.NotificationService, rootRepo *git.RootRepo, quotas *storage.ServerQuotas) {
	cleanup := func() {
		retentionDays := quotas.NotificationRetentionDays
		if retentionDays <= 0 {
			retentionDays = 90
		}
		maxPerUser := quotas.MaxNotificationsPerUser
		if maxPerUser <= 0 {
			maxPerUser = 500
		}

		cutoff := storage.ToTime(time.Now().AddDate(0, 0, -retentionDays))
		var totalDeleted int

		if count, err := notifSvc.DeleteOlderThan(cutoff); err != nil {
			slog.WarnContext(ctx, "Failed to delete old notifications", "error", err)
		} else {
			totalDeleted += count
		}

		if count, err := notifSvc.DeleteExcessPerUser(maxPerUser); err != nil {
			slog.WarnContext(ctx, "Failed to cap notifications per user", "error", err)
		} else {
			totalDeleted += count
		}

		if totalDeleted > 0 {
			slog.InfoContext(ctx, "Notification cleanup", "deleted", totalDeleted)
			if err := rootRepo.CommitDBChanges(ctx, git.Author{}, fmt.Sprintf("cleanup %d notifications", totalDeleted)); err != nil {
				slog.WarnContext(ctx, "Failed to commit notification cleanup", "error", err)
			}
		}
	}

	// Run once at startup.
	cleanup()

	// Then run daily.
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cleanup()
		}
	}
}

// watchExecutable watches the current executable for modifications and calls
// stop to trigger graceful shutdown when detected. This enables seamless
// restarts during development.
func watchExecutable(ctx context.Context, stop context.CancelFunc) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return err
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	if err := w.Add(exe); err != nil {
		_ = w.Close()
		return err
	}
	go func() {
		defer func() { _ = w.Close() }()
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-w.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Chmod) {
					slog.InfoContext(ctx, "Executable modified, initiating shutdown")
					stop()
					return
				}
			case err, ok := <-w.Errors:
				if !ok {
					return
				}
				slog.WarnContext(ctx, "Error watching executable", "err", err)
			}
		}
	}()
	return nil
}
