package agent

import "time"

// AgentStep 单步推理记录
type AgentStep struct {
	StepNumber  int           `json:"step_number"`
	Thought     string        `json:"thought"`
	Action      string        `json:"action,omitempty"`
	ActionInput string        `json:"action_input,omitempty"`
	Observation string        `json:"observation,omitempty"`
	DurationMs  time.Duration `json:"duration_ms"`
}

// AgentTrace 完整推理轨迹
type AgentTrace struct {
	Steps       []AgentStep `json:"steps"`
	FinalAnswer string      `json:"final_answer"`
	TotalTimeMs int64       `json:"total_time_ms"`
	ToolCalls   int         `json:"tool_calls"`
	Iterations  int         `json:"iterations"`
	Fallback    bool        `json:"fallback"`
}

// ParsedLLMOutput LLM输出解析结果
type ParsedLLMOutput struct {
	Thought     string
	Action      string
	ActionInput string
	FinalAnswer string
	HasAction   bool
}
