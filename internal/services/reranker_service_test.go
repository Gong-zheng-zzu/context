package services

import (
	"context"
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/contextkeeper/service/internal/llm"
	"github.com/contextkeeper/service/internal/models"
)

// stubRerankerLLMClient 用于验证 LLM 重排序分支的桩客户端（calls 用原子操作，支持并发测试）
type stubRerankerLLMClient struct {
	response *llm.LLMResponse
	err      error
	calls    int64
}

func (s *stubRerankerLLMClient) Complete(ctx context.Context, req *llm.LLMRequest) (*llm.LLMResponse, error) {
	atomic.AddInt64(&s.calls, 1)
	return s.response, s.err
}

func (s *stubRerankerLLMClient) BatchComplete(ctx context.Context, reqs []*llm.LLMRequest) ([]*llm.LLMResponse, error) {
	return nil, nil
}

func (s *stubRerankerLLMClient) StreamComplete(ctx context.Context, req *llm.LLMRequest) (<-chan *llm.LLMStreamResponse, error) {
	return nil, nil
}

func (s *stubRerankerLLMClient) HealthCheck(ctx context.Context) error { return nil }

func (s *stubRerankerLLMClient) GetProvider() llm.LLMProvider { return llm.ProviderDeepSeek }

func (s *stubRerankerLLMClient) GetModel() string { return "stub-model" }

func (s *stubRerankerLLMClient) GetCapabilities() *llm.LLMCapabilities {
	return &llm.LLMCapabilities{}
}

func (s *stubRerankerLLMClient) Close() error { return nil }

// rerankerTestResults 构造两个结果：low(语义0.2) 在前、high(语义1.0) 在后。
// 规则打分下 high 分数更高，因此未经 LLM 重排序时 high 应排在首位。
func rerankerTestResults() []models.SearchResult {
	return []models.SearchResult{
		{ID: "low", Score: 0.2, Fields: map[string]interface{}{"content": "alpha memo"}},
		{ID: "high", Score: 1.0, Fields: map[string]interface{}{"content": "beta memo"}},
	}
}

func rerankerResultIDs(results []models.SearchResult) []string {
	ids := make([]string, 0, len(results))
	for _, result := range results {
		ids = append(ids, result.ID)
	}
	return ids
}

// TestRerankDefaultConfigDoesNotInvokeLLM 默认配置（UseLLMRerank=false）必须与改动前一致：不调用LLM
func TestRerankDefaultConfigDoesNotInvokeLLM(t *testing.T) {
	stub := &stubRerankerLLMClient{err: errors.New("默认配置不应调用LLM")}
	service := NewRerankerService(stub)

	results, err := service.Rerank(context.Background(), "alpha", rerankerTestResults(), DefaultRerankConfig)
	if err != nil {
		t.Fatalf("重排序不应返回错误: %v", err)
	}
	if calls := atomic.LoadInt64(&stub.calls); calls != 0 {
		t.Errorf("默认配置下不应调用LLM，实际调用 %d 次", calls)
	}
	ids := rerankerResultIDs(results)
	if len(ids) != 2 || ids[0] != "high" || ids[1] != "low" {
		t.Errorf("默认规则排序错误，实际顺序: %v", ids)
	}
}

// TestRerankLLMFusionReorders 启用LLM重排序后，LLM分数按融合权重影响最终排序
func TestRerankLLMFusionReorders(t *testing.T) {
	// LLM 判定候选顺序 [low, high] 的相关性为 [0.9, 0.1]，与规则排序相反
	stub := &stubRerankerLLMClient{response: &llm.LLMResponse{Content: `{"scores":[0.9,0.1]}`}}
	service := NewRerankerService(stub)

	config := DefaultRerankConfig
	config.UseLLMRerank = true // 融合权重取默认 0.6

	results, err := service.Rerank(context.Background(), "alpha", rerankerTestResults(), config)
	if err != nil {
		t.Fatalf("重排序不应返回错误: %v", err)
	}
	if calls := atomic.LoadInt64(&stub.calls); calls != 1 {
		t.Errorf("启用LLM重排序应调用LLM 1 次，实际 %d 次", calls)
	}
	ids := rerankerResultIDs(results)
	if len(ids) != 2 || ids[0] != "low" {
		t.Fatalf("LLM判定 low 更相关，low 应升到首位，实际顺序: %v", ids)
	}
	// 规则分 min-max 归一化后 low 为最小值(0.0)，故 low 融合分 = 0.4*0 + 0.6*0.9 = 0.54
	wantLow := defaultLLMFusionWeight * 0.9
	if diff := math.Abs(results[0].Score - wantLow); diff > 1e-9 {
		t.Errorf("low 融合分数 = %v, 期望 %v", results[0].Score, wantLow)
	}
}

// TestRerankLLMFusionNormalizesRuleScores 规则分经提升后可能>1，融合前必须归一化到[0,1]，
// 否则规则分与[0,1]的LLM分不同量纲，会稀释LLM的配置权重。
func TestRerankLLMFusionNormalizesRuleScores(t *testing.T) {
	stub := &stubRerankerLLMClient{response: &llm.LLMResponse{Content: `{"scores":[0.9,0.1]}`}}
	service := NewRerankerService(stub)

	// 直接构造规则分：boosted 因答案/实体提升达到 2.0 (>1)
	scored := []ScoredResult{
		{Result: models.SearchResult{ID: "boosted"}, FinalScore: 2.0},
		{Result: models.SearchResult{ID: "plain"}, FinalScore: 0.5},
	}
	config := DefaultRerankConfig
	config.UseLLMRerank = true
	config.LLMFusionWeight = 0.6

	service.applyLLMRerank(context.Background(), "测试问题", scored, config)

	// 规则分 min-max: boosted=1.0, plain=0.0；融合 = 0.4*归一化规则分 + 0.6*LLM分
	wantBoosted := 0.4*1.0 + 0.6*0.9 // = 0.94
	wantPlain := 0.4*0.0 + 0.6*0.1   // = 0.06
	if diff := math.Abs(scored[0].FinalScore - wantBoosted); diff > 1e-9 {
		t.Errorf("boosted 融合分数 = %v, 期望 %v（规则分>1 不得稀释LLM权重）", scored[0].FinalScore, wantBoosted)
	}
	if diff := math.Abs(scored[1].FinalScore - wantPlain); diff > 1e-9 {
		t.Errorf("plain 融合分数 = %v, 期望 %v", scored[1].FinalScore, wantPlain)
	}
}

// TestRerankLazyFactoryNotTriggeredBeforeUse 校验惰性工厂：构造期与未启用时都不得触发客户端创建
func TestRerankLazyFactoryNotTriggeredBeforeUse(t *testing.T) {
	factoryCalls := 0
	service := NewRerankerServiceWithFactory(func() llm.LLMClient {
		factoryCalls++
		return &stubRerankerLLMClient{response: &llm.LLMResponse{Content: `{"scores":[0.9,0.1]}`}}
	})

	if factoryCalls != 0 {
		t.Fatalf("构造期不得触发工厂，实际调用 %d 次", factoryCalls)
	}

	// UseLLMRerank=false：即使调用 Rerank 也不应触发工厂
	if _, err := service.Rerank(context.Background(), "alpha", rerankerTestResults(), DefaultRerankConfig); err != nil {
		t.Fatalf("重排序不应返回错误: %v", err)
	}
	if factoryCalls != 0 {
		t.Errorf("UseLLMRerank=false 时不得触发工厂，实际调用 %d 次", factoryCalls)
	}

	// 首次启用并调用 Rerank 时才触发，且只创建一次
	config := DefaultRerankConfig
	config.UseLLMRerank = true
	if _, err := service.Rerank(context.Background(), "alpha", rerankerTestResults(), config); err != nil {
		t.Fatalf("重排序不应返回错误: %v", err)
	}
	if factoryCalls != 1 {
		t.Fatalf("首次启用应触发工厂 1 次，实际 %d 次", factoryCalls)
	}
	if _, err := service.Rerank(context.Background(), "alpha", rerankerTestResults(), config); err != nil {
		t.Fatalf("重排序不应返回错误: %v", err)
	}
	if factoryCalls != 1 {
		t.Errorf("工厂结果应被缓存复用，实际调用 %d 次", factoryCalls)
	}
}

// TestRerankLazyFactoryNilFallback 工厂返回nil时降级为纯规则打分，不 panic
func TestRerankLazyFactoryNilFallback(t *testing.T) {
	service := NewRerankerServiceWithFactory(func() llm.LLMClient { return nil })

	config := DefaultRerankConfig
	config.UseLLMRerank = true

	results, err := service.Rerank(context.Background(), "alpha", rerankerTestResults(), config)
	if err != nil {
		t.Fatalf("工厂返回nil应降级而非返回错误: %v", err)
	}
	ids := rerankerResultIDs(results)
	if len(ids) != 2 || ids[0] != "high" || ids[1] != "low" {
		t.Errorf("降级后应保持规则排序，实际顺序: %v", ids)
	}
}

// TestRerankLLMDegradesOnError LLM调用失败时降级为纯规则打分，顺序与默认一致
func TestRerankLLMDegradesOnError(t *testing.T) {
	stub := &stubRerankerLLMClient{err: errors.New("模拟LLM故障")}
	service := NewRerankerService(stub)

	config := DefaultRerankConfig
	config.UseLLMRerank = true

	results, err := service.Rerank(context.Background(), "alpha", rerankerTestResults(), config)
	if err != nil {
		t.Fatalf("LLM失败应降级而非返回错误: %v", err)
	}
	ids := rerankerResultIDs(results)
	if len(ids) != 2 || ids[0] != "high" || ids[1] != "low" {
		t.Errorf("降级后应保持规则排序，实际顺序: %v", ids)
	}
}

// TestRerankLLMDegradesOnMalformedResponse LLM返回无法解析的内容时降级为纯规则打分
func TestRerankLLMDegradesOnMalformedResponse(t *testing.T) {
	stub := &stubRerankerLLMClient{response: &llm.LLMResponse{Content: "not-a-json"}}
	service := NewRerankerService(stub)

	config := DefaultRerankConfig
	config.UseLLMRerank = true

	results, err := service.Rerank(context.Background(), "alpha", rerankerTestResults(), config)
	if err != nil {
		t.Fatalf("解析失败应降级而非返回错误: %v", err)
	}
	ids := rerankerResultIDs(results)
	if len(ids) != 2 || ids[0] != "high" || ids[1] != "low" {
		t.Errorf("降级后应保持规则排序，实际顺序: %v", ids)
	}
}

// TestRerankLLMDegradesWithoutClient 启用LLM重排序但客户端为nil时不 panic，降级为规则排序
func TestRerankLLMDegradesWithoutClient(t *testing.T) {
	service := NewRerankerService(nil)

	config := DefaultRerankConfig
	config.UseLLMRerank = true

	results, err := service.Rerank(context.Background(), "alpha", rerankerTestResults(), config)
	if err != nil {
		t.Fatalf("客户端缺失应降级而非返回错误: %v", err)
	}
	ids := rerankerResultIDs(results)
	if len(ids) != 2 || ids[0] != "high" || ids[1] != "low" {
		t.Errorf("降级后应保持规则排序，实际顺序: %v", ids)
	}
}

// TestRerankLazyFactoryConcurrent 并发调用 Rerank 时工厂只应被触发一次（配合 -race 验证）
func TestRerankLazyFactoryConcurrent(t *testing.T) {
	var factoryCalls int64
	service := NewRerankerServiceWithFactory(func() llm.LLMClient {
		atomic.AddInt64(&factoryCalls, 1)
		return &stubRerankerLLMClient{response: &llm.LLMResponse{Content: `{"scores":[0.9,0.1]}`}}
	})

	config := DefaultRerankConfig
	config.UseLLMRerank = true

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := service.Rerank(context.Background(), "alpha", rerankerTestResults(), config); err != nil {
				t.Errorf("并发重排序不应返回错误: %v", err)
			}
		}()
	}
	wg.Wait()

	if calls := atomic.LoadInt64(&factoryCalls); calls != 1 {
		t.Errorf("并发下工厂应只被触发 1 次，实际 %d 次", calls)
	}
}
