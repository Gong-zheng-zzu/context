package health

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/contextkeeper/service/internal/security"
)

// MedicalReportGenerator 就医报告生成器
// 用于生成格式化的就医报告，供用户携带就医时向医生展示
type MedicalReportGenerator struct {
	detector  *security.Detector // 敏感信息检测器
	organizer *ArchiveOrganizer  // 档案整理器
}

// NewMedicalReportGenerator 创建就医报告生成器
// 如果 detector 或 organizer 为 nil，则创建默认实例
func NewMedicalReportGenerator(detector *security.Detector, organizer *ArchiveOrganizer) *MedicalReportGenerator {
	if detector == nil {
		detector = security.NewDetector()
	}
	if organizer == nil {
		organizer = NewArchiveOrganizer(detector)
	}
	return &MedicalReportGenerator{
		detector:  detector,
		organizer: organizer,
	}
}

// MedicalReport 就医报告结构
type MedicalReport struct {
	Title            string                 // 报告标题
	GeneratedAt      time.Time              // 生成时间
	UserID           string                 // 用户ID（已脱敏）
	TimeRange        string                 // 时间范围描述
	MainIssues       []MainIssue            // 主要健康问题
	Allergies        []string               // 过敏史
	FamilyHistory    []string               // 家族史
	Timeline         []TimelineEvent        // 就诊记录时间线
	VitalSigns       map[string]string      // 生命体征（最近一次）
	Medications      []MedicationRecord     // 当前用药
	LabResults       []LabResult            // 最近化验结果
	Disclaimer       string                 // 免责声明
}

// MainIssue 主要健康问题
type MainIssue struct {
	Category    string    // 问题类别（如"慢性病"、"急性症状"）
	Description string    // 问题描述
	FirstDate   time.Time // 首次记录时间
	LastDate    time.Time // 最近记录时间
	Frequency   int       // 记录次数
}

// MedicationRecord 用药记录
type MedicationRecord struct {
	Name      string    // 药品名称
	Dosage    string    // 剂量
	Frequency string    // 频率
	StartDate time.Time // 开始时间
	Purpose   string    // 用途
}

// LabResult 化验结果
type LabResult struct {
	Date      time.Time // 化验日期
	TestName  string    // 检查项目
	Result    string    // 结果（已脱敏）
	Reference string    // 参考范围
}

// GenerateReport 生成就医报告
// 输入：
//   - userID: 用户ID
//   - records: 健康记录列表
//   - daysRange: 时间范围（天数），0表示所有记录
// 输出：
//   - 格式化的就医报告
func (mrg *MedicalReportGenerator) GenerateReport(userID string, records []HealthRecord, daysRange int) (*MedicalReport, error) {
	if len(records) == 0 {
		return nil, fmt.Errorf("没有可用的健康记录")
	}

	// 1. 过滤时间范围内的记录
	filteredRecords := mrg.filterRecordsByTimeRange(records, daysRange)
	if len(filteredRecords) == 0 {
		return nil, fmt.Errorf("指定时间范围内没有健康记录")
	}

	// 2. 脱敏处理用户ID
	sanitizedUserID := mrg.sanitizeUserID(userID)

	// 3. 提取主要健康问题
	mainIssues := mrg.extractMainIssues(filteredRecords)

	// 4. 提取过敏史
	allergies := mrg.extractAllergies(filteredRecords)

	// 5. 提取家族史
	familyHistory := mrg.extractFamilyHistory(filteredRecords)

	// 6. 生成时间线
	timeline := mrg.formatTimeline(filteredRecords)

	// 7. 提取生命体征（最近一次）
	vitalSigns := mrg.extractLatestVitalSigns(filteredRecords)

	// 8. 提取当前用药
	medications := mrg.extractCurrentMedications(filteredRecords)

	// 9. 提取最近化验结果
	labResults := mrg.extractRecentLabResults(filteredRecords)

	// 10. 构建报告
	report := &MedicalReport{
		Title:         "就医报告",
		GeneratedAt:   time.Now(),
		UserID:        sanitizedUserID,
		TimeRange:     mrg.formatTimeRange(daysRange),
		MainIssues:    mainIssues,
		Allergies:     allergies,
		FamilyHistory: familyHistory,
		Timeline:      timeline,
		VitalSigns:    vitalSigns,
		Medications:   medications,
		LabResults:    labResults,
		Disclaimer:    "本报告由用户自行记录，仅供医生参考，不构成诊断依据。所有敏感信息已脱敏处理。",
	}

	return report, nil
}

// FormatAsText 将报告格式化为易读的文本格式
// 使用分隔线和缩进，适合终端显示或打印
func (mrg *MedicalReportGenerator) FormatAsText(report *MedicalReport) string {
	var output strings.Builder

	// 标题和生成时间
	output.WriteString(strings.Repeat("=", 60) + "\n")
	output.WriteString(centerText(report.Title, 60) + "\n")
	output.WriteString(strings.Repeat("=", 60) + "\n\n")

	output.WriteString(fmt.Sprintf("生成时间：%s\n", report.GeneratedAt.Format("2006-01-02 15:04:05")))
	output.WriteString(fmt.Sprintf("时间范围：%s\n", report.TimeRange))
	output.WriteString(fmt.Sprintf("用户ID：%s\n\n", report.UserID))

	// 主要健康问题
	if len(report.MainIssues) > 0 {
		output.WriteString(strings.Repeat("-", 60) + "\n")
		output.WriteString("主要健康问题\n")
		output.WriteString(strings.Repeat("-", 60) + "\n")
		for i, issue := range report.MainIssues {
			output.WriteString(fmt.Sprintf("\n%d. [%s] %s\n", i+1, issue.Category, issue.Description))
			output.WriteString(fmt.Sprintf("   首次记录：%s\n", issue.FirstDate.Format("2006-01-02")))
			output.WriteString(fmt.Sprintf("   最近记录：%s\n", issue.LastDate.Format("2006-01-02")))
			output.WriteString(fmt.Sprintf("   记录次数：%d次\n", issue.Frequency))
		}
		output.WriteString("\n")
	}

	// 过敏史
	if len(report.Allergies) > 0 {
		output.WriteString(strings.Repeat("-", 60) + "\n")
		output.WriteString("过敏史\n")
		output.WriteString(strings.Repeat("-", 60) + "\n")
		for _, allergy := range report.Allergies {
			output.WriteString(fmt.Sprintf("  • %s\n", allergy))
		}
		output.WriteString("\n")
	}

	// 家族史
	if len(report.FamilyHistory) > 0 {
		output.WriteString(strings.Repeat("-", 60) + "\n")
		output.WriteString("家族史\n")
		output.WriteString(strings.Repeat("-", 60) + "\n")
		for _, history := range report.FamilyHistory {
			output.WriteString(fmt.Sprintf("  • %s\n", history))
		}
		output.WriteString("\n")
	}

	// 生命体征
	if len(report.VitalSigns) > 0 {
		output.WriteString(strings.Repeat("-", 60) + "\n")
		output.WriteString("最近生命体征\n")
		output.WriteString(strings.Repeat("-", 60) + "\n")
		for name, value := range report.VitalSigns {
			output.WriteString(fmt.Sprintf("  %s: %s\n", name, value))
		}
		output.WriteString("\n")
	}

	// 当前用药
	if len(report.Medications) > 0 {
		output.WriteString(strings.Repeat("-", 60) + "\n")
		output.WriteString("当前用药\n")
		output.WriteString(strings.Repeat("-", 60) + "\n")
		for i, med := range report.Medications {
			output.WriteString(fmt.Sprintf("\n%d. %s\n", i+1, med.Name))
			output.WriteString(fmt.Sprintf("   剂量：%s\n", med.Dosage))
			output.WriteString(fmt.Sprintf("   频率：%s\n", med.Frequency))
			output.WriteString(fmt.Sprintf("   开始时间：%s\n", med.StartDate.Format("2006-01-02")))
			if med.Purpose != "" {
				output.WriteString(fmt.Sprintf("   用途：%s\n", med.Purpose))
			}
		}
		output.WriteString("\n")
	}

	// 最近化验结果
	if len(report.LabResults) > 0 {
		output.WriteString(strings.Repeat("-", 60) + "\n")
		output.WriteString("最近化验结果\n")
		output.WriteString(strings.Repeat("-", 60) + "\n")
		for _, lab := range report.LabResults {
			output.WriteString(fmt.Sprintf("\n[%s] %s\n", lab.Date.Format("2006-01-02"), lab.TestName))
			output.WriteString(fmt.Sprintf("  结果：%s\n", lab.Result))
			if lab.Reference != "" {
				output.WriteString(fmt.Sprintf("  参考范围：%s\n", lab.Reference))
			}
		}
		output.WriteString("\n")
	}

	// 就诊记录时间线
	if len(report.Timeline) > 0 {
		output.WriteString(strings.Repeat("-", 60) + "\n")
		output.WriteString("就诊记录时间线\n")
		output.WriteString(strings.Repeat("-", 60) + "\n")
		for _, event := range report.Timeline {
			marker := "○"
			if event.IsImportant {
				marker = "●"
			}
			output.WriteString(fmt.Sprintf("\n%s [%s] %s\n",
				marker,
				event.Date.Format("2006-01-02"),
				event.Title,
			))
			if event.Description != "" {
				output.WriteString(fmt.Sprintf("  %s\n", event.Description))
			}
		}
		output.WriteString("\n")
	}

	// 免责声明
	output.WriteString(strings.Repeat("=", 60) + "\n")
	output.WriteString("免责声明\n")
	output.WriteString(strings.Repeat("=", 60) + "\n")
	output.WriteString(wrapText(report.Disclaimer, 60) + "\n")

	return output.String()
}

// filterRecordsByTimeRange 过滤指定时间范围内的记录
func (mrg *MedicalReportGenerator) filterRecordsByTimeRange(records []HealthRecord, daysRange int) []HealthRecord {
	if daysRange <= 0 {
		return records // 返回所有记录
	}

	cutoffDate := time.Now().AddDate(0, 0, -daysRange)
	var filtered []HealthRecord

	for _, record := range records {
		if record.Date.After(cutoffDate) || record.Date.Equal(cutoffDate) {
			filtered = append(filtered, record)
		}
	}

	return filtered
}

// sanitizeUserID 脱敏用户ID
func (mrg *MedicalReportGenerator) sanitizeUserID(userID string) string {
	if len(userID) <= 8 {
		return "****"
	}
	return userID[:4] + "****" + userID[len(userID)-4:]
}

// extractMainIssues 提取主要健康问题
func (mrg *MedicalReportGenerator) extractMainIssues(records []HealthRecord) []MainIssue {
	// 按问题描述分组统计
	issueMap := make(map[string]*MainIssue)

	for _, record := range records {
		// 从标题或内容中提取问题关键词
		key := mrg.extractIssueKey(record)
		if key == "" {
			continue
		}

		if issue, exists := issueMap[key]; exists {
			// 更新现有问题
			issue.Frequency++
			if record.Date.Before(issue.FirstDate) {
				issue.FirstDate = record.Date
			}
			if record.Date.After(issue.LastDate) {
				issue.LastDate = record.Date
			}
		} else {
			// 创建新问题
			issueMap[key] = &MainIssue{
				Category:    mrg.categorizeIssue(record),
				Description: key,
				FirstDate:   record.Date,
				LastDate:    record.Date,
				Frequency:   1,
			}
		}
	}

	// 转换为切片并排序（按频率降序）
	var issues []MainIssue
	for _, issue := range issueMap {
		issues = append(issues, *issue)
	}

	sort.Slice(issues, func(i, j int) bool {
		return issues[i].Frequency > issues[j].Frequency
	})

	// 只返回前5个主要问题
	if len(issues) > 5 {
		issues = issues[:5]
	}

	return issues
}

// extractIssueKey 从记录中提取问题关键词
func (mrg *MedicalReportGenerator) extractIssueKey(record HealthRecord) string {
	// 优先使用标题
	if record.Title != "" {
		// 脱敏处理
		sanitized, _ := mrg.detector.DetectAndRedact(record.Title)
		return sanitized
	}

	// 否则从内容中提取前50个字符
	content := strings.TrimSpace(record.Content)
	if len(content) > 50 {
		content = content[:50] + "..."
	}

	// 脱敏处理
	sanitized, _ := mrg.detector.DetectAndRedact(content)
	return sanitized
}

// categorizeIssue 对问题进行分类
func (mrg *MedicalReportGenerator) categorizeIssue(record HealthRecord) string {
	// 根据记录类别和内容判断问题类别
	switch record.Category {
	case CategoryMedicalVisit:
		if record.IsImportant {
			return "重要就诊"
		}
		return "就诊记录"
	case CategorySymptom:
		return "症状"
	case CategoryMedication:
		return "用药"
	case CategoryLabTest:
		return "检查"
	case CategoryPhysicalExam:
		return "体检"
	default:
		return "其他"
	}
}

// extractAllergies 提取过敏史
func (mrg *MedicalReportGenerator) extractAllergies(records []HealthRecord) []string {
	allergySet := make(map[string]bool)

	for _, record := range records {
		// 从metadata中提取
		if record.Metadata != nil {
			if allergies, ok := record.Metadata["allergies"].([]interface{}); ok {
				for _, allergy := range allergies {
					if allergyStr, ok := allergy.(string); ok {
						// 脱敏处理
						sanitized, _ := mrg.detector.DetectAndRedact(allergyStr)
						allergySet[sanitized] = true
					}
				}
			}
		}

		// 从内容中提取（简单的关键词匹配）
		content := strings.ToLower(record.Content)
		if strings.Contains(content, "过敏") || strings.Contains(content, "allergy") {
			// 提取过敏相关的句子
			lines := strings.Split(record.Content, "\n")
			for _, line := range lines {
				lowerLine := strings.ToLower(line)
				if strings.Contains(lowerLine, "过敏") || strings.Contains(lowerLine, "allergy") {
					// 脱敏处理
					sanitized, _ := mrg.detector.DetectAndRedact(strings.TrimSpace(line))
					if sanitized != "" {
						allergySet[sanitized] = true
					}
				}
			}
		}
	}

	// 转换为切片
	var allergies []string
	for allergy := range allergySet {
		allergies = append(allergies, allergy)
	}

	sort.Strings(allergies)
	return allergies
}

// extractFamilyHistory 提取家族史
func (mrg *MedicalReportGenerator) extractFamilyHistory(records []HealthRecord) []string {
	historySet := make(map[string]bool)

	for _, record := range records {
		// 从metadata中提取
		if record.Metadata != nil {
			if history, ok := record.Metadata["family_history"].([]interface{}); ok {
				for _, item := range history {
					if historyStr, ok := item.(string); ok {
						// 脱敏处理
						sanitized, _ := mrg.detector.DetectAndRedact(historyStr)
						historySet[sanitized] = true
					}
				}
			}
		}

		// 从内容中提取（简单的关键词匹配）
		content := strings.ToLower(record.Content)
		if strings.Contains(content, "家族史") || strings.Contains(content, "family history") {
			lines := strings.Split(record.Content, "\n")
			for _, line := range lines {
				lowerLine := strings.ToLower(line)
				if strings.Contains(lowerLine, "家族史") || strings.Contains(lowerLine, "family history") {
					// 脱敏处理
					sanitized, _ := mrg.detector.DetectAndRedact(strings.TrimSpace(line))
					if sanitized != "" {
						historySet[sanitized] = true
					}
				}
			}
		}
	}

	// 转换为切片
	var history []string
	for item := range historySet {
		history = append(history, item)
	}

	sort.Strings(history)
	return history
}

// formatTimeline 格式化时间线
func (mrg *MedicalReportGenerator) formatTimeline(records []HealthRecord) []TimelineEvent {
	var timeline []TimelineEvent

	for _, record := range records {
		// 脱敏处理
		sanitizedTitle, _ := mrg.detector.DetectAndRedact(record.Title)
		sanitizedContent, _ := mrg.detector.DetectAndRedact(record.Content)

		// 提取简要描述
		description := sanitizedContent
		if len(description) > 100 {
			description = description[:100] + "..."
		}

		event := TimelineEvent{
			Date:        record.Date,
			Category:    record.Category,
			Title:       sanitizedTitle,
			Description: description,
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

// extractLatestVitalSigns 提取最近的生命体征
func (mrg *MedicalReportGenerator) extractLatestVitalSigns(records []HealthRecord) map[string]string {
	vitalSigns := make(map[string]string)

	// 按时间排序（最新的在前）
	sortedRecords := make([]HealthRecord, len(records))
	copy(sortedRecords, records)
	sort.Slice(sortedRecords, func(i, j int) bool {
		return sortedRecords[i].Date.After(sortedRecords[j].Date)
	})

	// 从最新的记录中提取生命体征
	for _, record := range sortedRecords {
		if record.Metadata != nil {
			if vitals, ok := record.Metadata["vital_signs"].(map[string]interface{}); ok {
				for name, value := range vitals {
					if _, exists := vitalSigns[name]; !exists {
						// 脱敏处理
						valueStr := fmt.Sprintf("%v", value)
						sanitized, _ := mrg.detector.DetectAndRedact(valueStr)
						vitalSigns[name] = sanitized
					}
				}
			}
		}

		// 如果已经收集到足够的生命体征，停止
		if len(vitalSigns) >= 5 {
			break
		}
	}

	return vitalSigns
}

// extractCurrentMedications 提取当前用药
func (mrg *MedicalReportGenerator) extractCurrentMedications(records []HealthRecord) []MedicationRecord {
	var medications []MedicationRecord

	// 只从用药记录中提取
	for _, record := range records {
		if record.Category != CategoryMedication {
			continue
		}

		if record.Metadata != nil {
			if meds, ok := record.Metadata["medications"].([]interface{}); ok {
				for _, med := range meds {
					if medMap, ok := med.(map[string]interface{}); ok {
						medication := MedicationRecord{
							StartDate: record.Date,
						}

						if name, ok := medMap["name"].(string); ok {
							sanitized, _ := mrg.detector.DetectAndRedact(name)
							medication.Name = sanitized
						}
						if dosage, ok := medMap["dosage"].(string); ok {
							medication.Dosage = dosage
						}
						if frequency, ok := medMap["frequency"].(string); ok {
							medication.Frequency = frequency
						}
						if purpose, ok := medMap["purpose"].(string); ok {
							sanitized, _ := mrg.detector.DetectAndRedact(purpose)
							medication.Purpose = sanitized
						}

						if medication.Name != "" {
							medications = append(medications, medication)
						}
					}
				}
			}
		}
	}

	// 按开始时间排序（最新的在前）
	sort.Slice(medications, func(i, j int) bool {
		return medications[i].StartDate.After(medications[j].StartDate)
	})

	return medications
}

// extractRecentLabResults 提取最近的化验结果
func (mrg *MedicalReportGenerator) extractRecentLabResults(records []HealthRecord) []LabResult {
	var labResults []LabResult

	// 只从化验记录中提取
	for _, record := range records {
		if record.Category != CategoryLabTest {
			continue
		}

		if record.Metadata != nil {
			if labs, ok := record.Metadata["lab_results"].([]interface{}); ok {
				for _, lab := range labs {
					if labMap, ok := lab.(map[string]interface{}); ok {
						result := LabResult{
							Date: record.Date,
						}

						if testName, ok := labMap["test_name"].(string); ok {
							result.TestName = testName
						}
						if resultValue, ok := labMap["result"].(string); ok {
							sanitized, _ := mrg.detector.DetectAndRedact(resultValue)
							result.Result = sanitized
						}
						if reference, ok := labMap["reference"].(string); ok {
							result.Reference = reference
						}

						if result.TestName != "" {
							labResults = append(labResults, result)
						}
					}
				}
			}
		}
	}

	// 按日期排序（最新的在前）
	sort.Slice(labResults, func(i, j int) bool {
		return labResults[i].Date.After(labResults[j].Date)
	})

	// 只返回最近5条
	if len(labResults) > 5 {
		labResults = labResults[:5]
	}

	return labResults
}

// formatTimeRange 格式化时间范围描述
func (mrg *MedicalReportGenerator) formatTimeRange(daysRange int) string {
	if daysRange <= 0 {
		return "全部记录"
	}
	if daysRange == 1 {
		return "最近1天"
	}
	if daysRange == 7 {
		return "最近1周"
	}
	if daysRange == 30 {
		return "最近1个月"
	}
	if daysRange == 90 {
		return "最近3个月"
	}
	if daysRange == 180 {
		return "最近6个月"
	}
	if daysRange == 365 {
		return "最近1年"
	}
	return fmt.Sprintf("最近%d天", daysRange)
}

// centerText 居中文本
func centerText(text string, width int) string {
	textLen := len([]rune(text)) // 使用rune计算长度以支持中文
	if textLen >= width {
		return text
	}
	padding := (width - textLen) / 2
	return strings.Repeat(" ", padding) + text
}

// wrapText 文本换行
func wrapText(text string, width int) string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return text
	}

	var lines []string
	var currentLine string

	for _, word := range words {
		if currentLine == "" {
			currentLine = word
		} else if len([]rune(currentLine))+len([]rune(word))+1 <= width {
			currentLine += " " + word
		} else {
			lines = append(lines, currentLine)
			currentLine = word
		}
	}

	if currentLine != "" {
		lines = append(lines, currentLine)
	}

	return strings.Join(lines, "\n")
}
