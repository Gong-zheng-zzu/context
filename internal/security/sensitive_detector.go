package security

import (
	"github.com/contextkeeper/service/internal/cache"
	"log"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

type SensitiveType string

const (
	SensitiveTypeAPIKey      SensitiveType = "api_key"
	SensitiveTypePassword    SensitiveType = "password"
	SensitiveTypeToken       SensitiveType = "token"
	SensitiveTypePrivateKey  SensitiveType = "private_key"
	SensitiveTypeBearerToken SensitiveType = "bearer_token"
	SensitiveTypeAWSKey      SensitiveType = "aws_key"
	SensitiveTypeDatabase    SensitiveType = "database_credential"
	SensitiveTypeCreditCard  SensitiveType = "credit_card"
	SensitiveTypeSSN         SensitiveType = "ssn"
	SensitiveTypePhone       SensitiveType = "phone"
	SensitiveTypeEmail       SensitiveType = "email"
	SensitiveTypeIPAddress   SensitiveType = "ip_address"
	SensitiveTypeSecretKey   SensitiveType = "secret_key"
	SensitiveTypeIDCard      SensitiveType = "id_card"
	SensitiveTypeBankCard    SensitiveType = "bank_card"
	SensitiveTypePassport    SensitiveType = "passport"
	SensitiveTypeJWT         SensitiveType = "jwt_token"
	SensitiveTypeOAuth       SensitiveType = "oauth_token"
)

type SensitiveInfo struct {
	Type       SensitiveType  `json:"type"`
	Value      string         `json:"value"`
	Start      int            `json:"start"`
	End        int            `json:"end"`
	Label      string         `json:"label"`
	Position   int            `json:"position"`   // 位置（与Start相同，为了兼容性）
	Length     int            `json:"length"`     // 长度
	Confidence float64        `json:"confidence"` // 置信度
	Encrypted  bool           `json:"encrypted"`  // 是否已加密
	CASIA      *CASIAEvidence `json:"casia_evidence,omitempty"`
}

type Detector struct {
	rules  map[SensitiveType]*regexp.Regexp
	labels map[SensitiveType]string
	cache  *cache.LRUCache // 检测结果缓存
}

func NewDetector() *Detector {
	d := &Detector{
		rules:  make(map[SensitiveType]*regexp.Regexp),
		labels: make(map[SensitiveType]string),
		cache:  cache.NewLRUCache(1000, 10*time.Minute), // 缓存1000条，10分钟过期
	}

	d.initRules()

	// 启动定期清理过期缓存
	go d.startCacheCleanup()

	return d
}

// startCacheCleanup 定期清理过期缓存
func (d *Detector) startCacheCleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		removed := d.cache.CleanupExpired()
		if removed > 0 {
			log.Printf("[敏感信息检测] 清理过期缓存: %d条", removed)
		}
	}
}

func (d *Detector) initRules() {
	d.rules[SensitiveTypeAPIKey] = regexp.MustCompile(`(?i)(api[\s_-]?key|apikey|apisecret|api[\s_-]?secret)\s*([:=]|\s+is|\s+was|\s+be)\s*['"]?([a-zA-Z0-9_\-]{16,64})['"]?`)
	d.rules[SensitiveTypePassword] = regexp.MustCompile(`(?i)(password|passwd|pwd|secret)\s*([:=]|\s+is|\s+was|\s+be)\s*['"]?([^\s'"]{6,128})['"]?`)
	d.rules[SensitiveTypeToken] = regexp.MustCompile(`(?i)(auth[_-]?token|access[_-]?token)\s*[:=]\s*['"]?([a-zA-Z0-9_\-\.]{16,128})['"]?`)
	d.rules[SensitiveTypePrivateKey] = regexp.MustCompile(`-----BEGIN\s+(RSA\s+)?PRIVATE\s+KEY-----`)
	d.rules[SensitiveTypeBearerToken] = regexp.MustCompile(`(?i)Bearer\s+[a-zA-Z0-9_\-\.]+`)
	d.rules[SensitiveTypeAWSKey] = regexp.MustCompile(`(?i)(AKIA|ABIA|ACCA|ASIA)[A-Z0-9]{16}`)
	d.rules[SensitiveTypeDatabase] = regexp.MustCompile(`(?i)(mongodb|mysql|postgresql|postgres|redis|sqlserver)[^\s]*\s+(connection|string|uri|url)\s*[:=]\s*['"]?[^\s'"]{8,}['"]?`)
	// 🔥 信用卡正则：支持连续和带空格的格式
	d.rules[SensitiveTypeCreditCard] = regexp.MustCompile(`\b(?:4\s?[0-9]{3}\s?[0-9]{4}\s?[0-9]{4}\s?(?:[0-9]{3})?|5[1-5]\s?[0-9]{2}\s?[0-9]{4}\s?[0-9]{4}\s?[0-9]{4}|3[47]\s?[0-9]{2}\s?[0-9]{4}\s?[0-9]{4}\s?[0-9]{3}|6(?:011|5[0-9]{2})\s?[0-9]{4}\s?[0-9]{4}\s?[0-9]{4})\b`)
	d.rules[SensitiveTypeSSN] = regexp.MustCompile(`\b\d{3}[-]?\d{2}[-]?\d{4}\b`)
	// 🔥 增强版手机号正则：严格匹配中国大陆手机号段
	// 13x: 130-139, 14x: 145/147/149, 15x: 150-153/155-159
	// 16x: 166, 17x: 170-178, 18x: 180-189, 19x: 191/198/199
	d.rules[SensitiveTypePhone] = regexp.MustCompile(`\b1(?:3\d|4[5-9]|5[0-35-9]|6[2567]|7[0-8]|8\d|9[1389])\d{8}\b`)

	// 🔥 增强版邮箱正则：更严格的格式验证
	// 用户名：字母数字开头，可包含._-，但不能连续出现特殊字符
	// 域名：至少两级域名，顶级域名2-6位字母
	d.rules[SensitiveTypeEmail] = regexp.MustCompile(`\b[A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?@[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?)*\.[A-Za-z]{2,6}\b`)

	d.rules[SensitiveTypeIPAddress] = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	d.rules[SensitiveTypeSecretKey] = regexp.MustCompile(`(?i)(client[_-]?secret|secret[_-]?key|app[_-]?secret)\s*[:=]\s*['"]?([a-zA-Z0-9_\-]{16,64})['"]?`)

	// 🔥 增强版身份证正则：严格验证省份代码、日期范围、校验码
	// 省份代码：11-65（真实省份编码）
	// 年份：1900-2099
	// 月份：01-12
	// 日期：01-31
	// 校验码：0-9或X/x
	d.rules[SensitiveTypeIDCard] = regexp.MustCompile(`\b(?:1[1-5]|2[1-3]|3[1-7]|4[1-6]|5[0-4]|6[1-5])\d{4}(?:19|20)\d{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12]\d|3[01])\d{3}[\dXx]\b`)

	// 🔥 增强版银行卡正则：支持更多BIN号段
	// 62开头：银联卡（16-19位）
	// 4开头：VISA卡（16位）
	// 5开头：MasterCard卡（16位）
	// 6开头：银联/Discover卡（16-19位）
	// 9558开头：部分银行卡（19位）
	// 排除纯数字序列和带空格的格式
	d.rules[SensitiveTypeBankCard] = regexp.MustCompile(`\b(?:62[0-9]{14,17}|4[0-9]{15}|5[1-5][0-9]{14}|6(?:011|5[0-9]{2})[0-9]{12,15}|9558[0-9]{15})\b`)
	d.rules[SensitiveTypePassport] = regexp.MustCompile(`\b[A-Z]\d{8}\b`)
	d.rules[SensitiveTypeJWT] = regexp.MustCompile(`\beyJ[a-zA-Z0-9_-]+\.eyJ[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+\b`)
	d.rules[SensitiveTypeOAuth] = regexp.MustCompile(`(?i)(oauth[_-]?token|refresh[_-]?token)\s*[:=]\s*['"]?([a-zA-Z0-9_\-\.]{20,128})['"]?`)

	d.labels = map[SensitiveType]string{
		SensitiveTypeAPIKey:      "API 密钥",
		SensitiveTypePassword:    "密码",
		SensitiveTypeToken:       "认证令牌",
		SensitiveTypePrivateKey:  "私钥",
		SensitiveTypeBearerToken: "Bearer 令牌",
		SensitiveTypeAWSKey:      "AWS 凭证",
		SensitiveTypeDatabase:    "数据库凭证",
		SensitiveTypeCreditCard:  "信用卡号",
		SensitiveTypeSSN:         "社会安全号",
		SensitiveTypePhone:       "手机号",
		SensitiveTypeEmail:       "邮箱",
		SensitiveTypeIPAddress:   "IP 地址",
		SensitiveTypeSecretKey:   "密钥",
		SensitiveTypeIDCard:      "身份证号",
		SensitiveTypeBankCard:    "银行卡号",
		SensitiveTypePassport:    "护照号",
		SensitiveTypeJWT:         "JWT 令牌",
		SensitiveTypeOAuth:       "OAuth 令牌",
	}
}

func (d *Detector) Detect(text string) []SensitiveInfo {
	// 🔥 临时禁用缓存以解决批量测试问题
	// 生成缓存键
	// cacheKey := cache.HashKey(text)

	// 尝试从缓存获取
	// if cached, found := d.cache.Get(cacheKey); found {
	// 	if results, ok := cached.([]SensitiveInfo); ok {
	// 		log.Printf("[敏感信息检测] 缓存命中，跳过检测")
	// 		return results
	// 	}
	// }

	// 缓存未命中，执行检测
	var results []SensitiveInfo

	for tType, pattern := range d.rules {
		matches := pattern.FindAllStringSubmatchIndex(text, -1)
		for _, match := range matches {
			var value string
			var start, end int

			// 根据捕获组数量决定提取哪个部分
			if len(match) >= 4 {
				// 有捕获组的情况，提取最后一个捕获组（通常是实际的敏感值）
				start = match[len(match)-2]
				end = match[len(match)-1]
				value = text[start:end]
			} else if len(match) >= 2 {
				// 没有捕获组的情况，提取整个匹配
				start = match[0]
				end = match[1]
				value = text[start:end]
			} else {
				continue
			}

			// 使用增强的置信度计算
			confidence := d.calculateEnhancedConfidence(tType, value, text, start, end)

			info := SensitiveInfo{
				Type:       tType,
				Value:      value,
				Start:      start,
				End:        end,
				Label:      d.labels[tType],
				Position:   start,
				Length:     end - start,
				Confidence: confidence,
				Encrypted:  false,
			}
			results = append(results, info)
		}
	}

	// 按照 Start 位置排序，确保 DetectAndRedact 能正确从后往前替换
	sort.Slice(results, func(i, j int) bool {
		return results[i].Start < results[j].Start
	})

	// 🔥 临时禁用缓存写入
	// 存入缓存
	// d.cache.Set(cacheKey, results)
	// log.Printf("[敏感信息检测] 检测完成，结果已缓存")

	return results
}

// RedactionLevel 脱敏级别
type RedactionLevel int

const (
	RedactionLevelFull    RedactionLevel = iota // 完全隐藏
	RedactionLevelPartial                       // 部分显示
	RedactionLevelMasked                        // 保留格式掩码
)

// getRedactionLevel 根据敏感信息类型获取脱敏级别
func (d *Detector) getRedactionLevel(infoType SensitiveType) RedactionLevel {
	// 高敏感：完全隐藏
	highSensitive := []SensitiveType{
		SensitiveTypePassword,
		SensitiveTypePrivateKey,
		SensitiveTypeAWSKey,
		SensitiveTypeDatabase,
	}
	for _, t := range highSensitive {
		if t == infoType {
			return RedactionLevelFull
		}
	}

	// 中敏感：部分显示
	mediumSensitive := []SensitiveType{
		SensitiveTypeAPIKey,
		SensitiveTypeToken,
		SensitiveTypeBearerToken,
		SensitiveTypeSecretKey,
		SensitiveTypeJWT,
		SensitiveTypeOAuth,
		SensitiveTypeCreditCard,
	}
	for _, t := range mediumSensitive {
		if t == infoType {
			return RedactionLevelPartial
		}
	}

	// 低敏感：保留格式掩码
	return RedactionLevelMasked
}

// redactValue 根据级别脱敏值
func (d *Detector) redactValue(value string, infoType SensitiveType, level RedactionLevel) string {
	switch level {
	case RedactionLevelFull:
		// 完全隐藏
		switch infoType {
		case SensitiveTypePassword:
			return "[密码]"
		case SensitiveTypePrivateKey:
			return "[私钥]"
		case SensitiveTypeAWSKey:
			return "[AWS密钥]"
		case SensitiveTypeDatabase:
			return "[数据库凭证]"
		default:
			return "[已脱敏]"
		}

	case RedactionLevelPartial:
		// 部分显示（显示前后各2-4个字符）
		if len(value) <= 8 {
			return "****"
		}
		showLen := 3
		if len(value) > 20 {
			showLen = 4
		}
		return value[:showLen] + "****" + value[len(value)-showLen:]

	case RedactionLevelMasked:
		// 保留格式掩码
		switch infoType {
		case SensitiveTypePhone:
			// 手机号：138****5678
			if len(value) == 11 {
				return value[:3] + "****" + value[7:]
			}
			return "***-****-****"

		case SensitiveTypeIDCard:
			// 身份证：110***********1234
			if len(value) == 18 {
				return value[:3] + "***********" + value[14:]
			}
			return "***************"

		case SensitiveTypeBankCard:
			// 银行卡：6222 **** **** 1234
			if len(value) >= 16 {
				return value[:4] + " **** **** " + value[len(value)-4:]
			}
			return "**** **** **** ****"

		case SensitiveTypeEmail:
			// 邮箱：u***@example.com
			parts := strings.Split(value, "@")
			if len(parts) == 2 && len(parts[0]) > 2 {
				return parts[0][:1] + "***@" + parts[1]
			}
			return "***@***.com"

		case SensitiveTypeIPAddress:
			// IP：192.168.***.***
			parts := strings.Split(value, ".")
			if len(parts) == 4 {
				return parts[0] + "." + parts[1] + ".***.***"
			}
			return "***.***.***.***"

		default:
			return "****"
		}

	default:
		return "[已脱敏]"
	}
}

func (d *Detector) DetectAndRedact(text string) (string, []SensitiveInfo) {
	// 🔥 步骤1: 使用ASDF框架进行对抗样本检测和归一化
	normalizedText, isAdversarial, attackTypes, asdfConfidence := d.detectWithASDF(text)

	if isAdversarial {
		log.Printf("🛡️ [ASDF] 检测到对抗样本攻击: %v, 置信度: %.2f", attackTypes, asdfConfidence)
		log.Printf("🛡️ [ASDF] input normalized: input_bytes=%d normalized_bytes=%d", len(text), len(normalizedText))
	}

	// 🔥 步骤2: 对归一化后的文本进行正则检测
	infos := d.Detect(normalizedText)

	// 🔥 步骤3: 在原文本上进行脱敏
	redacted := text

	if len(infos) > 0 {
		// 策略：在归一化文本中找到敏感信息，然后在原文本中查找并替换
		for _, info := range infos {
			level := d.getRedactionLevel(info.Type)
			replacement := d.redactValue(info.Value, info.Type, level)

			// 在原文本中查找敏感信息的所有可能形式
			// 1. 标准形式（归一化后的值）
			// 2. 带空格的形式
			// 3. 带连字符的形式
			// 4. 带其他特殊字符的形式

			// 使用正则表达式匹配原文本中的对应敏感信息
			// 允许数字之间有空格、连字符等分隔符
			pattern := d.buildFlexiblePattern(info.Value, info.Type)
			re := regexp.MustCompile(pattern)
			redacted = re.ReplaceAllString(redacted, replacement)
		}
	}

	return redacted, infos
}

// buildFlexiblePattern 构建灵活的匹配模式，允许数字之间有分隔符
func (d *Detector) buildFlexiblePattern(value string, infoType SensitiveType) string {
	// 对于数字类型的敏感信息，构建允许分隔符的模式
	if infoType == SensitiveTypePhone || infoType == SensitiveTypeIDCard ||
		infoType == SensitiveTypeBankCard || infoType == SensitiveTypeCreditCard {
		// 将每个数字字符转换为 "数字[分隔符]*" 的模式
		var pattern strings.Builder
		for _, ch := range value {
			if unicode.IsDigit(ch) {
				pattern.WriteString(string(ch))
				// 允许数字后面跟空格、连字符、下划线等分隔符（0个或多个）
				pattern.WriteString(`[\s\-_\.\/\\|]*`)
			} else {
				// 非数字字符直接添加
				pattern.WriteString(regexp.QuoteMeta(string(ch)))
			}
		}
		return pattern.String()
	}

	// 对于API密钥等字母数字类型，允许字符间有分隔符（应对ASDF归一化后的匹配）
	if infoType == SensitiveTypeAPIKey || infoType == SensitiveTypeToken ||
		infoType == SensitiveTypeSecretKey || infoType == SensitiveTypeBearerToken {
		var pattern strings.Builder
		for _, ch := range value {
			pattern.WriteString(regexp.QuoteMeta(string(ch)))
			pattern.WriteString(`[\s\-_\.\/\\|]*`)
		}
		return pattern.String()
	}

	// 对于其他类型，直接返回原值的正则转义
	return regexp.QuoteMeta(value)
}

// detectWithASDF 使用ASDF框架检测对抗样本
func (d *Detector) detectWithASDF(text string) (normalizedText string, isAdversarial bool, attackTypes []string, confidence float64) {
	// 创建ASDF框架实例（如果还没有的话）
	asdf := NewAdversarialSampleDefenseFramework()
	return asdf.DefendAndNormalize(text)
}

func (d *Detector) HasSensitiveData(text string) bool {
	return len(d.Detect(text)) > 0
}

func (d *Detector) GetSensitiveTypes(text string) []SensitiveType {
	typeMap := make(map[SensitiveType]bool)
	for _, info := range d.Detect(text) {
		typeMap[info.Type] = true
	}

	var types []SensitiveType
	for t := range typeMap {
		types = append(types, t)
	}
	return types
}

func (d *Detector) ScanContent(text string) (isSensitive bool, types []SensitiveType, summary string) {
	infos := d.Detect(text)
	if len(infos) == 0 {
		return false, nil, ""
	}

	typeCount := make(map[SensitiveType]int)
	for _, info := range infos {
		typeCount[info.Type]++
	}

	var detectedTypes []SensitiveType
	var summaryParts []string
	for t := range typeCount {
		detectedTypes = append(detectedTypes, t)
		summaryParts = append(summaryParts, d.labels[t])
	}

	return true, detectedTypes, strings.Join(summaryParts, ", ")
}

func RedactSensitiveInfo(text string) string {
	detector := NewDetector()
	redacted, _ := detector.DetectAndRedact(text)
	return redacted
}

func DetectSensitiveInfo(text string) []SensitiveInfo {
	detector := NewDetector()
	return detector.Detect(text)
}

// calculateEnhancedConfidence 计算增强的置信度（基于上下文和格式验证）
func (d *Detector) calculateEnhancedConfidence(infoType SensitiveType, value, text string, start, end int) float64 {
	baseConfidence := 0.8 // 基础置信度

	// 1. 基于敏感信息类型的基础置信度
	typeConfidence := map[SensitiveType]float64{
		SensitiveTypeAPIKey:      0.95,
		SensitiveTypePassword:    0.85,
		SensitiveTypePrivateKey:  1.0,
		SensitiveTypeAWSKey:      0.98,
		SensitiveTypeBearerToken: 0.9,
		SensitiveTypeDatabase:    0.9,
		SensitiveTypeCreditCard:  0.85,
		SensitiveTypeSSN:         0.8,
		SensitiveTypePhone:       0.90, // 🔥 提高手机号基础置信度
		SensitiveTypeEmail:       0.95, // 🔥 降低邮箱基础置信度（需要格式验证）
		SensitiveTypeIPAddress:   0.7,
		SensitiveTypeToken:       0.85,
		SensitiveTypeSecretKey:   0.9,
		SensitiveTypeIDCard:      0.92, // 🔥 降低身份证基础置信度（需要校验码验证）
		SensitiveTypeBankCard:    0.88, // 🔥 降低银行卡基础置信度（需要Luhn验证）
		SensitiveTypePassport:    0.9,
		SensitiveTypeJWT:         0.95,
		SensitiveTypeOAuth:       0.9,
	}

	if conf, ok := typeConfidence[infoType]; ok {
		baseConfidence = conf
	}

	// 2. 上下文关键词加权
	contextKeywords := map[SensitiveType][]string{
		SensitiveTypeAPIKey:     {"api", "key", "secret", "token", "credential"},
		SensitiveTypePassword:   {"password", "passwd", "pwd", "pass"},
		SensitiveTypePrivateKey: {"private", "key", "rsa", "pem"},
		SensitiveTypeEmail:      {"email", "邮箱", "mail", "联系", "邮件"},
		SensitiveTypePhone:      {"phone", "电话", "手机", "tel", "mobile", "联系方式"},
		SensitiveTypeIDCard:     {"身份证", "id", "card", "证件"},
		SensitiveTypeBankCard:   {"银行卡", "bank", "card", "卡号"},
	}

	contextBonus := 0.0
	if keywords, ok := contextKeywords[infoType]; ok {
		// 检查前后50个字符的上下文
		contextStart := max(0, start-50)
		contextEnd := min(len(text), end+50)
		context := strings.ToLower(text[contextStart:contextEnd])

		for _, keyword := range keywords {
			if strings.Contains(context, strings.ToLower(keyword)) {
				contextBonus += 0.05
				break // 🔥 只加一次，避免过度加权
			}
		}
	}

	// 3. 🔥 增强的格式和逻辑验证
	validationBonus := 0.0
	switch infoType {
	case SensitiveTypePhone:
		// 手机号：严格11位，已通过正则验证号段
		if len(value) == 11 {
			validationBonus = 0.08
			// 🔥 检查是否为连续数字或重复数字（降低置信度）
			if isSequentialDigits(value) || isRepeatingDigits(value) {
				validationBonus = -0.3 // 大幅降低
			}
		}

	case SensitiveTypeIDCard:
		// 身份证：18位，验证校验码
		if len(value) == 18 {
			if validateIDCardChecksum(value) {
				validationBonus = 0.12 // 🔥 校验码正确，大幅提升
			} else {
				validationBonus = -0.4 // 🔥 校验码错误，大幅降低
			}
		}

	case SensitiveTypeBankCard:
		// 银行卡：Luhn算法验证
		if len(value) >= 16 && len(value) <= 19 {
			if validateLuhnChecksum(value) {
				validationBonus = 0.15 // 🔥 Luhn验证通过，大幅提升
			} else {
				validationBonus = -0.35 // 🔥 Luhn验证失败，大幅降低
			}
		}

	case SensitiveTypeEmail:
		// 邮箱：格式验证
		if strings.Count(value, "@") == 1 && strings.Contains(value, ".") {
			parts := strings.Split(value, "@")
			if len(parts) == 2 && len(parts[0]) > 0 && len(parts[1]) > 3 {
				validationBonus = 0.08
			}
		}

	case SensitiveTypeAPIKey, SensitiveTypeToken:
		// API密钥通常较长且包含多种字符
		if len(value) >= 32 {
			validationBonus = 0.1
		} else if len(value) < 20 {
			validationBonus = -0.15
		}
	}

	// 4. 计算最终置信度
	finalConfidence := baseConfidence + contextBonus + validationBonus

	// 确保置信度在 0-1 范围内
	if finalConfidence > 1.0 {
		finalConfidence = 1.0
	} else if finalConfidence < 0.0 {
		finalConfidence = 0.0
	}

	return finalConfidence
}

// 🔥 validateIDCardChecksum 验证身份证校验码（GB 11643-1999）
func validateIDCardChecksum(idCard string) bool {
	if len(idCard) != 18 {
		return false
	}

	// 加权因子
	weights := []int{7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2}
	// 校验码对照表
	checksums := []byte{'1', '0', 'X', '9', '8', '7', '6', '5', '4', '3', '2'}

	sum := 0
	for i := 0; i < 17; i++ {
		digit := int(idCard[i] - '0')
		if digit < 0 || digit > 9 {
			return false
		}
		sum += digit * weights[i]
	}

	expectedChecksum := checksums[sum%11]
	actualChecksum := idCard[17]

	// 支持小写x
	if actualChecksum == 'x' {
		actualChecksum = 'X'
	}

	return actualChecksum == expectedChecksum
}

// 🔥 validateLuhnChecksum 验证银行卡Luhn校验码
func validateLuhnChecksum(cardNumber string) bool {
	sum := 0
	isSecond := false

	// 从右往左遍历
	for i := len(cardNumber) - 1; i >= 0; i-- {
		digit := int(cardNumber[i] - '0')
		if digit < 0 || digit > 9 {
			return false
		}

		if isSecond {
			digit *= 2
			if digit > 9 {
				digit -= 9
			}
		}

		sum += digit
		isSecond = !isSecond
	}

	return sum%10 == 0
}

// 🔥 isSequentialDigits 检查是否为连续数字（如12345678901）
func isSequentialDigits(s string) bool {
	if len(s) < 5 {
		return false
	}

	ascending := 0
	descending := 0

	for i := 1; i < len(s); i++ {
		diff := int(s[i]) - int(s[i-1])
		if diff == 1 {
			ascending++
		} else if diff == -1 {
			descending++
		}
	}

	// 如果超过70%是连续的，认为是序列
	threshold := len(s) * 7 / 10
	return ascending >= threshold || descending >= threshold
}

// 🔥 isRepeatingDigits 检查是否为重复数字（如11111111111）
func isRepeatingDigits(s string) bool {
	if len(s) < 5 {
		return false
	}

	counts := make(map[rune]int)
	for _, ch := range s {
		counts[ch]++
	}

	// 如果某个数字出现超过70%，认为是重复
	threshold := len(s) * 7 / 10
	for _, count := range counts {
		if count >= threshold {
			return true
		}
	}

	return false
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// GetCacheStats 获取缓存统计信息
func (d *Detector) GetCacheStats() cache.CacheStats {
	return d.cache.GetStats()
}

// ClearCache 清空缓存（用于测试或重置）
func (d *Detector) ClearCache() {
	d.cache.Clear()
	log.Println("[敏感信息检测] 缓存已清空")
}
