package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/contextkeeper/service/internal/agent"
	"github.com/contextkeeper/service/internal/config"
	"github.com/contextkeeper/service/internal/models"
	"github.com/contextkeeper/service/internal/security"
	"github.com/contextkeeper/service/internal/services"
	"github.com/contextkeeper/service/internal/utils"
	"github.com/contextkeeper/service/pkg/aliyun"
	"github.com/gin-gonic/gin"
)

// 全局变量
var (
	startTime             = time.Now()              // 记录服务启动时间
	globalSecurityService *security.SecurityService // 全局安全服务，供health_handlers.go使用
)

// projectAnalysisJSON 定义与prompt JSON格式对应的结构
type projectAnalysisJSON struct {
	ProjectName        string   `json:"project_name"`
	Description        string   `json:"description"`
	ProjectType        string   `json:"project_type"`
	PrimaryLanguage    string   `json:"primary_language"`
	TechStack          string   `json:"tech_stack"`
	Architecture       string   `json:"architecture"`
	MainFramework      string   `json:"main_framework"`
	Database           string   `json:"database"`
	KeyDependencies    []string `json:"key_dependencies"`
	RecentFocus        string   `json:"recent_focus"`
	CurrentPainPoints  []string `json:"current_pain_points"`
	ActiveRequirements []string `json:"active_requirements"`
	MainComponents     []struct {
		Name       string `json:"name"`
		Purpose    string `json:"purpose"`
		Importance string `json:"importance"`
	} `json:"main_components"`
	ImportantFiles []struct {
		FilePath    string `json:"file_path"`
		Role        string `json:"role"`
		Criticality string `json:"criticality"`
	} `json:"important_files"`
	CurrentPhase    string  `json:"current_phase"`
	ConfidenceLevel float64 `json:"confidence_level"`
}

// 活跃的SSE连接请求通道
var (
	sseRequestChannels     = make(map[uint64]chan map[string]interface{})
	sseRequestChannelMutex sync.RWMutex
)

// RegisterSSERequestChannel 注册一个SSE连接的请求通道
func RegisterSSERequestChannel(connID uint64, channel chan map[string]interface{}) {
	sseRequestChannelMutex.Lock()
	defer sseRequestChannelMutex.Unlock()
	sseRequestChannels[connID] = channel
}

// UnregisterSSERequestChannel 注销一个SSE连接的请求通道
func UnregisterSSERequestChannel(connID uint64) {
	sseRequestChannelMutex.Lock()
	defer sseRequestChannelMutex.Unlock()
	delete(sseRequestChannels, connID)
}

// BroadcastRequest 广播请求到所有活跃的SSE连接
func BroadcastRequest(request map[string]interface{}) {
	method, _ := request["method"].(string)
	id, _ := request["id"].(string)

	log.Printf("[广播] 正在广播请求, 方法: %s, ID: %s", method, id)

	sseRequestChannelMutex.RLock()

	// 如果没有活跃连接，记录警告
	if len(sseRequestChannels) == 0 {
		log.Printf("[广播警告] 没有活跃的SSE连接，请求将不会被处理")
		sseRequestChannelMutex.RUnlock()
		return
	}

	log.Printf("[广播] 共有 %d 个活跃的SSE连接", len(sseRequestChannels))

	// 创建一个副本避免死锁
	channelCopy := make(map[uint64]chan map[string]interface{}, len(sseRequestChannels))
	for connID, ch := range sseRequestChannels {
		channelCopy[connID] = ch
	}

	// 复制请求对象，防止并发修改
	requestCopy := make(map[string]interface{})
	for k, v := range request {
		requestCopy[k] = v
	}

	// 完成数据复制后释放锁
	sseRequestChannelMutex.RUnlock()

	// 广播到所有通道，不持有锁
	for connID, channel := range channelCopy {
		// 使用goroutine避免阻塞
		go func(id uint64, ch chan map[string]interface{}) {
			// 使用超时机制发送
			select {
			case ch <- requestCopy:
				log.Printf("[广播] 已将请求发送到SSE连接 %d, 方法: %s, ID: %d", id, method, id)
			case <-time.After(500 * time.Millisecond):
				log.Printf("[广播错误] 发送请求到SSE连接 %d 超时: 通道可能已满, 方法: %s, ID: %d", id, method, id)
			}
		}(connID, channel)
	}
}

// Handler API处理器
type Handler struct {
	contextService          *services.LLMDrivenContextService // 🔥 修改为LLMDrivenContextService以支持LLM驱动智能功能
	vectorService           *aliyun.VectorService
	userRepository          models.UserRepository             // 新增：用户存储接口
	localInstructionService *services.LocalInstructionService // 新增：本地指令服务
	config                  *config.Config                    // 新增：配置
	batchEmbeddingHandler   *BatchEmbeddingHandler            // 🔥 新增：批量embedding处理器
	unifiedContextManager   *services.UnifiedContextManager   // 🔥 新增：统一上下文管理器
	wideRecallService       *services.WideRecallService       // 🔥 新增：宽召回服务
	securityService         *security.SecurityService         // 🔥 新增：安全服务
	agentCaller             agent.LLMCaller                   // 受保护 Agent 入口的可替换调用方（契约测试使用）
	controlledAgentRunner   controlledAgentRunner             // 受保护 Agent 入口的可替换执行器（契约测试使用）
	startTime               time.Time
}

// GetBatchEmbeddingHandler 获取批量embedding处理器
func (h *Handler) GetBatchEmbeddingHandler() *BatchEmbeddingHandler {
	return h.batchEmbeddingHandler
}

// GetSecurityService exposes the shared security chain to protected feature
// handlers without duplicating detector configuration.
func (h *Handler) GetSecurityService() *security.SecurityService {
	return h.securityService
}

// NewHandler 创建新的API处理器（🔥 修改：现在接受LLMDrivenContextService）
func NewHandler(contextService *services.LLMDrivenContextService, vectorService *aliyun.VectorService, userRepository models.UserRepository, cfg *config.Config, securityService *security.SecurityService) *Handler {
	// 设置全局安全服务，供health_handlers.go中的独立函数使用
	globalSecurityService = securityService

	h := &Handler{
		contextService:          contextService,
		vectorService:           vectorService,
		userRepository:          userRepository,
		localInstructionService: services.NewLocalInstructionService(), // 使用系统标准路径
		config:                  cfg,
		wideRecallService:       nil,             // TODO: 初始化宽召回服务
		securityService:         securityService, // 🔥 新增：安全服务
		startTime:               time.Now(),
	}

	// 🔥 新增：初始化批量embedding服务
	if cfg.BatchEmbeddingAPIURL != "" && cfg.BatchEmbeddingAPIKey != "" {
		log.Printf("[批量Embedding] 初始化批量embedding服务...")
		batchService := aliyun.NewBatchEmbeddingService(
			cfg.BatchEmbeddingAPIURL,
			cfg.BatchEmbeddingAPIKey,
			cfg.BatchQueueSize,
		)

		// 启动异步worker
		if err := batchService.StartWorker(); err != nil {
			log.Printf("[批量Embedding] 启动异步worker失败: %v", err)
		} else {
			log.Printf("[批量Embedding] 异步worker启动成功")
		}

		h.batchEmbeddingHandler = NewBatchEmbeddingHandler(batchService)
		log.Printf("[批量Embedding] 批量embedding服务初始化完成")
	} else {
		log.Printf("[批量Embedding] 批量embedding配置未设置，跳过初始化")
	}

	// 🔥 新增：初始化统一上下文管理器
	log.Printf("🧠 [统一上下文] 初始化统一上下文管理器...")

	// 创建真实的LLM服务
	provider := os.Getenv("LLM_PROVIDER")
	if provider == "" {
		provider = "deepseek"
	}

	model := os.Getenv("LLM_MODEL")
	if model == "" {
		model = "deepseek-chat"
	}

	var apiKey string
	switch provider {
	case "deepseek":
		apiKey = os.Getenv("DEEPSEEK_API_KEY")
	case "openai":
		apiKey = os.Getenv("OPENAI_API_KEY")
	case "claude":
		apiKey = os.Getenv("CLAUDE_API_KEY")
	case "qianwen":
		apiKey = os.Getenv("QIANWEN_API_KEY")
	case "ollama_local":
		// 🆕 本地模型不需要API密钥
		apiKey = "local-model" // 设置一个占位符，避免空值检查
	}

	if apiKey == "" {
		log.Printf("⚠️ [统一上下文] LLM API密钥未配置，提供商: %s，跳过统一上下文管理器初始化", provider)
		// 不中断Handler创建，只是不初始化统一上下文管理器
		h.unifiedContextManager = nil
	} else {
		realLLMService, err := services.NewRealLLMService(provider, model, apiKey)
		if err != nil {
			log.Printf("❌ [统一上下文] 创建真实LLM服务失败: %v，跳过统一上下文管理器初始化", err)
			h.unifiedContextManager = nil
		} else {
			// 设置全局LLM服务，供chat_handlers.go使用
			InitChatService(realLLMService)
			log.Printf("✅ [全局LLM服务] 已通过InitChatService设置全局LLM服务实例")

			sessionManager := contextService.SessionStore()

			h.unifiedContextManager = services.NewUnifiedContextManager(
				contextService.GetContextService(), // 获取底层的ContextService
				sessionManager,
				realLLMService,
			)
			contextService.SetContextManager(h.unifiedContextManager)
			InitChatContextService(contextService.GetContextService())
			log.Printf("✅ [统一上下文] 统一上下文管理器初始化完成，使用真实LLM: %s/%s", provider, model)
		}
	}

	// 🔥 新增：设置WebSocket管理器的全局处理器引用
	// 这样WebSocket心跳就能调用会话活跃度更新方法
	services.SetGlobalHandler(h)

	return h
}

// GetContextService 暴露底层 ContextService，便于中间件注入上下文
func (h *Handler) GetContextService() *services.ContextService {
	return h.contextService.GetContextService()
}

// RegisterRoutes 注册MCP协议相关路由
// 注意：/health, /api/sessions, /api/security/* 等路由已在 main_http.go 中注册
func (h *Handler) RegisterRoutes(router *gin.Engine) {
	// 🔥 调试端点 - 查看WebSocket连接详情
	router.GET("/debug/ws/connections", h.HandleDebugWSConnections)

	// MCP SSE端点
	router.GET("/sse", h.HandleSSE)

	// MCP JSON-RPC端点
	router.POST("/rpc", h.HandleMCPRequest)

	// MCP初始化端点
	router.POST("/api/mcp/initialize", h.HandleMCPInitialize)

	// MCP调试端点
	router.GET("/debug/mcp/status", h.HandleMCPStatus)

	// 路由信息端点
	router.GET("/api/routes", h.HandleListRoutes)

	// MCP工具列表接口
	// router.POST("/api/mcp/tools/list", h.HandleMCPToolsList) // 已移至main_http.go的protected组

	// MCP工具调用通用接口
	// router.POST("/api/mcp/tools/call", h.HandleMCPToolCall) // 已移至main_http.go的protected组

	// MCP标准工具路由 - 全部已移至main_http.go的protected组
	// router.POST("/api/mcp/tools/associate_file", h.HandleMCPAssociateFile)
	// router.POST("/api/mcp/tools/record_edit", h.HandleMCPRecordEdit)
	// router.POST("/api/mcp/tools/retrieve_context", h.HandleMCPRetrieveContext)
	// router.POST("/api/mcp/tools/programming_context", h.HandleMCPProgrammingContext)

	// 新增：本地操作回调处理路由
	// router.POST("/api/mcp/tools/local_operation_callback", h.HandleLocalOperationCallback) // 已移至main_http.go的protected组

	// 主要MCP工具API（完全符合MCP规范）
	router.POST("/mcp/tools/create_context", h.HandleStoreContext)
	router.POST("/mcp/tools/read_context", h.HandleRetrieveContext)

	// 原有API路径保持不变，兼容已有客户端
	mcp := router.Group("/api/mcp/context-keeper")
	{
		mcp.POST("/storeContext", h.HandleStoreContext)
		mcp.POST("/retrieveContext", h.HandleRetrieveContext)
		mcp.POST("/summarizeContext", h.HandleSummarizeContext)
		mcp.POST("/searchContext", h.HandleSearchContext)
		mcp.POST("/associateFile", h.HandleAssociateFile)
		mcp.POST("/recordEdit", h.HandleRecordEdit)
		mcp.POST("/storeMessages", h.HandleStoreMessages)
		mcp.POST("/retrieveConversation", h.HandleRetrieveConversation)
		mcp.GET("/sessionState", h.HandleSessionState)
		mcp.POST("/memorizeContext", h.HandleSummarizeToLongTerm)
	}

	// 集合管理路由
	collections := router.Group("/api/collections")
	{
		collections.GET("", h.HandleListCollections)
		collections.POST("", h.HandleCreateCollection)
		collections.GET("/:name", h.HandleGetCollection)
		collections.DELETE("/:name", h.HandleDeleteCollection)
	}

	log.Println("✅ MCP协议路由已注册:")
	log.Println("  GET  /sse - SSE流式接口")
	log.Println("  POST /rpc - MCP JSON-RPC接口")
	log.Println("  POST /api/mcp/* - MCP工具接口")
	log.Println("  GET  /api/collections - 集合管理接口")
}

// 健康检查处理函数
func (h *Handler) HandleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "healthy",
	})
}

// handleStoreContext 处理存储上下文的请求
func (h *Handler) HandleStoreContext(c *gin.Context) {
	var req models.StoreContextRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "无效的请求格式: " + err.Error(),
		})
		return
	}

	// 验证必填字段
	if req.SessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "缺少必填字段: sessionId",
		})
		return
	}

	if req.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "缺少必填字段: content",
		})
		return
	}

	// This route is JWT-protected. Persist the authenticated identity rather
	// than accepting a caller-controlled user ID in the request body.
	if authenticatedUserID, exists := c.Get("user_id"); exists {
		if value, ok := authenticatedUserID.(string); ok && value != "" {
			req.UserID = value
		}
	}
	// 🔥 安全拦截：获取用户ID并进行内容安全检查
	userID := req.UserID
	if userID == "" {
		resolvedUserID, err := h.contextService.GetUserIDFromSessionID(req.SessionID)
		if err != nil {
			log.Printf("[存储上下文] 从会话获取用户ID失败: %v", err)
			userID = "unknown" // 使用默认值继续处理
		} else {
			userID = resolvedUserID
		}
	}

	// The session owner is part of the storage-security boundary. Persist the
	// JWT identity before any vector, timeline, or graph write so a later
	// cascade deletion can verify ownership instead of guessing it.
	if req.UserID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "error": "authenticated user_id is required"})
		return
	}
	sessionStore := h.contextService.SessionStore()
	if sessionStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "session store is unavailable"})
		return
	}
	if err := sessionStore.SetSessionMetadata(req.SessionID, "userId", req.UserID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "failed to persist session owner"})
		return
	}

	// 调用安全服务进行智能决策
	if h.securityService != nil {
		decision, err := h.securityService.ScanContentWithSmartDecision(
			c.Request.Context(),
			req.SessionID,
			userID,
			req.Content,
			[]string{}, // 可根据需要传入合规规则
		)
		if err != nil {
			log.Printf("[存储上下文] 安全扫描失败: %v", err)
		} else {
			// 如果需要阻止
			if decision.ShouldBlock {
				log.Printf("[存储上下文] 内容被阻止: 风险等级=%v, 原因=%s", decision.RiskLevel, decision.Reasoning)
				c.JSON(http.StatusForbidden, gin.H{
					"status":    "error",
					"error":     "内容包含敏感信息，已被安全策略阻止",
					"riskLevel": decision.RiskLevel,
					"reasoning": decision.Reasoning,
				})
				return
			}

			// 如果有脱敏内容，使用脱敏后的内容
			if decision.RedactedContent != "" {
				log.Printf("[存储上下文] 使用脱敏后的内容: 风险等级=%v", decision.RiskLevel)
				req.Content = decision.RedactedContent
			}

			// 记录审计日志
			if decision.ShouldAlert {
				log.Printf("[安全审计] 存储上下文触发告警: sessionID=%s, userID=%s, 风险分数=%.2f, 原因=%s",
					req.SessionID, userID, decision.RiskScore, decision.Reasoning)
			}
		}
	}

	// 处理存储逻辑
	memoryID, err := h.contextService.StoreContext(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "存储上下文失败: " + err.Error(),
		})
		return
	}

	// 返回成功响应
	c.JSON(http.StatusOK, gin.H{
		"status":   "success",
		"memoryId": memoryID,
	})
}

// handleRetrieveContext 处理检索上下文请求
func (h *Handler) HandleRetrieveContext(c *gin.Context) {
	var req models.RetrieveContextRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"type":    "error",
			"message": "无效的请求格式: " + err.Error(),
		})
		return
	}

	// 验证必需字段
	if req.SessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"type":    "error",
			"message": "缺少必需的sessionId字段",
		})
		return
	}

	// 调用服务处理请求
	resp, err := h.contextService.RetrieveContext(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"type":    "error",
			"message": "检索上下文失败: " + err.Error(),
		})
		return
	}

	// 🔥 安全拦截：对输出内容进行脱敏处理
	if h.securityService != nil {
		userID, err := h.contextService.GetUserIDFromSessionID(req.SessionID)
		if err != nil {
			log.Printf("[检索上下文] 从会话获取用户ID失败: %v", err)
			userID = "unknown"
		}

		// 对短期记忆进行脱敏
		if resp.ShortTermMemory != "" {
			decision, err := h.securityService.ScanContentWithSmartDecision(
				c.Request.Context(),
				req.SessionID,
				userID,
				resp.ShortTermMemory,
				[]string{},
			)
			if err == nil && decision.RedactedContent != "" {
				resp.ShortTermMemory = decision.RedactedContent
			}
		}

		// 对长期记忆进行脱敏
		if resp.LongTermMemory != "" {
			decision, err := h.securityService.ScanContentWithSmartDecision(
				c.Request.Context(),
				req.SessionID,
				userID,
				resp.LongTermMemory,
				[]string{},
			)
			if err == nil && decision.RedactedContent != "" {
				resp.LongTermMemory = decision.RedactedContent
			}
		}

		// 对相关知识进行脱敏
		if resp.RelevantKnowledge != "" {
			decision, err := h.securityService.ScanContentWithSmartDecision(
				c.Request.Context(),
				req.SessionID,
				userID,
				resp.RelevantKnowledge,
				[]string{},
			)
			if err == nil && decision.RedactedContent != "" {
				resp.RelevantKnowledge = decision.RedactedContent
			}
		}
	}

	// 响应标准化的数据
	c.JSON(http.StatusOK, gin.H{
		"type": "success",
		"data": gin.H{
			"session_state":      resp.SessionState,
			"short_term_memory":  resp.ShortTermMemory,
			"long_term_memory":   resp.LongTermMemory,
			"relevant_knowledge": resp.RelevantKnowledge,
		},
	})
}

// 处理生成上下文摘要请求
func (h *Handler) HandleSummarizeContext(c *gin.Context) {
	var req models.SummarizeContextRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"type":    "error",
			"message": "无效的请求格式: " + err.Error(),
		})
		return
	}

	// 验证必需字段
	if req.SessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"type":    "error",
			"message": "缺少必需的sessionId字段",
		})
		return
	}

	// 调用服务处理请求
	summary, err := h.contextService.SummarizeContext(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"type":    "error",
			"message": "生成摘要失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"type": "success",
		"data": gin.H{
			"summary": summary,
		},
	})
}

// 处理列出集合请求
func (h *Handler) HandleListCollections(c *gin.Context) {
	collections, err := h.vectorService.ListCollections()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "获取集合列表失败",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"collections": collections,
	})
}

// 处理获取集合详情请求
func (h *Handler) HandleGetCollection(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "缺少集合名称",
		})
		return
	}

	exists, err := h.vectorService.CheckCollectionExists(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "检查集合是否存在失败",
			"details": err.Error(),
		})
		return
	}

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "集合不存在",
			"name":  name,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"name":      name,
		"exists":    true,
		"dimension": h.vectorService.GetDimension(),
		"metric":    h.vectorService.GetMetric(),
	})
}

// 处理创建集合请求
func (h *Handler) HandleCreateCollection(c *gin.Context) {
	var req struct {
		Name      string `json:"name" binding:"required"`
		Dimension int    `json:"dimension"`
		Metric    string `json:"metric"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "无效的请求格式",
			"details": err.Error(),
		})
		return
	}

	// 使用默认维度和度量方式
	if req.Dimension <= 0 {
		req.Dimension = h.vectorService.GetDimension()
	}
	if req.Metric == "" {
		req.Metric = h.vectorService.GetMetric()
	}

	err := h.vectorService.CreateCollection(req.Name, req.Dimension, req.Metric)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "创建集合失败",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"status":    "success",
		"name":      req.Name,
		"dimension": req.Dimension,
		"metric":    req.Metric,
	})
}

// 处理删除集合请求
func (h *Handler) HandleDeleteCollection(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "缺少集合名称",
		})
		return
	}

	err := h.vectorService.DeleteCollection(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "删除集合失败",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"name":   name,
	})
}

// handleStoreMessages 处理存储消息请求
func (h *Handler) HandleStoreMessages(c *gin.Context) {
	var req models.StoreMessagesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "无效的请求格式",
			"details": err.Error(),
		})
		return
	}

	// 验证必需字段
	if req.SessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "缺少必需的sessionId字段",
		})
		return
	}

	if len(req.Messages) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "消息列表为空",
		})
		return
	}

	// 🔥 安全拦截：获取用户ID并对每条消息进行安全检查
	userID, err := h.contextService.GetUserIDFromSessionID(req.SessionID)
	if err != nil {
		log.Printf("[存储消息] 从会话获取用户ID失败: %v", err)
		userID = "unknown" // 使用默认值继续处理
	}

	// 对每条消息进行安全扫描和脱敏
	if h.securityService != nil {
		for i := range req.Messages {
			// 直接访问结构体字段
			content := req.Messages[i].Content

			if content == "" {
				continue
			}

			// 调用安全服务进行智能决策
			decision, err := h.securityService.ScanContentWithSmartDecision(
				c.Request.Context(),
				req.SessionID,
				userID,
				content,
				[]string{}, // 可根据需要传入合规规则
			)
			if err != nil {
				log.Printf("[存储消息] 消息%d安全扫描失败: %v", i, err)
				continue
			}

			// 如果需要阻止
			if decision.ShouldBlock {
				log.Printf("[存储消息] 消息%d被阻止: 风险等级=%v, 原因=%s", i, decision.RiskLevel, decision.Reasoning)
				c.JSON(http.StatusForbidden, gin.H{
					"error":     "消息包含敏感信息，已被安全策略阻止",
					"messageId": i,
					"riskLevel": decision.RiskLevel,
					"reasoning": decision.Reasoning,
				})
				return
			}

			// 如果有脱敏内容，使用脱敏后的内容
			if decision.RedactedContent != "" {
				log.Printf("[存储消息] 消息%d使用脱敏后的内容: 风险等级=%v", i, decision.RiskLevel)
				req.Messages[i].Content = decision.RedactedContent
			}

			// 记录审计日志
			if decision.ShouldAlert {
				log.Printf("[安全审计] 存储消息触发告警: sessionID=%s, userID=%s, messageId=%d, 风险分数=%.2f, 原因=%s",
					req.SessionID, userID, i, decision.RiskScore, decision.Reasoning)
			}
		}
	}

	// 调用服务处理请求
	response, err := h.contextService.StoreSessionMessages(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "存储消息失败",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, response)
}

// handleRetrieveConversation 处理检索对话请求
func (h *Handler) HandleRetrieveConversation(c *gin.Context) {
	var req models.RetrieveConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "无效的请求格式: " + err.Error(),
		})
		return
	}

	// 验证必需字段 - 支持使用SessionID或MessageID或BatchID
	if req.SessionID == "" && req.MessageID == "" && req.BatchID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "至少需要提供sessionId、messageId或batchId其中一个",
		})
		return
	}

	// 设置默认值
	if req.Limit <= 0 {
		req.Limit = 10 // 默认返回10条记录
	}

	// 调用服务检索对话
	resp, err := h.contextService.RetrieveConversation(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "检索对话失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// 处理会话状态请求
func (h *Handler) HandleSessionState(c *gin.Context) {
	sessionID := c.Query("sessionId")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "缺少sessionId参数",
		})
		return
	}

	// 获取会话状态
	response, err := h.contextService.GetSessionState(c.Request.Context(), sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "获取会话状态失败",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, response)
}

// handleMCPStatus 处理MCP服务状态查询
func (h *Handler) HandleMCPStatus(c *gin.Context) {
	// 读取连接统计数据
	connMutex.RLock()
	active := activeConnections
	total := totalConnections
	connMutex.RUnlock()

	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"service": gin.H{
			"name":         "Context-Keeper",
			"version":      "1.0.0",
			"protocol":     "mcp",
			"mcp_version":  "v1",
			"description":  "代码上下文管理服务",
			"connections":  active,
			"total_conns":  total,
			"sse_endpoint": "/sse",
			"uptime":       time.Since(h.startTime).String(),
		},
		"capabilities": gin.H{
			"tools": []string{
				"associate_file",
				"record_edit",
				"retrieve_context",
				"programming_context",
			},
			"resources": h.generateResourcesDefinition(),
		},
		"config": gin.H{
			"heartbeat_interval": "10s",
			"manifest_interval":  "30s",
		},
	})
}

// handleListRoutes 列出所有可用的API路由
func (h *Handler) HandleListRoutes(c *gin.Context) {
	routes := []map[string]interface{}{
		{
			"path":        "/health",
			"method":      "GET",
			"description": "健康检查端点",
		},
		{
			"path":        "/sse",
			"method":      "GET",
			"description": "MCP Server-Sent Events连接端点",
		},
		{
			"path":        "/api/mcp/context-keeper/storeContext",
			"method":      "POST",
			"description": "存储代码上下文",
		},
		{
			"path":        "/api/mcp/context-keeper/retrieveContext",
			"method":      "POST",
			"description": "检索代码上下文",
		},
		{
			"path":        "/api/mcp/context-keeper/associateFile",
			"method":      "POST",
			"description": "关联代码文件",
		},
		{
			"path":        "/api/mcp/context-keeper/recordEdit",
			"method":      "POST",
			"description": "记录编辑操作",
		},
		{
			"path":        "/api/mcp/context-keeper/sessionState",
			"method":      "GET",
			"description": "获取会话状态",
		},
		{
			"path":        "/debug/mcp/status",
			"method":      "GET",
			"description": "获取MCP连接状态",
		},
		{
			"path":        "/api/routes",
			"method":      "GET",
			"description": "列出所有API路由",
		},
	}

	c.JSON(http.StatusOK, routes)
}

// handleSearchContext 处理上下文搜索工具请求
func (h *Handler) HandleSearchContext(c *gin.Context) {
	var req models.SearchContextRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"type":    "error",
			"message": "无效的请求格式: " + err.Error(),
		})
		return
	}

	// 验证必需字段
	if req.SessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"type":    "error",
			"message": "缺少必需的sessionId字段",
		})
		return
	}

	if req.Query == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"type":    "error",
			"message": "缺少必需的query字段",
		})
		return
	}

	// 调用服务层搜索相关内容
	results, err := h.contextService.SearchContext(c.Request.Context(), req.SessionID, req.Query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"type":    "error",
			"message": "搜索失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"type": "success",
		"data": gin.H{
			"results": results,
		},
	})
}

// handleAssociateFile 处理文件关联请求
func (h *Handler) HandleAssociateFile(c *gin.Context) {
	var req models.AssociateFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"type":    "error",
			"message": "无效的请求格式: " + err.Error(),
		})
		return
	}

	// 验证必需字段
	if req.SessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"type":    "error",
			"message": "缺少必需的sessionId字段",
		})
		return
	}

	if req.FilePath == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"type":    "error",
			"message": "缺少必需的filePath字段",
		})
		return
	}

	// 调用服务处理请求
	err := h.contextService.AssociateFile(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"type":    "error",
			"message": "关联文件失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"type":    "success",
		"message": "文件关联成功",
	})
}

// handleRecordEdit 处理编辑记录请求
func (h *Handler) HandleRecordEdit(c *gin.Context) {
	var req models.RecordEditRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"type":    "error",
			"message": "无效的请求格式: " + err.Error(),
		})
		return
	}

	// 验证必需字段
	if req.SessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"type":    "error",
			"message": "缺少必需的sessionId字段",
		})
		return
	}

	if req.FilePath == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"type":    "error",
			"message": "缺少必需的filePath字段",
		})
		return
	}

	if req.Diff == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"type":    "error",
			"message": "缺少必需的diff字段",
		})
		return
	}

	// 调用服务处理请求
	err := h.contextService.RecordEdit(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"type":    "error",
			"message": "记录编辑操作失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"type":    "success",
		"message": "编辑操作记录成功",
	})
}

// generateToolsDefinition 生成工具定义
func (h *Handler) generateToolsDefinition() []string {
	return []string{
		"store_context",
		"retrieve_context",
		"summarize_context",
		"search_context",
		"associate_file",
		"record_edit",
	}
}

// generateResourcesDefinition 生成资源定义
func (h *Handler) generateResourcesDefinition() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"id":          "context://keeper",
			"name":        "Context Keeper",
			"description": "代码上下文管理接口",
			"routes": []map[string]interface{}{
				{
					"id":          "context-keeper",
					"path":        "/",
					"description": "Context-Keeper服务根路径",
				},
			},
		},
	}
}

// handleMCPAssociateFile 处理MCP工具调用 - 关联文件
func (h *Handler) HandleMCPAssociateFile(c *gin.Context) {
	// 解析MCP工具调用请求
	var req struct {
		SessionId string `json:"sessionId" binding:"required"`
		FilePath  string `json:"filePath" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的请求格式: " + err.Error(),
		})
		return
	}

	// 转换为内部请求格式
	internalReq := models.AssociateFileRequest{
		SessionID: req.SessionId,
		FilePath:  req.FilePath,
	}

	// 调用服务处理
	err := h.contextService.AssociateFile(c.Request.Context(), internalReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "关联文件失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "文件关联成功",
	})
}

// handleMCPRecordEdit 处理MCP工具调用 - 记录编辑
func (h *Handler) HandleMCPRecordEdit(c *gin.Context) {
	// 解析MCP工具调用请求
	var req struct {
		SessionId string `json:"sessionId" binding:"required"`
		FilePath  string `json:"filePath" binding:"required"`
		Diff      string `json:"diff" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的请求格式: " + err.Error(),
		})
		return
	}

	// 转换为内部请求格式
	internalReq := models.RecordEditRequest{
		SessionID: req.SessionId,
		FilePath:  req.FilePath,
		Diff:      req.Diff,
	}

	// 调用服务处理
	err := h.contextService.RecordEdit(c.Request.Context(), internalReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "记录编辑操作失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "编辑操作记录成功",
	})
}

// handleMCPRetrieveContext 处理MCP工具调用 - 检索上下文
func (h *Handler) HandleMCPRetrieveContext(c *gin.Context) {
	log.Printf("🔍 [检索API] 收到retrieve_context请求")

	// 🔒 获取JWT中的userID
	userID, exists := c.Get("user_id")
	log.Printf("🔍 [检索API] JWT中user_id存在: %v, 值: %v", exists, userID)

	if !exists {
		log.Printf("❌ [检索API] 未授权：缺少用户身份信息")
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "未授权：缺少用户身份信息",
		})
		return
	}

	// 解析MCP工具调用请求
	var req struct {
		SessionId               string `json:"sessionId" binding:"required"`
		Query                   string `json:"query" binding:"required"`
		MaxResults              int    `json:"maxResults,omitempty"`              // 最大返回结果数
		ProjectAnalysis         string `json:"projectAnalysis,omitempty"`         // 🆕 可选的工程分析参数
		ContextsOnly            bool   `json:"contextsOnly,omitempty"`            // 🆕 仅返回检索结果，不生成LLM回答
		EvaluationRetrievalOnly bool   `json:"evaluationRetrievalOnly,omitempty"` // 评测用三路RRF检索，不生成LLM回答
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("❌ [检索API] 请求格式错误: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的请求格式: " + err.Error(),
		})
		return
	}

	log.Printf("🔍 [检索API] 解析请求成功: sessionId=%s, query=%s", req.SessionId, req.Query)

	// 🔒 确保会话metadata中包含userId
	sessionStore := h.contextService.SessionStore()
	log.Printf("🔍 [检索API] sessionStore为nil: %v", sessionStore == nil)
	if sessionStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "会话隔离服务不可用"})
		return
	}
	if err := ensureProtectedSessionOwner(sessionStore, req.SessionId, userID.(string)); err != nil {
		log.Printf("[Retrieve] denied session ownership change: user_id=%s session_id=%s err=%v", userID.(string), req.SessionId, err)
		c.JSON(http.StatusForbidden, gin.H{"error": "无权访问该受保护会话"})
		return
	}

	if sessionStore != nil {
		log.Printf("🔍 [检索API] 准备设置会话userId: sessionId=%s, userId=%s", req.SessionId, userID.(string))
		if err := sessionStore.SetSessionMetadata(req.SessionId, "userId", userID.(string)); err != nil {
			log.Printf("⚠️ [检索API] 设置会话userId失败: %v", err)
		} else {
			log.Printf("🔒 [检索API] 已设置会话userId: %s", userID.(string))
		}
	} else {
		log.Printf("⚠️ [检索API] sessionStore为nil，无法设置userId")
	}

	// 转换为内部请求格式
	internalReq := models.RetrieveContextRequest{
		SessionID:               req.SessionId,
		Query:                   req.Query,
		Limit:                   req.MaxResults,      // 映射maxResults到Limit
		UserID:                  userID.(string),     // 🔒 添加UserID进行数据隔离
		ProjectAnalysis:         req.ProjectAnalysis, // 🆕 传递工程分析结果
		ContextsOnly:            req.ContextsOnly,    // 🆕 传递contextsOnly标志
		EvaluationRetrievalOnly: req.EvaluationRetrievalOnly,
	}

	// 调用服务处理
	resp, err := h.contextService.RetrieveContext(c.Request.Context(), internalReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "检索上下文失败: " + err.Error(),
		})
		return
	}

	// 🆕 如果是contexts_only模式，只返回contexts字段
	if req.ContextsOnly || req.EvaluationRetrievalOnly {
		c.JSON(http.StatusOK, gin.H{
			"contexts":           resp.Contexts,
			"retrieval_metadata": resp.RetrievalMetadata,
		})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// handleMCPProgrammingContext 处理MCP工具调用 - 获取编程上下文
func (h *Handler) HandleMCPProgrammingContext(c *gin.Context) {
	// 解析MCP工具调用请求
	var req struct {
		SessionId string `json:"sessionId" binding:"required"`
		Query     string `json:"query,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的请求格式: " + err.Error(),
		})
		return
	}

	// 使用GetProgrammingContext获取编程上下文
	progContext, err := h.contextService.GetProgrammingContext(c.Request.Context(), req.SessionId, req.Query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取编程上下文失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, progContext)
}

// 🔥 新增：公开的会话活跃度更新方法，供WebSocket管理器调用
func (h *Handler) UpdateSessionActivity(sessionID string) {
	h.updateSessionActivity(sessionID)
}

// updateSessionActivity 更新会话活跃度（私有方法）
func (h *Handler) updateSessionActivity(sessionID string) {
	if sessionID == "" {
		return
	}

	// 🔥 修复：从会话ID获取用户ID，实现真正的多用户隔离
	userID, err := h.contextService.GetUserIDFromSessionID(sessionID)
	if err != nil {
		log.Printf("[会话活跃度更新] 从会话获取用户ID失败，跳过更新: %v", err)
		return
	}

	// 获取用户会话存储
	userSessionStore, err := h.contextService.GetUserSessionStore(userID)
	if err != nil {
		log.Printf("[会话活跃度更新] 获取用户会话存储失败: %v", err)
		return
	}

	// 🔥 修复：直接获取已存在的会话，不创建新会话
	session, err := userSessionStore.GetSession(sessionID)
	if err != nil {
		log.Printf("[会话活跃度更新] 获取会话失败: %v", err)
		return
	}

	// 更新最后活跃时间
	session.LastActive = time.Now()
	if err := userSessionStore.SaveSession(session); err != nil {
		log.Printf("[会话活跃度更新] 保存会话失败: %v", err)
	} else {
		log.Printf("[会话活跃度更新] ✅ 已更新会话 %s 的活跃时间", sessionID)
	}
}

func isSessionScopedMCPTool(name string) bool {
	switch name {
	case "associate_file", "record_edit", "retrieve_context", "programming_context":
		return true
	default:
		return false
	}
}

func authenticatedUserID(c *gin.Context) string {
	value, ok := c.Get("user_id")
	if !ok {
		return ""
	}
	userID, _ := value.(string)
	return strings.TrimSpace(userID)
}

func (h *Handler) authorizeMCPToolSession(c *gin.Context, sessionID string) error {
	userID := authenticatedUserID(c)
	if userID == "" {
		return fmt.Errorf("missing authenticated user")
	}
	if h.contextService == nil || h.contextService.SessionStore() == nil {
		return fmt.Errorf("session ownership service unavailable")
	}
	return ensureProtectedSessionOwner(h.contextService.SessionStore(), sessionID, userID)
}

// handleMCPToolCall 处理MCP工具调用通用接口
func (h *Handler) HandleMCPToolCall(c *gin.Context) {
	var request struct {
		JSONRPC string `json:"jsonrpc"`
		ID      string `json:"id"`
		Method  string `json:"method"`
		Params  struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
		} `json:"params"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"error": gin.H{
				"code":    -32700,
				"message": "无效的请求格式: " + err.Error(),
			},
		})
		return
	}

	// 记录工具调用请求
	log.Printf("[MCP工具调用] 工具: %s, 参数: %+v", request.Params.Name, request.Params.Arguments)

	// All session-scoped tools share the same ownership boundary as their
	// versioned HTTP endpoints. This protects the generic JSON-RPC route from
	// cross-user access when a caller supplies another user's session ID.
	if isSessionScopedMCPTool(request.Params.Name) {
		sessionID, ok := request.Params.Arguments["sessionId"].(string)
		if !ok || strings.TrimSpace(sessionID) == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"error": gin.H{
					"code":    -32602,
					"message": "缺少必要参数或参数类型错误",
				},
			})
			return
		}
		if err := h.authorizeMCPToolSession(c, sessionID); err != nil {
			c.JSON(http.StatusForbidden, gin.H{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"error": gin.H{
					"code":    -32003,
					"message": "无权访问该会话",
				},
			})
			return
		}
	}

	// 🔥 自动更新会话活跃时间（在工具执行前）
	if sessionId, ok := request.Params.Arguments["sessionId"].(string); ok && sessionId != "" {
		h.updateSessionActivity(sessionId)
	}

	// 根据工具名称分发请求
	switch request.Params.Name {
	case "associate_file":
		// 提取参数
		sessionId, ok1 := request.Params.Arguments["sessionId"].(string)
		filePath, ok2 := request.Params.Arguments["filePath"].(string)

		if !ok1 || !ok2 {
			c.JSON(http.StatusBadRequest, gin.H{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"error": gin.H{
					"code":    -32602,
					"message": "缺少必要参数或参数类型错误",
				},
			})
			return
		}

		// 转换为内部请求格式
		internalReq := models.AssociateFileRequest{
			SessionID: sessionId,
			FilePath:  filePath,
		}

		// 调用服务处理
		err := h.contextService.AssociateFile(c.Request.Context(), internalReq)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"result": gin.H{
					"isError": true,
					"content": []gin.H{
						{
							"type": "text",
							"text": "关联文件失败: " + err.Error(),
						},
					},
				},
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"result": gin.H{
				"content": []gin.H{
					{
						"type": "text",
						"text": "文件关联成功",
					},
				},
			},
		})

	case "record_edit":
		// 提取参数
		sessionId, ok1 := request.Params.Arguments["sessionId"].(string)
		filePath, ok2 := request.Params.Arguments["filePath"].(string)
		diff, ok3 := request.Params.Arguments["diff"].(string)

		if !ok1 || !ok2 || !ok3 {
			c.JSON(http.StatusBadRequest, gin.H{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"error": gin.H{
					"code":    -32602,
					"message": "缺少必要参数或参数类型错误",
				},
			})
			return
		}

		// 转换为内部请求格式
		internalReq := models.RecordEditRequest{
			SessionID: sessionId,
			FilePath:  filePath,
			Diff:      diff,
		}

		// 调用服务处理
		err := h.contextService.RecordEdit(c.Request.Context(), internalReq)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"result": gin.H{
					"isError": true,
					"content": []gin.H{
						{
							"type": "text",
							"text": "记录编辑操作失败: " + err.Error(),
						},
					},
				},
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"result": gin.H{
				"content": []gin.H{
					{
						"type": "text",
						"text": "编辑操作记录成功",
					},
				},
			},
		})

	case "retrieve_context":
		// 提取参数
		sessionId, ok1 := request.Params.Arguments["sessionId"].(string)
		query, ok2 := request.Params.Arguments["query"].(string)

		if !ok1 || !ok2 {
			c.JSON(http.StatusBadRequest, gin.H{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"error": gin.H{
					"code":    -32602,
					"message": "缺少必要参数或参数类型错误",
				},
			})
			return
		}

		// 🆕 提取可选的工程分析参数
		projectAnalysis, _ := request.Params.Arguments["projectAnalysis"].(string)

		// 转换为内部请求格式
		internalReq := models.RetrieveContextRequest{
			SessionID:       sessionId,
			Query:           query,
			UserID:          authenticatedUserID(c),
			ProjectAnalysis: projectAnalysis, // 🆕 传递工程分析结果
		}

		// 调用服务处理
		resp, err := h.contextService.RetrieveContext(c.Request.Context(), internalReq)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"result": gin.H{
					"isError": true,
					"content": []gin.H{
						{
							"type": "text",
							"text": "检索上下文失败: " + err.Error(),
						},
					},
				},
			})
			return
		}

		// 组织响应内容
		contextText := ""
		if resp.ShortTermMemory != "" {
			contextText += "短期记忆:\n" + resp.ShortTermMemory + "\n\n"
		}
		if resp.LongTermMemory != "" {
			contextText += "长期记忆:\n" + resp.LongTermMemory + "\n\n"
		}
		if resp.RelevantKnowledge != "" {
			contextText += "相关知识:\n" + resp.RelevantKnowledge
		}

		c.JSON(http.StatusOK, gin.H{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"result": gin.H{
				"content": []gin.H{
					{
						"type": "text",
						"text": contextText,
					},
				},
			},
		})

	case "programming_context":
		// 提取参数
		sessionId, ok := request.Params.Arguments["sessionId"].(string)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"error": gin.H{
					"code":    -32602,
					"message": "缺少必要参数或参数类型错误",
				},
			})
			return
		}

		// 可选的查询参数
		query := ""
		if q, ok := request.Params.Arguments["query"].(string); ok {
			query = q
		}

		// 转换为内部请求格式
		internalReq := models.RetrieveContextRequest{
			SessionID: sessionId,
			Query:     query,
			Strategy:  "programming_context", // 使用编程上下文检索策略
		}

		// 调用服务处理
		resp, err := h.contextService.RetrieveContext(c.Request.Context(), internalReq)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"result": gin.H{
					"isError": true,
					"content": []gin.H{
						{
							"type": "text",
							"text": "获取编程上下文失败: " + err.Error(),
						},
					},
				},
			})
			return
		}

		// 组织响应内容
		c.JSON(http.StatusOK, gin.H{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"result": gin.H{
				"content": []gin.H{
					{
						"type": "text",
						"text": resp.RelevantKnowledge,
					},
				},
			},
		})

	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"error": gin.H{
				"code":    -32601,
				"message": "未知的工具: " + request.Params.Name,
			},
		})
	}
}

// handleMCPToolsList 处理MCP工具列表请求
func (h *Handler) HandleMCPToolsList(c *gin.Context) {
	var request struct {
		JSONRPC string `json:"jsonrpc"`
		ID      string `json:"id"`
		Method  string `json:"method"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"error": gin.H{
				"code":    -32700,
				"message": "无效的请求格式: " + err.Error(),
			},
		})
		return
	}

	// 记录工具列表请求
	log.Printf("[MCP] 收到工具列表请求: ID=%s", request.ID)

	// 返回工具列表
	c.JSON(http.StatusOK, gin.H{
		"jsonrpc": "2.0",
		"id":      request.ID,
		"result": gin.H{
			"tools": []gin.H{
				{
					"name":        "associate_file",
					"description": "关联代码文件到当前编程会话",
					"schema": gin.H{
						"type": "object",
						"properties": gin.H{
							"sessionId": gin.H{
								"type":        "string",
								"description": "当前会话ID",
							},
							"filePath": gin.H{
								"type":        "string",
								"description": "文件路径",
							},
						},
						"required": []string{"sessionId", "filePath"},
					},
				},
				{
					"name":        "record_edit",
					"description": "记录代码编辑操作",
					"schema": gin.H{
						"type": "object",
						"properties": gin.H{
							"sessionId": gin.H{
								"type":        "string",
								"description": "当前会话ID",
							},
							"filePath": gin.H{
								"type":        "string",
								"description": "文件路径",
							},
							"diff": gin.H{
								"type":        "string",
								"description": "编辑差异内容",
							},
						},
						"required": []string{"sessionId", "filePath", "diff"},
					},
				},
				{
					"name":        "retrieve_context",
					"description": "基于查询检索相关编程上下文",
					"schema": gin.H{
						"type": "object",
						"properties": gin.H{
							"sessionId": gin.H{
								"type":        "string",
								"description": "当前会话ID",
							},
							"query": gin.H{
								"type":        "string",
								"description": "查询内容",
							},
							"projectAnalysis": gin.H{
								"type":        "string",
								"description": "工程分析结果（可选，用于检索增强）",
							},
						},
						"required": []string{"sessionId", "query"},
					},
				},
				{
					"name":        "programming_context",
					"description": "获取编程特征和上下文摘要",
					"schema": gin.H{
						"type": "object",
						"properties": gin.H{
							"sessionId": gin.H{
								"type":        "string",
								"description": "当前会话ID",
							},
							"query": gin.H{
								"type":        "string",
								"description": "可选查询参数",
							},
						},
						"required": []string{"sessionId"},
					},
				},
			},
		},
	})
}

// handleMCPInitialize 处理MCP初始化请求
func (h *Handler) HandleMCPInitialize(c *gin.Context) {
	var request struct {
		JSONRPC string `json:"jsonrpc"`
		ID      string `json:"id"`
		Method  string `json:"method"`
		Params  struct {
			ProtocolVersion string                 `json:"protocolVersion"`
			Capabilities    map[string]interface{} `json:"capabilities"`
			ClientInfo      map[string]interface{} `json:"clientInfo"`
		} `json:"params"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"error": gin.H{
				"code":    -32700,
				"message": "无效的请求格式: " + err.Error(),
			},
		})
		return
	}

	// 记录初始化请求
	log.Printf("[MCP] 收到初始化请求: ID=%s, 协议版本=%s, 客户端=%v",
		request.ID, request.Params.ProtocolVersion, request.Params.ClientInfo)

	// 返回初始化响应
	c.JSON(http.StatusOK, gin.H{
		"jsonrpc": "2.0",
		"id":      request.ID,
		"result": gin.H{
			"protocolVersion": "mcp/v1",
			"capabilities": gin.H{
				"tools": gin.H{
					"listChanged": true,
				},
			},
			"serverInfo": gin.H{
				"name":    "context-keeper",
				"version": "1.0.0",
			},
		},
	})

	// 触发工具列表变更通知(如果客户端支持)
}

// handleMCPRequest 处理MCP JSON-RPC请求
func (h *Handler) HandleMCPRequest(c *gin.Context) {
	var request map[string]interface{}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"jsonrpc": "2.0",
			"error": gin.H{
				"code":    -32700,
				"message": "Parse error: " + err.Error(),
			},
		})
		return
	}

	// 获取请求信息
	method, _ := request["method"].(string)
	id, _ := request["id"].(string)

	log.Printf("[RPC] 收到请求: method=%s, id=%s", method, id)

	// 广播请求到所有活跃的SSE连接
	BroadcastRequest(request)

	// 如果是initialize请求，等待一小段时间然后返回成功响应
	// 这是为了确保SSE连接有足够时间处理请求并发送响应
	if method == "initialize" {
		time.Sleep(100 * time.Millisecond)
		response := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      id,
			"result": map[string]interface{}{
				"capabilities": map[string]interface{}{
					"tools": map[string]interface{}{
						"listChanged": true,
					},
				},
				"protocolVersion": "mcp/v1",
				"serverInfo": map[string]interface{}{
					"name":    "context-keeper",
					"version": "1.0.0",
				},
			},
		}
		c.JSON(http.StatusOK, response)
		return
	}

	// 创建响应
	response := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
	}

	// 判断是否是已知请求类型
	switch method {
	case "tools/list":
		response["result"] = map[string]interface{}{
			"tools": []map[string]interface{}{
				{
					"name":        "associate_file",
					"description": "关联代码文件到当前编程会话",
				},
				{
					"name":        "record_edit",
					"description": "记录代码编辑操作",
				},
				{
					"name":        "retrieve_context",
					"description": "基于查询检索相关编程上下文",
				},
				{
					"name":        "programming_context",
					"description": "获取编程特征和上下文摘要",
				},
			},
		}
	case "tools/call":
		// 处理工具调用
		params, ok := request["params"].(map[string]interface{})
		if !ok {
			response["error"] = map[string]interface{}{
				"code":    -32602,
				"message": "Invalid params",
			}
		} else {
			toolName, _ := params["name"].(string)
			toolParams, _ := params["params"].(map[string]interface{})

			log.Printf("[RPC] 工具调用: %s, 参数: %+v", toolName, toolParams)

			// 🔥 使用支持上下文的分发器，传递请求上下文
			result, err := h.dispatchToolCallWithContext(c.Request.Context(), toolName, toolParams)
			if err != nil {
				response["error"] = map[string]interface{}{
					"code":    -32000,
					"message": err.Error(),
				}
			} else {
				response["result"] = result
			}
		}
	default:
		// 未知请求类型
		response["error"] = map[string]interface{}{
			"code":    -32601,
			"message": "Method not found: " + method,
		}
	}

	c.JSON(http.StatusOK, response)
}

// dispatchToolCall 分派工具调用到相应的处理函数（原版本，向后兼容）
func (h *Handler) dispatchToolCall(toolName string, params map[string]interface{}) (interface{}, error) {
	return h.dispatchToolCallWithContext(context.Background(), toolName, params)
}

// dispatchToolCallWithContext 分派工具调用到相应的处理函数（支持上下文传递）
func (h *Handler) dispatchToolCallWithContext(ctx context.Context, toolName string, params map[string]interface{}) (interface{}, error) {
	switch toolName {
	case "associate_file":
		return h.HandleToolAssociateFile(ctx, params)
	case "record_edit":
		return h.HandleToolRecordEdit(ctx, params)
	case "retrieve_context":
		return h.HandleToolRetrieveContext(ctx, params)
	case "programming_context":
		return h.HandleToolProgrammingContext(ctx, params)
	case "memorize_context":
		return h.HandleToolMemorizeContext(ctx, params)
	case "session_management":
		return h.HandleToolSessionManagement(ctx, params)
	case "store_conversation":
		return h.HandleToolStoreConversation(ctx, params)
	case "retrieve_memory":
		return h.HandleToolRetrieveMemory(ctx, params)
	case "retrieve_todos":
		return h.HandleToolRetrieveTodos(ctx, params)
	case "user_init_dialog":
		return h.HandleToolUserInitDialog(ctx, params)
	case "local_operation_callback":
		return h.HandleToolLocalOperationCallback(ctx, params)
	default:
		return nil, fmt.Errorf("未知的工具: %s", toolName)
	}
}

// handleToolAssociateFile 处理关联文件请求
func (h *Handler) HandleToolAssociateFile(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	sessionID, _ := params["sessionId"].(string)
	filePath, _ := params["filePath"].(string)

	if sessionID == "" || filePath == "" {
		return nil, fmt.Errorf("缺少必需参数sessionId或filePath")
	}

	// 🔥 修复：从会话ID获取用户ID，实现真正的多用户隔离
	userID, err := h.contextService.GetUserIDFromSessionID(sessionID)
	if err != nil {
		log.Printf("[关联文件] 从会话获取用户ID失败: %v", err)
		return map[string]interface{}{
			"status":  "error",
			"message": fmt.Sprintf("从会话获取用户ID失败: %v", err),
		}, nil
	}

	log.Printf("关联文件: 会话=%s, 用户ID=%s, 文件=%s", sessionID, userID, filePath)

	// 使用实际的文件关联逻辑（与STDIO版本保持一致）
	err = h.contextService.AssociateFile(context.Background(), models.AssociateFileRequest{
		SessionID: sessionID,
		FilePath:  filePath,
	})
	if err != nil {
		return nil, fmt.Errorf("关联文件失败: %v", err)
	}

	successMsg := fmt.Sprintf("成功关联文件: %s", filePath)
	log.Print(successMsg)

	// 构建基本响应
	result := map[string]interface{}{
		"status":  "success",
		"message": successMsg,
	}

	// 🔥 修复：获取会话的代码上下文用于本地存储，使用统一的会话获取逻辑
	userSessionStore, err := h.contextService.GetUserSessionStore(userID)
	if err != nil {
		log.Printf("[关联文件] 获取用户会话存储失败，跳过本地指令生成: %v", err)
		return result, nil
	}

	session, err := userSessionStore.GetSession(sessionID)
	if err != nil {
		log.Printf("[关联文件] 获取会话失败，跳过本地指令生成: %v", err)
		return result, nil
	}

	if session != nil && session.CodeContext != nil {
		context := map[string]interface{}{
			"codeContext":    session.CodeContext,
			"hasCodeContext": len(session.CodeContext) > 0,
		}

		return h.enhanceResponseWithLocalInstruction(result, sessionID, userID, models.LocalInstructionCodeContext, context), nil
	}

	return result, nil
}

// handleToolRecordEdit 处理记录编辑请求
func (h *Handler) HandleToolRecordEdit(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	sessionID, _ := params["sessionId"].(string)
	filePath, _ := params["filePath"].(string)
	diff, _ := params["diff"].(string)

	if sessionID == "" || filePath == "" || diff == "" {
		return nil, fmt.Errorf("缺少必需参数")
	}

	// 🔥 修复：从会话ID获取用户ID，实现真正的多用户隔离
	userID, err := h.contextService.GetUserIDFromSessionID(sessionID)
	if err != nil {
		log.Printf("[记录编辑] 从会话获取用户ID失败: %v", err)
		return map[string]interface{}{
			"status":  "error",
			"message": fmt.Sprintf("从会话获取用户ID失败: %v", err),
		}, nil
	}

	log.Printf("记录编辑: 会话=%s, 用户ID=%s, 文件=%s, 差异长度=%d", sessionID, userID, filePath, len(diff))

	// 使用实际的编辑记录逻辑（与STDIO版本保持一致）
	err = h.contextService.RecordEdit(context.Background(), models.RecordEditRequest{
		SessionID: sessionID,
		FilePath:  filePath,
		Diff:      diff,
	})
	if err != nil {
		return nil, fmt.Errorf("记录编辑失败: %v", err)
	}

	successMsg := "成功记录编辑操作"
	log.Print(successMsg)

	// 构建基本响应
	result := map[string]interface{}{
		"status":  "success",
		"message": successMsg,
	}

	// 🔥 修复：获取会话的代码上下文用于本地存储，使用统一的会话获取逻辑
	userSessionStore, err := h.contextService.GetUserSessionStore(userID)
	if err != nil {
		log.Printf("[记录编辑] 获取用户会话存储失败，跳过本地指令生成: %v", err)
		return result, nil
	}

	session, err := userSessionStore.GetSession(sessionID)
	if err != nil {
		log.Printf("[记录编辑] 获取会话失败，跳过本地指令生成: %v", err)
		return result, nil
	}

	if session != nil && session.CodeContext != nil {
		context := map[string]interface{}{
			"codeContext":    session.CodeContext,
			"hasCodeContext": len(session.CodeContext) > 0,
		}

		return h.enhanceResponseWithLocalInstruction(result, sessionID, userID, models.LocalInstructionCodeContext, context), nil
	}

	return result, nil
}

// handleToolRetrieveContext 处理检索上下文请求
func (h *Handler) HandleToolRetrieveContext(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	sessionID, _ := params["sessionId"].(string)
	query, _ := params["query"].(string)
	// 🔥 新增：获取项目分析参数
	projectAnalysis, _ := params["projectAnalysis"].(string)

	if sessionID == "" || query == "" {
		return nil, fmt.Errorf("缺少必需参数")
	}

	// 🔥 修复：从会话ID获取用户ID，实现真正的多用户隔离
	userID, err := h.contextService.GetUserIDFromSessionID(sessionID)
	if err != nil {
		log.Printf("[检索上下文] 从会话获取用户ID失败: %v", err)
		return map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("从会话获取用户ID失败: %v", err),
		}, nil
	}

	log.Printf("检索上下文: 会话=%s, 用户ID=%s, 查询=%s", sessionID, userID, query)

	// 🔥 关键优化：如果有projectAnalysis且UnifiedContextManager中没有ProjectContext，前置更新
	if projectAnalysis != "" && h.unifiedContextManager != nil {
		existingContext, err := h.unifiedContextManager.GetContext(sessionID)

		// 如果不存在或ProjectContext为空，立即创建并更新
		if err != nil || existingContext == nil || existingContext.Project == nil {
			log.Printf("🔧 [前置工程感知] 检测到projectAnalysis参数，立即更新ProjectContext")

			// 🔥 从 context 获取工作空间信息（统一拦截器已注入）
			workspaceID, ok := ctx.Value("workspacePath").(string)
			if !ok || workspaceID == "" {
				log.Printf("❌ [前置工程感知] 从 context 获取workspacePath失败，统一拦截器可能未生效")
				return map[string]interface{}{
					"success": false,
					"message": "系统错误：工作空间信息缺失，请检查请求参数",
				}, nil
			}

			log.Printf("✅ [前置工程感知] 从 context 获取workspacePath: %s", workspaceID)

			// 构建ProjectContext
			projectContext := h.buildProjectContextFromAnalysis(projectAnalysis, workspaceID)

			// 只有解析成功才存储
			if projectContext != nil {
				// 创建或更新UnifiedContext
				var unifiedContext *models.UnifiedContextModel
				if existingContext != nil {
					unifiedContext = existingContext
					unifiedContext.Project = projectContext
					unifiedContext.UpdatedAt = time.Now()
				} else {
					unifiedContext = &models.UnifiedContextModel{
						SessionID:   sessionID,
						UserID:      userID,
						WorkspaceID: workspaceID,
						Project:     projectContext,
						CreatedAt:   time.Now(),
						UpdatedAt:   time.Now(),
					}
				}

				// 立即存储到UnifiedContextManager
				h.unifiedContextManager.UpdateMemory(sessionID, unifiedContext)
				log.Printf("✅ [前置工程感知] ProjectContext已存储到统一上下文管理器，项目: %s", projectContext.ProjectName)
			} else {
				log.Printf("⚠️ [前置工程感知] ProjectContext构建失败，跳过存储，继续执行检索流程")
			}
		} else {
			log.Printf("✅ [前置工程感知] ProjectContext已存在，跳过更新")
		}
	}

	// 创建检索请求
	retrieveReq := models.RetrieveContextRequest{
		SessionID:       sessionID,
		Query:           query,
		ProjectAnalysis: projectAnalysis, // 🆕 传递工程分析结果
		Limit:           2000,            // 默认限制
	}

	// 🔥 直接使用传入的上下文（统一拦截器已注入会话信息）
	result, err := h.contextService.RetrieveContext(ctx, retrieveReq)
	if err != nil {
		return nil, fmt.Errorf("检索上下文失败: %v", err)
	}

	// 构建响应
	response := map[string]interface{}{
		"sessionState":      result.SessionState,
		"shortTermMemory":   result.ShortTermMemory,
		"longTermMemory":    result.LongTermMemory,
		"relevantKnowledge": result.RelevantKnowledge,
		"success":           true,
	}

	return response, nil
}

// handleToolProgrammingContext 处理获取编程上下文摘要请求
func (h *Handler) HandleToolProgrammingContext(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	sessionID, _ := params["sessionId"].(string)
	query, _ := params["query"].(string)

	if sessionID == "" {
		return nil, fmt.Errorf("缺少必需参数sessionId")
	}

	// 🔥 修复：从会话ID获取用户ID，实现真正的多用户隔离
	userID, err := h.contextService.GetUserIDFromSessionID(sessionID)
	if err != nil {
		log.Printf("[编程上下文] 从会话获取用户ID失败: %v", err)
		return map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("从会话获取用户ID失败: %v", err),
		}, nil
	}

	log.Printf("获取编程上下文摘要: 会话=%s, 用户ID=%s, 查询=%s", sessionID, userID, query)

	// 使用GetProgrammingContext方法获取编程上下文（与STDIO版本保持一致）
	result, err := h.contextService.GetProgrammingContext(context.Background(), sessionID, query)
	if err != nil {
		return nil, fmt.Errorf("获取编程上下文失败: %v", err)
	}

	log.Printf("获取编程上下文成功")
	return result, nil
}

// handleToolMemorizeContext 处理汇总到长期记忆的工具调用
func (h *Handler) HandleToolMemorizeContext(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	// 提取必需参数
	sessionID, ok := params["sessionId"].(string)
	if !ok || sessionID == "" {
		return nil, fmt.Errorf("缺少必要参数: sessionId")
	}

	content, ok := params["content"].(string)
	if !ok || content == "" {
		return nil, fmt.Errorf("缺少必要参数: content")
	}

	// 可选参数
	priority, _ := params["priority"].(string)
	if priority == "" {
		priority = "P2" // 默认中等优先级
	}

	// 处理元数据
	metadata := make(map[string]interface{})
	if metadataRaw, ok := params["metadata"]; ok {
		if metadataMap, ok := metadataRaw.(map[string]interface{}); ok {
			for k, v := range metadataMap {
				metadata[k] = v
			}
		}
	}

	// 设置基本元数据
	metadata["timestamp"] = time.Now().Unix()
	metadata["stored_at"] = time.Now().Format(time.RFC3339)
	metadata["manual_store"] = true // 标记为手动存储

	// 检查是否为待办事项
	bizType := 0 // 默认为常规记忆

	// 检查是否有显式标记为待办项
	if metadata != nil && metadata["type"] == "todo" {
		log.Printf("[记忆上下文] 元数据中显式标记为待办事项")
		metadata["type"] = "todo"
		bizType = models.BizTypeTodo
		log.Printf("[记忆上下文] 设置bizType=%d (BizTypeTodo)", models.BizTypeTodo)
	} else {
		// 使用正则表达式检查内容格式
		todoRegex := regexp.MustCompile(`(?i)^(- \[ \]|TODO:|待办:|提醒:|task:)`)
		todoKeywordsRegex := regexp.MustCompile(`(?i)(待办事项|todo item|task list|待完成|to-do|to do)`)

		if todoRegex.MatchString(content) || todoKeywordsRegex.MatchString(content) {
			log.Printf("[记忆上下文] 检测到待办事项: %s", content)
			metadata["type"] = "todo"
			bizType = models.BizTypeTodo
			log.Printf("[记忆上下文] 设置bizType=%d (BizTypeTodo)", models.BizTypeTodo)
		} else {
			metadata["type"] = "long_term_memory"
			log.Printf("[记忆上下文] 内容不匹配待办事项模式，设置为普通长期记忆")
		}
	}

	// 🔥 修复：从会话ID获取用户ID，实现真正的多用户隔离
	userID, err := h.contextService.GetUserIDFromSessionID(sessionID)
	if err != nil {
		log.Printf("[记忆上下文] 从会话获取用户ID失败: %v", err)
		return map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("从会话获取用户ID失败: %v", err),
		}, nil
	}

	log.Printf("[记忆上下文] 存储记忆: sessionID=%s, userID=%s, 类型=%s, 优先级=%s",
		sessionID, userID, metadata["type"], priority)

	// 创建存储上下文请求
	storeRequest := models.StoreContextRequest{
		SessionID: sessionID,
		UserID:    userID,
		Content:   content,
		Priority:  priority,
		Metadata:  metadata,
		BizType:   bizType,
	}

	log.Printf("存储长期记忆: sessionID=%s, 内容长度=%d, 优先级=%s, 类型=%s",
		sessionID, len(content), priority, metadata["type"])

	// 调用长期记忆存储
	memoryID, err := h.contextService.StoreContext(context.Background(), storeRequest)
	if err != nil {
		return nil, fmt.Errorf("存储长期记忆失败: %v", err)
	}

	response := map[string]interface{}{
		"memoryId": memoryID,
		"success":  true,
		"message":  "成功将内容存储到长期记忆",
		"type":     metadata["type"],
	}

	if userID != "" {
		response["userId"] = userID
	}

	log.Printf("[记忆上下文] 成功存储记忆: memoryID=%s, 类型=%s", memoryID, metadata["type"])
	return response, nil
}

// handleSummarizeToLongTerm 处理汇总到长期记忆的请求
func (h *Handler) HandleSummarizeToLongTerm(c *gin.Context) {
	var req models.SummarizeToLongTermRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的请求格式: " + err.Error(),
		})
		return
	}

	// 验证必需字段
	if req.SessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "缺少必需的sessionId字段",
		})
		return
	}

	// 调用服务处理请求
	memoryID, err := h.contextService.SummarizeToLongTermMemory(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "汇总到长期记忆失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":    "success",
		"memory_id": memoryID,
	})
}

// handleToolSessionManagement 处理会话管理请求
func (h *Handler) HandleToolSessionManagement(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	action, _ := params["action"].(string)
	if action == "" {
		return nil, fmt.Errorf("缺少必需参数: action")
	}

	sessionID, _ := params["sessionId"].(string)
	metadata, _ := params["metadata"].(map[string]interface{})

	log.Printf("会话管理: action=%s, sessionID=%s", action, sessionID)

	switch action {

	case "get_or_create":
		log.Printf("🔄 [MCP会话管理] ===== 开始get_or_create处理 =====")
		log.Printf("🔄 [MCP会话管理] 收到的原始参数: %+v", params)

		// 🔥 强制要求userId和workspaceRoot参数
		userID, _ := params["userId"].(string)
		workspaceRoot, _ := params["workspaceRoot"].(string)

		if userID == "" {
			return map[string]interface{}{
				"status":  "error",
				"message": "缺少必需参数: userId（用户ID不能为空）",
			}, nil
		}

		if workspaceRoot == "" {
			return map[string]interface{}{
				"status":  "error",
				"message": "缺少必需参数: workspaceRoot（工作空间路径不能为空）",
			}, nil
		}

		log.Printf("🔄 [MCP会话管理] 参数验证通过: userID=%s, workspaceRoot=%s", userID, workspaceRoot)

		// 🔥 正确的隔离逻辑：直接使用全局SessionStore，让GetWorkspaceSessionID处理用户+工作空间hash隔离
		sessionStore := h.contextService.SessionStore()

		// 调用工作空间会话ID生成逻辑 - 这里会自动处理用户+工作空间hash隔离
		log.Printf("🔄 [MCP会话管理] 调用GetWorkspaceSessionID: userID=%s, sessionID=%s, workspaceRoot=%s", userID, sessionID, workspaceRoot)
		session, isNewSession, err := utils.GetWorkspaceSessionID(sessionStore, userID, sessionID, workspaceRoot, metadata, h.config.SessionTimeout)
		if err != nil {
			log.Printf("🔄 [MCP会话管理] ❌ 获取或创建会话失败: %v", err)
			return nil, fmt.Errorf("获取或创建会话失败: %v", err)
		}

		log.Printf("🔄 [MCP会话管理] ✅ 会话处理成功: sessionID=%s, isNew=%t, 工作空间=%s", session.ID, isNewSession, workspaceRoot)

		// 构建响应
		sessionInfo := map[string]interface{}{
			"sessionId":    session.ID,
			"created":      session.CreatedAt,
			"lastActive":   session.LastActive,
			"status":       session.Status,
			"metadata":     session.Metadata,
			"summary":      session.Summary,
			"isNewSession": isNewSession,
			"codeContext":  make(map[string]interface{}),
		}

		if session.CodeContext != nil {
			for path, file := range session.CodeContext {
				sessionInfo["codeContext"].(map[string]interface{})[path] = map[string]interface{}{
					"language": file.Language,
					"lastEdit": file.LastEdit,
					"summary":  file.Summary,
				}
			}
		}

		// 更新会话活跃时间
		session.LastActive = time.Now()
		if err := sessionStore.SaveSession(session); err != nil {
			log.Printf("[会话管理-获取或创建] 更新会话活跃时间失败: %v", err)
		}

		// 🔥 新增：统一上下文完整性检查和工程感知分析
		if h.unifiedContextManager != nil {
			log.Printf("🧠 [统一上下文] 为会话 %s 检查上下文完整性", session.ID)

			// 检查统一上下文是否已存在
			unifiedContext, err := h.unifiedContextManager.GetContext(session.ID)
			var needProjectAnalysis bool

			if err != nil || unifiedContext == nil || unifiedContext.Project == nil {
				log.Printf("🔍 [工程感知] 检测到ProjectContext缺失，需要进行工程感知分析")
				needProjectAnalysis = true
			} else {
				// 检查ProjectContext的完整性
				project := unifiedContext.Project
				if project.ProjectName == "" || project.Description == "" {
					log.Printf("🔍 [工程感知] 检测到ProjectContext不完整，需要补充分析")
					needProjectAnalysis = true
				} else {
					log.Printf("✅ [工程感知] ProjectContext已存在且完整")
					needProjectAnalysis = false
				}
			}

			// 如果需要工程感知分析，生成分析prompt
			if needProjectAnalysis {
				analysisPrompt := h.buildProjectAnalysisPrompt(workspaceRoot, userID)
				sessionInfo["analysisPrompt"] = analysisPrompt
				log.Printf("📋 [工程感知] 已生成分析prompt，返回给客户端，长度: %d", len(analysisPrompt))
				log.Printf("📌 [工程感知] 客户端需要：1) 调用LLM分析工作空间 2) 首次检索时传入projectAnalysis")
			}
		} else {
			log.Printf("⚠️ [统一上下文] 统一上下文管理器未初始化")
		}

		return sessionInfo, nil

	default:
		return nil, fmt.Errorf("不支持的操作: %s", action)
	}
}

// buildProjectContextFromAnalysis 从工程分析结果构建ProjectContext
func (h *Handler) buildProjectContextFromAnalysis(projectAnalysis, workspaceID string) *models.ProjectContext {
	log.Printf("🔧 [工程感知] 开始构建ProjectContext，工作空间: %s", workspaceID)
	log.Printf("📝 [工程感知] 工程分析结果长度: %d", len(projectAnalysis))

	// 解析LLM返回的JSON结构化工程分析结果
	projectContext, err := h.parseProjectAnalysisJSON(projectAnalysis, workspaceID)
	if err != nil {
		log.Printf("❌ [工程感知] JSON解析失败: %v，跳过ProjectContext创建", err)
		return nil
	}

	log.Printf("✅ [工程感知] ProjectContext构建完成，项目: %s，语言: %s，置信度: %.2f",
		projectContext.ProjectName, projectContext.PrimaryLanguage, projectContext.ConfidenceLevel)

	return projectContext
}

// parseProjectAnalysisJSON 解析LLM返回的JSON结构化工程分析结果
func (h *Handler) parseProjectAnalysisJSON(projectAnalysis, workspaceID string) (*models.ProjectContext, error) {
	var analysisResult projectAnalysisJSON

	// 尝试解析JSON
	err := json.Unmarshal([]byte(projectAnalysis), &analysisResult)
	if err != nil {
		return nil, fmt.Errorf("JSON解析失败: %v", err)
	}

	// 转换为ProjectContext
	projectContext := &models.ProjectContext{
		ProjectName:     analysisResult.ProjectName,
		ProjectPath:     workspaceID,
		Description:     analysisResult.Description,
		PrimaryLanguage: analysisResult.PrimaryLanguage,
		TechStack:       h.convertTechStackFromString(analysisResult.TechStack, analysisResult.MainFramework),
		Architecture:    models.ArchitectureInfo{Pattern: analysisResult.Architecture},
		Dependencies:    h.convertDependenciesFromStringArray(analysisResult.KeyDependencies),
		LastAnalyzed:    time.Now(),
		ConfidenceLevel: analysisResult.ConfidenceLevel,
	}

	// 验证必要字段
	if projectContext.ProjectName == "" {
		return nil, fmt.Errorf("项目名称为空")
	}
	if projectContext.PrimaryLanguage == "" {
		return nil, fmt.Errorf("主要语言为空")
	}

	return projectContext, nil
}

// convertTechStackFromString 从字符串转换为TechStackItem数组
func (h *Handler) convertTechStackFromString(techStack, mainFramework string) []models.TechStackItem {
	var items []models.TechStackItem

	// 添加主框架
	if mainFramework != "" {
		items = append(items, models.TechStackItem{
			Name: mainFramework,
			Type: "framework",
		})
	}

	// 简单解析tech_stack字符串，如 "Go + Gin + PostgreSQL + Redis"
	if techStack != "" {
		parts := strings.Split(techStack, "+")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" && part != mainFramework { // 避免重复
				items = append(items, models.TechStackItem{
					Name: part,
					Type: "component",
				})
			}
		}
	}

	return items
}

// convertDependenciesFromStringArray 从字符串数组转换为DependencyInfo数组
func (h *Handler) convertDependenciesFromStringArray(dependencies []string) []models.DependencyInfo {
	var deps []models.DependencyInfo

	for _, dep := range dependencies {
		if dep != "" {
			deps = append(deps, models.DependencyInfo{
				Name:    dep,
				Version: "latest",
				Type:    "dependency",
			})
		}
	}

	return deps
}

// extractProjectName 从工作空间ID提取项目名
func extractProjectName(workspaceID string) string {
	// 简单实现：从路径中提取最后一段作为项目名
	parts := strings.Split(workspaceID, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return "unknown-project"
}

// buildProjectAnalysisPrompt 构建工程感知分析的精简高效prompt
func (h *Handler) buildProjectAnalysisPrompt(workspaceRoot, userID string) string {
	return fmt.Sprintf(`## 🎯 工程感知核心分析

你是工程分析专家，需要快速提取项目的核心特征，为AI上下文管理提供精准的工程基础信息。

### 🎯 分析目标
提取4个核心维度的关键信息：**业务感知、技术感知、时间感知、关联感知**

### 📊 核心分析维度

#### 1. 业务感知（What - 做什么业务）
- 项目核心功能和业务场景
- 主要业务模块和功能划分
- 目标用户和使用场景

#### 2. 技术感知（How - 怎么实现）  
- 技术栈和架构模式
- 核心组件和API设计
- 关键技术决策
- 关键依赖和技术约规

#### 3. 时间感知（When - 当前状态）
- 近期开发重点和活跃功能
- 当前痛点和技术挑战
- Git提交反映的需求变化

#### 4. 关联感知（Where - 关键入口）
- 重要文件和配置
- 模块间依赖关系
- 核心API和接口

### 📋 精简JSON输出（仅核心字段）

{
  "project_name": "<项目名称>",
  "description": "<核心业务功能，1-2句话>",
  "project_type": "<go/nodejs/python/java/rust/typescript/other>",
  "primary_language": "<主要编程语言>",
  "tech_stack": "<技术栈概述，如'Go + Gin + PostgreSQL + Redis'>",
  "architecture": "<架构模式，如'分层架构'、'微服务'、'RESTful API'>",
  "main_framework": "<主要框架>",
  "database": "<数据库方案>",
  "key_dependencies": ["<核心依赖1>", "<核心依赖2>", "<核心依赖3>"],
  "recent_focus": "<近期开发重点，1句话>",
  "current_pain_points": ["<当前痛点1>", "<当前痛点2>"],
  "active_requirements": ["<活跃需求1>", "<活跃需求2>"],
  "main_components": [
    {
      "name": "<组件名>",
      "purpose": "<职责>",
      "importance": "high/medium/low"
    }
  ],
  "important_files": [
    {
      "file_path": "<关键文件路径>",
      "role": "<作用>",
      "criticality": "critical/important"
    }
  ],
  "current_phase": "<development/testing/production/maintenance>",
  "confidence_level": <0.7-1.0的置信度>
}

### 💡 高效分析策略

**快速识别要点**：
1. **目录结构** → 快速判断项目类型和架构
2. **配置文件** → 识别技术栈（各语言配置文件：go.mod、package.json、pom.xml、Cargo.toml、requirements.txt、composer.json等）
3. **README/文档** → 理解业务功能和使用场景
4. **最近commit** → 了解当前开发重点和痛点
5. **API文件** → 识别核心业务逻辑和模块划分

**重点关注**：
- 🎯 **业务模块**：从目录结构/API设计看业务功能
- 🔧 **技术实现**：从配置和代码组织看技术选型
- ⏰ **当前状态**：从Git历史看最近在解决什么问题
- 🔗 **关键入口**：识别main文件、配置文件、核心API

### 📍 工作空间信息
路径: %s | 用户: %s

请基于实际代码和结构进行分析，输出精简但关键的ProjectContext信息。`, workspaceRoot, userID)
}

// handleToolStoreConversation 处理对话存储请求
func (h *Handler) HandleToolStoreConversation(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	sessionID, ok := params["sessionId"].(string)
	if !ok || sessionID == "" {
		return nil, fmt.Errorf("缺少必需参数: sessionId")
	}

	messagesRaw, ok := params["messages"]
	if !ok {
		return nil, fmt.Errorf("缺少必需参数: messages")
	}

	messages, ok := messagesRaw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("messages必须是数组")
	}

	batchID, _ := params["batchId"].(string)

	// 🔥 修复：从会话ID获取用户ID，实现真正的多用户隔离
	userID, err := h.contextService.GetUserIDFromSessionID(sessionID)
	if err != nil {
		log.Printf("[存储对话] 从会话获取用户ID失败: %v", err)
		return map[string]interface{}{
			"status":  "error",
			"message": fmt.Sprintf("从会话获取用户ID失败: %v", err),
		}, nil
	}

	log.Printf("存储对话: 会话=%s, 用户ID=%s, 消息数=%d", sessionID, userID, len(messages))

	// 如果未提供batchID，生成一个新的
	if batchID == "" {
		batchID = models.GenerateMemoryID("")
		log.Printf("生成新的batchId: %s", batchID)
	}

	// 构建消息请求
	var msgReqs []struct {
		Role        string                 `json:"role"`
		Content     string                 `json:"content"`
		ContentType string                 `json:"contentType,omitempty"`
		Priority    string                 `json:"priority,omitempty"`
		Metadata    map[string]interface{} `json:"metadata,omitempty"`
	}

	// 🔥 安全拦截：对每条消息进行安全检查和脱敏
	for i, msgRaw := range messages {
		if msgMap, ok := msgRaw.(map[string]interface{}); ok {
			role, _ := msgMap["role"].(string)
			content, _ := msgMap["content"].(string)

			if role != "" && content != "" {
				// 对消息内容进行安全扫描
				if h.securityService != nil {
					decision, err := h.securityService.ScanContentWithSmartDecision(
						ctx,
						sessionID,
						userID,
						content,
						[]string{},
					)
					if err != nil {
						log.Printf("[存储对话] 消息%d安全扫描失败: %v", i, err)
					} else {
						// 如果需要阻止
						if decision.ShouldBlock {
							log.Printf("[存储对话] 消息%d被阻止: 风险等级=%v, 原因=%s", i, decision.RiskLevel, decision.Reasoning)
							return map[string]interface{}{
								"status":    "error",
								"message":   "消息包含敏感信息，已被安全策略阻止",
								"messageId": i,
								"riskLevel": decision.RiskLevel,
								"reasoning": decision.Reasoning,
							}, nil
						}

						// 如果有脱敏内容，使用脱敏后的内容
						if decision.RedactedContent != "" {
							log.Printf("[存储对话] 消息%d使用脱敏后的内容: 风险等级=%v", i, decision.RiskLevel)
							content = decision.RedactedContent
						}

						// 记录审计日志
						if decision.ShouldAlert {
							log.Printf("[安全审计] 存储对话触发告警: sessionID=%s, userID=%s, messageId=%d, 风险分数=%.2f, 原因=%s",
								sessionID, userID, i, decision.RiskScore, decision.Reasoning)
						}
					}
				}

				metadata := map[string]interface{}{
					"batchId":   batchID,
					"timestamp": time.Now().Unix(),
					"type":      "conversation_message",
				}

				msgReqs = append(msgReqs, struct {
					Role        string                 `json:"role"`
					Content     string                 `json:"content"`
					ContentType string                 `json:"contentType,omitempty"`
					Priority    string                 `json:"priority,omitempty"`
					Metadata    map[string]interface{} `json:"metadata,omitempty"`
				}{
					Role:        role,
					Content:     content,
					ContentType: "text",
					Priority:    "P2",
					Metadata:    metadata,
				})
			}
		}
	}

	if len(msgReqs) == 0 {
		return nil, fmt.Errorf("没有有效的消息可存储")
	}

	// 调用上下文服务存储对话
	resp, err := h.contextService.StoreSessionMessages(context.Background(), models.StoreMessagesRequest{
		SessionID: sessionID,
		BatchID:   batchID,
		Messages:  msgReqs,
	})
	if err != nil {
		return nil, fmt.Errorf("存储对话失败: %v", err)
	}

	// 生成对话摘要
	summary, _ := h.contextService.SummarizeContext(context.Background(), models.SummarizeContextRequest{
		SessionID: sessionID,
		Format:    "text",
	})

	// 构建基本响应
	result := map[string]interface{}{
		"status":     "success",
		"batchId":    batchID,
		"messageIds": resp.MessageIDs,
		"summary":    summary,
	}

	// 🔥 添加LLM驱动的智能分析结果
	if resp.Metadata != nil {
		result["metadata"] = resp.Metadata
		log.Printf("✅ [存储对话] 添加LLM分析结果到响应: %+v", resp.Metadata)
	}

	// 转换消息格式用于本地存储
	var messageList []*models.Message
	for _, msgReq := range msgReqs {
		messageList = append(messageList, &models.Message{
			Role:      msgReq.Role,
			Content:   msgReq.Content,
			Timestamp: time.Now().Unix(),
		})
	}

	// 增强响应，添加本地存储指令
	context := map[string]interface{}{
		"messages":       messageList,
		"hasNewMessages": len(messageList) > 0,
	}

	// userID 已经在函数开头定义，直接使用
	return h.enhanceResponseWithLocalInstruction(result, sessionID, userID, models.LocalInstructionShortMemory, context), nil
}

// handleToolRetrieveMemory 处理记忆检索请求
func (h *Handler) HandleToolRetrieveMemory(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	sessionID, ok := params["sessionId"].(string)
	if !ok || sessionID == "" {
		return nil, fmt.Errorf("缺少必需参数: sessionId")
	}

	memoryID, _ := params["memoryId"].(string)
	batchID, _ := params["batchId"].(string)
	format, _ := params["format"].(string)

	if format == "" {
		format = "full"
	}

	// 严格按照一期stdio协议：获取用户ID并检查是否需要初始化
	userID, needUserInit, err := utils.GetUserID()
	if err != nil {
		log.Printf("[检索记忆] 获取用户ID失败: %v", err)
		return nil, fmt.Errorf("获取用户ID失败: %w", err)
	}

	// 严格按照一期逻辑：如果需要用户初始化，拒绝操作并返回初始化提示
	if needUserInit || userID == "" {
		log.Printf("[检索记忆] 用户未初始化，拒绝操作")
		return map[string]interface{}{
			"memories":          []string{},
			"shortTermMemory":   []string{},
			"sessionState":      map[string]interface{}{},
			"relevantKnowledge": []string{},
			"needUserInit":      true,
			"initPrompt":        "需要进行用户初始化才能检索记忆数据。请完成用户初始化流程。",
			"message":           "操作被拒绝：请先完成用户初始化",
		}, nil
	}

	log.Printf("检索记忆: 会话=%s, 用户ID=%s, memoryId=%s, batchId=%s", sessionID, userID, memoryID, batchID)

	// 调用上下文服务检索记忆
	// 🔥 修复：使用传入的ctx（包含拦截器注入的user_id等信息），而不是context.Background()
	result, err := h.contextService.RetrieveContext(ctx, models.RetrieveContextRequest{
		SessionID:     sessionID,
		MemoryID:      memoryID,
		BatchID:       batchID,
		SkipThreshold: true,
	})
	if err != nil {
		return nil, fmt.Errorf("检索记忆失败: %v", err)
	}

	return map[string]interface{}{
		"memories":          result.LongTermMemory,
		"shortTermMemory":   result.ShortTermMemory,
		"sessionState":      result.SessionState,
		"relevantKnowledge": result.RelevantKnowledge,
		"format":            format,
	}, nil
}

// handleToolRetrieveTodos 处理待办事项检索请求
func (h *Handler) HandleToolRetrieveTodos(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	sessionID, ok := params["sessionId"].(string)
	if !ok || sessionID == "" {
		return nil, fmt.Errorf("缺少必需参数: sessionId")
	}

	status, _ := params["status"].(string)
	if status == "" {
		status = "all"
	}

	limitStr, _ := params["limit"].(string)
	limit := 10
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil {
			limit = parsedLimit
		}
	}

	// 🔥 修复：从会话ID获取用户ID，实现真正的多用户隔离
	userID, err := h.contextService.GetUserIDFromSessionID(sessionID)
	if err != nil {
		log.Printf("[检索待办] 从会话获取用户ID失败: %v", err)
		return map[string]interface{}{
			"todos":   []*models.TodoItem{}, // 返回空列表
			"total":   0,
			"message": fmt.Sprintf("从会话获取用户ID失败: %v", err),
		}, nil
	}

	log.Printf("🔐 [DEBUG] 检索待办事项: 会话=%s, 用户ID=%s, 状态=%s, 限制=%d", sessionID, userID, status, limit)

	// 调用上下文服务检索待办事项 - 🔐 传递用户ID确保隔离
	todoResponse, err := h.contextService.RetrieveTodos(context.Background(), models.RetrieveTodosRequest{
		SessionID: sessionID,
		UserID:    userID, // 🔐 关键修复：传递用户ID
		Status:    status,
		Limit:     limit,
	})
	if err != nil {
		return nil, fmt.Errorf("检索待办事项失败: %v", err)
	}

	// 构建响应，包含用户隔离信息
	response := map[string]interface{}{
		"todos":  todoResponse.Items,
		"total":  todoResponse.Total,
		"userId": todoResponse.UserID,
	}

	return response, nil
}

// handleToolUserInitDialog 处理用户初始化对话请求（完全参照一期stdio协议实现）
func (h *Handler) HandleToolUserInitDialog(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	// 详细日志：开始处理用户初始化对话
	log.Printf("[用户初始化对话] 开始处理请求，参数: %+v", params)

	// 验证参数
	sessionID, ok := params["sessionId"].(string)
	if !ok || sessionID == "" {
		return nil, fmt.Errorf("错误: sessionId必须是非空字符串")
	}

	userResponse, _ := params["userResponse"].(string)
	log.Printf("[用户初始化对话] 处理会话ID=%s, 用户响应=%q", sessionID, userResponse)

	// 如果有用户响应，则处理响应
	var state *utils.DialogState
	var err error

	// 使用defer捕获和记录任何可能的panic
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[用户初始化对话] 发生panic: %v", r)
		}
	}()

	// 首先检查会话状态是否已经存在
	dialogExists := false
	// 这里不直接访问dialogStates，而是通过尝试初始化来检查
	tmpState, _ := utils.InitializeUserByDialog(sessionID)
	if tmpState != nil {
		dialogExists = true
		log.Printf("[用户初始化对话] 检测到会话状态已存在: state=%s", tmpState.State)
	}

	if userResponse != "" {
		log.Printf("[用户初始化对话] 处理用户响应: %q", userResponse)

		// 如果有响应但没有会话状态，可能是第一次调用，先确保初始化
		if !dialogExists {
			log.Printf("[用户初始化对话] 警告: 收到用户响应但会话状态不存在，先初始化状态")
			tmpState, err = utils.InitializeUserByDialog(sessionID)
			if err != nil {
				log.Printf("[用户初始化对话] 初始化对话状态失败: %v", err)
				return nil, fmt.Errorf("处理用户配置对话出错: 无法初始化会话状态: %v", err)
			}
		}

		// 添加详细的错误处理
		state, err = utils.HandleUserDialogResponse(sessionID, userResponse)
		if err != nil {
			log.Printf("[用户初始化对话] 处理用户响应失败: %v", err)
		}
	} else {
		log.Printf("[用户初始化对话] 初始化或获取当前对话状态")
		// 初始化或获取当前对话状态
		state, err = utils.InitializeUserByDialog(sessionID)
		if err != nil {
			log.Printf("[用户初始化对话] 初始化对话状态失败: %v", err)
		}
	}

	if err != nil {
		log.Printf("[用户初始化对话] 错误: %v", err)
		return nil, fmt.Errorf("处理用户配置对话出错: %v", err)
	}

	log.Printf("[用户初始化对话] 获取到对话状态: state=%s, userID=%s", state.State, state.UserID)

	// 严格按照一期逻辑：如果用户配置完成，强制更新全局缓存
	if state.State == utils.DialogStateCompleted && state.UserID != "" {
		log.Printf("[用户初始化对话] 用户配置完成，强制更新全局缓存，UserID: %s", state.UserID)
		// 强制确保用户ID被缓存（这是关键修复）
		utils.SetCachedUserID(state.UserID)
		log.Printf("[用户初始化对话] 缓存设置完成，验证: %s", utils.GetCachedUserID())
	} else if state.State == utils.DialogStateNewUser && state.UserID != "" {
		log.Printf("[用户初始化对话] 新用户创建完成，立即更新缓存，UserID: %s", state.UserID)
		// 新用户也需要立即设置缓存
		utils.SetCachedUserID(state.UserID)
		log.Printf("[用户初始化对话] 新用户缓存设置完成，验证: %s", utils.GetCachedUserID())
	}

	// 构建响应
	result := map[string]interface{}{
		"state": state.State,
	}

	// 根据状态添加相应字段
	switch state.State {
	case utils.DialogStateNewUser:
		result["userId"] = state.UserID
		result["message"] = "已为您创建新用户账号"
		result["welcomeMessage"] = "欢迎使用上下文记忆管理工具！您的数据将与您的用户ID关联。请妥善保管您的用户ID，当您在其他设备使用时需要输入它。"
		log.Printf("[用户初始化对话] 新用户状态: userID=%s", state.UserID)
	case utils.DialogStateExisting:
		result["message"] = "请输入您的用户ID以继续"
		result["prompt"] = "用户ID格式为'user_'开头加8位字母数字，您可以直接粘贴完整ID"
		result["helpText"] = "如果您没有用户ID或想创建新账号，请回复'创建新账号'。如需重置流程，请回复'重置'"
		log.Printf("[用户初始化对话] 已有用户状态，等待输入用户ID")
	case utils.DialogStateCompleted:
		result["userId"] = state.UserID
		result["message"] = "用户配置已完成"
		result["isFirstTime"] = (userResponse != "") // 标记是否是首次配置完成
		log.Printf("[用户初始化对话] 配置完成状态: userID=%s, isFirstTime=%v", state.UserID, userResponse != "")

		// 增强响应，添加本地存储指令
		context := map[string]interface{}{
			"userInitialized": true,
		}
		return h.enhanceResponseWithLocalInstruction(result, sessionID, state.UserID, models.LocalInstructionUserConfig, context), nil
	default:
		result["message"] = "欢迎使用 Context-Keeper！检测到您还未配置用户信息。为了更好地管理您的上下文数据，请在 Cursor/VSCode 中打开 Context-Keeper 扩展配置界面完成用户信息设置。"
		result["prompt"] = "请在 IDE 中配置用户信息"
		result["helpText"] = "您可以在扩展界面中：1) 输入已有的用户ID（如果您在其他设备使用过），或 2) 生成新的用户ID。配置完成后，所有功能将自动可用。"
		result["instructions"] = []string{
			"🔧 打开 Context-Keeper 状态面板：按 Ctrl+Shift+P，搜索 'Context-Keeper: 显示状态面板'",
			"👤 配置用户信息：在用户配置区域输入现有用户ID或点击生成新ID",
			"💾 保存配置：点击保存按钮，配置将自动写入本地文件",
			"✅ 完成设置：配置成功后即可正常使用所有功能",
		}
		log.Printf("[用户初始化对话] 引导用户到扩展配置界面")
	}

	return result, nil
}

// handleLocalOperationCallback 处理本地操作回调
func (h *Handler) HandleLocalOperationCallback(c *gin.Context) {
	var req models.LocalCallbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "无效的请求格式: " + err.Error(),
		})
		return
	}

	// 验证必填字段
	if req.CallbackID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "缺少必填字段: callbackId",
		})
		return
	}

	// 根据回调ID确定指令类型
	instructionType := h.localInstructionService.GetCallbackInstructionType(req.CallbackID)

	log.Printf("[本地回调] 接收到本地操作回调: callbackId=%s, success=%t, type=%s",
		req.CallbackID, req.Success, instructionType)

	// 处理回调结果
	if req.Success {
		log.Printf("[本地回调] 本地操作成功: %s", req.CallbackID)

		// 可以在这里添加成功后的后续处理逻辑
		if req.Data != nil {
			log.Printf("[本地回调] 回调数据: %+v", req.Data)
		}
	} else {
		log.Printf("[本地回调] 本地操作失败: %s, 错误: %s", req.CallbackID, req.Error)
	}

	// 返回确认响应
	c.JSON(http.StatusOK, gin.H{
		"status":     "success",
		"message":    "回调已处理",
		"callbackId": req.CallbackID,
		"timestamp":  time.Now().Unix(),
	})
}

// enhanceResponseWithLocalInstruction 增强响应，添加本地存储指令
func (h *Handler) enhanceResponseWithLocalInstruction(response map[string]interface{}, sessionID, userID string, instructionType models.LocalInstructionType, context map[string]interface{}) map[string]interface{} {
	// 检查是否应该生成本地指令
	if !h.localInstructionService.ShouldGenerateLocalInstruction(instructionType, context) {
		return response
	}

	var instruction *models.LocalInstruction

	switch instructionType {
	case models.LocalInstructionUserConfig:
		if userID != "" {
			instruction = h.localInstructionService.GenerateUserConfigUpdateInstruction(userID)
		}
	case models.LocalInstructionSessionStore:
		if session, ok := context["session"].(*models.Session); ok && userID != "" {
			instruction = h.localInstructionService.GenerateSessionStoreInstruction(session, userID)
		}
	case models.LocalInstructionShortMemory:
		if messages, ok := context["messages"].([]*models.Message); ok && userID != "" {
			instruction = h.localInstructionService.GenerateShortMemoryStoreInstruction(sessionID, messages, userID)
		}
	case models.LocalInstructionCodeContext:
		if codeContext, ok := context["codeContext"].(map[string]*models.CodeFile); ok && userID != "" {
			instruction = h.localInstructionService.GenerateCodeContextStoreInstruction(sessionID, codeContext, userID)
		}
	case models.LocalInstructionPreferences:
		if preferences, ok := context["preferences"].(*models.LocalPreferencesData); ok && userID != "" {
			instruction = h.localInstructionService.GeneratePreferencesStoreInstruction(preferences, userID)
		}
	case models.LocalInstructionCacheUpdate:
		if sessionStates, ok := context["sessionStates"].(map[string]interface{}); ok && userID != "" {
			instruction = h.localInstructionService.GenerateCacheUpdateInstruction(userID, sessionStates)
		}
	}

	// 如果生成了指令，添加到响应中
	if instruction != nil {
		log.Printf("[本地指令] 生成本地存储指令: type=%s, target=%s, callbackId=%s",
			instruction.Type, instruction.Target, instruction.CallbackID)
		response["localInstruction"] = instruction

		// 🔥 关键：通过WebSocket推送指令到客户端 - 使用精确推送
		if services.GlobalWSManager != nil {
			var callbackChan chan models.CallbackResult

			// 优先尝试基于sessionID的精确推送
			if sessionID != "" {
				if sessionChan, sessionErr := services.GlobalWSManager.PushInstructionToSession(sessionID, *instruction); sessionErr == nil {
					callbackChan = sessionChan
					log.Printf("[WebSocket] 本地指令已精确推送到会话 %s -> 用户: %s", sessionID, userID)
				} else {
					log.Printf("[WebSocket] 精确推送失败 (会话 %s 未注册)，回退到用户级别推送: %v", sessionID, sessionErr)
					// 回退到传统的用户级别推送
					if fallbackChan, fallbackErr := services.GlobalWSManager.PushInstruction(userID, *instruction); fallbackErr == nil {
						callbackChan = fallbackChan
						log.Printf("[WebSocket] 回退推送成功: %s -> 用户: %s", instruction.CallbackID, userID)
					} else {
						log.Printf("[WebSocket] 回退推送也失败: %v", fallbackErr)
					}
				}
			} else {
				// 如果没有sessionID，直接使用用户级别推送
				if userChan, userErr := services.GlobalWSManager.PushInstruction(userID, *instruction); userErr == nil {
					callbackChan = userChan
					log.Printf("[WebSocket] 本地指令已推送: %s -> 用户: %s", instruction.CallbackID, userID)
				} else {
					log.Printf("[WebSocket] 推送指令失败: %v, 用户可能未连接WebSocket: %s", userErr, userID)
				}
			}

			// 如果推送成功，异步等待回调结果
			if callbackChan != nil {
				go func() {
					select {
					case callbackResult := <-callbackChan:
						log.Printf("[WebSocket] 本地指令执行完成: %s - %s", instruction.CallbackID, callbackResult.Message)
					case <-time.After(30 * time.Second):
						log.Printf("[WebSocket] 本地指令执行超时: %s", instruction.CallbackID)
					}
				}()
			}
		} else {
			log.Printf("[WebSocket] WebSocket管理器未初始化，跳过推送")
		}
	}

	return response
}

// 辅助函数：创建增强响应
func (h *Handler) createEnhancedResponse(result interface{}, success bool, message string, sessionID, userID string, instructionType models.LocalInstructionType, context map[string]interface{}) map[string]interface{} {
	response := map[string]interface{}{
		"result":  result,
		"success": success,
		"message": message,
	}

	// 添加本地存储指令
	return h.enhanceResponseWithLocalInstruction(response, sessionID, userID, instructionType, context)
}

// handleToolLocalOperationCallback 处理本地操作回调工具调用
func (h *Handler) HandleToolLocalOperationCallback(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	callbackID, ok := params["callbackId"].(string)
	if !ok || callbackID == "" {
		return nil, fmt.Errorf("缺少必需参数: callbackId")
	}

	success, _ := params["success"].(bool)
	errorMsg, _ := params["error"].(string)
	data, _ := params["data"].(map[string]interface{})
	timestamp, _ := params["timestamp"].(float64)

	log.Printf("[工具回调] 本地操作回调: callbackId=%s, success=%t", callbackID, success)

	// 根据回调ID确定指令类型
	instructionType := h.localInstructionService.GetCallbackInstructionType(callbackID)

	// 处理回调结果
	if success {
		log.Printf("[工具回调] 本地操作成功: %s, 类型: %s", callbackID, instructionType)
		if data != nil {
			log.Printf("[工具回调] 回调数据: %+v", data)
		}
	} else {
		log.Printf("[工具回调] 本地操作失败: %s, 错误: %s", callbackID, errorMsg)
	}

	return map[string]interface{}{
		"status":       "success",
		"message":      "回调已处理",
		"callbackId":   callbackID,
		"acknowledged": true,
		"serverTime":   time.Now().Unix(),
		"clientTime":   int64(timestamp),
	}, nil
}

// 在init函数或者路由注册函数中添加WebSocket路由
func (h *Handler) RegisterWebSocketRoutes(router *gin.Engine) {
	// WebSocket连接端点
	router.GET("/ws", h.HandleWebSocket)

	// WebSocket状态查询端点
	router.GET("/ws/status", h.GetWebSocketStatus)

	// 🔥 WebSocket连接详情调试端点
	router.GET("/ws/debug", h.GetWSDebugStatus)

	// 🔥 WebSocket会话注册端点
	router.POST("/api/ws/register-session", h.HandleSessionRegister)

	log.Println("WebSocket路由已注册: /ws, /ws/status, /ws/debug, /api/ws/register-session")
}

// 🔥 新增：查询所有有效未过期session列表的API - 支持分页
func (h *Handler) HandleGetSessionsList(c *gin.Context) {
	log.Printf("[API] 收到查询会话列表请求")

	// 获取分页参数
	page := 1
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	pageSize := 20 // 默认每页20个
	if pageSizeStr := c.Query("pageSize"); pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 && ps <= 100 {
			pageSize = ps
		}
	}

	// 计算偏移量
	offset := (page - 1) * pageSize

	// 其他查询参数
	includeExpired := c.Query("includeExpired") == "true"

	// 获取所有用户的会话统计
	var allSessions []map[string]interface{}
	var totalCount int
	var activeCount int
	var expiredCount int

	// 遍历所有用户的会话存储
	baseStorePath := h.contextService.SessionStore().GetStorePath()
	usersPath := filepath.Join(baseStorePath, "users")

	if userDirs, err := os.ReadDir(usersPath); err == nil {
		for _, userDir := range userDirs {
			if !userDir.IsDir() {
				continue
			}

			userID := userDir.Name()
			userSessionStore, err := h.contextService.GetUserSessionStore(userID)
			if err != nil {
				log.Printf("[API] 警告: 获取用户%s的会话存储失败: %v", userID, err)
				continue
			}

			// 获取此用户的所有会话
			sessions := userSessionStore.GetSessionList()
			now := time.Now()
			sessionTimeout := time.Duration(h.config.SessionTimeout) * time.Minute

			for _, session := range sessions {
				totalCount++

				// 检查是否过期
				isExpired := session.Status != models.SessionStatusActive ||
					now.Sub(session.LastActive) > sessionTimeout

				if isExpired {
					expiredCount++
				} else {
					activeCount++
				}

				// 根据参数决定是否包含过期会话
				if !includeExpired && isExpired {
					continue
				}

				sessionInfo := map[string]interface{}{
					"sessionId":    session.ID,
					"userId":       userID,
					"createdAt":    session.CreatedAt,
					"lastActive":   session.LastActive,
					"status":       session.Status,
					"isExpired":    isExpired,
					"messageCount": len(session.Messages),
				}

				// 添加工作空间信息（如果有）
				if session.Metadata != nil {
					if workspaceHash, ok := session.Metadata["workspaceHash"].(string); ok {
						sessionInfo["workspaceHash"] = workspaceHash
					}
				}

				allSessions = append(allSessions, sessionInfo)
			}
		}
	} else {
		log.Printf("[API] 警告: 读取用户目录失败: %v", err)
	}

	// 按最后活动时间排序（最新的在前）
	sort.Slice(allSessions, func(i, j int) bool {
		timeI := allSessions[i]["lastActive"].(time.Time)
		timeJ := allSessions[j]["lastActive"].(time.Time)
		return timeI.After(timeJ)
	})

	// 🔥 分页处理
	totalFiltered := len(allSessions)
	totalPages := (totalFiltered + pageSize - 1) / pageSize

	var paginatedSessions []map[string]interface{}
	if offset < totalFiltered {
		end := offset + pageSize
		if end > totalFiltered {
			end = totalFiltered
		}
		paginatedSessions = allSessions[offset:end]
	}

	response := map[string]interface{}{
		"status":        "success",
		"totalCount":    totalCount,
		"activeCount":   activeCount,
		"expiredCount":  expiredCount,
		"filteredCount": totalFiltered,
		"returnedCount": len(paginatedSessions),
		"sessions":      paginatedSessions,
		"pagination": map[string]interface{}{
			"page":        page,
			"pageSize":    pageSize,
			"totalPages":  totalPages,
			"hasNext":     page < totalPages,
			"hasPrevious": page > 1,
		},
		"timestamp": time.Now(),
	}

	log.Printf("[API] 查询会话列表完成: 总数=%d, 活跃=%d, 过期=%d, 过滤后=%d, 返回=%d, 页码=%d/%d",
		totalCount, activeCount, expiredCount, totalFiltered, len(paginatedSessions), page, totalPages)

	c.JSON(http.StatusOK, response)
}

// 🔥 新增：根据用户ID查询session详情的API
func (h *Handler) HandleGetUserSessionDetail(c *gin.Context) {
	userID := c.Param("userId")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "用户ID不能为空",
		})
		return
	}

	log.Printf("[API] 收到查询用户会话详情请求: userID=%s", userID)

	// 获取查询参数
	includeExpired := c.Query("includeExpired") == "true"
	includeMessages := c.Query("includeMessages") == "true"

	// 获取用户的会话存储
	userSessionStore, err := h.contextService.GetUserSessionStore(userID)
	if err != nil {
		log.Printf("[API] 获取用户会话存储失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("获取用户会话存储失败: %v", err),
		})
		return
	}

	// 获取用户的所有会话
	sessions := userSessionStore.GetSessionList()
	now := time.Now()
	sessionTimeout := time.Duration(h.config.SessionTimeout) * time.Minute

	var userSessions []map[string]interface{}
	var totalCount int
	var activeCount int

	for _, session := range sessions {
		totalCount++

		// 检查是否过期
		isExpired := session.Status != models.SessionStatusActive ||
			now.Sub(session.LastActive) > sessionTimeout

		if !isExpired {
			activeCount++
		}

		// 根据参数决定是否包含过期会话
		if !includeExpired && isExpired {
			continue
		}

		sessionDetail := map[string]interface{}{
			"sessionId":    session.ID,
			"userId":       userID,
			"createdAt":    session.CreatedAt,
			"lastActive":   session.LastActive,
			"status":       session.Status,
			"isExpired":    isExpired,
			"messageCount": len(session.Messages),
			"summary":      session.Summary,
		}

		// 添加元数据信息
		if session.Metadata != nil {
			sessionDetail["metadata"] = session.Metadata
		}

		// 添加代码上下文信息
		if session.CodeContext != nil && len(session.CodeContext) > 0 {
			codeFiles := make([]map[string]interface{}, 0)
			for filePath, codeFile := range session.CodeContext {
				codeFiles = append(codeFiles, map[string]interface{}{
					"filePath": filePath,
					"language": codeFile.Language,
					"lastEdit": time.Unix(codeFile.LastEdit, 0),
					"summary":  codeFile.Summary,
				})
			}
			sessionDetail["codeContext"] = codeFiles
		}

		// 添加编辑历史信息
		if session.EditHistory != nil && len(session.EditHistory) > 0 {
			editCount := len(session.EditHistory)
			sessionDetail["editHistoryCount"] = editCount

			// 只返回最近几条编辑记录的摘要
			recentEdits := make([]map[string]interface{}, 0)
			maxRecent := 5
			if editCount > maxRecent {
				maxRecent = editCount
			}

			for i := editCount - maxRecent; i < editCount; i++ {
				edit := session.EditHistory[i]
				recentEdits = append(recentEdits, map[string]interface{}{
					"timestamp": time.Unix(edit.Timestamp, 0),
					"filePath":  edit.FilePath,
					"type":      edit.Type,
					"position":  edit.Position,
				})
			}
			sessionDetail["recentEdits"] = recentEdits
		}

		// 如果请求包含消息，添加最近的消息
		if includeMessages && session.Messages != nil && len(session.Messages) > 0 {
			messageCount := len(session.Messages)
			maxMessages := 10 // 最多返回最近10条消息
			if messageCount > maxMessages {
				maxMessages = messageCount
			}

			recentMessages := make([]map[string]interface{}, 0)
			for i := messageCount - maxMessages; i < messageCount; i++ {
				msg := session.Messages[i]
				recentMessages = append(recentMessages, map[string]interface{}{
					"id":        msg.ID,
					"role":      msg.Role,
					"content":   msg.Content[:min(200, len(msg.Content))], // 截断长内容
					"timestamp": time.Unix(msg.Timestamp, 0),
				})
			}
			sessionDetail["recentMessages"] = recentMessages
		}

		userSessions = append(userSessions, sessionDetail)
	}

	// 按最后活动时间排序（最新的在前）
	sort.Slice(userSessions, func(i, j int) bool {
		timeI := userSessions[i]["lastActive"].(time.Time)
		timeJ := userSessions[j]["lastActive"].(time.Time)
		return timeI.After(timeJ)
	})

	response := map[string]interface{}{
		"status":        "success",
		"userId":        userID,
		"totalCount":    totalCount,
		"activeCount":   activeCount,
		"returnedCount": len(userSessions),
		"sessions":      userSessions,
		"timestamp":     time.Now(),
	}

	log.Printf("[API] 查询用户会话详情完成: userID=%s, 总数=%d, 活跃=%d, 返回=%d",
		userID, totalCount, activeCount, len(userSessions))

	c.JSON(http.StatusOK, response)
}

// min 辅助函数
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// RegisterManagementRoutes 注册Session管理接口 - 独立于MCP协议的管理端点
func (h *Handler) RegisterManagementRoutes(router *gin.Engine) {
	h.RegisterSessionDeletionRoutes(router)

	// Session管理接口组 - 专用于系统监控和管理
	management := router.Group("/management")
	{
		// 查询所有有效未过期session列表（支持分页）
		management.GET("/sessions", h.HandleGetSessionsList)

		// 根据用户ID查询session详情
		management.GET("/users/:userId/sessions", h.HandleGetUserSessionDetail)
	}

	// 🔥 新增：用户管理接口组
	api := router.Group("/api")
	{
		// 🔥 新增：用户管理接口
		api.POST("/users", h.HandleCreateUser)        // 新增用户（包含唯一性校验）
		api.PUT("/users/:userId", h.HandleUpdateUser) // 变更用户信息
		api.GET("/users/:userId", h.HandleGetUser)    // 查询用户信息（用于验证）
	}

	log.Println("Session管理接口已注册:")
	log.Println("  GET  /management/sessions - 查询所有会话列表（分页）")
	log.Println("  GET  /management/users/:userId/sessions - 查询用户会话详情")
	log.Println("用户管理接口已注册:")
	log.Println("  POST /api/users - 新增用户（包含唯一性校验）")
	log.Println("  PUT  /api/users/:userId - 变更用户信息")
	log.Println("  GET  /api/users/:userId - 查询用户信息")
}

// handleCreateUser 新增用户接口（包含唯一性校验）
func (h *Handler) HandleCreateUser(c *gin.Context) {
	log.Printf("🔥 [用户管理] ===== 开始处理用户创建请求 =====")

	var req struct {
		UserID     string                 `json:"userId" binding:"required"`
		FirstUsed  string                 `json:"firstUsed"`
		LastActive string                 `json:"lastActive"`
		DeviceInfo map[string]interface{} `json:"deviceInfo"`
		Metadata   map[string]interface{} `json:"metadata"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("❌ [用户管理] 解析新增用户请求失败: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "请求格式错误: " + err.Error(),
		})
		return
	}

	log.Printf("📝 [用户管理] 解析用户创建请求成功 - 用户ID: %s, 设备信息: %+v", req.UserID, req.DeviceInfo)

	// 只有在使用向量存储类型的用户仓库时才需要向量服务
	if h.vectorService != nil {
		log.Printf("✅ [用户管理] 向量服务配置检查通过")

		// 确保用户集合已初始化
		log.Printf("🔧 [用户管理] 开始初始化用户集合...")
		if err := h.vectorService.InitUserCollection(); err != nil {
			log.Printf("❌ [用户管理] 初始化用户集合失败: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "初始化用户集合失败，请检查向量数据库配置",
			})
			return
		}
		log.Printf("✅ [用户管理] 用户集合初始化成功")
	} else {
		log.Printf("ℹ️ [用户管理] 使用非向量存储类型的用户仓库，跳过向量服务检查")
	}

	// 检查用户ID唯一性
	log.Printf("🔍 [用户管理] 开始检查用户ID唯一性: %s", req.UserID)
	exists, err := h.userRepository.CheckUserExists(req.UserID)
	if err != nil {
		log.Printf("❌ [用户管理] 检查用户ID唯一性失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "检查用户ID唯一性失败",
		})
		return
	}

	if exists {
		log.Printf("⚠️ [用户管理] 用户ID已存在: %s", req.UserID)
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"message": "用户ID已存在，请更换其他用户ID",
			"userId":  req.UserID,
		})
		return
	}
	log.Printf("✅ [用户管理] 用户ID唯一性检查通过: %s", req.UserID)

	// 设置默认值
	if req.FirstUsed == "" {
		req.FirstUsed = time.Now().Format(time.RFC3339)
		log.Printf("📅 [用户管理] 设置默认首次使用时间: %s", req.FirstUsed)
	}
	if req.LastActive == "" {
		req.LastActive = time.Now().Format(time.RFC3339)
		log.Printf("📅 [用户管理] 设置默认最后活跃时间: %s", req.LastActive)
	}

	// 创建用户信息
	userInfo := &models.UserInfo{
		UserID:     req.UserID,
		FirstUsed:  req.FirstUsed,
		LastActive: req.LastActive,
		DeviceInfo: req.DeviceInfo,
		Metadata:   req.Metadata,
	}
	log.Printf("📦 [用户管理] 构建用户信息对象完成: UserID=%s, FirstUsed=%s, LastActive=%s",
		userInfo.UserID, userInfo.FirstUsed, userInfo.LastActive)

	// 创建用户信息
	log.Printf("💾 [用户管理] 开始创建用户信息...")
	if err := h.userRepository.CreateUser(userInfo); err != nil {
		log.Printf("❌ [用户管理] 创建用户信息失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "创建用户信息失败",
		})
		return
	}
	log.Printf("✅ [用户管理] 用户信息创建成功: %s", req.UserID)

	log.Printf("🎉 [用户管理] 用户新增完成: %s", req.UserID)
	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "用户新增成功",
		"userId":  req.UserID,
		"data": gin.H{
			"userId":     userInfo.UserID,
			"firstUsed":  userInfo.FirstUsed,
			"lastActive": userInfo.LastActive,
			"createdAt":  userInfo.CreatedAt,
		},
	})
	log.Printf("🔥 [用户管理] ===== 用户创建请求处理完成 =====")
}

// handleUpdateUser 变更用户信息接口
func (h *Handler) HandleUpdateUser(c *gin.Context) {
	userID := c.Param("userId")
	log.Printf("🔥 [用户管理] ===== 开始处理用户更新请求: %s =====", userID)

	if userID == "" {
		log.Printf("❌ [用户管理] 用户ID参数为空")
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "用户ID不能为空",
		})
		return
	}

	var req struct {
		FirstUsed  string                 `json:"firstUsed"`
		LastActive string                 `json:"lastActive"`
		DeviceInfo map[string]interface{} `json:"deviceInfo"`
		Metadata   map[string]interface{} `json:"metadata"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("❌ [用户管理] 解析更新用户请求失败: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "请求格式错误: " + err.Error(),
		})
		return
	}
	log.Printf("📝 [用户管理] 解析用户更新请求成功 - 用户ID: %s", userID)

	// 确保用户存储可用
	if h.userRepository == nil {
		log.Printf("❌ [用户管理] 用户存储未配置")
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "用户存储未配置",
		})
		return
	}
	log.Printf("✅ [用户管理] 用户存储配置检查通过")

	// 先查询用户是否存在
	log.Printf("🔍 [用户管理] 查询现有用户信息: %s", userID)
	existingUser, err := h.userRepository.GetUser(userID)
	if err != nil {
		log.Printf("❌ [用户管理] 查询用户信息失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "查询用户信息失败",
		})
		return
	}

	if existingUser == nil {
		log.Printf("⚠️ [用户管理] 用户不存在: %s", userID)
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "用户不存在",
			"userId":  userID,
		})
		return
	}
	log.Printf("✅ [用户管理] 找到现有用户信息: %+v", existingUser)

	// 合并现有信息和更新信息
	updatedUser := &models.UserInfo{
		UserID:     userID,
		FirstUsed:  existingUser.FirstUsed, // 保持原有的首次使用时间
		LastActive: req.LastActive,
		DeviceInfo: req.DeviceInfo,
		Metadata:   req.Metadata,
		CreatedAt:  existingUser.CreatedAt, // 保持原有的创建时间
	}

	// 如果没有提供LastActive，使用当前时间
	if updatedUser.LastActive == "" {
		updatedUser.LastActive = time.Now().Format(time.RFC3339)
		log.Printf("📅 [用户管理] 设置默认最后活跃时间: %s", updatedUser.LastActive)
	}

	// 如果没有提供FirstUsed，使用原有值或当前时间
	if req.FirstUsed != "" {
		updatedUser.FirstUsed = req.FirstUsed
		log.Printf("📅 [用户管理] 更新首次使用时间: %s", updatedUser.FirstUsed)
	} else if updatedUser.FirstUsed == "" {
		updatedUser.FirstUsed = time.Now().Format(time.RFC3339)
		log.Printf("📅 [用户管理] 设置默认首次使用时间: %s", updatedUser.FirstUsed)
	}

	log.Printf("📦 [用户管理] 构建更新后的用户信息: %+v", updatedUser)

	// 更新用户信息
	log.Printf("💾 [用户管理] 开始更新用户信息...")
	if err := h.userRepository.UpdateUser(updatedUser); err != nil {
		log.Printf("❌ [用户管理] 更新用户信息失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "更新用户信息失败",
		})
		return
	}
	log.Printf("✅ [用户管理] 用户信息更新成功: %s", userID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "用户信息更新成功",
		"userId":  userID,
		"data": gin.H{
			"userId":     updatedUser.UserID,
			"firstUsed":  updatedUser.FirstUsed,
			"lastActive": updatedUser.LastActive,
			"updatedAt":  updatedUser.UpdatedAt,
		},
	})
	log.Printf("🔥 [用户管理] ===== 用户更新请求处理完成: %s =====", userID)
}

// handleGetUser 查询用户信息接口
func (h *Handler) HandleGetUser(c *gin.Context) {
	userID := c.Param("userId")
	log.Printf("🔥 [用户管理] ===== 开始处理用户查询请求: %s =====", userID)

	if userID == "" {
		log.Printf("❌ [用户管理] 用户ID参数为空")
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "用户ID不能为空",
		})
		return
	}

	// 确保用户存储可用
	if h.userRepository == nil {
		log.Printf("❌ [用户管理] 用户存储未配置")
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "用户存储未配置",
		})
		return
	}
	log.Printf("✅ [用户管理] 用户存储配置检查通过")

	// 查询用户信息
	log.Printf("🔍 [用户管理] 开始查询用户信息: %s", userID)
	userInfo, err := h.userRepository.GetUser(userID)
	if err != nil {
		log.Printf("❌ [用户管理] 查询用户信息失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "查询用户信息失败",
		})
		return
	}

	if userInfo == nil {
		log.Printf("⚠️ [用户管理] 用户不存在: %s", userID)
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "用户不存在",
			"userId":  userID,
		})
		return
	}

	log.Printf("✅ [用户管理] 用户信息查询成功: %s, 数据: %+v", userID, userInfo)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "查询成功",
		"userId":  userID,
		"data":    userInfo,
	})
	log.Printf("🔥 [用户管理] ===== 用户查询请求处理完成: %s =====", userID)
}

// 🔥 新增：调试WebSocket连接详情
func (h *Handler) HandleDebugWSConnections(c *gin.Context) {
	onlineUsers := services.GlobalWSManager.GetOnlineUsers()
	connectionStats := services.GlobalWSManager.GetConnectionStats()

	// 获取每个用户的详细连接信息
	userDetails := make(map[string]interface{})
	for _, userID := range onlineUsers {
		connections := services.GlobalWSManager.GetUserConnections(userID)
		userDetails[userID] = map[string]interface{}{
			"connectionCount": len(connections),
			"connectionIDs":   connections,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status":           "success",
		"onlineUsers":      onlineUsers,
		"onlineCount":      len(onlineUsers),
		"totalConnections": connectionStats["total_connections"],
		"userConnections":  connectionStats["user_connections"],
		"userDetails":      userDetails,
		"mode":             "debug-detailed",
	})
}

// triggerContextSynthesis 触发上下文合成验证
func (h *Handler) triggerContextSynthesis(sessionID, userID string, messages []map[string]interface{}) error {
	log.Printf("🧠 [上下文合成] 开始为会话 %s 触发上下文合成", sessionID)

	// 提取最后一条用户消息作为查询
	var userQuery string
	for i := len(messages) - 1; i >= 0; i-- {
		if role, ok := messages[i]["role"].(string); ok && role == "user" {
			if content, ok := messages[i]["content"].(string); ok {
				userQuery = content
				break
			}
		}
	}

	if userQuery == "" {
		log.Printf("⚠️ [上下文合成] 未找到用户查询，跳过上下文合成")
		return nil
	}

	log.Printf("🚀 [上下文合成] 开始真实的上下文合成流程...")
	log.Printf("📤 [上下文合成] 用户查询: %s", userQuery[:min(100, len(userQuery))])

	// 方案1: 如果有宽召回服务，使用真正的上下文合成
	if h.wideRecallService != nil {
		log.Printf("🎯 [上下文合成] 使用宽召回服务进行真实上下文合成")
		return h.executeRealContextSynthesis(sessionID, userID, userQuery)
	}

	// 方案2: 使用LLMDrivenContextService模拟上下文合成，但调用真实的LLM
	log.Printf("🎭 [上下文合成] 宽召回服务未初始化，使用LLMDrivenContextService模拟上下文合成")
	return h.executeMockContextSynthesis(sessionID, userID, userQuery)
}

// executeRealContextSynthesis 执行真实的上下文合成
func (h *Handler) executeRealContextSynthesis(sessionID, userID, userQuery string) error {
	// 构建上下文合成请求
	synthesisReq := &models.ContextSynthesisRequest{
		UserID:         userID,
		SessionID:      sessionID,
		WorkspaceID:    "/default/workspace",
		UserQuery:      userQuery,
		IntentAnalysis: nil,
		CurrentContext: nil,
		RetrievalResults: &models.RetrievalResults{
			TotalResults:     0,
			TimelineResults:  []models.TimelineResult{},
			KnowledgeResults: []models.KnowledgeResult{},
			VectorResults:    []models.VectorResult{},
		},
		SynthesisConfig: &models.SynthesisConfig{
			LLMTimeout:           60,
			MaxTokens:            8000,
			Temperature:          0.1,
			ConfidenceThreshold:  0.7,
			ConflictResolution:   "time_priority",
			InformationFusion:    "weighted_merge",
			QualityAssessment:    "comprehensive",
			UpdateThreshold:      0.4,
			PersistenceThreshold: 0.7,
		},
		RequestTime: time.Now(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	startTime := time.Now()
	synthesisResp, err := h.wideRecallService.ExecuteContextSynthesis(ctx, synthesisReq)
	processingTime := time.Since(startTime)

	if err != nil {
		log.Printf("❌ [上下文合成] 真实执行失败: %v", err)
		return err
	}

	log.Printf("✅ [上下文合成] 真实执行成功!")
	log.Printf("📊 [上下文合成] 处理时间: %dms", processingTime.Milliseconds())
	log.Printf("📊 [上下文合成] 评估结果: %+v", synthesisResp.EvaluationResult)
	log.Printf("📊 [上下文合成] 合成上下文是否为nil: %t", synthesisResp.SynthesizedContext == nil)

	return nil
}

// executeMockContextSynthesis 执行模拟的上下文合成
func (h *Handler) executeMockContextSynthesis(sessionID, userID, userQuery string) error {
	// 使用现有的LLMDrivenContextService进行上下文合成验证
	retrieveReq := models.RetrieveContextRequest{
		SessionID: sessionID,
		Query:     userQuery,
		Strategy:  "context_synthesis", // 使用上下文合成策略
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	startTime := time.Now()
	resp, err := h.contextService.RetrieveContext(ctx, retrieveReq)
	processingTime := time.Since(startTime)

	if err != nil {
		log.Printf("❌ [上下文合成] 模拟执行失败: %v", err)
		return err
	}

	log.Printf("✅ [上下文合成] 模拟执行成功!")
	log.Printf("📊 [上下文合成] 处理时间: %dms", processingTime.Milliseconds())
	log.Printf("📊 [上下文合成] 短期记忆长度: %d", len(resp.ShortTermMemory))
	log.Printf("📊 [上下文合成] 长期记忆长度: %d", len(resp.LongTermMemory))
	log.Printf("📊 [上下文合成] 相关知识长度: %d", len(resp.RelevantKnowledge))

	return nil
}

// RegisterFileRoutes 注册文件路由（包装器方法）
func (h *Handler) RegisterFileRoutes(router gin.IRouter) {
	RegisterFileRoutes(router)
}

// RegisterChatRoutes 注册聊天路由（包装器方法）
func (h *Handler) RegisterChatRoutes(router gin.IRouter) {
	RegisterChatRoutes(router)
}

// RegisterHealthRoutes 注册健康路由（包装器方法）
func (h *Handler) RegisterHealthRoutes(router gin.IRouter) {
	RegisterHealthRoutes(router)
}

// RegisterRoleRoutes 注册角色路由（包装器方法）
func (h *Handler) RegisterRoleRoutes(router gin.IRouter) {
	// 从router获取gin.Engine实例
	// 由于router是IRouter接口，我们需要传递合适的参数
	// 这里我们直接在protected路由组上注册，不需要公开路由
	RegisterRoleRoutes(nil, router)
}
