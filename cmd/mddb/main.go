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
	logger := initLogger(*logLevel)
	slog.SetDefault(logger)
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
		slog.Info("Starting server", "addr", addr)
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
		slog.Info("Shutting down server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown error: %w", err)
		}
		slog.Info("Server stopped")
	}
	return nil
}

// initLogger initializes a structured logger with the given level
func initLogger(level string) *slog.Logger {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
	}

	handler := slog.NewTextHandler(os.Stdout, opts)
	return slog.New(handler)
}
