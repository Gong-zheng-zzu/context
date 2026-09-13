//go:build ignore

package main

import (
	"fmt"
	"github.com/contextkeeper/service/internal/security"
)

func main() {
	fmt.Println("=== ASDF对抗性攻击检测框架测试 ===\n")

	// 创建ASDF框架
	asdf := security.NewAdversarialSampleDefenseFramework()

	// 测试用例
	testCases := []struct {
		name  string
		input string
	}{
		{"空格分隔攻击", "1 3 8 1 2 3 4 5 6 7 8"},
		{"特殊字符混淆", "138-1234-5678"},
		{"同音字替换", "幺三八幺二三四五六七八"},
		{"中文数字", "一三八一二三四五六七八"},
		{"正常文本", "我的电话是13812345678"},
	}

	for _, tc := range testCases {
		fmt.Printf("测试: %s\n", tc.name)
		fmt.Printf("输入: %s\n", tc.input)

		normalizedText, isAdversarial, attackTypes, confidence := asdf.DefendAndNormalize(tc.input)

		fmt.Printf("是否为对抗样本: %v\n", isAdversarial)
		if isAdversarial {
			fmt.Printf("攻击类型: %v\n", attackTypes)
			fmt.Printf("置信度: %.2f\n", confidence)
			fmt.Printf("归一化后: %s\n", normalizedText)
		} else {
			fmt.Printf("归一化后: %s\n", normalizedText)
		}
		fmt.Println("---")
	}

	fmt.Println("\n=== 测试完成 ===")
}
