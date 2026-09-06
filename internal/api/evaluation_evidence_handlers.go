package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/contextkeeper/service/internal/models"
	"github.com/gin-gonic/gin"
)

const (
	structuredThreeWayEvidenceUserID  = "eval_user_001"
	structuredThreeWayEvidenceSession = "eval_retrieval_test"
	structuredThreeWayEvidenceLimit   = 1000
)

// StructuredThreeWayEvidenceRequest is deliberately fixed to the audited
// evaluation corpus. This handler is intended for a JWT-protected route.
type StructuredThreeWayEvidenceRequest struct {
	UserID    string   `json:"user_id" binding:"required"`
	SessionID string   `json:"session_id" binding:"required"`
	DocIDs    []string `json:"doc_ids" binding:"required"`
}

type structuredThreeWayLaneEvidence struct {
	Verified bool   `json:"verified"`
	Count    int    `json:"count"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
}

type structuredThreeWayDocumentEvidence struct {
	DocID    string                         `json:"doc_id"`
	Vector   structuredThreeWayLaneEvidence `json:"vector"`
	Timeline structuredThreeWayLaneEvidence `json:"timeline"`
	Graph    structuredThreeWayLaneEvidence `json:"graph"`
}

// structuredThreeWayVectorCounter is deliberately narrower than the general
// vector-store interface. Qdrant implements it with one server-side predicate
// containing user_id, session_id, and doc_id.
type structuredThreeWayVectorCounter interface {
	DefaultCollection() string
	CountVectorsByUserSessionAndDocID(ctx context.Context, collectionName, userID, sessionID, docID string) (int, error)
}

// HandleStructuredThreeWayEvidence reports read-only, document-scoped
// persistence evidence for the fixed structured three-way evaluation corpus.
// It neither invokes ingestion nor changes sessions, vectors, timeline data,
// or graph data.
func (h *Handler) HandleStructuredThreeWayEvidence(c *gin.Context) {
	var req StructuredThreeWayEvidenceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "invalid request: " + err.Error()})
		return
	}
	if req.UserID != structuredThreeWayEvidenceUserID || req.SessionID != structuredThreeWayEvidenceSession {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "user_id and session_id are outside the structured evaluation scope"})
		return
	}

	authenticatedUserID, ok := c.Get("user_id")
	userID, stringOK := authenticatedUserID.(string)
	if !ok || !stringOK || strings.TrimSpace(userID) == "" {
		c.JSON(http.StatusUnauthorized, APIResponse{Success: false, Error: "missing authenticated user identity"})
		return
	}
	if userID != structuredThreeWayEvidenceUserID {
		c.JSON(http.StatusForbidden, APIResponse{Success: false, Error: "authenticated user is outside the structured evaluation scope"})
		return
	}

	docIDs, err := normalizedStructuredThreeWayDocIDs(req.DocIDs)
	if err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}

	vectorCounts, vectorError := h.readStructuredThreeWayVectorCounts(c, req.UserID, req.SessionID, docIDs)
	timelineCounts, timelineError := h.readStructuredThreeWayTimelineCounts(c, req.UserID, req.SessionID, docIDs)
	graphCounts, graphError := h.readStructuredThreeWayGraphCounts(c, req.UserID, req.SessionID, docIDs)
	documents := make([]structuredThreeWayDocumentEvidence, 0, len(docIDs))
	for _, docID := range docIDs {
		documents = append(documents, structuredThreeWayDocumentEvidence{
			DocID:    docID,
			Vector:   structuredThreeWayVectorLane(vectorCounts, vectorError, docID),
			Timeline: structuredThreeWayVectorLane(timelineCounts, timelineError, docID),
			Graph:    structuredThreeWayVectorLane(graphCounts, graphError, docID),
		})
	}

	c.JSON(http.StatusOK, APIResponse{Success: true, Data: gin.H{
		"user_id":    req.UserID,
		"session_id": req.SessionID,
		"read_only":  true,
		"documents":  documents,
	}})
}

func (h *Handler) readStructuredThreeWayTimelineCounts(c *gin.Context, userID, sessionID string, docIDs []string) (map[string]int, error) {
	if h == nil || h.contextService == nil || h.contextService.GetContextService() == nil {
		return nil, fmt.Errorf("context service is unavailable")
	}
	engine, err := h.contextService.GetContextService().GetTimelineEngine()
	if err != nil {
		return nil, fmt.Errorf("open timeline verification engine: %w", err)
	}
	defer engine.Close()
	counts := make(map[string]int, len(docIDs))
	for _, docID := range docIDs {
		count, err := engine.CountSessionDocumentRecords(c.Request.Context(), userID, sessionID, docID)
		if err != nil {
			return nil, err
		}
		counts[docID] = int(count)
	}
	return counts, nil
}

func (h *Handler) readStructuredThreeWayGraphCounts(c *gin.Context, userID, sessionID string, docIDs []string) (map[string]int, error) {
	if h == nil || h.contextService == nil || h.contextService.GetContextService() == nil {
		return nil, fmt.Errorf("context service is unavailable")
	}
	engine, err := h.contextService.GetContextService().GetKnowledgeEngine()
	if err != nil {
		return nil, fmt.Errorf("open graph verification engine: %w", err)
	}
	defer engine.Close(c.Request.Context())
	counts := make(map[string]int, len(docIDs))
	for _, docID := range docIDs {
		count, err := engine.CountSessionDocumentRecords(c.Request.Context(), userID, sessionID, docID)
		if err != nil {
			return nil, err
		}
		counts[docID] = int(count)
	}
	return counts, nil
}

func normalizedStructuredThreeWayDocIDs(docIDs []string) ([]string, error) {
	if len(docIDs) == 0 {
		return nil, fmt.Errorf("doc_ids is required")
	}
	if len(docIDs) > structuredThreeWayEvidenceLimit {
		return nil, fmt.Errorf("doc_ids exceeds the maximum of %d", structuredThreeWayEvidenceLimit)
	}

	seen := make(map[string]struct{}, len(docIDs))
	normalized := make([]string, 0, len(docIDs))
	for _, docID := range docIDs {
		docID = strings.TrimSpace(docID)
		if docID == "" {
			return nil, fmt.Errorf("doc_ids must not contain an empty value")
		}
		if _, exists := seen[docID]; exists {
			return nil, fmt.Errorf("doc_ids must not contain duplicates")
		}
		seen[docID] = struct{}{}
		normalized = append(normalized, docID)
	}
	return normalized, nil
}

func (h *Handler) readStructuredThreeWayVectorCounts(c *gin.Context, userID, sessionID string, docIDs []string) (map[string]int, error) {
	if h == nil || h.contextService == nil || h.contextService.GetContextService() == nil {
		return nil, fmt.Errorf("context service is unavailable")
	}

	contextService := h.contextService.GetContextService()
	if vectorStore := contextService.GetVectorStore(); vectorStore != nil {
		if counter, ok := vectorStore.(structuredThreeWayVectorCounter); ok {
			collectionName := counter.DefaultCollection()
			if strings.TrimSpace(collectionName) == "" {
				return nil, fmt.Errorf("vector collection is unavailable for verification")
			}
			counts := make(map[string]int, len(docIDs))
			for _, docID := range docIDs {
				count, err := counter.CountVectorsByUserSessionAndDocID(c.Request.Context(), collectionName, userID, sessionID, docID)
				if err != nil {
					return nil, fmt.Errorf("count vector record for %s: %w", docID, err)
				}
				counts[docID] = count
			}
			return counts, nil
		}
	}

	var (
		results []models.SearchResult
		err     error
	)
	if vectorStore := contextService.GetVectorStore(); vectorStore != nil {
		results, err = vectorStore.SearchByFilter(c.Request.Context(), `session_id="`+sessionID+`"`, &models.SearchOptions{
			Limit:         structuredThreeWayEvidenceLimit,
			SkipThreshold: true,
		})
	} else if h.vectorService != nil {
		results, err = h.vectorService.SearchBySessionID(sessionID, structuredThreeWayEvidenceLimit)
	} else {
		return nil, fmt.Errorf("vector storage is unavailable")
	}
	if err != nil {
		return nil, fmt.Errorf("read vector records: %w", err)
	}
	if len(results) >= structuredThreeWayEvidenceLimit {
		return nil, fmt.Errorf("vector query reached its limit; document counts are not complete")
	}

	requested := make(map[string]struct{}, len(docIDs))
	counts := make(map[string]int, len(docIDs))
	for _, docID := range docIDs {
		requested[docID] = struct{}{}
		counts[docID] = 0
	}
	for _, result := range results {
		if result.Fields == nil {
			continue
		}
		resultSessionID := structuredThreeWayFieldString(result.Fields, "session_id", "sessionId")
		if resultSessionID != sessionID {
			continue
		}
		resultUserID := structuredThreeWayFieldString(result.Fields, "user_id", "userId")
		if resultUserID == "" {
			return nil, fmt.Errorf("vector records do not expose user ownership required for verification")
		}
		if resultUserID != userID {
			continue
		}
		docID := structuredThreeWayVectorDocID(result)
		if _, wanted := requested[docID]; wanted {
			counts[docID]++
		}
	}
	return counts, nil
}

func structuredThreeWayVectorResultOwnedBy(result models.SearchResult, userID, sessionID string) bool {
	if result.Fields == nil {
		return false
	}
	return structuredThreeWayFieldString(result.Fields, "user_id", "userId") == userID &&
		structuredThreeWayFieldString(result.Fields, "session_id", "sessionId") == sessionID
}

func structuredThreeWayVectorDocID(result models.SearchResult) string {
	if result.Fields == nil {
		return ""
	}
	if docID := structuredThreeWayFieldString(result.Fields, "doc_id"); docID != "" {
		return docID
	}
	metadata, ok := structuredThreeWayMetadata(result.Fields["metadata"])
	if !ok {
		return ""
	}
	return structuredThreeWayFieldString(metadata, "doc_id")
}

func structuredThreeWayMetadata(value interface{}) (map[string]interface{}, bool) {
	switch typed := value.(type) {
	case map[string]interface{}:
		return typed, true
	case string:
		metadata := make(map[string]interface{})
		if err := json.Unmarshal([]byte(typed), &metadata); err != nil {
			return nil, false
		}
		return metadata, true
	default:
		return nil, false
	}
}

func structuredThreeWayFieldString(fields map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := fields[key].(string); ok {
			if value = strings.TrimSpace(value); value != "" {
				return value
			}
		}
	}
	return ""
}

func structuredThreeWayVectorLane(counts map[string]int, readErr error, docID string) structuredThreeWayLaneEvidence {
	if readErr != nil {
		return structuredThreeWayLaneEvidence{Status: "error", Error: readErr.Error()}
	}
	count := counts[docID]
	if count == 0 {
		return structuredThreeWayLaneEvidence{Status: "not_found"}
	}
	return structuredThreeWayLaneEvidence{Verified: true, Count: count, Status: "verified"}
}
