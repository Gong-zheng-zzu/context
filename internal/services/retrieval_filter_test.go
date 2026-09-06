package services

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestFilterRetrievalCandidates(t *testing.T) {
	config := RetrievalFilterConfig{
		Enabled:       true,
		RulesetSHA256: "test-sha256",
		Ruleset: RetrievalFilterRuleset{
			Version:             "v1",
			MinScore:            0.70,
			RequireUserMatch:    true,
			RequireSessionMatch: true,
			InvalidMetadataKeys: []string{"invalid"},
		},
	}
	candidates := []RetrievalFilterCandidate{
		{DocID: "keep", UserID: "user-1", SessionID: "session-1", Score: 0.70},
		{DocID: "user", UserID: "user-2", SessionID: "session-1", Score: 0.99},
		{DocID: "session", UserID: "user-1", SessionID: "session-2", Score: 0.99},
		{DocID: " ", UserID: "user-1", SessionID: "session-1", Score: 0.99},
		{DocID: "score", UserID: "user-1", SessionID: "session-1", Score: 0.69},
		{DocID: "metadata", UserID: "user-1", SessionID: "session-1", Score: 0.99, Metadata: map[string]interface{}{"invalid": "true"}},
	}

	filtered, audit := FilterRetrievalCandidates(config, RetrievalFilterScope{UserID: "user-1", SessionID: "session-1"}, candidates)
	if got, want := filtered, []RetrievalFilterCandidate{candidates[0]}; !reflect.DeepEqual(got, want) {
		t.Fatalf("filtered candidates = %#v, want %#v", got, want)
	}
	if audit.Before != 6 || audit.After != 1 || audit.RulesetVersion != "v1" || audit.RulesetSHA256 != "test-sha256" {
		t.Fatalf("unexpected audit counts or identity: %#v", audit)
	}
	wantReasons := map[string]int{
		RemovalReasonCrossUser:       1,
		RemovalReasonCrossSession:    1,
		RemovalReasonEmptyDocID:      1,
		RemovalReasonLowScore:        1,
		RemovalReasonInvalidMetadata: 1,
	}
	if !reflect.DeepEqual(audit.RemovalReasons, wantReasons) {
		t.Fatalf("removal reasons = %#v, want %#v", audit.RemovalReasons, wantReasons)
	}
}

func TestFilterRetrievalCandidatesDisabledPreservesCandidates(t *testing.T) {
	candidates := []RetrievalFilterCandidate{{DocID: "", Score: 0}, {DocID: "doc-2", Score: 1}}
	filtered, audit := FilterRetrievalCandidates(RetrievalFilterConfig{}, RetrievalFilterScope{}, candidates)
	if !reflect.DeepEqual(filtered, candidates) {
		t.Fatalf("disabled filter changed candidates: %#v", filtered)
	}
	if audit.Before != 2 || audit.After != 2 || len(audit.RemovalReasons) != 0 {
		t.Fatalf("unexpected disabled audit: %#v", audit)
	}
}

func TestLoadRetrievalFilterConfigFromEnv(t *testing.T) {
	ruleset := []byte("{\n  \"version\": \"v1\",\n  \"min_score\": 0.75,\n  \"require_user_match\": true,\n  \"require_session_match\": true,\n  \"invalid_metadata_keys\": [\"invalid\"]\n}\n")
	path := filepath.Join(t.TempDir(), "ruleset.json")
	if err := os.WriteFile(path, ruleset, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(RetrievalRuleFilterEnabledEnv, "true")
	t.Setenv(RetrievalRuleFilterRulesetEnv, path)

	config, err := LoadRetrievalFilterConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadRetrievalFilterConfigFromEnv() error = %v", err)
	}
	digest := sha256.Sum256(ruleset)
	if !config.Enabled || config.Ruleset.Version != "v1" || config.Ruleset.MinScore != 0.75 || config.RulesetSHA256 != hex.EncodeToString(digest[:]) {
		t.Fatalf("unexpected config: %#v", config)
	}
}
