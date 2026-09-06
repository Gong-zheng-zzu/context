package services

import (
	"testing"

	"github.com/contextkeeper/service/internal/models"
)

func TestStructuredThreeWayGateOnlyAllowsEvaluationTargetAndControlSessions(t *testing.T) {
	t.Setenv(structuredThreeWayEnabledEnv, "true")
	base := models.StoreContextRequest{
		UserID: structuredEvaluationUserID,
		Metadata: map[string]interface{}{
			"seed_schema": structuredEvaluationSchema,
			"doc_id":      "doc-1",
		},
	}

	for _, sessionID := range []string{structuredEvaluationSession, structuredEvaluationControl} {
		req := base
		req.SessionID = sessionID
		if !isStructuredThreeWayEvaluationRequest(req) {
			t.Fatalf("structured gate rejected allowed session %q", sessionID)
		}
	}

	rejected := base
	rejected.SessionID = "another-session"
	if isStructuredThreeWayEvaluationRequest(rejected) {
		t.Fatal("structured gate accepted an unrelated session")
	}

	rejected = base
	rejected.UserID = "another-user"
	rejected.SessionID = structuredEvaluationSession
	if isStructuredThreeWayEvaluationRequest(rejected) {
		t.Fatal("structured gate accepted an unrelated user")
	}
}
