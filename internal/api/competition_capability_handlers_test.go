package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/contextkeeper/service/internal/config"
	"github.com/contextkeeper/service/internal/middleware"
	"github.com/contextkeeper/service/internal/services"
	"github.com/contextkeeper/service/internal/utils"
	"github.com/gin-gonic/gin"
)

func capabilityTestRouter(t *testing.T, service *services.CompetitionCapabilityService, userID string) *gin.Engine {
	t.Helper()
	router := gin.New()
	router.Use(utils.TraceIDMiddleware())
	router.Use(func(c *gin.Context) {
		if userID != "" {
			c.Set("user_id", userID)
		}
		c.Next()
	})
	(&Handler{competitionCapabilities: service}).RegisterCompetitionCapabilityRoutes(router.Group("/api"))
	return router
}

func capabilityTestService(t *testing.T) *services.CompetitionCapabilityService {
	t.Helper()
	service, err := services.NewCompetitionCapabilityService(&config.Config{StoragePath: t.TempDir()})
	if err != nil {
		t.Fatalf("NewCompetitionCapabilityService: %v", err)
	}
	t.Cleanup(service.Stop)
	return service
}

func capabilityRequest(t *testing.T, router *gin.Engine, method, target, body, traceID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if traceID != "" {
		req.Header.Set("X-Trace-ID", traceID)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}

func TestCompetitionCapabilityEvidenceRoutesUseAuthenticatedOwnerAndHideContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := capabilityTestService(t)
	ownerRouter := capabilityTestRouter(t, service, "resident-a")
	traceID := "capability-trace-001"

	episode := capabilityRequest(t, ownerRouter, http.MethodPost, "/api/v1/evidence/episodes", `{"id":"ep-a","source":"text","content":"服药后头晕"}`, traceID)
	if episode.Code != http.StatusCreated || bytes.Contains(episode.Body.Bytes(), []byte("服药后头晕")) || !bytes.Contains(episode.Body.Bytes(), []byte(`"trace_id":"capability-trace-001"`)) {
		t.Fatalf("episode status=%d body=%s", episode.Code, episode.Body.String())
	}
	entity := capabilityRequest(t, ownerRouter, http.MethodPost, "/api/v1/evidence/entities", `{"id":"symptom-a","name":"头晕","kind":"symptom","episodes":["ep-a"]}`, traceID)
	if entity.Code != http.StatusCreated {
		t.Fatalf("entity status=%d body=%s", entity.Code, entity.Body.String())
	}
	second := capabilityRequest(t, ownerRouter, http.MethodPost, "/api/v1/evidence/entities", `{"id":"drug-a","name":"降压药","kind":"drug","episodes":["ep-a"]}`, traceID)
	if second.Code != http.StatusCreated {
		t.Fatalf("second entity status=%d body=%s", second.Code, second.Body.String())
	}
	edge := capabilityRequest(t, ownerRouter, http.MethodPost, "/api/v1/evidence/edges", `{"id":"edge-a","source":"drug-a","target":"symptom-a","relation":"causes","episodes":["ep-a"]}`, traceID)
	if edge.Code != http.StatusCreated {
		t.Fatalf("edge status=%d body=%s", edge.Code, edge.Body.String())
	}

	otherRouter := capabilityTestRouter(t, service, "resident-b")
	otherCommunities := capabilityRequest(t, otherRouter, http.MethodGet, "/api/v1/evidence/communities", "", traceID)
	if otherCommunities.Code != http.StatusOK || bytes.Contains(otherCommunities.Body.Bytes(), []byte("drug-a")) || !bytes.Contains(otherCommunities.Body.Bytes(), []byte(`"count":0`)) {
		t.Fatalf("owner isolation failed status=%d body=%s", otherCommunities.Code, otherCommunities.Body.String())
	}

	communities := capabilityRequest(t, ownerRouter, http.MethodGet, "/api/v1/evidence/communities", "", traceID)
	if communities.Code != http.StatusOK || !bytes.Contains(communities.Body.Bytes(), []byte("drug-a")) || bytes.Contains(communities.Body.Bytes(), []byte("服药后头晕")) {
		t.Fatalf("community evidence status=%d body=%s", communities.Code, communities.Body.String())
	}
}

func TestCompetitionCapabilityRoutesRequireAuthenticatedIdentityAndDeclareRiskRerankContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := capabilityTestService(t)
	unauthenticated := capabilityTestRouter(t, service, "")
	denied := capabilityRequest(t, unauthenticated, http.MethodPost, "/api/v1/evidence/episodes", `{"id":"ep","source":"text","content":"x"}`, "")
	if denied.Code != http.StatusUnauthorized || !bytes.Contains(denied.Body.Bytes(), []byte(`"status":"failed"`)) {
		t.Fatalf("unauthenticated status=%d body=%s", denied.Code, denied.Body.String())
	}

	router := capabilityTestRouter(t, service, "resident-a")
	badRerank := capabilityRequest(t, router, http.MethodPost, "/api/v1/retrieval/risk-rerank", `{"evaluated_evidence":false,"candidate_origin":"owner_scoped_evidence","candidates":[{"id":"doc-a","semantic":0.9}]}`, "")
	if badRerank.Code != http.StatusBadRequest {
		t.Fatalf("bad rerank status=%d body=%s", badRerank.Code, badRerank.Body.String())
	}
	for _, episodeID := range []string{"doc-a", "doc-b"} {
		created := capabilityRequest(t, router, http.MethodPost, "/api/v1/evidence/episodes", `{"id":"`+episodeID+`","source":"text","content":"synthetic evidence"}`, "")
		if created.Code != http.StatusCreated {
			t.Fatalf("seed episode %s status=%d body=%s", episodeID, created.Code, created.Body.String())
		}
	}
	rerank := capabilityRequest(t, router, http.MethodPost, "/api/v1/retrieval/risk-rerank", `{"evaluated_evidence":true,"candidate_origin":"owner_scoped_evidence","candidates":[{"id":"doc-a","semantic":0.9,"context_risk":0.1},{"id":"doc-b","semantic":0.8,"source_risk":1}]}`, "rerank-trace-001")
	if rerank.Code != http.StatusOK || !bytes.Contains(rerank.Body.Bytes(), []byte(`"rrf_invoked":false`)) || !bytes.Contains(rerank.Body.Bytes(), []byte(`"not_rrf_substitute":true`)) {
		t.Fatalf("rerank status=%d body=%s", rerank.Code, rerank.Body.String())
	}
	var payload struct {
		Data struct {
			Ranked []struct {
				Candidate struct {
					ID string `json:"id"`
				} `json:"candidate"`
			} `json:"ranked_candidates"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rerank.Body.Bytes(), &payload); err != nil || len(payload.Data.Ranked) != 2 || payload.Data.Ranked[0].Candidate.ID != "doc-a" {
		t.Fatalf("rerank payload=%s err=%v", rerank.Body.String(), err)
	}
}

func TestCompetitionCapabilityRoutesRequireJWTAtProductionBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("JWT_SECRET", "capability-route-test-secret")
	service := capabilityTestService(t)
	router := gin.New()
	protected := router.Group("/api")
	protected.Use(middleware.JWTAuth())
	(&Handler{competitionCapabilities: service}).RegisterCompetitionCapabilityRoutes(protected)

	unauthenticated := httptest.NewRequest(http.MethodGet, "/api/v1/lifecycle/status", nil)
	denied := httptest.NewRecorder()
	router.ServeHTTP(denied, unauthenticated)
	if denied.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d body=%s", denied.Code, denied.Body.String())
	}

	token, err := middleware.GenerateTokenWithRole("resident-a", "competition", "doctor")
	if err != nil {
		t.Fatalf("GenerateTokenWithRole: %v", err)
	}
	authenticated := httptest.NewRequest(http.MethodGet, "/api/v1/lifecycle/status", nil)
	authenticated.Header.Set("Authorization", "Bearer "+token)
	allowed := httptest.NewRecorder()
	router.ServeHTTP(allowed, authenticated)
	if allowed.Code != http.StatusOK || !bytes.Contains(allowed.Body.Bytes(), []byte(`"trace_id"`)) {
		t.Fatalf("authenticated status=%d body=%s", allowed.Code, allowed.Body.String())
	}
}

func TestCompetitionCapabilityServiceUnavailableAndLifecycleStatusAreExplicit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	missing := capabilityTestRouter(t, nil, "resident-a")
	unavailable := capabilityRequest(t, missing, http.MethodGet, "/api/v1/lifecycle/status", "", "")
	if unavailable.Code != http.StatusServiceUnavailable || !bytes.Contains(unavailable.Body.Bytes(), []byte(`"stage":"tiered_lifecycle"`)) {
		t.Fatalf("unavailable status=%d body=%s", unavailable.Code, unavailable.Body.String())
	}

	router := capabilityTestRouter(t, capabilityTestService(t), "resident-a")
	status := capabilityRequest(t, router, http.MethodGet, "/api/v1/lifecycle/status", "", "")
	if status.Code != http.StatusOK || !bytes.Contains(status.Body.Bytes(), []byte(`"implementation":"in_process_evidence_lifecycle"`)) || !bytes.Contains(status.Body.Bytes(), []byte(`"sensitive_scorer_configured":false`)) || !bytes.Contains(status.Body.Bytes(), []byte(`"deletion_integration":false`)) {
		t.Fatalf("lifecycle status=%d body=%s", status.Code, status.Body.String())
	}
}
