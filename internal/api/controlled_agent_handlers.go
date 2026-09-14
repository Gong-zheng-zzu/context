package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/contextkeeper/service/internal/agent"
	agenttools "github.com/contextkeeper/service/internal/agent/tools"
	"github.com/contextkeeper/service/internal/middleware"
	"github.com/contextkeeper/service/internal/store"
	"github.com/contextkeeper/service/internal/utils"
	"github.com/gin-gonic/gin"
)

const controlledAgentExecutionMode = "read_only_authoritative_tools"

// ControlledAgentRequest is deliberately small: the authenticated identity is
// taken only from JWT, while the supplied session is used for read-only scope.
type ControlledAgentRequest struct {
	SessionID string `json:"session_id" binding:"required,max=128"`
	Query     string `json:"query" binding:"required,max=8192"`
}

// ControlledAgentResponse never contains chain-of-thought, tool arguments,
// observations, or a write receipt. The trace ID links this response to logs.
type ControlledAgentResponse struct {
	TraceID       string                 `json:"trace_id"`
	Answer        string                 `json:"answer"`
	Execution     *AgentExecutionSummary `json:"execution"`
	WriteApplied  bool                   `json:"write_applied"`
	ExecutionMode string                 `json:"execution_mode"`
	SessionID     string                 `json:"session_id"`
}

type controlledAgentRunner func(context.Context, agent.LLMCaller, *agenttools.Deps, string, string) *agent.AgentTrace

// HandleControlledAgent runs the competition Agent through its explicit
// read-only allowlist. It never stores, archives, notifies, or deletes data.
func (h *Handler) HandleControlledAgent(c *gin.Context) {
	var req ControlledAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "request validation failed: " + err.Error()})
		return
	}
	req.SessionID = strings.TrimSpace(req.SessionID)
	req.Query = strings.TrimSpace(req.Query)
	if req.SessionID == "" || req.Query == "" {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "session_id and query are required"})
		return
	}

	userID, ok := middleware.GetUserID(c)
	if !ok || userID == "" {
		c.JSON(http.StatusUnauthorized, APIResponse{Success: false, Error: "unauthorized"})
		return
	}
	if h.contextService != nil {
		if err := verifyReadOnlySessionOwner(h.contextService.SessionStore(), req.SessionID, userID); err != nil {
			c.JSON(http.StatusForbidden, APIResponse{Success: false, Error: "无权访问该受保护会话"})
			return
		}
	}

	caller := h.agentCaller
	if caller == nil {
		var available bool
		caller, available = llmService.(agent.LLMCaller)
		if !available || caller == nil {
			c.JSON(http.StatusServiceUnavailable, APIResponse{Success: false, Error: "受控 Agent 模型服务不可用"})
			return
		}
	}

	var baseContextService = contextService
	if h.contextService != nil {
		baseContextService = h.contextService.GetContextService()
	}
	if baseContextService == nil && h.controlledAgentRunner == nil {
		c.JSON(http.StatusServiceUnavailable, APIResponse{Success: false, Error: "受控 Agent 记忆服务不可用"})
		return
	}

	traceID := utils.GetTraceIDFromGin(c)
	if traceID == "" {
		traceID = utils.GenerateTraceID()
		c.Header("X-Trace-ID", traceID)
	}
	requestContext := utils.WithTraceID(c.Request.Context(), traceID)
	deps := &agenttools.Deps{ContextService: baseContextService, UserID: userID, SessionID: req.SessionID}
	if h.contextService != nil {
		deps.SessionStore = h.contextService.SessionStore()
	}
	role, _ := middleware.GetRole(c)
	if role == "" {
		role = "caregiver"
	}

	runner := h.controlledAgentRunner
	if runner == nil {
		runner = runControlledAgent
	}
	trace := runner(agenttools.WithDeps(requestContext, deps), caller, deps, req.Query, role)
	if trace == nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Success: false, Error: "受控 Agent 未返回执行结果"})
		return
	}

	c.JSON(http.StatusOK, APIResponse{Success: true, Data: ControlledAgentResponse{
		TraceID:       traceID,
		Answer:        trace.FinalAnswer,
		Execution:     newAgentExecutionSummary(trace),
		WriteApplied:  false,
		ExecutionMode: controlledAgentExecutionMode,
		SessionID:     req.SessionID,
	}})
}

func runControlledAgent(ctx context.Context, caller agent.LLMCaller, _ *agenttools.Deps, query, role string) *agent.AgentTrace {
	registry := newControlledAgentRegistry(caller)
	prompt := agent.BuildAgentSystemPrompt(role, registry.ToolDescriptions()) + "\n\n当前 HTTP 入口仅允许只读工具。不得保存、修改、归档、通知或删除数据；权威资料工具仅可获取显式允许域名的 HTTPS 来源 URL。"
	return agent.NewOrchestrator(caller, registry, prompt).Run(ctx, agent.BuildAgentQueryPrompt(query, ""))
}

// verifyReadOnlySessionOwner performs an ownership read only. Unknown sessions
// are permitted for empty-scope retrieval but are never claimed or created.
func verifyReadOnlySessionOwner(sessionStore *store.SessionStore, sessionID, userID string) error {
	if sessionStore == nil {
		return nil
	}
	owner, err := sessionStore.GetSessionOwner(sessionID)
	if errors.Is(err, store.ErrSessionNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if owner != userID {
		return errors.New("session belongs to a different user")
	}
	return nil
}
