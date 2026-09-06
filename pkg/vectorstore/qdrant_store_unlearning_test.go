package vectorstore

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/contextkeeper/service/internal/models"
)

func newUnlearningTestStore(serverURL string) *QdrantVectorStore {
	return &QdrantVectorStore{
		baseURL:    serverURL,
		collection: "nursing_records",
		httpClient: &http.Client{},
		config: &models.VectorStoreConfig{
			EmbeddingConfig: &models.EmbeddingConfig{Dimension: 2, APIEndpoint: serverURL + "/embedding", Model: "test"},
		},
	}
}

func TestQdrantUserOperationsUseRequestedCollection(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/collections/nursing_records/points/scroll":
			io.WriteString(w, `{"result":{"points":[{"id":"v1","vector":[0.1,0.2],"payload":{"user_id":"user-a","session_id":"session-a"}}],"next_page_offset":null}}`)
		case "/collections/nursing_records/points/count":
			io.WriteString(w, `{"result":{"count":1}}`)
		case "/collections/nursing_records/points/v1":
			io.WriteString(w, `{"result":{"id":"v1","vector":[0.1,0.2],"payload":{"user_id":"user-a","session_id":"session-a"}}}`)
		case "/collections/nursing_records/points/delete", "/collections/nursing_records/points/vectors", "/collections/nursing_records/points/payload":
			io.WriteString(w, `{"result":{"status":"acknowledged"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	store := newUnlearningTestStore(server.URL)
	ctx := context.Background()

	vectors, err := store.GetVectorsByUserID(ctx, "nursing_records", "user-a")
	if err != nil || len(vectors) != 1 || vectors[0].UserID != "user-a" {
		t.Fatalf("GetVectorsByUserID() = %#v, %v", vectors, err)
	}
	count, err := store.CountVectorsByUserID(ctx, "nursing_records", "user-a")
	if err != nil || count != 1 {
		t.Fatalf("CountVectorsByUserID() = %d, %v", count, err)
	}
	record, err := store.GetVectorByID(ctx, "nursing_records", "v1")
	if err != nil || record.ID != "v1" || len(record.Vector) != 2 {
		t.Fatalf("GetVectorByID() = %#v, %v", record, err)
	}
	if err := store.BatchUpdateVectors(ctx, &models.BatchVectorUpdateRequest{
		CollectionName: "nursing_records",
		Updates:        []models.VectorUpdateItem{{ID: "v1", Vector: []float32{0.3, 0.4}, Metadata: map[string]interface{}{"tag": "updated"}}},
	}); err != nil {
		t.Fatalf("BatchUpdateVectors() error = %v", err)
	}
	if err := store.DeleteVectors(ctx, "nursing_records", []string{"v1"}); err != nil {
		t.Fatalf("DeleteVectors() error = %v", err)
	}

	for _, path := range paths {
		if !strings.HasPrefix(path, "/collections/nursing_records/") {
			t.Fatalf("operation used an unexpected collection path: %s", path)
		}
	}
}

func TestQdrantStorePayloadOwnershipCannotBeOverriddenByMetadata(t *testing.T) {
	var payloads []map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/embedding" {
			io.WriteString(w, `{"embedding":[0.1,0.2]}`)
			return
		}
		if r.URL.Path != "/collections/nursing_records/points" {
			http.NotFound(w, r)
			return
		}
		var request struct {
			Points []struct {
				Payload map[string]interface{} `json:"payload"`
			} `json:"points"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode write request: %v", err)
		}
		payloads = append(payloads, request.Points[0].Payload)
		io.WriteString(w, `{"result":{"status":"acknowledged"}}`)
	}))
	defer server.Close()

	store := newUnlearningTestStore(server.URL)
	if err := store.StoreMemory(&models.Memory{ID: "m1", UserID: "user-a", SessionID: "session-a", Content: "memory", Metadata: map[string]interface{}{"user_id": "spoofed", "session_id": "spoofed"}}); err != nil {
		t.Fatalf("StoreMemory() error = %v", err)
	}
	if err := store.StoreMessage(&models.Message{ID: "m2", UserID: "user-a", SessionID: "session-a", Content: "message", Metadata: map[string]interface{}{"user_id": "spoofed", "session_id": "spoofed"}}); err != nil {
		t.Fatalf("StoreMessage() error = %v", err)
	}
	if len(payloads) != 2 {
		t.Fatalf("stored payload count = %d, want 2", len(payloads))
	}
	for _, payload := range payloads {
		if payload["user_id"] != "user-a" || payload["session_id"] != "session-a" {
			t.Fatalf("ownership payload was overwritten: %#v", payload)
		}
	}
}

func TestQdrantRejectsOwnershipMetadataUpdate(t *testing.T) {
	store := newUnlearningTestStore("http://127.0.0.1:1")
	err := store.UpdateVector(context.Background(), &models.VectorUpdateRequest{
		CollectionName: "nursing_records",
		ID:             "v1",
		NewVector:      []float32{0.1, 0.2},
		Metadata:       map[string]interface{}{"user_id": "spoofed"},
	})
	if err == nil || !strings.Contains(err.Error(), "must not modify user_id") {
		t.Fatalf("UpdateVector() error = %v, want ownership metadata rejection", err)
	}
}

func TestQdrantSessionScopedUnlearningOperationsUseExactOwnershipFilter(t *testing.T) {
	type requestFilter struct {
		Filter struct {
			Must []struct {
				Key   string `json:"key"`
				Match struct {
					Value string `json:"value"`
				} `json:"match"`
			} `json:"must"`
		} `json:"filter"`
	}

	var filters []map[string]string
	var deleteFilter map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/collections/nursing_records/points/count" && r.URL.Path != "/collections/nursing_records/points/delete" {
			http.NotFound(w, r)
			return
		}
		var request requestFilter
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode Qdrant request: %v", err)
		}
		actual := make(map[string]string, len(request.Filter.Must))
		for _, condition := range request.Filter.Must {
			actual[condition.Key] = condition.Match.Value
		}
		if r.URL.Path == "/collections/nursing_records/points/delete" {
			deleteFilter = actual
			_, _ = io.WriteString(w, `{"result":{"status":"acknowledged"}}`)
			return
		}
		filters = append(filters, actual)
		_, _ = io.WriteString(w, `{"result":{"count":1}}`)
	}))
	defer server.Close()

	store := newUnlearningTestStore(server.URL)
	ctx := context.Background()
	if count, err := store.CountVectorsByUserAndSessionID(ctx, "nursing_records", "eval_user_001", "eval_retrieval_test"); err != nil || count != 1 {
		t.Fatalf("CountVectorsByUserAndSessionID() = %d, %v", count, err)
	}
	if count, err := store.CountVectorsByUserSessionAndDocID(ctx, "nursing_records", "eval_user_001", "eval_retrieval_test", "doc-1"); err != nil || count != 1 {
		t.Fatalf("CountVectorsByUserSessionAndDocID() = %d, %v", count, err)
	}
	if deleted, err := store.DeleteVectorsByUserAndSessionID(ctx, "nursing_records", "eval_user_001", "eval_retrieval_test"); err != nil || deleted != 1 {
		t.Fatalf("DeleteVectorsByUserAndSessionID() = %d, %v", deleted, err)
	}

	wantSession := map[string]string{"user_id": "eval_user_001", "session_id": "eval_retrieval_test"}
	if len(filters) != 3 || !reflect.DeepEqual(filters[0], wantSession) || !reflect.DeepEqual(filters[2], wantSession) {
		t.Fatalf("session filters = %#v, want exact user/session predicates", filters)
	}
	wantDocument := map[string]string{"user_id": "eval_user_001", "session_id": "eval_retrieval_test", "doc_id": "doc-1"}
	if !reflect.DeepEqual(filters[1], wantDocument) {
		t.Fatalf("document filter = %#v, want %#v", filters[1], wantDocument)
	}
	if !reflect.DeepEqual(deleteFilter, wantSession) {
		t.Fatalf("delete filter = %#v, want %#v", deleteFilter, wantSession)
	}
}
