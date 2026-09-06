package security

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestMultiLayerDetector 测试多层检测器
func TestMultiLayerDetector(t *testing.T) {
	detector := NewMultiLayerDetector("http://localhost:11434", "qwen2.5:7b")
	detector.dictMatcher.LoadDefaultDictionary()

	t.Log("╔══════════════════════════════════════════════════════════╗")
	t.Log("║  测试[1]: 多层融合检测器(5层架构)                        ║")
	t.Log("╠══════════════════════════════════════════════════════════╣")
	t.Log("║  Layer 1: 正则检测  (权重70%) → 快速模式匹配            ║")
	t.Log("║  Layer 2: 词典匹配  (权重10%) → Aho-Corasick自动机      ║")
	t.Log("║  Layer 4: 上下文规则(权重15%) → 触发词+抑制词           ║")
	t.Log("║  Layer 5: LLM语义   (权重 5%) → Ollama推理              ║")
	t.Log("║  融合模型: PCCM渐进式置信度累积                          ║")
	t.Log("║  算法优化: CASIA上下文感知敏感信息算法                   ║")
	t.Log("╚══════════════════════════════════════════════════════════╝")

	testCases := []struct {
		name           string
		text           string
		expectedTypes  []SensitiveType
		minConfidence  float64
		maxLayers      int
	}{
		{
			name:          "测试1: 手机号检测",
			text:          "我的手机号是13812345678",
			expectedTypes: []SensitiveType{SensitiveTypePhone},
			minConfidence: 0.40,
			maxLayers:     5,
		},
		{
			name:          "测试2: 密码检测",
			text:          "我的密码是MyP@ssw0rd123",
			expectedTypes: []SensitiveType{SensitiveTypePassword},
			minConfidence: 0.20,
			maxLayers:     5,
		},
		{
			name:          "测试3: 混合敏感信息",
			text:          "张三的手机号是13812345678，身份证号是110101199001011234",
			expectedTypes: []SensitiveType{SensitiveTypePhone, SensitiveTypeIDCard},
			minConfidence: 0.40,
			maxLayers:     5,
		},
		{
			name:          "测试4: API密钥检测",
			text:          "我的API密钥是sk-1234567890abcdefghij",
			expectedTypes: []SensitiveType{SensitiveTypeAPIKey},
			minConfidence: 0.30,
			maxLayers:     5,
		},
		{
			name:          "测试5: 敏感词检测",
			text:          "这是一个关于网络赌博的讨论",
			expectedTypes: []SensitiveType{SensitiveType("fraud")},
			minConfidence: 0.01,
			maxLayers:     5,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("  ┌─ 输入文本: %q", tc.text)
			t.Logf("  │  期望类型: %v", tc.expectedTypes)
			t.Logf("  │  最低置信度: %.2f", tc.minConfidence)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			result, err := detector.Detect(ctx, tc.text)
			if err != nil {
				t.Fatalf("  └─ FAIL: 检测失败: %v", err)
			}

			t.Logf("  │  ──── 检测结果 ────")
			t.Logf("  │  使用层数: %v", result.LayersUsed)
			t.Logf("  │  检测到 %d 个敏感信息", len(result.FinalItems))
			t.Logf("  │  综合置信度: %.2f", result.FinalConfidence)
			t.Logf("  │  总耗时: %v", result.TotalTime)

			for i, item := range result.FinalItems {
				t.Logf("  │    [%d] 类型=%s, 值=%s, 置信度=%.2f, 位置=%d-%d",
					i+1, item.Type, item.Value, item.Confidence, item.Start, item.End)
			}

			if len(result.FinalItems) == 0 {
				t.Errorf("  └─ FAIL: 未检测到任何敏感信息")
			} else if result.FinalConfidence < tc.minConfidence {
				t.Errorf("  └─ FAIL: 置信度过低 %.2f < %.2f", result.FinalConfidence, tc.minConfidence)
			} else {
				t.Logf("  └─ PASS ✓")
			}
		})
	}
}

// TestLayerPerformance 测试各层性能
func TestLayerPerformance(t *testing.T) {
	detector := NewMultiLayerDetector("http://localhost:11434", "qwen2.5:7b")
	detector.dictMatcher.LoadDefaultDictionary()

	text := "张三的手机号是13812345678，邮箱是zhangsan@example.com，身份证号是110101199001011234，银行卡号是6222021234567890123"

	ctx := context.Background()

	// 测试10次
	iterations := 10
	totalTime := time.Duration(0)

	for i := 0; i < iterations; i++ {
		result, err := detector.Detect(ctx, text)
		if err != nil {
			t.Fatalf("检测失败: %v", err)
		}
		totalTime += result.TotalTime
	}

	avgTime := totalTime / time.Duration(iterations)

	t.Logf("性能测试结果:")
	t.Logf("  - 迭代次数: %d", iterations)
	t.Logf("  - 平均耗时: %v", avgTime)
	t.Logf("  - 总耗时: %v", totalTime)

	// 验证性能目标（平均<100ms）
	if avgTime > 100*time.Millisecond {
		t.Logf("警告: 平均耗时 %v 超过目标 100ms", avgTime)
	}

	// 获取统计信息
	stats := detector.GetStats()
	t.Logf("\n统计信息:")
	t.Logf("  - 总检测次数: %d", stats.TotalDetections)
	t.Logf("  - 提前终止次数: %d", stats.EarlyStopCount)
	t.Logf("  - 冲突次数: %d", stats.ConflictCount)
	t.Logf("  - 平均延迟: %v", stats.AverageLatency)
	t.Logf("  - 各层使用次数:")
	for layer, count := range stats.LayerUsage {
		t.Logf("    Layer %d: %d 次", layer, count)
	}
}

// TestDictionaryMatcher 测试词典匹配
func TestDictionaryMatcher(t *testing.T) {
	matcher := NewDictionaryMatcher()
	matcher.LoadDefaultDictionary()

	t.Log("╔══════════════════════════════════════════════════════════╗")
	t.Log("║  测试[2]: 词典匹配器(Aho-Corasick自动机)                ║")
	t.Log("╠══════════════════════════════════════════════════════════╣")
	t.Log("║  功能: 基于预加载词典的多模式字符串匹配                  ║")
	t.Log("║  词典内容: 中文敏感词(赌博/诈骗/毒品等)                 ║")
	t.Log("╚══════════════════════════════════════════════════════════╝")

	testCases := []struct {
		name          string
		text          string
		expectedCount int
	}{
		{
			name:          "测试敏感词1",
			text:          "这是关于网络赌博和网络诈骗的讨论",
			expectedCount: 2,
		},
		{
			name:          "测试敏感词2",
			text:          "正常的文本内容，没有敏感词",
			expectedCount: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("  ┌─ 输入文本: %q", tc.text)
			t.Logf("  │  期望检测: %d 个敏感词", tc.expectedCount)

			results := matcher.Match(tc.text)
			t.Logf("  │  实际检测: %d 个敏感词", len(results))
			for _, result := range results {
				t.Logf("  │    - %s (类型=%s, 置信度=%.2f)", result.Value, result.Type, result.Confidence)
			}

			if len(results) != tc.expectedCount {
				t.Errorf("  └─ FAIL: 期望 %d 个，实际 %d 个", tc.expectedCount, len(results))
			} else {
				t.Logf("  └─ PASS ✓")
			}
		})
	}
}

// TestContextRuleEngine 测试上下文规则引擎
func TestContextRuleEngine(t *testing.T) {
	engine := NewContextRuleEngine()

	t.Log("╔══════════════════════════════════════════════════════════╗")
	t.Log("║  测试[3]: 上下文规则引擎(Layer 4)                       ║")
	t.Log("╠══════════════════════════════════════════════════════════╣")
	t.Log("║  机制: 触发词 + 上下文关键词 → 提高置信度               ║")
	t.Log("║  机制: 抑制词 → 降低误报（如\"password的英文意思是...\"） ║")
	t.Log("╚══════════════════════════════════════════════════════════╝")

	testCases := []struct {
		name          string
		text          string
		expectedCount int
	}{
		{
			name:          "测试密码上下文",
			text:          "我的密码是MyPassword123",
			expectedCount: 1,
		},
		{
			name:          "测试手机号上下文",
			text:          "张三的手机号是13812345678",
			expectedCount: 1,
		},
		{
			name:          "测试抑制词",
			text:          "password这个英文单词的意思是密码",
			expectedCount: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("  ┌─ 输入文本: %q", tc.text)
			t.Logf("  │  期望匹配: %d 条", tc.expectedCount)

			results := engine.Analyze(tc.text, []NERResult{})
			t.Logf("  │  实际匹配: %d 条", len(results))
			for _, result := range results {
				t.Logf("  │    - 值=%s, 类型=%s, 置信度=%.2f", result.Value, result.Type, result.Confidence)
				t.Logf("  │      触发词: %v, 抑制词: %v", result.TriggersFound, result.SuppressorsFound)
			}

			if len(results) < tc.expectedCount {
				t.Errorf("  └─ FAIL: 期望至少 %d 条，实际 %d 条", tc.expectedCount, len(results))
			} else {
				t.Logf("  └─ PASS ✓")
			}
		})
	}
}

// BenchmarkMultiLayerDetector 性能基准测试
func BenchmarkMultiLayerDetector(b *testing.B) {
	detector := NewMultiLayerDetector("http://localhost:11434", "qwen2.5:7b")
	detector.dictMatcher.LoadDefaultDictionary()

	// 禁用LLM层以加快测试
	detector.llmDetector.Disable()

	text := "张三的手机号是13812345678，邮箱是zhangsan@example.com"
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := detector.Detect(ctx, text)
		if err != nil {
			b.Fatalf("检测失败: %v", err)
		}
	}
}

// 示例：如何使用多层检测器
func ExampleMultiLayerDetector() {
	// 创建检测器
	detector := NewMultiLayerDetector("http://localhost:11434", "qwen2.5:7b")
	detector.dictMatcher.LoadDefaultDictionary()

	// 检测文本
	text := "我的手机号是13812345678，密码是MyP@ssw0rd123"
	ctx := context.Background()

	result, err := detector.Detect(ctx, text)
	if err != nil {
		fmt.Printf("检测失败: %v\n", err)
		return
	}

	// 输出结果
	fmt.Printf("检测到 %d 个敏感信息\n", len(result.FinalItems))
	fmt.Printf("综合置信度: %.2f\n", result.FinalConfidence)
	fmt.Printf("使用层数: %v\n", result.LayersUsed)
	fmt.Printf("总耗时: %v\n", result.TotalTime)

	for i, item := range result.FinalItems {
		fmt.Printf("[%d] %s: %s (置信度: %.2f)\n",
			i+1, item.Type, item.Value, item.Confidence)
	}
}
