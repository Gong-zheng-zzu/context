package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/contextkeeper/service/internal/agent"
)

func TestAgentExecutionSummaryDoesNotSerializeSensitiveTraceFields(t *testing.T) {
	trace := &agent.AgentTrace{
		Steps: []agent.AgentStep{{
			StepNumber:  1,
			Thought:     "private chain of thought",
			Action:      "memory_search",
			ActionInput: "sensitive tool arguments",
			Observation: "sensitive tool result",
			DurationMs:  25 * time.Millisecond,
		}},
		FinalAnswer: "internal final answer",
		ToolCalls:   1,
		TotalTimeMs: 25,
	}

	payload, err := json.Marshal(ChatResponse{AgentExecution: newAgentExecutionSummary(trace)})
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	serialized := string(payload)
	for _, forbidden := range []string{"agent_trace", "thought", "action_input", "observation", "final_answer", "sensitive tool"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("public response exposed forbidden field or value %q: %s", forbidden, serialized)
		}
	}
	if !strings.Contains(serialized, "memory_search") || !strings.Contains(serialized, "agent_execution") {
		t.Fatalf("public response omitted execution summary: %s", serialized)
	}
}
