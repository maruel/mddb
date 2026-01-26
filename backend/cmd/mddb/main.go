// Package main is the entry point for the mddb server.
//
// mddb is a local-first markdown database that stores content as files,
// provides OAuth authentication (Google/Microsoft), and exposes a RESTful
// HTTP API. Configuration is read from CLI flags and a .env file, with
// interactive onboarding on first run.
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
	"github.com/maruel/mddb/backend/internal/server"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
	"github.com/maruel/mddb/backend/internal/utils"
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
	port := flag.String("port", "8080", "Port to listen on")
	dataDir := flag.String("data-dir", "./data", "Data directory")
	logLevel := flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	baseURL := flag.String("base-url", "http://localhost", "Base URL for OAuth callbacks (e.g., https://example.com)")
	googleClientID := flag.String("google-client-id", "", "Google OAuth client ID")
	googleClientSecret := flag.String("google-client-secret", "", "Google OAuth client secret")
	msClientID := flag.String("ms-client-id", "", "Microsoft OAuth client ID")
	msClientSecret := flag.String("ms-client-secret", "", "Microsoft OAuth client secret")
	githubClientID := flag.String("github-client-id", "", "GitHub OAuth client ID")
	githubClientSecret := flag.String("github-client-secret", "", "GitHub OAuth client secret")
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
	logger := slog.New(tint.NewHandler(colorable.NewColorable(os.Stderr), &tint.Options{
		Level:      ll,
		TimeFormat: "15:04:05.000", // Like time.TimeOnly plus milliseconds.
		NoColor:    !isatty.IsTerminal(os.Stderr.Fd()),
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
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

	env, err := loadDotEnv(*dataDir)
	if err != nil {
		return err
	}

	jwtSecret := env["JWT_SECRET"]
	if jwtSecret == "" {
		var err error
		jwtSecret, err = utils.GenerateToken(32)
		if err != nil {
			return fmt.Errorf("failed to generate JWT secret: %w", err)
		}
		env["JWT_SECRET"] = jwtSecret
		if err := saveDotEnv(*dataDir, env); err != nil {
			return fmt.Errorf("failed to save .env file: %w", err)
		}
		slog.Info("Generated JWT_SECRET and saved to .env")
	}

	// Override with .env file values if not explicitly set via flags
	set := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) {
		set[f.Name] = true
	})

	if !set["port"] {
		if v := env["PORT"]; v != "" {
			*port = v
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

	// Append port to base URL if localhost and no port specified
	if u, err := url.Parse(*baseURL); err == nil && u.Port() == "" && u.Hostname() == "localhost" {
		u.Host = net.JoinHostPort(u.Hostname(), *port)
		*baseURL = u.String()
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

	fileStore, err := content.NewFileStoreService(*dataDir, gitMgr, wsService, orgService)
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

	// Watch own executable for modifications (for development restarts)
	if err := watchExecutable(ctx, stop); err != nil {
		return fmt.Errorf("failed to watch executable: %w", err)
	}

	addr := ":" + *port
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           server.NewRouter(fileStore, userService, orgService, wsService, orgInvService, wsInvService, orgMemService, wsMemService, sessionService, jwtSecret, *baseURL, *googleClientID, *googleClientSecret, *msClientID, *msClientSecret, *githubClientID, *githubClientSecret),
		BaseContext:       func(_ net.Listener) context.Context { return ctx },
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Run server in goroutine
	serverErr := make(chan error, 1)
	go func() {
		slog.InfoContext(ctx, "Starting server", "addr", addr)
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
	info, ok := debug.ReadBuildInfo()
	if !ok {
		fmt.Println("mddb version unknown")
		return
	}

	version := info.Main.Version
	if version == "" || version == "(devel)" {
		version = "dev"
	}

	revision := "unknown"
	dirty := false
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			dirty = setting.Value == "true"
		}
	}

	fmt.Printf("mddb %s\n", version)
	fmt.Printf("  Go version: %s\n", info.GoVersion)
	fmt.Printf("  Revision:   %s\n", revision)
	if dirty {
		fmt.Printf("  Modified:   true\n")
	}
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

	lines := strings.Split(string(envContent), "\n")
	for _, line := range lines {
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
	fmt.Println("This wizard will help you configure OAuth and security settings.")
	fmt.Println("")

	reader := bufio.NewReader(os.Stdin)
	env := make(map[string]string)

	// JWT Secret
	jwtSecret, err := utils.GenerateToken(32)
	if err != nil {
		return fmt.Errorf("failed to generate JWT secret: %w", err)
	}
	env["JWT_SECRET"] = jwtSecret

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
	fmt.Printf("Configure an OAuth 2.0 Client ID with redirect URI: %s/api/auth/oauth/google/callback\n", displayBaseURL)
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
	fmt.Printf("Configure a redirect URI: %s/api/auth/oauth/microsoft/callback\n", displayBaseURL)
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
	fmt.Printf("Configure a redirect URI: %s/api/auth/oauth/github/callback\n", displayBaseURL)
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
