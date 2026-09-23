package api

import (
	"strings"
	"testing"
)

func TestImageOCRUnavailableFeedbackDoesNotExposeRuntimeConfiguration(t *testing.T) {
	feedback := imageOCRUnavailableFeedback("lab-report.png")

	for _, forbidden := range []string{"Tesseract", "tessdata", "PATH", "C:\\"} {
		if strings.Contains(strings.ToLower(feedback), strings.ToLower(forbidden)) {
			t.Fatalf("feedback exposed runtime detail %q: %q", forbidden, feedback)
		}
	}
	for _, expected := range []string{"lab-report.png", "暂未识别", "手动"} {
		if !strings.Contains(strings.ToLower(feedback), strings.ToLower(expected)) {
			t.Fatalf("feedback missing %q: %q", expected, feedback)
		}
	}
}
