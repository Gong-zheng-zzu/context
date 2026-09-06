package security

import (
	"testing"
)

func TestConfidenceScorer_ScoreResponse(t *testing.T) {
	cs := NewConfidenceScorer()

	t.Log("╔══════════════════════════════════════════════════════════╗")
	t.Log("║  测试[1]: AI响应置信度评分                               ║")
	t.Log("╠══════════════════════════════════════════════════════════╣")
	t.Log("║  评分维度:                                               ║")
	t.Log("║  ① 响应长度与结构  ② 知识数据引用  ③ 具体数据引用      ║")
	t.Log("║  ④ 不确定性词汇    ⑤ 矛盾表述检测  ⑥ 敏感信息泄露     ║")
	t.Log("╚══════════════════════════════════════════════════════════╝")

	tests := []struct {
		name             string
		response         string
		hasKnowledgeData bool
		expectMinScore   float64
		expectMaxScore   float64
	}{
		{
			name:             "高质量响应",
			response:         "根据2023年研究显示，正常血压范围是120/80 mmHg。建议您：1. 定期监测血压 2. 保持健康饮食 3. 适量运动",
			hasKnowledgeData: true,
			expectMinScore:   0.8,
			expectMaxScore:   1.0,
		},
		{
			name:             "不确定响应",
			response:         "我不太确定，可能是这样，也许需要进一步检查。",
			hasKnowledgeData: false,
			expectMinScore:   0.0,
			expectMaxScore:   0.6,
		},
		{
			name:             "过短响应",
			response:         "好的",
			hasKnowledgeData: false,
			expectMinScore:   0.0,
			expectMaxScore:   0.7,
		},
		{
			name:             "有来源引用",
			response:         "根据《中国高血压防治指南》，高血压定义为收缩压≥140 mmHg或舒张压≥90 mmHg。",
			hasKnowledgeData: true,
			expectMinScore:   0.75,
			expectMaxScore:   1.0,
		},
		{
			name:             "包含具体数据",
			response:         "您的血糖值为5.6 mmol/L，心率为72 bpm，血压为118/75 mmHg，均在正常范围内。",
			hasKnowledgeData: false,
			expectMinScore:   0.7,
			expectMaxScore:   1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := cs.ScoreResponse(tt.response, tt.hasKnowledgeData)

			t.Logf("响应: %s", tt.response)
			t.Logf("置信度: %.2f", score)

			if score < tt.expectMinScore || score > tt.expectMaxScore {
				t.Errorf("置信度超出预期范围: 期望[%.2f, %.2f]，实际=%.2f",
					tt.expectMinScore, tt.expectMaxScore, score)
			}
		})
	}
}

func TestConfidenceScorer_ScoreWithDetails(t *testing.T) {
	cs := NewConfidenceScorer()

	response := "根据研究显示，您的血压120/80 mmHg属于正常范围。建议：1. 定期监测 2. 健康饮食"
	score, details := cs.ScoreWithDetails(response, true)

	t.Logf("置信度: %.2f", score)
	t.Logf("详细信息: %v", details)

	if score < 0.7 {
		t.Errorf("高质量响应的置信度应该>=0.7，实际=%.2f", score)
	}

	if details["confidence_level"] == nil {
		t.Errorf("应该包含置信度等级")
	}
}

func TestConfidenceScorer_GetConfidenceLevel(t *testing.T) {
	cs := NewConfidenceScorer()

	tests := []struct {
		score         float64
		expectLevel   string
	}{
		{0.95, "非常高"},
		{0.85, "高"},
		{0.75, "较高"},
		{0.65, "中等"},
		{0.55, "较低"},
		{0.45, "低"},
	}

	for _, tt := range tests {
		level := cs.GetConfidenceLevel(tt.score)
		t.Logf("分数: %.2f, 等级: %s", tt.score, level)

		if level != tt.expectLevel {
			t.Errorf("期望等级=%s，实际=%s", tt.expectLevel, level)
		}
	}
}

func TestConfidenceScorer_DetectSensitiveLeakage(t *testing.T) {
	cs := NewConfidenceScorer()

	// 包含敏感信息的响应
	response := "您的API密钥是 api_key=sk_test_1234567890abcdef"
	score := cs.ScoreResponse(response, false)

	t.Logf("包含敏感信息的响应置信度: %.2f", score)

	// 置信度应该被降低
	if score > 0.7 {
		t.Errorf("包含敏感信息的响应置信度应该较低，实际=%.2f", score)
	}
}

func TestConfidenceScorer_DetectContradiction(t *testing.T) {
	cs := NewConfidenceScorer()

	// 包含矛盾的响应
	response := "血压应该升高，但同时也应该降低。"
	score := cs.ScoreResponse(response, false)

	t.Logf("包含矛盾的响应置信度: %.2f", score)

	// 置信度应该被降低
	if score > 0.7 {
		t.Errorf("包含矛盾的响应置信度应该较低，实际=%.2f", score)
	}
}
