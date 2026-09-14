package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// Tool Agent可调用的工具接口
type Tool interface {
	Name() string
	Description() string
	Execute(ctx context.Context, input string) (string, error)
}

// ReadOnlyTool is the capability contract required by a read-only registry.
// Tools must explicitly declare their mutability; omission is rejected.
type ReadOnlyTool interface {
	IsReadOnly() bool
}

// ToolRegistry 工具注册表
type ToolRegistry struct {
	tools     map[string]Tool
	allowlist map[string]struct{}
	readOnly  bool
	maxInput  int
}

// NewToolRegistry 创建工具注册表
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{tools: make(map[string]Tool), readOnly: true, maxInput: 4096}
}

// SetAllowlist restricts executable tools to the supplied names. An empty
// list keeps backwards compatibility while the read-only policy remains on.
func (r *ToolRegistry) SetAllowlist(names ...string) {
	r.allowlist = make(map[string]struct{}, len(names))
	for _, name := range names {
		if name = strings.TrimSpace(name); name != "" {
			r.allowlist[name] = struct{}{}
		}
	}
}

// SetReadOnly controls whether tools that explicitly advertise mutating
// behavior may execute. Production Agent registries should keep this true.
func (r *ToolRegistry) SetReadOnly(enabled bool) { r.readOnly = enabled }

// SetMaxInputBytes limits untrusted model-generated tool arguments.
func (r *ToolRegistry) SetMaxInputBytes(max int) {
	if max > 0 {
		r.maxInput = max
	}
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
	if len(r.allowlist) > 0 {
		if _, allowed := r.allowlist[name]; !allowed {
			return "", fmt.Errorf("工具 '%s' 不在当前Agent白名单中", name)
		}
	}
	if r.readOnly {
		capability, declared := t.(ReadOnlyTool)
		if !declared || !capability.IsReadOnly() {
			return "", fmt.Errorf("工具 '%s' 具有写入能力，当前Agent仅允许只读工具", name)
		}
	}
	if r.maxInput > 0 && len([]byte(input)) > r.maxInput {
		return "", fmt.Errorf("工具 '%s' 输入超过 %d 字节限制", name, r.maxInput)
	}
	return t.Execute(ctx, input)
}

// ListNames 列出所有工具名
func (r *ToolRegistry) ListNames() string {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// ToolDescriptions 返回所有工具的描述，用于注入prompt
func (r *ToolRegistry) ToolDescriptions() string {
	var sb strings.Builder
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		tool := r.tools[name]
		fmt.Fprintf(&sb, "- %s: %s\n", tool.Name(), tool.Description())
	}
	return sb.String()
}
