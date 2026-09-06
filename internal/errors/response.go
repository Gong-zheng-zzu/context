package errors

import (
	"encoding/json"
	"fmt"
)

// ErrorResponse 标准错误响应格式
type ErrorResponse struct {
	// Code 错误码
	Code int `json:"code"`

	// Message 错误消息（用户友好的描述）
	Message string `json:"message"`

	// Detail 详细错误信息（可选，用于调试）
	Detail string `json:"detail,omitempty"`

	// RequestID 请求ID（可选，用于追踪）
	RequestID string `json:"request_id,omitempty"`
}

// Error 实现 error 接口
func (e *ErrorResponse) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("[错误码:%d] %s: %s", e.Code, e.Message, e.Detail)
	}
	return fmt.Sprintf("[错误码:%d] %s", e.Code, e.Message)
}

// ToJSON 转换为 JSON 字符串
func (e *ErrorResponse) ToJSON() string {
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Sprintf(`{"code":%d,"message":"JSON序列化失败"}`, CodeInternalError)
	}
	return string(data)
}

// NewError 创建新的错误响应
func NewError(code int, detail string) *ErrorResponse {
	return &ErrorResponse{
		Code:    code,
		Message: GetErrorMessage(code),
		Detail:  detail,
	}
}

// NewErrorWithMessage 创建带自定义消息的错误响应
func NewErrorWithMessage(code int, message string, detail string) *ErrorResponse {
	return &ErrorResponse{
		Code:    code,
		Message: message,
		Detail:  detail,
	}
}

// Wrap 包装原始错误为 ErrorResponse
func Wrap(code int, err error) *ErrorResponse {
	if err == nil {
		return nil
	}
	return &ErrorResponse{
		Code:    code,
		Message: GetErrorMessage(code),
		Detail:  err.Error(),
	}
}

// WrapWithMessage 包装原始错误并指定消息
func WrapWithMessage(code int, message string, err error) *ErrorResponse {
	if err == nil {
		return nil
	}
	return &ErrorResponse{
		Code:    code,
		Message: message,
		Detail:  err.Error(),
	}
}

// SuccessResponse 标准成功响应格式
type SuccessResponse struct {
	// Code 状态码（0 表示成功）
	Code int `json:"code"`

	// Message 消息
	Message string `json:"message"`

	// Data 返回的数据（可选）
	Data interface{} `json:"data,omitempty"`
}

// NewSuccess 创建成功响应
func NewSuccess(data interface{}) *SuccessResponse {
	return &SuccessResponse{
		Code:    CodeSuccess,
		Message: GetErrorMessage(CodeSuccess),
		Data:    data,
	}
}

// NewSuccessWithMessage 创建带自定义消息的成功响应
func NewSuccessWithMessage(message string, data interface{}) *SuccessResponse {
	return &SuccessResponse{
		Code:    CodeSuccess,
		Message: message,
		Data:    data,
	}
}

// ToJSON 转换为 JSON 字符串
func (s *SuccessResponse) ToJSON() string {
	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Sprintf(`{"code":%d,"message":"JSON序列化失败"}`, CodeInternalError)
	}
	return string(data)
}

// 常见错误快捷构造函数

// ErrInvalidParam 参数错误
func ErrInvalidParam(detail string) *ErrorResponse {
	return NewError(CodeInvalidParam, detail)
}

// ErrUnauthorized 未授权错误
func ErrUnauthorized(detail string) *ErrorResponse {
	return NewError(CodeUnauthorized, detail)
}

// ErrRateLimited 限流错误
func ErrRateLimited(detail string) *ErrorResponse {
	return NewError(CodeRateLimited, detail)
}

// ErrInternal 内部错误
func ErrInternal(detail string) *ErrorResponse {
	return NewError(CodeInternalError, detail)
}

// ErrConfigMissing 配置缺失错误
func ErrConfigMissing(configName string) *ErrorResponse {
	return NewError(CodeConfigMissing, fmt.Sprintf("缺少必需配置: %s", configName))
}

// ErrInitFailed 初始化失败错误
func ErrInitFailed(component string, err error) *ErrorResponse {
	detail := fmt.Sprintf("%s 初始化失败", component)
	if err != nil {
		detail = fmt.Sprintf("%s: %v", detail, err)
	}
	return NewError(CodeInitFailed, detail)
}

// ErrVectorDB 向量数据库错误
func ErrVectorDB(detail string) *ErrorResponse {
	return NewError(CodeVectorDBError, detail)
}

// ErrTimelineDB 时间线数据库错误
func ErrTimelineDB(detail string) *ErrorResponse {
	return NewError(CodeTimelineDBError, detail)
}

// ErrKnowledgeGraph 知识图谱错误
func ErrKnowledgeGraph(detail string) *ErrorResponse {
	return NewError(CodeKnowledgeGraphError, detail)
}

// ErrSessionStore 会话存储错误
func ErrSessionStore(detail string) *ErrorResponse {
	return NewError(CodeSessionStoreError, detail)
}

// ErrStoragePath 存储路径错误
func ErrStoragePath(detail string) *ErrorResponse {
	return NewError(CodeStoragePathError, detail)
}

// ErrEmbedding 嵌入生成错误
func ErrEmbedding(detail string) *ErrorResponse {
	return NewError(CodeEmbeddingFailed, detail)
}

// ErrLLMGeneration LLM 生成错误
func ErrLLMGeneration(detail string) *ErrorResponse {
	return NewError(CodeLLMGenerationFailed, detail)
}

// ErrLLMTimeout LLM 超时错误
func ErrLLMTimeout(detail string) *ErrorResponse {
	return NewError(CodeLLMTimeout, detail)
}

// ErrSensitiveInfo 敏感信息检测错误
func ErrSensitiveInfo(detail string) *ErrorResponse {
	return NewError(CodeSensitiveInfoDetected, detail)
}

// ErrInvalidToken Token 无效错误
func ErrInvalidToken(detail string) *ErrorResponse {
	return NewError(CodeInvalidToken, detail)
}

// ErrPermissionDenied 权限不足错误
func ErrPermissionDenied(detail string) *ErrorResponse {
	return NewError(CodePermissionDenied, detail)
}
