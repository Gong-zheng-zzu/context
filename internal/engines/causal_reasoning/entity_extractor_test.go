package causal_reasoning

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/contextkeeper/service/internal/llm"
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

func TestExtractionFallbackReasonDistinguishesModelTimeout(t *testing.T) {
	if got := extractionFallbackReason(context.DeadlineExceeded); got != "model_timeout" {
		t.Fatalf("fallback reason=%q, want model_timeout", got)
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

func TestExtractWithExecutionSkipsConfiguredLLMWhenRulesAreComplete(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		t.Fatalf("LLM endpoint called for a complete deterministic candidate")
	}))
	defer server.Close()

	client, err := llm.NewOllamaLocalClient(&llm.LLMConfig{BaseURL: server.URL, Model: "qwen2.5:3b"})
	if err != nil {
		t.Fatalf("NewOllamaLocalClient() error = %v", err)
	}
	extractor := NewEntityExtractor(client.(*llm.OllamaLocalClient))
	relations, execution, err := extractor.ExtractWithExecution(context.Background(), "李爷爷服用降压药后出现体位性低血压并在洗手间滑倒", true, false, true)
	if err != nil || len(relations) != 1 {
		t.Fatalf("relations=%#v err=%v", relations, err)
	}
	if called {
		t.Fatal("configured model should not run when rules already verify the tuple")
	}
	if execution.Mode != "rules" || execution.ModelStatus != "not_needed" || execution.FallbackReason != "rules_sufficient" || execution.ModelTier != "rules" {
		t.Fatalf("execution=%+v, want audited rules-first state", execution)
	}
}

func TestExtractWithExecutionUsesNormalizedInputCache(t *testing.T) {
	extractor := NewEntityExtractor(nil)
	text := "患者服用降压药后出现体位性低血压"

	first, firstExecution, err := extractor.ExtractWithExecution(context.Background(), text, true, false, false)
	if err != nil {
		t.Fatalf("first extraction error = %v", err)
	}
	if firstExecution.CacheHit {
		t.Fatal("first extraction unexpectedly reported cache hit")
	}
	if len(first) == 0 {
		t.Fatal("first extraction returned no relations")
	}

	second, secondExecution, err := extractor.ExtractWithExecution(context.Background(), "患者 服用降压药后出现体位性低血压", true, false, false)
	if err != nil {
		t.Fatalf("second extraction error = %v", err)
	}
	if !secondExecution.CacheHit {
		t.Fatal("second extraction did not report cache hit")
	}
	if len(second) != len(first) || second[0].CandidateID != first[0].CandidateID {
		t.Fatalf("cached relations = %#v, first = %#v", second, first)
	}
}

func TestExtractWithExecutionCacheSeparatesExecutionFlags(t *testing.T) {
	extractor := NewEntityExtractor(nil)
	text := "患者服用降压药后出现体位性低血压"
	if _, _, err := extractor.ExtractWithExecution(context.Background(), text, true, false, false); err != nil {
		t.Fatalf("seed extraction error = %v", err)
	}
	_, execution, err := extractor.ExtractWithExecution(context.Background(), text, true, true, false)
	if err != nil {
		t.Fatalf("different flags extraction error = %v", err)
	}
	if execution.CacheHit {
		t.Fatal("different execution flags reused cached result")
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

func TestNormalizeLLMExtractionUsesLaterUnnegatedMention(t *testing.T) {
	source := "张奶奶否认头晕。今晚服用安眠药后出现头晕，随后跌倒。"
	normalized, _, ok := normalizeLLMExtraction(source, LLMExtractionResult{
		Object:   "张奶奶",
		Mediator: "服用安眠药",
		Property: "头晕",
		Result:   "跌倒",
		Evidence: []string{"今晚服用安眠药后出现头晕，随后跌倒。"},
	})
	if !ok {
		t.Fatal("valid later positive evidence was rejected because an earlier occurrence was negated")
	}
	if normalized.Property != "头晕" || normalized.Result != "跌倒" {
		t.Fatalf("normalized relation = %+v", normalized)
	}
}

func TestNormalizeLLMExtractionStillRejectsOnlyNegatedEvidence(t *testing.T) {
	source := "张奶奶否认头晕，未见跌倒。"
	if _, _, ok := normalizeLLMExtraction(source, LLMExtractionResult{
		Object:   "张奶奶",
		Mediator: "安眠药",
		Property: "头晕",
		Result:   "跌倒",
		Evidence: []string{source},
	}); ok {
		t.Fatal("relation supported only by negated evidence was accepted")
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

func TestPCCMFusionDoesNotBoostDuplicateEvidence(t *testing.T) {
	engine := NewDefaultPCCMFusionEngine()
	one := engine.FuseConfidence(0.9, 0, 0, 1)
	many := engine.FuseConfidence(0.9, 0, 0, 20)
	if one != many {
		t.Fatalf("duplicate evidence changed confidence: one=%v many=%v", one, many)
	}
}

func TestRuleCandidateExposesFieldScoresAndDecision(t *testing.T) {
	extractor := NewEntityExtractor(nil)
	relations, _, err := extractor.ExtractWithExecution(context.Background(), "患者服用降压药后出现体位性低血压并有跌倒风险", true, false, false)
	if err != nil || len(relations) == 0 {
		t.Fatalf("extract rule candidate: relations=%#v err=%v", relations, err)
	}
	relation := relations[0]
	if relation.CandidateID == "" || relation.Decision == "" || len(relation.FieldScores) != 4 {
		t.Fatalf("candidate audit is incomplete: %#v", relation)
	}
	if relation.PCCMEvidence == nil || relation.PCCMEvidence.CalibrationVersion != "pccm-c-frozen-v1" {
		t.Fatalf("calibration evidence is incomplete: %#v", relation.PCCMEvidence)
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

func TestRuleExtractionDoesNotLetHistoricalNegationSuppressLaterObservedChain(t *testing.T) {
	extractor := NewEntityExtractor(nil)
	text := "患者否认既往服用降压药。今日服用安眠药后出现意识模糊，随后跌倒。"
	relations, _, err := extractor.ExtractWithExecution(context.Background(), text, true, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(relations) != 1 {
		t.Fatalf("relations=%#v, want exactly the later observed chain", relations)
	}
	relation := relations[0]
	if relation.Mediator != "服用安眠药" || relation.Property != "意识模糊" || relation.Result != "跌倒" {
		t.Fatalf("relation=%+v, want observed sedative chain", relation)
	}
	if relation.Quality == nil || !relation.Quality.TupleValid || !relation.Quality.NegationChecked {
		t.Fatalf("quality=%+v, want verified non-negated chain", relation.Quality)
	}
}

func TestGenericCausalExtractionRejectsCandidateWhoseMediatorContainsExplicitNegation(t *testing.T) {
	extractor := NewEntityExtractor(nil)
	relations, _, err := extractor.ExtractWithExecution(context.Background(), "王奶奶否认长期卧床导致肌肉萎缩，双腿无力难以行走", true, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(relations) != 0 {
		t.Fatalf("negated generic candidate=%+v, want no causal relation", relations)
	}
}

func TestGenericCausalExtractionRejectsUnadministeredMedicationCandidate(t *testing.T) {
	extractor := NewEntityExtractor(nil)
	relations, _, err := extractor.ExtractWithExecution(context.Background(), "患者未服用抗生素导致腹泻，随后脱水", true, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(relations) != 0 {
		t.Fatalf("unadministered medication candidate=%+v, want no causal relation", relations)
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

func TestEvidenceSpansRoundTripWithChineseRunesAndSeparators(t *testing.T) {
	text := "患者服用降压药后出现体位性低血压，然后跌倒"
	relation := CausalRelation{
		Object: "患者", Mediator: "服用降压药", Property: "体位性低血压", Result: "跌倒",
	}
	spans := spansForRelation(text, relation)
	if len(spans) != 3 {
		t.Fatalf("spans=%+v, want mediator/property/result spans", spans)
	}
	runes := []rune(text)
	for _, span := range spans {
		if span.Start < 0 || span.End <= span.Start || span.End > len(runes) {
			t.Fatalf("invalid rune offsets: %+v (runes=%d)", span, len(runes))
		}
		if got := string(runes[span.Start:span.End]); got != span.Text {
			t.Fatalf("span %s does not round trip: got %q offsets=%d:%d", span.Field, got, span.Start, span.End)
		}
	}
}
