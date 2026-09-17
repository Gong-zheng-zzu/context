package security

import (
	"fmt"
	"log"
	"math"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// CASIA 关键词配置来源标记。用于在配置版本/来源字段中区分「内置默认」与
// 「外部文件覆盖」，便于审计复现。
const (
	// CASIAKeywordSourceBuiltin 表示关键词表来自内置默认实现（initContextKeywords）。
	CASIAKeywordSourceBuiltin = "builtin"
	// CASIAKeywordSourceFile 表示关键词表来自外部配置文件。
	CASIAKeywordSourceFile = "file"
)

// casiaConfigFileSchema 是外部关键词配置文件的 YAML 结构。
//
// 对应 config/casia_keywords.yaml。使用 gopkg.in/yaml.v3（项目既有依赖），
// 不引入新的第三方依赖。
type casiaConfigFileSchema struct {
	Version           string                        `yaml:"version"`
	ContextWindow     int                           `yaml:"context_window_bytes"`
	DecisionThreshold float64                       `yaml:"decision_threshold"`
	KeywordWeights    map[string]map[string]float64 `yaml:"keyword_weights"`
}

// LoadCASIAConfigFromFile 从外部 YAML 文件加载 CASIA 关键词配置。
//
// 返回的配置 Version 会带 "-file" 后缀（若尚未带有），以区别于内置默认版本，
// 让 CASIAEvidence.ConfigurationVersion 能反映配置是否来自外部文件。
// 文件缺失、解析失败、结构非法（空类别/空关键词/非有限权重）时返回错误，
// 由调用方决定是否回退到内置默认值。
func LoadCASIAConfigFromFile(path string) (CASIAConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return CASIAConfig{}, fmt.Errorf("读取 CASIA 关键词配置失败: %w", err)
	}

	var schema casiaConfigFileSchema
	if err := yaml.Unmarshal(data, &schema); err != nil {
		return CASIAConfig{}, fmt.Errorf("解析 CASIA 关键词配置失败: %w", err)
	}
	if len(schema.KeywordWeights) == 0 {
		return CASIAConfig{}, fmt.Errorf("CASIA 关键词配置缺少 keyword_weights: %s", path)
	}
	for sensitiveType, keywords := range schema.KeywordWeights {
		if strings.TrimSpace(sensitiveType) == "" {
			return CASIAConfig{}, fmt.Errorf("CASIA 关键词配置存在空敏感类型: %s", path)
		}
		if len(keywords) == 0 {
			return CASIAConfig{}, fmt.Errorf("CASIA 关键词类别 %q 为空: %s", sensitiveType, path)
		}
		for keyword, weight := range keywords {
			if strings.TrimSpace(keyword) == "" {
				return CASIAConfig{}, fmt.Errorf("CASIA 关键词类别 %q 存在空关键词: %s", sensitiveType, path)
			}
			if math.IsNaN(weight) || math.IsInf(weight, 0) {
				return CASIAConfig{}, fmt.Errorf("CASIA 关键词 %q 权重非法(%v): %s", keyword, weight, path)
			}
		}
	}

	version := strings.TrimSpace(schema.Version)
	if version == "" {
		version = CASIAConfigVersion
	}
	if !strings.HasSuffix(version, "-"+CASIAKeywordSourceFile) {
		version += "-" + CASIAKeywordSourceFile
	}

	return CASIAConfig{
		Version:           version,
		ContextWindow:     schema.ContextWindow,
		DecisionThreshold: schema.DecisionThreshold,
		KeywordWeights:    schema.KeywordWeights,
		Source:            CASIAKeywordSourceFile,
	}, nil
}

// LoadCASIAConfigFromFileOrDefault 是回退包装：加载失败时返回内置默认配置
// （含 initContextKeywords 内置关键词表），并打警告日志。
func LoadCASIAConfigFromFileOrDefault(path string) CASIAConfig {
	config, err := LoadCASIAConfigFromFile(path)
	if err != nil {
		log.Printf("[CASIA] 外部关键词配置加载失败，回退到内置默认值: path=%s, err=%v", path, err)
		return NewContextAwareSensitiveInfoAlgorithm().GetConfiguration()
	}
	return config
}

// NewContextAwareSensitiveInfoAlgorithmFromFile 从外部配置文件构造 CASIA 算法。
//
// 文件缺失或非法时回退到内置默认权重（NewContextAwareSensitiveInfoAlgorithm）
// 并打警告日志，保证默认行为不变。
func NewContextAwareSensitiveInfoAlgorithmFromFile(path string) *ContextAwareSensitiveInfoAlgorithm {
	config, err := LoadCASIAConfigFromFile(path)
	if err != nil {
		log.Printf("[CASIA] 外部关键词配置加载失败，回退到内置默认权重: path=%s, err=%v", path, err)
		return NewContextAwareSensitiveInfoAlgorithm()
	}
	return NewContextAwareSensitiveInfoAlgorithmWithConfig(config)
}
