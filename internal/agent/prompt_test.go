package agent

import (
	"strings"
	"testing"
)

func TestControlledAgentPromptDoesNotRequestThought(t *testing.T) {
	prompt := BuildAgentSystemPrompt("caregiver", "- memory_search: retrieve records")
	if strings.Contains(prompt, "Thought:") || strings.Contains(prompt, "必须先Thought") {
		t.Fatalf("controlled Agent prompt must not request chain-of-thought: %s", prompt)
	}
	for _, required := range []string{"Action:", "ActionInput:", "FinalAnswer:", "人工确认"} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("controlled Agent prompt is missing %q", required)
		}
	}
}
