package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCompetitionEvidenceSummaryFailsClosedForTemplateManifest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	manifest := filepath.Join(root, "manifest.json")
	if err := os.WriteFile(manifest, []byte(`{"schema_version":"annotation-manifest-v1","status":"template_only","eligible_for_formal_report":false,"report_eligible":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COMPETITION_EVIDENCE_MANIFEST", manifest)

	router := gin.New()
	router.GET("/summary", (&Handler{}).HandleCompetitionEvidenceSummary)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/summary", nil)
	request.Header.Set("X-Trace-ID", "summary-trace-001")
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request
	context.Set("user_id", "evaluation-user")
	(&Handler{}).HandleCompetitionEvidenceSummary(context)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, expected := range []string{`"report_eligible":false`, `"manifest_status":"template_only"`, `"evidence_level":"controlled_preliminary"`, `"trace_id"`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("summary missing %s: %s", expected, body)
		}
	}
}

func TestCompetitionEvidenceSummaryMarksEligibleManifest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	manifest := filepath.Join(root, "manifest.json")
	if err := os.WriteFile(manifest, []byte(`{"schema_version":"annotation-manifest-v1","status":"eligible","eligible_for_formal_report":true,"report_eligible":true,"dataset_sha256":"0123456789abcdef"}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COMPETITION_EVIDENCE_MANIFEST", manifest)

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/summary", nil)
	context.Set("user_id", "evaluation-user")
	(&Handler{}).HandleCompetitionEvidenceSummary(context)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"report_eligible":true`) || !strings.Contains(recorder.Body.String(), `"evidence_level":"verified"`) {
		t.Fatalf("eligible summary = %d %s", recorder.Code, recorder.Body.String())
	}
}
