package calibration

import (
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/contextkeeper/service/internal/security"
)

func TestSyntheticCalibrationImprovesF1(t *testing.T) {
	devSet := SyntheticDevSet()
	options := SyntheticCalibrationOptions()

	before := Evaluate(options.BaseConfig, devSet.Samples, options.DecisionThreshold)
	config, summary, after, err := SearchWeights(devSet, options)
	if err != nil {
		t.Fatalf("SearchWeights error = %v", err)
	}

	if after.F1 < before.F1 {
		t.Fatalf("calibration must not worsen F1: before=%v after=%v", before.F1, after.F1)
	}
	if !(after.F1 > before.F1) {
		t.Fatalf("synthetic set should be improved by calibration: before=%v after=%v", before.F1, after.F1)
	}
	if len(summary.ImprovedSteps) == 0 {
		t.Fatal("expected at least one improved coordinate-descent step")
	}
	// 至少有两层被调整（Layer 1 与 Layer 5），验证多层协同。
	adjusted := map[int]bool{}
	for _, step := range summary.ImprovedSteps {
		adjusted[step.LayerID] = true
	}
	if len(adjusted) < 2 {
		t.Fatalf("expected at least two adjusted layers, got %v", adjusted)
	}
	if config.CalibrationSource != CalibrationSourceDevSetV1 {
		t.Fatalf("calibration source = %q, want %q", config.CalibrationSource, CalibrationSourceDevSetV1)
	}
	if config.Version != CalibratedConfigVersion {
		t.Fatalf("calibrated version = %q, want %q", config.Version, CalibratedConfigVersion)
	}
}

func TestSearchIsMonotonicAndConverges(t *testing.T) {
	devSet := SyntheticDevSet()
	options := SyntheticCalibrationOptions()

	_, summary, _, err := SearchWeights(devSet, options)
	if err != nil {
		t.Fatalf("SearchWeights error = %v", err)
	}
	if summary.Rounds > options.MaxRounds {
		t.Fatalf("rounds %d exceeded max rounds %d", summary.Rounds, options.MaxRounds)
	}
	if summary.Method != searchMethodCoordinateDescent {
		t.Fatalf("method = %q, want %q", summary.Method, searchMethodCoordinateDescent)
	}

	// 成功步骤的目标分数必须严格单调递增。
	for i := 1; i < len(summary.ImprovedSteps); i++ {
		if summary.ImprovedSteps[i].Score <= summary.ImprovedSteps[i-1].Score {
			t.Fatalf("improvement score not strictly increasing at step %d: %v <= %v",
				i, summary.ImprovedSteps[i].Score, summary.ImprovedSteps[i-1].Score)
		}
	}
}

func TestSearchIsDeterministic(t *testing.T) {
	devSet := SyntheticDevSet()
	options := SyntheticCalibrationOptions()

	first, _, _, err := SearchWeights(devSet, options)
	if err != nil {
		t.Fatalf("first SearchWeights error = %v", err)
	}
	second, _, _, err := SearchWeights(devSet, options)
	if err != nil {
		t.Fatalf("second SearchWeights error = %v", err)
	}
	if !reflect.DeepEqual(first.LayerWeights, second.LayerWeights) {
		t.Fatalf("search not deterministic: %v vs %v", first.LayerWeights, second.LayerWeights)
	}
}

func TestErrorWeightedObjectiveImproves(t *testing.T) {
	devSet := SyntheticDevSet()
	options := SyntheticCalibrationOptions()
	options.Objective = ObjectiveErrorWeighted
	options.FalsePositivePenalty = 1.0
	options.FalseNegativePenalty = 1.0

	before := Evaluate(options.BaseConfig, devSet.Samples, options.DecisionThreshold)
	_, _, after, err := SearchWeights(devSet, options)
	if err != nil {
		t.Fatalf("SearchWeights error = %v", err)
	}

	beforeScore := objectiveScore(ObjectiveErrorWeighted, before, 1, 1)
	afterScore := objectiveScore(ObjectiveErrorWeighted, after, 1, 1)
	if afterScore < beforeScore-searchScoreEpsilon {
		t.Fatalf("error_weighted objective worsened: before=%v after=%v", beforeScore, afterScore)
	}
}

func TestSearchRejectsUnknownObjective(t *testing.T) {
	options := SyntheticCalibrationOptions()
	options.Objective = "not-a-real-objective"
	if _, _, _, err := SearchWeights(SyntheticDevSet(), options); err == nil {
		t.Fatal("expected error for unknown objective")
	}
}

func TestSearchRejectsEmptyDevSet(t *testing.T) {
	if _, _, _, err := SearchWeights(DevSet{Name: "empty", Source: DevSetSourceSynthetic}, SyntheticCalibrationOptions()); err == nil {
		t.Fatal("expected error for empty dev set")
	}
}

func TestEvaluateSeparatesPerfectPredictions(t *testing.T) {
	devSet := SyntheticDevSet()
	// 训练到最优后用同配置评估，应达到无漏报误报。
	config, _, _, err := SearchWeights(devSet, SyntheticCalibrationOptions())
	if err != nil {
		t.Fatalf("SearchWeights error = %v", err)
	}
	metrics := Evaluate(config, devSet.Samples, SyntheticDecisionThreshold)
	if metrics.FalsePositives != 0 || metrics.FalseNegatives != 0 {
		t.Fatalf("expected perfectly separated synthetic set, got %+v", metrics)
	}
	if math.Abs(metrics.F1-1.0) > pccmTestEpsilon {
		t.Fatalf("expected F1 = 1.0, got %v", metrics.F1)
	}
}

const pccmTestEpsilon = 1e-9

func TestRunProducesTraceableArtifact(t *testing.T) {
	devSet := SyntheticDevSet()
	generatedAt := time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC)

	artifact, err := Run(devSet, SyntheticCalibrationOptions(), generatedAt)
	if err != nil {
		t.Fatalf("Run error = %v", err)
	}

	if artifact.SchemaVersion != SchemaVersion {
		t.Fatalf("schema version = %q, want %q", artifact.SchemaVersion, SchemaVersion)
	}
	if artifact.Fingerprint.DatasetHash != devSet.Hash() {
		t.Fatalf("dataset hash mismatch: %q vs %q", artifact.Fingerprint.DatasetHash, devSet.Hash())
	}
	if artifact.Fingerprint.SampleCount != len(devSet.Samples) {
		t.Fatalf("sample count = %d, want %d", artifact.Fingerprint.SampleCount, len(devSet.Samples))
	}
	if artifact.Fingerprint.DatasetSource != DevSetSourceSynthetic {
		t.Fatalf("dataset source = %q, want %q", artifact.Fingerprint.DatasetSource, DevSetSourceSynthetic)
	}
	if artifact.Fingerprint.GeneratedAt != "2026-09-17T08:00:00Z" {
		t.Fatalf("generated_at = %q", artifact.Fingerprint.GeneratedAt)
	}
	if artifact.Fingerprint.SearchSpace == "" || artifact.Fingerprint.Objective == "" {
		t.Fatal("fingerprint missing search space or objective")
	}
	if artifact.MetricsAfter.F1 < artifact.MetricsBefore.F1 {
		t.Fatalf("artifact metrics regressed: before=%v after=%v", artifact.MetricsBefore.F1, artifact.MetricsAfter.F1)
	}
}

func TestArtifactSerializationRoundTrip(t *testing.T) {
	artifact, err := Run(SyntheticDevSet(), SyntheticCalibrationOptions(), time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatalf("Run error = %v", err)
	}

	data, err := artifact.MarshalIndent()
	if err != nil {
		t.Fatalf("MarshalIndent error = %v", err)
	}
	parsed, err := ParseArtifact(data)
	if err != nil {
		t.Fatalf("ParseArtifact error = %v", err)
	}
	if !reflect.DeepEqual(artifact, parsed) {
		t.Fatalf("artifact round trip mismatch:\noriginal=%+v\nparsed=%+v", artifact, parsed)
	}
}

func TestLoadPCCMConfigFromFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pccm_calibration.json")
	artifact, err := Run(SyntheticDevSet(), SyntheticCalibrationOptions(), time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatalf("Run error = %v", err)
	}
	if err := WriteArtifact(path, artifact); err != nil {
		t.Fatalf("WriteArtifact error = %v", err)
	}

	config, err := LoadPCCMConfigFromFile(path)
	if err != nil {
		t.Fatalf("LoadPCCMConfigFromFile error = %v", err)
	}
	if config.CalibrationSource != CalibrationSourceDevSetV1 {
		t.Fatalf("loaded calibration source = %q", config.CalibrationSource)
	}
	if !reflect.DeepEqual(config.LayerWeights, artifact.Config.LayerWeights) {
		t.Fatalf("loaded weights = %v, want %v", config.LayerWeights, artifact.Config.LayerWeights)
	}
}

func TestLoadPCCMConfigMissingFileFallsBackToDefault(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.json")

	if _, err := LoadPCCMConfigFromFile(missing); err == nil {
		t.Fatal("expected error when artifact file is missing")
	}

	fallback := LoadPCCMConfigOrDefault(missing)
	want := security.DefaultPCCMSecurityConfig()
	if fallback.CalibrationSource != want.CalibrationSource {
		t.Fatalf("fallback calibration source = %q, want %q", fallback.CalibrationSource, want.CalibrationSource)
	}
	if !reflect.DeepEqual(fallback.LayerWeights, want.LayerWeights) {
		t.Fatalf("fallback weights = %v, want %v", fallback.LayerWeights, want.LayerWeights)
	}
}

func TestLoadPCCMConfigRejectsInvalidArtifact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.json")
	// 缺少层权重的产物应被拒绝。
	if err := os.WriteFile(path, []byte(`{"schema_version":"`+SchemaVersion+`","config":{}}`), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
	if _, err := LoadPCCMConfigFromFile(path); err == nil {
		t.Fatal("expected error for artifact without layer weights")
	}
	fallback := LoadPCCMConfigOrDefault(path)
	if fallback.CalibrationSource != security.DefaultPCCMSecurityConfig().CalibrationSource {
		t.Fatalf("fallback after invalid artifact = %q", fallback.CalibrationSource)
	}
}

func TestDevSetFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devset.json")
	devSet := SyntheticDevSet()
	if err := WriteDevSet(path, devSet); err != nil {
		t.Fatalf("WriteDevSet error = %v", err)
	}
	loaded, err := LoadDevSetFromFile(path)
	if err != nil {
		t.Fatalf("LoadDevSetFromFile error = %v", err)
	}
	if !reflect.DeepEqual(loaded, devSet) {
		t.Fatalf("dev set round trip mismatch:\nloaded=%+v\noriginal=%+v", loaded, devSet)
	}
	if loaded.Hash() != devSet.Hash() {
		t.Fatalf("dataset hash changed after round trip")
	}
}

func TestDevSetValidationRejectsInvalidInput(t *testing.T) {
	cases := map[string]DevSet{
		"empty": {Name: "empty"},
		"missing_layer_confidences": {
			Name:    "bad",
			Samples: []LabeledSample{{ID: "s1", ExpectedSensitive: true}},
		},
		"duplicate_id": {
			Name: "bad",
			Samples: []LabeledSample{
				{ID: "s1", LayerConfidences: map[int]float64{1: 0.5}},
				{ID: "s1", LayerConfidences: map[int]float64{1: 0.5}},
			},
		},
		"out_of_range": {
			Name:    "bad",
			Samples: []LabeledSample{{ID: "s1", LayerConfidences: map[int]float64{1: 1.5}}},
		},
	}
	for name, devSet := range cases {
		if err := devSet.Validate(); err == nil {
			t.Fatalf("%s: expected validation error", name)
		}
	}
}

func TestDefaultSecurityConfigIsUnchanged(t *testing.T) {
	config := security.DefaultPCCMSecurityConfig()
	if config.CalibrationSource != "fixed_unvalidated" {
		t.Fatalf("default calibration source = %q, want fixed_unvalidated", config.CalibrationSource)
	}
	if config.LayerWeights[1] != 0.70 || config.LayerWeights[5] != 0.05 {
		t.Fatalf("default weights changed: %v", config.LayerWeights)
	}
}
