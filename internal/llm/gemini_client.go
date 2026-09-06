package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	ProviderGemini LLMProvider = "gemini"
)

// =============================================================================
// Gemini 客户端实现 (兼容 OpenAI API 格式)
// =============================================================================

type GeminiClient struct {
	*BaseAdapter
	apiKey  string
	baseURL string
	model   string
}

type GeminiRequest struct {
	Model       string          `json:"model"`
	Messages    []OpenAIMessage `json:"messages"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
}

type GeminiResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type GeminiErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
		Param   string `json:"param"`
	} `json:"error"`
}

func NewGeminiClient(config *LLMConfig) (LLMClient, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("Gemini API key is required")
	}

	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com/v1beta/openai"
	}

	model := config.Model
	if model == "" {
		model = "gemini-3-flash-preview"
	}

	client := &GeminiClient{
		BaseAdapter: NewBaseAdapter(ProviderGemini, config),
		apiKey:      config.APIKey,
		baseURL:     baseURL,
		model:       model,
	}

	client.SetCapabilities(&LLMCapabilities{
		MaxTokens:         65536,
		SupportedFormats:  []string{"text", "json"},
		SupportsStreaming: true,
		SupportsBatch:     false,
		CostPerToken:      0.0005,
		LatencyMs:         500,
		Models:            []string{"gemini-3-flash-preview", "gemini-2.5-flash-preview", "gemini-3-pro-preview"},
	})

	return client, nil
}

func (gc *GeminiClient) Complete(ctx context.Context, req *LLMRequest) (*LLMResponse, error) {
	startTime := time.Now()

	if err := gc.CheckRateLimit(ctx); err != nil {
		return nil, err
	}

	if err := gc.CheckCircuitBreaker(); err != nil {
		return nil, err
	}

	geminiReq := gc.convertToGeminiFormat(req)
	resp, err := gc.sendRequest(ctx, geminiReq)
	if err != nil {
		gc.RecordFailure()
		return nil, err
	}

	gc.RecordSuccess()
	return gc.convertFromGeminiFormat(resp, time.Since(startTime)), nil
}

func (gc *GeminiClient) BatchComplete(ctx context.Context, reqs []*LLMRequest) ([]*LLMResponse, error) {
	responses := make([]*LLMResponse, len(reqs))

	for i, req := range reqs {
		resp, err := gc.Complete(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("batch request %d failed: %w", i, err)
		}
		responses[i] = resp
	}

	return responses, nil
}

func (gc *GeminiClient) StreamComplete(ctx context.Context, req *LLMRequest) (<-chan *LLMStreamResponse, error) {
	ch := make(chan *LLMStreamResponse, 1)

	go func() {
		defer close(ch)

		resp, err := gc.Complete(ctx, req)
		if err != nil {
			ch <- &LLMStreamResponse{
				Error:    err,
				Provider: ProviderGemini,
			}
			return
		}

		ch <- &LLMStreamResponse{
			Content:  resp.Content,
			Done:     true,
			Provider: ProviderGemini,
		}
	}()

	return ch, nil
}

func (gc *GeminiClient) HealthCheck(ctx context.Context) error {
	req := &LLMRequest{
		Prompt:      "Hello",
		MaxTokens:   1,
		Temperature: 0,
	}

	_, err := gc.Complete(ctx, req)
	return err
}

func (gc *GeminiClient) GetModel() string {
	return gc.model
}

func (gc *GeminiClient) convertToGeminiFormat(req *LLMRequest) *GeminiRequest {
	messages := []OpenAIMessage{}

	if req.SystemPrompt != "" {
		messages = append(messages, OpenAIMessage{
			Role:    "system",
			Content: req.SystemPrompt,
		})
	}

	messages = append(messages, OpenAIMessage{
		Role:    "user",
		Content: req.Prompt,
	})

	model := req.Model
	if model == "" {
		model = gc.model
	}

	return &GeminiRequest{
		Model:       model,
		Messages:    messages,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}
}

func (gc *GeminiClient) convertFromGeminiFormat(resp *GeminiResponse, duration time.Duration) *LLMResponse {
	content := ""
	if len(resp.Choices) > 0 {
		content = resp.Choices[0].Message.Content
	}

	return &LLMResponse{
		Content:    content,
		TokensUsed: resp.Usage.TotalTokens,
		Model:      resp.Model,
		Provider:   ProviderGemini,
		Duration:   duration,
		Metadata: map[string]interface{}{
			"id":            resp.ID,
			"finish_reason": resp.Choices[0].FinishReason,
		},
	}
}

func (gc *GeminiClient) sendRequest(ctx context.Context, req *GeminiRequest) (*GeminiResponse, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request failed: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions?key=%s", gc.baseURL, gc.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := gc.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request failed: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response failed: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		var errorResp GeminiErrorResponse
		if err := json.Unmarshal(respBody, &errorResp); err == nil {
			return nil, &LLMError{
				Provider:  ProviderGemini,
				Code:      errorResp.Error.Code,
				Message:   errorResp.Error.Message,
				Retryable: httpResp.StatusCode >= 500,
			}
		}
		return nil, fmt.Errorf("HTTP %d: %s", httpResp.StatusCode, string(respBody))
	}

	var resp GeminiResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response failed: %w", err)
	}

	return &resp, nil
}