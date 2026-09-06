package api

import (
	"strings"
	"testing"

	"github.com/contextkeeper/service/internal/store"
)

func TestEnsureProtectedSessionOwner(t *testing.T) {
	sessionStore, err := store.NewSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("create session store: %v", err)
	}

	if err := ensureProtectedSessionOwner(sessionStore, "session-a", "user-a"); err != nil {
		t.Fatalf("bind new session: %v", err)
	}
	if err := ensureProtectedSessionOwner(sessionStore, "session-a", "user-a"); err != nil {
		t.Fatalf("reuse owned session: %v", err)
	}
	if err := ensureProtectedSessionOwner(sessionStore, "session-a", "user-b"); err == nil {
		t.Fatal("expected cross-user session access to be rejected")
	}
}

func TestIsClinicalFactQuery(t *testing.T) {
	if !isClinicalFactQuery("王奶奶今天服用降压药了吗？") {
		t.Fatal("expected medication status question to require a record")
	}
	if isClinicalFactQuery("请给出日常护理建议") {
		t.Fatal("general nursing advice must remain available")
	}
	if isClinicalFactQuery("记录张奶奶血压130/80，体温36.5度") {
		t.Fatal("explicit record command must not be treated as a fact lookup")
	}
	if !isClinicalFactQuery("张奶奶血压是多少？") {
		t.Fatal("explicit blood pressure question must require an auditable record")
	}
}

func TestEnforceOutputRedactionMasksPhoneNumber(t *testing.T) {
	redacted, changed := enforceOutputRedaction("联系电话是18291810799")
	if !changed {
		t.Fatal("expected output redaction to report a change")
	}
	if strings.Contains(redacted, "18291810799") || !strings.Contains(redacted, "182****0799") {
		t.Fatalf("unexpected redaction result: %q", redacted)
	}
}
