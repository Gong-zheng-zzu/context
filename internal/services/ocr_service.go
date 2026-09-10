package services

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

// OCRService OCR文字识别服务
type OCRService struct {
	logger  *logrus.Logger
	binary  string
	execute func(string, ...string) ([]byte, error)
}

// NewOCRService 创建OCR服务实例
func NewOCRService(logger *logrus.Logger) *OCRService {
	binary := strings.TrimSpace(os.Getenv("TESSERACT_BIN"))
	if binary == "" {
		binary = "tesseract"
	}
	return &OCRService{
		logger:  logger,
		binary:  binary,
		execute: runOCRCommand,
	}
}

func runOCRCommand(binary string, args ...string) ([]byte, error) {
	return exec.Command(binary, args...).CombinedOutput()
}

// OCRRequest OCR识别请求
type OCRRequest struct {
	ImagePath string   // 图片路径
	Languages []string // 语言列表，如 ["chi_sim", "eng"]
	PSM       int      // 页面分割模式 (0-13)
}

// OCRResult OCR识别结果
type OCRResult struct {
	Text       string  // 识别的文本
	Confidence float64 // 置信度 (0-100)
	Language   string  // 使用的语言
}

// RecognizeText 识别图片中的文字
func (s *OCRService) RecognizeText(req OCRRequest) (*OCRResult, error) {
	// 验证文件存在
	if _, err := os.Stat(req.ImagePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("图片文件不存在: %s", req.ImagePath)
	}

	s.logger.Printf("🔍 [OCR] 开始识别图片: %s", filepath.Base(req.ImagePath))

	// 设置语言（默认中英文混合）
	languages := req.Languages
	if len(languages) == 0 {
		languages = []string{"chi_sim", "eng"}
	}
	// 设置页面分割模式
	// PSM 3: 全自动页面分割（默认）
	// PSM 6: 假设单个统一文本块
	// PSM 11: 稀疏文本，尽可能找到文字
	psm := req.PSM
	if psm == 0 {
		psm = 3 // 默认自动分割
	}
	if s.execute == nil {
		s.execute = runOCRCommand
	}
	output, err := s.execute(s.binary, req.ImagePath, "stdout", "-l", strings.Join(languages, "+"), "--psm", strconv.Itoa(psm))
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		s.logger.WithError(err).Error("OCR识别失败")
		return nil, fmt.Errorf("OCR识别失败（请安装 Tesseract 或设置 TESSERACT_BIN）: %s", message)
	}

	// Tesseract's plain-text mode does not expose a stable document-level
	// confidence. Returning zero makes that limitation explicit to callers.
	confidence := 0.0

	result := &OCRResult{
		Text:       strings.TrimSpace(string(output)),
		Confidence: float64(confidence),
		Language:   strings.Join(languages, "+"),
	}

	s.logger.Printf("✅ [OCR] 识别完成 - 置信度: %.2f%%, 文本长度: %d字符", confidence, len(result.Text))

	return result, nil
}

// RecognizeMedicalReport 识别医疗报告（针对医疗场景优化）
func (s *OCRService) RecognizeMedicalReport(imagePath string) (*OCRResult, error) {
	s.logger.Printf("🏥 [OCR] 识别医疗报告: %s", filepath.Base(imagePath))

	// 医疗报告通常是规整的文档，使用PSM 6
	req := OCRRequest{
		ImagePath: imagePath,
		Languages: []string{"chi_sim", "eng"}, // 中英文混合
		PSM:       6,                          // 单个文本块模式
	}

	return s.RecognizeText(req)
}

// GetSupportedLanguages 获取支持的语言列表
func (s *OCRService) GetSupportedLanguages() ([]string, error) {
	if s.execute == nil {
		s.execute = runOCRCommand
	}
	output, err := s.execute(s.binary, "--list-langs")
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return nil, fmt.Errorf("无法读取 Tesseract 语言包: %s", message)
	}

	lines := strings.Split(string(output), "\n")
	languages := make([]string, 0, len(lines))
	for _, line := range lines {
		language := strings.TrimSpace(line)
		if language == "" || strings.Contains(language, "List of available languages") {
			continue
		}
		languages = append(languages, language)
	}
	return languages, nil
}
