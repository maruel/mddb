// Package main is the entry point for the mddb application.
//
// mddb is a local-first markdown database that stores content as files
// and provides a web interface for management.
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
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/maruel/mddb/backend/internal/server"
	"github.com/maruel/mddb/backend/internal/storage"
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
			return fmt.Errorf(".env file not found and stdin is not a TTY; cannot run onboarding")
		}
	}

	env, err := loadDotEnv(*dataDir)
	if err != nil {
		return err
	}

	jwtSecret := env["JWT_SECRET"]
	if jwtSecret == "" {
		return fmt.Errorf("JWT_SECRET is required in .env file; run onboarding or create it manually")
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

	fileStore, err := storage.NewFileStore(*dataDir)
	if err != nil {
		return fmt.Errorf("failed to initialize file store: %w", err)
	}

	gitService, err := storage.NewGitService(*dataDir)
	if err != nil {
		return fmt.Errorf("failed to initialize git service: %w", err)
	}

	memService, err := storage.NewMembershipService(*dataDir)
	if err != nil {
		return fmt.Errorf("failed to initialize membership service: %w", err)
	}

	orgService, err := storage.NewOrganizationService(*dataDir, fileStore, gitService)
	if err != nil {
		return fmt.Errorf("failed to initialize organization service: %w", err)
	}

	userService, err := storage.NewUserService(*dataDir, memService, orgService)
	if err != nil {
		return fmt.Errorf("failed to initialize user service: %w", err)
	}

	invService, err := storage.NewInvitationService(*dataDir)
	if err != nil {
		return fmt.Errorf("failed to initialize invitation service: %w", err)
	}

	remoteService, err := storage.NewGitRemoteService(*dataDir)
	if err != nil {
		return fmt.Errorf("failed to initialize git remote service: %w", err)
	}

	// Create context that cancels on SIGTERM and SIGINT
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	addr := ":" + *port
	httpServer := &http.Server{
		Addr:        addr,
		Handler:     server.NewRouter(fileStore, gitService, userService, orgService, invService, memService, remoteService, jwtSecret, *googleClientID, *googleClientSecret, *msClientID, *msClientSecret),
		BaseContext: func(_ net.Listener) context.Context { return ctx },
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
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return env, nil
		}
		return nil, err
	}

	lines := strings.Split(string(content), "\n")
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
	jwtSecret, _ := storage.GenerateToken(32)
	env["JWT_SECRET"] = jwtSecret

	// Google OAuth
	fmt.Println("\n--- Google OAuth Setup ---")
	fmt.Println("To use Google login, create a project at https://console.cloud.google.com/")
	fmt.Println("Configure an OAuth 2.0 Client ID with redirect URI: http://localhost:8080/api/auth/oauth/google/callback")
	fmt.Print("Google Client ID (optional): ")
	val, _ := reader.ReadString('\n')
	env["GOOGLE_CLIENT_ID"] = strings.TrimSpace(val)
	if env["GOOGLE_CLIENT_ID"] != "" {
		fmt.Print("Google Client Secret: ")
		val, _ = reader.ReadString('\n')
		env["GOOGLE_CLIENT_SECRET"] = strings.TrimSpace(val)
	}

	// Microsoft OAuth
	fmt.Println("\n--- Microsoft OAuth Setup ---")
	fmt.Println("To use Microsoft login, register an app at https://portal.azure.com/")
	fmt.Println("Configure a redirect URI: http://localhost:8080/api/auth/oauth/microsoft/callback")
	fmt.Print("Microsoft Client ID (optional): ")
	val, _ = reader.ReadString('\n')
	env["MS_CLIENT_ID"] = strings.TrimSpace(val)
	if env["MS_CLIENT_ID"] != "" {
		fmt.Print("Microsoft Client Secret: ")
		val, _ = reader.ReadString('\n')
		env["MS_CLIENT_SECRET"] = strings.TrimSpace(val)
	}

	fmt.Println("")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
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