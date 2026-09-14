package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type mutatingTestTool struct{}

func (mutatingTestTool) Name() string                                    { return "mutate" }
func (mutatingTestTool) Description() string                             { return "test mutating tool" }
func (mutatingTestTool) IsReadOnly() bool                                { return false }
func (mutatingTestTool) Execute(context.Context, string) (string, error) { return "mutated", nil }

func TestToolRegistryReadOnlyPolicy(t *testing.T) {
	r := NewToolRegistry()
	r.Register(mutatingTestTool{})
	if _, err := r.Execute(context.Background(), "mutate", "ok"); err == nil {
		t.Fatal("read-only registry must reject mutating tools")
	}
	r.SetReadOnly(false)
	if got, err := r.Execute(context.Background(), "mutate", "ok"); err != nil || got != "mutated" {
		t.Fatalf("write-enabled registry should execute tool, got %q, %v", got, err)
	}
}

func TestToolRegistryRejectsUndeclaredCapability(t *testing.T) {
	r := NewToolRegistry()
	r.Register(undeclaredTestTool{})
	if _, err := r.Execute(context.Background(), "undeclared", "ok"); err == nil {
		t.Fatal("read-only registry must reject tools without an explicit capability")
	}
}

type undeclaredTestTool struct{}

func (undeclaredTestTool) Name() string                                    { return "undeclared" }
func (undeclaredTestTool) Description() string                             { return "test tool without capability" }
func (undeclaredTestTool) Execute(context.Context, string) (string, error) { return "ok", nil }

func TestToolRegistryAllowlistAndInputLimit(t *testing.T) {
	r := NewToolRegistry()
	r.Register(&mockTool{name: "allowed", execute: func(context.Context, string) (string, error) { return "ok", nil }})
	r.Register(&mockTool{name: "other", execute: func(context.Context, string) (string, error) { return "other", nil }})
	r.SetAllowlist("allowed")
	if _, err := r.Execute(context.Background(), "other", ""); err == nil || !strings.Contains(err.Error(), "白名单") {
		t.Fatalf("expected allowlist rejection, got %v", err)
	}
	r.SetMaxInputBytes(3)
	if _, err := r.Execute(context.Background(), "allowed", "abcd"); err == nil || !strings.Contains(err.Error(), "字节限制") {
		t.Fatalf("expected input limit rejection, got %v", err)
	}
}

func TestAgentStepDoesNotSerializePrivateFields(t *testing.T) {
	step := AgentStep{StepNumber: 1, Thought: "private", Action: "memory_search", ActionInput: "secret", Observation: "sensitive"}
	b, err := json.Marshal(step)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, forbidden := range []string{"private", "secret", "sensitive", "thought", "action_input", "observation"} {
		if strings.Contains(strings.ToLower(s), forbidden) {
			t.Fatalf("serialized step exposed %q: %s", forbidden, s)
		}
	}
	if !strings.Contains(s, "memory_search") {
		t.Fatalf("serialized step omitted auditable tool name: %s", s)
	}
}

func TestOrchestratorRejectsOversizedQueryBeforeLLM(t *testing.T) {
	llm := &mockLLMCaller{}
	trace := NewOrchestrator(llm, NewToolRegistry(), "system").Run(context.Background(), strings.Repeat("x", maxQueryBytes+1))
	if !trace.Fallback || trace.ToolCalls != 0 || !strings.Contains(trace.FinalAnswer, "字节限制") {
		t.Fatalf("expected bounded-query fallback, got %+v", trace)
	}
	if llm.callIndex != 0 {
		t.Fatal("oversized query must not invoke the LLM")
	}
}
