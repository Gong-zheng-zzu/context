package agent

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestPublicSourceEvidenceFromToolOutputRetainsOnlyPublicProvenance(t *testing.T) {
	result := WebSearchResult{
		Status:        WebSearchOK,
		Title:         "Public guidance",
		Summary:       "This observation must not reach the browser contract.",
		URL:           "https://www.who.int/example",
		RetrievedAt:   time.Date(2026, 9, 14, 7, 0, 0, 0, time.UTC),
		ContentSHA256: strings.Repeat("a", 64),
		SourceDomain:  "www.who.int",
	}
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	evidence, ok := PublicSourceEvidenceFromToolOutput("authoritative_web_search", string(payload))
	if !ok || evidence.URL != result.URL || evidence.ContentSHA256 != result.ContentSHA256 {
		t.Fatalf("evidence = %#v, ok=%v", evidence, ok)
	}
	serialized, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(serialized), result.Summary) {
		t.Fatalf("public provenance leaked web observation: %s", serialized)
	}
}

func TestPublicSourceEvidenceFromToolOutputRejectsInvalidOrUnrelatedOutput(t *testing.T) {
	if _, ok := PublicSourceEvidenceFromToolOutput("memory_search", `{"status":"ok"}`); ok {
		t.Fatal("unrelated tool output must not become web evidence")
	}
	if _, ok := PublicSourceEvidenceFromToolOutput("authoritative_web_search", `{"status":"ok","url":"https://evil.example"}`); ok {
		t.Fatal("unapproved URL must not become web evidence")
	}
}
