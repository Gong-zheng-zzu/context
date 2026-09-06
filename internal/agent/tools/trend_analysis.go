package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/contextkeeper/service/internal/bigdata"
)

// TrendAnalysisTool 健康趋势分析工具
type TrendAnalysisTool struct{}

func (t *TrendAnalysisTool) Name() string        { return "trend_analysis" }
func (t *TrendAnalysisTool) Description() string  { return "分析居民的健康数据趋势，支持按时间范围查询。输入格式：居民ID,数据类型,天数（如：张大爷,血压,7）" }

func (t *TrendAnalysisTool) Execute(ctx context.Context, input string) (string, error) {
	deps := GetDeps(ctx)
	if deps == nil {
		return "依赖服务不可用", nil
	}

	parts := strings.Split(input, ",")
	residentID := ""
	dataType := "血压"
	days := 7

	if len(parts) >= 1 {
		residentID = strings.TrimSpace(parts[0])
	}
	if len(parts) >= 2 {
		dataType = strings.TrimSpace(parts[1])
	}
	if len(parts) >= 3 {
		fmt.Sscanf(strings.TrimSpace(parts[2]), "%d", &days)
	}
	if residentID == "" {
		residentID = deps.UserID
	}

	if deps.VitalService == nil {
		return generateDemoTrend(residentID, dataType, days), nil
	}

	end := time.Now()
	start := end.AddDate(0, 0, -days)

	data, err := deps.VitalService.QueryVitalSignsByTimeRange(ctx, residentID, bigdata.VitalSignType(dataType), start, end)
	if err != nil {
		return generateDemoTrend(residentID, dataType, days), nil
	}

	if len(data) == 0 {
		return fmt.Sprintf("未找到 %s 在过去 %d 天内的 %s 数据", residentID, days, dataType), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s 的 %s 趋势（过去%d天）：\n", residentID, dataType, days))
	for _, d := range data {
		sb.WriteString(fmt.Sprintf("- %v\n", d))
	}
	return sb.String(), nil
}

func generateDemoTrend(residentID, dataType string, days int) string {
	return fmt.Sprintf(`%s 的 %s 趋势分析（过去%d天，模拟数据）：

日期        | 数值       | 状态
-----------|-----------|------
06-01      | 138/88    | 偏高
06-02      | 132/82    | 偏高
06-03      | 128/80    | 正常偏高
06-04      | 135/85    | 偏高

趋势分析:
- 整体趋势: 略有波动，整体偏高
- 建议: 继续监测，注意低盐饮食
- 提醒: 如持续高于140/90请就医`,
		residentID, dataType, days)
}
