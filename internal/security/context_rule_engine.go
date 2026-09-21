package security

import (
	"encoding/json"
	"math"
	"regexp"
	"strings"
	"sync"
)

// ContextRuleEngine 上下文规则引擎
type ContextRuleEngine struct {
	rules   []*ContextRule
	mu      sync.RWMutex
	enabled bool
}

// ContextRule 上下文规则
type ContextRule struct {
	ID           string              `json:"id"`
	Type         SensitiveType       `json:"type"`
	Pattern      string              `json:"pattern"`
	Regex        *regexp.Regexp      `json:"-"`
	Triggers     []string            `json:"triggers"`
	Suppressors  []string            `json:"suppressors"`
	WindowSize   int                 `json:"window_size"`
	Associations []Association       `json:"associations"`
	BaseScore    float64             `json:"base_score"`
	Priority     int                 `json:"priority"`
}

// Association 实体关联规则
type Association struct {
	EntityType string  `json:"entity_type"` // PERSON, LOCATION, ORGANIZATION
	Distance   int     `json:"distance"`    // 最大距离（字符数）
	Boost      float64 `json:"boost"`       // 置信度加成
}

// ContextMatch 上下文匹配结果
type ContextMatch struct {
	SensitiveInfo
	TriggersFound    []string  `json:"triggers_found"`
	SuppressorsFound []string  `json:"suppressors_found"`
	AssociationsFound []string `json:"associations_found"`
	ContextWindow    string    `json:"context_window"`
	RawScore         float64   `json:"raw_score"`
}

// NewContextRuleEngine 创建上下文规则引擎
func NewContextRuleEngine() *ContextRuleEngine {
	engine := &ContextRuleEngine{
		rules:   make([]*ContextRule, 0),
		enabled: true,
	}

	// 加载默认规则
	engine.LoadDefaultRules()

	return engine
}

// AddRule 添加规则
func (cre *ContextRuleEngine) AddRule(rule *ContextRule) error {
	// 编译正则表达式
	regex, err := regexp.Compile(rule.Pattern)
	if err != nil {
		return err
	}
	rule.Regex = regex

	cre.mu.Lock()
	defer cre.mu.Unlock()

	cre.rules = append(cre.rules, rule)

	// 按优先级排序
	cre.sortRulesByPriority()

	return nil
}

// sortRulesByPriority 按优先级排序规则
func (cre *ContextRuleEngine) sortRulesByPriority() {
	// 简单的冒泡排序（规则数量不多）
	for i := 0; i < len(cre.rules); i++ {
		for j := i + 1; j < len(cre.rules); j++ {
			if cre.rules[i].Priority < cre.rules[j].Priority {
				cre.rules[i], cre.rules[j] = cre.rules[j], cre.rules[i]
			}
		}
	}
}

// Analyze 分析文本
func (cre *ContextRuleEngine) Analyze(text string, nerResults []NERResult) []ContextMatch {
	if !cre.enabled {
		return nil
	}

	cre.mu.RLock()
	defer cre.mu.RUnlock()

	var matches []ContextMatch

	for _, rule := range cre.rules {
		ruleMatches := cre.applyRule(text, rule, nerResults)
		matches = append(matches, ruleMatches...)
	}

	return matches
}

// applyRule 应用单个规则
func (cre *ContextRuleEngine) applyRule(text string, rule *ContextRule, nerResults []NERResult) []ContextMatch {
	var matches []ContextMatch

	// 使用正则表达式查找所有匹配
	regexMatches := rule.Regex.FindAllStringIndex(text, -1)

	for _, match := range regexMatches {
		start := match[0]
		end := match[1]
		value := text[start:end]

		// 提取上下文窗口
		windowStart := max(0, start-rule.WindowSize)
		windowEnd := min(len(text), end+rule.WindowSize)
		contextWindow := text[windowStart:windowEnd]

		// 计算置信度
		score, triggersFound, suppressorsFound := cre.calculateScore(
			contextWindow,
			rule,
		)

		// 检查实体关联
		associationsFound := cre.checkAssociations(
			text,
			start,
			end,
			rule.Associations,
			nerResults,
		)

		// 应用关联加成
		for _, assoc := range associationsFound {
			for _, ruleAssoc := range rule.Associations {
				if assoc == ruleAssoc.EntityType {
					score += ruleAssoc.Boost
				}
			}
		}

		// 限制置信度在0-1之间
		score = math.Max(0, math.Min(1, score))

		// 只保留置信度>0.5的结果
		if score > 0.5 {
			matches = append(matches, ContextMatch{
				SensitiveInfo: SensitiveInfo{
					Type:       rule.Type,
					Value:      value,
					Start:      start,
					End:        end,
					Label:      string(rule.Type),
					Position:   start,
					Length:     end - start,
					Confidence: score,
					Encrypted:  false,
				},
				TriggersFound:     triggersFound,
				SuppressorsFound:  suppressorsFound,
				AssociationsFound: associationsFound,
				ContextWindow:     contextWindow,
				RawScore:          score,
			})
		}
	}

	return matches
}

// calculateScore 计算置信度分数
func (cre *ContextRuleEngine) calculateScore(
	context string,
	rule *ContextRule,
) (float64, []string, []string) {
	score := rule.BaseScore
	triggersFound := make([]string, 0)
	suppressorsFound := make([]string, 0)

	contextLower := strings.ToLower(context)

	// 检查触发词
	for _, trigger := range rule.Triggers {
		if strings.Contains(contextLower, strings.ToLower(trigger)) {
			score += 0.1
			triggersFound = append(triggersFound, trigger)
		}
	}

	// 检查抑制词
	for _, suppressor := range rule.Suppressors {
		if strings.Contains(contextLower, strings.ToLower(suppressor)) {
			score -= 0.2
			suppressorsFound = append(suppressorsFound, suppressor)
		}
	}

	return score, triggersFound, suppressorsFound
}

// checkAssociations 检查实体关联
func (cre *ContextRuleEngine) checkAssociations(
	text string,
	matchStart int,
	matchEnd int,
	associations []Association,
	nerResults []NERResult,
) []string {
	found := make([]string, 0)

	for _, assoc := range associations {
		for _, ner := range nerResults {
			if ner.Type != assoc.EntityType {
				continue
			}

			// 计算距离
			distance := 0
			if ner.End < matchStart {
				distance = matchStart - ner.End
			} else if ner.Start > matchEnd {
				distance = ner.Start - matchEnd
			}

			// 如果在距离范围内
			if distance <= assoc.Distance {
				found = append(found, assoc.EntityType)
				break
			}
		}
	}

	return found
}

// LoadDefaultRules 加载默认规则
func (cre *ContextRuleEngine) LoadDefaultRules() {
	// 规则1：密码上下文检测
	cre.AddRule(&ContextRule{
		ID:       "pwd_context_001",
		Type:     SensitiveTypePassword,
		Pattern:  `[A-Za-z0-9@#$%^&*!]{6,}`,
		Triggers: []string{"密码", "password", "pwd", "口令", "密钥", "凭证"},
		Suppressors: []string{"字段", "变量", "单词", "英文", "示例", "example"},
		WindowSize:  20,
		BaseScore:   0.6,
		Priority:    10,
	})

	// 规则2：手机号关联检测
	cre.AddRule(&ContextRule{
		ID:       "phone_assoc_002",
		Type:     SensitiveTypePhone,
		// 加 \b 边界并使用与 Layer 1 一致的号段（含 19 全号段），避免在身份证号等
		// 更长数字串内部误匹配出一个 11 位子串（如 230103193205078321 里的 19320507832）。
		Pattern:  `\b1(?:3\d|4[5-9]|5[0-35-9]|6[2567]|7[0-8]|8\d|9[0-9])\d{8}\b`,
		Triggers: []string{"手机", "电话", "联系方式", "联系人", "号码"},
		Suppressors: []string{"示例", "测试", "假设"},
		WindowSize:  15,
		Associations: []Association{
			{EntityType: "PERSON", Distance: 15, Boost: 0.2},
		},
		BaseScore: 0.7,
		Priority:  9,
	})

	// 规则3：身份证关联检测
	cre.AddRule(&ContextRule{
		ID:       "idcard_assoc_003",
		Type:     SensitiveTypeIDCard,
		Pattern:  `[1-9]\d{5}(18|19|20)\d{2}(0[1-9]|1[0-2])(0[1-9]|[12]\d|3[01])\d{3}[\dXx]`,
		Triggers: []string{"身份证", "身份证号", "证件号", "ID"},
		Suppressors: []string{"示例", "测试", "假设"},
		WindowSize:  20,
		Associations: []Association{
			{EntityType: "PERSON", Distance: 20, Boost: 0.25},
		},
		BaseScore: 0.75,
		Priority:  9,
	})

	// 规则4：邮箱关联检测
	cre.AddRule(&ContextRule{
		ID:       "email_assoc_004",
		Type:     SensitiveTypeEmail,
		Pattern:  `[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`,
		Triggers: []string{"邮箱", "email", "邮件", "联系方式"},
		Suppressors: []string{"示例", "测试", "example@"},
		WindowSize:  15,
		Associations: []Association{
			{EntityType: "PERSON", Distance: 15, Boost: 0.15},
			{EntityType: "ORGANIZATION", Distance: 10, Boost: 0.1},
		},
		BaseScore: 0.7,
		Priority:  8,
	})

	// 规则5：银行卡关联检测
	cre.AddRule(&ContextRule{
		ID:       "bankcard_assoc_005",
		Type:     SensitiveTypeBankCard,
		// 使用与 Layer 1 一致的 BIN 号段锚定（62/4/5/6/9558 开头），而不是裸
		// [1-9]\d{15,18}：后者会把 18 位身份证号、订单号、快递单号等任意长数字
		// 串误判为银行卡（实测 230103193205078321 被误报）。
		Pattern:  `\b(?:62\d{14,17}|4\d{15}|5[1-5]\d{14}|6(?:011|5\d{2})\d{12,15}|9558\d{15})\b`,
		Triggers: []string{"银行卡", "卡号", "账号", "账户"},
		Suppressors: []string{"示例", "测试", "假设"},
		WindowSize:  20,
		Associations: []Association{
			{EntityType: "PERSON", Distance: 20, Boost: 0.2},
		},
		BaseScore: 0.65,
		Priority:  8,
	})

	// 规则6：API密钥上下文检测
	cre.AddRule(&ContextRule{
		ID:       "apikey_context_006",
		Type:     SensitiveTypeAPIKey,
		Pattern:  `(sk|pk|api)[-_]?[a-zA-Z0-9]{20,}`,
		Triggers: []string{"api", "key", "密钥", "token", "令牌"},
		Suppressors: []string{"示例", "example", "测试"},
		WindowSize:  25,
		BaseScore:   0.75,
		Priority:    10,
	})

	// 规则7：地址关联检测
	cre.AddRule(&ContextRule{
		ID:       "address_assoc_007",
		Type:     SensitiveType("address"),
		Pattern:  `[一-龥]{2,}(省|市|区|县|镇|街道|路|号|室|栋|楼)[一-龥\d]{2,}`,
		Triggers: []string{"地址", "住址", "居住", "位于"},
		Suppressors: []string{"示例", "测试"},
		WindowSize:  20,
		Associations: []Association{
			{EntityType: "PERSON", Distance: 25, Boost: 0.2},
			{EntityType: "LOCATION", Distance: 10, Boost: 0.15},
		},
		BaseScore: 0.6,
		Priority:  7,
	})
}

// LoadRulesFromJSON 从JSON加载规则
func (cre *ContextRuleEngine) LoadRulesFromJSON(jsonData []byte) error {
	var rules []*ContextRule
	if err := json.Unmarshal(jsonData, &rules); err != nil {
		return err
	}

	for _, rule := range rules {
		if err := cre.AddRule(rule); err != nil {
			return err
		}
	}

	return nil
}

// ExportRulesToJSON 导出规则为JSON
func (cre *ContextRuleEngine) ExportRulesToJSON() ([]byte, error) {
	cre.mu.RLock()
	defer cre.mu.RUnlock()

	return json.MarshalIndent(cre.rules, "", "  ")
}

// Enable 启用规则引擎
func (cre *ContextRuleEngine) Enable() {
	cre.mu.Lock()
	defer cre.mu.Unlock()
	cre.enabled = true
}

// Disable 禁用规则引擎
func (cre *ContextRuleEngine) Disable() {
	cre.mu.Lock()
	defer cre.mu.Unlock()
	cre.enabled = false
}

// GetRuleCount 获取规则数量
func (cre *ContextRuleEngine) GetRuleCount() int {
	cre.mu.RLock()
	defer cre.mu.RUnlock()
	return len(cre.rules)
}

// RemoveRule 删除规则
func (cre *ContextRuleEngine) RemoveRule(ruleID string) {
	cre.mu.Lock()
	defer cre.mu.Unlock()

	for i, rule := range cre.rules {
		if rule.ID == ruleID {
			cre.rules = append(cre.rules[:i], cre.rules[i+1:]...)
			break
		}
	}
}

// NERResult NER识别结果（占位符，等待Layer 3实现）
type NERResult struct {
	Text       string
	Type       string // PERSON, LOCATION, ORGANIZATION
	Start      int
	End        int
	Confidence float64
}
