package api

import (
	"context"
	"strings"
	"testing"

	"github.com/contextkeeper/service/internal/agent"
)

func TestControlledAgentRegistryOnlyRegistersEvidenceTools(t *testing.T) {
	registry := newControlledAgentRegistry(nil)
	allowed := []string{"memory_search", "data_manage", "auto_summary", "authoritative_web_search"}
	for _, name := range allowed {
		if _, ok := registry.Get(name); !ok {
			t.Fatalf("controlled registry is missing %q", name)
		}
	}
	for _, name := range []string{"vital_signs", "resident_profile", "trend_analysis", "generate_chart"} {
		if _, ok := registry.Get(name); ok {
			t.Fatalf("controlled registry must not register legacy simulated tool %q", name)
		}
	}
}

func TestControlledAgentRegistryRejectsUnapprovedTool(t *testing.T) {
	registry := newControlledAgentRegistry(nil)
	registry.Register(&registryTestTool{name: "not_approved"})
	_, err := registry.Execute(context.Background(), "not_approved", "query")
	if err == nil || !strings.Contains(err.Error(), "白名单") {
		t.Fatalf("unapproved tool error = %v, want allowlist rejection", err)
	}
}

type registryTestTool struct{ name string }

func (t *registryTestTool) Name() string                                    { return t.name }
func (t *registryTestTool) Description() string                             { return "test only" }
func (t *registryTestTool) IsReadOnly() bool                                { return true }
func (t *registryTestTool) Execute(context.Context, string) (string, error) { return "ok", nil }

var _ agent.Tool = (*registryTestTool)(nil)
