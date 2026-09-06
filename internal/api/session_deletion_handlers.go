package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/contextkeeper/service/internal/middleware"
	"github.com/contextkeeper/service/internal/models"
	"github.com/contextkeeper/service/internal/services"
	"github.com/contextkeeper/service/internal/store"
	"github.com/gin-gonic/gin"
)

// SessionDeletionExecutor allows the production cascade service to be tested
// independently from the JWT-protected HTTP boundary.
type SessionDeletionExecutor interface {
	Delete(ctx context.Context, userID, sessionID string, dryRun bool) (*services.SessionDeletionResult, error)
}

// SessionDeletionHandler deletes a session only for the JWT-authenticated owner.
// Register it exclusively on a route group that already has middleware.JWTAuth().
type SessionDeletionHandler struct {
	executor SessionDeletionExecutor
}

func NewSessionDeletionHandler(executor SessionDeletionExecutor) *SessionDeletionHandler {
	return &SessionDeletionHandler{executor: executor}
}

type productionSessionDeletionExecutor struct {
	resolveSessionStore func(userID string) (*store.SessionStore, error)
	vectorStore         models.VectorStore
	vectorCollection    string
}

func (e *productionSessionDeletionExecutor) Delete(ctx context.Context, userID, sessionID string, dryRun bool) (*services.SessionDeletionResult, error) {
	if e == nil || e.resolveSessionStore == nil {
		return nil, errors.New("session deletion is not configured")
	}

	sessionStore, err := e.resolveSessionStore(userID)
	if err != nil {
		return nil, err
	}
	// Fail ownership checks before opening replica connections. This keeps an
	// unavailable replica from masking an unauthorized deletion attempt.
	owner, err := sessionStore.GetSessionOwner(sessionID)
	if err != nil && !errors.Is(err, store.ErrSessionNotFound) {
		return nil, err
	}
	// Durable replicas are always filtered by the authenticated user and
	// session. A missing local artifact must not prevent cleanup of an orphaned
	// replica, but an existing session owned by somebody else remains forbidden.
	if err == nil && owner != userID {
		return nil, services.ErrSessionOwnerMismatch
	}

	var deletionService *services.SessionDeletionService
	var closeReplicas func()
	if e.vectorStore != nil {
		deletionService, closeReplicas, err = services.NewProductionSessionDeletionServiceWithVectorStore(sessionStore, e.vectorStore, e.vectorCollection)
	} else {
		deletionService, closeReplicas, err = services.NewProductionSessionDeletionService(sessionStore)
	}
	if err != nil {
		return nil, err
	}
	defer closeReplicas()
	return deletionService.Delete(ctx, userID, sessionID, dryRun)
}

// DeleteSession handles DELETE /api/sessions/:session_id?dry_run=true.
func (h *SessionDeletionHandler) DeleteSession(c *gin.Context) {
	if h == nil || h.executor == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"complete": false, "error": "session deletion is not configured"})
		return
	}

	userID, ok := c.Get("user_id")
	if !ok {
		// This is defense in depth. The protected route must reject this earlier.
		c.JSON(http.StatusUnauthorized, gin.H{"complete": false, "error": "authentication required"})
		return
	}
	owner, ok := userID.(string)
	if !ok || owner == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"complete": false, "error": "invalid authenticated user"})
		return
	}

	dryRun, err := strconv.ParseBool(c.DefaultQuery("dry_run", "false"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"complete": false, "error": "dry_run must be true or false"})
		return
	}

	result, err := h.executor.Delete(c.Request.Context(), owner, c.Param("session_id"), dryRun)
	if err == nil {
		c.JSON(http.StatusOK, gin.H{"complete": result.Complete, "result": result})
		return
	}

	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, services.ErrSessionOwnerMismatch):
		status = http.StatusForbidden
	case errors.Is(err, store.ErrSessionNotFound):
		status = http.StatusNotFound
	}
	c.JSON(status, gin.H{"complete": false, "result": result, "error": err.Error()})
}

// RegisterSessionDeletionRoutes is deliberately separate from generic route
// registration so the server can mount it only under its JWT-protected group.
func RegisterSessionDeletionRoutes(router gin.IRoutes, handler *SessionDeletionHandler) {
	router.DELETE("/sessions/:session_id", handler.DeleteSession)
}

// RegisterSessionDeletionRoutes mounts the production cascade beneath an explicit
// JWT middleware because RegisterManagementRoutes is called outside the server's
// protected group.
func (h *Handler) RegisterSessionDeletionRoutes(router *gin.Engine) {
	var vectorStore models.VectorStore
	if h != nil && h.contextService != nil {
		vectorStore = h.contextService.GetContextService().GetVectorStore()
	}
	vectorCollection := ""
	if h != nil && h.config != nil {
		vectorCollection = h.config.VectorDBCollection
	}

	// Chat and MCP context writes use the shared SessionStore. Ownership remains
	// enforced from the persisted userId metadata before any replica is touched.
	// Resolving a separate per-user store here made legitimate sessions invisible
	// to the cascade endpoint.
	executor := &productionSessionDeletionExecutor{
		resolveSessionStore: func(_ string) (*store.SessionStore, error) {
			if h == nil || h.contextService == nil || h.contextService.GetContextService() == nil {
				return nil, errors.New("context service is not configured")
			}
			return h.contextService.GetContextService().SessionStore(), nil
		},
		vectorStore:      vectorStore,
		vectorCollection: vectorCollection,
	}
	group := router.Group("/api")
	group.Use(middleware.JWTAuth())
	RegisterSessionDeletionRoutes(group, NewSessionDeletionHandler(executor))
}
