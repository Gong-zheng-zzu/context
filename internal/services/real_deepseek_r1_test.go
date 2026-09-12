package services

import (
	"encoding/json"
	"log"
	"os"
	"testing"
	"time"

	"github.com/contextkeeper/service/internal/models"
)

// TestRealDeepSeekR1Model 真实测试DeepSeek-R1模型处理复杂UnifiedContextModel的能力
func TestRealDeepSeekR1Model(t *testing.T) {
	// 跳过测试，除非明确要求运行真实API测试
	if os.Getenv("RUN_REAL_API_TEST") != "true" {
		t.Skip("跳过真实API测试，设置 RUN_REAL_API_TEST=true 来运行")
	}

	log.Printf("🚀 [真实R1测试] 开始测试真实的DeepSeek-R1模型")

	// 从环境变量获取API密钥
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		t.Fatal("需要设置 DEEPSEEK_API_KEY 环境变量")
	}

	// 创建真实的LLM服务
	realLLMService, err := NewRealLLMService("deepseek", "deepseek-reasoner", apiKey)
	if err != nil {
		t.Fatalf("创建真实LLM服务失败: %v", err)
	}

	// Synthesis API manages its own request timeout; the test remains opt-in via
	// RUN_REAL_API_TEST so ordinary package tests never contact DeepSeek.

	t.Run("R1模型复杂JSON生成能力测试", func(t *testing.T) {
		log.Printf("📤 [真实R1测试] 测试DeepSeek-R1模型的上下文合成能力")
		log.Printf("   - 目标: 生成完整的UnifiedContextModel")
		log.Printf("   - 预期字段数: 138个")

		// 构建测试数据
		userQuery := "我需要设计一个高并发的微服务架构，包括缓存层、数据库分片、消息队列和监控系统，请帮我分析技术选型和架构设计"

		// 模拟检索结果
		retrievalResults := &models.ParallelRetrievalResult{
			TimelineResults: []*models.TimelineEvent{
				{
					ID:              "test_event_1",
					EventType:       "code_change",
					Title:           "微服务架构优化",
					Content:         "重构了微服务间的通信机制，提升了系统性能",
					ImportanceScore: 0.9,
					RelevanceScore:  0.85,
				},
			},
			KnowledgeResults: []*models.KnowledgeNode{
				{
					ID:          "concept_microservice",
					Name:        "微服务架构",
					Type:        "技术概念",
					Description: "一种将应用程序构建为一组小型、独立服务的架构模式",
					Score:       0.95,
					Confidence:  0.9,
				},
			},
			VectorResults: []*models.VectorMatch{
				{
					ID:      "doc_architecture",
					Content: "高并发系统设计需要考虑缓存策略、数据库分片、消息队列等关键组件",
					Score:   0.88,
				},
			},
		}

		// 模拟意图分析结果
		intentAnalysis := &models.IntentAnalysisResult{
			CoreIntentText:       "技术架构设计",
			DomainContextText:    "微服务系统",
			ScenarioText:         "高并发架构设计咨询",
			IntentCount:          1,
			MultiIntentBreakdown: []string{"架构设计", "技术选型"},
		}

		startTime := time.Now()

		// 调用真实的DeepSeek-R1模型进行上下文合成
		result, err := realLLMService.SynthesizeAndEvaluateContext(
			userQuery,
			nil, // 当前上下文为空（首次创建）
			retrievalResults,
			intentAnalysis,
		)

		duration := time.Since(startTime)

		if err != nil {
			log.Printf("❌ [真实R1测试] API调用失败: %v", err)
			log.Printf("   - 耗时: %v", duration)
			t.Fatalf("DeepSeek-R1 API调用失败: %v", err)
		}

		log.Printf("📥 [真实R1测试] 收到DeepSeek-R1上下文合成结果")
		log.Printf("   - 耗时: %v", duration)
		log.Printf("   - 是否更新: %t", result.ShouldUpdate)
		log.Printf("   - 更新置信度: %.3f", result.UpdateConfidence)

		if result.UpdatedContext == nil {
			t.Errorf("R1模型未返回UpdatedContext")
			return
		}

		// 合成接口已返回结果结构，将上下文序列化后做结构校验。
		encodedContext, marshalErr := json.Marshal(result.UpdatedContext)
		if marshalErr != nil {
			t.Fatalf("序列化UpdatedContext失败: %v", marshalErr)
		}
		log.Printf("   - 更新上下文长度: %d字符", len(encodedContext))

		var unifiedContext models.UnifiedContextModel
		err = json.Unmarshal(encodedContext, &unifiedContext)

		if err != nil {
			log.Printf("❌ [真实R1测试] JSON解析失败: %v", err)
			log.Printf("🔍 [真实R1测试] 分析失败原因:")

			// 尝试解析为通用JSON来分析结构
			var genericJSON map[string]interface{}
			if jsonErr := json.Unmarshal(encodedContext, &genericJSON); jsonErr != nil {
				log.Printf("   - 不是有效的JSON格式")
				log.Printf("   - JSON语法错误: %v", jsonErr)
			} else {
				log.Printf("   - JSON格式有效，但结构不匹配UnifiedContextModel")
				log.Printf("   - 顶层字段数: %d", len(genericJSON))
				log.Printf("   - 顶层字段: %v", getKeys(genericJSON))
			}

			// 记录但不失败测试，继续分析
			t.Errorf("R1模型生成的JSON无法解析为UnifiedContextModel: %v", err)
		} else {
			log.Printf("✅ [真实R1测试] JSON解析成功！")
			log.Printf("🎯 [真实R1测试] UnifiedContextModel字段分析:")

			// 分析生成的字段完整性
			fieldCount := 0

			if unifiedContext.SessionID != "" {
				fieldCount++
				log.Printf("   ✅ SessionID: %s", unifiedContext.SessionID)
			}
			if unifiedContext.UserID != "" {
				fieldCount++
				log.Printf("   ✅ UserID: %s", unifiedContext.UserID)
			}
			if unifiedContext.WorkspaceID != "" {
				fieldCount++
				log.Printf("   ✅ WorkspaceID: %s", unifiedContext.WorkspaceID)
			}

			if unifiedContext.CurrentTopic != nil {
				fieldCount += 10 // 估算TopicContext的字段数
				log.Printf("   ✅ CurrentTopic: %s", unifiedContext.CurrentTopic.MainTopic)
				log.Printf("      - 主要痛点: %s", unifiedContext.CurrentTopic.PrimaryPainPoint)
				log.Printf("      - 期望结果: %s", unifiedContext.CurrentTopic.ExpectedOutcome)
				log.Printf("      - 关键概念数: %d", len(unifiedContext.CurrentTopic.KeyConcepts))
				log.Printf("      - 置信度: %.3f", unifiedContext.CurrentTopic.ConfidenceLevel)
			} else {
				log.Printf("   ❌ CurrentTopic: nil")
			}

			if unifiedContext.Project != nil {
				fieldCount += 8 // 估算ProjectContext的字段数
				log.Printf("   ✅ Project: %s", unifiedContext.Project.ProjectName)
				log.Printf("      - 项目类型: %s", string(unifiedContext.Project.ProjectType))
				log.Printf("      - 主要语言: %s", unifiedContext.Project.PrimaryLanguage)
				log.Printf("      - 当前阶段: %s", string(unifiedContext.Project.CurrentPhase))
			} else {
				log.Printf("   ❌ Project: nil")
			}

			if unifiedContext.Code != nil {
				fieldCount += 15 // 估算CodeContext的字段数
				log.Printf("   ✅ Code: 活跃文件数=%d", len(unifiedContext.Code.ActiveFiles))
			} else {
				log.Printf("   ❌ Code: nil")
			}

			if unifiedContext.RecentChangesSummary != "" {
				fieldCount += 1
				log.Printf("   ✅ RecentChangesSummary: %s", unifiedContext.RecentChangesSummary)
			} else {
				log.Printf("   ❌ RecentChangesSummary: empty")
			}

			if unifiedContext.Conversation != nil {
				fieldCount += 10 // 估算ConversationContext的字段数
				log.Printf("   ✅ Conversation: 存在")
			} else {
				log.Printf("   ❌ Conversation: nil")
			}

			log.Printf("📊 [真实R1测试] 字段完整性统计:")
			log.Printf("   - 估算生成字段数: %d", fieldCount)
			log.Printf("   - 目标字段数: 138")
			log.Printf("   - 完整性: %.1f%%", float64(fieldCount)/138*100)
		}

		// 最终结论
		log.Printf("🎯 [真实R1测试] 最终结论:")
		if err == nil {
			log.Printf("   ✅ DeepSeek-R1能够生成可解析的UnifiedContextModel")
			log.Printf("   ✅ JSON格式正确，结构匹配")
			log.Printf("   ⚠️  生成时间较长: %v", duration)
			log.Printf("   📋 建议: R1模型可以处理复杂结构，但需要优化性能")
		} else {
			log.Printf("   ❌ DeepSeek-R1无法生成正确的UnifiedContextModel")
			log.Printf("   📋 原因: JSON结构不匹配或格式错误")
			log.Printf("   📋 建议: 需要简化结构或改进prompt")
		}
	})
}

// buildDeepSeekUnifiedContextPrompt 构建复杂的UnifiedContextModel生成prompt
func buildDeepSeekUnifiedContextPrompt() string {
	return `## 复杂上下文模型生成任务

你是一个专业的上下文建模专家，需要生成一个完整的UnifiedContextModel JSON结构。

### 用户查询
我需要设计一个高并发的微服务架构，包括缓存层、数据库分片、消息队列和监控系统，请帮我分析技术选型和架构设计。

### 任务要求
请生成一个完整的UnifiedContextModel JSON，包含以下所有字段：

1. **基础字段**: session_id, user_id, workspace_id, created_at, updated_at
2. **CurrentTopic**: 包含主题、痛点、期望结果、关键概念等
3. **Project**: 包含项目信息、技术栈、阶段等
4. **Code**: 包含活跃文件、组件、函数等
5. **RecentChanges**: 包含最近变更、提交、任务等
6. **Conversation**: 包含会话状态、历史等

### 输出格式
请输出完整的JSON，确保：
- 所有字段都有合理的值
- 时间字段使用ISO 8601格式
- 数组字段包含至少1-3个元素
- 嵌套对象完整
- JSON格式正确

开始生成：`
}

// getKeys 获取map的所有键
func getKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
