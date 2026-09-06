package security

import (
	"regexp"
	"strings"
)

// OutputFilter AI输出过滤器，用于过滤AI响应中的敏感信息
type OutputFilter struct {
	detector  *Detector
	blacklist []string
	sqlPatterns []*regexp.Regexp
}

// NewOutputFilter 创建输出过滤器
func NewOutputFilter() *OutputFilter {
	of := &OutputFilter{
		detector: NewDetector(),
		blacklist: []string{
			// 系统路径黑名单
			"/etc/passwd", "/etc/shadow", "C:\\Windows\\System32",
			"/root/", "/home/", "C:\\Users\\",

			// 内网IP黑名单
			"192.168.", "10.", "172.16.", "172.17.", "172.18.", "172.19.",
			"172.20.", "172.21.", "172.22.", "172.23.", "172.24.", "172.25.",
			"172.26.", "172.27.", "172.28.", "172.29.", "172.30.", "172.31.",
			"127.0.0.1", "localhost",

			// 凭证关键词
			"password=", "api_key=", "secret=", "token=",
			"密码=", "密钥=", "令牌=",

			// 数据库连接字符串
			"mongodb://", "mysql://", "postgresql://", "redis://",
			"jdbc:", "Server=", "Database=",

			// 敏感命令
			"DROP TABLE", "DELETE FROM", "TRUNCATE",
			"rm -rf", "del /f", "format",
		},
	}

	// 编译SQL注入检测模式
	of.sqlPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(union|select|insert|update|delete|drop)\s+[\*\w\s]+?(from|into|table|set|where|database)`),
		regexp.MustCompile(`(?i)'\s*(or|and)\s*'?\d*'?\s*=\s*'?\d*'?`),
		regexp.MustCompile(`(?i)(exec|execute|xp_cmdshell)\s*\(`),
		regexp.MustCompile(`(?i);\s*(drop|delete|truncate|alter)\s+`),
		regexp.MustCompile(`(?i)--\s*$`), // SQL注释
		regexp.MustCompile(`(?i)/\*.*\*/`), // SQL多行注释
	}

	return of
}

// FilterOutput 过滤AI输出内容
// 返回：过滤后的输出、警告列表
func (of *OutputFilter) FilterOutput(output string) (filtered string, warnings []string) {
	filtered = output
	warnings = []string{}

	// 1. 检测并过滤敏感信息
	sensitiveInfos := of.detector.Detect(output)
	if len(sensitiveInfos) > 0 {
		// 使用脱敏后的内容
		filtered, _ = of.detector.DetectAndRedact(output)

		// 记录警告
		for _, info := range sensitiveInfos {
			warning := "检测到" + info.Label + "已自动脱敏"
			warnings = append(warnings, warning)
		}
	}

	// 2. 检测黑名单关键词
	lowerOutput := strings.ToLower(filtered)
	for _, keyword := range of.blacklist {
		if strings.Contains(lowerOutput, strings.ToLower(keyword)) {
			// 替换黑名单关键词
			filtered = strings.ReplaceAll(filtered, keyword, "[已过滤]")
			filtered = strings.ReplaceAll(filtered, strings.ToUpper(keyword), "[已过滤]")
			filtered = strings.ReplaceAll(filtered, strings.ToLower(keyword), "[已过滤]")

			warnings = append(warnings, "检测到敏感路径/凭证信息已过滤: "+keyword)
		}
	}

	// 3. 检测SQL注入尝试
	for _, pattern := range of.sqlPatterns {
		if pattern.MatchString(filtered) {
			// 找到所有匹配并替换
			matches := pattern.FindAllString(filtered, -1)
			for _, match := range matches {
				filtered = strings.ReplaceAll(filtered, match, "[SQL语句已过滤]")
				warnings = append(warnings, "检测到SQL语句已过滤")
			}
		}
	}

	// 4. 检测内网IP地址
	ipPattern := regexp.MustCompile(`\b(?:192\.168|10\.|172\.(?:1[6-9]|2[0-9]|3[01]))\.\d{1,3}\.\d{1,3}\b`)
	if ipPattern.MatchString(filtered) {
		filtered = ipPattern.ReplaceAllString(filtered, "[内网IP已过滤]")
		warnings = append(warnings, "检测到内网IP地址已过滤")
	}

	// 5. 检测系统路径
	pathPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)[C-Z]:\\[^\s<>"]+`), // Windows路径
		regexp.MustCompile(`(?i)/(?:etc|root|home|var|usr)/[^\s<>"]+`), // Linux路径
	}
	for _, pattern := range pathPatterns {
		if pattern.MatchString(filtered) {
			filtered = pattern.ReplaceAllString(filtered, "[系统路径已过滤]")
			warnings = append(warnings, "检测到系统路径已过滤")
		}
	}

	// 6. 检测Base64编码的长字符串（可能是凭证）
	base64Pattern := regexp.MustCompile(`\b[A-Za-z0-9+/]{40,}={0,2}\b`)
	if base64Pattern.MatchString(filtered) {
		matches := base64Pattern.FindAllString(filtered, -1)
		for _, match := range matches {
			// 只过滤长度超过60的Base64字符串（更可能是凭证）
			if len(match) > 60 {
				filtered = strings.ReplaceAll(filtered, match, "[编码内容已过滤]")
				warnings = append(warnings, "检测到可疑编码内容已过滤")
			}
		}
	}

	// 7. 检测私钥内容
	if strings.Contains(filtered, "-----BEGIN") && strings.Contains(filtered, "PRIVATE KEY-----") {
		// 移除私钥块
		privateKeyPattern := regexp.MustCompile(`-----BEGIN[^-]+PRIVATE KEY-----[\s\S]+?-----END[^-]+PRIVATE KEY-----`)
		filtered = privateKeyPattern.ReplaceAllString(filtered, "[私钥内容已过滤]")
		warnings = append(warnings, "检测到私钥内容已过滤")
	}

	// 8. 检测JWT令牌
	jwtPattern := regexp.MustCompile(`\beyJ[a-zA-Z0-9_-]+\.eyJ[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+\b`)
	if jwtPattern.MatchString(filtered) {
		filtered = jwtPattern.ReplaceAllString(filtered, "[JWT令牌已过滤]")
		warnings = append(warnings, "检测到JWT令牌已过滤")
	}

	// 9. 检测环境变量泄露
	envPattern := regexp.MustCompile(`(?i)(export\s+|set\s+)?[A-Z_]{3,}=[^\s]+`)
	matches := envPattern.FindAllString(filtered, -1)
	for _, match := range matches {
		// 检查是否包含敏感关键词
		lowerMatch := strings.ToLower(match)
		if strings.Contains(lowerMatch, "key") ||
		   strings.Contains(lowerMatch, "secret") ||
		   strings.Contains(lowerMatch, "password") ||
		   strings.Contains(lowerMatch, "token") {
			filtered = strings.ReplaceAll(filtered, match, "[环境变量已过滤]")
			warnings = append(warnings, "检测到敏感环境变量已过滤")
		}
	}

	return filtered, warnings
}

// FilterOutputWithContext 带上下文的输出过滤（用于更精确的过滤）
func (of *OutputFilter) FilterOutputWithContext(output string, userQuery string) (filtered string, warnings []string) {
	// 先执行基础过滤
	filtered, warnings = of.FilterOutput(output)

	// 如果用户查询中没有要求显示敏感信息，则进行额外过滤
	lowerQuery := strings.ToLower(userQuery)

	// 检查用户是否明确要求显示某些信息
	requestedInfo := []string{"密码", "password", "密钥", "key", "token", "令牌"}
	isRequested := false
	for _, keyword := range requestedInfo {
		if strings.Contains(lowerQuery, keyword) {
			isRequested = true
			break
		}
	}

	// 如果用户没有明确要求，则更严格地过滤
	if !isRequested {
		// 过滤所有看起来像凭证的内容
		credentialPattern := regexp.MustCompile(`(?i)(password|passwd|pwd|key|secret|token)\s*[:=]\s*['"]?[^\s'"]{6,}['"]?`)
		if credentialPattern.MatchString(filtered) {
			filtered = credentialPattern.ReplaceAllString(filtered, "[凭证信息已过滤]")
			warnings = append(warnings, "检测到凭证信息已过滤")
		}
	}

	return filtered, warnings
}

// ValidateOutput 验证输出是否安全（不修改内容，仅返回是否安全）
func (of *OutputFilter) ValidateOutput(output string) (isSafe bool, risks []string) {
	risks = []string{}

	// 检测敏感信息
	sensitiveInfos := of.detector.Detect(output)
	for _, info := range sensitiveInfos {
		risks = append(risks, "包含"+info.Label)
	}

	// 检测黑名单关键词
	lowerOutput := strings.ToLower(output)
	for _, keyword := range of.blacklist {
		if strings.Contains(lowerOutput, strings.ToLower(keyword)) {
			risks = append(risks, "包含黑名单关键词: "+keyword)
		}
	}

	// 检测SQL注入
	for _, pattern := range of.sqlPatterns {
		if pattern.MatchString(output) {
			risks = append(risks, "包含SQL语句")
			break
		}
	}

	isSafe = len(risks) == 0
	return isSafe, risks
}

// GetStatistics 获取过滤统计信息
func (of *OutputFilter) GetStatistics(output string) map[string]int {
	stats := make(map[string]int)

	// 统计敏感信息类型
	sensitiveInfos := of.detector.Detect(output)
	for _, info := range sensitiveInfos {
		stats[string(info.Type)]++
	}

	// 统计黑名单命中
	blacklistHits := 0
	lowerOutput := strings.ToLower(output)
	for _, keyword := range of.blacklist {
		if strings.Contains(lowerOutput, strings.ToLower(keyword)) {
			blacklistHits++
		}
	}
	stats["blacklist_hits"] = blacklistHits

	// 统计SQL模式命中
	sqlHits := 0
	for _, pattern := range of.sqlPatterns {
		if pattern.MatchString(output) {
			sqlHits++
		}
	}
	stats["sql_patterns"] = sqlHits

	return stats
}
