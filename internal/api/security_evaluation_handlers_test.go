package api

import "testing"

func TestInputSecurityDecisionMatchesChatPreGenerationSemantics(t *testing.T) {
	tests := []struct {
		name            string
		inputRedacted   bool
		inputNormalized bool
		decision        string
		stage           string
	}{
		{"unchanged input", false, false, "allow", "none"},
		{"ASDF normalization only", false, true, "allow", "asdf_normalized"},
		{"sensitive data redacted", true, false, "redact", "sensitive_data_policy"},
		{"redaction takes precedence", true, true, "redact", "sensitive_data_policy"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision, stage := inputSecurityDecision(test.inputRedacted, test.inputNormalized)
			if decision != test.decision || stage != test.stage {
				t.Fatalf("inputSecurityDecision(%t, %t) = (%q, %q), want (%q, %q)", test.inputRedacted, test.inputNormalized, decision, stage, test.decision, test.stage)
			}
		})
	}
}
