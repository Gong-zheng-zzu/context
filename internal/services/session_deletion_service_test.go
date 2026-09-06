package services

import (
	"context"
	"errors"
	"testing"

	"github.com/contextkeeper/service/internal/models"
	"github.com/contextkeeper/service/internal/store"
)

type fakeSessionReplica struct {
	count     int64
	deleteErr error
	leaveOne  bool
}

type fakeSessionVectorStore struct {
	records   []*models.VectorRecord
	deleted   []string
	deleteErr error
}

func (f *fakeSessionVectorStore) GetVectorsByUserID(_ context.Context, _ string, _ string) ([]*models.VectorRecord, error) {
	return f.records, nil
}

func (f *fakeSessionVectorStore) DeleteVectors(_ context.Context, _ string, ids []string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = append(f.deleted, ids...)
	remaining := make([]*models.VectorRecord, 0, len(f.records))
	for _, record := range f.records {
		deleteRecord := false
		for _, id := range ids {
			if record.ID == id {
				deleteRecord = true
				break
			}
		}
		if !deleteRecord {
			remaining = append(remaining, record)
		}
	}
	f.records = remaining
	return nil
}

func (f *fakeSessionReplica) CountSessionReplicaRecords(context.Context, string, string) (int64, error) {
	return f.count, nil
}

func (f *fakeSessionReplica) DeleteSessionReplicaRecords(context.Context, string, string) (int64, error) {
	if f.deleteErr != nil {
		return 0, f.deleteErr
	}
	deleted := f.count
	if f.leaveOne && f.count > 0 {
		f.count = 1
	} else {
		f.count = 0
	}
	return deleted - f.count, nil
}

func newOwnedSessionStore(t *testing.T, userID, sessionID string) *store.SessionStore {
	t.Helper()
	sessionStore, err := store.NewSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("create session store: %v", err)
	}
	if err := sessionStore.SetSessionMetadata(sessionID, "userId", userID); err != nil {
		t.Fatalf("set session owner: %v", err)
	}
	return sessionStore
}

func TestSessionDeletionDeletesAllReplicasBeforeLocalArtifacts(t *testing.T) {
	sessionStore := newOwnedSessionStore(t, "owner", "session-a")
	timelineReplica := &fakeSessionReplica{count: 3}
	graphReplica := &fakeSessionReplica{count: 2}
	service := NewSessionDeletionService(sessionStore,
		sessionReplicaBinding{name: "timescaledb", replica: timelineReplica},
		sessionReplicaBinding{name: "neo4j", replica: graphReplica},
	)

	result, err := service.Delete(context.Background(), "owner", "session-a", false)
	if err != nil {
		t.Fatalf("delete cascade: %v", err)
	}
	if !result.Complete {
		t.Fatal("successful verified cascade must be marked complete")
	}
	if len(result.Stores) != 3 {
		t.Fatalf("stores = %d, want 3", len(result.Stores))
	}
	for _, storeResult := range result.Stores {
		if storeResult.After != 0 || storeResult.Status != "deleted" {
			t.Fatalf("unexpected store result: %#v", storeResult)
		}
	}
	if _, err := sessionStore.GetSessionOwner("session-a"); !errors.Is(err, store.ErrSessionNotFound) {
		t.Fatalf("local session should be removed, err=%v", err)
	}
}

func TestSessionDeletionReplicaFailureLeavesLocalSessionRetryable(t *testing.T) {
	sessionStore := newOwnedSessionStore(t, "owner", "session-b")
	service := NewSessionDeletionService(sessionStore,
		sessionReplicaBinding{name: "timescaledb", replica: &fakeSessionReplica{count: 1, deleteErr: errors.New("db unavailable")}},
	)

	result, err := service.Delete(context.Background(), "owner", "session-b", false)
	if err == nil {
		t.Fatal("replica failure must fail the cascade")
	}
	if result.Complete {
		t.Fatal("failed cascade must never be marked complete")
	}
	if _, err := sessionStore.GetSessionOwner("session-b"); err != nil {
		t.Fatalf("local session must remain for retry, err=%v", err)
	}
}

func TestSessionDeletionResidualReplicaFailsVerification(t *testing.T) {
	sessionStore := newOwnedSessionStore(t, "owner", "session-c")
	service := NewSessionDeletionService(sessionStore,
		sessionReplicaBinding{name: "neo4j", replica: &fakeSessionReplica{count: 2, leaveOne: true}},
	)

	result, err := service.Delete(context.Background(), "owner", "session-c", false)
	if err == nil {
		t.Fatal("residual data must fail verification")
	}
	if result.Complete || result.Stores[0].After != 1 {
		t.Fatalf("unexpected failed result: %#v", result)
	}
	if _, err := sessionStore.GetSessionOwner("session-c"); err != nil {
		t.Fatalf("local session must remain for retry, err=%v", err)
	}
}

func TestSessionDeletionRejectsNonOwner(t *testing.T) {
	sessionStore := newOwnedSessionStore(t, "owner", "session-d")
	service := NewSessionDeletionService(sessionStore)

	result, err := service.Delete(context.Background(), "other-user", "session-d", false)
	if !errors.Is(err, ErrSessionOwnerMismatch) {
		t.Fatalf("err = %v, want owner mismatch", err)
	}
	if result.Complete {
		t.Fatal("owner mismatch must not be marked complete")
	}
	if _, err := sessionStore.GetSessionOwner("session-d"); err != nil {
		t.Fatalf("other user must not delete the session, err=%v", err)
	}
}

func TestSessionDeletionDryRunPreservesAllStores(t *testing.T) {
	sessionStore := newOwnedSessionStore(t, "owner", "session-dry-run")
	replica := &fakeSessionReplica{count: 4}
	service := NewSessionDeletionService(sessionStore,
		sessionReplicaBinding{name: "timescaledb", replica: replica},
	)

	result, err := service.Delete(context.Background(), "owner", "session-dry-run", true)
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if result.Complete || replica.count != 4 {
		t.Fatalf("dry-run modified completion or replica state: %#v", result)
	}
	for _, storeResult := range result.Stores {
		if storeResult.Status != "dry_run" || storeResult.Before != storeResult.After || storeResult.Deleted != 0 {
			t.Fatalf("unexpected dry-run result: %#v", storeResult)
		}
	}
	if _, err := sessionStore.GetSessionOwner("session-dry-run"); err != nil {
		t.Fatalf("dry-run removed local session: %v", err)
	}
}

func TestSessionDeletionVectorReplicaUsesConfiguredCollectionAndSessionScope(t *testing.T) {
	sessionStore := newOwnedSessionStore(t, "owner", "session-e")
	vectorStore := &fakeSessionVectorStore{records: []*models.VectorRecord{
		{ID: "target", Metadata: map[string]interface{}{"user_id": "owner", "session_id": "session-e"}},
		{ID: "other-session", Metadata: map[string]interface{}{"user_id": "owner", "session_id": "session-other"}},
		{ID: "other-user", Metadata: map[string]interface{}{"user_id": "other-user", "session_id": "session-e"}},
	}}
	service, err := NewSessionDeletionServiceWithVectorStore(sessionStore, vectorStore, "nursing_records")
	if err != nil {
		t.Fatalf("create complete deletion service: %v", err)
	}

	result, err := service.Delete(context.Background(), "owner", "session-e", false)
	if err != nil {
		t.Fatalf("delete cascade: %v", err)
	}
	if !result.Complete || len(vectorStore.deleted) != 1 || vectorStore.deleted[0] != "target" {
		t.Fatalf("unexpected Qdrant deletion result: %#v, deleted=%v", result, vectorStore.deleted)
	}
	if len(vectorStore.records) != 2 || vectorStore.records[0].ID != "other-session" || vectorStore.records[1].ID != "other-user" {
		t.Fatalf("another session's vector was affected: %#v", vectorStore.records)
	}
}
