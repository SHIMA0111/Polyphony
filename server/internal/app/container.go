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

	"github.com/go-redis/redis_rate/v10"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/ai"
	domainattachment "github.com/SHIMA0111/multi-user-ai/server/internal/domain/attachment"
	domainauth "github.com/SHIMA0111/multi-user-ai/server/internal/domain/auth"
	domainbilling "github.com/SHIMA0111/multi-user-ai/server/internal/domain/billing"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/event"
	domaingroup "github.com/SHIMA0111/multi-user-ai/server/internal/domain/group"
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
	billingusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/billing"
	groupusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/group"
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

	// RedisClient is the shared Redis client, constructed whenever
	// Config.RedisURL is non-empty regardless of which MessageHubDriver is
	// selected — Step 33's rate limiter and Kratos whoami cache both need a
	// client even when MessageHubDriver is "inprocess" (e.g. AUTH_MODE=kratos
	// with no Redis-backed MessageHub). It is nil only when RedisURL is
	// unset entirely (e.g. local `go run ./cmd/api` with neither Redis nor
	// Kratos configured), in which case RateLimiter is also nil and the
	// AUTH_MODE=kratos branch below skips wrapping AuthService in
	// CachedAuthService — both fail open to "no rate limiting"/"no caching"
	// rather than panicking on a nil client. It is kept on the Container
	// (rather than only captured in a closure) so later steps can reuse the
	// same client instead of opening a second connection pool. Callers are
	// responsible for closing it (typically via a deferred RedisClient.Close()
	// in main, guarded by a nil check).
	RedisClient *redis.Client
	// RateLimiter is the shared Redis-backed GCRA token-bucket limiter (Step
	// 33) built on RedisClient, used by middleware.RateLimit for the
	// /auth/register, /auth/login, and AI-invoke routes. It is nil whenever
	// RedisClient is nil (see RedisClient's GoDoc); routes_auth.go/
	// routes_message.go must not dereference a nil RateLimiter, but
	// middleware.RateLimit itself never runs at all in that case since the
	// route registrars only attach it when this field is non-nil.
	RateLimiter *redis_rate.Limiter

	// Repositories
	UserRepo domainuser.UserRepository
	RoomRepo domainroom.RoomRepository
	MsgRepo  domainmessage.MessageRepository
	// AttachmentRepo is the domain/attachment.AttachmentRepository backing
	// AttachmentUC's presign/link/list operations.
	AttachmentRepo domainattachment.AttachmentRepository
	InvitationRepo domaininvitation.InvitationRepository
	BillingRepo    domainbilling.BalanceRepository
	// GroupRepo backs GroupUC's cross-room group persistence.
	GroupRepo domaingroup.GroupRepository
	// SubscriptionRepo backs BillingUC's Step 49 subscription lifecycle
	// methods (GetSubscription, CancelSubscription, CreateBillingPortalSession).
	SubscriptionRepo domainbilling.SubscriptionRepository
	// PaymentRepo backs BillingUC's Step 49 payment-history and
	// checkout/webhook credit-recording methods (ListPaymentHistory,
	// HandleWebhookEvent).
	PaymentRepo domainbilling.PaymentRepository
	// ContextSummaryRepository caches and invalidates per-room AI context
	// summaries (Step 50, phases.md Phase 18); see
	// usecase/message.MessageUsecase.assembleAIContext.
	ContextSummaryRepository ai.ContextSummaryRepository

	// Services / Gateways
	AuthService domainauth.AuthService
	LLMGateway  ai.LLMGateway
	// ObjectStorage is the domain/storage.ObjectStorage adapter (backed by
	// MinIO/S3 via aws-sdk-go-v2) used to presign attachment upload/view URLs.
	ObjectStorage domainstorage.ObjectStorage
	// StripeGateway is the domain/billing.StripeGateway adapter used for
	// Checkout/Billing Portal/webhook operations (Step 49). It is nil when
	// Config.StripeSecretKey is empty — BillingUsecase's Stripe-dependent
	// methods check for this and return domain.ErrStripeNotConfigured
	// rather than the container failing to build.
	StripeGateway domainbilling.StripeGateway
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
	BillingUC    *billingusecase.BillingUsecase
	// GroupUC implements cross-room group business logic, backed by GroupRepo.
	GroupUC *groupusecase.GroupUsecase

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
	BillingHandler    *handler.BillingHandler
	// GroupHandler serves the cross-room group endpoints, delegating to GroupUC.
	GroupHandler *handler.GroupHandler
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
	billingRepo := postgres.NewBillingRepository(pool)
	groupRepo := postgres.NewGroupRepository(pool)
	subscriptionRepo := postgres.NewSubscriptionRepository(pool)
	paymentRepo := postgres.NewPaymentRepository(pool)
	forkJobRepo := postgres.NewRoomForkRepository(pool)
	contextSummaryRepo := postgres.NewContextSummaryRepository(pool)

	// RedisClient/RateLimiter: constructed whenever Config.RedisURL is
	// non-empty, independent of MessageHubDriver (see Container.RedisClient's
	// GoDoc for why Step 33's rate limiter and Kratos whoami cache need a
	// client even when MessageHubDriver is "inprocess"). MessageHubDriver's
	// own "redis" branch below reuses this exact client rather than opening a
	// second connection pool.
	var redisClient *redis.Client
	var rateLimiter *redis_rate.Limiter
	if cfg.RedisURL != "" {
		opts, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			pool.Close()
			return nil, fmt.Errorf("parse REDIS_URL: %w", err)
		}
		redisClient = redis.NewClient(opts)

		// Verify connectivity eagerly, mirroring database.NewPool's Ping check,
		// so a misconfigured/unreachable Redis fails container construction
		// immediately instead of lazily on the first message hub/rate
		// limiter/Kratos-cache operation (e.g. the first WebSocket
		// Publish/Subscribe call from a real user).
		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		pingErr := redisClient.Ping(pingCtx).Err()
		cancel()
		if pingErr != nil {
			_ = redisClient.Close()
			pool.Close()
			return nil, fmt.Errorf("ping redis: %w", pingErr)
		}

		rateLimiter = redis_rate.NewLimiter(redisClient)
	}

	// Services / Gateways
	//
	// AuthService is the Phase 9 swap point (see CLAUDE.md's Interface Swap
	// Points table): AUTH_MODE selects SimpleJWTService (default) or
	// KratosAuthService, both of which satisfy domainauth.AuthService, so no
	// downstream usecase/handler code needs to change based on this branch.
	var authService domainauth.AuthService
	switch cfg.AuthMode {
	case "kratos":
		kratosService := ifauth.NewKratosAuthService(userRepo, cfg.KratosPublicURL, cfg.KratosAdminURL, cfg.KratosCookieName,
			&http.Client{Timeout: 10 * time.Second})
		if redisClient != nil {
			// Step 33: cache ValidateToken (Kratos's real /sessions/whoami
			// round trip) behind a short-TTL Redis cache. Skipped when no
			// Redis client is configured at all, in which case AuthService
			// falls open to always calling Kratos directly (no caching,
			// same behavior as before this step).
			authService = ifauth.NewCachedAuthService(kratosService, redisClient, cfg.WhoamiCacheTTL)
		} else {
			authService = kratosService
		}
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
			if redisClient != nil {
				_ = redisClient.Close()
			}
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
	// StripeGateway is left nil when STRIPE_SECRET_KEY is unconfigured (see
	// CLAUDE.md's Token billing section and step49.md): BillingUsecase's
	// checkout/portal/cancel methods check for nil and return
	// domain.ErrStripeNotConfigured instead of the container failing to
	// build, so local development without a Stripe test account still works
	// for every other feature.
	var stripeGateway domainbilling.StripeGateway
	if cfg.StripeSecretKey != "" {
		stripeGateway = gateway.NewStripeClient(cfg.StripeSecretKey, cfg.StripeWebhookSecret)
	}
	stripePlans := make([]domainbilling.Plan, len(cfg.StripePlans))
	for i, p := range cfg.StripePlans {
		stripePlans[i] = domainbilling.Plan{
			Code:                   p.PlanCode,
			StripePriceID:          p.PriceID,
			Name:                   p.Name,
			Description:            p.Description,
			PriceCents:             p.PriceCents,
			Currency:               p.Currency,
			MonthlyTokenAllocation: p.MonthlyTokenAllocation,
		}
	}
	stripeTokenPackages := make([]domainbilling.TokenPackage, len(cfg.StripeTokenPackages))
	for i, p := range cfg.StripeTokenPackages {
		stripeTokenPackages[i] = domainbilling.TokenPackage{
			Code:          p.PackageCode,
			StripePriceID: p.PriceID,
			Name:          p.Name,
			Description:   p.Description,
			PriceCents:    p.PriceCents,
			Currency:      p.Currency,
			Tokens:        p.Tokens,
		}
	}
	// MessageHub is the Phase 10 swap point (see CLAUDE.md's Interface Swap
	// Points table): MESSAGE_HUB_DRIVER selects InProcessHub (default), which
	// only fans out within this single process, or RedisHub, which fans out
	// via Redis Pub/Sub so multiple API server replicas share message
	// delivery. Both satisfy event.MessageHub, so nothing downstream (MsgUC,
	// the WebSocket handler) needs to change based on this branch.
	var messageHub event.MessageHub
	switch cfg.MessageHubDriver {
	case "redis":
		// redisClient is guaranteed non-nil (and already Ping-verified) here:
		// config.Load requires REDIS_URL whenever MESSAGE_HUB_DRIVER=redis, so
		// the RedisClient/RateLimiter construction above already built and
		// health-checked it from the same cfg.RedisURL.
		messageHub = infraevent.NewRedisHub(redisClient)
	default:
		messageHub = event.NewInProcessHub()
	}
	slog.Info("message hub driver selected", "driver", cfg.MessageHubDriver)

	ticketIssuer := wsticket.NewIssuer([]byte(cfg.WSTicketSecret), wsTicketTTL)

	// Usecases
	authUC := authusecase.NewAuthUsecase(authService)
	roomUC := roomusecase.NewRoomUsecase(roomRepo, msgRepo, forkJobRepo)
	billingUC := billingusecase.NewBillingUsecase(
		billingRepo, roomRepo, subscriptionRepo, paymentRepo, stripeGateway,
		stripePlans, stripeTokenPackages, cfg.StripeCheckoutSuccessURL, cfg.StripeCheckoutCancelURL,
	)
	msgUC := msgusecase.NewMessageUsecase(msgRepo, roomRepo, llmGateway, messageHub, billingUC, attachmentRepo, objectStorage, contextSummaryRepo, cfg.DefaultAIModel)
	userUC := userusecase.NewUserUsecase(userRepo)
	attachmentUC := attachmentusecase.NewAttachmentUsecase(attachmentRepo, roomRepo, msgRepo, objectStorage)
	modelUC := modelusecase.NewModelUsecase(llmGateway)
	invitationUC := invitationusecase.NewInvitationUsecase(invitationRepo, roomRepo, userRepo)
	groupUC := groupusecase.NewGroupUsecase(groupRepo, userRepo, invitationUC)

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
	billingHandler := handler.NewBillingHandler(billingUC)
	groupHandler := handler.NewGroupHandler(groupUC)

	return &Container{
		Config:      cfg,
		Pool:        pool,
		Logger:      slog.Default(),
		RedisClient: redisClient,
		RateLimiter: rateLimiter,

		UserRepo:         userRepo,
		RoomRepo:         roomRepo,
		MsgRepo:          msgRepo,
		AttachmentRepo:   attachmentRepo,
		InvitationRepo:   invitationRepo,
		BillingRepo:      billingRepo,
		GroupRepo:        groupRepo,
		SubscriptionRepo: subscriptionRepo,
		PaymentRepo:      paymentRepo,

		ContextSummaryRepository: contextSummaryRepo,

		AuthService:   authService,
		LLMGateway:    llmGateway,
		ObjectStorage: objectStorage,
		MessageHub:    messageHub,
		StripeGateway: stripeGateway,

		AuthUC:       authUC,
		RoomUC:       roomUC,
		MsgUC:        msgUC,
		UserUC:       userUC,
		AttachmentUC: attachmentUC,
		ModelUC:      modelUC,
		InvitationUC: invitationUC,
		BillingUC:    billingUC,
		GroupUC:      groupUC,

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
		BillingHandler:    billingHandler,
		GroupHandler:      groupHandler,
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
