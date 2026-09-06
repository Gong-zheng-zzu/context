package vectorstore

import (
	"fmt"
	"log"
	"time"

	"github.com/contextkeeper/service/internal/models"
)

// VearchUserRepository Vearch向量存储的用户信息存储实现
type VearchUserRepository struct {
	client    VearchClient
	spaceName string
}

// NewVearchUserRepository 创建Vearch向量存储用户仓库实例
func NewVearchUserRepository(client VearchClient) models.UserRepository {
	return &VearchUserRepository{
		client:    client,
		spaceName: "context_keeper_users", // 用户信息专用空间
	}
}

// CreateUser 创建新用户
func (repo *VearchUserRepository) CreateUser(userInfo *models.UserInfo) error {
	log.Printf("🔥 [用户仓库-Vearch] ===== 开始创建用户: %s =====", userInfo.UserID)

	// 1. 检查用户是否已存在
	exists, err := repo.CheckUserExists(userInfo.UserID)
	if err != nil {
		log.Printf("❌ [用户仓库-Vearch] 检查用户存在性失败: %v", err)
		return fmt.Errorf("检查用户存在性失败: %w", err)
	}
	if exists {
		log.Printf("❌ [用户仓库-Vearch] 用户已存在: %s", userInfo.UserID)
		return fmt.Errorf("用户ID已存在: %s", userInfo.UserID)
	}

	// 2. 设置时间戳
	now := time.Now().Format(time.RFC3339)
	if userInfo.CreatedAt == "" {
		userInfo.CreatedAt = now
	}
	userInfo.UpdatedAt = now

	// 3. 构建用户文档
	doc := map[string]interface{}{
		"_id":         userInfo.UserID,
		"user_id":     userInfo.UserID,
		"first_used":  userInfo.FirstUsed,
		"last_active": userInfo.LastActive,
		"device_info": userInfo.DeviceInfo,
		"created_at":  userInfo.CreatedAt,
		"updated_at":  userInfo.UpdatedAt,
		"metadata":    userInfo.Metadata,
	}

	// 4. 插入到用户空间
	err = repo.client.Insert("db", repo.spaceName, []map[string]interface{}{doc})
	if err != nil {
		log.Printf("❌ [用户仓库-Vearch] 存储用户信息失败: %v", err)
		return fmt.Errorf("存储用户信息失败: %w", err)
	}

	log.Printf("✅ [用户仓库-Vearch] 用户创建成功: %s", userInfo.UserID)
	return nil
}

// UpdateUser 更新用户信息
func (repo *VearchUserRepository) UpdateUser(userInfo *models.UserInfo) error {
	log.Printf("🔥 [用户仓库-Vearch] ===== 开始更新用户: %s =====", userInfo.UserID)

	// 1. 检查用户是否存在
	exists, err := repo.CheckUserExists(userInfo.UserID)
	if err != nil {
		log.Printf("❌ [用户仓库-Vearch] 检查用户存在性失败: %v", err)
		return fmt.Errorf("检查用户存在性失败: %w", err)
	}
	if !exists {
		log.Printf("❌ [用户仓库-Vearch] 用户不存在: %s", userInfo.UserID)
		return fmt.Errorf("用户不存在: %s", userInfo.UserID)
	}

	// 2. 获取现有用户信息（保留创建时间）
	existingUser, err := repo.GetUser(userInfo.UserID)
	if err != nil {
		log.Printf("❌ [用户仓库-Vearch] 获取现有用户信息失败: %v", err)
		return fmt.Errorf("获取现有用户信息失败: %w", err)
	}
	if existingUser != nil {
		userInfo.CreatedAt = existingUser.CreatedAt // 保留原创建时间
	}

	// 3. 设置更新时间
	userInfo.UpdatedAt = time.Now().Format(time.RFC3339)

	// 4. 构建用户文档
	doc := map[string]interface{}{
		"_id":         userInfo.UserID,
		"user_id":     userInfo.UserID,
		"first_used":  userInfo.FirstUsed,
		"last_active": userInfo.LastActive,
		"device_info": userInfo.DeviceInfo,
		"created_at":  userInfo.CreatedAt,
		"updated_at":  userInfo.UpdatedAt,
		"metadata":    userInfo.Metadata,
	}

	// 5. 插入到用户空间（实际是重新存储）
	err = repo.client.Insert("db", repo.spaceName, []map[string]interface{}{doc})
	if err != nil {
		log.Printf("❌ [用户仓库-Vearch] 更新用户信息失败: %v", err)
		return fmt.Errorf("更新用户信息失败: %w", err)
	}

	log.Printf("✅ [用户仓库-Vearch] 用户更新成功: %s", userInfo.UserID)
	return nil
}

// GetUser 根据用户ID获取用户信息
func (repo *VearchUserRepository) GetUser(userID string) (*models.UserInfo, error) {
	log.Printf("🔍 [用户仓库-Vearch] 开始查询用户: %s", userID)

	// 构建搜索请求（按用户ID过滤）
	searchRequest := &VearchSearchRequest{
		Vectors: []VearchVector{
			{
				Field:   "vector",
				Feature: make([]float32, 512), // 零向量用于ID搜索
			},
		},
		Filters: &VearchFilter{
			Operator: "AND",
			Conditions: []VearchCondition{
				{
					Field:    "user_id",
					Operator: "IN",
					Value:    []interface{}{userID},
				},
			},
		},
		Limit:     1,
		Fields:    []string{"user_id", "first_used", "last_active", "device_info", "created_at", "updated_at", "metadata"},
		DbName:    "db",
		SpaceName: repo.spaceName,
	}

	// 执行搜索
	response, err := repo.client.Search("db", repo.spaceName, searchRequest)
	if err != nil {
		log.Printf("❌ [用户仓库-Vearch] 查询用户失败: %v", err)
		return nil, fmt.Errorf("查询用户失败: %w", err)
	}

	// 检查搜索结果
	if len(response.Data.Documents) == 0 {
		log.Printf("⚠️ [用户仓库-Vearch] 用户不存在: %s", userID)
		return nil, nil
	}

	// 解析用户信息
	doc := response.Data.Documents[0][0] // 取第一个文档的第一个结果
	userInfo := &models.UserInfo{
		UserID:     getString(doc, "user_id"),
		FirstUsed:  getString(doc, "first_used"),
		LastActive: getString(doc, "last_active"),
		CreatedAt:  getString(doc, "created_at"),
		UpdatedAt:  getString(doc, "updated_at"),
	}

	// 处理复杂字段
	if deviceInfo, ok := doc["device_info"].(map[string]interface{}); ok {
		userInfo.DeviceInfo = deviceInfo
	}
	if metadata, ok := doc["metadata"].(map[string]interface{}); ok {
		userInfo.Metadata = metadata
	}

	log.Printf("✅ [用户仓库-Vearch] 用户查询成功: %s", userID)
	return userInfo, nil
}

// CheckUserExists 检查用户是否存在
func (repo *VearchUserRepository) CheckUserExists(userID string) (bool, error) {
	log.Printf("🔍 [用户仓库-Vearch] 检查用户存在性: %s", userID)

	// 直接使用GetUser方法来检查用户是否存在
	userInfo, err := repo.GetUser(userID)
	if err != nil {
		log.Printf("❌ [用户仓库-Vearch] 检查用户存在性失败: %v", err)
		return false, fmt.Errorf("检查用户存在性失败: %w", err)
	}

	exists := userInfo != nil
	log.Printf("📊 [用户仓库-Vearch] 用户存在性检查结果: %s -> 存在: %t", userID, exists)
	return exists, nil
}

// InitRepository 初始化存储库
func (repo *VearchUserRepository) InitRepository() error {
	log.Printf("🔧 [用户仓库-Vearch] 开始初始化用户存储库")

	// 检查用户空间是否存在
	exists, err := repo.client.SpaceExists("db", repo.spaceName)
	if err != nil {
		log.Printf("❌ [用户仓库-Vearch] 检查用户空间存在性失败: %v", err)
		return fmt.Errorf("检查用户空间存在性失败: %w", err)
	}

	if !exists {
		log.Printf("⚠️ [用户仓库-Vearch] 用户空间不存在: %s", repo.spaceName)
		return fmt.Errorf("用户空间 '%s' 不存在，请先创建", repo.spaceName)
	}

	log.Printf("✅ [用户仓库-Vearch] 用户存储库初始化成功")
	return nil
}

// getStringFromUserData 安全地从map中获取字符串值
func getStringFromUserData(data map[string]interface{}, key string) string {
	if value, ok := data[key].(string); ok {
		return value
	}
	return ""
}
