package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestAuditTraceRequiresOwnerAndHidesUnknownTrace(t *testing.T) {
	gin.SetMode(gin.TestMode)

	request := httptest.NewRequest(http.MethodGet, "/trace/trace-unknown", nil)
	request.Header.Set("X-Trace-ID", "request-trace-001")
	withoutOwner := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(withoutOwner)
	ctx.Request = request
	ctx.Params = gin.Params{{Key: "trace_id", Value: "trace-unknown"}}
	(&Handler{}).HandleAuditTrace(ctx)
	if withoutOwner.Code != http.StatusUnauthorized {
		t.Fatalf("status without owner = %d, want 401", withoutOwner.Code)
	}

	owner := httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(owner)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/trace/trace-unknown", nil)
	ctx.Params = gin.Params{{Key: "trace_id", Value: "trace-unknown"}}
	ctx.Set("user_id", "user-a")
	(&Handler{}).HandleAuditTrace(ctx)
	if owner.Code != http.StatusNotFound || !strings.Contains(owner.Body.String(), "trace evidence is unavailable") {
		t.Fatalf("unknown trace response = %d %s", owner.Code, owner.Body.String())
	}
}

func TestAuditTraceIsOwnerScopedAndRedacted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	competitionTraceEvidence = &traceEvidenceCache{entries: make(map[string]TraceEvidence)}

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/trace/trace-owner-001", nil)
	ctx.Params = gin.Params{{Key: "trace_id", Value: "trace-owner-001"}}
	ctx.Set("user_id", "user-a")
	ctx.Set("traceId", "trace-owner-001")
	rememberTraceEvidence(ctx, "causal_extract", "completed", time.Unix(10, 0), time.Unix(11, 0), map[string]interface{}{
		"decision": "verified",
		"nested": map[string]interface{}{
			"prompt":   "do not expose",
			"safe_key": "kept",
		},
		"original_text": "不得返回",
	})

	owner := httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(owner)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/trace/trace-owner-001", nil)
	ctx.Params = gin.Params{{Key: "trace_id", Value: "trace-owner-001"}}
	ctx.Set("user_id", "user-a")
	(&Handler{}).HandleAuditTrace(ctx)
	if owner.Code != http.StatusOK || !strings.Contains(owner.Body.String(), `"decision":"verified"`) {
		t.Fatalf("owner trace response = %d %s", owner.Code, owner.Body.String())
	}
	if strings.Contains(owner.Body.String(), "不得返回") || strings.Contains(owner.Body.String(), `"user_id"`) {
		t.Fatalf("trace response leaked sensitive evidence: %s", owner.Body.String())
	}

	if strings.Contains(owner.Body.String(), "do not expose") || !strings.Contains(owner.Body.String(), `"safe_key":"kept"`) {
		t.Fatalf("nested trace redaction failed: %s", owner.Body.String())
	}

	other := httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(other)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/trace/trace-owner-001", nil)
	ctx.Params = gin.Params{{Key: "trace_id", Value: "trace-owner-001"}}
	ctx.Set("user_id", "user-b")
	(&Handler{}).HandleAuditTrace(ctx)
	if other.Code != http.StatusNotFound {
		t.Fatalf("cross-owner trace status = %d, want 404", other.Code)
	}
}
