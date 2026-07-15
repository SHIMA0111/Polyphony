package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/SHIMA0111/multi-user-ai/server/internal/app"
	"github.com/SHIMA0111/multi-user-ai/server/internal/infrastructure/config"
)

func main() {
	// Structured JSON logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Build the DI container (database pool, repositories, services, use
	// cases, and handlers).
	ctx := context.Background()
	container, err := app.NewContainer(ctx, cfg)
	if err != nil {
		slog.Error("failed to build container", "error", err)
		os.Exit(1)
	}
	defer container.Pool.Close()

	// Build the Echo router.
	e := app.NewRouter(container)

	// Start server
	addr := fmt.Sprintf(":%s", cfg.Port)
	slog.Info("starting server", "addr", addr)

	errCh := make(chan error, 1)
	go func() {
		if err := e.Start(addr); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "error", err)
			errCh <- err
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-quit:
	case err := <-errCh:
		slog.Error("server terminated", "error", err)
		return
	}

	slog.Info("shutting down server")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err = e.Shutdown(shutdownCtx); err != nil {
		slog.Error("failed to gracefully shutdown", "error", err)
	}

	slog.Info("server stopped")
}
