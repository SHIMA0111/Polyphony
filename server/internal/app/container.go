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
	domainattachment "github.com/SHIMA0111/multi-user-ai/server/internal/domain/attachment"
	domainauth "github.com/SHIMA0111/multi-user-ai/server/internal/domain/auth"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/event"
	domainmessage "github.com/SHIMA0111/multi-user-ai/server/internal/domain/message"
	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
	domainstorage "github.com/SHIMA0111/multi-user-ai/server/internal/domain/storage"
	domainuser "github.com/SHIMA0111/multi-user-ai/server/internal/domain/user"
	"github.com/SHIMA0111/multi-user-ai/server/internal/infrastructure/config"
	"github.com/SHIMA0111/multi-user-ai/server/internal/infrastructure/database"
	ifauth "github.com/SHIMA0111/multi-user-ai/server/internal/interface/auth"
	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/gateway"
	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/handler"
	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/repository/postgres"
	ifstorage "github.com/SHIMA0111/multi-user-ai/server/internal/interface/storage"
	attachmentusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/attachment"
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
	UserRepo       domainuser.UserRepository
	RoomRepo       domainroom.RoomRepository
	MsgRepo        domainmessage.MessageRepository
	AttachmentRepo domainattachment.AttachmentRepository

	// Services / Gateways
	AuthService domainauth.AuthService
	LLMGateway  ai.LLMGateway
	// ObjectStorage is the domain/storage.ObjectStorage adapter (backed by
	// MinIO/S3 via aws-sdk-go-v2) used to presign attachment upload/view URLs.
	ObjectStorage domainstorage.ObjectStorage
	// MessageHub is the event.MessageHub used by MsgUC to broadcast
	// message_created/message_updated events. It is exposed on the
	// Container (rather than kept private) so later steps (e.g. Step 15's
	// WebSocket endpoint) can call Subscribe on the same instance.
	MessageHub event.MessageHub

	// Use cases
	AuthUC       *authusecase.AuthUsecase
	RoomUC       *roomusecase.RoomUsecase
	MsgUC        *msgusecase.MessageUsecase
	UserUC       *userusecase.UserUsecase
	AttachmentUC *attachmentusecase.AttachmentUsecase

	// Handlers
	HealthHandler     *handler.HealthHandler
	AuthHandler       *handler.AuthHandler
	RoomHandler       *handler.RoomHandler
	MessageHandler    *handler.MessageHandler
	ModelHandler      *handler.ModelHandler
	UserHandler       *handler.UserHandler
	AttachmentHandler *handler.AttachmentHandler
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
	attachmentRepo := postgres.NewAttachmentRepository(pool)

	// Services / Gateways
	authService := ifauth.NewSimpleJWTService(userRepo, cfg.JWTSecret)
	llmClient := gateway.NewLLMClient(cfg.LLMGatewayURL)
	objectStorage := ifstorage.NewS3Storage(
		cfg.S3Endpoint, cfg.S3Region, cfg.S3Bucket, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3ForcePathStyle,
	)
	messageHub := event.NewInProcessHub()

	// Usecases
	authUC := authusecase.NewAuthUsecase(authService)
	roomUC := roomusecase.NewRoomUsecase(roomRepo)
	msgUC := msgusecase.NewMessageUsecase(msgRepo, roomRepo, llmClient, messageHub)
	userUC := userusecase.NewUserUsecase(userRepo)
	attachmentUC := attachmentusecase.NewAttachmentUsecase(attachmentRepo, roomRepo, msgRepo, objectStorage)

	// Handlers
	healthHandler := handler.NewHealthHandler()
	authHandler := handler.NewAuthHandler(authUC)
	roomHandler := handler.NewRoomHandler(roomUC)
	msgHandler := handler.NewMessageHandler(msgUC)
	modelHandler := handler.NewModelHandler(llmClient)
	userHandler := handler.NewUserHandler(userUC)
	attachmentHandler := handler.NewAttachmentHandler(attachmentUC)

	return &Container{
		Config: cfg,
		Pool:   pool,
		Logger: slog.Default(),

		UserRepo:       userRepo,
		RoomRepo:       roomRepo,
		MsgRepo:        msgRepo,
		AttachmentRepo: attachmentRepo,

		AuthService:   authService,
		LLMGateway:    llmClient,
		ObjectStorage: objectStorage,
		MessageHub:    messageHub,

		AuthUC:       authUC,
		RoomUC:       roomUC,
		MsgUC:        msgUC,
		UserUC:       userUC,
		AttachmentUC: attachmentUC,

		HealthHandler:     healthHandler,
		AuthHandler:       authHandler,
		RoomHandler:       roomHandler,
		MessageHandler:    msgHandler,
		ModelHandler:      modelHandler,
		UserHandler:       userHandler,
		AttachmentHandler: attachmentHandler,
	}, nil
}
