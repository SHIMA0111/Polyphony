// Package app wires together the dependencies of the API server (repositories,
// services, use cases, and handlers) and builds the Echo router. It exists so
// that later features only need to add a new field to Container or a new
// route-registrar file, rather than editing a single monolithic main function.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/ai"
	domainattachment "github.com/SHIMA0111/multi-user-ai/server/internal/domain/attachment"
	domainauth "github.com/SHIMA0111/multi-user-ai/server/internal/domain/auth"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/event"
	domaininvitation "github.com/SHIMA0111/multi-user-ai/server/internal/domain/invitation"
	domainmessage "github.com/SHIMA0111/multi-user-ai/server/internal/domain/message"
	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
	domainstorage "github.com/SHIMA0111/multi-user-ai/server/internal/domain/storage"
	domainuser "github.com/SHIMA0111/multi-user-ai/server/internal/domain/user"
	"github.com/SHIMA0111/multi-user-ai/server/internal/infrastructure/config"
	"github.com/SHIMA0111/multi-user-ai/server/internal/infrastructure/database"
	infraevent "github.com/SHIMA0111/multi-user-ai/server/internal/infrastructure/event"
	ifauth "github.com/SHIMA0111/multi-user-ai/server/internal/interface/auth"
	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/gateway"
	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/handler"
	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/repository/postgres"
	ifstorage "github.com/SHIMA0111/multi-user-ai/server/internal/interface/storage"
	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/wsticket"
	attachmentusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/attachment"
	authusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/auth"
	invitationusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/invitation"
	msgusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/message"
	modelusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/model"
	roomusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/room"
	userusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/user"
)

// wsTicketTTL is the lifetime given to WebSocket upgrade tickets minted by
// the wsticket.Issuer wired up below. Kept short since a ticket only needs
// to bridge the gap between the authenticated POST /ws/ticket call and the
// WebSocket upgrade that immediately follows it.
const wsTicketTTL = 60 * time.Second

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

	// RedisClient is the shared Redis client used when Config.MessageHubDriver
	// is "redis". It is nil when the inprocess driver is selected. It is kept
	// on the Container (rather than only captured in a closure) so later
	// steps (e.g. Step 33's rate limiting and Kratos session cache) can reuse
	// the same client instead of opening a second connection pool. Callers
	// are responsible for closing it (typically via a deferred
	// RedisClient.Close() in main, guarded by a nil check).
	RedisClient *redis.Client

	// Repositories
	UserRepo domainuser.UserRepository
	RoomRepo domainroom.RoomRepository
	MsgRepo  domainmessage.MessageRepository
	// AttachmentRepo is the domain/attachment.AttachmentRepository backing
	// AttachmentUC's presign/link/list operations.
	AttachmentRepo domainattachment.AttachmentRepository
	InvitationRepo domaininvitation.InvitationRepository

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
	AuthUC *authusecase.AuthUsecase
	RoomUC *roomusecase.RoomUsecase
	MsgUC  *msgusecase.MessageUsecase
	UserUC *userusecase.UserUsecase
	// AttachmentUC implements the attachment presign/link/list business
	// logic (see usecase/attachment.AttachmentUsecase).
	AttachmentUC *attachmentusecase.AttachmentUsecase
	// ModelUC lists available AI models across all configured providers via
	// LLMGateway (see usecase/model.ModelUsecase).
	ModelUC      *modelusecase.ModelUsecase
	InvitationUC *invitationusecase.InvitationUsecase

	// Handlers
	HealthHandler  *handler.HealthHandler
	AuthHandler    *handler.AuthHandler
	RoomHandler    *handler.RoomHandler
	MessageHandler *handler.MessageHandler
	ModelHandler   *handler.ModelHandler
	UserHandler    *handler.UserHandler
	// AttachmentHandler serves the presigned-upload/attach/list attachment
	// endpoints, delegating to AttachmentUC.
	AttachmentHandler *handler.AttachmentHandler
	// WebSocketHandler serves the ticket-issuance and connection-upgrade
	// endpoints that push real-time event.RoomEvent updates to clients.
	WebSocketHandler  *handler.WebSocketHandler
	InvitationHandler *handler.InvitationHandler
	TokenHandler      *handler.TokenHandler
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
	invitationRepo := postgres.NewInvitationRepository(pool)

	// Services / Gateways
	//
	// AuthService is the Phase 9 swap point (see CLAUDE.md's Interface Swap
	// Points table): AUTH_MODE selects SimpleJWTService (default) or
	// KratosAuthService, both of which satisfy domainauth.AuthService, so no
	// downstream usecase/handler code needs to change based on this branch.
	var authService domainauth.AuthService
	switch cfg.AuthMode {
	case "kratos":
		authService = ifauth.NewKratosAuthService(userRepo, cfg.KratosPublicURL, cfg.KratosAdminURL, cfg.KratosCookieName,
			&http.Client{Timeout: 10 * time.Second})
	default:
		authService = ifauth.NewSimpleJWTService(userRepo, cfg.JWTSecret)
	}
	// LLMGateway is the Phase 8 swap point (see CLAUDE.md's Interface Swap
	// Points table): LLM_GATEWAY_TRANSPORT selects the REST LLMClient
	// (default) or the gRPC GRPCClient, both of which satisfy
	// ai.LLMGateway, so no downstream usecase/handler code needs to change
	// based on this branch.
	var llmGateway ai.LLMGateway = gateway.NewLLMClient(cfg.LLMGatewayURL)
	if cfg.LLMGatewayTransport == "grpc" {
		grpcClient, err := gateway.NewGRPCClient(
			cfg.LLMGatewayGRPCAddr, cfg.LLMGatewayGRPCMaxRetries, cfg.LLMGatewayGRPCBaseBackoff)
		if err != nil {
			pool.Close()
			return nil, fmt.Errorf("build gRPC LLM Gateway client: %w", err)
		}
		llmGateway = grpcClient

		// Fail fast/log a warning if the gateway isn't reachable, but never
		// fail container construction on it: in Compose, the api container
		// may start before the llm-gateway container becomes healthy, and
		// individual Complete/ListModels calls already surface their own
		// errors.
		healthCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if healthErr := grpcClient.CheckHealth(healthCtx); healthErr != nil {
			slog.Warn("LLM Gateway gRPC health check failed at startup", "error", healthErr)
		} else {
			slog.Info("LLM Gateway gRPC health check succeeded")
		}
		cancel()
	}
	objectStorage := ifstorage.NewS3Storage(
		cfg.S3Endpoint, cfg.S3Region, cfg.S3Bucket, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3ForcePathStyle,
	)
	// MessageHub is the Phase 10 swap point (see CLAUDE.md's Interface Swap
	// Points table): MESSAGE_HUB_DRIVER selects InProcessHub (default), which
	// only fans out within this single process, or RedisHub, which fans out
	// via Redis Pub/Sub so multiple API server replicas share message
	// delivery. Both satisfy event.MessageHub, so nothing downstream (MsgUC,
	// the WebSocket handler) needs to change based on this branch.
	var messageHub event.MessageHub
	var redisClient *redis.Client
	switch cfg.MessageHubDriver {
	case "redis":
		opts, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			pool.Close()
			return nil, fmt.Errorf("parse REDIS_URL: %w", err)
		}
		redisClient = redis.NewClient(opts)
		messageHub = infraevent.NewRedisHub(redisClient)
	default:
		messageHub = event.NewInProcessHub()
	}
	slog.Info("message hub driver selected", "driver", cfg.MessageHubDriver)

	ticketIssuer := wsticket.NewIssuer([]byte(cfg.WSTicketSecret), wsTicketTTL)

	// Usecases
	authUC := authusecase.NewAuthUsecase(authService)
	roomUC := roomusecase.NewRoomUsecase(roomRepo)
	msgUC := msgusecase.NewMessageUsecase(msgRepo, roomRepo, llmGateway, messageHub)
	userUC := userusecase.NewUserUsecase(userRepo)
	attachmentUC := attachmentusecase.NewAttachmentUsecase(attachmentRepo, roomRepo, msgRepo, objectStorage)
	modelUC := modelusecase.NewModelUsecase(llmGateway)
	invitationUC := invitationusecase.NewInvitationUsecase(invitationRepo, roomRepo, userRepo)

	// Handlers
	healthHandler := handler.NewHealthHandler()
	authHandler := handler.NewAuthHandler(authUC)
	roomHandler := handler.NewRoomHandler(roomUC)
	msgHandler := handler.NewMessageHandler(msgUC)
	modelHandler := handler.NewModelHandler(modelUC)
	userHandler := handler.NewUserHandler(userUC)
	attachmentHandler := handler.NewAttachmentHandler(attachmentUC)
	wsHandler := handler.NewWebSocketHandler(roomUC, messageHub, ticketIssuer, originPatternsFromCORS(cfg.CORSOrigins))
	invitationHandler := handler.NewInvitationHandler(invitationUC)
	tokenHandler := handler.NewTokenHandler(llmGateway)

	return &Container{
		Config:      cfg,
		Pool:        pool,
		Logger:      slog.Default(),
		RedisClient: redisClient,

		UserRepo:       userRepo,
		RoomRepo:       roomRepo,
		MsgRepo:        msgRepo,
		AttachmentRepo: attachmentRepo,
		InvitationRepo: invitationRepo,

		AuthService:   authService,
		LLMGateway:    llmGateway,
		ObjectStorage: objectStorage,
		MessageHub:    messageHub,

		AuthUC:       authUC,
		RoomUC:       roomUC,
		MsgUC:        msgUC,
		UserUC:       userUC,
		AttachmentUC: attachmentUC,
		ModelUC:      modelUC,
		InvitationUC: invitationUC,

		HealthHandler:     healthHandler,
		AuthHandler:       authHandler,
		RoomHandler:       roomHandler,
		MessageHandler:    msgHandler,
		ModelHandler:      modelHandler,
		UserHandler:       userHandler,
		AttachmentHandler: attachmentHandler,
		WebSocketHandler:  wsHandler,
		InvitationHandler: invitationHandler,
		TokenHandler:      tokenHandler,
	}, nil
}

// originPatternsFromCORS converts the server's comma-separated
// CORS_ORIGINS configuration (full origin URLs, e.g.
// "http://localhost:3000") into the host[:port] patterns expected by
// websocket.AcceptOptions.OriginPatterns for the WebSocket handshake's
// origin check. Entries that fail to parse or have no host are skipped.
func originPatternsFromCORS(corsOrigins string) []string {
	origins := strings.Split(corsOrigins, ",")
	patterns := make([]string, 0, len(origins))
	for _, origin := range origins {
		u, err := url.Parse(strings.TrimSpace(origin))
		if err != nil || u.Host == "" {
			continue
		}
		patterns = append(patterns, u.Host)
	}
	return patterns
}
