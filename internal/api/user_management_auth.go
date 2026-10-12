package api

import (
	"net/http"
	"strings"

	"github.com/contextkeeper/service/internal/middleware"
	"github.com/gin-gonic/gin"
)

// authorizeUserManagement protects the user-management endpoints from being
// used as a cross-tenant profile or account administration API. Doctors and
// admins may manage users; ordinary roles may read only their own profile.
func authorizeUserManagement(c *gin.Context, targetUserID, operation string) bool {
	requester := authenticatedUserID(c)
	if requester == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "authenticated user is required"})
		return false
	}
	role, _ := middleware.GetRole(c)
	privileged := role == "doctor" || role == "admin"
	allowed := false
	switch strings.ToLower(operation) {
	case "read":
		allowed = privileged || requester == targetUserID
	case "write", "create":
		allowed = privileged
	}
	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "user management access is not permitted"})
		return false
	}
	return true
}
