package middleware

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// JWT配置
var (
	jwtExpiry = 24 * time.Hour // Token有效期24小时
)

// jwtSigningKey resolves the secret at signing and validation time. This keeps
// the process configuration authoritative and makes credential configuration
// testable without relying on package initialization order.
func jwtSigningKey() []byte {
	return []byte(getEnvOrDefault("JWT_SECRET", ""))
}

// Claims JWT声明
type Claims struct {
	UserID      string `json:"user_id"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	Role        string `json:"role,omitempty"` // 用户角色：caregiver/doctor/family/elder
	jwt.RegisteredClaims
}

// GenerateToken 生成JWT Token
func GenerateToken(userID, workspaceID string) (string, error) {
	return GenerateTokenWithRole(userID, workspaceID, "")
}

// GenerateTokenWithRole 生成带角色的JWT Token
func GenerateTokenWithRole(userID, workspaceID, role string) (string, error) {
	jwtSecret := jwtSigningKey()
	if len(jwtSecret) == 0 {
		return "", fmt.Errorf("JWT_SECRET is not configured")
	}

	claims := Claims{
		UserID:      userID,
		WorkspaceID: workspaceID,
		Role:        role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(jwtExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			// Include a per-token identifier so two tokens minted in the same
			// second are still distinct and independently revocable/auditable.
			ID:     fmt.Sprintf("%d", time.Now().UnixNano()),
			Issuer: "context-keeper",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// ParseToken 解析JWT Token
func ParseToken(tokenString string) (*Claims, error) {
	jwtSecret := jwtSigningKey()
	if len(jwtSecret) == 0 {
		return nil, fmt.Errorf("JWT_SECRET is not configured")
	}

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// 验证签名方法
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return jwtSecret, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}

// JWTAuth JWT认证中间件
func JWTAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从Header获取Token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "缺少Authorization header",
			})
			c.Abort()
			return
		}

		// 验证Bearer格式
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Authorization格式错误，应为: Bearer {token}",
			})
			c.Abort()
			return
		}

		tokenString := parts[1]

		// 解析Token
		claims, err := ParseToken(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   fmt.Sprintf("Token验证失败: %v", err),
			})
			c.Abort()
			return
		}

		// 将用户信息存入上下文
		c.Set("user_id", claims.UserID)
		c.Set("workspace_id", claims.WorkspaceID)
		c.Set("role", claims.Role)

		c.Next()
	}
}

// OptionalJWTAuth 可选JWT认证（用于兼容旧接口）
func OptionalJWTAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && parts[0] == "Bearer" {
				claims, err := ParseToken(parts[1])
				if err == nil {
					c.Set("user_id", claims.UserID)
					c.Set("workspace_id", claims.WorkspaceID)
				}
			}
		}
		c.Next()
	}
}

// GetUserID 从上下文获取用户ID
func GetUserID(c *gin.Context) (string, bool) {
	userID, exists := c.Get("user_id")
	if !exists {
		return "", false
	}
	return userID.(string), true
}

// GetWorkspaceID 从上下文获取工作空间ID
func GetWorkspaceID(c *gin.Context) (string, bool) {
	workspaceID, exists := c.Get("workspace_id")
	if !exists {
		return "", false
	}
	return workspaceID.(string), true
}

// GetRole 从上下文获取用户角色
func GetRole(c *gin.Context) (string, bool) {
	role, exists := c.Get("role")
	if !exists {
		return "", false
	}
	return role.(string), true
}

// getEnvOrDefault 获取环境变量或默认值
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
