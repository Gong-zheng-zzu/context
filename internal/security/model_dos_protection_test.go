package security

import (
	"testing"
	"time"
)

func TestModelDosProtection_CheckLimit(t *testing.T) {
	mdp := NewModelDosProtection()

	t.Log("╔══════════════════════════════════════════════════════════╗")
	t.Log("║  测试[1]: 模型DoS防护 - 请求频率限制                     ║")
	t.Log("╠══════════════════════════════════════════════════════════╣")
	t.Log("║  限制策略:                                               ║")
	t.Log("║  ① 单次请求Token上限: 2000                              ║")
	t.Log("║  ② 每分钟最大请求数: 20次                               ║")
	t.Log("║  ③ 每分钟最大Token: 10000                               ║")
	t.Log("║  ④ 请求最小间隔: 1秒                                    ║")
	t.Log("╚══════════════════════════════════════════════════════════╝")

	userID := "test_user_1"

	// 测试1: 首次请求应该通过
	err := mdp.CheckLimit(userID, 100)
	if err != nil {
		t.Errorf("首次请求应该通过，但返回错误: %v", err)
	}

	// 测试2: 单次请求Token超限
	err = mdp.CheckLimit(userID, 3000)
	if err == nil {
		t.Errorf("单次请求Token超限应该被拒绝")
	}
	t.Logf("单次Token超限错误: %v", err)
	time.Sleep(1100 * time.Millisecond) // 等待速率限制重置

	// 测试3: 正常请求并记录（每次间隔1.1秒以满足1秒最小间隔要求）
	for i := 0; i < 5; i++ {
		err := mdp.CheckLimit(userID, 100)
		if err != nil {
			t.Errorf("第%d次请求失败: %v", i+1, err)
		}
		mdp.RecordRequest(userID, 100)
		time.Sleep(1100 * time.Millisecond)
	}

	// 测试4: 超过请求频率限制（失败时也记录时间以保持LastRequest更新）
	for i := 0; i < 10; i++ {
		err := mdp.CheckLimit(userID, 100)
		mdp.RecordRequest(userID, 100)
		if err != nil {
			t.Logf("达到频率限制: %v", err)
			break
		}
	}
}

func TestModelDosProtection_GetUserStats(t *testing.T) {
	mdp := NewModelDosProtection()

	userID := "test_user_2"

	// 记录一些请求
	for i := 0; i < 5; i++ {
		mdp.RecordRequest(userID, 200)
		time.Sleep(50 * time.Millisecond)
	}

	// 获取统计
	requests, tokens, windowStart, exists := mdp.GetUserStats(userID)
	if !exists {
		t.Errorf("用户统计应该存在")
	}

	t.Logf("用户统计: 请求=%d, Tokens=%d, 窗口开始=%v", requests, tokens, windowStart)

	if requests != 5 {
		t.Errorf("期望请求数=5，实际=%d", requests)
	}

	if tokens != 1000 {
		t.Errorf("期望Token数=1000，实际=%d", tokens)
	}
}

func TestModelDosProtection_GetRemainingQuota(t *testing.T) {
	mdp := NewModelDosProtection()

	userID := "test_user_3"

	// 初始配额
	remainingReq, remainingTokens := mdp.GetRemainingQuota(userID)
	t.Logf("初始配额: 请求=%d, Tokens=%d", remainingReq, remainingTokens)

	if remainingReq != 20 {
		t.Errorf("期望初始请求配额=20，实际=%d", remainingReq)
	}

	// 使用一些配额
	mdp.RecordRequest(userID, 500)
	mdp.RecordRequest(userID, 500)

	remainingReq, remainingTokens = mdp.GetRemainingQuota(userID)
	t.Logf("使用后配额: 请求=%d, Tokens=%d", remainingReq, remainingTokens)

	if remainingReq != 18 {
		t.Errorf("期望剩余请求=18，实际=%d", remainingReq)
	}

	if remainingTokens != 9000 {
		t.Errorf("期望剩余Tokens=9000，实际=%d", remainingTokens)
	}
}

func TestModelDosProtection_ResetUserQuota(t *testing.T) {
	mdp := NewModelDosProtection()

	userID := "test_user_4"

	// 记录请求
	mdp.RecordRequest(userID, 500)

	// 重置配额
	mdp.ResetUserQuota(userID)

	// 检查是否重置
	_, _, _, exists := mdp.GetUserStats(userID)
	if exists {
		t.Errorf("重置后用户统计应该不存在")
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		minToken int
	}{
		{
			name:     "英文文本",
			text:     "Hello, how are you today?",
			minToken: 5,
		},
		{
			name:     "中文文本",
			text:     "你好，今天天气怎么样？",
			minToken: 5,
		},
		{
			name:     "混合文本",
			text:     "Hello 你好 World 世界",
			minToken: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := EstimateTokens(tt.text)
			t.Logf("文本: %s, 估算Tokens: %d", tt.text, tokens)

			if tokens < tt.minToken {
				t.Errorf("估算Token数过低: 期望>=%d，实际=%d", tt.minToken, tokens)
			}
		})
	}
}
