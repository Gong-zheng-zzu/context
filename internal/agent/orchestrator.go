package agent

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/contextkeeper/service/internal/services"
)

const (
	maxIterations = 5
	timeout       = 60 * time.Second
)

// LLMCaller LLM调用接口（简化版，仅需GenerateResponse）
type LLMCaller interface {
	GenerateResponse(ctx context.Context, req *services.GenerateRequest) (*services.GenerateResponse, error)
}

// Orchestrator ReAct编排器
type Orchestrator struct {
	llm    LLMCaller
	tools  *ToolRegistry
	prompt string
}

// NewOrchestrator 创建编排器
func NewOrchestrator(llm LLMCaller, tools *ToolRegistry, systemPrompt string) *Orchestrator {
	return &Orchestrator{llm: llm, tools: tools, prompt: systemPrompt}
}

// Run 执行Agent推理循环
func (o *Orchestrator) Run(ctx context.Context, userQuery string) *AgentTrace {
	start := time.Now()
	trace := &AgentTrace{}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 初始LLM调用
	currentPrompt := fmt.Sprintf("%s\n\n%s", o.prompt, userQuery)

	for i := 0; i < maxIterations; i++ {
		stepStart := time.Now()

		// 调用LLM
		resp, err := o.llm.GenerateResponse(ctx, &services.GenerateRequest{
			Prompt:      currentPrompt,
			MaxTokens:   2000,
			Temperature: 0.7,
		})
		if err != nil {
			log.Printf("❌ [Agent] LLM调用失败: %v", err)
			trace.Fallback = true
			trace.FinalAnswer = fmt.Sprintf("抱歉，AI推理过程中遇到问题: %v", err)
			trace.TotalTimeMs = time.Since(start).Milliseconds()
			return trace
		}

		// 解析LLM输出
		parsed := ParseOutput(resp.Content)

		step := AgentStep{
			StepNumber: i + 1,
			Thought:    parsed.Thought,
			DurationMs: time.Since(stepStart),
		}

		// 如果有FinalAnswer，结束循环
		if parsed.FinalAnswer != "" {
			trace.Steps = append(trace.Steps, step)
			trace.FinalAnswer = parsed.FinalAnswer
			trace.Iterations = i + 1
			break
		}

		// 如果没有Action也没有FinalAnswer，视为解析失败（fallback）
		if !parsed.HasAction {
			log.Printf("⚠️ [Agent] 第%d轮解析失败，无Action无FinalAnswer，触发fallback", i+1)
			trace.Fallback = true
			trace.FinalAnswer = resp.Content
			trace.Iterations = i + 1
			break
		}

		// 执行工具
		step.Action = parsed.Action
		step.ActionInput = parsed.ActionInput

		log.Printf("🔧 [Agent] 第%d轮: Action=%s, Input=%s", i+1, parsed.Action, truncate(parsed.ActionInput, 100))

		obs, err := o.tools.Execute(ctx, parsed.Action, parsed.ActionInput)
		if err != nil {
			obs = fmt.Sprintf("工具执行错误: %v", err)
			log.Printf("❌ [Agent] 工具 %s 执行失败: %v", parsed.Action, err)
		} else {
			log.Printf("✅ [Agent] 工具 %s 执行成功，结果长度: %d", parsed.Action, len(obs))
		}

		step.Observation = obs
		trace.Steps = append(trace.Steps, step)
		trace.ToolCalls++

		// 构建下一轮prompt
		currentPrompt = fmt.Sprintf("%s\n\n%s", o.prompt, userQuery)
		for _, s := range trace.Steps {
			if s.Thought != "" {
				currentPrompt += fmt.Sprintf("\n\nThought: %s", s.Thought)
			}
			if s.Action != "" {
				currentPrompt += fmt.Sprintf("\nAction: %s\nActionInput: %s", s.Action, s.ActionInput)
			}
			if s.Observation != "" {
				currentPrompt += fmt.Sprintf("\n\n[Observation]\n%s", s.Observation)
			}
		}
		currentPrompt += "\n\n请根据以上信息继续思考。如果已有足够信息请给出FinalAnswer。"
	}

	// 如果循环结束仍未给出FinalAnswer
	if trace.FinalAnswer == "" {
		trace.Fallback = true
		trace.FinalAnswer = "抱歉，我在尝试了多次推理后仍无法确定最佳答案。请尝试换一种方式提问。"
	}

	trace.TotalTimeMs = time.Since(start).Milliseconds()
	return trace
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
