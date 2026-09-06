package contract

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/contextkeeper/service/internal/models"
	"github.com/contextkeeper/service/pkg/vectorstore"
	"github.com/google/uuid"
)

// TestVectorSearchContract 最小契约测试：写入一条固定记忆，查询能返回对应 doc_id 和分数
func TestVectorSearchContract(t *testing.T) {
	log.Println("🧪 [契约测试] 向量检索最小契约测试")

	// 1. 初始化向量存储（使用实际配置）
	config := &models.VectorStoreConfig{
		Provider: os.Getenv("VECTOR_STORE_PROVIDER"), // 从环境变量读取
		EmbeddingConfig: &models.EmbeddingConfig{
			APIEndpoint: os.Getenv("EMBEDDING_API_ENDPOINT"),
			APIKey:      os.Getenv("EMBEDDING_API_KEY"),
			Model:       os.Getenv("EMBEDDING_MODEL"),
			Dimension:   768, // nomic-embed-text 维度
		},
		DatabaseConfig: &models.DatabaseConfig{
			Endpoint:   os.Getenv("VECTOR_DB_ENDPOINT"),
			APIKey:     os.Getenv("VECTOR_DB_API_KEY"),
			Collection: "contract_test",
		},
		DefaultCollection:   "contract_test",
		SimilarityThreshold: 0.5,
	}

	// 设置默认值
	if config.Provider == "" {
		config.Provider = "qdrant"
	}
	if config.EmbeddingConfig.APIEndpoint == "" {
		config.EmbeddingConfig.APIEndpoint = "http://localhost:11434/api/embeddings"
	}
	if config.EmbeddingConfig.Model == "" {
		config.EmbeddingConfig.Model = "nomic-embed-text"
	}
	if config.DatabaseConfig.Endpoint == "" {
		config.DatabaseConfig.Endpoint = "http://localhost:6333"
	}

	// 使用工厂创建向量存储
	factory := vectorstore.NewVectorStoreFactory()
	factory.RegisterConfig(models.VectorStoreType(config.Provider), config)

	store, err := factory.CreateVectorStore(models.VectorStoreType(config.Provider))
	if err != nil {
		t.Fatalf("❌ 初始化向量存储失败: %v", err)
	}
	log.Println("✅ 向量存储初始化成功")

	// 2. 确保集合存在
	err = store.EnsureCollection(config.DefaultCollection)
	if err != nil {
		t.Fatalf("❌ 确保集合存在失败: %v", err)
	}
	log.Println("✅ 集合准备就绪")

	// 3. 写入固定测试记忆（使用 UUID 作为 ID）
	testID := uuid.New().String()
	testMemory := &models.Memory{
		ID:        testID,
		UserID:    "contract_test_user",
		SessionID: "contract_test_session",
		Content:   "这是一条用于契约测试的固定记忆：患者张三，82岁，阿尔茨海默症，今日血压140/90",
		Timestamp: time.Now().Unix(),
		Metadata: map[string]interface{}{
			"test_marker": "contract_test",
			"patient_id":  "zhang_san",
			"age":         82,
		},
	}

	err = store.StoreMemory(testMemory)
	if err != nil {
		t.Fatalf("❌ 写入测试记忆失败: %v", err)
	}
	log.Printf("✅ 写入测试记忆成功: ID=%s", testMemory.ID)

	// 等待索引生效
	time.Sleep(2 * time.Second)

	// 4. 执行查询，验证能返回对应 doc_id 和分数
	ctx := context.Background()
	searchOptions := &models.SearchOptions{
		Limit:  5,
		UserID: "contract_test_user",
	}

	results, err := store.SearchByText(ctx, "阿尔茨海默症患者血压", searchOptions)
	if err != nil {
		t.Fatalf("❌ 查询失败: %v", err)
	}

	log.Printf("✅ 查询返回 %d 条结果", len(results))

	// 5. 验证契约
	if len(results) == 0 {
		t.Fatal("❌ 契约失败：查询返回空结果")
	}

	found := false
	for i, result := range results {
		// 从 Fields 中提取 content
		content := ""
		if result.Fields != nil {
			if c, ok := result.Fields["content"].(string); ok {
				content = c
			}
		}

		log.Printf("  [%d] ID=%s, Score=%.4f, Content=%s",
			i, result.ID, result.Score, truncate(content, 50))

		if result.ID == testMemory.ID {
			found = true
			if result.Score <= 0 {
				t.Errorf("❌ 契约失败：找到目标文档但分数无效 (score=%.4f)", result.Score)
			}
			log.Printf("✅ 契约通过：找到目标文档 ID=%s, Score=%.4f", result.ID, result.Score)
		}
	}

	if !found {
		t.Error("❌ 契约失败：返回结果中未找到目标文档 ID")
		t.Logf("   期望找到: %s", testMemory.ID)
	}

	// 6. 清理测试数据
	err = store.DeleteVectors(ctx, config.DefaultCollection, []string{testMemory.ID})
	if err != nil {
		log.Printf("⚠️  清理测试数据失败: %v", err)
	} else {
		log.Println("✅ 清理测试数据成功")
	}
}

// truncate 截断字符串用于日志输出
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
