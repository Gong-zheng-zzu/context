package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ExtractTextFromImage 从图片文件中提取文本（OCR功能）
// 参数:
//   - filePath: 图片文件的完整路径
//
// 返回:
//   - string: 提取的文本内容
//   - error: 错误信息（如果有）
//
// 实现方案：使用本地Ollama的视觉模型（如果可用）或返回图片描述
func ExtractTextFromImage(filePath string) (string, error) {
	// 验证文件扩展名
	ext := strings.ToLower(filepath.Ext(filePath))
	supportedExts := []string{".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp"}

	isSupported := false
	for _, supportedExt := range supportedExts {
		if ext == supportedExt {
			isSupported = true
			break
		}
	}

	if !isSupported {
		return "", fmt.Errorf("不支持的图片格式: %s", ext)
	}

	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return "", fmt.Errorf("图片文件不存在: %s", filePath)
	}

	fileName := filepath.Base(filePath)

	// 尝试使用Ollama视觉模型进行OCR
	ollamaHost := os.Getenv("OLLAMA_HOST")
	if ollamaHost == "" {
		ollamaHost = "http://localhost:11434"
	}

	// 尝试使用llava或其他视觉模型
	visionModels := []string{"llava:latest", "llava:7b", "llava:13b", "bakllava:latest"}

	for _, model := range visionModels {
		text, err := extractWithOllamaVision(filePath, model, ollamaHost)
		if err == nil && text != "" {
			return text, nil
		}
	}

	// 如果没有可用的视觉模型，返回友好的提示信息
	placeholder := fmt.Sprintf("[图片文件: %s]\n", fileName)
	placeholder += "提示: 当前系统未配置OCR功能。\n"
	placeholder += "建议方案:\n"
	placeholder += "1. 安装Ollama视觉模型: ollama pull llava\n"
	placeholder += "2. 或者手动描述图片内容\n"
	placeholder += "[图片内容暂时无法自动识别，请手动输入图片中的信息]"

	return placeholder, nil
}

// extractWithOllamaVision 使用Ollama视觉模型提取图片文本
func extractWithOllamaVision(imagePath, model, ollamaHost string) (string, error) {
	// 读取图片文件
	imageData, err := os.ReadFile(imagePath)
	if err != nil {
		return "", fmt.Errorf("读取图片失败: %w", err)
	}

	// 构建Ollama API请求
	requestBody := map[string]interface{}{
		"model":  model,
		"prompt": "请仔细识别这张图片中的所有文字内容，包括数字、符号等。如果是健康数据（如血压、体温、心率等），请准确提取数值。请只返回识别到的文字，不要添加额外说明。",
		"images": []string{encodeImageToBase64(imageData)},
		"stream": false,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("构建请求失败: %w", err)
	}

	// 发送请求
	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	resp, err := client.Post(
		fmt.Sprintf("%s/api/generate", ollamaHost),
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return "", fmt.Errorf("请求Ollama失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Ollama返回错误: %s, 响应: %s", resp.Status, string(body))
	}

	// 解析响应
	var result struct {
		Response string `json:"response"`
		Done     bool   `json:"done"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}

	if result.Response == "" {
		return "", fmt.Errorf("模型未返回内容")
	}

	return fmt.Sprintf("[图片识别结果]\n%s\n[识别结束]", result.Response), nil
}

// encodeImageToBase64 将图片数据编码为base64
func encodeImageToBase64(imageData []byte) string {
	// Ollama API接受原始base64编码
	return base64Encode(imageData)
}

// base64Encode 简单的base64编码实现
func base64Encode(data []byte) string {
	const base64Table = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var result strings.Builder

	for i := 0; i < len(data); i += 3 {
		b1 := data[i]
		var b2, b3 byte

		if i+1 < len(data) {
			b2 = data[i+1]
		}
		if i+2 < len(data) {
			b3 = data[i+2]
		}

		result.WriteByte(base64Table[b1>>2])
		result.WriteByte(base64Table[((b1&0x03)<<4)|(b2>>4)])

		if i+1 < len(data) {
			result.WriteByte(base64Table[((b2&0x0F)<<2)|(b3>>6)])
		} else {
			result.WriteByte('=')
		}

		if i+2 < len(data) {
			result.WriteByte(base64Table[b3&0x3F])
		} else {
			result.WriteByte('=')
		}
	}

	return result.String()
}

// IsImageFile 判断文件是否为支持的图片格式
func IsImageFile(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	supportedExts := []string{".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp"}

	for _, supportedExt := range supportedExts {
		if ext == supportedExt {
			return true
		}
	}
	return false
}

// 以下是预留的OCR实现接口，供将来扩展使用

// OCRConfig OCR配置结构
type OCRConfig struct {
	Provider  string // OCR提供商: "ollama_vision", "baidu", "tesseract", "google", etc.
	APIKey    string // API密钥（如果需要）
	APISecret string // API密钥（如果需要）
	Language  string // 识别语言: "chi_sim"(简体中文), "eng"(英文), etc.
	Model     string // 模型名称（用于Ollama等）
}

// ExtractTextFromImageWithConfig 使用指定配置从图片中提取文本（预留接口）
func ExtractTextFromImageWithConfig(filePath string, config OCRConfig) (string, error) {
	// 根据config.Provider选择不同的OCR实现
	switch config.Provider {
	case "ollama_vision":
		ollamaHost := os.Getenv("OLLAMA_HOST")
		if ollamaHost == "" {
			ollamaHost = "http://localhost:11434"
		}
		model := config.Model
		if model == "" {
			model = "llava:latest"
		}
		return extractWithOllamaVision(filePath, model, ollamaHost)
	case "baidu":
		return extractWithBaiduOCR(filePath, config)
	case "tesseract":
		return extractWithTesseract(filePath, config)
	default:
		return "", fmt.Errorf("不支持的OCR提供商: %s", config.Provider)
	}
}

// extractWithBaiduOCR 使用百度OCR API提取文本（预留实现）
func extractWithBaiduOCR(filePath string, config OCRConfig) (string, error) {
	// TODO: 实现百度OCR API调用
	// 1. 读取图片文件
	// 2. Base64编码
	// 3. 调用百度OCR API
	// 4. 解析返回结果
	return "", fmt.Errorf("百度OCR功能尚未实现")
}

// extractWithTesseract 使用Tesseract OCR提取文本（预留实现）
func extractWithTesseract(filePath string, config OCRConfig) (string, error) {
	// TODO: 实现Tesseract OCR调用
	// 需要安装: github.com/otiai10/gosseract/v2
	// 1. 初始化Tesseract客户端
	// 2. 设置语言
	// 3. 识别图片
	// 4. 返回结果
	return "", fmt.Errorf("Tesseract OCR功能尚未实现")
}
