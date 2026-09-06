package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
)

// FileEncryption 文件加密工具
type FileEncryption struct {
	key []byte
}

// NewFileEncryption 创建文件加密工具
// userID: 用户ID，用于生成加密密钥
func NewFileEncryption(userID string) *FileEncryption {
	// 使用用户ID生成32字节的AES-256密钥
	hash := sha256.Sum256([]byte(userID + "_file_encryption_key_salt_2024"))
	return &FileEncryption{
		key: hash[:],
	}
}

// Encrypt 加密文件内容
func (fe *FileEncryption) Encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(fe.key)
	if err != nil {
		return nil, fmt.Errorf("创建加密器失败: %w", err)
	}

	// 创建GCM模式（提供认证加密）
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("创建GCM失败: %w", err)
	}

	// 生成随机nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("生成nonce失败: %w", err)
	}

	// 加密并附加认证标签
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt 解密文件内容
func (fe *FileEncryption) Decrypt(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(fe.key)
	if err != nil {
		return nil, fmt.Errorf("创建解密器失败: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("创建GCM失败: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("密文太短")
	}

	// 提取nonce和密文
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

	// 解密并验证
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("解密失败: %w", err)
	}

	return plaintext, nil
}

// GetKeyFingerprint 获取密钥指纹（用于日志记录，不泄露实际密钥）
func (fe *FileEncryption) GetKeyFingerprint() string {
	hash := sha256.Sum256(fe.key)
	return hex.EncodeToString(hash[:8]) // 只返回前8字节
}
