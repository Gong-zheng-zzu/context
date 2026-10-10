package services

import (
	"context"
	"errors"
	"testing"

	"github.com/contextkeeper/service/internal/models"
)

type failingVectorStore struct{}

func (failingVectorStore) SearchByText(context.Context, string, *models.SearchOptions) ([]models.SearchResult, error) {
	return nil, errors.New("embedding unavailable")
}

func TestVectorStoreAdapterPreservesSearchFailureForAudit(t *testing.T) {
	adapter := &VectorStoreAdapter{Engine: failingVectorStore{}}
	_, err := adapter.SearchByQuery(context.Background(), "血压", 5)
	if err == nil {
		t.Fatal("SearchByQuery() error = nil, want the source failure preserved")
	}
	if got := err.Error(); got != "vector search failed: embedding unavailable" {
		t.Fatalf("SearchByQuery() error = %q, want wrapped source error", got)
	}
}

func TestVectorStoreAdapterWithoutEngineIsExplicitlyEmpty(t *testing.T) {
	adapter := &VectorStoreAdapter{}
	results, err := adapter.SearchByQuery(context.Background(), "血压", 5)
	if err != nil {
		t.Fatalf("SearchByQuery() error = %v, want nil for an intentionally disabled lane", err)
	}
	if len(results) != 0 {
		t.Fatalf("SearchByQuery() returned %d results, want empty", len(results))
	}
}
