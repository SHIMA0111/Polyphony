// Package ai defines the domain model for AI completions: chat messages,
// completion requests/responses, model metadata, and the LLMGateway port
// implemented by the interface layer.
package ai

// ChatMessage represents a single message in a conversation context sent to the LLM.
//
// Content holds the message's plain-text body, the original (and still the common)
// shape. Parts, when non-empty, holds multimodal content (text mixed with images)
// built from the message's attachments (see usecase/message's attachment-enrichment
// step, added in Step 39) and takes precedence over Content when this message is
// serialized to the LLM Gateway (see interface/gateway/llm_client.go's chatMsgDTO and
// grpc_client.go's toPBChatMessages): Content is left populated alongside Parts so
// existing readers of Content (e.g. logging) keep working, but a non-empty Parts
// always wins on the wire.
type ChatMessage struct {
	Role    string
	Content string
	Parts   []ContentPart
}

// ContentPart represents a single part of a multimodal message's content, mirroring
// the LLM Gateway's `ContentPart` enum (`llm-gateway/src/domain/model.rs`) and its
// REST/gRPC wire contracts.
//
// Exactly one of Text, ImageURL, or ImageBase64 holds meaningful data, discriminated
// by Type (one of the ContentPartType* constants):
//   - Type == ContentPartTypeText: Text holds the segment's text.
//   - Type == ContentPartTypeImageURL: ImageURL holds the image's URL.
//   - Type == ContentPartTypeImageBase64: ImageBase64 holds the inline image bytes.
type ContentPart struct {
	Type        string
	Text        string
	ImageURL    string
	ImageBase64 *ImageBase64Data
}

// ImageBase64Data holds an inline base64-encoded image, referenced by a ContentPart
// whose Type is ContentPartTypeImageBase64.
type ImageBase64Data struct {
	MediaType string
	Data      string
}

// Content part type discriminators, matching the wire-format `"type"` values used by
// the LLM Gateway's REST content-part contract
// (llm-gateway/src/adapters/inbound/rest/request.rs's ContentPartDto).
const (
	// ContentPartTypeText marks a ContentPart carrying a plain text segment.
	ContentPartTypeText = "text"
	// ContentPartTypeImageURL marks a ContentPart referencing an image by URL.
	ContentPartTypeImageURL = "image_url"
	// ContentPartTypeImageBase64 marks a ContentPart carrying an inline base64-encoded image.
	ContentPartTypeImageBase64 = "image_base64"
)

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

// Usage holds token accounting for a single completion, shared by the
// streaming (StreamChunk) and non-streaming (CompletionResponse) response
// shapes.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// StreamChunk represents a single incremental item from an LLMGateway.Stream
// response, mirroring the LLM Gateway's CompletionChunk SSE payload (Step
// 43's `POST /completions/stream`).
//
// Delta is the incremental text carried by this chunk; it may be empty on a
// chunk that only carries FinishReason and/or Usage (e.g. the final chunk of
// a stream). Usage is non-nil only on the final chunk of a successful
// stream -- every other chunk (including ones with a non-empty
// FinishReason) leaves it nil.
type StreamChunk struct {
	ID           string
	Model        string
	Delta        string
	FinishReason string
	Usage        *Usage
}

// StreamResult is a single item delivered on the channel returned by
// LLMGateway.Stream. Exactly one of Chunk or Err is set per item: a
// Chunk-carrying item represents one incremental delta (or the final
// summary chunk), while an Err-carrying item represents a mid-stream
// failure.
//
// A channel of StreamResult is closed after either the stream completes
// naturally (no further item is sent after the last Chunk) or after exactly
// one Err-carrying item is sent -- never both, and never more than one
// Err-carrying item.
type StreamResult struct {
	Chunk *StreamChunk
	Err   error
}
