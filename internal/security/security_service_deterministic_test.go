package security

import (
	"context"
	"strings"
	"testing"
)

func TestScanContentDeterministicDoesNotInvokeLLM(t *testing.T) {
	t.Setenv("SECURITY_ENCRYPTION_SECRET", "deterministic-security-test-secret")
	service, err := NewSecurityService("../../config/security_policy.yaml", t.TempDir()+"/audit.log")
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()

	result, err := service.ScanContentDeterministic(context.Background(), "causal-test", "user-test", "联系手机号 138-1234-5678")
	if err != nil {
		t.Fatal(err)
	}
	if result.Metadata["llm_invoked"] != false {
		t.Fatalf("llm_invoked = %#v, want false", result.Metadata["llm_invoked"])
	}
	if result.Metadata["profile"] != "asdf_casia_pccm" {
		t.Fatalf("profile = %#v", result.Metadata["profile"])
	}
	if strings.Contains(result.RedactedContent, "138-1234-5678") || strings.Contains(result.RedactedContent, "13812345678") {
		t.Fatalf("redacted content leaked phone: %q", result.RedactedContent)
	}
}
