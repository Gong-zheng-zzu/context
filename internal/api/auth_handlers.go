package api

import (
	"net/http"

	"github.com/contextkeeper/service/internal/auth"
	"github.com/contextkeeper/service/internal/middleware"
	"github.com/gin-gonic/gin"
)

type LoginRequest struct {
	UserID      string `json:"user_id" binding:"required"`
	Password    string `json:"password" binding:"required"`
	WorkspaceID string `json:"workspace_id"`
}

type LoginResponse struct {
	Token       string `json:"token"`
	UserID      string `json:"user_id"`
	WorkspaceID string `json:"workspace_id"`
	ExpiresIn   int64  `json:"expires_in"`
}

func LoginHandler(c *gin.Context) {
	var req LoginRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{
			Success: false,
			Error:   "request validation failed: " + err.Error(),
		})
		return
	}

	if err := auth.ValidateCredentials(req.UserID, req.Password); err != nil {
		statusCode := http.StatusUnauthorized
		if err == auth.ErrAuthNotConfigured {
			statusCode = http.StatusServiceUnavailable
		}

		c.JSON(statusCode, APIResponse{
			Success: false,
			Error:   buildAuthErrorMessage(err),
		})
		return
	}

	token, err := middleware.GenerateToken(req.UserID, req.WorkspaceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{
			Success: false,
			Error:   "failed to generate token: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data: LoginResponse{
			Token:       token,
			UserID:      req.UserID,
			WorkspaceID: req.WorkspaceID,
			ExpiresIn:   86400,
		},
	})
}

func RefreshTokenHandler(c *gin.Context) {
	userID, exists := middleware.GetUserID(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, APIResponse{
			Success: false,
			Error:   "unauthorized",
		})
		return
	}

	workspaceID, _ := middleware.GetWorkspaceID(c)
	token, err := middleware.GenerateToken(userID, workspaceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{
			Success: false,
			Error:   "failed to refresh token: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data: LoginResponse{
			Token:       token,
			UserID:      userID,
			WorkspaceID: workspaceID,
			ExpiresIn:   86400,
		},
	})
}

func buildAuthErrorMessage(err error) string {
	switch err {
	case nil:
		return ""
	case auth.ErrAuthNotConfigured:
		return "demo auth not configured: " + auth.AuthConfigHint()
	case auth.ErrRoleMismatch:
		return "role does not match the authenticated user"
	default:
		return "invalid username or password"
	}
}
