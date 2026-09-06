package bigdata

import (
	"context"
	"fmt"
	"log"
	"sort"
	"time"
)

// QueryType 查询类型
type QueryType string

const (
	QueryByTime     QueryType = "time"     // 按时间范围查询
	QueryByCategory QueryType = "category" // 按类别查询
	QueryBySemantic QueryType = "semantic" // 语义搜索
)

// UnifiedQuery 统一查询请求
type UnifiedQuery struct {
	Type       QueryType              `json:"type"`
	ResidentID string                 `json:"resident_id"`
	StartTime  *time.Time             `json:"start_time"`
	EndTime    *time.Time             `json:"end_time"`
	Category   string                 `json:"category"`
	Keyword    string                 `json:"keyword"`
	Limit      int                    `json:"limit"`
	Offset     int                    `json:"offset"`
	Filters    map[string]interface{} `json:"filters"`
}

// UnifiedQueryResult 统一查询结果
type UnifiedQueryResult struct {
	Source    DataSourceType         `json:"source"`
	Total     int                    `json:"total"`
	Records   []map[string]interface{} `json:"records"`
	QueryTime time.Duration          `json:"query_time"`
}

// UnifiedQueryService 统一查询服务
type UnifiedQueryService struct {
	influxClient *InfluxDBClient
	vitalService *VitalSignService
}

// NewUnifiedQueryService 创建统一查询服务
func NewUnifiedQueryService(influxClient *InfluxDBClient) *UnifiedQueryService {
	return &UnifiedQueryService{
		influxClient: influxClient,
		vitalService: NewVitalSignService(influxClient),
	}
}

// Query 执行统一查询
func (s *UnifiedQueryService) Query(ctx context.Context, query *UnifiedQuery) (*UnifiedQueryResult, error) {
	startTime := time.Now()

	var result *UnifiedQueryResult
	var err error

	switch query.Type {
	case QueryByTime:
		result, err = s.queryByTime(ctx, query)
	case QueryByCategory:
		result, err = s.queryByCategory(ctx, query)
	case QueryBySemantic:
		result, err = s.queryBySemantic(ctx, query)
	default:
		return nil, fmt.Errorf("不支持的查询类型: %s", query.Type)
	}

	if err != nil {
		return nil, err
	}

	result.QueryTime = time.Since(startTime)
	log.Printf("[统一查询] 查询完成: type=%s, source=%s, total=%d, time=%v",
		query.Type, result.Source, result.Total, result.QueryTime)

	return result, nil
}

// queryByTime 按时间范围查询（主要查询InfluxDB）
func (s *UnifiedQueryService) queryByTime(ctx context.Context, query *UnifiedQuery) (*UnifiedQueryResult, error) {
	if query.StartTime == nil || query.EndTime == nil {
		return nil, fmt.Errorf("时间范围查询需要提供开始和结束时间")
	}

	// 查询所有类型的生命体征
	allRecords := make([]map[string]interface{}, 0)

	signTypes := []VitalSignType{BloodPressure, Temperature, HeartRate, BloodOxygen, BloodSugar}

	for _, signType := range signTypes {
		records, err := s.vitalService.QueryVitalSignsByTimeRange(
			ctx,
			query.ResidentID,
			signType,
			*query.StartTime,
			*query.EndTime,
		)
		if err != nil {
			log.Printf("[统一查询] 查询失败: type=%s, error=%v", signType, err)
			continue
		}

		allRecords = append(allRecords, records...)
	}

	// 按时间排序
	sort.Slice(allRecords, func(i, j int) bool {
		ti, _ := allRecords[i]["_time"].(time.Time)
		tj, _ := allRecords[j]["_time"].(time.Time)
		return ti.Before(tj)
	})

	// 应用分页
	total := len(allRecords)
	start := query.Offset
	end := query.Offset + query.Limit

	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	pagedRecords := allRecords[start:end]

	return &UnifiedQueryResult{
		Source:  DataSourceInfluxDB,
		Total:   total,
		Records: pagedRecords,
	}, nil
}

// queryByCategory 按类别查询（主要查询SQLite）
func (s *UnifiedQueryService) queryByCategory(ctx context.Context, query *UnifiedQuery) (*UnifiedQueryResult, error) {
	// TODO: 实现SQLite查询逻辑
	log.Printf("[统一查询] 按类别查询: category=%s", query.Category)

	// 暂时返回空结果
	return &UnifiedQueryResult{
		Source:  DataSourceSQLite,
		Total:   0,
		Records: []map[string]interface{}{},
	}, nil
}

// queryBySemantic 语义搜索（主要查询Qdrant）
func (s *UnifiedQueryService) queryBySemantic(ctx context.Context, query *UnifiedQuery) (*UnifiedQueryResult, error) {
	// TODO: 实现Qdrant语义搜索逻辑
	log.Printf("[统一查询] 语义搜索: keyword=%s", query.Keyword)

	// 暂时返回空结果
	return &UnifiedQueryResult{
		Source:  DataSourceQdrant,
		Total:   0,
		Records: []map[string]interface{}{},
	}, nil
}

// QueryLatestVitalSigns 查询最新生命体征
func (s *UnifiedQueryService) QueryLatestVitalSigns(ctx context.Context, residentID string) (map[VitalSignType]map[string]interface{}, error) {
	return s.vitalService.QueryLatestVitalSigns(ctx, residentID)
}

// QueryAbnormalVitalSigns 查询异常生命体征
func (s *UnifiedQueryService) QueryAbnormalVitalSigns(ctx context.Context, start, end time.Time) ([]map[string]interface{}, error) {
	return s.vitalService.QueryAbnormalVitalSigns(ctx, start, end)
}

// MergeResults 合并多个数据源的查询结果
func (s *UnifiedQueryService) MergeResults(results []*UnifiedQueryResult) *UnifiedQueryResult {
	if len(results) == 0 {
		return &UnifiedQueryResult{
			Total:   0,
			Records: []map[string]interface{}{},
		}
	}

	if len(results) == 1 {
		return results[0]
	}

	// 合并所有记录
	allRecords := make([]map[string]interface{}, 0)
	totalCount := 0

	for _, result := range results {
		allRecords = append(allRecords, result.Records...)
		totalCount += result.Total
	}

	// 按时间排序（如果有时间字段）
	sort.Slice(allRecords, func(i, j int) bool {
		ti, okI := allRecords[i]["_time"].(time.Time)
		tj, okJ := allRecords[j]["_time"].(time.Time)

		if okI && okJ {
			return ti.After(tj) // 降序
		}
		return false
	})

	return &UnifiedQueryResult{
		Source:  "merged",
		Total:   totalCount,
		Records: allRecords,
	}
}

// AggregateByDay 按天聚合数据
func (s *UnifiedQueryService) AggregateByDay(ctx context.Context, residentID string, signType VitalSignType, start, end time.Time) ([]map[string]interface{}, error) {
	query := fmt.Sprintf(`
		from(bucket: "%s")
		|> range(start: %s, stop: %s)
		|> filter(fn: (r) => r["_measurement"] == "%s")
		|> filter(fn: (r) => r["resident_id"] == "%s")
		|> aggregateWindow(every: 1d, fn: mean, createEmpty: false)
		|> yield(name: "mean")
	`, s.influxClient.GetBucket(), start.Format(time.RFC3339), end.Format(time.RFC3339), signType, residentID)

	records, err := s.influxClient.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("聚合查询失败: %w", err)
	}

	log.Printf("[统一查询] 按天聚合成功: resident=%s, type=%s, 记录数=%d", residentID, signType, len(records))
	return records, nil
}

// AggregateByHour 按小时聚合数据
func (s *UnifiedQueryService) AggregateByHour(ctx context.Context, residentID string, signType VitalSignType, start, end time.Time) ([]map[string]interface{}, error) {
	query := fmt.Sprintf(`
		from(bucket: "%s")
		|> range(start: %s, stop: %s)
		|> filter(fn: (r) => r["_measurement"] == "%s")
		|> filter(fn: (r) => r["resident_id"] == "%s")
		|> aggregateWindow(every: 1h, fn: mean, createEmpty: false)
		|> yield(name: "mean")
	`, s.influxClient.GetBucket(), start.Format(time.RFC3339), end.Format(time.RFC3339), signType, residentID)

	records, err := s.influxClient.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("聚合查询失败: %w", err)
	}

	log.Printf("[统一查询] 按小时聚合成功: resident=%s, type=%s, 记录数=%d", residentID, signType, len(records))
	return records, nil
}

// GetStatistics 获取统计信息
func (s *UnifiedQueryService) GetStatistics(ctx context.Context, residentID string, signType VitalSignType, start, end time.Time) (map[string]float64, error) {
	query := fmt.Sprintf(`
		from(bucket: "%s")
		|> range(start: %s, stop: %s)
		|> filter(fn: (r) => r["_measurement"] == "%s")
		|> filter(fn: (r) => r["resident_id"] == "%s")
		|> filter(fn: (r) => r["_field"] == "value")
	`, s.influxClient.GetBucket(), start.Format(time.RFC3339), end.Format(time.RFC3339), signType, residentID)

	records, err := s.influxClient.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("统计查询失败: %w", err)
	}

	if len(records) == 0 {
		return map[string]float64{
			"count": 0,
			"min":   0,
			"max":   0,
			"mean":  0,
		}, nil
	}

	// 计算统计值
	var sum, min, max float64
	count := 0

	for _, record := range records {
		if val, ok := record["_value"].(float64); ok {
			if count == 0 {
				min = val
				max = val
			} else {
				if val < min {
					min = val
				}
				if val > max {
					max = val
				}
			}
			sum += val
			count++
		}
	}

	mean := 0.0
	if count > 0 {
		mean = sum / float64(count)
	}

	stats := map[string]float64{
		"count": float64(count),
		"min":   min,
		"max":   max,
		"mean":  mean,
	}

	log.Printf("[统一查询] 统计完成: resident=%s, type=%s, stats=%+v", residentID, signType, stats)
	return stats, nil
}
