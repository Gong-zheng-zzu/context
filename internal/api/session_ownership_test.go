package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/contextkeeper/service/internal/config"
	"github.com/contextkeeper/service/internal/services"
	"github.com/contextkeeper/service/internal/store"
	"github.com/gin-gonic/gin"
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

func TestDirectSessionRoutesRejectCrossUserRequests(t *testing.T) {
	t.Setenv("LLM_DRIVEN_ENABLED", "false")
	sessionStore, err := store.NewSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("create session store: %v", err)
	}
	if err := ensureProtectedSessionOwner(sessionStore, "owned-session", "user-a"); err != nil {
		t.Fatalf("seed session owner: %v", err)
	}
	base := services.NewContextService(nil, sessionStore, &config.Config{})
	service := services.NewLLMDrivenContextServiceWithEngines(base, nil)
	handler := &Handler{contextService: service}

	tests := []struct {
		name string
		path string
		body string
		hand func(*Handler, *gin.Context)
	}{
		{"store", "/mcp/tools/create_context", `{"sessionId":"owned-session","content":"x"}`, (*Handler).HandleStoreContext},
		{"read", "/mcp/tools/read_context", `{"sessionId":"owned-session","query":"x"}`, (*Handler).HandleRetrieveContext},
		{"associate", "/api/mcp/tools/associate_file", `{"sessionId":"owned-session","filePath":"a.go"}`, (*Handler).HandleMCPAssociateFile},
		{"edit", "/api/mcp/tools/record_edit", `{"sessionId":"owned-session","filePath":"a.go","diff":"x"}`, (*Handler).HandleMCPRecordEdit},
		{"programming", "/api/mcp/tools/programming_context", `{"sessionId":"owned-session","query":"x"}`, (*Handler).HandleMCPProgrammingContext},
	}
	gin.SetMode(gin.TestMode)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			router.POST(test.path, func(c *gin.Context) {
				c.Set("user_id", "user-b")
				test.hand(handler, c)
			})
			req := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
			}
			owner, err := sessionStore.GetSessionOwner("owned-session")
			if err != nil || owner != "user-a" {
				t.Fatalf("session owner = %q, err=%v; takeover occurred", owner, err)
			}
		})
	}
}

func TestSessionScopedMCPToolAllowlist(t *testing.T) {
	for _, name := range []string{"associate_file", "record_edit", "retrieve_context", "programming_context"} {
		if !isSessionScopedMCPTool(name) {
			t.Fatalf("%q must require session ownership", name)
		}
	}
	for _, name := range []string{"tools/list", "unknown", "health"} {
		if isSessionScopedMCPTool(name) {
			t.Fatalf("%q must not be classified as a session-scoped tool", name)
		}
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
