package services

import (
	"strings"
	"testing"

	"github.com/contextkeeper/service/internal/models"
)

func TestBuildSmartAnalysisPromptPreservesPercentLiteral(t *testing.T) {
	service := &ContextService{}
	contextData := &models.LLMDrivenContextModel{
		SessionID: "session-1",
		Core: &models.CoreContext{
			CurrentFocus:   "护理风险",
			IntentCategory: models.IntentType("analysis"),
			Complexity:     "medium",
		},
	}

	prompt := service.buildSmartAnalysisPrompt(contextData, "当前风险下降 30%")
	if !strings.Contains(prompt, "当前风险下降 30%") {
		t.Fatalf("prompt lost percent literal: %s", prompt)
	}
	if strings.Contains(prompt, "%!s") {
		t.Fatalf("prompt contains formatting artifact: %s", prompt)
	}
}
