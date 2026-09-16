package api

import (
	"net/http"

	"github.com/contextkeeper/service/internal/utils"
	"github.com/gin-gonic/gin"
)

func competitionTraceID(c *gin.Context) string {
	traceID := utils.GetTraceIDFromGin(c)
	if traceID == "" {
		traceID = utils.GenerateTraceID()
		c.Set(utils.TraceIDKey, traceID)
	}
	c.Header("X-Trace-ID", traceID)
	return traceID
}

func writeCompetitionError(c *gin.Context, status int, stage, message string) {
	c.JSON(status, gin.H{
		"success":  false,
		"trace_id": competitionTraceID(c),
		"stage":    stage,
		"status":   "failed",
		"error":    message,
	})
}

func writeCompetitionUnavailable(c *gin.Context, stage, message string) {
	writeCompetitionError(c, http.StatusServiceUnavailable, stage, message)
}
