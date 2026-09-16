package api

import (
	"context"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/contextkeeper/service/internal/security"
	"github.com/gin-gonic/gin"
)

const defaultSecurityEvaluationUserID = "eval_user_001"

var syntheticSecuritySampleIDPattern = regexp.MustCompile(`^synthetic_[A-Za-z0-9_-]{1,64}$`)

// SecurityInputEvaluationRequest is intentionally limited to the authenticated
// input pipeline. It is used by the experiment runner and never calls the LLM.
type SecurityInputEvaluationRequest struct {
	SessionID  string `json:"session_id"`
	Message    string `json:"message"`
	Input      string `json:"input"`
	SampleID   string `json:"sample_id"`
	AttackType string `json:"attack_type"`
	Name       string `json:"name"`
}

type SecurityAblationEvaluationRequest struct {
	Message   string `json:"message" binding:"required"`
	SampleID  string `json:"sample_id" binding:"required"`
	Synthetic bool   `json:"synthetic" binding:"required"`
}

func configuredSecurityEvaluationUserID() string {
	if value := strings.TrimSpace(os.Getenv("SECURITY_EVAL_USER_ID")); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv("EVAL_USER_ID")); value != "" {
		return value
	}
	return defaultSecurityEvaluationUserID
}

func (h *Handler) securityEvaluationService() *security.SecurityService {
	if h != nil && h.securityService != nil {
		return h.securityService
	}
	if contextService != nil {
		return contextService.GetSecurityService()
	}
	return nil
}

// inputSecurityDecision is the pre-generation decision contract shared by the
// lightweight endpoint and the controlled chat evaluation mode.
func inputSecurityDecision(inputRedacted, inputNormalized bool) (string, string) {
	if inputRedacted {
		return "redact", "sensitive_data_policy"
	}
	if inputNormalized {
		return "allow", "asdf_normalized"
	}
	return "allow", "none"
}

func newSecurityExecutionEvidence(securityService *security.SecurityService) gin.H {
	evidence := gin.H{
		"asdf_checked":     securityService != nil,
		"input_normalized": false,
		"multi_layer_used": false,
		"pccm_enabled":     false,
		"casia_enabled":    false,
	}
	if securityService != nil {
		configuration := securityService.GetMultiLayerConfiguration()
		evidence["multi_layer_enabled"] = configuration.Enabled
		evidence["pccm_enabled"] = configuration.PCCMEnabled
		evidence["casia_enabled"] = configuration.CASIAEnabled
		evidence["early_stop"] = configuration.EarlyStop
	}
	return evidence
}

// HandleSecurityInputEvaluation evaluates the same deterministic input guards
// used before chat generation. It is registered only in the JWT-protected API
// group so the endpoint cannot be used as an unauthenticated policy oracle.
func (h *Handler) HandleSecurityInputEvaluation(c *gin.Context) {
	var req SecurityInputEvaluationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "invalid request: " + err.Error()})
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		req.Message = req.Input
	}
	if strings.TrimSpace(req.Message) == "" {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "message or input is required"})
		return
	}
	if strings.TrimSpace(req.SessionID) == "" {
		req.SessionID = "security_eval"
	}

	userID, ok := c.Get("user_id")
	userIDValue, stringOK := userID.(string)
	if !ok || !stringOK || strings.TrimSpace(userIDValue) == "" {
		c.JSON(http.StatusUnauthorized, APIResponse{Success: false, Error: "missing authenticated user identity"})
		return
	}

	started := time.Now()
	message := req.Message
	decision := "allow"
	stage := "none"
	warnings := make([]string, 0)
	attackTypes := make([]string, 0)
	inputNormalized := false
	originalMessage := message
	redactedMessage := message
	sensitiveTypes := make([]string, 0)
	// Resolve the service before creating execution evidence so the response
	// truthfully reports whether ASDF and multi-layer scanning were available.
	// Prefer the handler's injected service. The global chat service is kept as
	// a compatibility fallback, but must not hide the route's actual config.
	securityService := h.securityEvaluationService()
	securityExecution := newSecurityExecutionEvidence(securityService)

	// Match ChatHandler's pre-generation order without persisting a message,
	// retrieving memory, or invoking the language model.
	if securityService != nil {
		asdfAudit := securityService.DefendAndNormalizeWithAudit(message)
		securityExecution["asdf"] = asdfAudit
		if asdfAudit.IsAdversarial {
			message = asdfAudit.NormalizedText
			attackTypes = append(attackTypes, asdfAudit.AttackTypes...)
			warnings = append(warnings, "asdf_normalized")
			inputNormalized = asdfAudit.NormalizedText != originalMessage
			securityExecution["input_normalized"] = inputNormalized
			securityExecution["asdf_confidence"] = asdfAudit.Confidence
		}
	}

	if decision == "allow" && adversarialDetector != nil {
		if adversarial, reason := adversarialDetector.DetectAdversarial(message); adversarial {
			decision = "block"
			stage = "adversarial_detector"
			warnings = append(warnings, reason)
		}
	}

	if decision == "allow" && inputRequestDetector != nil {
		if blocked, reason := inputRequestDetector.Detect(message); blocked {
			decision = "block"
			stage = "input_request_policy"
			warnings = append(warnings, reason)
		}
	}

	if decision == "allow" && dataPoisoningDetector != nil && len(message) > 500 {
		if valid, reasons := dataPoisoningDetector.ValidateKnowledgeInput(message); !valid {
			warnings = append(warnings, reasons...)
		}
	}

	originalMessage = message
	redactedMessage = message
	if decision == "allow" && securityService != nil {
		scan, err := securityService.ScanContent(context.Background(), req.SessionID, userIDValue, originalMessage)
		if err != nil {
			log.Printf("[security evaluation] sensitive input scan failed: %v", err)
		} else {
			if scan.Metadata != nil {
				if used, ok := scan.Metadata["multi_layer_used"].(bool); ok {
					securityExecution["multi_layer_used"] = used
				}
				if configuration, ok := scan.Metadata["configuration"].(security.MultiLayerConfiguration); ok {
					securityExecution["pccm_enabled"] = configuration.PCCMEnabled
					securityExecution["casia_enabled"] = configuration.CASIAEnabled
				}
				if configuration, ok := scan.Metadata["configuration"].(security.MultiLayerSecurityConfiguration); ok {
					securityExecution["multi_layer_enabled"] = configuration.Enabled
					securityExecution["pccm_enabled"] = configuration.PCCMEnabled
					securityExecution["casia_enabled"] = configuration.CASIAEnabled
					securityExecution["early_stop"] = configuration.EarlyStop
				}
				for _, field := range []string{"layers_used", "final_confidence", "conflicts_resolved", "multi_layer_fallback", "multi_layer_error"} {
					if value, exists := scan.Metadata[field]; exists {
						securityExecution[field] = value
					}
				}
			}
			if len(scan.SensitiveInfos) > 0 {
				redactedMessage = scan.RedactedContent
				for _, info := range scan.SensitiveInfos {
					sensitiveTypes = append(sensitiveTypes, string(info.Type))
				}
			}
		}
	}
	if decision == "allow" {
		decision, stage = inputSecurityDecision(redactedMessage != originalMessage, inputNormalized)
	}

	// The result is intentionally metadata-only: never return raw or redacted input.
	c.JSON(http.StatusOK, APIResponse{Success: true, Data: gin.H{
		"decision":              decision,
		"stage":                 stage,
		"attack_types":          attackTypes,
		"warnings":              warnings,
		"sensitive_types":       sensitiveTypes,
		"redacted":              redactedMessage != originalMessage,
		"latency_ms":            float64(time.Since(started).Microseconds()) / 1000.0,
		"pipeline_version":      "input-security-v1",
		"security_execution":    securityExecution,
		"generation_skipped":    true,
		"authenticated_user_id": userIDValue,
		"sample_id":             req.SampleID,
		"attack_type":           req.AttackType,
		"endpoint_name":         req.Name,
		"memory_write_applied":  false,
		"authorization_granted": false,
		"disclosure_permitted":  false,
		"claim_generated":       false,
		"deleted_data_returned": false,
	}})
}

// HandleSecurityAblationEvaluation computes all security-lab profiles on the
// server. The route is JWT-protected at registration and additionally scoped
// to one configured evaluation identity and explicit synthetic samples.
func (h *Handler) HandleSecurityAblationEvaluation(c *gin.Context) {
	started := time.Now().UTC()
	userValue, exists := c.Get("user_id")
	userID, ok := userValue.(string)
	if !exists || !ok || strings.TrimSpace(userID) == "" {
		writeCompetitionError(c, http.StatusUnauthorized, "security_ablation", "missing authenticated user identity")
		return
	}
	if userID != configuredSecurityEvaluationUserID() {
		writeCompetitionError(c, http.StatusForbidden, "security_ablation", "authenticated user is outside the security evaluation scope")
		return
	}

	var req SecurityAblationEvaluationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeCompetitionError(c, http.StatusBadRequest, "security_ablation", "invalid request: "+err.Error())
		return
	}
	if !req.Synthetic || !syntheticSecuritySampleIDPattern.MatchString(req.SampleID) {
		writeCompetitionError(c, http.StatusBadRequest, "security_ablation", "security lab accepts only explicit synthetic samples with a synthetic_ sample_id")
		return
	}
	if message := strings.TrimSpace(req.Message); message == "" || len(message) > 4096 {
		writeCompetitionError(c, http.StatusBadRequest, "security_ablation", "message must contain 1 to 4096 bytes")
		return
	}
	securityService := h.securityEvaluationService()
	if securityService == nil {
		writeCompetitionUnavailable(c, "security_ablation", "security evaluation pipeline is unavailable")
		return
	}

	results := make([]security.SecurityProfileEvaluation, 0, len(security.SecurityLabProfiles))
	for _, profile := range security.SecurityLabProfiles {
		result, err := securityService.EvaluateSecurityProfile(c.Request.Context(), profile, req.Message)
		if err != nil {
			writeCompetitionUnavailable(c, "security_ablation", err.Error())
			return
		}
		results = append(results, result)
	}
	completed := time.Now().UTC()
	traceID := competitionTraceID(c)
	c.JSON(http.StatusOK, APIResponse{Success: true, Data: gin.H{
		"trace_id": traceID, "sample_id": req.SampleID, "synthetic": true,
		"started_at": started.Format(time.RFC3339Nano), "completed_at": completed.Format(time.RFC3339Nano),
		"latency_ms":       float64(completed.Sub(started).Microseconds()) / 1000,
		"pipeline_version": security.SecurityLabPipelineVersion,
		"profiles":         results, "memory_write_applied": false, "generation_skipped": true,
	}})
}
