package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/contextkeeper/service/internal/engines/causal_reasoning"
	"github.com/contextkeeper/service/internal/engines/multi_dimensional_retrieval/knowledge"
	"github.com/contextkeeper/service/internal/llm"
	"github.com/contextkeeper/service/internal/security"
	"github.com/contextkeeper/service/internal/utils"
	"github.com/gin-gonic/gin"
)

// CausalReasoningHandler 因果推理API处理器
type CausalReasoningHandler struct {
	securityService *security.SecurityService
	extractor       *causal_reasoning.EntityExtractor
	inferenceEngine *causal_reasoning.InferenceEngine
	graphBuilder    *causal_reasoning.GraphBuilder
	graphWriter     causalGraphWriter
	reviewStore     CausalReviewStore
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
		reviewStore:     NewFileCausalReviewStore("./data/causal_reviews.jsonl"),
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
	traceID := utils.GetTraceIDFromGin(c)
	latency := make(map[string]int64, 4)
	securityStart := time.Now()
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
	latency["security"] = time.Since(securityStart).Milliseconds()

	// 抽取因果关系。该入口保留实际执行路径，避免客户端从关系内容猜测
	// 是模型、规则还是降级路径。
	extractionStart := time.Now()
	relations, execution, err := h.extractor.ExtractWithExecution(c.Request.Context(), req.Text, req.UseRules, req.UsePMI, req.UseLLM)
	latency["extract"] = time.Since(extractionStart).Milliseconds()
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, causal_reasoning.ErrLLMUnavailable) {
			status = http.StatusServiceUnavailable
		}
		quality := causalAuditQuality(relations)
		finalizeCausalExecution(execution, latency, time.Since(startTime).Milliseconds())
		response := causal_reasoning.ExtractResponse{
			TraceID: traceID, CalibrationVersion: "pccm-c-frozen-v1", Abstained: true, AbstainReason: "model_unavailable",
			AnalysisOnly:      true,
			SecurityExecution: securityExecution,
			Execution:         execution,
			Quality:           quality,
			Persistence: causal_reasoning.PersistenceExecution{
				Requested: req.Persist,
				Status:    "not_attempted",
			},
			Relations: []causal_reasoning.CausalRelation{},
			Error:     "因果关系抽取失败：真实模型不可用且没有启用规则回退",
		}
		finalizeCausalResponse(&response, startTime, "failed")
		c.JSON(status, response)
		return
	}

	// 过滤低置信度
	relations = h.extractor.FilterByConfidence(relations, req.MinConfidence)

	// The caller must explicitly request persistence. Analysis-only requests
	// remain side-effect free so they can be used safely in the live demo.
	graphPersisted := false
	persistenceStart := time.Now()
	persistence := causal_reasoning.PersistenceExecution{Requested: req.Persist, Status: "analysis_only"}
	if req.Persist && len(relations) > 0 {
		persistence.Attempted = true
		if h.graphWriter == nil {
			persistence.Status = "unavailable"
			latency["persistence"] = time.Since(persistenceStart).Milliseconds()
			processTime := time.Since(startTime).Milliseconds()
			finalizeCausalExecution(execution, latency, processTime)
			response := causal_reasoning.ExtractResponse{
				TraceID: traceID, CalibrationVersion: "pccm-c-frozen-v1", Candidates: relations,
				AnalysisOnly: true, SecurityExecution: securityExecution, Execution: execution,
				Quality: causalAuditQuality(relations), Persistence: persistence, Relations: relations, Count: len(relations), ProcessTimeMs: processTime,
				Error: "因果图谱存储不可用",
			}
			finalizeCausalResponse(&response, startTime, "failed")
			c.JSON(http.StatusServiceUnavailable, response)
			return
		}
		if err := h.graphWriter.BuildCausalGraph(c.Request.Context(), relations); err != nil {
			persistence.Status = "failed"
			latency["persistence"] = time.Since(persistenceStart).Milliseconds()
			processTime := time.Since(startTime).Milliseconds()
			finalizeCausalExecution(execution, latency, processTime)
			response := causal_reasoning.ExtractResponse{
				TraceID: traceID, CalibrationVersion: "pccm-c-frozen-v1", Candidates: relations,
				AnalysisOnly: true, SecurityExecution: securityExecution, Execution: execution,
				Quality: causalAuditQuality(relations), Persistence: persistence, Relations: relations, Count: len(relations), ProcessTimeMs: processTime,
				Error: "构建因果图谱失败",
			}
			finalizeCausalResponse(&response, startTime, "failed")
			c.JSON(http.StatusInternalServerError, response)
			return
		}
		graphPersisted = true
		persistence.Succeeded = true
		persistence.Status = "persisted"
	} else if req.Persist {
		persistence.Status = "no_relations"
	}
	latency["persistence"] = time.Since(persistenceStart).Milliseconds()

	processTime := time.Since(startTime).Milliseconds()
	quality := causalAuditQuality(relations)
	finalizeCausalExecution(execution, latency, processTime)

	response := causal_reasoning.ExtractResponse{
		TraceID:            traceID,
		CalibrationVersion: "pccm-c-frozen-v1",
		Candidates:         relations,
		Abstained:          len(relations) == 0,
		AbstainReason:      causalAbstainReason(relations),
		AnalysisOnly:       !graphPersisted,
		GraphPersisted:     graphPersisted,
		SecurityExecution:  securityExecution,
		Execution:          execution,
		Quality:            quality,
		Persistence:        persistence,
		Relations:          relations,
		Count:              len(relations),
		ProcessTimeMs:      processTime,
	}
	finalizeCausalResponse(&response, startTime, "completed")
	c.JSON(http.StatusOK, response)
}

func finalizeCausalResponse(response *causal_reasoning.ExtractResponse, startedAt time.Time, status string) {
	response.Stage = "causal_extract"
	response.Status = status
	response.PipelineVersion = "causal-evidence-v1"
	response.StartedAt = startedAt.UTC()
	response.CompletedAt = time.Now().UTC()
}

func causalAbstainReason(relations []causal_reasoning.CausalRelation) string {
	if len(relations) == 0 {
		return "no_supported_candidates"
	}
	return ""
}

func causalUserID(c *gin.Context) string {
	if value, ok := c.Get("user_id"); ok {
		if userID, ok := value.(string); ok {
			return userID
		}
	}
	return ""
}

// finalizeCausalExecution enriches the execution record at the HTTP boundary.
// The extractor owns model/fallback truth; the handler owns request timing.
func finalizeCausalExecution(execution *causal_reasoning.ExtractionExecution, latency map[string]int64, total int64) {
	if execution == nil {
		return
	}
	if latency == nil {
		latency = make(map[string]int64)
	}
	latency["total"] = total
	execution.LatencyBreakdownMs = latency
	if execution.ModelTier == "" {
		switch execution.Mode {
		case "llm":
			model := strings.ToLower(execution.Model)
			switch {
			case strings.Contains(model, "7b"):
				execution.ModelTier = "qwen7b_q4"
			case strings.Contains(model, "3b"):
				execution.ModelTier = "qwen3b"
			default:
				execution.ModelTier = "llm"
			}
		case "rules", "rules_fallback":
			execution.ModelTier = "rules"
		case "model_unavailable":
			execution.ModelTier = "unavailable"
		default:
			execution.ModelTier = execution.Mode
		}
	}
}

// causalAuditQuality aggregates relation-level quality without upgrading an
// incomplete relation. It is deliberately conservative for legacy extractors
// that do not yet provide RelationQuality.
func causalAuditQuality(relations []causal_reasoning.CausalRelation) *causal_reasoning.RelationQuality {
	quality := &causal_reasoning.RelationQuality{TupleValid: true, NegationChecked: true, TemporalConsistent: true}
	if len(relations) == 0 {
		quality.ReviewRequired = true
		quality.ConfidenceLevel = "insufficient_evidence"
		quality.ValidationErrors = []string{"no_relations"}
		return quality
	}
	var coverage float64
	for i := range relations {
		relation := &relations[i]
		complete := relation.Object != "" && relation.Mediator != "" && relation.Property != "" && relation.Result != ""
		if relation.Quality != nil {
			coverage += relation.Quality.EvidenceCoverage
			complete = complete && relation.Quality.TupleValid
			quality.NegationChecked = quality.NegationChecked && relation.Quality.NegationChecked
			quality.TemporalConsistent = quality.TemporalConsistent && relation.Quality.TemporalConsistent
			quality.ReviewRequired = quality.ReviewRequired || relation.Quality.ReviewRequired
			quality.ValidationErrors = appendUniqueStrings(quality.ValidationErrors, relation.Quality.ValidationErrors...)
		} else {
			coverage += relationEvidenceCoverage(*relation)
			quality.NegationChecked = false
			quality.TemporalConsistent = false
		}
		quality.TupleValid = quality.TupleValid && complete
		if !complete {
			quality.ValidationErrors = appendUniqueStrings(quality.ValidationErrors, "incomplete_tuple")
			quality.ReviewRequired = true
		}
	}
	quality.EvidenceCoverage = coverage / float64(len(relations))
	if quality.TupleValid && !quality.ReviewRequired && quality.NegationChecked && quality.TemporalConsistent {
		quality.ConfidenceLevel = "verified"
	} else if quality.EvidenceCoverage > 0 {
		quality.ConfidenceLevel = "needs_review"
	} else {
		quality.ConfidenceLevel = "insufficient_evidence"
		quality.ReviewRequired = true
	}
	return quality
}

func relationEvidenceCoverage(relation causal_reasoning.CausalRelation) float64 {
	if len(relation.Evidence) == 0 {
		return 0
	}
	joined := strings.ToLower(strings.Join(relation.Evidence, " "))
	supported := 0
	for _, field := range []string{relation.Object, relation.Mediator, relation.Property, relation.Result} {
		if field != "" && strings.Contains(joined, strings.ToLower(field)) {
			supported++
		}
	}
	return float64(supported) / 4
}

func appendUniqueStrings(dst []string, values ...string) []string {
	seen := make(map[string]struct{}, len(dst)+len(values))
	for _, value := range dst {
		seen[value] = struct{}{}
	}
	for _, value := range values {
		if value != "" {
			if _, ok := seen[value]; !ok {
				dst = append(dst, value)
				seen[value] = struct{}{}
			}
		}
	}
	return dst
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
		causal.POST("/reviews", handler.CreateCausalReview)
		causal.GET("/reviews", handler.ListCausalReviews)
	}
}
