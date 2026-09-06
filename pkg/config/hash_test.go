package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHashCalculator(t *testing.T) {
	hc := NewHashCalculator()

	t.Run("CalculateStringHash", func(t *testing.T) {
		content := "test content"
		hash1 := hc.CalculateStringHash(content)
		hash2 := hc.CalculateStringHash(content)

		if hash1 != hash2 {
			t.Errorf("相同内容的hash不一致: %s != %s", hash1, hash2)
		}

		if len(hash1) != 64 {
			t.Errorf("hash长度不正确: %d, 期望 64", len(hash1))
		}
	})

	t.Run("CalculateBytesHash", func(t *testing.T) {
		data := []byte("test content")
		hash := hc.CalculateBytesHash(data)

		if len(hash) != 64 {
			t.Errorf("hash长度不正确: %d, 期望 64", len(hash))
		}
	})

	t.Run("CalculateFileHash", func(t *testing.T) {
		// 创建临时文件
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "test.txt")
		content := []byte("test file content")

		if err := os.WriteFile(tmpFile, content, 0644); err != nil {
			t.Fatalf("创建临时文件失败: %v", err)
		}

		hash, err := hc.CalculateFileHash(tmpFile)
		if err != nil {
			t.Fatalf("计算文件hash失败: %v", err)
		}

		if len(hash) != 64 {
			t.Errorf("hash长度不正确: %d, 期望 64", len(hash))
		}

		// 验证幂等性
		hash2, err := hc.CalculateFileHash(tmpFile)
		if err != nil {
			t.Fatalf("第二次计算文件hash失败: %v", err)
		}

		if hash != hash2 {
			t.Errorf("相同文件的hash不一致: %s != %s", hash, hash2)
		}
	})

	t.Run("ValidateHash", func(t *testing.T) {
		validHash := "a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3"
		if !hc.ValidateHash(validHash) {
			t.Error("有效hash被判定为无效")
		}

		invalidHashes := []string{
			"",
			"invalid",
			"a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae", // 太短
			"a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae33", // 太长
			"g665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3", // 无效字符
		}

		for _, invalidHash := range invalidHashes {
			if hc.ValidateHash(invalidHash) {
				t.Errorf("无效hash被判定为有效: %s", invalidHash)
			}
		}
	})

	t.Run("CompareHashes", func(t *testing.T) {
		hash1 := "a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3"
		hash2 := "a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3"
		hash3 := "b665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3"

		if !hc.CompareHashes(hash1, hash2) {
			t.Error("相同hash比较失败")
		}

		if hc.CompareHashes(hash1, hash3) {
			t.Error("不同hash比较错误")
		}
	})
}
