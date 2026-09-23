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

func TestExtractCausalRelationsIsAnalysisOnlyWithoutPersistFlag(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := &recordingCausalGraphWriter{}
	handler := &CausalReasoningHandler{
		extractor:   causal_reasoning.NewEntityExtractor(nil),
		graphWriter: writer,
	}

	request := httptest.NewRequest(http.MethodPost, "/causal/extract", strings.NewReader(`{"text":"患者服用降压药后出现体位性低血压","use_rules":true}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router := gin.New()
	router.POST("/causal/extract", handler.ExtractCausalRelations)

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if writer.calls != 0 {
		t.Fatalf("persistence calls = %d; want no write for analysis-only request", writer.calls)
	}

	var body causal_reasoning.ExtractResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.AnalysisOnly || body.GraphPersisted {
		t.Fatalf("response persistence flags = persisted:%t analysis_only:%t", body.GraphPersisted, body.AnalysisOnly)
	}
	if body.Quality == nil || body.Quality.ConfidenceLevel == "" {
		t.Fatalf("quality = %#v; response must expose an aggregate quality gate", body.Quality)
	}
	if body.Execution == nil || body.Execution.LatencyBreakdownMs == nil || body.Execution.LatencyBreakdownMs["total"] < 0 {
		t.Fatalf("execution = %#v; response must expose request timing", body.Execution)
	}
	if body.Stage != "causal_extract" || body.Status != "completed" || body.PipelineVersion == "" {
		t.Fatalf("lifecycle evidence = stage:%q status:%q version:%q", body.Stage, body.Status, body.PipelineVersion)
	}
	if body.StartedAt.IsZero() || body.CompletedAt.IsZero() || body.CompletedAt.Before(body.StartedAt) {
		t.Fatalf("invalid lifecycle timestamps: started=%v completed=%v", body.StartedAt, body.CompletedAt)
	}
}

func TestCausalResultLimit(t *testing.T) {
	testCases := []struct {
		name    string
		raw     string
		want    int
		wantErr bool
	}{
		{name: "default", raw: "", want: 10},
		{name: "custom", raw: "25", want: 25},
		{name: "zero", raw: "0", wantErr: true},
		{name: "negative", raw: "-1", wantErr: true},
		{name: "too large", raw: "101", wantErr: true},
		{name: "not a number", raw: "many", wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := causalResultLimit(testCase.raw)
			if (err != nil) != testCase.wantErr {
				t.Fatalf("causalResultLimit(%q) error = %v, wantErr %t", testCase.raw, err, testCase.wantErr)
			}
			if got != testCase.want {
				t.Fatalf("causalResultLimit(%q) = %d, want %d", testCase.raw, got, testCase.want)
			}
		})
	}
}

func TestRelatedCausalEndpointsRejectInvalidLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := &CausalReasoningHandler{}
	router := gin.New()
	router.GET("/causal/causes/:entity", handler.GetRelatedCauses)
	router.GET("/causal/effects/:entity", handler.GetRelatedEffects)

	for _, path := range []string{
		"/causal/causes/hypotension?limit=many",
		"/causal/effects/hypotension?limit=0",
	} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
		})
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

func TestExtractCausalRelationsExposesRuleAndPCCMAuditEvidence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := &CausalReasoningHandler{
		extractor: causal_reasoning.NewEntityExtractor(nil),
	}

	request := httptest.NewRequest(http.MethodPost, "/causal/extract", strings.NewReader(`{"text":"患者服用降压药后出现体位性低血压并有跌倒风险","use_rules":true}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router := gin.New()
	router.POST("/causal/extract", handler.ExtractCausalRelations)
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body causal_reasoning.ExtractResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Execution == nil || body.Execution.Mode != "rules" || body.Execution.ModelStatus != "not_requested" {
		t.Fatalf("execution = %#v, want rule-only audit state", body.Execution)
	}
	if len(body.Execution.MatchedRules) < 2 || body.Execution.MatchedRules[0].ID != "rule_001" {
		t.Fatalf("matched rules = %#v, want sorted rule provenance", body.Execution.MatchedRules)
	}
	if len(body.Relations) == 0 || body.Relations[0].PCCMEvidence == nil {
		t.Fatalf("relations = %#v, want relation PCCM evidence", body.Relations)
	}
	pccm := body.Relations[0].PCCMEvidence
	if len(pccm.ActiveSources) != 1 || pccm.ActiveSources[0] != "rule" || pccm.FinalConfidence != body.Relations[0].Confidence {
		t.Fatalf("PCCM evidence = %#v, relation = %#v", pccm, body.Relations[0])
	}
	if body.Persistence.Status != "analysis_only" || body.Persistence.Requested || body.GraphPersisted {
		t.Fatalf("persistence = %#v, want analysis-only outcome", body.Persistence)
	}
}

func TestExtractCausalRelationsReportsUnavailableModelWithoutFabricatingOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := &CausalReasoningHandler{
		extractor: causal_reasoning.NewEntityExtractor(nil),
	}

	request := httptest.NewRequest(http.MethodPost, "/causal/extract", strings.NewReader(`{"text":"任意未见于规则库的护理描述","use_llm":true}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router := gin.New()
	router.POST("/causal/extract", handler.ExtractCausalRelations)
	router.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body causal_reasoning.ExtractResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Execution == nil || body.Execution.Mode != "model_unavailable" || body.Execution.LLMAvailable || body.Execution.ModelStatus != "unavailable" {
		t.Fatalf("execution = %#v, want unavailable model state", body.Execution)
	}
	if len(body.Relations) != 0 || body.Error == "" {
		t.Fatalf("relations = %#v, error = %q; arbitrary input must not receive fabricated output", body.Relations, body.Error)
	}
	if body.Quality == nil || body.Quality.ConfidenceLevel != "insufficient_evidence" || !body.Quality.ReviewRequired {
		t.Fatalf("quality = %#v; unavailable model must be marked insufficient", body.Quality)
	}
	if body.Execution.LatencyBreakdownMs == nil || body.Execution.ModelTier != "unavailable" {
		t.Fatalf("execution = %#v; unavailable model audit is incomplete", body.Execution)
	}
	if body.Stage != "causal_extract" || body.Status != "failed" || body.StartedAt.IsZero() || body.CompletedAt.IsZero() {
		t.Fatalf("failure lifecycle evidence is incomplete: %#v", body)
	}
}

func TestExtractCausalRelationsReportsRulesFallbackWhenModelUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := &CausalReasoningHandler{
		extractor: causal_reasoning.NewEntityExtractor(nil),
	}

	request := httptest.NewRequest(http.MethodPost, "/causal/extract", strings.NewReader(`{"text":"患者服用降压药后出现体位性低血压","use_rules":true,"use_llm":true}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router := gin.New()
	router.POST("/causal/extract", handler.ExtractCausalRelations)
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body causal_reasoning.ExtractResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Execution == nil || body.Execution.Mode != "rules_fallback" || body.Execution.FallbackReason == "" || body.Execution.ModelStatus != "unavailable" {
		t.Fatalf("execution = %#v, want rules fallback audit state", body.Execution)
	}
	if len(body.Relations) == 0 || len(body.Relations[0].RuleMatches) == 0 {
		t.Fatalf("relations = %#v, want rule-backed fallback output", body.Relations)
	}
	if body.Execution.ModelTier != "rules" || body.Execution.LatencyBreakdownMs == nil {
		t.Fatalf("execution = %#v, want explicit rules fallback tier and timing", body.Execution)
	}
}
