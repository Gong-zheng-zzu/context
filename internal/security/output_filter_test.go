package security

import (
	"testing"
)

func TestOutputFilter_FilterOutput(t *testing.T) {
	of := NewOutputFilter()

	t.Log("╔══════════════════════════════════════════════════════════╗")
	t.Log("║  测试[1]: 输出过滤器                                     ║")
	t.Log("╠══════════════════════════════════════════════════════════╣")
	t.Log("║  过滤规则:                                               ║")
	t.Log("║  ① 手机号/身份证号脱敏   ② 系统路径过滤                 ║")
	t.Log("║  ③ 内网IP地址过滤        ④ SQL语句检测                  ║")
	t.Log("║  ⑤ 密码信息过滤          ⑥ 敏感关键词黑名单            ║")
	t.Log("╚══════════════════════════════════════════════════════════╝")

	tests := []struct {
		name            string
		input           string
		expectFiltered  bool
		expectWarnings  bool
	}{
		{
			name:           "正常输出",
			input:          "您的血压是120/80 mmHg，属于正常范围。",
			expectFiltered: false,
			expectWarnings: false,
		},
		{
			name:           "包含手机号",
			input:          "请联系13812345678获取更多信息。",
			expectFiltered: true,
			expectWarnings: true,
		},
		{
			name:           "包含系统路径",
			input:          "配置文件位于 C:\\Windows\\System32\\config",
			expectFiltered: true,
			expectWarnings: true,
		},
		{
			name:           "包含内网IP",
			input:          "服务器地址是 192.168.1.100",
			expectFiltered: true,
			expectWarnings: true,
		},
		{
			name:           "包含SQL语句",
			input:          "执行 SELECT * FROM users WHERE id=1",
			expectFiltered: true,
			expectWarnings: true,
		},
		{
			name:           "包含密码",
			input:          "您的密码是 password=abc123",
			expectFiltered: true,
			expectWarnings: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("  ┌─ 测试用例: %s", tt.name)
			t.Logf("  │  输入文本: %q", tt.input)
			t.Logf("  │  期望过滤: %v, 期望警告: %v", tt.expectFiltered, tt.expectWarnings)

			filtered, warnings := of.FilterOutput(tt.input)
			t.Logf("  │  过滤输出: %q", filtered)
			t.Logf("  │  警告信息: %v", warnings)

			if tt.expectFiltered && filtered == tt.input {
				t.Errorf("  └─ FAIL: 期望过滤但未过滤")
			} else if tt.expectWarnings && len(warnings) == 0 {
				t.Errorf("  └─ FAIL: 期望有警告但没有")
			} else if !tt.expectWarnings && len(warnings) > 0 {
				t.Errorf("  └─ FAIL: 不期望有警告但出现了: %v", warnings)
			} else {
				t.Logf("  └─ PASS ✓")
			}
		})
	}
}

func TestOutputFilter_ValidateOutput(t *testing.T) {
	of := NewOutputFilter()

	t.Log("╔══════════════════════════════════════════════════════════╗")
	t.Log("║  测试[2]: 输出安全性验证                                 ║")
	t.Log("╠══════════════════════════════════════════════════════════╣")
	t.Log("║  功能: 检查AI输出是否包含安全风险                        ║")
	t.Log("╚══════════════════════════════════════════════════════════╝")

	tests := []struct {
		name       string
		input      string
		expectSafe bool
	}{
		{
			name:       "安全输出",
			input:      "建议您多喝水，注意休息。",
			expectSafe: true,
		},
		{
			name:       "包含敏感信息",
			input:      "您的身份证号是 110101199001011234",
			expectSafe: false,
		},
		{
			name:       "包含SQL注入",
			input:      "' OR '1'='1",
			expectSafe: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("  ┌─ 用例: %s", tt.name)
			t.Logf("  │  输入文本: %q", tt.input)

			isSafe, risks := of.ValidateOutput(tt.input)
			t.Logf("  │  安全性: %v, 风险项: %v", isSafe, risks)

			if isSafe != tt.expectSafe {
				t.Errorf("  └─ FAIL: 期望安全=%v, 实际=%v", tt.expectSafe, isSafe)
			} else {
				t.Logf("  └─ PASS ✓")
			}
		})
	}
}

func TestOutputFilter_GetStatistics(t *testing.T) {
	of := NewOutputFilter()

	input := "联系方式：13812345678，邮箱：test@example.com，身份证：110101199001011234"
	stats := of.GetStatistics(input)

	t.Logf("统计信息: %v", stats)

	// 应该检测到多个敏感信息
	totalSensitive := 0
	for key, count := range stats {
		if key != "blacklist_hits" && key != "sql_patterns" {
			totalSensitive += count
		}
	}

	if totalSensitive == 0 {
		t.Errorf("期望检测到敏感信息，但没有检测到")
	}
}
