package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/contextkeeper/service/internal/bigdata"
	"github.com/contextkeeper/service/internal/health"
	"github.com/contextkeeper/service/internal/security"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// HealthRecord 健康记录数据结构
type HealthRecord struct {
	RecordID       string   `json:"record_id"`       // 记录ID
	UserID         string   `json:"user_id"`         // 用户ID
	SessionID      string   `json:"session_id"`      // 会话ID
	Content        string   `json:"content"`         // 脱敏后的内容
	OriginalHash   string   `json:"original_hash"`   // 原始内容哈希（用于验证）
	Category       string   `json:"category"`        // 类别：体检/就诊/用药/症状
	Timestamp      int64    `json:"timestamp"`       // 时间戳（Unix时间）
	SensitiveTypes []string `json:"sensitive_types"` // 检测到的敏感信息类型
	CreatedAt      string   `json:"created_at"`      // 创建时间（可读格式）
}

// HealthRecordRequest 记录健康信息的请求结构
type HealthRecordRequest struct {
	UserID    string `json:"user_id" binding:"required"`    // 用户ID（必填）
	SessionID string `json:"session_id" binding:"required"` // 会话ID（必填）
	Content   string `json:"content" binding:"required"`    // 健康信息内容（必填）
	Category  string `json:"category" binding:"required"`   // 类别（必填）
}

// HealthRecordResponse 记录健康信息的响应结构
type HealthRecordResponse struct {
	RecordID        string   `json:"record_id"`        // 记录ID
	RedactedContent string   `json:"redacted_content"` // 脱敏后的内容
	SensitiveTypes  []string `json:"sensitive_types"`  // 检测到的敏感信息类型
	Category        string   `json:"category"`         // 类别
	Timestamp       int64    `json:"timestamp"`        // 时间戳
}

// NursingRecordRequest is the lightweight, session-scoped write contract used
// by the caregiver UI. The authenticated user is always taken from the JWT.
type NursingRecordRequest struct {
	SessionID string `json:"session_id" binding:"required"`
	Content   string `json:"content" binding:"required"`
	Category  string `json:"category" binding:"required"`
}

// NursingRecordResponse deliberately returns only write evidence, not the
// record content, which may contain sensitive health information.
type NursingRecordResponse struct {
	RecordID          string                   `json:"record_id"`
	SessionID         string                   `json:"session_id"`
	Category          string                   `json:"category"`
	Timestamp         int64                    `json:"timestamp"`
	CreatedAt         string                   `json:"created_at"`
	StorageScope      string                   `json:"storage_scope"`
	SensitiveTypes    []string                 `json:"sensitive_types,omitempty"`
	SecurityExecution NursingSecurityExecution `json:"security_execution"`
}

// NursingSecurityExecution is limited to auditable status and contains no
// raw clinical content, detector payload, or internal reasoning.
type NursingSecurityExecution struct {
	ASDFChecked     bool     `json:"asdf_checked"`
	InputNormalized bool     `json:"input_normalized"`
	AttackTypes     []string `json:"attack_types,omitempty"`
	SensitiveTypes  []string `json:"sensitive_types,omitempty"`
	StorageScope    string   `json:"storage_scope"`
}

// HealthHistoryRequest 查询健康记录历史的请求结构
type HealthHistoryRequest struct {
	UserID   string `form:"user_id" binding:"required"` // 用户ID（必填）
	Days     int    `form:"days"`                       // 查询天数（默认30天）
	Category string `form:"category"`                   // 类别过滤（可选）
}

// HealthHistoryResponse 查询健康记录历史的响应结构
type HealthHistoryResponse struct {
	Records    []HealthRecord   `json:"records"`    // 记录列表
	Statistics HealthStatistics `json:"statistics"` // 统计信息
}

// HealthStatistics 健康记录统计信息
type HealthStatistics struct {
	TotalRecords   int            `json:"total_records"`   // 总记录数
	CategoryCount  map[string]int `json:"category_count"`  // 各类别记录数
	DateRange      string         `json:"date_range"`      // 时间范围
	SensitiveCount int            `json:"sensitive_count"` // 包含敏感信息的记录数
}

// HealthSummaryResponse 健康档案摘要的响应结构
type HealthSummaryResponse struct {
	UserID          string                  `json:"user_id"`          // 用户ID
	TotalRecords    int                     `json:"total_records"`    // 总记录数
	CategorySummary map[string]CategoryInfo `json:"category_summary"` // 分类摘要
	LatestRecords   []HealthRecord          `json:"latest_records"`   // 最新记录
	TimeRange       string                  `json:"time_range"`       // 时间范围
}

// CategoryInfo 类别信息
type CategoryInfo struct {
	Count        int           `json:"count"`         // 记录数量
	LatestRecord *HealthRecord `json:"latest_record"` // 最新记录
	FirstRecord  *HealthRecord `json:"first_record"`  // 最早记录
}

// APIResponse 统一的API响应结构
type APIResponse struct {
	Success bool        `json:"success"` // 是否成功
	Data    interface{} `json:"data"`    // 响应数据
	Error   string      `json:"error"`   // 错误信息
}

// 内存存储（后续可改为数据库）
var (
	healthRecords      = make(map[string]*HealthRecord) // key: recordID
	userRecordsIndex   = make(map[string][]string)      // key: userID, value: []recordID
	healthRecordsMutex sync.RWMutex                     // 读写锁
	sensitiveDetector  = security.NewDetector()         // 敏感信息检测器

	// 大数据服务（InfluxDB时序数据库）
	influxDBClient   *bigdata.InfluxDBClient
	vitalSignService *bigdata.VitalSignService
	ingestionService *bigdata.DataIngestionService
	queryService     *bigdata.UnifiedQueryService
	privacyManager   *security.DifferentialPrivacy
)

// 有效的健康记录类别（养老院护理场景）
var validCategories = map[string]bool{
	// 新类别（养老院场景）
	"日常护理": true, // daily_care
	"生命体征": true, // vital_signs
	"用药":   true, // medication
	"异常事件": true, // incident
	"活动":   true, // activity
	"饮食":   true, // diet

	// 保留旧类别（向后兼容）
	"体检": true, // physical_exam -> vital_signs
	"就诊": true, // medical_visit -> incident
	"症状": true, // symptom -> incident
}

// HandleNursingRecord persists a nursing record without invoking the LLM
// analysis pipeline. It is intentionally separate from conversational chat so
// the UI can give an auditable write acknowledgement immediately.
func (h *Handler) HandleNursingRecord(c *gin.Context) {
	var req NursingRecordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Data: nil, Error: "请求参数错误: " + err.Error()})
		return
	}
	if !validCategories[req.Category] {
		c.JSON(http.StatusBadRequest, APIResponse{Success: false, Data: nil, Error: "无效的护理记录类别"})
		return
	}

	authenticatedUserID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, APIResponse{Success: false, Data: nil, Error: "未授权：缺少用户身份信息"})
		return
	}
	userID, ok := authenticatedUserID.(string)
	if !ok || userID == "" {
		c.JSON(http.StatusUnauthorized, APIResponse{Success: false, Data: nil, Error: "未授权：用户身份无效"})
		return
	}

	if h.contextService == nil {
		c.JSON(http.StatusServiceUnavailable, APIResponse{Success: false, Data: nil, Error: "护理记录服务不可用"})
		return
	}
	sessionStore := h.contextService.SessionStore()
	if sessionStore == nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Success: false, Data: nil, Error: "会话存储服务不可用"})
		return
	}
	if owner, err := sessionStore.GetSessionOwner(req.SessionID); err == nil && owner != userID {
		c.JSON(http.StatusForbidden, APIResponse{Success: false, Data: nil, Error: "无权向其他用户会话写入护理记录"})
		return
	}
	if err := sessionStore.SetSessionMetadata(req.SessionID, "userId", userID); err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Success: false, Data: nil, Error: "无法确认会话归属"})
		return
	}

	redactedContent := req.Content
	sensitiveTypes := make([]string, 0)
	securityExecution := NursingSecurityExecution{ASDFChecked: h.securityService != nil, StorageScope: "protected_session"}
	if h.securityService != nil {
		normalizedContent, isAdversarial, attackTypes, _ := h.securityService.DefendAndNormalize(req.Content)
		securityExecution.InputNormalized = normalizedContent != req.Content
		if isAdversarial {
			securityExecution.AttackTypes = attackTypes
		}
		scanResult, err := h.securityService.ScanContent(c.Request.Context(), req.SessionID, userID, req.Content)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, APIResponse{Success: false, Data: nil, Error: "护理记录安全检测失败"})
			return
		}
		if scanResult.RedactedContent != "" {
			redactedContent = scanResult.RedactedContent
		}
		seen := make(map[string]struct{})
		for _, item := range scanResult.SensitiveInfos {
			typeName := string(item.Type)
			if _, duplicate := seen[typeName]; !duplicate {
				sensitiveTypes = append(sensitiveTypes, typeName)
				seen[typeName] = struct{}{}
			}
		}
		securityExecution.SensitiveTypes = sensitiveTypes
	}

	now := time.Now()
	recordID := uuid.NewString()
	record := &HealthRecord{
		RecordID:       recordID,
		UserID:         userID,
		SessionID:      req.SessionID,
		Content:        redactedContent,
		Category:       req.Category,
		Timestamp:      now.Unix(),
		SensitiveTypes: sensitiveTypes,
		CreatedAt:      now.Format("2006-01-02 15:04:05"),
	}

	healthRecordsMutex.Lock()
	healthRecords[recordID] = record
	userRecordsIndex[userID] = append(userRecordsIndex[userID], recordID)
	healthRecordsMutex.Unlock()

	historyEntry := fmt.Sprintf("护理记录[%s][%s][%s]: %s", recordID, req.Category, record.CreatedAt, redactedContent)
	if err := sessionStore.UpdateSession(req.SessionID, "用户: "+historyEntry); err != nil {
		healthRecordsMutex.Lock()
		delete(healthRecords, recordID)
		userRecordsIndex[userID] = userRecordsIndex[userID][:len(userRecordsIndex[userID])-1]
		healthRecordsMutex.Unlock()
		c.JSON(http.StatusInternalServerError, APIResponse{Success: false, Data: nil, Error: "护理记录会话写入失败"})
		return
	}

	log.Printf("[护理记录] 已写入受保护会话: record_id=%s user_id=%s session_id=%s category=%s", recordID, userID, req.SessionID, req.Category)
	c.JSON(http.StatusCreated, APIResponse{Success: true, Data: NursingRecordResponse{
		RecordID:          recordID,
		SessionID:         req.SessionID,
		Category:          req.Category,
		Timestamp:         record.Timestamp,
		CreatedAt:         record.CreatedAt,
		StorageScope:      "protected_session",
		SensitiveTypes:    sensitiveTypes,
		SecurityExecution: securityExecution,
	}})
}

// RecordHealthHandler 记录健康信息的处理函数
// POST /api/health/record
func RecordHealthHandler(c *gin.Context) {
	var req HealthRecordRequest

	// 解析请求参数
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{
			Success: false,
			Data:    nil,
			Error:   fmt.Sprintf("请求参数错误: %v", err),
		})
		return
	}

	// 🔒 【安全修复】从JWT上下文获取真实的user_id，防止伪造
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, APIResponse{
			Success: false,
			Data:    nil,
			Error:   "未授权：缺少用户身份信息",
		})
		return
	}
	req.UserID = userID.(string) // 强制使用JWT中的user_id，忽略请求体中的user_id

	// 🔒 【可选】从JWT获取workspace_id（多租户隔离）
	workspaceID, _ := c.Get("workspace_id")
	if workspaceID != nil {
		log.Printf("🔒 [健康记录] 多租户隔离: user_id=%s, workspace_id=%s", req.UserID, workspaceID)
	}

	// 验证类别是否有效
	if !validCategories[req.Category] {
		c.JSON(http.StatusBadRequest, APIResponse{
			Success: false,
			Data:    nil,
			Error:   fmt.Sprintf("无效的类别，支持的类别: 体检/就诊/用药/症状"),
		})
		return
	}

	// 验证内容不为空
	if len(req.Content) == 0 {
		c.JSON(http.StatusBadRequest, APIResponse{
			Success: false,
			Data:    nil,
			Error:   "健康信息内容不能为空",
		})
		return
	}

	// 🛡️ 创新点1: ASDF - 对抗样本防御与归一化
	log.Printf("🛡️ [ASDF] 开始对健康记录进行对抗样本检测")

	// 如果全局安全服务可用，使用它进行对抗样本检测
	if globalSecurityService != nil {
		normalizedContent, isAdversarial, attackTypes, advConfidence := globalSecurityService.DefendAndNormalize(req.Content)

		if isAdversarial {
			log.Printf("🛡️ [ASDF] 检测到对抗样本攻击: types=%v, confidence=%.2f", attackTypes, advConfidence)
			contentPreview := req.Content
			if len(contentPreview) > 50 {
				contentPreview = contentPreview[:50]
			}
			normalizedPreview := normalizedContent
			if len(normalizedPreview) > 50 {
				normalizedPreview = normalizedPreview[:50]
			}
			log.Printf("🛡️ [ASDF] 原始内容: %s", contentPreview)
			log.Printf("🛡️ [ASDF] health record input normalized: bytes=%d", len(normalizedPreview))

			// 使用归一化后的内容继续处理
			req.Content = normalizedContent
		}
	}

	// 🔒 【多层检测】检测并脱敏敏感信息
	log.Printf("🔍 [健康记录] 开始多层敏感信息检测，内容长度: %d", len(req.Content))
	redactedContent, sensitiveInfos := sensitiveDetector.DetectAndRedact(req.Content)
	log.Printf("🔒 [健康记录] 检测到 %d 个敏感信息", len(sensitiveInfos))

	// 提取敏感信息类型
	sensitiveTypes := make([]string, 0)
	typeMap := make(map[string]bool)
	for _, info := range sensitiveInfos {
		typeStr := string(info.Type)
		if !typeMap[typeStr] {
			sensitiveTypes = append(sensitiveTypes, typeStr)
			typeMap[typeStr] = true
		}
	}

	// 🔒 【审计日志】记录敏感信息检测事件
	if len(sensitiveTypes) > 0 {
		log.Printf("🔒 [审计] 用户 %s 的健康记录包含敏感信息: %v", req.UserID, sensitiveTypes)
	}

	// 生成记录ID
	recordID := uuid.New().String()
	timestamp := time.Now().Unix()

	// 创建健康记录
	record := &HealthRecord{
		RecordID:       recordID,
		UserID:         req.UserID,
		SessionID:      req.SessionID,
		Content:        redactedContent, // 🔒 存储脱敏后的内容
		Category:       req.Category,
		Timestamp:      timestamp,
		SensitiveTypes: sensitiveTypes,
		CreatedAt:      time.Unix(timestamp, 0).Format("2006-01-02 15:04:05"),
	}

	// 🔒 【加密存储】存储到内存（后续可改为加密数据库）
	healthRecordsMutex.Lock()
	healthRecords[recordID] = record
	userRecordsIndex[req.UserID] = append(userRecordsIndex[req.UserID], recordID)
	healthRecordsMutex.Unlock()

	log.Printf("✅ [健康记录] 记录已保存: record_id=%s, user_id=%s, category=%s, sensitive_types=%d",
		recordID, req.UserID, req.Category, len(sensitiveTypes))

	// 返回响应
	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data: HealthRecordResponse{
			RecordID:        recordID,
			RedactedContent: redactedContent,
			SensitiveTypes:  sensitiveTypes,
			Category:        req.Category,
			Timestamp:       timestamp,
		},
		Error: "",
	})
}

// GetHealthHistoryHandler 查询健康记录历史的处理函数
// GET /api/health/history?user_id=xxx&days=30&category=体检
func GetHealthHistoryHandler(c *gin.Context) {
	var req HealthHistoryRequest

	// 🔒 【安全修复】从JWT上下文获取真实的user_id，防止伪造
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, APIResponse{
			Success: false,
			Data:    nil,
			Error:   "未授权：缺少用户身份信息",
		})
		return
	}
	req.UserID = userID.(string) // 强制使用JWT中的user_id

	// 解析查询参数
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{
			Success: false,
			Data:    nil,
			Error:   fmt.Sprintf("请求参数错误: %v", err),
		})
		return
	}

	// 再次确保使用JWT中的user_id（防止查询参数伪造）
	req.UserID = userID.(string)

	// 设置默认查询天数
	if req.Days <= 0 {
		req.Days = 30
	}

	// 验证类别（如果提供）
	if req.Category != "" && !validCategories[req.Category] {
		c.JSON(http.StatusBadRequest, APIResponse{
			Success: false,
			Data:    nil,
			Error:   fmt.Sprintf("无效的类别，支持的类别: 体检/就诊/用药/症状"),
		})
		return
	}

	// 计算时间范围
	now := time.Now()
	startTime := now.AddDate(0, 0, -req.Days).Unix()

	// 获取用户的所有记录
	healthRecordsMutex.RLock()
	recordIDs, exists := userRecordsIndex[req.UserID]
	if !exists {
		healthRecordsMutex.RUnlock()
		c.JSON(http.StatusOK, APIResponse{
			Success: true,
			Data: HealthHistoryResponse{
				Records: []HealthRecord{},
				Statistics: HealthStatistics{
					TotalRecords:   0,
					CategoryCount:  make(map[string]int),
					DateRange:      fmt.Sprintf("%d天", req.Days),
					SensitiveCount: 0,
				},
			},
			Error: "",
		})
		return
	}

	// 过滤记录
	var filteredRecords []HealthRecord
	categoryCount := make(map[string]int)
	sensitiveCount := 0

	for _, recordID := range recordIDs {
		record, exists := healthRecords[recordID]
		if !exists {
			continue
		}

		// 时间范围过滤
		if record.Timestamp < startTime {
			continue
		}

		// 类别过滤
		if req.Category != "" && record.Category != req.Category {
			continue
		}

		filteredRecords = append(filteredRecords, *record)
		categoryCount[record.Category]++

		if len(record.SensitiveTypes) > 0 {
			sensitiveCount++
		}
	}
	healthRecordsMutex.RUnlock()

	// 按时间倒序排序（最新的在前）
	sort.Slice(filteredRecords, func(i, j int) bool {
		return filteredRecords[i].Timestamp > filteredRecords[j].Timestamp
	})

	// 🔒 【二次脱敏】返回前再次检测和脱敏（双重保险）
	log.Printf("🔒 [健康历史] 返回前进行二次脱敏检查，记录数: %d", len(filteredRecords))
	for i := range filteredRecords {
		redacted, _ := sensitiveDetector.DetectAndRedact(filteredRecords[i].Content)
		filteredRecords[i].Content = redacted
	}

	// 构建响应
	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data: HealthHistoryResponse{
			Records: filteredRecords,
			Statistics: HealthStatistics{
				TotalRecords:   len(filteredRecords),
				CategoryCount:  categoryCount,
				DateRange:      fmt.Sprintf("%d天", req.Days),
				SensitiveCount: sensitiveCount,
			},
		},
		Error: "",
	})
}

// GetHealthSummaryHandler 健康档案摘要的处理函数
// GET /api/health/summary?user_id=xxx
func GetHealthSummaryHandler(c *gin.Context) {
	// 🔒 【安全修复】从JWT上下文获取真实的user_id，防止伪造
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, APIResponse{
			Success: false,
			Data:    nil,
			Error:   "未授权：缺少用户身份信息",
		})
		return
	}
	userIDStr := userID.(string)

	log.Printf("🔍 [健康摘要] 用户 %s 请求健康档案摘要", userIDStr)

	// 获取用户的所有记录
	healthRecordsMutex.RLock()
	recordIDs, exists := userRecordsIndex[userIDStr]
	if !exists {
		healthRecordsMutex.RUnlock()
		c.JSON(http.StatusOK, APIResponse{
			Success: true,
			Data: HealthSummaryResponse{
				UserID:          userIDStr,
				TotalRecords:    0,
				CategorySummary: make(map[string]CategoryInfo),
				LatestRecords:   []HealthRecord{},
				TimeRange:       "无记录",
			},
			Error: "",
		})
		return
	}

	// 收集所有记录
	var allRecords []HealthRecord
	for _, recordID := range recordIDs {
		record, exists := healthRecords[recordID]
		if exists {
			allRecords = append(allRecords, *record)
		}
	}
	healthRecordsMutex.RUnlock()

	// 如果没有记录
	if len(allRecords) == 0 {
		c.JSON(http.StatusOK, APIResponse{
			Success: true,
			Data: HealthSummaryResponse{
				UserID:          userIDStr,
				TotalRecords:    0,
				CategorySummary: make(map[string]CategoryInfo),
				LatestRecords:   []HealthRecord{},
				TimeRange:       "无记录",
			},
			Error: "",
		})
		return
	}

	// 🔒 【二次脱敏】对所有记录进行二次脱敏检查
	log.Printf("🔒 [健康摘要] 对 %d 条记录进行二次脱敏", len(allRecords))
	for i := range allRecords {
		redacted, _ := sensitiveDetector.DetectAndRedact(allRecords[i].Content)
		allRecords[i].Content = redacted
	}

	// 按时间排序
	sort.Slice(allRecords, func(i, j int) bool {
		return allRecords[i].Timestamp > allRecords[j].Timestamp
	})

	// 按类别分组统计
	categorySummary := make(map[string]CategoryInfo)
	for _, record := range allRecords {
		info, exists := categorySummary[record.Category]
		if !exists {
			info = CategoryInfo{
				Count:        0,
				LatestRecord: nil,
				FirstRecord:  nil,
			}
		}

		info.Count++

		// 更新最新记录
		if info.LatestRecord == nil || record.Timestamp > info.LatestRecord.Timestamp {
			recordCopy := record
			info.LatestRecord = &recordCopy
		}

		// 更新最早记录
		if info.FirstRecord == nil || record.Timestamp < info.FirstRecord.Timestamp {
			recordCopy := record
			info.FirstRecord = &recordCopy
		}

		categorySummary[record.Category] = info
	}

	// 获取最新的5条记录
	latestRecords := allRecords
	if len(latestRecords) > 5 {
		latestRecords = latestRecords[:5]
	}

	// 计算时间范围
	oldestTime := allRecords[len(allRecords)-1].Timestamp
	newestTime := allRecords[0].Timestamp
	timeRange := fmt.Sprintf("%s 至 %s",
		time.Unix(oldestTime, 0).Format("2006-01-02"),
		time.Unix(newestTime, 0).Format("2006-01-02"))

	log.Printf("✅ [健康摘要] 摘要生成成功: user_id=%s, total_records=%d, categories=%d",
		userIDStr, len(allRecords), len(categorySummary))

	// 构建响应
	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data: HealthSummaryResponse{
			UserID:          userIDStr,
			TotalRecords:    len(allRecords),
			CategorySummary: categorySummary,
			LatestRecords:   latestRecords,
			TimeRange:       timeRange,
		},
		Error: "",
	})
}

// GenerateReportHandler 生成就医报告的处理函数
// GET /api/health/report?userId=xxx&days=30
func GenerateReportHandler(c *gin.Context) {
	// 🔒 【安全修复】从JWT上下文获取真实的user_id，防止伪造
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, APIResponse{
			Success: false,
			Data:    nil,
			Error:   "未授权：缺少用户身份信息",
		})
		return
	}
	userIDStr := userID.(string)

	log.Printf("📄 [就医报告] 用户 %s 请求生成就医报告", userIDStr)

	// 获取天数参数，默认30天
	daysStr := c.Query("days")
	days := 30
	if daysStr != "" {
		if d, err := strconv.Atoi(daysStr); err == nil && d > 0 {
			days = d
		}
	}

	// 获取用户的所有记录
	healthRecordsMutex.RLock()
	recordIDs, exists := userRecordsIndex[userIDStr]
	if !exists {
		healthRecordsMutex.RUnlock()
		c.JSON(http.StatusOK, APIResponse{
			Success: true,
			Data:    "该用户暂无健康记录",
			Error:   "",
		})
		return
	}

	// 转换为health.HealthRecord格式
	var healthRecordsList []health.HealthRecord
	for _, recordID := range recordIDs {
		record, exists := healthRecords[recordID]
		if !exists {
			continue
		}

		// 🔒 【二次脱敏】报告生成前再次脱敏
		redactedContent, _ := sensitiveDetector.DetectAndRedact(record.Content)

		// 转换类别
		var category health.HealthRecordCategory
		switch record.Category {
		case "体检":
			category = health.CategoryPhysicalExam
		case "就诊":
			category = health.CategoryMedicalVisit
		case "用药":
			category = health.CategoryMedication
		case "症状":
			category = health.CategorySymptom
		default:
			category = health.CategoryOther
		}

		healthRecordsList = append(healthRecordsList, health.HealthRecord{
			ID:       record.RecordID,
			UserID:   record.UserID,
			Category: category,
			Date:     time.Unix(record.Timestamp, 0),
			Title:    record.Category,
			Content:  redactedContent, // 🔒 使用脱敏后的内容
			Metadata: map[string]interface{}{
				"sensitiveTypes": record.SensitiveTypes,
			},
		})
	}
	healthRecordsMutex.RUnlock()

	log.Printf("📄 [就医报告] 准备生成报告: user_id=%s, records=%d, days=%d",
		userIDStr, len(healthRecordsList), days)

	// 创建报告生成器
	generator := health.NewMedicalReportGenerator(nil, nil)

	// 生成报告
	report, err := generator.GenerateReport(userIDStr, healthRecordsList, days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{
			Success: false,
			Data:    nil,
			Error:   fmt.Sprintf("生成报告失败: %v", err),
		})
		return
	}

	// 格式化为文本
	reportText := generator.FormatAsText(report)

	// 🔒 【三次脱敏】报告输出前最后一次脱敏检查（三重保险）
	finalReport, _ := sensitiveDetector.DetectAndRedact(reportText)

	log.Printf("✅ [就医报告] 报告生成成功: user_id=%s, report_length=%d",
		userIDStr, len(finalReport))

	// 返回响应
	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data:    finalReport, // 🔒 返回三次脱敏后的报告
		Error:   "",
	})
}

// InitBigDataServices 初始化大数据服务
func InitBigDataServices(influxURL, influxToken, influxOrg, influxBucket string) error {
	// 创建InfluxDB客户端
	config := bigdata.InfluxDBConfig{
		URL:    influxURL,
		Token:  influxToken,
		Org:    influxOrg,
		Bucket: influxBucket,
	}

	client, err := bigdata.NewInfluxDBClient(config)
	if err != nil {
		return fmt.Errorf("初始化InfluxDB客户端失败: %w", err)
	}

	influxDBClient = client
	vitalSignService = bigdata.NewVitalSignService(client)
	ingestionService = bigdata.NewDataIngestionService(client)
	queryService = bigdata.NewUnifiedQueryService(client)

	// 初始化差分隐私管理器（epsilon=1.0, delta=1e-5）
	privacyManager = security.NewDifferentialPrivacy(1.0, 1e-5)

	log.Println("[大数据] 服务初始化成功")
	return nil
}

// RecordVitalSignHandler 记录生命体征数据
// POST /api/vital-signs
func RecordVitalSignHandler(c *gin.Context) {
	var req struct {
		ResidentID string   `json:"resident_id" binding:"required"`
		Type       string   `json:"type" binding:"required"`
		Value      float64  `json:"value" binding:"required"`
		Value2     *float64 `json:"value2"`
		Unit       string   `json:"unit" binding:"required"`
		RoomNumber string   `json:"room_number"`
		DeviceID   string   `json:"device_id"`
		RecordedBy string   `json:"recorded_by"`
		Notes      string   `json:"notes"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{
			Success: false,
			Error:   fmt.Sprintf("请求参数错误: %v", err),
		})
		return
	}

	// 检查服务是否初始化
	if vitalSignService == nil {
		c.JSON(http.StatusServiceUnavailable, APIResponse{
			Success: false,
			Error:   "大数据服务未初始化",
		})
		return
	}

	// 创建生命体征数据
	vitalSign := &bigdata.VitalSign{
		ResidentID: req.ResidentID,
		Type:       bigdata.VitalSignType(req.Type),
		Value:      req.Value,
		Value2:     req.Value2,
		Unit:       req.Unit,
		RoomNumber: req.RoomNumber,
		DeviceID:   req.DeviceID,
		MeasuredAt: time.Now(),
		RecordedBy: req.RecordedBy,
		Notes:      req.Notes,
	}

	// 写入数据
	ctx := context.Background()
	if err := vitalSignService.WriteVitalSign(ctx, vitalSign); err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{
			Success: false,
			Error:   fmt.Sprintf("写入失败: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"resident_id": vitalSign.ResidentID,
			"type":        vitalSign.Type,
			"value":       vitalSign.Value,
			"abnormal":    vitalSign.IsAbnormal(),
			"level":       vitalSign.GetAbnormalLevel(),
			"measured_at": vitalSign.MeasuredAt,
		},
	})
}

// GetVitalSignsHistoryHandler 查询生命体征历史数据
// GET /api/vital-signs/history?resident_id=xxx&type=blood_pressure&days=7
func GetVitalSignsHistoryHandler(c *gin.Context) {
	residentID := c.Query("resident_id")
	signType := c.Query("type")
	daysStr := c.DefaultQuery("days", "7")

	if residentID == "" || signType == "" {
		c.JSON(http.StatusBadRequest, APIResponse{
			Success: false,
			Error:   "缺少必要参数: resident_id 和 type",
		})
		return
	}

	days, err := strconv.Atoi(daysStr)
	if err != nil || days <= 0 {
		days = 7
	}

	// 检查服务是否初始化
	if queryService == nil {
		c.JSON(http.StatusServiceUnavailable, APIResponse{
			Success: false,
			Error:   "大数据服务未初始化",
		})
		return
	}

	// 计算时间范围
	end := time.Now()
	start := end.AddDate(0, 0, -days)

	// 查询数据
	ctx := context.Background()
	records, err := vitalSignService.QueryVitalSignsByTimeRange(
		ctx,
		residentID,
		bigdata.VitalSignType(signType),
		start,
		end,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{
			Success: false,
			Error:   fmt.Sprintf("查询失败: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"resident_id": residentID,
			"type":        signType,
			"start_time":  start.Format("2006-01-02 15:04:05"),
			"end_time":    end.Format("2006-01-02 15:04:05"),
			"total":       len(records),
			"records":     records,
		},
	})
}

// GetVitalSignsStatsHandler 获取生命体征统计数据（带差分隐私保护）
// GET /api/vital-signs/stats?resident_id=xxx&type=blood_pressure&days=30
func GetVitalSignsStatsHandler(c *gin.Context) {
	residentID := c.Query("resident_id")
	signType := c.Query("type")
	daysStr := c.DefaultQuery("days", "30")

	if residentID == "" || signType == "" {
		c.JSON(http.StatusBadRequest, APIResponse{
			Success: false,
			Error:   "缺少必要参数: resident_id 和 type",
		})
		return
	}

	days, err := strconv.Atoi(daysStr)
	if err != nil || days <= 0 {
		days = 30
	}

	// 检查服务是否初始化
	if queryService == nil || privacyManager == nil {
		c.JSON(http.StatusServiceUnavailable, APIResponse{
			Success: false,
			Error:   "大数据服务未初始化",
		})
		return
	}

	// 计算时间范围
	end := time.Now()
	start := end.AddDate(0, 0, -days)

	// 查询统计数据
	ctx := context.Background()
	stats, err := queryService.GetStatistics(
		ctx,
		residentID,
		bigdata.VitalSignType(signType),
		start,
		end,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{
			Success: false,
			Error:   fmt.Sprintf("统计查询失败: %v", err),
		})
		return
	}

	// 应用差分隐私保护
	// 为统计值添加拉普拉斯噪声
	if stats["count"] > 0 {
		// 将统计值转换为向量
		values := []float32{
			float32(stats["min"]),
			float32(stats["max"]),
			float32(stats["mean"]),
		}

		// 添加噪声
		noisyValues := privacyManager.AddLaplaceNoise(values)

		// 更新统计值
		stats["min"] = float64(noisyValues[0])
		stats["max"] = float64(noisyValues[1])
		stats["mean"] = float64(noisyValues[2])
		stats["privacy_protected"] = 1.0

		log.Printf("[差分隐私] 统计数据已添加噪声保护: resident=%s, type=%s", residentID, signType)
	}

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"resident_id": residentID,
			"type":        signType,
			"start_time":  start.Format("2006-01-02"),
			"end_time":    end.Format("2006-01-02"),
			"statistics":  stats,
		},
	})
}

// RegisterHealthRoutes 注册健康信息相关的路由
func RegisterHealthRoutes(router gin.IRouter) {
	// 直接在传入的router上注册路由（可能是RouterGroup）
	// POST /api/health/record - 记录健康信息
	router.POST("/record", RecordHealthHandler)

	// GET /api/health/history - 查询健康记录历史
	router.GET("/history", GetHealthHistoryHandler)

	// GET /api/health/summary - 健康档案摘要
	router.GET("/summary", GetHealthSummaryHandler)

	// GET /api/health/report - 生成就医报告
	router.GET("/report", GenerateReportHandler)

	// 新增：生命体征相关路由（大数据治理）
	// POST /api/vital-signs - 记录生命体征
	router.POST("/vital-signs", RecordVitalSignHandler)

	// GET /api/vital-signs/history - 查询生命体征历史
	router.GET("/vital-signs/history", GetVitalSignsHistoryHandler)

	// GET /api/vital-signs/stats - 获取生命体征统计（带差分隐私）
	router.GET("/vital-signs/stats", GetVitalSignsStatsHandler)
}

// GetHealthRecordStats 获取健康记录统计信息（辅助函数，可用于监控）
func GetHealthRecordStats() map[string]interface{} {
	healthRecordsMutex.RLock()
	defer healthRecordsMutex.RUnlock()

	totalRecords := len(healthRecords)
	totalUsers := len(userRecordsIndex)

	categoryCount := make(map[string]int)
	sensitiveCount := 0

	for _, record := range healthRecords {
		categoryCount[record.Category]++
		if len(record.SensitiveTypes) > 0 {
			sensitiveCount++
		}
	}

	return map[string]interface{}{
		"total_records":   totalRecords,
		"total_users":     totalUsers,
		"category_count":  categoryCount,
		"sensitive_count": sensitiveCount,
	}
}

// ClearHealthRecords 清空所有健康记录（用于测试或重置）
func ClearHealthRecords() {
	healthRecordsMutex.Lock()
	defer healthRecordsMutex.Unlock()

	healthRecords = make(map[string]*HealthRecord)
	userRecordsIndex = make(map[string][]string)
}
