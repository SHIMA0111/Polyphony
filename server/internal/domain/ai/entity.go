// Package ai defines the domain model for AI completions: chat messages,
// completion requests/responses, model metadata, and the LLMGateway port
// implemented by the interface layer.
package ai

// ChatMessage represents a single message in a conversation context sent to the LLM.
type ChatMessage struct {
	Role    string
	Content string
}

// CompletionRequest holds the parameters for an LLM completion request.
type CompletionRequest struct {
	Model       string
	Messages    []ChatMessage
	MaxTokens   *int
	Temperature *float64
}

// CompletionResponse holds the result of an LLM completion.
type CompletionResponse struct {
	Content      string
	Model        string
	PromptTokens int
	OutputTokens int
}

// ModelInfo describes an available LLM model.
type ModelInfo struct {
	ID       string
	Name     string
	Provider string
	// ContextWindow is the maximum input+output token count the model
	// supports, as reported by the LLM Gateway. Zero means "unknown" -- the
	// gateway did not report a context window for this model (its `pricing`
	// field was `None`/absent on the wire) -- not that the model has no
	// context limit.
	ContextWindow int
	// InputPricePerMillionTokens is the USD price per 1,000,000 input
	// (prompt) tokens, per-1M being the project-wide canonical pricing unit
	// (see llm-gateway's ModelPricing). Zero means "unknown", not "free".
	InputPricePerMillionTokens float64
	// OutputPricePerMillionTokens is the USD price per 1,000,000 output
	// (completion) tokens. Zero means "unknown", not "free".
	OutputPricePerMillionTokens float64
	// SupportsImageInput reports whether the model accepts image/Vision
	// content parts. False means either "no" or "unknown" -- the LLM Gateway
	// collapses an absent value to false, since callers must treat an
	// unreported capability as unsupported.
	SupportsImageInput bool
}

// TokenEstimateRequest holds the parameters for a token estimation request:
// the target model (for future model-specific tuning) and the messages to
// estimate.
type TokenEstimateRequest struct {
	Model    string
	Messages []ChatMessage
}

// TokenEstimateResponse holds the result of a token estimation request.
// EstimatedTokens is an approximation produced by the LLM Gateway's
// character-based heuristic, not an exact count from the target model's
// real tokenizer.
type TokenEstimateResponse struct {
	Model           string
	EstimatedTokens int
}
