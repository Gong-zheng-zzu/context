package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/contextkeeper/service/internal/models"
)

// ceTestRetrieval 构造含两条 *models.VectorMatch 的检索结果
func ceTestRetrieval() *RetrievalResults {
	return &RetrievalResults{
		Results: []interface{}{
			&models.VectorMatch{ID: "id-0", Content: "第一个文档", Score: 0.9, Metadata: map[string]interface{}{"doc_id": "doc-0"}},
			&models.VectorMatch{ID: "id-1", Content: "第二个文档", Score: 0.4, Metadata: map[string]interface{}{"doc_id": "doc-1"}},
		},
		Sources: []string{"vector", "vector"},
	}
}

// ceTestReranker 构造指向给定 mock 服务地址的精排客户端（关闭重试，避免测试等待退避）
func ceTestReranker(serverURL string) *CrossEncoderReranker {
	return &CrossEncoderReranker{
		config: CrossEncoderRerankerConfig{
			Enabled:    true,
			APIURL:     serverURL,
			APIKey:     "test-key",
			Model:      "test-model",
			TopN:       20,
			Timeout:    5 * time.Second,
			MaxRetries: 0,
		},
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

// TestCrossEncoderRerankSuccess 校验成功路径：按 relevance_score 重排并写入审计元数据
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
	first, ok := retrieval.Results[0].(*models.VectorMatch)
	if !ok || first.ID != "id-1" {
		t.Fatalf("index=1(得分0.9) 的候选应升到首位，实际 %v", retrieval.Results[0])
	}
	second, ok := retrieval.Results[1].(*models.VectorMatch)
	if !ok || second.ID != "id-0" {
		t.Fatalf("index=0(得分0.1) 的候选应降到次位，实际 %v", retrieval.Results[1])
	}
	if retrieval.FusionMetadata["rerank_applied"] != true {
		t.Errorf("rerank_applied 应为 true，实际 %v", retrieval.FusionMetadata["rerank_applied"])
	}
}

// TestCrossEncoderRerankDegradedOnAPIError 校验失败降级：返回 error 且标记 api_unavailable
func TestCrossEncoderRerankDegradedOnAPIError(t *testing.T) {
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
	if retrieval.FusionMetadata["rerank_degraded"] != "api_unavailable" {
		t.Errorf("rerank_degraded 应为 api_unavailable，实际 %v", retrieval.FusionMetadata["rerank_degraded"])
	}
	if retrieval.FusionMetadata["rerank_applied"] != false {
		t.Errorf("rerank_applied 应为 false，实际 %v", retrieval.FusionMetadata["rerank_applied"])
	}
	// 降级时必须保留原顺序
	if retrieval.Results[0].(*models.VectorMatch).ID != "id-0" {
		t.Error("降级时不应改变结果顺序")
	}
}

// TestCrossEncoderRerankDisabledFromEnv 校验关闭直通：RERANK_ENABLED 未启用或 false 时返回 nil
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
