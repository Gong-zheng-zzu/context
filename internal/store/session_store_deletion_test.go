package store

import "testing"

func TestSessionArtifactCountAfterDeletionVerifiesNoCacheOrFilesRemain(t *testing.T) {
	store, err := NewSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewSessionStore() error = %v", err)
	}
	if err := store.SetSessionMetadata("session-a", "userId", "owner"); err != nil {
		t.Fatalf("SetSessionMetadata() error = %v", err)
	}

	if _, err := store.DeleteSession("session-a"); err != nil {
		t.Fatalf("DeleteSession() error = %v", err)
	}
	count, err := store.SessionArtifactCountAfterDeletion("session-a")
	if err != nil {
		t.Fatalf("SessionArtifactCountAfterDeletion() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("SessionArtifactCountAfterDeletion() = %d, want 0", count)
	}
}
