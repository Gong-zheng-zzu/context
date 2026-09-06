package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestLoginHandlerRejectsMissingAuthConfig(t *testing.T) {
	t.Setenv("DEMO_AUTH_CREDENTIALS", "")
	t.Setenv("DEMO_AUTH_PASSWORD", "")

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(recorder)
	router.POST("/login", LoginHandler)

	body, _ := json.Marshal(LoginRequest{
		UserID:   "doctor_001",
		Password: "secret",
	})

	req, _ := http.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req

	LoginHandler(ctx)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected %d, got %d", http.StatusServiceUnavailable, recorder.Code)
	}
}

func TestLoginHandlerAcceptsConfiguredCredentials(t *testing.T) {
	t.Setenv("DEMO_AUTH_CREDENTIALS", "doctor_001:secret")
	t.Setenv("DEMO_AUTH_PASSWORD", "")

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	body, _ := json.Marshal(LoginRequest{
		UserID:      "doctor_001",
		Password:    "secret",
		WorkspaceID: "ws-1",
	})

	req, _ := http.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req

	LoginHandler(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, recorder.Code)
	}
}
