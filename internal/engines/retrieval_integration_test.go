package engines

import (
	"context"
	"strings"
	"testing"
)

func TestLegacyRetrievalNeverFabricatesEvidence(t *testing.T) {
	engine := &RetrievalIntegrationEngine{}
	results, sources, err := engine.executeLegacyRetrieval(
		context.Background(),
		&IntegratedRetrievalRequest{Query: "护理记录"},
		&SemanticAnalysisResult{},
	)
	if err == nil {
		t.Fatal("legacy retrieval must fail instead of returning fabricated evidence")
	}
	if !strings.Contains(err.Error(), "simulated retrieval is disabled") {
		t.Fatalf("unexpected legacy retrieval error: %v", err)
	}
	if results != nil || sources != nil {
		t.Fatalf("disabled legacy retrieval returned data: results=%#v sources=%#v", results, sources)
	}
}
