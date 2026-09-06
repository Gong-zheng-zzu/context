package bigdata

import (
	"context"
	"fmt"
	"log"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
)

// VitalSignType 生命体征类型
type VitalSignType string

const (
	BloodPressure VitalSignType = "blood_pressure" // 血压
	Temperature   VitalSignType = "temperature"    // 体温
	HeartRate     VitalSignType = "heart_rate"     // 心率
	BloodOxygen   VitalSignType = "blood_oxygen"   // 血氧
	BloodSugar    VitalSignType = "blood_sugar"    // 血糖
)

// VitalSign 生命体征数据
type VitalSign struct {
	ResidentID  string        `json:"resident_id"`  // 老人ID
	Type        VitalSignType `json:"type"`         // 类型
	Value       float64       `json:"value"`        // 数值
	Value2      *float64      `json:"value2"`       // 第二个数值（如血压的舒张压）
	Unit        string        `json:"unit"`         // 单位
	RoomNumber  string        `json:"room_number"`  // 房间号
	DeviceID    string        `json:"device_id"`    // 设备ID
	MeasuredAt  time.Time     `json:"measured_at"`  // 测量时间
	RecordedBy  string        `json:"recorded_by"`  // 记录人
	Notes       string        `json:"notes"`        // 备注
}

// VitalSignRange 生命体征正常范围
type VitalSignRange struct {
	Min  float64
	Max  float64
	Unit string
}

// 正常范围定义
var normalRanges = map[VitalSignType]VitalSignRange{
	BloodPressure: {Min: 90, Max: 140, Unit: "mmHg"},   // 收缩压
	Temperature:   {Min: 36.0, Max: 37.5, Unit: "°C"},  // 体温
	HeartRate:     {Min: 60, Max: 100, Unit: "bpm"},    // 心率
	BloodOxygen:   {Min: 95, Max: 100, Unit: "%"},      // 血氧
	BloodSugar:    {Min: 3.9, Max: 6.1, Unit: "mmol/L"}, // 空腹血糖
}

// Validate 验证生命体征数据
func (vs *VitalSign) Validate() error {
	if vs.ResidentID == "" {
		return fmt.Errorf("老人ID不能为空")
	}

	if vs.Type == "" {
		return fmt.Errorf("生命体征类型不能为空")
	}

	// 检查类型是否有效
	validTypes := []VitalSignType{BloodPressure, Temperature, HeartRate, BloodOxygen, BloodSugar}
	isValidType := false
	for _, t := range validTypes {
		if vs.Type == t {
			isValidType = true
			break
		}
	}
	if !isValidType {
		return fmt.Errorf("无效的生命体征类型: %s", vs.Type)
	}

	// 血压需要两个值
	if vs.Type == BloodPressure && vs.Value2 == nil {
		return fmt.Errorf("血压需要提供收缩压和舒张压")
	}

	if vs.MeasuredAt.IsZero() {
		vs.MeasuredAt = time.Now()
	}

	return nil
}

// IsAbnormal 判断是否异常
func (vs *VitalSign) IsAbnormal() bool {
	normalRange, exists := normalRanges[vs.Type]
	if !exists {
		return false
	}

	// 血压特殊处理
	if vs.Type == BloodPressure {
		// 收缩压异常
		if vs.Value < normalRange.Min || vs.Value > normalRange.Max {
			return true
		}
		// 舒张压异常（60-90 mmHg）
		if vs.Value2 != nil && (*vs.Value2 < 60 || *vs.Value2 > 90) {
			return true
		}
		return false
	}

	// 其他类型
	return vs.Value < normalRange.Min || vs.Value > normalRange.Max
}

// GetAbnormalLevel 获取异常等级
func (vs *VitalSign) GetAbnormalLevel() string {
	if !vs.IsAbnormal() {
		return "normal"
	}

	normalRange := normalRanges[vs.Type]

	// 严重异常（超出正常范围20%以上）
	deviation := 0.0
	if vs.Value < normalRange.Min {
		deviation = (normalRange.Min - vs.Value) / normalRange.Min
	} else {
		deviation = (vs.Value - normalRange.Max) / normalRange.Max
	}

	if deviation > 0.2 {
		return "critical"
	} else if deviation > 0.1 {
		return "warning"
	}
	return "mild"
}

// ToInfluxPoint 转换为InfluxDB数据点
func (vs *VitalSign) ToInfluxPoint() *write.Point {
	point := influxdb2.NewPoint(
		string(vs.Type),
		map[string]string{
			"resident_id": vs.ResidentID,
			"room_number": vs.RoomNumber,
			"device_id":   vs.DeviceID,
			"recorded_by": vs.RecordedBy,
		},
		map[string]interface{}{
			"value":    vs.Value,
			"unit":     vs.Unit,
			"abnormal": vs.IsAbnormal(),
			"level":    vs.GetAbnormalLevel(),
		},
		vs.MeasuredAt,
	)

	// 血压添加第二个值
	if vs.Type == BloodPressure && vs.Value2 != nil {
		point = influxdb2.NewPoint(
			string(vs.Type),
			map[string]string{
				"resident_id": vs.ResidentID,
				"room_number": vs.RoomNumber,
				"device_id":   vs.DeviceID,
				"recorded_by": vs.RecordedBy,
			},
			map[string]interface{}{
				"systolic":  vs.Value,
				"diastolic": *vs.Value2,
				"unit":      vs.Unit,
				"abnormal":  vs.IsAbnormal(),
				"level":     vs.GetAbnormalLevel(),
			},
			vs.MeasuredAt,
		)
	}

	// 添加备注（如果有）
	if vs.Notes != "" {
		point.AddTag("notes", vs.Notes)
	}

	return point
}

// VitalSignService 生命体征服务
type VitalSignService struct {
	influxClient *InfluxDBClient
}

// NewVitalSignService 创建生命体征服务
func NewVitalSignService(influxClient *InfluxDBClient) *VitalSignService {
	return &VitalSignService{
		influxClient: influxClient,
	}
}

// WriteVitalSign 写入生命体征数据
func (s *VitalSignService) WriteVitalSign(ctx context.Context, vs *VitalSign) error {
	// 验证数据
	if err := vs.Validate(); err != nil {
		return fmt.Errorf("数据验证失败: %w", err)
	}

	// 转换为InfluxDB数据点
	point := vs.ToInfluxPoint()

	// 写入数据
	if err := s.influxClient.WritePoint(ctx, point); err != nil {
		return fmt.Errorf("写入失败: %w", err)
	}

	log.Printf("[生命体征] 写入成功: resident=%s, type=%s, value=%.2f, abnormal=%v",
		vs.ResidentID, vs.Type, vs.Value, vs.IsAbnormal())

	return nil
}

// WriteBatchVitalSigns 批量写入生命体征数据
func (s *VitalSignService) WriteBatchVitalSigns(ctx context.Context, vitalSigns []*VitalSign) error {
	points := make([]*write.Point, 0, len(vitalSigns))

	for _, vs := range vitalSigns {
		// 验证数据
		if err := vs.Validate(); err != nil {
			log.Printf("[生命体征] 数据验证失败，跳过: %v", err)
			continue
		}

		// 转换为InfluxDB数据点
		point := vs.ToInfluxPoint()
		points = append(points, point)
	}

	// 批量写入
	if err := s.influxClient.WritePoints(ctx, points); err != nil {
		return fmt.Errorf("批量写入失败: %w", err)
	}

	log.Printf("[生命体征] 批量写入成功: %d条记录", len(points))
	return nil
}

// QueryVitalSignsByTimeRange 按时间范围查询生命体征
func (s *VitalSignService) QueryVitalSignsByTimeRange(ctx context.Context, residentID string, signType VitalSignType, start, end time.Time) ([]map[string]interface{}, error) {
	query := fmt.Sprintf(`
		from(bucket: "%s")
		|> range(start: %s, stop: %s)
		|> filter(fn: (r) => r["_measurement"] == "%s")
		|> filter(fn: (r) => r["resident_id"] == "%s")
		|> sort(columns: ["_time"], desc: false)
	`, s.influxClient.GetBucket(), start.Format(time.RFC3339), end.Format(time.RFC3339), signType, residentID)

	records, err := s.influxClient.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("查询失败: %w", err)
	}

	log.Printf("[生命体征] 查询成功: resident=%s, type=%s, 时间范围=%s~%s, 记录数=%d",
		residentID, signType, start.Format("2006-01-02"), end.Format("2006-01-02"), len(records))

	return records, nil
}

// QueryLatestVitalSigns 查询最新的生命体征
func (s *VitalSignService) QueryLatestVitalSigns(ctx context.Context, residentID string) (map[VitalSignType]map[string]interface{}, error) {
	result := make(map[VitalSignType]map[string]interface{})

	signTypes := []VitalSignType{BloodPressure, Temperature, HeartRate, BloodOxygen, BloodSugar}

	for _, signType := range signTypes {
		query := fmt.Sprintf(`
			from(bucket: "%s")
			|> range(start: -7d)
			|> filter(fn: (r) => r["_measurement"] == "%s")
			|> filter(fn: (r) => r["resident_id"] == "%s")
			|> sort(columns: ["_time"], desc: true)
			|> limit(n: 1)
		`, s.influxClient.GetBucket(), signType, residentID)

		records, err := s.influxClient.Query(ctx, query)
		if err != nil {
			log.Printf("[生命体征] 查询最新数据失败: type=%s, error=%v", signType, err)
			continue
		}

		if len(records) > 0 {
			result[signType] = records[0]
		}
	}

	log.Printf("[生命体征] 查询最新数据成功: resident=%s, 类型数=%d", residentID, len(result))
	return result, nil
}

// QueryAbnormalVitalSigns 查询异常生命体征
func (s *VitalSignService) QueryAbnormalVitalSigns(ctx context.Context, start, end time.Time) ([]map[string]interface{}, error) {
	query := fmt.Sprintf(`
		from(bucket: "%s")
		|> range(start: %s, stop: %s)
		|> filter(fn: (r) => r["abnormal"] == true)
		|> sort(columns: ["_time"], desc: true)
	`, s.influxClient.GetBucket(), start.Format(time.RFC3339), end.Format(time.RFC3339))

	records, err := s.influxClient.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("查询异常数据失败: %w", err)
	}

	log.Printf("[生命体征] 查询异常数据成功: 时间范围=%s~%s, 记录数=%d",
		start.Format("2006-01-02"), end.Format("2006-01-02"), len(records))

	return records, nil
}
