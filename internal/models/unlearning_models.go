package models

import "time"

// UnlearningRequest 机器遗忘请求
type UnlearningRequest struct {
	// UserID 待遗忘用户ID
	UserID string `json:"user_id" binding:"required"`

	// Epsilon 差分隐私预算（默认1.0）
	Epsilon float64 `json:"epsilon,omitempty"`

	// LearningRate 学习率（默认0.001）
	LearningRate float64 `json:"learning_rate,omitempty"`

	// MaxIterations 最大迭代次数（默认50）
	MaxIterations int `json:"max_iterations,omitempty"`

	// CollectionNames 要清除的集合列表（空则清除所有）
	CollectionNames []string `json:"collection_names,omitempty"`

	// ConvergenceThreshold 收敛阈值（默认0.001）
	ConvergenceThreshold float64 `json:"convergence_threshold,omitempty"`

	// RetainedSampleRatio 留存数据采样比例（默认10倍）
	RetainedSampleRatio int `json:"retained_sample_ratio,omitempty"`

	// RebuildIndex 是否重建HNSW索引（默认false）
	RebuildIndex bool `json:"rebuild_index,omitempty"`
}

// UnlearningResult 机器遗忘执行结果
type UnlearningResult struct {
	// UserID 用户ID
	UserID string `json:"user_id"`

	// Status 执行状态：success/partial/failed
	Status string `json:"status"`

	// IterationsUsed 实际使用的迭代次数
	IterationsUsed int `json:"iterations_used"`

	// InitialFairnessLoss 初始公平性损失
	InitialFairnessLoss float64 `json:"initial_fairness_loss"`

	// FinalFairnessLoss 最终公平性损失
	FinalFairnessLoss float64 `json:"final_fairness_loss"`

	// FairnessLossChange 公平性损失变化
	FairnessLossChange float64 `json:"fairness_loss_change"`

	// RemovedVectorCount 删除的向量数量
	RemovedVectorCount int `json:"removed_vector_count"`

	// UpdatedVectorCount 更新的向量数量
	UpdatedVectorCount int `json:"updated_vector_count"`

	// Duration 执行耗时
	Duration time.Duration `json:"duration"`

	// CollectionsAffected 受影响的集合列表
	CollectionsAffected []string `json:"collections_affected"`

	// PrivacyBudgetConsumed 消耗的隐私预算
	PrivacyBudgetConsumed float64 `json:"privacy_budget_consumed"`

	// PrivacyBudgetUsedTotal 该用户累计消耗的隐私预算（含本次，进程内存态记账）
	PrivacyBudgetUsedTotal float64 `json:"privacy_budget_used_total,omitempty"`

	// IndexRebuildStatus 索引重建真实状态：
	// 空 = 未启用重建；not_supported_by_backend = 后端无重建/优化接口；
	// rebuilt = 重建成功；failed:<原因> = 重建失败
	IndexRebuildStatus string `json:"index_rebuild,omitempty"`

	// ConvergenceAchieved 是否达到收敛
	ConvergenceAchieved bool `json:"convergence_achieved"`

	// ErrorMessage 错误信息（如果失败）
	ErrorMessage string `json:"error_message,omitempty"`

	// Timestamp 执行时间戳
	Timestamp time.Time `json:"timestamp"`
}

// UnlearningVerifyRequest 遗忘验证请求
type UnlearningVerifyRequest struct {
	// UserID 用户ID
	UserID string `json:"user_id" binding:"required"`

	// QueryText 查询文本（可选，用于相似度验证）
	QueryText string `json:"query_text,omitempty"`

	// CollectionNames 要验证的集合列表
	CollectionNames []string `json:"collection_names,omitempty"`

	// SimilarityThreshold 相似度阈值（默认0.1）
	SimilarityThreshold float64 `json:"similarity_threshold,omitempty"`
}

// UnlearningVerifyResult 遗忘验证结果
type UnlearningVerifyResult struct {
	// UserID 用户ID
	UserID string `json:"user_id"`

	// IsFullyUnlearned 是否完全遗忘（所有指标通过）
	IsFullyUnlearned bool `json:"is_fully_unlearned"`

	// RemainingVectorCount 剩余向量数量（应为0）
	RemainingVectorCount int `json:"remaining_vector_count"`

	// MaxSimilarity 最大相似度（应<0.1）
	MaxSimilarity float64 `json:"max_similarity"`

	// AverageSimilarity 平均相似度
	AverageSimilarity float64 `json:"average_similarity"`

	// CollectionsChecked 已检查的集合列表
	CollectionsChecked []string `json:"collections_checked"`

	// VerificationDetails 验证详情（每个集合的结果）
	VerificationDetails []*CollectionVerifyDetail `json:"verification_details"`

	// Timestamp 验证时间戳
	Timestamp time.Time `json:"timestamp"`
}

// CollectionVerifyDetail 集合验证详情
type CollectionVerifyDetail struct {
	// CollectionName 集合名称
	CollectionName string `json:"collection_name"`

	// RemainingVectors 剩余向量数量
	RemainingVectors int `json:"remaining_vectors"`

	// MaxSimilarity 最大相似度
	MaxSimilarity float64 `json:"max_similarity"`

	// Passed 是否通过验证
	Passed bool `json:"passed"`
}

// VectorUpdateRequest 向量更新请求
type VectorUpdateRequest struct {
	// ID 向量ID
	ID string `json:"id" binding:"required"`

	// CollectionName 集合名称
	CollectionName string `json:"collection_name" binding:"required"`

	// NewVector 新向量值
	NewVector []float32 `json:"new_vector" binding:"required"`

	// Metadata 元数据（可选更新）
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// BatchVectorUpdateRequest 批量向量更新请求
type BatchVectorUpdateRequest struct {
	// CollectionName 集合名称
	CollectionName string `json:"collection_name" binding:"required"`

	// Updates 更新列表
	Updates []VectorUpdateItem `json:"updates" binding:"required"`
}

// VectorUpdateItem 向量更新项
type VectorUpdateItem struct {
	// ID 向量ID
	ID string `json:"id"`

	// Vector 新向量值
	Vector []float32 `json:"vector"`

	// Metadata 元数据（可选）
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// VectorRecord 向量记录（用于查询结果）
type VectorRecord struct {
	// ID 向量ID
	ID string `json:"id"`

	// CollectionName 集合名称
	CollectionName string `json:"collection_name"`

	// Vector 向量值
	Vector []float32 `json:"vector"`

	// Metadata 元数据
	Metadata map[string]interface{} `json:"metadata"`

	// UserID 用户ID（如果有）
	UserID string `json:"user_id,omitempty"`

	// CreatedAt 创建时间
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// UnlearningAuditEvent 机器遗忘审计事件
type UnlearningAuditEvent struct {
	// EventID 事件ID
	EventID string `json:"event_id"`

	// Timestamp 时间戳
	Timestamp time.Time `json:"timestamp"`

	// UserID 用户ID
	UserID string `json:"user_id"`

	// RequestIP 请求IP
	RequestIP string `json:"request_ip"`

	// OperationType 操作类型：unlearn/verify
	OperationType string `json:"operation_type"`

	// Request 请求详情（JSON）
	Request interface{} `json:"request"`

	// Result 执行结果（JSON）
	Result interface{} `json:"result"`

	// Status 状态：success/failed
	Status string `json:"status"`

	// ErrorMessage 错误信息
	ErrorMessage string `json:"error_message,omitempty"`

	// PrivacyBudget 隐私预算消耗
	PrivacyBudget float64 `json:"privacy_budget"`

	// AffectedCollections 受影响的集合
	AffectedCollections []string `json:"affected_collections"`

	// Duration 执行耗时（毫秒）
	Duration int64 `json:"duration_ms"`
}

// UnlearningConfig 机器遗忘配置
type UnlearningConfig struct {
	// Enabled 是否启用机器遗忘功能
	Enabled bool `json:"enabled"`

	// DefaultEpsilon 默认隐私预算
	DefaultEpsilon float64 `json:"default_epsilon"`

	// DefaultLearningRate 默认学习率
	DefaultLearningRate float64 `json:"default_learning_rate"`

	// DefaultMaxIterations 默认最大迭代次数
	DefaultMaxIterations int `json:"default_max_iterations"`

	// DefaultConvergenceThreshold 默认收敛阈值
	DefaultConvergenceThreshold float64 `json:"default_convergence_threshold"`

	// DefaultRetainedSampleRatio 默认留存数据采样比例
	DefaultRetainedSampleRatio int `json:"default_retained_sample_ratio"`

	// MaxPrivacyBudgetPerUser 每用户最大隐私预算
	MaxPrivacyBudgetPerUser float64 `json:"max_privacy_budget_per_user"`

	// AutoRebuildIndex 是否自动重建索引
	AutoRebuildIndex bool `json:"auto_rebuild_index"`

	// EnableAuditLog 是否启用审计日志
	EnableAuditLog bool `json:"enable_audit_log"`
}
