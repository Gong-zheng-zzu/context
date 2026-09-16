package api

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/contextkeeper/service/internal/engines/causal_reasoning"
	"github.com/contextkeeper/service/internal/middleware"
	"github.com/contextkeeper/service/internal/utils"
	"github.com/gin-gonic/gin"
)

type CausalReviewRequest struct {
	RecordID       string                          `json:"record_id" binding:"required,max=128"`
	SessionID      string                          `json:"session_id" binding:"required,max=128"`
	SourceText     string                          `json:"source_text" binding:"required,max=8192"`
	Synthetic      bool                            `json:"synthetic"`
	DatasetVersion string                          `json:"dataset_version" binding:"required,max=128"`
	RuleVersion    string                          `json:"rule_version" binding:"required,max=128"`
	ModelVersion   string                          `json:"model_version" binding:"required,max=128"`
	Predicted      causal_reasoning.CausalRelation `json:"predicted"`
	Corrected      causal_reasoning.CausalRelation `json:"corrected"`
	Issues         []string                        `json:"issues"`
}

type CausalReviewRecord struct {
	ReviewID       string                          `json:"review_id"`
	TraceID        string                          `json:"trace_id"`
	RecordID       string                          `json:"record_id"`
	SessionID      string                          `json:"session_id"`
	ReviewerID     string                          `json:"reviewer_id"`
	SourceText     string                          `json:"source_text,omitempty"`
	SourceSHA256   string                          `json:"source_sha256"`
	Synthetic      bool                            `json:"synthetic"`
	DatasetVersion string                          `json:"dataset_version"`
	RuleVersion    string                          `json:"rule_version"`
	ModelVersion   string                          `json:"model_version"`
	Predicted      causal_reasoning.CausalRelation `json:"predicted"`
	Corrected      causal_reasoning.CausalRelation `json:"corrected"`
	Issues         []string                        `json:"issues"`
	Status         string                          `json:"status"`
	CreatedAt      time.Time                       `json:"created_at"`
}

type CausalReviewStore interface {
	Append(CausalReviewRecord) error
	List() ([]CausalReviewRecord, error)
}

type FileCausalReviewStore struct {
	path string
	mu   sync.Mutex
}

func NewFileCausalReviewStore(path string) *FileCausalReviewStore {
	return &FileCausalReviewStore{path: path}
}

func (s *FileCausalReviewStore) Append(record CausalReviewRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	file, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = file.Write(append(payload, '\n'))
	return err
}

func (s *FileCausalReviewStore) List() ([]CausalReviewRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := os.Open(s.path)
	if os.IsNotExist(err) {
		return []CausalReviewRecord{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var records []CausalReviewRecord
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var record CausalReviewRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, err
		}
		record.SourceText = ""
		records = append(records, record)
	}
	return records, scanner.Err()
}

func (h *CausalReasoningHandler) CreateCausalReview(c *gin.Context) {
	if !causalReviewAuthorized(c) {
		writeCompetitionError(c, http.StatusForbidden, "causal_review", "causal review requires doctor or evaluation scope")
		return
	}
	var req CausalReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeCompetitionError(c, http.StatusBadRequest, "causal_review", "invalid review request")
		return
	}
	if !req.Synthetic || strings.TrimSpace(req.SourceText) == "" {
		writeCompetitionError(c, http.StatusBadRequest, "causal_review", "only explicit synthetic review data is accepted")
		return
	}
	if req.SessionID != evaluationSessionID() {
		writeCompetitionError(c, http.StatusForbidden, "causal_review", "review session is outside the evaluation scope")
		return
	}
	if h.reviewStore == nil {
		writeCompetitionUnavailable(c, "causal_review", "causal review queue is not configured")
		return
	}
	userID, _ := middleware.GetUserID(c)
	createdAt := time.Now().UTC()
	sourceHash := fmt.Sprintf("%x", sha256.Sum256([]byte(req.SourceText)))
	idHash := sha256.Sum256([]byte(req.RecordID + "\x1f" + userID + "\x1f" + createdAt.Format(time.RFC3339Nano)))
	record := CausalReviewRecord{
		ReviewID: fmt.Sprintf("review-%x", idHash[:8]), TraceID: utils.GetTraceIDFromGin(c),
		RecordID: req.RecordID, SessionID: req.SessionID, ReviewerID: userID,
		SourceText: req.SourceText, SourceSHA256: sourceHash, Synthetic: true,
		DatasetVersion: req.DatasetVersion, RuleVersion: req.RuleVersion, ModelVersion: req.ModelVersion,
		Predicted: req.Predicted, Corrected: req.Corrected, Issues: req.Issues,
		Status: "pending_second_review", CreatedAt: createdAt,
	}
	if err := h.reviewStore.Append(record); err != nil {
		writeCompetitionError(c, http.StatusInternalServerError, "causal_review", "failed to append review")
		return
	}
	record.SourceText = ""
	c.JSON(http.StatusCreated, gin.H{"trace_id": record.TraceID, "stage": "causal_review", "status": "queued", "review": record, "training_applied": false})
}

func (h *CausalReasoningHandler) ListCausalReviews(c *gin.Context) {
	if !causalReviewAuthorized(c) {
		writeCompetitionError(c, http.StatusForbidden, "causal_review", "causal review requires doctor or evaluation scope")
		return
	}
	if h.reviewStore == nil {
		writeCompetitionUnavailable(c, "causal_review", "causal review queue is not configured")
		return
	}
	records, err := h.reviewStore.List()
	if err != nil {
		writeCompetitionError(c, http.StatusInternalServerError, "causal_review", "failed to read review queue")
		return
	}
	c.JSON(http.StatusOK, gin.H{"trace_id": competitionTraceID(c), "stage": "causal_review", "status": "ok", "reviews": records, "count": len(records)})
}

func causalReviewAuthorized(c *gin.Context) bool {
	role, _ := middleware.GetRole(c)
	userID, _ := middleware.GetUserID(c)
	return role == "doctor" || userID == evaluationUserID()
}

func evaluationUserID() string {
	if value := strings.TrimSpace(os.Getenv("EVAL_USER_ID")); value != "" {
		return value
	}
	return "eval_user_001"
}

func evaluationSessionID() string {
	if value := strings.TrimSpace(os.Getenv("EVAL_SESSION_ID")); value != "" {
		return value
	}
	return "eval_retrieval_test"
}
