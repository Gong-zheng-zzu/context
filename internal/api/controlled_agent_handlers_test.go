package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/contextkeeper/service/internal/agent"
	agenttools "github.com/contextkeeper/service/internal/agent/tools"
	"github.com/contextkeeper/service/internal/middleware"
	"github.com/contextkeeper/service/internal/services"
	"github.com/contextkeeper/service/internal/utils"
	"github.com/gin-gonic/gin"
)

type controlledAgentFakeCaller struct{}

func (controlledAgentFakeCaller) GenerateResponse(context.Context, *services.GenerateRequest) (*services.GenerateResponse, error) {
	return &services.GenerateResponse{Content: "FinalAnswer: 已完成只读分析"}, nil
}

func TestControlledAgentEndpointRequiresJWT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/agent/execute", (&Handler{agentCaller: controlledAgentFakeCaller{}}).HandleControlledAgent)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/execute", strings.NewReader(`{"session_id":"session-a","query":"查询血压"}`))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusUnauthorized, response.Body.String())
	}
}

func TestControlledAgentEndpointReturnsReadOnlyAuditableSummary(t *testing.T) {
	t.Setenv("JWT_SECRET", "controlled-agent-test-secret")
	gin.SetMode(gin.TestMode)
	token, err := middleware.GenerateTokenWithRole("nurse-01", "ward-a", "caregiver")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	var receivedDeps *agenttools.Deps
	handler := &Handler{
		agentCaller: controlledAgentFakeCaller{},
		controlledAgentRunner: func(_ context.Context, _ agent.LLMCaller, deps *agenttools.Deps, query, role string) *agent.AgentTrace {
			receivedDeps = deps
			if query != "请获取权威资料" || role != "caregiver" {
				t.Fatalf("runner inputs = query:%q role:%q", query, role)
			}
			return &agent.AgentTrace{
				FinalAnswer: "仅获取允许权威来源 URL 的资料。",
				ToolCalls:   1,
				Iterations:  2,
				TotalTimeMs: 18,
				Steps: []agent.AgentStep{{
					StepNumber: 1, Action: "authoritative_web_search", DurationMs: 7 * time.Millisecond,
					Thought: "private", ActionInput: "secret URL", Observation: "private source body",
				}},
			}
		},
	}
	router := gin.New()
	router.Use(utils.TraceIDMiddleware())
	protected := router.Group("/api")
	protected.Use(middleware.JWTAuth())
	protected.POST("/v1/agent/execute", handler.HandleControlledAgent)

	body := bytes.NewBufferString(`{"session_id":"session-read-only","query":"请获取权威资料"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/execute", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Trace-ID", "agent-trace-001")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	if receivedDeps == nil || receivedDeps.UserID != "nurse-01" || receivedDeps.SessionID != "session-read-only" {
		t.Fatalf("deps = %#v; want JWT identity and request read-only scope", receivedDeps)
	}

	serialized := response.Body.String()
	for _, forbidden := range []string{"private", "secret URL", "private source body", "action_input", "observation", "thought"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("response exposed private trace field/value %q: %s", forbidden, serialized)
		}
	}
	if !strings.Contains(serialized, `"trace_id":"agent-trace-001"`) ||
		!strings.Contains(serialized, `"write_applied":false`) ||
		!strings.Contains(serialized, `"execution_mode":"read_only_authoritative_tools"`) ||
		!strings.Contains(serialized, `"capability":"authoritative_source_fetch"`) {
		t.Fatalf("response omitted required auditable contract fields: %s", serialized)
	}

	var envelope APIResponse
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || !envelope.Success {
		t.Fatalf("response envelope = %#v, err = %v", envelope, err)
	}
}

func TestControlledAgentEndpointRejectsOversizedQuery(t *testing.T) {
	t.Setenv("JWT_SECRET", "controlled-agent-test-secret")
	token, err := middleware.GenerateToken("nurse-01", "ward-a")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	router := gin.New()
	protected := router.Group("/api")
	protected.Use(middleware.JWTAuth())
	protected.POST("/v1/agent/execute", (&Handler{agentCaller: controlledAgentFakeCaller{}}).HandleControlledAgent)

	payload, _ := json.Marshal(ControlledAgentRequest{SessionID: "session-a", Query: strings.Repeat("a", 8193)})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/execute", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusBadRequest, response.Body.String())
	}
}
