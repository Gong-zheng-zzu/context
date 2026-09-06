package bigdata

import (
	"context"
	"fmt"
	"log"
	"time"
)

// AlertLevel 告警级别
type AlertLevel string

const (
	AlertLevelInfo     AlertLevel = "info"     // 信息
	AlertLevelWarning  AlertLevel = "warning"  // 警告
	AlertLevelCritical AlertLevel = "critical" // 严重
)

// Alert 告警信息
type Alert struct {
	ID          string                 `json:"id"`           // 告警ID
	ElderID     string                 `json:"elder_id"`     // 老人ID
	ElderName   string                 `json:"elder_name"`   // 老人姓名
	Level       AlertLevel             `json:"level"`        // 告警级别
	Type        string                 `json:"type"`         // 告警类型
	Title       string                 `json:"title"`        // 告警标题
	Message     string                 `json:"message"`      // 告警消息
	Timestamp   time.Time              `json:"timestamp"`    // 告警时间
	TargetRoles []string               `json:"target_roles"` // 目标角色
	Data        map[string]interface{} `json:"data"`         // 附加数据
	Resolved    bool                   `json:"resolved"`     // 是否已解决
}

// AlertEngine 告警引擎
type AlertEngine struct {
	influxClient *InfluxDBClient
	vitalService *VitalSignService
}

// NewAlertEngine 创建告警引擎
func NewAlertEngine(influxClient *InfluxDBClient) *AlertEngine {
	return &AlertEngine{
		influxClient: influxClient,
		vitalService: NewVitalSignService(influxClient),
	}
}

// CheckVitalSignAlerts 检查生命体征告警
func (e *AlertEngine) CheckVitalSignAlerts(ctx context.Context, elderID, elderName string) []Alert {
	var alerts []Alert

	// 查询最近24小时的生命体征数据
	end := time.Now()
	start := end.Add(-24 * time.Hour)

	// 检查血压
	bpRecords, err := e.vitalService.QueryVitalSignsByTimeRange(ctx, elderID, BloodPressure, start, end)
	if err == nil && len(bpRecords) > 0 {
		// 检查最近的血压记录
		latestBP := bpRecords[len(bpRecords)-1]

		// 从map中提取数据
		abnormal, _ := latestBP["abnormal"].(bool)
		if abnormal {
			level := AlertLevelWarning
			abnormalLevel, _ := latestBP["level"].(string)
			if abnormalLevel == "critical" {
				level = AlertLevelCritical
			}

			systolic, _ := latestBP["systolic"].(float64)
			diastolic, _ := latestBP["diastolic"].(float64)
			unit, _ := latestBP["unit"].(string)

			// 生成不同角色的告警
			alerts = append(alerts, Alert{
				ID:        fmt.Sprintf("bp_%s_%d", elderID, time.Now().Unix()),
				ElderID:   elderID,
				ElderName: elderName,
				Level:     level,
				Type:      "vital_sign_abnormal",
				Title:     "血压异常",
				Message:   e.generateBPAlertMessageFromMap(elderName, systolic, diastolic, unit, "caregiver"),
				Timestamp: time.Now(),
				TargetRoles: []string{"caregiver"},
				Data: map[string]interface{}{
					"systolic":  systolic,
					"diastolic": diastolic,
					"unit":      unit,
				},
			})

			// 医生端告警
			alerts = append(alerts, Alert{
				ID:        fmt.Sprintf("bp_doctor_%s_%d", elderID, time.Now().Unix()),
				ElderID:   elderID,
				ElderName: elderName,
				Level:     level,
				Type:      "vital_sign_abnormal",
				Title:     "血压异常",
				Message:   e.generateBPAlertMessageFromMap(elderName, systolic, diastolic, unit, "doctor"),
				Timestamp: time.Now(),
				TargetRoles: []string{"doctor"},
				Data: map[string]interface{}{
					"systolic":  systolic,
					"diastolic": diastolic,
					"unit":      unit,
				},
			})

			// 家属端告警（严重时才通知）
			if level == AlertLevelCritical {
				alerts = append(alerts, Alert{
					ID:        fmt.Sprintf("bp_family_%s_%d", elderID, time.Now().Unix()),
					ElderID:   elderID,
					ElderName: elderName,
					Level:     level,
					Type:      "vital_sign_abnormal",
					Title:     "血压异常",
					Message:   e.generateBPAlertMessageFromMap(elderName, systolic, diastolic, unit, "family"),
					Timestamp: time.Now(),
					TargetRoles: []string{"family"},
					Data: map[string]interface{}{
						"systolic":  systolic,
						"diastolic": diastolic,
					},
				})
			}
		}

		// 检查连续3天血压升高（医生端）
		if len(bpRecords) >= 3 {
			consecutiveHigh := true
			for i := len(bpRecords) - 3; i < len(bpRecords); i++ {
				abnormal, _ := bpRecords[i]["abnormal"].(bool)
				if !abnormal {
					consecutiveHigh = false
					break
				}
			}
			if consecutiveHigh {
				alerts = append(alerts, Alert{
					ID:          fmt.Sprintf("bp_trend_%s_%d", elderID, time.Now().Unix()),
					ElderID:     elderID,
					ElderName:   elderName,
					Level:       AlertLevelWarning,
					Type:        "vital_sign_trend",
					Title:       "血压趋势异常",
					Message:     fmt.Sprintf("%s连续3次血压偏高，建议调整用药方案", elderName),
					Timestamp:   time.Now(),
					TargetRoles: []string{"doctor"},
					Data: map[string]interface{}{
						"trend": "consecutive_high",
						"count": 3,
					},
				})
			}
		}
	}

	// 检查心率
	hrRecords, err := e.vitalService.QueryVitalSignsByTimeRange(ctx, elderID, HeartRate, start, end)
	if err == nil && len(hrRecords) > 0 {
		latestHR := hrRecords[len(hrRecords)-1]
		abnormal, _ := latestHR["abnormal"].(bool)
		if abnormal {
			level := AlertLevelWarning
			abnormalLevel, _ := latestHR["level"].(string)
			if abnormalLevel == "critical" {
				level = AlertLevelCritical
			}

			heartRate, _ := latestHR["value"].(float64)
			unit, _ := latestHR["unit"].(string)

			alerts = append(alerts, Alert{
				ID:        fmt.Sprintf("hr_%s_%d", elderID, time.Now().Unix()),
				ElderID:   elderID,
				ElderName: elderName,
				Level:     level,
				Type:      "vital_sign_abnormal",
				Title:     "心率异常",
				Message:   fmt.Sprintf("%s心率异常（%d次/分），请及时查看", elderName, int(heartRate)),
				Timestamp: time.Now(),
				TargetRoles: []string{"caregiver", "doctor"},
				Data: map[string]interface{}{
					"heart_rate": heartRate,
					"unit":       unit,
				},
			})
		}
	}

	// 检查体温
	tempRecords, err := e.vitalService.QueryVitalSignsByTimeRange(ctx, elderID, Temperature, start, end)
	if err == nil && len(tempRecords) > 0 {
		latestTemp := tempRecords[len(tempRecords)-1]
		abnormal, _ := latestTemp["abnormal"].(bool)
		if abnormal {
			temperature, _ := latestTemp["value"].(float64)
			unit, _ := latestTemp["unit"].(string)

			alerts = append(alerts, Alert{
				ID:        fmt.Sprintf("temp_%s_%d", elderID, time.Now().Unix()),
				ElderID:   elderID,
				ElderName: elderName,
				Level:     AlertLevelWarning,
				Type:      "vital_sign_abnormal",
				Title:     "体温异常",
				Message:   fmt.Sprintf("%s体温%.1f℃，可能有发烧，请关注", elderName, temperature),
				Timestamp: time.Now(),
				TargetRoles: []string{"caregiver", "doctor"},
				Data: map[string]interface{}{
					"temperature": temperature,
					"unit":        unit,
				},
			})

			// 家属端告警
			alerts = append(alerts, Alert{
				ID:        fmt.Sprintf("temp_family_%s_%d", elderID, time.Now().Unix()),
				ElderID:   elderID,
				ElderName: elderName,
				Level:     AlertLevelInfo,
				Type:      "vital_sign_abnormal",
				Title:     "体温异常",
				Message:   fmt.Sprintf("您的亲人%s今天有轻微发烧，医生已查看", elderName),
				Timestamp: time.Now(),
				TargetRoles: []string{"family"},
				Data: map[string]interface{}{
					"temperature": temperature,
				},
			})
		}
	}

	return alerts
}

// generateBPAlertMessage 生成血压告警消息（针对不同角色）
func (e *AlertEngine) generateBPAlertMessage(elderName string, bp *VitalSign, role string) string {
	systolic := int(bp.Value)
	diastolic := int(*bp.Value2)

	switch role {
	case "caregiver":
		return fmt.Sprintf("%s血压偏高（%d/%d mmHg），建议测量并记录", elderName, systolic, diastolic)
	case "doctor":
		return fmt.Sprintf("%s血压异常（%d/%d mmHg），建议查看历史趋势并调整用药", elderName, systolic, diastolic)
	case "family":
		return fmt.Sprintf("您的亲人%s血压偏高，医护人员正在关注", elderName)
	default:
		return fmt.Sprintf("%s血压异常（%d/%d mmHg）", elderName, systolic, diastolic)
	}
}

// generateBPAlertMessageFromMap 从map生成血压告警消息（针对不同角色）
func (e *AlertEngine) generateBPAlertMessageFromMap(elderName string, systolic, diastolic float64, unit, role string) string {
	systolicInt := int(systolic)
	diastolicInt := int(diastolic)

	switch role {
	case "caregiver":
		return fmt.Sprintf("%s血压偏高（%d/%d %s），建议测量并记录", elderName, systolicInt, diastolicInt, unit)
	case "doctor":
		return fmt.Sprintf("%s血压异常（%d/%d %s），建议查看历史趋势并调整用药", elderName, systolicInt, diastolicInt, unit)
	case "family":
		return fmt.Sprintf("您的亲人%s血压偏高，医护人员正在关注", elderName)
	default:
		return fmt.Sprintf("%s血压异常（%d/%d %s）", elderName, systolicInt, diastolicInt, unit)
	}
}

// CheckCareAlerts 检查护理告警
func (e *AlertEngine) CheckCareAlerts(ctx context.Context, elderID, elderName string) []Alert {
	var alerts []Alert

	// 这里可以根据护理记录生成告警
	// 例如：今天进食量不足、饮水量不足、未按时服药等

	// 示例：进食量不足告警
	alerts = append(alerts, Alert{
		ID:          fmt.Sprintf("food_%s_%d", elderID, time.Now().Unix()),
		ElderID:     elderID,
		ElderName:   elderName,
		Level:       AlertLevelInfo,
		Type:        "care_reminder",
		Title:       "进食提醒",
		Message:     fmt.Sprintf("%s今天进食量不足，需要关注", elderName),
		Timestamp:   time.Now(),
		TargetRoles: []string{"caregiver"},
		Data: map[string]interface{}{
			"type": "food_intake",
		},
	})

	return alerts
}

// GenerateDailyReport 生成每日报告（给家属）
func (e *AlertEngine) GenerateDailyReport(ctx context.Context, elderID, elderName string) Alert {
	// 查询今天的数据
	end := time.Now()
	start := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, end.Location())

	// 检查各项指标
	status := "良好"
	message := fmt.Sprintf("您的亲人%s今天精神状态良好，各项指标正常", elderName)

	// 检查是否有异常
	bpRecords, _ := e.vitalService.QueryVitalSignsByTimeRange(ctx, elderID, BloodPressure, start, end)
	if len(bpRecords) > 0 {
		latestBP := bpRecords[len(bpRecords)-1]
		abnormal, _ := latestBP["abnormal"].(bool)
		if abnormal {
			status = "需关注"
			message = fmt.Sprintf("您的亲人%s今天血压略高，医护人员正在关注", elderName)
		}
	}

	return Alert{
		ID:          fmt.Sprintf("daily_%s_%d", elderID, time.Now().Unix()),
		ElderID:     elderID,
		ElderName:   elderName,
		Level:       AlertLevelInfo,
		Type:        "daily_report",
		Title:       "每日健康报告",
		Message:     message,
		Timestamp:   time.Now(),
		TargetRoles: []string{"family"},
		Data: map[string]interface{}{
			"status": status,
			"date":   end.Format("2006-01-02"),
		},
	}
}

// CheckAllElders 检查所有老人的告警
func (e *AlertEngine) CheckAllElders(ctx context.Context, elders map[string]string) []Alert {
	var allAlerts []Alert

	for elderID, elderName := range elders {
		// 检查生命体征告警
		vitalAlerts := e.CheckVitalSignAlerts(ctx, elderID, elderName)
		allAlerts = append(allAlerts, vitalAlerts...)

		// 检查护理告警
		careAlerts := e.CheckCareAlerts(ctx, elderID, elderName)
		allAlerts = append(allAlerts, careAlerts...)
	}

	log.Printf("[告警引擎] 检查完成，共生成 %d 条告警", len(allAlerts))
	return allAlerts
}
