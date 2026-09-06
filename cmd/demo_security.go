package main

import (
	"fmt"

	"github.com/contextkeeper/service/internal/security"
)

func main() {
	fmt.Println("=" + "=" + "=" + "=" + "=" + "=" + "Demo: 敏感信息检测与脱敏系统" + "=" + "=" + "=" + "=" + "=")
	fmt.Println()

	detector := security.NewDetector()

	testCases := []string{
		"用户说: 记住我的名字叫张三",
		"我的 API key 是 sk-abcdef123456789，请帮我测试",
		"数据库密码是 admin123，API token 是 gho_xxxxxxxxxxxx",
		"My AWS key: AKIAIOSFODNN7EXAMPLE, 信用卡号: 4111111111111111",
		"Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
		"手机号: 13812345678, 邮箱: zhangsan@example.com",
	}

	for i, text := range testCases {
		fmt.Printf("【测试 %d】原始输入:\n  %s\n", i+1, text)

		isSensitive, types, summary := detector.ScanContent(text)
		if isSensitive {
			redacted, infos := detector.DetectAndRedact(text)
			fmt.Printf("  ✓ 检测到敏感信息: %s\n", summary)
			fmt.Printf("  ✗ 脱敏后输出: %s\n", redacted)
			fmt.Printf("  类型详情:\n")
			for _, info := range infos {
				fmt.Printf("    - %s: %s\n", info.Label, info.Value[:min(8, len(info.Value))+"***")
			}
		} else {
			fmt.Printf("  ○ 无敏感信息，正常存储\n")
		}
		fmt.Println()
	}

	fmt.Println("-" + "-" + "-" + "-" + "-" + "-" + "审计日志统计" + "-" + "-" + "-" + "-" + "-" + "-")
	infos := detector.Detect("api_key=sk-123 password=secret token=abc")
	fmt.Printf("已记录 %d 条敏感信息访问\n", len(infos))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}