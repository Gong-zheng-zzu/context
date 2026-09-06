package api

import "testing"

func TestControlledInputOnlyEvaluation(t *testing.T) {
	valid := ChatRequest{
		SessionID:      "security-input-chain-run-01-prompt_injection_001",
		SampleID:       "prompt_injection_001",
		AttackType:     "prompt_injection",
		EvaluationName: inputOnlyEvaluationName,
	}
	if !isControlledInputOnlyEvaluation(inputOnlyEvaluationUserID, valid) {
		t.Fatal("expected controlled evaluation request to be accepted")
	}

	tests := []struct {
		name   string
		userID string
		req    ChatRequest
	}{
		{"different user", "other_user", valid},
		{"wrong name", inputOnlyEvaluationUserID, ChatRequest{SessionID: valid.SessionID, SampleID: valid.SampleID, EvaluationName: "other"}},
		{"unscoped session", inputOnlyEvaluationUserID, ChatRequest{SessionID: "normal-session", SampleID: valid.SampleID, EvaluationName: inputOnlyEvaluationName}},
		{"sample mismatch", inputOnlyEvaluationUserID, ChatRequest{SessionID: "security-input-chain-run-01-other_sample", SampleID: valid.SampleID, EvaluationName: inputOnlyEvaluationName}},
		{"unsafe sample id", inputOnlyEvaluationUserID, ChatRequest{SessionID: "security-input-chain-run-01-bad_sample", SampleID: "bad/sample", EvaluationName: inputOnlyEvaluationName}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if isControlledInputOnlyEvaluation(test.userID, test.req) {
				t.Fatal("expected controlled evaluation request to be rejected")
			}
		})
	}
}
