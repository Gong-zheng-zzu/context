package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuditChainAppendsAndVerifies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit", "events.jsonl")
	chain, err := NewAuditChain(path)
	if err != nil {
		t.Fatalf("NewAuditChain() error = %v", err)
	}
	first, err := chain.Append(map[string]any{"trace_id": "trace-1", "event": "delete"})
	if err != nil || len(first) != 64 {
		t.Fatalf("first Append() = %q, %v", first, err)
	}
	second, err := chain.Append(map[string]any{"trace_id": "trace-2", "event": "delete"})
	if err != nil || len(second) != 64 || first == second {
		t.Fatalf("second Append() = %q, %v", second, err)
	}
	if err := chain.Verify(); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestAuditChainDetectsTampering(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	chain, err := NewAuditChain(path)
	if err != nil {
		t.Fatalf("NewAuditChain() error = %v", err)
	}
	if _, err := chain.Append(map[string]string{"event": "delete"}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(contents), "delete", "altered", 1)), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := chain.Verify(); err == nil {
		t.Fatal("Verify() accepted a tampered entry")
	}
}
