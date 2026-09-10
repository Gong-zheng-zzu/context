package services

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestRecognizeTextInvokesTesseractCLI(t *testing.T) {
	imagePath := filepath.Join(t.TempDir(), "report.png")
	if err := os.WriteFile(imagePath, []byte("image"), 0o600); err != nil {
		t.Fatalf("create image fixture: %v", err)
	}

	var command string
	var arguments []string
	service := &OCRService{
		logger: logrus.New(),
		binary: "tesseract-custom",
		execute: func(binary string, args ...string) ([]byte, error) {
			command = binary
			arguments = append([]string(nil), args...)
			return []byte("血压 128/80\n"), nil
		},
	}

	result, err := service.RecognizeText(OCRRequest{ImagePath: imagePath, Languages: []string{"chi_sim", "eng"}, PSM: 6})
	if err != nil {
		t.Fatalf("RecognizeText() error = %v", err)
	}
	if result.Text != "血压 128/80" || result.Language != "chi_sim+eng" {
		t.Fatalf("result = %+v", result)
	}
	if command != "tesseract-custom" {
		t.Fatalf("command = %q", command)
	}
	wantArgs := []string{imagePath, "stdout", "-l", "chi_sim+eng", "--psm", "6"}
	if !reflect.DeepEqual(arguments, wantArgs) {
		t.Fatalf("arguments = %#v, want %#v", arguments, wantArgs)
	}
}

func TestGetSupportedLanguagesReturnsCLIResult(t *testing.T) {
	service := &OCRService{
		logger: logrus.New(),
		binary: "tesseract",
		execute: func(_ string, args ...string) ([]byte, error) {
			if !reflect.DeepEqual(args, []string{"--list-langs"}) {
				t.Fatalf("args = %#v", args)
			}
			return []byte("List of available languages in /usr/share/tessdata (2):\nchi_sim\neng\n"), nil
		},
	}

	languages, err := service.GetSupportedLanguages()
	if err != nil {
		t.Fatalf("GetSupportedLanguages() error = %v", err)
	}
	if !reflect.DeepEqual(languages, []string{"chi_sim", "eng"}) {
		t.Fatalf("languages = %#v", languages)
	}
}

func TestRecognizeTextExplainsMissingTesseract(t *testing.T) {
	imagePath := filepath.Join(t.TempDir(), "report.png")
	if err := os.WriteFile(imagePath, []byte("image"), 0o600); err != nil {
		t.Fatalf("create image fixture: %v", err)
	}

	service := &OCRService{
		logger: logrus.New(),
		binary: "tesseract",
		execute: func(_ string, _ ...string) ([]byte, error) {
			return nil, errors.New("executable file not found")
		},
	}

	if _, err := service.RecognizeText(OCRRequest{ImagePath: imagePath}); err == nil {
		t.Fatal("expected missing Tesseract error")
	}
}
