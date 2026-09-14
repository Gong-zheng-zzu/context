package agent

import (
	"context"
	"fmt"
	"log"

	"github.com/contextkeeper/service/internal/agent/tools"
	"github.com/contextkeeper/service/internal/models"
	"github.com/contextkeeper/service/internal/services"
)

// SummaryTool 自主总结工具 - 调用LLM对数据进行智能总结
type SummaryTool struct {
	llm LLMCaller
}

func (t *SummaryTool) IsReadOnly() bool { return true }

func (t *SummaryTool) Name() string { return "auto_summary" }
func (t *SummaryTool) Description() string {
	return "对健康数据进行智能总结分析，生成结构化报告。输入：总结主题或数据范围"
}

func (t *SummaryTool) Execute(ctx context.Context, input string) (string, error) {
	deps := tools.GetDeps(ctx)
	if deps == nil || deps.ContextService == nil {
		return "服务不可用", nil
	}

	// 先检索相关数据
	req := models.RetrieveContextRequest{
		SessionID: deps.SessionID,
		UserID:    deps.UserID,
		Query:     input,
		Limit:     2000,
		Strategy:  "balanced",
	}
	resp, err := deps.ContextService.RetrieveContext(ctx, req)
	if err != nil {
		return "数据检索失败: " + err.Error(), nil
	}

	dataContext := resp.ShortTermMemory + "\n" + resp.LongTermMemory + "\n" + resp.RelevantKnowledge
	if dataContext == "" {
		return fmt.Sprintf("未找到与「%s」相关的数据，无法生成总结。", input), nil
	}

	// 调用LLM进行智能总结
	prompt := fmt.Sprintf(`请基于以下健康数据，生成一份简洁的总结报告：

数据内容：
%s

要求：
1. 提取关键健康指标和变化趋势
2. 标注异常值和需要关注的指标
3. 给出简要建议
4. 用结构化格式输出

用户请求：%s`, dataContext, input)

	llmResp, err := t.llm.GenerateResponse(ctx, &services.GenerateRequest{
		Prompt:      prompt,
		MaxTokens:   1500,
		Temperature: 0.3,
	})
	if err != nil {
		log.Printf("❌ [Agent] 总结工具LLM调用失败: %v", err)
		return fmt.Sprintf("数据总结生成失败: %v", err), nil
	}

	return llmResp.Content, nil
}

// NewSummaryTool 创建总结工具
func NewSummaryTool(llm LLMCaller) *SummaryTool {
	return &SummaryTool{llm: llm}
}
