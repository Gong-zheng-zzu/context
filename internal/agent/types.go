package agent

import "time"

// AgentStep 单步推理记录
type AgentStep struct {
	StepNumber int `json:"step_number"`
	// Thought, ActionInput and Observation are request-local model context. They
	// must never be serialized as part of a public execution trace.
	Thought     string        `json:"-"`
	Action      string        `json:"action,omitempty"`
	ActionInput string        `json:"-"`
	Observation string        `json:"-"`
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
	// SourceEvidence contains narrowly scoped, public provenance from the
	// authoritative-source tool. General trace serialization excludes it.
	SourceEvidence []PublicSourceEvidence `json:"-"`
}

// ParsedLLMOutput LLM输出解析结果
type ParsedLLMOutput struct {
	Thought     string
	Action      string
	ActionInput string
	FinalAnswer string
	HasAction   bool
}
