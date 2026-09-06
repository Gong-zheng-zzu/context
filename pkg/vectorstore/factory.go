package vectorstore

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/contextkeeper/service/internal/models"
	"github.com/contextkeeper/service/pkg/aliyun"
)

// AliyunEmbeddingAdapter 将阿里云VectorService适配为EmbeddingProvider接口
type AliyunEmbeddingAdapter struct {
	service *aliyun.VectorService
}

func (a *AliyunEmbeddingAdapter) GenerateEmbedding(text string) ([]float32, error) {
	return a.service.GenerateEmbedding(text)
}

func (a *AliyunEmbeddingAdapter) GetEmbeddingDimension() int {
	return a.service.GetDimension()
}

// VectorStoreFactory 向量存储工厂
type VectorStoreFactory struct {
	config map[models.VectorStoreType]*models.VectorStoreConfig
	// 预初始化的实例缓存
	instances map[models.VectorStoreType]models.VectorStore
	// 阿里云embedding服务，供其他向量存储复用
	aliyunEmbeddingService *aliyun.VectorService
}

// NewVectorStoreFactory 创建向量存储工厂
func NewVectorStoreFactory() *VectorStoreFactory {
	return &VectorStoreFactory{
		config:    make(map[models.VectorStoreType]*models.VectorStoreConfig),
		instances: make(map[models.VectorStoreType]models.VectorStore),
	}
}

// RegisterConfig 注册厂商配置
func (f *VectorStoreFactory) RegisterConfig(storeType models.VectorStoreType, config *models.VectorStoreConfig) {
	f.config[storeType] = config
	log.Printf("[向量存储工厂] 注册配置: %s", storeType)
}

// InitializeAllInstances 预初始化所有已注册的向量存储实例
func (f *VectorStoreFactory) InitializeAllInstances() error {
	log.Printf("[向量存储工厂] 开始预初始化所有向量存储实例...")

	// 首先初始化阿里云embedding服务（供其他向量存储复用）
	if aliyunConfig, exists := f.config[models.VectorStoreTypeAliyun]; exists {
		embeddingService := aliyun.NewVectorService(
			aliyunConfig.EmbeddingConfig.APIEndpoint,
			aliyunConfig.EmbeddingConfig.APIKey,
			aliyunConfig.DatabaseConfig.Endpoint,
			aliyunConfig.DatabaseConfig.APIKey,
			aliyunConfig.DatabaseConfig.Collection,
			aliyunConfig.EmbeddingConfig.Dimension,
			aliyunConfig.DatabaseConfig.Metric,
			aliyunConfig.SimilarityThreshold,
		)
		f.aliyunEmbeddingService = embeddingService
		log.Printf("[向量存储工厂] 阿里云embedding服务初始化完成，供其他向量存储复用")
	}

	// 初始化所有已注册的向量存储实例
	for storeType := range f.config {
		instance, err := f.createVectorStoreInstance(storeType)
		if err != nil {
			log.Printf("[向量存储工厂] 初始化 %s 失败: %v", storeType, err)
			return fmt.Errorf("初始化 %s 失败: %w", storeType, err)
		}
		f.instances[storeType] = instance
		log.Printf("[向量存储工厂] ✅ %s 实例初始化完成", storeType)
	}

	log.Printf("[向量存储工厂] 🎉 所有向量存储实例预初始化完成！")
	return nil
}

// CreateVectorStore 创建向量存储实例（优先使用预初始化的实例）
func (f *VectorStoreFactory) CreateVectorStore(storeType models.VectorStoreType) (models.VectorStore, error) {
	// 优先返回预初始化的实例
	if instance, exists := f.instances[storeType]; exists {
		log.Printf("[向量存储工厂] 使用预初始化的 %s 实例", storeType)
		return instance, nil
	}

	// 如果没有预初始化实例，动态创建
	log.Printf("[向量存储工厂] 动态创建向量存储: %s", storeType)
	return f.createVectorStoreInstance(storeType)
}

// createVectorStoreInstance 创建向量存储实例的内部方法
func (f *VectorStoreFactory) createVectorStoreInstance(storeType models.VectorStoreType) (models.VectorStore, error) {
	config, exists := f.config[storeType]
	if !exists {
		return nil, fmt.Errorf("未找到向量存储配置: %s", storeType)
	}

	switch storeType {
	case models.VectorStoreTypeAliyun:
		return f.createAliyunVectorStore(config)
	case models.VectorStoreTypeVearch:
		return f.createVearchVectorStore(config)
	case models.VectorStoreTypeTencent:
		return f.createTencentVectorStore(config)
	case models.VectorStoreTypeOpenAI:
		return f.createOpenAIVectorStore(config)
	case models.VectorStoreTypePinecone:
		return f.createPineconeVectorStore(config)
	case models.VectorStoreTypeLocal:
		return f.createLocalVectorStore(config)
	case models.VectorStoreTypeQdrant:
		return f.createQdrantVectorStore(config)
	default:
		return nil, fmt.Errorf("不支持的向量存储类型: %s", storeType)
	}
}

// createAliyunVectorStore 创建阿里云向量存储
func (f *VectorStoreFactory) createAliyunVectorStore(config *models.VectorStoreConfig) (models.VectorStore, error) {
	log.Printf("[向量存储工厂] 创建阿里云向量存储")

	// 创建原有的阿里云向量服务（不修改原有逻辑）
	vectorService := aliyun.NewVectorService(
		config.EmbeddingConfig.APIEndpoint,
		config.EmbeddingConfig.APIKey,
		config.DatabaseConfig.Endpoint,
		config.DatabaseConfig.APIKey,
		config.DatabaseConfig.Collection,
		config.EmbeddingConfig.Dimension,
		config.DatabaseConfig.Metric,
		config.SimilarityThreshold,
	)

	// 包装成新的抽象接口实现
	return NewAliyunVectorStore(vectorService, config), nil
}

// createVearchVectorStore 创建Vearch向量存储
func (f *VectorStoreFactory) createVearchVectorStore(config *models.VectorStoreConfig) (models.VectorStore, error) {
	// 确保阿里云embedding服务已初始化
	if f.aliyunEmbeddingService == nil {
		return nil, fmt.Errorf("阿里云embedding服务未初始化，无法创建Vearch存储")
	}

	vearchConfig := &VearchConfig{
		Endpoints: []string{getExtraParam(config.DatabaseConfig.ExtraParams, "url", config.DatabaseConfig.Endpoint)},
		Username:  getExtraParam(config.DatabaseConfig.ExtraParams, "username", ""),
		Password:  getExtraParam(config.DatabaseConfig.ExtraParams, "password", ""),
		Database:  config.DatabaseConfig.Collection, // 使用Collection作为Database名称

		// Embedding配置（实际使用阿里云服务）
		EmbeddingModel:    config.EmbeddingConfig.Model,
		EmbeddingEndpoint: config.EmbeddingConfig.APIEndpoint,
		EmbeddingAPIKey:   config.EmbeddingConfig.APIKey,
		Dimension:         config.EmbeddingConfig.Dimension,

		// 搜索配置
		DefaultTopK:          getExtraParamInt(config.DatabaseConfig.ExtraParams, "default_top_k", 10),
		SimilarityThreshold:  config.SimilarityThreshold,
		SearchTimeoutSeconds: getExtraParamInt(config.DatabaseConfig.ExtraParams, "search_timeout_seconds", 30),

		// 性能配置
		ConnectionPoolSize:    getExtraParamInt(config.DatabaseConfig.ExtraParams, "connection_pool_size", 10),
		RequestTimeoutSeconds: getExtraParamInt(config.DatabaseConfig.ExtraParams, "request_timeout_seconds", 60),
	}

	// 从环境变量覆盖配置
	if endpoint := os.Getenv("VEARCH_URL"); endpoint != "" {
		vearchConfig.Endpoints = []string{endpoint}
	}
	if username := os.Getenv("VEARCH_USERNAME"); username != "" {
		vearchConfig.Username = username
	}
	if password := os.Getenv("VEARCH_PASSWORD"); password != "" {
		vearchConfig.Password = password
	}
	if database := os.Getenv("VEARCH_DATABASE"); database != "" {
		vearchConfig.Database = database
	}

	client := NewDefaultVearchClient(vearchConfig)

	// 创建获取embedding服务的回调函数，避免直接依赖
	getEmbeddingService := func() EmbeddingProvider {
		if f.aliyunEmbeddingService != nil {
			// 创建适配器将aliyun.VectorService适配为EmbeddingProvider
			return &AliyunEmbeddingAdapter{service: f.aliyunEmbeddingService}
		}
		return nil
	}

	store := NewVearchStore(client, vearchConfig, getEmbeddingService)

	// 初始化存储
	if err := store.Initialize(); err != nil {
		return nil, fmt.Errorf("Vearch存储初始化失败: %v", err)
	}

	log.Printf("[向量工厂] Vearch存储创建成功: endpoints=%v, database=%s",
		vearchConfig.Endpoints, vearchConfig.Database)

	return store, nil
}

// createQdrantVectorStore 创建Qdrant向量存储
func (f *VectorStoreFactory) createQdrantVectorStore(config *models.VectorStoreConfig) (models.VectorStore, error) {
	log.Printf("[向量存储工厂] 创建Qdrant向量存储")

	store, err := NewQdrantVectorStore(config)
	if err != nil {
		return nil, fmt.Errorf("创建Qdrant存储失败: %w", err)
	}

	log.Printf("[向量工厂] Qdrant存储创建成功: endpoint=%s, collection=%s",
		config.DatabaseConfig.Endpoint, config.DefaultCollection)

	return store, nil
}

// 辅助函数：从ExtraParams中获取字符串参数
func getExtraParam(params map[string]interface{}, key, defaultValue string) string {
	if params == nil {
		return defaultValue
	}
	if value, exists := params[key]; exists {
		if str, ok := value.(string); ok {
			return str
		}
	}
	return defaultValue
}

// 辅助函数：从ExtraParams中获取整数参数
func getExtraParamInt(params map[string]interface{}, key string, defaultValue int) int {
	if params == nil {
		return defaultValue
	}
	if value, exists := params[key]; exists {
		if intVal, ok := value.(int); ok {
			return intVal
		}
		if floatVal, ok := value.(float64); ok {
			return int(floatVal)
		}
	}
	return defaultValue
}

// 辅助函数：从ExtraParams中获取浮点数参数
func getExtraParamFloat(params map[string]interface{}, key string, defaultValue float64) float64 {
	if params == nil {
		return defaultValue
	}
	if value, exists := params[key]; exists {
		if floatVal, ok := value.(float64); ok {
			return floatVal
		}
		if intVal, ok := value.(int); ok {
			return float64(intVal)
		}
	}
	return defaultValue
}

// createTencentVectorStore 创建腾讯云向量存储（待实现）
func (f *VectorStoreFactory) createTencentVectorStore(config *models.VectorStoreConfig) (models.VectorStore, error) {
	return nil, fmt.Errorf("腾讯云向量存储尚未实现")
}

// createOpenAIVectorStore 创建OpenAI向量存储（待实现）
func (f *VectorStoreFactory) createOpenAIVectorStore(config *models.VectorStoreConfig) (models.VectorStore, error) {
	return nil, fmt.Errorf("OpenAI向量存储尚未实现")
}

// createPineconeVectorStore 创建Pinecone向量存储（待实现）
func (f *VectorStoreFactory) createPineconeVectorStore(config *models.VectorStoreConfig) (models.VectorStore, error) {
	return nil, fmt.Errorf("Pinecone向量存储尚未实现")
}

// createLocalVectorStore 创建本地向量存储（待实现）
func (f *VectorStoreFactory) createLocalVectorStore(config *models.VectorStoreConfig) (models.VectorStore, error) {
	return nil, fmt.Errorf("本地向量存储尚未实现")
}

// GetVectorStoreTypeFromEnv 从环境变量获取向量存储类型
func GetVectorStoreTypeFromEnv() models.VectorStoreType {
	envType := os.Getenv("VECTOR_STORE_TYPE")
	if envType == "" {
		envType = "aliyun" // 默认使用阿里云
	}

	envType = strings.ToLower(strings.TrimSpace(envType))
	log.Printf("[向量存储工厂] 从环境变量读取存储类型: %s", envType)

	switch envType {
	case "aliyun":
		return models.VectorStoreTypeAliyun
	case "vearch":
		return models.VectorStoreTypeVearch
	case "tencent":
		return models.VectorStoreTypeTencent
	case "openai":
		return models.VectorStoreTypeOpenAI
	case "pinecone":
		return models.VectorStoreTypePinecone
	case "weaviate":
		return models.VectorStoreTypeWeaviate
	case "local":
		return models.VectorStoreTypeLocal
	case "qdrant":
		return models.VectorStoreTypeQdrant
	default:
		log.Printf("[向量存储工厂] 未知存储类型 '%s'，使用默认: aliyun", envType)
		return models.VectorStoreTypeAliyun
	}
}

// LoadConfigFromEnv 从环境变量加载配置
func LoadConfigFromEnv(storeType models.VectorStoreType) (*models.VectorStoreConfig, error) {
	log.Printf("[向量存储工厂] 从环境变量加载配置: %s", storeType)

	switch storeType {
	case models.VectorStoreTypeAliyun:
		return loadAliyunConfigFromEnv()
	case models.VectorStoreTypeVearch:
		return loadVearchConfigFromEnv()
	case models.VectorStoreTypeTencent:
		return loadTencentConfigFromEnv()
	case models.VectorStoreTypeQdrant:
		return loadQdrantConfigFromEnv()
	default:
		return nil, fmt.Errorf("不支持从环境变量加载配置: %s", storeType)
	}
}

// loadAliyunConfigFromEnv 从环境变量加载阿里云配置
// 支持新的ALIYUN_前缀和原有的配置变量，保持向后兼容
func loadAliyunConfigFromEnv() (*models.VectorStoreConfig, error) {
	// 优先使用新的ALIYUN_前缀，如果不存在则使用原有变量名（保持兼容性）
	embeddingAPIURL := getEnvWithFallback("ALIYUN_EMBEDDING_API_URL", "EMBEDDING_API_URL")
	embeddingAPIKey := getEnvWithFallback("ALIYUN_EMBEDDING_API_KEY", "EMBEDDING_API_KEY")
	vectorDBURL := getEnvWithFallback("ALIYUN_VECTOR_DB_URL", "VECTOR_DB_URL")
	vectorDBAPIKey := getEnvWithFallback("ALIYUN_VECTOR_DB_API_KEY", "VECTOR_DB_API_KEY")
	vectorDBCollection := getEnvWithFallback("ALIYUN_VECTOR_DB_COLLECTION", "VECTOR_DB_COLLECTION")

	if embeddingAPIURL == "" || embeddingAPIKey == "" || vectorDBURL == "" || vectorDBAPIKey == "" {
		return nil, fmt.Errorf("阿里云配置不完整，请检查环境变量: EMBEDDING_API_URL, EMBEDDING_API_KEY, VECTOR_DB_URL, VECTOR_DB_API_KEY")
	}

	// 获取可选配置（也支持向后兼容）
	dimensionStr := getEnvWithFallback("ALIYUN_VECTOR_DB_DIMENSION", "VECTOR_DB_DIMENSION")
	dimension := 1536 // 默认维度
	if dimensionStr != "" {
		if dim, err := strconv.Atoi(dimensionStr); err == nil {
			dimension = dim
		}
	}
	metric := getEnvWithFallback("ALIYUN_VECTOR_DB_METRIC", "VECTOR_DB_METRIC")
	if metric == "" {
		metric = "cosine" // 阿里云默认使用cosine
	}

	similarityThreshold := 0.3 // 默认阈值
	if thresholdStr := getEnvWithFallback("ALIYUN_SIMILARITY_THRESHOLD", "SIMILARITY_THRESHOLD"); thresholdStr != "" {
		if threshold, err := strconv.ParseFloat(thresholdStr, 64); err == nil {
			similarityThreshold = threshold
		}
	}

	config := &models.VectorStoreConfig{
		Provider: string(models.VectorStoreTypeAliyun),
		EmbeddingConfig: &models.EmbeddingConfig{
			APIEndpoint: embeddingAPIURL,
			APIKey:      embeddingAPIKey,
			Model:       "text-embedding-v1",
			Dimension:   dimension,
		},
		DatabaseConfig: &models.DatabaseConfig{
			Endpoint:   vectorDBURL,
			APIKey:     vectorDBAPIKey,
			Collection: vectorDBCollection,
			Metric:     metric,
		},
		DefaultCollection:   vectorDBCollection,
		SimilarityThreshold: similarityThreshold,
	}

	log.Printf("[向量存储工厂] 阿里云配置加载完成")
	return config, nil
}

// loadVearchConfigFromEnv 从环境变量加载Vearch配置
func loadVearchConfigFromEnv() (*models.VectorStoreConfig, error) {
	// 基础配置检查
	vearchURL := os.Getenv("VEARCH_URL")
	vearchUsername := os.Getenv("VEARCH_USERNAME")
	vearchPassword := os.Getenv("VEARCH_PASSWORD")
	vearchDatabase := os.Getenv("VEARCH_DATABASE")

	if vearchURL == "" || vearchUsername == "" || vearchPassword == "" || vearchDatabase == "" {
		return nil, fmt.Errorf("Vearch配置不完整，请检查环境变量: VEARCH_URL, VEARCH_USERNAME, VEARCH_PASSWORD, VEARCH_DATABASE")
	}

	// 获取embedding配置（复用阿里云的）
	embeddingAPIURL := os.Getenv("EMBEDDING_API_URL")
	embeddingAPIKey := os.Getenv("EMBEDDING_API_KEY")

	if embeddingAPIURL == "" || embeddingAPIKey == "" {
		return nil, fmt.Errorf("Embedding配置不完整，请检查环境变量: EMBEDDING_API_URL, EMBEDDING_API_KEY")
	}

	// 获取可选配置
	vearchDimension := 1536 // 默认维度
	if envDimension := os.Getenv("VEARCH_DIMENSION"); envDimension != "" {
		if dim, err := strconv.Atoi(envDimension); err == nil {
			vearchDimension = dim
		}
	}

	vearchMetric := os.Getenv("VEARCH_METRIC")
	if vearchMetric == "" {
		vearchMetric = "inner_product" // 默认使用内积
	}

	similarityThreshold := 0.2 // 默认阈值
	if envThreshold := os.Getenv("VEARCH_SIMILARITY_THRESHOLD"); envThreshold != "" {
		if threshold, err := strconv.ParseFloat(envThreshold, 64); err == nil {
			similarityThreshold = threshold
		}
	}

	config := &models.VectorStoreConfig{
		Provider: string(models.VectorStoreTypeVearch),
		EmbeddingConfig: &models.EmbeddingConfig{
			APIEndpoint: embeddingAPIURL,
			APIKey:      embeddingAPIKey,
			Model:       "text-embedding-ada-002", // 默认模型
			Dimension:   vearchDimension,
		},
		DatabaseConfig: &models.DatabaseConfig{
			Endpoint:   vearchURL,
			Collection: vearchDatabase,
			Metric:     vearchMetric,
			ExtraParams: map[string]interface{}{
				"username":                vearchUsername,
				"password":                vearchPassword,
				"url":                     vearchURL,
				"database":                vearchDatabase,
				"connection_pool_size":    getEnvInt("VEARCH_CONNECTION_POOL_SIZE", 10),
				"request_timeout_seconds": getEnvInt("VEARCH_REQUEST_TIMEOUT", 30),
				"default_top_k":           getEnvInt("VEARCH_DEFAULT_TOP_K", 10),
			},
		},
		DefaultCollection:   vearchDatabase,
		SimilarityThreshold: similarityThreshold,
	}

	log.Printf("[向量存储工厂] Vearch配置加载完成: URL=%s, Database=%s", vearchURL, vearchDatabase)
	return config, nil
}

// loadTencentConfigFromEnv 从环境变量加载腾讯云配置（待实现）
func loadTencentConfigFromEnv() (*models.VectorStoreConfig, error) {
	return nil, fmt.Errorf("腾讯云配置加载尚未实现")
}

// getEnvWithFallback 优先获取第一个环境变量，如果不存在则使用第二个（向后兼容）
func getEnvWithFallback(primary, fallback string) string {
	if value := os.Getenv(primary); value != "" {
		return value
	}
	return os.Getenv(fallback)
}

// getEnvInt 从环境变量获取整数值，提供默认值
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

// CreateVectorStoreFromEnv 从环境变量创建向量存储（传统单一实例方式）
func CreateVectorStoreFromEnv() (models.VectorStore, error) {
	log.Printf("[向量存储工厂] 从环境变量创建向量存储")

	// 获取存储类型
	storeType := GetVectorStoreTypeFromEnv()

	// 加载配置
	config, err := LoadConfigFromEnv(storeType)
	if err != nil {
		return nil, fmt.Errorf("加载配置失败: %w", err)
	}

	// 创建工厂并注册配置
	factory := NewVectorStoreFactory()
	factory.RegisterConfig(storeType, config)

	// 创建向量存储
	return factory.CreateVectorStore(storeType)
}

// InitializeFactoryFromEnv 从环境变量初始化工厂并预加载所有支持的向量存储类型
// 这是推荐的启动时初始化方式，能够预初始化所有支持的向量存储类型
func InitializeFactoryFromEnv() (*VectorStoreFactory, error) {
	log.Printf("[向量存储工厂] 🚀 开始从环境变量初始化工厂...")

	factory := NewVectorStoreFactory()

	// 1. 加载阿里云配置（如果环境变量存在）
	if aliyunConfig, err := loadAliyunConfigFromEnv(); err == nil {
		factory.RegisterConfig(models.VectorStoreTypeAliyun, aliyunConfig)
		log.Printf("[向量存储工厂] ✅ 阿里云配置注册成功")
	} else {
		log.Printf("[向量存储工厂] ⚠️ 阿里云配置加载失败: %v", err)
	}

	// 2. 加载Vearch配置（如果环境变量存在）
	if vearchConfig, err := loadVearchConfigFromEnv(); err == nil {
		factory.RegisterConfig(models.VectorStoreTypeVearch, vearchConfig)
		log.Printf("[向量存储工厂] ✅ Vearch配置注册成功")
	} else {
		log.Printf("[向量存储工厂] ⚠️ Vearch配置加载失败: %v", err)
	}

	// 3. 加载Qdrant配置（如果环境变量存在）
	if qdrantConfig, err := loadQdrantConfigFromEnv(); err == nil {
		factory.RegisterConfig(models.VectorStoreTypeQdrant, qdrantConfig)
		log.Printf("[向量存储工厂] ✅ Qdrant配置注册成功")
	} else {
		log.Printf("[向量存储工厂] ⚠️ Qdrant配置加载失败: %v", err)
	}

	// 4. 可以扩展更多类型...
	// if tencentConfig, err := loadTencentConfigFromEnv(); err == nil {
	//     factory.RegisterConfig(models.VectorStoreTypeTencent, tencentConfig)
	// }

	// 4. 预初始化所有已注册的向量存储实例
	if err := factory.InitializeAllInstances(); err != nil {
		log.Printf("[向量存储工厂] ⚠️ 部分实例初始化失败: %v", err)
		// 不返回错误，允许部分初始化成功
	}

	log.Printf("[向量存储工厂] 🎉 工厂初始化完成，已预加载 %d 种向量存储类型", len(factory.instances))
	return factory, nil
}

// GetCurrentVectorStore 根据环境变量配置获取当前应该使用的向量存储实例
func (f *VectorStoreFactory) GetCurrentVectorStore() (models.VectorStore, error) {
	// 从环境变量获取当前配置的存储类型
	currentType := GetVectorStoreTypeFromEnv()
	log.Printf("[向量存储工厂] 当前配置的存储类型: %s", currentType)

	// 返回对应的预初始化实例
	if instance, exists := f.instances[currentType]; exists {
		log.Printf("[向量存储工厂] ✅ 使用预初始化的 %s 实例", currentType)
		return instance, nil
	}

	return nil, fmt.Errorf("当前配置的存储类型 %s 未找到预初始化实例", currentType)
}

// GetVearchClient 获取Vearch客户端
func (f *VectorStoreFactory) GetVearchClient() (VearchClient, error) {
	log.Printf("[向量存储工厂] 获取VearchClient...")

	// 检查是否有预初始化的Vearch实例
	if vearchInstance, exists := f.instances[models.VectorStoreTypeVearch]; exists {
		log.Printf("[向量存储工厂] ✅ 使用预初始化的Vearch实例")

		// 将VectorStore转换为VearchStore以获取客户端
		if vearchStore, ok := vearchInstance.(*VearchStore); ok {
			return vearchStore.GetClient(), nil
		}

		return nil, fmt.Errorf("Vearch实例类型转换失败")
	}

	// 如果没有预初始化实例，动态创建
	log.Printf("[向量存储工厂] 动态创建Vearch客户端...")
	vearchConfig, exists := f.config[models.VectorStoreTypeVearch]
	if !exists {
		return nil, fmt.Errorf("未找到Vearch配置")
	}

	// 创建Vearch配置
	config := &VearchConfig{
		Endpoints:             []string{getExtraParam(vearchConfig.DatabaseConfig.ExtraParams, "url", vearchConfig.DatabaseConfig.Endpoint)},
		Username:              getExtraParam(vearchConfig.DatabaseConfig.ExtraParams, "username", ""),
		Password:              getExtraParam(vearchConfig.DatabaseConfig.ExtraParams, "password", ""),
		Database:              vearchConfig.DatabaseConfig.Collection,
		Dimension:             vearchConfig.EmbeddingConfig.Dimension,
		RequestTimeoutSeconds: getExtraParamInt(vearchConfig.DatabaseConfig.ExtraParams, "request_timeout_seconds", 60),
	}

	// 从环境变量覆盖配置
	if endpoint := os.Getenv("VEARCH_URL"); endpoint != "" {
		config.Endpoints = []string{endpoint}
	}
	if username := os.Getenv("VEARCH_USERNAME"); username != "" {
		config.Username = username
	}
	if password := os.Getenv("VEARCH_PASSWORD"); password != "" {
		config.Password = password
	}
	if database := os.Getenv("VEARCH_DATABASE"); database != "" {
		config.Database = database
	}

	// 创建Vearch客户端
	client := NewDefaultVearchClient(config)

	log.Printf("[向量存储工厂] ✅ VearchClient创建成功")
	return client, nil
}

// CreateAliyunVectorStoreFromLegacyService 从原有的VectorService创建抽象接口实现
// 这是一个过渡方法，用于将原有的VectorService包装成新的抽象接口
func CreateAliyunVectorStoreFromLegacyService(vectorService *aliyun.VectorService) models.VectorStore {
	log.Printf("[向量存储工厂] 从原有VectorService创建抽象接口实现")

	// 构建配置（从原有服务中获取信息）
	config := &models.VectorStoreConfig{
		Provider: string(models.VectorStoreTypeAliyun),
		EmbeddingConfig: &models.EmbeddingConfig{
			Dimension: vectorService.GetDimension(),
		},
		DatabaseConfig: &models.DatabaseConfig{
			Metric: vectorService.GetMetric(),
		},
		SimilarityThreshold: 0.3, // 默认值
	}

	// 创建适配器
	return NewAliyunVectorStore(vectorService, config)
}

// GetAvailableVectorStoreTypes 获取可用的向量存储类型
func GetAvailableVectorStoreTypes() []models.VectorStoreType {
	return models.GetSupportedVectorStoreTypes()
}

// ValidateVectorStoreType 验证向量存储类型
func ValidateVectorStoreType(storeType models.VectorStoreType) bool {
	return storeType.IsValid()
}

// loadQdrantConfigFromEnv 从环境变量加载Qdrant配置
func loadQdrantConfigFromEnv() (*models.VectorStoreConfig, error) {
	// 获取Qdrant配置
	qdrantURL := os.Getenv("QDRANT_URL")
	qdrantAPIKey := os.Getenv("QDRANT_API_KEY")
	// VECTOR_DB_COLLECTION is the authoritative collection for retrieval and
	// unlearning. Keep QDRANT_COLLECTION only as a legacy fallback so all
	// destructive and evaluation paths resolve the same collection.
	qdrantCollection := os.Getenv("VECTOR_DB_COLLECTION")
	if qdrantCollection == "" {
		qdrantCollection = os.Getenv("QDRANT_COLLECTION")
	}

	if qdrantURL == "" {
		return nil, fmt.Errorf("Qdrant配置不完整，请检查环境变量: QDRANT_URL")
	}

	if qdrantCollection == "" {
		qdrantCollection = "context_keeper" // 默认集合名
	}

	// 获取embedding配置
	embeddingAPIURL := os.Getenv("EMBEDDING_API_URL")
	// 🔥 修复：如果EMBEDDING_API_URL未设置，尝试使用OLLAMA_HOST
	if embeddingAPIURL == "" {
		ollamaHost := os.Getenv("OLLAMA_HOST")
		if ollamaHost != "" {
			embeddingAPIURL = ollamaHost + "/api/embeddings"
			log.Printf("[向量存储工厂] 使用OLLAMA_HOST作为embedding端点: %s", embeddingAPIURL)
		}
	}
	embeddingAPIKey := os.Getenv("EMBEDDING_API_KEY")
	embeddingModel := os.Getenv("EMBEDDING_MODEL")

	if embeddingAPIURL == "" {
		return nil, fmt.Errorf("Embedding配置不完整，请检查环境变量: EMBEDDING_API_URL 或 OLLAMA_HOST")
	}

	if embeddingModel == "" {
		embeddingModel = "nomic-embed-text" // 默认模型
	}

	// 获取维度配置
	dimensionStr := os.Getenv("QDRANT_DIMENSION")
	dimension := 768 // 默认维度（nomic-embed-text）
	if dimensionStr != "" {
		if dim, err := strconv.Atoi(dimensionStr); err == nil {
			dimension = dim
		}
	}

	// 获取相似度度量
	metric := os.Getenv("QDRANT_METRIC")
	if metric == "" {
		metric = "cosine" // 默认使用cosine
	}

	// 获取相似度阈值
	similarityThreshold := 0.3 // 默认阈值
	if thresholdStr := os.Getenv("SIMILARITY_THRESHOLD"); thresholdStr != "" {
		if threshold, err := strconv.ParseFloat(thresholdStr, 64); err == nil {
			similarityThreshold = threshold
		}
	}

	config := &models.VectorStoreConfig{
		Provider: string(models.VectorStoreTypeQdrant),
		EmbeddingConfig: &models.EmbeddingConfig{
			APIEndpoint: embeddingAPIURL,
			APIKey:      embeddingAPIKey,
			Model:       embeddingModel,
			Dimension:   dimension,
		},
		DatabaseConfig: &models.DatabaseConfig{
			Endpoint:   qdrantURL,
			APIKey:     qdrantAPIKey,
			Collection: qdrantCollection,
			Metric:     metric,
		},
		DefaultCollection:   qdrantCollection,
		SimilarityThreshold: similarityThreshold,
	}

	log.Printf("[向量存储工厂] Qdrant配置加载完成")
	return config, nil
}
