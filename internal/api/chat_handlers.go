package api

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/contextkeeper/service/internal/agent"
	agenttools "github.com/contextkeeper/service/internal/agent/tools"
	"github.com/contextkeeper/service/internal/models"
	"github.com/contextkeeper/service/internal/security"
	"github.com/contextkeeper/service/internal/services"
	"github.com/contextkeeper/service/internal/store"
	"github.com/contextkeeper/service/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// ChatMessage 聊天消息结构
type ChatMessage struct {
	Role      string `json:"role"`      // user 或 assistant
	Content   string `json:"content"`   // 消息内容
	Timestamp int64  `json:"timestamp"` // 时间戳
}

// FileAttachment 文件附件结构
type FileAttachment struct {
	FileID   string `json:"file_id"`   // 文件ID
	FileName string `json:"file_name"` // 文件名
	FileSize int64  `json:"file_size"` // 文件大小
}

// ChatRequest 聊天请求结构
type ChatRequest struct {
	UserID              string           `json:"user_id"`                    // 用户ID（从JWT获取，不再从请求体读取）
	SessionID           string           `json:"session_id"`                 // 会话ID（可选，不提供则自动生成）
	Message             string           `json:"message" binding:"required"` // 用户消息
	EvaluationInputOnly bool             `json:"evaluationInputOnly"`
	SampleID            string           `json:"sample_id"`
	AttackType          string           `json:"attack_type"`
	EvaluationName      string           `json:"name"`
	History             []ChatMessage    `json:"history"`    // 历史消息（可选）
	Files               []FileAttachment `json:"files"`      // 文件附件（可选）
	AgentMode           bool             `json:"agent_mode"` // Agent模式开关
}

const (
	inputOnlyEvaluationUserID      = "eval_user_001"
	inputOnlyEvaluationSessionHead = "security-input-chain-"
	inputOnlyEvaluationName        = "security_input_chain_consistency"
)

func isControlledInputOnlyEvaluation(userID string, req ChatRequest) bool {
	if userID != inputOnlyEvaluationUserID || req.EvaluationName != inputOnlyEvaluationName || req.SampleID == "" {
		return false
	}
	for _, character := range req.SampleID {
		if !(character >= 'a' && character <= 'z') && !(character >= '0' && character <= '9') && character != '_' && character != '-' {
			return false
		}
	}
	return strings.HasPrefix(req.SessionID, inputOnlyEvaluationSessionHead) && strings.HasSuffix(req.SessionID, "-"+req.SampleID)
}

// ensureProtectedSessionOwner prevents a caller-provided session ID from
// rebinding an existing session to another authenticated user.
func ensureProtectedSessionOwner(sessionStore *store.SessionStore, sessionID, userID string) error {
	owner, err := sessionStore.GetSessionOwner(sessionID)
	switch {
	case err == nil:
		if owner != userID {
			return fmt.Errorf("session belongs to a different user")
		}
		return nil
	case errors.Is(err, store.ErrSessionNotFound):
		return sessionStore.SetSessionMetadata(sessionID, "userId", userID)
	default:
		// An ownerless legacy session cannot be safely claimed by a request.
		return fmt.Errorf("session ownership cannot be verified: %w", err)
	}
}

func writeInputOnlyDecision(c *gin.Context, decision, stage string, inputRedacted, inputNormalized bool, sensitiveInfoCount, warningCount int) {
	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data: gin.H{
			"decision":               decision,
			"stage":                  stage,
			"input_redacted":         inputRedacted,
			"input_normalized":       inputNormalized,
			"sensitive_info_count":   sensitiveInfoCount,
			"security_warning_count": warningCount,
			"generation_skipped":     true,
			"pipeline_version":       "chat-pre-generation-input-v1",
			"comparison_mode":        "chat_pre_generation_input_chain",
		},
		Error: "",
	})
}

func isHandoverSummaryCommand(message string) bool {
	message = strings.TrimSpace(message)
	return strings.Contains(message, "交接摘要") || strings.Contains(message, "整理今日护理记录")
}

// isClinicalFactQuery identifies questions that require an auditable record,
// rather than general nursing advice. It intentionally has a narrow scope.
func isClinicalFactQuery(message string) bool {
	message = strings.TrimSpace(message)
	// Explicit write commands must reach the protected nursing-record workflow.
	for _, prefix := range []string{"记录", "护理记录", "新增记录", "保存记录"} {
		if strings.HasPrefix(message, prefix) {
			return false
		}
	}
	// Use UTF-8 bytes for the highest-risk nursing facts because this project
	// contains legacy source files with mixed text encodings.
	for _, keyword := range []string{
		string([]byte{0xe6, 0x9c, 0x8d, 0xe7, 0x94, 0xa8}), // 服用
		string([]byte{0xe7, 0x94, 0xa8, 0xe8, 0x8d, 0xaf}), // 用药
		string([]byte{0xe5, 0x90, 0x83, 0xe8, 0x8d, 0xaf}), // 吃药
		string([]byte{0xe8, 0xa1, 0x80, 0xe5, 0x8e, 0x8b}), // 血压
		string([]byte{0xe8, 0xb7, 0x8c, 0xe5, 0x80, 0x92}), // 跌倒
	} {
		if strings.Contains(message, keyword) {
			return true
		}
	}
	text := func(runes ...rune) string { return string(runes) }
	questionMarkers := []string{
		text(0x5417),
		text(0x662F, 0x5426),
		text(0x6709, 0x6CA1, 0x6709),
		text(0x591A, 0x5C11),
	}
	containsQuestion := false
	for _, marker := range questionMarkers {
		if strings.Contains(message, marker) {
			containsQuestion = true
			break
		}
	}
	if !containsQuestion {
		return false
	}
	keywords := []string{
		text(0x670D, 0x836F), text(0x7528, 0x836F), text(0x5403, 0x836F),
		text(0x8840, 0x538B), text(0x4F53, 0x6E29), text(0x8840, 0x7CD6),
		text(0x8DCC, 0x5012), text(0x5F02, 0x5E38), text(0x8BB0, 0x5F55),
	}
	for _, keyword := range keywords {
		if strings.Contains(message, keyword) {
			return true
		}
	}
	return false
}

// writeHandoverSummary deliberately summarizes only the current protected session.
// It is a deterministic workflow, not a semantic retrieval query.
func writeHandoverSummary(c *gin.Context, req ChatRequest, userMessage string, memoryWriteApplied bool, memoryID string) {
	if contextService == nil || contextService.SessionStore() == nil {
		c.JSON(http.StatusServiceUnavailable, APIResponse{Success: false, Data: nil, Error: "会话记忆服务不可用"})
		return
	}

	history, err := contextService.SessionStore().GetRecentHistory(req.SessionID, 20)
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Success: false, Data: nil, Error: "读取当前会话记录失败"})
		return
	}

	records := make([]string, 0, len(history))
	for _, entry := range history {
		if !strings.HasPrefix(entry, "用户: ") {
			continue
		}
		content := strings.TrimSpace(strings.TrimPrefix(entry, "用户: "))
		if content == "" || isHandoverSummaryCommand(content) {
			continue
		}
		records = append(records, content)
	}

	summary := "当前会话没有可用于交接的护理记录。"
	if len(records) > 0 {
		summary = "交接摘要（仅基于当前受保护会话已保存记录）：\n"
		for index, record := range records {
			summary += fmt.Sprintf("%d. %s\n", index+1, record)
		}
		summary += "请在交接前由护理人员核对原始记录。"
	}

	filtered := false
	var warnings []string
	if outputFilter != nil {
		filteredSummary, filterWarnings := outputFilter.FilterOutput(summary)
		if len(filterWarnings) > 0 {
			summary = filteredSummary
			warnings = append(warnings, filterWarnings...)
			filtered = true
		}
	}
	if securityService := contextService.GetSecurityService(); securityService != nil {
		scanResult, scanErr := securityService.ScanContent(c.Request.Context(), req.SessionID, req.UserID, summary)
		if scanErr == nil && len(scanResult.SensitiveInfos) > 0 {
			summary = scanResult.RedactedContent
			warnings = append(warnings, "交接摘要中的敏感字段已脱敏")
			filtered = true
		}
	}

	// A handover summary is a derived session view. Keep it in the protected
	// session history, but do not trigger embedding, graph extraction, or LLM
	// storage analysis for the summary command itself.
	if err := contextService.SessionStore().UpdateSession(req.SessionID, "助手: "+summary); err != nil {
		warnings = append(warnings, "交接摘要未写入会话历史")
	}

	c.JSON(http.StatusOK, APIResponse{Success: true, Data: ChatResponse{
		SessionID:          req.SessionID,
		Message:            summary,
		UserMessage:        userMessage,
		Timestamp:          time.Now().Unix(),
		SecurityWarnings:   warnings,
		Filtered:           filtered,
		MemoryWriteApplied: memoryWriteApplied,
		MemoryID:           memoryID,
	}})
}

// ChatResponse 聊天响应结构
type ChatResponse struct {
	SessionID          string                 `json:"session_id"`                  // 会话ID
	Message            string                 `json:"message"`                     // AI回复
	UserMessage        string                 `json:"user_message,omitempty"`      // 脱敏后的用户消息（用于前端显示）
	Timestamp          int64                  `json:"timestamp"`                   // 时间戳
	SensitiveInfos     []SensitiveInfoDisplay `json:"sensitive_infos,omitempty"`   // 检测到的敏感信息
	RetrievedMemory    string                 `json:"retrieved_memory,omitempty"`  // 检索到的记忆
	MemoryCount        int                    `json:"memory_count,omitempty"`      // 记忆条数
	ConfidenceScore    float64                `json:"confidence_score,omitempty"`  // 置信度分数
	SecurityWarnings   []string               `json:"security_warnings,omitempty"` // 安全警告
	Filtered           bool                   `json:"filtered,omitempty"`          // 是否被过滤
	MemoryWriteApplied bool                   `json:"memory_write_applied"`
	MemoryID           string                 `json:"memory_id,omitempty"`
	AgentExecution     *AgentExecutionSummary `json:"agent_execution,omitempty"` // 受控执行摘要（仅Agent模式返回）
}

// AgentExecutionStep is the browser-safe view of one controlled tool execution.
// Raw reasoning, tool inputs, observations and outputs remain in the server-side trace.
type AgentExecutionStep struct {
	StepNumber int    `json:"step_number"`
	ToolName   string `json:"tool_name,omitempty"`
	Capability string `json:"capability,omitempty"`
	Status     string `json:"status"`
	DurationMs int64  `json:"duration_ms"`
}

// AgentExecutionSummary is deliberately limited to auditable execution metadata.
type AgentExecutionSummary struct {
	Steps         []AgentExecutionStep `json:"steps"`
	ToolCalls     int                  `json:"tool_calls"`
	TotalTimeMs   int64                `json:"total_time_ms"`
	Fallback      bool                 `json:"fallback"`
	ExecutionMode string               `json:"execution_mode"`
}

func newAgentExecutionSummary(trace *agent.AgentTrace) *AgentExecutionSummary {
	if trace == nil {
		return nil
	}

	steps := make([]AgentExecutionStep, 0, len(trace.Steps))
	for _, step := range trace.Steps {
		status := "completed"
		if step.Action == "" {
			status = "completed_without_tool"
		}
		steps = append(steps, AgentExecutionStep{
			StepNumber: step.StepNumber,
			ToolName:   step.Action,
			Capability: agentToolCapability(step.Action),
			Status:     status,
			DurationMs: step.DurationMs.Milliseconds(),
		})
	}

	return &AgentExecutionSummary{
		Steps:         steps,
		ToolCalls:     trace.ToolCalls,
		TotalTimeMs:   trace.TotalTimeMs,
		Fallback:      trace.Fallback,
		ExecutionMode: "controlled_registered_tools",
	}
}

func agentToolCapability(toolName string) string {
	switch toolName {
	case "memory_search":
		return "memory_retrieval"
	case "data_manage":
		return "read_only_data_summary"
	case "auto_summary":
		return "read_only_model_summary"
	case "authoritative_web_search":
		return "authoritative_source_fetch"
	default:
		return ""
	}
}

// SensitiveInfoDisplay 敏感信息展示结构
type SensitiveInfoDisplay struct {
	Type           string  `json:"type"`            // 敏感信息类型
	Redacted       string  `json:"redacted"`        // 脱敏后内容
	DetectionLayer string  `json:"detection_layer"` // 检测层（regex/ner/semantic/context/adversarial）
	Confidence     float64 `json:"confidence"`      // 置信度
}

// 全局服务实例
var llmService services.LLMService
var contextService *services.ContextService

// AI安全防护模块实例
var (
	outputFilter          *security.OutputFilter
	modelDosProtection    *security.ModelDosProtection
	dataPoisoningDetector *security.DataPoisoningDetector
	confidenceScorer      *security.ConfidenceScorer
	modelAccessMonitor    *security.ModelAccessMonitor
	adversarialDetector   *security.AdversarialDetector
	inputRequestDetector  = security.NewInputRequestPolicyDetector()
)

var mainlandPhonePattern = regexp.MustCompile(`\b1(?:3\d|4[5-9]|5[0-35-9]|6[2567]|7[0-8]|8\d|9[1389])\d{8}\b`)

// enforceOutputRedaction is the final response boundary. It intentionally
// repeats basic detector redaction and applies a deterministic phone mask so a
// missed policy decision can never return a plaintext phone number.
func enforceOutputRedaction(content string) (string, bool) {
	redacted := security.RedactSensitiveInfo(content)
	redacted = mainlandPhonePattern.ReplaceAllStringFunc(redacted, func(value string) string {
		return value[:3] + "****" + value[7:]
	})
	return redacted, redacted != content
}

// InitChatService 初始化聊天服务
func InitChatService(service services.LLMService) {
	llmService = service
	log.Println("✅ 聊天服务初始化完成")
}

// InitChatContextService 初始化聊天上下文服务（用于记忆检索）
func InitChatContextService(service *services.ContextService) {
	contextService = service
	log.Println("✅ 聊天上下文服务初始化完成")
}

// InitAISecurityModules 初始化AI安全防护模块
func InitAISecurityModules() {
	outputFilter = security.NewOutputFilter()
	modelDosProtection = security.NewModelDosProtection()
	dataPoisoningDetector = security.NewDataPoisoningDetector()
	confidenceScorer = security.NewConfidenceScorer()
	modelAccessMonitor = security.NewModelAccessMonitor()
	adversarialDetector = security.NewAdversarialDetector()
	inputRequestDetector = security.NewInputRequestPolicyDetector()
	log.Println("✅ AI安全防护模块初始化完成")
}

// ChatHandler 处理聊天请求
// POST /api/chat
func ChatHandler(c *gin.Context) {
	var req ChatRequest

	// 解析请求参数
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{
			Success: false,
			Data:    nil,
			Error:   fmt.Sprintf("请求参数错误: %v", err),
		})
		return
	}

	// 🔒 【安全修复】从JWT上下文获取真实的user_id，防止伪造
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, APIResponse{
			Success: false,
			Data:    nil,
			Error:   "未授权：缺少用户身份信息",
		})
		return
	}
	req.UserID = userID.(string) // 强制使用JWT中的user_id，忽略请求体中的user_id

	// 生成或使用现有的会话ID
	if req.SessionID == "" {
		req.SessionID = uuid.New().String()
	}
	if req.EvaluationInputOnly && !isControlledInputOnlyEvaluation(req.UserID, req) {
		c.JSON(http.StatusForbidden, APIResponse{Success: false, Data: nil, Error: "input-only evaluation is restricted to the controlled evaluation user and session"})
		return
	}
	if contextService == nil || contextService.SessionStore() == nil {
		c.JSON(http.StatusServiceUnavailable, APIResponse{Success: false, Data: nil, Error: "会话隔离服务不可用"})
		return
	}
	if err := ensureProtectedSessionOwner(contextService.SessionStore(), req.SessionID, req.UserID); err != nil {
		log.Printf("[Chat] denied session ownership change: user_id=%s session_id=%s err=%v", req.UserID, req.SessionID, err)
		c.JSON(http.StatusForbidden, APIResponse{Success: false, Data: nil, Error: "无权访问该受保护会话"})
		return
	}
	// This branch emits a fixed response and never echoes the submitted input,
	// so it can safely reject clinical fact questions before expensive security
	// analysis, retrieval, or model generation.
	clinicalFactQuery := isClinicalFactQuery(req.Message)
	log.Printf("[Chat] clinical fact gate: session_id=%s matched=%t message_bytes=%d", req.SessionID, clinicalFactQuery, len(req.Message))
	if false && clinicalFactQuery { // Deprecated: security checks must run before the fact-query gate below.
		c.JSON(http.StatusOK, APIResponse{Success: true, Data: ChatResponse{
			SessionID: req.SessionID,
			Message:   "当前普通对话不用于确认护理事实。请通过受保护的护理记录保存与交接摘要流程核对；未提供可核验记录时，系统不会确认是否已服药或发生异常。",
			Timestamp: time.Now().Unix(),
		}})
		return
	}

	// 检查LLM服务是否已初始化
	if !req.EvaluationInputOnly && llmService == nil {
		log.Printf("❌ [聊天API] LLM服务未初始化，llmService为nil")
		c.JSON(http.StatusInternalServerError, APIResponse{
			Success: false,
			Data:    nil,
			Error:   "LLM服务未初始化",
		})
		return
	}
	log.Printf("✅ [聊天API] LLM服务已初始化，准备处理请求")
	// Keep the submitted text for protected storage. Security normalisation below
	// is used for detection, but must not alter clinical values such as 120/80
	// or 36.5 before they are stored or summarized.
	submittedMessage := req.Message

	// 🛡️ AI安全检查 - 步骤0: 请求前安全检查
	var securityWarnings []string
	inputNormalized := false

	// 0.0 对抗样本防御（ASDF框架）- 在所有检测之前先归一化
	log.Printf("🛡️ [ASDF] 开始对抗样本检测与归一化")
	if contextService != nil {
		securityService := contextService.GetSecurityService()
		if securityService != nil {
			// 使用ASDF框架进行对抗样本防御
			normalizedMessage, isAdversarial, attackTypes, advConfidence := securityService.DefendAndNormalize(req.Message)

			if isAdversarial {
				log.Printf("🛡️ [ASDF] 检测到对抗样本攻击: types=%v, confidence=%.2f", attackTypes, advConfidence)
				log.Printf("🛡️ [ASDF] input normalized: session_id=%s message_bytes=%d", req.SessionID, len(req.Message))

				// 记录安全警告
				securityWarnings = append(securityWarnings,
					fmt.Sprintf("检测到对抗样本攻击: %v", attackTypes))

				// 使用归一化后的消息继续处理
				inputNormalized = normalizedMessage != req.Message
				req.Message = normalizedMessage
			}
		}
	}

	// 0.1 对抗样本检测（旧版检测器，保留作为双重保险）
	if adversarialDetector != nil {
		if isAdversarial, reason := adversarialDetector.DetectAdversarial(req.Message); isAdversarial {
			log.Printf("🚨 [AI安全] 检测到对抗样本攻击: %s", reason)
			if req.EvaluationInputOnly {
				writeInputOnlyDecision(c, "block", "adversarial_detector", false, inputNormalized, 0, len(securityWarnings)+1)
				return
			}
			c.JSON(http.StatusBadRequest, APIResponse{
				Success: false,
				Data:    nil,
				Error:   "检测到可疑输入，请求被拒绝: " + reason,
			})
			return
		}
	}

	if inputRequestDetector != nil {
		if blocked, reason := inputRequestDetector.Detect(req.Message); blocked {
			log.Printf("[AI security] blocked input request policy violation: %s", reason)
			if req.EvaluationInputOnly {
				writeInputOnlyDecision(c, "block", "input_request_policy", false, inputNormalized, 0, len(securityWarnings)+1)
				return
			}
			c.JSON(http.StatusBadRequest, APIResponse{
				Success: false,
				Data:    nil,
				Error:   "request cannot be processed because it may disclose protected information",
			})
			return
		}
	}

	// 0.2 DoS防护 - 检查速率限制
	if !req.EvaluationInputOnly && modelDosProtection != nil {
		estimatedTokens := security.EstimateTokens(req.Message)
		if err := modelDosProtection.CheckLimit(req.UserID, estimatedTokens); err != nil {
			log.Printf("🚨 [AI安全] 用户 %s 触发DoS防护: %v", req.UserID, err)
			c.JSON(http.StatusTooManyRequests, APIResponse{
				Success: false,
				Data:    nil,
				Error:   err.Error(),
			})
			return
		}
		// 记录请求
		modelDosProtection.RecordRequest(req.UserID, estimatedTokens)
	}

	// 0.3 数据投毒检测（如果消息过长，可能是知识库投毒尝试）
	if dataPoisoningDetector != nil && len(req.Message) > 500 {
		if isValid, reasons := dataPoisoningDetector.ValidateKnowledgeInput(req.Message); !isValid {
			log.Printf("🚨 [AI安全] 检测到数据投毒尝试: %v", reasons)
			securityWarnings = append(securityWarnings, "输入内容包含可疑模式")
			// 不直接拒绝，但记录警告
		}
	}

	// 🔒 步骤1: 敏感信息检测和脱敏
	var sensitiveInfos []SensitiveInfoDisplay
	originalMessage := submittedMessage
	redactedMessage := submittedMessage

	if contextService != nil {
		securityService := contextService.GetSecurityService()
		if securityService != nil {
			ctx := context.Background()
			scanResult, err := securityService.ScanContent(ctx, req.SessionID, req.UserID, originalMessage)
			if err != nil {
				log.Printf("⚠️ [聊天API] 敏感信息检测失败: %v", err)
			} else if len(scanResult.SensitiveInfos) > 0 {
				log.Printf("🔒 [聊天API] 检测到 %d 个敏感信息", len(scanResult.SensitiveInfos))

				// 转换为展示格式
				for _, info := range scanResult.SensitiveInfos {
					detectionLayer := "regex"
					if scanResult.Metadata != nil {
						if multiLayer, ok := scanResult.Metadata["multi_layer_used"].(bool); ok && multiLayer {
							detectionLayer = "multi_layer"
						}
					}

					sensitiveInfos = append(sensitiveInfos, SensitiveInfoDisplay{
						Type:           string(info.Type),
						Redacted:       "***",
						DetectionLayer: detectionLayer,
						Confidence:     info.Confidence,
					})
				}

				// 使用脱敏后的消息
				redactedMessage = scanResult.RedactedContent
				log.Printf("🔒 [聊天API] 已完成输入脱敏: sensitive_fields=%d", len(scanResult.SensitiveInfos))
			}
		}
	}

	if req.EvaluationInputOnly {
		inputRedacted := redactedMessage != originalMessage
		decision, stage := inputSecurityDecision(inputRedacted, inputNormalized)
		writeInputOnlyDecision(c, decision, stage, inputRedacted, inputNormalized, len(sensitiveInfos), len(securityWarnings))
		return
	}
	// General chat is not a clinical record lookup API. Short-circuit factual
	// status questions before retrieval or generation so an LLM cannot infer a
	// patient fact from the wording of the question itself.
	if isClinicalFactQuery(redactedMessage) {
		c.JSON(http.StatusOK, APIResponse{Success: true, Data: ChatResponse{
			SessionID:        req.SessionID,
			Message:          "当前普通对话不用于确认护理事实。请通过受保护的护理记录保存与交接摘要流程核对；未提供可核验记录时，系统不会确认是否已服药或发生异常。",
			UserMessage:      redactedMessage,
			Timestamp:        time.Now().Unix(),
			SecurityWarnings: securityWarnings,
		}})
		return
	}

	if isHandoverSummaryCommand(redactedMessage) {
		if contextService != nil && contextService.SessionStore() != nil {
			if err := contextService.SessionStore().SetSessionMetadata(req.SessionID, "userId", req.UserID); err != nil {
				log.Printf("⚠️ [聊天API] 设置会话userId失败: %v", err)
			}
		}
		writeHandoverSummary(c, req, redactedMessage, false, "")
		return
	}

	// Retrieve before persisting the current question so it cannot become its own evidence.
	userMemoryWriteApplied := false
	userMemoryID := ""

	// 💾 步骤2: 存储用户消息到记忆系统
	if false { // The write is deferred until after retrieval below.
		sessionStore := contextService.SessionStore()
		if sessionStore != nil {
			if err := sessionStore.SetSessionMetadata(req.SessionID, "userId", req.UserID); err != nil {
				log.Printf("⚠️ [聊天API] 设置会话userId失败: %v", err)
			}
		}

		storeReq := models.StoreContextRequest{
			SessionID: req.SessionID,
			UserID:    req.UserID,
			Content:   originalMessage, // 存储原始消息（系统内部加密）
			Metadata: map[string]interface{}{
				"role":      "user",
				"timestamp": time.Now().Unix(),
				"user_id":   req.UserID,
			},
		}

		ctx := context.Background()
		memoryID, err := contextService.StoreContext(ctx, storeReq)
		if err != nil {
			log.Printf("⚠️ [聊天API] 存储用户消息失败: %v", err)
		} else {
			userMemoryWriteApplied = true
			userMemoryID = memoryID
			log.Printf("💾 [聊天API] 用户消息已存储到记忆系统")
		}

		// 🔥 修复：同时存储到会话历史（用于短期记忆检索）
		if sessionStore != nil {
			userMessage := fmt.Sprintf("用户: %s", originalMessage)
			if err := sessionStore.UpdateSession(req.SessionID, userMessage); err != nil {
				log.Printf("⚠️ [聊天API] 更新会话历史失败: %v", err)
			} else {
				log.Printf("💾 [聊天API] 用户消息已添加到会话历史")
			}
		}
	}

	// 🔍 步骤3: 检索相关记忆
	var retrievedMemory string
	var memoryCount int

	if contextService != nil {
		retrieveReq := models.RetrieveContextRequest{
			SessionID: req.SessionID,
			UserID:    req.UserID,      // 🔒 添加UserID进行数据隔离
			Query:     redactedMessage, // 使用脱敏后的消息检索
			Limit:     2000,
			Strategy:  "balanced",
		}

		ctx := context.Background()
		retrieveResp, err := contextService.RetrieveContext(ctx, retrieveReq)
		if err != nil {
			log.Printf("⚠️ [聊天API] 检索记忆失败: %v", err)
		} else {
			// 组合检索到的记忆
			var memories []string
			if retrieveResp.ShortTermMemory != "" {
				memories = append(memories, "【短期记忆】\n"+retrieveResp.ShortTermMemory)
				memoryCount++
			}
			if retrieveResp.LongTermMemory != "" {
				memories = append(memories, "【长期记忆】\n"+retrieveResp.LongTermMemory)
				memoryCount++
			}
			if retrieveResp.RelevantKnowledge != "" {
				memories = append(memories, "【相关知识】\n"+retrieveResp.RelevantKnowledge)
				memoryCount++
			}

			if len(memories) > 0 {
				retrievedMemory = strings.Join(memories, "\n\n")
				log.Printf("🔍 [聊天API] 检索到 %d 条相关记忆", memoryCount)
				// 添加记忆内容预览（最多200字符）
				previewLen := 200
				if len(retrievedMemory) < previewLen {
					previewLen = len(retrievedMemory)
				}
				log.Printf("🔍 [聊天API] 记忆内容预览: %s", retrievedMemory[:previewLen])
			}
		}
	}

	// 📎 处理文件附件
	var fileContents string
	if len(req.Files) > 0 {
		log.Printf("📎 [聊天API] 检测到 %d 个文件附件", len(req.Files))
		var fileTexts []string
		for _, fileAttachment := range req.Files {
			// 从file_handlers.go的全局存储中获取文件元数据
			if metadata, exists := fileMetadataStore[fileAttachment.FileID]; exists {
				log.Printf("📎 [聊天API] 读取文件: %s (ID: %s)", metadata.OriginalName, fileAttachment.FileID)

				// 读取文件内容（支持文本、PDF和图片文件）
				// 🔧 使用原始文件名的扩展名，而不是加密后的文件路径扩展名
				fileExt := strings.ToLower(filepath.Ext(metadata.OriginalName))

				// 处理文本文件
				if fileExt == ".txt" || fileExt == ".md" {
					// 🔓 读取并解密文本文件
					encryptedContent, err := os.ReadFile(metadata.FilePath)
					if err != nil {
						log.Printf("⚠️ [聊天API] 读取加密文件失败: %v", err)
						fileTexts = append(fileTexts, fmt.Sprintf("\n[文件: %s - 读取失败: %v]", metadata.OriginalName, err))
						continue
					}

					// 解密文件内容
					encryption := security.NewFileEncryption(metadata.UserID)
					content, err := encryption.Decrypt(encryptedContent)
					if err != nil {
						log.Printf("⚠️ [聊天API] 解密文件失败: %v", err)
						fileTexts = append(fileTexts, fmt.Sprintf("\n[文件: %s - 解密失败: %v]", metadata.OriginalName, err))
						continue
					}

					fileTexts = append(fileTexts, fmt.Sprintf("\n[文件: %s]\n%s\n[文件结束]", metadata.OriginalName, string(content)))
					log.Printf("📎 [聊天API] 成功读取并解密文本文件，长度: %d 字节", len(content))
				} else if fileExt == ".pdf" {
					// 处理PDF文件
					log.Printf("📄 [聊天API] 开始提取PDF文本: %s", metadata.OriginalName)

					// 🔓 读取并解密PDF文件
					encryptedContent, err := os.ReadFile(metadata.FilePath)
					if err != nil {
						log.Printf("⚠️ [聊天API] 读取加密PDF失败: %v", err)
						fileTexts = append(fileTexts, fmt.Sprintf("\n[文件: %s - 读取失败: %v]", metadata.OriginalName, err))
						continue
					}

					// 解密文件内容
					encryption := security.NewFileEncryption(metadata.UserID)
					pdfContent, err := encryption.Decrypt(encryptedContent)
					if err != nil {
						log.Printf("⚠️ [聊天API] 解密PDF失败: %v", err)
						fileTexts = append(fileTexts, fmt.Sprintf("\n[文件: %s - 解密失败: %v]", metadata.OriginalName, err))
						continue
					}

					// 保存临时PDF文件用于文本提取
					tempDir := filepath.Join("./data/temp", metadata.UserID)
					if err := os.MkdirAll(tempDir, 0755); err != nil {
						log.Printf("⚠️ [聊天API] 创建临时目录失败: %v", err)
						fileTexts = append(fileTexts, fmt.Sprintf("\n[文件: %s - 处理失败]", metadata.OriginalName))
						continue
					}

					tempPDFPath := filepath.Join(tempDir, fmt.Sprintf("%s_%s", fileAttachment.FileID, metadata.OriginalName))
					if err := os.WriteFile(tempPDFPath, pdfContent, 0644); err != nil {
						log.Printf("⚠️ [聊天API] 保存临时PDF失败: %v", err)
						fileTexts = append(fileTexts, fmt.Sprintf("\n[文件: %s - 处理失败]", metadata.OriginalName))
						continue
					}

					// 确保处理完后删除临时文件
					defer os.Remove(tempPDFPath)

					// 提取PDF文本
					pdfText, err := utils.ExtractTextFromPDF(tempPDFPath)
					if err != nil {
						log.Printf("⚠️ [聊天API] PDF文本提取失败: %v", err)
						fileTexts = append(fileTexts, fmt.Sprintf("\n[文件: %s - PDF文本提取失败: %v]", metadata.OriginalName, err))
					} else {
						fileTexts = append(fileTexts, fmt.Sprintf("\n[PDF文件: %s]\n%s\n[PDF文件结束]", metadata.OriginalName, pdfText))
						log.Printf("📄 [聊天API] 成功提取PDF文本，长度: %d 字节", len(pdfText))
					}
				} else if fileExt == ".jpg" || fileExt == ".jpeg" || fileExt == ".png" || fileExt == ".gif" || fileExt == ".bmp" || fileExt == ".webp" {
					// 处理图片文件 - 使用OCR识别文字
					log.Printf("🖼️ [聊天API] 开始处理图片文件: %s", metadata.OriginalName)

					// 🔓 步骤1: 读取并解密图片文件
					encryptedContent, err := os.ReadFile(metadata.FilePath)
					if err != nil {
						log.Printf("⚠️ [聊天API] 读取加密图片失败: %v", err)
						fileTexts = append(fileTexts, fmt.Sprintf("\n[文件: %s - 读取失败: %v]", metadata.OriginalName, err))
						continue
					}

					// 解密文件内容
					encryption := security.NewFileEncryption(metadata.UserID)
					imageContent, err := encryption.Decrypt(encryptedContent)
					if err != nil {
						log.Printf("⚠️ [聊天API] 解密图片失败: %v", err)
						fileTexts = append(fileTexts, fmt.Sprintf("\n[文件: %s - 解密失败: %v]", metadata.OriginalName, err))
						continue
					}

					log.Printf("✅ [聊天API] 图片解密成功，大小: %d 字节", len(imageContent))

					// 🔍 步骤2: 使用OCR识别图片中的文字
					// 保存临时图片文件用于OCR识别
					tempDir := filepath.Join("./data/temp", metadata.UserID)
					if err := os.MkdirAll(tempDir, 0755); err != nil {
						log.Printf("⚠️ [聊天API] 创建临时目录失败: %v", err)
						fileTexts = append(fileTexts, fmt.Sprintf("\n[文件: %s - 处理失败]", metadata.OriginalName))
						continue
					}

					tempImagePath := filepath.Join(tempDir, fmt.Sprintf("%s_%s", fileAttachment.FileID, metadata.OriginalName))
					if err := os.WriteFile(tempImagePath, imageContent, 0644); err != nil {
						log.Printf("⚠️ [聊天API] 保存临时图片失败: %v", err)
						fileTexts = append(fileTexts, fmt.Sprintf("\n[文件: %s - 处理失败]", metadata.OriginalName))
						continue
					}

					// 确保处理完后删除临时文件
					defer os.Remove(tempImagePath)

					// 使用OCR识别图片中的文字
					ocrService := services.NewOCRService(logrus.New())
					ocrResult, err := ocrService.RecognizeMedicalReport(tempImagePath)
					if err != nil {
						log.Printf("⚠️ [聊天API] OCR识别失败: %v", err)
						fileTexts = append(fileTexts, fmt.Sprintf("\n[图片: %s - OCR识别失败: %v]", metadata.OriginalName, err))
					} else if len(ocrResult.Text) == 0 {
						log.Printf("⚠️ [聊天API] OCR未识别到文字内容")
						fileTexts = append(fileTexts, fmt.Sprintf("\n[图片: %s - 未识别到文字内容]", metadata.OriginalName))
					} else {
						log.Printf("✅ [聊天API] OCR识别成功 - 置信度: %.2f%%, 文本长度: %d字符", ocrResult.Confidence, len(ocrResult.Text))
						fileTexts = append(fileTexts, fmt.Sprintf("\n[图片: %s - OCR识别结果]\n%s\n[图片识别结束]", metadata.OriginalName, ocrResult.Text))
					}
				} else if fileExt == ".docx" || fileExt == ".doc" {
					// 对于Office文档，提示用户这些文件类型暂不支持直接读取
					fileTexts = append(fileTexts, fmt.Sprintf("\n[文件: %s - 文件类型 %s 暂不支持直接读取，请转换为文本格式]", metadata.OriginalName, fileExt))
					log.Printf("⚠️ [聊天API] 文件类型 %s 暂不支持", fileExt)
				} else {
					fileTexts = append(fileTexts, fmt.Sprintf("\n[文件: %s - 非文本文件]", metadata.OriginalName))
					log.Printf("⚠️ [聊天API] 不支持的文件类型: %s", fileExt)
				}
			} else {
				log.Printf("⚠️ [聊天API] 文件ID %s 未找到", fileAttachment.FileID)
				fileTexts = append(fileTexts, fmt.Sprintf("\n[文件ID: %s - 未找到]", fileAttachment.FileID))
			}
		}
		if len(fileTexts) > 0 {
			fileContents = strings.Join(fileTexts, "\n")
		}
	}

	if isClinicalFactQuery(redactedMessage) && memoryCount == 0 {
		c.JSON(http.StatusOK, APIResponse{Success: true, Data: ChatResponse{
			SessionID:          req.SessionID,
			Message:            "当前受保护会话未检索到可核验的护理记录，无法确认该事实。请先使用保存护理记录功能录入，或由护理人员核对原始记录。",
			UserMessage:        redactedMessage,
			Timestamp:          time.Now().Unix(),
			SecurityWarnings:   securityWarnings,
			MemoryWriteApplied: false,
		}})
		return
	}

	// Persist the message after recall. It will be available to later turns but
	// cannot be mistaken for evidence when answering this turn.
	storeReq := models.StoreContextRequest{
		SessionID: req.SessionID,
		UserID:    req.UserID,
		Content:   originalMessage,
		Metadata: map[string]interface{}{
			"role":      "user",
			"timestamp": time.Now().Unix(),
			"user_id":   req.UserID,
		},
	}
	if memoryID, err := contextService.StoreContext(context.Background(), storeReq); err != nil {
		log.Printf("[Chat] failed to persist user message: %v", err)
	} else {
		userMemoryWriteApplied = true
		userMemoryID = memoryID
	}
	if err := contextService.SessionStore().UpdateSession(req.SessionID, fmt.Sprintf("用户: %s", originalMessage)); err != nil {
		log.Printf("[Chat] failed to update protected session history: %v", err)
	}

	// 🤖 Agent模式分支：如果开启Agent，走ReAct推理循环
	if req.AgentMode {
		runAgentMode(c, req, redactedMessage, retrievedMemory, memoryCount, sensitiveInfos, securityWarnings, userMemoryWriteApplied, userMemoryID)
		return
	}

	// 构建健康助手的系统提示词（根据用户角色）
	systemPrompt := buildHealthAssistantPrompt(req.UserID)

	// 🔥 新增：使用幻觉检测器增强提示词，强制要求AI标注来源
	hallucinationDetector := security.NewHallucinationDetector(logrus.New())
	systemPrompt = hallucinationDetector.EnhancePromptWithSourceRequirement(systemPrompt)

	// 如果有文件内容，将其添加到用户消息中
	messageWithFiles := redactedMessage
	if fileContents != "" {
		messageWithFiles = redactedMessage + "\n\n" + fileContents
	}

	// 构建完整的对话上下文（包含检索到的记忆和文件内容）
	conversationContext := buildConversationContextWithMemory(req.History, messageWithFiles, retrievedMemory)

	// 组合系统提示词和对话上下文
	fullPrompt := fmt.Sprintf("%s\n\n%s", systemPrompt, conversationContext)

	// 调用LLM服务生成回复
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// 🤖 调用LLM生成回复（OCR已经提取了图片中的文字，统一使用文本模型处理）
	generateReq := &services.GenerateRequest{
		Prompt:      fullPrompt,
		MaxTokens:   2000,
		Temperature: 0.7,
		Format:      "",
	}

	log.Printf("🤖 [聊天API] 开始调用LLM，用户: %s, 会话: %s, 消息长度: %d, 记忆条数: %d",
		req.UserID, req.SessionID, len(redactedMessage), memoryCount)

	response, err := llmService.GenerateResponse(ctx, generateReq)

	if err != nil {
		log.Printf("❌ [聊天API] LLM调用失败: %v", err)
		c.JSON(http.StatusInternalServerError, APIResponse{
			Success: false,
			Data:    nil,
			Error:   fmt.Sprintf("生成回复失败: %v", err),
		})
		return
	}
	storedAssistantResponse, _ := enforceOutputRedaction(response.Content)

	// 💾 步骤4: 存储文件内容到记忆系统（如果有文件）
	if contextService != nil && fileContents != "" {
		fileStoreReq := models.StoreContextRequest{
			SessionID: req.SessionID,
			UserID:    req.UserID,
			Content:   fileContents,
			Metadata: map[string]interface{}{
				"role":       "user",
				"type":       "file_content",
				"file_count": len(req.Files),
				"timestamp":  time.Now().Unix(),
			},
		}

		ctx := context.Background()
		_, err := contextService.StoreContext(ctx, fileStoreReq)
		if err != nil {
			log.Printf("⚠️ [聊天API] 存储文件内容到记忆失败: %v", err)
		} else {
			log.Printf("💾 [聊天API] 文件内容已存储到记忆系统，文件数: %d", len(req.Files))
		}
	}

	// 💾 步骤5: 存储AI回复到记忆系统
	if contextService != nil {
		storeReq := models.StoreContextRequest{
			SessionID: req.SessionID,
			UserID:    req.UserID,
			Content:   storedAssistantResponse,
			Metadata: map[string]interface{}{
				"role":      "assistant",
				"timestamp": time.Now().Unix(),
			},
		}

		ctx := context.Background()
		_, err := contextService.StoreContext(ctx, storeReq)
		if err != nil {
			log.Printf("⚠️ [聊天API] 存储AI回复失败: %v", err)
		} else {
			log.Printf("💾 [聊天API] AI回复已存储到记忆系统")
		}

		// 🔥 修复：同时存储到会话历史（用于短期记忆检索）
		sessionStore := contextService.SessionStore()
		if sessionStore != nil {
			assistantMessage := fmt.Sprintf("助手: %s", storedAssistantResponse)
			if err := sessionStore.UpdateSession(req.SessionID, assistantMessage); err != nil {
				log.Printf("⚠️ [聊天API] 更新会话历史失败: %v", err)
			} else {
				log.Printf("💾 [聊天API] AI回复已添加到会话历史")
			}
		}
	}

	// 🛡️ AI安全检查 - 步骤5: 响应后处理
	filteredResponse := response.Content
	var outputWarnings []string
	confidenceScore := 0.0
	isFiltered := false

	// 5.0 幻觉检测 - 检测AI是否可能在编造信息
	hallucinationResult := hallucinationDetector.DetectHallucination(
		response.Content,
		retrievedMemory,
		redactedMessage,
	)

	if hallucinationResult.IsLikelyHallucination {
		log.Printf("🚨 [AI安全] 检测到可能的AI幻觉，风险等级: %s, 置信度: %.2f, 原因: %v",
			hallucinationResult.RiskLevel, hallucinationResult.Confidence, hallucinationResult.Reasons)

		// 根据风险等级采取不同措施
		if hallucinationResult.RiskLevel == "high" {
			// 高风险：拒绝输出，返回安全提示
			log.Printf("❌ [AI安全] 高风险幻觉，拒绝输出原始回复")
			filteredResponse = "抱歉，我无法确认这些信息的准确性。请使用系统的查询功能获取准确的健康数据，或联系医护人员获取帮助。"
			isFiltered = true
			outputWarnings = append(outputWarnings, "AI回复可能包含不准确信息，已替换为安全提示")
		} else if hallucinationResult.RiskLevel == "medium" {
			// 中风险：添加警告标签
			warningMsg := hallucinationDetector.GenerateWarningMessage(hallucinationResult)
			outputWarnings = append(outputWarnings, warningMsg)
			log.Printf("⚠️ [AI安全] 中风险幻觉，添加警告标签")
		}
		// 低风险：正常输出，但记录日志
	}

	// 5.1 输出过滤 - 过滤敏感信息
	if outputFilter != nil {
		var warnings []string
		filteredResponse, warnings = outputFilter.FilterOutput(filteredResponse)
		if len(warnings) > 0 {
			outputWarnings = append(outputWarnings, warnings...)
			isFiltered = true
			log.Printf("🔒 [AI安全] 输出已过滤，警告: %v", warnings)
		}
	}

	// 🔒 额外检查：使用敏感信息检测器再次扫描AI回复
	// 防止AI在回复中重复用户输入的敏感信息
	if contextService != nil {
		securityService := contextService.GetSecurityService()
		if securityService != nil {
			ctx := context.Background()
			scanResult, err := securityService.ScanContent(ctx, req.SessionID, req.UserID, filteredResponse)
			if err != nil {
				log.Printf("⚠️ [AI安全] AI回复敏感信息检测失败: %v", err)
			} else if len(scanResult.SensitiveInfos) > 0 {
				log.Printf("🚨 [AI安全] AI回复中检测到 %d 个敏感信息，进行脱敏", len(scanResult.SensitiveInfos))
				filteredResponse = scanResult.RedactedContent
				isFiltered = true
				outputWarnings = append(outputWarnings, fmt.Sprintf("AI回复中检测到%d个敏感信息已自动脱敏", len(scanResult.SensitiveInfos)))
			}
		}
	}
	if enforcedResponse, redacted := enforceOutputRedaction(filteredResponse); redacted {
		filteredResponse = enforcedResponse
		isFiltered = true
		outputWarnings = append(outputWarnings, "AI回复中的敏感字段已按最终输出策略脱敏")
	}

	// 5.2 置信度评分
	if confidenceScorer != nil {
		hasKnowledgeData := memoryCount > 0
		confidenceScore = confidenceScorer.ScoreResponse(filteredResponse, hasKnowledgeData)
		log.Printf("📊 [AI安全] 响应置信度: %.2f", confidenceScore)
	}

	// 5.3 访问监控 - 记录访问并检测模型窃取
	if modelAccessMonitor != nil {
		estimatedTokens := security.EstimateTokens(response.Content)
		modelAccessMonitor.RecordAccess(req.UserID, estimatedTokens)

		// 检测模型窃取
		if isTheft, reason := modelAccessMonitor.DetectTheft(req.UserID); isTheft {
			log.Printf("🚨 [AI安全] 检测到模型窃取行为: 用户=%s, 原因=%s", req.UserID, reason)
			securityWarnings = append(securityWarnings, "检测到异常访问模式")
		}
	}

	// 合并所有安全警告
	allWarnings := append(securityWarnings, outputWarnings...)

	timestamp := time.Now().Unix()

	log.Printf("✅ [聊天API] LLM回复成功，会话: %s, 回复长度: %d, 敏感信息: %d, 置信度: %.2f, 过滤: %v",
		req.SessionID, len(filteredResponse), len(sensitiveInfos), confidenceScore, isFiltered)

	// 返回响应
	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data: ChatResponse{
			SessionID:          req.SessionID,
			Message:            filteredResponse,
			UserMessage:        redactedMessage, // 返回脱敏后的用户消息
			Timestamp:          timestamp,
			SensitiveInfos:     sensitiveInfos,
			RetrievedMemory:    retrievedMemory,
			MemoryCount:        memoryCount,
			ConfidenceScore:    confidenceScore,
			SecurityWarnings:   allWarnings,
			Filtered:           isFiltered,
			MemoryWriteApplied: userMemoryWriteApplied,
			MemoryID:           userMemoryID,
		},
		Error: "",
	})
}

// runAgentMode 执行Agent模式的ReAct推理循环
func runAgentMode(c *gin.Context, req ChatRequest, redactedMessage, retrievedMemory string, memoryCount int, sensitiveInfos []SensitiveInfoDisplay, securityWarnings []string, memoryWriteApplied bool, memoryID string) {
	// The JWT role is authoritative. Legacy user-ID prefixes are only a
	// compatibility fallback for callers using the generic demo login.
	role := "caregiver"
	if value, exists := c.Get("role"); exists {
		if jwtRole, ok := value.(string); ok && jwtRole != "" {
			role = jwtRole
		}
	} else if strings.HasPrefix(req.UserID, "doctor_") {
		role = "doctor"
	} else if strings.HasPrefix(req.UserID, "family_") {
		role = "family"
	} else if strings.HasPrefix(req.UserID, "elder_") {
		role = "elder"
	}

	// 创建工具依赖
	deps := &agenttools.Deps{
		ContextService: contextService,
		SessionStore:   contextService.SessionStore(),
		UserID:         req.UserID,
		SessionID:      req.SessionID,
	}

	registry := newControlledAgentRegistry(llmService.(agent.LLMCaller))

	// 构建Agent系统提示词
	toolDescs := registry.ToolDescriptions()
	systemPrompt := agent.BuildAgentSystemPrompt(role, toolDescs)
	userPrompt := agent.BuildAgentQueryPrompt(redactedMessage, retrievedMemory)

	// 创建编排器并执行
	orchestrator := agent.NewOrchestrator(llmService.(agent.LLMCaller), registry, systemPrompt)
	agentCtx := agenttools.WithDeps(c.Request.Context(), deps)
	trace := orchestrator.Run(agentCtx, userPrompt)

	log.Printf("🤖 [Agent] 推理完成: %d轮, %d次工具调用, %dms, fallback=%v",
		trace.Iterations, trace.ToolCalls, trace.TotalTimeMs, trace.Fallback)

	// Agent output is a reviewable draft. It is intentionally not persisted
	// automatically: nursing-record writes, notifications and deletion require
	// an explicit, separately authorised confirmation workflow.

	// 输出过滤
	filteredResponse := trace.FinalAnswer
	if outputFilter != nil {
		var warnings []string
		filteredResponse, warnings = outputFilter.FilterOutput(trace.FinalAnswer)
		if len(warnings) > 0 {
			securityWarnings = append(securityWarnings, warnings...)
		}
	}

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data: ChatResponse{
			SessionID:          req.SessionID,
			Message:            filteredResponse,
			UserMessage:        redactedMessage,
			Timestamp:          time.Now().Unix(),
			SensitiveInfos:     sensitiveInfos,
			RetrievedMemory:    retrievedMemory,
			MemoryCount:        memoryCount,
			SecurityWarnings:   securityWarnings,
			MemoryWriteApplied: memoryWriteApplied,
			MemoryID:           memoryID,
			AgentExecution:     newAgentExecutionSummary(trace),
		},
		Error: "",
	})
}

// newControlledAgentRegistry is the allowlist for the HTTP competition path.
// Every registered tool is read-only and produces evidence or a review draft.
func newControlledAgentRegistry(caller agent.LLMCaller) *agent.ToolRegistry {
	registry := agent.NewToolRegistry()
	registry.SetAllowlist("memory_search", "data_manage", "auto_summary", "authoritative_web_search")
	registry.Register(&agenttools.MemorySearchTool{})
	registry.Register(&agenttools.DataManageTool{})
	registry.Register(agent.NewSummaryTool(caller))
	registry.Register(agent.NewAuthoritativeWebSearchTool())
	return registry
}

// buildHealthAssistantPrompt 构建养老院护理助手的系统提示词（根据用户角色）
func buildHealthAssistantPrompt(userID string) string {
	// 根据用户ID前缀判断角色
	var rolePrompt string

	if strings.HasPrefix(userID, "caregiver_") {
		// 护工端提示词
		rolePrompt = `你是养老院护工助手。你的职责是：
1. 回答护理相关的问题和提供建议
2. 帮助护工记录、查询和整理护理信息
3. 提供护理建议和注意事项
4. 识别异常数据并提醒

重要原则：
- 当前会话中的护理输入会由系统尝试安全写入受保护记忆；只有接口返回写入成功状态时，才可确认记录已保存
- 对用户提出的“生成交接摘要”，只汇总当前会话中已保存的记录，不编造或补充未记录的数据
- 如果用户发送了敏感信息（身份证号、电话号码等），提醒注意隐私保护，不要在对话中传输敏感信息
- 只基于实际检索到的记忆数据回答问题，不要编造任何信息
- 如果没有相关数据，诚实告知"暂无数据"

回复要简洁专业，使用护理术语。`
	} else if strings.HasPrefix(userID, "doctor_") {
		// 医生端提示词
		rolePrompt = `你是养老院医生助手。你的职责是：
1. 帮助医生查询和分析老人健康数据
2. 基于实际数据提供趋势分析
3. 识别异常数据并给出诊断建议
4. 回答医疗相关问题

重要原则：
- 只基于【相关记忆】中实际检索到的数据进行分析
- 如果没有查询到相关数据，明确告知"暂无该老人的健康数据记录"
- 不要编造任何健康数据、趋势或图表
- 如果数据不足以做出分析，诚实说明需要更多数据
- 提醒医生使用系统的专门查询功能获取完整数据

回复要专业准确，使用医学术语，基于实际数据提供分析。`
	} else if strings.HasPrefix(userID, "family_") {
		// 家属端提示词
		rolePrompt = `你是养老院家属助手。你的职责是：
1. 向家属报告老人的健康状况
2. 解答家属的关心和疑问
3. 提供探视建议和注意事项
4. 用通俗易懂的语言解释医疗信息

重要原则：
- 只基于【相关记忆】中实际检索到的数据回答问题
- 如果没有老人的健康数据，诚实告知"暂无该老人的最新健康记录"
- 不要编造老人的健康状况、护理情况或任何数据
- 如果家属需要详细信息，建议联系护工或医生获取最新情况

使用温暖、关怀的语气，让家属放心。避免使用过于专业的医学术语，用家属能理解的方式解释。`
	} else if strings.HasPrefix(userID, "elder_") {
		// 老人端提示词
		rolePrompt = `你是老人健康助手。你的职责是：
1. 用简单、温暖的语言与老人交流
2. 基于实际数据告诉老人他们的健康状况
3. 提供生活建议和鼓励
4. 回答老人的日常问题

重要原则：
- 只基于【相关记忆】中实际检索到的数据回答健康问题
- 如果没有健康数据，不要编造，可以说"我还没有看到您最近的健康记录"
- 不要编造血压、体温等任何健康数据
- 保持温暖关怀的语气，但确保信息真实

使用：
- 简短的句子
- 通俗的词汇
- 温暖的语气
- 适合语音播报的表达

避免使用医学术语，用老人能理解的方式说话。`
	} else {
		// 默认护理助手提示词
		rolePrompt = `你是养老院护理助手AI，帮助护理人员提供专业建议。

核心功能：
1. 提供护理建议：日常护理、生命体征监测、用药提醒、饮食建议
2. 异常识别：识别跌倒风险、情绪异常、生命体征异常等情况
3. 护理知识：压疮预防、跌倒预防、营养支持等专业知识
4. 信息查询：基于实际记录的数据回答问题

重要原则：
- 系统没有自动记录功能，不要假装已经记录了任何数据
- 如果用户想记录数据，明确告知需要使用系统的专门记录功能
- 只基于【相关记忆】中实际检索到的数据回答问题
- 如果没有相关数据，诚实告知"暂无相关记录"
- 不要编造任何老人信息、健康数据或护理记录
- 如果用户发送了敏感信息，提醒注意隐私保护

回复原则：
- 简洁专业，使用护理术语
- 关注老人安全
- 紧急情况建议立即通知医生
- 不提供诊断，只提供护理建议`
	}

	// 添加通用的记忆使用规则
	commonRules := `

记忆使用规则：
- 优先使用【相关记忆】中的老人信息
- 记忆中有的信息直接使用，不要重复询问
- 记忆中没有的信息再询问用户`

	commonRules += `

事实核验规则：
- 对“是否已服药、是否发生异常、生命体征是多少”等事实问题，只有【相关记忆】包含可核验记录时才可肯定回答。
- 不得将用户的提问、推测或常识当作护理事实；没有可核验记录时明确说明无法确认。`
	return rolePrompt + commonRules
}

// buildConversationContextWithMemory 构建对话上下文（包含检索到的记忆）
func buildConversationContextWithMemory(history []ChatMessage, currentMessage string, retrievedMemory string) string {
	context := ""

	// 如果有检索到的记忆，用醒目的方式展示
	if retrievedMemory != "" {
		context += "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"
		context += "📋 【相关记忆】（请仔细阅读并使用这些信息回答用户问题）\n"
		context += "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"
		context += retrievedMemory
		context += "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n"
	}

	context += "对话历史：\n"

	// 添加历史消息（最多保留最近5轮对话）
	startIndex := 0
	if len(history) > 10 { // 5轮对话 = 10条消息（用户+助手）
		startIndex = len(history) - 10
	}

	for i := startIndex; i < len(history); i++ {
		msg := history[i]
		if msg.Role == "user" {
			context += fmt.Sprintf("用户：%s\n", msg.Content)
		} else if msg.Role == "assistant" {
			context += fmt.Sprintf("助手：%s\n", msg.Content)
		}
	}

	// 添加当前用户消息
	context += fmt.Sprintf("\n当前用户消息：%s\n\n请回复：", currentMessage)

	return context
}

// buildConversationContext 构建对话上下文（保留原有函数以兼容）
func buildConversationContext(history []ChatMessage, currentMessage string) string {
	return buildConversationContextWithMemory(history, currentMessage, "")
}

// RegisterChatRoutes 注册聊天相关的路由
func RegisterChatRoutes(router gin.IRouter) {
	// 直接在传入的router上注册路由（可能是RouterGroup）
	// POST /api/chat - 发送聊天消息
	router.POST("", ChatHandler)
}
