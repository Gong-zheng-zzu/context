package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config 应用配置
type Config struct {
	// 服务配置
	ServiceName string
	Port        int
	Debug       bool
	StoragePath string
	Host        string // 服务监听地址
	GinMode     string // Gin运行模式

	// 向量存储配置
	VectorStoreType string // 向量存储类型: aliyun, vearch

	// 用户存储配置
	UserRepositoryType string // 用户存储类型: aliyun, vearch, memory, mysql, tencent

	// 阿里云文本嵌入配置
	EmbeddingAPIURL string
	EmbeddingAPIKey string

	// 🔥 新增：批量embedding配置
	BatchEmbeddingAPIURL    string        // 批量embedding API端点
	BatchEmbeddingAPIKey    string        // 批量embedding API密钥
	BatchQueueSize          int           // 批量任务队列大小
	BatchWorkerPollInterval time.Duration // Worker轮询间隔
	BatchMaxRetries         int           // 最大重试次数

	// 阿里云向量数据库配置
	VectorDBURL         string
	VectorDBAPIKey      string
	VectorDBCollection  string
	VectorDBDimension   int
	VectorDBMetric      string
	SimilarityThreshold float64

	// 服务器端口配置
	HTTPServerPort      string // HTTP服务端口
	WebSocketServerPort string // WebSocket服务端口

	// ======== 时间阈值配置 ========
	// 会话管理相关
	SessionTimeout    time.Duration // 会话超时时间，默认30分钟
	CleanupInterval   time.Duration // 清理检查间隔，默认10分钟
	ShortMemoryMaxAge int           // 短期记忆保留天数，默认2天

	// 自动汇总相关
	SummaryIntervalMultiplier int // 自动汇总间隔倍数（相对于清理间隔），默认5倍
	MinMessageCount           int // 最小消息数阈值，少于此数量不汇总，默认20
	MinTimeSinceLastSummary   int // 距离上次汇总的最小小时数，默认24小时
	MaxMessageCount           int // 触发汇总的消息数阈值，默认100

	// 多维度存储配置
	EnableMultiDimensionalStorage bool   `json:"enable_multi_dimensional_storage"` // 多维度存储总开关
	MultiDimTimelineEnabled       bool   `json:"multi_dim_timeline_enabled"`       // 时间线存储开关
	MultiDimKnowledgeEnabled      bool   `json:"multi_dim_knowledge_enabled"`      // 知识图谱存储开关
	MultiDimVectorEnabled         bool   `json:"multi_dim_vector_enabled"`         // 增强向量存储开关
	MultiDimLLMProvider           string `json:"multi_dim_llm_provider"`           // LLM提供商
	MultiDimLLMModel              string `json:"multi_dim_llm_model"`              // LLM模型
}

// Load 从环境变量加载配置
func Load() *Config {
	// 尝试加载.env文件，优先尝试新的目录结构，然后兼容原来的结构
	envPaths := []string{
		"config/.env",
		".env",
	}

	loaded := false
	for _, path := range envPaths {
		if _, err := os.Stat(path); err == nil {
			if err := godotenv.Load(path); err == nil {
				log.Printf("成功加载.env文件: %s", path)
				loaded = true
				break
			}
		}
	}

	if !loaded {
		log.Printf("警告: 未找到.env文件，尝试使用系统环境变量")
	}

	// Preserve legacy experiment flags while allowing an explicit master override.
	legacyMultiDimEnabled := getEnvAsBool("TIMELINE_ENABLED", false) || getEnvAsBool("GRAPH_STORE_ENABLED", false)
	multiDimEnabled := getEnvAsBool("ENABLE_MULTI_DIMENSIONAL_STORAGE", legacyMultiDimEnabled)

	// 创建配置实例
	config := &Config{
		// 服务配置默认值
		ServiceName: getEnv("SERVICE_NAME", "context-keeper"),
		Port:        getEnvAsInt("PORT", 8088),
		Debug:       getEnvAsBool("DEBUG", false),
		StoragePath: getStoragePathDefault(),
		Host:        getEnv("HOST", "0.0.0.0"),
		GinMode:     getEnv("GIN_MODE", "release"),

		// 向量存储配置
		VectorStoreType: getEnv("VECTOR_STORE_TYPE", "aliyun"),

		// 用户存储配置
		UserRepositoryType: getEnv("USER_REPOSITORY_TYPE", "aliyun"),

		// 嵌入服务配置
		EmbeddingAPIURL: getEnv("EMBEDDING_API_URL", "https://dashscope.aliyuncs.com/compatible-mode/v1/embeddings"),
		EmbeddingAPIKey: getEnv("EMBEDDING_API_KEY", ""),

		// 🔥 新增：批量embedding配置
		BatchEmbeddingAPIURL:    getEnv("BATCH_EMBEDDING_API_URL", "https://dashscope.aliyuncs.com/api/v1/services/embeddings/text-embedding/text-embedding"),
		BatchEmbeddingAPIKey:    getEnv("BATCH_EMBEDDING_API_KEY", getEnv("EMBEDDING_API_KEY", "")), // 默认继承单一embedding的API密钥
		BatchQueueSize:          getEnvAsInt("BATCH_QUEUE_SIZE", 100),                               // 默认队列大小100
		BatchWorkerPollInterval: getEnvAsDuration("BATCH_WORKER_POLL_INTERVAL", 5*time.Second),      // 默认轮询间隔5秒
		BatchMaxRetries:         getEnvAsInt("BATCH_MAX_RETRIES", 3),                                // 默认最大重试3次

		// 向量数据库配置
		VectorDBURL:         getEnv("VECTOR_DB_URL", ""),
		VectorDBAPIKey:      getEnv("VECTOR_DB_API_KEY", ""),
		VectorDBCollection:  getEnv("VECTOR_DB_COLLECTION", "context_keeper"),
		VectorDBDimension:   getEnvAsInt("VECTOR_DB_DIMENSION", 1536),
		VectorDBMetric:      getEnv("VECTOR_DB_METRIC", "cosine"),
		SimilarityThreshold: getEnvAsFloat("SIMILARITY_THRESHOLD", 0.5),

		// 服务器端口配置
		HTTPServerPort:      getEnv("HTTP_SERVER_PORT", "8088"),
		WebSocketServerPort: getEnv("WEBSOCKET_SERVER_PORT", "8088"),

		// ======== 时间阈值配置 ========
		// 会话管理相关
		SessionTimeout:    getEnvAsDuration("SESSION_TIMEOUT", 30*time.Minute),
		CleanupInterval:   getEnvAsDuration("CLEANUP_INTERVAL", 10*time.Minute),
		ShortMemoryMaxAge: getEnvAsInt("SHORT_MEMORY_MAX_AGE", 2),

		// 自动汇总相关
		SummaryIntervalMultiplier: getEnvAsInt("SUMMARY_INTERVAL_MULTIPLIER", 5),
		MinMessageCount:           getEnvAsInt("MIN_MESSAGE_COUNT", 20),
		MinTimeSinceLastSummary:   getEnvAsInt("MIN_TIME_SINCE_LAST_SUMMARY", 24),
		MaxMessageCount:           getEnvAsInt("MAX_MESSAGE_COUNT", 100),

		// 多维度存储配置
		EnableMultiDimensionalStorage: multiDimEnabled,
		MultiDimTimelineEnabled:       getEnvAsBool("MULTI_DIM_TIMELINE_ENABLED", getEnvAsBool("TIMELINE_ENABLED", false)),
		MultiDimKnowledgeEnabled:      getEnvAsBool("MULTI_DIM_KNOWLEDGE_ENABLED", getEnvAsBool("GRAPH_STORE_ENABLED", false)),
		MultiDimVectorEnabled:         getEnvAsBool("MULTI_DIM_VECTOR_ENABLED", true), // 向量存储默认启用
		MultiDimLLMProvider:           getEnv("MULTI_DIM_LLM_PROVIDER", "deepseek"),
		MultiDimLLMModel:              getEnv("MULTI_DIM_LLM_MODEL", "deepseek-chat"),
	}

	// 确保存储路径存在
	if err := ensureDir(config.StoragePath); err != nil {
		log.Printf("警告: 创建存储目录失败: %v", err)
	}

	return config
}

// String 返回配置的字符串表示
func (c *Config) String() string {
	return fmt.Sprintf(
		"服务名称: %s, 端口: %d, 调试模式: %v, 存储路径: %s, 向量DB: %s, 嵌入API: %s, "+
			"会话超时: %v, 清理间隔: %v, 短期记忆保留: %d天, 汇总间隔倍数: %dx, "+
			"最小消息数: %d, 最大消息数: %d, 汇总间隔: %d小时",
		c.ServiceName, c.Port, c.Debug, c.StoragePath,
		maskString(c.VectorDBURL), maskString(c.EmbeddingAPIURL),
		c.SessionTimeout, c.CleanupInterval, c.ShortMemoryMaxAge, c.SummaryIntervalMultiplier,
		c.MinMessageCount, c.MaxMessageCount, c.MinTimeSinceLastSummary,
	)
}

// 从环境变量获取字符串值
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

// 从环境变量获取整数值
func getEnvAsInt(key string, defaultValue int) int {
	strValue := getEnv(key, "")
	if value, err := strconv.Atoi(strValue); err == nil {
		return value
	}
	return defaultValue
}

// 从环境变量获取布尔值
func getEnvAsBool(key string, defaultValue bool) bool {
	strValue := getEnv(key, "")
	if value, err := strconv.ParseBool(strValue); err == nil {
		return value
	}
	return defaultValue
}

// 从环境变量获取浮点值
func getEnvAsFloat(key string, defaultValue float64) float64 {
	strValue := getEnv(key, "")
	if value, err := strconv.ParseFloat(strValue, 64); err == nil {
		return value
	}
	return defaultValue
}

// 从环境变量获取时间值
func getEnvAsDuration(key string, defaultValue time.Duration) time.Duration {
	strValue := getEnv(key, "")
	if value, err := time.ParseDuration(strValue); err == nil {
		return value
	}
	return defaultValue
}

// 确保目录存在
func ensureDir(dirPath string) error {
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		return os.MkdirAll(dirPath, 0755)
	}
	return nil
}

// 掩码字符串，用于日志输出安全
func maskString(input string) string {
	if len(input) <= 8 {
		return "***"
	}
	return input[:4] + "..." + input[len(input)-4:]
}

// 获取存储路径的默认值（使用操作系统标准应用数据目录）
func getStoragePathDefault() string {
	// 应用名称，用于创建子目录
	appName := "context-keeper"

	// 尝试获取用户主目录
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Printf("警告: 无法获取用户主目录: %v", err)
		// 回退到相对路径
		return "./data"
	}

	var dataPath string

	// 根据操作系统选择标准应用数据目录
	switch runtime.GOOS {
	case "darwin": // macOS
		// ~/Library/Application Support/context-keeper/
		dataPath = filepath.Join(homeDir, "Library", "Application Support", appName)

	case "windows":
		// 尝试使用APPDATA环境变量
		appData := os.Getenv("APPDATA")
		if appData != "" {
			dataPath = filepath.Join(appData, appName)
		} else {
			// 回退到用户目录下的标准位置
			dataPath = filepath.Join(homeDir, "AppData", "Roaming", appName)
		}

	default: // Linux和其他UNIX系统
		// ~/.local/share/context-keeper/
		dataPath = filepath.Join(homeDir, ".local", "share", appName)

		// 检查XDG_DATA_HOME环境变量
		xdgDataHome := os.Getenv("XDG_DATA_HOME")
		if xdgDataHome != "" {
			dataPath = filepath.Join(xdgDataHome, appName)
		}
	}

	log.Printf("使用系统标准应用数据目录: %s", dataPath)

	// 确保目录存在
	if err := os.MkdirAll(dataPath, 0755); err != nil {
		log.Printf("警告: 创建数据目录失败: %v", err)

		// 如果创建失败，回退到用户主目录下的隐藏目录
		fallbackPath := filepath.Join(homeDir, "."+appName)
		log.Printf("尝试使用回退目录: %s", fallbackPath)

		if err := os.MkdirAll(fallbackPath, 0755); err != nil {
			log.Printf("警告: 创建回退目录也失败: %v", err)
			return "./data" // 最终回退到相对路径
		}
		return fallbackPath
	}

	return dataPath
}
