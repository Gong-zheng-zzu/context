package security

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestMultiLayerConfigurationAuditsFixedUnvalidatedPCCMSWeights(t *testing.T) {
	detector := NewMultiLayerDetector("http://127.0.0.1:1", "unused")
	configuration := detector.GetConfiguration()
	if configuration.PCCM.Version != PCCMSecurityConfigVersion || configuration.PCCM.CalibrationSource != "fixed_unvalidated" {
		t.Fatalf("PCCM-S configuration = %+v", configuration.PCCM)
	}
	if configuration.PCCM.LayerWeights[1] != 0.70 || configuration.PCCM.LayerWeights[4] != 0.15 {
		t.Fatalf("PCCM-S weights = %+v", configuration.PCCM.LayerWeights)
	}
	configuration.PCCM.LayerWeights[1] = 0
	if detector.GetConfiguration().PCCM.LayerWeights[1] != 0.70 {
		t.Fatal("configuration snapshot mutated live PCCM-S weights")
	}
}

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
		name          string
		text          string
		expectedTypes []SensitiveType
		minConfidence float64
		maxLayers     int
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

func TestMultiLayerConfigurationReflectsAlgorithmSwitches(t *testing.T) {
	detector := NewMultiLayerDetector("http://localhost:11434", "qwen2.5:7b")
	if configuration := detector.GetConfiguration(); !configuration.PCCMEnabled || !configuration.CASIAEnabled || !configuration.EarlyStop {
		t.Fatalf("default configuration = %+v", configuration)
	}

	detector.DisablePCCM()
	detector.DisableCASIA()
	detector.DisableEarlyStop()
	configuration := detector.GetConfiguration()
	if configuration.PCCMEnabled || configuration.CASIAEnabled || configuration.EarlyStop {
		t.Fatalf("disabled configuration = %+v", configuration)
	}
}

func TestMultiLayerDetectorAttachesCASIAEvidence(t *testing.T) {
	detector := NewMultiLayerDetector("http://localhost:11434", "qwen2.5:3b")
	result, err := detector.Detect(context.Background(), "患者身份证号是110101199001011234")
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if len(result.FinalItems) == 0 {
		t.Fatal("expected an ID-card detection")
	}
	for _, item := range result.FinalItems {
		if item.Type == SensitiveTypeIDCard {
			if item.CASIA == nil {
				t.Fatal("ID-card result is missing CASIA evidence")
			}
			if item.CASIA.AdjustedScore != item.Confidence {
				t.Fatalf("CASIA adjusted score=%v, item confidence=%v", item.CASIA.AdjustedScore, item.Confidence)
			}
			return
		}
	}
	t.Fatalf("ID-card detection missing from results: %#v", result.FinalItems)
}

func TestConfigureMultiLayerAlgorithmsUsesConfiguredSwitches(t *testing.T) {
	detector := NewMultiLayerDetector("http://localhost:11434", "qwen2.5:7b")
	configureMultiLayerAlgorithms(detector, false, true)
	configuration := detector.GetConfiguration()
	if configuration.PCCMEnabled || !configuration.CASIAEnabled {
		t.Fatalf("configuration after applying switches = %+v", configuration)
	}
}

// TestResolvePCCMConfigWithoutArtifactKeepsDefault 确认未指定校准产物时，
// 配置来源与环境变量/默认值完全一致（默认路径零影响）。
func TestResolvePCCMConfigWithoutArtifactKeepsDefault(t *testing.T) {
	t.Setenv(pccmCalibrationArtifactEnv, "")

	config := resolvePCCMConfig()
	if config.CalibrationSource != "fixed_unvalidated" {
		t.Fatalf("calibration source = %q, want fixed_unvalidated", config.CalibrationSource)
	}
	if config.LayerWeights[1] != 0.70 || config.LayerWeights[5] != 0.05 {
		t.Fatalf("layer weights = %+v, want the default fixed weights", config.LayerWeights)
	}
}

// TestResolvePCCMConfigWithoutProviderFallsBack 确认设置了产物路径但未注册
// 加载实现时安全回退，不 panic、不改变默认权重。
func TestResolvePCCMConfigWithoutProviderFallsBack(t *testing.T) {
	t.Setenv(pccmCalibrationArtifactEnv, "/nonexistent/pccm-artifact.json")

	previous := pccmCalibrationProvider
	RegisterPCCMCalibrationProvider(nil)
	defer RegisterPCCMCalibrationProvider(previous)

	config := resolvePCCMConfig()
	if config.CalibrationSource != "fixed_unvalidated" {
		t.Fatalf("calibration source = %q, want fallback to fixed_unvalidated", config.CalibrationSource)
	}
}

// TestResolvePCCMConfigUsesProviderResult 确认注册了实现时采用产物配置，
// 且产物配置同时进入模型与配置快照（避免二者不一致）。
func TestResolvePCCMConfigUsesProviderResult(t *testing.T) {
	t.Setenv(pccmCalibrationArtifactEnv, "/tmp/pccm-artifact.json")

	calibrated := PCCMSecurityConfig{
		Version:                  "pccm-s-devset-v1",
		CalibrationSource:        "devset_calibrated_v1",
		LayerWeights:             map[int]float64{1: 0.55, 2: 0.20, 4: 0.20, 5: 0.05},
		EnhancementPerExtraLayer: 0.05,
		DecisionThreshold:        0.45,
		LayerActivationThreshold: 0.3,
	}

	previous := pccmCalibrationProvider
	RegisterPCCMCalibrationProvider(func(path string) (PCCMSecurityConfig, error) {
		return calibrated, nil
	})
	defer RegisterPCCMCalibrationProvider(previous)

	config := resolvePCCMConfig()
	if config.CalibrationSource != "devset_calibrated_v1" {
		t.Fatalf("calibration source = %q, want devset_calibrated_v1", config.CalibrationSource)
	}
	if config.LayerWeights[1] != 0.55 {
		t.Fatalf("layer weights = %+v, want the calibrated weights", config.LayerWeights)
	}
}

// TestResolvePCCMConfigFallsBackOnProviderError 确认加载失败时回退到默认权重。
func TestResolvePCCMConfigFallsBackOnProviderError(t *testing.T) {
	t.Setenv(pccmCalibrationArtifactEnv, "/tmp/broken-artifact.json")

	previous := pccmCalibrationProvider
	RegisterPCCMCalibrationProvider(func(path string) (PCCMSecurityConfig, error) {
		return PCCMSecurityConfig{}, fmt.Errorf("artifact is corrupt")
	})
	defer RegisterPCCMCalibrationProvider(previous)

	config := resolvePCCMConfig()
	if config.CalibrationSource != "fixed_unvalidated" || config.LayerWeights[1] != 0.70 {
		t.Fatalf("config = %+v, want a fallback to the default fixed weights", config)
	}
}

// TestNewMultiLayerDetectorSharesOnePCCMConfig 确认检测器把同一份配置同时用于
// 模型计算与审计快照（此前二者不一致：模型用 env 配置、快照恒定用默认值）。
func TestNewMultiLayerDetectorSharesOnePCCMConfig(t *testing.T) {
	t.Setenv("PCCM_LAYER_WEIGHT_1", "0.66")
	defer t.Setenv("PCCM_LAYER_WEIGHT_1", "")

	detector := NewMultiLayerDetector("http://localhost:11434", "qwen2.5:7b")
	snapshot := detector.GetConfiguration().PCCM
	model := detector.pccmModel.Config()

	if snapshot.LayerWeights[1] != model.LayerWeights[1] {
		t.Fatalf("snapshot weight %v != model weight %v; the two must share one config",
			snapshot.LayerWeights[1], model.LayerWeights[1])
	}
	if snapshot.LayerWeights[1] != 0.66 {
		t.Fatalf("layer 1 weight = %v, want the env override 0.66 reflected in the snapshot", snapshot.LayerWeights[1])
	}
}

func TestCASIAConfidenceFloorDefaultsAndClamps(t *testing.T) {
	if got := casiaConfidenceFloor(CASIAConfig{}); got != DefaultCASIAConfidenceFloor {
		t.Fatalf("unset floor = %v, want default %v", got, DefaultCASIAConfidenceFloor)
	}
	if got := casiaConfidenceFloor(CASIAConfig{ConfidenceFloor: 0}); got != DefaultCASIAConfidenceFloor {
		t.Fatalf("zero floor = %v, want default %v", got, DefaultCASIAConfidenceFloor)
	}
	if got := casiaConfidenceFloor(CASIAConfig{ConfidenceFloor: 0.4}); got != 0.4 {
		t.Fatalf("explicit floor = %v, want 0.4", got)
	}
	if got := casiaConfidenceFloor(CASIAConfig{ConfidenceFloor: 2}); got != 1 {
		t.Fatalf("oversized floor = %v, want clamped to 1", got)
	}
}

// TestCASIAKeepsConfirmedMatchWhenContextHasNoKeywords 覆盖消融实验暴露的缺陷：
// 当上下文里没有该类型的任何关键词时 NormalizedWeight 为 0.5，纯乘法会把已被
// 正则确证的匹配压到过滤阈值以下。保底必须让这条结果存活下来。
func TestCASIAKeepsConfirmedMatchWhenContextHasNoKeywords(t *testing.T) {
	detector := NewMultiLayerDetector("http://127.0.0.1:1", "unused")

	// "联系方式" 是 phone 的正向词，不在 id_card 关键词表内，因此 id_card 的
	// 上下文权重为中性（0.5）。这正是 Base64/特殊字符混淆样本的形状。
	text := "联系方式:110101199001011234"

	regexItems := detector.regexDetector.Detect(text)
	if len(regexItems) == 0 {
		t.Fatal("regex layer should confirm an 18-digit ID card")
	}
	var baseConfidence float64
	for _, item := range regexItems {
		if item.Type == SensitiveTypeIDCard {
			baseConfidence = item.Confidence
		}
	}
	if baseConfidence == 0 {
		t.Fatalf("regex layer matched %d items but none was an ID card: %#v", len(regexItems), regexItems)
	}

	adjusted := detector.applyCASIA(text, regexItems, true)
	if len(adjusted) == 0 {
		t.Fatal("CASIA dropped a regex-confirmed match whose context carries no ID-card keyword")
	}

	// 中性上下文下应恰好落在保底线上，而不再是无保底的 base*0.5。
	expected := baseConfidence * DefaultCASIAConfidenceFloor
	for _, item := range adjusted {
		if item.Type != SensitiveTypeIDCard {
			continue
		}
		if item.Confidence < expected-1e-9 {
			t.Fatalf("adjusted confidence = %v, want at least the floor %v", item.Confidence, expected)
		}
		if item.CASIA == nil || item.CASIA.AdjustedScore != item.Confidence {
			t.Fatalf("CASIA evidence does not mirror the final confidence: %+v", item.CASIA)
		}
	}
}

// TestCASIAFloorDoesNotApplyToInferredLayers 覆盖保底的适用范围：确证型层
// （正则/词典）享受保底，推断型层（上下文规则）必须继续被抑制，否则
// "邮政编码 110101" 这类正常文本会被误判为敏感。
func TestCASIAFloorDoesNotApplyToInferredLayers(t *testing.T) {
	detector := NewMultiLayerDetector("http://127.0.0.1:1", "unused")

	text := "联系方式:110101199001011234"
	items := detector.regexDetector.Detect(text)
	if len(items) == 0 {
		t.Fatal("regex layer should confirm an 18-digit ID card")
	}

	confirmed := detector.applyCASIA(text, items, true)
	inferred := detector.applyCASIA(text, items, false)

	if len(confirmed) == 0 {
		t.Fatal("confirmed layer must keep the match")
	}
	if len(inferred) == 0 {
		// 推断型层被判为弱匹配并被过滤，同样说明保底没有生效。
		return
	}
	if inferred[0].Confidence >= confirmed[0].Confidence {
		t.Fatalf("inferred confidence %v must stay below the floored confirmed confidence %v",
			inferred[0].Confidence, confirmed[0].Confidence)
	}
}

// TestCASIANegativeContextSuppressesConfirmedMatch 确认负向上下文能真正抑制
// 正则确证的形状误报：保底只保护中性/正向上下文，一旦命中负向关键词，结果应被
// 压到过滤阈值以下并从候选中移除，而不是被保底抵消（"快递单号 110101199001011234"
// 是身份证形状，但语境否定了它）。
func TestCASIANegativeContextSuppressesConfirmedMatch(t *testing.T) {
	detector := NewMultiLayerDetector("http://127.0.0.1:1", "unused")

	positive := detector.regexDetector.Detect("身份证号 110101199001011234")
	negative := detector.regexDetector.Detect("快递单号 110101199001011234")
	if len(positive) == 0 || len(negative) == 0 {
		t.Skip("regex layer did not confirm the probe value in both contexts")
	}

	positiveAdjusted := detector.applyCASIA("身份证号 110101199001011234", positive, true)
	negativeAdjusted := detector.applyCASIA("快递单号 110101199001011234", negative, true)

	if len(positiveAdjusted) == 0 {
		t.Fatal("positive context must keep the regex-confirmed identity match")
	}
	if len(negativeAdjusted) != 0 {
		t.Fatalf(
			"negative context must suppress the identity-shaped probe, got %d kept item(s) at confidence %v",
			len(negativeAdjusted), negativeAdjusted[0].Confidence,
		)
	}
}

func TestSecurityServiceConfigurationIncludesEnabledPath(t *testing.T) {
	service := &SecurityService{
		multiLayerDetector: NewMultiLayerDetector("http://localhost:11434", "qwen2.5:3b"),
		useMultiLayer:      true,
	}
	configuration := service.GetMultiLayerConfiguration()
	if !configuration.Enabled || !configuration.PCCMEnabled || !configuration.CASIAEnabled {
		t.Fatalf("configuration = %+v, want enabled CASIA/PCCM path", configuration)
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
