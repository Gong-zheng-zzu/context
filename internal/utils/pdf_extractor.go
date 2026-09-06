package utils

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/ledongthuc/pdf"
)

// ExtractTextFromPDF 从PDF文件中提取文本内容
// 参数:
//   - filePath: PDF文件的完整路径
// 返回:
//   - string: 提取的文本内容（限制最大10000字符）
//   - error: 错误信息（如果有）
func ExtractTextFromPDF(filePath string) (string, error) {
	// 打开PDF文件
	f, r, err := pdf.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("无法打开PDF文件: %w", err)
	}
	defer f.Close()

	// 获取总页数
	totalPages := r.NumPage()
	if totalPages == 0 {
		return "", fmt.Errorf("PDF文件没有页面")
	}

	const maxChars = 5000 // 限制最大字符数，避免LLM超时
	var textBuilder strings.Builder
	textBuilder.WriteString(fmt.Sprintf("PDF文档共 %d 页（为避免处理超时，仅显示前5000字符）\n\n", totalPages))

	// 遍历每一页提取文本
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		// 检查是否已达到字符限制
		if textBuilder.Len() >= maxChars {
			textBuilder.WriteString(fmt.Sprintf("\n\n[已达到字符限制，剩余 %d 页未显示]", totalPages-pageNum+1))
			break
		}

		page := r.Page(pageNum)
		if page.V.IsNull() {
			continue
		}

		// 提取页面文本
		text, err := extractPageText(page)
		if err != nil {
			// 如果某一页提取失败，记录但继续处理其他页
			textBuilder.WriteString(fmt.Sprintf("[第 %d 页提取失败: %v]\n\n", pageNum, err))
			continue
		}

		// 清理和格式化文本
		cleanedText := cleanText(text)
		if len(cleanedText) > 0 {
			textBuilder.WriteString(fmt.Sprintf("--- 第 %d 页 ---\n", pageNum))

			// 检查添加这一页后是否会超过限制
			if textBuilder.Len()+len(cleanedText) > maxChars {
				remaining := maxChars - textBuilder.Len()
				if remaining > 100 {
					textBuilder.WriteString(cleanedText[:remaining])
					textBuilder.WriteString("...\n\n[已达到字符限制]")
				}
				break
			}

			textBuilder.WriteString(cleanedText)
			textBuilder.WriteString("\n\n")
		}
	}

	result := textBuilder.String()
	if len(strings.TrimSpace(result)) == 0 {
		return "", fmt.Errorf("PDF文件中未提取到任何文本内容")
	}

	return result, nil
}

// extractPageText 从PDF页面中提取文本
func extractPageText(page pdf.Page) (string, error) {
	var textBuilder strings.Builder

	// 获取页面内容
	content := page.Content()
	if content.Text == nil {
		return "", nil
	}

	// 遍历文本对象
	for _, text := range content.Text {
		textBuilder.WriteString(text.S)
	}

	return textBuilder.String(), nil
}

// cleanText 清理和格式化提取的文本
func cleanText(text string) string {
	// 移除多余的空白字符
	text = strings.TrimSpace(text)

	// 将多个连续空格替换为单个空格
	text = strings.Join(strings.Fields(text), " ")

	// 处理常见的PDF文本问题
	// 有些PDF会在每个字符之间插入空格，这里尝试修复
	text = strings.ReplaceAll(text, "  ", " ")

	return text
}

// GetPDFInfo 获取PDF文件的基本信息（可选功能）
func GetPDFInfo(filePath string) (map[string]interface{}, error) {
	f, r, err := pdf.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("无法打开PDF文件: %w", err)
	}
	defer f.Close()

	info := make(map[string]interface{})
	info["pages"] = r.NumPage()

	// 尝试获取PDF元数据
	trailer := r.Trailer()
	if !trailer.IsNull() {
		infoDict := trailer.Key("Info")
		if !infoDict.IsNull() {
			// 提取标题
			if title := infoDict.Key("Title"); !title.IsNull() {
				info["title"] = title.String()
			}
			// 提取作者
			if author := infoDict.Key("Author"); !author.IsNull() {
				info["author"] = author.String()
			}
			// 提取创建日期
			if creationDate := infoDict.Key("CreationDate"); !creationDate.IsNull() {
				info["creation_date"] = creationDate.String()
			}
		}
	}

	return info, nil
}

// ReadPDFContent 读取PDF文件内容的辅助函数（用于调试）
func ReadPDFContent(reader io.Reader) (string, error) {
	buf := new(bytes.Buffer)
	_, err := buf.ReadFrom(reader)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}
