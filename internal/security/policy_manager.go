package security

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// 类型定义已移至 types.go

// PolicyRule 策略规则
type PolicyRule struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	SensitiveTypes  []SensitiveType `json:"sensitive_types"`
	SecurityLevel   SecurityLevel   `json:"security_level"`
	Actions         []Action        `json:"actions"`
	Enabled         bool            `json:"enabled"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	ThresholdCount  int             `json:"threshold_count"`  // 触发阈值
	ThresholdWindow int             `json:"threshold_window"` // 时间窗口（秒）
}

// PolicyManager 策略管理器
type PolicyManager struct {
	mu           sync.RWMutex
	policies     map[string]*PolicyRule
	configPath   string
	defaultRules []*PolicyRule
}

// NewPolicyManager 创建策略管理器
func NewPolicyManager(configPath string) *PolicyManager {
	pm := &PolicyManager{
		policies:   make(map[string]*PolicyRule),
		configPath: configPath,
	}
	pm.initDefaultRules()
	pm.loadPolicies()
	return pm
}

// initDefaultRules 初始化默认规则
func (pm *PolicyManager) initDefaultRules() {
	pm.defaultRules = []*PolicyRule{
		{
			ID:          "rule-001",
			Name:        "高危凭证检测",
			Description: "检测 API 密钥、密码、私钥等高危凭证",
			SensitiveTypes: []SensitiveType{
				SensitiveTypeAPIKey,
				SensitiveTypePassword,
				SensitiveTypePrivateKey,
				SensitiveTypeAWSKey,
				SensitiveTypeSecretKey,
			},
			SecurityLevel:   SecurityLevelHigh,
			Actions:         []Action{ActionLog, ActionRedact, ActionAlert},
			Enabled:         true,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
			ThresholdCount:  1,
			ThresholdWindow: 60,
		},
		{
			ID:          "rule-002",
			Name:        "认证令牌检测",
			Description: "检测各类认证令牌",
			SensitiveTypes: []SensitiveType{
				SensitiveTypeToken,
				SensitiveTypeBearerToken,
			},
			SecurityLevel:   SecurityLevelMedium,
			Actions:         []Action{ActionLog, ActionRedact},
			Enabled:         true,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
			ThresholdCount:  3,
			ThresholdWindow: 300,
		},
		{
			ID:          "rule-003",
			Name:        "个人信息检测",
			Description: "检测身份证、银行卡、信用卡、手机号、邮箱等个人敏感信息",
			SensitiveTypes: []SensitiveType{
				SensitiveTypeIDCard,
				SensitiveTypeBankCard,
				SensitiveTypeCreditCard,
				SensitiveTypeSSN,
				SensitiveTypePhone,
				SensitiveTypeEmail,
			},
			SecurityLevel:   SecurityLevelMedium,
			Actions:         []Action{ActionLog, ActionRedact},
			Enabled:         true,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
			ThresholdCount:  5,
			ThresholdWindow: 600,
		},
		{
			ID:          "rule-004",
			Name:        "数据库凭证检测",
			Description: "检测数据库连接字符串和凭证",
			SensitiveTypes: []SensitiveType{
				SensitiveTypeDatabase,
			},
			SecurityLevel:   SecurityLevelHigh,
			Actions:         []Action{ActionLog, ActionRedact, ActionAlert},
			Enabled:         true,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
			ThresholdCount:  1,
			ThresholdWindow: 60,
		},
	}
}

// loadPolicies 从配置文件加载策略
func (pm *PolicyManager) loadPolicies() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// 如果配置文件不存在，使用默认规则
	if _, err := os.Stat(pm.configPath); os.IsNotExist(err) {
		for _, rule := range pm.defaultRules {
			pm.policies[rule.ID] = rule
		}
		return pm.savePoliciesLocked()
	}

	// 读取配置文件
	data, err := os.ReadFile(pm.configPath)
	if err != nil {
		return fmt.Errorf("failed to read policy config: %w", err)
	}

	var policies []*PolicyRule
	if err := json.Unmarshal(data, &policies); err != nil {
		return fmt.Errorf("failed to parse policy config: %w", err)
	}

	pm.policies = make(map[string]*PolicyRule)
	for _, policy := range policies {
		pm.policies[policy.ID] = policy
	}

	return nil
}

// savePoliciesLocked 保存策略到配置文件（需要持有锁）
func (pm *PolicyManager) savePoliciesLocked() error {
	// 确保目录存在
	dir := filepath.Dir(pm.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	var policies []*PolicyRule
	for _, policy := range pm.policies {
		policies = append(policies, policy)
	}

	data, err := json.MarshalIndent(policies, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal policies: %w", err)
	}

	if err := os.WriteFile(pm.configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write policy config: %w", err)
	}

	return nil
}

// SavePolicies 保存策略到配置文件
func (pm *PolicyManager) SavePolicies() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.savePoliciesLocked()
}

// GetPolicy 获取指定策略
func (pm *PolicyManager) GetPolicy(id string) (*PolicyRule, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	policy, exists := pm.policies[id]
	if !exists {
		return nil, fmt.Errorf("policy not found: %s", id)
	}

	return policy, nil
}

// GetAllPolicies 获取所有策略
func (pm *PolicyManager) GetAllPolicies() []*PolicyRule {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	policies := make([]*PolicyRule, 0, len(pm.policies))
	for _, policy := range pm.policies {
		policies = append(policies, policy)
	}

	return policies
}

// GetEnabledPolicies 获取所有启用的策略
func (pm *PolicyManager) GetEnabledPolicies() []*PolicyRule {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var policies []*PolicyRule
	for _, policy := range pm.policies {
		if policy.Enabled {
			policies = append(policies, policy)
		}
	}

	return policies
}

// AddPolicy 添加新策略
func (pm *PolicyManager) AddPolicy(policy *PolicyRule) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, exists := pm.policies[policy.ID]; exists {
		return fmt.Errorf("policy already exists: %s", policy.ID)
	}

	policy.CreatedAt = time.Now()
	policy.UpdatedAt = time.Now()
	pm.policies[policy.ID] = policy

	return pm.savePoliciesLocked()
}

// UpdatePolicy 更新策略
func (pm *PolicyManager) UpdatePolicy(policy *PolicyRule) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, exists := pm.policies[policy.ID]; !exists {
		return fmt.Errorf("policy not found: %s", policy.ID)
	}

	policy.UpdatedAt = time.Now()
	pm.policies[policy.ID] = policy

	return pm.savePoliciesLocked()
}

// DeletePolicy 删除策略
func (pm *PolicyManager) DeletePolicy(id string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, exists := pm.policies[id]; !exists {
		return fmt.Errorf("policy not found: %s", id)
	}

	delete(pm.policies, id)

	return pm.savePoliciesLocked()
}

// EnablePolicy 启用策略
func (pm *PolicyManager) EnablePolicy(id string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	policy, exists := pm.policies[id]
	if !exists {
		return fmt.Errorf("policy not found: %s", id)
	}

	policy.Enabled = true
	policy.UpdatedAt = time.Now()

	return pm.savePoliciesLocked()
}

// DisablePolicy 禁用策略
func (pm *PolicyManager) DisablePolicy(id string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	policy, exists := pm.policies[id]
	if !exists {
		return fmt.Errorf("policy not found: %s", id)
	}

	policy.Enabled = false
	policy.UpdatedAt = time.Now()

	return pm.savePoliciesLocked()
}

// MatchPolicies 匹配敏感信息对应的策略
func (pm *PolicyManager) MatchPolicies(sensitiveTypes []SensitiveType) []*PolicyRule {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var matched []*PolicyRule
	typeSet := make(map[SensitiveType]bool)
	for _, t := range sensitiveTypes {
		typeSet[t] = true
	}

	for _, policy := range pm.policies {
		if !policy.Enabled {
			continue
		}

		for _, policyType := range policy.SensitiveTypes {
			if typeSet[policyType] {
				matched = append(matched, policy)
				break
			}
		}
	}

	return matched
}

// GetActionsForSensitiveType 获取敏感类型对应的动作
func (pm *PolicyManager) GetActionsForSensitiveType(sensitiveType SensitiveType) []Action {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	actionSet := make(map[Action]bool)

	for _, policy := range pm.policies {
		if !policy.Enabled {
			continue
		}

		for _, policyType := range policy.SensitiveTypes {
			if policyType == sensitiveType {
				for _, action := range policy.Actions {
					actionSet[action] = true
				}
				break
			}
		}
	}

	var actions []Action
	for action := range actionSet {
		actions = append(actions, action)
	}

	return actions
}
