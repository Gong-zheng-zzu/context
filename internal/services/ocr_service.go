package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/otiai10/gosseract/v2"
	"github.com/sirupsen/logrus"
)

// OCRService OCR文字识别服务
type OCRService struct {
	logger *logrus.Logger
}

// NewOCRService 创建OCR服务实例
func NewOCRService(logger *logrus.Logger) *OCRService {
	return &OCRService{
		logger: logger,
	}
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

	// 创建Tesseract客户端
	client := gosseract.NewClient()
	defer client.Close()

	// 设置语言（默认中英文混合）
	languages := req.Languages
	if len(languages) == 0 {
		languages = []string{"chi_sim", "eng"}
	}
	client.SetLanguage(strings.Join(languages, "+"))

	// 设置页面分割模式
	// PSM 3: 全自动页面分割（默认）
	// PSM 6: 假设单个统一文本块
	// PSM 11: 稀疏文本，尽可能找到文字
	psm := req.PSM
	if psm == 0 {
		psm = 3 // 默认自动分割
	}
	client.SetPageSegMode(gosseract.PageSegMode(psm))

	// 设置图片路径
	if err := client.SetImage(req.ImagePath); err != nil {
		s.logger.WithError(err).Error("❌ [OCR] 设置图片失败")
		return nil, fmt.Errorf("设置图片失败: %w", err)
	}

	// 执行识别
	text, err := client.Text()
	if err != nil {
		s.logger.WithError(err).Error("❌ [OCR] 识别失败")
		return nil, fmt.Errorf("OCR识别失败: %w", err)
	}

	// 获取置信度（gosseract v2没有GetMeanConfidence方法）
	// confidence, err := client.GetMeanConfidence()
	// if err != nil {
	// 	s.logger.WithError(err).Warn("⚠️ [OCR] 获取置信度失败，使用默认值")
	// 	confidence = 0
	// }
	confidence := 0.0 // 默认置信度

	result := &OCRResult{
		Text:       strings.TrimSpace(text),
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
	client := gosseract.NewClient()
	defer client.Close()

	// gosseract v2没有GetAvailableLanguages方法，返回默认语言列表
	// langs, err := client.GetAvailableLanguages()
	// if err != nil {
	// 	return nil, fmt.Errorf("获取语言列表失败: %w", err)
	// }

	// 返回常用语言列表
	langs := []string{"eng", "chi_sim", "chi_tra", "jpn", "kor"}

	return langs, nil
}
