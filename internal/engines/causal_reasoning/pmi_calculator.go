package causal_reasoning

import (
	"math"
	"sync"
)

// PMICalculator PMI共现强度计算器
// 实现策划书第2.2.2节的PMI公式: PMI(O,R) = log(P(O,R) / (P(O)·P(R)))
type PMICalculator struct {
	coOccurrences map[string]int    // 共现计数: "entityA|entityB" -> count
	entityCounts  map[string]int    // 单实体计数: "entity" -> count
	totalDocs     int               // 总文档数
	mu            sync.RWMutex
}

// NewPMICalculator 创建PMI计算器实例
func NewPMICalculator() *PMICalculator {
	return &PMICalculator{
		coOccurrences: make(map[string]int),
		entityCounts:  make(map[string]int),
		totalDocs:     0,
	}
}

// RecordDocument 记录文档中的实体共现
func (pc *PMICalculator) RecordDocument(entities []string) {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	pc.totalDocs++

	// 记录单实体计数
	seen := make(map[string]bool)
	for _, entity := range entities {
		if !seen[entity] {
			pc.entityCounts[entity]++
			seen[entity] = true
		}
	}

	// 记录共现计数（去重）
	for i := 0; i < len(entities); i++ {
		for j := i + 1; j < len(entities); j++ {
			key := makeCoOccurrenceKey(entities[i], entities[j])
			if _, exists := seen[key]; !exists {
				pc.coOccurrences[key]++
				seen[key] = true
			}
		}
	}
}

// CalculatePMI 计算两个实体之间的PMI分数
func (pc *PMICalculator) CalculatePMI(entityA, entityB string) *PMIScore {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	if pc.totalDocs == 0 {
		return &PMIScore{
			EntityA:    entityA,
			EntityB:    entityB,
			PMI:        0.0,
			Confidence: 0.0,
		}
	}

	// 获取计数
	countA := pc.entityCounts[entityA]
	countB := pc.entityCounts[entityB]
	coOccurKey := makeCoOccurrenceKey(entityA, entityB)
	coOccur := pc.coOccurrences[coOccurKey]

	// 如果没有共现或任一实体未出现，返回0分数
	if coOccur == 0 || countA == 0 || countB == 0 {
		return &PMIScore{
			EntityA:    entityA,
			EntityB:    entityB,
			PMI:        0.0,
			CoOccur:    coOccur,
			CountA:     countA,
			CountB:     countB,
			TotalDocs:  pc.totalDocs,
			Confidence: 0.0,
		}
	}

	// 计算概率
	pA := float64(countA) / float64(pc.totalDocs)
	pB := float64(countB) / float64(pc.totalDocs)
	pAB := float64(coOccur) / float64(pc.totalDocs)

	// PMI(A,B) = log(P(A,B) / (P(A) * P(B)))
	pmi := math.Log(pAB / (pA * pB))

	// 归一化PMI到[0,1]区间作为置信度
	// 使用sigmoid函数: confidence = 1 / (1 + exp(-pmi))
	confidence := 1.0 / (1.0 + math.Exp(-pmi))

	return &PMIScore{
		EntityA:    entityA,
		EntityB:    entityB,
		PMI:        pmi,
		CoOccur:    coOccur,
		CountA:     countA,
		CountB:     countB,
		TotalDocs:  pc.totalDocs,
		Confidence: confidence,
	}
}

// GetEntityCount 获取实体出现次数
func (pc *PMICalculator) GetEntityCount(entity string) int {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.entityCounts[entity]
}

// GetCoOccurrenceCount 获取共现次数
func (pc *PMICalculator) GetCoOccurrenceCount(entityA, entityB string) int {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	key := makeCoOccurrenceKey(entityA, entityB)
	return pc.coOccurrences[key]
}

// GetTotalDocuments 获取总文档数
func (pc *PMICalculator) GetTotalDocuments() int {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.totalDocs
}

// GetTopCoOccurrences 获取与指定实体共现最强的Top-N实体
func (pc *PMICalculator) GetTopCoOccurrences(entity string, topN int) []*PMIScore {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	var scores []*PMIScore

	for otherEntity := range pc.entityCounts {
		if otherEntity == entity {
			continue
		}

		key := makeCoOccurrenceKey(entity, otherEntity)
		coOccur := pc.coOccurrences[key]

		if coOccur > 0 {
			pc.mu.RUnlock()
			score := pc.CalculatePMI(entity, otherEntity)
			pc.mu.RLock()
			scores = append(scores, score)
		}
	}

	// 按PMI分数降序排序
	sortPMIScores(scores)

	if len(scores) > topN {
		scores = scores[:topN]
	}

	return scores
}

// Reset 重置所有统计数据
func (pc *PMICalculator) Reset() {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	pc.coOccurrences = make(map[string]int)
	pc.entityCounts = make(map[string]int)
	pc.totalDocs = 0
}

// GetStats 获取统计信息
func (pc *PMICalculator) GetStats() map[string]interface{} {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	return map[string]interface{}{
		"total_docs":        pc.totalDocs,
		"unique_entities":   len(pc.entityCounts),
		"unique_co_occurs":  len(pc.coOccurrences),
	}
}

// makeCoOccurrenceKey 生成共现键（保证顺序一致性）
func makeCoOccurrenceKey(entityA, entityB string) string {
	if entityA < entityB {
		return entityA + "|" + entityB
	}
	return entityB + "|" + entityA
}

// sortPMIScores 按PMI分数降序排序
func sortPMIScores(scores []*PMIScore) {
	for i := 0; i < len(scores)-1; i++ {
		for j := i + 1; j < len(scores); j++ {
			if scores[j].PMI > scores[i].PMI {
				scores[i], scores[j] = scores[j], scores[i]
			}
		}
	}
}
