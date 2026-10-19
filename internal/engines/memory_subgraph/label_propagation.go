package memory_subgraph

import (
	"math"
	"sort"
)

// labelPropagation 在给定的实体子集上执行标签传播（Label Propagation
// Algorithm, LPA）社区发现。这是策划书 2.2.1 节"社区子图 Gc 采用
// LabelPropagation 等经典社区发现算法对实体进行高维聚类"的实现。
//
// 返回值是若干组成员 ID 切片，已按成员 ID 升序排列；组之间无交集且并集
// 等于传入的 members。
//
// 确定性保证（项目对可复现性的硬要求）：
//  1. 节点按 ID 字典序处理，而非随机顺序；
//  2. 平局（多个标签权重相同）一律选择字典序最小的标签；
//  3. 迭代上限固定。
//
// 因此同一张图在任意机器、任意次数调用下产出完全一致的分组，
// 不受 Go map 迭代随机化影响。
//
// 算法本身不区分 owner：调用方已在 partitionByOwner 中按"一人一库"
// 切好分区，跨 owner 的边不会出现在传入的 members 与之对应的邻接集内。
func labelPropagation(
	members []string,
	adjacency map[string]map[string]float64,
	maxIterations int,
) [][]string {
	if len(members) == 0 {
		return nil
	}

	// 副本排序，避免调用方持有的切片被就地修改。
	nodes := append([]string(nil), members...)
	sort.Strings(nodes)

	// 标签初始化：每个节点持有一个唯一的初始标签（用节点 ID 表示）。
	label := make(map[string]string, len(nodes))
	for _, id := range nodes {
		label[id] = id
	}

	// 成员集合，用于过滤跨 owner 的邻居（双保险）。
	inPartition := make(map[string]struct{}, len(nodes))
	for _, id := range nodes {
		inPartition[id] = struct{}{}
	}

	for iteration := 0; iteration < maxIterations; iteration++ {
		changed := 0

		// 异步更新：每处理完一个节点，其新标签立刻对后续节点可见。
		// 相比同步更新，异步更新收敛更快且实践中不易陷入二元振荡。
		for _, id := range nodes {
			neighbors := adjacency[id]
			if len(neighbors) == 0 {
				continue // 孤立节点保持自身标签，自成一个社区。
			}

			// 累计每个候选标签的邻居边权之和。
			weights := make(map[string]float64, len(neighbors))
			for neighbor, weight := range neighbors {
				if _, ok := inPartition[neighbor]; !ok {
					continue // 忽略分区外邻居（StrictOwnerPartition 语义）。
				}
				weights[label[neighbor]] += weight
			}
			if len(weights) == 0 {
				continue
			}

			best := chooseBestLabel(weights)
			if best != label[id] {
				label[id] = best
				changed++
			}
		}

		if changed == 0 {
			break // 达到稳定状态。
		}
	}

	// 按最终标签归组。
	groups := make(map[string][]string)
	for _, id := range nodes {
		tag := label[id]
		groups[tag] = append(groups[tag], id)
	}

	out := make([][]string, 0, len(groups))
	for _, group := range groups {
		sort.Strings(group)
		out = append(out, group)
	}
	// 按最小成员 ID 排序，令输出顺序稳定。
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	return out
}

// chooseBestLabel 在候选标签中选出权重最大者；权重相同时取字典序最小者，
// 以此消除随机性。返回值必为候选之一。
func chooseBestLabel(weights map[string]float64) string {
	bestLabel := ""
	bestWeight := math.Inf(-1)

	for _, candidate := range sortedKeys(weights) {
		weight := weights[candidate]
		if weight > bestWeight {
			bestWeight = weight
			bestLabel = candidate
			continue
		}
		// 权重相同时 sortedKeys 已保证 candidate 字典序递增，
		// 因此首个出现的标签即为最小标签，无需额外比较。
	}
	return bestLabel
}

// Modularity 计算给定划分的模块度（Newman-Girvan），取值约在 [-0.5, 1]。
//
// 用于评估社区划分质量，并作为回归测试的稳定断言依据。
// 模块度越高，说明"社区内部的连接显著多于随机情况下的期望"。
func (m *MemorySubgraph) Modularity() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.communities) == 0 {
		return 0
	}

	// 累计无向总边权（adjacency 中每条边被记录两次，故 2m = 总度数和）。
	totalWeight := 0.0
	for _, neighbors := range m.adjacency {
		for _, weight := range neighbors {
			totalWeight += weight
		}
	}
	if totalWeight == 0 {
		return 0
	}
	twoM := totalWeight

	// 预计算每个节点的度。
	degree := make(map[string]float64, len(m.adjacency))
	for id, neighbors := range m.adjacency {
		sum := 0.0
		for _, weight := range neighbors {
			sum += weight
		}
		degree[id] = sum
	}

	modularity := 0.0
	for _, community := range m.communities {
		members := make(map[string]struct{}, len(community.Members))
		for _, id := range community.Members {
			members[id] = struct{}{}
		}

		internalWeight := 0.0
		degreeSum := 0.0
		for _, id := range community.Members {
			degreeSum += degree[id]
			for neighbor, weight := range m.adjacency[id] {
				if _, inside := members[neighbor]; inside {
					internalWeight += weight
				}
			}
		}

		// Σw_in/m  -  (Σdegree/2m)² ;  internalWeight 已计入两个方向，故直接除以 2m。
		modularity += internalWeight/twoM - math.Pow(degreeSum/twoM, 2)
	}

	return modularity
}
