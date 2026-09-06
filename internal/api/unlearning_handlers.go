package api

import (
	"net/http"
	"time"

	"github.com/contextkeeper/service/internal/models"
	"github.com/contextkeeper/service/internal/services"
	"github.com/gin-gonic/gin"
)

// UnlearningHandler 机器遗忘API处理器
type UnlearningHandler struct {
	unlearningService *services.MachineUnlearningService
	auditEnabled      bool
}

// NewUnlearningHandler 创建机器遗忘处理器
func NewUnlearningHandler(unlearningService *services.MachineUnlearningService, auditEnabled bool) *UnlearningHandler {
	return &UnlearningHandler{
		unlearningService: unlearningService,
		auditEnabled:      auditEnabled,
	}
}

// ForgetUser 执行用户数据遗忘
// DELETE /api/v1/users/:user_id/forget
func (h *UnlearningHandler) ForgetUser(c *gin.Context) {
	userID := c.Param("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is required"})
		return
	}

	var req models.UnlearningRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 允许空body，使用默认参数
		req = models.UnlearningRequest{}
	}

	// 设置用户ID
	req.UserID = userID

	startTime := time.Now()
	requestIP := c.ClientIP()

	// 执行遗忘
	result, err := h.unlearningService.ExecuteUnlearning(c.Request.Context(), &req)
	if err != nil {
		h.logAuditEvent(c, userID, requestIP, &req, nil, "failed", err.Error(), time.Since(startTime))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "机器遗忘执行失败",
			"details": err.Error(),
		})
		return
	}

	// 记录审计日志
	if h.auditEnabled {
		h.logAuditEvent(c, userID, requestIP, &req, result, "success", "", time.Since(startTime))
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "机器遗忘执行完成",
		"result":  result,
	})
}

// VerifyUnlearning 验证遗忘效果
// GET /api/v1/unlearning/verify/:user_id
func (h *UnlearningHandler) VerifyUnlearning(c *gin.Context) {
	userID := c.Param("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is required"})
		return
	}

	var req models.UnlearningVerifyRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		req = models.UnlearningVerifyRequest{}
	}
	req.UserID = userID

	// 执行验证
	result, err := h.unlearningService.VerifyUnlearning(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "验证失败",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetUnlearningConfig 获取机器遗忘配置
// GET /api/v1/unlearning/config
func (h *UnlearningHandler) GetUnlearningConfig(c *gin.Context) {
	config := &models.UnlearningConfig{
		Enabled:                     true,
		DefaultEpsilon:              1.0,
		DefaultLearningRate:         0.001,
		DefaultMaxIterations:        50,
		DefaultConvergenceThreshold: 0.001,
		DefaultRetainedSampleRatio:  10,
		MaxPrivacyBudgetPerUser:     5.0,
		AutoRebuildIndex:            false,
		EnableAuditLog:              true,
	}

	c.JSON(http.StatusOK, config)
}

// BatchForgetUsers 批量遗忘用户
// POST /api/v1/unlearning/batch
func (h *UnlearningHandler) BatchForgetUsers(c *gin.Context) {
	var req struct {
		UserIDs             []string  `json:"user_ids" binding:"required"`
		Epsilon             float64   `json:"epsilon,omitempty"`
		LearningRate        float64   `json:"learning_rate,omitempty"`
		MaxIterations       int       `json:"max_iterations,omitempty"`
		CollectionNames     []string  `json:"collection_names,omitempty"`
		ConvergenceThreshold float64  `json:"convergence_threshold,omitempty"`
		RetainedSampleRatio int       `json:"retained_sample_ratio,omitempty"`
		RebuildIndex        bool      `json:"rebuild_index,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数", "details": err.Error()})
		return
	}

	if len(req.UserIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_ids不能为空"})
		return
	}

	results := make(map[string]*models.UnlearningResult)
	successCount := 0
	failCount := 0

	// 遍历每个用户执行遗忘
	for _, userID := range req.UserIDs {
		unlearningReq := &models.UnlearningRequest{
			UserID:               userID,
			Epsilon:              req.Epsilon,
			LearningRate:         req.LearningRate,
			MaxIterations:        req.MaxIterations,
			CollectionNames:      req.CollectionNames,
			ConvergenceThreshold: req.ConvergenceThreshold,
			RetainedSampleRatio:  req.RetainedSampleRatio,
			RebuildIndex:         req.RebuildIndex,
		}

		result, err := h.unlearningService.ExecuteUnlearning(c.Request.Context(), unlearningReq)
		if err != nil {
			results[userID] = &models.UnlearningResult{
				UserID:       userID,
				Status:       "failed",
				ErrorMessage: err.Error(),
				Timestamp:    time.Now(),
			}
			failCount++
		} else {
			results[userID] = result
			if result.Status == "success" {
				successCount++
			} else {
				failCount++
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"total":        len(req.UserIDs),
		"success":      successCount,
		"failed":       failCount,
		"results":      results,
	})
}

// GetUnlearningHistory 获取用户的遗忘历史
// GET /api/v1/unlearning/history/:user_id
func (h *UnlearningHandler) GetUnlearningHistory(c *gin.Context) {
	userID := c.Param("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is required"})
		return
	}

	// TODO: 从审计日志中查询历史记录
	// 这里返回模拟数据
	c.JSON(http.StatusOK, gin.H{
		"user_id": userID,
		"history": []interface{}{},
		"message": "历史记录查询功能待实现",
	})
}

// logAuditEvent 记录审计事件
func (h *UnlearningHandler) logAuditEvent(
	c *gin.Context,
	userID string,
	requestIP string,
	req *models.UnlearningRequest,
	result *models.UnlearningResult,
	status string,
	errorMsg string,
	duration time.Duration,
) {
	if !h.auditEnabled {
		return
	}

	event := &models.UnlearningAuditEvent{
		EventID:       generateEventID(),
		Timestamp:     time.Now(),
		UserID:        userID,
		RequestIP:     requestIP,
		OperationType: "unlearn",
		Request:       req,
		Result:        result,
		Status:        status,
		ErrorMessage:  errorMsg,
		Duration:      duration.Milliseconds(),
	}

	if result != nil {
		event.PrivacyBudget = result.PrivacyBudgetConsumed
		event.AffectedCollections = result.CollectionsAffected
	} else if req != nil {
		event.PrivacyBudget = req.Epsilon
		event.AffectedCollections = req.CollectionNames
	}

	// TODO: 将审计事件写入日志系统或数据库
	// logger.LogUnlearningAudit(event)

	// 临时输出到控制台
	c.Set("audit_event", event)
}

// generateEventID 生成事件ID
func generateEventID() string {
	return time.Now().Format("20060102150405") + "-" + randomString(8)
}

func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}

// RegisterUnlearningRoutes 注册机器遗忘路由
func RegisterUnlearningRoutes(router *gin.RouterGroup, handler *UnlearningHandler) {
	unlearning := router.Group("/unlearning")
	{
		unlearning.DELETE("/users/:user_id", handler.ForgetUser)
		unlearning.GET("/verify/:user_id", handler.VerifyUnlearning)
		unlearning.GET("/config", handler.GetUnlearningConfig)
		unlearning.POST("/batch", handler.BatchForgetUsers)
		unlearning.GET("/history/:user_id", handler.GetUnlearningHistory)
	}

	// 兼容旧版路由
	router.DELETE("/users/:user_id/forget", handler.ForgetUser)
}
