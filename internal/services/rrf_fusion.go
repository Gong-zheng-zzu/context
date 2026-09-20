package services

import (
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/contextkeeper/service/internal/models"
)

// =============================================================================
// 通用加权 RRF（Reciprocal Rank Fusion）融合单元
//
// 该模块是多源检索结果融合的单一实现，同时服务于：
//  1. 评测通道（retrieveEvaluationRRF / buildEvaluationRRFResponse）
//  2. 生产链路（executeLLMDrivenFlow 中 ParallelRetrieve 之后、SynthesizeResponse 之前）
//
// 默认参数（K=60，来源权重 vector=1.0 / knowledge=1.2 / timeline=0.8）与历史
// 评测通道常量 evaluationRRFConstant / evaluationRRFWeights 保持一致，未配置
// 环境变量时两条路径的行为与既有实现完全相同，保证向后兼容。
// =============================================================================

// defaultRRFK 默认 RRF 常数 K（与历史评测常量一致）
const defaultRRFK = 60.0

// defaultRRFSourceWeights 返回默认来源权重。
//
// 取值来源说明：这组数字**继承自历史常量**（最早定义在
// internal/engines/multi_dimensional_retrieval/knowledge/retrieval_strategy.go 的
// DefaultRRFConfig，仅把 graph/time 改名为 knowledge/timeline），不是搜索或标定的
// 结果。本注释此前只引用另一处同样引用本函数的常量，属循环引用、无独立依据，故补充
// 现有实测证据如下。
//
// 实测证据（`experiments/scripts/retrieval_eval.py`，30 条查询，语料
// `retrieval_corpus_30.json` sha256=75157240…efa6ef，经 `run_configured_eval.ps1` 门禁产出）：
//   - 本默认权重下三路融合 MRR 0.4172 / P@5 0.1533 / R@5 0.6000，全面优于向量单路
//     基线（0.3461 / 0.1333 / 0.5167）；结果文件
//     `experiments/results/raw/retrieval_20260920_010138.json`。
//   - 该正收益的前提是知识图谱通道按查询相关度打分。此前该通道候选分数恒为 0、
//     来源内排序退化，同一组默认权重表现为**负收益**（MRR 0.2717）；详见
//     `docs/competition/对比实验报告.md` 的缺陷清单第 7 项。
//
// 结论：本组默认值在当前评测语料上已被验证为有效，但**仍是继承值而非搜索所得**。
// 如需重新标定，应通过 `RRF_SOURCE_WEIGHTS` 做门禁对照，不要直接修改本函数。
func defaultRRFSourceWeights() map[string]float64 {
	return map[string]float64{
		"vector":    1.0,
		"knowledge": 1.2,
		"timeline":  0.8,
	}
}

// RRFConfig 加权 RRF 融合配置
type RRFConfig struct {
	// K RRF 常数（越大排名差异越平滑），<=0 时回退到 defaultRRFK
	K float64
	// SourceWeights 来源权重，未知来源不参与融合；为空时回退到默认权重
	SourceWeights map[string]float64
	// Enabled 融合开关。Enabled=false 时生产链路跳过融合走原路径
	// （基线对照配置如 baseline_vanilla_llm / baseline_naive_rag 依赖此语义）。
	// 评测 RRF 通道作为专用证据通道不受该开关影响，始终执行融合。
	Enabled bool
}

// DefaultRRFConfig 返回默认 RRF 配置（K=60，权重同现状，Enabled=true）
func DefaultRRFConfig() RRFConfig {
	return RRFConfig{
		K:             defaultRRFK,
		SourceWeights: defaultRRFSourceWeights(),
		Enabled:       true,
	}
}

// LoadRRFConfigFromEnv 从环境变量加载 RRF 配置：
//   - RRF_ENABLED:            是否启用融合（默认 true）
//   - RRF_K:                  RRF 常数（默认 60）
//   - RRF_SOURCE_WEIGHTS:     来源权重，格式 "vector:1.0,knowledge:1.2,timeline:0.8"
//
// 未配置的项回退到默认值（K=60、权重同现状），保证向后兼容。
func LoadRRFConfigFromEnv() RRFConfig {
	config := DefaultRRFConfig()

	if raw, exists := os.LookupEnv("RRF_ENABLED"); exists {
		if enabled, err := strconv.ParseBool(strings.TrimSpace(raw)); err == nil {
			config.Enabled = enabled
		}
	}
	if raw := strings.TrimSpace(os.Getenv("RRF_K")); raw != "" {
		if k, err := strconv.ParseFloat(raw, 64); err == nil && k > 0 {
			config.K = k
		}
	}
	if raw := strings.TrimSpace(os.Getenv("RRF_SOURCE_WEIGHTS")); raw != "" {
		if weights, err := parseRRFSourceWeights(raw); err != nil {
			log.Printf("⚠️ [RRF配置] RRF_SOURCE_WEIGHTS 解析失败，回退默认权重: %v", err)
		} else if len(weights) == 0 {
			// 形如 "," 或 " , " 的取值会解析出空权重表。此前该分支既不覆盖也不告警，
			// 会让调用方误以为覆盖已生效，故显式告警并保留默认权重。
			log.Printf("⚠️ [RRF配置] RRF_SOURCE_WEIGHTS=%q 未解析出任何来源权重，保留默认权重 %v", raw, config.SourceWeights)
		} else {
			config.SourceWeights = weights
		}
	}
	return config
}

// parseRRFSourceWeights 解析 "key:value,key:value" 格式的来源权重。
//
// 校验边界：拒绝 NaN/Inf 与非正权重。这两类取值在融合中都会产生无意义结果
// （NaN 会传染整个 RRF 分数；0 权重等价于把该来源静默排除，若确实想排除，正确做法
// 是直接不写该 key）。因此宁可让整串解析失败、由调用方回退默认权重并告警，
// 也不接受这类取值悄悄改变融合行为。
func parseRRFSourceWeights(raw string) (map[string]float64, error) {
	weights := make(map[string]float64)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		keyValue := strings.SplitN(part, ":", 2)
		if len(keyValue) != 2 {
			return nil, fmt.Errorf("无效的权重片段: %q（期望 key:value）", part)
		}
		key := strings.TrimSpace(keyValue[0])
		value, err := strconv.ParseFloat(strings.TrimSpace(keyValue[1]), 64)
		if err != nil || key == "" {
			return nil, fmt.Errorf("无效的权重片段: %q", part)
		}
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, fmt.Errorf("来源 %q 的权重不是有限数值: %v", key, value)
		}
		if value <= 0 {
			return nil, fmt.Errorf("来源 %q 的权重必须为正数（当前 %v；若要排除该来源，请勿写入该 key）", key, value)
		}
		weights[key] = value
	}
	return weights, nil
}

// SourcedCandidate 带来源的待融合候选
type SourcedCandidate struct {
	DocID    string
	Content  string
	Score    float64
	Source   string
	Metadata map[string]interface{}
}

// FusedCandidate 融合后的候选（已按 RRF 分数降序排序）
type FusedCandidate struct {
	DocID       string
	Content     string
	Score       float64         // 加权 RRF 分数：sum(weight[source] / (K + rank))
	SourceRanks map[string]int  // 各来源中的名次（从1开始）
	Sources     []string        // 命中的来源（已排序）
	Metadata    map[string]interface{}
}

// FusionAudit 融合审计信息（来源状态、融合模式等，随结果一起传递给下游）
type FusionAudit struct {
	// FusionMode 融合模式：
	//   - "rrf_N_sources"：N(>=2) 个来源产出证据
	//   - "<source>_only_fallback"：仅单一来源产出证据
	//   - "no_evidence"：所有来源均无证据
	FusionMode string
	// ActiveSources 产出证据的来源（按固定顺序）
	ActiveSources []string
	// EmptySources 未产出证据的来源（按固定顺序）
	EmptySources []string
	// SourceCandidateCounts 各来源参与融合的候选数
	SourceCandidateCounts map[string]int
}

// canonicalRRFSources 固定来源顺序，保证审计输出稳定可对比
var canonicalRRFSources = []string{"vector", "knowledge", "timeline"}

// Fuse 加权 RRF 融合。
// 算法：按来源分组并去重（同源同文档保留原始最高分）→ 来源内按原始分数降序
// 排名 → RRF 累加 weight/(K+rank) → 总体按 RRF 分数降序输出。
// 复杂度：O(N log N) 排序 + O(N) 融合，适配 N<=200 的候选池。
// 未知来源（不在 SourceWeights 中）与空 DocID 的候选不参与融合。
func Fuse(candidates []SourcedCandidate, cfg RRFConfig) ([]FusedCandidate, FusionAudit) {
	weights := cfg.SourceWeights
	if len(weights) == 0 {
		weights = defaultRRFSourceWeights()
	}
	k := cfg.K
	if k <= 0 {
		k = defaultRRFK
	}

	// 1. 按来源分组并去重（同源同文档保留原始最高分候选，含其元数据）
	bySource := make(map[string]map[string]SourcedCandidate)
	for _, candidate := range candidates {
		if candidate.DocID == "" {
			continue
		}
		if _, supported := weights[candidate.Source]; !supported {
			continue
		}
		if bySource[candidate.Source] == nil {
			bySource[candidate.Source] = make(map[string]SourcedCandidate)
		}
		existing, exists := bySource[candidate.Source][candidate.DocID]
		if !exists || candidate.Score > existing.Score {
			bySource[candidate.Source][candidate.DocID] = candidate
		}
	}

	// 2. 来源内排序并累加加权 RRF
	fused := make(map[string]*FusedCandidate)
	for source, sourceCandidates := range bySource {
		ranked := make([]SourcedCandidate, 0, len(sourceCandidates))
		for _, candidate := range sourceCandidates {
			ranked = append(ranked, candidate)
		}
		sort.Slice(ranked, func(left, right int) bool {
			if ranked[left].Score == ranked[right].Score {
				return ranked[left].DocID < ranked[right].DocID
			}
			return ranked[left].Score > ranked[right].Score
		})

		for position, candidate := range ranked {
			item, exists := fused[candidate.DocID]
			if !exists {
				item = &FusedCandidate{
					DocID:       candidate.DocID,
					Content:     candidate.Content,
					SourceRanks: make(map[string]int),
					Metadata:    candidate.Metadata,
				}
				fused[candidate.DocID] = item
			}
			if item.Content == "" && candidate.Content != "" {
				item.Content = candidate.Content
			}
			item.Score += weights[source] / (k + float64(position+1))
			item.SourceRanks[source] = position + 1
		}
	}

	// 3. 融合审计：固定顺序遍历来源，保持输出稳定可对比
	audit := FusionAudit{
		SourceCandidateCounts: make(map[string]int),
	}
	allSources := make([]string, 0, len(canonicalRRFSources)+len(weights))
	allSources = append(allSources, canonicalRRFSources...)
	extras := make([]string, 0)
	for source := range weights {
		if !containsString(canonicalRRFSources, source) {
			extras = append(extras, source)
		}
	}
	sort.Strings(extras)
	allSources = append(allSources, extras...)
	for _, source := range allSources {
		count := len(bySource[source])
		audit.SourceCandidateCounts[source] = count
		if count == 0 {
			audit.EmptySources = append(audit.EmptySources, source)
		} else {
			audit.ActiveSources = append(audit.ActiveSources, source)
		}
	}
	switch {
	case len(audit.ActiveSources) == 0:
		audit.FusionMode = "no_evidence"
	case len(audit.ActiveSources) == 1:
		audit.FusionMode = audit.ActiveSources[0] + "_only_fallback"
	default:
		audit.FusionMode = fmt.Sprintf("rrf_%d_sources", len(audit.ActiveSources))
	}

	// 4. 总体按 RRF 分数降序输出（并列时 DocID 升序，保证确定性）
	result := make([]FusedCandidate, 0, len(fused))
	for _, item := range fused {
		sources := make([]string, 0, len(item.SourceRanks))
		for source := range item.SourceRanks {
			sources = append(sources, source)
		}
		sort.Strings(sources)
		item.Sources = sources
		result = append(result, *item)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].Score == result[right].Score {
			return result[left].DocID < result[right].DocID
		}
		return result[left].Score > result[right].Score
	})
	return result, audit
}

// ApplyFusionAuditMetadata 将融合审计字段注入到候选元数据中。
// 注入的字段与历史评测通道语义完全一致（评测脚本与答辩证据依赖这些字段，不可裁剪）：
// rrf_score / rrf_sources / rrf_ranks / doc_id / retrieval_active_sources /
// retrieval_empty_sources / retrieval_fusion_mode / retrieval_source_statuses /
// retrieval_source_latency_ms / retrieval_source_candidate_counts /
// retrieval_wall_clock_latency_ms
func ApplyFusionAuditMetadata(
	metadata map[string]interface{},
	candidate FusedCandidate,
	audit FusionAudit,
	rawSourceStatuses map[string]string,
	rawSourceLatencies map[string]int64,
	wallClockLatencyMs int64,
) map[string]interface{} {
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	metadata["rrf_score"] = candidate.Score
	metadata["rrf_sources"] = candidate.Sources
	metadata["rrf_ranks"] = candidate.SourceRanks
	metadata["doc_id"] = candidate.DocID
	metadata["retrieval_active_sources"] = audit.ActiveSources
	metadata["retrieval_empty_sources"] = audit.EmptySources
	metadata["retrieval_fusion_mode"] = audit.FusionMode
	metadata["retrieval_source_statuses"] = evaluationSourceStatuses(rawSourceStatuses)
	metadata["retrieval_source_latency_ms"] = evaluationSourceLatencies(rawSourceLatencies)
	metadata["retrieval_source_candidate_counts"] = audit.SourceCandidateCounts
	metadata["retrieval_wall_clock_latency_ms"] = wallClockLatencyMs
	return metadata
}

// ApplyRRFFusionToRetrieval 在生产链路中对多维检索结果执行加权 RRF 融合：
//  1. 提取各来源候选并调用 Fuse
//  2. 按融合分数重排 Results（Sources 并行数组同序重排）
//  3. 向每个参与融合的结果注入 rrf_* / retrieval_* 审计元数据
//  4. 在 RetrievalResults.FusionMetadata 上记录融合级审计信息
//
// cfg.Enabled=false 时直接返回 nil 跳过融合走原路径（基线对照配置与未配置
// 环境依赖此语义）；无任何证据时同样不改变结果顺序，仅返回 no_evidence 审计。
func ApplyRRFFusionToRetrieval(retrieval *RetrievalResults, cfg RRFConfig) *FusionAudit {
	if retrieval == nil || !cfg.Enabled {
		return nil
	}
	weights := cfg.SourceWeights
	if len(weights) == 0 {
		weights = defaultRRFSourceWeights()
	}

	// 提取候选并记录候选所属的结果下标（用于把融合分数映射回原始结果）
	docIDToResultIndex := make(map[string]int)
	candidates := make([]SourcedCandidate, 0)
	for index, result := range retrieval.Results {
		if index >= len(retrieval.Sources) {
			continue
		}
		source := retrieval.Sources[index]
		if _, supported := weights[source]; !supported {
			continue
		}
		for _, candidate := range evaluationCandidatesFromResult(source, result) {
			if candidate.DocID == "" {
				continue
			}
			if _, seen := docIDToResultIndex[candidate.DocID]; !seen {
				docIDToResultIndex[candidate.DocID] = index
			}
			candidates = append(candidates, SourcedCandidate{
				DocID:    candidate.DocID,
				Content:  candidate.Content,
				Score:    candidate.Score,
				Source:   source,
				Metadata: candidate.Metadata,
			})
		}
	}

	fused, audit := Fuse(candidates, cfg)
	if len(fused) == 0 {
		log.Printf("[生产 RRF] no_evidence：所有来源均未产出可融合候选，保留原始排序")
		setRetrievalFusionMetadata(retrieval, &audit)
		return &audit
	}

	// 每个原始结果取其关联文档中 RRF 分数最高的融合候选
	best := make(map[int]*FusedCandidate)
	for index := range fused {
		fc := &fused[index]
		if resultIndex, ok := docIDToResultIndex[fc.DocID]; ok {
			if current, exists := best[resultIndex]; !exists || fc.Score > current.Score {
				best[resultIndex] = fc
			}
		}
	}

	// 注入审计元数据（TimelineEvent 无通用元数据字段，仅参与排序）
	for resultIndex, fc := range best {
		result := retrieval.Results[resultIndex]
		var metadata map[string]interface{}
		switch value := result.(type) {
		case *models.VectorMatch:
			metadata = cloneMetadataMap(value.Metadata)
		case *models.KnowledgeNode:
			metadata = cloneMetadataMap(value.Properties)
		default:
			continue
		}
		metadata = ApplyFusionAuditMetadata(metadata, *fc, audit, retrieval.SourceStatuses, retrieval.SourceLatencyMs, retrieval.WallClockLatencyMs)
		switch value := result.(type) {
		case *models.VectorMatch:
			value.Metadata = metadata
		case *models.KnowledgeNode:
			value.Properties = metadata
		}
	}

	// 按融合分数重排（稳定排序：分数并列时保持原始相对顺序，保证确定性）
	order := make([]int, len(retrieval.Results))
	for index := range order {
		order[index] = index
	}
	sort.SliceStable(order, func(left, right int) bool {
		leftScore, rightScore := 0.0, 0.0
		if fc, ok := best[order[left]]; ok {
			leftScore = fc.Score
		}
		if fc, ok := best[order[right]]; ok {
			rightScore = fc.Score
		}
		return leftScore > rightScore
	})
	newResults := make([]interface{}, len(retrieval.Results))
	newSources := make([]string, len(retrieval.Results))
	for position, originalIndex := range order {
		newResults[position] = retrieval.Results[originalIndex]
		if originalIndex < len(retrieval.Sources) {
			newSources[position] = retrieval.Sources[originalIndex]
		}
	}
	retrieval.Results = newResults
	retrieval.Sources = newSources

	setRetrievalFusionMetadata(retrieval, &audit)
	log.Printf("[生产 RRF] fusion_mode=%s active_sources=%v empty_sources=%v fused=%d",
		audit.FusionMode, audit.ActiveSources, audit.EmptySources, len(fused))
	return &audit
}

// setRetrievalFusionMetadata 在检索结果集上记录融合级审计信息
func setRetrievalFusionMetadata(retrieval *RetrievalResults, audit *FusionAudit) {
	if retrieval == nil || audit == nil {
		return
	}
	retrieval.FusionMetadata = map[string]interface{}{
		"retrieval_fusion_mode":   audit.FusionMode,
		"retrieval_active_sources": audit.ActiveSources,
		"retrieval_empty_sources":  audit.EmptySources,
		"source_candidate_counts":  audit.SourceCandidateCounts,
	}
}

// cloneMetadataMap 复制元数据映射，避免污染共享的原始结果元数据
func cloneMetadataMap(metadata map[string]interface{}) map[string]interface{} {
	cloned := make(map[string]interface{}, len(metadata)+8)
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
