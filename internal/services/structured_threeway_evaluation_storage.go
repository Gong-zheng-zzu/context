package services

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/contextkeeper/service/internal/engines/multi_dimensional_retrieval/knowledge"
	"github.com/contextkeeper/service/internal/engines/multi_dimensional_retrieval/timeline"
	"github.com/contextkeeper/service/internal/models"
	"github.com/google/uuid"
)

const (
	structuredThreeWayEnabledEnv = "EXPERIMENT_STRUCTURED_THREEWAY_ENABLED"
	structuredEvaluationUserID   = "eval_user_001"
	structuredEvaluationSession  = "eval_retrieval_test"
	structuredEvaluationControl  = "eval_unlearning_control"
	structuredEvaluationSchema   = "structured-threeway-v1"
)

// isStructuredThreeWayEvaluationRequest deliberately has a narrow scope. It
// prevents deterministic corpus metadata from becoming a general production
// shortcut around the normal LLM-driven ingestion path.
func isStructuredThreeWayEvaluationRequest(req models.StoreContextRequest) bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv(structuredThreeWayEnabledEnv)), "true") &&
		req.UserID == structuredEvaluationUserID &&
		isStructuredThreeWayEvaluationSession(req.SessionID) &&
		structuredMetadataString(req.Metadata, "seed_schema") == structuredEvaluationSchema &&
		strings.TrimSpace(structuredMetadataString(req.Metadata, "doc_id")) != ""
}

func isStructuredThreeWayEvaluationSession(sessionID string) bool {
	return sessionID == structuredEvaluationSession || sessionID == structuredEvaluationControl
}

// executeStructuredThreeWayEvaluationStorage writes the fixed, pre-annotated
// corpus to the same vector, timeline, and Neo4j engines used by retrieval. It
// performs no LLM extraction: timestamps and graph facts originate from the
// immutable evaluation corpus and are recorded through source doc_id fields.
func (s *ContextService) executeStructuredThreeWayEvaluationStorage(ctx context.Context, req models.StoreContextRequest) (string, error) {
	docID := structuredMetadataString(req.Metadata, "doc_id")
	if docID == "" {
		return "", fmt.Errorf("structured three-way seed requires metadata.doc_id")
	}

	// The vector lane retains normal embedding/storage behavior and security
	// scanning. Any downstream lane failure returns an error, so callers cannot
	// report a document as fully ingested without independent verification.
	memoryID, err := s.executeOriginalStorage(ctx, req)
	if err != nil {
		return "", fmt.Errorf("structured vector storage: %w", err)
	}

	workspace := structuredMetadataString(req.Metadata, "workspace")
	if workspace == "" {
		workspace = "default"
	}
	timestamp, err := structuredMetadataTime(req.Metadata, "timestamp")
	if err != nil {
		return "", fmt.Errorf("structured timeline timestamp for %s: %w", docID, err)
	}
	tags := structuredMetadataStrings(req.Metadata, "tags")
	eventType := structuredMetadataString(req.Metadata, "event_type")
	if eventType == "" {
		eventType = "nursing_record"
	}
	severity := structuredMetadataString(req.Metadata, "severity")
	if severity == "" {
		severity = "unknown"
	}
	patient := structuredMetadataString(req.Metadata, "patient_name")
	if patient == "" {
		patient = structuredMetadataString(req.Metadata, "patient_id")
	}
	if patient == "" {
		return "", fmt.Errorf("structured three-way seed requires patient_name or patient_id for %s", docID)
	}

	if err := s.storeStructuredTimelineEvent(ctx, req, memoryID, docID, workspace, timestamp, eventType, severity, tags); err != nil {
		return "", err
	}
	if err := s.storeStructuredKnowledgeGraph(ctx, req, memoryID, docID, workspace, timestamp, eventType, severity, patient, tags); err != nil {
		return "", err
	}

	return memoryID, nil
}

func (s *ContextService) storeStructuredTimelineEvent(ctx context.Context, req models.StoreContextRequest, memoryID, docID, workspace string, timestamp time.Time, eventType, severity string, tags []string) error {
	cfg := s.getTimescaleDBConfig()
	if cfg == nil {
		return fmt.Errorf("TimescaleDB is not enabled for structured three-way seed")
	}
	engine, err := s.createTimescaleDBEngine(cfg)
	if err != nil {
		return fmt.Errorf("create TimescaleDB engine: %w", err)
	}
	defer engine.Close()

	summary := s.simpleTitle(req.Content)
	intent := eventType
	event := &timeline.TimelineEvent{
		ID:              deterministicStructuredID("timeline", req.UserID, req.SessionID, docID),
		SourceDocID:     docID,
		UserID:          req.UserID,
		SessionID:       req.SessionID,
		WorkspaceID:     workspace,
		Timestamp:       timestamp,
		EventType:       eventType,
		Title:           eventType + ": " + severity,
		Content:         req.Content,
		Summary:         &summary,
		Intent:          &intent,
		Keywords:        tags,
		Categories:      []string{eventType, severity},
		ImportanceScore: 0.8,
		RelevanceScore:  1.0,
	}
	if _, err := engine.StoreEvent(ctx, event); err != nil {
		return fmt.Errorf("store TimescaleDB event for %s: %w", docID, err)
	}
	return nil
}

func (s *ContextService) storeStructuredKnowledgeGraph(ctx context.Context, req models.StoreContextRequest, memoryID, docID, workspace string, timestamp time.Time, eventType, severity, patient string, tags []string) error {
	cfg := s.getNeo4jConfig()
	if cfg == nil {
		return fmt.Errorf("Neo4j is not enabled for structured three-way seed")
	}
	engine, err := s.createNeo4jEngine(cfg)
	if err != nil {
		return fmt.Errorf("create Neo4j engine: %w", err)
	}
	defer engine.Close(ctx)

	patientID := deterministicStructuredID("patient", req.UserID, req.SessionID, patient)
	eventID := deterministicStructuredID("event", req.UserID, req.SessionID, docID)
	entities := []*knowledge.Entity{{
		ID: patientID, Name: patient, Type: knowledge.EntityTypePerson,
		Description: "Structured nursing-record patient", Workspace: workspace,
		MemoryIDs: []string{memoryID}, DocIDs: []string{docID}, UserID: req.UserID, SessionID: req.SessionID,
		CreatedAt: timestamp, UpdatedAt: timestamp,
	}}
	relations := make([]*knowledge.Relation, 0, len(tags)+1)
	for _, tag := range tags {
		tagID := deterministicStructuredID("tag", req.UserID, req.SessionID, tag)
		entities = append(entities, &knowledge.Entity{
			ID: tagID, Name: tag, Type: knowledge.EntityTypeConcept,
			Description: "Structured nursing-record tag", Workspace: workspace,
			MemoryIDs: []string{memoryID}, DocIDs: []string{docID}, UserID: req.UserID, SessionID: req.SessionID,
			CreatedAt: timestamp, UpdatedAt: timestamp,
		})
		relations = append(relations, &knowledge.Relation{
			SourceID: eventID, TargetID: tagID, Type: knowledge.RelationRelatesTo, Weight: 1.0,
			DocIDs: []string{docID}, UserID: req.UserID, SessionID: req.SessionID, Workspace: workspace, CreatedAt: timestamp,
		})
	}
	events := []*knowledge.Event{{
		ID: eventID, Name: eventType, Type: knowledge.EventTypeIssue,
		Description: "Severity=" + severity + "; " + s.simpleTitle(req.Content), Workspace: workspace,
		MemoryIDs: []string{memoryID}, DocIDs: []string{docID}, UserID: req.UserID, SessionID: req.SessionID,
		CreatedAt: timestamp, UpdatedAt: timestamp,
	}}
	relations = append(relations, &knowledge.Relation{
		SourceID: patientID, TargetID: eventID, Type: knowledge.RelationMentions, Weight: 1.0,
		DocIDs: []string{docID}, UserID: req.UserID, SessionID: req.SessionID, Workspace: workspace, CreatedAt: timestamp,
	})
	if err := engine.BatchUpsertEntities(ctx, entities, memoryID); err != nil {
		return fmt.Errorf("upsert Neo4j entities for %s: %w", docID, err)
	}
	if err := engine.BatchUpsertEvents(ctx, events, memoryID); err != nil {
		return fmt.Errorf("upsert Neo4j event for %s: %w", docID, err)
	}
	for _, relation := range relations {
		if err := engine.CreateRelation(ctx, relation); err != nil {
			return fmt.Errorf("create Neo4j relation for %s: %w", docID, err)
		}
	}
	return nil
}

func deterministicStructuredID(kind string, values ...string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(kind+"\x1f"+strings.Join(values, "\x1f"))).String()
}

func structuredMetadataString(metadata map[string]interface{}, key string) string {
	if metadata == nil {
		return ""
	}
	value, _ := metadata[key].(string)
	return strings.TrimSpace(value)
}

func structuredMetadataStrings(metadata map[string]interface{}, key string) []string {
	if metadata == nil {
		return nil
	}
	value, ok := metadata[key]
	if !ok {
		return nil
	}
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []interface{}:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				values = append(values, strings.TrimSpace(text))
			}
		}
		return values
	default:
		return nil
	}
}

func structuredMetadataTime(metadata map[string]interface{}, key string) (time.Time, error) {
	value := structuredMetadataString(metadata, key)
	if value == "" {
		return time.Time{}, fmt.Errorf("metadata.%s is required", key)
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err == nil {
		return parsed, nil
	}
	// The frozen nursing corpus uses ISO-8601 local timestamps without an
	// offset. Experiments interpret those source values as UTC deterministically
	// rather than injecting the host timezone into time-based retrieval.
	parsed, localErr := time.ParseInLocation("2006-01-02T15:04:05.999999", value, time.UTC)
	if localErr != nil {
		return time.Time{}, err
	}
	return parsed, nil
}
