package health

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/contextkeeper/service/internal/security"
)

// TestNewArchiveOrganizer 测试创建档案整理器
func TestNewArchiveOrganizer(t *testing.T) {
	t.Run("使用自定义检测器", func(t *testing.T) {
		detector := security.NewDetector()
		organizer := NewArchiveOrganizer(detector)

		if organizer == nil {
			t.Fatal("创建档案整理器失败")
		}

		if organizer.detector == nil {
			t.Error("检测器未正确设置")
		}
	})

	t.Run("使用默认检测器", func(t *testing.T) {
		organizer := NewArchiveOrganizer(nil)

		if organizer == nil {
			t.Fatal("创建档案整理器失败")
		}

		if organizer.detector == nil {
			t.Error("默认检测器未创建")
		}
	})
}

// TestOrganizeByCategory 测试按类别整理
func TestOrganizeByCategory(t *testing.T) {
	organizer := NewArchiveOrganizer(nil)

	records := []HealthRecord{
		{
			ID:       "1",
			Category: CategoryPhysicalExam,
			Date:     time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			Title:    "年度体检",
			Content:  "血压140/90，血糖6.5",
		},
		{
			ID:       "2",
			Category: CategoryMedicalVisit,
			Date:     time.Date(2024, 2, 20, 0, 0, 0, 0, time.UTC),
			Title:    "心内科就诊",
			Content:  "高血压复查",
		},
		{
			ID:       "3",
			Category: CategoryPhysicalExam,
			Date:     time.Date(2024, 4, 5, 0, 0, 0, 0, time.UTC),
			Title:    "复查体检",
			Content:  "血压130/85，血糖6.0",
		},
		{
			ID:       "4",
			Category: CategoryMedication,
			Date:     time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
			Title:    "开始用药",
			Content:  "开始服用降压药",
		},
	}

	result := organizer.OrganizeByCategory(records)

	// 验证分类
	if len(result) != 3 {
		t.Errorf("期望3个类别，实际得到%d个", len(result))
	}

	// 验证体检记录
	physicalExams := result[CategoryPhysicalExam]
	if len(physicalExams) != 2 {
		t.Errorf("期望2条体检记录，实际得到%d条", len(physicalExams))
	}

	// 验证时间排序（最新的在前）
	if !physicalExams[0].Date.After(physicalExams[1].Date) {
		t.Error("体检记录未按时间倒序排列")
	}

	// 验证就诊记录
	medicalVisits := result[CategoryMedicalVisit]
	if len(medicalVisits) != 1 {
		t.Errorf("期望1条就诊记录，实际得到%d条", len(medicalVisits))
	}

	// 验证用药记录
	medications := result[CategoryMedication]
	if len(medications) != 1 {
		t.Errorf("期望1条用药记录，实际得到%d条", len(medications))
	}
}

// TestGenerateTimeline 测试生成时间线
func TestGenerateTimeline(t *testing.T) {
	organizer := NewArchiveOrganizer(nil)

	records := []HealthRecord{
		{
			ID:          "1",
			Category:    CategoryPhysicalExam,
			Date:        time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			Title:       "年度体检",
			Content:     "血压140/90，血糖6.5",
			IsImportant: true,
		},
		{
			ID:       "2",
			Category: CategoryMedicalVisit,
			Date:     time.Date(2024, 2, 20, 0, 0, 0, 0, time.UTC),
			Title:    "心内科就诊",
			Content:  "高血压复查",
		},
		{
			ID:       "3",
			Category: CategoryMedication,
			Date:     time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
			Title:    "开始用药",
			Content:  "开始服用降压药，每日一次",
		},
	}

	timeline := organizer.GenerateTimeline(records)

	// 验证时间线长度
	if len(timeline) != 3 {
		t.Errorf("期望3个时间线事件，实际得到%d个", len(timeline))
	}

	// 验证时间排序（最新的在前）
	for i := 0; i < len(timeline)-1; i++ {
		if timeline[i].Date.Before(timeline[i+1].Date) {
			t.Error("时间线未按时间倒序排列")
		}
	}

	// 验证重要事件标记
	importantCount := 0
	for _, event := range timeline {
		if event.IsImportant {
			importantCount++
		}
	}
	if importantCount != 1 {
		t.Errorf("期望1个重要事件，实际得到%d个", importantCount)
	}

	// 验证描述提取
	for _, event := range timeline {
		if event.Description == "" {
			t.Error("时间线事件描述为空")
		}
	}
}

// TestAnalyzeTrend 测试趋势分析
func TestAnalyzeTrend(t *testing.T) {
	organizer := NewArchiveOrganizer(nil)

	records := []HealthRecord{
		{
			ID:       "1",
			Category: CategoryPhysicalExam,
			Date:     time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			Title:    "体检1",
			Content:  "血压检查",
			Metadata: map[string]interface{}{
				"indicators": map[string]interface{}{
					"收缩压": map[string]interface{}{
						"value": 140.0,
						"unit":  "mmHg",
					},
					"舒张压": map[string]interface{}{
						"value": 90.0,
						"unit":  "mmHg",
					},
					"血糖": map[string]interface{}{
						"value": 6.5,
						"unit":  "mmol/L",
					},
				},
			},
		},
		{
			ID:       "2",
			Category: CategoryPhysicalExam,
			Date:     time.Date(2024, 2, 15, 0, 0, 0, 0, time.UTC),
			Title:    "体检2",
			Content:  "血压检查",
			Metadata: map[string]interface{}{
				"indicators": map[string]interface{}{
					"收缩压": map[string]interface{}{
						"value": 135.0,
						"unit":  "mmHg",
					},
					"舒张压": map[string]interface{}{
						"value": 88.0,
						"unit":  "mmHg",
					},
					"血糖": map[string]interface{}{
						"value": 6.2,
						"unit":  "mmol/L",
					},
				},
			},
		},
		{
			ID:       "3",
			Category: CategoryPhysicalExam,
			Date:     time.Date(2024, 4, 5, 0, 0, 0, 0, time.UTC),
			Title:    "体检3",
			Content:  "血压检查",
			Metadata: map[string]interface{}{
				"indicators": map[string]interface{}{
					"收缩压": map[string]interface{}{
						"value": 130.0,
						"unit":  "mmHg",
					},
					"舒张压": map[string]interface{}{
						"value": 85.0,
						"unit":  "mmHg",
					},
					"血糖": map[string]interface{}{
						"value": 6.0,
						"unit":  "mmol/L",
					},
				},
			},
		},
	}

	trends := organizer.AnalyzeTrend(records)

	// 验证趋势数量
	if len(trends) == 0 {
		t.Fatal("未生成任何趋势分析")
	}

	// 验证趋势包含预期的指标
	expectedIndicators := map[string]bool{
		"收缩压": false,
		"舒张压": false,
		"血糖":  false,
	}

	for _, trend := range trends {
		if _, exists := expectedIndicators[trend.Indicator]; exists {
			expectedIndicators[trend.Indicator] = true
		}

		// 验证数据点
		if len(trend.DataPoints) < 2 {
			t.Errorf("指标%s的数据点不足", trend.Indicator)
		}

		// 验证趋势描述
		if trend.Trend == "" {
			t.Errorf("指标%s的趋势为空", trend.Indicator)
		}

		if trend.Description == "" {
			t.Errorf("指标%s的描述为空", trend.Indicator)
		}

		// 验证数据点按时间排序
		for i := 0; i < len(trend.DataPoints)-1; i++ {
			if trend.DataPoints[i].Date.After(trend.DataPoints[i+1].Date) {
				t.Errorf("指标%s的数据点未按时间正序排列", trend.Indicator)
			}
		}
	}

	// 验证所有预期指标都被分析
	for indicator, found := range expectedIndicators {
		if !found {
			t.Errorf("未找到指标%s的趋势分析", indicator)
		}
	}
}

// TestSanitizeRecords 测试脱敏处理
func TestSanitizeRecords(t *testing.T) {
	organizer := NewArchiveOrganizer(nil)

	records := []HealthRecord{
		{
			ID:       "1",
			Category: CategoryMedicalVisit,
			Date:     time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			Title:    "就诊记录",
			Content:  "患者张三，手机号13812345678，就诊于XX医院",
		},
		{
			ID:       "2",
			Category: CategoryPhysicalExam,
			Date:     time.Date(2024, 2, 20, 0, 0, 0, 0, time.UTC),
			Title:    "体检报告",
			Content:  "身份证号：110101199001011234，体检结果正常",
		},
	}

	sanitized, err := organizer.sanitizeRecords(records)
	if err != nil {
		t.Fatalf("脱敏处理失败: %v", err)
	}

	// 验证记录数量
	if len(sanitized) != len(records) {
		t.Errorf("脱敏后记录数量不匹配")
	}

	// 验证手机号已脱敏
	if strings.Contains(sanitized[0].Content, "13812345678") {
		t.Error("手机号未被脱敏")
	}

	// 验证身份证号已脱敏
	if strings.Contains(sanitized[1].Content, "110101199001011234") {
		t.Error("身份证号未被脱敏")
	}

	// 验证脱敏类型已记录
	if sanitized[0].Metadata == nil {
		t.Error("脱敏元数据未记录")
	} else {
		if types, ok := sanitized[0].Metadata["sanitized_types"].([]string); ok {
			if len(types) == 0 {
				t.Error("未记录脱敏类型")
			}
		}
	}
}

// TestOrganizeRecords 测试完整的整理流程
func TestOrganizeRecords(t *testing.T) {
	organizer := NewArchiveOrganizer(nil)
	ctx := context.Background()

	records := []HealthRecord{
		{
			ID:          "1",
			UserID:      "user123",
			Category:    CategoryPhysicalExam,
			Date:        time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			Title:       "年度体检",
			Content:     "血压140/90，血糖6.5",
			IsImportant: true,
			Metadata: map[string]interface{}{
				"indicators": map[string]interface{}{
					"收缩压": 140.0,
					"血糖":  6.5,
				},
			},
		},
		{
			ID:       "2",
			UserID:   "user123",
			Category: CategoryMedicalVisit,
			Date:     time.Date(2024, 2, 20, 0, 0, 0, 0, time.UTC),
			Title:    "心内科就诊",
			Content:  "高血压复查，手机号13812345678",
		},
		{
			ID:       "3",
			UserID:   "user123",
			Category: CategoryMedication,
			Date:     time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
			Title:    "开始用药",
			Content:  "开始服用降压药",
		},
	}

	archive, err := organizer.OrganizeRecords(ctx, "user123", records)
	if err != nil {
		t.Fatalf("整理记录失败: %v", err)
	}

	// 验证基本信息
	if archive.UserID != "user123" {
		t.Errorf("用户ID不匹配")
	}

	if archive.GeneratedAt.IsZero() {
		t.Error("生成时间未设置")
	}

	// 验证分类记录
	if len(archive.RecordsByCategory) == 0 {
		t.Error("未生成分类记录")
	}

	// 验证时间线
	if len(archive.Timeline) != 3 {
		t.Errorf("期望3个时间线事件，实际得到%d个", len(archive.Timeline))
	}

	// 验证摘要
	if archive.Summary == "" {
		t.Error("摘要为空")
	}

	// 验证免责声明
	if archive.Disclaimer == "" {
		t.Error("免责声明为空")
	}

	if !strings.Contains(archive.Disclaimer, "仅供参考") {
		t.Error("免责声明内容不完整")
	}

	// 验证敏感信息已脱敏
	for _, records := range archive.RecordsByCategory {
		for _, record := range records {
			if strings.Contains(record.Content, "13812345678") {
				t.Error("敏感信息未被脱敏")
			}
		}
	}
}

// TestOrganizeRecords_EmptyRecords 测试空记录处理
func TestOrganizeRecords_EmptyRecords(t *testing.T) {
	organizer := NewArchiveOrganizer(nil)
	ctx := context.Background()

	_, err := organizer.OrganizeRecords(ctx, "user123", []HealthRecord{})
	if err == nil {
		t.Error("期望返回错误，但未返回")
	}

	if !strings.Contains(err.Error(), "没有可整理的健康记录") {
		t.Errorf("错误信息不正确: %v", err)
	}
}

// TestCalculateTrend 测试趋势计算
func TestCalculateTrend(t *testing.T) {
	organizer := NewArchiveOrganizer(nil)

	tests := []struct {
		name       string
		dataPoints []DataPoint
		expected   string
	}{
		{
			name: "上升趋势",
			dataPoints: []DataPoint{
				{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Value: 100},
				{Date: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC), Value: 120},
			},
			expected: "上升",
		},
		{
			name: "下降趋势",
			dataPoints: []DataPoint{
				{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Value: 140},
				{Date: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC), Value: 130},
			},
			expected: "下降",
		},
		{
			name: "稳定趋势",
			dataPoints: []DataPoint{
				{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Value: 120},
				{Date: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC), Value: 122},
			},
			expected: "稳定",
		},
		{
			name: "数据不足",
			dataPoints: []DataPoint{
				{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Value: 100},
			},
			expected: "数据不足",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := organizer.calculateTrend(tt.dataPoints)
			if result != tt.expected {
				t.Errorf("期望趋势为%s，实际得到%s", tt.expected, result)
			}
		})
	}
}

// TestFormatArchiveAsText 测试文本格式化
func TestFormatArchiveAsText(t *testing.T) {
	organizer := NewArchiveOrganizer(nil)

	archive := &OrganizedArchive{
		UserID:      "user123",
		GeneratedAt: time.Date(2024, 5, 13, 10, 0, 0, 0, time.UTC),
		RecordsByCategory: map[HealthRecordCategory][]HealthRecord{
			CategoryPhysicalExam: {
				{
					ID:       "1",
					Category: CategoryPhysicalExam,
					Date:     time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
					Title:    "年度体检",
					Content:  "血压正常",
				},
			},
		},
		Timeline: []TimelineEvent{
			{
				Date:        time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
				Category:    CategoryPhysicalExam,
				Title:       "年度体检",
				Description: "血压正常",
				IsImportant: true,
			},
		},
		TrendAnalysis: []TrendAnalysis{
			{
				Indicator:   "血压",
				Trend:       "稳定",
				Description: "血压在观察期内保持稳定",
			},
		},
		Summary:    "健康档案摘要\n\n记录统计：\n- 体检记录：1条\n",
		Disclaimer: "本档案仅供参考，不构成医学诊断或治疗建议。",
	}

	text := organizer.FormatArchiveAsText(archive)

	// 验证包含关键信息
	if !strings.Contains(text, "健康档案") {
		t.Error("格式化文本缺少标题")
	}

	if !strings.Contains(text, "2024-05-13") {
		t.Error("格式化文本缺少生成时间")
	}

	if !strings.Contains(text, "体检记录") {
		t.Error("格式化文本缺少类别名称")
	}

	if !strings.Contains(text, "年度体检") {
		t.Error("格式化文本缺少记录标题")
	}

	if !strings.Contains(text, "健康趋势分析") {
		t.Error("格式化文本缺少趋势分析")
	}

	if !strings.Contains(text, "免责声明") {
		t.Error("格式化文本缺少免责声明")
	}

	if !strings.Contains(text, "仅供参考") {
		t.Error("格式化文本缺少免责声明内容")
	}
}

// TestGetDefaultUnit 测试获取默认单位
func TestGetDefaultUnit(t *testing.T) {
	organizer := NewArchiveOrganizer(nil)

	tests := []struct {
		indicator string
		expected  string
	}{
		{"血压", "mmHg"},
		{"收缩压", "mmHg"},
		{"舒张压", "mmHg"},
		{"血糖", "mmol/L"},
		{"体重", "kg"},
		{"身高", "cm"},
		{"体温", "℃"},
		{"心率", "次/分"},
		{"血氧", "%"},
		{"未知指标", ""},
	}

	for _, tt := range tests {
		t.Run(tt.indicator, func(t *testing.T) {
			result := organizer.getDefaultUnit(tt.indicator)
			if result != tt.expected {
				t.Errorf("指标%s期望单位为%s，实际得到%s", tt.indicator, tt.expected, result)
			}
		})
	}
}

// TestExtractBriefDescription 测试提取简要描述
func TestExtractBriefDescription(t *testing.T) {
	organizer := NewArchiveOrganizer(nil)

	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "短内容",
			content:  "血压正常",
			expected: "血压正常",
		},
		{
			name:     "长内容",
			content:  strings.Repeat("这是一段很长的内容。", 20),
			expected: strings.Repeat("这是一段很长的内容。", 20)[:100] + "...",
		},
		{
			name:     "包含换行符",
			content:  "第一行\n第二行\r\n第三行",
			expected: "第一行 第二行 第三行",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := organizer.extractBriefDescription(tt.content)
			if result != tt.expected {
				t.Errorf("期望描述为%s，实际得到%s", tt.expected, result)
			}
		})
	}
}

// BenchmarkOrganizeRecords 性能测试
func BenchmarkOrganizeRecords(b *testing.B) {
	organizer := NewArchiveOrganizer(nil)
	ctx := context.Background()

	// 准备测试数据
	records := make([]HealthRecord, 100)
	for i := 0; i < 100; i++ {
		records[i] = HealthRecord{
			ID:       fmt.Sprintf("record-%d", i),
			UserID:   "user123",
			Category: CategoryPhysicalExam,
			Date:     time.Now().AddDate(0, 0, -i),
			Title:    fmt.Sprintf("记录%d", i),
			Content:  "这是一条测试记录",
			Metadata: map[string]interface{}{
				"indicators": map[string]interface{}{
					"血压": 120.0 + float64(i%20),
				},
			},
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := organizer.OrganizeRecords(ctx, "user123", records)
		if err != nil {
			b.Fatalf("整理记录失败: %v", err)
		}
	}
}
