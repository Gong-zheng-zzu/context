package api

import "fmt"

// imageOCRUnavailableFeedback is deliberately safe for the model prompt and
// user-visible response. OCR binary paths and model/runtime details belong in
// server logs, not in a nursing conversation.
func imageOCRUnavailableFeedback(fileName string) string {
	return fmt.Sprintf("[图片: %s - 图片文字暂未识别，已跳过图片内容。请提示用户手动填写关键数值或稍后重试。]", fileName)
}
