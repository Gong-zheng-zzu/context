package health

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/contextkeeper/service/internal/security"
)

// HealthRecordCategory 健康记录类别
type HealthRecordCategory string

const (
	// 养老院护理场景类别
	CategoryDailyCare   HealthRecordCategory = "daily_care"   // 日常护理（洗漱、进食、如厕、翻身、清洁）
	CategoryVitalSigns  HealthRecordCategory = "vital_signs"  // 生命体征（血压、体温、心率、血氧、血糖）
	CategoryMedication  HealthRecordCategory = "medication"   // 用药记录（用药时间、剂量、反应）
	CategoryIncident    HealthRecordCategory = "incident"     // 异常事件（跌倒、走失、情绪异常、拒食）
	CategoryActivity    HealthRecordCategory = "activity"     // 活动记录（康复训练、娱乐活动、社交互动）
	CategoryDiet        HealthRecordCategory = "diet"         // 饮食记录（进食量、饮水量、特殊饮食）

	// 保留旧类别以支持向后兼容
	CategoryPhysicalExam HealthRecordCategory = "physical_exam" // 体检记录（已废弃，映射到vital_signs）
	CategoryMedicalVisit HealthRecordCategory = "medical_visit" // 就诊记录（已废弃，映射到incident）
	CategorySymptom      HealthRecordCategory = "symptom"       // 症状记录（已废弃，映射到incident）
	CategoryLabTest      HealthRecordCategory = "lab_test"      // 化验记录（已废弃，映射到vital_signs）
	CategoryOther        HealthRecordCategory = "other"         // 其他记录（已废弃，映射到daily_care）
)

// HealthRecord 健康记录
type HealthRecord struct {
	ID          string                 `json:"id"`
	UserID      string                 `json:"user_id"`
	Category    HealthRecordCategory   `json:"category"`
	Date        time.Time              `json:"date"`
	Title       string                 `json:"title"`
	Content     string                 `json:"content"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Tags        []string               `json:"tags,omitempty"`
	IsImportant bool                   `json:"is_important"` // 是否为重要事件
}

// OrganizedArchive 整理后的健康档案
type OrganizedArchive struct {
	UserID           string                              `json:"user_id"`
	GeneratedAt      time.Time                           `json:"generated_at"`
	RecordsByCategory map[HealthRecordCategory][]HealthRecord `json:"records_by_category"`
	Timeline         []TimelineEvent                     `json:"timeline"`
	TrendAnalysis    []TrendAnalysis                     `json:"trend_analysis"`
	Summary          string                              `json:"summary"`
	Disclaimer       string                              `json:"disclaimer"`
}

// TimelineEvent 时间线事件
type TimelineEvent struct {
	Date        time.Time            `json:"date"`
	Category    HealthRecordCategory `json:"category"`
	Title       string               `json:"title"`
	Description string               `json:"description"`
	IsImportant bool                 `json:"is_important"`
}

// TrendAnalysis 趋势分析
type TrendAnalysis struct {
	Indicator   string      `json:"indicator"`    // 指标名称（如"血压"、"血糖"）
	DataPoints  []DataPoint `json:"data_points"`  // 数据点
	Trend       string      `json:"trend"`        // 趋势描述（改善/稳定/恶化）
	Description string      `json:"description"`  // 详细描述
}

// DataPoint 数据点
type DataPoint struct {
	Date  time.Time `json:"date"`
	Value float64   `json:"value"`
	Unit  string    `json:"unit"`
	Note  string    `json:"note,omitempty"`
}

// ArchiveOrganizer 健康档案整理器
type ArchiveOrganizer struct {
	detector *security.Detector // 敏感信息检测器
}

// NewArchiveOrganizer 创建健康档案整理器
func NewArchiveOrganizer(detector *security.Detector) *ArchiveOrganizer {
	if detector == nil {
		detector = security.NewDetector()
	}
	return &ArchiveOrganizer{
		detector: detector,
	}
}

// OrganizeRecords 整理健康记录
// 输入：用户的所有健康记录
// 输出：按类别整理的档案
func (ao *ArchiveOrganizer) OrganizeRecords(ctx context.Context, userID string, records []HealthRecord) (*OrganizedArchive, error) {
	if len(records) == 0 {
		return nil, fmt.Errorf("没有可整理的健康记录")
	}

	// 1. 脱敏处理所有记录
	sanitizedRecords, err := ao.sanitizeRecords(records)
	if err != nil {
		return nil, fmt.Errorf("脱敏处理失败: %w", err)
	}

	// 2. 按类别分组
	recordsByCategory := ao.OrganizeByCategory(sanitizedRecords)

	// 3. 生成时间线
	timeline := ao.GenerateTimeline(sanitizedRecords)

	// 4. 分析趋势
	trendAnalysis := ao.AnalyzeTrend(sanitizedRecords)

	// 5. 生成摘要
	summary := ao.generateSummary(recordsByCategory, timeline, trendAnalysis)

	// 6. 构建整理后的档案
	archive := &OrganizedArchive{
		UserID:            userID,
		GeneratedAt:       time.Now(),
		RecordsByCategory: recordsByCategory,
		Timeline:          timeline,
		TrendAnalysis:     trendAnalysis,
		Summary:           summary,
		Disclaimer:        "本档案仅供参考，不构成医学诊断或治疗建议。如有健康问题，请咨询专业医疗机构。",
	}

	return archive, nil
}

// OrganizeByCategory 按类别整理记录
func (ao *ArchiveOrganizer) OrganizeByCategory(records []HealthRecord) map[HealthRecordCategory][]HealthRecord {
	categoryMap := make(map[HealthRecordCategory][]HealthRecord)

	for _, record := range records {
		categoryMap[record.Category] = append(categoryMap[record.Category], record)
	}

	// 对每个类别的记录按时间排序（最新的在前）
	for category := range categoryMap {
		sort.Slice(categoryMap[category], func(i, j int) bool {
			return categoryMap[category][i].Date.After(categoryMap[category][j].Date)
		})
	}

	return categoryMap
}

// GenerateTimeline 生成健康事件时间线
func (ao *ArchiveOrganizer) GenerateTimeline(records []HealthRecord) []TimelineEvent {
	var timeline []TimelineEvent

	for _, record := range records {
		event := TimelineEvent{
			Date:        record.Date,
			Category:    record.Category,
			Title:       record.Title,
			Description: ao.extractBriefDescription(record.Content),
			IsImportant: record.IsImportant,
		}
		timeline = append(timeline, event)
	}

	// 按时间排序（最新的在前）
	sort.Slice(timeline, func(i, j int) bool {
		return timeline[i].Date.After(timeline[j].Date)
	})

	return timeline
}

// AnalyzeTrend 分析健康数据趋势
// 识别改善/恶化趋势，不做诊断，只描述数据变化
func (ao *ArchiveOrganizer) AnalyzeTrend(records []HealthRecord) []TrendAnalysis {
	// 提取可量化的健康指标
	indicatorData := ao.extractIndicatorData(records)

	var trends []TrendAnalysis

	for indicator, dataPoints := range indicatorData {
		if len(dataPoints) < 2 {
			continue // 至少需要2个数据点才能分析趋势
		}

		// 按时间排序
		sort.Slice(dataPoints, func(i, j int) bool {
			return dataPoints[i].Date.Before(dataPoints[j].Date)
		})

		// 计算趋势
		trend := ao.calculateTrend(dataPoints)
		description := ao.generateTrendDescription(indicator, dataPoints, trend)

		trends = append(trends, TrendAnalysis{
			Indicator:   indicator,
			DataPoints:  dataPoints,
			Trend:       trend,
			Description: description,
		})
	}

	return trends
}

// sanitizeRecords 脱敏处理记录
func (ao *ArchiveOrganizer) sanitizeRecords(records []HealthRecord) ([]HealthRecord, error) {
	sanitized := make([]HealthRecord, len(records))

	for i, record := range records {
		// 脱敏标题
		sanitizedTitle, _ := ao.detector.DetectAndRedact(record.Title)

		// 脱敏内容
		sanitizedContent, sensitiveInfos := ao.detector.DetectAndRedact(record.Content)

		// 记录检测到的敏感信息类型（用于审计）
		if len(sensitiveInfos) > 0 {
			if record.Metadata == nil {
				record.Metadata = make(map[string]interface{})
			}
			var types []string
			for _, info := range sensitiveInfos {
				types = append(types, string(info.Type))
			}
			record.Metadata["sanitized_types"] = types
		}

		sanitized[i] = HealthRecord{
			ID:          record.ID,
			UserID:      record.UserID,
			Category:    record.Category,
			Date:        record.Date,
			Title:       sanitizedTitle,
			Content:     sanitizedContent,
			Metadata:    record.Metadata,
			Tags:        record.Tags,
			IsImportant: record.IsImportant,
		}
	}

	return sanitized, nil
}

// extractBriefDescription 提取简要描述（前100个字符）
func (ao *ArchiveOrganizer) extractBriefDescription(content string) string {
	// 移除多余的空白字符
	content = strings.TrimSpace(content)
	content = strings.ReplaceAll(content, "\n", " ")
	content = strings.ReplaceAll(content, "\r", " ")

	// 限制长度
	maxLen := 100
	if len(content) <= maxLen {
		return content
	}

	return content[:maxLen] + "..."
}

// extractIndicatorData 从记录中提取可量化的健康指标数据
func (ao *ArchiveOrganizer) extractIndicatorData(records []HealthRecord) map[string][]DataPoint {
	indicatorData := make(map[string][]DataPoint)

	for _, record := range records {
		// 从metadata中提取结构化的健康指标
		if record.Metadata != nil {
			if indicators, ok := record.Metadata["indicators"].(map[string]interface{}); ok {
				for name, value := range indicators {
					dataPoint := ao.parseIndicatorValue(record.Date, name, value)
					if dataPoint != nil {
						indicatorData[name] = append(indicatorData[name], *dataPoint)
					}
				}
			}
		}

		// 从内容中提取常见健康指标（简单的模式匹配）
		ao.extractIndicatorsFromContent(record, indicatorData)
	}

	return indicatorData
}

// parseIndicatorValue 解析指标值
func (ao *ArchiveOrganizer) parseIndicatorValue(date time.Time, name string, value interface{}) *DataPoint {
	switch v := value.(type) {
	case float64:
		return &DataPoint{
			Date:  date,
			Value: v,
			Unit:  ao.getDefaultUnit(name),
		}
	case map[string]interface{}:
		// 支持更复杂的结构：{"value": 120, "unit": "mmHg"}
		if val, ok := v["value"].(float64); ok {
			unit := ao.getDefaultUnit(name)
			if u, ok := v["unit"].(string); ok {
				unit = u
			}
			note := ""
			if n, ok := v["note"].(string); ok {
				note = n
			}
			return &DataPoint{
				Date:  date,
				Value: val,
				Unit:  unit,
				Note:  note,
			}
		}
	}
	return nil
}

// getDefaultUnit 获取指标的默认单位
func (ao *ArchiveOrganizer) getDefaultUnit(indicator string) string {
	units := map[string]string{
		"血压":   "mmHg",
		"收缩压":  "mmHg",
		"舒张压":  "mmHg",
		"血糖":   "mmol/L",
		"体重":   "kg",
		"身高":   "cm",
		"体温":   "℃",
		"心率":   "次/分",
		"血氧":   "%",
		"胆固醇":  "mmol/L",
		"甘油三酯": "mmol/L",
	}

	if unit, ok := units[indicator]; ok {
		return unit
	}
	return ""
}

// extractIndicatorsFromContent 从内容中提取健康指标（简单实现）
func (ao *ArchiveOrganizer) extractIndicatorsFromContent(record HealthRecord, indicatorData map[string][]DataPoint) {
	// 这里可以实现更复杂的NLP提取逻辑
	// 当前仅作为示例，实际应用中可以使用正则表达式或NLP模型
	content := record.Content

	// 示例：提取血压数据 "血压140/90"
	if strings.Contains(content, "血压") {
		// 简化处理，实际应使用正则表达式
		// 这里仅作为占位符
	}

	// 示例：提取血糖数据 "血糖6.5"
	if strings.Contains(content, "血糖") {
		// 简化处理
	}
}

// calculateTrend 计算趋势
func (ao *ArchiveOrganizer) calculateTrend(dataPoints []DataPoint) string {
	if len(dataPoints) < 2 {
		return "数据不足"
	}

	// 简单的线性趋势计算
	firstValue := dataPoints[0].Value
	lastValue := dataPoints[len(dataPoints)-1].Value

	// 计算变化率
	changeRate := (lastValue - firstValue) / firstValue * 100

	if changeRate > 5 {
		return "上升"
	} else if changeRate < -5 {
		return "下降"
	} else {
		return "稳定"
	}
}

// generateTrendDescription 生成趋势描述
func (ao *ArchiveOrganizer) generateTrendDescription(indicator string, dataPoints []DataPoint, trend string) string {
	if len(dataPoints) < 2 {
		return fmt.Sprintf("%s数据不足，无法分析趋势", indicator)
	}

	firstPoint := dataPoints[0]
	lastPoint := dataPoints[len(dataPoints)-1]

	description := fmt.Sprintf("%s在%s至%s期间呈%s趋势。",
		indicator,
		firstPoint.Date.Format("2006-01-02"),
		lastPoint.Date.Format("2006-01-02"),
		trend,
	)

	// 添加具体数值
	description += fmt.Sprintf("从%.2f%s变化至%.2f%s。",
		firstPoint.Value,
		firstPoint.Unit,
		lastPoint.Value,
		lastPoint.Unit,
	)

	// 添加数据点数量
	description += fmt.Sprintf("（共%d次记录）", len(dataPoints))

	return description
}

// generateSummary 生成档案摘要
func (ao *ArchiveOrganizer) generateSummary(
	recordsByCategory map[HealthRecordCategory][]HealthRecord,
	timeline []TimelineEvent,
	trends []TrendAnalysis,
) string {
	var summary strings.Builder

	summary.WriteString("健康档案摘要\n\n")

	// 统计各类别记录数量
	summary.WriteString("记录统计：\n")
	categoryNames := map[HealthRecordCategory]string{
		CategoryPhysicalExam: "体检记录",
		CategoryMedicalVisit: "就诊记录",
		CategoryMedication:   "用药记录",
		CategorySymptom:      "症状记录",
		CategoryLabTest:      "化验记录",
		CategoryOther:        "其他记录",
	}

	for category, records := range recordsByCategory {
		if len(records) > 0 {
			name := categoryNames[category]
			if name == "" {
				name = string(category)
			}
			summary.WriteString(fmt.Sprintf("- %s：%d条\n", name, len(records)))
		}
	}

	// 重要事件
	importantEvents := 0
	for _, event := range timeline {
		if event.IsImportant {
			importantEvents++
		}
	}
	if importantEvents > 0 {
		summary.WriteString(fmt.Sprintf("\n重要事件：%d个\n", importantEvents))
	}

	// 趋势分析摘要
	if len(trends) > 0 {
		summary.WriteString("\n健康趋势：\n")
		for _, trend := range trends {
			summary.WriteString(fmt.Sprintf("- %s：%s\n", trend.Indicator, trend.Trend))
		}
	}

	return summary.String()
}

// FormatArchiveAsText 将档案格式化为文本（用于展示）
func (ao *ArchiveOrganizer) FormatArchiveAsText(archive *OrganizedArchive) string {
	var output strings.Builder

	output.WriteString("=" + strings.Repeat("=", 50) + "=\n")
	output.WriteString("健康档案\n")
	output.WriteString("生成时间：" + archive.GeneratedAt.Format("2006-01-02 15:04:05") + "\n")
	output.WriteString("=" + strings.Repeat("=", 50) + "=\n\n")

	// 摘要
	output.WriteString(archive.Summary)
	output.WriteString("\n")

	// 按类别展示记录
	categoryNames := map[HealthRecordCategory]string{
		CategoryPhysicalExam: "体检记录",
		CategoryMedicalVisit: "就诊记录",
		CategoryMedication:   "用药记录",
		CategorySymptom:      "症状记录",
		CategoryLabTest:      "化验记录",
		CategoryOther:        "其他记录",
	}

	for category, records := range archive.RecordsByCategory {
		if len(records) == 0 {
			continue
		}

		name := categoryNames[category]
		if name == "" {
			name = string(category)
		}

		output.WriteString("\n" + strings.Repeat("-", 50) + "\n")
		output.WriteString(name + "\n")
		output.WriteString(strings.Repeat("-", 50) + "\n")

		for _, record := range records {
			output.WriteString(fmt.Sprintf("\n[%s] %s\n",
				record.Date.Format("2006-01-02"),
				record.Title,
			))
			output.WriteString(record.Content + "\n")
		}
	}

	// 趋势分析
	if len(archive.TrendAnalysis) > 0 {
		output.WriteString("\n" + strings.Repeat("-", 50) + "\n")
		output.WriteString("健康趋势分析\n")
		output.WriteString(strings.Repeat("-", 50) + "\n")

		for _, trend := range archive.TrendAnalysis {
			output.WriteString(fmt.Sprintf("\n%s：\n", trend.Indicator))
			output.WriteString(trend.Description + "\n")
		}
	}

	// 免责声明
	output.WriteString("\n" + strings.Repeat("=", 50) + "\n")
	output.WriteString("免责声明\n")
	output.WriteString(strings.Repeat("=", 50) + "\n")
	output.WriteString(archive.Disclaimer + "\n")

	return output.String()
}
