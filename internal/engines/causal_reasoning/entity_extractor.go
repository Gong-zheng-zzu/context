package causal_reasoning

import (
	"context"
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

	ee.AttachAuditEvidence(relations, execution)
	return relations, execution, nil
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
			RuleConfidence:  relation.RuleConfidence,
			PMIConfidence:   relation.PMIConfidence,
			LLMConfidence:   relation.LLMConfidence,
			Weights:         execution.PCCMWeights,
			ActiveSources:   active,
			EvidenceCount:   evidenceCount,
			FinalConfidence: relation.Confidence,
		}
	}
}

func matchingRuleEvidence(relation CausalRelation, rules []RuleEvidence) []RuleEvidence {
	matched := make([]RuleEvidence, 0, len(rules))
	condition := strings.ToLower(relation.Mediator)
	if condition == "" {
		condition = strings.ToLower(relation.Property)
	}
	effect := strings.ToLower(relation.Property)
	if effect == "" {
		effect = strings.ToLower(relation.Result)
	}
	for _, rule := range rules {
		if strings.Contains(condition, strings.ToLower(rule.Condition)) && strings.Contains(effect+strings.ToLower(relation.Result), strings.ToLower(rule.Effect)) {
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
		})
	}
	sort.Slice(evidence, func(i, j int) bool { return evidence[i].ID < evidence[j].ID })
	return evidence
}

// buildExtractionPrompt 构建因果关系抽取提示词
var (
	markdownJSONFence   = regexp.MustCompile("(?s)^```(json)?\\s*|\\s*```$")
	fieldPrefix         = regexp.MustCompile(`^(object|mediator|property|result|o|c|p|r|对象|患者|中介|诱因|机制|属性|结果)\\s*[:：]\\s*`)
	trailingPunctuation = regexp.MustCompile(`[。；;，,、\\s]+$`)
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
	matchedRules := ee.ruleEngine.MatchRules(text)
	if len(matchedRules) == 0 {
		return []CausalRelation{}, nil
	}

	var relations []CausalRelation
	for _, rule := range matchedRules {
		relation := CausalRelation{
			Object:         "患者", // 规则引擎无法精确识别对象
			Mediator:       rule.Condition,
			Property:       "",
			Result:         rule.Effect,
			RuleConfidence: rule.Confidence,
			PMIConfidence:  0.0,
			LLMConfidence:  0.0,
			Evidence:       []string{text},
			Timestamp:      time.Now(),
		}

		// 仅使用规则置信度
		relation.Confidence = rule.Confidence

		relations = append(relations, relation)
	}

	return relations, nil
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

// RecordDocument 记录文档用于PMI统计
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
