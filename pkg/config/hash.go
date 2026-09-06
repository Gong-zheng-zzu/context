package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// HashCalculator 配置文件hash计算器
type HashCalculator struct{}

// NewHashCalculator 创建新的hash计算器
func NewHashCalculator() *HashCalculator {
	return &HashCalculator{}
}

// CalculateFileHash 计算文件的SHA-256 hash
func (hc *HashCalculator) CalculateFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("打开文件失败: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("计算hash失败: %w", err)
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// CalculateStringHash 计算字符串的SHA-256 hash
func (hc *HashCalculator) CalculateStringHash(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}

// CalculateBytesHash 计算字节数组的SHA-256 hash
func (hc *HashCalculator) CalculateBytesHash(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// ValidateHash 验证hash是否为有效的SHA-256格式（64个十六进制字符）
func (hc *HashCalculator) ValidateHash(hash string) bool {
	if len(hash) != 64 {
		return false
	}

	for _, c := range hash {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}

	return true
}

// CompareHashes 比较两个hash是否相同
func (hc *HashCalculator) CompareHashes(hash1, hash2 string) bool {
	return hash1 == hash2
}
