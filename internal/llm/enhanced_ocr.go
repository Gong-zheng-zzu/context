package llm

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// HealthDataType 健康数据类型
type HealthDataType string

const (
	HealthDataBloodPressure HealthDataType = "blood_pressure" // 血压
	HealthDataBloodSugar    HealthDataType = "blood_sugar"    // 血糖
	HealthDataTemperature   HealthDataType = "temperature"    // 体温
	HealthDataHeartRate     HealthDataType = "heart_rate"     // 心率
	HealthDataWeight        HealthDataType = "weight"         // 体重
	HealthDataOxygen        HealthDataType = "oxygen"         // 血氧
	HealthDataMedication    HealthDataType = "medication"     // 用药
	HealthDataReport        HealthDataType = "report"         // 体检报告
)

// StructuredHealthData 结构化健康数据
type StructuredHealthData struct {
	Type        HealthDataType         `json:"type"`
	RawText     string                 `json:"raw_text"`
	ExtractedAt time.Time              `json:"extracted_at"`
	Data        map[string]interface{} `json:"data"`
	RiskLevel   string                 `json:"risk_level"`   // "正常", "偏高", "偏低", "异常"
	Suggestions []string               `json:"suggestions"`  // 建议
	Confidence  float64                `json:"confidence"`   // 置信度 0-1
}

// EnhancedOCRService 增强的OCR服务
type EnhancedOCRService struct {
	baseClient LLMClient
}

// NewEnhancedOCRService 创建增强OCR服务
func NewEnhancedOCRService(baseClient LLMClient) *EnhancedOCRService {
	return &EnhancedOCRService{
		baseClient: baseClient,
	}
}

// ExtractStructuredData 从图片中提取结构化健康数据
func (s *EnhancedOCRService) ExtractStructuredData(imagePath string) (*StructuredHealthData, error) {
	// 注意：当前版本暂不支持图片分析，返回空数据
	log.Printf("[增强OCR] 图片分析功能暂未实现: %s", imagePath)

	// 返回一个默认的结构化数据
	structuredData := &StructuredHealthData{
		Type:        HealthDataReport,
		RawText:     "图片分析功能暂未实现",
		ExtractedAt: time.Now(),
		Data:        make(map[string]interface{}),
		Confidence:  0.0,
		RiskLevel:   "未知",
		Suggestions: []string{"请手动输入健康数据"},
	}

	return structuredData, nil
}

// detectHealthDataType 检测健康数据类型
func (s *EnhancedOCRService) detectHealthDataType(text string) HealthDataType {
	text = strings.ToLower(text)

	// 血压关键词
	if strings.Contains(text, "血压") || strings.Contains(text, "收缩压") || strings.Contains(text, "舒张压") ||
		regexp.MustCompile(`\d{2,3}/\d{2,3}`).MatchString(text) {
		return HealthDataBloodPressure
	}

	// 血糖关键词
	if strings.Contains(text, "血糖") || strings.Contains(text, "glucose") ||
		strings.Contains(text, "mmol/l") && regexp.MustCompile(`\d+\.\d+`).MatchString(text) {
		return HealthDataBloodSugar
	}

	// 体温关键词
	if strings.Contains(text, "体温") || strings.Contains(text, "temperature") ||
		regexp.MustCompile(`3[6-9]\.\d`).MatchString(text) {
		return HealthDataTemperature
	}

	// 心率关键词
	if strings.Contains(text, "心率") || strings.Contains(text, "heart rate") ||
		strings.Contains(text, "bpm") || strings.Contains(text, "次/分") {
		return HealthDataHeartRate
	}

	// 体重关键词
	if strings.Contains(text, "体重") || strings.Contains(text, "weight") ||
		strings.Contains(text, "kg") || strings.Contains(text, "公斤") {
		return HealthDataWeight
	}

	// 血氧关键词
	if strings.Contains(text, "血氧") || strings.Contains(text, "spo2") ||
		strings.Contains(text, "氧饱和度") {
		return HealthDataOxygen
	}

	// 用药关键词
	if strings.Contains(text, "用药") || strings.Contains(text, "服药") ||
		strings.Contains(text, "药品") || strings.Contains(text, "medication") {
		return HealthDataMedication
	}

	// 体检报告关键词
	if strings.Contains(text, "体检") || strings.Contains(text, "检查报告") ||
		strings.Contains(text, "化验单") {
		return HealthDataReport
	}

	return HealthDataReport // 默认为体检报告
}

// extractBloodPressure 提取血压数据
func (s *EnhancedOCRService) extractBloodPressure(text string, data *StructuredHealthData) {
	// 匹配血压格式：120/80 或 收缩压120 舒张压80
	bpPattern := regexp.MustCompile(`(\d{2,3})\s*/\s*(\d{2,3})`)
	matches := bpPattern.FindStringSubmatch(text)

	if len(matches) >= 3 {
		systolic, _ := strconv.Atoi(matches[1])
		diastolic, _ := strconv.Atoi(matches[2])

		data.Data["systolic"] = systolic   // 收缩压
		data.Data["diastolic"] = diastolic // 舒张压
		data.Data["unit"] = "mmHg"

		log.Printf("[增强OCR] 提取血压: 收缩压=%d, 舒张压=%d", systolic, diastolic)
	} else {
		// 尝试分别匹配
		systolicPattern := regexp.MustCompile(`(?:收缩压|高压)[:：]?\s*(\d{2,3})`)
		diastolicPattern := regexp.MustCompile(`(?:舒张压|低压)[:：]?\s*(\d{2,3})`)

		if m := systolicPattern.FindStringSubmatch(text); len(m) >= 2 {
			systolic, _ := strconv.Atoi(m[1])
			data.Data["systolic"] = systolic
		}

		if m := diastolicPattern.FindStringSubmatch(text); len(m) >= 2 {
			diastolic, _ := strconv.Atoi(m[1])
			data.Data["diastolic"] = diastolic
		}

		data.Data["unit"] = "mmHg"
	}
}

// extractBloodSugar 提取血糖数据
func (s *EnhancedOCRService) extractBloodSugar(text string, data *StructuredHealthData) {
	// 匹配血糖格式：5.6 mmol/L 或 血糖 5.6
	bsPattern := regexp.MustCompile(`(\d+\.\d+)\s*(?:mmol/l|mmol)?`)
	matches := bsPattern.FindStringSubmatch(strings.ToLower(text))

	if len(matches) >= 2 {
		value, _ := strconv.ParseFloat(matches[1], 64)
		data.Data["value"] = value
		data.Data["unit"] = "mmol/L"

		// 判断是空腹还是餐后
		if strings.Contains(text, "空腹") || strings.Contains(text, "fasting") {
			data.Data["type"] = "fasting"
		} else if strings.Contains(text, "餐后") || strings.Contains(text, "postprandial") {
			data.Data["type"] = "postprandial"
		}

		log.Printf("[增强OCR] 提取血糖: %.2f mmol/L", value)
	}
}

// extractTemperature 提取体温数据
func (s *EnhancedOCRService) extractTemperature(text string, data *StructuredHealthData) {
	// 匹配体温格式：36.5°C 或 体温 36.5
	tempPattern := regexp.MustCompile(`(3[6-9]\.\d)`)
	matches := tempPattern.FindStringSubmatch(text)

	if len(matches) >= 2 {
		value, _ := strconv.ParseFloat(matches[1], 64)
		data.Data["value"] = value
		data.Data["unit"] = "°C"

		log.Printf("[增强OCR] 提取体温: %.1f°C", value)
	}
}

// extractHeartRate 提取心率数据
func (s *EnhancedOCRService) extractHeartRate(text string, data *StructuredHealthData) {
	// 匹配心率格式：72 bpm 或 心率 72
	hrPattern := regexp.MustCompile(`(\d{2,3})\s*(?:bpm|次/分)?`)
	matches := hrPattern.FindStringSubmatch(text)

	if len(matches) >= 2 {
		value, _ := strconv.Atoi(matches[1])
		if value >= 40 && value <= 200 { // 合理范围
			data.Data["value"] = value
			data.Data["unit"] = "bpm"

			log.Printf("[增强OCR] 提取心率: %d bpm", value)
		}
	}
}

// extractWeight 提取体重数据
func (s *EnhancedOCRService) extractWeight(text string, data *StructuredHealthData) {
	// 匹配体重格式：65.5 kg
	weightPattern := regexp.MustCompile(`(\d{2,3}(?:\.\d+)?)\s*(?:kg|公斤|千克)`)
	matches := weightPattern.FindStringSubmatch(strings.ToLower(text))

	if len(matches) >= 2 {
		value, _ := strconv.ParseFloat(matches[1], 64)
		data.Data["value"] = value
		data.Data["unit"] = "kg"

		log.Printf("[增强OCR] 提取体重: %.1f kg", value)
	}
}

// extractOxygen 提取血氧数据
func (s *EnhancedOCRService) extractOxygen(text string, data *StructuredHealthData) {
	// 匹配血氧格式：98%
	oxygenPattern := regexp.MustCompile(`(\d{2,3})\s*%`)
	matches := oxygenPattern.FindStringSubmatch(text)

	if len(matches) >= 2 {
		value, _ := strconv.Atoi(matches[1])
		if value >= 70 && value <= 100 { // 合理范围
			data.Data["value"] = value
			data.Data["unit"] = "%"

			log.Printf("[增强OCR] 提取血氧: %d%%", value)
		}
	}
}

// extractMedication 提取用药信息
func (s *EnhancedOCRService) extractMedication(text string, data *StructuredHealthData) {
	// 简单提取药品名称和剂量
	data.Data["raw_medication"] = text

	// 可以进一步使用NLP提取药品名称、剂量、频次等
	log.Printf("[增强OCR] 提取用药信息: %s", text)
}

// extractReport 提取体检报告
func (s *EnhancedOCRService) extractReport(text string, data *StructuredHealthData) {
	// 提取报告的关键信息
	data.Data["raw_report"] = text

	log.Printf("[增强OCR] 提取体检报告: %s", text)
}

// assessRiskLevel 评估风险等级
func (s *EnhancedOCRService) assessRiskLevel(data *StructuredHealthData) {
	switch data.Type {
	case HealthDataBloodPressure:
		systolic, ok1 := data.Data["systolic"].(int)
		diastolic, ok2 := data.Data["diastolic"].(int)

		if ok1 && ok2 {
			if systolic >= 140 || diastolic >= 90 {
				data.RiskLevel = "偏高"
			} else if systolic < 90 || diastolic < 60 {
				data.RiskLevel = "偏低"
			} else {
				data.RiskLevel = "正常"
			}
		}

	case HealthDataBloodSugar:
		value, ok := data.Data["value"].(float64)
		bsType, _ := data.Data["type"].(string)

		if ok {
			if bsType == "fasting" {
				// 空腹血糖
				if value >= 7.0 {
					data.RiskLevel = "偏高"
				} else if value < 3.9 {
					data.RiskLevel = "偏低"
				} else {
					data.RiskLevel = "正常"
				}
			} else {
				// 餐后血糖
				if value >= 11.1 {
					data.RiskLevel = "偏高"
				} else if value < 3.9 {
					data.RiskLevel = "偏低"
				} else {
					data.RiskLevel = "正常"
				}
			}
		}

	case HealthDataTemperature:
		value, ok := data.Data["value"].(float64)
		if ok {
			if value >= 37.3 {
				data.RiskLevel = "偏高"
			} else if value < 36.0 {
				data.RiskLevel = "偏低"
			} else {
				data.RiskLevel = "正常"
			}
		}

	case HealthDataHeartRate:
		value, ok := data.Data["value"].(int)
		if ok {
			if value > 100 {
				data.RiskLevel = "偏高"
			} else if value < 60 {
				data.RiskLevel = "偏低"
			} else {
				data.RiskLevel = "正常"
			}
		}

	case HealthDataOxygen:
		value, ok := data.Data["value"].(int)
		if ok {
			if value < 95 {
				data.RiskLevel = "偏低"
			} else {
				data.RiskLevel = "正常"
			}
		}

	default:
		data.RiskLevel = "未知"
	}

	log.Printf("[增强OCR] 风险评估: %s", data.RiskLevel)
}

// generateSuggestions 生成建议
func (s *EnhancedOCRService) generateSuggestions(data *StructuredHealthData) {
	suggestions := []string{}

	switch data.Type {
	case HealthDataBloodPressure:
		if data.RiskLevel == "偏高" {
			suggestions = append(suggestions,
				"建议减少盐分摄入",
				"增加有氧运动",
				"保持充足睡眠",
				"定期监测血压",
			)
		} else if data.RiskLevel == "偏低" {
			suggestions = append(suggestions,
				"注意补充水分",
				"避免突然站立",
				"适当增加盐分摄入",
			)
		}

	case HealthDataBloodSugar:
		if data.RiskLevel == "偏高" {
			suggestions = append(suggestions,
				"控制碳水化合物摄入",
				"增加运动量",
				"定期监测血糖",
				"必要时咨询医生调整用药",
			)
		} else if data.RiskLevel == "偏低" {
			suggestions = append(suggestions,
				"及时补充糖分",
				"随身携带糖果",
				"避免空腹运动",
			)
		}

	case HealthDataTemperature:
		if data.RiskLevel == "偏高" {
			suggestions = append(suggestions,
				"多喝水",
				"注意休息",
				"如持续发热请就医",
			)
		}

	case HealthDataHeartRate:
		if data.RiskLevel == "偏高" {
			suggestions = append(suggestions,
				"避免剧烈运动",
				"保持情绪稳定",
				"如有不适请就医",
			)
		}

	case HealthDataOxygen:
		if data.RiskLevel == "偏低" {
			suggestions = append(suggestions,
				"保持室内通风",
				"深呼吸练习",
				"如持续偏低请就医",
			)
		}
	}

	data.Suggestions = suggestions
	log.Printf("[增强OCR] 生成建议: %v", suggestions)
}

// FormatForDisplay 格式化为易读的显示文本
func (data *StructuredHealthData) FormatForDisplay() string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("📊 **数据类型**: %s\n\n", data.getTypeName()))

	// 显示提取的数据
	builder.WriteString("📈 **检测结果**:\n")
	switch data.Type {
	case HealthDataBloodPressure:
		systolic, _ := data.Data["systolic"].(int)
		diastolic, _ := data.Data["diastolic"].(int)
		builder.WriteString(fmt.Sprintf("- 收缩压: %d mmHg\n", systolic))
		builder.WriteString(fmt.Sprintf("- 舒张压: %d mmHg\n", diastolic))

	case HealthDataBloodSugar:
		value, _ := data.Data["value"].(float64)
		bsType, _ := data.Data["type"].(string)
		typeStr := "未知"
		if bsType == "fasting" {
			typeStr = "空腹"
		} else if bsType == "postprandial" {
			typeStr = "餐后"
		}
		builder.WriteString(fmt.Sprintf("- 血糖(%s): %.2f mmol/L\n", typeStr, value))

	case HealthDataTemperature:
		value, _ := data.Data["value"].(float64)
		builder.WriteString(fmt.Sprintf("- 体温: %.1f°C\n", value))

	case HealthDataHeartRate:
		value, _ := data.Data["value"].(int)
		builder.WriteString(fmt.Sprintf("- 心率: %d bpm\n", value))

	case HealthDataWeight:
		value, _ := data.Data["value"].(float64)
		builder.WriteString(fmt.Sprintf("- 体重: %.1f kg\n", value))

	case HealthDataOxygen:
		value, _ := data.Data["value"].(int)
		builder.WriteString(fmt.Sprintf("- 血氧: %d%%\n", value))
	}

	// 显示风险等级
	riskEmoji := "✅"
	if data.RiskLevel == "偏高" || data.RiskLevel == "偏低" {
		riskEmoji = "⚠️"
	} else if data.RiskLevel == "异常" {
		riskEmoji = "🚨"
	}
	builder.WriteString(fmt.Sprintf("\n%s **风险等级**: %s\n", riskEmoji, data.RiskLevel))

	// 显示建议
	if len(data.Suggestions) > 0 {
		builder.WriteString("\n💡 **健康建议**:\n")
		for _, suggestion := range data.Suggestions {
			builder.WriteString(fmt.Sprintf("- %s\n", suggestion))
		}
	}

	return builder.String()
}

func (data *StructuredHealthData) getTypeName() string {
	names := map[HealthDataType]string{
		HealthDataBloodPressure: "血压",
		HealthDataBloodSugar:    "血糖",
		HealthDataTemperature:   "体温",
		HealthDataHeartRate:     "心率",
		HealthDataWeight:        "体重",
		HealthDataOxygen:        "血氧",
		HealthDataMedication:    "用药记录",
		HealthDataReport:        "体检报告",
	}
	return names[data.Type]
}
