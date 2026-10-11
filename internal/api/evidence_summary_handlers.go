package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// competitionEvidenceSummary is deliberately a compact, server-owned view.
// It lets the competition console explain which PPT claims have evidence
// without exposing sample text, credentials, or client-controlled numbers.
type competitionEvidenceSummary struct {
	SchemaVersion     string                  `json:"schema_version"`
	EvidenceLevel     string                  `json:"evidence_level"`
	ReportEligible    bool                    `json:"report_eligible"`
	ManifestStatus    string                  `json:"manifest_status"`
	ManifestPath      string                  `json:"manifest_path"`
	ManifestSHA256    string                  `json:"manifest_sha256,omitempty"`
	DatasetSHA256     string                  `json:"dataset_sha256,omitempty"`
	ConfigFingerprint string                  `json:"config_fingerprint"`
	Capabilities      []competitionCapability `json:"capabilities"`
	Limitations       []string                `json:"limitations"`
	CheckedAt         time.Time               `json:"checked_at"`
}

type competitionCapability struct {
	Name           string `json:"name"`
	Status         string `json:"status"`
	EvidenceLevel  string `json:"evidence_level"`
	ReportEligible bool   `json:"report_eligible"`
	Reason         string `json:"reason"`
}

type competitionEvidenceManifest struct {
	SchemaVersion  string `json:"schema_version"`
	Status         string `json:"status"`
	Eligible       bool   `json:"eligible_for_formal_report"`
	DatasetSHA256  string `json:"dataset_sha256"`
	ReportEligible bool   `json:"report_eligible"`
}

func competitionEvidenceManifestPath() string {
	if configured := strings.TrimSpace(os.Getenv("COMPETITION_EVIDENCE_MANIFEST")); configured != "" {
		return configured
	}
	return filepath.FromSlash("experiments/datasets/annotation_manifest_v1.json")
}

func readCompetitionEvidenceSummary() competitionEvidenceSummary {
	path := competitionEvidenceManifestPath()
	summary := competitionEvidenceSummary{
		SchemaVersion:     "competition-evidence-summary-v1",
		EvidenceLevel:     "controlled_preliminary",
		ManifestStatus:    "unavailable",
		ManifestPath:      filepath.ToSlash(path),
		ConfigFingerprint: strings.TrimSpace(os.Getenv("COMPETITION_CONFIG_FINGERPRINT")),
		CheckedAt:         time.Now().UTC(),
		Capabilities: []competitionCapability{
			{Name: "security_asdf_casia_pccm", Status: "needs_review", EvidenceLevel: "controlled_preliminary", Reason: "formal security manifest is not eligible"},
			{Name: "causal_ompr", Status: "needs_review", EvidenceLevel: "historical_problem_baseline", Reason: "frozen causal evaluation evidence is not attached"},
			{Name: "audited_rrf", Status: "needs_review", EvidenceLevel: "controlled_preliminary", Reason: "retrieval ground truth must be reviewed before comparison"},
			{Name: "cross_store_deletion", Status: "needs_review", EvidenceLevel: "controlled_preliminary", Reason: "deletion proof is an operational verification, not model forgetting"},
			{Name: "research_agent", Status: "prototype", EvidenceLevel: "controlled_prototype", Reason: "read-only research extension; excluded from formal metrics"},
		},
		Limitations: []string{
			"未通过数据集、配置和运行记录门禁的数字不能作为正式比赛成绩。",
			"多模态、三维记忆、冷热分层和研究 Agent 仍按受控扩展展示。",
			"删除证明只表示隔离用户/会话跨存储删除验证，不代表参数级机器遗忘。",
		},
	}
	if summary.ConfigFingerprint == "" {
		summary.ConfigFingerprint = "runtime-unreported"
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		return summary
	}
	digest := sha256.Sum256(contents)
	summary.ManifestSHA256 = hex.EncodeToString(digest[:])
	var manifest competitionEvidenceManifest
	if json.Unmarshal(contents, &manifest) != nil {
		summary.ManifestStatus = "invalid"
		return summary
	}
	summary.DatasetSHA256 = manifest.DatasetSHA256
	summary.ManifestStatus = strings.TrimSpace(manifest.Status)
	if summary.ManifestStatus == "" {
		summary.ManifestStatus = "unverified"
	}
	if manifest.Eligible && manifest.ReportEligible && summary.ManifestStatus == "eligible" {
		summary.EvidenceLevel = "verified"
		summary.ReportEligible = true
		for i := range summary.Capabilities {
			if summary.Capabilities[i].Name == "research_agent" {
				continue
			}
			summary.Capabilities[i].Status = "verified"
			summary.Capabilities[i].EvidenceLevel = "verified"
			summary.Capabilities[i].ReportEligible = true
			summary.Capabilities[i].Reason = "server manifest is eligible; inspect the attached run artifact before publication"
		}
	}
	return summary
}

// HandleCompetitionEvidenceSummary returns server-owned evidence metadata.
// Authentication is provided by the protected route group in main_http.go.
func (h *Handler) HandleCompetitionEvidenceSummary(c *gin.Context) {
	started := time.Now()
	data := readCompetitionEvidenceSummary()
	completed := time.Now()
	rememberTraceEvidence(c, "evidence_summary", "ok", started, completed, gin.H{
		"manifest_status": data.ManifestStatus,
		"report_eligible": data.ReportEligible,
		"evidence_level":  data.EvidenceLevel,
		"capabilities":    len(data.Capabilities),
	})
	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"trace_id": competitionTraceID(c),
		"stage":    "evidence_summary",
		"status":   "ok",
		"data":     data,
	})
}
