package security

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MultiLayerDetector 多层检测器
type MultiLayerDetector struct {
	// 各层检测器
	regexDetector *Detector          // Layer 1: 正则
	dictMatcher   *DictionaryMatcher // Layer 2: 词典
	contextEngine *ContextRuleEngine // Layer 4: 上下文规则
	llmDetector   *LLMDetector       // Layer 5: LLM

	// 🆕 理论创新模型
	pccmModel      *ProgressiveConfidenceModel         // PCCM渐进式置信度累积模型
	casiaAlgorithm *ContextAwareSensitiveInfoAlgorithm // CASIA上下文感知算法

	// 配置
	weights        map[int]float64 // 各层权重
	threshold      float64         // 置信度阈值
	earlyStop      bool            // 是否提前终止
	parallelLayers []int           // 可并行执行的层
	usePCCM        bool            // 🆕 是否使用PCCM模型
	useCASIA       bool            // 🆕 是否使用CASIA算法
	pccmConfig     PCCMSecurityConfig

	// 统计
	stats *DetectionStats
	mu    sync.RWMutex
}

const PCCMSecurityConfigVersion = "pccm-s-fixed-v1"

// PCCMSecurityConfig is the frozen, hand-configured PCCM-S fusion contract.
// CalibrationSource is explicit so fixed weights are never presented as a
// learned or validated calibration artifact.
type PCCMSecurityConfig struct {
	Version                  string          `json:"version"`
	CalibrationSource        string          `json:"calibration_source"`
	LayerWeights             map[int]float64 `json:"layer_weights"`
	EnhancementPerExtraLayer float64         `json:"enhancement_per_extra_layer"`
	DecisionThreshold        float64         `json:"decision_threshold"`
	// LayerActivationThreshold 是层数计入协同增强因子的置信度下限，
	// 与既有生产行为（>0.3 计为有效层）保持一致。
	LayerActivationThreshold float64 `json:"layer_activation_threshold"`
}

func DefaultPCCMSecurityConfig() PCCMSecurityConfig {
	return PCCMSecurityConfig{
		Version:                  PCCMSecurityConfigVersion,
		CalibrationSource:        "fixed_unvalidated",
		LayerWeights:             map[int]float64{1: 0.70, 2: 0.10, 4: 0.15, 5: 0.05},
		EnhancementPerExtraLayer: 0.05,
		DecisionThreshold:        0.45,
		LayerActivationThreshold: 0.3,
	}
}

// DetectionStats 检测统计
type DetectionStats struct {
	TotalDetections int64
	LayerUsage      map[int]int64
	AverageLatency  time.Duration
	EarlyStopCount  int64
	ConflictCount   int64
}

// LayerResult 单层检测结果
type LayerResult struct {
	LayerID     int
	Items       []SensitiveInfo
	Confidence  float64
	ProcessTime time.Duration
	Error       error
}

// FusionResult 融合结果
type FusionResult struct {
	FinalItems        []SensitiveInfo
	FinalConfidence   float64
	LayersUsed        []int
	ConflictsResolved int
	TotalTime         time.Duration
	Details           map[int]*LayerResult
}

// MultiLayerConfiguration is emitted with a scan result so an experiment can
// prove which detection path actually produced its outcome.
type MultiLayerConfiguration struct {
	PCCMEnabled  bool               `json:"pccm_enabled"`
	CASIAEnabled bool               `json:"casia_enabled"`
	EarlyStop    bool               `json:"early_stop"`
	PCCM         PCCMSecurityConfig `json:"pccm_s"`
	CASIA        CASIAConfig        `json:"casia"`
}

// NewMultiLayerDetector 创建多层检测器
func NewMultiLayerDetector(ollamaURL, llmModel string) *MultiLayerDetector {
	// PCCM-S 配置唯一来源：环境变量覆盖默认值，模型与配置快照共享同一份。
	pccmConfig := PCCMSecurityConfigFromEnv()

	return &MultiLayerDetector{
		regexDetector: NewDetector(),
		dictMatcher:   NewDictionaryMatcher(),
		contextEngine: NewContextRuleEngine(),
		llmDetector:   NewLLMDetector(ollamaURL, llmModel, 500*time.Millisecond),

		// 🆕 初始化理论创新模型（PCCM-S 单一真源）
		pccmModel:      NewProgressiveConfidenceModelWithConfig(pccmConfig),
		casiaAlgorithm: NewContextAwareSensitiveInfoAlgorithm(),

		weights: map[int]float64{
			1: 0.10, // 正则
			2: 0.15, // 词典
			4: 0.15, // 上下文
			5: 0.40, // LLM
		},
		threshold:      0.45, // 平衡精确率和召回率的最优阈值
		earlyStop:      true,
		parallelLayers: []int{1, 2, 4}, // L1, L2, L4可并行
		usePCCM:        true,           // 🆕 默认启用PCCM
		useCASIA:       true,           // 🆕 默认启用CASIA
		pccmConfig:     DefaultPCCMSecurityConfig(),
		stats: &DetectionStats{
			LayerUsage: make(map[int]int64),
		},
	}
}

// Detect 多层检测
func (mld *MultiLayerDetector) Detect(ctx context.Context, text string) (*FusionResult, error) {
	startTime := time.Now()

	result := &FusionResult{
		FinalItems: make([]SensitiveInfo, 0),
		LayersUsed: make([]int, 0),
		Details:    make(map[int]*LayerResult),
	}

	// Layer 1: 正则检测（必须执行）
	layer1Result := mld.executeLayer1(text)
	result.Details[1] = layer1Result
	result.LayersUsed = append(result.LayersUsed, 1)

	// 检查是否可以提前终止
	if mld.earlyStop && layer1Result.Confidence >= mld.threshold {
		result.FinalItems = layer1Result.Items
		result.FinalConfidence = layer1Result.Confidence
		result.TotalTime = time.Since(startTime)
		mld.updateStats(result)
		return result, nil
	}

	// Layer 2: 词典匹配（快速）
	layer2Result := mld.executeLayer2(text)
	result.Details[2] = layer2Result
	result.LayersUsed = append(result.LayersUsed, 2)

	// 融合L1和L2
	merged12 := mld.mergeResults([]LayerResult{*layer1Result, *layer2Result})
	confidence12 := mld.calculateConfidence([]LayerResult{*layer1Result, *layer2Result})

	if mld.earlyStop && confidence12 >= mld.threshold {
		result.FinalItems = merged12
		result.FinalConfidence = confidence12
		result.TotalTime = time.Since(startTime)
		mld.updateStats(result)
		return result, nil
	}

	// Layer 4: 上下文规则（NER结果为空，暂时传空）
	layer4Result := mld.executeLayer4(text, []NERResult{})
	result.Details[4] = layer4Result
	result.LayersUsed = append(result.LayersUsed, 4)

	// 融合L1, L2, L4
	merged124 := mld.mergeResults([]LayerResult{*layer1Result, *layer2Result, *layer4Result})
	confidence124 := mld.calculateConfidence([]LayerResult{*layer1Result, *layer2Result, *layer4Result})

	if mld.earlyStop && confidence124 >= mld.threshold {
		result.FinalItems = merged124
		result.FinalConfidence = confidence124
		result.TotalTime = time.Since(startTime)
		mld.updateStats(result)
		return result, nil
	}

	// Layer 5: LLM检测（最慢，按需调用）
	// 提取已标记的范围
	markedRanges := mld.extractMarkedRanges(merged124)
	layer5Result := mld.executeLayer5(ctx, text, markedRanges)
	result.Details[5] = layer5Result
	result.LayersUsed = append(result.LayersUsed, 5)

	// 最终融合
	allResults := []LayerResult{*layer1Result, *layer2Result, *layer4Result, *layer5Result}
	result.FinalItems = mld.mergeResults(allResults)
	result.FinalConfidence = mld.calculateConfidence(allResults)
	result.ConflictsResolved = mld.countConflicts(allResults)
	result.TotalTime = time.Since(startTime)

	mld.updateStats(result)

	return result, nil
}

// executeLayer1 执行Layer 1（正则检测）
func (mld *MultiLayerDetector) executeLayer1(text string) *LayerResult {
	start := time.Now()

	items := mld.regexDetector.Detect(text)

	// 🆕 如果启用CASIA算法，对正则检测结果进行上下文感知调整
	if mld.useCASIA && mld.casiaAlgorithm != nil {
		items = mld.applyCASIA(text, items)
	}

	return &LayerResult{
		LayerID:     1,
		Items:       items,
		Confidence:  mld.calculateLayerConfidence(items, 1),
		ProcessTime: time.Since(start),
	}
}

// executeLayer2 执行Layer 2（词典匹配）
func (mld *MultiLayerDetector) executeLayer2(text string) *LayerResult {
	start := time.Now()

	items := mld.dictMatcher.Match(text)

	// 🆕 如果启用CASIA算法，对词典匹配结果进行上下文感知调整
	if mld.useCASIA && mld.casiaAlgorithm != nil {
		items = mld.applyCASIA(text, items)
	}

	return &LayerResult{
		LayerID:     2,
		Items:       items,
		Confidence:  mld.calculateLayerConfidence(items, 2),
		ProcessTime: time.Since(start),
	}
}

// executeLayer4 执行Layer 4（上下文规则）
func (mld *MultiLayerDetector) executeLayer4(text string, nerResults []NERResult) *LayerResult {
	start := time.Now()

	matches := mld.contextEngine.Analyze(text, nerResults)

	// 转换为SensitiveInfo
	items := make([]SensitiveInfo, len(matches))
	for i, match := range matches {
		items[i] = match.SensitiveInfo
	}

	// 🆕 如果启用CASIA算法，对每个检测结果进行上下文感知调整
	if mld.useCASIA && mld.casiaAlgorithm != nil {
		items = mld.applyCASIA(text, items)
	}

	return &LayerResult{
		LayerID:     4,
		Items:       items,
		Confidence:  mld.calculateLayerConfidence(items, 4),
		ProcessTime: time.Since(start),
	}
}

// 🆕 applyCASIA 应用CASIA算法调整置信度
func (mld *MultiLayerDetector) applyCASIA(text string, items []SensitiveInfo) []SensitiveInfo {
	adjustedItems := make([]SensitiveInfo, 0, len(items))

	for _, item := range items {
		// 使用CASIA算法分析上下文
		evidence := mld.casiaAlgorithm.AnalyzeContext(
			text,
			item.Start,
			item.End,
			string(item.Type),
			mld.casiaAlgorithm.config.ContextWindow,
		)
		evidence.BaseConfidence = item.Confidence
		adjustedConf := item.Confidence * evidence.NormalizedWeight
		evidence.AdjustedScore = adjustedConf

		// 更新置信度
		item.Confidence = adjustedConf
		item.CASIA = &evidence

		// 只保留置信度足够高的结果（过滤掉被CASIA判定为误报的）
		if adjustedConf > 0.3 {
			adjustedItems = append(adjustedItems, item)
		}
	}

	return adjustedItems
}

// executeLayer5 执行Layer 5（LLM检测）
func (mld *MultiLayerDetector) executeLayer5(ctx context.Context, text string, markedRanges [][2]int) *LayerResult {
	start := time.Now()

	llmResult, err := mld.llmDetector.Detect(ctx, text, markedRanges)
	if err != nil {
		return &LayerResult{
			LayerID:     5,
			Items:       []SensitiveInfo{},
			Confidence:  0,
			ProcessTime: time.Since(start),
			Error:       err,
		}
	}

	// 转换为SensitiveInfo
	items := make([]SensitiveInfo, len(llmResult.Items))
	for i, item := range llmResult.Items {
		items[i] = SensitiveInfo{
			Type:       SensitiveType(item.Type),
			Value:      text[item.Start:item.End],
			Start:      item.Start,
			End:        item.End,
			Label:      item.Type,
			Position:   item.Start,
			Length:     item.End - item.Start,
			Confidence: item.Confidence,
			Encrypted:  false,
		}
	}

	return &LayerResult{
		LayerID:     5,
		Items:       items,
		Confidence:  mld.calculateLayerConfidence(items, 5),
		ProcessTime: time.Since(start),
	}
}

// mergeResults 合并多层结果
func (mld *MultiLayerDetector) mergeResults(results []LayerResult) []SensitiveInfo {
	// 使用map去重（基于位置）
	itemMap := make(map[string]SensitiveInfo)

	for _, result := range results {
		for _, item := range result.Items {
			key := fmt.Sprintf("%d-%d", item.Start, item.End)

			// 如果已存在，选择置信度更高的
			if existing, ok := itemMap[key]; ok {
				if item.Confidence > existing.Confidence {
					itemMap[key] = item
				}
			} else {
				itemMap[key] = item
			}
		}
	}

	// 转换为切片
	merged := make([]SensitiveInfo, 0, len(itemMap))
	for _, item := range itemMap {
		merged = append(merged, item)
	}

	return merged
}

// calculateConfidence 计算综合置信度
func (mld *MultiLayerDetector) calculateConfidence(results []LayerResult) float64 {
	if len(results) == 0 {
		return 0
	}

	// 🆕 如果启用PCCM模型，使用PCCM计算置信度
	if mld.usePCCM && mld.pccmModel != nil {
		return mld.calculateConfidenceWithPCCM(results)
	}

	// 原有的简单加权计算（作为后备方案）
	totalWeight := 0.0
	weightedSum := 0.0

	for _, result := range results {
		weight := mld.weights[result.LayerID]
		totalWeight += weight
		weightedSum += weight * result.Confidence
	}

	if totalWeight == 0 {
		return 0
	}

	baseConfidence := weightedSum / totalWeight

	// 一致性加成
	consistencyBonus := mld.calculateConsistencyBonus(results)

	finalConfidence := baseConfidence + consistencyBonus

	// 限制在0-1之间
	if finalConfidence > 1.0 {
		finalConfidence = 1.0
	}

	return finalConfidence
}

// calculateConfidenceWithPCCM 使用PCCM模型计算置信度。
// 置信度融合公式的唯一实现位于 ProgressiveConfidenceModel.CalculateFinalConfidence，
// 此处仅做委托：把实际执行层的置信度组装为 map 交给模型。
func (mld *MultiLayerDetector) calculateConfidenceWithPCCM(results []LayerResult) float64 {
	layerConfidences := make(map[int]float64, len(results))
	for _, result := range results {
		layerConfidences[result.LayerID] = result.Confidence
	}
	return mld.pccmModel.CalculateFinalConfidence(layerConfidences)
}

// calculateLayerConfidence 计算单层置信度
func (mld *MultiLayerDetector) calculateLayerConfidence(items []SensitiveInfo, layerID int) float64 {
	if len(items) == 0 {
		return 0
	}

	// 计算平均置信度
	sum := 0.0
	for _, item := range items {
		sum += item.Confidence
	}

	return sum / float64(len(items))
}

// calculateConsistencyBonus 计算一致性加成
func (mld *MultiLayerDetector) calculateConsistencyBonus(results []LayerResult) float64 {
	// 统计有检测结果的层数
	layersWithResults := 0
	for _, result := range results {
		if len(result.Items) > 0 {
			layersWithResults++
		}
	}

	// 一致性因子 = 0.1 × (一致层数 - 1)
	if layersWithResults > 1 {
		return 0.1 * float64(layersWithResults-1)
	}

	return 0
}

// countConflicts 统计冲突数量
func (mld *MultiLayerDetector) countConflicts(results []LayerResult) int {
	conflicts := 0

	// 简单统计：同一位置有不同类型的检测结果
	positionTypes := make(map[string]map[SensitiveType]bool)

	for _, result := range results {
		for _, item := range result.Items {
			key := fmt.Sprintf("%d-%d", item.Start, item.End)
			if positionTypes[key] == nil {
				positionTypes[key] = make(map[SensitiveType]bool)
			}
			positionTypes[key][item.Type] = true
		}
	}

	for _, types := range positionTypes {
		if len(types) > 1 {
			conflicts++
		}
	}

	return conflicts
}

// extractMarkedRanges 提取已标记的范围
func (mld *MultiLayerDetector) extractMarkedRanges(items []SensitiveInfo) [][2]int {
	ranges := make([][2]int, len(items))
	for i, item := range items {
		ranges[i] = [2]int{item.Start, item.End}
	}
	return ranges
}

// updateStats 更新统计信息
func (mld *MultiLayerDetector) updateStats(result *FusionResult) {
	mld.mu.Lock()
	defer mld.mu.Unlock()

	mld.stats.TotalDetections++

	for _, layerID := range result.LayersUsed {
		mld.stats.LayerUsage[layerID]++
	}

	// 更新平均延迟
	if mld.stats.TotalDetections == 1 {
		mld.stats.AverageLatency = result.TotalTime
	} else {
		mld.stats.AverageLatency = (mld.stats.AverageLatency*time.Duration(mld.stats.TotalDetections-1) + result.TotalTime) / time.Duration(mld.stats.TotalDetections)
	}

	if len(result.LayersUsed) < 5 {
		mld.stats.EarlyStopCount++
	}

	if result.ConflictsResolved > 0 {
		mld.stats.ConflictCount++
	}
}

// GetStats 获取统计信息
func (mld *MultiLayerDetector) GetStats() *DetectionStats {
	mld.mu.RLock()
	defer mld.mu.RUnlock()

	// 返回副本
	statsCopy := &DetectionStats{
		TotalDetections: mld.stats.TotalDetections,
		LayerUsage:      make(map[int]int64),
		AverageLatency:  mld.stats.AverageLatency,
		EarlyStopCount:  mld.stats.EarlyStopCount,
		ConflictCount:   mld.stats.ConflictCount,
	}

	for k, v := range mld.stats.LayerUsage {
		statsCopy.LayerUsage[k] = v
	}

	return statsCopy
}

// SetThreshold 设置置信度阈值
func (mld *MultiLayerDetector) SetThreshold(threshold float64) {
	mld.mu.Lock()
	defer mld.mu.Unlock()
	mld.threshold = threshold
}

// EnableEarlyStop 启用提前终止
func (mld *MultiLayerDetector) EnableEarlyStop() {
	mld.mu.Lock()
	defer mld.mu.Unlock()
	mld.earlyStop = true
}

// DisableEarlyStop 禁用提前终止
func (mld *MultiLayerDetector) DisableEarlyStop() {
	mld.mu.Lock()
	defer mld.mu.Unlock()
	mld.earlyStop = false
}

// SetLayerWeight 设置层权重
func (mld *MultiLayerDetector) SetLayerWeight(layerID int, weight float64) {
	mld.mu.Lock()
	defer mld.mu.Unlock()
	mld.weights[layerID] = weight
}

// GetLayerWeight 获取层权重
func (mld *MultiLayerDetector) GetLayerWeight(layerID int) float64 {
	mld.mu.RLock()
	defer mld.mu.RUnlock()
	return mld.weights[layerID]
}

// 🆕 EnablePCCM 启用PCCM模型
func (mld *MultiLayerDetector) EnablePCCM() {
	mld.mu.Lock()
	defer mld.mu.Unlock()
	mld.usePCCM = true
}

// 🆕 DisablePCCM 禁用PCCM模型
func (mld *MultiLayerDetector) DisablePCCM() {
	mld.mu.Lock()
	defer mld.mu.Unlock()
	mld.usePCCM = false
}

// 🆕 EnableCASIA 启用CASIA算法
func (mld *MultiLayerDetector) EnableCASIA() {
	mld.mu.Lock()
	defer mld.mu.Unlock()
	mld.useCASIA = true
}

// 🆕 DisableCASIA 禁用CASIA算法
func (mld *MultiLayerDetector) DisableCASIA() {
	mld.mu.Lock()
	defer mld.mu.Unlock()
	mld.useCASIA = false
}

// GetConfiguration returns a stable snapshot for audit and evaluation output.
func (mld *MultiLayerDetector) GetConfiguration() MultiLayerConfiguration {
	mld.mu.RLock()
	defer mld.mu.RUnlock()
	return MultiLayerConfiguration{
		PCCMEnabled:  mld.usePCCM,
		CASIAEnabled: mld.useCASIA,
		EarlyStop:    mld.earlyStop,
		PCCM:         clonePCCMSecurityConfig(mld.pccmConfig),
		CASIA:        mld.casiaAlgorithm.GetConfiguration(),
	}
}

func clonePCCMSecurityConfig(source PCCMSecurityConfig) PCCMSecurityConfig {
	cloned := source
	cloned.LayerWeights = make(map[int]float64, len(source.LayerWeights))
	for layerID, weight := range source.LayerWeights {
		cloned.LayerWeights[layerID] = weight
	}
	return cloned
}

// 🆕 GetPCCMModel 获取PCCM模型（用于调优）
func (mld *MultiLayerDetector) GetPCCMModel() *ProgressiveConfidenceModel {
	return mld.pccmModel
}

// 🆕 GetCASIAAlgorithm 获取CASIA算法（用于调优）
func (mld *MultiLayerDetector) GetCASIAAlgorithm() *ContextAwareSensitiveInfoAlgorithm {
	return mld.casiaAlgorithm
}
