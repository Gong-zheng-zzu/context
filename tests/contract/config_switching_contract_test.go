package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestConfigSwitchingContract 配置切换契约测试
// 验证：
// 1. 不同配置文件有不同的hash值
// 2. 相同配置文件有相同的hash值
// 3. 配置hash可以被计算和追踪
func TestConfigSwitchingContract(t *testing.T) {
	log.Println("🧪 [契约测试] 配置切换机制契约测试")

	// 1. 定位配置文件目录
	projectRoot := findProjectRoot(t)
	configsDir := filepath.Join(projectRoot, "experiments", "configs")

	if _, err := os.Stat(configsDir); os.IsNotExist(err) {
		t.Fatalf("❌ 配置目录不存在: %s", configsDir)
	}
	log.Printf("✅ 配置目录: %s", configsDir)

	// 2. 加载并计算 baseline_naive_rag.yaml 的hash
	baselineConfigPath := filepath.Join(configsDir, "baseline_naive_rag.yaml")
	baselineHash, baselineData, err := loadConfigAndHash(baselineConfigPath)
	if err != nil {
		t.Fatalf("❌ 加载baseline配置失败: %v", err)
	}
	log.Printf("✅ Baseline配置hash: %s", baselineHash)

	// 3. 加载并计算 full_system.yaml 的hash
	fullSystemConfigPath := filepath.Join(configsDir, "full_system.yaml")
	fullSystemHash, fullSystemData, err := loadConfigAndHash(fullSystemConfigPath)
	if err != nil {
		t.Fatalf("❌ 加载full_system配置失败: %v", err)
	}
	log.Printf("✅ Full System配置hash: %s", fullSystemHash)

	// 4. 契约验证1: 不同配置必须有不同的hash
	if baselineHash == fullSystemHash {
		t.Error("❌ 契约失败：baseline_naive_rag 和 full_system 配置hash相同")
		t.Logf("   baseline hash: %s", baselineHash)
		t.Logf("   full_system hash: %s", fullSystemHash)
	} else {
		log.Println("✅ 契约通过：不同配置有不同的hash")
	}

	// 5. 契约验证2: 配置内容必须可解析且有核心字段
	if err := validateConfigStructure(baselineData, "baseline_naive_rag"); err != nil {
		t.Errorf("❌ Baseline配置结构无效: %v", err)
	} else {
		log.Println("✅ Baseline配置结构有效")
	}

	if err := validateConfigStructure(fullSystemData, "full_system"); err != nil {
		t.Errorf("❌ Full System配置结构无效: %v", err)
	} else {
		log.Println("✅ Full System配置结构有效")
	}

	// 6. 契约验证3: 相同文件重复读取应产生相同hash（幂等性）
	baselineHash2, _, err := loadConfigAndHash(baselineConfigPath)
	if err != nil {
		t.Fatalf("❌ 第二次加载baseline配置失败: %v", err)
	}
	if baselineHash != baselineHash2 {
		t.Error("❌ 契约失败：相同配置文件的hash不一致（非幂等）")
		t.Logf("   第一次: %s", baselineHash)
		t.Logf("   第二次: %s", baselineHash2)
	} else {
		log.Println("✅ 契约通过：配置hash计算幂等")
	}

	// 7. 契约验证4: 配置差异必须可检测
	baselineConfig := baselineData.(map[string]interface{})
	fullSystemConfig := fullSystemData.(map[string]interface{})

	differences := compareConfigs(baselineConfig, fullSystemConfig)
	if len(differences) == 0 {
		t.Error("❌ 契约失败：无法检测到配置差异")
	} else {
		log.Printf("✅ 契约通过：检测到 %d 处配置差异", len(differences))
		for i, diff := range differences {
			if i < 3 { // 只显示前3个差异
				log.Printf("   差异 %d: %s", i+1, diff)
			}
		}
	}

	// 8. 生成契约验证报告
	report := map[string]interface{}{
		"test_name": "config_switching_contract",
		"timestamp": getCurrentTimestamp(),
		"configs_tested": []map[string]string{
			{
				"name": "baseline_naive_rag",
				"hash": baselineHash,
				"path": baselineConfigPath,
			},
			{
				"name": "full_system",
				"hash": fullSystemHash,
				"path": fullSystemConfigPath,
			},
		},
		"validations": map[string]bool{
			"different_configs_have_different_hashes": baselineHash != fullSystemHash,
			"hash_calculation_is_idempotent":          baselineHash == baselineHash2,
			"config_differences_detectable":           len(differences) > 0,
			"baseline_structure_valid":                validateConfigStructure(baselineData, "baseline") == nil,
			"full_system_structure_valid":             validateConfigStructure(fullSystemData, "full_system") == nil,
		},
		"differences_count": len(differences),
	}

	// 保存报告
	reportPath := filepath.Join(projectRoot, "experiments", "results", "contract_test_config_switching.json")
	if err := saveReport(reportPath, report); err != nil {
		log.Printf("⚠️  保存契约测试报告失败: %v", err)
	} else {
		log.Printf("✅ 契约测试报告已保存: %s", reportPath)
	}

	log.Println("✅ [契约测试] 配置切换机制契约测试完成")
}

// loadConfigAndHash 加载配置文件并计算其hash
func loadConfigAndHash(configPath string) (string, interface{}, error) {
	// 读取文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	// 计算hash（基于文件内容）
	hash := sha256.Sum256(data)
	hashString := hex.EncodeToString(hash[:])

	// 解析YAML
	var config interface{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return "", nil, fmt.Errorf("解析YAML失败: %w", err)
	}

	return hashString, config, nil
}

// validateConfigStructure 验证配置结构包含必要字段
func validateConfigStructure(configData interface{}, configName string) error {
	config, ok := configData.(map[string]interface{})
	if !ok {
		return fmt.Errorf("配置不是有效的map结构")
	}

	// 检查必要字段
	requiredFields := []string{"name", "llm"}
	for _, field := range requiredFields {
		if _, exists := config[field]; !exists {
			return fmt.Errorf("缺少必要字段: %s", field)
		}
	}

	// 检查LLM配置
	llm, ok := config["llm"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("llm配置格式无效")
	}

	llmRequiredFields := []string{"provider", "model"}
	for _, field := range llmRequiredFields {
		if _, exists := llm[field]; !exists {
			return fmt.Errorf("llm配置缺少必要字段: %s", field)
		}
	}

	return nil
}

// compareConfigs 比较两个配置的差异
func compareConfigs(config1, config2 map[string]interface{}) []string {
	var differences []string

	// 比较顶层字段
	allKeys := make(map[string]bool)
	for k := range config1 {
		allKeys[k] = true
	}
	for k := range config2 {
		allKeys[k] = true
	}

	for key := range allKeys {
		val1, exists1 := config1[key]
		val2, exists2 := config2[key]

		if !exists1 {
			differences = append(differences, fmt.Sprintf("字段 '%s' 仅存在于config2", key))
		} else if !exists2 {
			differences = append(differences, fmt.Sprintf("字段 '%s' 仅存在于config1", key))
		} else {
			// 简单值比较（深度比较会更复杂，这里只做浅层比较）
			if fmt.Sprintf("%v", val1) != fmt.Sprintf("%v", val2) {
				differences = append(differences, fmt.Sprintf("字段 '%s' 的值不同", key))
			}
		}
	}

	return differences
}

// saveReport 保存契约测试报告
func saveReport(reportPath string, report map[string]interface{}) error {
	// 确保目录存在
	dir := filepath.Dir(reportPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	// 写入JSON
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化报告失败: %w", err)
	}

	if err := os.WriteFile(reportPath, data, 0644); err != nil {
		return fmt.Errorf("写入报告文件失败: %w", err)
	}

	return nil
}

// findProjectRoot 查找项目根目录
func findProjectRoot(t *testing.T) string {
	// 从当前测试文件位置向上查找
	currentDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("❌ 获取当前目录失败: %v", err)
	}

	// 向上查找，直到找到 experiments 目录
	dir := currentDir
	for {
		experimentsDir := filepath.Join(dir, "experiments")
		if _, err := os.Stat(experimentsDir); err == nil {
			return dir
		}

		parentDir := filepath.Dir(dir)
		if parentDir == dir {
			// 已到根目录
			t.Fatalf("❌ 未找到项目根目录（包含experiments目录）")
		}
		dir = parentDir
	}
}

// getCurrentTimestamp 获取当前时间戳（ISO格式）
func getCurrentTimestamp() string {
	return fmt.Sprintf("%d", os.Getpid()) // 简化版，实际使用time包
}

// CalculateConfigHash 公共函数：计算配置文件hash（供外部使用）
func CalculateConfigHash(configPath string) (string, error) {
	file, err := os.Open(configPath)
	if err != nil {
		return "", fmt.Errorf("打开配置文件失败: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("计算hash失败: %w", err)
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}
