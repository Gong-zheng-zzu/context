package security

import (
	"encoding/base64"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

// AdversarialDetector 对抗样本检测器
type AdversarialDetector struct {
	homoglyphMap map[rune]rune // 同形字符映射表
}

// NewAdversarialDetector 创建对抗样本检测器
func NewAdversarialDetector() *AdversarialDetector {
	ad := &AdversarialDetector{
		homoglyphMap: make(map[rune]rune),
	}

	// 初始化同形字符映射表
	ad.initHomoglyphMap()

	return ad
}

// initHomoglyphMap 初始化同形字符映射表
func (ad *AdversarialDetector) initHomoglyphMap() {
	// 西里尔字母 -> 拉丁字母
	homoglyphs := map[rune]rune{
		0x0430: 'a', // а -> a
		0x0435: 'e', // е -> e
		0x043E: 'o', // о -> o
		0x0440: 'p', // р -> p
		0x0441: 'c', // с -> c
		0x0445: 'x', // х -> x
		0x0443: 'y', // у -> y
		0x0456: 'i', // і -> i
		0x0458: 'j', // ј -> j
		0x0455: 's', // ѕ -> s

		// 希腊字母 -> 拉丁字母
		0x03B1: 'a', // α -> a
		0x03B5: 'e', // ε -> e
		0x03BF: 'o', // ο -> o
		0x03C1: 'p', // ρ -> p
		0x03C5: 'u', // υ -> u
		0x03C7: 'x', // χ -> x

		// 其他相似字符
		0x0131: 'i', // ı -> i (无点i)
		0x04CF: 'l', // ӏ -> l
	}

	ad.homoglyphMap = homoglyphs
}

// DetectAdversarial 检测对抗样本攻击
func (ad *AdversarialDetector) DetectAdversarial(input string) (isAdversarial bool, reason string) {
	// 1. 检测Unicode同形字符
	if hasHomoglyph, details := ad.detectHomoglyphs(input); hasHomoglyph {
		return true, "检测到Unicode同形字符攻击: " + details
	}

	// 2. 检测零宽字符
	if hasZeroWidth, details := ad.detectZeroWidthCharacters(input); hasZeroWidth {
		return true, "检测到零宽字符攻击: " + details
	}

	// 3. 检测Base64编码payload
	if hasBase64, details := ad.detectBase64Payload(input); hasBase64 {
		return true, "检测到Base64编码payload: " + details
	}

	// 4. 检测URL编码payload
	if hasURLEncoded, details := ad.detectURLEncodedPayload(input); hasURLEncoded {
		return true, "检测到URL编码payload: " + details
	}

	// 5. 检测Unicode编码混淆
	if hasUnicodeEscape, details := ad.detectUnicodeEscape(input); hasUnicodeEscape {
		return true, "检测到Unicode编码混淆: " + details
	}

	// 6. 检测反向文本
	if hasReversed, details := ad.detectReversedText(input); hasReversed {
		return true, "检测到反向文本攻击: " + details
	}

	// 7. 检测字符重叠
	if hasOverlap, details := ad.detectCharacterOverlap(input); hasOverlap {
		return true, "检测到字符重叠攻击: " + details
	}

	// 8. 检测不可见字符
	if hasInvisible, details := ad.detectInvisibleCharacters(input); hasInvisible {
		return true, "检测到不可见字符: " + details
	}

	// 9. 检测混合脚本攻击
	if hasMixedScript, details := ad.detectMixedScriptAttack(input); hasMixedScript {
		return true, "检测到混合脚本攻击: " + details
	}

	// 10. 检测同形词攻击
	if hasHomoglyphWord, details := ad.detectHomoglyphWords(input); hasHomoglyphWord {
		return true, "检测到同形词攻击: " + details
	}

	return false, ""
}

// detectHomoglyphs 检测同形字符
func (ad *AdversarialDetector) detectHomoglyphs(input string) (bool, string) {
	count := 0
	detectedChars := []rune{}

	for _, r := range input {
		if _, exists := ad.homoglyphMap[r]; exists {
			count++
			detectedChars = append(detectedChars, r)
			if count >= 3 { // 检测到3个以上同形字符
				return true, "发现多个同形字符"
			}
		}
	}

	if count > 0 {
		return true, "发现同形字符"
	}

	return false, ""
}

// detectZeroWidthCharacters 检测零宽字符
func (ad *AdversarialDetector) detectZeroWidthCharacters(input string) (bool, string) {
	zeroWidthChars := []rune{
		0x200B, // 零宽空格 (ZERO WIDTH SPACE)
		0x200C, // 零宽非连接符 (ZERO WIDTH NON-JOINER)
		0x200D, // 零宽连接符 (ZERO WIDTH JOINER)
		0xFEFF, // 零宽非断空格 (ZERO WIDTH NO-BREAK SPACE)
		0x180E, // 蒙古文元音分隔符
		0x2060, // 词连接符
		0x2061, // 函数应用
		0x2062, // 不可见乘号
		0x2063, // 不可见分隔符
		0x2064, // 不可见加号
	}

	count := 0
	for _, r := range input {
		for _, zw := range zeroWidthChars {
			if r == zw {
				count++
				break
			}
		}
	}

	if count > 0 {
		return true, "发现" + string(rune(count+'0')) + "个零宽字符"
	}

	return false, ""
}

// detectBase64Payload 检测Base64编码payload
func (ad *AdversarialDetector) detectBase64Payload(input string) (bool, string) {
	// 查找长Base64字符串（降低阈值以捕获较短的攻击payload）
	base64Pattern := regexp.MustCompile(`[A-Za-z0-9+/]{20,}={0,2}`)
	matches := base64Pattern.FindAllString(input, -1)

	for _, match := range matches {
		// 尝试解码
		decoded, err := base64.StdEncoding.DecodeString(match)
		if err == nil && len(decoded) > 0 {
			decodedStr := string(decoded)
			// 检查解码后的内容是否包含可疑关键词
			suspiciousKeywords := []string{
				"ignore", "bypass", "admin", "password", "token",
				"忽略", "绕过", "管理员", "密码",
			}

			lowerDecoded := strings.ToLower(decodedStr)
			for _, keyword := range suspiciousKeywords {
				if strings.Contains(lowerDecoded, keyword) {
					return true, "Base64编码内容包含可疑关键词: " + keyword
				}
			}

			// 检查是否包含脚本标签
			if strings.Contains(lowerDecoded, "<script") || strings.Contains(lowerDecoded, "javascript:") {
				return true, "Base64编码内容包含脚本代码"
			}
		}
	}

	return false, ""
}

// detectURLEncodedPayload 检测URL编码payload
func (ad *AdversarialDetector) detectURLEncodedPayload(input string) (bool, string) {
	// 检测URL编码模式 %XX（降低阈值以捕获较短的编码payload）
	urlEncodedPattern := regexp.MustCompile(`(?:%[0-9A-Fa-f]{2}){4,}`)
	if urlEncodedPattern.MatchString(input) {
		// 尝试解码
		decoded, err := url.QueryUnescape(input)
		if err == nil && decoded != input {
			// 检查解码后的内容
			suspiciousKeywords := []string{
				"<script", "javascript:", "onerror=", "onclick=",
				"ignore", "bypass", "admin",
			}

			lowerDecoded := strings.ToLower(decoded)
			for _, keyword := range suspiciousKeywords {
				if strings.Contains(lowerDecoded, keyword) {
					return true, "URL编码内容包含可疑关键词: " + keyword
				}
			}
		}
	}

	return false, ""
}

// detectUnicodeEscape 检测Unicode转义序列
func (ad *AdversarialDetector) detectUnicodeEscape(input string) (bool, string) {
	// 检测 \uXXXX 或 \xXX 模式
	unicodeEscapePattern := regexp.MustCompile(`(?:\\u[0-9A-Fa-f]{4}|\\x[0-9A-Fa-f]{2}){3,}`)
	if unicodeEscapePattern.MatchString(input) {
		return true, "发现Unicode转义序列"
	}

	return false, ""
}

// detectReversedText 检测反向文本
func (ad *AdversarialDetector) detectReversedText(input string) (bool, string) {
	// 检测常见的反向指令
	reversedPatterns := []string{
		"snoitcurtsni", // instructions反向
		"drowssap",     // password反向
		"nimda",        // admin反向
		"ssapyb",       // bypass反向
	}

	lowerInput := strings.ToLower(input)
	for _, pattern := range reversedPatterns {
		if strings.Contains(lowerInput, pattern) {
			return true, "发现反向文本: " + pattern
		}
	}

	return false, ""
}

// detectCharacterOverlap 检测字符重叠
func (ad *AdversarialDetector) detectCharacterOverlap(input string) (bool, string) {
	// 检测组合字符过度使用
	combiningChars := 0
	for _, r := range input {
		if unicode.In(r, unicode.Mn, unicode.Me, unicode.Mc) { // 组合标记
			combiningChars++
		}
	}

	if combiningChars > len([]rune(input))/10 { // 超过10%是组合字符
		return true, "组合字符使用过多"
	}

	return false, ""
}

// detectInvisibleCharacters 检测不可见字符
func (ad *AdversarialDetector) detectInvisibleCharacters(input string) (bool, string) {
	invisibleCount := 0
	for _, r := range input {
		// 检测控制字符（除了常见的换行、制表符）
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			invisibleCount++
		}
		// 检测格式字符
		if unicode.In(r, unicode.Cf) {
			invisibleCount++
		}
	}

	if invisibleCount > 3 {
		return true, "发现多个不可见控制字符"
	}

	return false, ""
}

// detectMixedScriptAttack 检测混合脚本攻击
func (ad *AdversarialDetector) detectMixedScriptAttack(input string) (bool, string) {
	// 统计不同脚本的字符数
	scripts := make(map[string]int)

	for _, r := range input {
		if unicode.In(r, unicode.Latin) {
			scripts["Latin"]++
		} else if unicode.In(r, unicode.Cyrillic) {
			scripts["Cyrillic"]++
		} else if unicode.In(r, unicode.Greek) {
			scripts["Greek"]++
		} else if unicode.In(r, unicode.Han) {
			scripts["Han"]++
		} else if unicode.In(r, unicode.Hiragana, unicode.Katakana) {
			scripts["Japanese"]++
		} else if unicode.In(r, unicode.Arabic) {
			scripts["Arabic"]++
		}
	}

	// 如果混合了3种以上的脚本，且每种都有一定数量
	significantScripts := 0
	for _, count := range scripts {
		if count >= 3 {
			significantScripts++
		}
	}

	if significantScripts >= 3 {
		return true, "检测到多种脚本混合使用"
	}

	return false, ""
}

// detectHomoglyphWords 检测同形词攻击
func (ad *AdversarialDetector) detectHomoglyphWords(input string) (bool, string) {
	// 检测常见的同形词替换
	suspiciousWords := []string{
		"аdmin",    // 使用西里尔字母 а
		"pаssword", // 使用西里尔字母 а
		"tоken",    // 使用西里尔字母 о
		"ехecute",  // 使用西里尔字母 е 和 х
	}

	for _, word := range suspiciousWords {
		if strings.Contains(input, word) {
			return true, "发现同形词: " + word
		}
	}

	return false, ""
}

// NormalizeText 规范化文本（移除对抗性字符）
func (ad *AdversarialDetector) NormalizeText(input string) string {
	var normalized strings.Builder

	for _, r := range input {
		// 替换同形字符
		if replacement, exists := ad.homoglyphMap[r]; exists {
			normalized.WriteRune(replacement)
			continue
		}

		// 移除零宽字符
		if r == 0x200B || r == 0x200C || r == 0x200D || r == 0xFEFF {
			continue
		}

		// 移除不可见控制字符
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			continue
		}

		// 保留正常字符
		normalized.WriteRune(r)
	}

	return normalized.String()
}

// GetSafetyScore 获取输入的安全评分（0-100，越高越安全）
func (ad *AdversarialDetector) GetSafetyScore(input string) int {
	score := 100

	// 检测各种攻击，每种扣分
	if hasHomoglyph, _ := ad.detectHomoglyphs(input); hasHomoglyph {
		score -= 20
	}
	if hasZeroWidth, _ := ad.detectZeroWidthCharacters(input); hasZeroWidth {
		score -= 25
	}
	if hasBase64, _ := ad.detectBase64Payload(input); hasBase64 {
		score -= 30
	}
	if hasURLEncoded, _ := ad.detectURLEncodedPayload(input); hasURLEncoded {
		score -= 25
	}
	if hasUnicodeEscape, _ := ad.detectUnicodeEscape(input); hasUnicodeEscape {
		score -= 15
	}
	if hasReversed, _ := ad.detectReversedText(input); hasReversed {
		score -= 20
	}
	if hasInvisible, _ := ad.detectInvisibleCharacters(input); hasInvisible {
		score -= 15
	}
	if hasMixedScript, _ := ad.detectMixedScriptAttack(input); hasMixedScript {
		score -= 10
	}

	if score < 0 {
		score = 0
	}

	return score
}
