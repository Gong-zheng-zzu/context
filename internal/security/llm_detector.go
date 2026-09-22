package security

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// LLMDetector LLM智能检测器
type LLMDetector struct {
	ollamaURL   string
	model       string
	timeout     time.Duration
	cache       *LLMCache
	enabled     bool
	mu          sync.RWMutex
	httpClient  *http.Client
}

// LLMCache LLM结果缓存
type LLMCache struct {
	data map[string]*LLMDetectionResponse
	mu   sync.RWMutex
	ttl  time.Duration
}

// LLMDetectionRequest LLM检测请求
type LLMDetectionRequest struct {
	Text         string    `json:"text"`
	MarkedRanges [][2]int  `json:"marked_ranges"` // 已检测区域
}

// LLMDetectionResponse LLM检测响应
type LLMDetectionResponse struct {
	Items     []LLMDetectionItem `json:"items"`
	Timestamp time.Time          `json:"timestamp"`
}

// LLMDetectionItem LLM检测项
type LLMDetectionItem struct {
	Type       string  `json:"type"`
	Start      int     `json:"start"`
	End        int     `json:"end"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

// OllamaRequest Ollama API请求
type OllamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
	Options map[string]interface{} `json:"options,omitempty"`
}

// OllamaResponse Ollama API响应
type OllamaResponse struct {
	Response string `json:"response"`
}

// NewLLMDetector 创建LLM检测器
func NewLLMDetector(ollamaURL, model string, timeout time.Duration) *LLMDetector {
	return &LLMDetector{
		ollamaURL: ollamaURL,
		model:     model,
		timeout:   timeout,
		cache: &LLMCache{
			data: make(map[string]*LLMDetectionResponse),
			ttl:  10 * time.Minute,
		},
		enabled: true,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// Detect 检测敏感信息
func (ld *LLMDetector) Detect(ctx context.Context, text string, markedRanges [][2]int) (*LLMDetectionResponse, error) {
	if !ld.enabled {
		return &LLMDetectionResponse{Items: []LLMDetectionItem{}}, nil
	}

	// 检查缓存
	cacheKey := ld.getCacheKey(text, markedRanges)
	if cached := ld.cache.Get(cacheKey); cached != nil {
		return cached, nil
	}

	// 分段处理（超过500字符）
	if len(text) > 500 {
		return ld.detectSegmented(ctx, text, markedRanges)
	}

	// 单段检测
	result, err := ld.detectSingle(ctx, text, markedRanges)
	if err != nil {
		return nil, err
	}

	// 缓存结果
	ld.cache.Set(cacheKey, result)

	return result, nil
}

// detectSingle 单段检测
func (ld *LLMDetector) detectSingle(ctx context.Context, text string, markedRanges [][2]int) (*LLMDetectionResponse, error) {
	// 构建Prompt
	prompt := ld.buildPrompt(text, markedRanges)

	// 调用Ollama
	response, err := ld.callOllama(ctx, prompt)
	if err != nil {
		return nil, err
	}

	// 解析响应
	result, err := ld.parseResponse(response)
	if err != nil {
		return nil, err
	}

	// 过滤低置信度结果
	filtered := ld.filterByConfidence(result, 0.7)

	return filtered, nil
}

// detectSegmented 分段检测
func (ld *LLMDetector) detectSegmented(ctx context.Context, text string, markedRanges [][2]int) (*LLMDetectionResponse, error) {
	segments := ld.segmentText(text, 500)

	// 并发检测各段
	type segmentResult struct {
		offset int
		result *LLMDetectionResponse
		err    error
	}

	resultChan := make(chan segmentResult, len(segments))
	var wg sync.WaitGroup

	for i, segment := range segments {
		wg.Add(1)
		go func(idx int, seg string, offset int) {
			defer wg.Done()

			// 调整markedRanges到当前段
			adjustedRanges := ld.adjustRanges(markedRanges, offset, offset+len(seg))

			result, err := ld.detectSingle(ctx, seg, adjustedRanges)
			resultChan <- segmentResult{
				offset: offset,
				result: result,
				err:    err,
			}
		}(i, segment.text, segment.offset)
	}

	// 等待所有goroutine完成
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// 合并结果
	merged := &LLMDetectionResponse{
		Items:     make([]LLMDetectionItem, 0),
		Timestamp: time.Now(),
	}

	for sr := range resultChan {
		if sr.err != nil {
			continue // 忽略错误的段
		}

		// 调整位置并合并
		for _, item := range sr.result.Items {
			item.Start += sr.offset
			item.End += sr.offset
			merged.Items = append(merged.Items, item)
		}
	}

	return merged, nil
}

// buildPrompt 构建检测Prompt
func (ld *LLMDetector) buildPrompt(text string, markedRanges [][2]int) string {
	markedRangesStr := "[]"
	if len(markedRanges) > 0 {
		rangesJSON, _ := json.Marshal(markedRanges)
		markedRangesStr = string(rangesJSON)
	}

	prompt := fmt.Sprintf(`你是敏感信息检测专家。分析文本中的隐晦敏感表达（隐喻、暗示、变体表达），返回JSON。

规则：
- 检测文本中隐含的、但没有直接写出具体数值的敏感信息（例如"手机号是170开头的十一位号码"里隐含了手机号）
- 忽略已标记位置：%s
- type 只能从下面选，不要发明新类型：
  id_card(身份证号)、phone(手机号)、medical_record(病历号)、blood_pressure(血压)、bank_card(银行卡号)、credit_card(信用卡号)、email(邮箱)、ip_address(IP地址)、password(密码)、api_key(API密钥)

文本：%s

输出格式（必须是有效的JSON）：
{"items":[{"type":"类型","confidence":0.0-1.0,"reason":"简短原因"}]}

如果无敏感内容，返回：{"items":[]}

注意：只返回JSON，不要有其他文字。不要输出start/end位置字段。`, markedRangesStr, text)

	return prompt
}

// callOllama 调用Ollama API
func (ld *LLMDetector) callOllama(ctx context.Context, prompt string) (string, error) {
	reqBody := OllamaRequest{
		Model:  ld.model,
		Prompt: prompt,
		Stream: false,
		Options: map[string]interface{}{
			"temperature": 0.1,
			"num_predict": 500,
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", ld.ollamaURL+"/api/generate", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := ld.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama API error: %d, %s", resp.StatusCode, string(body))
	}

	var ollamaResp OllamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return "", err
	}

	return ollamaResp.Response, nil
}

// parseResponse 解析LLM响应
func (ld *LLMDetector) parseResponse(response string) (*LLMDetectionResponse, error) {
	var result LLMDetectionResponse

	// 尝试提取JSON（LLM可能返回额外文字）
	jsonStart := -1
	jsonEnd := -1

	for i, ch := range response {
		if ch == '{' && jsonStart == -1 {
			jsonStart = i
		}
		if ch == '}' {
			jsonEnd = i + 1
		}
	}

	if jsonStart == -1 || jsonEnd == -1 {
		return &LLMDetectionResponse{Items: []LLMDetectionItem{}}, nil
	}

	jsonStr := response[jsonStart:jsonEnd]

	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return &LLMDetectionResponse{Items: []LLMDetectionItem{}}, nil
	}

	result.Timestamp = time.Now()
	return &result, nil
}

// filterByConfidence 按置信度过滤
func (ld *LLMDetector) filterByConfidence(result *LLMDetectionResponse, threshold float64) *LLMDetectionResponse {
	filtered := &LLMDetectionResponse{
		Items:     make([]LLMDetectionItem, 0),
		Timestamp: result.Timestamp,
	}

	for _, item := range result.Items {
		if item.Confidence >= threshold {
			filtered.Items = append(filtered.Items, item)
		}
	}

	return filtered
}

// segmentText 分段文本
func (ld *LLMDetector) segmentText(text string, maxLen int) []struct {
	text   string
	offset int
} {
	segments := make([]struct {
		text   string
		offset int
	}, 0)

	runes := []rune(text)
	for i := 0; i < len(runes); i += maxLen {
		end := i + maxLen
		if end > len(runes) {
			end = len(runes)
		}

		segments = append(segments, struct {
			text   string
			offset int
		}{
			text:   string(runes[i:end]),
			offset: i,
		})
	}

	return segments
}

// adjustRanges 调整范围到当前段
func (ld *LLMDetector) adjustRanges(ranges [][2]int, segStart, segEnd int) [][2]int {
	adjusted := make([][2]int, 0)

	for _, r := range ranges {
		// 如果范围与当前段有交集
		if r[1] > segStart && r[0] < segEnd {
			newStart := max(0, r[0]-segStart)
			newEnd := min(segEnd-segStart, r[1]-segStart)
			adjusted = append(adjusted, [2]int{newStart, newEnd})
		}
	}

	return adjusted
}

// getCacheKey 生成缓存键
func (ld *LLMDetector) getCacheKey(text string, markedRanges [][2]int) string {
	data := fmt.Sprintf("%s|%v", text, markedRanges)
	hash := md5.Sum([]byte(data))
	return fmt.Sprintf("%x", hash)
}

// Enable 启用LLM检测
func (ld *LLMDetector) Enable() {
	ld.mu.Lock()
	defer ld.mu.Unlock()
	ld.enabled = true
}

// Disable 禁用LLM检测
func (ld *LLMDetector) Disable() {
	ld.mu.Lock()
	defer ld.mu.Unlock()
	ld.enabled = false
}

// IsEnabled 检查是否启用
func (ld *LLMDetector) IsEnabled() bool {
	ld.mu.RLock()
	defer ld.mu.RUnlock()
	return ld.enabled
}

// ClearCache 清空缓存
func (ld *LLMDetector) ClearCache() {
	ld.cache.Clear()
}

// === LLMCache 方法 ===

// Get 获取缓存
func (lc *LLMCache) Get(key string) *LLMDetectionResponse {
	lc.mu.RLock()
	defer lc.mu.RUnlock()

	if result, ok := lc.data[key]; ok {
		// 检查是否过期
		if time.Since(result.Timestamp) < lc.ttl {
			return result
		}
		// 过期则删除
		delete(lc.data, key)
	}

	return nil
}

// Set 设置缓存
func (lc *LLMCache) Set(key string, result *LLMDetectionResponse) {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	lc.data[key] = result

	// 简单的缓存清理（如果超过1000条）
	if len(lc.data) > 1000 {
		// 删除最旧的一半
		count := 0
		for k := range lc.data {
			delete(lc.data, k)
			count++
			if count >= 500 {
				break
			}
		}
	}
}

// Clear 清空缓存
func (lc *LLMCache) Clear() {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	lc.data = make(map[string]*LLMDetectionResponse)
}
