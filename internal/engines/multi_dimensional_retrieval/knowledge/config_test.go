package knowledge

import (
	"os"
	"testing"
)

func TestDefaultKnowledgeGraphConfig(t *testing.T) {
	config := DefaultKnowledgeGraphConfig()

	if !config.Enabled {
		t.Error("默认应该启用")
	}

	if !config.EnableGraphRetrieval {
		t.Error("默认应该启用图谱检索")
	}

	if !config.EnableGraphStorage {
		t.Error("默认应该启用图谱存储")
	}

	if !config.EnableRRFFusion {
		t.Error("默认应该启用RRF融合")
	}

	if config.DefaultStrategy != StrategyVectorOnly {
		t.Errorf("默认策略应该是vector_only, got: %s", config.DefaultStrategy)
	}

	if !config.AutoSelectStrategy {
		t.Error("默认应该自动选择策略")
	}

	if config.MaxGraphTraversalDepth != 2 {
		t.Errorf("默认遍历深度应该是2, got: %d", config.MaxGraphTraversalDepth)
	}

	if config.RRFConfig == nil {
		t.Error("RRFConfig不应该为nil")
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	// 设置测试环境变量
	os.Setenv("KNOWLEDGE_GRAPH_ENABLED", "false")
	os.Setenv("ENABLE_GRAPH_RETRIEVAL", "true")
	os.Setenv("DEFAULT_RETRIEVAL_STRATEGY", "graph_priority")
	os.Setenv("MAX_GRAPH_TRAVERSAL_DEPTH", "3")
	os.Setenv("RRF_K", "100")
	defer func() {
		os.Unsetenv("KNOWLEDGE_GRAPH_ENABLED")
		os.Unsetenv("ENABLE_GRAPH_RETRIEVAL")
		os.Unsetenv("DEFAULT_RETRIEVAL_STRATEGY")
		os.Unsetenv("MAX_GRAPH_TRAVERSAL_DEPTH")
		os.Unsetenv("RRF_K")
	}()

	config := LoadConfigFromEnv()

	if config.Enabled {
		t.Error("应该被禁用")
	}

	if !config.EnableGraphRetrieval {
		t.Error("应该启用图谱检索")
	}

	if config.DefaultStrategy != StrategyGraphPriority {
		t.Errorf("策略应该是graph_priority, got: %s", config.DefaultStrategy)
	}

	if config.MaxGraphTraversalDepth != 3 {
		t.Errorf("遍历深度应该是3, got: %d", config.MaxGraphTraversalDepth)
	}

	if config.RRFConfig.K != 100.0 {
		t.Errorf("RRF K应该是100, got: %f", config.RRFConfig.K)
	}
}

func TestIsGraphRetrievalEnabled(t *testing.T) {
	config := DefaultKnowledgeGraphConfig()

	// 默认情况：全部启用
	if !config.IsGraphRetrievalEnabled() {
		t.Error("默认应该启用图谱检索")
	}

	// 禁用总开关
	config.Enabled = false
	if config.IsGraphRetrievalEnabled() {
		t.Error("总开关禁用后，图谱检索应该禁用")
	}

	// 重新启用总开关，禁用图谱检索
	config.Enabled = true
	config.EnableGraphRetrieval = false
	if config.IsGraphRetrievalEnabled() {
		t.Error("图谱检索开关禁用后，应该禁用")
	}
}

func TestIsGraphStorageEnabled(t *testing.T) {
	config := DefaultKnowledgeGraphConfig()

	if !config.IsGraphStorageEnabled() {
		t.Error("默认应该启用图谱存储")
	}

	config.Enabled = false
	if config.IsGraphStorageEnabled() {
		t.Error("总开关禁用后，图谱存储应该禁用")
	}
}

func TestParseBool(t *testing.T) {
	tests := []struct {
		input    string
		defaultV bool
		expected bool
	}{
		{"true", false, true},
		{"TRUE", false, true},
		{"1", false, true},
		{"yes", false, true},
		{"on", false, true},
		{"false", true, false},
		{"FALSE", true, false},
		{"0", true, false},
		{"no", true, false},
		{"off", true, false},
		{"invalid", true, true},   // 返回默认值
		{"invalid", false, false}, // 返回默认值
		{"", true, true},          // 空字符串返回默认值
	}

	for _, test := range tests {
		result := parseBool(test.input, test.defaultV)
		if result != test.expected {
			t.Errorf("parseBool(%q, %v) = %v, 期望 %v", 
				test.input, test.defaultV, result, test.expected)
		}
	}
}

func TestGetEnvWithDefault(t *testing.T) {
	testKey := "TEST_ENV_KEY_12345"
	
	// 环境变量不存在时返回默认值
	result := getEnvWithDefault(testKey, "default_value")
	if result != "default_value" {
		t.Errorf("期望默认值, got: %s", result)
	}

	// 设置环境变量
	os.Setenv(testKey, "actual_value")
	defer os.Unsetenv(testKey)

	result = getEnvWithDefault(testKey, "default_value")
	if result != "actual_value" {
		t.Errorf("期望actual_value, got: %s", result)
	}
}

func TestLoadNeo4jConfigFromEnv(t *testing.T) {
	// 设置Neo4j环境变量
	os.Setenv("NEO4J_URI", "bolt://test:7687")
	os.Setenv("NEO4J_USERNAME", "testuser")
	os.Setenv("NEO4J_PASSWORD", "testpass")
	os.Setenv("NEO4J_DATABASE", "testdb")
	defer func() {
		os.Unsetenv("NEO4J_URI")
		os.Unsetenv("NEO4J_USERNAME")
		os.Unsetenv("NEO4J_PASSWORD")
		os.Unsetenv("NEO4J_DATABASE")
	}()

	config := LoadNeo4jConfigFromEnv()

	if config.URI != "bolt://test:7687" {
		t.Errorf("URI错误: %s", config.URI)
	}

	if config.Username != "testuser" {
		t.Errorf("Username错误: %s", config.Username)
	}

	if config.Password != "testpass" {
		t.Errorf("Password错误: %s", config.Password)
	}

	if config.Database != "testdb" {
		t.Errorf("Database错误: %s", config.Database)
	}
}

