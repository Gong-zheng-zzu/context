package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/contextkeeper/service/internal/models"
	"github.com/contextkeeper/service/pkg/aliyun"
)

// EntityVectorService 实体向量存储服务
// 实现知识图谱+向量混合检索的实体向量存储功能
type EntityVectorService struct {
	vectorService *aliyun.VectorService // 复用现有向量服务
}

// EntityVectorRecord 实体向量记录
type EntityVectorRecord struct {
	ID       string         `json:"id"`
	Content  string         `json:"content"` // 实体名作为content
	Vector   []float32      `json:"vector"`
	Metadata EntityMetadata `json:"metadata"`
}

// EntityMetadata 实体元数据
type EntityMetadata struct {
	Type        string   `json:"type"`           // 固定为 "entity"
	EntityType  string   `json:"entity_type"`    // Technical/Issue/Solution/Config/Concept
	UserID      string   `json:"user_id"`        // 用户ID
	MemoryIDs   []string `json:"memory_ids"`     // 关联的memory列表
	Neo4jNodeID string   `json:"neo4j_node_id"`  // 图谱节点ID
	Description string   `json:"description"`    // 实体描述
	CreatedAt   int64    `json:"created_at"`     // 创建时间
	UpdatedAt   int64    `json:"updated_at"`     // 更新时间
}

// ExtractedEntity LLM抽取的实体信息
type ExtractedEntity struct {
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	Description string  `json:"description"`
	Confidence  float64 `json:"confidence"`
	Neo4jNodeID string  `json:"neo4j_node_id,omitempty"`
}

// NewEntityVectorService 创建新的实体向量服务
func NewEntityVectorService(vectorService *aliyun.VectorService) *EntityVectorService {
	if vectorService == nil {
		// Qdrant deployments use the VectorStore abstraction, while this legacy
		// compatibility service requires an aliyun.VectorService instance.
		log.Printf("⚠️ [实体向量服务] 未配置兼容VectorService，禁用实体向量兼容功能")
		return nil
	}

	log.Printf("✅ [实体向量服务] 初始化完成")
	return &EntityVectorService{
		vectorService: vectorService,
	}
}

// StoreEntityVectors 批量存储实体向量
func (s *EntityVectorService) StoreEntityVectors(
	ctx context.Context,
	entities []ExtractedEntity,
	memoryID string,
	userID string,
) error {
	if len(entities) == 0 {
		log.Printf("⚠️ [实体向量] 无实体需要存储")
		return nil
	}

	log.Printf("🔄 [实体向量] 开始存储 %d 个实体向量, memoryID=%s, userID=%s",
		len(entities), memoryID, userID)

	successCount := 0
	existCount := 0
	failCount := 0

	for _, entity := range entities {
		if entity.Name == "" {
			log.Printf("⚠️ [实体向量] 跳过空实体名")
			continue
		}

		// 1. 检查实体是否已存在
		existingID, exists, existingMemoryIDs := s.findExistingEntity(ctx, entity.Name, userID)

		if exists {
			// 2a. 已存在：追加memory_id
			err := s.appendMemoryID(ctx, existingID, memoryID, existingMemoryIDs, entity, userID)
			if err != nil {
				log.Printf("⚠️ [实体向量] 追加memory_id失败: %v", err)
				failCount++
				continue
			}
			log.Printf("✅ [实体向量] 实体已存在，追加关联: %s -> %s", entity.Name, memoryID)
			existCount++
		} else {
			// 2b. 不存在：新建实体向量记录
			err := s.createEntityVector(ctx, entity, memoryID, userID)
			if err != nil {
				log.Printf("⚠️ [实体向量] 创建实体向量失败: %v", err)
				failCount++
				continue
			}
			log.Printf("✅ [实体向量] 创建新实体向量: %s", entity.Name)
			successCount++
		}
	}

	log.Printf("📊 [实体向量] 存储完成 - 新建: %d, 更新: %d, 失败: %d",
		successCount, existCount, failCount)

	return nil
}

// findExistingEntity 查找已存在的实体
// 返回: entityID, exists, existingMemoryIDs
func (s *EntityVectorService) findExistingEntity(
	ctx context.Context,
	entityName string,
	userID string,
) (string, bool, []string) {
	log.Printf("🔍 [实体向量] 检查实体是否存在: %s (user=%s)", entityName, userID)

	// 生成实体名的向量
	entityVector, err := s.vectorService.GenerateEmbedding(entityName)
	if err != nil {
		log.Printf("⚠️ [实体向量] 生成查询向量失败: %v", err)
		return "", false, nil
	}

	// 构建过滤条件：仅搜索entity类型且属于该用户的记录
	filter := fmt.Sprintf(`type = "entity" AND userId = "%s"`, userID)

	// 高相似度搜索（阈值0.1以下认为是同一实体，因为使用余弦距离）
	results, err := s.searchEntityWithFilter(ctx, entityVector, filter, 5)
	if err != nil {
		log.Printf("⚠️ [实体向量] 搜索已存在实体失败: %v", err)
		return "", false, nil
	}

	// 检查是否有精确匹配的实体
	for _, result := range results {
		content, _ := result.Fields["content"].(string)
		// 精确匹配实体名（忽略大小写）
		if strings.EqualFold(content, entityName) && result.Score <= 0.15 {
			log.Printf("✅ [实体向量] 找到已存在实体: %s (score=%.4f)", entityName, result.Score)

			// 获取现有的memory_ids
			var existingMemoryIDs []string
			if memoryIDsStr, ok := result.Fields["memory_ids"].(string); ok && memoryIDsStr != "" {
				json.Unmarshal([]byte(memoryIDsStr), &existingMemoryIDs)
			}

			return result.ID, true, existingMemoryIDs
		}
	}

	log.Printf("📭 [实体向量] 实体不存在: %s", entityName)
	return "", false, nil
}

// searchEntityWithFilter 带过滤条件的实体向量搜索
func (s *EntityVectorService) searchEntityWithFilter(
	ctx context.Context,
	vector []float32,
	filter string,
	topK int,
) ([]models.SearchResult, error) {
	// 构建请求体
	searchReq := map[string]interface{}{
		"vector":         vector,
		"topk":           topK,
		"filter":         filter,
		"include_vector": false,
	}

	reqBody, err := json.Marshal(searchReq)
	if err != nil {
		return nil, fmt.Errorf("序列化搜索请求失败: %w", err)
	}

	// 创建HTTP请求
	url := fmt.Sprintf("%s/v1/collections/%s/query",
		s.vectorService.VectorDBURL, s.vectorService.VectorDBCollection)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("创建HTTP请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("dashvector-auth-token", s.vectorService.VectorDBAPIKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API返回错误状态码: %d, 响应: %s", resp.StatusCode, string(respBody))
	}

	// 解析响应
	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Output  []struct {
			Id     string                 `json:"id"`
			Score  float64                `json:"score"`
			Fields map[string]interface{} `json:"fields"`
		} `json:"output"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %d, %s", result.Code, result.Message)
	}

	// 转换结果
	var searchResults []models.SearchResult
	for _, item := range result.Output {
		searchResults = append(searchResults, models.SearchResult{
			ID:     item.Id,
			Score:  item.Score,
			Fields: item.Fields,
		})
	}

	return searchResults, nil
}

// createEntityVector 创建新的实体向量记录
func (s *EntityVectorService) createEntityVector(
	ctx context.Context,
	entity ExtractedEntity,
	memoryID string,
	userID string,
) error {
	// 1. 生成实体名的向量
	entityVector, err := s.vectorService.GenerateEmbedding(entity.Name)
	if err != nil {
		return fmt.Errorf("生成实体向量失败: %w", err)
	}

	// 2. 生成实体ID
	entityID := s.generateEntityID(entity.Name, userID)
	now := time.Now().Unix()

	// 3. 构建memory_ids的JSON字符串
	memoryIDsJSON, _ := json.Marshal([]string{memoryID})

	// 4. 构建文档
	doc := map[string]interface{}{
		"id":     entityID,
		"vector": entityVector,
		"fields": map[string]interface{}{
			"content":       entity.Name,
			"type":          "entity",
			"entity_type":   entity.Type,
			"userId":        userID,
			"memory_ids":    string(memoryIDsJSON),
			"neo4j_node_id": entity.Neo4jNodeID,
			"description":   entity.Description,
			"created_at":    now,
			"updated_at":    now,
		},
	}

	// 5. 发送存储请求
	return s.upsertDocument(ctx, doc)
}

// appendMemoryID 追加memory关联（更新已存在的实体）
func (s *EntityVectorService) appendMemoryID(
	ctx context.Context,
	entityID string,
	newMemoryID string,
	existingMemoryIDs []string,
	entity ExtractedEntity,
	userID string,
) error {
	// 检查是否已经包含该memory_id
	for _, mid := range existingMemoryIDs {
		if mid == newMemoryID {
			log.Printf("ℹ️ [实体向量] memory_id已存在，跳过: %s", newMemoryID)
			return nil
		}
	}

	// 追加新的memory_id
	updatedMemoryIDs := append(existingMemoryIDs, newMemoryID)
	memoryIDsJSON, _ := json.Marshal(updatedMemoryIDs)

	now := time.Now().Unix()

	// 重新生成向量（保持一致性）
	entityVector, err := s.vectorService.GenerateEmbedding(entity.Name)
	if err != nil {
		return fmt.Errorf("生成实体向量失败: %w", err)
	}

	// 构建更新文档（使用相同ID进行upsert）
	doc := map[string]interface{}{
		"id":     entityID,
		"vector": entityVector,
		"fields": map[string]interface{}{
			"content":       entity.Name,
			"type":          "entity",
			"entity_type":   entity.Type,
			"userId":        userID,
			"memory_ids":    string(memoryIDsJSON),
			"neo4j_node_id": entity.Neo4jNodeID,
			"description":   entity.Description,
			"updated_at":    now,
		},
	}

	return s.upsertDocument(ctx, doc)
}

// upsertDocument 插入或更新文档
func (s *EntityVectorService) upsertDocument(ctx context.Context, doc map[string]interface{}) error {
	// 构建插入请求
	insertReq := map[string]interface{}{
		"docs": []map[string]interface{}{doc},
	}

	reqBody, err := json.Marshal(insertReq)
	if err != nil {
		return fmt.Errorf("序列化插入请求失败: %w", err)
	}

	// 创建HTTP请求
	url := fmt.Sprintf("%s/v1/collections/%s/docs",
		s.vectorService.VectorDBURL, s.vectorService.VectorDBCollection)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return fmt.Errorf("创建HTTP请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("dashvector-auth-token", s.vectorService.VectorDBAPIKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("API请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API返回错误状态码: %d, 响应: %s", resp.StatusCode, string(respBody))
	}

	// 解析响应
	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}

	if result.Code != 0 {
		return fmt.Errorf("API返回错误: %d, %s", result.Code, result.Message)
	}

	return nil
}

// generateEntityID 生成实体ID（基于实体名+用户ID的哈希）
func (s *EntityVectorService) generateEntityID(entityName, userID string) string {
	hash := sha256.Sum256([]byte(entityName + userID))
	return "entity-" + hex.EncodeToString(hash[:8])
}

// SearchEntityVectors 实体向量检索（用于检索链路）
func (s *EntityVectorService) SearchEntityVectors(
	ctx context.Context,
	query string,
	userID string,
	limit int,
) ([]EntityVectorSearchResult, error) {
	log.Printf("🔍 [实体向量检索] 开始检索: %s (user=%s)", query, userID)

	if limit <= 0 {
		limit = 10
	}

	// 1. 计算查询向量
	queryVector, err := s.vectorService.GenerateEmbedding(query)
	if err != nil {
		return nil, fmt.Errorf("生成查询向量失败: %w", err)
	}

	// 2. 仅检索entity类型的记录
	filter := fmt.Sprintf(`type = "entity" AND userId = "%s"`, userID)

	// 3. 执行向量检索
	results, err := s.searchEntityWithFilter(ctx, queryVector, filter, limit)
	if err != nil {
		return nil, fmt.Errorf("实体向量检索失败: %w", err)
	}

	// 4. 转换结果并应用相似度阈值
	entityResults := make([]EntityVectorSearchResult, 0, len(results))
	for _, r := range results {
		// 设置相似度阈值（余弦距离0.5以下才认为相关）
		if r.Score > 0.5 {
			continue
		}

		// 解析memory_ids
		var memoryIDs []string
		if memoryIDsStr, ok := r.Fields["memory_ids"].(string); ok && memoryIDsStr != "" {
			json.Unmarshal([]byte(memoryIDsStr), &memoryIDs)
		}

		entityType, _ := r.Fields["entity_type"].(string)
		neo4jNodeID, _ := r.Fields["neo4j_node_id"].(string)
		content, _ := r.Fields["content"].(string)

		entityResults = append(entityResults, EntityVectorSearchResult{
			EntityName:  content,
			EntityType:  entityType,
			Similarity:  float32(1 - r.Score), // 转换为相似度（余弦距离 -> 相似度）
			MemoryIDs:   memoryIDs,
			Neo4jNodeID: neo4jNodeID,
		})
	}

	log.Printf("✅ [实体向量检索] 找到 %d 个相关实体", len(entityResults))
	return entityResults, nil
}

// EntityVectorSearchResult 实体向量检索结果
type EntityVectorSearchResult struct {
	EntityName  string   `json:"entity_name"`
	EntityType  string   `json:"entity_type"`
	Similarity  float32  `json:"similarity"`
	MemoryIDs   []string `json:"memory_ids"`
	Neo4jNodeID string   `json:"neo4j_node_id"`
}
