// Package app wires together the dependencies of the API server (repositories,
// services, use cases, and handlers) and builds the Echo router. It exists so
// that later features only need to add a new field to Container or a new
// route-registrar file, rather than editing a single monolithic main function.
package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/ai"
	domainauth "github.com/SHIMA0111/multi-user-ai/server/internal/domain/auth"
	domainmessage "github.com/SHIMA0111/multi-user-ai/server/internal/domain/message"
	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
	domainuser "github.com/SHIMA0111/multi-user-ai/server/internal/domain/user"
	"github.com/SHIMA0111/multi-user-ai/server/internal/infrastructure/config"
	"github.com/SHIMA0111/multi-user-ai/server/internal/infrastructure/database"
	ifauth "github.com/SHIMA0111/multi-user-ai/server/internal/interface/auth"
	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/gateway"
	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/handler"
	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/repository/postgres"
	authusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/auth"
	msgusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/message"
	roomusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/room"
	userusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/user"
)

// Container holds every dependency wired up for the API server: the loaded
// configuration, the database connection pool, the base logger, repositories,
// services/gateways, use cases, and HTTP handlers. Later steps that add a new
// feature should add a new field here (and construct it in NewContainer)
// rather than modifying cmd/api/main.go.
type Container struct {
	// Config is the application configuration loaded from environment variables.
	Config *config.Config
	// Pool is the PostgreSQL connection pool. Callers are responsible for
	// closing it (typically via a deferred Pool.Close() in main).
	Pool *pgxpool.Pool
	// Logger is the base structured logger used to build request-scoped loggers.
	Logger *slog.Logger

	// Repositories
	UserRepo domainuser.UserRepository
	RoomRepo domainroom.RoomRepository
	MsgRepo  domainmessage.MessageRepository

	// Services / Gateways
	AuthService domainauth.AuthService
	LLMGateway  ai.LLMGateway

	// Use cases
	AuthUC *authusecase.AuthUsecase
	RoomUC *roomusecase.RoomUsecase
	MsgUC  *msgusecase.MessageUsecase
	UserUC *userusecase.UserUsecase

	// Handlers
	HealthHandler  *handler.HealthHandler
	AuthHandler    *handler.AuthHandler
	RoomHandler    *handler.RoomHandler
	MessageHandler *handler.MessageHandler
	ModelHandler   *handler.ModelHandler
	UserHandler    *handler.UserHandler
}

// NewContainer builds a Container: it opens the database connection pool,
// then wires repositories, services/gateways, use cases, and handlers in the
// same order previously inlined in cmd/api/main.go (repositories →
// services/gateway → usecases → handlers). If any step fails, it closes any
// already-opened pool and returns an error; callers do not need to close the
// pool themselves in that case.
func NewContainer(ctx context.Context, cfg *config.Config) (*Container, error) {
	pool, err := database.NewPool(ctx, cfg.DatabaseURL,
		cfg.DBMaxConnLifetime, cfg.DBMaxConnIdleTime, cfg.DBHealthCheckPeriod)
	if err != nil {
		return nil, fmt.Errorf("connect to database: %w", err)
	}

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
	userUC := userusecase.NewUserUsecase(userRepo)

	// Handlers
	healthHandler := handler.NewHealthHandler()
	authHandler := handler.NewAuthHandler(authUC)
	roomHandler := handler.NewRoomHandler(roomUC)
	msgHandler := handler.NewMessageHandler(msgUC)
	modelHandler := handler.NewModelHandler(llmClient)
	userHandler := handler.NewUserHandler(userUC)

	return &Container{
		Config: cfg,
		Pool:   pool,
		Logger: slog.Default(),

		UserRepo: userRepo,
		RoomRepo: roomRepo,
		MsgRepo:  msgRepo,

		AuthService: authService,
		LLMGateway:  llmClient,

		AuthUC: authUC,
		RoomUC: roomUC,
		MsgUC:  msgUC,
		UserUC: userUC,

		HealthHandler:  healthHandler,
		AuthHandler:    authHandler,
		RoomHandler:    roomHandler,
		MessageHandler: msgHandler,
		ModelHandler:   modelHandler,
		UserHandler:    userHandler,
	}, nil
}
