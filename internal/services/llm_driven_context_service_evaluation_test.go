package services

import (
	"context"
	"reflect"
	"testing"

	"github.com/contextkeeper/service/internal/models"
)

type evaluationFastPathRetriever struct {
	calls   int
	queries *RetrievalQueries
}

func (r *evaluationFastPathRetriever) ParallelRetrieve(_ context.Context, queries *RetrievalQueries) (*RetrievalResults, error) {
	r.calls++
	copy := *queries
	copy.TimelineQueries = append([]string(nil), queries.TimelineQueries...)
	copy.KnowledgeQueries = append([]string(nil), queries.KnowledgeQueries...)
	copy.VectorQueries = append([]string(nil), queries.VectorQueries...)
	r.queries = &copy

	return &RetrievalResults{
		Results: []interface{}{
			&models.VectorMatch{Content: "vector content", Score: 0.9, Metadata: map[string]interface{}{"doc_id": "doc-1"}},
			&models.KnowledgeNode{Content: "knowledge content", Score: 0.8, Properties: map[string]interface{}{"doc_ids": []string{"doc-1"}}},
			&models.TimelineEvent{ID: "event-1", SourceDocID: "doc-1", Content: "timeline content", RelevanceScore: 0.7},
		},
		Sources: []string{"vector", "knowledge", "timeline"},
		SourceStatuses: map[string]string{
			"vector": "success", "knowledge": "success", "timeline": "success",
		},
		SourceLatencyMs:    map[string]int64{"vector": 11, "knowledge": 7, "timeline": 5},
		WallClockLatencyMs: 12,
	}, nil
}

func (r *evaluationFastPathRetriever) GetTimelineAdapter() TimelineAdapter {
	return nil
}

func (r *evaluationFastPathRetriever) DirectTimelineQuery(context.Context, *models.TimelineSearchRequest) ([]*models.TimelineEvent, error) {
	return nil, nil
}

func TestEvaluationRetrievalOnlyUsesDirectRRFPath(t *testing.T) {
	retriever := &evaluationFastPathRetriever{}
	service := &LLMDrivenContextService{
		enabled:        true,
		config:         &LLMDrivenConfig{SemanticAnalysis: true, MultiDimensional: true, ContentSynthesis: true},
		multiRetriever: retriever,
		metrics:        &LLMDrivenMetrics{},
	}

	response, err := service.RetrieveContext(context.Background(), models.RetrieveContextRequest{
		SessionID:               "evaluation-session",
		UserID:                  "evaluation-user",
		Query:                   "original query",
		Limit:                   5,
		EvaluationRetrievalOnly: true,
	})
	if err != nil {
		t.Fatalf("RetrieveContext() error = %v", err)
	}
	if retriever.calls != 1 {
		t.Fatalf("ParallelRetrieve calls = %d, want 1", retriever.calls)
	}
	wantQueries := []string{"original query"}
	if !reflect.DeepEqual(retriever.queries.TimelineQueries, wantQueries) ||
		!reflect.DeepEqual(retriever.queries.KnowledgeQueries, wantQueries) ||
		!reflect.DeepEqual(retriever.queries.VectorQueries, wantQueries) {
		t.Fatalf("evaluation queries = %#v, want original query in all three lanes", retriever.queries)
	}
	if response.SessionState != "evaluation_retrieval_only" || len(response.Contexts) != 1 {
		t.Fatalf("unexpected evaluation response: %#v", response)
	}
	contextItem := response.Contexts[0]
	if contextItem.DocID != "doc-1" || contextItem.ID != "doc-1" {
		t.Fatalf("context lacks verifiable doc id: %#v", contextItem)
	}
	if got, want := response.RetrievalMetadata["retrieval_execution_path"], "direct_rrf"; got != want {
		t.Fatalf("retrieval_execution_path = %#v, want %#v", got, want)
	}
	if got, want := contextItem.Metadata["rrf_sources"], []string{"knowledge", "timeline", "vector"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("rrf_sources = %#v, want %#v", got, want)
	}
	if got, want := response.RetrievalMetadata["source_statuses"], map[string]string{"vector": "success", "knowledge": "success", "timeline": "success"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("source_statuses = %#v, want %#v", got, want)
	}
	if got, want := response.RetrievalMetadata["source_latency_ms"], map[string]int64{"vector": 11, "knowledge": 7, "timeline": 5}; !reflect.DeepEqual(got, want) {
		t.Fatalf("source_latency_ms = %#v, want %#v", got, want)
	}
	if got, want := response.RetrievalMetadata["wall_clock_latency_ms"], int64(12); got != want {
		t.Fatalf("wall_clock_latency_ms = %#v, want %#v", got, want)
	}
	if got, want := contextItem.Metadata["doc_id"], "doc-1"; got != want {
		t.Fatalf("context doc_id metadata = %#v, want %#v", got, want)
	}
	if got, want := contextItem.Metadata["retrieval_wall_clock_latency_ms"], int64(12); got != want {
		t.Fatalf("context wall-clock latency = %#v, want %#v", got, want)
	}
}

func TestBuildEvaluationRRFResponseFusesDuplicateDocumentWithAuditableRanks(t *testing.T) {
	response := buildEvaluationRRFResponse(&RetrievalResults{
		Results: []interface{}{
			&models.VectorMatch{ID: "doc-shared", Content: "vector", Score: 0.9},
			&models.VectorMatch{ID: "doc-shared", Content: "vector duplicate", Score: 0.8},
			&models.VectorMatch{ID: "doc-vector", Content: "vector-only", Score: 0.8},
			&models.KnowledgeNode{ID: "node-shared", Content: "knowledge", Score: 0.7, Properties: map[string]interface{}{"doc_ids": []string{"doc-shared"}}},
			&models.TimelineEvent{ID: "event-shared", SourceDocID: "doc-shared", Content: "timeline", RelevanceScore: 0.6},
		},
		// Sources is positional and must contain one entry per result. The
		// vector-only candidate is intentionally included to verify deduplication
		// without misattributing later graph/timeline evidence.
		Sources: []string{"vector", "vector", "vector", "knowledge", "timeline"},
		SourceStatuses: map[string]string{
			"vector": "success", "knowledge": "success", "timeline": "success",
		},
		SourceLatencyMs:    map[string]int64{"vector": 15, "knowledge": 9, "timeline": 4},
		WallClockLatencyMs: 16,
	}, 5)

	if got, want := response.RetrievalMetadata["retrieval_fusion_mode"], "rrf_3_sources"; got != want {
		t.Fatalf("retrieval_fusion_mode = %#v, want %#v", got, want)
	}
	if len(response.Contexts) != 2 {
		t.Fatalf("fused contexts = %d, want 2 deduplicated documents", len(response.Contexts))
	}
	shared := response.Contexts[0]
	if shared.DocID != "doc-shared" || shared.Source != "rrf" {
		t.Fatalf("top fused document = %#v, want doc-shared from RRF", shared)
	}
	if got, want := shared.Metadata["rrf_sources"], []string{"knowledge", "timeline", "vector"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("rrf_sources = %#v, want %#v", got, want)
	}
	if got, want := shared.Metadata["rrf_ranks"], map[string]int{"vector": 1, "knowledge": 1, "timeline": 1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("rrf_ranks = %#v, want %#v", got, want)
	}
	if _, ok := shared.Metadata["rrf_score"].(float64); !ok {
		t.Fatalf("rrf_score = %#v, want float64", shared.Metadata["rrf_score"])
	}
	if got, want := response.RetrievalMetadata["source_candidate_counts"], map[string]int{"vector": 2, "knowledge": 1, "timeline": 1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("source_candidate_counts = %#v, want %#v", got, want)
	}
	if got, want := shared.Metadata["retrieval_source_candidate_counts"], map[string]int{"vector": 2, "knowledge": 1, "timeline": 1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("context source_candidate_counts = %#v, want %#v", got, want)
	}
}

func TestBuildEvaluationRRFResponseCountsOnlyTraceableCandidates(t *testing.T) {
	response := buildEvaluationRRFResponse(&RetrievalResults{
		Results: []interface{}{
			&models.VectorMatch{ID: "vector-doc", Score: 0.9},
			&models.KnowledgeNode{ID: "node-without-doc-id", Score: 0.8},
			&models.TimelineEvent{ID: "event-without-doc-id", RelevanceScore: 0.7},
		},
		Sources: []string{"vector", "knowledge", "timeline"},
	}, 5)

	if got, want := response.RetrievalMetadata["source_candidate_counts"], map[string]int{"vector": 1, "knowledge": 0, "timeline": 0}; !reflect.DeepEqual(got, want) {
		t.Fatalf("source_candidate_counts = %#v, want %#v", got, want)
	}
	if got, want := response.RetrievalMetadata["retrieval_fusion_mode"], "vector_only_fallback"; got != want {
		t.Fatalf("retrieval_fusion_mode = %#v, want %#v", got, want)
	}
}

func TestBuildEvaluationRRFResponseReportsSingleSourceFallback(t *testing.T) {
	response := buildEvaluationRRFResponse(&RetrievalResults{
		Results: []interface{}{
			&models.VectorMatch{ID: "vector-document", Content: "vector evidence", Score: 0.9},
		},
		Sources: []string{"vector"},
		SourceStatuses: map[string]string{
			"vector": "success", "knowledge": "failure", "timeline": "skipped",
		},
		SourceLatencyMs:    map[string]int64{"vector": 8, "knowledge": 3, "timeline": 0},
		WallClockLatencyMs: 9,
	}, 5)

	if got, want := response.RetrievalMetadata["retrieval_fusion_mode"], "vector_only_fallback"; got != want {
		t.Fatalf("retrieval_fusion_mode = %#v, want %#v", got, want)
	}
	if got, want := response.RetrievalMetadata["source_statuses"], map[string]string{"vector": "success", "knowledge": "failure", "timeline": "skipped"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("source_statuses = %#v, want %#v", got, want)
	}
	if len(response.Contexts) != 1 || response.Contexts[0].DocID != "vector-document" || response.Contexts[0].Source != "vector" {
		t.Fatalf("single-source response = %#v", response.Contexts)
	}
	metadata := response.Contexts[0].Metadata
	if got, want := metadata["retrieval_empty_sources"], []string{"knowledge", "timeline"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("retrieval_empty_sources = %#v, want %#v", got, want)
	}
	if got, want := metadata["retrieval_wall_clock_latency_ms"], int64(9); got != want {
		t.Fatalf("retrieval_wall_clock_latency_ms = %#v, want %#v", got, want)
	}
}

// TestMarkEvaluationVectorFallbackEmitsEvidenceContractWithoutResults 覆盖基线配置
// （关闭图谱/时间线）走的向量回退路径：即使没有结果也必须下发完整证据字段，
// 否则门禁的 retrieval contract 检查会失败，使基线配置无法产出 eligible 结果。
func TestMarkEvaluationVectorFallbackEmitsEvidenceContractWithoutResults(t *testing.T) {
	response := markEvaluationVectorFallback(models.ContextResponse{}, 42)
	if got, want := response.RetrievalMetadata["retrieval_fusion_mode"], "vector_only_fallback"; got != want {
		t.Fatalf("retrieval_fusion_mode = %#v, want %#v", got, want)
	}
	if got, want := response.RetrievalMetadata["source_statuses"], map[string]string{"vector": "empty", "knowledge": "skipped", "timeline": "skipped"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("source_statuses = %#v, want %#v", got, want)
	}
	if got, want := response.RetrievalMetadata["source_candidate_counts"], map[string]int{"vector": 0, "knowledge": 0, "timeline": 0}; !reflect.DeepEqual(got, want) {
		t.Fatalf("source_candidate_counts = %#v, want %#v", got, want)
	}
	// 证据契约要求这两个字段必须存在；图谱/时间线被配置显式关闭，延迟如实记为 0。
	if got, want := response.RetrievalMetadata["source_latency_ms"], map[string]int64{"vector": 42, "knowledge": 0, "timeline": 0}; !reflect.DeepEqual(got, want) {
		t.Fatalf("source_latency_ms = %#v, want %#v", got, want)
	}
	if got, want := response.RetrievalMetadata["wall_clock_latency_ms"], int64(42); got != want {
		t.Fatalf("wall_clock_latency_ms = %#v, want %#v", got, want)
	}
	// 零结果时 Contexts/RetrievedContexts 必须是非 nil 空切片：nil slice 会被
	// encoding/json 序列化成 `null`，使 smoke 契约的 contexts 列表检查失败。
	if response.Contexts == nil {
		t.Fatal("Contexts must be a non-nil empty slice when there are no results")
	}
	if len(response.Contexts) != 0 {
		t.Fatalf("Contexts = %#v, want empty", response.Contexts)
	}
	if response.RetrievedContexts == nil {
		t.Fatal("RetrievedContexts must be a non-nil empty slice when there are no results")
	}
	if len(response.RetrievedContexts) != 0 {
		t.Fatalf("RetrievedContexts = %#v, want empty", response.RetrievedContexts)
	}
}

// TestMarkEvaluationVectorFallbackPropagatesLatencyToContexts 确认逐条上下文也带上与
// RRF 通道同名的审计字段，使两条通道的证据结构保持一致。
func TestMarkEvaluationVectorFallbackPropagatesLatencyToContexts(t *testing.T) {
	response := markEvaluationVectorFallback(models.ContextResponse{
		Contexts: []models.ContextItem{{DocID: "doc-1", Source: "vector"}},
	}, 17)

	if len(response.Contexts) != 1 {
		t.Fatalf("contexts = %#v", response.Contexts)
	}
	metadata := response.Contexts[0].Metadata
	if got, want := metadata["retrieval_source_latency_ms"], map[string]int64{"vector": 17, "knowledge": 0, "timeline": 0}; !reflect.DeepEqual(got, want) {
		t.Fatalf("retrieval_source_latency_ms = %#v, want %#v", got, want)
	}
	if got, want := metadata["retrieval_wall_clock_latency_ms"], int64(17); got != want {
		t.Fatalf("retrieval_wall_clock_latency_ms = %#v, want %#v", got, want)
	}
}
