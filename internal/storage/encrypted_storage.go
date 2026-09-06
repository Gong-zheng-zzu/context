package storage

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"
	"sync"

	"github.com/contextkeeper/service/internal/api"
	"github.com/contextkeeper/service/internal/security"
)

// EncryptedStorage 加密存储服务
type EncryptedStorage struct {
	encryptionKey []byte                      // AES-256密钥（32字节）
	detector      *security.Detector          // 敏感信息检测器
	records       map[string]*api.HealthRecord // 内存存储（key: recordID）
	userIndex     map[string][]string         // 用户索引（key: userID, value: []recordID）
	mutex         sync.RWMutex                // 读写锁
}

// NewEncryptedStorage 创建加密存储服务
func NewEncryptedStorage() (*EncryptedStorage, error) {
	// 从环境变量获取加密密钥，如果没有则生成一个
	keyStr := os.Getenv("ENCRYPTION_KEY")
	var key []byte

	if keyStr != "" {
		// 使用提供的密钥
		decoded, err := base64.StdEncoding.DecodeString(keyStr)
		if err != nil {
			return nil, fmt.Errorf("解码加密密钥失败: %w", err)
		}
		if len(decoded) != 32 {
			return nil, fmt.Errorf("加密密钥长度必须为32字节，当前: %d", len(decoded))
		}
		key = decoded
		log.Println("✅ [加密存储] 使用环境变量中的加密密钥")
	} else {
		// 生成新密钥
		key = make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, key); err != nil {
			return nil, fmt.Errorf("生成加密密钥失败: %w", err)
		}
		log.Println("⚠️ [加密存储] 未设置ENCRYPTION_KEY环境变量，已生成临时密钥")
		log.Printf("   建议设置环境变量: ENCRYPTION_KEY=%s", base64.StdEncoding.EncodeToString(key))
	}

	return &EncryptedStorage{
		encryptionKey: key,
		detector:      security.NewDetector(),
		records:       make(map[string]*api.HealthRecord),
		userIndex:     make(map[string][]string),
	}, nil
}

// Encrypt 加密数据（AES-256-GCM）
func (s *EncryptedStorage) Encrypt(plaintext string) (string, error) {
	// 1. 创建AES cipher
	block, err := aes.NewCipher(s.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("创建AES cipher失败: %w", err)
	}

	// 2. 创建GCM模式（认证加密）
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("创建GCM失败: %w", err)
	}

	// 3. 生成随机nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("生成nonce失败: %w", err)
	}

	// 4. 加密（附带认证标签）
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)

	// 5. Base64编码
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt 解密数据（AES-256-GCM）
func (s *EncryptedStorage) Decrypt(ciphertext string) (string, error) {
	// 1. Base64解码
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("Base64解码失败: %w", err)
	}

	// 2. 创建AES cipher
	block, err := aes.NewCipher(s.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("创建AES cipher失败: %w", err)
	}

	// 3. 创建GCM模式
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("创建GCM失败: %w", err)
	}

	// 4. 提取nonce
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("密文长度不足")
	}
	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]

	// 5. 解密并验证
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", fmt.Errorf("解密失败: %w", err)
	}

	return string(plaintext), nil
}

// SaveHealthRecord 保存健康记录（加密）
func (s *EncryptedStorage) SaveHealthRecord(record *api.HealthRecord) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// 🔒 敏感字段加密
	encryptedContent, err := s.Encrypt(record.Content)
	if err != nil {
		return fmt.Errorf("加密内容失败: %w", err)
	}

	// 创建加密后的记录副本
	encryptedRecord := *record
	encryptedRecord.Content = encryptedContent

	// 存储到内存
	s.records[record.RecordID] = &encryptedRecord
	s.userIndex[record.UserID] = append(s.userIndex[record.UserID], record.RecordID)

	log.Printf("🔒 [加密存储] 记录已加密保存: record_id=%s, user_id=%s",
		record.RecordID, record.UserID)

	return nil
}

// GetHealthRecord 获取健康记录（解密）
func (s *EncryptedStorage) GetHealthRecord(recordID string) (*api.HealthRecord, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	encryptedRecord, exists := s.records[recordID]
	if !exists {
		return nil, fmt.Errorf("记录不存在: %s", recordID)
	}

	// 🔒 解密内容
	decryptedContent, err := s.Decrypt(encryptedRecord.Content)
	if err != nil {
		return nil, fmt.Errorf("解密内容失败: %w", err)
	}

	// 创建解密后的记录副本
	decryptedRecord := *encryptedRecord
	decryptedRecord.Content = decryptedContent

	return &decryptedRecord, nil
}

// GetUserHealthRecords 获取用户的所有健康记录（解密）
func (s *EncryptedStorage) GetUserHealthRecords(userID string) ([]*api.HealthRecord, error) {
	s.mutex.RLock()
	recordIDs, exists := s.userIndex[userID]
	s.mutex.RUnlock()

	if !exists {
		return []*api.HealthRecord{}, nil
	}

	var records []*api.HealthRecord
	for _, recordID := range recordIDs {
		record, err := s.GetHealthRecord(recordID)
		if err != nil {
			log.Printf("⚠️ [加密存储] 获取记录失败: %v", err)
			continue
		}

		// 🔒 返回前再次脱敏（双重保险）
		redactedContent, _ := s.detector.DetectAndRedact(record.Content)
		record.Content = redactedContent

		records = append(records, record)
	}

	log.Printf("🔒 [加密存储] 获取用户记录: user_id=%s, count=%d", userID, len(records))

	return records, nil
}

// DeleteHealthRecord 删除健康记录
func (s *EncryptedStorage) DeleteHealthRecord(recordID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	record, exists := s.records[recordID]
	if !exists {
		return fmt.Errorf("记录不存在: %s", recordID)
	}

	// 从用户索引中删除
	userID := record.UserID
	if recordIDs, exists := s.userIndex[userID]; exists {
		newRecordIDs := make([]string, 0)
		for _, id := range recordIDs {
			if id != recordID {
				newRecordIDs = append(newRecordIDs, id)
			}
		}
		s.userIndex[userID] = newRecordIDs
	}

	// 删除记录
	delete(s.records, recordID)

	log.Printf("🔒 [加密存储] 记录已删除: record_id=%s", recordID)

	return nil
}

// GetStats 获取存储统计信息
func (s *EncryptedStorage) GetStats() map[string]interface{} {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return map[string]interface{}{
		"total_records": len(s.records),
		"total_users":   len(s.userIndex),
		"encryption":    "AES-256-GCM",
	}
}

// Clear 清空所有记录（用于测试）
func (s *EncryptedStorage) Clear() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.records = make(map[string]*api.HealthRecord)
	s.userIndex = make(map[string][]string)

	log.Println("🔒 [加密存储] 所有记录已清空")
}
