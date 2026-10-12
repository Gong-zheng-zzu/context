package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/contextkeeper/service/internal/auth"
	"github.com/gin-gonic/gin"
)

func TestCallCaregiverRejectsUnrelatedElder(t *testing.T) {
	auth.InitDemoRelations()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/call", func(c *gin.Context) {
		c.Set("user_id", "caregiver_001")
		c.Set("role", "caregiver")
		CallCaregiverHandler(c)
	})
	req := httptest.NewRequest(http.MethodPost, "/call", strings.NewReader(`{"elder_id":"elder_003","reason":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestFamilyMessageAllowsRelatedElder(t *testing.T) {
	auth.InitDemoRelations()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/message", func(c *gin.Context) {
		c.Set("user_id", "family_001")
		c.Set("role", "family")
		FamilyMessageHandler(c)
	})
	req := httptest.NewRequest(http.MethodPost, "/message", strings.NewReader(`{"elder_id":"elder_001","message":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

