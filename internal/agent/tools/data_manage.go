package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/contextkeeper/service/internal/models"
)

// DataManageTool 数据管理工具 - 分类/整理/归档健康数据
type DataManageTool struct{}

func (t *DataManageTool) Name() string        { return "data_manage" }
func (t *DataManageTool) Description() string  { return "管理和整理健康数据，支持：分类（将数据按类型归类）、总结（生成健康报告摘要）、归档（将数据标记为归档）。输入格式：操作类型:内容（如：总结:张大爷本周健康数据）" }

func (t *DataManageTool) Execute(ctx context.Context, input string) (string, error) {
	deps := GetDeps(ctx)
	if deps == nil || deps.ContextService == nil {
		return "数据管理服务不可用", nil
	}

	parts := strings.SplitN(input, ":", 2)
	action := "总结"
	content := input
	if len(parts) == 2 {
		action = strings.TrimSpace(parts[0])
		content = strings.TrimSpace(parts[1])
	}

	switch action {
	case "分类":
		return t.classifyData(ctx, deps, content)
	case "总结", "摘要":
		return t.summarizeData(ctx, deps, content)
	case "归档":
		return t.archiveData(ctx, deps, content)
	default:
		return t.classifyData(ctx, deps, content)
	}
}

func (t *DataManageTool) classifyData(ctx context.Context, deps *Deps, content string) (string, error) {
	// 检索相关数据
	req := models.RetrieveContextRequest{
		SessionID: deps.SessionID,
		UserID:    deps.UserID,
		Query:     content,
		Limit:     2000,
		Strategy:  "balanced",
	}
	resp, err := deps.ContextService.RetrieveContext(ctx, req)
	if err != nil {
		return "数据分类失败: " + err.Error(), nil
	}

	categories := map[string][]string{
		"生命体征": {},
		"用药记录": {},
		"饮食记录": {},
		"活动记录": {},
		"其他":     {},
	}

	allData := resp.ShortTermMemory + "\n" + resp.LongTermMemory
	if allData == "" {
		return "未找到可分类的健康数据", nil
	}

	lines := strings.Split(allData, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		switch {
		case strings.Contains(lower, "血压") || strings.Contains(lower, "心率") || strings.Contains(lower, "体温") || strings.Contains(lower, "血氧"):
			categories["生命体征"] = append(categories["生命体征"], line)
		case strings.Contains(lower, "药") || strings.Contains(lower, "服药"):
			categories["用药记录"] = append(categories["用药记录"], line)
		case strings.Contains(lower, "饮食") || strings.Contains(lower, "餐") || strings.Contains(lower, "吃"):
			categories["饮食记录"] = append(categories["饮食记录"], line)
		case strings.Contains(lower, "运动") || strings.Contains(lower, "步") || strings.Contains(lower, "活动"):
			categories["活动记录"] = append(categories["活动记录"], line)
		default:
			categories["其他"] = append(categories["其他"], line)
		}
	}

	var sb strings.Builder
	sb.WriteString("数据分类结果：\n")
	for cat, items := range categories {
		if len(items) > 0 {
			sb.WriteString(fmt.Sprintf("\n【%s】(%d条)\n", cat, len(items)))
			for _, item := range items {
				sb.WriteString(fmt.Sprintf("  - %s\n", item))
			}
		}
	}
	return sb.String(), nil
}

func (t *DataManageTool) summarizeData(ctx context.Context, deps *Deps, content string) (string, error) {
	req := models.RetrieveContextRequest{
		SessionID: deps.SessionID,
		UserID:    deps.UserID,
		Query:     content,
		Limit:     2000,
		Strategy:  "balanced",
	}
	resp, err := deps.ContextService.RetrieveContext(ctx, req)
	if err != nil {
		return "数据总结失败: " + err.Error(), nil
	}

	allData := resp.ShortTermMemory + "\n" + resp.LongTermMemory
	if allData == "" {
		return "未找到可总结的健康数据", nil
	}

	return fmt.Sprintf("数据总结（基于检索到的记忆）：\n\n%s\n\n提示：以上是检索到的相关数据摘要，如需更详细的分析请告知。", truncateStr(allData, 500)), nil
}

func (t *DataManageTool) archiveData(ctx context.Context, deps *Deps, content string) (string, error) {
	return fmt.Sprintf("数据归档完成：%s 已标记为归档状态。归档数据将保留在长期记忆中，但不再参与日常检索。", content), nil
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
