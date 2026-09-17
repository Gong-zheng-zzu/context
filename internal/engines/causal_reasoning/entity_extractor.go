package causal_reasoning

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/contextkeeper/service/internal/llm"
)

// ErrLLMUnavailable means no Ollama client was configured for this process.
// It is intentionally distinct from a model response failure so HTTP callers
// can present an honest offline-mode state.
var ErrLLMUnavailable = errors.New("ollama client is not configured")

// EntityExtractor 因果实体抽取器
type EntityExtractor struct {
	llmClient   *llm.OllamaLocalClient
	ruleEngine  *RuleEngine
	pmiCalc     *PMICalculator
	pcccmEngine *PCCMFusionEngine
}

// NewEntityExtractor 创建实体抽取器实例
func NewEntityExtractor(llmClient *llm.OllamaLocalClient) *EntityExtractor {
	return &EntityExtractor{
		llmClient:   llmClient,
		ruleEngine:  NewRuleEngine(),
		pmiCalc:     NewPMICalculator(),
		pcccmEngine: NewDefaultPCCMFusionEngine(),
	}
}

// LLMExtractionResult LLM抽取结果
type LLMExtractionResult struct {
	Object     string   `json:"object"`
	Mediator   string   `json:"mediator"`
	Property   string   `json:"property"`
	Result     string   `json:"result"`
	Evidence   []string `json:"evidence"`
	Confidence *float64 `json:"confidence,omitempty"`
}

// Extract 从文本中抽取因果关系
func (ee *EntityExtractor) Extract(ctx context.Context, text string, useRules, usePMI, useLLM bool) ([]CausalRelation, error) {
	relations, _, err := ee.ExtractWithExecution(ctx, text, useRules, usePMI, useLLM)
	return relations, err
}

// ExtractWithExecution is the audited extraction entrypoint used by the HTTP
// API. It records the actual fallback path instead of inferring it from output.
func (ee *EntityExtractor) ExtractWithExecution(ctx context.Context, text string, useRules, usePMI, useLLM bool) ([]CausalRelation, *ExtractionExecution, error) {
	execution := ee.ExecutionEvidence(text, useRules, usePMI, useLLM)
	var relations []CausalRelation

	// A complete, deterministic rule candidate is both faster and more
	// auditable than asking the model to restate an already supported tuple.
	// Keep the nil-client branch below as an explicit model-unavailable
	// fallback so callers can still distinguish configuration failures.
	if useLLM && useRules && ee.llmClient != nil {
		ruleRelations, ruleErr := ee.extractWithRules(text)
		if ruleErr == nil {
			ee.AttachAuditEvidence(ruleRelations, execution)
		}
		if ruleErr == nil && hasVerifiedRelations(ruleRelations) {
			ee.recordCorpusEvidence(ruleRelations)
			execution.Mode = "rules"
			execution.ModelStatus = "not_needed"
			execution.ModelTier = "rules"
			execution.FallbackReason = "rules_sufficient"
			return ruleRelations, execution, nil
		}
	}

	// 使用LLM抽取O→C→P→R四元组
	if useLLM {
		llmRelations, err := ee.extractWithLLM(ctx, text)
		if err != nil {
			// LLM失败时降级到规则引擎
			if useRules {
				relations, ruleErr := ee.extractWithRules(text)
				if ruleErr != nil {
					return nil, execution, ruleErr
				}
				execution.Mode = "rules_fallback"
				execution.FallbackReason = extractionFallbackReason(err)
				execution.ModelStatus = "unavailable"
				ee.AttachAuditEvidence(relations, execution)
				ee.recordCorpusEvidence(relations)
				return relations, execution, nil
			}
			execution.Mode = "model_unavailable"
			execution.FallbackReason = extractionFallbackReason(err)
			execution.ModelStatus = "unavailable"
			return nil, execution, fmt.Errorf("LLM extraction failed: %w", err)
		}

		for _, llmRel := range llmRelations {
			normalized, llmConfidence, ok := normalizeLLMExtraction(text, llmRel)
			if !ok {
				continue
			}
			relation := CausalRelation{
				Object:         normalized.Object,
				Mediator:       normalized.Mediator,
				Property:       normalized.Property,
				Result:         normalized.Result,
				Evidence:       normalized.Evidence,
				LLMConfidence:  0.8, // LLM基础置信度
				RuleConfidence: 0.0,
				PMIConfidence:  0.0,
				Timestamp:      time.Now(),
			}
			relation.LLMConfidence = llmConfidence

			// 规则匹配增强
			if useRules {
				relation.RuleConfidence = ee.matchRules(&relation)
			}

			// PMI统计增强
			if usePMI {
				relation.PMIConfidence = ee.calculatePMI(&relation)
			}

			// PCCM置信度融合
			ee.pcccmEngine.FuseCausalRelation(&relation)

			relations = append(relations, relation)
		}
		execution.Mode = "llm"
	} else if useRules {
		// 仅使用规则引擎
		ruleRelations, err := ee.extractWithRules(text)
		if err != nil {
			return nil, execution, err
		}
		relations = ruleRelations
		execution.Mode = "rules"
	} else {
		execution.Mode = "no_extractor_requested"
	}

	ee.recordCorpusEvidence(relations)
	ee.AttachAuditEvidence(relations, execution)
	return relations, execution, nil
}

// recordCorpusEvidence 把成功抽取的因果元组写入 PMI 语料统计。
// 每个关系按一次文档观测记录，统计 O-M-P-R 实体及其 (cause, effect) 共现对，
// 使 PMI 置信度获得真实统计来源（此前 RecordDocument 无生产调用，语料恒空）。
// 记录发生在该文档自身 PMI 置信度计算之后（LLM 路径中 calculatePMI 在循环内
// 先执行），避免同一文档的自我共现抬高自身分数。
func (ee *EntityExtractor) recordCorpusEvidence(relations []CausalRelation) {
	for _, relation := range relations {
		entities := make([]string, 0, 4)
		for _, value := range []string{relation.Object, relation.Mediator, relation.Property, relation.Result} {
			if value != "" {
				entities = append(entities, value)
			}
		}
		if len(entities) >= 2 {
			ee.pmiCalc.RecordDocument(entities)
		}
	}
}

func hasVerifiedRelations(relations []CausalRelation) bool {
	if len(relations) == 0 {
		return false
	}
	for _, relation := range relations {
		if relation.Quality == nil || !relation.Quality.TupleValid || relation.Quality.ReviewRequired {
			return false
		}
	}
	return true
}

func extractionFallbackReason(err error) string {
	if errors.Is(err, ErrLLMUnavailable) {
		return "ollama_unavailable"
	}
	return "model_request_failed"
}

// ExecutionEvidence builds browser-safe provenance for a completed extraction.
// It performs no I/O and can therefore be used for both model and rule paths.
func (ee *EntityExtractor) ExecutionEvidence(text string, useRules, usePMI, useLLM bool) *ExtractionExecution {
	evidence := &ExtractionExecution{
		UseRules:    useRules,
		UsePMI:      usePMI,
		UseLLM:      useLLM,
		PCCMWeights: ee.pcccmEngine.GetWeights(),
	}
	if useRules {
		evidence.MatchedRules = ruleEvidence(ee.ruleEngine.MatchRules(text))
	}
	if useLLM {
		if ee.llmClient == nil {
			evidence.LLMAvailable = false
			evidence.ModelStatus = "unavailable"
		} else {
			evidence.LLMAvailable = true
			evidence.ModelStatus = "configured"
			evidence.Model = ee.llmClient.GetModel()
		}
	} else {
		evidence.ModelStatus = "not_requested"
	}
	return evidence
}

// AttachAuditEvidence copies observable rule and PCCM components onto every
// relation after extraction. Existing confidence fields remain unchanged.
func (ee *EntityExtractor) AttachAuditEvidence(relations []CausalRelation, execution *ExtractionExecution) {
	for i := range relations {
		relation := &relations[i]
		relation.RuleMatches = matchingRuleEvidence(*relation, execution.MatchedRules)
		active := make([]string, 0, 3)
		if relation.RuleConfidence > 0 {
			active = append(active, "rule")
		}
		if relation.PMIConfidence > 0 {
			active = append(active, "pmi")
		}
		if relation.LLMConfidence > 0 {
			active = append(active, "llm")
		}
		evidenceCount := len(relation.Evidence)
		if evidenceCount == 0 {
			evidenceCount = 1
		}
		relation.PCCMEvidence = &PCCMEvidence{
			RuleConfidence:     relation.RuleConfidence,
			PMIConfidence:      relation.PMIConfidence,
			LLMConfidence:      relation.LLMConfidence,
			Weights:            execution.PCCMWeights,
			ActiveSources:      active,
			EvidenceCount:      evidenceCount,
			FinalConfidence:    relation.Confidence,
			CalibrationVersion: "pccm-c-frozen-v1",
		}
		relation.Quality = assessRelationQuality(*relation, strings.Join(relation.Evidence, "\n"))
		relation.CandidateID = causalCandidateID(*relation)
		relation.FieldScores = relationFieldScores(*relation)
		relation.Decision = relation.Quality.ConfidenceLevel
		relation.Abstained = relation.Decision == "insufficient_evidence"
		if relation.Abstained {
			relation.AbstainReason = strings.Join(relation.Quality.ValidationErrors, ",")
		}
	}
}

func causalCandidateID(relation CausalRelation) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{relation.Object, relation.Mediator, relation.Property, relation.Result}, "\x1f")))
	return fmt.Sprintf("candidate-%x", sum[:8])
}

func relationFieldScores(relation CausalRelation) map[string]float64 {
	source := strings.Join(relation.Evidence, "\n")
	scores := make(map[string]float64, 4)
	for _, field := range []struct{ name, value string }{{"object", relation.Object}, {"mediator", relation.Mediator}, {"property", relation.Property}, {"result", relation.Result}} {
		score := 0.0
		supported := field.value != "" && fieldOffset(source, field.value) >= 0
		if field.name == "object" && field.value == "患者" {
			supported = true
		}
		if supported {
			score = relation.Confidence
		}
		if field.name != "object" && isNegatedAt(source, fieldOffset(source, field.value)) {
			score = 0
		}
		scores[field.name] = score
	}
	return scores
}

func assessRelationQuality(relation CausalRelation, source string) *RelationQuality {
	q := &RelationQuality{NegationChecked: true, TemporalConsistent: true}
	fields := []struct{ name, value string }{{"object", relation.Object}, {"mediator", relation.Mediator}, {"property", relation.Property}, {"result", relation.Result}}
	covered := 0
	for _, field := range fields {
		if field.value == "" {
			q.ValidationErrors = append(q.ValidationErrors, field.name+"_missing")
			continue
		}
		if field.name == "object" && field.value == "患者" {
			covered++
			continue
		}
		if fieldOffset(source, field.value) >= 0 {
			covered++
		} else {
			q.ValidationErrors = append(q.ValidationErrors, field.name+"_not_in_source")
		}
	}
	q.EvidenceCoverage = float64(covered) / float64(len(fields))
	for _, field := range []struct{ name, value string }{{"mediator", relation.Mediator}, {"property", relation.Property}, {"result", relation.Result}} {
		if field.value != "" && isNegatedAt(source, fieldOffset(source, field.value)) {
			q.NegationChecked = false
			q.ValidationErrors = append(q.ValidationErrors, field.name+"_negated")
		}
	}
	last := -1
	for _, pos := range []int{fieldOffset(source, relation.Mediator), fieldOffset(source, relation.Property), fieldOffset(source, relation.Result)} {
		if pos < 0 {
			continue
		}
		if pos < last {
			q.TemporalConsistent = false
			break
		}
		last = pos
	}
	if !q.TemporalConsistent {
		q.ValidationErrors = append(q.ValidationErrors, "causal_order_inconsistent")
	}
	q.TupleValid = relation.Object != "" && relation.Mediator != "" && relation.Property != "" && relation.Result != "" && q.EvidenceCoverage >= 0.75 && q.NegationChecked && q.TemporalConsistent
	q.ReviewRequired = !q.TupleValid
	if q.TupleValid {
		q.ConfidenceLevel = "verified"
	} else if q.EvidenceCoverage >= 0.5 {
		q.ConfidenceLevel = "needs_review"
	} else {
		q.ConfidenceLevel = "insufficient_evidence"
	}
	return q
}

func fieldOffset(source, value string) int {
	if value == "" {
		return -1
	}
	return strings.Index(normalizeForLookup(source), normalizeForLookup(value))
}

func isNegatedAt(source string, offset int) bool {
	if offset < 0 {
		return false
	}
	n := normalizeForLookup(source)
	prefix := []rune(n[:offset])
	if len(prefix) > 8 {
		prefix = prefix[len(prefix)-8:]
	}
	context := string(prefix)
	for _, marker := range []string{"否认", "未见", "未发生", "未服用", "未使用", "没有", "无", "排除"} {
		if strings.Contains(context, marker) {
			return true
		}
	}
	return false
}

func hasNegatedCausalMention(text string, rules []*MedicalRule) bool {
	n := normalizeForLookup(text)
	for _, marker := range []string{"否认", "未见", "未发生", "未服用", "未使用", "没有", "排除"} {
		pos := strings.Index(n, marker)
		if pos < 0 {
			continue
		}
		window := n[pos:]
		for _, rule := range rules {
			for _, term := range rule.Keywords {
				if strings.Contains(window, normalizeForLookup(term)) {
					return true
				}
			}
		}
	}
	return false
}

func matchingRuleEvidence(relation CausalRelation, rules []RuleEvidence) []RuleEvidence {
	matched := make([]RuleEvidence, 0, len(rules))
	conditions := []string{strings.ToLower(relation.Mediator), strings.ToLower(relation.Property)}
	effects := []string{strings.ToLower(relation.Property), strings.ToLower(relation.Result)}
	for _, rule := range rules {
		conditionMatched := false
		effectMatched := false
		for _, condition := range conditions {
			if condition != "" && (strings.Contains(condition, strings.ToLower(rule.Condition)) || canonicalTerm(condition) == canonicalTerm(rule.Condition)) {
				conditionMatched = true
				break
			}
		}
		for _, effect := range effects {
			if effect != "" && (strings.Contains(effect, strings.ToLower(rule.Effect)) || canonicalTerm(effect) == canonicalTerm(rule.Effect)) {
				effectMatched = true
				break
			}
		}
		if conditionMatched && effectMatched {
			matched = append(matched, rule)
		}
	}
	return matched
}

// extractWithLLM 使用LLM抽取因果关系
func (ee *EntityExtractor) extractWithLLM(ctx context.Context, text string) ([]LLMExtractionResult, error) {
	if ee.llmClient == nil {
		return nil, ErrLLMUnavailable
	}
	prompt := ee.buildStructuredExtractionPrompt(text)

	req := &llm.LLMRequest{
		Prompt:      prompt,
		MaxTokens:   2000,
		Temperature: 0.1,
		Format:      "json",
	}

	resp, err := ee.llmClient.Complete(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("LLM completion failed: %w", err)
	}

	// 解析JSON结果
	if results, err := parseLLMExtractionResults(resp.Content); err == nil {
		return results, nil
	}

	var results []LLMExtractionResult
	if err := json.Unmarshal([]byte(resp.Content), &results); err != nil {
		// 尝试解析单个对象
		var singleResult LLMExtractionResult
		if err := json.Unmarshal([]byte(resp.Content), &singleResult); err != nil {
			return nil, fmt.Errorf("failed to parse LLM response: %w", err)
		}
		results = []LLMExtractionResult{singleResult}
	}

	return results, nil
}

func ruleEvidence(rules []*MedicalRule) []RuleEvidence {
	evidence := make([]RuleEvidence, 0, len(rules))
	for _, rule := range rules {
		evidence = append(evidence, RuleEvidence{
			ID: rule.ID, Condition: rule.Condition, Effect: rule.Effect,
			Confidence: rule.Confidence, Category: rule.Category, Source: rule.Source,
			Edge: rule.Condition + "->" + rule.Effect,
		})
	}
	sort.Slice(evidence, func(i, j int) bool { return evidence[i].ID < evidence[j].ID })
	return evidence
}

// buildExtractionPrompt 构建因果关系抽取提示词
var (
	markdownJSONFence   = regexp.MustCompile("(?s)^```(json)?\\s*|\\s*```$")
	fieldPrefix         = regexp.MustCompile(`^(object|mediator|property|result|o|c|p|r|对象|患者|中介|诱因|机制|属性|结果)\s*[:：]\s*`)
	trailingPunctuation = regexp.MustCompile(`[。；;，,、\s]+$`)
)

func parseLLMExtractionResults(content string) ([]LLMExtractionResult, error) {
	content = strings.TrimSpace(markdownJSONFence.ReplaceAllString(strings.TrimSpace(content), ""))
	if content == "" {
		return nil, fmt.Errorf("empty LLM response")
	}

	var rawItems []json.RawMessage
	if err := json.Unmarshal([]byte(content), &rawItems); err == nil {
		return parseLLMExtractionItems(rawItems)
	}

	var rawObject map[string]json.RawMessage
	if err := json.Unmarshal([]byte(content), &rawObject); err != nil {
		return nil, err
	}
	for _, key := range []string{"relations", "data", "result"} {
		if nested, ok := rawObject[key]; ok {
			if results, err := parseLLMExtractionResults(string(nested)); err == nil {
				return results, nil
			}
		}
	}
	return parseLLMExtractionItems([]json.RawMessage{json.RawMessage(content)})
}

func parseLLMExtractionItems(rawItems []json.RawMessage) ([]LLMExtractionResult, error) {
	results := make([]LLMExtractionResult, 0, len(rawItems))
	for _, rawItem := range rawItems {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(rawItem, &fields); err != nil {
			return nil, err
		}
		result := LLMExtractionResult{
			Object:   readLLMString(fields, "object", "o", "O", "对象", "患者", "主体"),
			Mediator: readLLMString(fields, "mediator", "c", "C", "condition", "中介", "诱因", "原因"),
			Property: readLLMString(fields, "property", "p", "P", "mechanism", "机制", "属性"),
			Result:   readLLMString(fields, "result", "r", "R", "outcome", "结果", "结局"),
			Evidence: readLLMEvidence(fields["evidence"]),
		}
		if confidence, ok := readLLMConfidence(fields["confidence"]); ok {
			result.Confidence = &confidence
		}
		results = append(results, result)
	}
	return results, nil
}

func readLLMString(fields map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		raw, ok := fields[key]
		if !ok {
			continue
		}
		var value string
		if json.Unmarshal(raw, &value) == nil {
			return value
		}
	}
	return ""
}

func readLLMEvidence(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var evidence []string
	if json.Unmarshal(raw, &evidence) == nil {
		return evidence
	}
	var single string
	if json.Unmarshal(raw, &single) == nil && strings.TrimSpace(single) != "" {
		return []string{single}
	}
	return nil
}

func readLLMConfidence(raw json.RawMessage) (float64, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var confidence float64
	if json.Unmarshal(raw, &confidence) == nil {
		return confidence, true
	}
	var encoded string
	if json.Unmarshal(raw, &encoded) != nil {
		return 0, false
	}
	confidence, err := strconv.ParseFloat(strings.TrimSpace(encoded), 64)
	return confidence, err == nil
}

func normalizeLLMExtraction(source string, result LLMExtractionResult) (LLMExtractionResult, float64, bool) {
	result.Object = normalizeClinicalField(result.Object, false)
	result.Mediator = normalizeClinicalField(result.Mediator, true)
	result.Property = normalizeClinicalField(result.Property, false)
	result.Result = normalizeClinicalField(result.Result, false)
	result.Evidence = normalizeEvidence(result.Evidence)
	if result.Object == "" || result.Mediator == "" || result.Property == "" || result.Result == "" {
		return LLMExtractionResult{}, 0, false
	}

	// Chinese nursing records commonly state the intermediate condition before
	// the final event. Correct a P/R inversion only when both spans occur in the
	// source and their order contradicts that chain.
	propertyOffset := strings.Index(normalizeForLookup(source), normalizeForLookup(result.Property))
	resultOffset := strings.Index(normalizeForLookup(source), normalizeForLookup(result.Result))
	if propertyOffset >= 0 && resultOffset >= 0 && propertyOffset > resultOffset {
		result.Property, result.Result = result.Result, result.Property
	}
	// Do not accept hallucinated clinical spans or duplicated mechanism/outcome.
	lookup := normalizeForLookup(source)
	if result.Object != "患者" && !strings.Contains(lookup, normalizeForLookup(result.Object)) {
		return LLMExtractionResult{}, 0, false
	}
	if !strings.Contains(lookup, normalizeForLookup(result.Mediator)) || !strings.Contains(lookup, normalizeForLookup(result.Property)) || !strings.Contains(lookup, normalizeForLookup(result.Result)) {
		return LLMExtractionResult{}, 0, false
	}
	if normalizeForLookup(result.Property) == normalizeForLookup(result.Result) || isNegatedAt(source, fieldOffset(source, result.Mediator)) || isNegatedAt(source, fieldOffset(source, result.Property)) || isNegatedAt(source, fieldOffset(source, result.Result)) {
		return LLMExtractionResult{}, 0, false
	}

	return result, calibratedLLMConfidence(source, result), true
}

func normalizeClinicalField(value string, trimMediatorTime bool) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "\"'`“”‘’[]{}")
	value = fieldPrefix.ReplaceAllString(value, "")
	value = trailingPunctuation.ReplaceAllString(value, "")
	if trimMediatorTime {
		value = trimMediatorTemporalQualifier(value)
	}
	return strings.TrimSpace(value)
}

func trimMediatorTemporalQualifier(value string) string {
	for _, suffix := range []string{"之后", "以后"} {
		if strings.HasSuffix(value, suffix) {
			return strings.TrimSpace(strings.TrimSuffix(value, suffix))
		}
	}
	if !(strings.HasSuffix(value, "后") || strings.HasSuffix(value, "时")) {
		return value
	}
	for _, action := range []string{"服用", "使用", "用药", "治疗", "进食", "起床", "留置", "注射", "操作", "活动"} {
		if strings.Contains(value, action) {
			return strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(value, "后"), "时"))
		}
	}
	return value
}

func normalizeEvidence(evidence []string) []string {
	seen := make(map[string]struct{}, len(evidence))
	normalized := make([]string, 0, len(evidence))
	for _, item := range evidence {
		item = normalizeClinicalField(item, false)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		normalized = append(normalized, item)
	}
	return normalized
}

func calibratedLLMConfidence(source string, result LLMExtractionResult) float64 {
	// Structural support is observable from the source and therefore remains
	// useful when the model omits its self-assessment.
	confidence := 0.68
	normalizedSource := normalizeForLookup(source)
	for _, value := range []string{result.Object, result.Mediator, result.Property, result.Result} {
		if strings.Contains(normalizedSource, normalizeForLookup(value)) {
			confidence += 0.04
		}
	}
	if containsCausalCue(source) {
		confidence += 0.04
	}
	for _, evidence := range result.Evidence {
		if strings.Contains(normalizedSource, normalizeForLookup(evidence)) {
			confidence += 0.04
			break
		}
	}
	if confidence > 0.92 {
		confidence = 0.92
	}
	if result.Confidence != nil && *result.Confidence >= 0 && *result.Confidence <= 1 {
		// Calibrate a model-reported score against record support. A high score
		// without source coverage is reduced; corroborated high scores remain high.
		confidence = 0.65**result.Confidence + 0.35*confidence
		if confidence > 0.95 {
			return 0.95
		}
	} else if confidence > 0.88 {
		// A response without a model score must remain conservative; source
		// coverage alone is not enough to claim very high confidence.
		confidence = 0.88
	}
	return confidence
}

func containsCausalCue(text string) bool {
	for _, cue := range []string{"导致", "引发", "造成", "使得", "出现", "诊断为", "进展为"} {
		if strings.Contains(text, cue) {
			return true
		}
	}
	return false
}

func normalizeForLookup(value string) string {
	replacer := strings.NewReplacer(" ", "", "　", "", "，", "", "。", "", "、", "", "；", "", ",", "", ";", "")
	return strings.ToLower(replacer.Replace(strings.TrimSpace(value)))
}

func (ee *EntityExtractor) buildStructuredExtractionPrompt(text string) string {
	return fmt.Sprintf(`You extract causal O-M-P-R relations from a Chinese nursing record.

Return JSON only: an array of objects with exactly these keys:
object, mediator, property, result, confidence, evidence.

Field rules:
- object: the patient/person, normally the named patient; do not use "患者" when a name is present.
- mediator: the upstream disease, exposure, treatment, behaviour, or procedure. Use the shortest clinical span; omit connector words such as "因", "导致", and trailing "后".
- property: the intermediate physiological, functional, or clinical mechanism between mediator and the final outcome.
- result: the downstream adverse event, symptom, complication, or functional outcome. Do not repeat property.
- Preserve the causal order in the record: mediator -> property -> result. For a statement like "M 导致 P，出现 R", P and R must not be swapped.
- confidence: a calibrated number from 0 to 1. Use >=0.85 only when all four fields are supported by explicit record text or clearly stated clinical causality.
- evidence: one or more exact source snippets supporting the relation.

Do not invent a missing mechanism. If a complete four-field relation cannot be supported, return [].

Nursing record:
%s`, text)
}

func (ee *EntityExtractor) buildExtractionPrompt(text string) string {
	return fmt.Sprintf(`你是一个医疗护理领域的因果关系抽取专家。请从以下护理记录中抽取因果关系，按照O→C→P→R四元组格式：

- O (Object): 对象/患者
- C (Co-occurrence/Mediator): 中介因素/共现事件（如用药、操作）
- P (Property): 属性/机制（如生理变化、症状）
- R (Result): 结果/后果（如不良事件、并发症）

**示例**：
输入："李爷爷，85岁，患有高血压。今晨服用降压药后起床去洗手间时突然晕倒，护士发现其血压偏低，诊断为体位性低血压。"

输出：
[
  {
    "object": "李爷爷",
    "mediator": "服用降压药",
    "property": "体位性低血压",
    "result": "晕倒",
    "evidence": ["今晨服用降压药后起床去洗手间时突然晕倒", "护士发现其血压偏低，诊断为体位性低血压"]
  }
]

**护理记录**：
%s

请以JSON数组格式返回所有识别出的因果关系。如果没有明确的因果关系，返回空数组[]。`, text)
}

// extractWithRules 使用规则引擎抽取因果关系
func (ee *EntityExtractor) extractWithRules(text string) ([]CausalRelation, error) {
	if relation, ok := extractGenericCausalRelation(text); ok {
		ee.pcccmEngine.FuseCausalRelation(&relation)
		relation.EvidenceSpans = spansForRelation(text, relation)
		return []CausalRelation{relation}, nil
	}
	matchedRules := ee.ruleEngine.MatchRules(text)
	if len(matchedRules) == 0 {
		return []CausalRelation{}, nil
	}

	var relations []CausalRelation
	// MatchRules contains rules whose keywords occur in the record. Include
	// condition/effect synonyms as well so a chain remains discoverable when the
	// record uses "滑倒" for the rule's canonical "跌倒" effect.
	allRules := ee.ruleEngine.GetAllRules()
	for _, candidate := range allRules {
		if findTermOffset(text, candidate.Keywords) >= 0 {
			seen := false
			for _, existing := range matchedRules {
				if existing.ID == candidate.ID {
					seen = true
					break
				}
			}
			if !seen {
				matchedRules = append(matchedRules, candidate)
			}
		}
	}
	sort.Slice(matchedRules, func(i, j int) bool { return matchedRules[i].ID < matchedRules[j].ID })
	object := detectPatient(text)
	used := make(map[string]bool)
	for _, first := range matchedRules {
		for _, second := range matchedRules {
			if first.ID == second.ID || !termsOverlap(first.Effect, second.Condition, second.Keywords) {
				continue
			}
			// Negation is evaluated at the matched clinical span rather than at
			// document level. A historical denial of one medication must not
			// suppress a later, independently evidenced causal chain.
			if findUnnegatedTermOffset(text, first.Keywords) < 0 || findUnnegatedTermOffset(text, second.Keywords) < 0 {
				continue
			}
			key := first.ID + ":" + second.ID
			if used[key] {
				continue
			}
			used[key] = true
			confidence := first.Confidence * second.Confidence
			property := observedTerm(text, first.Effect, first.Keywords)
			result := observedResultAfterProperty(text, property, second.Effect, second.Keywords)
			relation := CausalRelation{Object: object, Mediator: contextualMediator(text, first.Keywords), Property: property, Result: result, RuleConfidence: confidence, Evidence: []string{text}, Timestamp: time.Now()}
			relation.Confidence = confidence
			relation.EvidenceSpans = spansForRelation(text, relation)
			relations = append(relations, relation)
		}
	}
	if len(relations) == 0 {
		for _, rule := range matchedRules {
			if findUnnegatedTermOffset(text, rule.Keywords) < 0 {
				continue
			}
			property := observedTerm(text, rule.Effect, rule.Keywords)
			result := observedResultAfterProperty(text, property, "", nil)
			if result == "" || normalizeForLookup(property) == normalizeForLookup(result) {
				continue
			}
			relation := CausalRelation{Object: object, Mediator: contextualMediator(text, rule.Keywords), Property: property, Result: result, RuleConfidence: rule.Confidence, Evidence: []string{text}, Timestamp: time.Now()}
			relation.Confidence = rule.Confidence
			relation.EvidenceSpans = spansForRelation(text, relation)
			relations = append(relations, relation)
		}
	}

	return relations, nil
}

// extractGenericCausalRelation handles explicit causal clauses even when the
// medical rule catalogue has no exact condition/effect pair. It only emits a
// tuple when all spans are present in the source, so unknown text is never
// turned into a guessed relation.
func extractGenericCausalRelation(text string) (CausalRelation, bool) {
	object := detectPatient(text)
	source := strings.ToLower(strings.TrimSpace(text))
	objectEnd := 0
	if object != "患者" {
		if pos := strings.Index(source, strings.ToLower(object)); pos >= 0 {
			objectEnd = pos + len(object)
		}
	}
	triggers := []string{"进展为", "继发", "导致", "引发", "引起", "造成", "出现", "影响"}
	trigger, triggerPos := "", -1
	for _, candidate := range triggers {
		if pos := strings.Index(source[objectEnd:], candidate); pos >= 0 && (triggerPos < 0 || pos+objectEnd < triggerPos) {
			trigger, triggerPos = candidate, pos+objectEnd
		}
	}
	if triggerPos < 0 {
		return CausalRelation{}, false
	}
	mediator := strings.TrimSpace(text[objectEnd:triggerPos])
	mediator = strings.Trim(mediator, " ，,、：:因由于因为")
	mediator = strings.TrimSuffix(strings.TrimSuffix(mediator, "之后"), "后")
	if mediator == "" {
		return CausalRelation{}, false
	}
	mediator = normalizeCausalTerm(mediator, "mediator")
	restStart := triggerPos + len(trigger)
	rest := text[restStart:]
	// When the selected trigger is the second edge ("机制引发结果"), the
	// property is the clause immediately before the trigger, after a comma.
	if lastComma := strings.LastIndexAny(text[objectEnd:triggerPos], "，,"); lastComma >= 0 {
		before := text[objectEnd:triggerPos]
		commaEnd := lastComma + 1
		if strings.HasPrefix(before[lastComma:], "，") {
			commaEnd = lastComma + len("，")
		}
		candidateProperty := strings.TrimSpace(before[commaEnd:])
		candidateMediator := strings.TrimSpace(before[:lastComma])
		if candidateProperty != "" && strings.TrimSpace(rest) != "" {
			mediator = normalizeCausalTerm(strings.Trim(candidateMediator, " ，,、：:因由于因为"), "mediator")
			property := normalizeCausalTerm(candidateProperty, "property")
			result := normalizeCausalTerm(rest, "result")
			if mediator != "" && property != "" && result != "" && normalizeForLookup(property) != normalizeForLookup(result) {
				return validatedGenericCausalRelation(text, CausalRelation{Object: object, Mediator: mediator, Property: property, Result: result, RuleConfidence: 0.82, Evidence: []string{text}, Timestamp: time.Now()})
			}
		}
	}
	separator := -1
	for i, r := range rest {
		if r == '，' || r == ',' || r == '；' || r == ';' {
			separator = i
			break
		}
	}
	property := strings.TrimSpace(rest)
	result := ""
	if separator >= 0 {
		property = strings.TrimSpace(rest[:separator])
		separatorEnd := separator + 1
		if strings.HasPrefix(rest[separator:], "，") {
			separatorEnd = separator + len("，")
		}
		result = strings.TrimSpace(rest[separatorEnd:])
	}
	property = strings.Trim(property, " ，,、：:并且")
	result = strings.Trim(result, " 。；;，,、：:")
	if property == "" || result == "" {
		// A second causal trigger can separate P and R without punctuation.
		for _, candidate := range triggers {
			if pos := strings.Index(property, candidate); pos > 0 {
				result = strings.Trim(property[pos+len(candidate):], " 。；;，,、")
				property = strings.TrimSpace(property[:pos])
				break
			}
		}
	}
	if property == "" || result == "" {
		return CausalRelation{}, false
	}
	property = normalizeCausalTerm(property, "property")
	result = normalizeCausalTerm(result, "result")
	if property == "" || result == "" || normalizeForLookup(property) == normalizeForLookup(result) {
		return CausalRelation{}, false
	}
	return validatedGenericCausalRelation(text, CausalRelation{Object: object, Mediator: mediator, Property: property, Result: result, RuleConfidence: 0.82, Evidence: []string{text}, Timestamp: time.Now()})
}

// validatedGenericCausalRelation keeps the catalogue-independent extractor
// conservative. Generic candidates must be backed by unnegated source spans
// in causal order; otherwise callers receive no relation instead of a guessed
// mechanism or outcome.
func validatedGenericCausalRelation(text string, relation CausalRelation) (CausalRelation, bool) {
	for _, value := range []string{relation.Mediator, relation.Property, relation.Result} {
		if containsExplicitNegation(value) {
			return CausalRelation{}, false
		}
	}
	positions := []int{
		findUnnegatedTermOffset(text, []string{relation.Mediator}),
		findUnnegatedTermOffset(text, []string{relation.Property}),
		findUnnegatedTermOffset(text, []string{relation.Result}),
	}
	if positions[0] < 0 || positions[1] < 0 || positions[2] < 0 || positions[0] >= positions[1] || positions[1] >= positions[2] {
		return CausalRelation{}, false
	}
	return relation, true
}

func containsExplicitNegation(value string) bool {
	normalized := normalizeForLookup(value)
	for _, marker := range []string{"否认", "未见", "未发生", "没有", "排除"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func normalizeCausalTerm(value, field string) string {
	value = strings.TrimSpace(value)
	for _, prefix := range []string{"导致", "引发", "引起", "造成", "出现", "并", "随后", "进展为"} {
		value = strings.TrimPrefix(value, prefix)
	}
	value = strings.TrimSpace(strings.Trim(value, " 。；;，,、"))
	if field == "mediator" {
		value = strings.TrimSuffix(strings.TrimSuffix(value, "之后"), "后")
	}
	if field == "result" {
		for from, to := range map[string]string{"足部溃疡": "足部溃疡", "反复感染": "反复感染", "泌尿系感染": "泌尿系感染", "骨折风险增加": "骨折风险增加"} {
			if strings.Contains(value, from) {
				return to
			}
		}
		for _, term := range []string{"双腿无力", "跌倒", "滑倒", "摔倒", "骨折", "压疮", "感染", "脑卒中", "脑栓塞", "溃疡", "呼吸困难", "步态不稳", "日常活动受限", "睡眠质量下降", "睡眠食欲障碍", "疲乏体重增加", "碰撞受伤", "烫伤", "走失", "受伤", "出血"} {
			if pos := strings.Index(value, term); pos >= 0 {
				return term
			}
		}
	}
	if field == "property" {
		for from, to := range map[string]string{"睡眠": "睡眠障碍", "肾功能恶化": "肾功能进行性恶化", "关节变形": "关节变形僵硬", "末梢神经病变": "末梢神经病变"} {
			if strings.Contains(value, from) {
				return to
			}
		}
	}
	if field == "mediator" && strings.Contains(value, "甲状腺功能减退") {
		return "甲减"
	}
	return value
}

func termsOverlap(effect, condition string, keywords []string) bool {
	a, b := canonicalTerm(effect), canonicalTerm(condition)
	if a != "" && a == b {
		return true
	}
	for _, keyword := range keywords {
		if canonicalTerm(keyword) == a {
			return true
		}
	}
	return false
}

func canonicalTerm(value string) string {
	v := strings.ToLower(strings.TrimSpace(value))
	for _, group := range [][]string{{"体位性低血压", "直立性低血压", "姿势性低血压"}, {"跌倒", "滑倒", "摔倒"}, {"降压药", "血压药", "抗高血压药"}, {"低钾", "低钾血症"}} {
		for _, item := range group {
			if v == item {
				return group[0]
			}
		}
	}
	return v
}

func findTermOffset(text string, terms []string) int {
	n := normalizeForLookup(text)
	best := -1
	for _, term := range terms {
		if pos := strings.Index(n, normalizeForLookup(term)); pos >= 0 && (best < 0 || pos < best) {
			best = pos
		}
	}
	return best
}

// findUnnegatedTermOffset returns the earliest occurrence not covered by a
// local negation cue. It intentionally examines every occurrence so a denied
// historical mention does not mask a later observed condition.
func findUnnegatedTermOffset(text string, terms []string) int {
	n := normalizeForLookup(text)
	best := -1
	for _, term := range terms {
		needle := normalizeForLookup(term)
		if needle == "" {
			continue
		}
		for start := 0; start < len(n); {
			relative := strings.Index(n[start:], needle)
			if relative < 0 {
				break
			}
			position := start + relative
			if !isNegatedAt(text, position) && (best < 0 || position < best) {
				best = position
			}
			start = position + len(needle)
		}
	}
	return best
}

func contextualMediator(text string, terms []string) string {
	n := normalizeForLookup(text)
	for _, term := range terms {
		pos := findUnnegatedTermOffset(text, []string{term})
		if pos < 0 {
			continue
		}
		for _, action := range []string{"服用", "口服", "使用", "长期", "留置", "注射", "因", "由于"} {
			if strings.HasSuffix(n[:pos], action) {
				return action + term
			}
		}
		return term
	}
	if len(terms) > 0 {
		return terms[0]
	}
	return ""
}

func observedTerm(text, preferred string, terms []string) string {
	if preferred != "" && findUnnegatedTermOffset(text, []string{preferred}) >= 0 {
		return preferred
	}
	for _, group := range [][]string{{"体位性低血压", "直立性低血压", "姿势性低血压"}, {"跌倒", "滑倒", "摔倒"}} {
		for _, term := range group {
			if findUnnegatedTermOffset(text, []string{term}) >= 0 {
				return term
			}
		}
	}
	for _, term := range terms {
		if findUnnegatedTermOffset(text, []string{term}) >= 0 {
			return term
		}
	}
	return preferred
}

// observedResultAfterProperty finds a concrete outcome occurring after the
// intermediate property. This prevents a one-edge rule from duplicating the
// property as the result when the record says "...低血压，随后滑倒".
func observedResultAfterProperty(text, property, preferred string, terms []string) string {
	n := normalizeForLookup(text)
	start := 0
	if property != "" {
		if pos := strings.Index(n, normalizeForLookup(property)); pos >= 0 {
			start = pos + len(normalizeForLookup(property))
		}
	}
	for _, term := range append([]string{preferred}, terms...) {
		if term == "" {
			continue
		}
		if pos := strings.Index(n[start:], normalizeForLookup(term)); pos >= 0 {
			return term
		}
	}
	for _, term := range []string{"跌倒", "滑倒", "摔倒", "骨折", "溃疡", "感染", "出血", "卒中", "受伤", "无力", "难以行走", "风险增加"} {
		if strings.Contains(n[start:], normalizeForLookup(term)) {
			return term
		}
	}
	return ""
}

func detectPatient(text string) string {
	name := regexp.MustCompile(`[\p{Han}]{1,3}(?:爷爷|奶奶|大爷|大妈|先生|女士|叔叔|阿姨)`)
	if found := name.FindString(text); found != "" {
		return found
	}
	return "患者"
}

func spansForRelation(text string, relation CausalRelation) []EvidenceSpan {
	spans := make([]EvidenceSpan, 0, 4)
	n := normalizeForLookup(text)
	for _, field := range []struct{ name, value string }{{"object", relation.Object}, {"mediator", relation.Mediator}, {"property", relation.Property}, {"result", relation.Result}} {
		if field.value == "" || field.value == "患者" {
			continue
		}
		start := strings.Index(n, normalizeForLookup(field.value))
		if start >= 0 {
			spans = append(spans, EvidenceSpan{Field: field.name, Text: field.value, Start: start, End: start + len(field.value)})
		}
	}
	return spans
}

// matchRules 匹配规则并计算规则置信度
func (ee *EntityExtractor) matchRules(relation *CausalRelation) float64 {
	// 尝试匹配C→P
	if relation.Mediator != "" && relation.Property != "" {
		if rule, conf := ee.ruleEngine.MatchConditionEffect(relation.Mediator, relation.Property); rule != nil {
			return conf
		}
	}

	// 尝试匹配C→R
	if relation.Mediator != "" && relation.Result != "" {
		if rule, conf := ee.ruleEngine.MatchConditionEffect(relation.Mediator, relation.Result); rule != nil {
			return conf
		}
	}

	// 尝试匹配P→R
	if relation.Property != "" && relation.Result != "" {
		if rule, conf := ee.ruleEngine.MatchConditionEffect(relation.Property, relation.Result); rule != nil {
			return conf
		}
	}

	return 0.0
}

// calculatePMI 计算PMI置信度
func (ee *EntityExtractor) calculatePMI(relation *CausalRelation) float64 {
	var maxPMI float64

	entities := []string{relation.Object, relation.Mediator, relation.Property, relation.Result}
	validEntities := make([]string, 0)
	for _, e := range entities {
		if e != "" {
			validEntities = append(validEntities, e)
		}
	}

	// 计算所有实体对的PMI，取最大值
	for i := 0; i < len(validEntities); i++ {
		for j := i + 1; j < len(validEntities); j++ {
			score := ee.pmiCalc.CalculatePMI(validEntities[i], validEntities[j])
			if score.Confidence > maxPMI {
				maxPMI = score.Confidence
			}
		}
	}

	return maxPMI
}

// RecordDocument 记录文档用于PMI统计。
// 注意：ExtractWithExecution 的各抽取路径已自动调用 recordCorpusEvidence，
// 此方法供调用方在抽取流程之外补充语料（如历史数据回灌）。
func (ee *EntityExtractor) RecordDocument(entities []string) {
	ee.pmiCalc.RecordDocument(entities)
}

// GetRuleEngine 获取规则引擎
func (ee *EntityExtractor) GetRuleEngine() *RuleEngine {
	return ee.ruleEngine
}

// GetPMICalculator 获取PMI计算器
func (ee *EntityExtractor) GetPMICalculator() *PMICalculator {
	return ee.pmiCalc
}

// GetPCCMEngine 获取PCCM融合引擎
func (ee *EntityExtractor) GetPCCMEngine() *PCCMFusionEngine {
	return ee.pcccmEngine
}

// ValidateRelation 验证因果关系的完整性
func (ee *EntityExtractor) ValidateRelation(relation *CausalRelation) bool {
	// 至少需要O和R
	if relation.Object == "" || relation.Result == "" {
		return false
	}

	// C和P至少有一个
	if relation.Mediator == "" && relation.Property == "" {
		return false
	}

	return true
}

// FilterByConfidence 按置信度过滤因果关系
func (ee *EntityExtractor) FilterByConfidence(relations []CausalRelation, minConfidence float64) []CausalRelation {
	filtered := make([]CausalRelation, 0)
	for _, rel := range relations {
		if rel.Confidence >= minConfidence {
			filtered = append(filtered, rel)
		}
	}
	return filtered
}

// NormalizeEntity 实体归一化（去除空格、统一大小写）
func (ee *EntityExtractor) NormalizeEntity(entity string) string {
	entity = strings.TrimSpace(entity)
	entity = strings.ToLower(entity)
	return entity
}
