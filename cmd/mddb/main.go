package main

import (
	"context"
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
	if err := mainImpl(); err != nil && err != context.Canceled {
		fmt.Fprintf(os.Stderr, "mddb: %v\n", err)
		os.Exit(1)
	}
}

func mainImpl() error {
	port := flag.String("port", "8080", "Port to listen on")
	dataDir := flag.String("data-dir", "./data", "Data directory")
	logLevel := flag.String("log-level", "info", "Log level (debug, info, warn, error)")
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

	// Create context that cancels on SIGTERM and SIGINT
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	addr := ":" + *port
	httpServer := &http.Server{
		Addr:        addr,
		Handler:     server.NewRouter(fileStore),
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
		if err != http.ErrServerClosed {
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
