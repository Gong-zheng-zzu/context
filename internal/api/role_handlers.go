package api

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/contextkeeper/service/internal/auth"
	"github.com/contextkeeper/service/internal/bigdata"
	"github.com/contextkeeper/service/internal/middleware"
	"github.com/gin-gonic/gin"
)

type RoleLoginRequest struct {
	UserID      string `json:"user_id" binding:"required"`
	Password    string `json:"password" binding:"required"`
	Role        string `json:"role" binding:"required"`
	WorkspaceID string `json:"workspace_id"`
}

type RoleLoginResponse struct {
	Token       string `json:"token"`
	UserID      string `json:"user_id"`
	Role        string `json:"role"`
	RoleName    string `json:"role_name"`
	WorkspaceID string `json:"workspace_id"`
	ExpiresIn   int64  `json:"expires_in"`
}

type DashboardData struct {
	Role            string                   `json:"role"`
	RoleName        string                   `json:"role_name"`
	UserID          string                   `json:"user_id"`
	Elders          []ElderInfo              `json:"elders"`
	Alerts          []bigdata.Alert          `json:"alerts"`
	Recommendations []bigdata.Recommendation `json:"recommendations"`
	Statistics      map[string]interface{}   `json:"statistics"`
}

type ElderInfo struct {
	ElderID  string `json:"elder_id"`
	Name     string `json:"name"`
	Age      int    `json:"age"`
	Room     string `json:"room"`
	Status   string `json:"status"`
	Relation string `json:"relation,omitempty"`
}

type CallCaregiverRequest struct {
	ElderID string `json:"elder_id" binding:"required"`
	Reason  string `json:"reason"`
}

type FamilyMessageRequest struct {
	ElderID string `json:"elder_id" binding:"required"`
	Message string `json:"message" binding:"required"`
}

var (
	alertEngine          *bigdata.AlertEngine
	recommendationEngine *bigdata.RecommendationEngine
)

func InitRoleServices(influxClient *bigdata.InfluxDBClient) {
	alertEngine = bigdata.NewAlertEngine(influxClient)
	recommendationEngine = bigdata.NewRecommendationEngine(influxClient)
	auth.InitDemoRelations()
}

func RegisterRoleRoutes(_ *Handler, router gin.IRouter) {
	if router == nil {
		return
	}

	public := router.Group("/")
	public.POST("/role/login", RoleLoginHandler)

	protected := router.Group("/")
	protected.Use(middleware.JWTAuth())
	protected.GET("/dashboard", GetDashboardHandler)
	protected.GET("/alerts", GetAlertsHandler)
	protected.GET("/recommendations", GetRecommendationsHandler)
	protected.POST("/call-caregiver", CallCaregiverHandler)
	protected.POST("/family-message", FamilyMessageHandler)
}

func RoleLoginHandler(c *gin.Context) {
	var req RoleLoginRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{
			Success: false,
			Error:   "request validation failed: " + err.Error(),
		})
		return
	}

	if !auth.IsValidRole(req.Role) {
		c.JSON(http.StatusBadRequest, APIResponse{
			Success: false,
			Error:   "invalid role",
		})
		return
	}

	if err := auth.ValidateRoleCredentials(req.UserID, req.Password, req.Role); err != nil {
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

	token, err := middleware.GenerateTokenWithRole(req.UserID, req.WorkspaceID, req.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{
			Success: false,
			Error:   "failed to generate token: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data: RoleLoginResponse{
			Token:       token,
			UserID:      req.UserID,
			Role:        req.Role,
			RoleName:    auth.GetRoleName(auth.Role(req.Role)),
			WorkspaceID: req.WorkspaceID,
			ExpiresIn:   86400,
		},
	})
}

func GetDashboardHandler(c *gin.Context) {
	userID, exists := middleware.GetUserID(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, APIResponse{Success: false, Error: "unauthorized"})
		return
	}

	role, exists := middleware.GetRole(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, APIResponse{Success: false, Error: "missing role"})
		return
	}

	elders := getEldersByRole(userID, role)
	alerts := getAlertsForRole(role, elders)
	recommendations := getRecommendationsForRole(role, elders)

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data: DashboardData{
			Role:            role,
			RoleName:        auth.GetRoleName(auth.Role(role)),
			UserID:          userID,
			Elders:          elders,
			Alerts:          alerts,
			Recommendations: recommendations,
			Statistics:      generateStatistics(role, elders, alerts),
		},
	})
}

func GetAlertsHandler(c *gin.Context) {
	role, exists := middleware.GetRole(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, APIResponse{Success: false, Error: "missing role"})
		return
	}

	userID, _ := middleware.GetUserID(c)
	alerts := getAlertsForRole(role, getEldersByRole(userID, role))
	c.JSON(http.StatusOK, APIResponse{Success: true, Data: alerts})
}

func GetRecommendationsHandler(c *gin.Context) {
	role, exists := middleware.GetRole(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, APIResponse{Success: false, Error: "missing role"})
		return
	}

	userID, _ := middleware.GetUserID(c)
	recommendations := getRecommendationsForRole(role, getEldersByRole(userID, role))
	c.JSON(http.StatusOK, APIResponse{Success: true, Data: recommendations})
}

func CallCaregiverHandler(c *gin.Context) {
	var req CallCaregiverRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "request validation failed: " + err.Error()})
		return
	}

	userID, _ := middleware.GetUserID(c)
	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"user_id":    userID,
			"elder_id":   req.ElderID,
			"reason":     req.Reason,
			"status":     "queued",
			"created_at": time.Now().Format(time.RFC3339),
		},
	})
}

func FamilyMessageHandler(c *gin.Context) {
	var req FamilyMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Error: "request validation failed: " + err.Error()})
		return
	}

	userID, _ := middleware.GetUserID(c)
	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"user_id":    userID,
			"elder_id":   req.ElderID,
			"message":    req.Message,
			"status":     "queued",
			"created_at": time.Now().Format(time.RFC3339),
		},
	})
}

func getAlertsForRole(role string, elders []ElderInfo) []bigdata.Alert {
	if alertEngine == nil {
		return nil
	}

	ctx := context.Background()
	var alerts []bigdata.Alert
	for _, elder := range elders {
		for _, alert := range alertEngine.CheckVitalSignAlerts(ctx, elder.ElderID, elder.Name) {
			for _, targetRole := range alert.TargetRoles {
				if targetRole == role {
					alerts = append(alerts, alert)
					break
				}
			}
		}
	}
	return alerts
}

func getRecommendationsForRole(role string, elders []ElderInfo) []bigdata.Recommendation {
	if recommendationEngine == nil {
		return nil
	}

	ctx := context.Background()
	var recommendations []bigdata.Recommendation
	for _, elder := range elders {
		recommendations = append(recommendations, recommendationEngine.GenerateAllRecommendations(ctx, elder.ElderID, elder.Name, role)...)
	}
	return recommendations
}

func getEldersByRole(userID, role string) []ElderInfo {
	all := map[string]ElderInfo{
		"elder_001": {ElderID: "elder_001", Name: "张奶奶", Age: 82, Room: "A-101", Status: "stable"},
		"elder_002": {ElderID: "elder_002", Name: "李爷爷", Age: 79, Room: "A-203", Status: "attention"},
		"elder_003": {ElderID: "elder_003", Name: "王奶奶", Age: 85, Room: "B-105", Status: "stable"},
	}

	elderIDs := auth.GetUserElders(userID, auth.Role(role))
	if role == string(auth.RoleDoctor) && len(elderIDs) == 0 {
		elderIDs = []string{"elder_001", "elder_002", "elder_003"}
	}
	if role == string(auth.RoleElder) && len(elderIDs) == 0 {
		elderIDs = []string{userID}
	}

	elders := make([]ElderInfo, 0, len(elderIDs))
	for _, elderID := range elderIDs {
		elder, exists := all[elderID]
		if !exists {
			continue
		}
		for _, relation := range auth.GetElderRelations(elderID) {
			if relation.UserID == userID && string(relation.Role) == role {
				elder.Relation = relation.Relation
				break
			}
		}
		elders = append(elders, elder)
	}

	return elders
}

func generateStatistics(role string, elders []ElderInfo, alerts []bigdata.Alert) map[string]interface{} {
	statusCount := map[string]int{}
	for _, elder := range elders {
		statusCount[elder.Status]++
	}

	return map[string]interface{}{
		"role":         role,
		"elder_count":  len(elders),
		"alert_count":  len(alerts),
		"status_count": statusCount,
	}
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}
