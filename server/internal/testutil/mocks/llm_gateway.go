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
// ListModels returns an empty slice. Setting ShouldErr makes Complete return
// a domain.ErrLLMGateway-wrapped error, as if the LLM Gateway call failed.
// For full control over the response, set CompleteFunc / ListModelsFunc,
// which take priority over ShouldErr / CompletionResponse / Models.
//
// The zero value (mocks.LLMGateway{}) is ready to use.
type LLMGateway struct {
	// ShouldErr, if true, makes Complete return a domain.ErrLLMGateway-wrapped
	// error instead of a canned response.
	ShouldErr bool
	// CompletionResponse, if non-nil, overrides the default canned
	// completion response returned by Complete.
	CompletionResponse *ai.CompletionResponse
	// Models, if non-nil, overrides the default (empty) ListModels result.
	Models []ai.ModelInfo

	// CompleteFunc, if set, overrides Complete entirely.
	CompleteFunc func(ctx context.Context, req *ai.CompletionRequest) (*ai.CompletionResponse, error)
	// ListModelsFunc, if set, overrides ListModels entirely.
	ListModelsFunc func(ctx context.Context) ([]ai.ModelInfo, error)
}

// Complete sends a completion request and returns the response. See the
// LLMGateway doc comment for how ShouldErr, CompletionResponse, and
// CompleteFunc interact.
func (g *LLMGateway) Complete(ctx context.Context, req *ai.CompletionRequest) (*ai.CompletionResponse, error) {
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
