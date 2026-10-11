package api

import (
	"net/http"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

var traceIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)

// TraceEvidence is the redacted, server-owned view used by the black-box
// page. It intentionally contains no request text, model prompts, JWTs, or
// credentials. The store is bounded because it is an evidence display cache,
// not the source of record for compliance logs.
type TraceEvidence struct {
	TraceID     string                 `json:"trace_id"`
	UserID      string                 `json:"-"`
	Stage       string                 `json:"stage"`
	Status      string                 `json:"status"`
	StartedAt   time.Time              `json:"started_at"`
	CompletedAt time.Time              `json:"completed_at"`
	LatencyMS   int64                  `json:"latency_ms"`
	Evidence    map[string]interface{} `json:"evidence"`
}

type traceEvidenceCache struct {
	mu      sync.RWMutex
	entries map[string]TraceEvidence
}

const maxTraceEvidenceEntries = 512

var competitionTraceEvidence = &traceEvidenceCache{entries: make(map[string]TraceEvidence)}

func rememberTraceEvidence(c *gin.Context, stage, status string, started, completed time.Time, evidence map[string]interface{}) {
	if c == nil {
		return
	}
	traceID := competitionTraceID(c)
	if !traceIDPattern.MatchString(traceID) {
		return
	}
	userID, _ := c.Get("user_id")
	owner, _ := userID.(string)
	entry := TraceEvidence{TraceID: traceID, UserID: owner, Stage: stage, Status: status, StartedAt: started.UTC(), CompletedAt: completed.UTC(), Evidence: evidence}
	entry.LatencyMS = completed.Sub(started).Milliseconds()
	if entry.LatencyMS < 0 {
		entry.LatencyMS = 0
	}
	competitionTraceEvidence.mu.Lock()
	if len(competitionTraceEvidence.entries) >= maxTraceEvidenceEntries {
		keys := make([]string, 0, len(competitionTraceEvidence.entries))
		for key := range competitionTraceEvidence.entries {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		delete(competitionTraceEvidence.entries, keys[0])
	}
	competitionTraceEvidence.entries[traceID] = entry
	competitionTraceEvidence.mu.Unlock()
}

func (h *Handler) RegisterAuditTraceRoutes(group *gin.RouterGroup) {
	group.GET("/v1/audit/traces/:trace_id", h.HandleAuditTrace)
}

func (h *Handler) HandleAuditTrace(c *gin.Context) {
	traceID := c.Param("trace_id")
	if !traceIDPattern.MatchString(traceID) {
		writeCompetitionError(c, http.StatusBadRequest, "audit_trace", "invalid trace_id")
		return
	}
	ownerValue, ok := c.Get("user_id")
	owner, ownerOK := ownerValue.(string)
	if !ok || !ownerOK || owner == "" {
		writeCompetitionError(c, http.StatusUnauthorized, "audit_trace", "missing authenticated user identity")
		return
	}
	competitionTraceEvidence.mu.RLock()
	entry, found := competitionTraceEvidence.entries[traceID]
	competitionTraceEvidence.mu.RUnlock()
	// Do not distinguish an unknown trace from another user's trace.
	if !found || entry.UserID != owner {
		writeCompetitionError(c, http.StatusNotFound, "audit_trace", "trace evidence is unavailable")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"trace_id": entry.TraceID,
		"stage":    "audit_trace",
		"status":   "ok",
		"data":     entry,
	})
}
