package security

import (
	"strings"
	"sync"
)

// DictionaryMatcher 词典匹配器（基于Trie树和AC自动机）
type DictionaryMatcher struct {
	root    *TrieNode
	mu      sync.RWMutex
	enabled bool
}

// TrieNode Trie树节点
type TrieNode struct {
	children map[rune]*TrieNode
	fail     *TrieNode // AC自动机失败指针
	isEnd    bool
	value    string
	category SensitiveType
}

// NewDictionaryMatcher 创建词典匹配器
func NewDictionaryMatcher() *DictionaryMatcher {
	return &DictionaryMatcher{
		root: &TrieNode{
			children: make(map[rune]*TrieNode),
		},
		enabled: true,
	}
}

// AddWord 添加敏感词
func (dm *DictionaryMatcher) AddWord(word string, category SensitiveType) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	node := dm.root
	runes := []rune(word)

	for _, r := range runes {
		if node.children[r] == nil {
			node.children[r] = &TrieNode{
				children: make(map[rune]*TrieNode),
			}
		}
		node = node.children[r]
	}

	node.isEnd = true
	node.value = word
	node.category = category
}

// AddWords 批量添加敏感词
func (dm *DictionaryMatcher) AddWords(words map[string]SensitiveType) {
	for word, category := range words {
		dm.AddWord(word, category)
	}
}

// BuildFailureLinks 构建AC自动机的失败指针
func (dm *DictionaryMatcher) BuildFailureLinks() {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	queue := make([]*TrieNode, 0)

	// 第一层节点的失败指针指向根节点
	for _, child := range dm.root.children {
		child.fail = dm.root
		queue = append(queue, child)
	}

	// BFS构建失败指针
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for char, child := range current.children {
			queue = append(queue, child)

			// 寻找失败指针
			failNode := current.fail
			for failNode != nil {
				if failNode.children[char] != nil {
					child.fail = failNode.children[char]
					break
				}
				if failNode == dm.root {
					child.fail = dm.root
					break
				}
				failNode = failNode.fail
			}
		}
	}
}

// Match 匹配文本中的敏感词
func (dm *DictionaryMatcher) Match(text string) []SensitiveInfo {
	if !dm.enabled {
		return nil
	}

	dm.mu.RLock()
	defer dm.mu.RUnlock()

	var results []SensitiveInfo
	runes := []rune(text)
	node := dm.root

	for i, r := range runes {
		// 查找匹配或失败指针
		for node != dm.root && node.children[r] == nil {
			node = node.fail
		}

		if node.children[r] != nil {
			node = node.children[r]
		}

		// 检查当前节点及其失败链上的所有匹配
		temp := node
		for temp != dm.root {
			if temp.isEnd {
				wordLen := len([]rune(temp.value))
				start := i - wordLen + 1
				end := i + 1

				results = append(results, SensitiveInfo{
					Type:       temp.category,
					Value:      temp.value,
					Start:      start,
					End:        end,
					Label:      "敏感词",
					Position:   start,
					Length:     wordLen,
					Confidence: 0.9, // 词典匹配置信度较高
					Encrypted:  false,
				})
			}
			temp = temp.fail
		}
	}

	return results
}

// LoadDefaultDictionary 加载默认敏感词词典
func (dm *DictionaryMatcher) LoadDefaultDictionary() {
	// 政治敏感词（示例）
	politicalWords := map[string]SensitiveType{
		"法轮功":   SensitiveType("political"),
		"六四":    SensitiveType("political"),
		"台独":    SensitiveType("political"),
		"藏独":    SensitiveType("political"),
		"疆独":    SensitiveType("political"),
	}

	// 暴力恐怖词汇（示例）
	violenceWords := map[string]SensitiveType{
		"炸弹":    SensitiveType("violence"),
		"恐怖袭击":  SensitiveType("violence"),
		"杀人":    SensitiveType("violence"),
		"自杀":    SensitiveType("violence"),
	}

	// 色情低俗词汇（示例）
	adultWords := map[string]SensitiveType{
		"色情":    SensitiveType("adult"),
		"黄色网站":  SensitiveType("adult"),
		"裸聊":    SensitiveType("adult"),
	}

	// 赌博诈骗词汇（示例）
	fraudWords := map[string]SensitiveType{
		"网络赌博":  SensitiveType("fraud"),
		"网络诈骗":  SensitiveType("fraud"),
		"刷单":    SensitiveType("fraud"),
		"洗钱":    SensitiveType("fraud"),
		"高利贷":   SensitiveType("fraud"),
	}

	// 毒品相关词汇（示例）
	drugWords := map[string]SensitiveType{
		"冰毒":    SensitiveType("drug"),
		"海洛因":   SensitiveType("drug"),
		"大麻":    SensitiveType("drug"),
		"摇头丸":   SensitiveType("drug"),
	}

	// 添加所有词典
	dm.AddWords(politicalWords)
	dm.AddWords(violenceWords)
	dm.AddWords(adultWords)
	dm.AddWords(fraudWords)
	dm.AddWords(drugWords)

	// 构建失败指针
	dm.BuildFailureLinks()
}

// LoadCustomDictionary 加载自定义词典
func (dm *DictionaryMatcher) LoadCustomDictionary(words map[string]SensitiveType) {
	dm.AddWords(words)
	dm.BuildFailureLinks()
}

// RemoveWord 删除敏感词
func (dm *DictionaryMatcher) RemoveWord(word string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	node := dm.root
	runes := []rune(word)

	for _, r := range runes {
		if node.children[r] == nil {
			return // 词不存在
		}
		node = node.children[r]
	}

	node.isEnd = false
	node.value = ""
}

// Clear 清空词典
func (dm *DictionaryMatcher) Clear() {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	dm.root = &TrieNode{
		children: make(map[rune]*TrieNode),
	}
}

// Enable 启用词典匹配
func (dm *DictionaryMatcher) Enable() {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.enabled = true
}

// Disable 禁用词典匹配
func (dm *DictionaryMatcher) Disable() {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.enabled = false
}

// IsEnabled 检查是否启用
func (dm *DictionaryMatcher) IsEnabled() bool {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.enabled
}

// GetWordCount 获取词典中的词数量
func (dm *DictionaryMatcher) GetWordCount() int {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.countWords(dm.root)
}

func (dm *DictionaryMatcher) countWords(node *TrieNode) int {
	count := 0
	if node.isEnd {
		count++
	}
	for _, child := range node.children {
		count += dm.countWords(child)
	}
	return count
}

// MatchWithContext 带上下文的匹配（可以过滤误报）
func (dm *DictionaryMatcher) MatchWithContext(text string, windowSize int) []SensitiveInfo {
	matches := dm.Match(text)

	// 过滤明显的误报
	filtered := make([]SensitiveInfo, 0)
	for _, match := range matches {
		// 检查上下文，判断是否为误报
		start := max(0, match.Start-windowSize)
		end := min(len(text), match.End+windowSize)
		context := text[start:end]

		// 如果上下文中包含"示例"、"测试"等词，降低置信度
		if strings.Contains(context, "示例") ||
		   strings.Contains(context, "测试") ||
		   strings.Contains(context, "example") {
			match.Confidence *= 0.5
		}

		// 只保留置信度>0.3的结果
		if match.Confidence > 0.3 {
			filtered = append(filtered, match)
		}
	}

	return filtered
}
