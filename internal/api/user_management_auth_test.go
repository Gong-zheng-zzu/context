package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/contextkeeper/service/internal/models"
	"github.com/contextkeeper/service/internal/store"
	"github.com/gin-gonic/gin"
)

// userManagementRouter deliberately injects the identity claims the JWT
// middleware would normally set. The handlers are tested directly so these
// tests remain independent of signing keys and exercise the authorization
// boundary before repository access.
func userManagementRouter(h *Handler, userID, role string) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if userID != "" {
			c.Set("user_id", userID)
		}
		if role != "" {
			c.Set("role", role)
		}
		c.Next()
	})
	router.POST("/api/users", h.HandleCreateUser)
	router.PUT("/api/users/:userId", h.HandleUpdateUser)
	router.GET("/api/users/:userId", h.HandleGetUser)
	return router
}

func userManagementRequest(router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

func seededUserManagementHandler(t *testing.T) *Handler {
	t.Helper()
	repo := store.NewMemoryUserRepository()
	if err := repo.CreateUser(&models.UserInfo{UserID: "caregiver-a", FirstUsed: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := repo.CreateUser(&models.UserInfo{UserID: "resident-b", FirstUsed: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return &Handler{userRepository: repo}
}

func TestUserManagementRejectsCrossUserCaregiverRead(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := seededUserManagementHandler(t)
	resp := userManagementRequest(userManagementRouter(h, "caregiver-a", "caregiver"), http.MethodGet, "/api/users/resident-b", "")
	if resp.Code != http.StatusForbidden {
		t.Fatalf("cross-user read status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func TestUserManagementRejectsCaregiverWriteAndCreateBeforeRepositoryAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &Handler{} // nil repository proves authorization runs first.
	router := userManagementRouter(h, "caregiver-a", "caregiver")

	update := userManagementRequest(router, http.MethodPut, "/api/users/resident-b", `{"lastActive":"2026-01-02T00:00:00Z"}`)
	if update.Code != http.StatusForbidden {
		t.Fatalf("cross-user update status=%d body=%s", update.Code, update.Body.String())
	}
	create := userManagementRequest(router, http.MethodPost, "/api/users", `not-json`)
	if create.Code != http.StatusForbidden {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}
}

func TestUserManagementAllowsSelfReadAndPrivilegedCrossUserManagement(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := seededUserManagementHandler(t)

	self := userManagementRequest(userManagementRouter(h, "caregiver-a", "caregiver"), http.MethodGet, "/api/users/caregiver-a", "")
	if self.Code != http.StatusOK {
		t.Fatalf("self read status=%d body=%s", self.Code, self.Body.String())
	}

	for _, role := range []string{"doctor", "admin"} {
		router := userManagementRouter(h, "staff-"+role, role)
		read := userManagementRequest(router, http.MethodGet, "/api/users/resident-b", "")
		if read.Code != http.StatusOK {
			t.Fatalf("%s cross-user read status=%d body=%s", role, read.Code, read.Body.String())
		}
		update := userManagementRequest(router, http.MethodPut, "/api/users/resident-b", `{"lastActive":"2026-01-02T00:00:00Z"}`)
		if update.Code != http.StatusOK {
			t.Fatalf("%s cross-user update status=%d body=%s", role, update.Code, update.Body.String())
		}
		create := userManagementRequest(router, http.MethodPost, "/api/users", `{"userId":"created-`+role+`"}`)
		if create.Code != http.StatusCreated {
			t.Fatalf("%s create status=%d body=%s", role, create.Code, create.Body.String())
		}
	}
}

func TestUserManagementRequiresIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &Handler{}
	resp := userManagementRequest(userManagementRouter(h, "", "caregiver"), http.MethodGet, "/api/users/any", "")
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d body=%s", resp.Code, resp.Body.String())
	}
}
