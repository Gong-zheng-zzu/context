package services

import (
	"testing"
	"time"

	"github.com/contextkeeper/service/internal/config"
	"github.com/contextkeeper/service/internal/engines/memory_subgraph"
)

func TestCompetitionCapabilitiesKeepsOwnerEvidenceAndScoresCandidates(t *testing.T) {
	service, err := NewCompetitionCapabilityService(&config.Config{StoragePath: t.TempDir()})
	if err != nil {
		t.Fatalf("NewCompetitionCapabilityService: %v", err)
	}
	if err := service.AddEpisode(&memory_subgraph.EpisodicNode{ID: "ep-1", OwnerID: "resident-a", Source: memory_subgraph.SourceText, Content: "服药后头晕"}); err != nil {
		t.Fatalf("AddEpisode: %v", err)
	}
	if err := service.AddEntity(&memory_subgraph.SemanticEntity{ID: "symptom-1", OwnerID: "resident-a", Name: "头晕", Kind: memory_subgraph.EntitySymptom, Episodes: []string{"ep-1"}}); err != nil {
		t.Fatalf("AddEntity: %v", err)
	}
	if _, err := service.Communities("resident-a"); err != nil {
		t.Fatalf("Communities: %v", err)
	}
	if communities, err := service.Communities("resident-b"); err != nil || len(communities) != 0 {
		t.Fatalf("Communities for another owner = %#v, %v; want an empty owner-scoped list", communities, err)
	}
	ranked, err := service.Rank("resident-a", []RiskRankInput{{ID: "ep-1", Content: "untrusted browser payload", Semantic: 0.9}})
	if err != nil || len(ranked) != 1 || ranked[0].ID != "ep-1" {
		t.Fatalf("Rank = %#v, %v", ranked, err)
	}
	if _, err := service.Rank("resident-b", []RiskRankInput{{ID: "ep-1", Semantic: 0.9}}); err == nil {
		t.Fatal("Rank accepted another owner's evidence")
	}
	if err := service.AddEdge(&memory_subgraph.FactEdge{ID: "cross-owner-evidence", OwnerID: "resident-b", Episodes: []string{"ep-1"}}); err == nil {
		t.Fatal("AddEdge accepted another owner's evidence")
	}
	if status := service.LifecycleStatus(); status.Enabled || status.Running || status.Implementation != "in_process_evidence_lifecycle" {
		t.Fatalf("LifecycleStatus = %#v", status)
	}
}

func TestCompetitionCapabilitiesReportsEnabledLifecycleWithoutLeakingArchivePaths(t *testing.T) {
	service, err := NewCompetitionCapabilityService(&config.Config{
		StoragePath:            t.TempDir(),
		TieredLifecycleEnabled: true,
		TierEvalInterval:       time.Hour,
		TierHotMaxAge:          time.Hour,
		TierWarmMaxAge:         2 * time.Hour,
		TierColdMaxAge:         3 * time.Hour,
		TierFrozenMaxAge:       4 * time.Hour,
		TierHotBudgetMB:        1,
		TierWarmBudgetMB:       1,
		TierMinAccessKeepWarm:  1,
	})
	if err != nil {
		t.Fatalf("NewCompetitionCapabilityService: %v", err)
	}
	t.Cleanup(service.Stop)
	if status := service.LifecycleStatus(); !status.Enabled || !status.Running || status.Implementation != "in_process_evidence_lifecycle" {
		t.Fatalf("LifecycleStatus = %#v", status)
	}
	if err := service.AddEpisode(&memory_subgraph.EpisodicNode{ID: "lifecycle-episode", OwnerID: "resident-a", Source: memory_subgraph.SourceText, Content: "synthetic"}); err != nil {
		t.Fatalf("AddEpisode: %v", err)
	}
}
