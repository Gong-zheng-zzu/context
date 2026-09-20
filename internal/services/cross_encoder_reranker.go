package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/contextkeeper/service/internal/models"
)

// =============================================================================
// 云端 cross-encoder 精排层
//
// 位置：RRF 融合之后、内容合成之前（llm_driven_context_service.go 的
// applyCrossEncoderRerank）。对融合后的 top-N 候选调用云端 cross-encoder
// （Cohere Rerank / OpenAI 兼容端点）得到相关性分数并重排。
//
// 降级策略（任一情形均不 panic、不阻塞主流程，失败时保留 RRF 排序）：
//   - 未启用（RERANK_ENABLED!=true）：NewCrossEncoderRerankerFromEnv 返回 nil，
//     调用方直接跳过精排（不写任何 rerank_* 审计，行为与改动前完全一致）。
//   - 已启用但配置不完整（缺 API Key / URL 为空 / URL 非法）：构造函数仍返回非
//     nil 客户端，RerankResults 直接记录可区分的 rerank_degraded 并回退，不发请求。
//   - 已启用且配置完整：运行时 HTTP 非 2xx / 响应体非法 / 超时 / 网络不可达等
//     由 callRerankAPI 分类标记 rerank_degraded 后回退。
//
// 审计：精排状态写入 RetrievalResults.FusionMetadata：
//   - 成功：rerank_applied=true / rerank_model / rerank_top_n / rerank_latency_ms
//   - 降级：rerank_applied=false / rerank_degraded / rerank_error / rerank_attempts
//     / rerank_latency_ms
//
// 安全：API Key 仅写入请求头，绝不进入日志或审计元数据（错误文本经 redact 处理）。
// =============================================================================

const (
	defaultCrossEncoderTopN       = 20
	defaultCrossEncoderTimeout    = 30 * time.Second
	defaultCrossEncoderMaxRetries = 2
	defaultCrossEncoderModel      = "rerank-v3.5"
)

// 精排降级原因码，写入 FusionMetadata["rerank_degraded"]，值彼此可区分。
const (
	// rerankDegradedMissingAPIURL 已启用但 RERANK_API_URL 为空
	rerankDegradedMissingAPIURL = "missing_api_url"
	// rerankDegradedInvalidAPIURL 已启用但 RERANK_API_URL 非法（非 http/https 或缺 host）
	rerankDegradedInvalidAPIURL = "invalid_api_url"
	// rerankDegradedMissingAPIKey 已启用但 RERANK_API_KEY 为空
	rerankDegradedMissingAPIKey = "missing_api_key"
	// rerankDegradedHTTPError 精排服务返回非 2xx
	rerankDegradedHTTPError = "http_error"
	// rerankDegradedInvalidResponse 响应体不是预期结构（解析失败 / results 为空）
	rerankDegradedInvalidResponse = "invalid_response"
	// rerankDegradedTimeout 请求超时（客户端超时 / 上下文超时）
	rerankDegradedTimeout = "timeout"
	// rerankDegradedAPIUnavailable 连接失败等其它网络错误（兜底原因）
	rerankDegradedAPIUnavailable = "api_unavailable"
)

// CrossEncoderRerankerConfig 云端 cross-encoder 精排配置
type CrossEncoderRerankerConfig struct {
	Enabled    bool
	APIURL     string
	APIKey     string
	Model      string
	TopN       int
	Timeout    time.Duration
	MaxRetries int
}

// LoadCrossEncoderRerankerConfigFromEnv 从环境变量加载精排配置：
//   - RERANK_ENABLED:         是否启用（默认 false，关闭时行为与改动前一致）
//   - RERANK_API_URL:         精排服务地址（Cohere /v2/rerank 或 OpenAI 兼容端点）
//   - RERANK_API_KEY:         鉴权密钥
//   - RERANK_MODEL:           模型名（默认 rerank-v3.5）
//   - RERANK_TOP_N:           参与精排的候选数（默认 20）
//   - RERANK_TIMEOUT_SECONDS: 单次请求超时秒数（默认 30）
//   - RERANK_MAX_RETRIES:     失败重试次数（默认 2）
func LoadCrossEncoderRerankerConfigFromEnv() CrossEncoderRerankerConfig {
	config := CrossEncoderRerankerConfig{
		Enabled:    false,
		APIURL:     strings.TrimSpace(os.Getenv("RERANK_API_URL")),
		APIKey:     strings.TrimSpace(os.Getenv("RERANK_API_KEY")),
		Model:      strings.TrimSpace(os.Getenv("RERANK_MODEL")),
		TopN:       defaultCrossEncoderTopN,
		Timeout:    defaultCrossEncoderTimeout,
		MaxRetries: defaultCrossEncoderMaxRetries,
	}
	if config.Model == "" {
		config.Model = defaultCrossEncoderModel
	}
	if raw, exists := os.LookupEnv("RERANK_ENABLED"); exists {
		if enabled, err := strconv.ParseBool(strings.TrimSpace(raw)); err == nil {
			config.Enabled = enabled
		}
	}
	if raw := strings.TrimSpace(os.Getenv("RERANK_TOP_N")); raw != "" {
		if topN, err := strconv.Atoi(raw); err == nil && topN > 0 {
			config.TopN = topN
		}
	}
	if raw := strings.TrimSpace(os.Getenv("RERANK_TIMEOUT_SECONDS")); raw != "" {
		if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
			config.Timeout = time.Duration(seconds) * time.Second
		}
	}
	if raw := strings.TrimSpace(os.Getenv("RERANK_MAX_RETRIES")); raw != "" {
		if retries, err := strconv.Atoi(raw); err == nil && retries >= 0 {
			config.MaxRetries = retries
		}
	}
	return config
}

// CrossEncoderReranker 云端 cross-encoder 精排客户端。
// nil 值表示精排关闭，调用方据此跳过精排。
type CrossEncoderReranker struct {
	config CrossEncoderRerankerConfig
	client *http.Client
	// degradedReason 非空表示客户端虽被创建（RERANK_ENABLED=true）但配置不完整，
	// 每次精排将直接记录该降级原因并回退，不发起任何请求。
	degradedReason string
}

// NewCrossEncoderRerankerFromEnv 依据环境变量创建精排客户端：
//   - 未启用（RERANK_ENABLED!=true）：返回 nil（调用方跳过精排，行为与改动前一致）。
//   - 已启用但缺 URL/Key 或 URL 非法：返回非 nil 客户端并标记降级原因，
//     运行时记录可区分的 rerank_degraded，不发起请求。
func NewCrossEncoderRerankerFromEnv() *CrossEncoderReranker {
	config := LoadCrossEncoderRerankerConfigFromEnv()
	if !config.Enabled {
		return nil
	}
	reranker := &CrossEncoderReranker{
		config: config,
		client: &http.Client{Timeout: config.Timeout},
	}
	if reason := crossEncoderConfigDegradation(config); reason != "" {
		reranker.degradedReason = reason
		log.Printf("[CrossEncoder精排] RERANK_ENABLED=true 但配置不完整(%s)，精排降级并保留 RRF 排序", reason)
	}
	return reranker
}

// crossEncoderConfigDegradation 返回配置不完整时的降级原因码；配置完整返回空串。
// 校验顺序：URL 为空 > URL 非法 > Key 为空（保证原因可区分）。
func crossEncoderConfigDegradation(config CrossEncoderRerankerConfig) string {
	if strings.TrimSpace(config.APIURL) == "" {
		return rerankDegradedMissingAPIURL
	}
	if !isValidRerankURL(config.APIURL) {
		return rerankDegradedInvalidAPIURL
	}
	if strings.TrimSpace(config.APIKey) == "" {
		return rerankDegradedMissingAPIKey
	}
	return ""
}

// isValidRerankURL 校验精排地址为带 host 的 http/https URL。
func isValidRerankURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	return parsed.Host != ""
}

// rerankRequest 精排请求体（兼容 Cohere v2 与主流 OpenAI 兼容实现）
type rerankRequest struct {
	Model     string   `json:"model"`
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopN      int      `json:"top_n,omitempty"`
}

// rerankResponse 精排响应体：results 按 relevance_score 降序，index 指向请求 documents 的下标
type rerankResponse struct {
	Results []struct {
		Index          int     `json:"index"`
		RelevanceScore float64 `json:"relevance_score"`
	} `json:"results"`
}

// rerankRequestError 携带可区分的降级原因码，供 RerankResults 写入审计元数据
type rerankRequestError struct {
	reason string
	err    error
}

func (e *rerankRequestError) Error() string { return e.err.Error() }
func (e *rerankRequestError) Unwrap() error { return e.err }

func newRerankRequestError(reason string, err error) error {
	return &rerankRequestError{reason: reason, err: err}
}

// rerankDegradedReason 从错误链中提取降级原因码；未标注时兜底为 api_unavailable
func rerankDegradedReason(err error) string {
	var target *rerankRequestError
	if errors.As(err, &target) {
		return target.reason
	}
	return rerankDegradedAPIUnavailable
}

// isRerankTimeoutError 判断错误是否为超时（客户端超时或上下文 DeadlineExceeded）
func isRerankTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}
	return false
}

// RerankResults 对 retrieval.Results 的前 TopN 条候选执行云端 cross-encoder 精排，
// 按 relevance_score 重排并注入审计元数据；失败时返回 error，由调用方降级保留 RRF 排序。
func (r *CrossEncoderReranker) RerankResults(ctx context.Context, query string, retrieval *RetrievalResults) error {
	if r == nil || retrieval == nil {
		return nil
	}

	// 已启用但配置不完整：不发请求，记录可区分的降级原因后保留原顺序
	if r.degradedReason != "" {
		r.recordRerankAudit(retrieval, map[string]interface{}{
			"rerank_applied":  false,
			"rerank_degraded": r.degradedReason,
			"rerank_error":    fmt.Sprintf("精排配置不完整: %s", r.degradedReason),
		})
		return fmt.Errorf("cross-encoder 精排不可用: %s", r.degradedReason)
	}

	// 候选数为 0/1 时无精排意义：不发起请求，直接保持原顺序
	if len(retrieval.Results) <= 1 {
		return nil
	}

	startedAt := time.Now()

	limit := r.config.TopN
	if limit <= 0 || limit > len(retrieval.Results) {
		limit = len(retrieval.Results)
	}

	documents := make([]string, 0, limit)
	for index := 0; index < limit; index++ {
		text := crossEncoderDocumentText(retrieval.Results[index])
		if strings.TrimSpace(text) == "" {
			text = fmt.Sprintf("[document-%d]", index)
		}
		documents = append(documents, text)
	}

	scores, attempts, err := r.callRerankAPI(ctx, query, documents)
	if err != nil {
		r.recordRerankAudit(retrieval, map[string]interface{}{
			"rerank_applied":    false,
			"rerank_degraded":   rerankDegradedReason(err),
			"rerank_error":      r.redact(err.Error()),
			"rerank_attempts":   attempts,
			"rerank_latency_ms": time.Since(startedAt).Milliseconds(),
		})
		return fmt.Errorf("cross-encoder 精排调用失败: %s", r.redact(err.Error()))
	}

	// 仅重排前 limit 条候选，其余保持原有相对顺序
	type rankedIndex struct {
		index int
		score float64
	}
	ranked := make([]rankedIndex, 0, limit)
	for index := 0; index < limit; index++ {
		ranked = append(ranked, rankedIndex{index: index, score: scores[index]})
	}
	sort.SliceStable(ranked, func(left, right int) bool {
		if ranked[left].score == ranked[right].score {
			return ranked[left].index < ranked[right].index
		}
		return ranked[left].score > ranked[right].score
	})

	reorderedResults := make([]interface{}, 0, len(retrieval.Results))
	reorderedSources := make([]string, 0, len(retrieval.Sources))
	for _, item := range ranked {
		reorderedResults = append(reorderedResults, retrieval.Results[item.index])
		if item.index < len(retrieval.Sources) {
			reorderedSources = append(reorderedSources, retrieval.Sources[item.index])
		}
	}
	for index := limit; index < len(retrieval.Results); index++ {
		reorderedResults = append(reorderedResults, retrieval.Results[index])
		if index < len(retrieval.Sources) {
			reorderedSources = append(reorderedSources, retrieval.Sources[index])
		}
	}
	retrieval.Results = reorderedResults
	// Sources 与 Results 为并行数组，仅在长度一致时同步重排，避免破坏下游审计
	if len(reorderedSources) == len(retrieval.Sources) {
		retrieval.Sources = reorderedSources
	}

	r.recordRerankAudit(retrieval, map[string]interface{}{
		"rerank_applied":    true,
		"rerank_model":      r.config.Model,
		"rerank_top_n":      limit,
		"rerank_latency_ms": time.Since(startedAt).Milliseconds(),
	})
	log.Printf("[CrossEncoder精排] 完成 model=%s candidates=%d latency=%dms",
		r.config.Model, limit, time.Since(startedAt).Milliseconds())
	return nil
}

// callRerankAPI 调用云端精排接口，带指数退避重试，返回与 documents 同序的 relevance 分数、
// 实际尝试次数与（失败时的）最后一个错误。超时不重试（重试只会再次超时）。
func (r *CrossEncoderReranker) callRerankAPI(ctx context.Context, query string, documents []string) ([]float64, int, error) {
	payload, err := json.Marshal(rerankRequest{
		Model:     r.config.Model,
		Query:     query,
		Documents: documents,
		TopN:      len(documents),
	})
	if err != nil {
		return nil, 0, newRerankRequestError(rerankDegradedAPIUnavailable, fmt.Errorf("序列化精排请求失败: %w", err))
	}

	attempts := r.config.MaxRetries + 1
	if attempts < 1 {
		attempts = 1
	}
	made := 0
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt) * 500 * time.Millisecond
			select {
			case <-ctx.Done():
				return nil, made, newRerankRequestError(rerankDegradedTimeout, ctx.Err())
			case <-time.After(backoff):
			}
		}
		made++
		scores, err := r.doRerankRequest(ctx, payload, len(documents))
		if err == nil {
			return scores, made, nil
		}
		lastErr = err
		if rerankDegradedReason(err) == rerankDegradedTimeout {
			break
		}
	}
	if lastErr == nil {
		lastErr = newRerankRequestError(rerankDegradedAPIUnavailable, errors.New("精排请求未执行"))
	}
	return nil, made, lastErr
}

// doRerankRequest 执行单次精排 HTTP 请求并解析分数，所有错误均带可区分的降级原因码。
func (r *CrossEncoderReranker) doRerankRequest(ctx context.Context, payload []byte, documentCount int) ([]float64, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, r.config.APIURL, bytes.NewReader(payload))
	if err != nil {
		return nil, newRerankRequestError(rerankDegradedInvalidAPIURL, fmt.Errorf("构造精排请求失败: %w", err))
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+r.config.APIKey)

	response, err := r.client.Do(request)
	if err != nil {
		if isRerankTimeoutError(err) {
			return nil, newRerankRequestError(rerankDegradedTimeout, err)
		}
		return nil, newRerankRequestError(rerankDegradedAPIUnavailable, err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, newRerankRequestError(rerankDegradedInvalidResponse, err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, newRerankRequestError(rerankDegradedHTTPError,
			fmt.Errorf("精排服务返回 %d: %s", response.StatusCode, r.redact(strings.TrimSpace(string(body)))))
	}

	var parsed rerankResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, newRerankRequestError(rerankDegradedInvalidResponse, fmt.Errorf("解析精排响应失败: %w", err))
	}
	if len(parsed.Results) == 0 {
		return nil, newRerankRequestError(rerankDegradedInvalidResponse, errors.New("精排响应为空"))
	}

	scores := make([]float64, documentCount)
	for _, item := range parsed.Results {
		if item.Index >= 0 && item.Index < documentCount {
			scores[item.Index] = item.RelevanceScore
		}
	}
	return scores, nil
}

// crossEncoderDocumentText 从检索结果中提取用于精排的正文文本
func crossEncoderDocumentText(result interface{}) string {
	switch value := result.(type) {
	case *models.VectorMatch:
		if value != nil {
			return value.Content
		}
	case *models.TimelineEvent:
		if value != nil {
			return value.Content
		}
	case *models.KnowledgeNode:
		if value != nil {
			if value.Content != "" {
				return value.Content
			}
			if value.Description != "" {
				return value.Description
			}
			return value.Name
		}
	}
	return ""
}

// redact 将 API Key 从文本中移除，确保密钥绝不出现在日志或审计元数据中
func (r *CrossEncoderReranker) redact(text string) string {
	key := strings.TrimSpace(r.config.APIKey)
	if key == "" || !strings.Contains(text, key) {
		return text
	}
	return strings.ReplaceAll(text, key, "[REDACTED]")
}

// recordRerankAudit 将精排审计字段合并写入检索结果的融合元数据
func (r *CrossEncoderReranker) recordRerankAudit(retrieval *RetrievalResults, fields map[string]interface{}) {
	if retrieval == nil {
		return
	}
	if retrieval.FusionMetadata == nil {
		retrieval.FusionMetadata = make(map[string]interface{})
	}
	for key, value := range fields {
		retrieval.FusionMetadata[key] = value
	}
}
