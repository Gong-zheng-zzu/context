package security

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/sirupsen/logrus"
)

// HallucinationDetector AI幻觉检测器
type HallucinationDetector struct {
	logger *logrus.Logger
}

// NewHallucinationDetector 创建幻觉检测器
func NewHallucinationDetector(logger *logrus.Logger) *HallucinationDetector {
	return &HallucinationDetector{
		logger: logger,
	}
}

// HallucinationCheckResult 幻觉检测结果
type HallucinationCheckResult struct {
	IsLikelyHallucination bool     // 是否可能是幻觉
	Confidence            float64  // 置信度 (0-1)
	Reasons               []string // 判定原因
	RiskLevel             string   // 风险等级: low/medium/high
	SuggestedAction       string   // 建议操作
}

// DetectHallucination 检测AI回复是否可能包含幻觉
func (h *HallucinationDetector) DetectHallucination(
	aiResponse string,
	retrievedMemory string,
	userQuery string,
) *HallucinationCheckResult {
	result := &HallucinationCheckResult{
		IsLikelyHallucination: false,
		Confidence:            0.0,
		Reasons:               []string{},
		RiskLevel:             "low",
		SuggestedAction:       "无需操作",
	}

	// 检测1: 是否包含具体数据但没有引用来源
	if h.containsSpecificData(aiResponse) && !h.hasSourceCitation(aiResponse) {
		result.Reasons = append(result.Reasons, "回复包含具体数据但未标注来源")
		result.Confidence += 0.3
	}

	// 检测2: 回复中的数据是否在检索到的记忆中
	if retrievedMemory != "" {
		if !h.dataExistsInMemory(aiResponse, retrievedMemory) {
			result.Reasons = append(result.Reasons, "回复中的数据未在检索到的记忆中找到")
			result.Confidence += 0.4
		}
	} else {
		// 没有检索到记忆，但AI给出了具体数据
		if h.containsSpecificData(aiResponse) {
			result.Reasons = append(result.Reasons, "无相关记忆但AI给出了具体数据")
			result.Confidence += 0.5
		}
	}

	// 检测3: 是否包含过度自信的表述
	if h.containsOverconfidentPhrases(aiResponse) {
		result.Reasons = append(result.Reasons, "包含过度自信的表述")
		result.Confidence += 0.2
	}

	// 检测4: 是否包含医疗建议但没有免责声明
	if h.containsMedicalAdvice(aiResponse) && !h.hasDisclaimer(aiResponse) {
		result.Reasons = append(result.Reasons, "包含医疗建议但缺少免责声明")
		result.Confidence += 0.1
	}

	// 检测5: 是否包含不一致的信息
	if h.containsInconsistentInfo(aiResponse) {
		result.Reasons = append(result.Reasons, "回复中包含不一致的信息")
		result.Confidence += 0.3
	}

	// 判定是否为幻觉
	if result.Confidence >= 0.5 {
		result.IsLikelyHallucination = true
	}

	// 确定风险等级
	if result.Confidence >= 0.7 {
		result.RiskLevel = "high"
		result.SuggestedAction = "拒绝输出，要求AI重新生成并标注来源"
	} else if result.Confidence >= 0.4 {
		result.RiskLevel = "medium"
		result.SuggestedAction = "添加警告标签，提示用户核实信息"
	} else {
		result.RiskLevel = "low"
		result.SuggestedAction = "正常输出"
	}

	return result
}

// containsSpecificData 检测是否包含具体数据（数字、日期等）
func (h *HallucinationDetector) containsSpecificData(text string) bool {
	// 检测健康数据模式
	patterns := []string{
		`血压[:：]\s*\d+/\d+`,                    // 血压: 120/80
		`体温[:：]\s*\d+\.?\d*`,                  // 体温: 36.5
		`血糖[:：]\s*\d+\.?\d*`,                  // 血糖: 5.6
		`心率[:：]\s*\d+`,                       // 心率: 72
		`\d{4}[-/年]\d{1,2}[-/月]\d{1,2}[日号]?`, // 日期
		`ALT[:：]\s*\d+`,                       // 肝功能指标
		`AST[:：]\s*\d+`,
		`肌酐[:：]\s*\d+`,
		`尿酸[:：]\s*\d+`,
	}

	for _, pattern := range patterns {
		matched, _ := regexp.MatchString(pattern, text)
		if matched {
			return true
		}
	}

	return false
}

// hasSourceCitation 检测是否有来源标注
func (h *HallucinationDetector) hasSourceCitation(text string) bool {
	// 检测来源标注模式
	citationPatterns := []string{
		`根据.*记录`,
		`根据.*数据`,
		`根据.*检索`,
		`根据.*记忆`,
		`\[来源[:：].*\]`,
		`\[记录时间[:：].*\]`,
		`根据\d{4}年\d{1,2}月\d{1,2}日的记录`,
		`上次记录显示`,
		`历史数据显示`,
	}

	for _, pattern := range citationPatterns {
		matched, _ := regexp.MatchString(pattern, text)
		if matched {
			return true
		}
	}

	return false
}

// dataExistsInMemory 检测回复中的数据是否在记忆中存在
func (h *HallucinationDetector) dataExistsInMemory(aiResponse, memory string) bool {
	// 提取AI回复中的数字
	numberPattern := regexp.MustCompile(`\d+\.?\d*`)
	aiNumbers := numberPattern.FindAllString(aiResponse, -1)

	if len(aiNumbers) == 0 {
		return true // 没有具体数字，无法判断
	}

	// 检查这些数字是否在记忆中出现
	matchCount := 0
	for _, num := range aiNumbers {
		if strings.Contains(memory, num) {
			matchCount++
		}
	}

	// 如果超过50%的数字在记忆中找到，认为数据可信
	return float64(matchCount)/float64(len(aiNumbers)) >= 0.5
}

// containsOverconfidentPhrases 检测过度自信的表述
func (h *HallucinationDetector) containsOverconfidentPhrases(text string) bool {
	overconfidentPhrases := []string{
		"一定是",
		"肯定是",
		"绝对是",
		"毫无疑问",
		"百分之百",
		"完全可以确定",
		"我确信",
		"我保证",
	}

	for _, phrase := range overconfidentPhrases {
		if strings.Contains(text, phrase) {
			return true
		}
	}

	return false
}

// containsMedicalAdvice 检测是否包含医疗建议
func (h *HallucinationDetector) containsMedicalAdvice(text string) bool {
	medicalAdvicePatterns := []string{
		"建议.*服用",
		"建议.*用药",
		"需要.*治疗",
		"应该.*吃药",
		"可以.*停药",
		"建议.*就医",
		"诊断为",
		"可能是.*病",
	}

	for _, pattern := range medicalAdvicePatterns {
		matched, _ := regexp.MatchString(pattern, text)
		if matched {
			return true
		}
	}

	return false
}

// hasDisclaimer 检测是否有免责声明
func (h *HallucinationDetector) hasDisclaimer(text string) bool {
	disclaimerPhrases := []string{
		"建议咨询医生",
		"请联系医生",
		"仅供参考",
		"不能替代医生诊断",
		"请及时就医",
		"建议到医院",
	}

	for _, phrase := range disclaimerPhrases {
		if strings.Contains(text, phrase) {
			return true
		}
	}

	return false
}

// containsInconsistentInfo 检测是否包含不一致的信息
func (h *HallucinationDetector) containsInconsistentInfo(text string) bool {
	// 检测矛盾的表述
	contradictions := [][]string{
		{"正常", "异常"},
		{"偏高", "偏低"},
		{"升高", "降低"},
		{"增加", "减少"},
	}

	for _, pair := range contradictions {
		if strings.Contains(text, pair[0]) && strings.Contains(text, pair[1]) {
			// 简单检测：如果同时出现矛盾词汇，可能存在不一致
			// 注意：这是简化版本，实际需要更复杂的语义分析
			return true
		}
	}

	return false
}

// EnhancePromptWithSourceRequirement 增强提示词，要求AI标注来源
func (h *HallucinationDetector) EnhancePromptWithSourceRequirement(originalPrompt string) string {
	sourceRequirement := `

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
⚠️ 【强制要求：来源标注】
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

当你回答包含具体数据（血压、体温、血糖、日期等）时，必须标注来源：

✅ 正确示例：
"根据2024年1月15日的记录，张奶奶的血压是130/85。"
"历史数据显示，最近一周的平均体温是36.8℃。"

❌ 错误示例：
"张奶奶的血压是130/85。"（缺少来源）
"最近体温正常。"（缺少具体数据和来源）

如果【相关记忆】中没有相关数据，必须明确告知：
"抱歉，我没有查询到相关的健康数据记录。建议使用系统的查询功能获取最新数据。"

禁止编造任何数据！如果不确定，宁可说"不知道"。

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
`

	return originalPrompt + sourceRequirement
}

// GenerateWarningMessage 生成警告消息
func (h *HallucinationDetector) GenerateWarningMessage(result *HallucinationCheckResult) string {
	if !result.IsLikelyHallucination {
		return ""
	}

	warning := fmt.Sprintf("⚠️ AI幻觉风险警告（风险等级：%s）\n", result.RiskLevel)
	warning += "检测到以下问题：\n"
	for i, reason := range result.Reasons {
		warning += fmt.Sprintf("%d. %s\n", i+1, reason)
	}
	warning += fmt.Sprintf("\n建议操作：%s\n", result.SuggestedAction)
	warning += "\n请核实AI回复中的信息，特别是具体的数据和日期。"

	return warning
}
