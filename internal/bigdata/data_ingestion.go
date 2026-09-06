package bigdata

import (
	"context"
	"fmt"
	"log"
	"sync"
)

// DataSourceType 数据源类型
type DataSourceType string

const (
	DataSourceSQLite   DataSourceType = "sqlite"   // 结构化数据
	DataSourceInfluxDB DataSourceType = "influxdb" // 时序数据
	DataSourceQdrant   DataSourceType = "qdrant"   // 向量数据
)

// DataRecord 统一数据记录接口
type DataRecord interface {
	GetType() string
	Validate() error
}

// DataIngestionService 数据接入服务
type DataIngestionService struct {
	influxClient *InfluxDBClient
	vitalService *VitalSignService
	mu           sync.RWMutex
	stats        *IngestionStats
}

// IngestionStats 接入统计
type IngestionStats struct {
	TotalRecords    int64
	SuccessRecords  int64
	FailedRecords   int64
	InfluxDBRecords int64
	SQLiteRecords   int64
	QdrantRecords   int64
	mu              sync.RWMutex
}

// NewDataIngestionService 创建数据接入服务
func NewDataIngestionService(influxClient *InfluxDBClient) *DataIngestionService {
	return &DataIngestionService{
		influxClient: influxClient,
		vitalService: NewVitalSignService(influxClient),
		stats:        &IngestionStats{},
	}
}

// RouteData 路由数据到对应的数据源
func (s *DataIngestionService) RouteData(ctx context.Context, dataType string, data interface{}) error {
	s.stats.mu.Lock()
	s.stats.TotalRecords++
	s.stats.mu.Unlock()

	var err error

	switch dataType {
	case "vital_sign":
		// 生命体征数据 -> InfluxDB
		err = s.routeToInfluxDB(ctx, data)
		if err == nil {
			s.stats.mu.Lock()
			s.stats.InfluxDBRecords++
			s.stats.SuccessRecords++
			s.stats.mu.Unlock()
		}

	case "resident_info", "staff_info", "room_info":
		// 结构化数据 -> SQLite
		err = s.routeToSQLite(ctx, data)
		if err == nil {
			s.stats.mu.Lock()
			s.stats.SQLiteRecords++
			s.stats.SuccessRecords++
			s.stats.mu.Unlock()
		}

	case "conversation", "memory":
		// 对话记忆 -> Qdrant
		err = s.routeToQdrant(ctx, data)
		if err == nil {
			s.stats.mu.Lock()
			s.stats.QdrantRecords++
			s.stats.SuccessRecords++
			s.stats.mu.Unlock()
		}

	default:
		err = fmt.Errorf("未知的数据类型: %s", dataType)
	}

	if err != nil {
		s.stats.mu.Lock()
		s.stats.FailedRecords++
		s.stats.mu.Unlock()
		log.Printf("[数据接入] 路由失败: type=%s, error=%v", dataType, err)

		// 降级处理：尝试写入备份存储
		if fallbackErr := s.fallbackStorage(ctx, dataType, data); fallbackErr != nil {
			log.Printf("[数据接入] 降级存储也失败: %v", fallbackErr)
		}
	}

	return err
}

// routeToInfluxDB 路由到InfluxDB
func (s *DataIngestionService) routeToInfluxDB(ctx context.Context, data interface{}) error {
	vitalSign, ok := data.(*VitalSign)
	if !ok {
		return fmt.Errorf("数据类型不匹配，期望 *VitalSign")
	}

	if err := s.vitalService.WriteVitalSign(ctx, vitalSign); err != nil {
		return fmt.Errorf("写入InfluxDB失败: %w", err)
	}

	log.Printf("[数据接入] InfluxDB写入成功: resident=%s, type=%s", vitalSign.ResidentID, vitalSign.Type)
	return nil
}

// routeToSQLite 路由到SQLite
func (s *DataIngestionService) routeToSQLite(ctx context.Context, data interface{}) error {
	// TODO: 实现SQLite写入逻辑
	// 这里暂时只记录日志
	log.Printf("[数据接入] SQLite写入: data=%+v", data)
	return nil
}

// routeToQdrant 路由到Qdrant
func (s *DataIngestionService) routeToQdrant(ctx context.Context, data interface{}) error {
	// TODO: 实现Qdrant写入逻辑
	// 这里暂时只记录日志
	log.Printf("[数据接入] Qdrant写入: data=%+v", data)
	return nil
}

// fallbackStorage 降级存储
func (s *DataIngestionService) fallbackStorage(ctx context.Context, dataType string, data interface{}) error {
	// 降级策略：写入本地文件或内存队列
	log.Printf("[数据接入] 启动降级存储: type=%s", dataType)

	// TODO: 实现降级存储逻辑
	// 1. 写入本地文件
	// 2. 或者放入内存队列，稍后重试

	return nil
}

// BatchIngest 批量接入数据
func (s *DataIngestionService) BatchIngest(ctx context.Context, dataType string, dataList []interface{}) error {
	if dataType == "vital_sign" {
		// 批量写入生命体征
		vitalSigns := make([]*VitalSign, 0, len(dataList))
		for _, data := range dataList {
			if vs, ok := data.(*VitalSign); ok {
				vitalSigns = append(vitalSigns, vs)
			}
		}

		if len(vitalSigns) > 0 {
			if err := s.vitalService.WriteBatchVitalSigns(ctx, vitalSigns); err != nil {
				return fmt.Errorf("批量写入失败: %w", err)
			}

			s.stats.mu.Lock()
			s.stats.TotalRecords += int64(len(vitalSigns))
			s.stats.SuccessRecords += int64(len(vitalSigns))
			s.stats.InfluxDBRecords += int64(len(vitalSigns))
			s.stats.mu.Unlock()

			log.Printf("[数据接入] 批量写入成功: %d条生命体征记录", len(vitalSigns))
		}
		return nil
	}

	// 其他类型逐条处理
	for _, data := range dataList {
		if err := s.RouteData(ctx, dataType, data); err != nil {
			log.Printf("[数据接入] 批量接入单条失败: %v", err)
		}
	}

	return nil
}

// GetStats 获取接入统计
func (s *DataIngestionService) GetStats() IngestionStats {
	s.stats.mu.RLock()
	defer s.stats.mu.RUnlock()

	return IngestionStats{
		TotalRecords:    s.stats.TotalRecords,
		SuccessRecords:  s.stats.SuccessRecords,
		FailedRecords:   s.stats.FailedRecords,
		InfluxDBRecords: s.stats.InfluxDBRecords,
		SQLiteRecords:   s.stats.SQLiteRecords,
		QdrantRecords:   s.stats.QdrantRecords,
	}
}

// ResetStats 重置统计
func (s *DataIngestionService) ResetStats() {
	s.stats.mu.Lock()
	defer s.stats.mu.Unlock()

	s.stats.TotalRecords = 0
	s.stats.SuccessRecords = 0
	s.stats.FailedRecords = 0
	s.stats.InfluxDBRecords = 0
	s.stats.SQLiteRecords = 0
	s.stats.QdrantRecords = 0

	log.Println("[数据接入] 统计已重置")
}

// HealthCheck 健康检查
func (s *DataIngestionService) HealthCheck(ctx context.Context) map[string]bool {
	health := make(map[string]bool)

	// 检查InfluxDB
	if s.influxClient != nil {
		health["influxdb"] = s.influxClient.HealthCheck() == nil
	} else {
		health["influxdb"] = false
	}

	// TODO: 检查SQLite
	health["sqlite"] = true

	// TODO: 检查Qdrant
	health["qdrant"] = true

	return health
}
