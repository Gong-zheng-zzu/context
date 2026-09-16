package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/contextkeeper/service/internal/middleware"
	"github.com/contextkeeper/service/internal/security"
	"github.com/contextkeeper/service/internal/utils"
	"github.com/gin-gonic/gin"
)

func TestInputSecurityDecisionMatchesChatPreGenerationSemantics(t *testing.T) {
	tests := []struct {
		name            string
		inputRedacted   bool
		inputNormalized bool
		decision        string
		stage           string
	}{
		{"unchanged input", false, false, "allow", "none"},
		{"ASDF normalization only", false, true, "allow", "asdf_normalized"},
		{"sensitive data redacted", true, false, "redact", "sensitive_data_policy"},
		{"redaction takes precedence", true, true, "redact", "sensitive_data_policy"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision, stage := inputSecurityDecision(test.inputRedacted, test.inputNormalized)
			if decision != test.decision || stage != test.stage {
				t.Fatalf("inputSecurityDecision(%t, %t) = (%q, %q), want (%q, %q)", test.inputRedacted, test.inputNormalized, decision, stage, test.decision, test.stage)
			}
		})
	}
}

func TestSecurityExecutionEvidenceReportsUnavailableSecurityService(t *testing.T) {
	evidence := newSecurityExecutionEvidence(nil)
	if got, ok := evidence["asdf_checked"].(bool); !ok || got {
		t.Fatalf("asdf_checked = %#v, want false when security service is unavailable", evidence["asdf_checked"])
	}
	for _, field := range []string{"input_normalized", "multi_layer_used", "pccm_enabled", "casia_enabled"} {
		if got, ok := evidence[field].(bool); !ok || got {
			t.Fatalf("%s = %#v, want false initial state", field, evidence[field])
		}
	}
}

func newSecurityLabTestHandler(t *testing.T) *Handler {
	t.Helper()
	t.Setenv("SECURITY_ENCRYPTION_SECRET", "security-lab-test-secret-with-sufficient-length")
	service, err := security.NewSecurityService("../../config/security_policy.yaml", t.TempDir()+"/audit.log")
	if err != nil {
		t.Fatalf("NewSecurityService: %v", err)
	}
	t.Cleanup(func() { _ = service.Close() })
	return &Handler{securityService: service}
}

func TestSecurityAblationEvaluationRequiresConfiguredUserAndSyntheticScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := newSecurityLabTestHandler(t)
	t.Setenv("SECURITY_EVAL_USER_ID", "security_eval_user")

	for _, test := range []struct {
		name, userID, body string
		status             int
	}{
		{"missing identity", "", `{"message":"正常护理记录","sample_id":"synthetic_normal_1","synthetic":true}`, http.StatusUnauthorized},
		{"wrong identity", "someone_else", `{"message":"正常护理记录","sample_id":"synthetic_normal_1","synthetic":true}`, http.StatusForbidden},
		{"not declared synthetic", "security_eval_user", `{"message":"正常护理记录","sample_id":"synthetic_normal_1","synthetic":false}`, http.StatusBadRequest},
		{"invalid sample scope", "security_eval_user", `{"message":"正常护理记录","sample_id":"normal_1","synthetic":true}`, http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/security/ablation", strings.NewReader(test.body))
			ctx.Request.Header.Set("Content-Type", "application/json")
			if test.userID != "" {
				ctx.Set("user_id", test.userID)
			}
			handler.HandleSecurityAblationEvaluation(ctx)
			if ctx.Writer.Status() != test.status {
				t.Fatalf("status = %d, want %d", ctx.Writer.Status(), test.status)
			}
			var payload map[string]interface{}
			if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if payload["trace_id"] == "" || payload["stage"] != "security_ablation" || payload["status"] != "failed" {
				t.Fatalf("missing failure evidence: %v", payload)
			}
		})
	}
}

func TestSecurityAblationEvaluationIsJWTProtectedAndReturnsServerProfiles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := newSecurityLabTestHandler(t)
	t.Setenv("JWT_SECRET", "security-lab-jwt-secret")
	t.Setenv("SECURITY_EVAL_USER_ID", "security_eval_user")
	router := gin.New()
	router.Use(utils.TraceIDMiddleware())
	group := router.Group("/api/v1/security")
	group.Use(middleware.JWTAuth())
	group.POST("/ablation", handler.HandleSecurityAblationEvaluation)
	body := `{"message":"合成样本手机号 138-1234-5678","sample_id":"synthetic_phone_1","synthetic":true}`

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/api/v1/security/ablation", strings.NewReader(body)))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}

	token, err := middleware.GenerateTokenWithRole("security_eval_user", "competition", "doctor")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/security/ablation", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-Trace-ID", "security-lab-trace-001")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "138-1234-5678") || strings.Contains(response.Body.String(), "13812345678") {
		t.Fatalf("response leaked plaintext input: %s", response.Body.String())
	}
	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			TraceID  string                               `json:"trace_id"`
			Profiles []security.SecurityProfileEvaluation `json:"profiles"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Success || payload.Data.TraceID != "security-lab-trace-001" || len(payload.Data.Profiles) != 4 {
		t.Fatalf("response = %+v", payload)
	}
	for index, profile := range payload.Data.Profiles {
		if profile.Profile != security.SecurityLabProfiles[index] || len(profile.Layers) == 0 {
			t.Fatalf("profile[%d] = %+v", index, profile)
		}
	}
	full := payload.Data.Profiles[3]
	if full.PCCMS == nil || full.PCCMS.CalibrationSource != "fixed_unvalidated" || full.CASIA == nil {
		t.Fatalf("full profile audit = %+v", full)
	}
}
