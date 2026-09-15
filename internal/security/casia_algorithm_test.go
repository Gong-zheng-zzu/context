package security

import (
	"strings"
	"testing"
)

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
