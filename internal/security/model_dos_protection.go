package security

import (
	"fmt"
	"sync"
	"time"
)

// RequestTracker 请求追踪器
type RequestTracker struct {
	RequestCount int       // 请求次数
	TokenCount   int       // Token使用量
	WindowStart  time.Time // 时间窗口开始时间
	LastRequest  time.Time // 最后一次请求时间
}

// ModelDosProtection 模型DoS防护
type ModelDosProtection struct {
	userRequests map[string]*RequestTracker
	mu           sync.RWMutex

	// 限制配置
	maxRequestsPerMinute int // 每分钟最大请求数
	maxTokensPerMinute   int // 每分钟最大Token数
	maxTokensPerRequest  int // 单次请求最大Token数
	windowDuration       time.Duration // 时间窗口大小
}

// NewModelDosProtection 创建模型DoS防护实例
func NewModelDosProtection() *ModelDosProtection {
	mdp := &ModelDosProtection{
		userRequests:         make(map[string]*RequestTracker),
		maxRequestsPerMinute: 20,    // 每用户每分钟20次请求
		maxTokensPerMinute:   10000, // 每用户每分钟10,000 tokens
		maxTokensPerRequest:  2000,  // 单次请求最大2,000 tokens
		windowDuration:       time.Minute,
	}

	// 启动清理协程，定期清理过期数据
	go mdp.cleanupExpiredTrackers()

	return mdp
}

// NewModelDosProtectionWithConfig 使用自定义配置创建实例
func NewModelDosProtectionWithConfig(maxReqPerMin, maxTokensPerMin, maxTokensPerReq int) *ModelDosProtection {
	mdp := &ModelDosProtection{
		userRequests:         make(map[string]*RequestTracker),
		maxRequestsPerMinute: maxReqPerMin,
		maxTokensPerMinute:   maxTokensPerMin,
		maxTokensPerRequest:  maxTokensPerReq,
		windowDuration:       time.Minute,
	}

	go mdp.cleanupExpiredTrackers()

	return mdp
}

// CheckLimit 检查用户是否超过限制
func (mdp *ModelDosProtection) CheckLimit(userID string, inputTokens int) error {
	mdp.mu.Lock()
	defer mdp.mu.Unlock()

	// 1. 检查单次请求Token限制
	if inputTokens > mdp.maxTokensPerRequest {
		return fmt.Errorf("单次请求Token数(%d)超过限制(%d)", inputTokens, mdp.maxTokensPerRequest)
	}

	// 获取或创建用户追踪器
	tracker, exists := mdp.userRequests[userID]
	if !exists {
		// 首次请求，创建新追踪器
		mdp.userRequests[userID] = &RequestTracker{
			RequestCount: 0,
			TokenCount:   0,
			WindowStart:  time.Now(),
			LastRequest:  time.Now(),
		}
		return nil // 首次请求直接通过
	}

	// 2. 检查时间窗口是否过期
	now := time.Now()
	if now.Sub(tracker.WindowStart) > mdp.windowDuration {
		// 时间窗口过期，重置计数器
		tracker.RequestCount = 0
		tracker.TokenCount = 0
		tracker.WindowStart = now
		tracker.LastRequest = now
		return nil
	}

	// 3. 检查请求频率限制
	if tracker.RequestCount >= mdp.maxRequestsPerMinute {
		remainingTime := mdp.windowDuration - now.Sub(tracker.WindowStart)
		return fmt.Errorf("请求频率超限：已达到%d次/分钟上限，请在%d秒后重试",
			mdp.maxRequestsPerMinute, int(remainingTime.Seconds()))
	}

	// 4. 检查Token使用量限制
	if tracker.TokenCount+inputTokens > mdp.maxTokensPerMinute {
		remainingTime := mdp.windowDuration - now.Sub(tracker.WindowStart)
		return fmt.Errorf("Token使用量超限：已使用%d tokens，本次请求%d tokens，超过%d tokens/分钟上限，请在%d秒后重试",
			tracker.TokenCount, inputTokens, mdp.maxTokensPerMinute, int(remainingTime.Seconds()))
	}

	// 5. 检查请求间隔（防止突发请求）
	minInterval := time.Second // 最小请求间隔1秒
	if now.Sub(tracker.LastRequest) < minInterval {
		return fmt.Errorf("请求过于频繁，请至少间隔%d秒", int(minInterval.Seconds()))
	}

	return nil
}

// RecordRequest 记录用户请求
func (mdp *ModelDosProtection) RecordRequest(userID string, tokens int) {
	mdp.mu.Lock()
	defer mdp.mu.Unlock()

	tracker, exists := mdp.userRequests[userID]
	if !exists {
		// 创建新追踪器
		mdp.userRequests[userID] = &RequestTracker{
			RequestCount: 1,
			TokenCount:   tokens,
			WindowStart:  time.Now(),
			LastRequest:  time.Now(),
		}
		return
	}

	// 检查时间窗口
	now := time.Now()
	if now.Sub(tracker.WindowStart) > mdp.windowDuration {
		// 时间窗口过期，重置计数器
		tracker.RequestCount = 1
		tracker.TokenCount = tokens
		tracker.WindowStart = now
	} else {
		// 累加计数
		tracker.RequestCount++
		tracker.TokenCount += tokens
	}

	tracker.LastRequest = now
}

// GetUserStats 获取用户统计信息
func (mdp *ModelDosProtection) GetUserStats(userID string) (requests int, tokens int, windowStart time.Time, exists bool) {
	mdp.mu.RLock()
	defer mdp.mu.RUnlock()

	tracker, exists := mdp.userRequests[userID]
	if !exists {
		return 0, 0, time.Time{}, false
	}

	// 检查时间窗口是否过期
	if time.Now().Sub(tracker.WindowStart) > mdp.windowDuration {
		return 0, 0, time.Time{}, false
	}

	return tracker.RequestCount, tracker.TokenCount, tracker.WindowStart, true
}

// GetRemainingQuota 获取用户剩余配额
func (mdp *ModelDosProtection) GetRemainingQuota(userID string) (remainingRequests int, remainingTokens int) {
	mdp.mu.RLock()
	defer mdp.mu.RUnlock()

	tracker, exists := mdp.userRequests[userID]
	if !exists {
		return mdp.maxRequestsPerMinute, mdp.maxTokensPerMinute
	}

	// 检查时间窗口是否过期
	if time.Now().Sub(tracker.WindowStart) > mdp.windowDuration {
		return mdp.maxRequestsPerMinute, mdp.maxTokensPerMinute
	}

	remainingRequests = mdp.maxRequestsPerMinute - tracker.RequestCount
	if remainingRequests < 0 {
		remainingRequests = 0
	}

	remainingTokens = mdp.maxTokensPerMinute - tracker.TokenCount
	if remainingTokens < 0 {
		remainingTokens = 0
	}

	return remainingRequests, remainingTokens
}

// ResetUserQuota 重置用户配额（管理员功能）
func (mdp *ModelDosProtection) ResetUserQuota(userID string) {
	mdp.mu.Lock()
	defer mdp.mu.Unlock()

	delete(mdp.userRequests, userID)
}

// GetAllUserStats 获取所有用户统计（管理员功能）
func (mdp *ModelDosProtection) GetAllUserStats() map[string]*RequestTracker {
	mdp.mu.RLock()
	defer mdp.mu.RUnlock()

	// 复制一份数据返回
	stats := make(map[string]*RequestTracker)
	now := time.Now()

	for userID, tracker := range mdp.userRequests {
		// 只返回有效时间窗口内的数据
		if now.Sub(tracker.WindowStart) <= mdp.windowDuration {
			stats[userID] = &RequestTracker{
				RequestCount: tracker.RequestCount,
				TokenCount:   tracker.TokenCount,
				WindowStart:  tracker.WindowStart,
				LastRequest:  tracker.LastRequest,
			}
		}
	}

	return stats
}

// cleanupExpiredTrackers 清理过期的追踪器（后台任务）
func (mdp *ModelDosProtection) cleanupExpiredTrackers() {
	ticker := time.NewTicker(5 * time.Minute) // 每5分钟清理一次
	defer ticker.Stop()

	for range ticker.C {
		mdp.mu.Lock()

		now := time.Now()
		expiredUsers := []string{}

		// 找出过期的用户
		for userID, tracker := range mdp.userRequests {
			// 如果超过2个时间窗口没有活动，则清理
			if now.Sub(tracker.LastRequest) > 2*mdp.windowDuration {
				expiredUsers = append(expiredUsers, userID)
			}
		}

		// 删除过期用户
		for _, userID := range expiredUsers {
			delete(mdp.userRequests, userID)
		}

		mdp.mu.Unlock()

		if len(expiredUsers) > 0 {
			// 可以记录日志
			// log.Printf("[DoS防护] 清理了%d个过期用户追踪器", len(expiredUsers))
		}
	}
}

// IsUserBlocked 检查用户是否被阻止（超过限制）
func (mdp *ModelDosProtection) IsUserBlocked(userID string) bool {
	mdp.mu.RLock()
	defer mdp.mu.RUnlock()

	tracker, exists := mdp.userRequests[userID]
	if !exists {
		return false
	}

	// 检查时间窗口
	now := time.Now()
	if now.Sub(tracker.WindowStart) > mdp.windowDuration {
		return false
	}

	// 检查是否超过限制
	return tracker.RequestCount >= mdp.maxRequestsPerMinute ||
		tracker.TokenCount >= mdp.maxTokensPerMinute
}

// GetConfig 获取当前配置
func (mdp *ModelDosProtection) GetConfig() map[string]int {
	return map[string]int{
		"max_requests_per_minute": mdp.maxRequestsPerMinute,
		"max_tokens_per_minute":   mdp.maxTokensPerMinute,
		"max_tokens_per_request":  mdp.maxTokensPerRequest,
	}
}

// UpdateConfig 更新配置（管理员功能）
func (mdp *ModelDosProtection) UpdateConfig(maxReqPerMin, maxTokensPerMin, maxTokensPerReq int) {
	mdp.mu.Lock()
	defer mdp.mu.Unlock()

	if maxReqPerMin > 0 {
		mdp.maxRequestsPerMinute = maxReqPerMin
	}
	if maxTokensPerMin > 0 {
		mdp.maxTokensPerMinute = maxTokensPerMin
	}
	if maxTokensPerReq > 0 {
		mdp.maxTokensPerRequest = maxTokensPerReq
	}
}

// EstimateTokens 估算文本的Token数量（简化版）
func EstimateTokens(text string) int {
	// 简化的Token估算：
	// 英文：约4个字符 = 1个token
	// 中文：约1.5个字符 = 1个token
	// 这是一个粗略估算，实际应该使用tokenizer

	chineseCount := 0
	totalChars := len([]rune(text))

	for _, r := range text {
		// 检查是否是中文字符
		if r >= 0x4E00 && r <= 0x9FFF {
			chineseCount++
		}
	}

	englishCount := totalChars - chineseCount

	// 估算Token数
	tokens := (englishCount / 4) + (chineseCount * 2 / 3)

	if tokens < 1 {
		tokens = 1
	}

	return tokens
}
