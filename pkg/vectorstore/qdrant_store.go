package vectorstore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/contextkeeper/service/internal/models"
)

// QdrantVectorStore Qdrant向量存储实现
type QdrantVectorStore struct {
	config          *models.VectorStoreConfig
	httpClient      *http.Client
	baseURL         string
	apiKey          string
	collection      string
	embeddingURL    string
	embeddingModel  string
	embeddingAPIKey string
	securityService interface {
		ScanContent(text string) (bool, []interface{}, string)
		DetectAndRedact(text string) (string, []interface{})
	} // 安全服务（用于脱敏）
}

// NewQdrantVectorStore 创建Qdrant向量存储实例
func NewQdrantVectorStore(config *models.VectorStoreConfig) (*QdrantVectorStore, error) {
	collection := os.Getenv("VECTOR_DB_COLLECTION")
	if collection == "" {
		collection = config.DefaultCollection
	}
	if collection == "" {
		collection = config.DatabaseConfig.Collection
	}
	if collection == "" {
		return nil, fmt.Errorf("Qdrant collection is required")
	}

	log.Printf("[Qdrant向量存储] 初始化Qdrant客户端: %s", config.DatabaseConfig.Endpoint)

	store := &QdrantVectorStore{
		config:          config,
		baseURL:         config.DatabaseConfig.Endpoint,
		apiKey:          config.DatabaseConfig.APIKey,
		collection:      collection,
		embeddingURL:    config.EmbeddingConfig.APIEndpoint,
		embeddingModel:  config.EmbeddingConfig.Model,
		embeddingAPIKey: config.EmbeddingConfig.APIKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	log.Printf("[Qdrant向量存储] Embedding配置: URL=%s, Model=%s", store.embeddingURL, store.embeddingModel)

	// 确保集合存在
	if err := store.EnsureCollection(store.collection); err != nil {
		return nil, fmt.Errorf("初始化集合失败: %w", err)
	}

	log.Printf("[Qdrant向量存储] ✅ 初始化完成")
	return store, nil
}

// EnsureCollection 确保集合存在
func (q *QdrantVectorStore) EnsureCollection(collectionName string) error {
	// 检查集合是否存在
	url := fmt.Sprintf("%s/collections/%s", q.baseURL, collectionName)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	if q.apiKey != "" {
		req.Header.Set("api-key", q.apiKey)
	}

	resp, err := q.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// 如果集合存在，直接返回
	if resp.StatusCode == 200 {
		log.Printf("[Qdrant向量存储] 集合已存在: %s", collectionName)
		return nil
	}

	// 创建集合
	log.Printf("[Qdrant向量存储] 创建集合: %s, 维度: %d", collectionName, q.config.EmbeddingConfig.Dimension)

	createPayload := map[string]interface{}{
		"vectors": map[string]interface{}{
			"size":     q.config.EmbeddingConfig.Dimension,
			"distance": "Cosine",
		},
	}

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(createPayload); err != nil {
		return err
	}

	createReq, err := http.NewRequest("PUT", url, &buf)
	if err != nil {
		return err
	}

	createReq.Header.Set("Content-Type", "application/json; charset=utf-8")
	if q.apiKey != "" {
		createReq.Header.Set("api-key", q.apiKey)
	}

	createResp, err := q.httpClient.Do(createReq)
	if err != nil {
		return err
	}
	defer createResp.Body.Close()

	if createResp.StatusCode != 200 {
		body, _ := io.ReadAll(createResp.Body)
		return fmt.Errorf("创建集合失败: %s", string(body))
	}

	log.Printf("[Qdrant向量存储] ✅ 集合创建成功")
	return nil
}

// GenerateEmbedding 生成文本嵌入向量（使用Ollama）
func (q *QdrantVectorStore) GenerateEmbedding(text string) ([]float32, error) {
	return q.GenerateEmbeddingContext(context.Background(), text)
}

// GenerateEmbeddingContext generates an embedding while honoring caller
// cancellation. Retrieval requests use this variant so an unavailable Ollama
// endpoint cannot hold the whole multi-source query past its deadline.
func (q *QdrantVectorStore) GenerateEmbeddingContext(ctx context.Context, text string) ([]float32, error) {
	log.Printf("[Qdrant向量存储] 生成嵌入向量，文本长度: %d", len(text))

	payload := map[string]interface{}{
		"model":  q.config.EmbeddingConfig.Model,
		"prompt": text,
	}

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(payload); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", q.config.EmbeddingConfig.APIEndpoint, &buf)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := q.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用embedding API失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embedding API返回错误: %s", string(body))
	}

	var result struct {
		Embedding []float32 `json:"embedding"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	log.Printf("[Qdrant向量存储] ✅ 嵌入向量生成成功，维度: %d", len(result.Embedding))
	return result.Embedding, nil
}

// GetEmbeddingDimension 获取向量维度
func (q *QdrantVectorStore) GetEmbeddingDimension() int {
	return q.config.EmbeddingConfig.Dimension
}

// StoreMemory 存储记忆
func (q *QdrantVectorStore) StoreMemory(memory *models.Memory) error {
	if memory == nil {
		return fmt.Errorf("memory is required")
	}
	if strings.TrimSpace(memory.UserID) == "" || strings.TrimSpace(memory.SessionID) == "" {
		return fmt.Errorf("memory user_id and session_id are required")
	}
	log.Printf("[Qdrant向量存储] 存储记忆: ID=%s", memory.ID)

	// 🔒 向量生成前脱敏保护（最后一道防线）
	contentToEmbed := memory.Content
	if q.securityService != nil {
		if isSensitive, _, summary := q.securityService.ScanContent(memory.Content); isSensitive {
			log.Printf("[Qdrant向量存储] ⚠️ 检测到记忆包含敏感信息: %s", summary)
			redacted, _ := q.securityService.DetectAndRedact(memory.Content)
			contentToEmbed = redacted
			memory.Content = redacted // 更新原始内容
			log.Printf("[Qdrant向量存储] ✅ 已脱敏记忆内容")
		}
	}

	// 生成嵌入向量（使用脱敏后的内容）
	vector, err := q.GenerateEmbedding(contentToEmbed)
	if err != nil {
		return fmt.Errorf("生成嵌入向量失败: %w", err)
	}

	// 构建点数据
	payload := make(map[string]interface{}, len(memory.Metadata)+4)
	if memory.Metadata != nil {
		for k, v := range memory.Metadata {
			payload[k] = v
		}
	}
	payload["content"] = memory.Content
	payload["session_id"] = memory.SessionID
	payload["user_id"] = memory.UserID
	payload["timestamp"] = memory.Timestamp

	point := map[string]interface{}{
		"id":      memory.ID,
		"vector":  vector,
		"payload": payload,
	}

	// 上传到Qdrant
	return q.upsertPoints([]interface{}{point})
}

// StoreMessage 存储消息
func (q *QdrantVectorStore) StoreMessage(message *models.Message) error {
	if message == nil {
		return fmt.Errorf("message is required")
	}
	if strings.TrimSpace(message.UserID) == "" || strings.TrimSpace(message.SessionID) == "" {
		return fmt.Errorf("message user_id and session_id are required")
	}
	log.Printf("[Qdrant向量存储] 存储消息: ID=%s", message.ID)

	// 🔒 向量生成前脱敏保护（最后一道防线）
	contentToEmbed := message.Content
	if q.securityService != nil {
		if isSensitive, _, summary := q.securityService.ScanContent(message.Content); isSensitive {
			log.Printf("[Qdrant向量存储] ⚠️ 检测到消息包含敏感信息: %s", summary)
			redacted, _ := q.securityService.DetectAndRedact(message.Content)
			contentToEmbed = redacted
			message.Content = redacted // 更新原始内容
			log.Printf("[Qdrant向量存储] ✅ 已脱敏消息内容")
		}
	}

	// 生成嵌入向量（使用脱敏后的内容）
	vector, err := q.GenerateEmbedding(contentToEmbed)
	if err != nil {
		return fmt.Errorf("生成嵌入向量失败: %w", err)
	}

	payload := make(map[string]interface{}, len(message.Metadata)+5)
	for k, v := range message.Metadata {
		payload[k] = v
	}
	payload["content"] = message.Content
	payload["session_id"] = message.SessionID
	// Ownership fields are assigned after metadata so arbitrary metadata cannot
	// override the actual security boundary.
	payload["user_id"] = message.UserID
	payload["role"] = message.Role
	payload["timestamp"] = message.Timestamp

	// 构建点数据
	point := map[string]interface{}{
		"id":      message.ID,
		"vector":  vector,
		"payload": payload,
	}

	// 上传到Qdrant
	return q.upsertPoints([]interface{}{point})
}

// upsertPoints 上传点到Qdrant
func (q *QdrantVectorStore) upsertPoints(points []interface{}) error {
	url := fmt.Sprintf("%s/collections/%s/points", q.baseURL, q.collection)

	payload := map[string]interface{}{
		"points": points,
	}

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(payload); err != nil {
		return err
	}

	req, err := http.NewRequest("PUT", url, &buf)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	if q.apiKey != "" {
		req.Header.Set("api-key", q.apiKey)
	}

	resp, err := q.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("上传点失败: %s", string(body))
	}

	log.Printf("[Qdrant向量存储] ✅ 成功上传 %d 个点", len(points))
	return nil
}

// SearchByVector 向量搜索
func (q *QdrantVectorStore) SearchByVector(ctx context.Context, vector []float32, options *models.SearchOptions) ([]models.SearchResult, error) {
	return q.SearchByVectorInCollection(ctx, q.collection, vector, options)
}

func (q *QdrantVectorStore) SearchByVectorInCollection(ctx context.Context, collectionName string, vector []float32, options *models.SearchOptions) ([]models.SearchResult, error) {
	if collectionName == "" {
		return nil, fmt.Errorf("Qdrant collection is required")
	}
	if options == nil {
		return nil, fmt.Errorf("search options are required")
	}
	log.Printf("[Qdrant向量存储] 向量搜索，限制: %d", options.Limit)

	url := q.collectionURL(collectionName, "/points/search")

	filter := make(map[string]interface{})
	must := make([]map[string]interface{}, 0, 2)
	if options.SessionID != "" {
		must = append(must, map[string]interface{}{
			"key": "session_id",
			"match": map[string]interface{}{
				"value": options.SessionID,
			},
		})
	}
	if options.UserID != "" {
		must = append(must, map[string]interface{}{
			"key": "user_id",
			"match": map[string]interface{}{
				"value": options.UserID,
			},
		})
	}
	if len(must) > 0 {
		filter["must"] = must
	}
	if excludeUserID, ok := options.ExtraFilters["user_id_ne"].(string); ok && excludeUserID != "" {
		filter["must_not"] = []map[string]interface{}{{
			"key": "user_id",
			"match": map[string]interface{}{
				"value": excludeUserID,
			},
		}}
	}

	payload := map[string]interface{}{
		"vector":       vector,
		"limit":        options.Limit,
		"with_payload": true,
	}

	if len(filter) > 0 {
		payload["filter"] = filter
	}

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(payload); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, &buf)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	if q.apiKey != "" {
		req.Header.Set("api-key", q.apiKey)
	}

	resp, err := q.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("搜索失败: %s", string(body))
	}

	var searchResp struct {
		Result []struct {
			ID      string                 `json:"id"`
			Score   float64                `json:"score"`
			Payload map[string]interface{} `json:"payload"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, err
	}

	// 转换结果
	results := make([]models.SearchResult, 0, len(searchResp.Result))
	for _, item := range searchResp.Result {
		results = append(results, models.SearchResult{
			ID:     item.ID,
			Score:  item.Score,
			Fields: item.Payload,
		})
	}

	log.Printf("[Qdrant向量存储] ✅ 搜索完成，返回 %d 个结果", len(results))
	return results, nil
}

// SearchByText 文本搜索
func (q *QdrantVectorStore) SearchByText(ctx context.Context, query string, options *models.SearchOptions) ([]models.SearchResult, error) {
	return q.SearchByTextInCollection(ctx, q.collection, query, options)
}

func (q *QdrantVectorStore) SearchByTextInCollection(ctx context.Context, collectionName, query string, options *models.SearchOptions) ([]models.SearchResult, error) {
	log.Printf("[Qdrant向量存储] 文本搜索: %s", query)

	// 生成查询向量
	vector, err := q.GenerateEmbeddingContext(ctx, query)
	if err != nil {
		return nil, err
	}

	// 使用向量搜索
	return q.SearchByVectorInCollection(ctx, collectionName, vector, options)
}

// SearchByID 根据ID精确搜索
func (q *QdrantVectorStore) SearchByID(ctx context.Context, id string, options *models.SearchOptions) ([]models.SearchResult, error) {
	// 简化实现：暂不实现
	return nil, fmt.Errorf("not implemented")
}

// SearchByFilter 根据过滤条件搜索
func (q *QdrantVectorStore) SearchByFilter(ctx context.Context, filter string, options *models.SearchOptions) ([]models.SearchResult, error) {
	log.Printf("[Qdrant向量存储] 按过滤条件搜索: %s", filter)

	// 构建滚动查询请求（不需要向量，纯基于过滤条件）
	scrollReq := map[string]interface{}{
		"limit":        options.Limit,
		"with_payload": true,
		"with_vector":  false,
	}

	// 解析过滤条件（简单实现：支持 key="value" 格式）
	if filter != "" {
		// 解析 session_id="xxx" 或 userId="xxx" 格式
		parts := strings.Split(filter, "=")
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.Trim(strings.TrimSpace(parts[1]), `"`)

			scrollReq["filter"] = map[string]interface{}{
				"must": []map[string]interface{}{
					{
						"key": key,
						"match": map[string]interface{}{
							"value": value,
						},
					},
				},
			}
			log.Printf("[Qdrant向量存储] 过滤条件解析: %s = %s", key, value)
		}
	}

	reqBody, err := json.Marshal(scrollReq)
	if err != nil {
		return nil, fmt.Errorf("序列化滚动请求失败: %w", err)
	}

	url := fmt.Sprintf("%s/collections/%s/points/scroll", q.baseURL, q.collection)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("创建HTTP请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if q.apiKey != "" {
		req.Header.Set("api-key", q.apiKey)
	}

	resp, err := q.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("滚动查询失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("滚动查询失败: %s", string(body))
	}

	var scrollResp struct {
		Result struct {
			Points []struct {
				ID      interface{}            `json:"id"`
				Payload map[string]interface{} `json:"payload"`
			} `json:"points"`
		} `json:"result"`
	}

	if err := json.Unmarshal(body, &scrollResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	// 转换为 SearchResult
	results := make([]models.SearchResult, 0, len(scrollResp.Result.Points))
	for _, point := range scrollResp.Result.Points {
		var idStr string
		switch v := point.ID.(type) {
		case string:
			idStr = v
		case float64:
			idStr = fmt.Sprintf("%.0f", v)
		default:
			idStr = fmt.Sprintf("%v", v)
		}

		results = append(results, models.SearchResult{
			ID:     idStr,
			Score:  1.0, // 过滤查询没有相似度分数，设为1.0
			Fields: point.Payload,
		})
	}

	log.Printf("[Qdrant向量存储] ✅ 过滤查询完成，返回 %d 个结果", len(results))
	return results, nil
}

// CountMemories 统计记忆数量
func (q *QdrantVectorStore) CountMemories(sessionID string) (int, error) {
	// 简化实现：返回固定值
	return 0, nil
}

// StoreEnhancedMemory 存储增强记忆
func (q *QdrantVectorStore) StoreEnhancedMemory(memory *models.EnhancedMemory) error {
	return q.StoreMemory(memory.Memory)
}

// StoreEnhancedMessage 存储增强消息
func (q *QdrantVectorStore) StoreEnhancedMessage(message *models.EnhancedMessage) error {
	return q.StoreMessage(message.Message)
}

// GetConfig 获取配置
func (q *QdrantVectorStore) GetConfig() *models.VectorStoreConfig {
	return q.config
}

func (q *QdrantVectorStore) DefaultCollection() string {
	return q.collection
}

// GetProvider 获取提供商类型
func (q *QdrantVectorStore) GetProvider() models.VectorStoreType {
	return models.VectorStoreTypeQdrant
}

// CheckUserExists 检查用户是否存在
func (q *QdrantVectorStore) CheckUserExists(userID string) (bool, error) {
	// 简化实现：假设用户总是存在
	return true, nil
}

// StoreUserInfo 存储用户信息
func (q *QdrantVectorStore) StoreUserInfo(userInfo *models.UserInfo) error {
	// 简化实现：暂不实现
	return nil
}

// GetUserInfo 获取用户信息
func (q *QdrantVectorStore) GetUserInfo(userID string) (*models.UserInfo, error) {
	// 简化实现：返回空
	return nil, fmt.Errorf("user not found")
}

// InitUserStorage 初始化用户存储
func (q *QdrantVectorStore) InitUserStorage() error {
	// 简化实现：暂不实现
	return nil
}

// CollectionExists 检查集合是否存在
func (q *QdrantVectorStore) CollectionExists(name string) (bool, error) {
	url := fmt.Sprintf("%s/collections/%s", q.config.DatabaseConfig.Endpoint, name)

	resp, err := q.httpClient.Get(url)
	if err != nil {
		return false, fmt.Errorf("failed to check collection: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return true, nil
	} else if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}

	return false, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
}

// CreateCollection 创建新集合
func (q *QdrantVectorStore) CreateCollection(name string, config *models.CollectionConfig) error {
	url := fmt.Sprintf("%s/collections/%s", q.config.DatabaseConfig.Endpoint, name)

	// 构建 Qdrant 集合配置
	reqBody := map[string]interface{}{
		"vectors": map[string]interface{}{
			"size":     config.Dimension,
			"distance": config.Metric,
		},
	}

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(reqBody); err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPut, url, &buf)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := q.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create collection: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to create collection, status: %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}

// DeleteCollection 删除集合
func (q *QdrantVectorStore) DeleteCollection(name string) error {
	url := fmt.Sprintf("%s/collections/%s", q.config.DatabaseConfig.Endpoint, name)

	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := q.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete collection: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete collection, status: %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}

// Close 关闭连接
func (q *QdrantVectorStore) Close() error {
	return nil
}

// =============================================================================
// VectorUpdater 接口实现（机器遗忘支持）
// =============================================================================

// UpdateVector 更新单个向量
func (q *QdrantVectorStore) UpdateVector(ctx context.Context, req *models.VectorUpdateRequest) error {
	if req == nil {
		return fmt.Errorf("vector update request is required")
	}
	return q.BatchUpdateVectors(ctx, &models.BatchVectorUpdateRequest{
		CollectionName: req.CollectionName,
		Updates: []models.VectorUpdateItem{{
			ID:       req.ID,
			Vector:   req.NewVector,
			Metadata: req.Metadata,
		}},
	})
}

// BatchUpdateVectors 批量更新向量
func (q *QdrantVectorStore) BatchUpdateVectors(ctx context.Context, req *models.BatchVectorUpdateRequest) error {
	if req == nil {
		return fmt.Errorf("batch vector update request is required")
	}
	if req.CollectionName == "" {
		return fmt.Errorf("Qdrant collection is required")
	}
	if len(req.Updates) == 0 {
		return nil
	}

	points := make([]map[string]interface{}, 0, len(req.Updates))
	for _, update := range req.Updates {
		if err := q.validateVectorUpdate(update); err != nil {
			return err
		}
		points = append(points, map[string]interface{}{
			"id":     update.ID,
			"vector": update.Vector,
		})
	}

	if err := q.doJSON(ctx, http.MethodPut, q.collectionURL(req.CollectionName, "/points/vectors?wait=true"), map[string]interface{}{
		"points": points,
	}); err != nil {
		return fmt.Errorf("update Qdrant vectors: %w", err)
	}

	for _, update := range req.Updates {
		if len(update.Metadata) == 0 {
			continue
		}
		if err := q.doJSON(ctx, http.MethodPost, q.collectionURL(req.CollectionName, "/points/payload?wait=true"), map[string]interface{}{
			"payload": update.Metadata,
			"points":  []string{update.ID},
		}); err != nil {
			return fmt.Errorf("update Qdrant vector metadata for %s: %w", update.ID, err)
		}
	}

	return nil
}

// GetVectorByID 根据ID获取向量
func (q *QdrantVectorStore) GetVectorByID(ctx context.Context, collectionName, vectorID string) (*models.VectorRecord, error) {
	if collectionName == "" || vectorID == "" {
		return nil, fmt.Errorf("Qdrant collection name and vector ID are required")
	}

	endpoint := q.collectionURL(collectionName, "/points/"+url.PathEscape(vectorID)+"?with_payload=true&with_vector=true")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create Qdrant get request: %w", err)
	}
	q.setHeaders(req)
	resp, err := q.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get Qdrant vector %s: %w", vectorID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("Qdrant vector %s not found in collection %s", vectorID, collectionName)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get Qdrant vector %s: status %d: %s", vectorID, resp.StatusCode, string(body))
	}

	var response struct {
		Result qdrantPoint `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("decode Qdrant vector %s: %w", vectorID, err)
	}
	return q.vectorRecord(collectionName, response.Result), nil
}

// GetVectorsByUserID 获取指定用户的所有向量
func (q *QdrantVectorStore) GetVectorsByUserID(ctx context.Context, collectionName, userID string) ([]*models.VectorRecord, error) {
	if collectionName == "" || userID == "" {
		return nil, fmt.Errorf("Qdrant collection name and user ID are required")
	}
	return q.scrollVectors(ctx, collectionName, qdrantOwnershipFilter(userID, "", ""))
}

// GetVectorsByUserAndSessionID reads exactly one owner's session in Qdrant.
func (q *QdrantVectorStore) GetVectorsByUserAndSessionID(ctx context.Context, collectionName, userID, sessionID string) ([]*models.VectorRecord, error) {
	if collectionName == "" || userID == "" || sessionID == "" {
		return nil, fmt.Errorf("Qdrant collection name, user ID, and session ID are required")
	}
	return q.scrollVectors(ctx, collectionName, qdrantOwnershipFilter(userID, sessionID, ""))
}

func (q *QdrantVectorStore) scrollVectors(ctx context.Context, collectionName string, filter map[string]interface{}) ([]*models.VectorRecord, error) {

	points := make([]*models.VectorRecord, 0)
	var offset interface{}
	for {
		payload := map[string]interface{}{
			"limit":        256,
			"with_payload": true,
			"with_vector":  true,
			"filter":       filter,
		}
		if offset != nil {
			payload["offset"] = offset
		}

		var response struct {
			Result struct {
				Points         []qdrantPoint `json:"points"`
				NextPageOffset interface{}   `json:"next_page_offset"`
			} `json:"result"`
		}
		if err := q.doJSONDecode(ctx, http.MethodPost, q.collectionURL(collectionName, "/points/scroll"), payload, &response); err != nil {
			return nil, fmt.Errorf("scroll Qdrant vectors: %w", err)
		}

		for _, point := range response.Result.Points {
			points = append(points, q.vectorRecord(collectionName, point))
		}
		if response.Result.NextPageOffset == nil || len(response.Result.Points) == 0 {
			break
		}
		offset = response.Result.NextPageOffset
	}

	return points, nil
}

// CountVectorsByUserID 统计指定用户的向量数量
func (q *QdrantVectorStore) CountVectorsByUserID(ctx context.Context, collectionName, userID string) (int, error) {
	if collectionName == "" || userID == "" {
		return 0, fmt.Errorf("Qdrant collection name and user ID are required")
	}
	return q.countVectors(ctx, collectionName, qdrantOwnershipFilter(userID, "", ""))
}

// CountVectorsByUserAndSessionID counts exactly one owner-scoped session.
func (q *QdrantVectorStore) CountVectorsByUserAndSessionID(ctx context.Context, collectionName, userID, sessionID string) (int, error) {
	if collectionName == "" || userID == "" || sessionID == "" {
		return 0, fmt.Errorf("Qdrant collection name, user ID, and session ID are required")
	}
	return q.countVectors(ctx, collectionName, qdrantOwnershipFilter(userID, sessionID, ""))
}

// CountVectorsByUserSessionAndDocID counts provenance only when every scope
// field matches. It prevents control-session evidence from satisfying target
// document verification.
func (q *QdrantVectorStore) CountVectorsByUserSessionAndDocID(ctx context.Context, collectionName, userID, sessionID, docID string) (int, error) {
	if collectionName == "" || userID == "" || sessionID == "" || docID == "" {
		return 0, fmt.Errorf("Qdrant collection name, user ID, session ID, and doc ID are required")
	}
	return q.countVectors(ctx, collectionName, qdrantOwnershipFilter(userID, sessionID, docID))
}

func (q *QdrantVectorStore) countVectors(ctx context.Context, collectionName string, filter map[string]interface{}) (int, error) {

	payload := map[string]interface{}{
		"exact":  true,
		"filter": filter,
	}
	var response struct {
		Result struct {
			Count int `json:"count"`
		} `json:"result"`
	}
	if err := q.doJSONDecode(ctx, http.MethodPost, q.collectionURL(collectionName, "/points/count"), payload, &response); err != nil {
		return 0, fmt.Errorf("count Qdrant vectors: %w", err)
	}
	return response.Result.Count, nil
}

// DeleteVectors 删除向量
func (q *QdrantVectorStore) DeleteVectors(ctx context.Context, collectionName string, vectorIDs []string) error {
	if collectionName == "" {
		return fmt.Errorf("Qdrant collection is required")
	}
	if len(vectorIDs) == 0 {
		return nil
	}
	for _, vectorID := range vectorIDs {
		if vectorID == "" {
			return fmt.Errorf("vector ID is required")
		}
	}
	if err := q.doJSON(ctx, http.MethodPost, q.collectionURL(collectionName, "/points/delete?wait=true"), map[string]interface{}{
		"points": vectorIDs,
	}); err != nil {
		return fmt.Errorf("delete Qdrant vectors: %w", err)
	}
	return nil
}

// DeleteVectorsByUserAndSessionID deletes only the exact Qdrant predicate.
// The returned count is measured before deletion; callers must verify the
// subsequent exact count instead of treating an acknowledged write as proof.
func (q *QdrantVectorStore) DeleteVectorsByUserAndSessionID(ctx context.Context, collectionName, userID, sessionID string) (int, error) {
	if collectionName == "" || userID == "" || sessionID == "" {
		return 0, fmt.Errorf("Qdrant collection name, user ID, and session ID are required")
	}
	filter := qdrantOwnershipFilter(userID, sessionID, "")
	count, err := q.countVectors(ctx, collectionName, filter)
	if err != nil {
		return 0, err
	}
	if count == 0 {
		return 0, nil
	}
	if err := q.doJSON(ctx, http.MethodPost, q.collectionURL(collectionName, "/points/delete?wait=true"), map[string]interface{}{"filter": filter}); err != nil {
		return 0, fmt.Errorf("delete Qdrant session vectors: %w", err)
	}
	return count, nil
}

func qdrantOwnershipFilter(userID, sessionID, docID string) map[string]interface{} {
	must := make([]map[string]interface{}, 0, 3)
	for _, field := range []struct {
		key   string
		value string
	}{
		{key: "user_id", value: userID},
		{key: "session_id", value: sessionID},
		{key: "doc_id", value: docID},
	} {
		if field.value == "" {
			continue
		}
		must = append(must, map[string]interface{}{
			"key":   field.key,
			"match": map[string]interface{}{"value": field.value},
		})
	}
	return map[string]interface{}{"must": must}
}

type qdrantPoint struct {
	ID      interface{}            `json:"id"`
	Vector  []float32              `json:"vector"`
	Payload map[string]interface{} `json:"payload"`
}

func (q *QdrantVectorStore) validateVectorUpdate(update models.VectorUpdateItem) error {
	if update.ID == "" {
		return fmt.Errorf("vector ID is required")
	}
	if len(update.Vector) == 0 {
		return fmt.Errorf("vector %s is empty", update.ID)
	}
	if q.config.EmbeddingConfig != nil && q.config.EmbeddingConfig.Dimension > 0 && len(update.Vector) != q.config.EmbeddingConfig.Dimension {
		return fmt.Errorf("vector %s has dimension %d, expected %d", update.ID, len(update.Vector), q.config.EmbeddingConfig.Dimension)
	}
	if _, ok := update.Metadata["user_id"]; ok {
		return fmt.Errorf("vector %s metadata must not modify user_id", update.ID)
	}
	if _, ok := update.Metadata["session_id"]; ok {
		return fmt.Errorf("vector %s metadata must not modify session_id", update.ID)
	}
	return nil
}

func (q *QdrantVectorStore) vectorRecord(collectionName string, point qdrantPoint) *models.VectorRecord {
	metadata := point.Payload
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	record := &models.VectorRecord{
		ID:             fmt.Sprint(point.ID),
		CollectionName: collectionName,
		Vector:         point.Vector,
		Metadata:       metadata,
	}
	if userID, ok := metadata["user_id"].(string); ok {
		record.UserID = userID
	}
	if timestamp, ok := metadata["timestamp"].(float64); ok {
		record.CreatedAt = time.Unix(int64(timestamp), 0)
	}
	return record
}

func (q *QdrantVectorStore) collectionURL(collectionName, suffix string) string {
	return fmt.Sprintf("%s/collections/%s%s", strings.TrimRight(q.baseURL, "/"), url.PathEscape(collectionName), suffix)
}

func (q *QdrantVectorStore) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	if q.apiKey != "" {
		req.Header.Set("api-key", q.apiKey)
	}
}

func (q *QdrantVectorStore) doJSON(ctx context.Context, method, endpoint string, payload interface{}) error {
	return q.doJSONDecode(ctx, method, endpoint, payload, nil)
}

func (q *QdrantVectorStore) doJSONDecode(ctx context.Context, method, endpoint string, payload, output interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode Qdrant request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create Qdrant request: %w", err)
	}
	q.setHeaders(req)
	resp, err := q.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		responseBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Qdrant returned status %d: %s", resp.StatusCode, string(responseBody))
	}
	if output == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(output); err != nil {
		return fmt.Errorf("decode Qdrant response: %w", err)
	}
	return nil
}
