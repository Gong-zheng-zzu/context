package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/contextkeeper/service/internal/agent/tools"
	"github.com/contextkeeper/service/internal/models"
)

// VisualizationTool 数据可视化工具 - 根据用户需求生成真实的图表
type VisualizationTool struct{}

func (t *VisualizationTool) IsReadOnly() bool { return true }

func (t *VisualizationTool) Name() string { return "generate_chart" }
func (t *VisualizationTool) Description() string {
	return "生成数据可视化图表（折线图、柱状图、饼图等）。输入JSON格式：{\"chart_type\":\"line\",\"data_query\":\"查询条件\",\"title\":\"图表标题\"}"
}

// ChartRequest 图表生成请求
type ChartRequest struct {
	ChartType string `json:"chart_type"` // line, bar, pie
	DataQuery string `json:"data_query"` // 数据查询条件
	Title     string `json:"title"`      // 图表标题
}

func (t *VisualizationTool) Execute(ctx context.Context, input string) (string, error) {
	deps := tools.GetDeps(ctx)
	if deps == nil || deps.ContextService == nil {
		return "服务不可用", nil
	}

	// 解析输入
	var req ChartRequest
	if err := json.Unmarshal([]byte(input), &req); err != nil {
		return fmt.Sprintf("输入格式错误，需要JSON格式: %v", err), nil
	}

	// 检索相关数据
	retrieveReq := models.RetrieveContextRequest{
		SessionID: deps.SessionID,
		UserID:    deps.UserID,
		Query:     req.DataQuery,
		Limit:     1000,
		Strategy:  "timeline",
	}
	resp, err := deps.ContextService.RetrieveContext(ctx, retrieveReq)
	if err != nil {
		return "数据检索失败: " + err.Error(), nil
	}

	// 提取数值数据
	dataContext := resp.ShortTermMemory + "\n" + resp.LongTermMemory
	if dataContext == "" {
		dataContext = "未找到相关历史数据"
	}

	// 调用可视化服务API
	chartURL, chartPath, err := t.callVisualizationService(req, dataContext)
	if err != nil {
		return fmt.Sprintf("图表生成失败: %v", err), nil
	}

	return fmt.Sprintf("✅ 图表已生成成功！\n\n📊 图表文件：%s\n🔗 访问链接：%s\n\n您可以通过以上路径查看生成的图表。", chartPath, chartURL), nil
}

// callVisualizationService 调用独立的可视化服务
func (t *VisualizationTool) callVisualizationService(req ChartRequest, dataContext string) (string, string, error) {
	// 构建请求
	requestBody := map[string]string{
		"chart_type":   req.ChartType,
		"title":        req.Title,
		"data_context": dataContext,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", "", err
	}

	// 调用可视化服务API
	apiURL := "http://localhost:5001/generate"
	httpReq, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", "", fmt.Errorf("可视化服务连接失败，请确保visualization_service.py正在运行: %v", err)
	}
	defer resp.Body.Close()

	// 解析响应
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", err
	}

	if !result["success"].(bool) {
		return "", "", fmt.Errorf("图表生成失败: %v", result["error"])
	}

	chartURL := result["chart_url"].(string)
	chartPath := result["chart_path"].(string)

	log.Printf("✅ [VisualizationTool] 图表生成成功: %s", chartPath)

	return chartURL, chartPath, nil
}

// NewVisualizationTool 创建可视化工具
func NewVisualizationTool() *VisualizationTool {
	return &VisualizationTool{}
}
