package services

import (
	"testing"

	"github.com/contextkeeper/service/internal/engines/multi_dimensional_retrieval/knowledge"
	"github.com/contextkeeper/service/internal/models"
)

func TestExtractKnowledgeNodesAddsDeterministicPatientEntityWithProvenance(t *testing.T) {
	service := &ContextService{}
	req := models.StoreContextRequest{
		UserID:    "user-1",
		SessionID: "session-1",
		Content:   "Patient Name: Ada Lovelace presented with a persistent cough.",
		Metadata: map[string]interface{}{
			"doc_id":    "clinical-note-42",
			"workspace": "clinical-eval",
		},
	}
	analysis := &models.SmartAnalysisResult{
		KnowledgeGraphExtraction: &models.KnowledgeGraphExtraction{
			Entities: []models.LLMExtractedEntity{{Title: "Persistent cough", Type: "concept"}},
		},
	}

	entities, _, _, _ := service.extractKnowledgeNodesFromAnalysis(analysis, req, "memory-1", KnowledgeNodeIDs{
		EntityNameToUUID: map[string]string{"Persistent cough": "existing-entity"},
	})

	patient := findKnowledgeEntity(entities, "Ada Lovelace")
	if patient == nil {
		t.Fatalf("patient entity missing from %#v", entities)
	}
	wantID := knowledge.GenerateEntityUUID("Ada Lovelace", knowledge.EntityTypePerson, "user-1\x1fsession-1\x1fclinical-eval")
	if patient.ID != wantID {
		t.Fatalf("patient ID = %q, want deterministic ID %q", patient.ID, wantID)
	}
	if patient.Type != knowledge.EntityTypePerson || patient.Workspace != "clinical-eval" {
		t.Fatalf("patient entity scope/type = %#v", patient)
	}
	if patient.UserID != "user-1" || patient.SessionID != "session-1" {
		t.Fatalf("patient tenancy provenance = %#v", patient)
	}
	if len(patient.DocIDs) != 1 || patient.DocIDs[0] != "clinical-note-42" {
		t.Fatalf("patient document provenance = %#v", patient.DocIDs)
	}
	if len(patient.MemoryIDs) != 1 || patient.MemoryIDs[0] != "memory-1" {
		t.Fatalf("patient memory provenance = %#v", patient.MemoryIDs)
	}

	entitiesAgain, _, _, _ := service.extractKnowledgeNodesFromAnalysis(analysis, req, "memory-2", KnowledgeNodeIDs{
		EntityNameToUUID: map[string]string{"Persistent cough": "existing-entity"},
	})
	if patientAgain := findKnowledgeEntity(entitiesAgain, "Ada Lovelace"); patientAgain == nil || patientAgain.ID != patient.ID {
		t.Fatalf("patient entity was not deterministic: %#v", patientAgain)
	}
}

func TestExtractPatientNameRejectsGenericPatientReference(t *testing.T) {
	name := extractPatientName(models.StoreContextRequest{Content: "The patient reported chest pain after exercise."})
	if name != "" {
		t.Fatalf("extractPatientName() = %q, want no patient entity", name)
	}
}

func findKnowledgeEntity(entities []*knowledge.Entity, name string) *knowledge.Entity {
	for _, entity := range entities {
		if entity.Name == name {
			return entity
		}
	}
	return nil
}
