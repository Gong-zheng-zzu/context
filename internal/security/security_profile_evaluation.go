package security

import (
	"context"
	"fmt"
	"sort"
	"time"
)

const SecurityLabPipelineVersion = "security-ablation-v1"

var SecurityLabProfiles = []string{"regex_only", "asdf", "asdf_casia", "asdf_casia_pccm"}

type SecuritySpanEvidence struct {
	SensitiveType   SensitiveType  `json:"sensitive_type"`
	Start           int            `json:"start"`
	End             int            `json:"end"`
	Confidence      float64        `json:"confidence"`
	CoordinateSpace string         `json:"coordinate_space"`
	CASIA           *CASIAEvidence `json:"casia,omitempty"`
}

type SecurityLayerEvidence struct {
	Layer      string                 `json:"layer"`
	Status     string                 `json:"status"`
	Confidence float64                `json:"confidence"`
	MatchCount int                    `json:"match_count"`
	LatencyMS  float64                `json:"latency_ms"`
	Spans      []SecuritySpanEvidence `json:"spans"`
}

type SecurityProfileEvaluation struct {
	Profile         string                  `json:"profile"`
	Decision        string                  `json:"decision"`
	Reason          string                  `json:"reason"`
	Confidence      float64                 `json:"confidence"`
	SensitiveTypes  []SensitiveType         `json:"sensitive_types"`
	Layers          []SecurityLayerEvidence `json:"layers"`
	ASDF            ASDFAuditResult         `json:"asdf"`
	CASIA           *CASIAConfig            `json:"casia,omitempty"`
	PCCMS           *PCCMSecurityConfig     `json:"pccm_s,omitempty"`
	EarlyStopReason string                  `json:"early_stop_reason"`
	LatencyMS       float64                 `json:"latency_ms"`
	PipelineVersion string                  `json:"pipeline_version"`
}

// EvaluateSecurityProfile runs one server-owned ablation profile without
// persisting data or invoking the LLM. It never places candidate values in the
// returned evidence.
func (s *SecurityService) EvaluateSecurityProfile(_ context.Context, profile, text string) (SecurityProfileEvaluation, error) {
	started := time.Now()
	result := SecurityProfileEvaluation{
		Profile: profile, Layers: []SecurityLayerEvidence{}, SensitiveTypes: []SensitiveType{},
		PipelineVersion: SecurityLabPipelineVersion,
	}
	if s == nil || s.detector == nil || s.asdfFramework == nil {
		return result, fmt.Errorf("security evaluation pipeline is unavailable")
	}
	valid := false
	for _, candidate := range SecurityLabProfiles {
		if profile == candidate {
			valid = true
			break
		}
	}
	if !valid {
		return result, fmt.Errorf("unsupported security profile %q", profile)
	}

	normalized := text
	result.ASDF = ASDFAuditResult{
		NormalizedText: text, PipelineVersion: ASDFPipelineVersion,
		OriginalSHA256: asdfTextHash(text), NormalizedSHA256: asdfTextHash(text),
		AttackTypes: []string{}, NormalizationSteps: []ASDFNormalizationStep{}, ResidualAttackTypes: []string{},
	}
	if profile != "regex_only" {
		result.ASDF = s.asdfFramework.DefendAndNormalizeWithAudit(text)
		normalized = result.ASDF.NormalizedText
	}

	var items []SensitiveInfo
	if profile == "regex_only" || profile == "asdf" {
		layerStarted := time.Now()
		items = s.detector.Detect(normalized)
		coordinateSpace := "normalized"
		if profile == "regex_only" {
			coordinateSpace = "original"
		}
		result.Layers = append(result.Layers, securityLayerEvidence("regex", items, time.Since(layerStarted), coordinateSpace))
		result.EarlyStopReason = "profile_complete_after_regex"
	} else {
		if s.multiLayerDetector == nil || s.multiLayerDetector.casiaAlgorithm == nil {
			return result, fmt.Errorf("CASIA evaluation pipeline is unavailable")
		}
		casiaConfig := s.multiLayerDetector.casiaAlgorithm.GetConfiguration()
		result.CASIA = &casiaConfig
		layerStarted := time.Now()
		regexItems := s.multiLayerDetector.applyCASIA(normalized, s.multiLayerDetector.regexDetector.Detect(normalized), true)
		result.Layers = append(result.Layers, securityLayerEvidence("regex_casia", regexItems, time.Since(layerStarted), "normalized"))
		items = regexItems
		result.EarlyStopReason = "profile_complete_after_casia"

		if profile == "asdf_casia_pccm" {
			dictionaryStarted := time.Now()
			dictionaryItems := s.multiLayerDetector.applyCASIA(normalized, s.multiLayerDetector.dictMatcher.Match(normalized), true)
			result.Layers = append(result.Layers, securityLayerEvidence("dictionary_casia", dictionaryItems, time.Since(dictionaryStarted), "normalized"))

			contextStarted := time.Now()
			matches := s.multiLayerDetector.contextEngine.Analyze(normalized, []NERResult{})
			contextItems := make([]SensitiveInfo, 0, len(matches))
			for _, match := range matches {
				contextItems = append(contextItems, match.SensitiveInfo)
			}
			// 上下文规则层是推断型，不享受保底，避免"邮政编码"类正常文本被误判。
			contextItems = s.multiLayerDetector.applyCASIA(normalized, contextItems, false)
			result.Layers = append(result.Layers, securityLayerEvidence("context_casia", contextItems, time.Since(contextStarted), "normalized"))

			layers := []LayerResult{
				{LayerID: 1, Items: regexItems, Confidence: s.multiLayerDetector.calculateLayerConfidence(regexItems, 1)},
				{LayerID: 2, Items: dictionaryItems, Confidence: s.multiLayerDetector.calculateLayerConfidence(dictionaryItems, 2)},
				{LayerID: 4, Items: contextItems, Confidence: s.multiLayerDetector.calculateLayerConfidence(contextItems, 4)},
			}
			items = s.multiLayerDetector.mergeResults(layers)
			result.Confidence = s.multiLayerDetector.calculateConfidenceWithPCCM(layers)
			pccm := clonePCCMSecurityConfig(s.multiLayerDetector.pccmConfig)
			result.PCCMS = &pccm
			result.EarlyStopReason = "deterministic_lab_profile_excludes_llm"
		}
	}

	if result.Confidence == 0 {
		result.Confidence = averageSensitiveConfidence(items)
	}
	result.SensitiveTypes = uniqueSensitiveTypes(items)
	if len(items) > 0 {
		result.Decision = "redact"
		result.Reason = "sensitive_span_detected"
	} else if result.ASDF.IsAdversarial {
		result.Decision = "allow"
		result.Reason = "asdf_normalized_no_sensitive_span"
	} else {
		result.Decision = "allow"
		result.Reason = "no_sensitive_span"
	}
	result.LatencyMS = float64(time.Since(started).Microseconds()) / 1000
	return result, nil
}

func securityLayerEvidence(name string, items []SensitiveInfo, latency time.Duration, coordinateSpace string) SecurityLayerEvidence {
	evidence := SecurityLayerEvidence{Layer: name, Status: "ok", MatchCount: len(items), LatencyMS: float64(latency.Microseconds()) / 1000, Spans: []SecuritySpanEvidence{}}
	evidence.Confidence = averageSensitiveConfidence(items)
	for _, item := range items {
		evidence.Spans = append(evidence.Spans, SecuritySpanEvidence{
			SensitiveType: item.Type, Start: item.Start, End: item.End,
			Confidence: item.Confidence, CoordinateSpace: coordinateSpace, CASIA: item.CASIA,
		})
	}
	return evidence
}

func averageSensitiveConfidence(items []SensitiveInfo) float64 {
	if len(items) == 0 {
		return 0
	}
	total := 0.0
	for _, item := range items {
		total += item.Confidence
	}
	return total / float64(len(items))
}

func uniqueSensitiveTypes(items []SensitiveInfo) []SensitiveType {
	set := make(map[SensitiveType]struct{})
	for _, item := range items {
		set[item.Type] = struct{}{}
	}
	result := make([]SensitiveType, 0, len(set))
	for sensitiveType := range set {
		result = append(result, sensitiveType)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}
