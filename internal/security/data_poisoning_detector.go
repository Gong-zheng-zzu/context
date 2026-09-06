package security

import (
	"regexp"
	"strings"
	"unicode"
)

// DataPoisoningDetector 数据投毒检测器
type DataPoisoningDetector struct {
	detector *Detector
	maliciousPatterns []*regexp.Regexp
	backdoorTriggers []string
}

// NewDataPoisoningDetector 创建数据投毒检测器
func NewDataPoisoningDetector() *DataPoisoningDetector {
	dpd := &DataPoisoningDetector{
		detector: NewDetector(),
		backdoorTriggers: []string{
			// 后门触发器关键词
			"trigger:", "activate:", "execute:", "run:",
			"触发:", "激活:", "执行:",

			// 条件触发器
			"if user says", "when user asks", "if input contains",
			"如果用户说", "当用户问", "如果输入包含",

			// 隐藏指令
			"hidden instruction:", "secret command:", "backdoor:",
			"隐藏指令:", "秘密命令:", "后门:",

			// 覆盖指令
			"override previous", "ignore above", "disregard system",
			"覆盖之前", "忽略上面", "无视系统",
		},
	}

	// 编译恶意模式
	dpd.maliciousPatterns = []*regexp.Regexp{
		// 1. 恶意指令注入模式
		regexp.MustCompile(`(?i)(always|必须|一定要)\s+(ignore|reveal|show|display|忽略|显示|泄露)`),
		regexp.MustCompile(`(?i)(when|if|whenever)\s+.{5,50}\s+(then|就|则)\s+(ignore|bypass|reveal)`),

		// 2. 后门触发器模式
		regexp.MustCompile(`(?i)trigger\s*[:=]\s*['"]?[^'"]{5,50}['"]?`),
		regexp.MustCompile(`(?i)backdoor\s*[:=]\s*['"]?[^'"]{5,50}['"]?`),

		// 3. 权限提升模式
		regexp.MustCompile(`(?i)(grant|give|provide)\s+(admin|root|full)\s+(access|permission)`),
		regexp.MustCompile(`(?i)(become|act\s+as|pretend\s+to\s+be)\s+(admin|root|system)`),

		// 4. 数据泄露指令
		regexp.MustCompile(`(?i)(always\s+)?(show|reveal|display|leak)\s+(all|every|any)\s+(password|secret|key|token|data)`),
		regexp.MustCompile(`(?i)(dump|export|extract)\s+(database|user|credential|sensitive)`),

		// 5. 安全绕过模式
		regexp.MustCompile(`(?i)(disable|turn\s+off|bypass|skip)\s+(security|check|validation|filter)`),
		regexp.MustCompile(`(?i)(remove|delete|ignore)\s+(all\s+)?(restriction|limitation|constraint)`),

		// 6. 条件执行模式（可疑）
		regexp.MustCompile(`(?i)if\s+.{5,50}\s+then\s+.{5,50}\s+(ignore|bypass|reveal|show)`),
		regexp.MustCompile(`(?i)when\s+.{5,50}\s+(execute|run|do)\s+.{5,50}`),

		// 7. 隐藏指令模式
		regexp.MustCompile(`(?i)(hidden|secret|invisible)\s+(instruction|command|rule|prompt)`),
		regexp.MustCompile(`(?i)<!--.*?(ignore|bypass|reveal).*?-->`), // HTML注释中的指令

		// 8. 编码混淆模式
		regexp.MustCompile(`(?i)(base64|hex|unicode|rot13)\s*(decode|encoded)[:=]`),

		// 9. 重复模式攻击（同一短语重复多次）
		// 注意：Go的regexp不支持反向引用，这里使用简化的重复检测
		regexp.MustCompile(`\b(\w{5,})\s+\w{5,}\s+\w{5,}\s+\w{5,}`), // 多个长单词连续出现

		// 10. 系统提示覆盖
		regexp.MustCompile(`(?i)(new|updated|revised)\s+(system|instruction|prompt|rule)`),
		regexp.MustCompile(`(?i)(replace|override|change)\s+(original|previous|system)\s+(prompt|instruction)`),

		// 11. 角色扮演注入
		regexp.MustCompile(`(?i)from\s+now\s+on,?\s+you\s+are\s+.{5,50}`),
		regexp.MustCompile(`(?i)pretend\s+(you\s+are|to\s+be)\s+.{5,50}`),

		// 12. 多语言混合攻击
		regexp.MustCompile(`[a-zA-Z]{3,}\s*[一-龥ぁ-んァ-ヶ]{2,}\s*[a-zA-Z]{3,}`),

		// 13. SQL/代码注入
		regexp.MustCompile(`(?i)(union|select|insert|update|delete)\s+.{5,50}\s+(from|into|where)`),
		regexp.MustCompile(`(?i)(eval|exec|system|shell)\s*\(`),

		// 14. XSS注入
		regexp.MustCompile(`<script[^>]*>.*?</script>`),
		regexp.MustCompile(`javascript:\s*[a-z]+\(`),

		// 15. 命令注入
		regexp.MustCompile(`[;|&]\s*(cat|ls|pwd|whoami|rm|del|format)`),
	}

	return dpd
}

// ValidateKnowledgeInput 验证知识库输入内容
// 返回：是否有效、不通过的原因列表
func (dpd *DataPoisoningDetector) ValidateKnowledgeInput(content string) (isValid bool, reasons []string) {
	reasons = []string{}

	// 1. 检查内容长度
	if len(content) == 0 {
		reasons = append(reasons, "内容为空")
		return false, reasons
	}

	if len(content) > 100000 { // 限制单次输入最大10万字符
		reasons = append(reasons, "内容过长（超过10万字符），可能是攻击")
	}

	// 2. 检测恶意指令注入
	for i, pattern := range dpd.maliciousPatterns {
		if pattern.MatchString(content) {
			match := pattern.FindString(content)
			if len(match) > 100 {
				match = match[:100] + "..."
			}
			reasons = append(reasons, "检测到恶意模式 #"+string(rune(i+1))+": "+match)
		}
	}

	// 3. 检测后门触发器
	lowerContent := strings.ToLower(content)
	for _, trigger := range dpd.backdoorTriggers {
		if strings.Contains(lowerContent, strings.ToLower(trigger)) {
			reasons = append(reasons, "检测到后门触发器关键词: "+trigger)
		}
	}

	// 4. 检测重复模式攻击
	if dpd.detectRepetitionAttack(content) {
		reasons = append(reasons, "检测到重复模式攻击（同一内容重复过多）")
	}

	// 5. 检测敏感信息泄露尝试
	sensitiveInfos := dpd.detector.Detect(content)
	if len(sensitiveInfos) > 10 { // 如果包含超过10个敏感信息，可疑
		reasons = append(reasons, "包含过多敏感信息（可能是数据泄露尝试）")
	}

	// 6. 检测异常字符比例
	if dpd.detectAbnormalCharacters(content) {
		reasons = append(reasons, "包含异常字符比例（可能是编码混淆）")
	}

	// 7. 检测零宽字符（隐藏内容）
	if dpd.detectZeroWidthCharacters(content) {
		reasons = append(reasons, "检测到零宽字符（可能隐藏恶意内容）")
	}

	// 8. 检测Unicode同形字符攻击
	if dpd.detectHomoglyphAttack(content) {
		reasons = append(reasons, "检测到Unicode同形字符攻击")
	}

	// 9. 检测过度嵌套结构
	if dpd.detectExcessiveNesting(content) {
		reasons = append(reasons, "检测到过度嵌套结构（可能是混淆攻击）")
	}

	// 10. 检测Prompt注入尝试
	injectionDetector := NewPromptInjectionDetector()
	if isInjection, confidence, reason := injectionDetector.DetectInjection(content); isInjection && confidence > 0.7 {
		reasons = append(reasons, "检测到Prompt注入尝试: "+reason)
	}

	// 判断是否有效
	isValid = len(reasons) == 0
	return isValid, reasons
}

// detectRepetitionAttack 检测重复模式攻击
func (dpd *DataPoisoningDetector) detectRepetitionAttack(content string) bool {
	// 检测短语重复
	words := strings.Fields(content)
	if len(words) < 10 {
		return false
	}

	// 统计连续重复
	maxRepeat := 1
	currentRepeat := 1
	for i := 1; i < len(words); i++ {
		if words[i] == words[i-1] {
			currentRepeat++
			if currentRepeat > maxRepeat {
				maxRepeat = currentRepeat
			}
		} else {
			currentRepeat = 1
		}
	}

	// 如果同一单词连续重复5次以上，判定为攻击
	if maxRepeat >= 5 {
		return true
	}

	// 检测整体重复度
	uniqueWords := make(map[string]bool)
	for _, word := range words {
		uniqueWords[strings.ToLower(word)] = true
	}

	uniqueRatio := float64(len(uniqueWords)) / float64(len(words))
	// 如果唯一词汇比例低于30%，可疑
	return uniqueRatio < 0.3
}

// detectAbnormalCharacters 检测异常字符
func (dpd *DataPoisoningDetector) detectAbnormalCharacters(content string) bool {
	totalChars := 0
	abnormalChars := 0

	for _, r := range content {
		totalChars++
		// 检测控制字符、特殊符号等
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			abnormalChars++
		}
		// 检测私有使用区字符
		if r >= 0xE000 && r <= 0xF8FF {
			abnormalChars++
		}
	}

	if totalChars == 0 {
		return false
	}

	abnormalRatio := float64(abnormalChars) / float64(totalChars)
	// 如果异常字符超过5%，判定为可疑
	return abnormalRatio > 0.05
}

// detectZeroWidthCharacters 检测零宽字符
func (dpd *DataPoisoningDetector) detectZeroWidthCharacters(content string) bool {
	zeroWidthChars := []rune{
		0x200B, // 零宽空格
		0x200C, // 零宽非连接符
		0x200D, // 零宽连接符
		0xFEFF, // 零宽非断空格
	}

	count := 0
	for _, r := range content {
		for _, zw := range zeroWidthChars {
			if r == zw {
				count++
				break
			}
		}
	}

	// 如果包含超过3个零宽字符，可疑
	return count > 3
}

// detectHomoglyphAttack 检测同形字符攻击
func (dpd *DataPoisoningDetector) detectHomoglyphAttack(content string) bool {
	// 检测常见的同形字符对
	homoglyphs := map[rune][]rune{
		'a': {0x0430}, // 西里尔字母 а
		'e': {0x0435}, // 西里尔字母 е
		'o': {0x043E}, // 西里尔字母 о
		'p': {0x0440}, // 西里尔字母 р
		'c': {0x0441}, // 西里尔字母 с
		'x': {0x0445}, // 西里尔字母 х
	}

	suspiciousCount := 0
	for _, r := range content {
		for _, homoglyphList := range homoglyphs {
			for _, h := range homoglyphList {
				if r == h {
					suspiciousCount++
					break
				}
			}
		}
	}

	// 如果包含超过5个同形字符，可疑
	return suspiciousCount > 5
}

// detectExcessiveNesting 检测过度嵌套
func (dpd *DataPoisoningDetector) detectExcessiveNesting(content string) bool {
	// 检测括号嵌套深度
	maxDepth := 0
	currentDepth := 0

	for _, r := range content {
		if r == '(' || r == '[' || r == '{' {
			currentDepth++
			if currentDepth > maxDepth {
				maxDepth = currentDepth
			}
		} else if r == ')' || r == ']' || r == '}' {
			currentDepth--
		}
	}

	// 如果嵌套深度超过10层，可疑
	return maxDepth > 10
}

// ValidateBatch 批量验证多个输入
func (dpd *DataPoisoningDetector) ValidateBatch(contents []string) map[int][]string {
	results := make(map[int][]string)

	for i, content := range contents {
		isValid, reasons := dpd.ValidateKnowledgeInput(content)
		if !isValid {
			results[i] = reasons
		}
	}

	return results
}

// GetRiskScore 获取内容的风险评分（0-100，越高越危险）
func (dpd *DataPoisoningDetector) GetRiskScore(content string) int {
	score := 0

	// 检测各种风险因素
	isValid, reasons := dpd.ValidateKnowledgeInput(content)
	if isValid {
		return 0
	}

	// 根据检测到的问题数量计算风险分数
	score = len(reasons) * 15

	// 检测严重问题，额外加分
	for _, reason := range reasons {
		lowerReason := strings.ToLower(reason)
		if strings.Contains(lowerReason, "后门") || strings.Contains(lowerReason, "backdoor") {
			score += 30
		}
		if strings.Contains(lowerReason, "注入") || strings.Contains(lowerReason, "injection") {
			score += 25
		}
		if strings.Contains(lowerReason, "恶意") || strings.Contains(lowerReason, "malicious") {
			score += 20
		}
	}

	// 限制在0-100范围内
	if score > 100 {
		score = 100
	}

	return score
}
