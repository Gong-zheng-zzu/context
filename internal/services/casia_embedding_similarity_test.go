package services

import (
	"errors"
	"testing"

	"github.com/contextkeeper/service/internal/security"
)

// casiaStubEmbedder 是可注入的假 embedder，避免测试依赖真实模型下载/ONNX 运行时。
type casiaStubEmbedder struct {
	embeddings [][]float32
	err        error
	model      string
}

func (s *casiaStubEmbedder) embedPassages(_ []string) ([][]float32, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.embeddings, nil
}

func (s *casiaStubEmbedder) modelIdentifier() string { return s.model }

func TestCASIAEmbeddingProviderDisabledByDefault(t *testing.T) {
	t.Setenv(CASIAEmbeddingSimilarityEnabledEnv, "")
	fn, _, model := provideCASIAEmbeddingSimilarity()
	if fn != nil {
		t.Fatal("provider must not enable the semantic path when the env var is unset")
	}
	if model != "" {
		t.Fatalf("model = %q, want empty when disabled", model)
	}
}

func TestCASIAEmbeddingProviderRespectsExplicitFalse(t *testing.T) {
	t.Setenv(CASIAEmbeddingSimilarityEnabledEnv, "false")
	if fn, _, _ := provideCASIAEmbeddingSimilarity(); fn != nil {
		t.Fatal("provider must stay disabled when the env var is false")
	}
}

func TestCASIAEmbeddingProviderEnabledReturnsFuncAndDefaults(t *testing.T) {
	t.Setenv(CASIAEmbeddingSimilarityEnabledEnv, "true")
	t.Setenv(CASIAEmbeddingSimilarityThresholdEnv, "")
	fn, threshold, model := provideCASIAEmbeddingSimilarity()
	if fn == nil {
		t.Fatal("expected a non-nil similarity func when enabled")
	}
	if threshold != security.DefaultCASIASimilarityThreshold {
		t.Fatalf("threshold = %v, want default %v", threshold, security.DefaultCASIASimilarityThreshold)
	}
	if model == "" {
		t.Fatal("expected a model identifier for audit")
	}
}

func TestCASIAEmbeddingThresholdOverrideAndClamp(t *testing.T) {
	t.Setenv(CASIAEmbeddingSimilarityEnabledEnv, "true")

	t.Setenv(CASIAEmbeddingSimilarityThresholdEnv, "0.75")
	if _, threshold, _ := provideCASIAEmbeddingSimilarity(); threshold != 0.75 {
		t.Fatalf("threshold = %v, want 0.75", threshold)
	}

	t.Setenv(CASIAEmbeddingSimilarityThresholdEnv, "2")
	if _, threshold, _ := provideCASIAEmbeddingSimilarity(); threshold != security.DefaultCASIASimilarityThreshold {
		t.Fatalf("out-of-range threshold must fall back to default, got %v", threshold)
	}
}

// 编码失败时返回 ok=false，绝不伪造相似度：CASIA 会降级到 strings.Contains。
func TestCASIASimilarityFuncFailsClosedWithoutFaking(t *testing.T) {
	fn := newCASIASimilarityFunc(&casiaStubEmbedder{err: errors.New("onnx unavailable")})
	if similarity, ok := fn("既往就医记录如下", "病历"); ok {
		t.Fatalf("expected ok=false on embedder failure, got similarity=%v", similarity)
	}
}

func TestCASIASimilarityFuncFailsClosedOnShortResult(t *testing.T) {
	fn := newCASIASimilarityFunc(&casiaStubEmbedder{embeddings: [][]float32{{1, 0}}})
	if _, ok := fn("text", "kw"); ok {
		t.Fatal("expected ok=false when embedder returns fewer than two vectors")
	}
}

func TestCASIASimilarityFuncNilEmbedderFailsClosed(t *testing.T) {
	fn := newCASIASimilarityFunc(nil)
	if _, ok := fn("text", "kw"); ok {
		t.Fatal("nil embedder must fail closed")
	}
}

func TestCASIASimilarityFuncComputesCosine(t *testing.T) {
	fn := newCASIASimilarityFunc(&casiaStubEmbedder{embeddings: [][]float32{{1, 0}, {1, 0}}})
	similarity, ok := fn("a", "b")
	if !ok {
		t.Fatal("expected ok=true for identical vectors")
	}
	if similarity < 0.999 || similarity > 1.001 {
		t.Fatalf("similarity = %v, want ~1.0", similarity)
	}
}

func TestCASIAEmbeddingProviderRegistersHook(t *testing.T) {
	// init() 已注册提供方；未启用环境变量时它必须返回 nil，从而不改变默认行为。
	t.Setenv(CASIAEmbeddingSimilarityEnabledEnv, "")
	fn, _, _ := provideCASIAEmbeddingSimilarity()
	if fn != nil {
		t.Fatal("registered provider must stay disabled unless explicitly enabled")
	}
}
