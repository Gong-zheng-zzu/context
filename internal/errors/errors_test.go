package errors

import (
	"errors"
	"testing"
)

// TestErrorCodes 测试错误码定义
func TestErrorCodes(t *testing.T) {
	tests := []struct {
		name     string
		code     int
		wantMsg  string
		category string
	}{
		{"成功", CodeSuccess, "成功", "success"},
		{"参数错误", CodeInvalidParam, "参数错误", "client"},
		{"未授权", CodeUnauthorized, "未授权", "client"},
		{"限流", CodeRateLimited, "请求过于频繁，请稍后重试", "client"},
		{"向量数据库错误", CodeVectorDBError, "向量数据库错误", "storage"},
		{"时间线数据库错误", CodeTimelineDBError, "时间线数据库错误", "storage"},
		{"知识图谱错误", CodeKnowledgeGraphError, "知识图谱错误", "storage"},
		{"嵌入失败", CodeEmbeddingFailed, "向量嵌入生成失败", "llm"},
		{"LLM生成失败", CodeLLMGenerationFailed, "LLM 生成失败", "llm"},
		{"LLM超时", CodeLLMTimeout, "LLM 请求超时", "llm"},
		{"敏感信息检测", CodeSensitiveInfoDetected, "检测到敏感信息", "security"},
		{"Token无效", CodeInvalidToken, "Token 无效", "security"},
		{"权限不足", CodePermissionDenied, "权限不足", "security"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := GetErrorMessage(tt.code)
			if msg != tt.wantMsg {
				t.Errorf("GetErrorMessage(%d) = %v, want %v", tt.code, msg, tt.wantMsg)
			}

			// 测试错误类别判断
			switch tt.category {
			case "client":
				if !IsClientError(tt.code) {
					t.Errorf("IsClientError(%d) = false, want true", tt.code)
				}
			case "storage":
				if !IsStorageError(tt.code) {
					t.Errorf("IsStorageError(%d) = false, want true", tt.code)
				}
			case "llm":
				if !IsLLMError(tt.code) {
					t.Errorf("IsLLMError(%d) = false, want true", tt.code)
				}
			case "security":
				if !IsSecurityError(tt.code) {
					t.Errorf("IsSecurityError(%d) = false, want true", tt.code)
				}
			}
		})
	}
}

// TestErrorResponse 测试错误响应
func TestErrorResponse(t *testing.T) {
	t.Run("NewError", func(t *testing.T) {
		err := NewError(CodeInvalidParam, "缺少必需参数: userId")
		if err.Code != CodeInvalidParam {
			t.Errorf("Code = %d, want %d", err.Code, CodeInvalidParam)
		}
		if err.Message != "参数错误" {
			t.Errorf("Message = %s, want %s", err.Message, "参数错误")
		}
		if err.Detail != "缺少必需参数: userId" {
			t.Errorf("Detail = %s, want %s", err.Detail, "缺少必需参数: userId")
		}
	})

	t.Run("Error方法", func(t *testing.T) {
		err := NewError(CodeVectorDBError, "连接失败")
		expected := "[错误码:2001] 向量数据库错误: 连接失败"
		if err.Error() != expected {
			t.Errorf("Error() = %s, want %s", err.Error(), expected)
		}
	})

	t.Run("ToJSON", func(t *testing.T) {
		err := NewError(CodeUnauthorized, "Token已过期")
		json := err.ToJSON()
		if json == "" {
			t.Error("ToJSON() returned empty string")
		}
		// 验证JSON包含关键字段
		if !contains(json, `"code":1002`) {
			t.Error("JSON missing code field")
		}
		if !contains(json, `"message"`) {
			t.Error("JSON missing message field")
		}
	})
}

// TestWrapError 测试错误包装
func TestWrapError(t *testing.T) {
	t.Run("包装nil错误", func(t *testing.T) {
		err := Wrap(CodeInternalError, nil)
		if err != nil {
			t.Errorf("Wrap(nil) = %v, want nil", err)
		}
	})

	t.Run("包装标准错误", func(t *testing.T) {
		originalErr := errors.New("数据库连接超时")
		err := Wrap(CodeVectorDBError, originalErr)
		if err == nil {
			t.Fatal("Wrap() returned nil")
		}
		if err.Code != CodeVectorDBError {
			t.Errorf("Code = %d, want %d", err.Code, CodeVectorDBError)
		}
		if err.Detail != "数据库连接超时" {
			t.Errorf("Detail = %s, want %s", err.Detail, "数据库连接超时")
		}
	})
}

// TestSuccessResponse 测试成功响应
func TestSuccessResponse(t *testing.T) {
	t.Run("NewSuccess", func(t *testing.T) {
		data := map[string]interface{}{"userId": "user_123", "count": 10}
		resp := NewSuccess(data)
		if resp.Code != CodeSuccess {
			t.Errorf("Code = %d, want %d", resp.Code, CodeSuccess)
		}
		if resp.Message != "成功" {
			t.Errorf("Message = %s, want %s", resp.Message, "成功")
		}
		if resp.Data == nil {
			t.Error("Data is nil")
		}
	})

	t.Run("ToJSON", func(t *testing.T) {
		resp := NewSuccess(map[string]string{"status": "ok"})
		json := resp.ToJSON()
		if json == "" {
			t.Error("ToJSON() returned empty string")
		}
		if !contains(json, `"code":0`) {
			t.Error("JSON missing code field")
		}
	})
}

// TestErrorHelpers 测试错误辅助函数
func TestErrorHelpers(t *testing.T) {
	tests := []struct {
		name     string
		fn       func() *ErrorResponse
		wantCode int
	}{
		{"ErrInvalidParam", func() *ErrorResponse { return ErrInvalidParam("test") }, CodeInvalidParam},
		{"ErrUnauthorized", func() *ErrorResponse { return ErrUnauthorized("test") }, CodeUnauthorized},
		{"ErrRateLimited", func() *ErrorResponse { return ErrRateLimited("test") }, CodeRateLimited},
		{"ErrInternal", func() *ErrorResponse { return ErrInternal("test") }, CodeInternalError},
		{"ErrConfigMissing", func() *ErrorResponse { return ErrConfigMissing("API_KEY") }, CodeConfigMissing},
		{"ErrVectorDB", func() *ErrorResponse { return ErrVectorDB("test") }, CodeVectorDBError},
		{"ErrTimelineDB", func() *ErrorResponse { return ErrTimelineDB("test") }, CodeTimelineDBError},
		{"ErrKnowledgeGraph", func() *ErrorResponse { return ErrKnowledgeGraph("test") }, CodeKnowledgeGraphError},
		{"ErrSessionStore", func() *ErrorResponse { return ErrSessionStore("test") }, CodeSessionStoreError},
		{"ErrStoragePath", func() *ErrorResponse { return ErrStoragePath("test") }, CodeStoragePathError},
		{"ErrEmbedding", func() *ErrorResponse { return ErrEmbedding("test") }, CodeEmbeddingFailed},
		{"ErrLLMGeneration", func() *ErrorResponse { return ErrLLMGeneration("test") }, CodeLLMGenerationFailed},
		{"ErrLLMTimeout", func() *ErrorResponse { return ErrLLMTimeout("test") }, CodeLLMTimeout},
		{"ErrSensitiveInfo", func() *ErrorResponse { return ErrSensitiveInfo("test") }, CodeSensitiveInfoDetected},
		{"ErrInvalidToken", func() *ErrorResponse { return ErrInvalidToken("test") }, CodeInvalidToken},
		{"ErrPermissionDenied", func() *ErrorResponse { return ErrPermissionDenied("test") }, CodePermissionDenied},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			if err.Code != tt.wantCode {
				t.Errorf("Code = %d, want %d", err.Code, tt.wantCode)
			}
		})
	}
}

// 辅助函数：检查字符串是否包含子串
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
