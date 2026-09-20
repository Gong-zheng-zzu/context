package services

import (
	"math"
	"testing"

	"github.com/contextkeeper/service/internal/models"
)

// TestRRFFusionFormula 校验加权RRF公式：score = Σ weight[source] / (K + rank)
func TestRRFFusionFormula(t *testing.T) {
	candidates := []SourcedCandidate{
		{DocID: "docA", Content: "a", Score: 0.9, Source: "vector"},
		{DocID: "docB", Content: "b", Score: 0.5, Source: "vector"},
		{DocID: "docA", Content: "a-k", Score: 0.7, Source: "knowledge"},
	}

	fused, audit := Fuse(candidates, DefaultRRFConfig())
	if len(fused) != 2 {
		t.Fatalf("期望 2 个融合候选，实际 %d", len(fused))
	}

	// docA 命中 vector(rank1) 与 knowledge(rank1)；docB 仅命中 vector(rank2)
	wantA := 1.0/(defaultRRFK+1) + 1.2/(defaultRRFK+1)
	wantB := 1.0 / (defaultRRFK + 2)

	if fused[0].DocID != "docA" {
		t.Fatalf("期望 docA 排第一，实际 %s", fused[0].DocID)
	}
	if diff := math.Abs(fused[0].Score - wantA); diff > 1e-9 {
		t.Errorf("docA 分数 = %v, 期望 %v", fused[0].Score, wantA)
	}
	if diff := math.Abs(fused[1].Score - wantB); diff > 1e-9 {
		t.Errorf("docB 分数 = %v, 期望 %v", fused[1].Score, wantB)
	}

	if fused[0].SourceRanks["vector"] != 1 || fused[0].SourceRanks["knowledge"] != 1 {
		t.Errorf("docA 来源名次错误: %v", fused[0].SourceRanks)
	}
	if fused[1].SourceRanks["vector"] != 2 {
		t.Errorf("docB 来源名次错误: %v", fused[1].SourceRanks)
	}

	if audit.FusionMode != "rrf_2_sources" {
		t.Errorf("FusionMode = %s, 期望 rrf_2_sources", audit.FusionMode)
	}
	if audit.SourceCandidateCounts["vector"] != 2 ||
		audit.SourceCandidateCounts["knowledge"] != 1 ||
		audit.SourceCandidateCounts["timeline"] != 0 {
		t.Errorf("SourceCandidateCounts 错误: %v", audit.SourceCandidateCounts)
	}
}

// TestRRFFusionSourceWeight 校验来源权重生效：同一 rank 下 knowledge(1.2) 贡献大于 vector(1.0)
func TestRRFFusionSourceWeight(t *testing.T) {
	candidates := []SourcedCandidate{
		{DocID: "vectorDoc", Content: "v", Score: 0.5, Source: "vector"},
		{DocID: "knowledgeDoc", Content: "k", Score: 0.5, Source: "knowledge"},
	}

	fused, _ := Fuse(candidates, DefaultRRFConfig())
	if len(fused) != 2 {
		t.Fatalf("期望 2 个融合候选，实际 %d", len(fused))
	}
	if fused[0].DocID != "knowledgeDoc" {
		t.Fatalf("同 rank 下 knowledge 权重更大，应排首位，实际 %s", fused[0].DocID)
	}

	wantKnowledge := 1.2 / (defaultRRFK + 1)
	wantVector := 1.0 / (defaultRRFK + 1)
	if diff := math.Abs(fused[0].Score - wantKnowledge); diff > 1e-9 {
		t.Errorf("knowledge 分数 = %v, 期望 %v", fused[0].Score, wantKnowledge)
	}
	if diff := math.Abs(fused[1].Score - wantVector); diff > 1e-9 {
		t.Errorf("vector 分数 = %v, 期望 %v", fused[1].Score, wantVector)
	}
}

// TestRRFFusionDedupKeepsHighest 校验同源同 DocID 去重且保留原始最高分
func TestRRFFusionDedupKeepsHighest(t *testing.T) {
	candidates := []SourcedCandidate{
		{DocID: "dup", Content: "low", Score: 0.2, Source: "vector"},
		{DocID: "dup", Content: "high", Score: 0.9, Source: "vector"},
		{DocID: "other", Content: "o", Score: 0.5, Source: "vector"},
	}

	fused, audit := Fuse(candidates, DefaultRRFConfig())
	if len(fused) != 2 {
		t.Fatalf("同源同 DocID 应去重，期望 2 个候选，实际 %d", len(fused))
	}
	if fused[0].DocID != "dup" {
		t.Fatalf("保留最高分(0.9)的 dup 应排第一，实际 %s", fused[0].DocID)
	}
	// 若去重保留了较低分(0.2)，other(0.5) 的 rank 会反超 dup
	if fused[0].SourceRanks["vector"] != 1 {
		t.Errorf("dup 的 vector 名次 = %d, 期望 1", fused[0].SourceRanks["vector"])
	}
	if audit.SourceCandidateCounts["vector"] != 2 {
		t.Errorf("去重后 vector 候选数 = %d, 期望 2", audit.SourceCandidateCounts["vector"])
	}
}

// TestRRFFusionExclusions 校验未知来源与空 DocID 被排除
func TestRRFFusionExclusions(t *testing.T) {
	candidates := []SourcedCandidate{
		{DocID: "", Content: "empty-id", Score: 1.0, Source: "vector"},
		{DocID: "ghost", Content: "unknown-source", Score: 1.0, Source: "unknown_source"},
	}

	fused, audit := Fuse(candidates, DefaultRRFConfig())
	if len(fused) != 0 {
		t.Fatalf("空 DocID 与未知来源应被排除，期望 0 个候选，实际 %d", len(fused))
	}
	if audit.FusionMode != "no_evidence" {
		t.Errorf("FusionMode = %s, 期望 no_evidence", audit.FusionMode)
	}
}

// TestRRFFusionModes 校验 FusionMode 判定与来源审计
func TestRRFFusionModes(t *testing.T) {
	t.Run("单来源回退", func(t *testing.T) {
		_, audit := Fuse([]SourcedCandidate{{DocID: "v1", Score: 0.9, Source: "vector"}}, DefaultRRFConfig())
		if audit.FusionMode != "vector_only_fallback" {
			t.Errorf("FusionMode = %s, 期望 vector_only_fallback", audit.FusionMode)
		}
		if len(audit.ActiveSources) != 1 || audit.ActiveSources[0] != "vector" {
			t.Errorf("ActiveSources = %v, 期望 [vector]", audit.ActiveSources)
		}
		if len(audit.EmptySources) != 2 {
			t.Errorf("EmptySources = %v, 期望包含 knowledge/timeline", audit.EmptySources)
		}
	})

	t.Run("无证据", func(t *testing.T) {
		_, audit := Fuse(nil, DefaultRRFConfig())
		if audit.FusionMode != "no_evidence" {
			t.Errorf("FusionMode = %s, 期望 no_evidence", audit.FusionMode)
		}
		if len(audit.ActiveSources) != 0 {
			t.Errorf("ActiveSources = %v, 期望为空", audit.ActiveSources)
		}
	})

	t.Run("多来源融合", func(t *testing.T) {
		_, audit := Fuse([]SourcedCandidate{
			{DocID: "a", Score: 0.9, Source: "vector"},
			{DocID: "b", Score: 0.9, Source: "timeline"},
		}, DefaultRRFConfig())
		if audit.FusionMode != "rrf_2_sources" {
			t.Errorf("FusionMode = %s, 期望 rrf_2_sources", audit.FusionMode)
		}
	})
}

// TestLoadRRFConfigFromEnv 校验 RRF 环境变量解析与非法值回退
func TestLoadRRFConfigFromEnv(t *testing.T) {
	t.Run("默认值", func(t *testing.T) {
		t.Setenv("RRF_ENABLED", "")
		t.Setenv("RRF_K", "")
		t.Setenv("RRF_SOURCE_WEIGHTS", "")

		config := LoadRRFConfigFromEnv()
		if !config.Enabled {
			t.Error("未配置 RRF_ENABLED 时期望默认启用")
		}
		if config.K != defaultRRFK {
			t.Errorf("K = %v, 期望 %v", config.K, defaultRRFK)
		}
		if config.SourceWeights["knowledge"] != 1.2 {
			t.Errorf("默认 knowledge 权重 = %v, 期望 1.2", config.SourceWeights["knowledge"])
		}
	})

	t.Run("显式配置", func(t *testing.T) {
		t.Setenv("RRF_ENABLED", "false")
		t.Setenv("RRF_K", "42")
		t.Setenv("RRF_SOURCE_WEIGHTS", "vector:2.0,knowledge:0.5")

		config := LoadRRFConfigFromEnv()
		if config.Enabled {
			t.Error("RRF_ENABLED=false 时期望关闭")
		}
		if config.K != 42 {
			t.Errorf("K = %v, 期望 42", config.K)
		}
		if config.SourceWeights["vector"] != 2.0 || config.SourceWeights["knowledge"] != 0.5 {
			t.Errorf("SourceWeights = %v", config.SourceWeights)
		}
	})

	t.Run("非法值回退", func(t *testing.T) {
		t.Setenv("RRF_ENABLED", "not-a-bool")
		t.Setenv("RRF_K", "-3")
		t.Setenv("RRF_SOURCE_WEIGHTS", "broken-format")

		config := LoadRRFConfigFromEnv()
		if !config.Enabled {
			t.Error("非法 RRF_ENABLED 应回退为默认启用")
		}
		if config.K != defaultRRFK {
			t.Errorf("非法 RRF_K 应回退 %v, 实际 %v", defaultRRFK, config.K)
		}
		if config.SourceWeights["knowledge"] != 1.2 {
			t.Errorf("非法权重应回退默认, 实际 %v", config.SourceWeights)
		}
	})
}

// TestParseRRFSourceWeightsRejectsUnusableValues 覆盖收紧后的解析边界。
//
// 背景：此前 strconv.ParseFloat 会接受 NaN/Inf，且不拒绝非正权重 —— NaN 会传染整个
// RRF 分数，0 权重等价于静默排除该来源。这类取值不应悄悄改变融合行为，而应让整串
// 解析失败、由调用方回退默认权重并告警。
func TestParseRRFSourceWeightsRejectsUnusableValues(t *testing.T) {
	valid, err := parseRRFSourceWeights("vector:1.0,knowledge:0.3")
	if err != nil || valid["vector"] != 1.0 || valid["knowledge"] != 0.3 {
		t.Fatalf("合法输入解析失败: weights=%v err=%v", valid, err)
	}

	for _, raw := range []string{
		"vector:NaN",
		"vector:Inf",
		"vector:-Inf",
		"vector:0",
		"vector:-1",
		"vector:1.0,timeline:0",
		"broken-format",
	} {
		if weights, err := parseRRFSourceWeights(raw); err == nil {
			t.Errorf("输入 %q 应被拒绝，实际得到 %v", raw, weights)
		}
	}
}

// TestLoadRRFConfigFromEnvEmptyWeightsKeepsDefault 覆盖「非空但不含任何有效片段」的边角。
//
// 此前形如 " , " 的取值既不覆盖配置也不告警（parse 返回空表且无 error），会让调用方
// 误以为覆盖已生效；现在必须显式告警并保留默认权重。
func TestLoadRRFConfigFromEnvEmptyWeightsKeepsDefault(t *testing.T) {
	t.Setenv("RRF_SOURCE_WEIGHTS", " , ")

	config := LoadRRFConfigFromEnv()
	if config.SourceWeights["vector"] != 1.0 || config.SourceWeights["knowledge"] != 1.2 || config.SourceWeights["timeline"] != 0.8 {
		t.Fatalf("空权重表时应保留默认权重，实际 %v", config.SourceWeights)
	}
}

// TestApplyRRFFusionToRetrievalDisabled 基线对照语义：Enabled=false 时返回 nil 且不改变结果顺序
func TestApplyRRFFusionToRetrievalDisabled(t *testing.T) {
	matchA := &models.VectorMatch{ID: "a", Content: "a", Score: 0.3, Metadata: map[string]interface{}{"doc_id": "docA"}}
	matchB := &models.VectorMatch{ID: "b", Content: "b", Score: 0.9, Metadata: map[string]interface{}{"doc_id": "docB"}}
	retrieval := &RetrievalResults{
		Results: []interface{}{matchA, matchB},
		Sources: []string{"vector", "vector"},
	}
	cfg := RRFConfig{Enabled: false, K: defaultRRFK, SourceWeights: defaultRRFSourceWeights()}

	audit := ApplyRRFFusionToRetrieval(retrieval, cfg)

	if audit != nil {
		t.Errorf("Enabled=false 时应返回 nil，实际 %+v", audit)
	}
	if retrieval.Results[0] != matchA || retrieval.Results[1] != matchB {
		t.Error("Enabled=false 时不得改变结果顺序")
	}
	if retrieval.FusionMetadata != nil {
		t.Errorf("Enabled=false 时不应写入 FusionMetadata，实际 %v", retrieval.FusionMetadata)
	}
}

// TestApplyRRFFusionToRetrievalEnabled 校验生产链路融合后的重排与审计元数据注入
func TestApplyRRFFusionToRetrievalEnabled(t *testing.T) {
	matchA := &models.VectorMatch{ID: "a", Content: "a", Score: 0.3, Metadata: map[string]interface{}{"doc_id": "docA"}}
	matchB := &models.VectorMatch{ID: "b", Content: "b", Score: 0.9, Metadata: map[string]interface{}{"doc_id": "docB"}}
	retrieval := &RetrievalResults{
		Results: []interface{}{matchA, matchB},
		Sources: []string{"vector", "vector"},
	}
	cfg := RRFConfig{Enabled: true, K: defaultRRFK, SourceWeights: defaultRRFSourceWeights()}

	audit := ApplyRRFFusionToRetrieval(retrieval, cfg)
	if audit == nil {
		t.Fatal("Enabled=true 且存在证据时应返回审计信息")
	}
	if audit.FusionMode != "vector_only_fallback" {
		t.Errorf("FusionMode = %s, 期望 vector_only_fallback", audit.FusionMode)
	}
	if retrieval.Results[0] != matchB {
		t.Errorf("docB(0.9) 应排第一，实际 %v", retrieval.Results[0])
	}
	if retrieval.FusionMetadata == nil {
		t.Fatal("FusionMetadata 未写入")
	}
	if retrieval.FusionMetadata["retrieval_fusion_mode"] != "vector_only_fallback" {
		t.Errorf("FusionMetadata 融合模式错误: %v", retrieval.FusionMetadata["retrieval_fusion_mode"])
	}
	if _, exists := matchB.Metadata["rrf_score"]; !exists {
		t.Errorf("应注入 rrf_score 审计元数据, 实际 %v", matchB.Metadata)
	}
	if matchB.Metadata["doc_id"] != "docB" {
		t.Errorf("原始 doc_id 应保留, 实际 %v", matchB.Metadata["doc_id"])
	}
}
