package security

import (
	"strings"
	"testing"
)

func TestCASIAConfigurationIsVersionedAndDefensivelyCopied(t *testing.T) {
	algo := NewContextAwareSensitiveInfoAlgorithm()
	config := algo.GetConfiguration()
	if config.Version != CASIAConfigVersion || config.ContextWindow != defaultCASIAContextWindow || config.DecisionThreshold != 0.6 {
		t.Fatalf("configuration = %+v", config)
	}
	config.KeywordWeights["phone"]["手机"] = 99
	evidence := algo.AnalyzeContext("手机号13812345678", len("手机号"), len("手机号13812345678"), "phone", 0)
	if evidence.ConfigurationVersion != CASIAConfigVersion || evidence.DecisionThreshold != 0.6 {
		t.Fatalf("evidence metadata = %+v", evidence)
	}
	if evidence.RawWeight == 99 {
		t.Fatal("configuration snapshot mutated the live CASIA keyword table")
	}
}

func TestCASIACustomWindowAndThresholdReachEvidence(t *testing.T) {
	config := DefaultCASIAConfig()
	config.Version = "casia-test-v2"
	config.ContextWindow = 12
	config.DecisionThreshold = 0.75
	algo := NewContextAwareSensitiveInfoAlgorithmWithConfig(config)
	evidence := algo.AnalyzeContext("手机号13812345678", len("手机号"), len("手机号13812345678"), "phone", 0)
	if evidence.ConfigurationVersion != "casia-test-v2" || evidence.WindowBytes != 12 || evidence.DecisionThreshold != 0.75 {
		t.Fatalf("custom evidence = %+v", evidence)
	}
}

func TestCASIADescriptionDoesNotClaimUnverifiedMetricsOrLearnedWeights(t *testing.T) {
	description := NewContextAwareSensitiveInfoAlgorithm().GetAlgorithmDescription()
	for _, unsupported := range []string{"误报率降低：从", "准确率提升：从", "权重学习："} {
		if strings.Contains(description, unsupported) {
			t.Fatalf("algorithm description contains unsupported claim %q", unsupported)
		}
	}
	for _, boundary := range []string{"固定配置", "数据集哈希", "历史数字不代表当前代码成绩"} {
		if !strings.Contains(description, boundary) {
			t.Fatalf("algorithm description is missing evidence boundary %q", boundary)
		}
	}
}

func TestCASIAEvidenceCapturesPositiveAndNegativeKeywords(t *testing.T) {
	algorithm := NewContextAwareSensitiveInfoAlgorithm()
	text := "邮编110101。患者身份证110101199001011234。"
	start := strings.Index(text, "110101199001011234")
	end := start + len("110101199001011234")
	evidence := algorithm.AnalyzeContext(text, start, end, string(SensitiveTypeIDCard), 64)
	if evidence.ContextStart < strings.Index(text, "患者") {
		t.Fatalf("context start=%d escaped sentence boundary", evidence.ContextStart)
	}
	if evidence.ContextEnd > len(text) {
		t.Fatalf("context end=%d exceeds text length", evidence.ContextEnd)
	}
	if evidence.NormalizedWeight <= 0.5 {
		t.Fatalf("normalized weight=%v, want positive identity context", evidence.NormalizedWeight)
	}
	var found bool
	for _, match := range evidence.MatchedKeywords {
		if match.Keyword == "身份证" && match.Weight > 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("evidence=%#v, missing positive 身份证 keyword", evidence)
	}
}

func TestCASIAContextWindowDoesNotCrossSentence(t *testing.T) {
	algorithm := NewContextAwareSensitiveInfoAlgorithm()
	text := "身份证号码请登记。邮编110101。"
	start := strings.Index(text, "110101")
	end := start + len("110101")
	evidence := algorithm.AnalyzeContext(text, start, end, string(SensitiveTypeIDCard), 64)
	for _, match := range evidence.MatchedKeywords {
		if match.Keyword == "身份证" {
			t.Fatalf("cross-sentence keyword leaked into evidence: %#v", evidence)
		}
	}
}

func TestCASIAAnalyzeContextClampsInvalidSpans(t *testing.T) {
	algorithm := NewContextAwareSensitiveInfoAlgorithm()
	evidence := algorithm.AnalyzeContext("血压120/80", 999, 1200, "blood_pressure", 10)
	if evidence.ContextStart < 0 || evidence.ContextEnd > len("血压120/80") || evidence.ContextStart > evidence.ContextEnd {
		t.Fatalf("invalid clamped bounds: %#v", evidence)
	}
}
