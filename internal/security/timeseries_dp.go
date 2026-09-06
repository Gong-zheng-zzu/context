package security

import (
	"github.com/contextkeeper/service/internal/cache"
	"fmt"
	"log"
	"math"
	"math/rand"
	"sync"
	"time"
)

// TimeSeriesDP 时间序列差分隐私保护
// 专门用于保护按时间轴组织的健康数据（体检报告、用药记录、症状变化）
type TimeSeriesDP struct {
	epsilon        float64       // 隐私预算
	delta          float64       // 失败概率
	windowSize     time.Duration // 时间窗口大小（用于聚合）
	minInterval    time.Duration // 最小时间间隔（防止精确推断）
	rng            *rand.Rand
	mu             sync.Mutex
	budgetManager  *PrivacyBudgetManager
	queryCache     *cache.LRUCache // 查询结果缓存
}

// TimeSeriesRecord 时间序列记录
type TimeSeriesRecord struct {
	Timestamp   time.Time
	UserID      string
	RecordType  string // "体检报告"、"用药记录"、"症状变化"
	Value       float64
	Description string
}

// AggregatedTimeData 聚合后的时间数据
type AggregatedTimeData struct {
	WindowStart time.Time
	WindowEnd   time.Time
	Count       int
	AvgValue    float64
	RecordType  string
}

// NewTimeSeriesDP 创建时间序列差分隐私保护器
func NewTimeSeriesDP(epsilon, delta float64, windowSize time.Duration) *TimeSeriesDP {
	return &TimeSeriesDP{
		epsilon:       epsilon,
		delta:         delta,
		windowSize:    windowSize,
		minInterval:   30 * time.Minute, // 默认最小间隔30分钟
		rng:           rand.New(rand.NewSource(time.Now().UnixNano())),
		budgetManager: NewPrivacyBudgetManager(10.0, 24*time.Hour), // 每天10.0的总预算
		queryCache:    cache.NewLRUCache(500, 5*time.Minute),       // 缓存500条查询，5分钟过期
	}
}

// FuzzTimestamp 时间戳模糊化
// 添加随机偏移，防止通过精确时间推断敏感信息
func (tsdp *TimeSeriesDP) FuzzTimestamp(t time.Time, userID string) time.Time {
	tsdp.mu.Lock()
	defer tsdp.mu.Unlock()

	// 检查并消耗隐私预算
	if !tsdp.budgetManager.CheckAndConsumeBudget(userID, 0.1) {
		log.Printf("[时间序列DP] 用户%s隐私预算不足，返回原始时间戳", userID)
		return t
	}

	// 添加±minInterval范围内的随机偏移
	maxOffsetMinutes := int(tsdp.minInterval.Minutes())
	offsetMinutes := tsdp.rng.Intn(2*maxOffsetMinutes) - maxOffsetMinutes

	fuzzedTime := t.Add(time.Duration(offsetMinutes) * time.Minute)

	log.Printf("[时间序列DP] 时间戳模糊化: 原始=%s, 偏移=%d分钟, 模糊后=%s",
		t.Format("2006-01-02 15:04:05"), offsetMinutes, fuzzedTime.Format("2006-01-02 15:04:05"))

	return fuzzedTime
}

// AggregateByWindow 按时间窗口聚合数据
// 将精确的时间点数据聚合到时间窗口，添加差分隐私噪声
func (tsdp *TimeSeriesDP) AggregateByWindow(records []TimeSeriesRecord, userID string) []AggregatedTimeData {
	if len(records) == 0 {
		return []AggregatedTimeData{}
	}

	// 检查隐私预算
	if !tsdp.budgetManager.CheckAndConsumeBudget(userID, tsdp.epsilon) {
		log.Printf("[时间序列DP] 用户%s隐私预算不足，拒绝聚合查询", userID)
		return []AggregatedTimeData{}
	}

	// 按时间窗口分组
	windowMap := make(map[time.Time][]TimeSeriesRecord)

	for _, record := range records {
		// 计算记录所属的时间窗口起始时间
		windowStart := tsdp.getWindowStart(record.Timestamp)
		windowMap[windowStart] = append(windowMap[windowStart], record)
	}

	// 聚合每个窗口的数据并添加噪声
	aggregated := []AggregatedTimeData{}

	for windowStart, windowRecords := range windowMap {
		// 计算真实统计值
		count := len(windowRecords)
		sumValue := 0.0
		recordType := ""

		for _, r := range windowRecords {
			sumValue += r.Value
			if recordType == "" {
				recordType = r.RecordType
			}
		}

		avgValue := sumValue / float64(count)

		// 添加拉普拉斯噪声
		// 敏感度：计数的敏感度为1，平均值的敏感度取决于值域
		countNoise := tsdp.sampleLaplace(0, 1.0/tsdp.epsilon)
		avgNoise := tsdp.sampleLaplace(0, 10.0/tsdp.epsilon) // 假设值域为[0, 100]，敏感度约为10

		noisyCount := int(math.Max(0, float64(count)+countNoise))
		noisyAvg := math.Max(0, avgValue+avgNoise)

		aggregated = append(aggregated, AggregatedTimeData{
			WindowStart: windowStart,
			WindowEnd:   windowStart.Add(tsdp.windowSize),
			Count:       noisyCount,
			AvgValue:    noisyAvg,
			RecordType:  recordType,
		})

		log.Printf("[时间序列DP] 窗口聚合: 时间=%s, 真实计数=%d, 噪声计数=%d, 真实均值=%.2f, 噪声均值=%.2f",
			windowStart.Format("2006-01-02 15:04"), count, noisyCount, avgValue, noisyAvg)
	}

	return aggregated
}

// getWindowStart 获取时间戳所属的时间窗口起始时间
func (tsdp *TimeSeriesDP) getWindowStart(t time.Time) time.Time {
	// 将时间戳向下取整到最近的窗口边界
	windowSeconds := int64(tsdp.windowSize.Seconds())
	timestamp := t.Unix()
	windowStartTimestamp := (timestamp / windowSeconds) * windowSeconds

	return time.Unix(windowStartTimestamp, 0)
}

// sampleLaplace 从拉普拉斯分布采样
func (tsdp *TimeSeriesDP) sampleLaplace(mu, b float64) float64 {
	// 使用逆变换采样法生成拉普拉斯分布
	u := tsdp.rng.Float64() - 0.5
	return mu - b*math.Copysign(1.0, u)*math.Log(1-2*math.Abs(u))
}

// QueryTimeRange 查询时间范围内的数据（带差分隐私保护）
// 返回聚合后的数据，而不是精确的时间点数据
func (tsdp *TimeSeriesDP) QueryTimeRange(
	records []TimeSeriesRecord,
	startTime, endTime time.Time,
	userID string,
) []AggregatedTimeData {
	// 过滤时间范围内的记录
	filteredRecords := []TimeSeriesRecord{}

	for _, record := range records {
		if record.Timestamp.After(startTime) && record.Timestamp.Before(endTime) {
			filteredRecords = append(filteredRecords, record)
		}
	}

	log.Printf("[时间序列DP] 时间范围查询: 开始=%s, 结束=%s, 记录数=%d",
		startTime.Format("2006-01-02"), endTime.Format("2006-01-02"), len(filteredRecords))

	// 聚合并添加噪声
	return tsdp.AggregateByWindow(filteredRecords, userID)
}

// DetectTemporalPattern 检测时间模式攻击
// 防止攻击者通过多次查询不同时间段来推断精确时间
func (tsdp *TimeSeriesDP) DetectTemporalPattern(
	queryHistory []time.Time,
	currentQuery time.Time,
) (isSuspicious bool, reason string) {
	if len(queryHistory) < 3 {
		return false, ""
	}

	// 检查查询间隔是否过于密集
	recentQueries := 0
	for _, queryTime := range queryHistory {
		if currentQuery.Sub(queryTime) < 5*time.Minute {
			recentQueries++
		}
	}

	if recentQueries > 5 {
		return true, "短时间内查询次数过多（5分钟内超过5次）"
	}

	// 检查是否在进行二分查询攻击
	// 攻击者可能通过二分法缩小时间范围来精确定位事件时间
	if len(queryHistory) >= 5 {
		intervals := []time.Duration{}
		for i := 1; i < len(queryHistory); i++ {
			interval := queryHistory[i].Sub(queryHistory[i-1])
			intervals = append(intervals, interval)
		}

		// 检查间隔是否呈现递减模式（二分特征）
		isDecreasing := true
		for i := 1; i < len(intervals); i++ {
			if intervals[i] > intervals[i-1] {
				isDecreasing = false
				break
			}
		}

		if isDecreasing {
			return true, "检测到二分查询模式（可能在尝试精确定位事件时间）"
		}
	}

	return false, ""
}

// AddGaussianNoise 添加高斯噪声（用于(ε, δ)-差分隐私）
// 比拉普拉斯噪声更精确，适用于需要更高准确性的场景
func (tsdp *TimeSeriesDP) AddGaussianNoise(value float64, sensitivity float64) float64 {
	tsdp.mu.Lock()
	defer tsdp.mu.Unlock()

	// 计算高斯分布的标准差
	// σ = sensitivity × sqrt(2 × ln(1.25/δ)) / ε
	sigma := sensitivity * math.Sqrt(2*math.Log(1.25/tsdp.delta)) / tsdp.epsilon

	// 生成高斯噪声
	noise := tsdp.rng.NormFloat64() * sigma

	noisyValue := value + noise

	log.Printf("[时间序列DP] 高斯噪声: 原始值=%.2f, 噪声=%.2f, 噪声值=%.2f, σ=%.4f",
		value, noise, noisyValue, sigma)

	return noisyValue
}

// SlidingWindowQuery 滑动窗口查询（带隐私预算管理）
// 用于查询"最近N天的平均血压"等场景
func (tsdp *TimeSeriesDP) SlidingWindowQuery(
	records []TimeSeriesRecord,
	windowDuration time.Duration,
	userID string,
) (avgValue float64, count int, err error) {
	// 生成缓存键
	cacheKey := fmt.Sprintf("sliding_%s_%s_%d", userID, windowDuration.String(), len(records))

	// 尝试从缓存获取
	if cached, found := tsdp.queryCache.Get(cacheKey); found {
		if result, ok := cached.(map[string]interface{}); ok {
			log.Printf("[时间序列DP] 滑动窗口查询缓存命中: 用户=%s", userID)
			return result["avgValue"].(float64), result["count"].(int), nil
		}
	}

	// 检查隐私预算
	if !tsdp.budgetManager.CheckAndConsumeBudget(userID, tsdp.epsilon*0.5) {
		log.Printf("[时间序列DP] 用户%s隐私预算不足，拒绝滑动窗口查询", userID)
		return 0, 0, nil
	}

	// 计算窗口起始时间
	now := time.Now()
	windowStart := now.Add(-windowDuration)

	// 过滤窗口内的记录
	windowRecords := []TimeSeriesRecord{}
	for _, record := range records {
		if record.Timestamp.After(windowStart) {
			windowRecords = append(windowRecords, record)
		}
	}

	if len(windowRecords) == 0 {
		return 0, 0, nil
	}

	// 计算真实统计值
	sum := 0.0
	for _, record := range windowRecords {
		sum += record.Value
	}
	trueAvg := sum / float64(len(windowRecords))
	trueCount := len(windowRecords)

	// 添加噪声
	noisyAvg := tsdp.AddGaussianNoise(trueAvg, 10.0) // 假设敏感度为10
	noisyCount := int(math.Max(0, float64(trueCount)+tsdp.sampleLaplace(0, 1.0/tsdp.epsilon)))

	log.Printf("[时间序列DP] 滑动窗口查询: 窗口=%s, 真实均值=%.2f, 噪声均值=%.2f, 真实计数=%d, 噪声计数=%d",
		windowDuration.String(), trueAvg, noisyAvg, trueCount, noisyCount)

	// 存入缓存
	result := map[string]interface{}{
		"avgValue": noisyAvg,
		"count":    noisyCount,
	}
	tsdp.queryCache.Set(cacheKey, result)

	return noisyAvg, noisyCount, nil
}

// GetRemainingBudget 获取用户剩余隐私预算
func (tsdp *TimeSeriesDP) GetRemainingBudget(userID string) float64 {
	return tsdp.budgetManager.GetUserRemainingBudget(userID)
}

// ResetUserBudget 重置用户隐私预算（管理员功能）
func (tsdp *TimeSeriesDP) ResetUserBudget(userID string) {
	// 通过重置整个预算管理器来实现用户预算重置
	tsdp.budgetManager.mu.Lock()
	delete(tsdp.budgetManager.budgetPerUser, userID)
	tsdp.budgetManager.mu.Unlock()
	log.Printf("[时间序列DP] 已重置用户%s的隐私预算", userID)
}

// GetQueryCacheStats 获取查询缓存统计信息
func (tsdp *TimeSeriesDP) GetQueryCacheStats() cache.CacheStats {
	return tsdp.queryCache.GetStats()
}

// ClearQueryCache 清空查询缓存
func (tsdp *TimeSeriesDP) ClearQueryCache() {
	tsdp.queryCache.Clear()
	log.Println("[时间序列DP] 查询缓存已清空")
}
