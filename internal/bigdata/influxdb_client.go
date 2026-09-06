package bigdata

import (
	"context"
	"fmt"
	"log"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
)

// InfluxDBClient InfluxDB客户端封装
type InfluxDBClient struct {
	client   influxdb2.Client
	writeAPI api.WriteAPIBlocking
	queryAPI api.QueryAPI
	org      string
	bucket   string
}

// InfluxDBConfig InfluxDB配置
type InfluxDBConfig struct {
	URL    string
	Token  string
	Org    string
	Bucket string
}

// NewInfluxDBClient 创建InfluxDB客户端
func NewInfluxDBClient(config InfluxDBConfig) (*InfluxDBClient, error) {
	// 创建客户端
	client := influxdb2.NewClient(config.URL, config.Token)

	// 创建写入API（阻塞模式，确保数据写入成功）
	writeAPI := client.WriteAPIBlocking(config.Org, config.Bucket)

	// 创建查询API
	queryAPI := client.QueryAPI(config.Org)

	influxClient := &InfluxDBClient{
		client:   client,
		writeAPI: writeAPI,
		queryAPI: queryAPI,
		org:      config.Org,
		bucket:   config.Bucket,
	}

	// 健康检查
	if err := influxClient.HealthCheck(); err != nil {
		return nil, fmt.Errorf("InfluxDB健康检查失败: %w", err)
	}

	log.Printf("[InfluxDB] 客户端初始化成功: URL=%s, Org=%s, Bucket=%s", config.URL, config.Org, config.Bucket)
	return influxClient, nil
}

// HealthCheck 健康检查
func (c *InfluxDBClient) HealthCheck() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	health, err := c.client.Health(ctx)
	if err != nil {
		return fmt.Errorf("健康检查请求失败: %w", err)
	}

	if health.Status != "pass" {
		return fmt.Errorf("InfluxDB状态异常: %s", health.Status)
	}

	log.Printf("[InfluxDB] 健康检查通过: status=%s, version=%s", health.Status, *health.Version)
	return nil
}

// WritePoint 写入单个数据点
func (c *InfluxDBClient) WritePoint(ctx context.Context, point *write.Point) error {
	if err := c.writeAPI.WritePoint(ctx, point); err != nil {
		return fmt.Errorf("写入数据点失败: %w", err)
	}
	return nil
}

// WritePoints 批量写入数据点
func (c *InfluxDBClient) WritePoints(ctx context.Context, points []*write.Point) error {
	// 使用批量写入API
	for _, point := range points {
		if err := c.writeAPI.WritePoint(ctx, point); err != nil {
			log.Printf("[InfluxDB] 写入数据点失败: %v", err)
			return fmt.Errorf("批量写入失败: %w", err)
		}
	}
	log.Printf("[InfluxDB] 批量写入成功: %d个数据点", len(points))
	return nil
}

// Query 执行查询
func (c *InfluxDBClient) Query(ctx context.Context, query string) ([]map[string]interface{}, error) {
	result, err := c.queryAPI.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("查询失败: %w", err)
	}
	defer result.Close()

	var records []map[string]interface{}
	for result.Next() {
		record := make(map[string]interface{})

		// 获取所有列
		for key, value := range result.Record().Values() {
			record[key] = value
		}

		// 添加时间戳
		record["_time"] = result.Record().Time()

		records = append(records, record)
	}

	if result.Err() != nil {
		return nil, fmt.Errorf("查询结果处理失败: %w", result.Err())
	}

	log.Printf("[InfluxDB] 查询成功: 返回%d条记录", len(records))
	return records, nil
}

// QueryWithRetry 带重试的查询
func (c *InfluxDBClient) QueryWithRetry(ctx context.Context, query string, maxRetries int) ([]map[string]interface{}, error) {
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		records, err := c.Query(ctx, query)
		if err == nil {
			return records, nil
		}

		lastErr = err
		log.Printf("[InfluxDB] 查询失败，重试 %d/%d: %v", i+1, maxRetries, err)

		// 指数退避
		time.Sleep(time.Duration(1<<uint(i)) * time.Second)
	}

	return nil, fmt.Errorf("查询失败，已重试%d次: %w", maxRetries, lastErr)
}

// Close 关闭客户端
func (c *InfluxDBClient) Close() {
	if c.client != nil {
		c.client.Close()
		log.Println("[InfluxDB] 客户端已关闭")
	}
}

// GetBucket 获取bucket名称
func (c *InfluxDBClient) GetBucket() string {
	return c.bucket
}

// GetOrg 获取组织名称
func (c *InfluxDBClient) GetOrg() string {
	return c.org
}
