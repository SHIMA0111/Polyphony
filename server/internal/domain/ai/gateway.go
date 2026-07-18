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

	// EstimateTokens returns an approximate token count for the given
	// messages, computed by the LLM Gateway's character-based heuristic (not
	// an exact tokenizer count). Returns ErrLLMGateway-wrapped errors on
	// communication or decode failures, matching Complete/ListModels.
	EstimateTokens(ctx context.Context, req *TokenEstimateRequest) (*TokenEstimateResponse, error)

	// Stream sends a streaming completion request and returns a channel of
	// incremental StreamResult items.
	//
	// The returned error is non-nil only for a *synchronous* dispatch
	// failure -- the same failure modes Complete can return (bad model,
	// connection refused, a non-2xx initial response) -- in which case the
	// returned channel is nil and must not be read from.
	//
	// On a nil error, the caller owns draining the returned channel until it
	// is closed by the implementation (see StreamResult's doc comment for
	// the exact termination contract). Stream itself does not bound how long
	// the stream may run; the caller's ctx is what bounds it -- callers that
	// need a stream to outlive a request-scoped context (e.g. so that other
	// subscribers keep receiving delivery after the originating HTTP request
	// has returned) must pass in a context they control independently of the
	// request.
	Stream(ctx context.Context, req *CompletionRequest) (<-chan StreamResult, error)
}
