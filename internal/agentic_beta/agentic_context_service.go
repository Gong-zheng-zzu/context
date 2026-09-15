package agentic_beta

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/contextkeeper/service/internal/agentic_beta/components"
	"github.com/contextkeeper/service/internal/agentic_beta/config"
	"github.com/contextkeeper/service/internal/interfaces"
	"github.com/contextkeeper/service/internal/models"
	"github.com/contextkeeper/service/internal/services"
	"github.com/contextkeeper/service/internal/store"
)

// ============================================================================
// 🚀 Agentic上下文服务 - 完整智能上下文解决方案
// ============================================================================

// AgenticContextService Agentic智能上下文服务
// 🔥 重构：直接基于ContextService，集成智能查询优化和意图分析决策功能
type AgenticContextService struct {
	// 🏗️ 基础服务层 - 直接使用ContextService
	contextService *services.ContextService
	// retriever keeps the service testable and allows compatible adapters to be
	// used without manufacturing an incomplete *ContextService value.
	retriever interface {
		RetrieveContext(context.Context, models.RetrieveContextRequest) (models.ContextResponse, error)
	}

	// 🤖 Agentic组件（A→B→C）
	intentAnalyzer *components.BasicQueryIntentAnalyzer
	decisionCenter *components.BasicIntelligentDecisionCenter

	// 🔧 统一相似度服务 - 新增
	similarityService *services.UnifiedSimilarityService

	// ⚙️ 配置和状态
	enabled      bool
	smartEnabled bool // 🔥 新增：控制智能查询优化功能
	name         string
	version      string
	stats        *AgenticServiceStats
}

// AgenticServiceStats Agentic服务统计
type AgenticServiceStats struct {
	TotalRequests      int                        `json:"total_requests"`
	AgenticEnhanced    int                        `json:"agentic_enhanced"`
	SmartOptimized     int                        `json:"smart_optimized"` // 🔥 新增：智能优化统计
	IntentAnalysisTime time.Duration              `json:"intent_analysis_time"`
	DecisionMakingTime time.Duration              `json:"decision_making_time"`
	RetrievalTime      time.Duration              `json:"retrieval_time"`
	IntentDistribution map[string]int             `json:"intent_distribution"`
	DomainDistribution map[string]int             `json:"domain_distribution"`
	StrategyUsage      map[string]int             `json:"strategy_usage"`
	PerformanceHistory []AgenticPerformanceRecord `json:"performance_history"`
	LastUpdated        time.Time                  `json:"last_updated"`
}

// AgenticPerformanceRecord 性能记录
type AgenticPerformanceRecord struct {
	Timestamp      time.Time `json:"timestamp"`
	Query          string    `json:"query"`
	IntentType     string    `json:"intent_type"`
	Domain         string    `json:"domain"`
	ProcessingTime int64     `json:"processing_time_ns"`
	Success        bool      `json:"success"`
}

// NewAgenticContextService 创建Agentic上下文服务
// 🔥 重构：直接基于ContextService创建完整的智能上下文服务
func NewAgenticContextService(serviceInput interface{}) *AgenticContextService {
	var contextService *services.ContextService
	var retriever interface {
		RetrieveContext(context.Context, models.RetrieveContextRequest) (models.ContextResponse, error)
	}
	switch service := serviceInput.(type) {
	case *services.ContextService:
		contextService = service
		retriever = service
	case interface {
		GetContextService() *services.ContextService
	}:
		contextService = service.GetContextService()
		if contextService != nil {
			retriever = contextService
		} else if compatible, ok := serviceInput.(interface {
			RetrieveContext(context.Context, models.RetrieveContextRequest) (models.ContextResponse, error)
		}); ok {
			retriever = compatible
		}
	case interface {
		RetrieveContext(context.Context, models.RetrieveContextRequest) (models.ContextResponse, error)
	}:
		retriever = service
	default:
		panic("NewAgenticContextService requires a ContextService or compatible retriever")
	}
	// 🔍 创建意图分析器
	analyzer := components.NewBasicQueryIntentAnalyzer()

	// 🧠 创建决策中心
	decisionCenter := components.NewBasicIntelligentDecisionCenter()

	// 🔧 创建统一相似度服务 - 新增
	similarityConfig := &services.SimilarityConfig{
		DefaultStrategy:  "enhanced_local",
		FallbackStrategy: "basic_local",
		EnableFallback:   true,
		PerformanceTarget: services.PerformanceTarget{
			MaxLatency:    500 * time.Millisecond,
			MinAccuracy:   0.7,
			PreferOffline: true,
		},
	}
	similarityService := services.NewUnifiedSimilarityService(similarityConfig)

	// 🔥 修复：从配置文件读取查询重写器启用状态
	flagManager := config.NewFeatureFlagManager()
	agenticConfig := flagManager.GetConfig()
	queryRewriterEnabled := agenticConfig.Components.QueryRewriter.Enabled

	log.Printf("🔧 [配置加载] 查询重写器配置状态: %t", queryRewriterEnabled)

	service := &AgenticContextService{
		contextService:    contextService,
		retriever:         retriever,
		intentAnalyzer:    analyzer,
		decisionCenter:    decisionCenter,
		similarityService: similarityService, // 新增
		enabled:           true,
		smartEnabled:      queryRewriterEnabled, // 🔥 修复：从配置读取而非硬编码
		name:              "AgenticContextService",
		version:           "v2.1.0-unified-similarity", // 版本更新
		stats: &AgenticServiceStats{
			IntentDistribution: make(map[string]int),
			DomainDistribution: make(map[string]int),
			StrategyUsage:      make(map[string]int),
			PerformanceHistory: make([]AgenticPerformanceRecord, 0),
		},
	}

	// 启动决策中心
	ctx := context.Background()
	if err := service.decisionCenter.Start(ctx); err != nil {
		log.Printf("⚠️ 决策中心启动失败: %v，降级到普通模式", err)
		service.enabled = false
	}

	log.Printf("🚀 AgenticContextService v2.0 初始化完成")
	log.Printf("📋 完整智能功能:")
	log.Printf("  ✅ A-意图分析器 - 自动识别查询意图和领域")
	log.Printf("  ✅ B-智能决策中心 - 基于意图制定处理策略")
	log.Printf("  ✅ C-增强检索流程 - 领域特定的噪声过滤和语义增强")
	log.Printf("  🔧 智能查询优化 - 状态: %t (来源: config/agentic.json)", queryRewriterEnabled)
	log.Printf("  ✅ 🔧 统一相似度服务 - 4种策略：enhanced_local, basic_local, fastembed_local, huggingface_online")
	log.Printf("  ✅ 性能监控 - 完整的处理过程可观测性")
	log.Printf("  ✅ 直接架构 - 基于ContextService的完整解决方案")

	return service
}

func (acs *AgenticContextService) retrieveContext(ctx context.Context, req models.RetrieveContextRequest) (models.ContextResponse, error) {
	if acs.retriever == nil {
		return models.ContextResponse{}, fmt.Errorf("context retrieval service is not configured")
	}
	return acs.retriever.RetrieveContext(ctx, req)
}

// NewAgenticContextServiceFromSmart 从SmartContextService创建Agentic上下文服务
// 🔥 已废弃：直接使用NewAgenticContextService(contextService)替代
// func NewAgenticContextServiceFromSmart(smartService *services.SmartContextService) *AgenticContextService {
// 	contextService := smartService.GetContextService()
// 	return NewAgenticContextService(contextService)
// }

// ============================================================================
// 🎯 核心检索方法 - A→B→C流程集成 + 智能查询优化
// ============================================================================

// RetrieveContext 智能检索上下文 - 集成完整智能流程
func (acs *AgenticContextService) RetrieveContext(ctx context.Context, req models.RetrieveContextRequest) (models.ContextResponse, error) {
	startTime := time.Now()
	acs.stats.TotalRequests++

	// 记录原始查询
	originalQuery := req.Query
	log.Print("\n" + strings.Repeat("=", 100))
	log.Printf("【AgenticContextService】🚀 智能检索流程启动")
	log.Printf("【AgenticContextService】📝 原始查询: \"%s\"", originalQuery)
	log.Printf("【AgenticContextService】📅 开始时间: %s", startTime.Format("15:04:05.000"))
	log.Print(strings.Repeat("=", 100))

	// 如果Agentic功能禁用，直接使用基础ContextService
	if !acs.smartEnabled {
		log.Printf("【AgenticContextService】⚪ Agentic功能已禁用，降级到基础服务模式")
		return acs.retrieveContext(ctx, req)
	}

	// 如果查询为空，直接使用基础ContextService
	if strings.TrimSpace(originalQuery) == "" {
		log.Printf("【AgenticContextService】ℹ️ 查询为空，使用标准检索流程")
		return acs.retrieveContext(ctx, req)
	}

	acs.stats.AgenticEnhanced++

	// ==================== 🔍 阶段A：查询意图分析 ====================
	log.Print("\n" + strings.Repeat("-", 100))
	log.Printf("【AgenticContextService】【意图分析】🔵 进入黑盒")
	log.Printf("【AgenticContextService】【意图分析】📥 输入查询: \"%s\"", originalQuery)
	log.Printf("【AgenticContextService】【意图分析】⏰ 开始时间: %s", time.Now().Format("15:04:05.000"))

	intentStartTime := time.Now()
	intent, err := acs.intentAnalyzer.AnalyzeIntent(ctx, originalQuery)
	intentAnalysisTime := time.Since(intentStartTime)

	if err != nil {
		log.Printf("【AgenticContextService】【意图分析】❌ 黑盒异常: %v", err)
		log.Printf("【AgenticContextService】【意图分析】🔴 退出黑盒 - 降级到智能服务模式")
		// 🔥 修改：降级到智能查询优化模式
		return acs.smartRetrieveContext(ctx, req)
	}

	// 更新统计
	acs.stats.IntentAnalysisTime += intentAnalysisTime
	acs.stats.IntentDistribution[intent.IntentType]++
	acs.stats.DomainDistribution[intent.Domain]++

	log.Printf("【AgenticContextService】【意图分析】🟢 退出黑盒 - 分析成功")
	log.Printf("【AgenticContextService】【意图分析】📤 输出结果:")
	log.Printf("【AgenticContextService】【意图分析】   ├── 意图类型: %s", intent.IntentType)
	log.Printf("【AgenticContextService】【意图分析】   ├── 技术领域: %s", intent.Domain)
	log.Printf("【AgenticContextService】【意图分析】   ├── 复杂度: %.2f", intent.Complexity)
	log.Printf("【AgenticContextService】【意图分析】   ├── 置信度: %.2f", intent.Confidence)
	log.Printf("【AgenticContextService】【意图分析】   ├── 关键词: %v", intent.Keywords)
	log.Printf("【AgenticContextService】【意图分析】   └── 技术栈: %v", intent.TechStack)
	log.Printf("【AgenticContextService】【意图分析】⏱️ 耗时: %v", intentAnalysisTime)

	// ==================== 🧠 阶段B：智能决策制定 ====================
	log.Print("\n" + strings.Repeat("-", 100))
	log.Printf("【AgenticContextService】【智能决策】🔵 进入黑盒")
	log.Printf("【AgenticContextService】【智能决策】📥 输入意图:")
	log.Printf("【AgenticContextService】【智能决策】   ├── 类型: %s", intent.IntentType)
	log.Printf("【AgenticContextService】【智能决策】   ├── 领域: %s", intent.Domain)
	log.Printf("【AgenticContextService】【智能决策】   └── 复杂度: %.2f", intent.Complexity)
	log.Printf("【AgenticContextService】【智能决策】⏰ 开始时间: %s", time.Now().Format("15:04:05.000"))

	decisionStartTime := time.Now()
	decision, err := acs.decisionCenter.MakeDecision(ctx, intent)
	decisionMakingTime := time.Since(decisionStartTime)

	if err != nil {
		log.Printf("【AgenticContextService】【智能决策】❌ 黑盒异常: %v", err)
		log.Printf("【AgenticContextService】【智能决策】🔴 退出黑盒 - 使用默认检索策略")
		return acs.enhancedRetrieve(ctx, req, intent, nil)
	}

	// 更新策略使用统计
	acs.stats.DecisionMakingTime += decisionMakingTime
	for _, strategy := range decision.SelectedStrategies {
		acs.stats.StrategyUsage[strategy]++
	}

	log.Printf("【AgenticContextService】【智能决策】🟢 退出黑盒 - 决策制定完成")
	log.Printf("【AgenticContextService】【智能决策】📤 输出结果:")
	log.Printf("【AgenticContextService】【智能决策】   ├── 决策ID: %s", decision.DecisionID)
	log.Printf("【AgenticContextService】【智能决策】   ├── 任务数量: %d", len(decision.TaskPlan.Tasks))
	log.Printf("【AgenticContextService】【智能决策】   ├── 选择策略: %v", decision.SelectedStrategies)
	log.Printf("【AgenticContextService】【智能决策】   ├── 决策理由: %s", decision.DecisionReasoning)
	log.Printf("【AgenticContextService】【智能决策】   └── 置信度: %.2f", decision.Confidence)
	log.Printf("【AgenticContextService】【智能决策】⏱️ 耗时: %v", decisionMakingTime)

	// ==================== 🚀 阶段C：增强检索执行 ====================
	log.Print("\n" + strings.Repeat("-", 100))
	log.Printf("【AgenticContextService】【增强检索】🔵 进入黑盒")
	log.Printf("【AgenticContextService】【增强检索】📥 输入参数:")
	log.Printf("【AgenticContextService】【增强检索】   ├── 原始查询: \"%s\"", originalQuery)
	log.Printf("【AgenticContextService】【增强检索】   ├── 会话ID: %s", req.SessionID)
	log.Printf("【AgenticContextService】【增强检索】   ├── 意图类型: %s", intent.IntentType)
	log.Printf("【AgenticContextService】【增强检索】   └── 策略: %v", decision.SelectedStrategies)
	log.Printf("【AgenticContextService】【增强检索】⏰ 开始时间: %s", time.Now().Format("15:04:05.000"))

	retrievalStartTime := time.Now()
	response, err := acs.enhancedRetrieve(ctx, req, intent, decision)
	retrievalTime := time.Since(retrievalStartTime)

	if err != nil {
		log.Printf("【AgenticContextService】【增强检索】❌ 黑盒异常: %v", err)
		log.Printf("【AgenticContextService】【增强检索】🔴 退出黑盒 - 检索失败")
		return models.ContextResponse{}, err
	}

	acs.stats.RetrievalTime += retrievalTime

	log.Printf("【AgenticContextService】【增强检索】🟢 退出黑盒 - 检索完成")
	log.Printf("【AgenticContextService】【增强检索】📤 输出结果:")
	log.Printf("【AgenticContextService】【增强检索】   ├── 成功状态: %t", response.LongTermMemory != "" || response.ShortTermMemory != "" || response.SessionState != "")
	log.Printf("【AgenticContextService】【增强检索】   ├── 长期记忆条数: %d", len(strings.Split(response.LongTermMemory, "\n")))
	log.Printf("【AgenticContextService】【增强检索】   ├── 短期记忆条数: %d", len(strings.Split(response.ShortTermMemory, "\n")))
	log.Printf("【AgenticContextService】【增强检索】   └── 会话状态长度: %d字符", len(response.SessionState))
	log.Printf("【AgenticContextService】【增强检索】⏱️ 耗时: %v", retrievalTime)

	// ==================== 📊 流程总结 ====================
	totalTime := time.Since(startTime)

	log.Print("\n" + strings.Repeat("=", 100))
	log.Printf("【AgenticContextService】🎉 智能检索流程完成")
	log.Printf("【AgenticContextService】📊 性能统计:")
	log.Printf("【AgenticContextService】   ├── A阶段-意图分析: %v (%.1f%%)", intentAnalysisTime, float64(intentAnalysisTime)/float64(totalTime)*100)
	log.Printf("【AgenticContextService】   ├── B阶段-智能决策: %v (%.1f%%)", decisionMakingTime, float64(decisionMakingTime)/float64(totalTime)*100)
	log.Printf("【AgenticContextService】   ├── C阶段-增强检索: %v (%.1f%%)", retrievalTime, float64(retrievalTime)/float64(totalTime)*100)
	log.Printf("【AgenticContextService】   └── 总耗时: %v", totalTime)
	log.Printf("【AgenticContextService】✅ 返回最终结果")
	log.Print(strings.Repeat("=", 100))

	return response, nil
}

// ============================================================================
// 🧠 智能查询优化功能（从SmartContextService集成）
// ============================================================================

// smartRetrieveContext 智能查询优化检索（集成SmartContextService功能）
func (acs *AgenticContextService) smartRetrieveContext(ctx context.Context, req models.RetrieveContextRequest) (models.ContextResponse, error) {
	// 如果智能功能被禁用，直接调用原始方法
	if !acs.smartEnabled {
		log.Printf("【AgenticContextService】⚪ 智能优化功能已禁用，使用基础服务模式")
		return acs.retrieveContext(ctx, req)
	}

	acs.stats.SmartOptimized++

	// 记录原始查询
	originalQuery := req.Query

	// ==================== 🧠 智能查询优化模式 ====================
	log.Print("\n" + strings.Repeat("=", 100))
	log.Printf("【AgenticContextService】🧠 智能查询优化模式启动")
	log.Printf("【AgenticContextService】📝 原始查询: \"%s\"", originalQuery)
	log.Printf("【AgenticContextService】📅 开始时间: %s", time.Now().Format("15:04:05.000"))
	log.Print(strings.Repeat("=", 100))

	// ==================== 🔧 查询优化处理 ====================
	log.Print("\n" + strings.Repeat("-", 100))
	log.Printf("【AgenticContextService】【查询优化】🔵 进入黑盒")
	log.Printf("【AgenticContextService】【查询优化】📥 输入查询: \"%s\"", originalQuery)
	log.Printf("【AgenticContextService】【查询优化】⏰ 开始时间: %s", time.Now().Format("15:04:05.000"))

	startTime := time.Now()

	// 执行查询优化
	optimizedQuery := acs.optimizeQuery(originalQuery)

	optimizeTime := time.Since(startTime)

	log.Printf("【AgenticContextService】【查询优化】🟢 退出黑盒 - 优化完成")
	log.Printf("【AgenticContextService】【查询优化】📤 输出结果:")

	if optimizedQuery != originalQuery {
		log.Printf("【AgenticContextService】【查询优化】   ├── 优化状态: ✅ 已优化")
		log.Printf("【AgenticContextService】【查询优化】   ├── 原始查询: \"%s\"", originalQuery)
		log.Printf("【AgenticContextService】【查询优化】   └── 优化查询: \"%s\"", optimizedQuery)
		req.Query = optimizedQuery
	} else {
		log.Printf("【AgenticContextService】【查询优化】   ├── 优化状态: ⚪ 无需优化")
		log.Printf("【AgenticContextService】【查询优化】   └── 保持原样: \"%s\"", originalQuery)
	}
	log.Printf("【AgenticContextService】【查询优化】⏱️ 耗时: %v", optimizeTime)

	// 打印详细的查询改写对比日志
	acs.printSmartQueryRewriteComparison(originalQuery, optimizedQuery)

	// ==================== 🔍 基础检索执行 ====================
	log.Print("\n" + strings.Repeat("-", 100))
	log.Printf("【AgenticContextService】【基础检索】🔵 进入黑盒")
	log.Printf("【AgenticContextService】【基础检索】📥 输入参数:")
	log.Printf("【AgenticContextService】【基础检索】   ├── 查询内容: \"%s\"", req.Query)
	log.Printf("【AgenticContextService】【基础检索】   └── 会话ID: %s", req.SessionID)
	log.Printf("【AgenticContextService】【基础检索】⏰ 开始时间: %s", time.Now().Format("15:04:05.000"))

	retrievalStartTime := time.Now()

	// 调用原始检索方法
	response, err := acs.retrieveContext(ctx, req)

	retrievalTime := time.Since(retrievalStartTime)

	if err != nil {
		log.Printf("【AgenticContextService】【基础检索】❌ 黑盒异常: %v", err)
		log.Printf("【AgenticContextService】【基础检索】🔴 退出黑盒 - 检索失败")
		return response, err
	}

	log.Printf("【AgenticContextService】【基础检索】🟢 退出黑盒 - 检索完成")
	log.Printf("【AgenticContextService】【基础检索】📤 输出结果:")
	log.Printf("【AgenticContextService】【基础检索】   ├── 长期记忆条数: %d", len(strings.Split(response.LongTermMemory, "\\n")))
	log.Printf("【AgenticContextService】【基础检索】   ├── 短期记忆条数: %d", len(strings.Split(response.ShortTermMemory, "\\n")))
	log.Printf("【AgenticContextService】【基础检索】   └── 会话状态长度: %d字符", len(response.SessionState))
	log.Printf("【AgenticContextService】【基础检索】⏱️ 耗时: %v", retrievalTime)

	// 增强智能响应
	response = acs.enhanceSmartResponse(response, originalQuery, optimizedQuery)

	// ==================== 📊 智能优化总结 ====================
	totalTime := time.Since(startTime)

	log.Print("\n" + strings.Repeat("=", 100))
	log.Printf("【AgenticContextService】🎉 智能查询优化完成")
	log.Printf("【AgenticContextService】📊 性能统计:")
	log.Printf("【AgenticContextService】   ├── 查询优化: %v (%.1f%%)", optimizeTime, float64(optimizeTime)/float64(totalTime)*100)
	log.Printf("【AgenticContextService】   ├── 基础检索: %v (%.1f%%)", retrievalTime, float64(retrievalTime)/float64(totalTime)*100)
	log.Printf("【AgenticContextService】   └── 总耗时: %v", totalTime)
	log.Printf("【AgenticContextService】✅ 返回智能优化结果")
	log.Print(strings.Repeat("=", 100))

	return response, nil
}

// optimizeQuery 优化查询（从SmartContextService移植）
func (acs *AgenticContextService) optimizeQuery(query string) string {
	if !acs.smartEnabled {
		return query
	}

	optimized := query

	// 1. 去除噪声词汇
	optimized = acs.removeNoiseWords(optimized)

	// 2. 增强技术术语
	optimized = acs.enhanceTechnicalTerms(optimized)

	// 3. 丰富上下文
	optimized = acs.enrichContext(optimized)

	return strings.TrimSpace(optimized)
}

// enhanceTechnicalTerms 增强技术术语（从SmartContextService移植）
func (acs *AgenticContextService) enhanceTechnicalTerms(query string) string {
	// 检测代码相关关键词
	codeKeywords := map[string][]string{
		"Python": {"算法优化", "内存管理"},
		"性能":     {"优化", "调优"},
		"代码":     {"重构", "优化"},
		"数据库":    {"查询优化", "索引"},
		"API":    {"接口设计", "RESTful"},
	}

	result := query
	for keyword, enhancements := range codeKeywords {
		if strings.Contains(query, keyword) {
			for _, enhancement := range enhancements {
				if !strings.Contains(result, enhancement) {
					result += " " + enhancement
				}
			}
		}
	}

	return result
}

// enrichContext 丰富上下文（从SmartContextService移植）
func (acs *AgenticContextService) enrichContext(query string) string {
	// 基于查询内容添加相关概念
	contextMappings := map[string][]string{
		"性能优化":   {"并发编程", "缓存策略"},
		"算法":     {"数据结构优化"},
		"Python": {"pandas", "numpy"},
		"数据库":    {"SQL优化", "事务处理"},
	}

	result := query
	for pattern, contexts := range contextMappings {
		if strings.Contains(strings.ToLower(query), strings.ToLower(pattern)) {
			for _, context := range contexts {
				if !strings.Contains(result, context) {
					result += " " + context
				}
			}
		}
	}

	return result
}

// enhanceSmartResponse 增强智能响应（从SmartContextService移植）
func (acs *AgenticContextService) enhanceSmartResponse(response models.ContextResponse, originalQuery, optimizedQuery string) models.ContextResponse {
	// 这里可以添加响应增强逻辑，比如添加优化信息到响应中
	// 由于当前的ContextResponse结构限制，我们暂时保持原样
	return response
}

// printSmartQueryRewriteComparison 打印智能查询改写对比日志（从SmartContextService移植）
func (acs *AgenticContextService) printSmartQueryRewriteComparison(originalQuery, optimizedQuery string) {
	// ==================== 📊 查询改写分析 ====================
	log.Print("\n" + strings.Repeat("-", 100))
	log.Printf("【AgenticContextService】【查询分析】🔵 进入黑盒")
	log.Printf("【AgenticContextService】【查询分析】📥 输入参数:")
	log.Printf("【AgenticContextService】【查询分析】   ├── 原始查询: \"%s\"", originalQuery)
	log.Printf("【AgenticContextService】【查询分析】   └── 优化查询: \"%s\"", optimizedQuery)
	log.Printf("【AgenticContextService】【查询分析】⏰ 开始时间: %s", time.Now().Format("15:04:05.000"))

	// 1. 原始查询特征分析
	log.Printf("【AgenticContextService】【查询分析】🔍 原始查询特征:")
	log.Printf("【AgenticContextService】【查询分析】   ├── 字符数: %d", len(originalQuery))
	log.Printf("【AgenticContextService】【查询分析】   ├── 词汇数: %d", len(strings.Fields(originalQuery)))
	log.Printf("【AgenticContextService】【查询分析】   ├── 技术术语: %t", acs.containsTechnicalTerms(originalQuery))
	log.Printf("【AgenticContextService】【查询分析】   └── 包含问号: %t", strings.Contains(originalQuery, "？") || strings.Contains(originalQuery, "?"))

	// 2. 改写变化分析
	if originalQuery != optimizedQuery {
		changes := acs.analyzeSmartChanges(originalQuery, optimizedQuery)
		log.Printf("【AgenticContextService】【查询分析】🔄 智能改写步骤:")
		for i, change := range changes {
			log.Printf("【AgenticContextService】【查询分析】   %d. %s", i+1, change)
		}
		log.Printf("【AgenticContextService】【查询分析】   ✅ 查询已通过智能优化")
	} else {
		log.Printf("【AgenticContextService】【查询分析】   ⚪ 查询无需改写，保持原样")
	}

	// 3. 最终结果分析
	log.Printf("【AgenticContextService】【查询分析】🟢 退出黑盒 - 分析完成")
	log.Printf("【AgenticContextService】【查询分析】📤 输出结果:")
	log.Printf("【AgenticContextService】【查询分析】   ├── 最终查询: \"%s\"", optimizedQuery)
	log.Printf("【AgenticContextService】【查询分析】   ├── 最终长度: %d字符", len(optimizedQuery))
	log.Printf("【AgenticContextService】【查询分析】   └── 改写状态: %s",
		func() string {
			if originalQuery != optimizedQuery {
				return "✅ 已优化"
			}
			return "⚪ 无变化"
		}())
}

// analyzeSmartChanges 分析智能查询变化（从SmartContextService移植）
func (acs *AgenticContextService) analyzeSmartChanges(original, optimized string) []string {
	var changes []string

	if len(optimized) > len(original) {
		changes = append(changes, fmt.Sprintf("查询扩展: 从 %d 字符增加到 %d 字符", len(original), len(optimized)))
	}

	// 检测新增的关键词
	originalWords := strings.Fields(original)
	optimizedWords := strings.Fields(optimized)

	// 找出新增的词汇
	originalSet := make(map[string]bool)
	for _, word := range originalWords {
		originalSet[word] = true
	}

	var newWords []string
	for _, word := range optimizedWords {
		if !originalSet[word] {
			newWords = append(newWords, word)
		}
	}

	if len(newWords) > 0 {
		changes = append(changes, fmt.Sprintf("新增关键词: %s", strings.Join(newWords, ", ")))
	}

	// 检测去除的词汇
	optimizedSet := make(map[string]bool)
	for _, word := range optimizedWords {
		optimizedSet[word] = true
	}

	var removedWords []string
	for _, word := range originalWords {
		if !optimizedSet[word] {
			removedWords = append(removedWords, word)
		}
	}

	if len(removedWords) > 0 {
		changes = append(changes, fmt.Sprintf("去除噪声词: %s", strings.Join(removedWords, ", ")))
	}

	if len(changes) == 0 {
		changes = append(changes, "智能微调优化")
	}

	return changes
}

// ============================================================================
// 🚀 增强检索实现
// ============================================================================

// enhancedRetrieve 执行增强检索
func (acs *AgenticContextService) enhancedRetrieve(ctx context.Context, req models.RetrieveContextRequest, intent *interfaces.QueryIntent, decision *interfaces.ProcessingDecision) (models.ContextResponse, error) {
	// 根据意图和决策优化查询
	optimizedReq := acs.optimizeRequestByIntent(req, intent, decision)

	// 🔥 新增：打印详细的查询改写对比日志
	acs.printAgenticQueryRewriteComparison(req.Query, optimizedReq.Query, intent, decision)

	// 调用底层ContextService执行检索
	response, err := acs.retrieveContext(ctx, optimizedReq)
	if err != nil {
		return response, err
	}

	// 根据意图和决策增强响应
	enhancedResponse := acs.enhanceResponseByDecision(response, intent, decision)

	return enhancedResponse, nil
}

// optimizeRequestByIntent 根据意图优化请求
func (acs *AgenticContextService) optimizeRequestByIntent(req models.RetrieveContextRequest, intent *interfaces.QueryIntent, decision *interfaces.ProcessingDecision) models.RetrieveContextRequest {
	optimizedReq := req

	// 🔥 新增：0. 去除噪声词汇（从SmartContextService移植）
	optimizedReq.Query = acs.removeNoiseWords(req.Query)

	// 1. 基于领域的关键词增强
	domainEnhancements := acs.getDomainEnhancements(intent.Domain)
	if len(domainEnhancements) > 0 {
		optimizedReq.Query = optimizedReq.Query + " " + strings.Join(domainEnhancements, " ")
	}

	// 2. 基于技术栈的上下文丰富
	if len(intent.TechStack) > 0 {
		techContext := strings.Join(intent.TechStack, " ")
		optimizedReq.Query = optimizedReq.Query + " " + techContext
	}

	// 3. 基于复杂度调整检索参数
	if intent.Complexity > 0.7 {
		// 复杂查询需要更多上下文
		optimizedReq.Limit = int(float64(req.Limit) * 1.5)
		// 启用暴力搜索以获得更好的召回率
		optimizedReq.IsBruteSearch = 1
	} else if intent.Complexity < 0.3 {
		// 简单查询减少噪声
		optimizedReq.SkipThreshold = false
	}

	// 4. 基于决策任务优化搜索策略
	if decision != nil {
		for _, task := range decision.TaskPlan.Tasks {
			switch task.Type {
			case "enhance":
				// 语义增强任务：扩展查询词汇
				optimizedReq.Query = acs.expandQueryTerms(optimizedReq.Query, intent)
			case "filter":
				// 噪声过滤任务：启用更严格的相关性过滤
				optimizedReq.SkipThreshold = false
			case "adapt":
				// 领域适配任务：添加领域特定术语
				optimizedReq.Query = acs.adaptToDomain(optimizedReq.Query, intent.Domain)
			}
		}
	}

	// 清理查询字符串
	optimizedReq.Query = strings.TrimSpace(optimizedReq.Query)

	return optimizedReq
}

// enhanceResponseByDecision 根据决策增强响应
func (acs *AgenticContextService) enhanceResponseByDecision(response models.ContextResponse, intent *interfaces.QueryIntent, decision *interfaces.ProcessingDecision) models.ContextResponse {
	enhanced := response

	// 添加Agentic处理信息到会话状态
	agenticInfo := fmt.Sprintf("\n🤖 Agentic智能处理:")
	agenticInfo += fmt.Sprintf("\n  📊 意图: %s (%s领域)", intent.IntentType, intent.Domain)
	agenticInfo += fmt.Sprintf("\n  🔬 复杂度: %.2f", intent.Complexity)

	if decision != nil {
		agenticInfo += fmt.Sprintf("\n  🎯 策略: %s", strings.Join(decision.SelectedStrategies, ", "))
		agenticInfo += fmt.Sprintf("\n  💡 理由: %s", decision.DecisionReasoning)
	}

	enhanced.SessionState = response.SessionState + agenticInfo

	return enhanced
}

// ============================================================================
// 🛠️ 辅助方法
// ============================================================================

// getDomainEnhancements 获取领域增强词汇
func (acs *AgenticContextService) getDomainEnhancements(domain string) []string {
	domainKeywords := map[string][]string{
		"architecture": {"设计模式", "系统架构", "最佳实践", "可扩展性"},
		"database":     {"性能优化", "索引策略", "查询调优", "事务处理"},
		"frontend":     {"用户体验", "性能优化", "响应式设计", "组件化"},
		"backend":      {"API设计", "服务架构", "扩展性", "性能调优"},
		"devops":       {"自动化", "容器化", "CI/CD", "监控"},
		"programming":  {"代码质量", "算法优化", "最佳实践", "重构"},
	}

	if keywords, exists := domainKeywords[domain]; exists {
		return keywords[:2] // 限制数量避免过度增强
	}
	return []string{}
}

// expandQueryTerms 扩展查询词汇
func (acs *AgenticContextService) expandQueryTerms(query string, intent *interfaces.QueryIntent) string {
	// 基于意图类型添加相关术语
	switch intent.IntentType {
	case "debugging":
		return query + " 调试 问题排查 错误分析"
	case "procedural":
		return query + " 步骤 教程 操作指南"
	case "conceptual":
		return query + " 概念 原理 理论"
	case "technical":
		return query + " 实现 技术方案 代码"
	default:
		return query
	}
}

// adaptToDomain 领域适配
func (acs *AgenticContextService) adaptToDomain(query string, domain string) string {
	domainTerms := map[string]string{
		"architecture": " 架构设计 系统设计",
		"database":     " 数据库设计 SQL优化",
		"frontend":     " 前端开发 用户界面",
		"backend":      " 后端开发 服务端",
		"devops":       " 运维 部署 自动化",
		"programming":  " 编程 代码 算法",
	}

	if terms, exists := domainTerms[domain]; exists {
		return query + terms
	}
	return query
}

// recordPerformance 记录性能数据
func (acs *AgenticContextService) recordPerformance(query string, intent *interfaces.QueryIntent, duration time.Duration, success bool) {
	record := AgenticPerformanceRecord{
		Timestamp:      time.Now(),
		Query:          query,
		IntentType:     intent.IntentType,
		Domain:         intent.Domain,
		ProcessingTime: duration.Nanoseconds(),
		Success:        success,
	}

	// 保持性能历史记录数量限制
	maxHistory := 100
	acs.stats.PerformanceHistory = append(acs.stats.PerformanceHistory, record)
	if len(acs.stats.PerformanceHistory) > maxHistory {
		acs.stats.PerformanceHistory = acs.stats.PerformanceHistory[1:]
	}

	acs.stats.LastUpdated = time.Now()
}

// ============================================================================
// 📊 服务管理和统计方法
// ============================================================================

// GetStats 获取Agentic服务统计
func (acs *AgenticContextService) GetStats() *AgenticServiceStats {
	return acs.stats
}

// EnableAgentic 启用/禁用Agentic功能
func (acs *AgenticContextService) EnableAgentic(enabled bool) {
	acs.enabled = enabled
	if enabled {
		log.Printf("✅ AgenticContextService 智能功能已启用")
	} else {
		log.Printf("⚪ AgenticContextService 智能功能已禁用，降级到SmartContextService")
	}
}

// GetServiceInfo 获取服务信息
func (acs *AgenticContextService) GetServiceInfo() map[string]interface{} {
	analyzerStats := acs.intentAnalyzer.GetStats()
	decisionStats := acs.decisionCenter.GetStats()

	return map[string]interface{}{
		"name":                  acs.name,
		"version":               acs.version,
		"enabled":               acs.enabled,
		"agentic_stats":         acs.stats,
		"intent_analyzer_stats": analyzerStats,
		"decision_center_stats": decisionStats,
		"components": map[string]interface{}{
			"intent_analyzer": map[string]interface{}{
				"name":    "BasicQueryIntentAnalyzer",
				"enabled": true,
			},
			"decision_center": map[string]interface{}{
				"name":    "BasicIntelligentDecisionCenter",
				"enabled": true,
			},
		},
	}
}

// Stop 停止Agentic服务
func (acs *AgenticContextService) Stop(ctx context.Context) error {
	log.Printf("⏹️ 停止AgenticContextService...")

	// 停止决策中心
	if err := acs.decisionCenter.Stop(ctx); err != nil {
		log.Printf("⚠️ 停止决策中心失败: %v", err)
	}

	log.Printf("✅ AgenticContextService 已停止")
	return nil
}

// ============================================================================
// 🔄 代理方法 - 完全兼容ContextService接口
// ============================================================================

// 以下方法直接代理到ContextService，确保完全兼容

// RetrieveTodos 获取待办事项
func (acs *AgenticContextService) RetrieveTodos(ctx context.Context, req models.RetrieveTodosRequest) (*models.RetrieveTodosResponse, error) {
	return acs.contextService.RetrieveTodos(ctx, req)
}

// AssociateFile 关联文件
func (acs *AgenticContextService) AssociateFile(ctx context.Context, req models.AssociateFileRequest) error {
	return acs.contextService.AssociateFile(ctx, req)
}

// RecordEdit 记录编辑
func (acs *AgenticContextService) RecordEdit(ctx context.Context, req models.RecordEditRequest) error {
	return acs.contextService.RecordEdit(ctx, req)
}

// GetProgrammingContext 获取编程上下文
func (acs *AgenticContextService) GetProgrammingContext(ctx context.Context, sessionID string, query string) (*models.ProgrammingContext, error) {
	return acs.contextService.GetProgrammingContext(ctx, sessionID, query)
}

// StartSessionCleanupTask 启动会话清理任务
func (acs *AgenticContextService) StartSessionCleanupTask(ctx context.Context, timeout, interval time.Duration) {
	acs.contextService.StartSessionCleanupTask(ctx, timeout, interval)
}

// SummarizeToLongTermMemory 总结到长期记忆
func (acs *AgenticContextService) SummarizeToLongTermMemory(ctx context.Context, req models.SummarizeToLongTermRequest) (string, error) {
	return acs.contextService.SummarizeToLongTermMemory(ctx, req)
}

// StoreContext 存储上下文
func (acs *AgenticContextService) StoreContext(ctx context.Context, req models.StoreContextRequest) (string, error) {
	return acs.contextService.StoreContext(ctx, req)
}

// SummarizeContext 总结上下文
func (acs *AgenticContextService) SummarizeContext(ctx context.Context, req models.SummarizeContextRequest) (string, error) {
	return acs.contextService.SummarizeContext(ctx, req)
}

// StoreSessionMessages 存储会话消息
func (acs *AgenticContextService) StoreSessionMessages(ctx context.Context, req models.StoreMessagesRequest) (*models.StoreMessagesResponse, error) {
	return acs.contextService.StoreSessionMessages(ctx, req)
}

// RetrieveConversation 检索对话
func (acs *AgenticContextService) RetrieveConversation(ctx context.Context, req models.RetrieveConversationRequest) (*models.ConversationResponse, error) {
	return acs.contextService.RetrieveConversation(ctx, req)
}

// GetSessionState 获取会话状态
func (acs *AgenticContextService) GetSessionState(ctx context.Context, sessionID string) (*models.MCPSessionResponse, error) {
	return acs.contextService.GetSessionState(ctx, sessionID)
}

// SearchContext 搜索上下文
func (acs *AgenticContextService) SearchContext(ctx context.Context, sessionID, query string) ([]string, error) {
	return acs.contextService.SearchContext(ctx, sessionID, query)
}

// GetUserIDFromSessionID 从会话ID获取用户ID
func (acs *AgenticContextService) GetUserIDFromSessionID(sessionID string) (string, error) {
	return acs.contextService.GetUserIDFromSessionID(sessionID)
}

// GetUserSessionStore 获取用户会话存储
func (acs *AgenticContextService) GetUserSessionStore(userID string) (*store.SessionStore, error) {
	return acs.contextService.GetUserSessionStore(userID)
}

// SessionStore 获取会话存储
func (acs *AgenticContextService) SessionStore() *store.SessionStore {
	return acs.contextService.SessionStore()
}

// GetContextService 获取内部的ContextService实例
func (acs *AgenticContextService) GetContextService() *services.ContextService {
	return acs.contextService
}

// EnableSmart 启用/禁用智能功能 (代理到SmartContextService)
func (acs *AgenticContextService) EnableSmart(enabled bool) {
	acs.smartEnabled = enabled
	if enabled {
		log.Printf("✅ AgenticContextService 智能查询优化功能已启用")
	} else {
		log.Printf("⚪ AgenticContextService 智能查询优化功能已禁用")
	}
}

// ============================================================================
// 🔥 新增：从SmartContextService移植的功能
// ============================================================================

// removeNoiseWords 去除噪声词汇（从SmartContextService移植）
func (acs *AgenticContextService) removeNoiseWords(query string) string {
	noiseWords := []string{"请问", "帮我", "看看", "一下", "怎么样", "如何"}

	result := query
	for _, noise := range noiseWords {
		result = strings.ReplaceAll(result, noise, "")
	}

	// 清理多余空格
	result = regexp.MustCompile(`\s+`).ReplaceAllString(result, " ")
	return strings.TrimSpace(result)
}

// printAgenticQueryRewriteComparison 打印Agentic查询改写对比日志（增强版）
func (acs *AgenticContextService) printAgenticQueryRewriteComparison(originalQuery, optimizedQuery string, intent *interfaces.QueryIntent, decision *interfaces.ProcessingDecision) {
	log.Print("\n" + strings.Repeat("=", 80))
	log.Printf("🤖 AGENTIC QUERY REWRITE ANALYSIS - Agentic查询改写优化分析")
	log.Print(strings.Repeat("=", 80))

	// 1. 原始查询分析
	log.Printf("\n📝 1. ORIGINAL QUERY - 用户原始提问")
	log.Print(strings.Repeat("-", 80))
	log.Printf("原始查询: %s", originalQuery)
	log.Printf("查询长度: %d 字符", len(originalQuery))
	log.Printf("🔎 原始查询特征分析:")
	log.Printf("  - 字符数: %d", len(originalQuery))
	log.Printf("  - 词汇数: %d", len(strings.Fields(originalQuery)))
	log.Printf("  - 包含技术术语: %t", acs.containsTechnicalTerms(originalQuery))
	log.Printf("  - 包含问号: %t", strings.Contains(originalQuery, "？") || strings.Contains(originalQuery, "?"))

	// 2. Agentic意图分析结果
	log.Printf("\n🧠 2. AGENTIC INTENT ANALYSIS - Agentic意图分析结果")
	log.Print(strings.Repeat("-", 80))
	log.Printf("📊 意图类型: %s", intent.IntentType)
	log.Printf("🏗️ 技术领域: %s", intent.Domain)
	log.Printf("🔬 复杂度: %.2f", intent.Complexity)
	log.Printf("🎯 置信度: %.2f", intent.Confidence)
	log.Printf("🔑 关键词: %v", intent.Keywords)
	log.Printf("💻 技术栈: %v", intent.TechStack)

	// 3. 智能决策信息
	log.Printf("\n🎮 3. INTELLIGENT DECISION - 智能决策信息")
	log.Print(strings.Repeat("-", 80))
	if decision != nil {
		log.Printf("🎮 决策ID: %s", decision.DecisionID)
		log.Printf("📋 任务数量: %d", len(decision.TaskPlan.Tasks))
		log.Printf("🔧 选择策略: %v", decision.SelectedStrategies)
		log.Printf("💡 决策理由: %s", decision.DecisionReasoning)
		log.Printf("📊 决策置信度: %.2f", decision.Confidence)
	} else {
		log.Printf("⚪ 使用默认决策策略")
	}

	// 4. 改写过程详情
	log.Printf("\n⚙️ 4. AGENTIC REWRITE PROCESS - Agentic改写过程详情")
	log.Print(strings.Repeat("-", 80))

	if originalQuery != optimizedQuery {
		changes := acs.analyzeAgenticChanges(originalQuery, optimizedQuery, intent, decision)
		log.Printf("🔄 Agentic改写步骤:")
		for i, change := range changes {
			log.Printf("  %d. %s", i+1, change)
		}
		log.Printf("  ✅ 查询已通过Agentic智能优化")
	} else {
		log.Printf("  ⚪ 查询无需改写，保持原样")
	}

	// 5. 最终改写结果
	log.Printf("\n🎯 5. FINAL AGENTIC REWRITTEN QUERY - 最终Agentic改写结果")
	log.Print(strings.Repeat("-", 80))
	log.Printf("最终查询: %s", optimizedQuery)
	log.Printf("查询长度: %d 字符", len(optimizedQuery))

	// 6. 对比分析
	log.Printf("\n📊 6. AGENTIC COMPARISON ANALYSIS - Agentic对比分析")
	log.Print(strings.Repeat("-", 80))
	lengthChange := len(optimizedQuery) - len(originalQuery)
	if lengthChange > 0 {
		log.Printf("长度变化: +%d 字符 (智能扩展)", lengthChange)
	} else if lengthChange < 0 {
		log.Printf("长度变化: %d 字符 (智能压缩)", lengthChange)
	} else {
		log.Printf("长度变化: 无变化")
	}

	similarity := acs.calculateSemanticSimilarity(originalQuery, optimizedQuery)
	log.Printf("语义相似度: %.3f %s", similarity, acs.getSimilarityIndicator(similarity))

	if originalQuery != optimizedQuery {
		log.Printf("Agentic改写效果: ✅ 查询已智能优化")
	} else {
		log.Printf("Agentic改写效果: ⚪ 无需改写")
	}

	// 7. Agentic改写效果总结
	log.Printf("\n📋 7. AGENTIC REWRITE EFFECTIVENESS - Agentic改写效果总结")
	log.Print(strings.Repeat("-", 80))
	effectiveness := acs.evaluateAgenticRewriteEffectiveness(originalQuery, optimizedQuery, intent, decision)
	log.Printf("整体评价: %s", effectiveness.Overall)
	log.Printf("意图匹配: %s", effectiveness.IntentMatching)
	log.Printf("智能增强: %s", effectiveness.IntelligentEnhancement)
	log.Printf("检索效果: %s", effectiveness.RetrievalEffectiveness)

	if len(effectiveness.Suggestions) > 0 {
		log.Printf("\n💡 Agentic优化建议:")
		for _, suggestion := range effectiveness.Suggestions {
			log.Printf("  • %s", suggestion)
		}
	}

	log.Print("\n" + strings.Repeat("=", 80))
	log.Printf("🤖 AGENTIC QUERY REWRITE ANALYSIS COMPLETED - Agentic查询改写分析完成")
	log.Print(strings.Repeat("=", 80) + "\n")
}

// AgenticRewriteEffectiveness Agentic改写效果评估结果
type AgenticRewriteEffectiveness struct {
	Overall                string
	IntentMatching         string
	IntelligentEnhancement string
	RetrievalEffectiveness string
	Suggestions            []string
}

// analyzeAgenticChanges 分析Agentic查询变化
func (acs *AgenticContextService) analyzeAgenticChanges(original, optimized string, intent *interfaces.QueryIntent, decision *interfaces.ProcessingDecision) []string {
	var changes []string

	// 去除噪声词分析
	cleanedQuery := acs.removeNoiseWords(original)
	if cleanedQuery != original {
		removedWords := acs.findRemovedWords(original, cleanedQuery)
		changes = append(changes, fmt.Sprintf("智能噪声过滤: 去除噪声词 [%s]", strings.Join(removedWords, ", ")))
	}

	// 领域增强分析
	domainEnhancements := acs.getDomainEnhancements(intent.Domain)
	if len(domainEnhancements) > 0 {
		changes = append(changes, fmt.Sprintf("领域智能增强: 添加%s领域关键词 [%s]", intent.Domain, strings.Join(domainEnhancements, ", ")))
	}

	// 技术栈丰富分析
	if len(intent.TechStack) > 0 {
		changes = append(changes, fmt.Sprintf("技术栈上下文丰富: 添加技术栈信息 [%s]", strings.Join(intent.TechStack, ", ")))
	}

	// 意图驱动扩展分析
	if decision != nil {
		for _, task := range decision.TaskPlan.Tasks {
			switch task.Type {
			case "enhance":
				changes = append(changes, "意图驱动语义增强: 基于"+intent.IntentType+"意图扩展查询")
			case "filter":
				changes = append(changes, "智能噪声过滤: 启用严格相关性过滤")
			case "adapt":
				changes = append(changes, "领域智能适配: 添加"+intent.Domain+"领域特定术语")
			}
		}
	}

	if len(changes) == 0 {
		changes = append(changes, "Agentic智能微调优化")
	}

	return changes
}

// findRemovedWords 找出被去除的词汇
func (acs *AgenticContextService) findRemovedWords(original, cleaned string) []string {
	originalWords := strings.Fields(original)
	cleanedWords := strings.Fields(cleaned)

	cleanedSet := make(map[string]bool)
	for _, word := range cleanedWords {
		cleanedSet[word] = true
	}

	var removedWords []string
	for _, word := range originalWords {
		if !cleanedSet[word] {
			removedWords = append(removedWords, word)
		}
	}

	return removedWords
}

// containsTechnicalTerms 检测是否包含技术术语
func (acs *AgenticContextService) containsTechnicalTerms(query string) bool {
	technicalTerms := []string{
		"Python", "Java", "JavaScript", "Go", "Golang", "React", "Vue", "Angular",
		"代码", "性能", "优化", "API", "数据库", "算法", "架构", "设计模式",
		"SQL", "NoSQL", "Redis", "MongoDB", "MySQL", "PostgreSQL",
		"Docker", "Kubernetes", "微服务", "容器", "部署", "CI/CD",
		"前端", "后端", "全栈", "开发", "编程", "软件工程",
	}

	queryLower := strings.ToLower(query)
	for _, term := range technicalTerms {
		if strings.Contains(queryLower, strings.ToLower(term)) {
			return true
		}
	}
	return false
}

// calculateSemanticSimilarity 计算语义相似度
func (acs *AgenticContextService) calculateSemanticSimilarity(query1, query2 string) float64 {
	if query1 == query2 {
		return 1.0
	}

	// 🔧 使用统一相似度服务
	similarity, err := acs.similarityService.QuickSimilarity(query1, query2)
	if err != nil {
		log.Printf("⚠️ 统一相似度服务计算失败，降级到简单Jaccard算法: %v", err)
		return acs.fallbackJaccardSimilarity(query1, query2)
	}

	log.Printf("🔧 使用统一相似度服务计算: %.3f (query1='%s', query2='%s')",
		similarity, query1, query2)

	return similarity
}

// fallbackJaccardSimilarity 降级Jaccard相似度计算（作为兜底）
func (acs *AgenticContextService) fallbackJaccardSimilarity(query1, query2 string) float64 {
	// 修复的Jaccard相似度计算
	words1 := make(map[string]bool)
	for _, word := range strings.Fields(strings.ToLower(query1)) {
		words1[word] = true
	}

	words2 := make(map[string]bool)
	for _, word := range strings.Fields(strings.ToLower(query2)) {
		words2[word] = true
	}

	// 计算交集
	intersection := 0
	for word := range words1 {
		if words2[word] {
			intersection++
		}
	}

	// 计算并集 = |A| + |B| - |A ∩ B|
	union := len(words1) + len(words2) - intersection

	if union == 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
}

// getSimilarityIndicator 获取相似度指示器
func (acs *AgenticContextService) getSimilarityIndicator(similarity float64) string {
	if similarity >= 0.8 {
		return "✅ (高度相似)"
	} else if similarity >= 0.5 {
		return "⚪ (中等相似)"
	} else {
		return "⚠️ (低相似度)"
	}
}

// evaluateAgenticRewriteEffectiveness 评估Agentic改写效果
func (acs *AgenticContextService) evaluateAgenticRewriteEffectiveness(originalQuery, optimizedQuery string, intent *interfaces.QueryIntent, decision *interfaces.ProcessingDecision) AgenticRewriteEffectiveness {
	similarity := acs.calculateSemanticSimilarity(originalQuery, optimizedQuery)
	lengthIncrease := len(optimizedQuery) - len(originalQuery)

	var overall, intentMatching, intelligentEnhancement, retrievalEffectiveness string
	var suggestions []string

	// 整体评价 - 修复评价逻辑
	if originalQuery != optimizedQuery && lengthIncrease > 0 && intent.Confidence > 0.7 && similarity >= 0.7 {
		overall = "优秀 ✅ (Agentic智能优化效果显著)"
	} else if originalQuery != optimizedQuery && similarity >= 0.5 {
		overall = "良好 ⚪ (Agentic智能优化有效)"
	} else if originalQuery != optimizedQuery {
		overall = "一般 ⚪ (查询已改写但相似度较低)" // 修复：改写了但相似度低的情况
	} else {
		overall = "无变化 ⚪ (原查询已足够精确)"
	}

	// 意图匹配评价
	if intent.Confidence >= 0.8 {
		intentMatching = "优秀 ✅ (意图识别准确)"
	} else if intent.Confidence >= 0.5 {
		intentMatching = "良好 ⚪ (意图识别可靠)"
	} else {
		intentMatching = "需改进 ⚠️ (意图识别不确定)"
		suggestions = append(suggestions, "建议改进意图分析算法的准确性")
	}

	// 智能增强评价
	domainEnhancements := acs.getDomainEnhancements(intent.Domain)
	hasEnhancements := len(domainEnhancements) > 0 || len(intent.TechStack) > 0
	if hasEnhancements && lengthIncrease > 10 {
		intelligentEnhancement = "优秀 ✅ (智能增强丰富)"
	} else if hasEnhancements {
		intelligentEnhancement = "良好 ⚪ (智能增强适中)"
	} else {
		intelligentEnhancement = "基础 ⚪ (基础增强)"
	}

	// 检索效果评价
	if decision != nil && len(decision.SelectedStrategies) > 0 && intent.Complexity > 0.5 {
		retrievalEffectiveness = "优秀 ✅ (决策策略精准)"
	} else if decision != nil {
		retrievalEffectiveness = "良好 ⚪ (决策策略合理)"
	} else {
		retrievalEffectiveness = "标准 ⚪ (使用默认策略)"
	}

	// 生成建议
	if originalQuery != optimizedQuery {
		suggestions = append(suggestions, "Agentic智能优化成功增强了查询的精确性和相关性")
	}

	if intent.Confidence < 0.7 {
		suggestions = append(suggestions, "建议增加更多上下文信息以提高意图识别准确性")
	}

	if decision != nil && len(decision.TaskPlan.Tasks) > 2 {
		suggestions = append(suggestions, "多任务决策策略运行良好，检索效果预期优化")
	}

	return AgenticRewriteEffectiveness{
		Overall:                overall,
		IntentMatching:         intentMatching,
		IntelligentEnhancement: intelligentEnhancement,
		RetrievalEffectiveness: retrievalEffectiveness,
		Suggestions:            suggestions,
	}
}
