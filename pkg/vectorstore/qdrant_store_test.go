package vectorstore

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/contextkeeper/service/internal/models"
)

func TestQdrantUnlearningOperationsUseRequestedCollection(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "POST /collections/target/points/scroll":
			_, _ = w.Write([]byte(`{"result":{"points":[{"id":"target-1","vector":[0.1,0.2],"payload":{"user_id":"forgotten","session_id":"s1","timestamp":10}}],"next_page_offset":null}}`))
		case "POST /collections/target/points/count":
			_, _ = w.Write([]byte(`{"result":{"count":1}}`))
		case "GET /collections/target/points/target-1":
			_, _ = w.Write([]byte(`{"result":{"id":"target-1","vector":[0.1,0.2],"payload":{"user_id":"forgotten","session_id":"s1"}}}`))
		case "PUT /collections/target/points/vectors", "POST /collections/target/points/delete":
			_, _ = w.Write([]byte(`{"result":{"status":"acknowledged"}}`))
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	store := newQdrantTestStore(server.URL)
	vectors, err := store.GetVectorsByUserID(context.Background(), "target", "forgotten")
	if err != nil {
		t.Fatalf("GetVectorsByUserID() error = %v", err)
	}
	if len(vectors) != 1 || vectors[0].CollectionName != "target" || vectors[0].UserID != "forgotten" {
		t.Fatalf("GetVectorsByUserID() = %#v", vectors)
	}

	count, err := store.CountVectorsByUserID(context.Background(), "target", "forgotten")
	if err != nil || count != 1 {
		t.Fatalf("CountVectorsByUserID() = %d, %v", count, err)
	}

	record, err := store.GetVectorByID(context.Background(), "target", "target-1")
	if err != nil || record.ID != "target-1" {
		t.Fatalf("GetVectorByID() = %#v, %v", record, err)
	}

	if err := store.BatchUpdateVectors(context.Background(), &models.BatchVectorUpdateRequest{
		CollectionName: "target",
		Updates:        []models.VectorUpdateItem{{ID: "target-1", Vector: []float32{0.3, 0.4}}},
	}); err != nil {
		t.Fatalf("BatchUpdateVectors() error = %v", err)
	}
	if err := store.DeleteVectors(context.Background(), "target", []string{"target-1"}); err != nil {
		t.Fatalf("DeleteVectors() error = %v", err)
	}

	wantPaths := []string{
		"POST /collections/target/points/scroll",
		"POST /collections/target/points/count",
		"GET /collections/target/points/target-1",
		"PUT /collections/target/points/vectors",
		"POST /collections/target/points/delete",
	}
	if !reflect.DeepEqual(paths, wantPaths) {
		t.Fatalf("paths = %v, want %v", paths, wantPaths)
	}
}

func TestGenerateEmbeddingContextHonorsCancellation(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer func() {
		close(release)
		server.Close()
	}()

	store := newQdrantTestStore(server.URL)
	store.config.EmbeddingConfig.APIEndpoint = server.URL + "/embedding"
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	started := time.Now()
	_, err := store.GenerateEmbeddingContext(ctx, "cancel me")
	if err == nil {
		t.Fatal("GenerateEmbeddingContext() error = nil, want cancellation error")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("cancellation took %v, want under 1s", elapsed)
	}
}

func TestStoreMemoryUsesConfiguredCollectionAndAuthoritativeIdentity(t *testing.T) {
	t.Setenv("VECTOR_DB_COLLECTION", "configured_collection")
	var pointPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /collections/configured_collection":
			_, _ = w.Write([]byte(`{"result":{}}`))
		case "POST /embedding":
			_, _ = w.Write([]byte(`{"embedding":[0.1,0.2]}`))
		case "PUT /collections/configured_collection/points":
			var request struct {
				Points []struct {
					Payload map[string]interface{} `json:"payload"`
				} `json:"points"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatalf("decode upsert request: %v", err)
			}
			pointPayload = request.Points[0].Payload
			_, _ = w.Write([]byte(`{"result":{"status":"acknowledged"}}`))
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	store, err := NewQdrantVectorStore(&models.VectorStoreConfig{
		DefaultCollection: "ignored_collection",
		DatabaseConfig:    &models.DatabaseConfig{Endpoint: server.URL},
		EmbeddingConfig:   &models.EmbeddingConfig{APIEndpoint: server.URL + "/embedding", Dimension: 2},
	})
	if err != nil {
		t.Fatalf("NewQdrantVectorStore() error = %v", err)
	}
	if err := store.StoreMemory(&models.Memory{
		ID:        "memory-1",
		UserID:    "authoritative-user",
		SessionID: "authoritative-session",
		Content:   "content",
		Metadata: map[string]interface{}{
			"user_id":    "metadata-user",
			"session_id": "metadata-session",
			"custom":     "kept",
		},
	}); err != nil {
		t.Fatalf("StoreMemory() error = %v", err)
	}

	if pointPayload["user_id"] != "authoritative-user" || pointPayload["session_id"] != "authoritative-session" {
		t.Fatalf("identity payload = %#v", pointPayload)
	}
	if pointPayload["custom"] != "kept" {
		t.Fatalf("custom metadata missing from payload = %#v", pointPayload)
	}
}

func TestQdrantUnlearningOperationsSurfaceStorageErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "storage unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	store := newQdrantTestStore(server.URL)
	if _, err := store.CountVectorsByUserID(context.Background(), "target", "forgotten"); err == nil {
		t.Fatal("CountVectorsByUserID() error = nil, want storage error")
	}
}

func newQdrantTestStore(baseURL string) *QdrantVectorStore {
	return &QdrantVectorStore{
		baseURL:    baseURL,
		collection: "default",
		httpClient: http.DefaultClient,
		config: &models.VectorStoreConfig{
			EmbeddingConfig: &models.EmbeddingConfig{Dimension: 2},
		},
	}
}
