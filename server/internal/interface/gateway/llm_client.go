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

// LLMClient implements ai.LLMGateway using REST calls to the LLM Gateway.
type LLMClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewLLMClient creates a new LLMClient.
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

type chatMsgDTO struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type completionRespDTO struct {
	Content string   `json:"content"`
	Model   string   `json:"model"`
	Usage   usageDTO `json:"usage"`
}

type usageDTO struct {
	PromptTokens int `json:"prompt_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type modelDTO struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
}

type modelsRespDTO struct {
	Models []modelDTO `json:"models"`
}

// Complete sends a completion request to the LLM Gateway.
func (c *LLMClient) Complete(ctx context.Context, req *ai.CompletionRequest) (*ai.CompletionResponse, error) {
	msgs := make([]chatMsgDTO, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = chatMsgDTO{Role: m.Role, Content: m.Content}
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

	return &ai.CompletionResponse{
		Content:      result.Content,
		Model:        result.Model,
		PromptTokens: result.Usage.PromptTokens,
		OutputTokens: result.Usage.OutputTokens,
	}, nil
}

// ListModels retrieves available models from the LLM Gateway.
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
		models[i] = ai.ModelInfo{ID: m.ID, Provider: m.Provider}
	}
	return models, nil
}
