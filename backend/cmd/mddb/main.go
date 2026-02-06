// Package main is the entry point for the mddb server.
//
// mddb is a local-first markdown database that stores content as files,
// provides OAuth authentication (Google/Microsoft), and exposes a RESTful
// HTTP API. Configuration is read from CLI flags, a .env file (for OAuth),
// and config.json (for JWT secret, SMTP, quotas).
package main

import (
	"bufio"
	"context"
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
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/lmittmann/tint"
	"github.com/maruel/mddb/backend/internal/email"
	"github.com/maruel/mddb/backend/internal/server"
	"github.com/maruel/mddb/backend/internal/server/handlers"
	"github.com/maruel/mddb/backend/internal/server/ipgeo"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
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
	version := flag.Bool("version", false, "Print version and exit")
	httpAddr := flag.String("http", "localhost:8080", "Address to listen on (e.g., localhost:8080, :8080, 0.0.0.0:8080). Use 0.0.0.0:port to listen on all interfaces.")
	dataDir := flag.String("data-dir", "./data", "Data directory")
	logLevel := flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	baseURL := flag.String("base-url", "http://localhost", "Base URL for OAuth callbacks (e.g., https://example.com)")
	googleClientID := flag.String("google-client-id", "", "Google OAuth client ID")
	googleClientSecret := flag.String("google-client-secret", "", "Google OAuth client secret")
	msClientID := flag.String("ms-client-id", "", "Microsoft OAuth client ID")
	msClientSecret := flag.String("ms-client-secret", "", "Microsoft OAuth client secret")
	githubClientID := flag.String("github-client-id", "", "GitHub OAuth client ID")
	githubClientSecret := flag.String("github-client-secret", "", "GitHub OAuth client secret")
	geoDB := flag.String("geo-db", "", "Path to MaxMind MMDB file for IP geolocation (optional)")
	flag.Parse()
	if len(flag.Args()) > 0 {
		return fmt.Errorf("unknown arguments: %v", flag.Args())
	}

	if *version {
		printVersion()
		return nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	defer stop()
	ll := &slog.LevelVar{}
	ll.Set(slog.LevelInfo)
	// Skip timestamps when running under systemd (it adds its own).
	underSystemd := os.Getenv("JOURNAL_STREAM") != ""
	logger := slog.New(tint.NewHandler(colorable.NewColorable(os.Stderr), &tint.Options{
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

	if err := os.MkdirAll(*dataDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for data directories
		return fmt.Errorf("failed to create data directory: %w", err)
	}
	// Run onboarding if no .env file exists and stdin is a TTY
	envPath := filepath.Join(*dataDir, ".env")
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		if isatty.IsTerminal(os.Stdin.Fd()) {
			if err := runOnboarding(*dataDir); err != nil {
				return fmt.Errorf("onboarding failed: %w", err)
			}
		}
	}

	// Load .env for OAuth credentials and bootstrap settings
	env, err := loadDotEnv(*dataDir)
	if err != nil {
		return err
	}

	// Load server_config.json for JWT secret, SMTP, and quotas (creates with defaults if missing)
	serverCfg, err := storage.LoadServerConfig(*dataDir)
	if err != nil {
		return fmt.Errorf("failed to load server_config.json: %w", err)
	}

	// Override with .env file values if not explicitly set via flags
	set := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) {
		set[f.Name] = true
	})

	if !set["http"] {
		if v := env["HTTP"]; v != "" {
			*httpAddr = v
		}
	}
	if !set["log-level"] {
		if v := env["LOG_LEVEL"]; v != "" {
			*logLevel = v
		}
	}
	if !set["base-url"] {
		if v := env["BASE_URL"]; v != "" {
			*baseURL = v
		}
	}
	if !set["google-client-id"] {
		if v := env["GOOGLE_CLIENT_ID"]; v != "" {
			*googleClientID = v
		}
	}
	if !set["google-client-secret"] {
		if v := env["GOOGLE_CLIENT_SECRET"]; v != "" {
			*googleClientSecret = v
		}
	}
	if !set["ms-client-id"] {
		if v := env["MS_CLIENT_ID"]; v != "" {
			*msClientID = v
		}
	}
	if !set["ms-client-secret"] {
		if v := env["MS_CLIENT_SECRET"]; v != "" {
			*msClientSecret = v
		}
	}
	if !set["github-client-id"] {
		if v := env["GITHUB_CLIENT_ID"]; v != "" {
			*githubClientID = v
		}
	}
	if !set["github-client-secret"] {
		if v := env["GITHUB_CLIENT_SECRET"]; v != "" {
			*githubClientSecret = v
		}
	}
	if !set["geo-db"] {
		if v := env["GEO_DB"]; v != "" {
			*geoDB = v
		}
	}

	// Test mode: use fake OAuth credentials for testing OAuth UI flow
	if os.Getenv("TEST_OAUTH") == "1" {
		if *googleClientID == "" {
			*googleClientID = "test-google-client-id"
			*googleClientSecret = "test-google-client-secret"
			slog.Info("TEST_OAUTH=1: Using fake Google OAuth credentials")
		}
		if *msClientID == "" {
			*msClientID = "test-ms-client-id"
			*msClientSecret = "test-ms-client-secret"
			slog.Info("TEST_OAUTH=1: Using fake Microsoft OAuth credentials")
		}
		if *githubClientID == "" {
			*githubClientID = "test-github-client-id"
			*githubClientSecret = "test-github-client-secret"
			slog.Info("TEST_OAUTH=1: Using fake GitHub OAuth credentials")
		}
	}

	// Validate OAuth credentials: both ID and secret must be set, or neither
	if (*googleClientID == "") != (*googleClientSecret == "") {
		return errors.New("google-client-id and google-client-secret must both be set or both be empty")
	}
	if (*msClientID == "") != (*msClientSecret == "") {
		return errors.New("ms-client-id and ms-client-secret must both be set or both be empty")
	}
	if (*githubClientID == "") != (*githubClientSecret == "") {
		return errors.New("github-client-id and github-client-secret must both be set or both be empty")
	}

	// Normalize addr: ":8080" becomes "localhost:8080"
	addr := *httpAddr
	if strings.HasPrefix(addr, ":") {
		addr = "localhost" + addr
	}

	// Append port to base URL if localhost and no port specified
	if u, err := url.Parse(*baseURL); err == nil && u.Port() == "" && u.Hostname() == "localhost" {
		if _, p, err := net.SplitHostPort(addr); err == nil {
			u.Host = net.JoinHostPort(u.Hostname(), p)
			*baseURL = u.String()
		}
	}

	switch *logLevel {
	case "debug":
		ll.Set(slog.LevelDebug)
	case "info":
	case "warn":
		ll.Set(slog.LevelWarn)
	case "error":
		ll.Set(slog.LevelError)
	default:
		return fmt.Errorf("unknown log level: %q", *logLevel)
	}

	// Create db directory for identity tables
	dbDir := filepath.Join(*dataDir, "db")
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

	gitMgr := git.NewManager(*dataDir, "", "")

	fileStore, err := content.NewFileStoreService(*dataDir, gitMgr, wsService, orgService, &serverCfg.Quotas.ResourceQuotas)
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

	svc := &handlers.Services{
		FileStore:     fileStore,
		Search:        content.NewSearchService(fileStore),
		User:          userService,
		Organization:  orgService,
		Workspace:     wsService,
		OrgInvitation: orgInvService,
		WSInvitation:  wsInvService,
		OrgMembership: orgMemService,
		WSMembership:  wsMemService,
		Session:       sessionService,
		EmailVerif:    emailVerificationService,
		Email:         emailService,
	}
	// Open IP geolocation database if configured
	var geoChecker *ipgeo.Checker
	if *geoDB != "" {
		var err error
		geoChecker, err = ipgeo.Open(*geoDB)
		if err != nil {
			return fmt.Errorf("failed to open geo database: %w", err)
		}
		defer func() { _ = geoChecker.Close() }()
		slog.InfoContext(ctx, "IP geolocation enabled", "db", *geoDB)
	}

	buildVersion, buildGoVersion, buildRevision, buildDirty := getBuildInfo()
	cfg := &server.Config{
		ServerConfig: serverCfg,
		DataDir:      *dataDir,
		BaseURL:      *baseURL,
		Version:      buildVersion,
		GoVersion:    buildGoVersion,
		Revision:     buildRevision,
		Dirty:        buildDirty,
		IPGeo:        geoChecker,
		OAuth: server.OAuthConfig{
			GoogleClientID:     *googleClientID,
			GoogleClientSecret: *googleClientSecret,
			MSClientID:         *msClientID,
			MSClientSecret:     *msClientSecret,
			GitHubClientID:     *githubClientID,
			GitHubClientSecret: *githubClientSecret,
			TestOAuth:          os.Getenv("TEST_OAUTH") == "1",
		},
	}

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           server.NewRouter(svc, cfg),
		BaseContext:       func(_ net.Listener) context.Context { return ctx },
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Run server in goroutine
	serverErr := make(chan error, 1)
	go func() {
		slog.InfoContext(ctx, "Starting server", "addr", addr, "baseURL", *baseURL, "version", buildVersion)
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

func loadDotEnv(dataDir string) (map[string]string, error) {
	env := make(map[string]string)
	path := filepath.Join(dataDir, ".env")
	envContent, err := os.ReadFile(path) //nolint:gosec // G304: path is constructed from dataDir flag, not user input
	if err != nil {
		if os.IsNotExist(err) {
			return env, nil
		}
		return nil, err
	}

	for line := range strings.SplitSeq(string(envContent), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		if strings.HasPrefix(val, "'") || strings.HasSuffix(val, "'") {
			if strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'") {
				return nil, fmt.Errorf("single quotes are not supported for wrapping in .env: %s", line)
			}
			return nil, fmt.Errorf("unbalanced single quotes in .env: %s", line)
		}

		if strings.HasPrefix(val, "\"") {
			unquoted, err := strconv.Unquote(val)
			if err != nil {
				return nil, fmt.Errorf("failed to unquote %s: %w", key, err)
			}
			val = unquoted
		}

		env[key] = val
	}
	return env, nil
}

func saveDotEnv(dataDir string, env map[string]string) error {
	path := filepath.Join(dataDir, ".env")
	var lines []string
	for k, v := range env {
		if v != "" {
			lines = append(lines, fmt.Sprintf("%s=%s", k, v))
		}
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
}

func runOnboarding(dataDir string) error {
	fmt.Println("Welcome to mddb! Let's set up your configuration.")
	fmt.Println("This wizard will help you configure OAuth settings.")
	fmt.Println("")

	reader := bufio.NewReader(os.Stdin)
	env := make(map[string]string)

	// Base URL
	fmt.Println("\n--- Base URL Setup ---")
	fmt.Println("The base URL is used for OAuth callback URLs.")
	fmt.Println("If no port is specified, it will use the server's port automatically.")
	fmt.Print("Base URL (default: http://localhost): ")
	val, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read base URL: %w", err)
	}
	baseURL := strings.TrimSpace(val)
	if baseURL == "" {
		baseURL = "http://localhost"
	}
	env["BASE_URL"] = baseURL
	// For display purposes in onboarding, show with default port if localhost
	displayBaseURL := baseURL
	if u, err := url.Parse(baseURL); err == nil && u.Port() == "" && u.Hostname() == "localhost" {
		u.Host = net.JoinHostPort(u.Hostname(), "8080")
		displayBaseURL = u.String()
	}

	// Google OAuth
	fmt.Println("\n--- Google OAuth Setup ---")
	fmt.Println("To use Google login, create a project at https://console.cloud.google.com/apis/credentials")
	fmt.Printf("Configure an OAuth 2.0 Client ID with redirect URI: %s/api/v1/auth/oauth/google/callback\n", displayBaseURL)
	fmt.Print("Google Client ID (optional): ")
	val, err = reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read Google Client ID: %w", err)
	}
	env["GOOGLE_CLIENT_ID"] = strings.TrimSpace(val)
	if env["GOOGLE_CLIENT_ID"] != "" {
		fmt.Print("Google Client Secret: ")
		val, err = reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read Google Client Secret: %w", err)
		}
		env["GOOGLE_CLIENT_SECRET"] = strings.TrimSpace(val)
	}

	// Microsoft OAuth
	fmt.Println("\n--- Microsoft OAuth Setup ---")
	fmt.Println("To use Microsoft login, register an app at https://portal.azure.com/")
	fmt.Printf("Configure a redirect URI: %s/api/v1/auth/oauth/microsoft/callback\n", displayBaseURL)
	fmt.Print("Microsoft Client ID (optional): ")
	val, err = reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read Microsoft Client ID: %w", err)
	}
	env["MS_CLIENT_ID"] = strings.TrimSpace(val)
	if env["MS_CLIENT_ID"] != "" {
		fmt.Print("Microsoft Client Secret: ")
		val, err = reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read Microsoft Client Secret: %w", err)
		}
		env["MS_CLIENT_SECRET"] = strings.TrimSpace(val)
	}

	// GitHub OAuth
	fmt.Println("\n--- GitHub OAuth Setup ---")
	fmt.Println("To use GitHub login, create an OAuth App at https://github.com/settings/developers")
	fmt.Printf("Configure a redirect URI: %s/api/v1/auth/oauth/github/callback\n", displayBaseURL)
	fmt.Print("GitHub Client ID (optional): ")
	val, err = reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read GitHub Client ID: %w", err)
	}
	env["GITHUB_CLIENT_ID"] = strings.TrimSpace(val)
	if env["GITHUB_CLIENT_ID"] != "" {
		fmt.Print("GitHub Client Secret: ")
		val, err = reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read GitHub Client Secret: %w", err)
		}
		env["GITHUB_CLIENT_SECRET"] = strings.TrimSpace(val)
	}

	fmt.Println("")
	if err := saveDotEnv(dataDir, env); err != nil {
		return fmt.Errorf("failed to save .env file: %w", err)
	}

	fmt.Printf("Configuration saved to %s/.env\n", dataDir)
	fmt.Println("You can edit this file later to change your settings.")
	fmt.Println("")

	return nil
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
