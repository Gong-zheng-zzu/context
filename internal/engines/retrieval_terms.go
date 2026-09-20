package engines

import "strings"

// maxRetrievalTerms 限制兜底词元数量，避免把查询膨胀成过大的 ANY/IN 参数。
const maxRetrievalTerms = 32

// cjkStopRunes 是虚词与疑问词字符集合，用作候选片段的分隔符。
//
// 语义是"切分点"而非"黑名单"：它把连续中文切成承载实体信息的片段，
// 从而避免生成跨越虚词边界的噪声词元（例如 "为什么杨桂英..." 里的 "么杨"）。
var cjkStopRunes = map[rune]struct{}{
	'为': {}, '什': {}, '么': {}, '怎': {}, '如': {}, '何': {}, '出': {}, '现': {},
	'会': {}, '能': {}, '可': {}, '要': {}, '想': {}, '知': {}, '道': {}, '吗': {},
	'呢': {}, '吧': {}, '啊': {}, '的': {}, '了': {}, '是': {}, '在': {}, '有': {},
	'和': {}, '与': {}, '或': {}, '也': {}, '都': {}, '还': {}, '很': {}, '更': {},
}

// ExtractCJKRetrievalTerms 从自然语言查询中抽取确定性的 CJK 候选检索词。
//
// 存在意义：时间线（TimescaleDB）与知识图谱（Neo4j）的关键词通道都依赖调用方
// 提供的 KeyConcepts。当调用方不做 LLM 查询分析时该字段为空，两路只剩"整句
// 匹配"，在中文语料上必然 0 命中：
//
//   - PostgreSQL 的 simple 解析器不对中文分词，整句会被当作单个 token，
//     plainto_tsquery('simple', 整句) 无法命中库里按整段内容建立的 tsvector；
//   - Neo4j 侧条件是 node.name / description / keywords CONTAINS term，
//     传入整句同样不可能命中实体名。
//
// 抽取流程（不依赖任何外部资源或模型）：
//  1. 取出长度 >= 2 的连续 CJK 片段；
//  2. 按虚词/疑问词把片段切成候选子串，避免跨虚词边界的噪声词元；
//  3. 在每个子串内生成 2~4 字的 n-gram。
//
// 它只扩充关键词通道的 OR 条件，不改变任何既有过滤语义
// （user / session / workspace / 时间窗 / 相关度阈值均不受影响）。
func ExtractCJKRetrievalTerms(text string, maxTerms int) []string {
	if maxTerms <= 0 {
		return nil
	}

	terms := make([]string, 0, maxTerms)
	seen := make(map[string]struct{}, maxTerms)

	for _, segment := range cjkSegments(text) {
		runes := []rune(segment)
		for size := 2; size <= 4; size++ {
			if size > len(runes) {
				break
			}
			for start := 0; start+size <= len(runes); start++ {
				gram := string(runes[start : start+size])
				if _, exists := seen[gram]; exists {
					continue
				}
				seen[gram] = struct{}{}
				terms = append(terms, gram)
				if len(terms) >= maxTerms {
					return terms
				}
			}
		}
	}

	return terms
}

// ComposeCJKRetrievalTerms 依次尝试多个查询文本，返回第一个抽得出词元的结果。
func ComposeCJKRetrievalTerms(queries ...string) []string {
	for _, query := range queries {
		trimmed := strings.TrimSpace(query)
		if trimmed == "" {
			continue
		}
		if terms := ExtractCJKRetrievalTerms(trimmed, maxRetrievalTerms); len(terms) > 0 {
			return terms
		}
	}
	return nil
}

// cjkSegments 返回文本中所有长度 >= 2 的连续 CJK 子串，子串以虚词/疑问词为边界。
func cjkSegments(text string) []string {
	segments := make([]string, 0, 4)
	current := make([]rune, 0, 16)

	flush := func() {
		if len(current) >= 2 {
			segments = append(segments, string(current))
		}
		current = current[:0]
	}

	for _, r := range text {
		if !isCJKRune(r) {
			flush()
			continue
		}
		if _, isStop := cjkStopRunes[r]; isStop {
			flush()
			continue
		}
		current = append(current, r)
	}
	flush()

	return segments
}

func isCJKRune(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) || // CJK 统一表意文字
		(r >= 0x3400 && r <= 0x4DBF) || // 扩展 A
		(r >= 0xF900 && r <= 0xFAFF) // 兼容表意文字
}
