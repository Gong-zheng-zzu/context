package api

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/contextkeeper/service/internal/engines/memory_subgraph"
	"github.com/contextkeeper/service/internal/services"
	"github.com/gin-gonic/gin"
)

const (
	competitionCapabilityStageEvidence  = "competition_evidence"
	competitionCapabilityStageRerank    = "risk_aware_rerank"
	competitionCapabilityStageLifecycle = "tiered_lifecycle"
)

// RegisterCompetitionCapabilityRoutes mounts routes below an already
// JWT-protected group. It does not apply authentication itself.
func (h *Handler) RegisterCompetitionCapabilityRoutes(group *gin.RouterGroup) {
	evidence := group.Group("/v1/evidence")
	evidence.POST("/episodes", h.HandleCompetitionEpisode)
	evidence.POST("/entities", h.HandleCompetitionEntity)
	evidence.POST("/edges", h.HandleCompetitionEdge)
	evidence.GET("/communities", h.HandleCompetitionCommunities)
	group.POST("/v1/retrieval/risk-rerank", h.HandleCompetitionRiskRerank)
	group.GET("/v1/lifecycle/status", h.HandleCompetitionLifecycleStatus)
}

type competitionEpisodeRequest struct {
	ID        string                        `json:"id" binding:"required,max=128"`
	Source    memory_subgraph.EpisodeSource `json:"source" binding:"required,max=16"`
	Content   string                        `json:"content" binding:"required,max=8192"`
	Timestamp *time.Time                    `json:"timestamp"`
	Numbers   map[string]float64            `json:"numbers"`
}

type competitionEntityRequest struct {
	ID         string                     `json:"id" binding:"required,max=128"`
	Name       string                     `json:"name" binding:"required,max=256"`
	Kind       memory_subgraph.EntityKind `json:"kind" binding:"required,max=32"`
	Episodes   []string                   `json:"episodes" binding:"required,min=1,max=50"`
	Confidence float64                    `json:"confidence"`
}

type competitionEdgeRequest struct {
	ID         string   `json:"id" binding:"required,max=128"`
	Source     string   `json:"source" binding:"required,max=128"`
	Target     string   `json:"target" binding:"required,max=128"`
	Relation   string   `json:"relation" binding:"required,max=128"`
	Weight     float64  `json:"weight"`
	Episodes   []string `json:"episodes" binding:"required,min=1,max=50"`
	Confidence float64  `json:"confidence"`
}

// competitionRiskRerankRequest accepts owner-scoped evidence IDs. It is not an
// RRF API and never accepts client-provided retrieval documents as evidence.
type competitionRiskRerankRequest struct {
	EvaluatedEvidence bool                     `json:"evaluated_evidence"`
	CandidateOrigin   string                   `json:"candidate_origin" binding:"required,max=64"`
	Candidates        []services.RiskRankInput `json:"candidates" binding:"required,min=1,max=50"`
}

func (h *Handler) HandleCompetitionEpisode(c *gin.Context) {
	owner, ok := competitionCapabilityOwner(c, competitionCapabilityStageEvidence)
	if !ok {
		return
	}
	var req competitionEpisodeRequest
	if err := c.ShouldBindJSON(&req); err != nil || !validEpisodeSource(req.Source) || strings.TrimSpace(req.Content) == "" {
		writeCompetitionError(c, http.StatusBadRequest, competitionCapabilityStageEvidence, "invalid evidence episode")
		return
	}
	service, ok := h.competitionCapabilityService(c, competitionCapabilityStageEvidence)
	if !ok {
		return
	}
	started := time.Now()
	episode := &memory_subgraph.EpisodicNode{ID: req.ID, OwnerID: owner, Source: req.Source, Content: req.Content, Numbers: req.Numbers}
	if req.Timestamp != nil {
		episode.Timestamp = req.Timestamp.UTC()
	}
	if err := service.AddEpisode(episode); err != nil {
		writeCompetitionError(c, http.StatusBadRequest, competitionCapabilityStageEvidence, "evidence episode was rejected")
		return
	}
	c.JSON(http.StatusCreated, competitionCapabilityResponse(c, competitionCapabilityStageEvidence, started, gin.H{
		"episode_id":        episode.ID,
		"source":            episode.Source,
		"timestamp":         episode.Timestamp,
		"content_sha256":    fmt.Sprintf("%x", sha256.Sum256([]byte(req.Content))),
		"lifecycle_enabled": service.LifecycleEnabled(),
	}))
}

func (h *Handler) HandleCompetitionEntity(c *gin.Context) {
	owner, ok := competitionCapabilityOwner(c, competitionCapabilityStageEvidence)
	if !ok {
		return
	}
	var req competitionEntityRequest
	if err := c.ShouldBindJSON(&req); err != nil || !validEntityKind(req.Kind) || len(req.Episodes) == 0 {
		writeCompetitionError(c, http.StatusBadRequest, competitionCapabilityStageEvidence, "invalid evidence entity")
		return
	}
	service, ok := h.competitionCapabilityService(c, competitionCapabilityStageEvidence)
	if !ok {
		return
	}
	started := time.Now()
	entity := &memory_subgraph.SemanticEntity{ID: req.ID, OwnerID: owner, Name: req.Name, Kind: req.Kind, Episodes: req.Episodes, Confidence: req.Confidence}
	if err := service.AddEntity(entity); err != nil {
		writeCompetitionError(c, http.StatusBadRequest, competitionCapabilityStageEvidence, "evidence entity was rejected")
		return
	}
	c.JSON(http.StatusCreated, competitionCapabilityResponse(c, competitionCapabilityStageEvidence, started, gin.H{
		"entity_id": entity.ID, "kind": entity.Kind, "episode_count": len(entity.Episodes),
	}))
}

func (h *Handler) HandleCompetitionEdge(c *gin.Context) {
	owner, ok := competitionCapabilityOwner(c, competitionCapabilityStageEvidence)
	if !ok {
		return
	}
	var req competitionEdgeRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Episodes) == 0 {
		writeCompetitionError(c, http.StatusBadRequest, competitionCapabilityStageEvidence, "invalid evidence edge")
		return
	}
	service, ok := h.competitionCapabilityService(c, competitionCapabilityStageEvidence)
	if !ok {
		return
	}
	started := time.Now()
	edge := &memory_subgraph.FactEdge{ID: req.ID, OwnerID: owner, Source: req.Source, Target: req.Target, Relation: req.Relation, Weight: req.Weight, Episodes: req.Episodes, Confidence: req.Confidence}
	if err := service.AddEdge(edge); err != nil {
		writeCompetitionError(c, http.StatusBadRequest, competitionCapabilityStageEvidence, "evidence edge was rejected")
		return
	}
	c.JSON(http.StatusCreated, competitionCapabilityResponse(c, competitionCapabilityStageEvidence, started, gin.H{
		"edge_id": edge.ID, "source": edge.Source, "target": edge.Target, "relation": edge.Relation,
		"provenance_episode_count": len(edge.Episodes),
	}))
}

func (h *Handler) HandleCompetitionCommunities(c *gin.Context) {
	owner, ok := competitionCapabilityOwner(c, competitionCapabilityStageEvidence)
	if !ok {
		return
	}
	service, ok := h.competitionCapabilityService(c, competitionCapabilityStageEvidence)
	if !ok {
		return
	}
	started := time.Now()
	communities, err := service.Communities(owner)
	if err != nil {
		if strings.Contains(err.Error(), "no evidence communities") {
			communities = []*memory_subgraph.Community{}
		} else {
			writeCompetitionError(c, http.StatusBadRequest, competitionCapabilityStageEvidence, "could not build evidence communities")
			return
		}
	}
	c.JSON(http.StatusOK, competitionCapabilityResponse(c, competitionCapabilityStageEvidence, started, gin.H{
		"communities": communities, "count": len(communities), "owner_scope": "authenticated_user",
	}))
}

func (h *Handler) HandleCompetitionRiskRerank(c *gin.Context) {
	owner, ok := competitionCapabilityOwner(c, competitionCapabilityStageRerank)
	if !ok {
		return
	}
	var req competitionRiskRerankRequest
	if err := c.ShouldBindJSON(&req); err != nil || !req.EvaluatedEvidence || req.CandidateOrigin != "owner_scoped_evidence" {
		writeCompetitionError(c, http.StatusBadRequest, competitionCapabilityStageRerank, "only owner-scoped evidence can be risk ranked")
		return
	}
	service, ok := h.competitionCapabilityService(c, competitionCapabilityStageRerank)
	if !ok {
		return
	}
	started := time.Now()
	ranked, err := service.Rank(owner, req.Candidates)
	if err != nil {
		writeCompetitionError(c, http.StatusBadRequest, competitionCapabilityStageRerank, "risk rerank input was rejected")
		return
	}
	c.JSON(http.StatusOK, competitionCapabilityResponse(c, competitionCapabilityStageRerank, started, gin.H{
		"ranked_candidates":  ranked,
		"candidate_origin":   req.CandidateOrigin,
		"status":             "risk_rerank_evidence_only",
		"rrf_invoked":        false,
		"not_rrf_substitute": true,
		"contract":           "risk-aware ordering for authenticated owner-scoped evidence only",
	}))
}

func (h *Handler) HandleCompetitionLifecycleStatus(c *gin.Context) {
	if _, ok := competitionCapabilityOwner(c, competitionCapabilityStageLifecycle); !ok {
		return
	}
	service, ok := h.competitionCapabilityService(c, competitionCapabilityStageLifecycle)
	if !ok {
		return
	}
	status := service.LifecycleStatus()
	started := time.Now()
	traceID := competitionTraceID(c)
	completed := time.Now().UTC()
	rememberTraceEvidence(c, competitionCapabilityStageLifecycle, "ok", started, completed, gin.H{
		"implementation":              status.Implementation,
		"lifecycle_status":            status,
		"sensitive_scorer_configured": service.SensitiveScorerConfigured(),
		"deletion_integration":        false,
	})
	c.JSON(http.StatusOK, competitionCapabilityResponse(c, competitionCapabilityStageLifecycle, time.Now(), gin.H{
		"trace_id":                    traceID,
		"status":                      status,
		"implementation":              status.Implementation,
		"sensitive_scorer_configured": service.SensitiveScorerConfigured(),
		"deletion_integration":        false,
	}))
}

func (h *Handler) competitionCapabilityService(c *gin.Context, stage string) (*services.CompetitionCapabilityService, bool) {
	if h == nil || h.competitionCapabilities == nil {
		writeCompetitionUnavailable(c, stage, "competition capability service is not configured")
		return nil, false
	}
	return h.competitionCapabilities, true
}

func competitionCapabilityOwner(c *gin.Context, stage string) (string, bool) {
	value, exists := c.Get("user_id")
	owner, valid := value.(string)
	if !exists || !valid || strings.TrimSpace(owner) == "" {
		writeCompetitionError(c, http.StatusUnauthorized, stage, "missing authenticated user identity")
		return "", false
	}
	return owner, true
}

func competitionCapabilityResponse(c *gin.Context, stage string, started time.Time, data gin.H) gin.H {
	return gin.H{
		"success": true, "trace_id": competitionTraceID(c), "stage": stage, "status": "ok",
		"latency_ms": time.Since(started).Milliseconds(), "data": data,
	}
}

func validEpisodeSource(source memory_subgraph.EpisodeSource) bool {
	switch source {
	case memory_subgraph.SourceVoice, memory_subgraph.SourceText, memory_subgraph.SourceSensor, memory_subgraph.SourceImage:
		return true
	default:
		return false
	}
}

func validEntityKind(kind memory_subgraph.EntityKind) bool {
	switch kind {
	case memory_subgraph.EntityElder, memory_subgraph.EntityDepartment, memory_subgraph.EntityLesion, memory_subgraph.EntityDrug, memory_subgraph.EntitySymptom, memory_subgraph.EntityDisease, memory_subgraph.EntityCareAction, memory_subgraph.EntityOther:
		return true
	default:
		return false
	}
}
