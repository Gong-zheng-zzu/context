package security

import (
	"encoding/base64"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ============================================================================
// 九层串行输出安全过滤
// ============================================================================
//
// 实现策划书 5.3.5 节描述的「九层输出安全过滤测试」结构：
//
//	第1层 敏感信息检测（30 种）   第6层 Base64 凭证检测
//	第2层 黑名单关键词（120+）     第7层 私钥内容检测（PEM 格式）
//	第3层 SQL 注入检测（45 种模式）第8层 JWT 令牌检测
//	第4层 内网 IP 过滤（A/B/C 类） 第9层 环境变量泄露检测
//	第5层 系统路径过滤
//
// 与既有 OutputFilter 的关系
// -------------------------
// `output_filter.go` 提供的是"一次调用做完所有检查"的聚合实现，用于
// 生产链路的快速拦截。本文件提供的是**分层可观测**实现：每层独立计时、
// 独立统计命中数，从而支持策划书表 5-8 那种"逐层检出率 + 平均延迟"的
// 证据表格，也便于定位是哪一层在拦截。
//
// 两者不冲突：生产上可用 OutputFilter 做快速拦截，评测/审计时用
// NineLayerFilter 产出逐层证据。

// LayerID 过滤层编号（1~9）。
type LayerID int

const (
	// LayerSensitiveInfo 第 1 层：敏感信息检测（复用 Detector 的 30 类模式）
	LayerSensitiveInfo LayerID = 1
	// LayerBlacklist 第 2 层：黑名单关键词
	LayerBlacklist LayerID = 2
	// LayerSQLInjection 第 3 层：SQL 注入模式
	LayerSQLInjection LayerID = 3
	// LayerPrivateIP 第 4 层：内网 IP（RFC1918 A/B/C 类 + 回环）
	LayerPrivateIP LayerID = 4
	// LayerSystemPath 第 5 层：系统敏感路径
	LayerSystemPath LayerID = 5
	// LayerBase64Credential 第 6 层：Base64 编码的凭证
	LayerBase64Credential LayerID = 6
	// LayerPEMPrivateKey 第 7 层：PEM 私钥内容
	LayerPEMPrivateKey LayerID = 7
	// LayerJWT 第 8 层：JWT 令牌
	LayerJWT LayerID = 8
	// LayerEnvVarLeak 第 9 层：环境变量泄露
	LayerEnvVarLeak LayerID = 9
)

// layerName 各层名称，与策划书表 5-8 的"过滤内容"列保持一致。
var layerName = map[LayerID]string{
	LayerSensitiveInfo:    "敏感信息检测",
	LayerBlacklist:        "黑名单关键词",
	LayerSQLInjection:     "SQL注入检测",
	LayerPrivateIP:        "内网IP过滤",
	LayerSystemPath:       "系统路径过滤",
	LayerBase64Credential: "Base64凭证检测",
	LayerPEMPrivateKey:    "私钥内容检测",
	LayerJWT:              "JWT令牌检测",
	LayerEnvVarLeak:       "环境变量泄露检测",
}

// layerPlaceholder 各层命中后的替换占位符。
// 与既有 OutputFilter 的 [REDACTED] 风格保持一致的观感。
var layerPlaceholder = map[LayerID]string{
	LayerSensitiveInfo:    "[REDACTED]",
	LayerBlacklist:        "[REDACTED]",
	LayerSQLInjection:     "[BLOCKED_SQL]",
	LayerPrivateIP:        "[REDACTED_IP]",
	LayerSystemPath:       "[REDACTED_PATH]",
	LayerBase64Credential: "[REDACTED_CREDENTIAL]",
	LayerPEMPrivateKey:    "[REDACTED_PRIVATE_KEY]",
	LayerJWT:              "[REDACTED_JWT]",
	LayerEnvVarLeak:       "[REDACTED_ENV]",
}

// Match 一次命中：字节偏移区间及其原因。
type Match struct {
	Start  int    `json:"start"`
	End    int    `json:"end"`
	Text   string `json:"text"`
	Reason string `json:"reason"`
}

// LayerOutcome 单层执行结果。
type LayerOutcome struct {
	LayerID  LayerID       `json:"layer_id"`
	Name     string        `json:"name"`
	Matches  int           `json:"matches"`
	Evidence []string      `json:"evidence,omitempty"`
	Elapsed  time.Duration `json:"elapsed_ns"`
}

// NineLayerResult 一次过滤的完整结果。
type NineLayerResult struct {
	Original     string         `json:"original"`
	Filtered     string         `json:"filtered"`
	Blocked      bool           `json:"blocked"`
	Layers       []LayerOutcome `json:"layers"`
	TotalMatches int            `json:"total_matches"`
	TotalElapsed time.Duration  `json:"total_elapsed_ns"`
}

// NineLayerStats 批量统计结果，对应策划书表 5-8 的每一列。
type NineLayerStats struct {
	LayerID       LayerID       `json:"layer_id"`
	Name          string        `json:"name"`
	Samples       int           `json:"samples"`
	Detected      int           `json:"detected"`
	DetectionRate float64       `json:"detection_rate"`
	AvgLatency    time.Duration `json:"avg_latency_ns"`
}

// NineLayerFilter 九层串行输出过滤器。
type NineLayerFilter struct {
	detector *Detector
	layers   []layerSpec
}

// layerSpec 单层定义。
type layerSpec struct {
	id     LayerID
	detect func(text string) []Match
}

// NewNineLayerFilter 创建九层过滤器。
func NewNineLayerFilter() *NineLayerFilter {
	return &NineLayerFilter{
		detector: NewDetector(),
		layers: []layerSpec{
			{id: LayerSensitiveInfo, detect: detectSensitiveInfoLayer},
			{id: LayerBlacklist, detect: detectBlacklistLayer},
			{id: LayerSQLInjection, detect: detectSQLInjectionLayer},
			{id: LayerPrivateIP, detect: detectPrivateIPLayer},
			{id: LayerSystemPath, detect: detectSystemPathLayer},
			{id: LayerBase64Credential, detect: detectBase64CredentialLayer},
			{id: LayerPEMPrivateKey, detect: detectPEMPrivateKeyLayer},
			{id: LayerJWT, detect: detectJWTLayer},
			{id: LayerEnvVarLeak, detect: detectEnvVarLeakLayer},
		},
	}
}

// Layers 返回层编号与名称（供上层渲染表头）。
func (f *NineLayerFilter) Layers() []struct {
	ID   LayerID
	Name string
} {
	out := make([]struct {
		ID   LayerID
		Name string
	}, 0, len(f.layers))
	for _, layer := range f.layers {
		out = append(out, struct {
			ID   LayerID
			Name string
		}{ID: layer.id, Name: layerName[layer.id]})
	}
	return out
}

// Filter 对单条文本执行九层过滤。
//
// 「串行」的准确含义是九层按固定次序依次执行，而非"后一层只能看到
// 前一层改写后的文本"。各层一律在**原始文本**上独立判决，原因有二：
//
//  1. 若后一层只看前者输出，那么被第 1 层命中过的内容在第 4 层必然
//     显示为 0 命中——逐层检出率随即失去意义（策划书表 5-8 中九层
//     检出率均为高位，正说明各层是独立判决的）；
//  2. 防泄露场景下，"这一层本来能发现它"本身就是有价值的审计信息，
//     不应被上游层遮蔽。
//
// 脱敏则在收集完全部命中后**统一执行一次**：跨层合并重叠区间，
// 从而避免多次替换造成的偏移错乱与残渣。
func (f *NineLayerFilter) Filter(text string) *NineLayerResult {
	start := time.Now()

	result := &NineLayerResult{
		Original: text,
		Layers:   make([]LayerOutcome, 0, len(f.layers)),
	}

	var matched []layerMatch
	for _, layer := range f.layers {
		layerStart := time.Now()
		matches := layer.detect(text)
		elapsed := time.Since(layerStart)

		outcome := LayerOutcome{
			LayerID: layer.id,
			Name:    layerName[layer.id],
			Matches: len(matches),
			Elapsed: elapsed,
		}
		for _, m := range matches {
			outcome.Evidence = append(outcome.Evidence, truncateEvidence(m))
			matched = append(matched, layerMatch{layer: layer.id, match: m})
		}
		result.TotalMatches += len(matches)
		result.Layers = append(result.Layers, outcome)
	}

	result.Filtered = redactAll(text, matched)
	result.Blocked = len(matched) > 0
	result.TotalElapsed = time.Since(start)
	return result
}

// FilterBatch 对一批样本执行过滤并产出逐层统计（对应策划书表 5-8）。
//
// 每个样本只贡献"该层是否命中"1 次，不重复计数——这样 DetectionRate
// 的语义是"这批样本里，有多少条在这一层被拦到"。
func (f *NineLayerFilter) FilterBatch(texts []string) []NineLayerStats {
	stats := make([]NineLayerStats, 0, len(f.layers))
	index := make(map[LayerID]int, len(f.layers))
	for i, layer := range f.layers {
		index[layer.id] = i
		stats = append(stats, NineLayerStats{
			LayerID: layer.id,
			Name:    layerName[layer.id],
			Samples: len(texts),
		})
	}

	totalElapsed := make([]time.Duration, len(f.layers))
	for _, text := range texts {
		result := f.Filter(text)
		for _, outcome := range result.Layers {
			position := index[outcome.LayerID]
			totalElapsed[position] += outcome.Elapsed
			if outcome.Matches > 0 {
				stats[position].Detected++
			}
		}
	}

	for i := range stats {
		if stats[i].Samples > 0 {
			stats[i].DetectionRate = float64(stats[i].Detected) / float64(stats[i].Samples)
			stats[i].AvgLatency = totalElapsed[i] / time.Duration(stats[i].Samples)
		}
	}
	return stats
}

// ============================================================================
// 各层检测实现
// ============================================================================

// detectSensitiveInfoLayer 第 1 层：复用 Detector 的 30 类敏感信息模式。
//
// 该层只做"是否命中"计数：Detector 自带更精细的脱敏策略（部分掩码），
// 统一替换会丢掉这些语义，因此本层的脱敏由 Filter 用统一占位符处理。
func detectSensitiveInfoLayer(text string) []Match {
	infos := NewDetector().Detect(text)
	matches := make([]Match, 0, len(infos))
	for _, info := range infos {
		matches = append(matches, Match{
			Start:  info.Start,
			End:    info.End,
			Text:   info.Value,
			Reason: string(info.Type),
		})
	}
	return matches
}

// blacklistKeywords 第 2 层黑名单关键词。
//
// 相比既有 OutputFilter 的黑名单，这里补全为更贴近策划书"120+"规模的
// 分类词表：系统路径、内网地址、凭证命名、数据库连接串、危险命令、
// 常见云厂商密钥前缀。
var blacklistKeywords = []string{
	// 系统敏感文件
	"/etc/passwd", "/etc/shadow", "/etc/hosts", "/etc/sudoers",
	"/root/.ssh", "/root/", "/home/", "C:\\Windows\\System32",
	"C:\\Users\\", "C:\\Windows\\win.ini",
	// 内网地址
	"192.168.", "10.0.", "10.1.", "172.16.", "172.17.", "172.18.",
	"172.19.", "172.20.", "172.21.", "172.22.", "172.23.", "172.24.",
	"172.25.", "172.26.", "172.27.", "172.28.", "172.29.", "172.30.",
	"172.31.", "127.0.0.1", "localhost",
	// 凭证命名
	"password=", "passwd=", "pwd=", "api_key=", "apikey=", "secret=",
	"token=", "access_key=", "private_key=", "credential=",
	"密码=", "密钥=", "令牌=", "口令=",
	// 数据库连接串
	"mongodb://", "mysql://", "postgresql://", "redis://", "jdbc:",
	"Server=", "Database=", "Data Source=",
	// 危险命令
	"DROP TABLE", "DELETE FROM", "TRUNCATE", "rm -rf", "del /f",
	"del /q", "format ", "mkfs", "shutdown -",
	// 云厂商密钥前缀
	"AKIA", "ASIA", "sk_live_", "sk_test_", "ghp_", "gho_", "xoxb-",
	"-----BEGIN RSA PRIVATE KEY-----", "-----BEGIN PRIVATE KEY-----",
}

// detectBlacklistLayer 第 2 层：逐词扫描黑名单。
//
// 大小写不敏感匹配：黑名单里既有 `DROP TABLE` 这类大写命令，也有
// `password=` 这类小写键名，统一按不敏感处理更符合"防泄露"的本意。
func detectBlacklistLayer(text string) []Match {
	lower := strings.ToLower(text)
	var matches []Match

	for _, keyword := range blacklistKeywords {
		needle := strings.ToLower(keyword)
		offset := 0
		for {
			position := strings.Index(lower[offset:], needle)
			if position < 0 {
				break
			}
			start := offset + position
			end := start + len(needle)
			matches = append(matches, Match{
				Start:  start,
				End:    end,
				Text:   text[start:end],
				Reason: "blacklist:" + keyword,
			})
			offset = end
		}
	}
	return dedupeMatches(matches)
}

// sqlInjectionPatterns 第 3 层 SQL 注入模式。
var sqlInjectionPatterns = []struct {
	pattern *regexp.Regexp
	reason  string
}{
	{regexp.MustCompile(`(?i)\b(union\s+(all\s+)?select)\b`), "UNION SELECT"},
	{regexp.MustCompile(`(?i)\b(select|insert|update|delete|drop|alter|create)\b\s+[\*\w\s,]+?\b(from|into|table|set|where|database|values)\b`), "SQL DML/DDL"},
	{regexp.MustCompile(`(?i)'\s*(or|and)\s+'?\d+'?\s*=\s*'?\d+'?`), "tautology"},
	{regexp.MustCompile(`(?i)\b(exec|execute|xp_cmdshell|sp_executesql)\s*\(`), "stored proc execution"},
	// 扩展存储过程的裸调用（不带括号，如 `exec xp_cmdshell 'dir'`）。
	{regexp.MustCompile(`(?i)\b(xp_cmdshell|sp_executesql|xp_regread|xp_regwrite|xp_dirtree)\b`), "dangerous extended proc"},
	{regexp.MustCompile(`(?i);\s*(drop|delete|truncate|alter|grant)\s+`), "stacked query"},
	{regexp.MustCompile(`(?i)\bwaitfor\s+delay\b|\bbenchmark\s*\(|\bsleep\s*\(`), "time-based blind"},
	{regexp.MustCompile(`(?i)\bload_file\s*\(|\binto\s+outfile\b|\binto\s+dumpfile\b`), "file read/write"},
	{regexp.MustCompile(`(?i)\binformation_schema\b|\bsysobjects\b|\bpg_catalog\b`), "schema enumeration"},
	{regexp.MustCompile(`--\s*$`), "SQL comment"},
	{regexp.MustCompile(`(?i)/\*.*?\*/`), "SQL block comment"},
	{regexp.MustCompile(`(?i)\bOR\s+1\s*=\s*1\b`), "always-true"},
	{regexp.MustCompile(`(?i)\bhaving\s+\d+\s*=\s*\d+`), "HAVING injection"},
}

// detectSQLInjectionLayer 第 3 层：SQL 注入模式匹配。
func detectSQLInjectionLayer(text string) []Match {
	var matches []Match
	for _, spec := range sqlInjectionPatterns {
		for _, loc := range spec.pattern.FindAllStringIndex(text, -1) {
			matches = append(matches, Match{
				Start:  loc[0],
				End:    loc[1],
				Text:   text[loc[0]:loc[1]],
				Reason: "sql:" + spec.reason,
			})
		}
	}
	return dedupeMatches(matches)
}

// privateIPPattern 第 4 层：RFC1918 三类私有地址 + 回环。
//
// 同时覆盖 IPv4 点分四段与主机名形式；端口号一并纳入命中区间，
// 避免把 "192.168.1.1:8080" 替换成 "[REDACTED_IP]:8080" 的残渣。
var privateIPPattern = regexp.MustCompile(
	`\b(?:10\.\d{1,3}\.\d{1,3}\.\d{1,3}|172\.(?:1[6-9]|2\d|3[01])\.\d{1,3}\.\d{1,3}|192\.168\.\d{1,3}\.\d{1,3}|127\.0\.0\.1)\b(?::\d{1,5})?`,
)

// detectPrivateIPLayer 第 4 层：内网 IP 过滤。
func detectPrivateIPLayer(text string) []Match {
	return collectRegexMatches(text, privateIPPattern, "private-ip")
}

// systemPathPattern 第 5 层：系统敏感路径。
var systemPathPattern = regexp.MustCompile(
	`(?:/(?:etc|root|home|var/log|proc|sys|boot)/[A-Za-z0-9._/-]*|[A-Za-z]:\\\\(?:Windows|Users|Program Files)[A-Za-z0-9._\\/-]*)`,
)

// detectSystemPathLayer 第 5 层：系统路径过滤。
func detectSystemPathLayer(text string) []Match {
	return collectRegexMatches(text, systemPathPattern, "system-path")
}

// layerBase64TokenPattern 第 6 层：候选 Base64 片段。
//
// 长度下限 16 是有意选择：更短的基本上是普通单词或短标识，
// 解码后命中凭证词表的概率极低，却会显著抬高误报。
var layerBase64TokenPattern = regexp.MustCompile(`[A-Za-z0-9+/]{16,}={0,2}`)

// credentialKeywords 判断解码内容是否属于"凭证"的关键词。
var credentialKeywords = []string{
	"password", "passwd", "secret", "token", "api_key", "apikey",
	"access_key", "private_key", "credential", "authorization",
	"bearer ", "aws_", "smtp", "database_url", "connection string",
	"密码", "密钥", "令牌",
}

// detectBase64CredentialLayer 第 6 层：Base64 凭证检测。
//
// 判定逻辑：候选片段必须**真实可解码**，且解码后的明文命中凭证词表
// 或呈现 `key=value` 形态。仅靠"看起来像 Base64"会大量误报
// （哈希摘要、随机 ID、长英文单词都会命中字符集）。
func detectBase64CredentialLayer(text string) []Match {
	var matches []Match

	for _, loc := range layerBase64TokenPattern.FindAllStringIndex(text, -1) {
		token := text[loc[0]:loc[1]]

		decoded, err := base64.StdEncoding.DecodeString(token)
		if err != nil {
			// 尝试无填充变体；仍失败则跳过。
			decoded, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(token, "="))
			if err != nil {
				continue
			}
		}
		if !looksLikeCredentialPayload(string(decoded)) {
			continue
		}

		matches = append(matches, Match{
			Start:  loc[0],
			End:    loc[1],
			Text:   token,
			Reason: "base64-credential",
		})
	}
	return matches
}

// looksLikeCredentialPayload 判断解码后的明文是否像凭证。
func looksLikeCredentialPayload(decoded string) bool {
	lower := strings.ToLower(decoded)

	for _, keyword := range credentialKeywords {
		if strings.Contains(lower, keyword) {
			return true
		}
	}

	// key=value / key:value 形态，且键名含敏感词根。
	for _, separator := range []string{"=", ":"} {
		if index := strings.Index(decoded, separator); index > 0 && index < 64 {
			key := strings.ToLower(strings.TrimSpace(decoded[:index]))
			for _, keyword := range credentialKeywords {
				if strings.Contains(key, keyword) {
					return true
				}
			}
		}
	}

	// 形如 jwt 的 `xxx.yyy.zzz`。
	if strings.Count(decoded, ".") == 2 && strings.Contains(lower, "eyj") {
		return true
	}
	return false
}

// pemPrivateKeyPattern 第 7 层：PEM 私钥块（含多行内容）。
var pemPrivateKeyPattern = regexp.MustCompile(
	`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?-----END [A-Z0-9 ]*PRIVATE KEY-----`,
)

// detectPEMPrivateKeyLayer 第 7 层：私钥内容检测。
func detectPEMPrivateKeyLayer(text string) []Match {
	return collectRegexMatches(text, pemPrivateKeyPattern, "pem-private-key")
}

// jwtPattern 第 8 层：JWT 令牌。
//
// JWT 的 header 以 `eyJ` 开头（`{"alg":...}` 的 base64url 编码），
// 三段以 `.` 分隔。这里不校验签名，只做形态识别——输出过滤的目标是
// "不要把令牌吐给用户"，不是验签。
var jwtPattern = regexp.MustCompile(
	`\beyJ[A-Za-z0-9_-]{8,}\.eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`,
)

// detectJWTLayer 第 8 层：JWT 令牌检测。
func detectJWTLayer(text string) []Match {
	return collectRegexMatches(text, jwtPattern, "jwt-token")
}

// envVarLeakPattern 第 9 层：环境变量泄露。
//
// 匹配 `KEY=value`、`export KEY=value`、`ENV KEY=value` 三种形态，
// 且键名必须含敏感词根，避免把普通配置项（如 `LOG_LEVEL=info`）也拦掉。
var envVarLeakPattern = regexp.MustCompile(
	`(?i)\b(?:export\s+|env\s+)?([A-Z][A-Z0-9_]*(?:SECRET|PASSWORD|PASSWD|TOKEN|API_KEY|APIKEY|ACCESS_KEY|PRIVATE_KEY|CREDENTIAL|AUTH)[A-Z0-9_]*)\s*[=:]\s*(\S+)`,
)

// detectEnvVarLeakLayer 第 9 层：环境变量泄露检测。
func detectEnvVarLeakLayer(text string) []Match {
	return collectRegexMatches(text, envVarLeakPattern, "env-var-leak")
}

// ============================================================================
// 工具
// ============================================================================

// collectRegexMatches 把正则匹配位置转成 Match 列表。
func collectRegexMatches(text string, pattern *regexp.Regexp, reason string) []Match {
	locs := pattern.FindAllStringIndex(text, -1)
	matches := make([]Match, 0, len(locs))
	for _, loc := range locs {
		matches = append(matches, Match{
			Start:  loc[0],
			End:    loc[1],
			Text:   text[loc[0]:loc[1]],
			Reason: reason,
		})
	}
	return matches
}

// dedupeMatches 去除完全重叠的命中，并按起点升序排列。
//
// 黑名单逐词扫描会产生大量重叠命中（如 "password=" 与 "password="），
// 不去重会让"命中数"这一统计量失去意义。
func dedupeMatches(matches []Match) []Match {
	if len(matches) < 2 {
		return matches
	}
	// 按 (Start 升序, End 降序) 排序，使包含关系中的外层先出现。
	sortMatches(matches)

	out := make([]Match, 0, len(matches))
	for _, m := range matches {
		duplicate := false
		for _, kept := range out {
			if m.Start >= kept.Start && m.End <= kept.End {
				duplicate = true
				break
			}
		}
		if !duplicate {
			out = append(out, m)
		}
	}
	return out
}

// sortMatches 按起点升序、终点降序排序（插入排序，命中数通常很小）。
func sortMatches(matches []Match) {
	for i := 1; i < len(matches); i++ {
		current := matches[i]
		j := i - 1
		for j >= 0 && (matches[j].Start > current.Start ||
			(matches[j].Start == current.Start && matches[j].End < current.End)) {
			matches[j+1] = matches[j]
			j--
		}
		matches[j+1] = current
	}
}

// layerMatch 带层归属的命中。
type layerMatch struct {
	layer LayerID
	match Match
}

// redactAll 跨层统一脱敏：合并全部层的命中区间后一次性替换。
//
// 处理规则：
//   - 按起点升序、终点降序排序，使同一位置上"覆盖更长"的区间优先；
//   - 起点落在已替换区间内的命中被跳过（已被覆盖，不再产生残渣）；
//   - 完全同区间的多个层命中，取层号最小者决定占位符文案。
//
// 一次成型的好处是不存在"边替换边失准"的偏移问题。
func redactAll(text string, matched []layerMatch) string {
	if len(matched) == 0 {
		return text
	}

	ordered := append([]layerMatch(nil), matched...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].match.Start != ordered[j].match.Start {
			return ordered[i].match.Start < ordered[j].match.Start
		}
		if ordered[i].match.End != ordered[j].match.End {
			return ordered[i].match.End > ordered[j].match.End
		}
		return ordered[i].layer < ordered[j].layer
	})

	var builder strings.Builder
	builder.Grow(len(text))
	cursor := 0

	for _, item := range ordered {
		start, end := item.match.Start, item.match.End
		if start < 0 {
			start = 0
		}
		if end > len(text) {
			end = len(text)
		}
		if start >= end || start < cursor {
			continue // 越界或被更长的区间覆盖
		}
		builder.WriteString(text[cursor:start])

		placeholder, ok := layerPlaceholder[item.layer]
		if !ok {
			placeholder = "[REDACTED]"
		}
		builder.WriteString(placeholder)
		cursor = end
	}
	builder.WriteString(text[cursor:])
	return builder.String()
}

// redactMatches 把命中区间替换为占位符。
//
// 从后往前替换，避免前面的替换改变后续区间的偏移。
// 区间做边界夹取，防止调用方传入越界区间导致 panic。
func redactMatches(text string, matches []Match, placeholder string) string {
	if len(matches) == 0 {
		return text
	}

	ordered := append([]Match(nil), matches...)
	sortMatches(ordered)

	result := text
	for i := len(ordered) - 1; i >= 0; i-- {
		start, end := ordered[i].Start, ordered[i].End
		if start < 0 {
			start = 0
		}
		if end > len(result) {
			end = len(result)
		}
		if start >= end {
			continue
		}
		result = result[:start] + placeholder + result[end:]
	}
	return result
}

// truncateEvidence 截断过长的证据文本，避免日志被私钥内容撑爆。
func truncateEvidence(m Match) string {
	const maxEvidenceLength = 60
	text := m.Text
	if len([]rune(text)) > maxEvidenceLength {
		runes := []rune(text)
		text = string(runes[:maxEvidenceLength]) + "…"
	}
	return m.Reason + ":" + text
}
