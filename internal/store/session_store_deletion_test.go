package store

import (
	"sync"
	"testing"
)

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

func TestEnsureSessionOwnerConcurrentFirstClaimIsAtomic(t *testing.T) {
	store, err := NewSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewSessionStore() error = %v", err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, owner := range []string{"user-a", "user-b"} {
		owner := owner
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- store.EnsureSessionOwner("shared-session", owner)
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	accepted, rejected := 0, 0
	for err := range results {
		if err == nil {
			accepted++
		} else {
			rejected++
		}
	}
	if accepted != 1 || rejected != 1 {
		t.Fatalf("concurrent claim results: accepted=%d rejected=%d; want one each", accepted, rejected)
	}

	owner, err := store.GetSessionOwner("shared-session")
	if err != nil {
		t.Fatalf("GetSessionOwner() error = %v", err)
	}
	if owner != "user-a" && owner != "user-b" {
		t.Fatalf("stored owner = %q; want one of the claimants", owner)
	}
}

func TestEnsureSessionOwnerDoesNotRebindOrClaimOwnerlessSession(t *testing.T) {
	store, err := NewSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewSessionStore() error = %v", err)
	}
	if err := store.EnsureSessionOwner("owned", "owner-a"); err != nil {
		t.Fatalf("initial claim: %v", err)
	}
	if err := store.EnsureSessionOwner("owned", "owner-b"); err == nil {
		t.Fatal("EnsureSessionOwner() allowed an owner rebind")
	}

	if _, err := store.GetSession("legacy"); err != nil {
		t.Fatalf("create ownerless legacy session: %v", err)
	}
	if err := store.EnsureSessionOwner("legacy", "owner-a"); err == nil {
		t.Fatal("EnsureSessionOwner() claimed an ownerless legacy session")
	}
}
