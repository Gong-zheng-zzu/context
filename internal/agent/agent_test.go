package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/contextkeeper/service/internal/services"
)

// === Parser 测试 ===

func TestParser_FinalAnswer(t *testing.T) {
	t.Log("╔══════════════════════════════════════════════════════════╗")
	t.Log("║  测试[1]: 解析FinalAnswer格式                           ║")
	t.Log("╚══════════════════════════════════════════════════════════╝")

	input := `Thought: 用户询问血压情况，我需要查询生命体征数据。
FinalAnswer: 张大爷最近的血压为135/85 mmHg，略高于正常范围。`

	result := ParseOutput(input)

	t.Logf("  Thought: %q", result.Thought)
	t.Logf("  FinalAnswer: %q", result.FinalAnswer)
	t.Logf("  HasAction: %v", result.HasAction)

	if result.FinalAnswer == "" {
		t.Error("FAIL: 期望解析到FinalAnswer")
	}
	if result.HasAction {
		t.Error("FAIL: 期望HasAction为false")
	}
	t.Log("  └─ PASS ✓")
}

func TestParser_Action(t *testing.T) {
	t.Log("╔══════════════════════════════════════════════════════════╗")
	t.Log("║  测试[2]: 解析Action格式                                ║")
	t.Log("╚══════════════════════════════════════════════════════════╝")

	input := `Thought: 用户想查看生命体征数据，我应该调用vital_signs工具。
Action: vital_signs
ActionInput: resident_001`

	result := ParseOutput(input)

	t.Logf("  Thought: %q", result.Thought)
	t.Logf("  Action: %q", result.Action)
	t.Logf("  ActionInput: %q", result.ActionInput)
	t.Logf("  HasAction: %v", result.HasAction)

	if !result.HasAction {
		t.Error("FAIL: 期望HasAction为true")
	}
	if result.Action != "vital_signs" {
		t.Errorf("FAIL: 期望Action=vital_signs, 实际=%s", result.Action)
	}
	if result.ActionInput != "resident_001" {
		t.Errorf("FAIL: 期望ActionInput=resident_001, 实际=%s", result.ActionInput)
	}
	t.Log("  └─ PASS ✓")
}

func TestParser_EmptyInput(t *testing.T) {
	t.Log("╔══════════════════════════════════════════════════════════╗")
	t.Log("║  测试[3]: 空输入解析                                    ║")
	t.Log("╚══════════════════════════════════════════════════════════╝")

	result := ParseOutput("")
	t.Logf("  空输入结果: HasAction=%v, FinalAnswer=%q", result.HasAction, result.FinalAnswer)

	if result.HasAction || result.FinalAnswer != "" {
		t.Error("FAIL: 空输入应返回空结果")
	}
	t.Log("  └─ PASS ✓")
}

func TestParser_PlaintextResponse(t *testing.T) {
	t.Log("╔══════════════════════════════════════════════════════════╗")
	t.Log("║  测试[4]: 纯文本回复（无结构化标记）                    ║")
	t.Log("╚══════════════════════════════════════════════════════════╝")

	input := "您好！张大爷今天血压正常，请放心。"
	result := ParseOutput(input)

	t.Logf("  HasAction: %v, FinalAnswer: %q", result.HasAction, result.FinalAnswer)

	if result.HasAction {
		t.Error("FAIL: 纯文本不应解析出Action")
	}
	t.Log("  └─ PASS ✓ (解析为无结构化内容)")
}

func TestParser_ThoughtOnly(t *testing.T) {
	t.Log("╔══════════════════════════════════════════════════════════╗")
	t.Log("║  测试[5]: 仅有Thought（无Action无FinalAnswer）          ║")
	t.Log("╚══════════════════════════════════════════════════════════╝")

	input := "Thought: 我需要分析用户的请求，但还不确定该怎么做。"
	result := ParseOutput(input)

	t.Logf("  Thought: %q, HasAction: %v", result.Thought, result.HasAction)

	if result.Thought == "" {
		t.Error("FAIL: 期望解析到Thought")
	}
	if result.HasAction || result.FinalAnswer != "" {
		t.Error("FAIL: 仅有Thought时不应有Action或FinalAnswer")
	}
	t.Log("  └─ PASS ✓")
}

// === ToolRegistry 测试 ===

type mockTool struct {
	name    string
	execute func(ctx context.Context, input string) (string, error)
}

func (t *mockTool) Name() string        { return t.name }
func (t *mockTool) Description() string { return "模拟工具: " + t.name }
func (t *mockTool) IsReadOnly() bool    { return true }
func (t *mockTool) Execute(ctx context.Context, input string) (string, error) {
	return t.execute(ctx, input)
}

func TestToolRegistry_RegisterAndGet(t *testing.T) {
	t.Log("╔══════════════════════════════════════════════════════════╗")
	t.Log("║  测试[6]: 工具注册和获取                                ║")
	t.Log("╚══════════════════════════════════════════════════════════╝")

	registry := NewToolRegistry()
	registry.Register(&mockTool{name: "test_tool", execute: func(ctx context.Context, input string) (string, error) {
		return "结果: " + input, nil
	}})

	got, ok := registry.Get("test_tool")
	if !ok {
		t.Fatal("FAIL: 未找到已注册的工具")
	}
	if got.Name() != "test_tool" {
		t.Errorf("FAIL: 工具名不匹配, 期望test_tool, 实际%s", got.Name())
	}
	t.Log("  └─ PASS ✓")
}

func TestToolRegistry_Execute(t *testing.T) {
	t.Log("╔══════════════════════════════════════════════════════════╗")
	t.Log("║  测试[7]: 工具执行                                      ║")
	t.Log("╚══════════════════════════════════════════════════════════╝")

	registry := NewToolRegistry()
	registry.Register(&mockTool{name: "echo", execute: func(ctx context.Context, input string) (string, error) {
		return "echo: " + input, nil
	}})

	result, err := registry.Execute(context.Background(), "echo", "hello")
	if err != nil {
		t.Fatalf("FAIL: 执行出错: %v", err)
	}
	t.Logf("  输入: hello, 输出: %s", result)
	if result != "echo: hello" {
		t.Errorf("FAIL: 期望echo: hello, 实际%s", result)
	}
	t.Log("  └─ PASS ✓")
}

func TestToolRegistry_ExecuteNotFound(t *testing.T) {
	t.Log("╔══════════════════════════════════════════════════════════╗")
	t.Log("║  测试[8]: 执行不存在的工具                              ║")
	t.Log("╚══════════════════════════════════════════════════════════╝")

	registry := NewToolRegistry()
	_, err := registry.Execute(context.Background(), "nonexistent", "input")

	if err == nil {
		t.Error("FAIL: 期望返回错误")
	} else {
		t.Logf("  预期错误: %v", err)
		t.Log("  └─ PASS ✓")
	}
}

func TestToolRegistry_ToolDescriptions(t *testing.T) {
	t.Log("╔══════════════════════════════════════════════════════════╗")
	t.Log("║  测试[9]: 工具描述列表                                  ║")
	t.Log("╚══════════════════════════════════════════════════════════╝")

	registry := NewToolRegistry()
	registry.Register(&mockTool{name: "tool_a"})
	registry.Register(&mockTool{name: "tool_b"})

	desc := registry.ToolDescriptions()
	t.Logf("  描述:\n%s", desc)

	if !strings.Contains(desc, "tool_a") || !strings.Contains(desc, "tool_b") {
		t.Error("FAIL: 描述中缺少工具名")
	}
	t.Log("  └─ PASS ✓")
}

// === Orchestrator 测试（使用mock LLM）===

type mockLLMCaller struct {
	responses []string
	callIndex int
}

func (m *mockLLMCaller) GenerateResponse(ctx context.Context, req *services.GenerateRequest) (*services.GenerateResponse, error) {
	if m.callIndex >= len(m.responses) {
		return nil, fmt.Errorf("no more mock responses")
	}
	resp := m.responses[m.callIndex]
	m.callIndex++
	return &services.GenerateResponse{Content: resp}, nil
}

func TestOrchestrator_DirectAnswer(t *testing.T) {
	t.Log("╔══════════════════════════════════════════════════════════╗")
	t.Log("║  测试[10]: 编排器 - 直接回答（无工具调用）              ║")
	t.Log("╚══════════════════════════════════════════════════════════╝")

	llm := &mockLLMCaller{
		responses: []string{
			"Thought: 用户问的是简单问题。\nFinalAnswer: 你好！张大爷今天状态不错。",
		},
	}
	registry := NewToolRegistry()

	orch := NewOrchestrator(llm, registry, "你是医疗助手")
	trace := orch.Run(context.Background(), "张大爷今天怎么样？")

	t.Logf("  迭代次数: %d", trace.Iterations)
	t.Logf("  工具调用: %d", trace.ToolCalls)
	t.Logf("  FinalAnswer: %s", trace.FinalAnswer)
	t.Logf("  Fallback: %v", trace.Fallback)

	if trace.FinalAnswer == "" {
		t.Error("FAIL: 期望有FinalAnswer")
	}
	if trace.ToolCalls != 0 {
		t.Errorf("FAIL: 期望0次工具调用, 实际%d", trace.ToolCalls)
	}
	t.Log("  └─ PASS ✓")
}

func TestOrchestrator_ToolCall(t *testing.T) {
	t.Log("╔══════════════════════════════════════════════════════════╗")
	t.Log("║  测试[11]: 编排器 - 工具调用后回答                      ║")
	t.Log("╚══════════════════════════════════════════════════════════╝")

	llm := &mockLLMCaller{
		responses: []string{
			"Thought: 需要查询血压数据。\nAction: vital_signs\nActionInput: 张大爷",
			"Thought: 已获得血压数据。\nFinalAnswer: 张大爷血压135/85，偏高。",
		},
	}
	registry := NewToolRegistry()
	registry.Register(&mockTool{name: "vital_signs", execute: func(ctx context.Context, input string) (string, error) {
		return "血压: 135/85 mmHg", nil
	}})

	orch := NewOrchestrator(llm, registry, "你是医疗助手")
	trace := orch.Run(context.Background(), "张大爷血压怎么样？")

	t.Logf("  迭代次数: %d", trace.Iterations)
	t.Logf("  工具调用: %d", trace.ToolCalls)
	t.Logf("  FinalAnswer: %s", trace.FinalAnswer)
	t.Logf("  步骤数: %d", len(trace.Steps))

	if trace.ToolCalls != 1 {
		t.Errorf("FAIL: 期望1次工具调用, 实际%d", trace.ToolCalls)
	}
	if trace.FinalAnswer == "" {
		t.Error("FAIL: 期望有FinalAnswer")
	}
	t.Log("  └─ PASS ✓")
}

func TestOrchestrator_Fallback(t *testing.T) {
	t.Log("╔══════════════════════════════════════════════════════════╗")
	t.Log("║  测试[12]: 编排器 - LLM无结构化输出时fallback           ║")
	t.Log("╚══════════════════════════════════════════════════════════╝")

	llm := &mockLLMCaller{
		responses: []string{
			"您好！张大爷今天状态不错，血压正常。",
		},
	}
	registry := NewToolRegistry()

	orch := NewOrchestrator(llm, registry, "你是医疗助手")
	trace := orch.Run(context.Background(), "张大爷怎么样？")

	t.Logf("  迭代次数: %d", trace.Iterations)
	t.Logf("  FinalAnswer: %s", trace.FinalAnswer)
	t.Logf("  Fallback: %v", trace.Fallback)

	if !trace.Fallback {
		t.Error("FAIL: 纯文本输出应触发fallback")
	}
	if trace.FinalAnswer == "" {
		t.Error("FAIL: fallback时应有FinalAnswer")
	}
	t.Log("  └─ PASS ✓")
}

// === Prompt 测试 ===

func TestBuildAgentSystemPrompt(t *testing.T) {
	t.Log("╔══════════════════════════════════════════════════════════╗")
	t.Log("║  测试[13]: Agent系统提示词构建                          ║")
	t.Log("╚══════════════════════════════════════════════════════════╝")

	prompt := BuildAgentSystemPrompt("doctor", "- vital_signs: 查询生命体征\n")
	t.Logf("  提示词长度: %d字符", len(prompt))

	if !strings.Contains(prompt, "医生助手") {
		t.Error("FAIL: doctor角色应包含医生助手")
	}
	if !strings.Contains(prompt, "vital_signs") {
		t.Error("FAIL: 提示词应包含工具描述")
	}
	if !strings.Contains(prompt, "Thought:") {
		t.Error("FAIL: 提示词应包含Thought格式说明")
	}
	t.Log("  └─ PASS ✓")
}

func TestBuildAgentQueryPrompt(t *testing.T) {
	t.Log("╔══════════════════════════════════════════════════════════╗")
	t.Log("║  测试[14]: Agent查询提示词构建                          ║")
	t.Log("╚══════════════════════════════════════════════════════════╝")

	prompt := BuildAgentQueryPrompt("血压怎么样？", "短期记忆: 上次血压130/80")
	t.Logf("  提示词: %s", prompt)

	if !strings.Contains(prompt, "血压怎么样？") {
		t.Error("FAIL: 应包含用户消息")
	}
	if !strings.Contains(prompt, "短期记忆") {
		t.Error("FAIL: 有记忆上下文时应包含记忆")
	}
	t.Log("  └─ PASS ✓")
}
