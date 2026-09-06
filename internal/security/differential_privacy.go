package security

import (
	"log"
	"math"
	"math/rand"
	"sync"
	"time"
)

// DifferentialPrivacy 差分隐私保护
type DifferentialPrivacy struct {
	epsilon float64 // 隐私预算
	delta   float64 // 失败概率
	rng     *rand.Rand
	mu      sync.Mutex
}

// NewDifferentialPrivacy 创建差分隐私保护器
func NewDifferentialPrivacy(epsilon, delta float64) *DifferentialPrivacy {
	return &DifferentialPrivacy{
		epsilon: epsilon,
		delta:   delta,
		rng:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// AddLaplaceNoise 添加拉普拉斯噪声到向量
func (dp *DifferentialPrivacy) AddLaplaceNoise(vector []float32) []float32 {
	noisyVector := make([]float32, len(vector))

	// 计算拉普拉斯分布的尺度参数
	scale := 1.0 / dp.epsilon

	for i, v := range vector {
		// 生成拉普拉斯噪声
		noise := dp.sampleLaplace(0, scale)
		noisyVector[i] = v + float32(noise)
	}

	log.Printf("[差分隐私] 已添加拉普拉斯噪声: epsilon=%.2f, scale=%.4f", dp.epsilon, scale)
	return noisyVector
}

// sampleLaplace 从拉普拉斯分布采样
func (dp *DifferentialPrivacy) sampleLaplace(mu, b float64) float64 {
	dp.mu.Lock()
	defer dp.mu.Unlock()

	// 使用逆变换采样法生成拉普拉斯分布
	u := dp.rng.Float64() - 0.5
	return mu - b*math.Copysign(1.0, u)*math.Log(1-2*math.Abs(u))
}

// GenerateLaplaceNoise 生成单个拉普拉斯噪声值
func (dp *DifferentialPrivacy) GenerateLaplaceNoise(scale float64) float64 {
	return dp.sampleLaplace(0, scale)
}

// ObfuscateVector 混淆向量（用于删除前）
func (dp *DifferentialPrivacy) ObfuscateVector(vector []float32) []float32 {
	// 1. 添加噪声
	noisyVector := dp.AddLaplaceNoise(vector)

	// 2. 归一化（保持向量在合理范围内）
	norm := float32(0.0)
	for _, v := range noisyVector {
		norm += v * v
	}
	norm = float32(math.Sqrt(float64(norm)))

	if norm > 0 {
		for i := range noisyVector {
			noisyVector[i] /= norm
		}
	}

	log.Printf("[差分隐私] 向量混淆完成: 原始维度=%d, 归一化后范数=1.0", len(vector))
	return noisyVector
}

// CalculateSimilarity 计算两个向量的余弦相似度
func (dp *DifferentialPrivacy) CalculateSimilarity(vec1, vec2 []float32) float64 {
	if len(vec1) != len(vec2) {
		log.Printf("[差分隐私] 向量维度不匹配: %d vs %d", len(vec1), len(vec2))
		return 0.0
	}

	dotProduct := float64(0.0)
	norm1 := float64(0.0)
	norm2 := float64(0.0)

	for i := 0; i < len(vec1); i++ {
		dotProduct += float64(vec1[i]) * float64(vec2[i])
		norm1 += float64(vec1[i]) * float64(vec1[i])
		norm2 += float64(vec2[i]) * float64(vec2[i])
	}

	norm1 = math.Sqrt(norm1)
	norm2 = math.Sqrt(norm2)

	if norm1 == 0 || norm2 == 0 {
		return 0.0
	}

	similarity := dotProduct / (norm1 * norm2)
	return similarity
}

// PrivacyBudgetManager 隐私预算管理器
type PrivacyBudgetManager struct {
	totalBudget     float64
	usedBudget      float64
	budgetPerUser   map[string]float64
	mu              sync.RWMutex
	resetInterval   time.Duration
	lastResetTime   time.Time
}

// NewPrivacyBudgetManager 创建隐私预算管理器
func NewPrivacyBudgetManager(totalBudget float64, resetInterval time.Duration) *PrivacyBudgetManager {
	return &PrivacyBudgetManager{
		totalBudget:   totalBudget,
		usedBudget:    0.0,
		budgetPerUser: make(map[string]float64),
		resetInterval: resetInterval,
		lastResetTime: time.Now(),
	}
}

// CheckAndConsumeBudget 检查并消耗隐私预算
func (pbm *PrivacyBudgetManager) CheckAndConsumeBudget(userID string, epsilon float64) bool {
	pbm.mu.Lock()
	defer pbm.mu.Unlock()

	// 检查是否需要重置预算
	if time.Since(pbm.lastResetTime) > pbm.resetInterval {
		pbm.resetBudget()
	}

	// 检查全局预算
	if pbm.usedBudget+epsilon > pbm.totalBudget {
		log.Printf("[隐私预算] 全局预算不足: 已用=%.2f, 总预算=%.2f, 请求=%.2f",
			pbm.usedBudget, pbm.totalBudget, epsilon)
		return false
	}

	// 检查用户预算（每个用户最多使用总预算的10%）
	userBudget := pbm.budgetPerUser[userID]
	maxUserBudget := pbm.totalBudget * 0.1
	if userBudget+epsilon > maxUserBudget {
		log.Printf("[隐私预算] 用户预算不足: userID=%s, 已用=%.2f, 最大=%.2f, 请求=%.2f",
			userID, userBudget, maxUserBudget, epsilon)
		return false
	}

	// 消耗预算
	pbm.usedBudget += epsilon
	pbm.budgetPerUser[userID] += epsilon

	log.Printf("[隐私预算] 预算消耗成功: userID=%s, 消耗=%.2f, 剩余全局=%.2f, 剩余用户=%.2f",
		userID, epsilon, pbm.totalBudget-pbm.usedBudget, maxUserBudget-pbm.budgetPerUser[userID])

	return true
}

// resetBudget 重置预算
func (pbm *PrivacyBudgetManager) resetBudget() {
	log.Printf("[隐私预算] 重置预算: 上次重置=%s, 已用预算=%.2f",
		pbm.lastResetTime.Format("2006-01-02 15:04:05"), pbm.usedBudget)

	pbm.usedBudget = 0.0
	pbm.budgetPerUser = make(map[string]float64)
	pbm.lastResetTime = time.Now()
}

// GetRemainingBudget 获取剩余预算
func (pbm *PrivacyBudgetManager) GetRemainingBudget() float64 {
	pbm.mu.RLock()
	defer pbm.mu.RUnlock()

	return pbm.totalBudget - pbm.usedBudget
}

// GetUserRemainingBudget 获取用户剩余预算
func (pbm *PrivacyBudgetManager) GetUserRemainingBudget(userID string) float64 {
	pbm.mu.RLock()
	defer pbm.mu.RUnlock()

	maxUserBudget := pbm.totalBudget * 0.1
	userBudget := pbm.budgetPerUser[userID]
	return maxUserBudget - userBudget
}
