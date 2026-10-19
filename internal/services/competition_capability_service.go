package services

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/contextkeeper/service/internal/config"
	"github.com/contextkeeper/service/internal/engines/memory_subgraph"
	"github.com/contextkeeper/service/internal/engines/multi_dimensional_retrieval"
)

// CompetitionCapabilityService is the HTTP runtime boundary for the optional
// lifecycle, evidence-subgraph, and risk-aware reranking capabilities. It is
// intentionally in-process until a persistent graph adapter has passed the
// same ownership and deletion tests.
type CompetitionCapabilityService struct {
	graph *memory_subgraph.MemorySubgraph
	tier  *TieredLifecycleService

	mu       sync.RWMutex
	payloads map[string][]byte
	owners   map[string]string
	assessor *multi_dimensional_retrieval.RiskAssessor
}

type RiskRankInput struct {
	ID          string  `json:"id"`
	Content     string  `json:"content"`
	Semantic    float64 `json:"semantic"`
	Temporal    float64 `json:"temporal"`
	Graph       float64 `json:"graph"`
	Structural  float64 `json:"structural"`
	ContextRisk float64 `json:"context_risk"`
	PrivacyCost float64 `json:"privacy_cost"`
	SourceRisk  float64 `json:"source_risk"`
}

// LifecycleStatus deliberately contains metadata only. Payload bytes and
// archive paths remain server-side, so status queries cannot become a local
// storage oracle.
type LifecycleStatus struct {
	Enabled        bool        `json:"enabled"`
	Running        bool        `json:"running"`
	Applied        int64       `json:"applied"`
	Failed         int64       `json:"failed"`
	LastCycle      *CycleStats `json:"last_cycle,omitempty"`
	Implementation string      `json:"implementation"`
}

// NewCompetitionCapabilityService joins optional capabilities to the HTTP
// runtime. A nil scorer is intentional and is reported to callers instead of
// fabricating a sensitive-content signal.
func NewCompetitionCapabilityService(cfg *config.Config, scorer ...multi_dimensional_retrieval.SensitiveScorer) (*CompetitionCapabilityService, error) {
	if cfg == nil {
		return nil, fmt.Errorf("competition capabilities: nil config")
	}
	s := &CompetitionCapabilityService{
		graph:    memory_subgraph.NewMemorySubgraph(memory_subgraph.DefaultConfig()),
		payloads: make(map[string][]byte),
		owners:   make(map[string]string),
	}
	var sensitiveScorer multi_dimensional_retrieval.SensitiveScorer
	if len(scorer) > 0 {
		sensitiveScorer = scorer[0]
	}
	s.assessor = multi_dimensional_retrieval.NewRiskAssessor(sensitiveScorer)
	if cfg.TieredLifecycleEnabled {
		tier, err := NewTieredLifecycleService(cfg, s.payload)
		if err != nil {
			return nil, err
		}
		s.tier = tier
		tier.Start()
	}
	return s, nil
}

func (s *CompetitionCapabilityService) payload(id string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	payload, ok := s.payloads[id]
	if !ok {
		return nil, fmt.Errorf("competition capabilities: payload %q not found", id)
	}
	return append([]byte(nil), payload...), nil
}

func (s *CompetitionCapabilityService) AddEpisode(ep *memory_subgraph.EpisodicNode) error {
	if ep == nil || strings.TrimSpace(ep.ID) == "" || strings.TrimSpace(ep.OwnerID) == "" {
		return fmt.Errorf("competition capabilities: episode ID and owner are required")
	}
	if ep.Timestamp.IsZero() {
		ep.Timestamp = time.Now().UTC()
	}
	if err := s.graph.AddEpisode(ep); err != nil {
		return err
	}
	s.mu.Lock()
	s.payloads[ep.ID] = []byte(ep.Content)
	s.owners[ep.ID] = ep.OwnerID
	s.mu.Unlock()
	if s.tier != nil {
		if err := s.tier.Register(ep.ID, int64(len([]byte(ep.Content)))); err != nil {
			return err
		}
	}
	return nil
}

func (s *CompetitionCapabilityService) AddEntity(entity *memory_subgraph.SemanticEntity) error {
	if entity == nil || strings.TrimSpace(entity.OwnerID) == "" {
		return fmt.Errorf("competition capabilities: entity owner is required")
	}
	for _, episodeID := range entity.Episodes {
		s.mu.RLock()
		owner, exists := s.owners[episodeID]
		s.mu.RUnlock()
		if !exists || owner != entity.OwnerID {
			return fmt.Errorf("competition capabilities: entity evidence must belong to its owner")
		}
	}
	return s.graph.AddEntity(entity)
}

func (s *CompetitionCapabilityService) AddEdge(edge *memory_subgraph.FactEdge) error {
	if edge == nil || strings.TrimSpace(edge.OwnerID) == "" {
		return fmt.Errorf("competition capabilities: edge owner is required")
	}
	for _, episodeID := range edge.Episodes {
		s.mu.RLock()
		owner, exists := s.owners[episodeID]
		s.mu.RUnlock()
		if !exists || owner != edge.OwnerID {
			return fmt.Errorf("competition capabilities: edge evidence must belong to its owner")
		}
	}
	return s.graph.AddFactEdge(edge)
}

func (s *CompetitionCapabilityService) Communities(owner string) ([]*memory_subgraph.Community, error) {
	if strings.TrimSpace(owner) == "" {
		return nil, fmt.Errorf("competition capabilities: owner is required")
	}
	communities, err := s.graph.BuildCommunities()
	if err != nil {
		return nil, err
	}
	out := make([]*memory_subgraph.Community, 0, len(communities))
	for _, community := range communities {
		if community.OwnerID == owner {
			out = append(out, community)
		}
	}
	return out, nil
}

func (s *CompetitionCapabilityService) Rank(owner string, inputs []RiskRankInput) ([]multi_dimensional_retrieval.RankedCandidate, error) {
	if strings.TrimSpace(owner) == "" {
		return nil, fmt.Errorf("competition capabilities: owner is required")
	}
	ranked := make([]multi_dimensional_retrieval.RankedCandidate, 0, len(inputs))
	for _, input := range inputs {
		if strings.TrimSpace(input.ID) == "" {
			return nil, fmt.Errorf("competition capabilities: candidate ID is required")
		}
		s.mu.RLock()
		candidateOwner, known := s.owners[input.ID]
		payload := append([]byte(nil), s.payloads[input.ID]...)
		s.mu.RUnlock()
		if !known || candidateOwner != owner {
			return nil, fmt.Errorf("competition capabilities: candidate is not owner-scoped evidence")
		}
		scores := multi_dimensional_retrieval.DimensionScores{Semantic: input.Semantic, Temporal: input.Temporal, Graph: input.Graph, Structured: input.Structural}
		sensitive := 0.0
		if s.assessor.SensitiveScorer != nil {
			sensitive = s.assessor.SensitiveScorer(string(payload))
			if sensitive < 0 || sensitive != sensitive {
				sensitive = 0
			} else if sensitive > 1 {
				sensitive = 1
			}
		}
		breakdown, err := s.assessor.Assess(input.ID, string(payload), scores, input.ContextRisk, input.PrivacyCost, input.SourceRisk)
		if err != nil {
			return nil, fmt.Errorf("competition capabilities: score candidate: %w", err)
		}
		ranked = append(ranked, multi_dimensional_retrieval.RankedCandidate{
			Candidate: multi_dimensional_retrieval.Candidate{
				ID: input.ID, Scores: scores,
				Risk: multi_dimensional_retrieval.RiskSignals{Context: input.ContextRisk, Sensitive: sensitive, PrivacyCost: input.PrivacyCost, SourceUntrust: input.SourceRisk},
			},
			Breakdown: breakdown,
		})
	}
	if s.tier != nil {
		for _, input := range inputs {
			if err := s.tier.Touch(input.ID); err != nil {
				return nil, fmt.Errorf("competition capabilities: register lifecycle access: %w", err)
			}
		}
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].Breakdown.Composite == ranked[j].Breakdown.Composite {
			return ranked[i].ID < ranked[j].ID
		}
		return ranked[i].Breakdown.Composite > ranked[j].Breakdown.Composite
	})
	for index := range ranked {
		ranked[index].Rank = index + 1
	}
	return ranked, nil
}

func (s *CompetitionCapabilityService) LifecycleEnabled() bool { return s.tier != nil }

func (s *CompetitionCapabilityService) SensitiveScorerConfigured() bool {
	return s != nil && s.assessor != nil && s.assessor.SensitiveScorerConfigured()
}

func (s *CompetitionCapabilityService) LifecycleStatus() LifecycleStatus {
	status := LifecycleStatus{Implementation: "in_process_evidence_lifecycle"}
	if s == nil || s.tier == nil {
		return status
	}
	status.Enabled = true
	status.Running = s.tier.IsRunning()
	status.LastCycle = s.tier.LastStats()
	status.Applied, status.Failed = s.tier.Totals()
	return status
}

func (s *CompetitionCapabilityService) Stop() {
	if s.tier != nil {
		s.tier.Stop()
	}
}
