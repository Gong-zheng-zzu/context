package store

import (
	"strings"
	"testing"
	"time"
)

func TestNewSessionUsesSM3UserIdentifier(t *testing.T) {
	const userID = "migration-test-user"

	store, err := NewSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewSessionStore() error = %v", err)
	}

	session, created, err := store.GetOrCreateActiveSession(userID, time.Minute)
	if err != nil {
		t.Fatalf("GetOrCreateActiveSession() error = %v", err)
	}
	if !created {
		t.Fatal("GetOrCreateActiveSession() created = false, want true")
	}

	identifier := sessionUserIdentifier(userID)
	if len(identifier) < sessionUserIdentifierHexLength {
		t.Fatalf("SM3 identifier length = %d, want at least %d", len(identifier), sessionUserIdentifierHexLength)
	}
	if !strings.Contains(session.ID, identifier) {
		t.Fatalf("new session ID %q does not contain SM3 identifier %q", session.ID, identifier)
	}
	if strings.Contains(session.ID, legacySessionUserIdentifier(userID)) {
		t.Fatalf("new session ID %q contains legacy MD5 identifier", session.ID)
	}
}

func TestLegacySessionUserIdentifierIsStable(t *testing.T) {
	const userID = "migration-test-user"
	const want = "f347542a"

	if got := legacySessionUserIdentifier(userID); got != want {
		t.Fatalf("legacySessionUserIdentifier(%q) = %q, want %q", userID, got, want)
	}
}
