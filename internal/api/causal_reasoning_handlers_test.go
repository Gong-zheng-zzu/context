package api

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCausalGraphEndpointsReturnUnavailableWithoutNeo4j(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewCausalReasoningHandler(nil, nil, nil)
	router := gin.New()
	router.POST("/infer", handler.InferCausal)
	router.GET("/stats", handler.GetGraphStats)

	for _, tc := range []struct {
		method string
		path   string
	}{
		{"POST", "/infer"},
		{"GET", "/stats"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != 503 {
			t.Fatalf("%s %s status = %d, want 503", tc.method, tc.path, rec.Code)
		}
	}
}
