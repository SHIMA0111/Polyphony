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
}
