package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/contextkeeper/service/internal/engines/causal_reasoning"
	"github.com/gin-gonic/gin"
)

type recordingCausalGraphWriter struct {
	calls     int
	relations []causal_reasoning.CausalRelation
	err       error
}

func (w *recordingCausalGraphWriter) BuildCausalGraph(_ context.Context, relations []causal_reasoning.CausalRelation) error {
	w.calls++
	w.relations = append([]causal_reasoning.CausalRelation(nil), relations...)
	return w.err
}

func TestExtractCausalRelationsPersistsOnlyWhenRequested(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := &recordingCausalGraphWriter{}
	handler := &CausalReasoningHandler{
		extractor:   causal_reasoning.NewEntityExtractor(nil),
		graphWriter: writer,
	}

	request := httptest.NewRequest(http.MethodPost, "/causal/extract", strings.NewReader(`{"text":"患者服用降压药后出现体位性低血压","persist":true,"use_rules":true}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router := gin.New()
	router.POST("/causal/extract", handler.ExtractCausalRelations)

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if writer.calls != 1 || len(writer.relations) == 0 {
		t.Fatalf("persistence calls = %d, relations = %d; want one non-empty write", writer.calls, len(writer.relations))
	}

	var body causal_reasoning.ExtractResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.GraphPersisted || body.AnalysisOnly {
		t.Fatalf("response persistence flags = persisted:%t analysis_only:%t", body.GraphPersisted, body.AnalysisOnly)
	}
}

func TestExtractCausalRelationsReportsPersistenceFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := &CausalReasoningHandler{
		extractor: causal_reasoning.NewEntityExtractor(nil),
		graphWriter: &recordingCausalGraphWriter{
			err: errors.New("neo4j unavailable"),
		},
	}

	request := httptest.NewRequest(http.MethodPost, "/causal/extract", strings.NewReader(`{"text":"患者服用降压药后出现体位性低血压","persist":true,"use_rules":true}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router := gin.New()
	router.POST("/causal/extract", handler.ExtractCausalRelations)

	router.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}
