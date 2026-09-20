package security

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// externalPIIItem mirrors one record of the third-party pii-bench-zh derivation
// stored at experiments/datasets/external/pii_bench_zh/external_pii_positive.json.
type externalPIIItem struct {
	ID                int      `json:"id"`
	Content           string   `json:"content"`
	ExpectedSensitive bool     `json:"expected_sensitive"`
	SensitiveTypes    []string `json:"sensitive_types"`
	AttackType        string   `json:"attack_type"`
	Category          string   `json:"category"`
}

func loadExternalPIIItems(t *testing.T) []externalPIIItem {
	t.Helper()
	path := filepath.Join("..", "..", "experiments", "datasets", "external", "pii_bench_zh", "external_pii_positive.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("external benchmark not present: %v", err)
	}
	var items []externalPIIItem
	if err := json.Unmarshal(raw, &items); err != nil {
		t.Fatalf("decode external benchmark: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("external benchmark is empty")
	}
	return items
}

// newAuditSecurityService builds the same evaluation pipeline the ablation
// endpoint uses, wired from the package's own constructors. EvaluateSecurityProfile
// only needs these three collaborators; the remaining SecurityService fields are
// for request handling, not for profile evaluation.
func newAuditSecurityService() *SecurityService {
	multiLayer := NewMultiLayerDetector("http://127.0.0.1:1", "unused")
	multiLayer.dictMatcher.LoadDefaultDictionary()
	// Both switches on, matching the server's ablation configuration.
	configureMultiLayerAlgorithms(multiLayer, true, true)
	return &SecurityService{
		detector:           NewDetector(),
		asdfFramework:      NewAdversarialSampleDefenseFramework(),
		multiLayerDetector: multiLayer,
	}
}

// TestASDFExternalDatasetRegressionAudit recomputes the full four-profile paired
// ablation in-process over the third-party corpus, instead of over HTTP.
//
// Why it exists: the HTTP run over this corpus is the only place that surfaced a
// class of regression the in-house corpus could never produce -- separator-joined
// groups of *distinct* identifiers were collapsed into a single digit blob,
// destroying every boundary-anchored match (162 regressions). Reproducing it
// in-process takes under a second and makes the guard permanent.
//
// The reported exact-McNemar p-values are derived from these 2x2 counts by the
// Python tooling (experiments/scripts/stat_tools.py), which is covered by its own
// consistency tests; this test deliberately reports counts only, so there is one
// implementation of the significance test rather than two.
//
// Skipped unless ASDF_EXTERNAL_AUDIT=1 so the default test run stays fast.
func TestASDFExternalDatasetRegressionAudit(t *testing.T) {
	if os.Getenv("ASDF_EXTERNAL_AUDIT") != "1" {
		t.Skip("set ASDF_EXTERNAL_AUDIT=1 to run the third-party corpus audit")
	}

	items := loadExternalPIIItems(t)
	service := newAuditSecurityService()
	ctx := context.Background()

	detects := make(map[string][]bool, len(SecurityLabProfiles))
	for _, profile := range SecurityLabProfiles {
		flags := make([]bool, 0, len(items))
		for _, item := range items {
			result, err := service.EvaluateSecurityProfile(ctx, profile, item.Content)
			if err != nil {
				t.Fatalf("profile %s on sample %d: %v", profile, item.ID, err)
			}
			flags = append(flags, result.Decision == "redact")
		}
		detects[profile] = flags
		positive := 0
		for _, flag := range flags {
			if flag {
				positive++
			}
		}
		t.Logf("profile=%-18s detected=%d/%d rate=%.4f", profile, positive, len(items), float64(positive)/float64(len(items)))
	}

	pairs := [][2]string{
		{"regex_only", "asdf"},
		{"asdf", "asdf_casia"},
		{"asdf_casia", "asdf_casia_pccm"},
	}
	type pairResult struct{ newlyDetected, newlyMissed, both, neither int }
	results := make(map[string]pairResult, len(pairs))

	t.Logf("paired 2x2 (previous -> next): newly_detected / newly_missed / both / neither")
	for _, pair := range pairs {
		previous, next := pair[0], pair[1]
		var r pairResult
		for i := range items {
			a, b := detects[previous][i], detects[next][i]
			switch {
			case a && b:
				r.both++
			case b && !a:
				r.newlyDetected++
			case a && !b:
				r.newlyMissed++
			default:
				r.neither++
			}
		}
		results[previous+" -> "+next] = r
		t.Logf("  %-26s newly_detected=%4d newly_missed=%4d both=%4d neither=%4d",
			previous+"->"+next, r.newlyDetected, r.newlyMissed, r.both, r.neither)
	}

	// The guard this audit exists for: normalization must never destroy a value
	// the regex-only baseline already caught.
	if r := results["regex_only -> asdf"]; r.newlyMissed != 0 {
		t.Fatalf("ASDF normalization regressed %d sample(s) that regex_only detected", r.newlyMissed)
	}
	// Normalization must not silently become a no-op either: the de-obfuscation
	// it is responsible for has to keep contributing.
	if r := results["regex_only -> asdf"]; r.newlyDetected == 0 {
		t.Fatal("ASDF normalization contributed no additional detections on the third-party corpus")
	}
	// The downstream layers are reported but not asserted: CASIA and PCCM depend
	// on corpus composition, and this test only guards the normalization contract.
}
