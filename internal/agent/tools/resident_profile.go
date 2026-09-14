package tools

import (
	"context"
	"fmt"
	"strings"
)

// ResidentProfileTool 居民档案查询工具
type ResidentProfileTool struct{}

func (t *ResidentProfileTool) IsReadOnly() bool { return true }

func (t *ResidentProfileTool) Name() string { return "resident_profile" }
func (t *ResidentProfileTool) Description() string {
	return "查询居民的基本信息档案，包括姓名、年龄、既往病史、过敏史等"
}

func (t *ResidentProfileTool) Execute(ctx context.Context, input string) (string, error) {
	deps := GetDeps(ctx)
	if deps == nil || deps.SessionStore == nil {
		return "会话存储服务不可用", nil
	}

	sessionID := strings.TrimSpace(input)
	if sessionID == "" {
		sessionID = deps.SessionID
	}

	session, err := deps.SessionStore.GetSession(sessionID)
	if err != nil {
		return generateDemoProfile(input), nil
	}

	if session == nil {
		return generateDemoProfile(input), nil
	}

	var info []string
	info = append(info, fmt.Sprintf("会话ID: %s", sessionID))
	if !session.CreatedAt.IsZero() {
		info = append(info, fmt.Sprintf("创建时间: %s", session.CreatedAt.Format("2006-01-02 15:04:05")))
	}

	return strings.Join(info, "\n"), nil
}

func generateDemoProfile(residentID string) string {
	return fmt.Sprintf(`居民档案信息（模拟数据）：
- 居民ID: %s
- 姓名: 张大爷
- 年龄: 78岁
- 性别: 男
- 床位: 3楼 305房 2号床
- 入住时间: 2024-01-15
- 既往病史: 高血压(II级)、2型糖尿病
- 过敏史: 青霉素过敏
- 紧急联系人: 张明（儿子）138-0000-1234
- 护理等级: 二级护理
- 饮食要求: 低盐低糖饮食`,
		residentID)
}
