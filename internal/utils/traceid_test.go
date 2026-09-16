package utils

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestTraceIDMiddlewareAcceptsOnlyBoundedSafeIdentifiers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name     string
		provided string
		wantSame bool
	}{
		{name: "valid", provided: "demo-trace_123", wantSame: true},
		{name: "too short", provided: "short"},
		{name: "log injection", provided: "trace\nforged"},
		{name: "too long", provided: "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789___"},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			router.Use(TraceIDMiddleware())
			router.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.Header.Set("X-Trace-ID", test.provided)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			got := response.Header().Get("X-Trace-ID")
			if !validExternalTraceID.MatchString(got) {
				t.Fatalf("generated trace ID %q is not safe", got)
			}
			if (got == test.provided) != test.wantSame {
				t.Fatalf("trace ID = %q, provided = %q, wantSame = %v", got, test.provided, test.wantSame)
			}
		})
	}
}
