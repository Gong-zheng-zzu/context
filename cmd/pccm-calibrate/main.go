// Command pccm-calibrate 用离线开发集执行 PCCM-S 层权重搜索，并产出一份可追溯的校准产物。
//
// 设计边界：
//   - 该命令只做离线搜索与落盘，不接触在线推理路径，也不修改任何默认配置。
//   - 产物不会被自动加载：调用方必须显式使用
//     calibration.LoadPCCMConfigFromFile / LoadPCCMConfigOrDefault 才会生效。
//   - 开发集与产物的溯源信息（数据集哈希、样本量、目标函数、搜索空间、评估次数）
//     由 calibration.Run 写入产物的 fingerprint，供人工核对。
//
// 用法：
//
//	go run ./cmd/pccm-calibrate -devset <devset.json> -out <artifact.json> [-objective f1]
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/contextkeeper/service/internal/security/calibration"
)

func main() {
	devSetPath := flag.String("devset", "", "带标注的开发集 JSON 路径（必填）")
	outputPath := flag.String("out", "", "校准产物输出路径（必填）")
	objective := flag.String("objective", "", "目标函数：f1 或 error_weighted（留空使用默认 f1）")
	flag.Parse()

	if err := calibrate(*devSetPath, *outputPath, *objective); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}

func calibrate(devSetPath, outputPath, objective string) error {
	if devSetPath == "" || outputPath == "" {
		return fmt.Errorf("必须同时提供 -devset 与 -out（用法见该命令的文件头注释）")
	}

	devSet, err := calibration.LoadDevSetFromFile(devSetPath)
	if err != nil {
		return err
	}
	fmt.Printf("[OK] 开发集已加载: name=%s source=%s samples=%d\n", devSet.Name, devSet.Source, len(devSet.Samples))
	fmt.Printf("     数据集哈希: %s\n", devSet.Hash())

	options := calibration.DefaultCalibrationOptions()
	if objective != "" {
		options.Objective = objective
	}
	fmt.Printf("[OK] 目标函数: %s（默认配置与搜索空间来自 DefaultCalibrationOptions）\n", options.Objective)

	artifact, err := calibration.Run(devSet, options, time.Now())
	if err != nil {
		return err
	}

	if err := calibration.WriteArtifact(outputPath, artifact); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("校准完成")
	fmt.Printf("  产物路径   : %s\n", outputPath)
	fmt.Printf("  指纹来源   : %s\n", artifact.Fingerprint.CalibrationSource)
	fmt.Printf("  目标函数   : %s\n", artifact.Fingerprint.Objective)
	fmt.Printf("  搜索空间   : %s\n", artifact.Fingerprint.SearchSpace)
	fmt.Printf("  评估次数   : %d\n", artifact.Fingerprint.Evaluations)
	fmt.Printf("  产物版本   : %s\n", artifact.Config.Version)
	fmt.Println()
	fmt.Println("  校准前 / 校准后指标（同一开发集、同一决策阈值）")
	printMetrics("  前", artifact.MetricsBefore)
	printMetrics("  后", artifact.MetricsAfter)
	fmt.Printf("  搜索方法   : %s（轮次 %d，改进步骤 %d）\n",
		artifact.Search.Method, artifact.Search.Rounds, len(artifact.Search.ImprovedSteps))
	fmt.Printf("  校准后权重 : %v\n", artifact.Config.LayerWeights)

	return nil
}

func printMetrics(label string, metrics calibration.Metrics) {
	fmt.Printf("  %s total=%d precision=%.4f recall=%.4f f1=%.4f fpr=%.4f fnr=%.4f\n",
		label,
		metrics.Total,
		metrics.Precision,
		metrics.Recall,
		metrics.F1,
		metrics.FalsePositiveRate,
		metrics.FalseNegativeRate,
	)
}
