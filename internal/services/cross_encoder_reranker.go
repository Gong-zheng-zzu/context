package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
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
// 降级策略：未启用（RERANK_ENABLED!=true）时 NewCrossEncoderRerankerFromEnv
// 返回 nil，调用方直接跳过精排，保留 RRF 排序；调用失败时返回 error，调用方
// 记录降级原因并保留 RRF 排序，不阻断主流程。
//
// 审计：精排状态写入 RetrievalResults.FusionMetadata（rerank_applied /
// rerank_model / rerank_top_n / rerank_latency_ms，失败时 rerank_degraded）。
// =============================================================================

const (
	defaultCrossEncoderTopN       = 20
	defaultCrossEncoderTimeout    = 30 * time.Second
	defaultCrossEncoderMaxRetries = 2
	defaultCrossEncoderModel      = "rerank-v3.5"
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
// nil 值表示精排关闭或配置不完整，调用方据此跳过精排。
type CrossEncoderReranker struct {
	config CrossEncoderRerankerConfig
	client *http.Client
}

// NewCrossEncoderRerankerFromEnv 依据环境变量创建精排客户端。
// 未启用（RERANK_ENABLED!=true）或缺少 API 地址/密钥时返回 nil（表示关闭或降级）。
func NewCrossEncoderRerankerFromEnv() *CrossEncoderReranker {
	config := LoadCrossEncoderRerankerConfigFromEnv()
	if !config.Enabled {
		return nil
	}
	if config.APIURL == "" || config.APIKey == "" {
		log.Printf("[CrossEncoder精排] RERANK_ENABLED=true 但缺少 RERANK_API_URL/RERANK_API_KEY，精排关闭")
		return nil
	}
	return &CrossEncoderReranker{
		config: config,
		client: &http.Client{Timeout: config.Timeout},
	}
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

// RerankResults 对 retrieval.Results 的前 TopN 条候选执行云端 cross-encoder 精排，
// 按 relevance_score 重排并注入审计元数据；失败时返回错误，由调用方降级保留 RRF 排序。
func (r *CrossEncoderReranker) RerankResults(ctx context.Context, query string, retrieval *RetrievalResults) error {
	if r == nil || retrieval == nil || len(retrieval.Results) == 0 {
		return nil
	}
	startedAt := time.Now()

	limit := r.config.TopN
	if limit > len(retrieval.Results) {
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

	scores, err := r.callRerankAPI(ctx, query, documents)
	if err != nil {
		r.recordRerankAudit(retrieval, map[string]interface{}{
			"rerank_applied":    false,
			"rerank_degraded":   "api_unavailable",
			"rerank_error":      err.Error(),
			"rerank_latency_ms": time.Since(startedAt).Milliseconds(),
		})
		return fmt.Errorf("cross-encoder 精排调用失败: %w", err)
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

// callRerankAPI 调用云端精排接口，带指数退避重试，返回与 documents 同序的 relevance 分数。
func (r *CrossEncoderReranker) callRerankAPI(ctx context.Context, query string, documents []string) ([]float64, error) {
	payload, err := json.Marshal(rerankRequest{
		Model:     r.config.Model,
		Query:     query,
		Documents: documents,
		TopN:      len(documents),
	})
	if err != nil {
		return nil, fmt.Errorf("序列化精排请求失败: %w", err)
	}

	var lastErr error
	attempts := r.config.MaxRetries + 1
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt) * 500 * time.Millisecond
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}
		scores, err := r.doRerankRequest(ctx, payload, len(documents))
		if err == nil {
			return scores, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// doRerankRequest 执行单次精排 HTTP 请求并解析分数
func (r *CrossEncoderReranker) doRerankRequest(ctx context.Context, payload []byte, documentCount int) ([]float64, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, r.config.APIURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+r.config.APIKey)

	response, err := r.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("精排服务返回 %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed rerankResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("解析精排响应失败: %w", err)
	}
	if len(parsed.Results) == 0 {
		return nil, errors.New("精排响应为空")
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
