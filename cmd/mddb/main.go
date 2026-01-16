// Package main is the entry point for the mddb application.
//
// mddb is a local-first markdown database that stores content as files
// and provides a web interface for management.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/maruel/mddb/internal/server"
	"github.com/maruel/mddb/internal/storage"
)

func main() {
	if err := mainImpl(); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(os.Stderr, "mddb: %v\n", err)
		os.Exit(1)
	}
}

func mainImpl() error {
	port := flag.String("port", "8080", "Port to listen on")
	dataDir := flag.String("data-dir", "./data", "Data directory")
	logLevel := flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	jwtSecret := flag.String("jwt-secret", "dev-secret-keep-it-safe", "JWT secret for authentication")
	flag.Parse()

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

	userService, err := storage.NewUserService(*dataDir)
	if err != nil {
		return fmt.Errorf("failed to initialize user service: %w", err)
	}

	orgService, err := storage.NewOrganizationService(*dataDir)
	if err != nil {
		return fmt.Errorf("failed to initialize organization service: %w", err)
	}

	// Create context that cancels on SIGTERM and SIGINT
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	addr := ":" + *port
	httpServer := &http.Server{
		Addr:        addr,
		Handler:     server.NewRouter(fileStore, gitService, userService, orgService, *jwtSecret),
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
