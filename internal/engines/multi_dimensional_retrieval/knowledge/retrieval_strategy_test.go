package knowledge

import (
	"testing"
)

func TestSelectRetrievalStrategy(t *testing.T) {
	tests := []struct {
		query    string
		expected RetrievalStrategy
	}{
		{"昨天讨论的Redis问题", StrategyTimeRecall},
		{"上周的数据库设计方案", StrategyTimeRecall},
		{"我们的API接口设计", StrategyGraphPriority},
		{"项目的架构是什么", StrategyGraphPriority},
		{"简单问题", StrategyVectorOnly},
		{"hello", StrategyVectorOnly},
	}

	for _, test := range tests {
		analysis := AnalyzeQuery(test.query)
		strategy := SelectRetrievalStrategy(test.query, analysis)
		if strategy != test.expected {
			t.Errorf("SelectRetrievalStrategy(%s) = %s, 期望 %s", 
				test.query, strategy, test.expected)
		}
	}
}

func TestAnalyzeQuery(t *testing.T) {
	// 测试时间引用检测
	analysis := AnalyzeQuery("昨天我们讨论了什么")
	if !analysis.HasTimeReference {
		t.Error("应该检测到时间引用")
	}
	if len(analysis.TimeKeywords) == 0 {
		t.Error("应该有时间关键词")
	}

	// 测试实体检测
	analysis = AnalyzeQuery("数据库的架构设计")
	if !analysis.HasEntityMention {
		t.Error("应该检测到实体提及")
	}
	if len(analysis.EntityKeywords) == 0 {
		t.Error("应该有实体关键词")
	}

	// 测试查询复杂度
	analysis = AnalyzeQuery("hi")
	if analysis.QueryComplexity != "simple" {
		t.Errorf("简单查询应该是simple, got: %s", analysis.QueryComplexity)
	}

	analysis = AnalyzeQuery("请告诉我昨天讨论的关于数据库架构设计和Redis缓存优化的问题")
	if analysis.QueryComplexity != "complex" {
		t.Errorf("复杂查询应该是complex, got: %s", analysis.QueryComplexity)
	}
}

func TestFuseResultsWithRRF(t *testing.T) {
	// 创建测试数据
	vectorResults := []RetrievalResult{
		{MemoryID: "m1", Score: 0.9, Source: "vector"},
		{MemoryID: "m2", Score: 0.8, Source: "vector"},
		{MemoryID: "m3", Score: 0.7, Source: "vector"},
	}

	graphResults := []RetrievalResult{
		{MemoryID: "m2", Score: 0.95, Source: "graph"}, // 与向量结果重叠
		{MemoryID: "m4", Score: 0.85, Source: "graph"},
	}

	timeResults := []RetrievalResult{
		{MemoryID: "m1", Score: 0.88, Source: "time"}, // 与向量结果重叠
		{MemoryID: "m5", Score: 0.75, Source: "time"},
	}

	resultSets := [][]RetrievalResult{vectorResults, graphResults, timeResults}
	fused := FuseResultsWithRRF(resultSets, nil)

	// 验证结果数量 (5个唯一的MemoryID)
	if len(fused) != 5 {
		t.Errorf("期望5个融合结果, 实际: %d", len(fused))
	}

	// 验证重叠的结果获得更高RRF分数
	// m1和m2出现在多个结果集中，应该排在前面
	foundM1 := false
	foundM2 := false
	for i, result := range fused {
		if result.MemoryID == "m1" {
			foundM1 = true
			if i > 2 {
				t.Errorf("m1应该在前3名, 实际排名: %d", i+1)
			}
		}
		if result.MemoryID == "m2" {
			foundM2 = true
			if i > 2 {
				t.Errorf("m2应该在前3名, 实际排名: %d", i+1)
			}
		}
	}

	if !foundM1 {
		t.Error("m1应该在结果中")
	}
	if !foundM2 {
		t.Error("m2应该在结果中")
	}
}

func TestDefaultRRFConfig(t *testing.T) {
	config := DefaultRRFConfig()

	if config.K != 60.0 {
		t.Errorf("默认K值应该是60, 实际: %f", config.K)
	}

	if config.SourceWeight["vector"] != 1.0 {
		t.Errorf("vector权重应该是1.0, 实际: %f", config.SourceWeight["vector"])
	}

	if config.SourceWeight["graph"] != 1.2 {
		t.Errorf("graph权重应该是1.2, 实际: %f", config.SourceWeight["graph"])
	}

	if config.SourceWeight["time"] != 0.8 {
		t.Errorf("time权重应该是0.8, 实际: %f", config.SourceWeight["time"])
	}
}

