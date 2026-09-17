package security

import (
	"reflect"
	"strings"
	"testing"
)

// newSimilarityTestAlgorithm 构造一个关键词表受控的 CASIA 实例，便于隔离验证
// 可选 embedding 语义路径。
func newSimilarityTestAlgorithm() *ContextAwareSensitiveInfoAlgorithm {
	config := DefaultCASIAConfig()
	config.KeywordWeights = map[string]map[string]float64{
		"medical_record": {
			"病历":  1.0,
			"订单号": -0.7,
		},
	}
	return NewContextAwareSensitiveInfoAlgorithmWithConfig(config)
}

func findKeywordEvidence(evidence CASIAEvidence, keyword string) (CASIAKeywordEvidence, bool) {
	for _, match := range evidence.MatchedKeywords {
		if match.Keyword == keyword {
			return match, true
		}
	}
	return CASIAKeywordEvidence{}, false
}

// TestCASIAEmbeddingSimilarityDisabledByDefault 验证语义路径默认关闭。
func TestCASIAEmbeddingSimilarityDisabledByDefault(t *testing.T) {
	algo := NewContextAwareSensitiveInfoAlgorithm()
	if algo.EmbeddingSimilarityEnabled() {
		t.Fatal("embedding similarity must be disabled by default")
	}
	if algo.EmbeddingSimilarityThreshold() != DefaultCASIASimilarityThreshold {
		t.Fatalf("default threshold = %v, want %v", algo.EmbeddingSimilarityThreshold(), DefaultCASIASimilarityThreshold)
	}

	// 注入 nil 函数不应启用该路径。
	algo.EnableEmbeddingSimilarity(nil, 0.9)
	if algo.EmbeddingSimilarityEnabled() {
		t.Fatal("nil similarity func must not enable the semantic path")
	}
}

// TestCASIAExactMatchRecordsExactMethod 验证默认路径命中记为 exact。
func TestCASIAExactMatchRecordsExactMethod(t *testing.T) {
	algo := newSimilarityTestAlgorithm()
	text := "病历号A123"
	evidence := algo.AnalyzeContext(text, 0, len(text), "medical_record", 64)
	match, ok := findKeywordEvidence(evidence, "病历")
	if !ok {
		t.Fatalf("expected exact match for 病历: %+v", evidence)
	}
	if match.MatchMethod != "exact" {
		t.Fatalf("match method = %q, want exact", match.MatchMethod)
	}
	if evidence.RawWeight != 1.0 {
		t.Fatalf("raw weight = %v, want 1.0", evidence.RawWeight)
	}
}

// TestCASIAEmbeddingSimilaritySemanticHit 验证注入实现可提供语义命中。
func TestCASIAEmbeddingSimilaritySemanticHit(t *testing.T) {
	algo := newSimilarityTestAlgorithm()
	text := "既往就医记录如下"
	algo.EnableEmbeddingSimilarity(func(contextText, keyword string) (float64, bool) {
		if keyword == "病历" && strings.Contains(contextText, "就医") {
			return 0.95, true
		}
		return 0.0, true
	}, 0.9)

	evidence := algo.AnalyzeContext(text, 0, len(text), "medical_record", 64)
	match, ok := findKeywordEvidence(evidence, "病历")
	if !ok {
		t.Fatalf("expected semantic match for 病历: %+v", evidence)
	}
	if match.MatchMethod != "semantic" {
		t.Fatalf("match method = %q, want semantic", match.MatchMethod)
	}
	if evidence.RawWeight != 1.0 {
		t.Fatalf("raw weight = %v, want 1.0", evidence.RawWeight)
	}
}

// TestCASIAEmbeddingSimilarityBelowThresholdNoMatch 验证低于阈值不命中。
func TestCASIAEmbeddingSimilarityBelowThresholdNoMatch(t *testing.T) {
	algo := newSimilarityTestAlgorithm()
	text := "既往就医记录如下"
	algo.EnableEmbeddingSimilarity(func(contextText, keyword string) (float64, bool) {
		return 0.5, true
	}, 0.9)

	evidence := algo.AnalyzeContext(text, 0, len(text), "medical_record", 64)
	if evidence.RawWeight != 0 {
		t.Fatalf("below-threshold similarity must not match, raw weight = %v", evidence.RawWeight)
	}
	if len(evidence.MatchedKeywords) != 0 {
		t.Fatalf("unexpected matches: %+v", evidence.MatchedKeywords)
	}
}

// TestCASIAEmbeddingSimilarityDegradesWhenCallFails 验证实现返回 ok=false 时降级。
func TestCASIAEmbeddingSimilarityDegradesWhenCallFails(t *testing.T) {
	algo := newSimilarityTestAlgorithm()
	text := "既往就医记录如下"
	algo.EnableEmbeddingSimilarity(func(contextText, keyword string) (float64, bool) {
		return 0.99, false
	}, 0.9)

	evidence := algo.AnalyzeContext(text, 0, len(text), "medical_record", 64)
	if evidence.RawWeight != 0 {
		t.Fatalf("failed similarity call must degrade to no match, raw weight = %v", evidence.RawWeight)
	}
}

// TestCASIAEmbeddingNotInjectedMatchesContains 验证未注入时行为与纯 strings.Contains 一致。
func TestCASIAEmbeddingNotInjectedMatchesContains(t *testing.T) {
	withHookButNoFunc := newSimilarityTestAlgorithm()
	withHookButNoFunc.EnableEmbeddingSimilarity(nil, 0.8) // 未注入 → 保持关闭

	baseline := newSimilarityTestAlgorithm()
	if baseline.EmbeddingSimilarityEnabled() || withHookButNoFunc.EmbeddingSimilarityEnabled() {
		t.Fatal("both algorithms should have the semantic path disabled")
	}

	text := "病历号A123，订单号 XYZ"
	got := withHookButNoFunc.AnalyzeContext(text, 0, len(text), "medical_record", 64)
	want := baseline.AnalyzeContext(text, 0, len(text), "medical_record", 64)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("behavior differs without injection:\ngot=%+v\nwant=%+v", got, want)
	}
}

// TestCASIAEmbeddingSimilarityCanBeDisabled 验证可关闭并回到等价行为。
func TestCASIAEmbeddingSimilarityCanBeDisabled(t *testing.T) {
	algo := newSimilarityTestAlgorithm()
	algo.EnableEmbeddingSimilarity(func(contextText, keyword string) (float64, bool) {
		return 0.99, true
	}, 0.9)
	if !algo.EmbeddingSimilarityEnabled() {
		t.Fatal("expected semantic path to be enabled")
	}
	algo.DisableEmbeddingSimilarity()
	if algo.EmbeddingSimilarityEnabled() {
		t.Fatal("expected semantic path to be disabled")
	}
}
