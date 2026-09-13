//go:build ignore

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/contextkeeper/service/internal/llm"
)

// 独立测试专门KG的LLM调用性能
func main() {
	fmt.Println("🧪 [KG性能测试] 开始独立测试专门KG的LLM调用性能")

	// 测试用例内容（与业务链路中使用的相同）
	testContent := "连接池优化测试：Docker容器化部署，使用Kubernetes编排和Nginx负载均衡，API网关服务出现高延迟问题，P99延迟从50ms增加到500ms，吞吐量从5000QPS降到1000QPS，计划优化容器资源配置和网络策略"

	// 1. 测试专门KG的prompt
	fmt.Println("\n🔥 [测试1] 测试专门KG的LLM调用...")
	dedicatedKGDuration, dedicatedKGTokens, err := testDedicatedKGPrompt(testContent)
	if err != nil {
		log.Fatalf("专门KG测试失败: %v", err)
	}

	// 2. 测试简化版prompt（对比）
	fmt.Println("\n🔥 [测试2] 测试简化版prompt...")
	simplifiedDuration, simplifiedTokens, err := testSimplifiedPrompt(testContent)
	if err != nil {
		log.Fatalf("简化版测试失败: %v", err)
	}

	// 3. 结果对比
	fmt.Println("\n📊 [性能对比] 测试结果:")
	fmt.Printf("专门KG版本: 耗时 %v, Token使用 %d\n", dedicatedKGDuration, dedicatedKGTokens)
	fmt.Printf("简化版本:   耗时 %v, Token使用 %d\n", simplifiedDuration, simplifiedTokens)
	fmt.Printf("性能差异:   专门KG比简化版慢 %.1fx\n", float64(dedicatedKGDuration)/float64(simplifiedDuration))

	if dedicatedKGDuration > 30*time.Second {
		fmt.Printf("⚠️ [结论] 专门KG版本确实耗时过长 (>30秒)，建议使用简化版本\n")
	} else {
		fmt.Printf("✅ [结论] 专门KG版本性能可接受\n")
	}
}

// testDedicatedKGPrompt 测试专门KG的prompt（完全复制业务链路中的逻辑）
func testDedicatedKGPrompt(content string) (time.Duration, int, error) {
	startTime := time.Now()
	fmt.Printf("🕸️ [专门KG测试] 开始时间: %s\n", startTime.Format("15:04:05.000"))

	// 🔥 完全复制业务链路中的专门KG prompt
	prompt := buildDedicatedKGPrompt("test-session", content)
	fmt.Printf("📝 [专门KG测试] Prompt长度: %d 字符\n", len(prompt))

	// 创建LLM客户端（复制业务逻辑）
	client, err := createTestLLMClient()
	if err != nil {
		return 0, 0, fmt.Errorf("创建LLM客户端失败: %w", err)
	}

	// 构建LLM请求（完全复制业务逻辑）
	llmRequest := &llm.LLMRequest{
		Prompt:      prompt,
		MaxTokens:   3000,
		Temperature: 0.1,
		Format:      "json",
		Model:       "deepseek-chat",
		Metadata: map[string]interface{}{
			"task":            "dedicated_knowledge_graph_extraction",
			"session_id":      "test-session",
			"content_length":  len(content),
			"skip_rate_limit": true,
			"parallel_call":   true,
		},
	}

	// 调用LLM API
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	apiCallStart := time.Now()
	fmt.Printf("🚀 [专门KG测试] 开始LLM API调用: %s\n", apiCallStart.Format("15:04:05.000"))

	llmResponse, err := client.Complete(ctx, llmRequest)

	apiCallEnd := time.Now()
	apiCallDuration := apiCallEnd.Sub(apiCallStart)

	if err != nil {
		return apiCallDuration, 0, fmt.Errorf("LLM API调用失败: %w", err)
	}

	totalDuration := time.Since(startTime)
	fmt.Printf("✅ [专门KG测试] 完成时间: %s, API耗时: %v, 总耗时: %v\n",
		apiCallEnd.Format("15:04:05.000"), apiCallDuration, totalDuration)
	fmt.Printf("📊 [专门KG测试] Token使用: %d, 响应长度: %d\n",
		llmResponse.TokensUsed, len(llmResponse.Content))

	return totalDuration, llmResponse.TokensUsed, nil
}

// testSimplifiedPrompt 测试简化版prompt
func testSimplifiedPrompt(content string) (time.Duration, int, error) {
	startTime := time.Now()
	fmt.Printf("💡 [简化测试] 开始时间: %s\n", startTime.Format("15:04:05.000"))

	// 简化版prompt（只要求基本的实体和关系）
	prompt := buildSimplifiedPrompt(content)
	fmt.Printf("📝 [简化测试] Prompt长度: %d 字符\n", len(prompt))

	// 创建LLM客户端
	client, err := createTestLLMClient()
	if err != nil {
		return 0, 0, fmt.Errorf("创建LLM客户端失败: %w", err)
	}

	// 构建简化的LLM请求
	llmRequest := &llm.LLMRequest{
		Prompt:      prompt,
		MaxTokens:   1500, // 更少的token
		Temperature: 0.1,
		Format:      "json",
		Model:       "deepseek-chat",
		Metadata: map[string]interface{}{
			"task":            "simplified_knowledge_extraction",
			"skip_rate_limit": true,
		},
	}

	// 调用LLM API
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	apiCallStart := time.Now()
	fmt.Printf("🚀 [简化测试] 开始LLM API调用: %s\n", apiCallStart.Format("15:04:05.000"))

	llmResponse, err := client.Complete(ctx, llmRequest)

	apiCallEnd := time.Now()
	apiCallDuration := apiCallEnd.Sub(apiCallStart)

	if err != nil {
		return apiCallDuration, 0, fmt.Errorf("LLM API调用失败: %w", err)
	}

	totalDuration := time.Since(startTime)
	fmt.Printf("✅ [简化测试] 完成时间: %s, API耗时: %v, 总耗时: %v\n",
		apiCallEnd.Format("15:04:05.000"), apiCallDuration, totalDuration)
	fmt.Printf("📊 [简化测试] Token使用: %d, 响应长度: %d\n",
		llmResponse.TokensUsed, len(llmResponse.Content))

	return totalDuration, llmResponse.TokensUsed, nil
}

// buildDedicatedKGPrompt 构建专门KG的prompt（完全复制业务逻辑）
func buildDedicatedKGPrompt(sessionID, content string) string {
	return fmt.Sprintf(`你是专业的知识图谱构建专家，专门从技术文档和对话中抽取实体和关系。

## 🎯 核心任务
从用户内容中构建高质量的知识图谱，提取实体和关系信息。

## 📊 实体抽取标准（6种通用类型）

### 1. Technical（技术实体）
- 编程语言: Go, Python, Java, JavaScript, C++
- 框架工具: Spring Boot, React, Vue, Docker, Kubernetes
- 数据库: MySQL, Redis, PostgreSQL, Neo4j, MongoDB
- 技术产品: Context-Keeper, 微服务系统, API网关

### 2. Project（项目工作）
- 项目: 电商系统开发, 性能优化项目, 架构重构
- 功能: 订单支付模块, 用户管理功能, 数据分析
- 任务: 数据库优化, 接口开发, 性能调优

### 3. Concept（技术概念）
- 架构概念: 微服务架构, 分层设计, 事件驱动
- 技术概念: 并发处理, 缓存策略, 负载均衡
- 设计模式: 单例模式, 工厂模式, 观察者模式

### 4. Issue（事件问题）
- 技术问题: 性能瓶颈, 内存泄漏, 并发问题
- 系统事件: 服务故障, 数据丢失, 网络中断
- 优化事件: 性能优化, 架构升级, 代码重构

### 5. Data（数据资源）
- 性能数据: 72秒, 1000TPS, 15%%失败率, 99.9%%可用性
- 配置参数: 超时时间, 连接池大小, 缓存大小
- 版本信息: v1.0.0, 2025-08-20, 第一阶段

### 6. Process（操作流程）
- 技术操作: 数据库查询, API调用, 缓存更新
- 部署操作: 服务部署, 配置更新, 环境切换
- 开发流程: 代码审查, 测试执行, 持续集成

## 🔗 关系抽取标准（5种核心关系）

### 1. USES（使用关系）
- 技术栈: Context-Keeper USES Neo4j
- 工具链: 项目 USES Spring Boot

### 2. SOLVES（解决关系）
- 问题解决: 性能优化 SOLVES 响应慢
- 技术解决: 缓存策略 SOLVES 并发问题

### 3. BELONGS_TO（归属关系）
- 模块归属: 支付模块 BELONGS_TO 电商系统
- 功能归属: 用户登录 BELONGS_TO 用户管理

### 4. CAUSES（因果关系）
- 问题原因: 高并发 CAUSES 性能下降
- 技术因果: 内存泄漏 CAUSES 系统崩溃

### 5. RELATED_TO（相关关系）
- 概念相关: 微服务 RELATED_TO 分布式架构
- 技术相关: Docker RELATED_TO Kubernetes

## 📝 分析内容
**会话ID**: %s
**用户内容**: %s

## 📋 输出格式
请严格按照以下JSON格式输出：

{
  "entities": [
    {
      "title": "Docker",
      "type": "Technical",
      "description": "容器化技术平台",
      "confidence": 0.95,
      "keywords": ["容器", "部署", "虚拟化"]
    }
  ],
  "relationships": [
    {
      "source": "API网关",
      "target": "高延迟问题",
      "relation_type": "CAUSES",
      "description": "API网关服务出现高延迟问题",
      "strength": 9,
      "confidence": 0.9,
      "evidence": "API网关服务出现高延迟问题，P99延迟从50ms增加到500ms"
    }
  ],
  "extraction_meta": {
    "entity_count": 0,
    "relationship_count": 0,
    "overall_quality": 0.85
  }
}`, sessionID, content)
}

// buildSimplifiedPrompt 构建简化版prompt
func buildSimplifiedPrompt(content string) string {
	return fmt.Sprintf(`从以下技术内容中提取关键实体和关系：

内容: %s

请输出JSON格式：
{
  "entities": ["实体1", "实体2", "实体3"],
  "relations": ["实体A->USES->实体B", "实体C->SOLVES->实体D"]
}`, content)
}

// createTestLLMClient 创建测试用的LLM客户端
func createTestLLMClient() (llm.LLMClient, error) {
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("DEEPSEEK_API_KEY未设置")
	}

	// 设置LLM配置
	config := &llm.LLMConfig{
		Provider:   llm.ProviderDeepSeek,
		APIKey:     apiKey,
		Model:      "deepseek-chat",
		MaxRetries: 3,
		Timeout:    120 * time.Second,
		RateLimit:  300,
	}

	// 设置全局配置
	llm.SetGlobalConfig(llm.ProviderDeepSeek, config)

	// 创建客户端
	client, err := llm.CreateGlobalClient(llm.ProviderDeepSeek)
	if err != nil {
		return nil, fmt.Errorf("创建LLM客户端失败: %w", err)
	}

	fmt.Printf("✅ [测试客户端] LLM客户端创建成功\n")
	return client, nil
}
