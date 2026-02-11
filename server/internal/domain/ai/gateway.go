package ai

import "context"

// LLMGateway defines the interface for communicating with the LLM Gateway.
// This is the swap point for Phase 8 (gRPC).
type LLMGateway interface {
	// Complete sends a completion request and returns the response.
	// Returns ErrLLMGateway on communication or processing errors.
	Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)

	// ListModels returns the available LLM models.
	ListModels(ctx context.Context) ([]ModelInfo, error)
}
