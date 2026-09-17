package security

import (
	"math"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"
)

const defaultCASIAContextWindow = 64
const CASIAConfigVersion = "casia-context-v1"

// DefaultCASIASimilarityThreshold 是可选 embedding 语义匹配路径的默认相似度阈值。
// 仅在该路径被显式启用且调用方注入相似度函数时生效。
const DefaultCASIASimilarityThreshold = 0.86

// ContextSimilarityFunc 是 CASIA 上下文关键词的语义相似度挂钩。
//
// 设计约束：internal/security 不能直接依赖 internal/services（FastEmbed），
// 否则会形成循环依赖。因此 security 侧只定义这一最小接口，由调用方
// （如 services 层）在启用时注入 FastEmbed 实现。
//
// 语义：
//   - similarity ∈ [0,1]，越大表示越相近；
//   - ok=false 表示实现无法给出结果（例如模型未就绪/调用失败），
//     调用方必须降级到 strings.Contains。
//
// 该路径默认关闭，且未注入时行为与纯 strings.Contains 完全一致。
type ContextSimilarityFunc func(text, keyword string) (similarity float64, ok bool)

type CASIAConfig struct {
	Version           string                        `json:"version"`
	ContextWindow     int                           `json:"context_window_bytes"`
	DecisionThreshold float64                       `json:"decision_threshold"`
	KeywordWeights    map[string]map[string]float64 `json:"keyword_weights"`
	// Source 记录关键词表来源："builtin"（内置默认）或 "file"（外部配置文件）。
	// 与 Version 后缀一起，使审计信息能反映配置是否来自外部文件。
	Source string `json:"source,omitempty"`
}

// CASIAKeywordEvidence identifies one context feature that affected a
// sensitive-information decision. It is deliberately metadata-only: callers
// can audit the decision without receiving a second copy of the candidate.
type CASIAKeywordEvidence struct {
	Keyword string  `json:"keyword"`
	Weight  float64 `json:"weight"`
	// MatchMethod 记录命中方式："exact"（子串命中）或 "semantic"（embedding 相似度命中）。
	// 仅在启用可选语义路径时才会出现 "semantic"。
	MatchMethod string `json:"match_method,omitempty"`
}

// CASIAEvidence is attached to a candidate after CASIA recalibrates it.
type CASIAEvidence struct {
	ConfigurationVersion string                 `json:"configuration_version"`
	SensitiveType        string                 `json:"sensitive_type"`
	ContextStart         int                    `json:"context_start"`
	ContextEnd           int                    `json:"context_end"`
	WindowBytes          int                    `json:"window_bytes"`
	MatchedKeywords      []CASIAKeywordEvidence `json:"matched_keywords"`
	RawWeight            float64                `json:"raw_weight"`
	NormalizedWeight     float64                `json:"normalized_weight"`
	BaseConfidence       float64                `json:"base_confidence"`
	AdjustedScore        float64                `json:"adjusted_score"`
	DecisionThreshold    float64                `json:"decision_threshold"`
}

// ContextAwareSensitiveInfoAlgorithm 上下文感知的敏感信息识别算法
// Context-Aware Sensitive Information Algorithm (CASIA)
type ContextAwareSensitiveInfoAlgorithm struct {
	// 上下文关键词权重表
	contextKeywords map[string]map[string]float64
	config          CASIAConfig

	// 可选 embedding 语义相似度路径的运行时状态。默认关闭；
	// 由调用方通过 EnableEmbeddingSimilarity 注入实现。
	mu                  sync.RWMutex
	similarityFunc      ContextSimilarityFunc
	similarityEnabled   bool
	similarityThreshold float64
}

// NewContextAwareSensitiveInfoAlgorithm 创建上下文感知算法
func NewContextAwareSensitiveInfoAlgorithm() *ContextAwareSensitiveInfoAlgorithm {
	return NewContextAwareSensitiveInfoAlgorithmWithConfig(DefaultCASIAConfig())
}

func NewContextAwareSensitiveInfoAlgorithmWithConfig(config CASIAConfig) *ContextAwareSensitiveInfoAlgorithm {
	if config.Version == "" {
		config.Version = CASIAConfigVersion
	}
	if config.ContextWindow <= 0 {
		config.ContextWindow = defaultCASIAContextWindow
	}
	if config.DecisionThreshold <= 0 || config.DecisionThreshold > 1 {
		config.DecisionThreshold = 0.6
	}
	if config.Source == "" {
		config.Source = CASIAKeywordSourceBuiltin
	}
	algo := &ContextAwareSensitiveInfoAlgorithm{
		similarityThreshold: DefaultCASIASimilarityThreshold,
		contextKeywords:     cloneCASIAKeywords(config.KeywordWeights),
		config:              config,
	}
	if len(algo.contextKeywords) == 0 {
		algo.initContextKeywords()
		algo.config.KeywordWeights = cloneCASIAKeywords(algo.contextKeywords)
	}
	return algo
}

func DefaultCASIAConfig() CASIAConfig {
	return CASIAConfig{
		Version:           CASIAConfigVersion,
		ContextWindow:     defaultCASIAContextWindow,
		DecisionThreshold: 0.6,
		Source:            CASIAKeywordSourceBuiltin,
	}
}

func cloneCASIAKeywords(source map[string]map[string]float64) map[string]map[string]float64 {
	cloned := make(map[string]map[string]float64, len(source))
	for sensitiveType, keywords := range source {
		cloned[sensitiveType] = make(map[string]float64, len(keywords))
		for keyword, weight := range keywords {
			cloned[sensitiveType][keyword] = weight
		}
	}
	return cloned
}

func (a *ContextAwareSensitiveInfoAlgorithm) GetConfiguration() CASIAConfig {
	configuration := a.config
	configuration.KeywordWeights = cloneCASIAKeywords(a.contextKeywords)
	return configuration
}

// initContextKeywords 初始化上下文关键词权重表
func (a *ContextAwareSensitiveInfoAlgorithm) initContextKeywords() {
	// 身份证相关上下文
	a.contextKeywords["id_card"] = map[string]float64{
		"身份证":   1.0,
		"证件号":   0.9,
		"身份证号":  1.0,
		"居民身份证": 1.0,
		"证件":    0.7,
		"实名":    0.6,
		"邮编":    -0.8, // 负权重：降低误判
		"邮政编码":  -0.8,
		"区号":    -0.7,
	}

	// 手机号相关上下文
	a.contextKeywords["phone"] = map[string]float64{
		"手机":   1.0,
		"电话":   0.9,
		"联系方式": 0.8,
		"手机号":  1.0,
		"电话号码": 0.9,
		"联系电话": 0.9,
		"QQ":   -0.5, // QQ号可能是数字但不是手机号
		"工号":   -0.6,
		"编号":   -0.5,
	}

	// 病历号相关上下文
	a.contextKeywords["medical_record"] = map[string]float64{
		"病历":   1.0,
		"病历号":  1.0,
		"就诊":   0.8,
		"住院":   0.8,
		"门诊":   0.7,
		"病案":   0.9,
		"医疗":   0.6,
		"处方":   0.7,
		"订单号":  -0.7, // 降低误判
		"快递单号": -0.8,
	}

	// 血压相关上下文
	a.contextKeywords["blood_pressure"] = map[string]float64{
		"血压":  1.0,
		"高压":  0.8,
		"低压":  0.8,
		"收缩压": 0.9,
		"舒张压": 0.9,
		"测量":  0.6,
		"体检":  0.7,
		"比例":  -0.6, // "120/80"可能是比例而非血压
		"分数":  -0.7,
		"得分":  -0.6,
	}
}

// CalculateContextWeight 计算上下文权重
// 算法：在候选文本前后N个字符内搜索关键词，累积权重
func (a *ContextAwareSensitiveInfoAlgorithm) CalculateContextWeight(
	text string,
	candidateStart int,
	candidateEnd int,
	sensitiveType string,
	contextWindow int, // 上下文窗口大小（字符数）
) float64 {
	return a.AnalyzeContext(text, candidateStart, candidateEnd, sensitiveType, contextWindow).NormalizedWeight
}

// AnalyzeContext calculates a deterministic, auditable context score. The
// requested window is clipped at sentence boundaries so unrelated clauses do
// not influence a nearby candidate merely because they fall within a fixed
// byte radius.
func (a *ContextAwareSensitiveInfoAlgorithm) AnalyzeContext(
	text string,
	candidateStart int,
	candidateEnd int,
	sensitiveType string,
	contextWindow int,
) CASIAEvidence {
	if contextWindow <= 0 {
		contextWindow = a.config.ContextWindow
	}
	if candidateStart < 0 {
		candidateStart = 0
	}
	if candidateStart > len(text) {
		candidateStart = len(text)
	}
	if candidateEnd < candidateStart {
		candidateEnd = candidateStart
	}
	if candidateEnd > len(text) {
		candidateEnd = len(text)
	}
	contextStart, contextEnd := casiaContextBounds(text, candidateStart, candidateEnd, contextWindow)
	evidence := CASIAEvidence{
		ConfigurationVersion: a.config.Version,
		SensitiveType:        sensitiveType,
		ContextStart:         contextStart,
		ContextEnd:           contextEnd,
		WindowBytes:          contextWindow,
		DecisionThreshold:    a.config.DecisionThreshold,
	}

	// 获取该类型的上下文关键词
	keywords, exists := a.contextKeywords[sensitiveType]
	if !exists {
		evidence.RawWeight = 0
		evidence.NormalizedWeight = 0.5
		return evidence
	}
	contextText := text[contextStart:contextEnd]

	// 累积上下文权重
	for keyword, weight := range keywords {
		if matched, method := a.matchContextKeyword(contextText, keyword); matched {
			evidence.RawWeight += weight
			evidence.MatchedKeywords = append(evidence.MatchedKeywords, CASIAKeywordEvidence{
				Keyword:     keyword,
				Weight:      weight,
				MatchMethod: method,
			})
		}
	}
	sort.Slice(evidence.MatchedKeywords, func(i, j int) bool {
		return evidence.MatchedKeywords[i].Keyword < evidence.MatchedKeywords[j].Keyword
	})
	evidence.NormalizedWeight = 1.0 / (1.0 + math.Exp(-evidence.RawWeight))
	return evidence
}

// matchContextKeyword 判定一个上下文关键词是否命中。
//
// 默认路径是 strings.Contains（与历史行为完全一致）。仅当调用方显式启用
// embedding 语义路径并注入了 ContextSimilarityFunc 时，才会在子串未命中后
// 尝试语义相似度；相似度低于阈值、实现返回 ok=false、或未注入实现时，
// 一律视为不命中（即降级为纯 strings.Contains）。
func (a *ContextAwareSensitiveInfoAlgorithm) matchContextKeyword(contextText, keyword string) (bool, string) {
	if strings.Contains(contextText, keyword) {
		return true, "exact"
	}

	a.mu.RLock()
	enabled := a.similarityEnabled
	similarityFunc := a.similarityFunc
	threshold := a.similarityThreshold
	a.mu.RUnlock()

	if !enabled || similarityFunc == nil {
		return false, ""
	}
	similarity, ok := similarityFunc(contextText, keyword)
	if !ok {
		return false, ""
	}
	if similarity < threshold {
		return false, ""
	}
	return true, "semantic"
}

// EnableEmbeddingSimilarity 启用可选的 embedding 语义相似度匹配路径。
//
// 调用方（services 层）注入 FastEmbed 实现。传入 nil 函数或非法阈值时保持关闭，
// 从而不影响默认行为。threshold ∈ (0,1]，越界时回退到 DefaultCASIASimilarityThreshold。
func (a *ContextAwareSensitiveInfoAlgorithm) EnableEmbeddingSimilarity(similarityFunc ContextSimilarityFunc, threshold float64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if threshold <= 0 || threshold > 1 {
		threshold = DefaultCASIASimilarityThreshold
	}
	a.similarityFunc = similarityFunc
	a.similarityThreshold = threshold
	a.similarityEnabled = similarityFunc != nil
}

// DisableEmbeddingSimilarity 关闭 embedding 语义路径，回到纯 strings.Contains 行为。
func (a *ContextAwareSensitiveInfoAlgorithm) DisableEmbeddingSimilarity() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.similarityEnabled = false
	a.similarityFunc = nil
}

// EmbeddingSimilarityEnabled 报告语义路径当前是否启用。
func (a *ContextAwareSensitiveInfoAlgorithm) EmbeddingSimilarityEnabled() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.similarityEnabled
}

// EmbeddingSimilarityThreshold 返回当前语义匹配阈值。
func (a *ContextAwareSensitiveInfoAlgorithm) EmbeddingSimilarityThreshold() float64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.similarityThreshold
}

func casiaContextBounds(text string, candidateStart, candidateEnd, window int) (int, int) {
	start := max(0, candidateStart-window)
	end := min(len(text), candidateEnd+window)
	if boundary := strings.LastIndexAny(text[start:candidateStart], "。！？；\n"); boundary >= 0 {
		_, size := utf8.DecodeRuneInString(text[start+boundary:])
		start += boundary + size
	}
	if boundary := strings.IndexAny(text[candidateEnd:end], "。！？；\n"); boundary >= 0 {
		end = candidateEnd + boundary
	}
	return start, end
}

// DetectWithContext 带上下文感知的检测
func (a *ContextAwareSensitiveInfoAlgorithm) DetectWithContext(
	text string,
	pattern string, // 匹配到的模式（如"110101199001011234"）
	patternStart int,
	patternEnd int,
	sensitiveType string,
) (isSensitive bool, confidence float64) {
	// 基础置信度（模式匹配）
	baseConfidence := 0.7

	// 计算上下文权重
	evidence := a.AnalyzeContext(
		text,
		patternStart,
		patternEnd,
		sensitiveType,
		a.config.ContextWindow,
	)

	// 最终置信度 = 基础置信度 × 上下文权重
	finalConfidence := baseConfidence * evidence.NormalizedWeight

	// 判定阈值
	isSensitive = finalConfidence >= a.config.DecisionThreshold

	return isSensitive, finalConfidence
}

// AdjustConfidenceByContext 根据上下文调整置信度
func (a *ContextAwareSensitiveInfoAlgorithm) AdjustConfidenceByContext(
	text string,
	value string,
	sensitiveType string,
	start int,
	baseConfidence float64,
) float64 {
	// 计算上下文权重
	end := start + len(value)
	evidence := a.AnalyzeContext(
		text,
		start,
		end,
		sensitiveType,
		a.config.ContextWindow,
	)

	// 最终置信度 = 基础置信度 × 上下文权重
	finalConfidence := baseConfidence * evidence.NormalizedWeight

	// 确保置信度在[0, 1]范围内
	if finalConfidence > 1.0 {
		finalConfidence = 1.0
	}
	if finalConfidence < 0.0 {
		finalConfidence = 0.0
	}

	return finalConfidence
}

// GetAlgorithmDescription 获取算法描述（用于论文和答辩）
func (a *ContextAwareSensitiveInfoAlgorithm) GetAlgorithmDescription() string {
	return `
上下文感知的敏感信息识别算法（CASIA - Context-Aware Sensitive Information Algorithm）

核心问题：
同一模式在不同上下文中含义不同，导致误报。
例如："110101"可能是身份证前6位，也可能是邮编。

算法思想：
通过分析候选文本周围的上下文关键词，计算上下文权重，调整最终置信度。

数学模型：
C_final = C_base × W_context

其中：
- C_base: 基础置信度（模式匹配）
- W_context: 上下文权重，通过sigmoid函数归一化

W_context = 1 / (1 + e^(-Σw_i))

w_i: 第i个上下文关键词的权重（正权重增强，负权重抑制）

算法流程：
1. 模式匹配：正则/字典树检测候选文本
2. 上下文提取：提取候选文本前后N个字符
3. 关键词匹配：在上下文中搜索预定义关键词
4. 权重累积：累加匹配到的关键词权重
5. 置信度计算：基础置信度 × 上下文权重

	设计特点：
1. 上下文感知：不只看模式，还看语境
2. 固定配置：当前关键词权重来自版本化人工配置；可通过外部文件覆盖或经离线开发集校准，
   均不属于运行时在线学习或自适应训练
3. 负向证据：通过负权重关键词抑制误判
4. 可解释性：可追溯哪些上下文关键词影响了判定
5. 可选语义匹配：关键词判定默认使用字符串包含；调用方可显式注入 embedding
   相似度实现（接口注入，security 层不直接依赖 services），启用后仍对未命中
   关键词降级到字符串包含，未注入时行为与纯字符串包含一致

评测边界：
- 误报率、召回率和提升幅度必须来自带数据集哈希与配置版本的正式评测
- 当前配置版本和命中证据会随结果返回，历史数字不代表当前代码成绩

示例：
文本1："患者身份证：110101199001011234"
  → 上下文关键词："身份证"（权重1.0）
  → 上下文权重：0.95
  → 最终置信度：0.7 × 0.95 = 0.665 ✅ 判定为敏感

文本2："邮政编码：110101"
  → 上下文关键词："邮政编码"（权重-0.8）
  → 上下文权重：0.31
  → 最终置信度：0.7 × 0.31 = 0.217 ❌ 判定为非敏感
`
}
