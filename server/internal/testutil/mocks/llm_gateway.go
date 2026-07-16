package mocks

import (
	"context"
	"fmt"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/ai"
)

// LLMGateway is a configurable fake implementing ai.LLMGateway.
//
// By default, Complete returns a canned successful completion and
// ListModels returns an empty slice. Setting ShouldErr makes both Complete
// and EstimateTokens return a domain.ErrLLMGateway-wrapped error, as if the
// LLM Gateway call failed. For full control over the response, set
// CompleteFunc / ListModelsFunc / EstimateTokensFunc, which take priority
// over ShouldErr / CompletionResponse / Models / TokenEstimateResponse.
//
// The zero value (mocks.LLMGateway{}) is ready to use.
type LLMGateway struct {
	// ShouldErr, if true, makes both Complete and EstimateTokens return a
	// domain.ErrLLMGateway-wrapped error instead of their canned responses.
	// It only governs each method's default behavior: it has no effect once
	// that method's own *Func override (CompleteFunc / EstimateTokensFunc) is
	// set, since the override is checked first and returns unconditionally.
	ShouldErr bool
	// CompletionResponse, if non-nil, overrides the default canned
	// completion response returned by Complete.
	CompletionResponse *ai.CompletionResponse
	// Models, if non-nil, overrides the default (empty) ListModels result.
	Models []ai.ModelInfo
	// TokenEstimateResponse, if non-nil, overrides the default canned
	// response returned by EstimateTokens.
	TokenEstimateResponse *ai.TokenEstimateResponse

	// CompleteFunc, if set, overrides Complete entirely.
	CompleteFunc func(ctx context.Context, req *ai.CompletionRequest) (*ai.CompletionResponse, error)
	// ListModelsFunc, if set, overrides ListModels entirely.
	ListModelsFunc func(ctx context.Context) ([]ai.ModelInfo, error)
	// EstimateTokensFunc, if set, overrides EstimateTokens entirely.
	EstimateTokensFunc func(ctx context.Context, req *ai.TokenEstimateRequest) (*ai.TokenEstimateResponse, error)

	// CompleteCallCount counts every Complete invocation (regardless of
	// which of CompleteFunc/ShouldErr/CompletionResponse served it), so
	// tests can assert exactly how many completion calls a usecase method
	// made -- e.g. that context summarization issues exactly one extra
	// Complete call beyond the final answer-generating one.
	CompleteCallCount int
}

// Complete sends a completion request and returns the response. See the
// LLMGateway doc comment for how ShouldErr, CompletionResponse, and
// CompleteFunc interact.
func (g *LLMGateway) Complete(ctx context.Context, req *ai.CompletionRequest) (*ai.CompletionResponse, error) {
	g.CompleteCallCount++
	if g.CompleteFunc != nil {
		return g.CompleteFunc(ctx, req)
	}
	if g.ShouldErr {
		return nil, fmt.Errorf("%w: mock error", domain.ErrLLMGateway)
	}
	if g.CompletionResponse != nil {
		return g.CompletionResponse, nil
	}
	return &ai.CompletionResponse{
		Content:      "AI response",
		Model:        "test-model",
		PromptTokens: 10,
		OutputTokens: 5,
	}, nil
}

// ListModels returns the available LLM models. See the LLMGateway doc
// comment for how Models and ListModelsFunc interact.
func (g *LLMGateway) ListModels(ctx context.Context) ([]ai.ModelInfo, error) {
	if g.ListModelsFunc != nil {
		return g.ListModelsFunc(ctx)
	}
	return g.Models, nil
}

// EstimateTokens returns an approximate token count for the given messages.
// See the LLMGateway doc comment for how ShouldErr, TokenEstimateResponse,
// and EstimateTokensFunc interact.
func (g *LLMGateway) EstimateTokens(ctx context.Context, req *ai.TokenEstimateRequest) (*ai.TokenEstimateResponse, error) {
	if g.EstimateTokensFunc != nil {
		return g.EstimateTokensFunc(ctx, req)
	}
	if g.ShouldErr {
		return nil, fmt.Errorf("%w: mock error", domain.ErrLLMGateway)
	}
	if g.TokenEstimateResponse != nil {
		return g.TokenEstimateResponse, nil
	}
	return &ai.TokenEstimateResponse{}, nil
}
