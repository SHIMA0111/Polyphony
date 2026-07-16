// Package gateway implements outbound gateway ports, including the REST client for the LLM Gateway service.
package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/ai"
)

// LLMClient implements the ai.LLMGateway interface using REST calls to the LLM Gateway service.
type LLMClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewLLMClient creates a new LLMClient configured to call the LLM Gateway at the given base URL.
// The HTTP client is configured with a 60-second timeout.
func NewLLMClient(baseURL string) *LLMClient {
	return &LLMClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// --- Private request/response DTOs matching LLM Gateway format ---

type completionReqDTO struct {
	Model       string       `json:"model"`
	Messages    []chatMsgDTO `json:"messages"`
	MaxTokens   *int         `json:"max_tokens,omitempty"`
	Temperature *float64     `json:"temperature,omitempty"`
}

// chatMsgDTO is the wire shape of a single chat message, matching the LLM
// Gateway's REST contract on both the request and response side.
//
// Content is `any` rather than `string` because the gateway's `content`
// field is a `#[serde(untagged)]` union (see
// llm-gateway/src/adapters/inbound/rest/request.rs's ContentDto): a plain
// JSON string for text-only messages, or an array of contentPartDTO objects
// for multimodal (Vision) messages. On the outbound (request) side, build it
// with toContentDTO rather than assigning a raw string/slice directly, so the
// two shapes stay centralized in one place. On the inbound (response) side,
// json.Unmarshal decodes a bare JSON string into a Go string held by this
// `any` (every completion response's message content is plain text today, no
// provider adapter yet echoes image content back) -- see Complete's use of a
// type assertion when reading it back out.
type chatMsgDTO struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

// contentPartDTO is the wire shape of a single multimodal content part,
// matching the LLM Gateway's ContentPartDto
// (llm-gateway/src/adapters/inbound/rest/request.rs): internally tagged by
// Type ("text" | "image_url" | "image_base64"), with only the fields
// relevant to that Type populated (the others are omitted via `omitempty`).
type contentPartDTO struct {
	Type      string       `json:"type"`
	Text      string       `json:"text,omitempty"`
	ImageURL  *imageURLDTO `json:"image_url,omitempty"`
	MediaType string       `json:"media_type,omitempty"`
	Data      string       `json:"data,omitempty"`
}

// imageURLDTO is the nested `image_url` object of a contentPartDTO whose
// Type is "image_url", matching the gateway's `{"url": "..."}` shape rather
// than a bare string.
type imageURLDTO struct {
	URL string `json:"url"`
}

// toContentDTO builds the wire-format `content` value for a single domain
// ai.ChatMessage: a non-empty Parts takes precedence over Content and
// becomes a []contentPartDTO (via toContentPartDTO); otherwise Content
// becomes a plain string (the common, backward-compatible text-only case).
func toContentDTO(m ai.ChatMessage) any {
	if len(m.Parts) == 0 {
		return m.Content
	}
	parts := make([]contentPartDTO, len(m.Parts))
	for i, p := range m.Parts {
		parts[i] = toContentPartDTO(p)
	}
	return parts
}

// toContentPartDTO converts a single domain ai.ContentPart to its wire-format
// contentPartDTO, dispatching on p.Type (one of the ai.ContentPartType*
// constants). An unrecognized Type falls back to a text part carrying
// p.Text (defaulting to the empty string).
func toContentPartDTO(p ai.ContentPart) contentPartDTO {
	switch p.Type {
	case ai.ContentPartTypeImageURL:
		return contentPartDTO{Type: "image_url", ImageURL: &imageURLDTO{URL: p.ImageURL}}
	case ai.ContentPartTypeImageBase64:
		var mediaType, data string
		if p.ImageBase64 != nil {
			mediaType, data = p.ImageBase64.MediaType, p.ImageBase64.Data
		}
		return contentPartDTO{Type: "image_base64", MediaType: mediaType, Data: data}
	default:
		return contentPartDTO{Type: "text", Text: p.Text}
	}
}

type completionRespDTO struct {
	Model   string      `json:"model"`
	Choices []choiceDTO `json:"choices"`
	Usage   usageDTO    `json:"usage"`
}

type choiceDTO struct {
	Message chatMsgDTO `json:"message"`
}

type usageDTO struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// modelPricingDTO mirrors the LLM Gateway's nested `ModelPricingDto`
// (`llm-gateway/src/adapters/inbound/rest/response.rs`): per-1M-token USD
// pricing, present only when `modelDTO.Pricing` is non-nil.
type modelPricingDTO struct {
	InputPricePerMillionTokens  float64 `json:"input_price_per_million_tokens"`
	OutputPricePerMillionTokens float64 `json:"output_price_per_million_tokens"`
	Currency                    string  `json:"currency"`
}

// modelDTO mirrors the LLM Gateway's `ModelInfoDto` wire shape. ContextWindow,
// SupportsImageInput, and Pricing are pointers because the gateway omits them
// from the JSON body entirely when unknown (`#[serde(skip_serializing_if =
// "Option::is_none")]`), rather than serializing `null`; ListModels flattens
// a nil pointer to Go's zero value ("0/false = unknown").
type modelDTO struct {
	ID                 string           `json:"id"`
	Name               string           `json:"name"`
	Provider           string           `json:"provider"`
	ContextWindow      *int             `json:"context_window"`
	SupportsImageInput *bool            `json:"supports_image_input"`
	Pricing            *modelPricingDTO `json:"pricing"`
}

type modelsRespDTO struct {
	Models []modelDTO `json:"models"`
}

type tokenEstimateReqDTO struct {
	Model    string       `json:"model"`
	Messages []chatMsgDTO `json:"messages"`
}

type tokenEstimateRespDTO struct {
	Model           string `json:"model"`
	EstimatedTokens int    `json:"estimated_tokens"`
}

// Complete sends a chat completion request to the LLM Gateway and returns the response.
// It returns a domain.ErrLLMGateway-wrapped error on request marshalling, HTTP, or decode failures.
func (c *LLMClient) Complete(ctx context.Context, req *ai.CompletionRequest) (*ai.CompletionResponse, error) {
	msgs := make([]chatMsgDTO, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = chatMsgDTO{Role: m.Role, Content: toContentDTO(m)}
	}

	body := completionReqDTO{
		Model:       req.Model,
		Messages:    msgs,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal request: %v", domain.ErrLLMGateway, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/completions", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: create request: %v", domain.ErrLLMGateway, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: send request: %v", domain.ErrLLMGateway, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%w: status %d: %s", domain.ErrLLMGateway, resp.StatusCode, string(respBody))
	}

	var result completionRespDTO
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("%w: decode response: %v", domain.ErrLLMGateway, err)
	}

	// The gateway's completion response message content is always plain text
	// today (no provider adapter yet echoes image content back in a
	// response), so it always decodes as a bare JSON string; a non-string
	// value here (e.g. a future multimodal response) is treated as empty
	// rather than panicking on a failed type assertion.
	content := ""
	if len(result.Choices) > 0 {
		if s, ok := result.Choices[0].Message.Content.(string); ok {
			content = s
		}
	}

	return &ai.CompletionResponse{
		Content:      content,
		Model:        result.Model,
		PromptTokens: result.Usage.PromptTokens,
		OutputTokens: result.Usage.CompletionTokens,
	}, nil
}

// ListModels retrieves the list of available models from the LLM Gateway.
// It returns a domain.ErrLLMGateway-wrapped error on HTTP or decode failures.
func (c *LLMClient) ListModels(ctx context.Context) ([]ai.ModelInfo, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("%w: create request: %v", domain.ErrLLMGateway, err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: send request: %v", domain.ErrLLMGateway, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%w: status %d: %s", domain.ErrLLMGateway, resp.StatusCode, string(respBody))
	}

	var result modelsRespDTO
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("%w: decode response: %v", domain.ErrLLMGateway, err)
	}

	models := make([]ai.ModelInfo, len(result.Models))
	for i, m := range result.Models {
		info := ai.ModelInfo{ID: m.ID, Name: m.Name, Provider: m.Provider}
		if m.ContextWindow != nil {
			info.ContextWindow = *m.ContextWindow
		}
		if m.SupportsImageInput != nil {
			info.SupportsImageInput = *m.SupportsImageInput
		}
		if m.Pricing != nil {
			info.InputPricePerMillionTokens = m.Pricing.InputPricePerMillionTokens
			info.OutputPricePerMillionTokens = m.Pricing.OutputPricePerMillionTokens
		}
		models[i] = info
	}
	return models, nil
}

// EstimateTokens sends a token estimation request to the LLM Gateway and returns the
// approximate token count. It returns a domain.ErrLLMGateway-wrapped error on request
// marshalling, HTTP, or decode failures.
func (c *LLMClient) EstimateTokens(ctx context.Context, req *ai.TokenEstimateRequest) (*ai.TokenEstimateResponse, error) {
	msgs := make([]chatMsgDTO, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = chatMsgDTO{Role: m.Role, Content: toContentDTO(m)}
	}

	body := tokenEstimateReqDTO{
		Model:    req.Model,
		Messages: msgs,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal request: %v", domain.ErrLLMGateway, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/tokens/estimate", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: create request: %v", domain.ErrLLMGateway, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: send request: %v", domain.ErrLLMGateway, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%w: status %d: %s", domain.ErrLLMGateway, resp.StatusCode, string(respBody))
	}

	var result tokenEstimateRespDTO
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("%w: decode response: %v", domain.ErrLLMGateway, err)
	}

	return &ai.TokenEstimateResponse{
		Model:           result.Model,
		EstimatedTokens: result.EstimatedTokens,
	}, nil
}
