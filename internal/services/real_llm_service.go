package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/contextkeeper/service/internal/llm"
	"github.com/contextkeeper/service/internal/models"
)

// RealLLMService 真实的LLM服务实现
// 基于现有的LLM客户端基础设施，提供智能的意图分析和上下文合成功能
type RealLLMService struct {
	// LLM客户端
	llmClient llm.LLMClient

	// 配置
	provider    string
	model       string
	maxTokens   int
	temperature float64

	// 并发安全
	mutex sync.RWMutex

	// 性能监控
	requestCount    int64
	successCount    int64
	errorCount      int64
	totalLatency    time.Duration
	lastRequestTime time.Time
}

// NewRealLLMService 创建真实的LLM服务
func NewRealLLMService(provider, model, apiKey string) (*RealLLMService, error) {
	log.Printf("🤖 [真实LLM] 开始初始化真实LLM服务，提供商: %s, 模型: %s", provider, model)

	// 创建LLM配置
	config := &llm.LLMConfig{
		Provider:   llm.LLMProvider(provider),
		APIKey:     apiKey,
		Model:      model,
		MaxRetries: 3,
		Timeout:    180 * time.Second, // 🔥 增加到180秒
		RateLimit:  60,                // 每分钟60次请求
	}

	// 🆕 设置本地模型的特殊配置
	if provider == "ollama_local" {
		// 优先从环境变量读取，支持 Docker 环境
		ollamaHost := os.Getenv("OLLAMA_HOST")
		if ollamaHost == "" {
			ollamaHost = "http://localhost:11434" // 本地直接访问
		}
		config.BaseURL = ollamaHost
		config.RateLimit = 0 // 本地模型无限流限制
		config.APIKey = ""   // 本地模型不需要API密钥

		// 🔥 修复：从环境变量读取超时配置，支持视觉模型的长时间处理
		// 视觉模型（如 llava）处理图片通常需要 30-90 秒，首次加载可能需要更长时间
		timeoutSeconds := 300 // 默认 5 分钟，足够处理视觉模型
		if envTimeout := os.Getenv("LLM_TIMEOUT_SECONDS"); envTimeout != "" {
			if parsed, err := time.ParseDuration(envTimeout + "s"); err == nil {
				config.Timeout = parsed
			} else {
				config.Timeout = time.Duration(timeoutSeconds) * time.Second
			}
		} else {
			config.Timeout = time.Duration(timeoutSeconds) * time.Second
		}
		log.Printf("🔧 [Ollama配置] 超时时间: %v", config.Timeout)
	}

	// 设置全局配置
	llm.SetGlobalConfig(llm.LLMProvider(provider), config)

	// 创建LLM客户端
	client, err := llm.CreateGlobalClient(llm.LLMProvider(provider))
	if err != nil {
		return nil, fmt.Errorf("创建LLM客户端失败: %w", err)
	}

	// 验证客户端连接
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := client.HealthCheck(ctx); err != nil {
		log.Printf("⚠️ [真实LLM] LLM客户端健康检查失败: %v，但继续初始化", err)
	} else {
		log.Printf("✅ [真实LLM] LLM客户端健康检查通过")
	}

	service := &RealLLMService{
		llmClient:   client,
		provider:    provider,
		model:       model,
		maxTokens:   4000,
		temperature: 0.7,
	}

	log.Printf("✅ [真实LLM] 真实LLM服务初始化完成，提供商: %s, 模型: %s", provider, model)
	return service, nil
}

// AnalyzeUserIntent 分析用户意图
func (rls *RealLLMService) AnalyzeUserIntent(userQuery string) (*models.IntentAnalysisResult, error) {
	startTime := time.Now()
	log.Printf("🎯 [真实LLM] 开始分析用户意图，查询长度: %d", len(userQuery))

	rls.mutex.Lock()
	rls.requestCount++
	rls.lastRequestTime = startTime
	rls.mutex.Unlock()

	// 构建意图分析的系统提示词
	systemPrompt := `你是一个专业的意图分析专家。请分析用户查询的核心意图、领域上下文和应用场景。

请以JSON格式返回分析结果，包含以下字段：
{
  "core_intent": "核心意图（如：开发、修复、优化、分析、设计、测试、部署、学习、查询、配置）",
  "domain_context": "技术领域（如：Go语言、前端开发、后端开发、数据库、系统架构、DevOps、机器学习、网络安全、云计算、移动开发）",
  "scenario": "应用场景（如：项目开发、问题排查、性能优化、系统维护、学习研究、代码审查、测试验证、架构设计）",
  "multi_intents": ["如果存在多个意图，列出所有检测到的意图"],
  "confidence": 0.95
}

请确保返回有效的JSON格式。`

	userPrompt := fmt.Sprintf("请分析以下用户查询的意图：\n\n%s", userQuery)

	// 调用LLM
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	req := &llm.LLMRequest{
		SystemPrompt: systemPrompt,
		Prompt:       userPrompt,
		MaxTokens:    1000,
		Temperature:  0.3, // 较低温度确保稳定输出
		Format:       "json",
	}

	resp, err := rls.llmClient.Complete(ctx, req)
	if err != nil {
		rls.recordError()
		log.Printf("❌ [真实LLM] 意图分析LLM调用失败: %v", err)
		return nil, fmt.Errorf("LLM意图分析失败: %w", err)
	}

	// 解析LLM响应
	result, err := rls.parseIntentAnalysisResponse(resp.Content)
	if err != nil {
		rls.recordError()
		log.Printf("❌ [真实LLM] 意图分析响应解析失败: %v", err)
		return nil, fmt.Errorf("意图分析响应解析失败: %w", err)
	}

	// 记录成功
	rls.recordSuccess(time.Since(startTime))

	log.Printf("✅ [真实LLM] 意图分析完成，核心意图: %s, 领域: %s, 场景: %s, 耗时: %v",
		result.CoreIntentText, result.DomainContextText, result.ScenarioText, time.Since(startTime))

	return result, nil
}

// SynthesizeAndEvaluateContext 合成和评估上下文
func (rls *RealLLMService) SynthesizeAndEvaluateContext(
	userQuery string,
	currentContext *models.UnifiedContextModel,
	retrievalResults *models.ParallelRetrievalResult,
	intentAnalysis *models.IntentAnalysisResult,
) (*models.ContextSynthesisResult, error) {
	startTime := time.Now()
	log.Printf("🧠 [真实LLM] 开始上下文合成与评估，查询: %s", truncateString(userQuery, 50))

	rls.mutex.Lock()
	rls.requestCount++
	rls.lastRequestTime = startTime
	rls.mutex.Unlock()

	// 构建上下文合成的系统提示词
	systemPrompt := rls.buildContextSynthesisPrompt()

	// 构建用户提示词
	userPrompt := rls.buildContextSynthesisUserPrompt(userQuery, currentContext, retrievalResults, intentAnalysis)

	// 调用LLM
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	req := &llm.LLMRequest{
		SystemPrompt: systemPrompt,
		Prompt:       userPrompt,
		MaxTokens:    2000,
		Temperature:  0.5,
		Format:       "json",
	}

	resp, err := rls.llmClient.Complete(ctx, req)
	if err != nil {
		rls.recordError()
		log.Printf("❌ [真实LLM] 上下文合成LLM调用失败: %v", err)
		return nil, fmt.Errorf("LLM上下文合成失败: %w", err)
	}

	// 解析LLM响应
	result, err := rls.parseContextSynthesisResponse(resp.Content, currentContext, intentAnalysis)
	if err != nil {
		rls.recordError()
		log.Printf("❌ [真实LLM] 上下文合成响应解析失败: %v", err)
		return nil, fmt.Errorf("上下文合成响应解析失败: %w", err)
	}

	// 记录成功
	rls.recordSuccess(time.Since(startTime))

	log.Printf("✅ [真实LLM] 上下文合成完成，是否更新: %t, 置信度: %.2f, 耗时: %v",
		result.ShouldUpdate, result.UpdateConfidence, time.Since(startTime))

	return result, nil
}

// GenerateResponse 生成响应（实现LLMService接口）
func (rls *RealLLMService) GenerateResponse(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	startTime := time.Now()
	log.Printf("🤖 [真实LLM] GenerateResponse调用开始")

	// 详细记录入参
	log.Printf("📤 [LLM入参] ==================== 请求详情 ====================")
	log.Printf("📤 [LLM入参] Prompt长度: %d字符", len(req.Prompt))
	log.Printf("📤 [LLM入参] MaxTokens: %d", req.MaxTokens)
	log.Printf("📤 [LLM入参] Temperature: %.2f", req.Temperature)
	log.Printf("📤 [LLM入参] Format: %s", req.Format)
	log.Printf("📤 [LLM入参] 超时设置: %v", ctx.Value("timeout"))

	// 显示完整的Prompt内容
	log.Printf("📤 [LLM入参] 完整Prompt内容:")
	log.Printf("=== PROMPT开始 ===")
	log.Printf("%s", req.Prompt)
	log.Printf("=== PROMPT结束 ===")

	rls.mutex.Lock()
	rls.requestCount++
	requestID := rls.requestCount
	rls.mutex.Unlock()

	log.Printf("📤 [LLM入参] 请求ID: %d", requestID)

	// 构建LLM请求
	llmRequest := &llm.LLMRequest{
		Prompt:      req.Prompt,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Format:      req.Format,
	}

	log.Printf("⏳ [真实LLM] 开始调用DeepSeek API，请求ID: %d", requestID)

	// 调用LLM客户端
	response, err := rls.llmClient.Complete(ctx, llmRequest)
	duration := time.Since(startTime)

	if err != nil {
		log.Printf("❌ [LLM出参] ==================== 错误响应 ====================")
		log.Printf("❌ [LLM出参] 请求ID: %d", requestID)
		log.Printf("❌ [LLM出参] 错误信息: %v", err)
		log.Printf("❌ [LLM出参] 耗时: %v", duration)
		log.Printf("❌ [LLM出参] ================================================")

		rls.recordError()
		return nil, fmt.Errorf("LLM调用失败: %w", err)
	}

	// 详细记录出参
	log.Printf("📥 [LLM出参] ==================== 成功响应 ====================")
	log.Printf("📥 [LLM出参] 请求ID: %d", requestID)
	log.Printf("📥 [LLM出参] 响应长度: %d字符", len(response.Content))
	log.Printf("📥 [LLM出参] Token使用: %d", response.TokensUsed)
	log.Printf("📥 [LLM出参] 模型: %s", response.Model)
	log.Printf("📥 [LLM出参] 提供商: %s", response.Provider)
	log.Printf("📥 [LLM出参] 耗时: %v", duration)
	log.Printf("📥 [LLM出参] 生成速度: %.1f tokens/秒", float64(response.TokensUsed)/duration.Seconds())

	// 显示完整的响应内容
	log.Printf("📥 [LLM出参] 完整响应内容:")
	log.Printf("=== RESPONSE开始 ===")
	log.Printf("%s", response.Content)
	log.Printf("=== RESPONSE结束 ===")
	log.Printf("📥 [LLM出参] ================================================")

	rls.recordSuccess(duration)

	return &GenerateResponse{
		Content: response.Content,
		Usage: Usage{
			PromptTokens:     response.TokensUsed / 2, // 估算
			CompletionTokens: response.TokensUsed / 2, // 估算
			TotalTokens:      response.TokensUsed,
		},
	}, nil
}

// parseIntentAnalysisResponse 解析意图分析响应
func (rls *RealLLMService) parseIntentAnalysisResponse(content string) (*models.IntentAnalysisResult, error) {
	// 清理响应内容
	content = strings.TrimSpace(content)

	// 尝试提取JSON部分
	if strings.Contains(content, "```json") {
		start := strings.Index(content, "```json") + 7
		end := strings.Index(content[start:], "```")
		if end > 0 {
			content = content[start : start+end]
		}
	} else if strings.Contains(content, "```") {
		start := strings.Index(content, "```") + 3
		end := strings.Index(content[start:], "```")
		if end > 0 {
			content = content[start : start+end]
		}
	}

	content = strings.TrimSpace(content)

	// 解析JSON
	var response struct {
		CoreIntent    string   `json:"core_intent"`
		DomainContext string   `json:"domain_context"`
		Scenario      string   `json:"scenario"`
		MultiIntents  []string `json:"multi_intents"`
		Confidence    float64  `json:"confidence"`
	}

	if err := json.Unmarshal([]byte(content), &response); err != nil {
		log.Printf("⚠️ [真实LLM] JSON解析失败，尝试文本解析: %v", err)
		return rls.parseIntentAnalysisFromText(content)
	}

	// 构建结果
	result := &models.IntentAnalysisResult{
		CoreIntentText:       response.CoreIntent,
		DomainContextText:    response.DomainContext,
		ScenarioText:         response.Scenario,
		IntentCount:          len(response.MultiIntents),
		MultiIntentBreakdown: response.MultiIntents,
	}

	// 设置默认值
	if result.CoreIntentText == "" {
		result.CoreIntentText = "通用查询"
	}
	if result.DomainContextText == "" {
		result.DomainContextText = "通用技术"
	}
	if result.ScenarioText == "" {
		result.ScenarioText = "日常开发"
	}
	if len(result.MultiIntentBreakdown) == 0 {
		result.MultiIntentBreakdown = []string{result.CoreIntentText}
		result.IntentCount = 1
	}

	return result, nil
}

// parseIntentAnalysisFromText 从文本解析意图分析（备用方案）
func (rls *RealLLMService) parseIntentAnalysisFromText(content string) (*models.IntentAnalysisResult, error) {
	log.Printf("🔄 [真实LLM] 使用文本解析备用方案")

	// 简单的文本解析逻辑
	contentLower := strings.ToLower(content)

	// 意图关键词映射
	intentKeywords := map[string][]string{
		"开发": {"开发", "实现", "创建", "构建", "编写"},
		"修复": {"修复", "解决", "调试", "修改", "纠正"},
		"优化": {"优化", "改进", "提升", "增强", "完善"},
		"分析": {"分析", "研究", "调查", "检查", "评估"},
		"设计": {"设计", "规划", "架构", "建模", "构思"},
		"测试": {"测试", "验证", "检验", "校验", "确认"},
		"部署": {"部署", "发布", "上线", "安装", "配置"},
		"学习": {"学习", "了解", "掌握", "理解", "研习"},
		"查询": {"查询", "搜索", "查找", "获取", "检索"},
	}

	coreIntent := "通用查询"
	for intent, keywords := range intentKeywords {
		for _, keyword := range keywords {
			if strings.Contains(contentLower, keyword) {
				coreIntent = intent
				break
			}
		}
		if coreIntent != "通用查询" {
			break
		}
	}

	return &models.IntentAnalysisResult{
		CoreIntentText:       coreIntent,
		DomainContextText:    "通用技术",
		ScenarioText:         "日常开发",
		IntentCount:          1,
		MultiIntentBreakdown: []string{coreIntent},
	}, nil
}

// recordSuccess 记录成功请求
func (rls *RealLLMService) recordSuccess(latency time.Duration) {
	rls.mutex.Lock()
	defer rls.mutex.Unlock()

	rls.successCount++
	rls.totalLatency += latency
}

// recordError 记录错误请求
func (rls *RealLLMService) recordError() {
	rls.mutex.Lock()
	defer rls.mutex.Unlock()

	rls.errorCount++
}

// buildContextSynthesisPrompt 构建上下文合成的系统提示词
func (rls *RealLLMService) buildContextSynthesisPrompt() string {
	return `你是一个专业的上下文合成与评估专家。你的任务是分析用户查询、现有上下文和检索结果，决定是否需要更新上下文，并提供合成后的上下文。

请以JSON格式返回分析结果，包含以下字段：
{
  "should_update": true/false,
  "update_confidence": 0.85,
  "evaluation_reason": "详细的评估原因",
  "updated_context": {
    "core_concepts": ["核心概念1", "核心概念2"],
    "key_relationships": ["关系1", "关系2"],
    "important_details": ["重要细节1", "重要细节2"],
    "context_summary": "上下文总结"
  },
  "information_sources": {
    "timeline_contribution": 0.3,
    "knowledge_contribution": 0.3,
    "vector_contribution": 0.3,
    "context_contribution": 0.1
  },
  "semantic_changes": [
    {
      "type": "addition/modification/removal",
      "description": "变更描述",
      "impact_level": "high/medium/low"
    }
  ]
}

评估标准：
1. 如果用户查询与现有上下文高度相关且无新信息，should_update=false
2. 如果检索到新的相关信息或用户查询带来新的上下文，should_update=true
3. update_confidence基于信息的相关性和可靠性
4. 优先保留重要的历史上下文，同时融合新信息

请确保返回有效的JSON格式。`
}

// buildContextSynthesisUserPrompt 构建上下文合成的用户提示词
func (rls *RealLLMService) buildContextSynthesisUserPrompt(
	userQuery string,
	currentContext *models.UnifiedContextModel,
	retrievalResults *models.ParallelRetrievalResult,
	intentAnalysis *models.IntentAnalysisResult,
) string {
	prompt := fmt.Sprintf("用户查询：%s\n\n", userQuery)

	// 添加意图分析信息
	prompt += fmt.Sprintf("意图分析：\n- 核心意图：%s\n- 技术领域：%s\n- 应用场景：%s\n\n",
		intentAnalysis.CoreIntentText, intentAnalysis.DomainContextText, intentAnalysis.ScenarioText)

	// 添加现有上下文信息
	if currentContext != nil {
		prompt += "现有上下文：\n"
		if currentContext.CurrentTopic != nil {
			prompt += fmt.Sprintf("- 主要话题：%s\n", currentContext.CurrentTopic.MainTopic)
			prompt += fmt.Sprintf("- 用户意图：%s\n", currentContext.CurrentTopic.UserIntent)
			if currentContext.CurrentTopic.PrimaryPainPoint != "" {
				prompt += fmt.Sprintf("- 主要痛点：%s\n", currentContext.CurrentTopic.PrimaryPainPoint)
			}
			if len(currentContext.CurrentTopic.KeyConcepts) > 0 {
				concepts := make([]string, len(currentContext.CurrentTopic.KeyConcepts))
				for i, concept := range currentContext.CurrentTopic.KeyConcepts {
					concepts[i] = concept.ConceptName
				}
				prompt += fmt.Sprintf("- 关键概念：%v\n", concepts)
			}
		}
		if currentContext.Project != nil && currentContext.Project.ProjectName != "" {
			prompt += fmt.Sprintf("- 项目：%s\n", currentContext.Project.ProjectName)
		}
		prompt += "\n"
	} else {
		prompt += "现有上下文：无（首次创建）\n\n"
	}

	// 添加检索结果信息
	prompt += "检索结果：\n"
	if retrievalResults != nil {
		prompt += fmt.Sprintf("- 时间线数据：%d条记录\n", retrievalResults.TimelineCount)
		prompt += fmt.Sprintf("- 知识图谱：%d条记录\n", retrievalResults.KnowledgeCount)
		prompt += fmt.Sprintf("- 向量检索：%d条记录\n", retrievalResults.VectorCount)

		// 添加具体的检索内容（截断显示）
		if len(retrievalResults.TimelineResults) > 0 {
			prompt += "时间线数据示例：\n"
			for i, item := range retrievalResults.TimelineResults {
				if i >= 3 { // 最多显示3条
					break
				}
				prompt += fmt.Sprintf("  - %s\n", truncateString(fmt.Sprintf("%v", item), 100))
			}
		}

		if len(retrievalResults.KnowledgeResults) > 0 {
			prompt += "知识图谱示例：\n"
			for i, item := range retrievalResults.KnowledgeResults {
				if i >= 3 { // 最多显示3条
					break
				}
				prompt += fmt.Sprintf("  - %s\n", truncateString(fmt.Sprintf("%v", item), 100))
			}
		}

		if len(retrievalResults.VectorResults) > 0 {
			prompt += "向量检索示例：\n"
			for i, item := range retrievalResults.VectorResults {
				if i >= 3 { // 最多显示3条
					break
				}
				prompt += fmt.Sprintf("  - %s\n", truncateString(fmt.Sprintf("%v", item), 100))
			}
		}
	} else {
		prompt += "- 无检索结果\n"
	}

	prompt += "\n请基于以上信息进行上下文合成与评估。"

	return prompt
}

// parseContextSynthesisResponse 解析上下文合成响应
func (rls *RealLLMService) parseContextSynthesisResponse(
	content string,
	currentContext *models.UnifiedContextModel,
	intentAnalysis *models.IntentAnalysisResult,
) (*models.ContextSynthesisResult, error) {
	// 清理响应内容
	content = strings.TrimSpace(content)

	// 尝试提取JSON部分
	if strings.Contains(content, "```json") {
		start := strings.Index(content, "```json") + 7
		end := strings.Index(content[start:], "```")
		if end > 0 {
			content = content[start : start+end]
		}
	} else if strings.Contains(content, "```") {
		start := strings.Index(content, "```") + 3
		end := strings.Index(content[start:], "```")
		if end > 0 {
			content = content[start : start+end]
		}
	}

	content = strings.TrimSpace(content)

	// 解析JSON
	var response struct {
		ShouldUpdate     bool    `json:"should_update"`
		UpdateConfidence float64 `json:"update_confidence"`
		EvaluationReason string  `json:"evaluation_reason"`
		UpdatedContext   struct {
			CoreConcepts     []string `json:"core_concepts"`
			KeyRelationships []string `json:"key_relationships"`
			ImportantDetails []string `json:"important_details"`
			ContextSummary   string   `json:"context_summary"`
		} `json:"updated_context"`
		InformationSources struct {
			TimelineContribution  float64 `json:"timeline_contribution"`
			KnowledgeContribution float64 `json:"knowledge_contribution"`
			VectorContribution    float64 `json:"vector_contribution"`
			ContextContribution   float64 `json:"context_contribution"`
		} `json:"information_sources"`
		SemanticChanges []struct {
			Type        string `json:"type"`
			Description string `json:"description"`
			ImpactLevel string `json:"impact_level"`
		} `json:"semantic_changes"`
	}

	if err := json.Unmarshal([]byte(content), &response); err != nil {
		log.Printf("⚠️ [真实LLM] JSON解析失败，使用备用方案: %v", err)
		return rls.createFallbackSynthesisResult(currentContext, intentAnalysis)
	}

	// 构建更新后的上下文
	var updatedContext *models.UnifiedContextModel
	if response.ShouldUpdate {
		// 创建新的上下文模型
		updatedContext = &models.UnifiedContextModel{
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		// 设置会话ID
		if currentContext != nil {
			updatedContext.SessionID = currentContext.SessionID
		} else {
			updatedContext.SessionID = "unknown" // 这应该由调用方设置
		}

		// 设置当前主题上下文
		if len(response.UpdatedContext.CoreConcepts) > 0 || response.UpdatedContext.ContextSummary != "" {
			updatedContext.CurrentTopic = &models.TopicContext{
				MainTopic:        response.UpdatedContext.ContextSummary,
				PrimaryPainPoint: "基于LLM分析的上下文",
			}

			// 添加关键概念
			if len(response.UpdatedContext.CoreConcepts) > 0 {
				updatedContext.CurrentTopic.KeyConcepts = make([]models.ConceptInfo, len(response.UpdatedContext.CoreConcepts))
				for i, concept := range response.UpdatedContext.CoreConcepts {
					updatedContext.CurrentTopic.KeyConcepts[i] = models.ConceptInfo{
						ConceptName: concept,
						Definition:  "LLM分析得出的关键概念",
						Importance:  0.8,
					}
				}
			}
		}
	}

	// 构建语义变更
	var semanticChanges []models.SemanticChange
	for _, change := range response.SemanticChanges {
		semanticChanges = append(semanticChanges, models.SemanticChange{
			Dimension:      "topic", // 默认维度
			ChangeType:     change.Type,
			NewSemantic:    change.Description,
			ChangeStrength: 0.8, // 默认变化强度
		})
	}

	// 构建结果
	result := &models.ContextSynthesisResult{
		UpdatedContext:   updatedContext,
		ShouldUpdate:     response.ShouldUpdate,
		UpdateConfidence: response.UpdateConfidence,
		EvaluationReason: response.EvaluationReason,
		InformationSources: models.InformationSources{
			TimelineContribution:  response.InformationSources.TimelineContribution,
			KnowledgeContribution: response.InformationSources.KnowledgeContribution,
			VectorContribution:    response.InformationSources.VectorContribution,
			ContextContribution:   response.InformationSources.ContextContribution,
		},
		SemanticChanges: semanticChanges,
	}

	// 设置默认值
	if result.UpdateConfidence == 0 {
		result.UpdateConfidence = 0.8
	}
	if result.EvaluationReason == "" {
		if result.ShouldUpdate {
			result.EvaluationReason = "基于检索结果更新上下文"
		} else {
			result.EvaluationReason = fmt.Sprintf("当前上下文足够，置信度: %.2f", result.UpdateConfidence)
		}
	}

	return result, nil
}

// createFallbackSynthesisResult 创建备用合成结果
func (rls *RealLLMService) createFallbackSynthesisResult(
	currentContext *models.UnifiedContextModel,
	intentAnalysis *models.IntentAnalysisResult,
) (*models.ContextSynthesisResult, error) {
	log.Printf("🔄 [真实LLM] 使用备用合成方案")

	shouldUpdate := currentContext == nil // 如果没有现有上下文，则需要创建
	confidence := 0.7                     // 备用方案的置信度较低

	var updatedContext *models.UnifiedContextModel
	var reason string

	if shouldUpdate {
		// 创建基础上下文
		updatedContext = &models.UnifiedContextModel{
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		// 设置当前主题上下文
		updatedContext.CurrentTopic = &models.TopicContext{
			MainTopic:        fmt.Sprintf("用户在%s领域进行%s相关的%s", intentAnalysis.DomainContextText, intentAnalysis.CoreIntentText, intentAnalysis.ScenarioText),
			PrimaryPainPoint: fmt.Sprintf("用户意图: %s", intentAnalysis.CoreIntentText),
			KeyConcepts: []models.ConceptInfo{
				{
					ConceptName: intentAnalysis.CoreIntentText,
					Definition:  "用户核心意图",
					Importance:  0.9,
				},
				{
					ConceptName: intentAnalysis.DomainContextText,
					Definition:  "技术领域上下文",
					Importance:  0.8,
				},
			},
		}

		reason = "初始化上下文完成"
	} else {
		reason = fmt.Sprintf("无需更新上下文，置信度: %.2f", confidence)
	}

	return &models.ContextSynthesisResult{
		UpdatedContext:   updatedContext,
		ShouldUpdate:     shouldUpdate,
		UpdateConfidence: confidence,
		EvaluationReason: reason,
		InformationSources: models.InformationSources{
			TimelineContribution:  0.25,
			KnowledgeContribution: 0.25,
			VectorContribution:    0.25,
			ContextContribution:   0.25,
		},
		SemanticChanges: []models.SemanticChange{},
	}, nil
}
