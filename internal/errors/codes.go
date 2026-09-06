package errors

// 错误码体系定义
// 1xxx: 通用错误
// 2xxx: 存储错误
// 3xxx: LLM 错误
// 4xxx: 安全错误

const (
	// ========== 1xxx: 通用错误 ==========

	// CodeSuccess 成功
	CodeSuccess = 0

	// CodeInvalidParam 参数错误
	CodeInvalidParam = 1001

	// CodeUnauthorized 未授权
	CodeUnauthorized = 1002

	// CodeRateLimited 请求限流
	CodeRateLimited = 1003

	// CodeInternalError 内部错误
	CodeInternalError = 1004

	// CodeConfigMissing 配置缺失
	CodeConfigMissing = 1005

	// CodeInitFailed 初始化失败
	CodeInitFailed = 1006

	// ========== 2xxx: 存储错误 ==========

	// CodeVectorDBError 向量数据库错误
	CodeVectorDBError = 2001

	// CodeTimelineDBError 时间线数据库错误
	CodeTimelineDBError = 2002

	// CodeKnowledgeGraphError 知识图谱错误
	CodeKnowledgeGraphError = 2003

	// CodeSessionStoreError 会话存储错误
	CodeSessionStoreError = 2004

	// CodeStoragePathError 存储路径错误
	CodeStoragePathError = 2005

	// ========== 3xxx: LLM 错误 ==========

	// CodeEmbeddingFailed 嵌入生成失败
	CodeEmbeddingFailed = 3001

	// CodeLLMGenerationFailed LLM 生成失败
	CodeLLMGenerationFailed = 3002

	// CodeLLMTimeout LLM 请求超时
	CodeLLMTimeout = 3003

	// CodeEmbeddingAPIError 嵌入 API 错误
	CodeEmbeddingAPIError = 3004

	// ========== 4xxx: 安全错误 ==========

	// CodeSensitiveInfoDetected 敏感信息检测
	CodeSensitiveInfoDetected = 4001

	// CodeInvalidToken Token 无效
	CodeInvalidToken = 4002

	// CodePermissionDenied 权限不足
	CodePermissionDenied = 4003
)

// ErrorMessage 错误码对应的中文消息
var ErrorMessage = map[int]string{
	// 通用错误
	CodeSuccess:        "成功",
	CodeInvalidParam:   "参数错误",
	CodeUnauthorized:   "未授权",
	CodeRateLimited:    "请求过于频繁，请稍后重试",
	CodeInternalError:  "内部错误",
	CodeConfigMissing:  "配置缺失",
	CodeInitFailed:     "初始化失败",

	// 存储错误
	CodeVectorDBError:       "向量数据库错误",
	CodeTimelineDBError:     "时间线数据库错误",
	CodeKnowledgeGraphError: "知识图谱错误",
	CodeSessionStoreError:   "会话存储错误",
	CodeStoragePathError:    "存储路径错误",

	// LLM 错误
	CodeEmbeddingFailed:     "向量嵌入生成失败",
	CodeLLMGenerationFailed: "LLM 生成失败",
	CodeLLMTimeout:          "LLM 请求超时",
	CodeEmbeddingAPIError:   "嵌入 API 错误",

	// 安全错误
	CodeSensitiveInfoDetected: "检测到敏感信息",
	CodeInvalidToken:          "Token 无效",
	CodePermissionDenied:      "权限不足",
}

// GetErrorMessage 获取错误码对应的中文消息
func GetErrorMessage(code int) string {
	if msg, ok := ErrorMessage[code]; ok {
		return msg
	}
	return "未知错误"
}

// IsClientError 判断是否为客户端错误（4xx 类）
func IsClientError(code int) bool {
	return code >= 1000 && code < 2000
}

// IsStorageError 判断是否为存储错误（2xxx 类）
func IsStorageError(code int) bool {
	return code >= 2000 && code < 3000
}

// IsLLMError 判断是否为 LLM 错误（3xxx 类）
func IsLLMError(code int) bool {
	return code >= 3000 && code < 4000
}

// IsSecurityError 判断是否为安全错误（4xxx 类）
func IsSecurityError(code int) bool {
	return code >= 4000 && code < 5000
}
