package knowledge

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// KnowledgeGraphConfig 知识图谱配置
type KnowledgeGraphConfig struct {
	// 功能开关
	Enabled              bool `json:"enabled"`                // 总开关
	EnableGraphRetrieval bool `json:"enable_graph_retrieval"` // 启用图谱检索
	EnableGraphStorage   bool `json:"enable_graph_storage"`   // 启用图谱存储
	EnableRRFFusion      bool `json:"enable_rrf_fusion"`      // 启用RRF融合

	// 检索配置
	DefaultStrategy        RetrievalStrategy `json:"default_strategy"`          // 默认检索策略
	AutoSelectStrategy     bool              `json:"auto_select_strategy"`      // 自动选择策略
	MaxGraphTraversalDepth int               `json:"max_graph_traversal_depth"` // 最大图遍历深度
	MinRetrievalScore      float64           `json:"min_retrieval_score"`       // 最小检索分数阈值

	// RRF配置
	RRFConfig *RRFConfig `json:"rrf_config"` // RRF融合配置

	// 存储配置
	ParallelStorageEnabled bool `json:"parallel_storage_enabled"` // 并行存储开关
	VectorStorageRequired  bool `json:"vector_storage_required"`  // 向量存储必须成功

	// Neo4j配置
	Neo4jConfig *Neo4jConfig `json:"neo4j_config"`
}

// DefaultKnowledgeGraphConfig 默认配置
func DefaultKnowledgeGraphConfig() *KnowledgeGraphConfig {
	return &KnowledgeGraphConfig{
		Enabled:                true,
		EnableGraphRetrieval:   true,
		EnableGraphStorage:     true,
		EnableRRFFusion:        true,
		DefaultStrategy:        StrategyVectorOnly,
		AutoSelectStrategy:     true,
		MaxGraphTraversalDepth: 2,
		MinRetrievalScore:      0.5,
		RRFConfig:              DefaultRRFConfig(),
		ParallelStorageEnabled: true,
		VectorStorageRequired:  true,
	}
}

// LoadConfigFromEnv 从环境变量加载配置
func LoadConfigFromEnv() *KnowledgeGraphConfig {
	config := DefaultKnowledgeGraphConfig()

	// 总开关
	if val := os.Getenv("KNOWLEDGE_GRAPH_ENABLED"); val != "" {
		config.Enabled = parseBool(val, true)
	}

	// 图谱检索开关
	if val := os.Getenv("ENABLE_GRAPH_RETRIEVAL"); val != "" {
		config.EnableGraphRetrieval = parseBool(val, true)
	}

	// 图谱存储开关
	if val := os.Getenv("ENABLE_GRAPH_STORAGE"); val != "" {
		config.EnableGraphStorage = parseBool(val, true)
	}

	// RRF融合开关
	if val := os.Getenv("ENABLE_RRF_FUSION"); val != "" {
		config.EnableRRFFusion = parseBool(val, true)
	}

	// 自动选择策略
	if val := os.Getenv("AUTO_SELECT_STRATEGY"); val != "" {
		config.AutoSelectStrategy = parseBool(val, true)
	}

	// 默认策略
	if val := os.Getenv("DEFAULT_RETRIEVAL_STRATEGY"); val != "" {
		switch strings.ToLower(val) {
		case "time_recall":
			config.DefaultStrategy = StrategyTimeRecall
		case "graph_priority":
			config.DefaultStrategy = StrategyGraphPriority
		case "time_content_hybrid":
			config.DefaultStrategy = StrategyTimeContentHybrid
		default:
			config.DefaultStrategy = StrategyVectorOnly
		}
	}

	// 最大遍历深度
	if val := os.Getenv("MAX_GRAPH_TRAVERSAL_DEPTH"); val != "" {
		if depth, err := strconv.Atoi(val); err == nil && depth > 0 {
			config.MaxGraphTraversalDepth = depth
		}
	}

	// 最小检索分数
	if val := os.Getenv("MIN_RETRIEVAL_SCORE"); val != "" {
		if score, err := strconv.ParseFloat(val, 64); err == nil {
			config.MinRetrievalScore = score
		}
	}

	// RRF K参数
	if val := os.Getenv("RRF_K"); val != "" {
		if k, err := strconv.ParseFloat(val, 64); err == nil {
			config.RRFConfig.K = k
		}
	}

	// Neo4j配置
	config.Neo4jConfig = LoadNeo4jConfigFromEnv()

	return config
}

// LoadNeo4jConfigFromEnv 从环境变量加载Neo4j配置
func LoadNeo4jConfigFromEnv() *Neo4jConfig {
	return &Neo4jConfig{
		URI:                     getEnvWithDefault("NEO4J_URI", "bolt://localhost:7687"),
		Username:                getEnvWithDefault("NEO4J_USERNAME", "neo4j"),
		Password:                getEnvWithDefault("NEO4J_PASSWORD", ""),
		Database:                getEnvWithDefault("NEO4J_DATABASE", "neo4j"),
		MaxConnectionPoolSize:   parseInt(os.Getenv("NEO4J_MAX_POOL_SIZE"), 50),
		ConnectionTimeout:       parseDuration(os.Getenv("NEO4J_CONN_TIMEOUT"), 30),
		MaxTransactionRetryTime: parseDuration(os.Getenv("NEO4J_RETRY_TIME"), 30),
	}
}

// IsGraphRetrievalEnabled 检查图谱检索是否启用
func (c *KnowledgeGraphConfig) IsGraphRetrievalEnabled() bool {
	return c.Enabled && c.EnableGraphRetrieval
}

// IsGraphStorageEnabled 检查图谱存储是否启用
func (c *KnowledgeGraphConfig) IsGraphStorageEnabled() bool {
	return c.Enabled && c.EnableGraphStorage
}

// IsRRFFusionEnabled 检查RRF融合是否启用
func (c *KnowledgeGraphConfig) IsRRFFusionEnabled() bool {
	return c.Enabled && c.EnableRRFFusion
}

// 辅助函数
func parseBool(val string, defaultVal bool) bool {
	val = strings.ToLower(strings.TrimSpace(val))
	if val == "true" || val == "1" || val == "yes" || val == "on" {
		return true
	}
	if val == "false" || val == "0" || val == "no" || val == "off" {
		return false
	}
	return defaultVal
}

func getEnvWithDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func parseInt(val string, defaultVal int) int {
	if val == "" {
		return defaultVal
	}
	if i, err := strconv.Atoi(val); err == nil {
		return i
	}
	return defaultVal
}

func parseDuration(val string, defaultSeconds int) time.Duration {
	if val == "" {
		return time.Duration(defaultSeconds) * time.Second
	}
	if d, err := time.ParseDuration(val); err == nil {
		return d
	}
	if i, err := strconv.Atoi(val); err == nil {
		return time.Duration(i) * time.Second
	}
	return time.Duration(defaultSeconds) * time.Second
}
