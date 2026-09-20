package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/contextkeeper/service/internal/models"
)

// ceTestRetrievalN 构造含 n 条 *models.VectorMatch 的检索结果（Sources 为并行数组）
func ceTestRetrievalN(n int) *RetrievalResults {
	results := make([]interface{}, 0, n)
	sources := make([]string, 0, n)
	for i := 0; i < n; i++ {
		results = append(results, &models.VectorMatch{
			ID:       fmt.Sprintf("id-%d", i),
			Content:  fmt.Sprintf("文档%d", i),
			Score:    1.0 - float64(i)*0.1,
			Metadata: map[string]interface{}{"doc_id": fmt.Sprintf("doc-%d", i)},
		})
		sources = append(sources, fmt.Sprintf("source-%d", i))
	}
	return &RetrievalResults{Results: results, Sources: sources}
}

// ceTestRetrieval 构造含两条候选的检索结果
func ceTestRetrieval() *RetrievalResults { return ceTestRetrievalN(2) }

// ceNewReranker 依据给定配置构造精排客户端（client 超时与配置一致）
func ceNewReranker(cfg CrossEncoderRerankerConfig) *CrossEncoderReranker {
	return &CrossEncoderReranker{config: cfg, client: &http.Client{Timeout: cfg.Timeout}}
}

// ceTestReranker 构造指向给定 mock 服务地址的精排客户端（关闭重试，避免测试等待退避）
func ceTestReranker(serverURL string) *CrossEncoderReranker {
	return ceNewReranker(CrossEncoderRerankerConfig{
		Enabled:    true,
		APIURL:     serverURL,
		APIKey:     "test-key",
		Model:      "test-model",
		TopN:       20,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
	})
}

// TestCrossEncoderRerankSuccess 成功路径：按 relevance_score 重排 Results/Sources 并写审计元数据
func TestCrossEncoderRerankSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []map[string]interface{}{
				{"index": 1, "relevance_score": 0.9},
				{"index": 0, "relevance_score": 0.1},
			},
		})
	}))
	defer server.Close()

	reranker := ceTestReranker(server.URL)
	retrieval := ceTestRetrieval()

	if err := reranker.RerankResults(context.Background(), "测试查询", retrieval); err != nil {
		t.Fatalf("精排应成功，实际错误: %v", err)
	}
	if len(retrieval.Results) != 2 {
		t.Fatalf("结果数应保持不变，实际 %d", len(retrieval.Results))
	}
	if first := retrieval.Results[0].(*models.VectorMatch); first.ID != "id-1" {
		t.Fatalf("index=1(得分0.9) 的候选应升到首位，实际 %v", first.ID)
	}
	if second := retrieval.Results[1].(*models.VectorMatch); second.ID != "id-0" {
		t.Fatalf("index=0(得分0.1) 的候选应降到次位，实际 %v", second.ID)
	}
	if retrieval.Sources[0] != "source-1" || retrieval.Sources[1] != "source-0" {
		t.Errorf("Sources 应与 Results 并行重排，实际 %v", retrieval.Sources)
	}
	if retrieval.FusionMetadata["rerank_applied"] != true {
		t.Errorf("rerank_applied 应为 true，实际 %v", retrieval.FusionMetadata["rerank_applied"])
	}
}

// TestCrossEncoderRerankTopNN 校验只有前 TopN 条参与重排，top-N 之外不被改动
func TestCrossEncoderRerankTopNN(t *testing.T) {
	var requestedDocs int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rerankRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		atomic.StoreInt32(&requestedDocs, int32(len(req.Documents)))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []map[string]interface{}{
				{"index": 1, "relevance_score": 0.9},
				{"index": 0, "relevance_score": 0.1},
			},
		})
	}))
	defer server.Close()

	cfg := CrossEncoderRerankerConfig{
		Enabled: true, APIURL: server.URL, APIKey: "test-key",
		Model: "test-model", TopN: 2, Timeout: 5 * time.Second, MaxRetries: 0,
	}
	reranker := ceNewReranker(cfg)
	retrieval := ceTestRetrievalN(3)

	if err := reranker.RerankResults(context.Background(), "测试查询", retrieval); err != nil {
		t.Fatalf("精排应成功，实际错误: %v", err)
	}
	if got := atomic.LoadInt32(&requestedDocs); got != 2 {
		t.Fatalf("应仅对 TopN=2 条候选发起精排，实际请求文档数=%d", got)
	}
	order := []string{
		retrieval.Results[0].(*models.VectorMatch).ID,
		retrieval.Results[1].(*models.VectorMatch).ID,
		retrieval.Results[2].(*models.VectorMatch).ID,
	}
	// 前 2 条按精排分数重排为 [id-1, id-0]，第 3 条（id-2）保持原位
	if order[0] != "id-1" || order[1] != "id-0" || order[2] != "id-2" {
		t.Fatalf("top-N 之外不应被改动，实际顺序 %v", order)
	}
}

// TestCrossEncoderRerankDisabledNoRequest 关闭直通：Enabled=false 时不发任何请求
func TestCrossEncoderRerankDisabledNoRequest(t *testing.T) {
	var calls int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&calls, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	t.Setenv("RERANK_API_URL", server.URL)
	t.Setenv("RERANK_API_KEY", "dummy")
	t.Setenv("RERANK_ENABLED", "false")

	reranker := NewCrossEncoderRerankerFromEnv()
	if reranker != nil {
		t.Fatalf("RERANK_ENABLED=false 时应返回 nil，实际 %+v", reranker)
	}
	retrieval := ceTestRetrieval()
	if err := reranker.RerankResults(context.Background(), "q", retrieval); err != nil {
		t.Fatalf("nil 客户端应为空操作，实际错误: %v", err)
	}
	if atomic.LoadInt64(&calls) != 0 {
		t.Errorf("关闭时不得发起请求，实际 %d 次", calls)
	}
	if retrieval.FusionMetadata != nil {
		t.Errorf("未启用时不应写入任何 rerank 审计元数据，实际 %v", retrieval.FusionMetadata)
	}
	if retrieval.Results[0].(*models.VectorMatch).ID != "id-0" {
		t.Error("未启用时不应改变结果顺序")
	}
}

// TestCrossEncoderRerankDisabledFromEnv 校验 RERANK_ENABLED 解析语义
func TestCrossEncoderRerankDisabledFromEnv(t *testing.T) {
	t.Setenv("RERANK_API_URL", "http://127.0.0.1:1/rerank")
	t.Setenv("RERANK_API_KEY", "dummy")

	t.Setenv("RERANK_ENABLED", "false")
	if reranker := NewCrossEncoderRerankerFromEnv(); reranker != nil {
		t.Errorf("RERANK_ENABLED=false 时应返回 nil，实际 %+v", reranker)
	}

	t.Setenv("RERANK_ENABLED", "")
	if reranker := NewCrossEncoderRerankerFromEnv(); reranker != nil {
		t.Errorf("RERANK_ENABLED 未启用时应返回 nil，实际 %+v", reranker)
	}

	t.Setenv("RERANK_ENABLED", "true")
	if reranker := NewCrossEncoderRerankerFromEnv(); reranker == nil {
		t.Error("RERANK_ENABLED=true 且配置完整时应返回非 nil 客户端")
	}
}

// TestCrossEncoderRerankDegradedMissingAPIKey 已启用但缺 API Key：不发请求，原因 missing_api_key
func TestCrossEncoderRerankDegradedMissingAPIKey(t *testing.T) {
	var calls int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&calls, 1)
	}))
	defer server.Close()

	t.Setenv("RERANK_ENABLED", "true")
	t.Setenv("RERANK_API_URL", server.URL)
	t.Setenv("RERANK_API_KEY", "")

	reranker := NewCrossEncoderRerankerFromEnv()
	if reranker == nil {
		t.Fatal("已启用但配置不完整时应返回非 nil 客户端以留下降级原因")
	}
	retrieval := ceTestRetrieval()
	if err := reranker.RerankResults(context.Background(), "q", retrieval); err == nil {
		t.Error("配置不完整时应返回错误")
	}
	if got := retrieval.FusionMetadata["rerank_degraded"]; got != "missing_api_key" {
		t.Errorf("rerank_degraded 应为 missing_api_key，实际 %v", got)
	}
	if retrieval.FusionMetadata["rerank_applied"] != false {
		t.Errorf("rerank_applied 应为 false，实际 %v", retrieval.FusionMetadata["rerank_applied"])
	}
	if atomic.LoadInt64(&calls) != 0 {
		t.Errorf("配置不完整时不得发起请求，实际 %d 次", calls)
	}
	if retrieval.Results[0].(*models.VectorMatch).ID != "id-0" {
		t.Error("降级时应保持原顺序")
	}
}

// TestCrossEncoderRerankDegradedMissingURL 已启用但 URL 为空：原因 missing_api_url
func TestCrossEncoderRerankDegradedMissingURL(t *testing.T) {
	t.Setenv("RERANK_ENABLED", "true")
	t.Setenv("RERANK_API_URL", "")
	t.Setenv("RERANK_API_KEY", "dummy")

	reranker := NewCrossEncoderRerankerFromEnv()
	if reranker == nil {
		t.Fatal("已启用但配置不完整时应返回非 nil 客户端")
	}
	retrieval := ceTestRetrieval()
	if err := reranker.RerankResults(context.Background(), "q", retrieval); err == nil {
		t.Error("URL 为空时应返回错误")
	}
	if got := retrieval.FusionMetadata["rerank_degraded"]; got != "missing_api_url" {
		t.Errorf("rerank_degraded 应为 missing_api_url，实际 %v", got)
	}
	if retrieval.Results[0].(*models.VectorMatch).ID != "id-0" {
		t.Error("降级时应保持原顺序")
	}
}

// TestCrossEncoderRerankDegradedInvalidURL 已启用但 URL 非法：原因 invalid_api_url
func TestCrossEncoderRerankDegradedInvalidURL(t *testing.T) {
	t.Setenv("RERANK_ENABLED", "true")
	t.Setenv("RERANK_API_URL", "://not-a-url")
	t.Setenv("RERANK_API_KEY", "dummy")

	reranker := NewCrossEncoderRerankerFromEnv()
	if reranker == nil {
		t.Fatal("已启用但配置不完整时应返回非 nil 客户端")
	}
	retrieval := ceTestRetrieval()
	if err := reranker.RerankResults(context.Background(), "q", retrieval); err == nil {
		t.Error("URL 非法时应返回错误")
	}
	if got := retrieval.FusionMetadata["rerank_degraded"]; got != "invalid_api_url" {
		t.Errorf("rerank_degraded 应为 invalid_api_url，实际 %v", got)
	}
	if retrieval.Results[0].(*models.VectorMatch).ID != "id-0" {
		t.Error("降级时应保持原顺序")
	}
}

// TestCrossEncoderRerankDegradedOnHTTPError HTTP 非 2xx：原因 http_error
func TestCrossEncoderRerankDegradedOnHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	reranker := ceTestReranker(server.URL)
	retrieval := ceTestRetrieval()

	err := reranker.RerankResults(context.Background(), "测试查询", retrieval)
	if err == nil {
		t.Fatal("接口返回 500 时应返回错误")
	}
	if retrieval.FusionMetadata["rerank_degraded"] != "http_error" {
		t.Errorf("rerank_degraded 应为 http_error，实际 %v", retrieval.FusionMetadata["rerank_degraded"])
	}
	if retrieval.FusionMetadata["rerank_applied"] != false {
		t.Errorf("rerank_applied 应为 false，实际 %v", retrieval.FusionMetadata["rerank_applied"])
	}
	if retrieval.Results[0].(*models.VectorMatch).ID != "id-0" {
		t.Error("降级时不应改变结果顺序")
	}
}

// TestCrossEncoderRerankDegradedInvalidResponse 响应体非法：原因 invalid_response
func TestCrossEncoderRerankDegradedInvalidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not-a-valid-json"))
	}))
	defer server.Close()

	reranker := ceTestReranker(server.URL)
	retrieval := ceTestRetrieval()

	err := reranker.RerankResults(context.Background(), "测试查询", retrieval)
	if err == nil {
		t.Fatal("响应体非法时应返回错误")
	}
	if retrieval.FusionMetadata["rerank_degraded"] != "invalid_response" {
		t.Errorf("rerank_degraded 应为 invalid_response，实际 %v", retrieval.FusionMetadata["rerank_degraded"])
	}
	if retrieval.Results[0].(*models.VectorMatch).ID != "id-0" {
		t.Error("降级时不应改变结果顺序")
	}
}

// TestCrossEncoderRerankDegradedTimeout 超时：原因 timeout
func TestCrossEncoderRerankDegradedTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"results": []map[string]interface{}{{"index": 0, "relevance_score": 1.0}}})
	}))
	defer server.Close()

	reranker := ceNewReranker(CrossEncoderRerankerConfig{
		Enabled: true, APIURL: server.URL, APIKey: "test-key",
		Model: "test-model", TopN: 20, Timeout: 100 * time.Millisecond, MaxRetries: 0,
	})
	retrieval := ceTestRetrieval()

	err := reranker.RerankResults(context.Background(), "测试查询", retrieval)
	if err == nil {
		t.Fatal("超时时应返回错误")
	}
	if retrieval.FusionMetadata["rerank_degraded"] != "timeout" {
		t.Errorf("rerank_degraded 应为 timeout，实际 %v", retrieval.FusionMetadata["rerank_degraded"])
	}
	if retrieval.Results[0].(*models.VectorMatch).ID != "id-0" {
		t.Error("降级时不应改变结果顺序")
	}
}

// TestCrossEncoderRerankDegradedRetriesExhausted 重试耗尽：保留底层原因并记录尝试次数
func TestCrossEncoderRerankDegradedRetriesExhausted(t *testing.T) {
	var calls int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&calls, 1)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	reranker := ceNewReranker(CrossEncoderRerankerConfig{
		Enabled: true, APIURL: server.URL, APIKey: "test-key",
		Model: "test-model", TopN: 20, Timeout: 5 * time.Second, MaxRetries: 2,
	})
	retrieval := ceTestRetrieval()

	err := reranker.RerankResults(context.Background(), "测试查询", retrieval)
	if err == nil {
		t.Fatal("重试耗尽时应返回错误")
	}
	if got := retrieval.FusionMetadata["rerank_degraded"]; got != "http_error" {
		t.Errorf("重试耗尽保留底层原因 http_error，实际 %v", got)
	}
	if got := retrieval.FusionMetadata["rerank_attempts"]; got != 3 {
		t.Errorf("尝试次数应为 MaxRetries+1=3，实际 %v", got)
	}
	if atomic.LoadInt64(&calls) != 3 {
		t.Errorf("应实际发起 3 次请求，实际 %d", calls)
	}
	if retrieval.Results[0].(*models.VectorMatch).ID != "id-0" {
		t.Error("降级时不应改变结果顺序")
	}
}

// TestCrossEncoderRerankSkipsInsufficientCandidates 候选 0/1 条时不得发起请求
func TestCrossEncoderRerankSkipsInsufficientCandidates(t *testing.T) {
	var calls int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&calls, 1)
	}))
	defer server.Close()

	reranker := ceTestReranker(server.URL)

	for _, n := range []int{0, 1} {
		retrieval := ceTestRetrievalN(n)
		if err := reranker.RerankResults(context.Background(), "q", retrieval); err != nil {
			t.Fatalf("候选数=%d 应为空操作，实际错误: %v", n, err)
		}
		if len(retrieval.Results) != n {
			t.Fatalf("候选数=%d 不应被改动，实际 %d", n, len(retrieval.Results))
		}
	}
	if atomic.LoadInt64(&calls) != 0 {
		t.Errorf("候选数 0/1 时不得发起请求，实际 %d 次", calls)
	}
}

// TestCrossEncoderRerankDoesNotLeakAPIKey 密钥绝不进入返回错误或审计元数据
func TestCrossEncoderRerankDoesNotLeakAPIKey(t *testing.T) {
	const secret = "secret-key-should-never-leak"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 恶意/异常服务把收到的 Authorization 回显到响应体
		http.Error(w, "echo:"+r.Header.Get("Authorization"), http.StatusBadRequest)
	}))
	defer server.Close()

	reranker := ceNewReranker(CrossEncoderRerankerConfig{
		Enabled: true, APIURL: server.URL, APIKey: secret,
		Model: "test-model", TopN: 20, Timeout: 5 * time.Second, MaxRetries: 0,
	})
	retrieval := ceTestRetrieval()

	err := reranker.RerankResults(context.Background(), "q", retrieval)
	if err == nil {
		t.Fatal("400 时应返回错误")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("返回错误泄露了 API Key: %s", err.Error())
	}
	if msg, ok := retrieval.FusionMetadata["rerank_error"].(string); ok && strings.Contains(msg, secret) {
		t.Errorf("审计元数据泄露了 API Key: %s", msg)
	}
}
