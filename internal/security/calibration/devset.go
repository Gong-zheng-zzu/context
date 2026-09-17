package calibration

import (
	"encoding/json"
	"fmt"
	"os"
)

// LoadDevSetFromFile 从 JSON 文件读取带标注的开发集。
//
// 期望格式见 DevSet / LabeledSample 的 json tag。真实开发集必须显式提供
// 每条样本的 LayerConfidences（各 PCCM-S 层的单层置信度）与 ExpectedSensitive
// 标注；Source 应如实填写（合成 / 仓库 / 业务采集）。
//
// 校验：样本非空、ID 唯一、LayerConfidences 非空且权重有限。
func LoadDevSetFromFile(path string) (DevSet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return DevSet{}, fmt.Errorf("读取开发集失败: %w", err)
	}
	var devSet DevSet
	if err := json.Unmarshal(data, &devSet); err != nil {
		return DevSet{}, fmt.Errorf("解析开发集失败: %w", err)
	}
	if err := devSet.Validate(); err != nil {
		return DevSet{}, err
	}
	return devSet, nil
}

// WriteDevSet 把开发集写入 JSON 文件（便于固化校准输入）。
func WriteDevSet(path string, devSet DevSet) error {
	data, err := json.MarshalIndent(devSet, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化开发集失败: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("写入开发集失败: %w", err)
	}
	return nil
}

// Validate 校验开发集结构，避免用无效输入做校准。
func (d DevSet) Validate() error {
	if len(d.Samples) == 0 {
		return fmt.Errorf("开发集 %q 没有样本", d.Name)
	}
	seen := make(map[string]struct{}, len(d.Samples))
	for i, sample := range d.Samples {
		if sample.ID == "" {
			return fmt.Errorf("开发集第 %d 条样本缺少 id", i)
		}
		if _, exists := seen[sample.ID]; exists {
			return fmt.Errorf("开发集存在重复样本 id: %s", sample.ID)
		}
		seen[sample.ID] = struct{}{}
		if len(sample.LayerConfidences) == 0 {
			return fmt.Errorf("样本 %s 缺少 layer_confidences", sample.ID)
		}
		for layerID, confidence := range sample.LayerConfidences {
			if confidence < 0 || confidence > 1 {
				return fmt.Errorf("样本 %s 层 %d 置信度越界: %v", sample.ID, layerID, confidence)
			}
		}
	}
	return nil
}
