package tools

import (
	"context"

	"github.com/contextkeeper/service/internal/bigdata"
	"github.com/contextkeeper/service/internal/services"
	"github.com/contextkeeper/service/internal/store"
)

// Deps Agent工具依赖的外部服务
type Deps struct {
	ContextService *services.ContextService
	SessionStore   *store.SessionStore
	VitalService   *bigdata.VitalSignService
	QueryService   *bigdata.UnifiedQueryService
	UserID         string // 当前用户ID，用于数据隔离
	SessionID      string // 当前会话ID
}

// contextKey 用于在context中传递Deps
type contextKey struct{}

// WithDeps 将Deps注入context
func WithDeps(ctx context.Context, deps *Deps) context.Context {
	return context.WithValue(ctx, contextKey{}, deps)
}

// GetDeps 从context中获取Deps
func GetDeps(ctx context.Context) *Deps {
	if deps, ok := ctx.Value(contextKey{}).(*Deps); ok {
		return deps
	}
	return nil
}
