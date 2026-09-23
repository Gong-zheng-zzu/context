package main

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestHandleTieredCycleDoesNotDeadlockDuringColdArchive(t *testing.T) {
	gin.SetMode(gin.TestMode)
	if err := initTiered(); err != nil {
		t.Fatalf("initTiered: %v", err)
	}

	add := httptest.NewRecorder()
	addCtx, _ := gin.CreateTestContext(add)
	addReq := httptest.NewRequest("POST", "/api/tiered/memory", nil)
	addReq.Header.Set("Content-Type", "application/json")
	// The handler reads JSON from the request body; use the public handler so
	// this test exercises the same state and manager wiring as the playground.
	addReq = httptest.NewRequest("POST", "/api/tiered/memory", strings.NewReader(`{"content":"cold archive test"}`))
	addReq.Header.Set("Content-Type", "application/json")
	addCtx.Request = addReq
	handleTieredAdd(addCtx)
	if add.Code != 200 {
		t.Fatalf("handleTieredAdd status = %d, body = %s", add.Code, add.Body.String())
	}

	// The demo policy transitions hot -> warm -> cold in separate cycles.
	time.Sleep(450 * time.Millisecond)
	cycle := httptest.NewRecorder()
	cycleCtx, _ := gin.CreateTestContext(cycle)
	cycleCtx.Request = httptest.NewRequest("POST", "/api/tiered/cycle", nil)
	done := make(chan struct{})
	go func() {
		handleTieredCycle(cycleCtx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handleTieredCycle timed out; possible state/provider lock deadlock")
	}
	if cycle.Code != 200 {
		t.Fatalf("handleTieredCycle status = %d, body = %s", cycle.Code, cycle.Body.String())
	}
}
