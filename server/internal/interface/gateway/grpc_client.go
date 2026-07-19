package gateway

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/ai"
	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/gateway/grpc/llmgatewaypb"
)

// maxBackoff caps the exponential backoff delay applied between retry
// attempts in GRPCClient.callWithRetry, regardless of how large maxRetries
// or baseBackoff are configured to be.
const maxBackoff = 2 * time.Second

// retryableCodes is the set of gRPC status codes GRPCClient.callWithRetry
// treats as transient and worth retrying. Any other code (including
// codes.OK, which never reaches this check) is returned to the caller
// immediately without a retry.
var retryableCodes = map[codes.Code]bool{
	codes.Unavailable:       true,
	codes.ResourceExhausted: true,
	codes.DeadlineExceeded:  true,
}

// GRPCClient implements ai.LLMGateway using gRPC calls to the LLM Gateway
// service, per Phase 8 of phases.md and CLAUDE.md's Interface Swap Points
// table (LLMClient: REST initially, gRPC in Phase 8). It is selected instead
// of the REST LLMClient when config.Config.LLMGatewayTransport is "grpc"
// (see app.NewContainer).
//
// GRPCClient adds exponential-backoff retry on transient gRPC status codes
// (see callWithRetry) that the REST LLMClient does not attempt, since gRPC
// surfaces transient failures as typed status codes the client can
// distinguish from permanent ones (e.g. codes.InvalidArgument).
type GRPCClient struct {
	conn             *grpc.ClientConn
	completionClient llmgatewaypb.CompletionServiceClient
	modelsClient     llmgatewaypb.ModelsServiceClient
	healthClient     grpc_health_v1.HealthClient

	// maxRetries is the maximum number of attempts (including the first)
	// callWithRetry will make for a single logical call before giving up.
	maxRetries int
	// baseBackoff is the initial delay used by the exponential backoff
	// schedule in callWithRetry (baseBackoff * 2^attempt, capped at
	// maxBackoff, plus jitter).
	baseBackoff time.Duration
}

// NewGRPCClient dials the LLM Gateway at target (e.g. "llm-gateway:50051")
// over plaintext gRPC -- matching the REST LLMClient's unauthenticated
// intra-Docker-network call today -- and constructs the CompletionService,
// ModelsService, and standard health-check clients used by GRPCClient's
// methods.
//
// It uses grpc.NewClient, which does not block on connection establishment:
// any dial-target parsing error is returned here, but connectivity/health
// failures only surface on the first RPC call (Complete, ListModels, or an
// explicit checkHealth), not at construction time.
func NewGRPCClient(target string, maxRetries int, baseBackoff time.Duration) (*GRPCClient, error) {
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("%w: dial %s: %v", domain.ErrLLMGateway, target, err)
	}

	return &GRPCClient{
		conn:             conn,
		completionClient: llmgatewaypb.NewCompletionServiceClient(conn),
		modelsClient:     llmgatewaypb.NewModelsServiceClient(conn),
		healthClient:     grpc_health_v1.NewHealthClient(conn),
		maxRetries:       maxRetries,
		baseBackoff:      baseBackoff,
	}, nil
}

// Close releases the underlying gRPC connection. Callers should invoke this
// during graceful shutdown, mirroring how Container's *pgxpool.Pool is
// closed in cmd/api/main.go.
func (c *GRPCClient) Close() error {
	return c.conn.Close()
}

// Complete sends a chat completion request to the LLM Gateway over gRPC and
// returns the response. It returns a domain.ErrLLMGateway-wrapped error if
// the request fails, and domain.ErrInvalidMaxTokens if req.MaxTokens is
// negative or exceeds math.MaxUint32 (the proto field is a uint32).
//
// Unlike ListModels and EstimateTokens, Complete deliberately does NOT use
// callWithRetry. Complete is not idempotent from the caller's perspective:
// the upstream provider call it triggers may have already been billed and
// partially or fully completed by the time a transient gRPC error (e.g.
// codes.Unavailable) is observed mid-RPC, so blindly retrying risks
// double-invoking (and double-billing) the same completion. Callers that
// want retry semantics for Complete must implement them above this layer
// with idempotency safeguards (e.g. request deduplication), not here.
func (c *GRPCClient) Complete(ctx context.Context, req *ai.CompletionRequest) (*ai.CompletionResponse, error) {
	pbReq := &llmgatewaypb.CompletionRequest{
		Model:    req.Model,
		Messages: toPBChatMessages(req.Messages),
	}
	if req.Temperature != nil {
		temp := float32(*req.Temperature)
		pbReq.Temperature = &temp
	}
	if req.MaxTokens != nil {
		if *req.MaxTokens < 0 || *req.MaxTokens > math.MaxUint32 {
			return nil, fmt.Errorf("%w: max_tokens %d out of range", domain.ErrInvalidMaxTokens, *req.MaxTokens)
		}
		maxTokens := uint32(*req.MaxTokens) // #nosec G115 -- validated non-negative and within uint32 range above
		pbReq.MaxTokens = &maxTokens
	}

	resp, err := c.completionClient.Complete(ctx, pbReq)
	if err != nil {
		return nil, fmt.Errorf("%w: complete: %v", domain.ErrLLMGateway, err)
	}

	content := ""
	if len(resp.GetChoices()) > 0 {
		content = pbContentToText(resp.GetChoices()[0].GetMessage())
	}

	return &ai.CompletionResponse{
		Content:      content,
		Model:        resp.GetModel(),
		PromptTokens: int(resp.GetUsage().GetPromptTokens()),
		OutputTokens: int(resp.GetUsage().GetCompletionTokens()),
	}, nil
}

// ListModels retrieves the list of available models from the LLM Gateway
// over gRPC, retrying transient failures per callWithRetry. It returns a
// domain.ErrLLMGateway-wrapped error if the request ultimately fails.
func (c *GRPCClient) ListModels(ctx context.Context) ([]ai.ModelInfo, error) {
	var resp *llmgatewaypb.ListModelsResponse
	err := c.callWithRetry(ctx, func(ctx context.Context) error {
		var callErr error
		resp, callErr = c.modelsClient.ListModels(ctx, &llmgatewaypb.ListModelsRequest{})
		return callErr
	})
	if err != nil {
		return nil, fmt.Errorf("%w: list models: %v", domain.ErrLLMGateway, err)
	}

	models := make([]ai.ModelInfo, len(resp.GetModels()))
	for i, m := range resp.GetModels() {
		info := ai.ModelInfo{
			ID:       m.GetId(),
			Name:     m.GetName(),
			Provider: m.GetProvider(),
		}
		if m.ContextWindow != nil {
			info.ContextWindow = int(m.GetContextWindow())
		}
		if m.SupportsImageInput != nil {
			info.SupportsImageInput = m.GetSupportsImageInput()
		}
		if pricing := m.GetPricing(); pricing != nil {
			info.InputPricePerMillionTokens = pricing.GetInputPricePerMillionTokens()
			info.OutputPricePerMillionTokens = pricing.GetOutputPricePerMillionTokens()
		}
		models[i] = info
	}
	return models, nil
}

// EstimateTokens sends a token estimation request to the LLM Gateway over
// gRPC and returns the approximate token count, retrying transient failures
// per callWithRetry. It returns a domain.ErrLLMGateway-wrapped error if the
// request ultimately fails.
func (c *GRPCClient) EstimateTokens(ctx context.Context, req *ai.TokenEstimateRequest) (*ai.TokenEstimateResponse, error) {
	pbReq := &llmgatewaypb.TokenEstimateRequest{
		Model:    req.Model,
		Messages: toPBChatMessages(req.Messages),
	}

	var resp *llmgatewaypb.TokenEstimateResponse
	err := c.callWithRetry(ctx, func(ctx context.Context) error {
		var callErr error
		resp, callErr = c.completionClient.EstimateTokens(ctx, pbReq)
		return callErr
	})
	if err != nil {
		return nil, fmt.Errorf("%w: estimate tokens: %v", domain.ErrLLMGateway, err)
	}

	return &ai.TokenEstimateResponse{
		Model:           resp.GetModel(),
		EstimatedTokens: int(resp.GetEstimatedTokens()),
	}, nil
}

// Stream is not implemented for the gRPC transport: gRPC streaming between
// the Go server and the LLM Gateway is out of scope for the step that
// introduced ai.LLMGateway.Stream (Step 51) -- streaming is available only
// through the REST LLMClient's Stream, which consumes the gateway's
// `POST /completions/stream` SSE endpoint. Calling Stream on a GRPCClient
// (selected via config.Config.LLMGatewayTransport == "grpc") always returns a
// synchronous error wrapping both domain.ErrLLMGateway and
// domain.ErrStreamingUnsupported, and a nil channel -- exactly the same
// "synchronous dispatch failure" shape Complete/ListModels/EstimateTokens use
// on failure -- rather than panicking.
//
// Post-review fix: this used to silently break every streaming send when
// LLM_GATEWAY_TRANSPORT=grpc, since a synchronous Stream failure would
// otherwise just mark the placeholder failed. Callers should check
// errors.Is(err, domain.ErrStreamingUnsupported) specifically and fall back
// to the unary Complete call instead of failing outright --
// usecase/message.MessageUsecase.SendAIMessageStream does exactly that.
func (c *GRPCClient) Stream(_ context.Context, _ *ai.CompletionRequest) (<-chan ai.StreamResult, error) {
	return nil, fmt.Errorf("%w: %w: streaming not supported over grpc transport", domain.ErrLLMGateway, domain.ErrStreamingUnsupported)
}

// checkHealth calls the standard grpc.health.v1.Health service with an empty
// Service field (checking overall server health per the standard protocol,
// rather than a specific service name) and returns an error if the gateway
// does not report SERVING.
//
// This is intended to be called once during app.NewContainer construction
// (to fail fast/log a warning if the gRPC transport is selected but the
// gateway isn't reachable yet), not before every Complete/ListModels call --
// gating every call on a health check would double request latency.
func (c *GRPCClient) checkHealth(ctx context.Context) error {
	resp, err := c.healthClient.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		return fmt.Errorf("%w: health check: %v", domain.ErrLLMGateway, err)
	}
	if resp.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		return fmt.Errorf("%w: health check: status %s", domain.ErrLLMGateway, resp.GetStatus())
	}
	return nil
}

// CheckHealth is the exported entry point for the startup gRPC health check
// described on checkHealth, called once from app.NewContainer.
func (c *GRPCClient) CheckHealth(ctx context.Context) error {
	return c.checkHealth(ctx)
}

// callWithRetry invokes fn up to c.maxRetries times, retrying only when fn
// returns an error whose gRPC status code is in retryableCodes (transient
// failures: Unavailable, ResourceExhausted, DeadlineExceeded). Any other
// error is returned immediately without a retry. Between attempts it waits
// an exponentially increasing backoff (c.baseBackoff * 2^attempt, capped at
// maxBackoff, with up to 20% jitter added to avoid thundering-herd retries),
// aborting early if ctx is done. If all attempts are exhausted, the error
// from the final attempt is returned.
func (c *GRPCClient) callWithRetry(ctx context.Context, fn func(context.Context) error) error {
	maxRetries := c.maxRetries
	if maxRetries < 1 {
		maxRetries = 1
	}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		lastErr = fn(ctx)
		if lastErr == nil {
			return nil
		}
		if !retryableCodes[status.Code(lastErr)] {
			return lastErr
		}
		if attempt == maxRetries-1 {
			break
		}

		delay := backoffDelay(c.baseBackoff, attempt)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return lastErr
}

// backoffDelay computes the exponential backoff delay for the given attempt
// (0-indexed): base * 2^attempt, capped at maxBackoff, plus up to 20% jitter.
// The shift amount is capped to avoid overflow when maxRetries is configured
// unusually high.
func backoffDelay(base time.Duration, attempt int) time.Duration {
	if attempt > 30 {
		attempt = 30
	}
	delay := base << attempt
	if delay <= 0 || delay > maxBackoff {
		delay = maxBackoff
	}
	jitter := time.Duration(rand.Int63n(int64(delay)/5 + 1)) // #nosec G404 -- non-cryptographic jitter is fine
	return delay + jitter
}

// toPBChatMessages converts domain ai.ChatMessage values to their gRPC
// message counterparts via toPBChatMessage.
func toPBChatMessages(messages []ai.ChatMessage) []*llmgatewaypb.ChatMessage {
	out := make([]*llmgatewaypb.ChatMessage, len(messages))
	for i, m := range messages {
		out[i] = toPBChatMessage(m)
	}
	return out
}

// toPBChatMessage converts a single domain ai.ChatMessage to its gRPC
// counterpart: Role maps to the matching ChatRole enum value (an
// unrecognized role maps to CHAT_ROLE_UNSPECIFIED), and content maps to the
// wire `content` oneof. A non-empty Parts takes precedence over Content
// (mirroring how the REST LLMClient's chatMsgDTO.Content serializes the same
// precedence): when Parts is set, the message becomes a
// llmgatewaypb.ChatMessage_Parts; otherwise it becomes a plain
// llmgatewaypb.ChatMessage_Text carrying Content (the common,
// backward-compatible text-only case, including the empty string).
//
// The oneof's field type (llmgatewaypb.isChatMessage_Content) is unexported,
// so this conversion must build the whole *llmgatewaypb.ChatMessage in one
// function rather than returning the oneof value on its own.
func toPBChatMessage(m ai.ChatMessage) *llmgatewaypb.ChatMessage {
	pbMsg := &llmgatewaypb.ChatMessage{Role: toPBChatRole(m.Role)}
	if len(m.Parts) > 0 {
		pbMsg.Content = &llmgatewaypb.ChatMessage_Parts{Parts: toPBContentParts(m.Parts)}
	} else {
		pbMsg.Content = &llmgatewaypb.ChatMessage_Text{Text: m.Content}
	}
	return pbMsg
}

// toPBContentParts converts domain ai.ContentPart values to the wire-format
// llmgatewaypb.ContentParts message.
func toPBContentParts(parts []ai.ContentPart) *llmgatewaypb.ContentParts {
	out := make([]*llmgatewaypb.ContentPart, len(parts))
	for i, p := range parts {
		out[i] = toPBContentPart(p)
	}
	return &llmgatewaypb.ContentParts{Parts: out}
}

// toPBContentPart converts a single domain ai.ContentPart to the wire-format
// llmgatewaypb.ContentPart oneof, dispatching on p.Type (one of the
// ai.ContentPartType* constants). An unrecognized Type falls back to a text
// part carrying p.Text (defaulting to the empty string), the same lenient
// fallback style used elsewhere in this client for unrecognized wire enums
// (see toPBChatRole).
func toPBContentPart(p ai.ContentPart) *llmgatewaypb.ContentPart {
	switch p.Type {
	case ai.ContentPartTypeImageURL:
		return &llmgatewaypb.ContentPart{
			Part: &llmgatewaypb.ContentPart_ImageUrl{ImageUrl: p.ImageURL},
		}
	case ai.ContentPartTypeImageBase64:
		var img *llmgatewaypb.ImageBase64Data
		if p.ImageBase64 != nil {
			img = &llmgatewaypb.ImageBase64Data{
				MediaType: p.ImageBase64.MediaType,
				Data:      p.ImageBase64.Data,
			}
		}
		return &llmgatewaypb.ContentPart{
			Part: &llmgatewaypb.ContentPart_ImageBase64{ImageBase64: img},
		}
	default:
		return &llmgatewaypb.ContentPart{
			Part: &llmgatewaypb.ContentPart_Text{Text: p.Text},
		}
	}
}

// pbContentToText extracts the plain-text representation of a gRPC
// ChatMessage's `content` oneof, mirroring the LLM Gateway domain's
// MessageContent::as_text(): a ChatMessage_Text yields its text directly; a
// ChatMessage_Parts concatenates only its text parts (image parts contribute
// nothing). This client only ever needs plain text (ai.CompletionResponse.Content
// is a plain string), so a multimodal response's non-text parts are
// intentionally dropped here rather than represented further.
func pbContentToText(msg *llmgatewaypb.ChatMessage) string {
	switch c := msg.GetContent().(type) {
	case *llmgatewaypb.ChatMessage_Text:
		return c.Text
	case *llmgatewaypb.ChatMessage_Parts:
		var sb strings.Builder
		for _, part := range c.Parts.GetParts() {
			if t, ok := part.GetPart().(*llmgatewaypb.ContentPart_Text); ok {
				sb.WriteString(t.Text)
			}
		}
		return sb.String()
	default:
		return ""
	}
}

// toPBChatRole maps a domain role string ("system", "user", "assistant",
// "tool") to the matching llmgatewaypb.ChatRole enum value.
func toPBChatRole(role string) llmgatewaypb.ChatRole {
	switch role {
	case "system":
		return llmgatewaypb.ChatRole_CHAT_ROLE_SYSTEM
	case "user":
		return llmgatewaypb.ChatRole_CHAT_ROLE_USER
	case "assistant":
		return llmgatewaypb.ChatRole_CHAT_ROLE_ASSISTANT
	case "tool":
		return llmgatewaypb.ChatRole_CHAT_ROLE_TOOL
	default:
		return llmgatewaypb.ChatRole_CHAT_ROLE_UNSPECIFIED
	}
}
