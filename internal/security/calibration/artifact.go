package calibration

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"os"

	"github.com/contextkeeper/service/internal/security"
)

// SchemaVersion 是校准产物 JSON 的 schema 版本。
const SchemaVersion = "pccm-calibration-artifact-v1"

// Fingerprint 是校准产物的可追溯指纹：记录数据集哈希、样本数、生成时间、
// 搜索空间与目标函数，使权重可复现、可审计。
type Fingerprint struct {
	CalibrationSource string `json:"calibration_source"`
	ArtifactSchema    string `json:"artifact_schema"`
	DatasetName       string `json:"dataset_name"`
	DatasetSource     string `json:"dataset_source"`
	DatasetHash       string `json:"dataset_hash"`
	SampleCount       int    `json:"sample_count"`
	GeneratedAt       string `json:"generated_at"`
	Objective         string `json:"objective"`
	SearchSpace       string `json:"search_space"`
	Evaluations       int    `json:"evaluations"`
	Tool              string `json:"tool"`
}

// Artifact 是离线校准的完整产物：校准后的 PCCM-S 配置 + 指纹 + 校准前后指标。
type Artifact struct {
	SchemaVersion string                      `json:"schema_version"`
	Fingerprint   Fingerprint                 `json:"fingerprint"`
	Config        security.PCCMSecurityConfig `json:"config"`
	Search        SearchSummary               `json:"search"`
	MetricsBefore Metrics                     `json:"metrics_before"`
	MetricsAfter  Metrics                     `json:"metrics_after"`
	Notes         string                      `json:"notes,omitempty"`
}

// Validate 校验产物是否可安全加载。
func (a Artifact) Validate() error {
	if a.SchemaVersion == "" {
		return errors.New("校准产物缺少 schema_version")
	}
	if len(a.Config.LayerWeights) == 0 {
		return errors.New("校准产物缺少层权重 layer_weights")
	}
	for layerID, weight := range a.Config.LayerWeights {
		if math.IsNaN(weight) || math.IsInf(weight, 0) || weight < 0 {
			return fmt.Errorf("校准产物层 %d 权重非法: %v", layerID, weight)
		}
	}
	return nil
}

// MarshalIndent 序列化产物（带缩进，便于人工审阅与版本化）。
func (a Artifact) MarshalIndent() ([]byte, error) {
	return json.MarshalIndent(a, "", "  ")
}

// WriteArtifact 把产物写入指定路径。
func WriteArtifact(path string, artifact Artifact) error {
	data, err := artifact.MarshalIndent()
	if err != nil {
		return fmt.Errorf("序列化校准产物失败: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("写入校准产物失败: %w", err)
	}
	return nil
}

// ParseArtifact 从 JSON 字节反序列化产物。
func ParseArtifact(data []byte) (Artifact, error) {
	var artifact Artifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		return Artifact{}, fmt.Errorf("解析校准产物失败: %w", err)
	}
	return artifact, nil
}

// LoadArtifact 从文件读取并解析校准产物。
func LoadArtifact(path string) (Artifact, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Artifact{}, fmt.Errorf("读取校准产物失败: %w", err)
	}
	return ParseArtifact(data)
}

// LoadPCCMConfigFromFile 从校准产物文件加载 PCCM-S 配置。
//
// 仅在显式调用时生效，不会改变 security.DefaultPCCMSecurityConfig() 或任何
// 默认构造路径。文件缺失、解析失败或校验失败时返回错误，由调用方决定回退策略。
func LoadPCCMConfigFromFile(path string) (security.PCCMSecurityConfig, error) {
	artifact, err := LoadArtifact(path)
	if err != nil {
		return security.PCCMSecurityConfig{}, err
	}
	if err := artifact.Validate(); err != nil {
		return security.PCCMSecurityConfig{}, fmt.Errorf("校准产物校验失败: %w", err)
	}
	// 经 ProgressiveConfidenceModel 归一化，补齐可能缺失的零值字段。
	return security.NewProgressiveConfidenceModelWithConfig(artifact.Config).Config(), nil
}

// LoadPCCMConfigOrDefault 是回退包装：任何加载失败都回退到默认固定权重配置
// （security.DefaultPCCMSecurityConfig）并打警告日志，保证默认行为不变。
func LoadPCCMConfigOrDefault(path string) security.PCCMSecurityConfig {
	config, err := LoadPCCMConfigFromFile(path)
	if err != nil {
		log.Printf("[PCCM校准] 校准产物加载失败，回退到默认固定权重: path=%s, err=%v", path, err)
		return security.DefaultPCCMSecurityConfig()
	}
	return config
}
