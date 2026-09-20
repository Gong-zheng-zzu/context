package services

import (
	"fmt"
	"log"
	"math"
	"sync"

	"github.com/anush008/fastembed-go"
	"github.com/contextkeeper/service/internal/security"
)

// CASIA 可选 embedding 语义路径的环境变量开关。
//
//   - CASIA_EMBEDDING_SIMILARITY_ENABLED：默认 false。仅在显式置为 true 时，
//     才由 services 层向 CASIA 注入 FastEmbed 语义相似度实现；未启用时行为与
//     改动前完全一致（纯 strings.Contains）。
//   - CASIA_EMBEDDING_SIMILARITY_THRESHOLD：默认沿用 security.DefaultCASIASimilarityThreshold
//     （0.86），仅接受 (0,1] 区间内的值，越界回退默认。
const (
	CASIAEmbeddingSimilarityEnabledEnv   = "CASIA_EMBEDDING_SIMILARITY_ENABLED"
	CASIAEmbeddingSimilarityThresholdEnv = "CASIA_EMBEDDING_SIMILARITY_THRESHOLD"
)

// 该路径解决的问题：当上下文出现"同义词/改写表达"（例如未包含"病历"二字，
// 但语义上仍在描述就诊记录）时，纯子串匹配会漏检；启用后可用 embedding 语义
// 相似度召回，命中方式记为 "semantic"，与精确命中的 "exact" 可在证据中区分。
func init() {
	// 通过注册钩子反向注入：security 不能依赖 services，由 services 主动注册提供方。
	// 是否真正启用由提供方内部读取环境变量决定，默认关闭。
	security.RegisterCASIAEmbeddingSimilarityProvider(provideCASIAEmbeddingSimilarity)
}

// casiaTextEmbedder 抽象"把一批文本编码为向量"的能力，便于在不加载真实模型的情况下
// 对相似度封装与失败降级进行单元测试。
type casiaTextEmbedder interface {
	embedPassages(texts []string) ([][]float32, error)
	modelIdentifier() string
}

// provideCASIAEmbeddingSimilarity 是注册给 security 层的提供方。
//
// 返回 nil 函数表示不启用（保持默认行为）。模型/ONNX 运行时未就绪时不在此处报错——
// 错误会延迟到首次编码时以 ok=false 的形式向上传递，由 CASIA 降级为 strings.Contains。
func provideCASIAEmbeddingSimilarity() (security.ContextSimilarityFunc, float64, string) {
	if !getEnvAsBool(CASIAEmbeddingSimilarityEnabledEnv, false) {
		return nil, 0, ""
	}
	embedder := newFastEmbedPassageEmbedder()
	threshold := casiaEmbeddingSimilarityThreshold()
	log.Printf("🔤 [CASIA] embedding 语义路径已启用: model=%s, threshold=%.2f", embedder.modelIdentifier(), threshold)
	return newCASIASimilarityFunc(embedder), threshold, embedder.modelIdentifier()
}

// casiaEmbeddingSimilarityThreshold 解析阈值：默认 DefaultCASIASimilarityThreshold，
// 仅在 (0,1] 内取值有效，越界回退默认。
func casiaEmbeddingSimilarityThreshold() float64 {
	threshold := getEnvAsFloat(CASIAEmbeddingSimilarityThresholdEnv, security.DefaultCASIASimilarityThreshold)
	if threshold <= 0 || threshold > 1 {
		return security.DefaultCASIASimilarityThreshold
	}
	return threshold
}

// newCASIASimilarityFunc 把一个 embedder 封装为 CASIA 的 ContextSimilarityFunc。
//
// 失败语义：embedder 为空、编码失败、或返回结果数量不足时一律返回 ok=false，
// 绝不伪造相似度，从而让 CASIA 安全降级回 strings.Contains。
func newCASIASimilarityFunc(embedder casiaTextEmbedder) security.ContextSimilarityFunc {
	return func(text, keyword string) (float64, bool) {
		if embedder == nil {
			return 0, false
		}
		embeddings, err := embedder.embedPassages([]string{text, keyword})
		if err != nil || len(embeddings) < 2 {
			return 0, false
		}
		return casiaCosineSimilarity(embeddings[0], embeddings[1]), true
	}
}

// fastEmbedPassageEmbedder 是 casiaTextEmbedder 的生产实现：惰性创建并复用 FastEmbed 模型
// （复用项目既有的 fastembed-go 与 FastEmbedModelConfig 创建方式，不新增第三方依赖）。
//
// 选用中文模型 bge-small-zh-v1.5：CASIA 上下文关键词表为中文，中文模型能更好地捕捉
// 同义词/改写表达。模型文件缺失或 ONNX Runtime 不可用时，创建失败会被缓存，后续调用
// 稳定返回 ok=false（降级），不会反复触发下载。
//
// 说明：这里刻意不在每次调用后 Destroy 模型。CASIA 上下文匹配会对每个候选反复求解，
// 逐次加载模型的开销不可接受；模型实例在进程生命周期内复用。因该路径默认关闭，
// 对默认行为零影响。
type fastEmbedPassageEmbedder struct {
	config *FastEmbedModelConfig
	once   sync.Once
	mu     sync.Mutex
	model  *fastembed.FlagEmbedding
	err    error
}

func newFastEmbedPassageEmbedder() *fastEmbedPassageEmbedder {
	config := GetAlternativeConfigs()["bge_small_zh"]
	if config == nil {
		config = GetDefaultFastEmbedConfig()
	}
	return &fastEmbedPassageEmbedder{config: config}
}

func (e *fastEmbedPassageEmbedder) modelIdentifier() string {
	if e == nil || e.config == nil {
		return ""
	}
	return e.config.ModelName
}

func (e *fastEmbedPassageEmbedder) init() {
	e.once.Do(func() {
		model, err := fastembed.NewFlagEmbedding(e.config.ToInitOptions())
		if err != nil {
			e.err = err
			log.Printf("⚠️ [CASIA] FastEmbed 模型初始化失败，语义路径将降级为字符串匹配: %v", err)
			return
		}
		e.model = model
	})
}

func (e *fastEmbedPassageEmbedder) embedPassages(texts []string) ([][]float32, error) {
	if e == nil {
		return nil, fmt.Errorf("casia: nil embedder")
	}
	e.init()
	if e.err != nil {
		return nil, e.err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.model == nil {
		return nil, fmt.Errorf("casia: embedder not initialized")
	}
	return e.model.PassageEmbed(texts, len(texts))
}

// casiaCosineSimilarity 计算两个向量的余弦相似度，结果裁剪到 [0,1]。
func casiaCosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	similarity := dot / (math.Sqrt(normA) * math.Sqrt(normB))
	if similarity < 0 {
		return 0
	}
	if similarity > 1 {
		return 1
	}
	return similarity
}
