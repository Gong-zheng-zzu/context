package causal_reasoning

import (
	"sort"
	"strings"
	"sync"
)

// RuleEngine 医学因果规则引擎
type RuleEngine struct {
	rules map[string]*MedicalRule
	mu    sync.RWMutex
}

// NewRuleEngine 创建规则引擎实例
func NewRuleEngine() *RuleEngine {
	engine := &RuleEngine{
		rules: make(map[string]*MedicalRule),
	}
	engine.loadDefaultRules()
	return engine
}

// loadDefaultRules 加载默认医学规则库
func (re *RuleEngine) loadDefaultRules() {
	defaultRules := []*MedicalRule{
		{
			ID:         "rule_001",
			Condition:  "降压药",
			Effect:     "体位性低血压",
			Confidence: 0.95,
			Category:   "药物副作用",
			Keywords:   []string{"降压药", "血压药", "抗高血压药"},
			Source:     "临床指南",
		},
		{
			ID:         "rule_002",
			Condition:  "体位性低血压",
			Effect:     "跌倒",
			Confidence: 0.88,
			Category:   "疾病后果",
			Keywords:   []string{"体位性低血压", "直立性低血压", "姿势性低血压"},
			Source:     "临床研究",
		},
		{
			ID:         "rule_003",
			Condition:  "镇静剂",
			Effect:     "意识模糊",
			Confidence: 0.92,
			Category:   "药物副作用",
			Keywords:   []string{"镇静剂", "安眠药", "苯二氮䓬类"},
			Source:     "药品说明书",
		},
		{
			ID:         "rule_004",
			Condition:  "意识模糊",
			Effect:     "跌倒",
			Confidence: 0.85,
			Category:   "疾病后果",
			Keywords:   []string{"意识模糊", "嗜睡", "精神状态改变"},
			Source:     "临床观察",
		},
		{
			ID:         "rule_005",
			Condition:  "糖尿病",
			Effect:     "周围神经病变",
			Confidence: 0.78,
			Category:   "疾病并发症",
			Keywords:   []string{"糖尿病", "高血糖", "血糖控制不佳"},
			Source:     "临床指南",
		},
		{
			ID:         "rule_006",
			Condition:  "周围神经病变",
			Effect:     "足部溃疡",
			Confidence: 0.82,
			Category:   "疾病后果",
			Keywords:   []string{"周围神经病变", "感觉减退", "神经损伤"},
			Source:     "临床研究",
		},
		{
			ID:         "rule_007",
			Condition:  "抗凝药",
			Effect:     "出血风险",
			Confidence: 0.90,
			Category:   "药物副作用",
			Keywords:   []string{"抗凝药", "华法林", "利伐沙班"},
			Source:     "药品说明书",
		},
		{
			ID:         "rule_008",
			Condition:  "长期卧床",
			Effect:     "压疮",
			Confidence: 0.87,
			Category:   "操作风险",
			Keywords:   []string{"长期卧床", "制动", "卧床不起"},
			Source:     "护理指南",
		},
		{
			ID:         "rule_009",
			Condition:  "利尿剂",
			Effect:     "电解质紊乱",
			Confidence: 0.83,
			Category:   "药物副作用",
			Keywords:   []string{"利尿剂", "呋塞米", "氢氯噻嗪"},
			Source:     "药品说明书",
		},
		{
			ID:         "rule_010",
			Condition:  "电解质紊乱",
			Effect:     "心律失常",
			Confidence: 0.86,
			Category:   "疾病后果",
			Keywords:   []string{"电解质紊乱", "低钾血症", "高钾血症"},
			Source:     "临床指南",
		},
	}

	for _, rule := range defaultRules {
		re.rules[rule.ID] = rule
	}
}

// AddRule 添加新规则
func (re *RuleEngine) AddRule(rule *MedicalRule) {
	re.mu.Lock()
	defer re.mu.Unlock()
	re.rules[rule.ID] = rule
}

// GetRule 获取规则
func (re *RuleEngine) GetRule(id string) (*MedicalRule, bool) {
	re.mu.RLock()
	defer re.mu.RUnlock()
	rule, exists := re.rules[id]
	return rule, exists
}

// MatchRules 匹配文本中的规则
func (re *RuleEngine) MatchRules(text string) []*MedicalRule {
	re.mu.RLock()
	defer re.mu.RUnlock()

	textLower := strings.ToLower(text)
	var matched []*MedicalRule

	for _, rule := range re.rules {
		for _, keyword := range rule.Keywords {
			if strings.Contains(textLower, strings.ToLower(keyword)) {
				matched = append(matched, rule)
				break
			}
		}
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].ID < matched[j].ID })
	return matched
}

// MatchConditionEffect 匹配条件-效应对
func (re *RuleEngine) MatchConditionEffect(condition, effect string) (*MedicalRule, float64) {
	re.mu.RLock()
	defer re.mu.RUnlock()

	conditionLower := strings.ToLower(condition)
	effectLower := strings.ToLower(effect)

	for _, rule := range re.rules {
		conditionMatch := false
		for _, keyword := range rule.Keywords {
			if strings.Contains(conditionLower, strings.ToLower(keyword)) {
				conditionMatch = true
				break
			}
		}

		if conditionMatch && strings.Contains(effectLower, strings.ToLower(rule.Effect)) {
			return rule, rule.Confidence
		}
	}

	return nil, 0.0
}

// GetRulesByCategory 获取特定类别的规则
func (re *RuleEngine) GetRulesByCategory(category string) []*MedicalRule {
	re.mu.RLock()
	defer re.mu.RUnlock()

	var rules []*MedicalRule
	for _, rule := range re.rules {
		if rule.Category == category {
			rules = append(rules, rule)
		}
	}

	return rules
}

// GetAllRules 获取所有规则
func (re *RuleEngine) GetAllRules() []*MedicalRule {
	re.mu.RLock()
	defer re.mu.RUnlock()

	rules := make([]*MedicalRule, 0, len(re.rules))
	for _, rule := range re.rules {
		rules = append(rules, rule)
	}

	return rules
}

// RemoveRule 删除规则
func (re *RuleEngine) RemoveRule(id string) bool {
	re.mu.Lock()
	defer re.mu.Unlock()

	if _, exists := re.rules[id]; exists {
		delete(re.rules, id)
		return true
	}
	return false
}

// Count 返回规则总数
func (re *RuleEngine) Count() int {
	re.mu.RLock()
	defer re.mu.RUnlock()
	return len(re.rules)
}
