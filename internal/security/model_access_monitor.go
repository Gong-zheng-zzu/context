package security

import (
	"sync"
	"time"
)

// AccessStats 访问统计信息
type AccessStats struct {
	TotalRequests   int       // 总请求数
	TotalTokens     int       // 总Token数
	HourlyRequests  int       // 每小时请求数
	HourlyTokens    int       // 每小时Token数
	LastHourStart   time.Time // 当前小时开始时间
	FirstAccessTime time.Time // 首次访问时间
	LastAccessTime  time.Time // 最后访问时间
	SuspiciousCount int       // 可疑行为计数
}

// ModelAccessMonitor 模型访问监控器
type ModelAccessMonitor struct {
	userStats map[string]*AccessStats
	mu        sync.RWMutex

	// 阈值配置
	maxRequestsPerHour int // 每小时最大请求数
	maxTokensPerHour   int // 每小时最大Token数
	theftThreshold     int // 模型窃取阈值（连续可疑行为次数）
}

// NewModelAccessMonitor 创建模型访问监控器
func NewModelAccessMonitor() *ModelAccessMonitor {
	mam := &ModelAccessMonitor{
		userStats:          make(map[string]*AccessStats),
		maxRequestsPerHour: 100,   // 每小时100次请求
		maxTokensPerHour:   50000, // 每小时50,000 tokens
		theftThreshold:     5,     // 连续5次可疑行为判定为窃取
	}

	// 启动后台清理任务
	go mam.cleanupOldStats()

	return mam
}

// NewModelAccessMonitorWithConfig 使用自定义配置创建监控器
func NewModelAccessMonitorWithConfig(maxReqPerHour, maxTokensPerHour, theftThreshold int) *ModelAccessMonitor {
	mam := &ModelAccessMonitor{
		userStats:          make(map[string]*AccessStats),
		maxRequestsPerHour: maxReqPerHour,
		maxTokensPerHour:   maxTokensPerHour,
		theftThreshold:     theftThreshold,
	}

	go mam.cleanupOldStats()

	return mam
}

// RecordAccess 记录用户访问
func (mam *ModelAccessMonitor) RecordAccess(userID string, tokens int) {
	mam.mu.Lock()
	defer mam.mu.Unlock()

	now := time.Now()

	stats, exists := mam.userStats[userID]
	if !exists {
		// 首次访问，创建统计记录
		mam.userStats[userID] = &AccessStats{
			TotalRequests:   1,
			TotalTokens:     tokens,
			HourlyRequests:  1,
			HourlyTokens:    tokens,
			LastHourStart:   now,
			FirstAccessTime: now,
			LastAccessTime:  now,
			SuspiciousCount: 0,
		}
		return
	}

	// 检查是否需要重置小时统计
	if now.Sub(stats.LastHourStart) >= time.Hour {
		// 新的一小时，重置小时统计
		stats.HourlyRequests = 1
		stats.HourlyTokens = tokens
		stats.LastHourStart = now
	} else {
		// 累加小时统计
		stats.HourlyRequests++
		stats.HourlyTokens += tokens
	}

	// 更新总计
	stats.TotalRequests++
	stats.TotalTokens += tokens
	stats.LastAccessTime = now

	// 检测异常行为
	if mam.isAbnormalBehavior(stats) {
		stats.SuspiciousCount++
	} else {
		// 正常行为，重置可疑计数
		if stats.SuspiciousCount > 0 {
			stats.SuspiciousCount--
		}
	}
}

// DetectTheft 检测模型窃取行为
func (mam *ModelAccessMonitor) DetectTheft(userID string) (isTheft bool, reason string) {
	mam.mu.RLock()
	defer mam.mu.RUnlock()

	stats, exists := mam.userStats[userID]
	if !exists {
		return false, ""
	}

	now := time.Now()

	// 1. 检查请求频率异常
	if now.Sub(stats.LastHourStart) < time.Hour {
		if stats.HourlyRequests > mam.maxRequestsPerHour {
			return true, "请求频率异常：超过每小时最大请求数限制"
		}
	}

	// 2. 检查Token使用异常
	if now.Sub(stats.LastHourStart) < time.Hour {
		if stats.HourlyTokens > mam.maxTokensPerHour {
			return true, "Token使用异常：超过每小时最大Token数限制"
		}
	}

	// 3. 检查连续可疑行为
	if stats.SuspiciousCount >= mam.theftThreshold {
		return true, "检测到连续可疑行为，疑似模型窃取"
	}

	// 4. 检查短时间内大量请求
	if stats.TotalRequests > 10 {
		avgInterval := stats.LastAccessTime.Sub(stats.FirstAccessTime).Seconds() / float64(stats.TotalRequests)
		if avgInterval < 2.0 { // 平均每2秒一次请求
			return true, "请求间隔过短，疑似自动化攻击"
		}
	}

	// 5. 检查Token使用模式异常
	if stats.TotalRequests > 20 {
		avgTokensPerRequest := stats.TotalTokens / stats.TotalRequests
		if avgTokensPerRequest > 1500 { // 平均每次请求超过1500 tokens
			return true, "单次请求Token数过高，疑似批量提取"
		}
	}

	// 6. 检查长期高频访问
	if now.Sub(stats.FirstAccessTime) > 24*time.Hour {
		dailyRequests := float64(stats.TotalRequests) / (now.Sub(stats.FirstAccessTime).Hours() / 24)
		if dailyRequests > 500 { // 每天超过500次请求
			return true, "长期高频访问，疑似模型窃取"
		}
	}

	return false, ""
}

// isAbnormalBehavior 判断是否为异常行为
func (mam *ModelAccessMonitor) isAbnormalBehavior(stats *AccessStats) bool {
	now := time.Now()

	// 1. 小时内请求过多
	if now.Sub(stats.LastHourStart) < time.Hour && stats.HourlyRequests > mam.maxRequestsPerHour*8/10 {
		return true
	}

	// 2. 小时内Token使用过多
	if now.Sub(stats.LastHourStart) < time.Hour && stats.HourlyTokens > mam.maxTokensPerHour*8/10 {
		return true
	}

	// 3. 请求间隔过短
	if stats.TotalRequests > 1 {
		interval := now.Sub(stats.LastAccessTime).Seconds()
		if interval < 1.0 { // 1秒内连续请求
			return true
		}
	}

	return false
}

// GetUserStats 获取用户统计信息
func (mam *ModelAccessMonitor) GetUserStats(userID string) (*AccessStats, bool) {
	mam.mu.RLock()
	defer mam.mu.RUnlock()

	stats, exists := mam.userStats[userID]
	if !exists {
		return nil, false
	}

	// 返回副本，避免外部修改
	statsCopy := &AccessStats{
		TotalRequests:   stats.TotalRequests,
		TotalTokens:     stats.TotalTokens,
		HourlyRequests:  stats.HourlyRequests,
		HourlyTokens:    stats.HourlyTokens,
		LastHourStart:   stats.LastHourStart,
		FirstAccessTime: stats.FirstAccessTime,
		LastAccessTime:  stats.LastAccessTime,
		SuspiciousCount: stats.SuspiciousCount,
	}

	return statsCopy, true
}

// GetAllStats 获取所有用户统计（管理员功能）
func (mam *ModelAccessMonitor) GetAllStats() map[string]*AccessStats {
	mam.mu.RLock()
	defer mam.mu.RUnlock()

	allStats := make(map[string]*AccessStats)
	for userID, stats := range mam.userStats {
		allStats[userID] = &AccessStats{
			TotalRequests:   stats.TotalRequests,
			TotalTokens:     stats.TotalTokens,
			HourlyRequests:  stats.HourlyRequests,
			HourlyTokens:    stats.HourlyTokens,
			LastHourStart:   stats.LastHourStart,
			FirstAccessTime: stats.FirstAccessTime,
			LastAccessTime:  stats.LastAccessTime,
			SuspiciousCount: stats.SuspiciousCount,
		}
	}

	return allStats
}

// GetSuspiciousUsers 获取可疑用户列表
func (mam *ModelAccessMonitor) GetSuspiciousUsers() []string {
	mam.mu.RLock()
	defer mam.mu.RUnlock()

	suspiciousUsers := []string{}

	for userID, stats := range mam.userStats {
		// 检查是否有可疑行为
		if stats.SuspiciousCount >= mam.theftThreshold/2 { // 达到一半阈值就列入可疑
			suspiciousUsers = append(suspiciousUsers, userID)
		}
	}

	return suspiciousUsers
}

// ResetUserStats 重置用户统计（管理员功能）
func (mam *ModelAccessMonitor) ResetUserStats(userID string) {
	mam.mu.Lock()
	defer mam.mu.Unlock()

	delete(mam.userStats, userID)
}

// BlockUser 阻止用户访问（管理员功能）
func (mam *ModelAccessMonitor) BlockUser(userID string) {
	mam.mu.Lock()
	defer mam.mu.Unlock()

	stats, exists := mam.userStats[userID]
	if exists {
		// 设置一个极高的可疑计数，标记为已阻止
		stats.SuspiciousCount = 9999
	}
}

// IsUserBlocked 检查用户是否被阻止
func (mam *ModelAccessMonitor) IsUserBlocked(userID string) bool {
	mam.mu.RLock()
	defer mam.mu.RUnlock()

	stats, exists := mam.userStats[userID]
	if !exists {
		return false
	}

	return stats.SuspiciousCount >= 9999
}

// GetRiskLevel 获取用户风险等级（0-5）
func (mam *ModelAccessMonitor) GetRiskLevel(userID string) int {
	mam.mu.RLock()
	defer mam.mu.RUnlock()

	stats, exists := mam.userStats[userID]
	if !exists {
		return 0 // 无风险
	}

	riskLevel := 0

	// 1. 基于可疑行为计数
	if stats.SuspiciousCount >= mam.theftThreshold {
		riskLevel += 3
	} else if stats.SuspiciousCount >= mam.theftThreshold/2 {
		riskLevel += 2
	} else if stats.SuspiciousCount > 0 {
		riskLevel += 1
	}

	// 2. 基于请求频率
	now := time.Now()
	if now.Sub(stats.LastHourStart) < time.Hour {
		if stats.HourlyRequests > mam.maxRequestsPerHour {
			riskLevel += 2
		} else if stats.HourlyRequests > mam.maxRequestsPerHour*8/10 {
			riskLevel += 1
		}
	}

	// 限制在0-5范围内
	if riskLevel > 5 {
		riskLevel = 5
	}

	return riskLevel
}

// cleanupOldStats 清理旧的统计数据（后台任务）
func (mam *ModelAccessMonitor) cleanupOldStats() {
	ticker := time.NewTicker(1 * time.Hour) // 每小时清理一次
	defer ticker.Stop()

	for range ticker.C {
		mam.mu.Lock()

		now := time.Now()
		expiredUsers := []string{}

		// 找出超过7天没有活动的用户
		for userID, stats := range mam.userStats {
			if now.Sub(stats.LastAccessTime) > 7*24*time.Hour {
				expiredUsers = append(expiredUsers, userID)
			}
		}

		// 删除过期用户
		for _, userID := range expiredUsers {
			delete(mam.userStats, userID)
		}

		mam.mu.Unlock()
	}
}

// GetConfig 获取当前配置
func (mam *ModelAccessMonitor) GetConfig() map[string]int {
	return map[string]int{
		"max_requests_per_hour": mam.maxRequestsPerHour,
		"max_tokens_per_hour":   mam.maxTokensPerHour,
		"theft_threshold":       mam.theftThreshold,
	}
}

// UpdateConfig 更新配置（管理员功能）
func (mam *ModelAccessMonitor) UpdateConfig(maxReqPerHour, maxTokensPerHour, theftThreshold int) {
	mam.mu.Lock()
	defer mam.mu.Unlock()

	if maxReqPerHour > 0 {
		mam.maxRequestsPerHour = maxReqPerHour
	}
	if maxTokensPerHour > 0 {
		mam.maxTokensPerHour = maxTokensPerHour
	}
	if theftThreshold > 0 {
		mam.theftThreshold = theftThreshold
	}
}
