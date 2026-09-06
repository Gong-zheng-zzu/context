package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/contextkeeper/service/internal/bigdata"
)

// VitalSignsTool 生命体征查询工具
type VitalSignsTool struct{}

func (t *VitalSignsTool) Name() string        { return "vital_signs" }
func (t *VitalSignsTool) Description() string  { return "查询居民的生命体征数据，包括血压、心率、体温、血氧等。输入格式：居民ID 或 最新（查询最新数据）" }

func (t *VitalSignsTool) Execute(ctx context.Context, input string) (string, error) {
	deps := GetDeps(ctx)
	if deps == nil {
		return "依赖服务不可用", nil
	}

	// 判断是否有InfluxDB服务
	if deps.VitalService == nil {
		return generateDemoVitalSigns(input), nil
	}

	residentID := strings.TrimSpace(input)
	if residentID == "" {
		residentID = deps.UserID
	}

	// 查询最新数据
	latest, err := deps.VitalService.QueryLatestVitalSigns(ctx, residentID)
	if err != nil {
		return generateDemoVitalSigns(residentID), nil
	}

	return formatVitalSigns(latest), nil
}

func formatVitalSigns(data map[bigdata.VitalSignType]map[string]interface{}) string {
	var sb strings.Builder
	sb.WriteString("生命体征数据：\n")
	for signType, values := range data {
		sb.WriteString(fmt.Sprintf("- %s: %v\n", signType, values))
	}
	return sb.String()
}

// generateDemoVitalSigns 当没有数据库时生成演示数据
func generateDemoVitalSigns(residentID string) string {
	return fmt.Sprintf(`生命体征数据（模拟数据）：
- 居民ID: %s
- 血压: 135/85 mmHg（偏高，建议关注）
- 心率: 78 次/分（正常）
- 体温: 36.5°C（正常）
- 血氧: 97%%（正常）
- 测量时间: %s
- 状态提示: 血压略高于正常范围，建议继续监测`,
		residentID, time.Now().Format("2006-01-02 15:04"))
}
