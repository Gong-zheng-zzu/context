package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func causalReviewRouter(handler *CausalReasoningHandler, userID, role string) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", userID)
		c.Set("role", role)
		c.Set("traceId", "review-trace-001")
		c.Next()
	})
	router.POST("/reviews", handler.CreateCausalReview)
	router.GET("/reviews", handler.ListCausalReviews)
	return router
}

func TestCausalReviewQueueAcceptsOnlySyntheticEvaluationData(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := &CausalReasoningHandler{reviewStore: NewFileCausalReviewStore(filepath.Join(t.TempDir(), "reviews.jsonl"))}
	router := causalReviewRouter(handler, "eval_user_001", "caregiver")
	body := `{"record_id":"synthetic-1","session_id":"eval_retrieval_test","source_text":"李爷爷服药后头晕并有跌倒风险","synthetic":true,"dataset_version":"causal-ompr-v2","rule_version":"rules-v1","model_version":"rules","predicted":{"object":"李爷爷","mediator":"服药","property":"头晕","result":"跌倒风险"},"corrected":{"object":"李爷爷","mediator":"服药","property":"头晕","result":"跌倒风险"},"issues":["confirmed"]}`
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/reviews", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || strings.Contains(response.Body.String(), "source_text") || !strings.Contains(response.Body.String(), `"training_applied":false`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}

	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/reviews", nil))
	if list.Code != http.StatusOK || strings.Contains(list.Body.String(), "source_text") {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(list.Body.Bytes(), &payload); err != nil || payload["count"].(float64) != 1 {
		t.Fatalf("list payload=%v err=%v", payload, err)
	}
}

func TestCausalReviewQueueRejectsRealOrUnauthorizedData(t *testing.T) {
	handler := &CausalReasoningHandler{reviewStore: NewFileCausalReviewStore(filepath.Join(t.TempDir(), "reviews.jsonl"))}
	for _, test := range []struct {
		user, role, body string
		want             int
	}{
		{"eval_user_001", "caregiver", `{"record_id":"r","session_id":"eval_retrieval_test","source_text":"real","synthetic":false,"dataset_version":"v","rule_version":"v","model_version":"v"}`, http.StatusBadRequest},
		{"other", "caregiver", `{"record_id":"r","session_id":"eval_retrieval_test","source_text":"synthetic","synthetic":true,"dataset_version":"v","rule_version":"v","model_version":"v"}`, http.StatusForbidden},
	} {
		router := causalReviewRouter(handler, test.user, test.role)
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/reviews", strings.NewReader(test.body))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(response, request)
		if response.Code != test.want {
			t.Fatalf("status=%d want=%d body=%s", response.Code, test.want, response.Body.String())
		}
		var payload map[string]interface{}
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode error response: %v", err)
		}
		if payload["trace_id"] != "review-trace-001" || payload["stage"] != "causal_review" || payload["status"] != "failed" {
			t.Fatalf("missing failure evidence: %v", payload)
		}
	}
}
