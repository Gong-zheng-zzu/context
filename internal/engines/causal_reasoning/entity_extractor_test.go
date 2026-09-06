package causal_reasoning

import (
	"math"
	"testing"
)

func TestParseLLMExtractionResultsAcceptsFencedEnvelopeAndAliases(t *testing.T) {
	results, err := parseLLMExtractionResults("```json\n{\"relations\":[{\"O\":\"张奶奶\",\"C\":\"服用降压药后\",\"P\":\"体位性低血压\",\"R\":\"滑倒\",\"confidence\":\"0.87\",\"evidence\":\"服药后出现低血压并滑倒\"}]}\n```")
	if err != nil {
		t.Fatalf("parseLLMExtractionResults() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("result count = %d, want 1", len(results))
	}
	result := results[0]
	if result.Object != "张奶奶" || result.Mediator != "服用降压药后" || result.Property != "体位性低血压" || result.Result != "滑倒" {
		t.Fatalf("parsed result = %+v", result)
	}
	if result.Confidence == nil || math.Abs(*result.Confidence-0.87) > 0.0001 {
		t.Fatalf("confidence = %v, want 0.87", result.Confidence)
	}
	if len(result.Evidence) != 1 {
		t.Fatalf("evidence = %#v, want one item", result.Evidence)
	}
}

func TestNormalizeLLMExtractionCorrectsClinicalSpanAndTemporalOrder(t *testing.T) {
	source := "张奶奶服用降压药后出现体位性低血压，在洗手间滑倒。"
	normalized, confidence, ok := normalizeLLMExtraction(source, LLMExtractionResult{
		Object:   "对象：张奶奶",
		Mediator: "服用降压药后",
		Property: "在洗手间滑倒。",
		Result:   "体位性低血压",
		Evidence: []string{"服用降压药后出现体位性低血压，在洗手间滑倒。"},
	})
	if !ok {
		t.Fatal("normalizeLLMExtraction() rejected complete relation")
	}
	if normalized.Mediator != "服用降压药" {
		t.Fatalf("mediator = %q, want temporal suffix removed", normalized.Mediator)
	}
	if normalized.Property != "体位性低血压" || normalized.Result != "在洗手间滑倒" {
		t.Fatalf("P/R order = %q -> %q, want mechanism before event", normalized.Property, normalized.Result)
	}
	if confidence < 0.90 || confidence > 0.92 {
		t.Fatalf("fallback confidence = %.3f, want evidence-based calibrated value", confidence)
	}
}

func TestPCCMFusionNormalizesOverAvailableEvidence(t *testing.T) {
	engine := NewDefaultPCCMFusionEngine()
	if got := engine.FuseConfidence(0, 0, 0.82, 1); math.Abs(got-0.82) > 0.0001 {
		t.Fatalf("LLM-only confidence = %.4f, want 0.82", got)
	}
	want := (0.85*0.92 + 0.05*0.82) / 0.90
	if got := engine.FuseConfidence(0.92, 0, 0.82, 1); math.Abs(got-want) > 0.0001 {
		t.Fatalf("rule+LLM confidence = %.4f, want %.4f", got, want)
	}
}
