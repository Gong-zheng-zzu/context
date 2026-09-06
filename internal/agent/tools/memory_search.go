package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/contextkeeper/service/internal/models"
)

// MemorySearchTool 记忆检索工具
type MemorySearchTool struct{}

func (t *MemorySearchTool) Name() string        { return "memory_search" }
func (t *MemorySearchTool) Description() string  { return "检索相关的记忆上下文，包括短期记忆、长期记忆和相关知识" }

func (t *MemorySearchTool) Execute(ctx context.Context, input string) (string, error) {
	deps := GetDeps(ctx)
	if deps == nil || deps.ContextService == nil {
		return "记忆服务不可用", nil
	}

	req := models.RetrieveContextRequest{
		SessionID: deps.SessionID,
		UserID:    deps.UserID,
		Query:     input,
		Limit:     2000,
		Strategy:  "balanced",
	}

	resp, err := deps.ContextService.RetrieveContext(ctx, req)
	if err != nil {
		return "", fmt.Errorf("记忆检索失败: %v", err)
	}

	var memories []string
	if resp.ShortTermMemory != "" {
		memories = append(memories, "【短期记忆】\n"+resp.ShortTermMemory)
	}
	if resp.LongTermMemory != "" {
		memories = append(memories, "【长期记忆】\n"+resp.LongTermMemory)
	}
	if resp.RelevantKnowledge != "" {
		memories = append(memories, "【相关知识】\n"+resp.RelevantKnowledge)
	}

	if len(memories) == 0 {
		return "未找到相关记忆", nil
	}
	return strings.Join(memories, "\n\n"), nil
}
