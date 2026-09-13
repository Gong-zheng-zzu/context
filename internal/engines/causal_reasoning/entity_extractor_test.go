package causal_reasoning

import (
	"context"
	"errors"
	"math"
	"testing"
)

func TestExtractWithExecutionRejectsLLMWithoutClient(t *testing.T) {
	extractor := NewEntityExtractor(nil)
	relations, execution, err := extractor.ExtractWithExecution(context.Background(), "任意文本", false, false, true)
	if !errors.Is(err, ErrLLMUnavailable) {
		t.Fatalf("error = %v, want ErrLLMUnavailable", err)
	}
	if relations != nil {
		t.Fatalf("relations = %#v, want no fabricated output", relations)
	}
	if execution.Mode != "model_unavailable" || execution.LLMAvailable || execution.ModelStatus != "unavailable" {
		t.Fatalf("execution = %#v, want unavailable model state", execution)
	}
}

func TestExtractWithExecutionRecordsRulesFallback(t *testing.T) {
	extractor := NewEntityExtractor(nil)
	relations, execution, err := extractor.ExtractWithExecution(context.Background(), "患者服用降压药后出现体位性低血压", true, false, true)
	if err != nil {
		t.Fatalf("ExtractWithExecution() error = %v", err)
	}
	if execution.Mode != "rules_fallback" || execution.FallbackReason == "" || execution.ModelStatus != "unavailable" {
		t.Fatalf("execution = %#v, want explicit fallback state", execution)
	}
	if len(relations) == 0 || len(relations[0].RuleMatches) == 0 || relations[0].PCCMEvidence == nil {
		t.Fatalf("relations = %#v, want rule and PCCM evidence", relations)
	}
}

func TestMatchingRuleEvidenceIncludesBothCausalEdges(t *testing.T) {
	relation := CausalRelation{Mediator: "服用降压药", Property: "体位性低血压", Result: "跌倒"}
	rules := []RuleEvidence{
		{ID: "rule_001", Condition: "降压药", Effect: "体位性低血压"},
		{ID: "rule_002", Condition: "体位性低血压", Effect: "跌倒"},
	}
	matched := matchingRuleEvidence(relation, rules)
	if len(matched) != 2 || matched[0].ID != "rule_001" || matched[1].ID != "rule_002" {
		t.Fatalf("matched rules = %#v, want both causal edges", matched)
	}
}

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
	// The deterministic fallback adds 0.04 for each of the four source-backed
	// fields and 0.04 for the causal cue: 0.68 + 4*0.04 + 0.04 = 0.88.
	if math.Abs(confidence-0.88) > 0.0001 {
		t.Fatalf("fallback confidence = %.3f, want source-backed calibrated value 0.88", confidence)
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

func TestRuleExtractionComposesTwoDirectedEdgesAndAudit(t *testing.T) {
	extractor := NewEntityExtractor(nil)
	relations, _, err := extractor.ExtractWithExecution(context.Background(), "李爷爷服用降压药后出现体位性低血压并在洗手间滑倒", true, false, false)
	if err != nil || len(relations) != 1 {
		t.Fatalf("relations=%#v err=%v", relations, err)
	}
	rel := relations[0]
	if rel.Object != "李爷爷" || rel.Mediator != "服用降压药" || rel.Property != "体位性低血压" || rel.Result != "滑倒" {
		t.Fatalf("relation=%+v", rel)
	}
	if rel.Quality == nil || !rel.Quality.TupleValid || rel.Quality.ReviewRequired || rel.Quality.EvidenceCoverage < 0.75 {
		t.Fatalf("quality=%+v", rel.Quality)
	}
	if len(rel.RuleMatches) != 2 || rel.RuleMatches[0].Edge == "" || rel.RuleMatches[1].Edge == "" {
		t.Fatalf("rule evidence=%+v", rel.RuleMatches)
	}
	if len(rel.EvidenceSpans) < 4 {
		t.Fatalf("spans=%+v", rel.EvidenceSpans)
	}
}

func TestRuleExtractionRejectsNegatedCause(t *testing.T) {
	extractor := NewEntityExtractor(nil)
	relations, _, err := extractor.ExtractWithExecution(context.Background(), "患者否认服用降压药后出现体位性低血压", true, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(relations) != 0 {
		t.Fatalf("negated relation=%+v", relations)
	}
}

func TestRuleExtractionNormalizesSynonymChain(t *testing.T) {
	extractor := NewEntityExtractor(nil)
	relations, _, err := extractor.ExtractWithExecution(context.Background(), "张奶奶使用血压药后发生直立性低血压，随后滑倒", true, false, false)
	if err != nil || len(relations) != 1 {
		t.Fatalf("relations=%#v err=%v", relations, err)
	}
	if relations[0].Property != "直立性低血压" || relations[0].Result != "滑倒" {
		t.Fatalf("relation=%+v", relations[0])
	}
}

func TestGenericCausalExtractionHandlesUncataloguedMedicalChain(t *testing.T) {
	extractor := NewEntityExtractor(nil)
	relations, _, err := extractor.ExtractWithExecution(context.Background(), "王奶奶因长期卧床导致肌肉萎缩，双腿无力难以行走", true, false, false)
	if err != nil || len(relations) != 1 {
		t.Fatalf("relations=%#v err=%v", relations, err)
	}
	relation := relations[0]
	if relation.Object != "王奶奶" || relation.Mediator != "长期卧床" || relation.Property != "肌肉萎缩" || relation.Result != "双腿无力" {
		t.Fatalf("relation=%+v", relation)
	}
	if relation.Quality == nil || !relation.Quality.TupleValid || len(relation.EvidenceSpans) < 4 {
		t.Fatalf("audit quality=%+v spans=%+v", relation.Quality, relation.EvidenceSpans)
	}
}

func TestGenericCausalExtractionHandlesSecondEdgeTrigger(t *testing.T) {
	extractor := NewEntityExtractor(nil)
	relations, _, err := extractor.ExtractWithExecution(context.Background(), "刘奶奶高血压未控制，脑血管压力增高引发脑卒中", true, false, false)
	if err != nil || len(relations) != 1 {
		t.Fatalf("relations=%#v err=%v", relations, err)
	}
	relation := relations[0]
	if relation.Mediator != "高血压未控制" || relation.Property != "脑血管压力增高" || relation.Result != "脑卒中" {
		t.Fatalf("relation=%+v", relation)
	}
}
