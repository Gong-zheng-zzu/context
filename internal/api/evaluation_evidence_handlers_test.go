package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/contextkeeper/service/internal/models"
	"github.com/gin-gonic/gin"
)

func TestHandleStructuredThreeWayEvidenceRejectsOutOfScopeRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("user_id", structuredThreeWayEvidenceUserID)
	ctx.Request = evidenceRequest(t, StructuredThreeWayEvidenceRequest{
		UserID:    "other_user",
		SessionID: structuredThreeWayEvidenceSession,
		DocIDs:    []string{"doc-1"},
	})

	(&Handler{}).HandleStructuredThreeWayEvidence(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestHandleStructuredThreeWayEvidenceRequiresEvaluationJWTIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("user_id", "another_user")
	ctx.Request = evidenceRequest(t, StructuredThreeWayEvidenceRequest{
		UserID:    structuredThreeWayEvidenceUserID,
		SessionID: structuredThreeWayEvidenceSession,
		DocIDs:    []string{"doc-1"},
	})

	(&Handler{}).HandleStructuredThreeWayEvidence(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestHandleStructuredThreeWayEvidenceReportsUnavailableLanes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("user_id", structuredThreeWayEvidenceUserID)
	ctx.Request = evidenceRequest(t, StructuredThreeWayEvidenceRequest{
		UserID:    structuredThreeWayEvidenceUserID,
		SessionID: structuredThreeWayEvidenceSession,
		DocIDs:    []string{"doc-1"},
	})

	(&Handler{}).HandleStructuredThreeWayEvidence(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			ReadOnly  bool                                 `json:"read_only"`
			Documents []structuredThreeWayDocumentEvidence `json:"documents"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.Success || !response.Data.ReadOnly || len(response.Data.Documents) != 1 {
		t.Fatalf("unexpected response: %s", recorder.Body.String())
	}
	document := response.Data.Documents[0]
	if document.Vector.Status != "error" || document.Vector.Verified || document.Vector.Count != 0 {
		t.Fatalf("vector evidence = %+v", document.Vector)
	}
	if document.Timeline.Status != "error" || document.Graph.Status != "error" {
		t.Fatalf("unavailable evidence = timeline=%+v graph=%+v", document.Timeline, document.Graph)
	}
}

func TestStructuredThreeWayVectorEvidenceCountsOnlyOwnedRecords(t *testing.T) {
	results := []models.SearchResult{
		{Fields: map[string]interface{}{"userId": structuredThreeWayEvidenceUserID, "session_id": structuredThreeWayEvidenceSession, "metadata": `{"doc_id":"doc-1"}`}},
		{Fields: map[string]interface{}{"userId": structuredThreeWayEvidenceUserID, "session_id": structuredThreeWayEvidenceSession, "doc_id": "doc-1"}},
		{Fields: map[string]interface{}{"userId": "other_user", "session_id": structuredThreeWayEvidenceSession, "doc_id": "doc-1"}},
		{Fields: map[string]interface{}{"userId": structuredThreeWayEvidenceUserID, "session_id": "other_session", "doc_id": "doc-1"}},
	}
	counts := map[string]int{"doc-1": 0}
	for _, result := range results {
		if structuredThreeWayVectorResultOwnedBy(result, structuredThreeWayEvidenceUserID, structuredThreeWayEvidenceSession) {
			if docID := structuredThreeWayVectorDocID(result); docID == "doc-1" {
				counts[docID]++
			}
		}
	}

	if evidence := structuredThreeWayVectorLane(counts, nil, "doc-1"); !evidence.Verified || evidence.Count != 2 || evidence.Status != "verified" {
		t.Fatalf("vector evidence = %+v", evidence)
	}
}

func evidenceRequest(t *testing.T, payload StructuredThreeWayEvidenceRequest) *http.Request {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/evaluation/structured-three-way/evidence", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	return request
}
