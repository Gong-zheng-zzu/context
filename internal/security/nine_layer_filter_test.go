package security

import (
	"encoding/base64"
	"strings"
	"testing"
)

// layerCase 单层正样本用例。
type layerCase struct {
	layer  LayerID
	name   string
	text   string
	marker string // 脱敏后原文中不应再出现的子串
}

// layerCases 九层各自的正样本。
func layerCases() []layerCase {
	return []layerCase{
		{
			layer:  LayerSensitiveInfo,
			name:   "敏感信息",
			text:   "老人身份证号 110101199001011234 请登记",
			marker: "110101199001011234",
		},
		{
			layer:  LayerBlacklist,
			name:   "黑名单命令",
			text:   "执行 DROP TABLE users 清空数据",
			marker: "DROP TABLE",
		},
		{
			layer:  LayerSQLInjection,
			name:   "SQL注入",
			text:   "搜索 union select password from users where id=1",
			marker: "union select password from users",
		},
		{
			layer:  LayerPrivateIP,
			name:   "内网IP",
			text:   "内网数据库地址 192.168.1.100:5432 请勿外传",
			marker: "192.168.1.100:5432",
		},
		{
			layer:  LayerSystemPath,
			name:   "系统路径",
			text:   "配置文件位于 /etc/passwd 中",
			marker: "/etc/passwd",
		},
		{
			layer: LayerBase64Credential,
			name:  "Base64凭证",
			// 下一行在测试函数中动态生成，避免硬编码出错。
		},
		{
			layer:  LayerPEMPrivateKey,
			name:   "PEM私钥",
			text:   "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA\n-----END RSA PRIVATE KEY-----",
			marker: "MIIEowIBAAKCAQEA",
		},
		{
			layer:  LayerJWT,
			name:   "JWT令牌",
			text:   "Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.abc123def456",
			marker: "eyJhbGciOiJIUzI1NiJ9",
		},
		{
			layer:  LayerEnvVarLeak,
			name:   "环境变量泄露",
			text:   "AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMIabcdefg",
			marker: "wJalrXUtnFEMIabcdefg",
		},
	}
}

// TestNineLayerFilterDetectsEachLayer 九层各自能命中其典型样本。
func TestNineLayerFilterDetectsEachLayer(t *testing.T) {
	filter := NewNineLayerFilter()

	cases := layerCases()
	// 为 Base64 层动态生成样本。
	encoded := base64.StdEncoding.EncodeToString([]byte("password=S3cr3tValue"))
	for i := range cases {
		if cases[i].layer == LayerBase64Credential {
			cases[i].text = "配置项 " + encoded + " 请勿提交"
			cases[i].marker = encoded
		}
	}

	for _, tc := range cases {
		if tc.text == "" {
			t.Fatalf("case %s has empty text", tc.name)
		}
		result := filter.Filter(tc.text)

		var hit bool
		for _, outcome := range result.Layers {
			if outcome.LayerID == tc.layer && outcome.Matches > 0 {
				hit = true
				break
			}
		}
		if !hit {
			t.Fatalf("%s: expected layer %d to fire on %q; outcomes=%+v",
				tc.name, tc.layer, tc.text, outcomeSummary(result))
		}
		if !result.Blocked {
			t.Fatalf("%s: result should be Blocked", tc.name)
		}
		if strings.Contains(result.Filtered, tc.marker) {
			t.Fatalf("%s: marker %q survived filtering: %q", tc.name, tc.marker, result.Filtered)
		}
	}
}

// TestNineLayerFilterLayersAreOrdered 层的执行顺序必须严格是 1..9，
// 且每层都被执行（便于统计口径稳定）。
func TestNineLayerFilterLayersAreOrdered(t *testing.T) {
	filter := NewNineLayerFilter()
	result := filter.Filter("今天天气不错")

	if len(result.Layers) != 9 {
		t.Fatalf("expected 9 layer outcomes, got %d", len(result.Layers))
	}
	for i, outcome := range result.Layers {
		want := LayerID(i + 1)
		if outcome.LayerID != want {
			t.Fatalf("layer %d: got ID %d, want %d", i, outcome.LayerID, want)
		}
		if outcome.Name == "" {
			t.Fatalf("layer %d has empty name", outcome.LayerID)
		}
	}
}

// TestNineLayerFilterPassesBenignText 良性文本不得被任何层拦截——
// 这是误报防线，也是"输出过滤不能妨碍正常护理对话"的底线。
func TestNineLayerFilterPassesBenignText(t *testing.T) {
	filter := NewNineLayerFilter()

	benign := []string{
		"今天天气不错，我们下午去公园散步吧。",
		"李爷爷今天胃口很好，午饭吃了两碗粥。",
		"记得提醒王奶奶晚上八点吃药。",
		"护理日志已提交，请护士长查收。",
	}

	for _, text := range benign {
		result := filter.Filter(text)
		if result.Blocked {
			t.Fatalf("benign text was blocked: %q; outcomes=%+v", text, outcomeSummary(result))
		}
		if result.Filtered != text {
			t.Fatalf("benign text was modified: %q -> %q", text, result.Filtered)
		}
	}
}

// TestSQLInjectionLayerPatterns 逐一验证 SQL 注入子模式。
func TestSQLInjectionLayerPatterns(t *testing.T) {
	positives := []string{
		"1 union select username from users",
		"' or '1'='1",
		"'; DROP TABLE users; --",
		"exec xp_cmdshell 'dir'",
		"select * from information_schema.tables",
		"1' and sleep(5)--",
	}
	for _, text := range positives {
		if matches := detectSQLInjectionLayer(text); len(matches) == 0 {
			t.Fatalf("SQL injection not detected: %q", text)
		}
	}

	negatives := []string{
		"今天下午三点给老人测血糖",
		"select 是英语单词，表示选择",
	}
	for _, text := range negatives {
		if matches := detectSQLInjectionLayer(text); len(matches) > 0 {
			t.Fatalf("false positive on %q: %+v", text, matches)
		}
	}
}

// TestPrivateIPLayerCoversAllRFC1918Classes A/B/C 三类私有地址都要覆盖，
// 公网地址不得被误拦。
func TestPrivateIPLayerCoversAllRFC1918Classes(t *testing.T) {
	private := []string{
		"10.0.0.1", "10.255.255.255",
		"172.16.0.1", "172.31.255.254",
		"192.168.0.1", "127.0.0.1",
	}
	for _, ip := range private {
		if matches := detectPrivateIPLayer("服务地址 " + ip + " 上线"); len(matches) == 0 {
			t.Fatalf("private IP not detected: %s", ip)
		}
	}

	public := []string{"8.8.8.8", "1.1.1.1", "223.5.5.5", "172.32.0.1"}
	for _, ip := range public {
		if matches := detectPrivateIPLayer("服务地址 " + ip + " 上线"); len(matches) > 0 {
			t.Fatalf("public IP falsely blocked: %s -> %+v", ip, matches)
		}
	}
}

// TestBase64CredentialLayerRequiresRealCredential 只"长得像 Base64"
// 不足以命中：必须是真实可解码且解码后确含凭证信息。这是该层误报控制的关键。
func TestBase64CredentialLayerRequiresRealCredential(t *testing.T) {
	credential := base64.StdEncoding.EncodeToString([]byte("password=SuperSecret123"))
	if matches := detectBase64CredentialLayer("token: " + credential); len(matches) == 0 {
		t.Fatalf("base64 credential not detected")
	}

	// 普通文本的 Base64 编码不构成凭证。
	harmless := base64.StdEncoding.EncodeToString([]byte("hello world, this is a benign sentence"))
	if matches := detectBase64CredentialLayer("内容 " + harmless); len(matches) > 0 {
		t.Fatalf("false positive on harmless base64: %+v", matches)
	}

	// 根本不是 Base64 的长串不应命中。
	if matches := detectBase64CredentialLayer("!!!not-base64!!!not-base64!!!"); len(matches) > 0 {
		t.Fatalf("non-base64 token matched: %+v", matches)
	}
}

// TestPEMAndJWTLayers PEM 私钥与 JWT 的形态识别。
func TestPEMAndJWTLayers(t *testing.T) {
	pem := "-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkq\n-----END PRIVATE KEY-----"
	if matches := detectPEMPrivateKeyLayer(pem); len(matches) != 1 {
		t.Fatalf("PEM private key: got %d matches, want 1", len(matches))
	}
	// 证书（非私钥）不应命中。
	cert := "-----BEGIN CERTIFICATE-----\nMIIEvQIBADANBgkq\n-----END CERTIFICATE-----"
	if matches := detectPEMPrivateKeyLayer(cert); len(matches) != 0 {
		t.Fatalf("certificate falsely blocked: %+v", matches)
	}

	jwt := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"
	if matches := detectJWTLayer(jwt); len(matches) != 1 {
		t.Fatalf("JWT: got %d matches, want 1", len(matches))
	}
}

// TestEnvVarLeakLayerRequiresSensitiveKey 只有敏感键名的环境变量才拦，
// 普通配置项不得误拦。
func TestEnvVarLeakLayerRequiresSensitiveKey(t *testing.T) {
	positive := []string{
		"AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI",
		"export DATABASE_PASSWORD=hunter2",
		"OPENAI_API_KEY: sk-abcdefghijklmn",
	}
	for _, text := range positive {
		if matches := detectEnvVarLeakLayer(text); len(matches) == 0 {
			t.Fatalf("env leak not detected: %q", text)
		}
	}

	negative := []string{
		"LOG_LEVEL=debug",
		"PORT=8080",
		"APP_NAME=yianzhihu",
	}
	for _, text := range negative {
		if matches := detectEnvVarLeakLayer(text); len(matches) > 0 {
			t.Fatalf("benign env var blocked: %q -> %+v", text, matches)
		}
	}
}

// TestFilterBatchProducesPerLayerStats 批量统计的语义：
// DetectionRate = 该层命中的样本数 / 样本总数。
func TestFilterBatchProducesPerLayerStats(t *testing.T) {
	filter := NewNineLayerFilter()

	texts := []string{
		"/etc/passwd 泄露了",
		"/etc/passwd 又泄露了",
		"今天天气不错",
		"192.168.1.1 是内网地址",
	}

	stats := filter.FilterBatch(texts)
	if len(stats) != 9 {
		t.Fatalf("expected 9 layer stats, got %d", len(stats))
	}

	byID := make(map[LayerID]NineLayerStats, len(stats))
	for _, s := range stats {
		byID[s.LayerID] = s
		if s.Samples != len(texts) {
			t.Fatalf("layer %d: samples = %d, want %d", s.LayerID, s.Samples, len(texts))
		}
	}

	pathStats := byID[LayerSystemPath]
	if pathStats.Detected != 2 {
		t.Fatalf("system path layer detected %d samples, want 2", pathStats.Detected)
	}
	if got := pathStats.DetectionRate; got != 0.5 {
		t.Fatalf("system path detection rate = %v, want 0.5", got)
	}

	ipStats := byID[LayerPrivateIP]
	if ipStats.Detected != 1 {
		t.Fatalf("private IP layer detected %d samples, want 1", ipStats.Detected)
	}

	// 未命中的层检出率必须为 0。
	for _, id := range []LayerID{LayerPEMPrivateKey, LayerJWT, LayerBase64Credential} {
		if byID[id].Detected != 0 {
			t.Fatalf("layer %d unexpectedly detected %d samples", id, byID[id].Detected)
		}
	}
}

// TestFilterBatchHandlesEmptyInput 空输入不得 panic，且返回完整层结构。
func TestFilterBatchHandlesEmptyInput(t *testing.T) {
	filter := NewNineLayerFilter()
	stats := filter.FilterBatch(nil)
	if len(stats) != 9 {
		t.Fatalf("expected 9 layer stats, got %d", len(stats))
	}
	for _, s := range stats {
		if s.Detected != 0 || s.DetectionRate != 0 {
			t.Fatalf("layer %d has non-zero stats on empty input: %+v", s.LayerID, s)
		}
	}
}

// TestSerialFilteringAppliesEveryLayer 串行过滤必须把每一层的命中
// 都替换掉：不能因为第 2 层改了文本就让第 5 层漏掉。
func TestSerialFilteringAppliesEveryLayer(t *testing.T) {
	filter := NewNineLayerFilter()

	text := "路径 /etc/passwd 暴露了，另有 10.1.2.3 内网地址"
	result := filter.Filter(text)

	if strings.Contains(result.Filtered, "/etc/passwd") {
		t.Fatalf("system path survived: %q", result.Filtered)
	}
	if strings.Contains(result.Filtered, "10.1.2.3") {
		t.Fatalf("private IP survived: %q", result.Filtered)
	}
}

// TestRedactMatchesHandlesOutOfRangeSpans 越界区间不得导致 panic。
func TestRedactMatchesHandlesOutOfRangeSpans(t *testing.T) {
	text := "abcdef"
	result := redactMatches(text, []Match{
		{Start: -5, End: 3},
		{Start: 2, End: 999},
	}, "[X]")
	if result == "" {
		t.Fatal("redaction produced empty string")
	}
}

// TestDedupeMatchesRemovesContainedSpans 重叠命中被去重，避免重复计数。
func TestDedupeMatchesRemovesContainedSpans(t *testing.T) {
	matches := []Match{
		{Start: 0, End: 10, Text: "0123456789"},
		{Start: 2, End: 5, Text: "234"},
		{Start: 3, End: 4, Text: "3"},
	}
	deduped := dedupeMatches(matches)
	if len(deduped) != 1 {
		t.Fatalf("expected 1 match after dedupe, got %d (%+v)", len(deduped), deduped)
	}
	if deduped[0].Start != 0 || deduped[0].End != 10 {
		t.Fatalf("kept wrong span: %+v", deduped[0])
	}
}

// TestLayersExposeIDsAndNames 层元信息可被上层渲染成表头。
func TestLayersExposeIDsAndNames(t *testing.T) {
	filter := NewNineLayerFilter()
	layers := filter.Layers()
	if len(layers) != 9 {
		t.Fatalf("expected 9 layers, got %d", len(layers))
	}
	for i, layer := range layers {
		if int(layer.ID) != i+1 {
			t.Fatalf("layer %d has ID %d", i, layer.ID)
		}
		if layer.Name == "" {
			t.Fatalf("layer %d has empty name", layer.ID)
		}
	}
}

// outcomeSummary 生成便于排查的层结果摘要。
func outcomeSummary(result *NineLayerResult) string {
	var b strings.Builder
	for _, outcome := range result.Layers {
		if outcome.Matches == 0 {
			continue
		}
		b.WriteString("\n  layer ")
		b.WriteString(string(rune('0' + int(outcome.LayerID))))
		b.WriteString(" ")
		b.WriteString(outcome.Name)
		b.WriteString(" matches=")
		if len(outcome.Evidence) > 0 {
			b.WriteString(strings.ReplaceAll(outcome.Evidence[0], "\n", "\\n"))
		}
	}
	return b.String()
}
