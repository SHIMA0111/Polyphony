package mocks

import (
	"context"
	"fmt"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/ai"
)

// LLMGateway is a configurable fake implementing ai.LLMGateway.
//
// By default, Complete and EstimateTokens return canned successful
// responses and ListModels returns an empty slice. Setting ShouldErr makes
// both Complete and EstimateTokens return a domain.ErrLLMGateway-wrapped
// error, as if the LLM Gateway call failed. For full control over the
// response, set CompleteFunc / ListModelsFunc / EstimateTokensFunc, which
// take priority over ShouldErr / CompletionResponse / Models /
// TokenEstimateResponse.
//
// The zero value (mocks.LLMGateway{}) is ready to use.
type LLMGateway struct {
	// ShouldErr, if true, makes both Complete and EstimateTokens return a
	// domain.ErrLLMGateway-wrapped error instead of a canned response, when
	// their respective CompleteFunc / EstimateTokensFunc override is unset.
	// It does not affect ListModels or Stream.
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

	// StreamErr, if non-nil, makes Stream return it as a synchronous
	// dispatch error (nil channel), as if the LLM Gateway's initial response
	// to a streaming request failed.
	StreamErr error
	// StreamChunks, if non-empty and StreamFunc/StreamErr are unset, is the
	// canned sequence of successful chunks Stream sends on its returned
	// channel (each wrapped in a StreamResult{Chunk: ...}) before closing it.
	StreamChunks []*ai.StreamChunk
	// StreamMidErr, if non-nil and StreamFunc/StreamErr are unset, is sent
	// as a single StreamResult{Err: ...} after StreamChunks, before the
	// channel closes -- simulating a mid-stream provider failure.
	StreamMidErr error

	// StreamFunc, if set, overrides Stream entirely.
	StreamFunc func(ctx context.Context, req *ai.CompletionRequest) (<-chan ai.StreamResult, error)
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

// Stream sends a streaming completion request and returns a channel of
// incremental results. See the LLMGateway doc comment for how StreamFunc,
// StreamErr, StreamChunks, and StreamMidErr interact:
//   - StreamFunc, if set, takes priority over everything else.
//   - Otherwise, a non-nil StreamErr is returned synchronously with a nil
//     channel (simulating a synchronous dispatch failure).
//   - Otherwise, a channel is returned that sends one StreamResult{Chunk: c}
//     per entry in StreamChunks (in order), then, if StreamMidErr is
//     non-nil, one final StreamResult{Err: StreamMidErr}, before closing.
//     The channel send happens in a background goroutine so callers observe
//     the same "returns immediately, delivers asynchronously" contract the
//     real gateway client has.
func (g *LLMGateway) Stream(ctx context.Context, req *ai.CompletionRequest) (<-chan ai.StreamResult, error) {
	if g.StreamFunc != nil {
		return g.StreamFunc(ctx, req)
	}
	if g.StreamErr != nil {
		return nil, g.StreamErr
	}

	out := make(chan ai.StreamResult)
	go func() {
		defer close(out)
		for _, chunk := range g.StreamChunks {
			out <- ai.StreamResult{Chunk: chunk}
		}
		if g.StreamMidErr != nil {
			out <- ai.StreamResult{Err: g.StreamMidErr}
		}
	}()
	return out, nil
}
