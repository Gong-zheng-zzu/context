package causal_reasoning

import (
	"time"
)

// CausalRelation O→C→P→R因果四元组
// 对应策划书第2.2.2节"因果推理模型的图谱构建"
type CausalRelation struct {
	CandidateID    string             `json:"candidate_id,omitempty"`
	Object         string             `json:"object"`          // O: 对象（如"李爷爷"）
	Mediator       string             `json:"mediator"`        // C: 中介/共现因素（如"服用降压药"）
	Property       string             `json:"property"`        // P: 属性/机制（如"体位性低血压"）
	Result         string             `json:"result"`          // R: 结果（如"洗手间滑倒"）
	Confidence     float64            `json:"confidence"`      // PCCM融合后的最终置信度
	RuleConfidence float64            `json:"rule_confidence"` // 规则匹配置信度
	PMIConfidence  float64            `json:"pmi_confidence"`  // PMI统计置信度
	LLMConfidence  float64            `json:"llm_confidence"`  // LLM推理置信度
	Evidence       []string           `json:"evidence"`        // 证据文本片段
	RuleMatches    []RuleEvidence     `json:"rule_matches,omitempty"`
	PCCMEvidence   *PCCMEvidence      `json:"pccm_evidence,omitempty"`
	Quality        *RelationQuality   `json:"quality,omitempty"`
	EvidenceSpans  []EvidenceSpan     `json:"evidence_spans,omitempty"`
	FieldScores    map[string]float64 `json:"field_scores,omitempty"`
	Decision       string             `json:"decision,omitempty"`
	Abstained      bool               `json:"abstained"`
	AbstainReason  string             `json:"abstain_reason,omitempty"`
	Timestamp      time.Time          `json:"timestamp"`
}

// EvidenceSpan identifies the exact source span supporting a field. Offsets
// are byte offsets, matching Go string slicing semantics.
type EvidenceSpan struct {
	Field string `json:"field"`
	Text  string `json:"text"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

// RelationQuality is an explicit, conservative quality gate for extracted
// tuples. A relation that lacks a verifiable field remains visible for review
// but is never presented as a fully verified causal tuple.
type RelationQuality struct {
	TupleValid         bool     `json:"tuple_valid"`
	EvidenceCoverage   float64  `json:"evidence_coverage"`
	NegationChecked    bool     `json:"negation_checked"`
	TemporalConsistent bool     `json:"temporal_consistent"`
	ReviewRequired     bool     `json:"review_required"`
	ConfidenceLevel    string   `json:"confidence_level"`
	ValidationErrors   []string `json:"validation_errors,omitempty"`
}

// RuleEvidence identifies the clinical rule that contributed observable
// evidence to an extraction. It deliberately contains no internal rule state.
type RuleEvidence struct {
	ID          string  `json:"id"`
	Condition   string  `json:"condition"`
	Effect      string  `json:"effect"`
	Confidence  float64 `json:"confidence"`
	Category    string  `json:"category"`
	Source      string  `json:"source"`
	Edge        string  `json:"edge,omitempty"`
	MatchedTerm string  `json:"matched_term,omitempty"`
}

// PCCMEvidence makes every final confidence auditable from its source signals.
type PCCMEvidence struct {
	RuleConfidence     float64     `json:"rule_confidence"`
	PMIConfidence      float64     `json:"pmi_confidence"`
	LLMConfidence      float64     `json:"llm_confidence"`
	Weights            PCCMWeights `json:"weights"`
	ActiveSources      []string    `json:"active_sources"`
	EvidenceCount      int         `json:"evidence_count"`
	FinalConfidence    float64     `json:"final_confidence"`
	CalibrationVersion string      `json:"calibration_version"`
}

// CausalPath 因果推理路径
type CausalPath struct {
	Nodes      []string `json:"nodes"`      // [O, C, P, R]
	Relations  []string `json:"relations"`  // 关系类型列表
	Confidence float64  `json:"confidence"` // 路径置信度（边置信度乘积）
	Evidence   []string `json:"evidence"`   // 支持证据
	Length     int      `json:"length"`     // 路径长度
}

// CausalChain 完整因果链（支持多跳推理）
type CausalChain struct {
	Source     string       `json:"source"`     // 起始节点
	Target     string       `json:"target"`     // 目标节点
	Paths      []CausalPath `json:"paths"`      // 所有可能路径
	Confidence float64      `json:"confidence"` // 最高置信度
	PathCount  int          `json:"path_count"` // 路径数量
	Timestamp  time.Time    `json:"timestamp"`
}

// ExtractRequest 因果关系抽取请求
type ExtractRequest struct {
	Persist       bool    `json:"persist"`
	Text          string  `json:"text" binding:"required"` // 待抽取文本
	UseRules      bool    `json:"use_rules"`               // 是否使用规则引擎
	UsePMI        bool    `json:"use_pmi"`                 // 是否使用PMI计算
	UseLLM        bool    `json:"use_llm"`                 // 是否使用LLM推理
	MinConfidence float64 `json:"min_confidence"`          // 最低置信度阈值
}

// ExtractResponse 因果关系抽取响应
type ExtractResponse struct {
	TraceID            string               `json:"trace_id,omitempty"`
	Stage              string               `json:"stage,omitempty"`
	Status             string               `json:"status,omitempty"`
	PipelineVersion    string               `json:"pipeline_version,omitempty"`
	StartedAt          time.Time            `json:"started_at,omitempty"`
	CompletedAt        time.Time            `json:"completed_at,omitempty"`
	CalibrationVersion string               `json:"calibration_version,omitempty"`
	Candidates         []CausalRelation     `json:"candidates,omitempty"`
	Abstained          bool                 `json:"abstained"`
	AbstainReason      string               `json:"abstain_reason,omitempty"`
	AnalysisOnly       bool                 `json:"analysis_only"`
	GraphPersisted     bool                 `json:"graph_persisted"`
	SecurityExecution  *SecurityExecution   `json:"security_execution,omitempty"`
	Execution          *ExtractionExecution `json:"execution,omitempty"`
	// Quality is the conservative aggregate quality gate for all returned
	// relations. Relation-level quality remains available on each relation.
	Quality       *RelationQuality     `json:"quality,omitempty"`
	Persistence   PersistenceExecution `json:"persistence"`
	Error         string               `json:"error,omitempty"`
	Relations     []CausalRelation     `json:"relations"`
	Count         int                  `json:"count"`
	ProcessTimeMs int64                `json:"process_time_ms"`
}

// ExtractionExecution documents which deterministic and model-backed paths
// actually ran. A configured model is not claimed to be reachable unless a
// model call succeeds.
type ExtractionExecution struct {
	Mode           string         `json:"mode"`
	UseRules       bool           `json:"use_rules"`
	UsePMI         bool           `json:"use_pmi"`
	UseLLM         bool           `json:"use_llm"`
	LLMAvailable   bool           `json:"llm_available"`
	ModelStatus    string         `json:"model_status"`
	Model          string         `json:"model,omitempty"`
	FallbackReason string         `json:"fallback_reason,omitempty"`
	MatchedRules   []RuleEvidence `json:"matched_rules,omitempty"`
	PCCMWeights    PCCMWeights    `json:"pccm_weights"`
	// ModelTier describes the actual serving tier (or deterministic fallback),
	// rather than the configured model alone.
	ModelTier          string           `json:"model_tier,omitempty"`
	CacheHit           bool             `json:"cache_hit"`
	LatencyBreakdownMs map[string]int64 `json:"latency_breakdown_ms,omitempty"`
}

// PersistenceExecution reports whether a request was analysis-only or wrote
// to the graph. It avoids exposing infrastructure error details in browsers.
type PersistenceExecution struct {
	Requested bool   `json:"requested"`
	Attempted bool   `json:"attempted"`
	Succeeded bool   `json:"succeeded"`
	Status    string `json:"status"`
}

// SecurityExecution is safe browser-visible status for analysis-only calls.
type SecurityExecution struct {
	ASDFChecked     bool     `json:"asdf_checked"`
	InputNormalized bool     `json:"input_normalized"`
	SensitiveTypes  []string `json:"sensitive_types,omitempty"`
}

// InferRequest 因果推理查询请求
type InferRequest struct {
	Query         string  `json:"query" binding:"required"` // 查询实体
	Direction     string  `json:"direction"`                // forward(正向推理)/backward(反向追溯)
	MaxDepth      int     `json:"max_depth"`                // 最大查询深度（默认4）
	MinConfidence float64 `json:"min_confidence"`           // 最低置信度阈值
	Limit         int     `json:"limit"`                    // 返回结果数量
}

// InferResponse 因果推理查询响应
type InferResponse struct {
	Query         string        `json:"query"`
	Direction     string        `json:"direction"`
	Chains        []CausalChain `json:"chains"`
	Count         int           `json:"count"`
	ProcessTimeMs int64         `json:"process_time_ms"`
}

// ChainQueryRequest 因果链查询请求
type ChainQueryRequest struct {
	From          string  `json:"from" binding:"required"` // 起始节点
	To            string  `json:"to" binding:"required"`   // 目标节点
	MaxDepth      int     `json:"max_depth"`               // 最大路径长度
	MinConfidence float64 `json:"min_confidence"`          // 最低置信度阈值
	Limit         int     `json:"limit"`                   // 返回路径数量
}

// ChainQueryResponse 因果链查询响应
type ChainQueryResponse struct {
	From          string       `json:"from"`
	To            string       `json:"to"`
	Chain         *CausalChain `json:"chain,omitempty"`
	Found         bool         `json:"found"`
	ProcessTimeMs int64        `json:"process_time_ms"`
}

// MedicalRule 医学因果规则
type MedicalRule struct {
	ID         string   `json:"id"`
	Condition  string   `json:"condition"`  // 前提条件（C）
	Effect     string   `json:"effect"`     // 结果效应（P/R）
	Confidence float64  `json:"confidence"` // 规则置信度
	Category   string   `json:"category"`   // 规则分类（药物/疾病/操作）
	Keywords   []string `json:"keywords"`   // 匹配关键词
	Source     string   `json:"source"`     // 规则来源（文献/专家）
}

// PMIScore PMI共现强度分数
type PMIScore struct {
	EntityA    string  `json:"entity_a"`
	EntityB    string  `json:"entity_b"`
	PMI        float64 `json:"pmi"`        // PMI(A,B) = log(P(A,B) / (P(A)*P(B)))
	CoOccur    int     `json:"co_occur"`   // 共现次数
	CountA     int     `json:"count_a"`    // A出现次数
	CountB     int     `json:"count_b"`    // B出现次数
	TotalDocs  int     `json:"total_docs"` // 总文档数
	Confidence float64 `json:"confidence"` // 归一化置信度[0,1]
}

// PCCMWeights PCCM置信度融合权重
type PCCMWeights struct {
	RuleWeight float64 `json:"rule_weight"` // 规则匹配权重（默认0.85）
	PMIWeight  float64 `json:"pmi_weight"`  // PMI统计权重（默认0.10）
	LLMWeight  float64 `json:"llm_weight"`  // LLM推理权重（默认0.05）
}

// DefaultPCCMWeights 默认PCCM权重配置
var DefaultPCCMWeights = PCCMWeights{
	RuleWeight: 0.85,
	PMIWeight:  0.10,
	LLMWeight:  0.05,
}

// CausalGraphStats 因果图谱统计信息
type CausalGraphStats struct {
	TotalNodes     int       `json:"total_nodes"`
	TotalRelations int       `json:"total_relations"`
	AvgConfidence  float64   `json:"avg_confidence"`
	MaxPathLength  int       `json:"max_path_length"`
	LastUpdated    time.Time `json:"last_updated"`
}
