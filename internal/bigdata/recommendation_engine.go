package bigdata

import (
	"context"
	"fmt"
	"time"
)

// Recommendation 推荐信息
type Recommendation struct {
	ID          string                 `json:"id"`           // 推荐ID
	ElderID     string                 `json:"elder_id"`     // 老人ID
	ElderName   string                 `json:"elder_name"`   // 老人姓名
	Type        string                 `json:"type"`         // 推荐类型
	Title       string                 `json:"title"`        // 推荐标题
	Content     string                 `json:"content"`      // 推荐内容
	Priority    int                    `json:"priority"`     // 优先级（1-5）
	TargetRoles []string               `json:"target_roles"` // 目标角色
	Timestamp   time.Time              `json:"timestamp"`    // 生成时间
	Data        map[string]interface{} `json:"data"`         // 附加数据
}

// RecommendationEngine 推荐引擎
type RecommendationEngine struct {
	influxClient *InfluxDBClient
	vitalService *VitalSignService
}

// NewRecommendationEngine 创建推荐引擎
func NewRecommendationEngine(influxClient *InfluxDBClient) *RecommendationEngine {
	return &RecommendationEngine{
		influxClient: influxClient,
		vitalService: NewVitalSignService(influxClient),
	}
}

// GenerateCaregiverRecommendations 生成护工推荐
func (e *RecommendationEngine) GenerateCaregiverRecommendations(ctx context.Context, elderID, elderName string) []Recommendation {
	var recommendations []Recommendation

	// 基于时间的护理提醒
	now := time.Now()
	hour := now.Hour()

	// 翻身提醒（每2小时）
	if hour%2 == 0 {
		recommendations = append(recommendations, Recommendation{
			ID:          fmt.Sprintf("turn_%s_%d", elderID, now.Unix()),
			ElderID:     elderID,
			ElderName:   elderName,
			Type:        "care_reminder",
			Title:       "翻身提醒",
			Content:     fmt.Sprintf("该为%s翻身了，预防褥疮", elderName),
			Priority:    3,
			TargetRoles: []string{"caregiver"},
			Timestamp:   now,
			Data: map[string]interface{}{
				"action": "turn_over",
			},
		})
	}

	// 饮水提醒（上午10点、下午3点）
	if hour == 10 || hour == 15 {
		recommendations = append(recommendations, Recommendation{
			ID:          fmt.Sprintf("water_%s_%d", elderID, now.Unix()),
			ElderID:     elderID,
			ElderName:   elderName,
			Type:        "care_reminder",
			Title:       "饮水提醒",
			Content:     fmt.Sprintf("提醒%s喝水，保持充足水分", elderName),
			Priority:    2,
			TargetRoles: []string{"caregiver"},
			Timestamp:   now,
			Data: map[string]interface{}{
				"action": "drink_water",
			},
		})
	}

	// 活动提醒（上午9点、下午4点）
	if hour == 9 || hour == 16 {
		recommendations = append(recommendations, Recommendation{
			ID:          fmt.Sprintf("activity_%s_%d", elderID, now.Unix()),
			ElderID:     elderID,
			ElderName:   elderName,
			Type:        "care_reminder",
			Title:       "活动提醒",
			Content:     fmt.Sprintf("天气不错，可以带%s散步或做康复活动", elderName),
			Priority:    2,
			TargetRoles: []string{"caregiver"},
			Timestamp:   now,
			Data: map[string]interface{}{
				"action": "activity",
			},
		})
	}

	// 基于历史数据的推荐
	end := time.Now()
	start := end.Add(-7 * 24 * time.Hour)

	// 检查血压趋势
	bpRecords, err := e.vitalService.QueryVitalSignsByTimeRange(ctx, elderID, BloodPressure, start, end)
	if err == nil && len(bpRecords) > 0 {
		// 如果最近血压偏高，建议增加测量频率
		latestBP := bpRecords[len(bpRecords)-1]
		if func() bool { abnormal, _ := latestBP["abnormal"].(bool); return abnormal }() {
			recommendations = append(recommendations, Recommendation{
				ID:          fmt.Sprintf("bp_monitor_%s_%d", elderID, now.Unix()),
				ElderID:     elderID,
				ElderName:   elderName,
				Type:        "monitoring_suggestion",
				Title:       "血压监测建议",
				Content:     fmt.Sprintf("%s血压偏高，建议增加测量频率（每天3次）", elderName),
				Priority:    4,
				TargetRoles: []string{"caregiver"},
				Timestamp:   now,
				Data: map[string]interface{}{
					"frequency": "3_times_daily",
				},
			})
		}
	}

	return recommendations
}

// GenerateDoctorRecommendations 生成医生推荐
func (e *RecommendationEngine) GenerateDoctorRecommendations(ctx context.Context, elderID, elderName string) []Recommendation {
	var recommendations []Recommendation

	now := time.Now()
	end := now
	start := end.Add(-30 * 24 * time.Hour)

	// 分析血压趋势
	bpRecords, err := e.vitalService.QueryVitalSignsByTimeRange(ctx, elderID, BloodPressure, start, end)
	if err == nil && len(bpRecords) >= 5 {
		// 计算平均值
		var systolicSum, diastolicSum float64
		abnormalCount := 0
		for _, record := range bpRecords {
			systolicSum += func() float64 { v, _ := record["systolic"].(float64); return v }()
			if func() bool { _, ok := record["diastolic"]; return ok }() {
				diastolicSum += func() float64 { v, _ := record["diastolic"].(float64); return v }()
			}
			if func() bool { abnormal, _ := record["abnormal"].(bool); return abnormal }() {
				abnormalCount++
			}
		}
		avgSystolic := systolicSum / float64(len(bpRecords))
		avgDiastolic := diastolicSum / float64(len(bpRecords))

		// 如果平均血压偏高
		if avgSystolic > 140 || avgDiastolic > 90 {
			recommendations = append(recommendations, Recommendation{
				ID:          fmt.Sprintf("bp_treatment_%s_%d", elderID, now.Unix()),
				ElderID:     elderID,
				ElderName:   elderName,
				Type:        "treatment_suggestion",
				Title:       "用药调整建议",
				Content:     fmt.Sprintf("%s近30天平均血压%.0f/%.0f mmHg，建议调整降压药剂量", elderName, avgSystolic, avgDiastolic),
				Priority:    5,
				TargetRoles: []string{"doctor"},
				Timestamp:   now,
				Data: map[string]interface{}{
					"avg_systolic":    avgSystolic,
					"avg_diastolic":   avgDiastolic,
					"abnormal_count":  abnormalCount,
					"total_records":   len(bpRecords),
					"abnormal_rate":   float64(abnormalCount) / float64(len(bpRecords)),
				},
			})
		}

		// 如果异常率超过50%
		if float64(abnormalCount)/float64(len(bpRecords)) > 0.5 {
			recommendations = append(recommendations, Recommendation{
				ID:          fmt.Sprintf("bp_checkup_%s_%d", elderID, now.Unix()),
				ElderID:     elderID,
				ElderName:   elderName,
				Type:        "checkup_suggestion",
				Title:       "检查建议",
				Content:     fmt.Sprintf("%s血压异常率%.0f%%，建议进行心血管系统检查", elderName, float64(abnormalCount)/float64(len(bpRecords))*100),
				Priority:    4,
				TargetRoles: []string{"doctor"},
				Timestamp:   now,
				Data: map[string]interface{}{
					"abnormal_rate": float64(abnormalCount) / float64(len(bpRecords)),
				},
			})
		}
	}

	// 检查血糖趋势
	bgRecords, err := e.vitalService.QueryVitalSignsByTimeRange(ctx, elderID, BloodSugar, start, end)
	if err == nil && len(bgRecords) >= 3 {
		var bgSum float64
		for _, record := range bgRecords {
			bgSum += func() float64 { v, _ := record["value"].(float64); return v }()
		}
		avgBG := bgSum / float64(len(bgRecords))

		if avgBG > 7.0 {
			recommendations = append(recommendations, Recommendation{
				ID:          fmt.Sprintf("bg_treatment_%s_%d", elderID, now.Unix()),
				ElderID:     elderID,
				ElderName:   elderName,
				Type:        "treatment_suggestion",
				Title:       "血糖管理建议",
				Content:     fmt.Sprintf("%s平均血糖%.1f mmol/L，建议调整降糖方案或饮食控制", elderName, avgBG),
				Priority:    4,
				TargetRoles: []string{"doctor"},
				Timestamp:   now,
				Data: map[string]interface{}{
					"avg_blood_glucose": avgBG,
				},
			})
		}
	}

	// 定期体检提醒
	recommendations = append(recommendations, Recommendation{
		ID:          fmt.Sprintf("checkup_%s_%d", elderID, now.Unix()),
		ElderID:     elderID,
		ElderName:   elderName,
		Type:        "checkup_reminder",
		Title:       "定期体检提醒",
		Content:     fmt.Sprintf("%s距离上次全面体检已3个月，建议安排体检", elderName),
		Priority:    3,
		TargetRoles: []string{"doctor"},
		Timestamp:   now,
		Data: map[string]interface{}{
			"last_checkup": "3_months_ago",
		},
	})

	return recommendations
}

// GenerateFamilyRecommendations 生成家属推荐
func (e *RecommendationEngine) GenerateFamilyRecommendations(ctx context.Context, elderID, elderName string) []Recommendation {
	var recommendations []Recommendation

	now := time.Now()

	// 探视建议
	dayOfWeek := now.Weekday()
	if dayOfWeek == time.Saturday || dayOfWeek == time.Sunday {
		recommendations = append(recommendations, Recommendation{
			ID:          fmt.Sprintf("visit_%s_%d", elderID, now.Unix()),
			ElderID:     elderID,
			ElderName:   elderName,
			Type:        "visit_suggestion",
			Title:       "探视建议",
			Content:     fmt.Sprintf("今天是周末，%s很想念您，有空可以来探望", elderName),
			Priority:    3,
			TargetRoles: []string{"family"},
			Timestamp:   now,
			Data: map[string]interface{}{
				"day": dayOfWeek.String(),
			},
		})
	}

	// 生日提醒（示例）
	// 这里可以根据实际的生日数据生成提醒

	// 健康状况总结
	end := now
	start := end.Add(-7 * 24 * time.Hour)
	bpRecords, err := e.vitalService.QueryVitalSignsByTimeRange(ctx, elderID, BloodPressure, start, end)
	if err == nil && len(bpRecords) > 0 {
		abnormalCount := 0
		for _, record := range bpRecords {
			if func() bool { abnormal, _ := record["abnormal"].(bool); return abnormal }() {
				abnormalCount++
			}
		}

		if abnormalCount == 0 {
			recommendations = append(recommendations, Recommendation{
				ID:          fmt.Sprintf("health_good_%s_%d", elderID, now.Unix()),
				ElderID:     elderID,
				ElderName:   elderName,
				Type:        "health_summary",
				Title:       "健康状况良好",
				Content:     fmt.Sprintf("%s本周各项指标正常，请放心", elderName),
				Priority:    2,
				TargetRoles: []string{"family"},
				Timestamp:   now,
				Data: map[string]interface{}{
					"status": "good",
				},
			})
		}
	}

	return recommendations
}

// GenerateElderRecommendations 生成老人推荐
func (e *RecommendationEngine) GenerateElderRecommendations(ctx context.Context, elderID, elderName string) []Recommendation {
	var recommendations []Recommendation

	now := time.Now()
	hour := now.Hour()

	// 用药提醒（早8点、中12点、晚6点）
	if hour == 8 || hour == 12 || hour == 18 {
		recommendations = append(recommendations, Recommendation{
			ID:          fmt.Sprintf("medicine_%s_%d", elderID, now.Unix()),
			ElderID:     elderID,
			ElderName:   elderName,
			Type:        "medicine_reminder",
			Title:       "用药提醒",
			Content:     "该吃药了，请按时服药",
			Priority:    5,
			TargetRoles: []string{"elder"},
			Timestamp:   now,
			Data: map[string]interface{}{
				"time": hour,
			},
		})
	}

	// 活动建议
	if hour >= 9 && hour <= 16 {
		recommendations = append(recommendations, Recommendation{
			ID:          fmt.Sprintf("activity_elder_%s_%d", elderID, now.Unix()),
			ElderID:     elderID,
			ElderName:   elderName,
			Type:        "activity_suggestion",
			Title:       "活动建议",
			Content:     "今天天气不错，适合散步或参加活动",
			Priority:    2,
			TargetRoles: []string{"elder"},
			Timestamp:   now,
			Data: map[string]interface{}{
				"weather": "good",
			},
		})
	}

	// 休息提醒（晚上9点）
	if hour == 21 {
		recommendations = append(recommendations, Recommendation{
			ID:          fmt.Sprintf("rest_%s_%d", elderID, now.Unix()),
			ElderID:     elderID,
			ElderName:   elderName,
			Type:        "rest_reminder",
			Title:       "休息提醒",
			Content:     "该准备休息了，早睡早起身体好",
			Priority:    3,
			TargetRoles: []string{"elder"},
			Timestamp:   now,
			Data: map[string]interface{}{
				"time": "bedtime",
			},
		})
	}

	return recommendations
}

// GenerateAllRecommendations 生成所有推荐
func (e *RecommendationEngine) GenerateAllRecommendations(ctx context.Context, elderID, elderName string, role string) []Recommendation {
	switch role {
	case "caregiver":
		return e.GenerateCaregiverRecommendations(ctx, elderID, elderName)
	case "doctor":
		return e.GenerateDoctorRecommendations(ctx, elderID, elderName)
	case "family":
		return e.GenerateFamilyRecommendations(ctx, elderID, elderName)
	case "elder":
		return e.GenerateElderRecommendations(ctx, elderID, elderName)
	default:
		return []Recommendation{}
	}
}
