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

	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"

	"github.com/SHIMA0111/multi-user-ai/server/internal/infrastructure/config"
	"github.com/SHIMA0111/multi-user-ai/server/internal/infrastructure/database"
	ifauth "github.com/SHIMA0111/multi-user-ai/server/internal/interface/auth"
	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/gateway"
	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/handler"
	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/middleware"
	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/repository/postgres"
	authusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/auth"
	msgusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/message"
	roomusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/room"
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

	// Database connection pool
	ctx := context.Background()
	pool, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	slog.Info("connected to database")

	// Repositories
	userRepo := postgres.NewUserRepository(pool)
	roomRepo := postgres.NewRoomRepository(pool)
	msgRepo := postgres.NewMessageRepository(pool)

	// Services / Gateways
	authService := ifauth.NewSimpleJWTService(userRepo, cfg.JWTSecret)
	llmClient := gateway.NewLLMClient(cfg.LLMGatewayURL)

	// Usecases
	authUC := authusecase.NewAuthUsecase(authService)
	roomUC := roomusecase.NewRoomUsecase(roomRepo)
	msgUC := msgusecase.NewMessageUsecase(msgRepo, roomRepo, llmClient)

	// Handlers
	healthHandler := handler.NewHealthHandler()
	authHandler := handler.NewAuthHandler(authUC)
	roomHandler := handler.NewRoomHandler(roomUC)
	msgHandler := handler.NewMessageHandler(msgUC)

	// Echo setup
	e := echo.New()
	e.HideBanner = true
	e.Use(echomw.Recover())
	e.Use(echomw.RequestID())

	// Public routes
	e.GET("/health", healthHandler.Health)
	e.POST("/auth/register", authHandler.Register)
	e.POST("/auth/login", authHandler.Login)

	// Authenticated routes
	auth := e.Group("", middleware.JWTAuth(authUC))
	auth.POST("/rooms", roomHandler.Create)
	auth.GET("/rooms", roomHandler.List)
	auth.GET("/rooms/:roomId", roomHandler.Get)
	auth.PUT("/rooms/:roomId", roomHandler.Update)
	auth.DELETE("/rooms/:roomId", roomHandler.Delete)
	auth.POST("/rooms/:roomId/messages", msgHandler.Send)
	auth.GET("/rooms/:roomId/messages", msgHandler.List)
	auth.POST("/rooms/:roomId/messages/ai", msgHandler.SendAI)
	auth.POST("/rooms/:roomId/messages/:messageId/regenerate", msgHandler.RegenerateAI)

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
