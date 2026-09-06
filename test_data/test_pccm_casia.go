package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/contextkeeper/service/internal/security"
)

func main() {
	fmt.Println("=== PCCM和CASIA集成测试 ===\n")

	// 创建多层检测器
	detector := security.NewMultiLayerDetector("http://localhost:11434", "qwen2.5:3b")
	// 词典已在NewMultiLayerDetector中自动加载

	// 测试用例
	testCases := []struct {
		name        string
		text        string
		description string
	}{
		{
			name:        "身份证号（真实场景）",
			text:        "我的身份证号是110101199001011234",
			description: "应该被检测为敏感信息",
		},
		{
			name:        "邮编（误报场景）",
			text:        "北京的邮编是110101",
			description: "CASIA应该降低置信度，避免误报",
		},
		{
			name:        "手机号（真实场景）",
			text:        "请联系我的手机13812345678",
			description: "应该被检测为敏感信息",
		},
		{
			name:        "工号（误报场景）",
			text:        "我的工号是13812345678",
			description: "CASIA应该降低置信度，避免误报",
		},
		{
			name:        "血压值（真实场景）",
			text:        "今天测量血压是120/80",
			description: "应该被检测为敏感信息",
		},
		{
			name:        "比例数据（误报场景）",
			text:        "这个比例是120/80",
			description: "CASIA应该降低置信度，避免误报",
		},
		{
			name:        "对抗样本（空格分隔）",
			text:        "我的身份证是 1 1 0 1 0 1 1 9 9 0 0 1 0 1 1 2 3 4",
			description: "ASDF应该检测并归一化",
		},
	}

	fmt.Println("📊 测试结果：\n")

	for i, tc := range testCases {
		fmt.Printf("[测试 %d] %s\n", i+1, tc.name)
		fmt.Printf("输入文本: %s\n", tc.text)
		fmt.Printf("预期效果: %s\n", tc.description)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// 执行检测
		result, err := detector.Detect(ctx, tc.text)
		if err != nil {
			log.Printf("❌ 检测失败: %v\n\n", err)
			continue
		}

		// 输出结果
		fmt.Printf("✅ 检测完成:\n")
		fmt.Printf("   - 最终置信度: %.2f%%\n", result.FinalConfidence*100)
		fmt.Printf("   - 使用层数: %v\n", result.LayersUsed)
		fmt.Printf("   - 检测到敏感信息: %d 个\n", len(result.FinalItems))
		fmt.Printf("   - 处理时间: %v\n", result.TotalTime)

		if len(result.FinalItems) > 0 {
			fmt.Println("   - 敏感信息详情:")
			for j, item := range result.FinalItems {
				fmt.Printf("     [%d] 类型=%s, 值=%s, 置信度=%.2f%%\n",
					j+1, item.Type, item.Value, item.Confidence*100)
			}
		}

		// 显示各层检测详情
		if len(result.Details) > 0 {
			fmt.Println("   - 各层检测详情:")
			for layerID, detail := range result.Details {
				fmt.Printf("     Layer %d: 检测到 %d 个, 置信度=%.2f%%, 耗时=%v\n",
					layerID, len(detail.Items), detail.Confidence*100, detail.ProcessTime)
			}
		}

		fmt.Println()
	}

	// 显示统计信息
	stats := detector.GetStats()
	fmt.Println("📈 统计信息:")
	fmt.Printf("   - 总检测次数: %d\n", stats.TotalDetections)
	fmt.Printf("   - 平均延迟: %v\n", stats.AverageLatency)
	fmt.Printf("   - 提前终止次数: %d\n", stats.EarlyStopCount)
	fmt.Printf("   - 冲突解决次数: %d\n", stats.ConflictCount)
	fmt.Println("   - 各层使用次数:")
	for layerID, count := range stats.LayerUsage {
		fmt.Printf("     Layer %d: %d 次\n", layerID, count)
	}

	fmt.Println("\n=== 测试完成 ===")
	fmt.Println("\n✅ PCCM模型状态: 已启用")
	fmt.Println("✅ CASIA算法状态: 已启用")
	fmt.Println("\n💡 说明:")
	fmt.Println("   - PCCM: 使用渐进式置信度累积模型计算最终置信度")
	fmt.Println("   - CASIA: 使用上下文感知算法降低误报率")
	fmt.Println("   - 对比有无上下文关键词的场景，CASIA会自动调整置信度")
}
