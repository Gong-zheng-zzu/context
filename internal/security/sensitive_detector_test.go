package security

import (
	"strings"
	"testing"
)

func TestDetector_DetectAPIKey(t *testing.T) {
	detector := NewDetector()

	t.Log("╔══════════════════════════════════════════════════════════╗")
	t.Log("║  测试[1]: API Key 敏感信息检测                           ║")
	t.Log("╠══════════════════════════════════════════════════════════╣")
	t.Log("║  检测目标: API密钥、API Secret等凭证信息                 ║")
	t.Log("║  检测方法: 正则表达式匹配 api_key/api_secret/bearer 等  ║")
	t.Log("╚══════════════════════════════════════════════════════════╝")

	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{
			name:     "API Key in text",
			text:     "My API key is sk-1234567890abcdef",
			expected: 1,
		},
		{
			name:     "Multiple API keys",
			text:     "api_key=sk_test_12345 and api_secret=secret_abcde12345",
			expected: 2,
		},
		{
			name:     "No sensitive data",
			text:     "Hello world, this is a normal message",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("  ┌─ 测试用例: %s", tt.name)
			t.Logf("  │  输入文本: %q", tt.text)
			t.Logf("  │  期望结果: 检测到 %d 条敏感信息", tt.expected)
			results := detector.Detect(tt.text)
			t.Logf("  │  实际结果: 检测到 %d 条敏感信息", len(results))
			for i, r := range results {
				t.Logf("  │    [%d] 类型=%s, 值=%s, 位置=%d-%d, 置信度=%.2f",
					i+1, r.Type, r.Value, r.Start, r.End, r.Confidence)
			}
			if len(results) != tt.expected {
				t.Errorf("  └─ FAIL: 期望 %d 条，实际 %d 条", tt.expected, len(results))
			} else {
				t.Logf("  └─ PASS ✓")
			}
		})
	}
}

func TestDetector_DetectPassword(t *testing.T) {
	detector := NewDetector()

	t.Log("╔══════════════════════════════════════════════════════════╗")
	t.Log("║  测试[2]: 密码(PASSWORD) 敏感信息检测                     ║")
	t.Log("╠══════════════════════════════════════════════════════════╣")
	t.Log("║  检测目标: password/pwd/passwd/secret 等密码关键词       ║")
	t.Log("║  分隔符支持: := / is / was / be                          ║")
	t.Log("╚══════════════════════════════════════════════════════════╝")

	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{
			name:     "Password in text",
			text:     "password: mySecretPassword123",
			expected: 1,
		},
		{
			name:     "pwd pattern",
			text:     "pwd = 123456",
			expected: 1,
		},
		{
			name:     "Multiple passwords",
			text:     "DB password is abc123, API password is xyz789",
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("  ┌─ 测试用例: %s", tt.name)
			t.Logf("  │  输入文本: %q", tt.text)
			t.Logf("  │  期望结果: 检测到 %d 条密码", tt.expected)
			results := detector.Detect(tt.text)
			t.Logf("  │  实际结果: 检测到 %d 条密码", len(results))
			for i, r := range results {
				t.Logf("  │    [%d] 类型=%s, 值=***（已脱敏）, 位置=%d-%d", i+1, r.Type, r.Start, r.End)
			}
			if len(results) != tt.expected {
				t.Errorf("  └─ FAIL: 期望 %d 条，实际 %d 条", tt.expected, len(results))
			} else {
				t.Logf("  └─ PASS ✓")
			}
		})
	}
}

func TestDetector_DetectToken(t *testing.T) {
	detector := NewDetector()

	t.Log("╔══════════════════════════════════════════════════════════╗")
	t.Log("║  测试[3]: 令牌(TOKEN) 敏感信息检测                       ║")
	t.Log("╠══════════════════════════════════════════════════════════╣")
	t.Log("║  检测目标: Bearer token / access_token / JWT 等令牌凭证  ║")
	t.Log("╚══════════════════════════════════════════════════════════╝")

	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{
			name:     "Bearer token",
			text:     "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
			expected: 1,
		},
		{
			name:     "Access token",
			text:     "access_token=gho_xxxxxxxxxxxxxxxxxxxx",
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("  ┌─ 测试用例: %s", tt.name)
			t.Logf("  │  输入文本: %q", tt.text)
			t.Logf("  │  期望结果: 检测到 %d 条令牌", tt.expected)
			results := detector.Detect(tt.text)
			t.Logf("  │  实际结果: 检测到 %d 条令牌", len(results))
			if len(results) != tt.expected {
				t.Errorf("  └─ FAIL: 期望 %d 条，实际 %d 条", tt.expected, len(results))
			} else {
				t.Logf("  └─ PASS ✓")
			}
		})
	}
}

func TestDetector_DetectAWSKey(t *testing.T) {
	detector := NewDetector()

	t.Log("╔══════════════════════════════════════════════════════════╗")
	t.Log("║  测试[4]: AWS密钥 敏感信息检测                           ║")
	t.Log("╠══════════════════════════════════════════════════════════╣")
	t.Log("║  检测目标: AWS Access Key ID (AKIA开头)                  ║")
	t.Log("╚══════════════════════════════════════════════════════════╝")

	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{
			name:     "AWS Access Key",
			text:     "AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE",
			expected: 1,
		},
		{
			name:     "AWS Key with prefix",
			text:     "AKIAJGZXC7ABCDEFGHIJK",
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("  ┌─ 测试用例: %s", tt.name)
			t.Logf("  │  输入文本: %q", tt.text)
			t.Logf("  │  期望结果: 检测到 %d 条AWS密钥", tt.expected)
			results := detector.Detect(tt.text)
			t.Logf("  │  实际结果: 检测到 %d 条AWS密钥", len(results))
			for i, r := range results {
				t.Logf("  │    [%d] 类型=%s, 值=%s, 位置=%d-%d", i+1, r.Type, r.Value, r.Start, r.End)
			}
			if len(results) != tt.expected {
				t.Errorf("  └─ FAIL: 期望 %d 条，实际 %d 条", tt.expected, len(results))
			} else {
				t.Logf("  └─ PASS ✓")
			}
		})
	}
}

func TestDetector_DetectCreditCard(t *testing.T) {
	detector := NewDetector()

	t.Log("╔══════════════════════════════════════════════════════════╗")
	t.Log("║  测试[5]: 信用卡号 敏感信息检测                          ║")
	t.Log("╠══════════════════════════════════════════════════════════╣")
	t.Log("║  检测目标: Visa/MasterCard/AmericanExpress/Discover      ║")
	t.Log("║  支持格式: 连续数字 / 空格分隔 (如 4111 1111 1111 1111) ║")
	t.Log("╚══════════════════════════════════════════════════════════╝")

	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{
			name:     "Credit card number",
			text:     "My card number is 4111 1111 1111 1111",
			expected: 1,
		},
		{
			name:     "Credit card with spaces",
			text:     "Credit card: 4532 1234 5678 9012",
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("  ┌─ 测试用例: %s", tt.name)
			t.Logf("  │  输入文本: %q", tt.text)
			t.Logf("  │  期望结果: 检测到 %d 张信用卡号", tt.expected)
			results := detector.Detect(tt.text)
			t.Logf("  │  实际结果: 检测到 %d 张信用卡号", len(results))
			for i, r := range results {
				t.Logf("  │    [%d] 类型=%s, 值=****（已脱敏）, 位置=%d-%d", i+1, r.Type, r.Start, r.End)
			}
			if len(results) != tt.expected {
				t.Errorf("  └─ FAIL: 期望 %d 条，实际 %d 条", tt.expected, len(results))
			} else {
				t.Logf("  └─ PASS ✓")
			}
		})
	}
}

func TestDetector_DetectAndRedact(t *testing.T) {
	detector := NewDetector()

	t.Log("╔══════════════════════════════════════════════════════════╗")
	t.Log("║  测试[6]: 敏感信息检测+自动脱敏                          ║")
	t.Log("╠══════════════════════════════════════════════════════════╣")
	t.Log("║  功能: 检测到敏感信息后自动替换为脱敏文本                ║")
	t.Log("╚══════════════════════════════════════════════════════════╝")

	tests := []struct {
		name         string
		text         string
		shouldRedact bool
	}{
		{
			name:         "API Key should be redacted",
			text:         "api_key = sk_test_1234567890abcdef",
			shouldRedact: true,
		},
		{
			name:         "Normal text not redacted",
			text:         "Hello world",
			shouldRedact: false,
		},
		{
			name:         "Separated phone and ID card should be redacted",
			text:         "身份证号为610 125 20060322 131 6，电话1-8291810799",
			shouldRedact: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("  ┌─ 测试用例: %s", tt.name)
			t.Logf("  │  输入文本: %q", tt.text)
			t.Logf("  │  期望脱敏: %v", tt.shouldRedact)
			redacted, infos := detector.DetectAndRedact(tt.text)
			hasSensitive := len(infos) > 0
			t.Logf("  │  检测结果: %d 条敏感信息", len(infos))
			t.Logf("  │  脱敏输出: %q", redacted)
			if hasSensitive != tt.shouldRedact {
				t.Errorf("  └─ FAIL: 期望 shouldRedact=%v, 实际=%v", tt.shouldRedact, hasSensitive)
			} else {
				t.Logf("  └─ PASS ✓")
			}
		})
	}
}

func TestDetector_DetectAndRedactSplitPhone(t *testing.T) {
	detector := NewDetector()
	input := "contact number: 182 9181 0799"
	redacted, infos := detector.DetectAndRedact(input)
	if len(infos) == 0 {
		t.Fatal("expected a split phone number to be detected")
	}
	if strings.Contains(redacted, "182 9181 0799") {
		t.Fatalf("expected split phone number to be redacted: %q", redacted)
	}
}

func TestDetector_HasSensitiveData(t *testing.T) {
	detector := NewDetector()

	t.Log("╔══════════════════════════════════════════════════════════╗")
	t.Log("║  测试[7]: 敏感数据快速布尔判断                           ║")
	t.Log("╠══════════════════════════════════════════════════════════╣")
	t.Log("║  功能: 快速判断文本是否包含敏感信息（返回true/false）     ║")
	t.Log("╚══════════════════════════════════════════════════════════╝")

	tests := []struct {
		name    string
		text    string
		boolean bool
	}{
		{
			name:    "Has API key",
			text:    "api_key=sk_1234567890abcde",
			boolean: true,
		},
		{
			name:    "Has password",
			text:    "password=supersecret",
			boolean: true,
		},
		{
			name:    "No sensitive data",
			text:    "Hello world",
			boolean: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("  ┌─ 测试用例: %s", tt.name)
			t.Logf("  │  输入文本: %q", tt.text)
			t.Logf("  │  期望结果: %v", tt.boolean)
			result := detector.HasSensitiveData(tt.text)
			t.Logf("  │  实际结果: %v", result)
			if result != tt.boolean {
				t.Errorf("  └─ FAIL: 期望 %v, 实际 %v", tt.boolean, result)
			} else {
				t.Logf("  └─ PASS ✓")
			}
		})
	}
}

func TestDetector_ScanContent(t *testing.T) {
	detector := NewDetector()

	t.Log("╔══════════════════════════════════════════════════════════╗")
	t.Log("║  测试[8]: 综合内容扫描                                   ║")
	t.Log("╠══════════════════════════════════════════════════════════╣")
	t.Log("║  功能: 一次性扫描文本中所有类型的敏感信息                ║")
	t.Log("╚══════════════════════════════════════════════════════════╝")

	text := "My api_key=sk-1234567890abcdefgh and password is secret123"
	t.Logf("  输入文本: %q", text)
	t.Logf("  包含: API密钥 + 密码（混合敏感信息场景）")

	isSensitive, types, summary := detector.ScanContent(text)

	t.Logf("  是否敏感: %v", isSensitive)
	t.Logf("  检测类型: %v", types)
	t.Logf("  摘要信息: %s", summary)

	if !isSensitive {
		t.Error("FAIL: 期望检测到敏感内容")
	}
	if len(types) == 0 {
		t.Error("FAIL: 期望至少检测到一种类型")
	}
	if summary == "" {
		t.Error("FAIL: 期望返回摘要信息")
	}
	t.Log("  └─ PASS ✓")
}

func TestEncryptor_EncryptDecrypt(t *testing.T) {
	t.Log("╔══════════════════════════════════════════════════════════╗")
	t.Log("║  测试[9]: AES-GCM 加密/解密                              ║")
	t.Log("╠══════════════════════════════════════════════════════════╣")
	t.Log("║  算法: AES-256-GCM + PBKDF2密钥派生 (100000轮)          ║")
	t.Log("║  密码盐值: SHA256(password) 确定性生成                   ║")
	t.Log("╚══════════════════════════════════════════════════════════╝")

	password := "test_password_123"
	original := `{"api_key": "sk-1234567890", "password": "secret123"}`
	t.Logf("  加密密码: %s", password)
	t.Logf("  原始明文: %s", original)

	enc, err := NewEncryptor(password)
	if err != nil {
		t.Fatalf("failed to create encryptor: %v", err)
	}

	cipherText, err := enc.Encrypt(original)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}

	if cipherText == original {
		t.Error("cipher text should differ from original")
	}

	decrypted, err := enc.Decrypt(cipherText)
	if err != nil {
		t.Fatalf("failed to decrypt: %v", err)
	}

	t.Logf("  解密结果: %s", decrypted)
	t.Logf("  加密前==解密后: %v", decrypted == original)

	if decrypted != original {
		t.Errorf("FAIL: 解密结果与原文不一致")
	} else {
		t.Log("  └─ PASS ✓ 加密→解密→比对一致")
	}
}

func TestEncryptor_DifferentPasswords(t *testing.T) {
	t.Log("╔══════════════════════════════════════════════════════════╗")
	t.Log("║  测试[10]: 不同密码产生不同密文                          ║")
	t.Log("╠══════════════════════════════════════════════════════════╣")
	t.Log("║  验证: 相同明文 + 不同密码 → 不同密文                    ║")
	t.Log("╚══════════════════════════════════════════════════════════╝")

	text := "secret message"
	t.Logf("  原始明文: %q", text)

	enc1, _ := NewEncryptor("password1")
	enc2, _ := NewEncryptor("password2")

	cipher1, _ := enc1.Encrypt(text)
	cipher2, _ := enc2.Encrypt(text)

	t.Logf("  密码[1] 密文: %s...", cipher1[:40])
	t.Logf("  密码[2] 密文: %s...", cipher2[:40])
	t.Logf("  密文相同: %v", cipher1 == cipher2)

	if cipher1 == cipher2 {
		t.Error("FAIL: 不同密码不应产生相同密文")
	} else {
		t.Log("  └─ PASS ✓ 不同密码产生不同密文")
	}
}

func TestEncryptJSON(t *testing.T) {
	t.Log("╔══════════════════════════════════════════════════════════╗")
	t.Log("║  测试[11]: JSON数据加密/解密                             ║")
	t.Log("╠══════════════════════════════════════════════════════════╣")
	t.Log("║  功能: 将JSON结构体加密为密文，再解密还原                ║")
	t.Log("╚══════════════════════════════════════════════════════════╝")

	data := map[string]string{
		"api_key":  "sk-1234567890",
		"password": "secret123",
	}
	password := "test_password"

	t.Logf("  原始数据: api_key=%s, password=%s", data["api_key"], data["password"])
	t.Logf("  加密密码: %s", password)

	cipherText, err := EncryptJSON(data, password)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}
	t.Logf("  加密密文: %s...", cipherText[:50])

	var decrypted map[string]string
	err = DecryptJSON(cipherText, password, &decrypted)
	if err != nil {
		t.Fatalf("failed to decrypt: %v", err)
	}

	t.Logf("  解密结果: api_key=%s, password=%s", decrypted["api_key"], decrypted["password"])

	if decrypted["api_key"] != data["api_key"] {
		t.Error("FAIL: 解密后api_key不匹配")
	} else {
		t.Log("  └─ PASS ✓ JSON加密→解密→比对一致")
	}
}

func TestAESStorage(t *testing.T) {
	t.Log("╔══════════════════════════════════════════════════════════╗")
	t.Log("║  测试[12]: AES加密安全存储                               ║")
	t.Log("╠══════════════════════════════════════════════════════════╣")
	t.Log("║  功能: Store(存储) → Retrieve(读取) → Delete(删除)       ║")
	t.Log("║  全程AES-GCM加密保护                                    ║")
	t.Log("╚══════════════════════════════════════════════════════════╝")

	password := "storage_password"

	storage, err := NewAESStorage(password)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	testData := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}
	t.Logf("  存储密码: %s", password)
	t.Logf("  存储数据: key1=value1, key2=value2")

	err = storage.Store("test", testData)
	if err != nil {
		t.Fatalf("failed to store: %v", err)
	}

	if !storage.Exists("test") {
		t.Error("key should exist")
	}

	var retrieved map[string]string
	err = storage.Retrieve("test", &retrieved)
	if err != nil {
		t.Fatalf("failed to retrieve: %v", err)
	}
	t.Logf("  读取结果: key1=%s, key2=%s", retrieved["key1"], retrieved["key2"])

	if retrieved["key1"] != "value1" {
		t.Error("FAIL: 读取的值不匹配")
	}

	storage.Delete("test")
	exists := storage.Exists("test")
	t.Logf("  删除后是否存在: %v", exists)
	if exists {
		t.Error("FAIL: 删除后仍存在")
	} else {
		t.Log("  └─ PASS ✓ 存储→读取→删除 全流程通过")
	}
}

func BenchmarkDetector_Detect(b *testing.B) {
	detector := NewDetector()
	text := "api_key=sk-1234567890abcdef password=secret123 token=abc123"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detector.Detect(text)
	}
}

func BenchmarkEncryptor_Encrypt(b *testing.B) {
	enc, _ := NewEncryptor("test_password")
	text := `{"api_key": "sk-1234567890", "data": "some very long text to encrypt for performance testing"}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enc.Encrypt(text)
	}
}
