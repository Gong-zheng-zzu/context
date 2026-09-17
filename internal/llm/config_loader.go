package llm

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// =============================================================================
// 配置文件加载器
// =============================================================================

// ConfigFile 配置文件结构
type ConfigFile struct {
	LLM LLMConfigFile `yaml:"llm"`
}

// LLMConfigFile LLM配置文件结构
type LLMConfigFile struct {
	Default   DefaultConfig                `yaml:"default"`
	Providers map[string]ProviderConfig    `yaml:"providers"`
	Routing   map[string]RoutingRuleConfig `yaml:"routing"`
	Prompt    PromptConfigFile             `yaml:"prompt"`
}

// DefaultConfig 默认配置
type DefaultConfig struct {
	PrimaryProvider  string `yaml:"primary_provider"`
	FallbackProvider string `yaml:"fallback_provider"`
	CacheEnabled     bool   `yaml:"cache_enabled"`
	CacheTTL         string `yaml:"cache_ttl"`
	MaxRetries       int    `yaml:"max_retries"`
	TimeoutSeconds   int    `yaml:"timeout_seconds"`
	EnableRouting    bool   `yaml:"enable_routing"`
}

// ProviderConfig 提供商配置
type ProviderConfig struct {
	APIKey     string                 `yaml:"api_key"`
	BaseURL    string                 `yaml:"base_url"`
	Model      string                 `yaml:"model"`
	MaxRetries int                    `yaml:"max_retries"`
	Timeout    string                 `yaml:"timeout"`
	RateLimit  int                    `yaml:"rate_limit"`
	Extra      map[string]interface{} `yaml:"extra"`
}

// RoutingRuleConfig 路由规则配置
type RoutingRuleConfig struct {
	Primary    string                 `yaml:"primary"`
	Fallback   []string               `yaml:"fallback"`
	Conditions map[string]interface{} `yaml:"conditions"`
}

// PromptConfigFile Prompt配置文件
type PromptConfigFile struct {
	DefaultLanguage string            `yaml:"default_language"`
	MaxTokens       int               `yaml:"max_tokens"`
	Temperature     float64           `yaml:"temperature"`
	CustomVars      map[string]string `yaml:"custom_vars"`
}

// ConfigLoader 配置加载器
type ConfigLoader struct {
	configPath string
	config     *ConfigFile
}

// NewConfigLoader 创建配置加载器
func NewConfigLoader(configPath string) *ConfigLoader {
	return &ConfigLoader{
		configPath: configPath,
	}
}

// LoadConfig 加载配置
func (cl *ConfigLoader) LoadConfig() error {
	// 读取配置文件
	data, err := os.ReadFile(cl.configPath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %w", err)
	}

	// 解析YAML
	var config ConfigFile
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 环境变量替换
	if err := cl.expandEnvVars(&config); err != nil {
		return fmt.Errorf("环境变量替换失败: %w", err)
	}

	cl.config = &config
	return nil
}

// expandEnvVars 展开环境变量
func (cl *ConfigLoader) expandEnvVars(config *ConfigFile) error {
	for providerName, providerConfig := range config.LLM.Providers {
		// 替换API密钥中的环境变量
		if strings.HasPrefix(providerConfig.APIKey, "${") && strings.HasSuffix(providerConfig.APIKey, "}") {
			envVar := strings.TrimSuffix(strings.TrimPrefix(providerConfig.APIKey, "${"), "}")
			envValue := os.Getenv(envVar)
			if envValue == "" {
				// 如果环境变量未设置，跳过该提供商（而不是报错）
				fmt.Printf("Warning: 环境变量 %s 未设置，跳过提供商 %s\n", envVar, providerName)
				delete(config.LLM.Providers, providerName)
				continue
			}
			providerConfig.APIKey = envValue
			config.LLM.Providers[providerName] = providerConfig
		}
	}
	return nil
}

// GetContextAwareLLMConfig 获取上下文感知LLM配置
func (cl *ConfigLoader) GetContextAwareLLMConfig() (*ContextAwareLLMConfig, error) {
	if cl.config == nil {
		return nil, fmt.Errorf("配置未加载")
	}

	defaultConfig := cl.config.LLM.Default

	// 解析缓存TTL
	cacheTTL, err := time.ParseDuration(defaultConfig.CacheTTL)
	if err != nil {
		cacheTTL = 30 * time.Minute
	}

	return &ContextAwareLLMConfig{
		PrimaryProvider:  LLMProvider(defaultConfig.PrimaryProvider),
		FallbackProvider: LLMProvider(defaultConfig.FallbackProvider),
		CacheEnabled:     defaultConfig.CacheEnabled,
		CacheTTL:         cacheTTL,
		MaxRetries:       defaultConfig.MaxRetries,
		TimeoutSeconds:   defaultConfig.TimeoutSeconds,
		EnableRouting:    defaultConfig.EnableRouting,
	}, nil
}

// GetProviderConfigs 获取提供商配置
func (cl *ConfigLoader) GetProviderConfigs() (map[LLMProvider]*LLMConfig, error) {
	if cl.config == nil {
		return nil, fmt.Errorf("配置未加载")
	}

	configs := make(map[LLMProvider]*LLMConfig)

	for providerName, providerConfig := range cl.config.LLM.Providers {
		// 解析超时时间
		timeout, err := time.ParseDuration(providerConfig.Timeout)
		if err != nil {
			timeout = 30 * time.Second
		}

		configs[LLMProvider(providerName)] = &LLMConfig{
			Provider:   LLMProvider(providerName),
			APIKey:     providerConfig.APIKey,
			BaseURL:    providerConfig.BaseURL,
			Model:      providerConfig.Model,
			MaxRetries: providerConfig.MaxRetries,
			Timeout:    timeout,
			RateLimit:  providerConfig.RateLimit,
			Extra:      providerConfig.Extra,
		}
	}

	return configs, nil
}

// GetPromptConfig 获取Prompt配置
func (cl *ConfigLoader) GetPromptConfig() (*PromptConfig, error) {
	if cl.config == nil {
		return nil, fmt.Errorf("配置未加载")
	}

	promptConfig := cl.config.LLM.Prompt

	return &PromptConfig{
		DefaultLanguage: promptConfig.DefaultLanguage,
		MaxTokens:       promptConfig.MaxTokens,
		Temperature:     promptConfig.Temperature,
		CustomVars:      promptConfig.CustomVars,
	}, nil
}

// GetRoutingRules 获取路由规则
func (cl *ConfigLoader) GetRoutingRules() (map[string]*RoutingRule, error) {
	if cl.config == nil {
		return nil, fmt.Errorf("配置未加载")
	}

	rules := make(map[string]*RoutingRule)

	for taskType, rule := range cl.config.LLM.Routing {
		fallbackProviders := make([]LLMProvider, len(rule.Fallback))
		for i, provider := range rule.Fallback {
			fallbackProviders[i] = LLMProvider(provider)
		}

		rules[taskType] = &RoutingRule{
			TaskType:          taskType,
			PreferredProvider: LLMProvider(rule.Primary),
			FallbackProviders: fallbackProviders,
			Conditions:        rule.Conditions,
		}
	}

	return rules, nil
}

// =============================================================================
// 全局配置管理器
// =============================================================================

// GlobalConfigManager 全局配置管理器
type GlobalConfigManager struct {
	loader  *ConfigLoader
	service *ContextAwareLLMService
}

var (
	globalConfigManager *GlobalConfigManager
)

// InitializeFromConfig 从配置文件初始化
func InitializeFromConfig(configPath string) error {
	// 如果没有提供路径，尝试默认路径
	if configPath == "" {
		// 尝试多个可能的路径
		possiblePaths := []string{
			"config/llm_config.yaml",
			"./config/llm_config.yaml",
			"../config/llm_config.yaml",
			"../../config/llm_config.yaml",
		}

		for _, path := range possiblePaths {
			if _, err := os.Stat(path); err == nil {
				configPath = path
				break
			}
		}

		if configPath == "" {
			return fmt.Errorf("未找到配置文件，请指定配置文件路径")
		}
	}

	// 创建配置加载器
	loader := NewConfigLoader(configPath)
	if err := loader.LoadConfig(); err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	// 获取配置
	contextConfig, err := loader.GetContextAwareLLMConfig()
	if err != nil {
		return fmt.Errorf("获取上下文配置失败: %w", err)
	}

	providerConfigs, err := loader.GetProviderConfigs()
	if err != nil {
		return fmt.Errorf("获取提供商配置失败: %w", err)
	}

	promptConfig, err := loader.GetPromptConfig()
	if err != nil {
		return fmt.Errorf("获取Prompt配置失败: %w", err)
	}

	// 设置全局配置
	for provider, config := range providerConfigs {
		SetGlobalConfig(provider, config)
	}

	// 创建服务
	service := NewContextAwareLLMService(contextConfig)
	service.promptManager = NewPromptManager(promptConfig)

	globalConfigManager = &GlobalConfigManager{
		loader:  loader,
		service: service,
	}

	return nil
}

// GetGlobalService 获取全局服务
func GetGlobalService() (*ContextAwareLLMService, error) {
	if globalConfigManager == nil {
		return nil, fmt.Errorf("全局配置管理器未初始化，请先调用 InitializeFromConfig")
	}
	return globalConfigManager.service, nil
}

// ReloadConfig 重新加载配置
func ReloadConfig() error {
	if globalConfigManager == nil {
		return fmt.Errorf("全局配置管理器未初始化")
	}

	if err := globalConfigManager.loader.LoadConfig(); err != nil {
		return fmt.Errorf("重新加载配置失败: %w", err)
	}

	// 重新初始化服务
	contextConfig, _ := globalConfigManager.loader.GetContextAwareLLMConfig()
	globalConfigManager.service.SetConfig(contextConfig)

	return nil
}

// FindConfigFile 查找配置文件
func FindConfigFile() (string, error) {
	// 获取当前工作目录
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// 向上查找配置文件
	for {
		configPath := filepath.Join(wd, "config", "llm_config.yaml")
		if _, err := os.Stat(configPath); err == nil {
			return configPath, nil
		}

		parent := filepath.Dir(wd)
		if parent == wd {
			break
		}
		wd = parent
	}

	return "", fmt.Errorf("未找到配置文件 config/llm_config.yaml")
}

// =============================================================================
// 默认 provider / model 选择（只读辅助）
// =============================================================================
//
// 背景：服务启动主链路（internal/services/llm_driven_context_service.go 的
// loadLLMDrivenConfig）历史上一只读取环境变量，导致 config/llm_config.yaml 中
// 的默认模型配置对主链路不生效。
//
// 这里新增一组「只读」辅助函数，用于在环境变量未设置时，从 config/llm_config.yaml
// 解析出默认 provider / model。它不会修改任何既有函数签名，也不会写入配置文件。
//
// 兼容两种 YAML 形态：
//  1. 嵌套 llm: 段（与 ConfigFile / README 描述的 llm_config.yaml 一致）：
//       llm:
//         default:
//           primary_provider: "deepseek"
//         providers:
//           deepseek:
//             model: "deepseek-chat"
//  2. 仓库当前实际使用的扁平形态（顶层 providers / models / environments）：
//       environments:
//         production:
//           active_providers: ["ollama_local", "deepseek"]
//           default_model: "deepseek-coder-v2:16b"
//
// 解析优先级：llm: 段 > 顶层 default_provider/default_model > environments 段。
// 若都取不到，返回 Source 为空的零值（调用方自行回退到内置默认值）。
//
// 兼容性影响（如实说明）：
//   - 只有当 LLM_PROVIDER / LLM_MODEL 环境变量未设置时，本回退才会生效，此时默认选择会
//     变为 environments.<env>（默认 production）的配置，例如 provider=ollama_local、
//     model=deepseek-coder-v2:16b。
//   - 生产部署通过 docker-compose 的 `env_file: ./config/.env` 注入了 LLM_PROVIDER /
//     LLM_MODEL，因此生产链路仍由环境变量决定，不受本回退影响。
//   - config/llm_config.yaml 的 environments.production.default_model 为
//     "deepseek-coder-v2:16b"（16B 模型），低显存机器上可能未安装该 Ollama 模型。这属于
//     配置文件表达的意图与运维配置范畴；本辅助函数刻意不做任何模型可用性探测。

// DefaultConfigPathEnv 允许通过环境变量覆盖默认配置文件的查找结果（主要用于测试隔离）。
const DefaultConfigPathEnv = "LLM_CONFIG_PATH"

// DefaultConfigEnvName 指定从 environments 段中读取哪个 profile（默认 production）。
const DefaultConfigEnvName = "LLM_CONFIG_ENV"

// DefaultLLMSelection 表示从配置文件中解析出的默认 LLM 提供商与模型。
type DefaultLLMSelection struct {
	Provider string
	Model    string
	// Source 描述该选择来自配置文件中的哪一段（如 "llm.default"、"environments.production"）。
	// 为空表示未从配置文件解析出任何可用值。
	Source string
}

// flatConfigSelection 仅用于读取扁平形态 config/llm_config.yaml 中的默认 provider/model。
// 不解析 two_stage_config / fallback_strategy / performance_settings 等其它段落。
type flatConfigSelection struct {
	DefaultProvider string `yaml:"default_provider"`
	DefaultModel    string `yaml:"default_model"`

	Environments map[string]struct {
		ActiveProviders []string `yaml:"active_providers"`
		DefaultModel    string   `yaml:"default_model"`
	} `yaml:"environments"`

	Models struct {
		LocalModels []modelRef `yaml:"local_models"`
		CloudModels []modelRef `yaml:"cloud_models"`
	} `yaml:"models"`
}

// modelRef 描述 models 段中单个模型与其所属 provider 的对应关系。
type modelRef struct {
	Name     string `yaml:"name"`
	Provider string `yaml:"provider"`
}

// ResolveDefaultConfigPath 返回用于读取默认 provider/model 的配置文件路径。
// 优先使用 LLM_CONFIG_PATH（便于测试隔离），否则复用 FindConfigFile 向上查找。
func ResolveDefaultConfigPath() (string, error) {
	if p := strings.TrimSpace(os.Getenv(DefaultConfigPathEnv)); p != "" {
		return p, nil
	}
	return FindConfigFile()
}

// LoadDefaultLLMSelection 只读地读取默认 LLM provider/model，不修改任何全局状态。
// 找不到任何可用值时返回零值（Provider/Model 均为空），调用方应回退到内置默认值。
func LoadDefaultLLMSelection() (DefaultLLMSelection, error) {
	path, err := ResolveDefaultConfigPath()
	if err != nil {
		return DefaultLLMSelection{}, err
	}
	return LoadDefaultLLMSelectionFromFile(path)
}

// LoadDefaultLLMSelectionFromFile 从指定文件解析默认 provider/model（便于测试注入临时文件）。
func LoadDefaultLLMSelectionFromFile(path string) (DefaultLLMSelection, error) {
	// 1) 先复用既有 ConfigLoader 解析嵌套 llm: 段。
	if selection, ok := loadSelectionViaConfigLoader(path); ok {
		return selection, nil
	}

	// 2) 回退到扁平形态（仓库当前 config/llm_config.yaml）。
	data, err := os.ReadFile(path)
	if err != nil {
		return DefaultLLMSelection{}, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var flat flatConfigSelection
	if err := yaml.Unmarshal(data, &flat); err != nil {
		return DefaultLLMSelection{}, fmt.Errorf("解析配置文件失败: %w", err)
	}

	return flat.resolve(), nil
}

// loadSelectionViaConfigLoader 复用既有 ConfigLoader 读取嵌套 llm: 段。
// 只有当 provider 与其 model 都能确定时才返回 ok=true。
func loadSelectionViaConfigLoader(path string) (DefaultLLMSelection, bool) {
	loader := NewConfigLoader(path)
	if err := loader.LoadConfig(); err != nil {
		return DefaultLLMSelection{}, false
	}

	contextConfig, err := loader.GetContextAwareLLMConfig()
	if err != nil || strings.TrimSpace(string(contextConfig.PrimaryProvider)) == "" {
		return DefaultLLMSelection{}, false
	}

	providerConfigs, err := loader.GetProviderConfigs()
	if err != nil {
		return DefaultLLMSelection{}, false
	}

	providerConfig, ok := providerConfigs[contextConfig.PrimaryProvider]
	if !ok || strings.TrimSpace(providerConfig.Model) == "" {
		return DefaultLLMSelection{}, false
	}

	return DefaultLLMSelection{
		Provider: strings.TrimSpace(string(contextConfig.PrimaryProvider)),
		Model:    strings.TrimSpace(providerConfig.Model),
		Source:   "llm.default",
	}, true
}

// resolve 从扁平形态配置中推导默认 provider/model，并记录来源（Source）。
func (f *flatConfigSelection) resolve() DefaultLLMSelection {
	selection := DefaultLLMSelection{
		Provider: strings.TrimSpace(f.DefaultProvider),
		Model:    strings.TrimSpace(f.DefaultModel),
	}
	if selection.Provider != "" || selection.Model != "" {
		selection.Source = "default_provider/default_model"
	}

	// 选择环境 profile：LLM_CONFIG_ENV 指定，默认 production；指定 profile 不存在时回退 production。
	profileName := strings.TrimSpace(os.Getenv(DefaultConfigEnvName))
	if profileName == "" {
		profileName = "production"
	}
	profile, ok := f.Environments[profileName]
	if !ok || (profile.DefaultModel == "" && len(profile.ActiveProviders) == 0) {
		profileName = "production"
		profile = f.Environments["production"]
	}

	if selection.Model == "" {
		if model := strings.TrimSpace(profile.DefaultModel); model != "" {
			selection.Model = model
			if selection.Source == "" {
				selection.Source = "environments." + profileName
			}
		}
	}
	if selection.Provider == "" {
		// 优先根据 default_model 反查其所属 provider，保证 provider/model 成对；
		// 查不到时退化为 active_providers 的第一个。
		if provider := f.providerForModel(selection.Model); provider != "" {
			selection.Provider = provider
		} else if len(profile.ActiveProviders) > 0 {
			selection.Provider = strings.TrimSpace(profile.ActiveProviders[0])
		}
		if selection.Provider != "" && selection.Source == "" {
			selection.Source = "environments." + profileName
		}
	}

	return selection
}

// providerForModel 在 models 段中查找指定模型名对应的 provider。
func (f *flatConfigSelection) providerForModel(model string) string {
	if model == "" {
		return ""
	}
	for _, candidate := range f.Models.LocalModels {
		if strings.EqualFold(strings.TrimSpace(candidate.Name), model) {
			return strings.TrimSpace(candidate.Provider)
		}
	}
	for _, candidate := range f.Models.CloudModels {
		if strings.EqualFold(strings.TrimSpace(candidate.Name), model) {
			return strings.TrimSpace(candidate.Provider)
		}
	}
	return ""
}
