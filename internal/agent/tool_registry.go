package agent

import (
	"context"
	"fmt"
	"strings"
)

// Tool Agent可调用的工具接口
type Tool interface {
	Name() string
	Description() string
	Execute(ctx context.Context, input string) (string, error)
}

// ToolRegistry 工具注册表
type ToolRegistry struct {
	tools map[string]Tool
}

// NewToolRegistry 创建工具注册表
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{tools: make(map[string]Tool)}
}

// Register 注册工具
func (r *ToolRegistry) Register(tool Tool) {
	r.tools[tool.Name()] = tool
}

// Get 获取工具
func (r *ToolRegistry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// Execute 执行工具
func (r *ToolRegistry) Execute(ctx context.Context, name, input string) (string, error) {
	t, ok := r.tools[name]
	if !ok {
		return "", fmt.Errorf("工具 '%s' 不存在，可用工具: %s", name, r.ListNames())
	}
	return t.Execute(ctx, input)
}

// ListNames 列出所有工具名
func (r *ToolRegistry) ListNames() string {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return strings.Join(names, ", ")
}

// ToolDescriptions 返回所有工具的描述，用于注入prompt
func (r *ToolRegistry) ToolDescriptions() string {
	var sb strings.Builder
	for _, tool := range r.tools {
		fmt.Fprintf(&sb, "- %s: %s\n", tool.Name(), tool.Description())
	}
	return sb.String()
}
