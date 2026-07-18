package gateway

import (
	"context"
	"errors"
	"math"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/ai"
	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/gateway/grpc/llmgatewaypb"
)

// bufconnSize is the in-memory buffer size for the bufconn.Listener used to
// serve the fake gRPC server in these tests.
const bufconnSize = 1024 * 1024

// fakeCompletionServer is a controllable llmgatewaypb.CompletionServiceServer
// used to exercise GRPCClient's retry and DTO-mapping behavior without a
// real network connection.
type fakeCompletionServer struct {
	llmgatewaypb.UnimplementedCompletionServiceServer

	mu           sync.Mutex
	calls        int
	failTimes    int
	failCode     codes.Code
	resp         *llmgatewaypb.CompletionResponse
	estimateResp *llmgatewaypb.TokenEstimateResponse
}

func (s *fakeCompletionServer) Complete(_ context.Context, _ *llmgatewaypb.CompletionRequest) (*llmgatewaypb.CompletionResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.calls <= s.failTimes {
		return nil, status.Error(s.failCode, "injected failure")
	}
	return s.resp, nil
}

// EstimateTokens returns the configured estimateResp, ignoring the
// failTimes/failCode retry-injection fields Complete uses (no test currently
// needs EstimateTokens retry coverage).
func (s *fakeCompletionServer) EstimateTokens(_ context.Context, _ *llmgatewaypb.TokenEstimateRequest) (*llmgatewaypb.TokenEstimateResponse, error) {
	return s.estimateResp, nil
}

func (s *fakeCompletionServer) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// fakeModelsServer is a controllable llmgatewaypb.ModelsServiceServer.
type fakeModelsServer struct {
	llmgatewaypb.UnimplementedModelsServiceServer

	mu        sync.Mutex
	calls     int
	failTimes int
	failCode  codes.Code
	resp      *llmgatewaypb.ListModelsResponse
}

func (s *fakeModelsServer) ListModels(_ context.Context, _ *llmgatewaypb.ListModelsRequest) (*llmgatewaypb.ListModelsResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.calls <= s.failTimes {
		return nil, status.Error(s.failCode, "injected failure")
	}
	return s.resp, nil
}

func (s *fakeModelsServer) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// fakeHealthServer is a controllable grpc_health_v1.HealthServer that always
// reports the configured status for the overall-server Check call.
type fakeHealthServer struct {
	grpc_health_v1.UnimplementedHealthServer

	status grpc_health_v1.HealthCheckResponse_ServingStatus
}

func (s *fakeHealthServer) Check(_ context.Context, _ *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	return &grpc_health_v1.HealthCheckResponse{Status: s.status}, nil
}

// testGRPCFixture bundles a fake in-memory gRPC server (via bufconn) with a
// *GRPCClient wired to talk to it, plus a cleanup func to be deferred by the
// caller.
type testGRPCFixture struct {
	client     *GRPCClient
	completion *fakeCompletionServer
	models     *fakeModelsServer
	health     *fakeHealthServer
}

// newTestGRPCFixture starts an in-memory gRPC server backed by bufconn,
// registers the given fakes, and returns a *GRPCClient dialed against it with
// the given retry configuration. The returned stop func shuts down the
// server and client connection; callers should defer it.
func newTestGRPCFixture(t *testing.T, maxRetries int, baseBackoff time.Duration) (*testGRPCFixture, func()) {
	t.Helper()

	listener := bufconn.Listen(bufconnSize)
	server := grpc.NewServer()

	completion := &fakeCompletionServer{failCode: codes.Unavailable}
	models := &fakeModelsServer{}
	health := &fakeHealthServer{status: grpc_health_v1.HealthCheckResponse_SERVING}

	llmgatewaypb.RegisterCompletionServiceServer(server, completion)
	llmgatewaypb.RegisterModelsServiceServer(server, models)
	grpc_health_v1.RegisterHealthServer(server, health)

	var serveErr atomic.Value
	go func() {
		if err := server.Serve(listener); err != nil {
			serveErr.Store(err)
		}
	}()

	dialer := func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial bufconn: %v", err)
	}

	client := &GRPCClient{
		conn:             conn,
		completionClient: llmgatewaypb.NewCompletionServiceClient(conn),
		modelsClient:     llmgatewaypb.NewModelsServiceClient(conn),
		healthClient:     grpc_health_v1.NewHealthClient(conn),
		maxRetries:       maxRetries,
		baseBackoff:      baseBackoff,
	}

	fixture := &testGRPCFixture{client: client, completion: completion, models: models, health: health}
	stop := func() {
		_ = conn.Close()
		server.Stop()
		_ = listener.Close()
	}
	return fixture, stop
}

func TestGRPCClientCompleteHappyPath(t *testing.T) {
	fixture, stop := newTestGRPCFixture(t, 3, time.Millisecond)
	defer stop()

	fixture.completion.resp = &llmgatewaypb.CompletionResponse{
		Id:    "cmpl-1",
		Model: "gpt-5.2",
		Choices: []*llmgatewaypb.Choice{
			{
				Index: 0,
				Message: &llmgatewaypb.ChatMessage{
					Role:    llmgatewaypb.ChatRole_CHAT_ROLE_ASSISTANT,
					Content: &llmgatewaypb.ChatMessage_Text{Text: "hello there"},
				},
				FinishReason: "stop",
			},
		},
		Usage: &llmgatewaypb.Usage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}

	temp := 0.7
	maxTokens := 256
	req := &ai.CompletionRequest{
		Model: "gpt-5.2",
		Messages: []ai.ChatMessage{
			{Role: "system", Content: "be nice"},
			{Role: "user", Content: "hi"},
		},
		Temperature: &temp,
		MaxTokens:   &maxTokens,
	}

	resp, err := fixture.client.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	if resp.Content != "hello there" {
		t.Errorf("expected content %q, got %q", "hello there", resp.Content)
	}
	if resp.Model != "gpt-5.2" {
		t.Errorf("expected model %q, got %q", "gpt-5.2", resp.Model)
	}
	if resp.PromptTokens != 10 {
		t.Errorf("expected PromptTokens 10, got %d", resp.PromptTokens)
	}
	if resp.OutputTokens != 5 {
		t.Errorf("expected OutputTokens 5, got %d", resp.OutputTokens)
	}
	if fixture.completion.callCount() != 1 {
		t.Errorf("expected exactly 1 invocation, got %d", fixture.completion.callCount())
	}
}

// TestGRPCClientCompleteRejectsOutOfRangeMaxTokens asserts that a negative
// or too-large MaxTokens value is rejected with a domain.ErrInvalidMaxTokens
// error before it would be cast to the gRPC wire type (uint32), and that no
// RPC is made.
func TestGRPCClientCompleteRejectsOutOfRangeMaxTokens(t *testing.T) {
	fixture, stop := newTestGRPCFixture(t, 3, time.Millisecond)
	defer stop()

	for name, maxTokens := range map[string]int{
		"negative":       -1,
		"aboveUint32Max": math.MaxUint32 + 1,
	} {
		t.Run(name, func(t *testing.T) {
			req := &ai.CompletionRequest{
				Model:     "gpt-5.2",
				Messages:  []ai.ChatMessage{{Role: "user", Content: "hi"}},
				MaxTokens: &maxTokens,
			}
			_, err := fixture.client.Complete(context.Background(), req)
			if err == nil {
				t.Fatal("expected an error for an out-of-range max_tokens value")
			}
			if !errors.Is(err, domain.ErrInvalidMaxTokens) {
				t.Errorf("expected domain.ErrInvalidMaxTokens, got: %v", err)
			}
		})
	}
	if got := fixture.completion.callCount(); got != 0 {
		t.Errorf("expected no RPC invocations for rejected requests, got %d", got)
	}
}

func TestGRPCClientListModelsHappyPath(t *testing.T) {
	fixture, stop := newTestGRPCFixture(t, 3, time.Millisecond)
	defer stop()

	contextWindow := uint32(272_000)
	supportsImageInput := true
	fixture.models.resp = &llmgatewaypb.ListModelsResponse{
		Models: []*llmgatewaypb.ModelInfo{
			{
				Id:                 "gpt-5.2",
				Name:               "GPT-5.2",
				Provider:           "openai",
				ContextWindow:      &contextWindow,
				SupportsImageInput: &supportsImageInput,
				Pricing: &llmgatewaypb.ModelPricing{
					InputPricePerMillionTokens:  2.5,
					OutputPricePerMillionTokens: 10.0,
					Currency:                    "USD",
				},
			},
			// No metadata set -- exercises the nil-pointer-to-zero-value path.
			{Id: "claude-opus", Name: "Claude Opus", Provider: "anthropic"},
		},
	}

	models, err := fixture.client.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels failed: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	want0 := ai.ModelInfo{
		ID:                          "gpt-5.2",
		Name:                        "GPT-5.2",
		Provider:                    "openai",
		ContextWindow:               272_000,
		InputPricePerMillionTokens:  2.5,
		OutputPricePerMillionTokens: 10.0,
		SupportsImageInput:          true,
	}
	if models[0] != want0 {
		t.Errorf("unexpected model[0]: got %+v, want %+v", models[0], want0)
	}
	want1 := ai.ModelInfo{ID: "claude-opus", Name: "Claude Opus", Provider: "anthropic"}
	if models[1] != want1 {
		t.Errorf("unexpected model[1]: got %+v, want %+v", models[1], want1)
	}
}

// TestGRPCClientEstimateTokensHappyPath exercises GRPCClient.EstimateTokens
// (Step 34's carryover gRPC support, previously an explicit "not supported"
// stub) against the fake CompletionService server.
func TestGRPCClientEstimateTokensHappyPath(t *testing.T) {
	fixture, stop := newTestGRPCFixture(t, 3, time.Millisecond)
	defer stop()

	fixture.completion.estimateResp = &llmgatewaypb.TokenEstimateResponse{
		Model:           "gpt-5.2",
		EstimatedTokens: 42,
	}

	req := &ai.TokenEstimateRequest{
		Model:    "gpt-5.2",
		Messages: []ai.ChatMessage{{Role: "user", Content: "hello"}},
	}
	resp, err := fixture.client.EstimateTokens(context.Background(), req)
	if err != nil {
		t.Fatalf("EstimateTokens failed: %v", err)
	}
	if resp.Model != "gpt-5.2" {
		t.Errorf("expected model %q, got %q", "gpt-5.2", resp.Model)
	}
	if resp.EstimatedTokens != 42 {
		t.Errorf("expected EstimatedTokens 42, got %d", resp.EstimatedTokens)
	}
}

// TestGRPCClientCompleteDoesNotRetryOnUnavailable asserts that Complete
// makes exactly one attempt even for a transient/retryable gRPC status code.
// Complete is intentionally excluded from callWithRetry because a retried
// mid-RPC failure could double-invoke (and double-bill) an upstream
// completion that already succeeded from the provider's perspective; see the
// GoDoc on GRPCClient.Complete.
func TestGRPCClientCompleteDoesNotRetryOnUnavailable(t *testing.T) {
	fixture, stop := newTestGRPCFixture(t, 3, time.Millisecond)
	defer stop()

	fixture.completion.failTimes = 100 // would eventually succeed if retried
	fixture.completion.failCode = codes.Unavailable
	fixture.completion.resp = &llmgatewaypb.CompletionResponse{
		Model: "gpt-5.2",
		Choices: []*llmgatewaypb.Choice{
			{Message: &llmgatewaypb.ChatMessage{Content: &llmgatewaypb.ChatMessage_Text{Text: "recovered"}}},
		},
		Usage: &llmgatewaypb.Usage{},
	}

	req := &ai.CompletionRequest{Model: "gpt-5.2", Messages: []ai.ChatMessage{{Role: "user", Content: "hi"}}}
	_, err := fixture.client.Complete(context.Background(), req)
	if err == nil {
		t.Fatal("expected an error since Complete must not retry a transient failure")
	}
	if !domain.IsLLMGatewayError(err) {
		t.Errorf("expected domain.ErrLLMGateway-wrapped error, got: %v", err)
	}
	if got := fixture.completion.callCount(); got != 1 {
		t.Errorf("expected exactly 1 invocation (no retry on Complete), got %d", got)
	}
}

// TestGRPCClientListModelsRetriesOnUnavailableThenSucceeds asserts that the
// read-only ListModels RPC still goes through callWithRetry (unlike
// Complete, it is safe/idempotent to retry).
func TestGRPCClientListModelsRetriesOnUnavailableThenSucceeds(t *testing.T) {
	fixture, stop := newTestGRPCFixture(t, 3, time.Millisecond)
	defer stop()

	fixture.models.failTimes = 2
	fixture.models.failCode = codes.Unavailable
	fixture.models.resp = &llmgatewaypb.ListModelsResponse{
		Models: []*llmgatewaypb.ModelInfo{
			{Id: "gpt-5.2", Name: "GPT-5.2", Provider: "openai"},
		},
	}

	models, err := fixture.client.ListModels(context.Background())
	if err != nil {
		t.Fatalf("expected success after retries, got error: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}
	if got := fixture.models.callCount(); got != 3 {
		t.Errorf("expected exactly 3 invocations (2 failures + 1 success), got %d", got)
	}
}

// TestGRPCClientCompleteNonRetryableCodeReturnsImmediately asserts that
// Complete returns after a single invocation, without retrying, when the
// gRPC failure code is not in the retryable set (e.g. InvalidArgument).
func TestGRPCClientCompleteNonRetryableCodeReturnsImmediately(t *testing.T) {
	fixture, stop := newTestGRPCFixture(t, 3, time.Millisecond)
	defer stop()

	fixture.completion.failTimes = 100 // would fail every call if retried
	fixture.completion.failCode = codes.InvalidArgument

	req := &ai.CompletionRequest{Model: "gpt-5.2", Messages: []ai.ChatMessage{{Role: "user", Content: "hi"}}}
	_, err := fixture.client.Complete(context.Background(), req)
	if err == nil {
		t.Fatal("expected an error for a non-retryable code")
	}
	if !domain.IsLLMGatewayError(err) {
		t.Errorf("expected domain.ErrLLMGateway-wrapped error, got: %v", err)
	}
	if got := fixture.completion.callCount(); got != 1 {
		t.Errorf("expected exactly 1 invocation for a non-retryable code, got %d", got)
	}
}

func TestGRPCClientCheckHealthServing(t *testing.T) {
	fixture, stop := newTestGRPCFixture(t, 3, time.Millisecond)
	defer stop()

	fixture.health.status = grpc_health_v1.HealthCheckResponse_SERVING
	if err := fixture.client.checkHealth(context.Background()); err != nil {
		t.Errorf("expected nil error for SERVING status, got: %v", err)
	}
}

func TestGRPCClientCheckHealthNotServing(t *testing.T) {
	fixture, stop := newTestGRPCFixture(t, 3, time.Millisecond)
	defer stop()

	fixture.health.status = grpc_health_v1.HealthCheckResponse_NOT_SERVING
	if err := fixture.client.checkHealth(context.Background()); err == nil {
		t.Error("expected a non-nil error for NOT_SERVING status")
	}
}
