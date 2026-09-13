package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// =============================================================================
// Ollama本地模型统一客户端实现
// =============================================================================

// OllamaLocalClient Ollama本地模型统一适配器
type OllamaLocalClient struct {
	*BaseAdapter
	baseURL     string
	modelName   string
	displayName string
}

// OllamaRequest Ollama请求格式（用于 /api/generate 端点）
type OllamaRequest struct {
	Model     string                 `json:"model"`
	Prompt    string                 `json:"prompt"`
	Stream    bool                   `json:"stream"`
	Context   []int                  `json:"context,omitempty"`
	KeepAlive string                 `json:"keep_alive,omitempty"`
	Options   map[string]interface{} `json:"options,omitempty"`
}

// OllamaChatRequest Ollama聊天请求格式（用于 /api/chat 端点，支持多模态）
type OllamaChatRequest struct {
	Model    string                 `json:"model"`
	Messages []OllamaChatMessage    `json:"messages"`
	Stream   bool                   `json:"stream"`
	Options  map[string]interface{} `json:"options,omitempty"`
}

// OllamaChatMessage Ollama聊天消息
type OllamaChatMessage struct {
	Role    string   `json:"role"`              // user, assistant, system
	Content string   `json:"content"`           // 文本内容
	Images  []string `json:"images,omitempty"`  // base64编码的图片
}

// OllamaResponse Ollama响应格式
type OllamaResponse struct {
	Model              string `json:"model"`
	Response           string `json:"response"`
	Done               bool   `json:"done"`
	Context            []int  `json:"context,omitempty"`
	TotalDuration      int64  `json:"total_duration"`
	LoadDuration       int64  `json:"load_duration"`
	PromptEvalCount    int    `json:"prompt_eval_count"`
	PromptEvalDuration int64  `json:"prompt_eval_duration"`
	EvalCount          int    `json:"eval_count"`
	EvalDuration       int64  `json:"eval_duration"`
}

// OllamaErrorResponse Ollama错误响应
type OllamaErrorResponse struct {
	Error string `json:"error"`
}

// NewOllamaLocalClient 创建Ollama本地客户端
func NewOllamaLocalClient(config *LLMConfig) (LLMClient, error) {
	// 优先从环境变量读取，支持 Docker 环境
	baseURL := os.Getenv("OLLAMA_HOST")
	if baseURL == "" {
		// 如果环境变量未设置，使用 config 中的值
		baseURL = config.BaseURL
		if baseURL == "" {
			baseURL = "http://localhost:11434" // 本地直接访问
		}
	}

	modelName := config.Model
	if modelName == "" {
		modelName = "codeqwen:7b" // 默认模型
	}

	// 生成友好的显示名称
	displayName := generateDisplayName(modelName)

	client := &OllamaLocalClient{
		BaseAdapter: NewBaseAdapter(ProviderOllamaLocal, config),
		baseURL:     baseURL,
		modelName:   modelName,
		displayName: displayName,
	}

	// 设置能力（根据模型类型动态调整）
	capabilities := getModelCapabilities(modelName)
	client.SetCapabilities(capabilities)

	return client, nil
}

// generateDisplayName 生成友好的显示名称
func generateDisplayName(modelName string) string {
	switch {
	case strings.Contains(modelName, "codeqwen"):
		return "CodeQwen-Local"
	case strings.Contains(modelName, "deepseek-coder"):
		if strings.Contains(modelName, "33b") {
			return "DeepSeekCoder-33B-Local"
		} else if strings.Contains(modelName, "16b") {
			return "DeepSeekCoder-16B-Local"
		} else if strings.Contains(modelName, "6.7b") {
			return "DeepSeekCoder-6.7B-Local"
		}
		return "DeepSeekCoder-Local"
	default:
		return fmt.Sprintf("Ollama-%s", modelName)
	}
}

// getModelCapabilities 根据模型获取能力配置
func getModelCapabilities(modelName string) *LLMCapabilities {
	baseCapabilities := &LLMCapabilities{
		SupportedFormats:  []string{"text", "json", "code"},
		SupportsStreaming: true,
		SupportsBatch:     false,
		CostPerToken:      0.0, // 本地模型免费
		LatencyMs:         200, // 本地响应更快
	}

	// 根据模型设置特定参数
	switch {
	case strings.Contains(modelName, "codeqwen"):
		baseCapabilities.MaxTokens = 8192
		baseCapabilities.Models = []string{"codeqwen:7b", "codeqwen:latest"}

	case strings.Contains(modelName, "deepseek-coder"):
		if strings.Contains(modelName, "33b") {
			baseCapabilities.MaxTokens = 16384
			baseCapabilities.LatencyMs = 800 // 大模型稍慢
		} else if strings.Contains(modelName, "16b") {
			baseCapabilities.MaxTokens = 16384
			baseCapabilities.LatencyMs = 500
		} else {
			baseCapabilities.MaxTokens = 8192
			baseCapabilities.LatencyMs = 300
		}
		baseCapabilities.Models = []string{
			"deepseek-coder:6.7b", "deepseek-coder:6.7b-instruct",
			"deepseek-coder:33b", "deepseek-coder:33b-instruct",
			"deepseek-coder-v2:16b",
		}

	default:
		baseCapabilities.MaxTokens = 4096
		baseCapabilities.Models = []string{modelName}
	}

	return baseCapabilities
}

// Complete 完成对话
func (oc *OllamaLocalClient) Complete(ctx context.Context, req *LLMRequest) (*LLMResponse, error) {
	startTime := time.Now()

	fmt.Printf("🤖 [%s] 开始处理本地模型请求...\n", oc.displayName)
	fmt.Printf("📝 请求参数: MaxTokens=%d, Temperature=%.1f, Format=%s\n",
		req.MaxTokens, req.Temperature, req.Format)
	fmt.Printf("📋 系统提示词长度: %d 字符\n", len(req.SystemPrompt))
	fmt.Printf("📋 用户提示词长度: %d 字符\n", len(req.Prompt))

	// 1. 检查限流（本地模型通常不需要限流）
	fmt.Printf("🚦 [步骤1] 检查限流...\n")

	// 🔥 本地模型跳过限流检查（性能优化）
	skipRateLimit := true
	if req.Metadata != nil {
		if skip, exists := req.Metadata["skip_rate_limit"]; exists {
			if skipBool, ok := skip.(bool); ok {
				skipRateLimit = skipBool
			}
		}
	}

	if !skipRateLimit {
		if err := oc.CheckRateLimit(ctx); err != nil {
			fmt.Printf("❌ 限流检查失败: %v\n", err)
			return nil, err
		}
		fmt.Printf("✅ 限流检查通过\n")
	} else {
		fmt.Printf("✅ 限流检查跳过（本地模型）\n")
	}

	// 2. 检查熔断器
	fmt.Printf("🔌 [步骤2] 检查熔断器...\n")
	if err := oc.CheckCircuitBreaker(); err != nil {
		fmt.Printf("❌ 熔断器检查失败: %v\n", err)
		return nil, err
	}
	fmt.Printf("✅ 熔断器检查通过\n")

	// 3. 转换请求格式
	fmt.Printf("🔄 [步骤3] 转换请求格式...\n")
	ollamaReq := oc.convertToOllamaFormat(req)
	fmt.Printf("✅ 请求格式转换完成: Model=%s\n", ollamaReq.Model)

	// 4. 发送请求到Ollama
	fmt.Printf("📡 [步骤4] 发送请求到Ollama (%s)...\n", oc.baseURL)
	resp, err := oc.sendRequest(ctx, ollamaReq)
	if err != nil {
		fmt.Printf("❌ Ollama请求失败: %v\n", err)
		oc.RecordFailure()
		return nil, err
	}
	fmt.Printf("✅ Ollama请求成功\n")

	// 5. 转换响应格式
	fmt.Printf("🔄 [步骤5] 转换响应格式...\n")
	oc.RecordSuccess()
	result := oc.convertFromOllamaFormat(resp, time.Since(startTime))

	fmt.Printf("✅ %s处理完成\n", oc.displayName)
	fmt.Printf("📊 响应统计: TokensUsed=%d, Duration=%v\n",
		result.TokensUsed, result.Duration)
	fmt.Printf("📄 响应内容长度: %d 字符\n", len(result.Content))

	return result, nil
}

// BatchComplete 批量完成
func (oc *OllamaLocalClient) BatchComplete(ctx context.Context, reqs []*LLMRequest) ([]*LLMResponse, error) {
	responses := make([]*LLMResponse, len(reqs))

	for i, req := range reqs {
		resp, err := oc.Complete(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("batch request %d failed: %w", i, err)
		}
		responses[i] = resp
	}

	return responses, nil
}

// StreamComplete 流式完成
func (oc *OllamaLocalClient) StreamComplete(ctx context.Context, req *LLMRequest) (<-chan *LLMStreamResponse, error) {
	ch := make(chan *LLMStreamResponse, 1)

	go func() {
		defer close(ch)

		resp, err := oc.Complete(ctx, req)
		if err != nil {
			ch <- &LLMStreamResponse{
				Error:    err,
				Provider: ProviderOllamaLocal,
			}
			return
		}

		ch <- &LLMStreamResponse{
			Content:  resp.Content,
			Done:     true,
			Provider: ProviderOllamaLocal,
		}
	}()

	return ch, nil
}

// HealthCheck 健康检查
func (oc *OllamaLocalClient) HealthCheck(ctx context.Context) error {
	// 检查Ollama服务是否可用
	healthURL := oc.baseURL + "/api/tags"

	httpReq, err := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
	if err != nil {
		return fmt.Errorf("create health check request failed: %w", err)
	}

	httpResp, err := oc.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("ollama service not available: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama service unhealthy: HTTP %d", httpResp.StatusCode)
	}

	return nil
}

// GetModel 获取模型名称
func (oc *OllamaLocalClient) GetModel() string {
	return oc.modelName
}

// convertToOllamaFormat 转换为Ollama格式
func (oc *OllamaLocalClient) convertToOllamaFormat(req *LLMRequest) *OllamaRequest {
	// 构建完整的prompt
	fullPrompt := ""
	if req.SystemPrompt != "" {
		fullPrompt = fmt.Sprintf("System: %s\n\nUser: %s", req.SystemPrompt, req.Prompt)
	} else {
		fullPrompt = req.Prompt
	}

	// 设置选项
	options := make(map[string]interface{})
	if req.MaxTokens > 0 {
		options["num_predict"] = req.MaxTokens
	}
	if req.Temperature >= 0 {
		options["temperature"] = req.Temperature
	}

	// 使用配置中指定的模型，或请求中指定的模型
	modelName := oc.modelName
	if req.Model != "" {
		modelName = req.Model
	}

	return &OllamaRequest{
		Model:     modelName,
		Prompt:    fullPrompt,
		Stream:    false,
		Options:   options,
		Context:   nil,  // 🔥 强制清除上下文缓存，避免历史对话污染
		KeepAlive: ollamaKeepAlive(),
	}
}

// ollamaKeepAlive keeps the model warm between requests without retaining
// conversation context. Set OLLAMA_KEEP_ALIVE=0s to restore immediate unload.
func ollamaKeepAlive() string {
	if value := strings.TrimSpace(os.Getenv("OLLAMA_KEEP_ALIVE")); value != "" {
		return value
	}
	return "10m"
}

// convertFromOllamaFormat 转换Ollama响应格式
func (oc *OllamaLocalClient) convertFromOllamaFormat(resp *OllamaResponse, duration time.Duration) *LLMResponse {
	// 估算token使用量（简单估算：字符数/4）
	tokensUsed := (len(resp.Response) + len(resp.Model)) / 4

	return &LLMResponse{
		Content:    resp.Response,
		TokensUsed: tokensUsed,
		Model:      resp.Model,
		Provider:   ProviderOllamaLocal,
		Duration:   duration,
		Metadata: map[string]interface{}{
			"display_name":         oc.displayName,
			"total_duration":       resp.TotalDuration,
			"load_duration":        resp.LoadDuration,
			"prompt_eval_count":    resp.PromptEvalCount,
			"prompt_eval_duration": resp.PromptEvalDuration,
			"eval_count":           resp.EvalCount,
			"eval_duration":        resp.EvalDuration,
			"tokens_per_second":    calculateTokensPerSecond(resp.EvalCount, resp.EvalDuration),
		},
	}
}

// calculateTokensPerSecond 计算每秒token数
func calculateTokensPerSecond(evalCount int, evalDuration int64) float64 {
	if evalDuration == 0 {
		return 0
	}
	// evalDuration is in nanoseconds
	seconds := float64(evalDuration) / 1e9
	return float64(evalCount) / seconds
}

// sendRequest 发送HTTP请求到Ollama
func (oc *OllamaLocalClient) sendRequest(ctx context.Context, req *OllamaRequest) (*OllamaResponse, error) {
	// 序列化请求
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request failed: %w", err)
	}

	// 🔥 打印入参
	log.Printf("📡 [Ollama入参] %s", string(reqBody))

	// 创建HTTP请求
	httpReq, err := http.NewRequestWithContext(ctx, "POST", oc.baseURL+"/api/generate", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	// 设置请求头
	httpReq.Header.Set("Content-Type", "application/json")

	// 发送请求
	httpResp, err := oc.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request failed: %w", err)
	}
	defer httpResp.Body.Close()

	// 读取响应
	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response failed: %w", err)
	}

	// 🔥 打印出参
	log.Printf("📨 [Ollama出参] %s", string(respBody))

	// 检查HTTP状态码
	if httpResp.StatusCode != http.StatusOK {
		var errorResp OllamaErrorResponse
		if err := json.Unmarshal(respBody, &errorResp); err == nil {
			return nil, &LLMError{
				Provider:  ProviderOllamaLocal,
				Code:      "OLLAMA_ERROR",
				Message:   errorResp.Error,
				Retryable: httpResp.StatusCode >= 500,
			}
		}
		return nil, fmt.Errorf("HTTP %d: %s", httpResp.StatusCode, string(respBody))
	}

	// 解析响应
	var resp OllamaResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response failed: %w", err)
	}

	return &resp, nil
}

// OllamaChatResponse Ollama聊天响应格式（保留用于其他功能）
type OllamaChatResponse struct {
	Model     string            `json:"model"`
	CreatedAt string            `json:"created_at"`
	Message   OllamaChatMessage `json:"message"`
	Done      bool              `json:"done"`
}
