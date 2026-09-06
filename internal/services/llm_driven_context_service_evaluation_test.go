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
}
