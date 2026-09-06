package services

import (
	"context"
	"errors"
	"testing"

	"github.com/contextkeeper/service/internal/models"
)

type unlearningTestVectorStore struct {
	countErr          error
	count             int
	defaultCollection string
	searchResults     []models.SearchResult
}

func (s *unlearningTestVectorStore) GenerateEmbedding(string) ([]float32, error) {
	return []float32{0.1}, nil
}
func (s *unlearningTestVectorStore) GetEmbeddingDimension() int         { return 1 }
func (s *unlearningTestVectorStore) StoreMemory(*models.Memory) error   { return nil }
func (s *unlearningTestVectorStore) StoreMessage(*models.Message) error { return nil }
func (s *unlearningTestVectorStore) CountMemories(string) (int, error)  { return 0, nil }
func (s *unlearningTestVectorStore) StoreEnhancedMemory(*models.EnhancedMemory) error {
	return nil
}
func (s *unlearningTestVectorStore) StoreEnhancedMessage(*models.EnhancedMessage) error {
	return nil
}
func (s *unlearningTestVectorStore) SearchByVector(context.Context, []float32, *models.SearchOptions) ([]models.SearchResult, error) {
	return s.searchResults, nil
}
func (s *unlearningTestVectorStore) SearchByText(context.Context, string, *models.SearchOptions) ([]models.SearchResult, error) {
	return s.searchResults, nil
}
func (s *unlearningTestVectorStore) SearchByID(context.Context, string, *models.SearchOptions) ([]models.SearchResult, error) {
	return nil, nil
}
func (s *unlearningTestVectorStore) SearchByFilter(context.Context, string, *models.SearchOptions) ([]models.SearchResult, error) {
	return nil, nil
}
func (s *unlearningTestVectorStore) EnsureCollection(string) error { return nil }
func (s *unlearningTestVectorStore) CreateCollection(string, *models.CollectionConfig) error {
	return nil
}
func (s *unlearningTestVectorStore) DeleteCollection(string) error         { return nil }
func (s *unlearningTestVectorStore) CollectionExists(string) (bool, error) { return true, nil }
func (s *unlearningTestVectorStore) StoreUserInfo(*models.UserInfo) error  { return nil }
func (s *unlearningTestVectorStore) GetUserInfo(string) (*models.UserInfo, error) {
	return nil, nil
}
func (s *unlearningTestVectorStore) CheckUserExists(string) (bool, error) { return true, nil }
func (s *unlearningTestVectorStore) InitUserStorage() error               { return nil }
func (s *unlearningTestVectorStore) UpdateVector(context.Context, *models.VectorUpdateRequest) error {
	return nil
}
func (s *unlearningTestVectorStore) BatchUpdateVectors(context.Context, *models.BatchVectorUpdateRequest) error {
	return nil
}
func (s *unlearningTestVectorStore) GetVectorByID(context.Context, string, string) (*models.VectorRecord, error) {
	return nil, nil
}
func (s *unlearningTestVectorStore) GetVectorsByUserID(context.Context, string, string) ([]*models.VectorRecord, error) {
	return nil, nil
}
func (s *unlearningTestVectorStore) DeleteVectors(context.Context, string, []string) error {
	return nil
}
func (s *unlearningTestVectorStore) CountVectorsByUserID(context.Context, string, string) (int, error) {
	return s.count, s.countErr
}
func (s *unlearningTestVectorStore) GetProvider() models.VectorStoreType {
	return models.VectorStoreTypeQdrant
}
func (s *unlearningTestVectorStore) DefaultCollection() string { return s.defaultCollection }

func TestVerifyUnlearningPropagatesVectorStoreError(t *testing.T) {
	store := &unlearningTestVectorStore{countErr: errors.New("qdrant unavailable"), defaultCollection: "nursing_records"}
	service := NewMachineUnlearningService(store, nil, nil)

	result, err := service.VerifyUnlearning(context.Background(), &models.UnlearningVerifyRequest{UserID: "user-a"})
	if err == nil {
		t.Fatalf("VerifyUnlearning() result = %#v, want storage error", result)
	}
}

func TestVerifyUnlearningDoesNotTreatOtherUsersAsForgottenData(t *testing.T) {
	store := &unlearningTestVectorStore{
		defaultCollection: "nursing_records",
		searchResults: []models.SearchResult{{
			ID:     "retained-record",
			Score:  0.99,
			Fields: map[string]interface{}{"user_id": "other-user"},
		}},
	}
	service := NewMachineUnlearningService(store, nil, nil)

	result, err := service.VerifyUnlearning(context.Background(), &models.UnlearningVerifyRequest{UserID: "forgotten-user", QueryText: "sensitive query"})
	if err != nil || !result.IsFullyUnlearned {
		t.Fatalf("VerifyUnlearning() = %#v, %v; retained-user result must not fail the target-user probe", result, err)
	}
}
