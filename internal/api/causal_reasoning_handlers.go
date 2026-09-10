package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/contextkeeper/service/internal/engines/causal_reasoning"
	"github.com/contextkeeper/service/internal/engines/multi_dimensional_retrieval/knowledge"
	"github.com/contextkeeper/service/internal/llm"
	"github.com/contextkeeper/service/internal/security"
	"github.com/gin-gonic/gin"
)

// CausalReasoningHandler 因果推理API处理器
type CausalReasoningHandler struct {
	securityService *security.SecurityService
	extractor       *causal_reasoning.EntityExtractor
	inferenceEngine *causal_reasoning.InferenceEngine
	graphBuilder    *causal_reasoning.GraphBuilder
	graphWriter     causalGraphWriter
}

// causalGraphWriter keeps the persistence boundary testable without changing
// the graph implementation used by inference and statistics endpoints.
type causalGraphWriter interface {
	BuildCausalGraph(ctx context.Context, relations []causal_reasoning.CausalRelation) error
}

// NewCausalReasoningHandler 创建因果推理处理器
func NewCausalReasoningHandler(llmClient *llm.OllamaLocalClient, kgEngine *knowledge.Neo4jEngine, securityService *security.SecurityService) *CausalReasoningHandler {
	graphBuilder := causal_reasoning.NewGraphBuilder(kgEngine)
	return &CausalReasoningHandler{
		extractor:       causal_reasoning.NewEntityExtractor(llmClient),
		inferenceEngine: causal_reasoning.NewInferenceEngine(kgEngine),
		graphBuilder:    graphBuilder,
		graphWriter:     graphBuilder,
		securityService: securityService,
	}
}

// ExtractCausalRelations 抽取因果关系
// POST /api/v1/causal/extract
func (h *CausalReasoningHandler) ExtractCausalRelations(c *gin.Context) {
	var req causal_reasoning.ExtractRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数", "details": err.Error()})
		return
	}

	// 默认配置
	if req.MinConfidence == 0 {
		req.MinConfidence = 0.5
	}
	if !req.UseRules && !req.UsePMI && !req.UseLLM {
		req.UseLLM = true // 默认使用LLM
	}

	startTime := time.Now()
	securityExecution := &causal_reasoning.SecurityExecution{ASDFChecked: h.securityService != nil}
	if h.securityService != nil {
		normalizedText, _, _, _ := h.securityService.DefendAndNormalize(req.Text)
		securityExecution.InputNormalized = normalizedText != req.Text
		scanResult, err := h.securityService.ScanContent(c.Request.Context(), "causal-analysis", causalUserID(c), req.Text)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "causal input security check failed"})
			return
		}
		req.Text = scanResult.RedactedContent
		seen := make(map[string]struct{})
		for _, item := range scanResult.SensitiveInfos {
			typeName := string(item.Type)
			if _, duplicate := seen[typeName]; !duplicate {
				securityExecution.SensitiveTypes = append(securityExecution.SensitiveTypes, typeName)
				seen[typeName] = struct{}{}
			}
		}
	}

	// 抽取因果关系
	relations, err := h.extractor.Extract(c.Request.Context(), req.Text, req.UseRules, req.UsePMI, req.UseLLM)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "因果关系抽取失败", "details": err.Error()})
		return
	}

	// 过滤低置信度
	relations = h.extractor.FilterByConfidence(relations, req.MinConfidence)

	// The caller must explicitly request persistence. Analysis-only requests
	// remain side-effect free so they can be used safely in the live demo.
	graphPersisted := false
	if req.Persist && len(relations) > 0 {
		if h.graphWriter == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "因果图谱存储不可用"})
			return
		}
		if err := h.graphWriter.BuildCausalGraph(c.Request.Context(), relations); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "构建因果图谱失败", "details": err.Error()})
			return
		}
		graphPersisted = true
	}

	processTime := time.Since(startTime).Milliseconds()

	c.JSON(http.StatusOK, causal_reasoning.ExtractResponse{
		AnalysisOnly:      !graphPersisted,
		GraphPersisted:    graphPersisted,
		SecurityExecution: securityExecution,
		Relations:         relations,
		Count:             len(relations),
		ProcessTimeMs:     processTime,
	})
}

func causalUserID(c *gin.Context) string {
	if value, ok := c.Get("user_id"); ok {
		if userID, ok := value.(string); ok {
			return userID
		}
	}
	return ""
}

func causalResultLimit(raw string) (int, error) {
	if raw == "" {
		return 10, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > 100 {
		return 0, strconv.ErrSyntax
	}
	return limit, nil
}

// InferCausal 因果推理查询
// POST /api/v1/causal/infer
func (h *CausalReasoningHandler) InferCausal(c *gin.Context) {
	var req causal_reasoning.InferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数", "details": err.Error()})
		return
	}

	// 默认配置
	if req.Direction == "" {
		req.Direction = "forward"
	}
	if req.MaxDepth == 0 {
		req.MaxDepth = 4
	}
	if req.MinConfidence == 0 {
		req.MinConfidence = 0.5
	}
	if req.Limit == 0 {
		req.Limit = 10
	}

	startTime := time.Now()

	var chains []causal_reasoning.CausalChain
	var err error

	// 根据方向选择推理方法
	switch req.Direction {
	case "forward":
		chains, err = h.inferenceEngine.InferForward(c.Request.Context(), req.Query, req.MaxDepth, req.MinConfidence, req.Limit)
	case "backward":
		chains, err = h.inferenceEngine.InferBackward(c.Request.Context(), req.Query, req.MaxDepth, req.MinConfidence, req.Limit)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的推理方向", "details": "direction必须为forward或backward"})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "因果推理查询失败", "details": err.Error()})
		return
	}

	processTime := time.Since(startTime).Milliseconds()

	c.JSON(http.StatusOK, causal_reasoning.InferResponse{
		Query:         req.Query,
		Direction:     req.Direction,
		Chains:        chains,
		Count:         len(chains),
		ProcessTimeMs: processTime,
	})
}

// QueryCausalChain 查询因果链
// GET /api/v1/causal/chain
func (h *CausalReasoningHandler) QueryCausalChain(c *gin.Context) {
	from := c.Query("from")
	to := c.Query("to")

	if from == "" || to == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少必填参数", "details": "from和to参数不能为空"})
		return
	}

	var req causal_reasoning.ChainQueryRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数", "details": err.Error()})
		return
	}

	req.From = from
	req.To = to

	// 默认配置
	if req.MaxDepth == 0 {
		req.MaxDepth = 4
	}
	if req.MinConfidence == 0 {
		req.MinConfidence = 0.5
	}
	if req.Limit == 0 {
		req.Limit = 10
	}

	startTime := time.Now()

	// 查询因果链
	chain, err := h.inferenceEngine.QueryCausalChain(c.Request.Context(), req.From, req.To, req.MaxDepth, req.MinConfidence, req.Limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询因果链失败", "details": err.Error()})
		return
	}

	processTime := time.Since(startTime).Milliseconds()

	found := chain != nil && len(chain.Paths) > 0

	c.JSON(http.StatusOK, causal_reasoning.ChainQueryResponse{
		From:          req.From,
		To:            req.To,
		Chain:         chain,
		Found:         found,
		ProcessTimeMs: processTime,
	})
}

// GetGraphStats 获取因果图谱统计信息
// GET /api/v1/causal/stats
func (h *CausalReasoningHandler) GetGraphStats(c *gin.Context) {
	stats, err := h.graphBuilder.GetGraphStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取统计信息失败", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// GetRelatedCauses 获取相关原因
// GET /api/v1/causal/causes/:entity
func (h *CausalReasoningHandler) GetRelatedCauses(c *gin.Context) {
	entity := c.Param("entity")
	if entity == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少实体参数"})
		return
	}

	limit, err := causalResultLimit(c.Query("limit"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的limit参数", "details": "limit必须是1到100之间的整数"})
		return
	}

	causes, err := h.inferenceEngine.GetRelatedCauses(c.Request.Context(), entity, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询相关原因失败", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"entity": entity,
		"causes": causes,
		"count":  len(causes),
	})
}

// GetRelatedEffects 获取相关结果
// GET /api/v1/causal/effects/:entity
func (h *CausalReasoningHandler) GetRelatedEffects(c *gin.Context) {
	entity := c.Param("entity")
	if entity == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少实体参数"})
		return
	}

	limit, err := causalResultLimit(c.Query("limit"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的limit参数", "details": "limit必须是1到100之间的整数"})
		return
	}

	effects, err := h.inferenceEngine.GetRelatedEffects(c.Request.Context(), entity, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询相关结果失败", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"entity":  entity,
		"effects": effects,
		"count":   len(effects),
	})
}

// RegisterCausalReasoningRoutes 注册因果推理路由
func RegisterCausalReasoningRoutes(router *gin.RouterGroup, handler *CausalReasoningHandler) {
	causal := router.Group("/causal")
	{
		causal.POST("/extract", handler.ExtractCausalRelations)
		causal.POST("/infer", handler.InferCausal)
		causal.GET("/chain", handler.QueryCausalChain)
		causal.GET("/stats", handler.GetGraphStats)
		causal.GET("/causes/:entity", handler.GetRelatedCauses)
		causal.GET("/effects/:entity", handler.GetRelatedEffects)
	}
}
