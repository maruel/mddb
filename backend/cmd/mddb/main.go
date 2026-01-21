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
	"github.com/maruel/mddb/backend/internal/server"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
	"github.com/maruel/mddb/backend/internal/utils"
)

var (
	errNoEnvFile   = errors.New(".env file not found and stdin is not a TTY; cannot run onboarding")
	errNoJWTSecret = errors.New("JWT_SECRET is required in .env file; run onboarding or create it manually")
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
	flag.Parse()

	if *version {
		printVersion()
		return nil
	}

	// Check if onboarding is needed (no .env file)
	envPath := filepath.Join(*dataDir, ".env")
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		// Only run onboarding if stdin is a TTY
		if info, err := os.Stdin.Stat(); err == nil && (info.Mode()&os.ModeCharDevice) != 0 {
			if err := runOnboarding(*dataDir); err != nil {
				return fmt.Errorf("onboarding failed: %w", err)
			}
		} else {
			return errNoEnvFile
		}
	}

	env, err := loadDotEnv(*dataDir)
	if err != nil {
		return err
	}

	jwtSecret := env["JWT_SECRET"]
	if jwtSecret == "" {
		return errNoJWTSecret
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

	// Append port to base URL if localhost and no port specified
	if u, err := url.Parse(*baseURL); err == nil && u.Port() == "" && u.Hostname() == "localhost" {
		u.Host = net.JoinHostPort(u.Hostname(), *port)
		*baseURL = u.String()
	}

	if len(flag.Args()) > 0 {
		return fmt.Errorf("unknown arguments: %v", flag.Args())
	}

	var ll slog.Level
	switch *logLevel {
	case "debug":
		ll = slog.LevelDebug
	case "info":
		ll = slog.LevelInfo
	case "warn":
		ll = slog.LevelWarn
	case "error":
		ll = slog.LevelError
	default:
		return fmt.Errorf("unknown log level: %q", *logLevel)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: ll})))

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

	memService, err := identity.NewMembershipService(filepath.Join(dbDir, "memberships.jsonl"), userService, orgService)
	if err != nil {
		return fmt.Errorf("failed to initialize membership service: %w", err)
	}

	gitService, err := git.New(context.Background(), *dataDir, "", "")
	if err != nil {
		return fmt.Errorf("failed to initialize git service: %w", err)
	}

	fileStore, err := content.NewFileStore(*dataDir, gitService, orgService)
	if err != nil {
		return fmt.Errorf("failed to initialize file store: %w", err)
	}

	invService, err := identity.NewInvitationService(filepath.Join(dbDir, "invitations.jsonl"))
	if err != nil {
		return fmt.Errorf("failed to initialize invitation service: %w", err)
	}

	// Create context that cancels on SIGTERM and SIGINT
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// Watch own executable for modifications (for development restarts)
	if err := watchExecutable(ctx, stop); err != nil {
		return fmt.Errorf("failed to watch executable: %w", err)
	}

	addr := ":" + *port
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           server.NewRouter(fileStore, userService, orgService, invService, memService, jwtSecret, *baseURL, *googleClientID, *googleClientSecret, *msClientID, *msClientSecret),
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

	fmt.Println("")
	if err := os.MkdirAll(dataDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for data directories
		return fmt.Errorf("failed to create data directory: %w", err)
	}

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

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	// Watch the directory since the file gets replaced on rebuild
	if err := watcher.Add(filepath.Dir(exe)); err != nil {
		_ = watcher.Close()
		return err
	}

	base := filepath.Base(exe)
	go func() {
		defer func() { _ = watcher.Close() }()
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if filepath.Base(event.Name) == base && (event.Op&(fsnotify.Write|fsnotify.Create) != 0) {
					slog.InfoContext(ctx, "Executable modified, initiating shutdown")
					stop()
					return
				}
			case <-watcher.Errors:
			}
		}
	}()
	return nil
}
