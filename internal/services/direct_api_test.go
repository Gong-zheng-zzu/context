package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// DeepSeekRequest DeepSeek API请求结构
type DeepSeekRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens"`
	Temperature float64   `json:"temperature"`
	TopP        float64   `json:"top_p"`
	Stream      bool      `json:"stream"`
}

// Message 消息结构
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// DeepSeekResponse DeepSeek API响应结构
type DeepSeekResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// TestDirectDeepSeekAPI 直接测试DeepSeek API
func TestDirectDeepSeekAPI(t *testing.T) {
	// 跳过测试，除非明确要求运行真实API测试
	if os.Getenv("RUN_REAL_API_TEST") != "true" {
		t.Skip("跳过真实API测试，设置 RUN_REAL_API_TEST=true 来运行")
	}

	log.Printf("🚀 [直接API测试] 开始测试真实的DeepSeek API")

	// API密钥
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		t.Skip("DEEPSEEK_API_KEY is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	// 测试DeepSeek-V3模型
	t.Run("DeepSeek-V3模型直接API测试", func(t *testing.T) {
		testDirectAPI(t, ctx, apiKey, "deepseek-chat", "DeepSeek-V3")
	})

	// 测试DeepSeek-R1模型
	t.Run("DeepSeek-R1模型直接API测试", func(t *testing.T) {
		testDirectAPI(t, ctx, apiKey, "deepseek-reasoner", "DeepSeek-R1")
	})
}

func testDirectAPI(t *testing.T, ctx context.Context, apiKey, model, modelName string) {
	log.Printf("🤖 [%s直接测试] 开始测试 %s 模型", modelName, model)

	// 构建复杂的UnifiedContextModel生成prompt
	prompt := buildComplexUnifiedContextPrompt()

	log.Printf("📤 [%s直接测试] 请求详情:", modelName)
	log.Printf("   🔗 API端点: https://api.deepseek.com/chat/completions")
	log.Printf("   🤖 模型: %s", model)
	log.Printf("   📝 Prompt长度: %d字符", len(prompt))
	log.Printf("   🎯 目标: 生成完整的UnifiedContextModel JSON")
	log.Printf("   ⚙️  参数: MaxTokens=8000, Temperature=0.1")

	// 显示prompt内容
	log.Printf("📄 [%s直接测试] 完整Prompt内容:", modelName)
	log.Printf("=== Prompt开始 ===")
	log.Printf("%s", prompt)
	log.Printf("=== Prompt结束 ===")

	// 构建请求
	request := DeepSeekRequest{
		Model: model,
		Messages: []Message{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		MaxTokens:   8000,
		Temperature: 0.1,
		TopP:        0.9,
		Stream:      false,
	}

	// 序列化请求
	requestBody, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("序列化请求失败: %v", err)
	}

	log.Printf("📤 [%s直接测试] 请求体长度: %d字节", modelName, len(requestBody))

	// 创建HTTP请求
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.deepseek.com/chat/completions", bytes.NewBuffer(requestBody))
	if err != nil {
		t.Fatalf("创建HTTP请求失败: %v", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	startTime := time.Now()

	// 发送请求
	log.Printf("⏳ [%s直接测试] 正在调用DeepSeek API...", modelName)
	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		duration := time.Since(startTime)
		log.Printf("❌ [%s直接测试] HTTP请求失败: %v", modelName, err)
		log.Printf("   ⏱️  耗时: %v", duration)
		t.Fatalf("%s HTTP请求失败: %v", modelName, err)
	}
	defer resp.Body.Close()

	duration := time.Since(startTime)

	// 读取响应
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("❌ [%s直接测试] 读取响应失败: %v", modelName, err)
		t.Fatalf("读取响应失败: %v", err)
	}

	log.Printf("📥 [%s直接测试] 收到HTTP响应:", modelName)
	log.Printf("   📊 HTTP状态码: %d", resp.StatusCode)
	log.Printf("   📊 响应体长度: %d字节", len(responseBody))
	log.Printf("   ⏱️  总耗时: %v", duration)

	// 显示完整的HTTP响应
	log.Printf("📄 [%s直接测试] 完整HTTP响应:", modelName)
	log.Printf("=== HTTP响应开始 ===")
	log.Printf("%s", string(responseBody))
	log.Printf("=== HTTP响应结束 ===")

	if resp.StatusCode != 200 {
		log.Printf("❌ [%s直接测试] API返回错误状态码: %d", modelName, resp.StatusCode)
		t.Errorf("%s API返回错误状态码: %d, 响应: %s", modelName, resp.StatusCode, string(responseBody))
		return
	}

	// 解析API响应
	var apiResponse DeepSeekResponse
	if err := json.Unmarshal(responseBody, &apiResponse); err != nil {
		log.Printf("❌ [%s直接测试] 解析API响应失败: %v", modelName, err)
		t.Errorf("解析API响应失败: %v", err)
		return
	}

	if len(apiResponse.Choices) == 0 {
		log.Printf("❌ [%s直接测试] API响应中没有choices", modelName)
		t.Errorf("API响应中没有choices")
		return
	}

	content := apiResponse.Choices[0].Message.Content

	log.Printf("✅ [%s直接测试] API调用成功!", modelName)
	log.Printf("📊 [%s直接测试] 响应统计:", modelName)
	log.Printf("   📝 生成内容长度: %d字符", len(content))
	log.Printf("   🔢 Token使用: Prompt=%d, Completion=%d, Total=%d",
		apiResponse.Usage.PromptTokens, apiResponse.Usage.CompletionTokens, apiResponse.Usage.TotalTokens)
	log.Printf("   🚀 生成速度: %.1f tokens/秒",
		float64(apiResponse.Usage.CompletionTokens)/duration.Seconds())

	// 显示生成的内容
	log.Printf("📄 [%s直接测试] 生成的内容:", modelName)
	log.Printf("=== 生成内容开始 ===")
	log.Printf("%s", content)
	log.Printf("=== 生成内容结束 ===")

	// JSON解析验证
	log.Printf("🔍 [%s直接测试] 开始JSON解析验证", modelName)

	// 清理内容
	cleanContent := strings.TrimSpace(content)
	if strings.HasPrefix(cleanContent, "```json") {
		cleanContent = strings.TrimPrefix(cleanContent, "```json")
	}
	if strings.HasSuffix(cleanContent, "```") {
		cleanContent = strings.TrimSuffix(cleanContent, "```")
	}
	cleanContent = strings.TrimSpace(cleanContent)

	log.Printf("🧹 [%s直接测试] 清理后内容长度: %d字符", modelName, len(cleanContent))

	// 验证JSON格式
	var genericJSON map[string]interface{}
	if err := json.Unmarshal([]byte(cleanContent), &genericJSON); err != nil {
		log.Printf("❌ [%s直接测试] JSON格式无效: %v", modelName, err)
		log.Printf("🔍 [%s直接测试] 清理后的内容:", modelName)
		log.Printf("%s", cleanContent)
		t.Errorf("%s生成的内容不是有效JSON: %v", modelName, err)
		return
	}

	log.Printf("✅ [%s直接测试] JSON格式有效!", modelName)
	log.Printf("📊 [%s直接测试] JSON结构分析:", modelName)
	log.Printf("   🔑 顶层字段数: %d", len(genericJSON))
	log.Printf("   🏷️  顶层字段: %v", getJSONKeys(genericJSON))

	// 分析JSON结构
	analyzeJSONStructureDetailed(genericJSON, modelName)

	// 尝试匹配UnifiedContextModel的关键字段
	matchScore := calculateUnifiedContextMatchScore(genericJSON)
	log.Printf("📊 [%s直接测试] UnifiedContextModel匹配度: %.1f%%", modelName, matchScore)

	// 最终结论
	log.Printf("🎯 [%s直接测试] 最终结论:", modelName)
	if matchScore > 80 {
		log.Printf("   ✅ %s能够生成高质量的复杂JSON结构", modelName)
		log.Printf("   ✅ 结构匹配度优秀: %.1f%%", matchScore)
		log.Printf("   ⏱️  生成时间: %v", duration)
		log.Printf("   📋 结论: 模型能力足够，问题可能在于prompt设计或结构定义")
	} else if matchScore > 50 {
		log.Printf("   ⚠️  %s能够生成中等复杂度的JSON结构", modelName)
		log.Printf("   ⚠️  结构匹配度中等: %.1f%%", matchScore)
		log.Printf("   📋 建议: 需要优化prompt或简化目标结构")
	} else {
		log.Printf("   ❌ %s难以生成复杂的JSON结构", modelName)
		log.Printf("   ❌ 结构匹配度较低: %.1f%%", matchScore)
		log.Printf("   📋 建议: 需要拆解任务或使用更强的模型")
	}
}

// buildComplexUnifiedContextPrompt 构建复杂的统一上下文prompt
func buildComplexUnifiedContextPrompt() string {
	return `请生成一个完整的UnifiedContextModel JSON结构，用于表示用户的上下文信息。

用户查询：我需要设计一个高并发的微服务架构，包括缓存层、数据库分片、消息队列和监控系统，请帮我分析技术选型和架构设计。

请严格按照以下JSON结构生成，所有字段都必须包含：

{
  "session_id": "session_12345",
  "user_id": "user_67890", 
  "workspace_id": "/workspace/microservice-project",
  "created_at": "2025-01-10T10:00:00Z",
  "updated_at": "2025-01-10T10:00:00Z",
  "current_topic": {
    "main_topic": "微服务架构设计",
    "topic_category": "technical",
    "user_intent": {
      "intent_type": "query",
      "intent_description": "寻求技术架构设计指导",
      "action_required": [{"action_type": "design", "description": "设计微服务架构", "priority": "high"}],
      "information_needed": [{"info_type": "technical", "description": "技术选型建议"}],
      "priority": "high"
    },
    "primary_pain_point": "需要设计高并发微服务架构",
    "secondary_pain_points": ["技术选型困难", "性能优化挑战"],
    "expected_outcome": "获得完整的架构设计方案",
    "key_concepts": [
      {"concept_name": "微服务", "concept_type": "technical", "definition": "独立部署的服务", "importance": 0.9},
      {"concept_name": "高并发", "concept_type": "technical", "definition": "处理大量并发请求", "importance": 0.8}
    ],
    "technical_terms": [
      {"term_name": "缓存层", "definition": "提高访问速度的临时存储", "category": "architecture"},
      {"term_name": "数据库分片", "definition": "水平分割数据库", "category": "database"}
    ],
    "business_terms": [
      {"term_name": "高可用", "definition": "系统持续可用", "domain": "运维"}
    ],
    "topic_evolution": [
      {"step_index": 1, "step_description": "需求分析", "timestamp": "2025-01-10T10:00:00Z"}
    ],
    "related_topics": [
      {"topic_name": "容器化", "relation_type": "related", "relevance_score": 0.8}
    ],
    "topic_start_time": "2025-01-10T10:00:00Z",
    "last_updated": "2025-01-10T10:00:00Z", 
    "update_count": 1,
    "confidence_level": 0.85
  },
  "project": {
    "project_name": "微服务架构项目",
    "project_path": "/workspace/microservice-project",
    "project_type": "backend",
    "description": "高并发微服务系统",
    "primary_language": "go",
    "current_phase": "planning",
    "confidence_level": 0.8
  },
  "code": {
    "session_id": "session_12345",
    "active_files": [
      {"file_path": "/src/main.go", "file_type": "go", "last_modified": "2025-01-10T10:00:00Z"}
    ],
    "recent_edits": [
      {"file_path": "/src/main.go", "edit_type": "create", "timestamp": "2025-01-10T10:00:00Z"}
    ],
    "focused_components": ["API网关", "服务注册"],
    "key_functions": [
      {"function_name": "main", "file_path": "/src/main.go", "description": "程序入口"}
    ],
    "important_types": [
      {"type_name": "Service", "file_path": "/src/service.go", "description": "服务接口"}
    ]
  },
  "recent_changes": {
    "time_range": {
      "start_time": "2025-01-09T10:00:00Z",
      "end_time": "2025-01-10T10:00:00Z"
    },
    "recent_commits": [
      {"commit_id": "abc123", "message": "初始化项目", "timestamp": "2025-01-10T10:00:00Z"}
    ],
    "modified_files": [
      {"file_path": "/src/main.go", "change_type": "created", "lines_changed": 50}
    ],
    "branch_activity": [
      {"branch_name": "main", "activity_type": "commit", "timestamp": "2025-01-10T10:00:00Z"}
    ],
    "new_features": [
      {"feature_name": "API网关", "description": "统一入口", "status": "planning"}
    ],
    "feature_updates": [
      {"feature_name": "服务发现", "update_description": "添加注册中心", "impact_level": "medium"}
    ],
    "bug_fixes": [
      {"bug_id": "bug001", "description": "修复连接问题", "severity": "low"}
    ],
    "completed_tasks": [
      {"task_id": "task001", "description": "项目初始化", "completion_time": "2025-01-10T10:00:00Z"}
    ],
    "ongoing_tasks": [
      {"task_id": "task002", "description": "架构设计", "progress": 0.3}
    ],
    "blocked_tasks": [
      {"task_id": "task003", "description": "技术选型", "blocker": "需要更多调研"}
    ]
  }
}

请直接输出完整的JSON，不要包含任何解释文字。确保所有字段都有合理的值，时间使用ISO 8601格式。`
}

// getJSONKeys 获取JSON对象的键
func getJSONKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// analyzeJSONStructureDetailed 详细分析JSON结构
func analyzeJSONStructureDetailed(data map[string]interface{}, modelName string) {
	log.Printf("🔍 [%s直接测试] JSON结构详细分析:", modelName)
	for key, value := range data {
		switch v := value.(type) {
		case map[string]interface{}:
			log.Printf("   📁 %s: 对象 (包含%d个字段)", key, len(v))
			// 递归分析嵌套对象的前几个字段
			if len(v) > 0 {
				count := 0
				for subKey := range v {
					if count < 3 { // 只显示前3个字段
						log.Printf("      └─ %s", subKey)
						count++
					} else {
						log.Printf("      └─ ... (还有%d个字段)", len(v)-3)
						break
					}
				}
			}
		case []interface{}:
			log.Printf("   📋 %s: 数组 (包含%d个元素)", key, len(v))
		case string:
			preview := v
			if len(preview) > 50 {
				preview = preview[:50] + "..."
			}
			log.Printf("   📝 %s: 字符串 \"%s\"", key, preview)
		case float64:
			log.Printf("   🔢 %s: 数字 (%.3f)", key, v)
		case bool:
			log.Printf("   ✅ %s: 布尔值 (%t)", key, v)
		default:
			log.Printf("   ❓ %s: 其他类型 (%T)", key, v)
		}
	}
}

// calculateUnifiedContextMatchScore 计算与UnifiedContextModel的匹配度
func calculateUnifiedContextMatchScore(data map[string]interface{}) float64 {
	expectedFields := []string{
		"session_id", "user_id", "workspace_id", "created_at", "updated_at",
		"current_topic", "project", "code", "recent_changes",
	}

	score := 0.0
	totalFields := float64(len(expectedFields))

	for _, field := range expectedFields {
		if _, exists := data[field]; exists {
			score += 1.0

			// 额外检查嵌套对象的完整性
			switch field {
			case "current_topic":
				if obj, ok := data[field].(map[string]interface{}); ok {
					nestedFields := []string{"main_topic", "topic_category", "user_intent", "primary_pain_point"}
					for _, nested := range nestedFields {
						if _, exists := obj[nested]; exists {
							score += 0.25 // 嵌套字段额外加分
						}
					}
				}
			case "project":
				if obj, ok := data[field].(map[string]interface{}); ok {
					nestedFields := []string{"project_name", "project_type", "primary_language"}
					for _, nested := range nestedFields {
						if _, exists := obj[nested]; exists {
							score += 0.2
						}
					}
				}
			}
		}
	}

	return (score / totalFields) * 100
}
