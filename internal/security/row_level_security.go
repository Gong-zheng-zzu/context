package security

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// RowLevelSecurity 数据级权限控制（Row-Level Security）
// 确保用户只能访问自己有权限的数据行
type RowLevelSecurity struct {
	policies map[string]*AccessPolicy // 用户ID -> 访问策略
	mu       sync.RWMutex
}

// AccessPolicy 访问策略
type AccessPolicy struct {
	UserID           string
	Role             string   // caregiver, doctor, family, elder
	AllowedPatients  []string // 允许访问的患者ID列表
	AllowedDataTypes []string // 允许访问的数据类型
	TimeRestriction  *TimeRestriction
	DepartmentID     string // 部门ID（用于医生）
}

// TimeRestriction 时间限制
type TimeRestriction struct {
	AllowedHours []int // 允许访问的小时（0-23）
	AllowedDays  []int // 允许访问的星期（0-6，0=周日）
}

// HealthRecord 健康记录（示例数据结构）
type HealthRecord struct {
	RecordID    string
	PatientID   string
	RecordType  string // "体检报告"、"用药记录"、"症状变化"
	Content     string
	Timestamp   time.Time
	Sensitivity string // "low", "medium", "high"
	DoctorID    string // 负责医生ID
	CaregiverID string // 负责护工ID
}

// NewRowLevelSecurity 创建数据级权限控制实例
func NewRowLevelSecurity() *RowLevelSecurity {
	return &RowLevelSecurity{
		policies: make(map[string]*AccessPolicy),
	}
}

// SetPolicy 设置用户访问策略
func (rls *RowLevelSecurity) SetPolicy(policy *AccessPolicy) {
	rls.mu.Lock()
	defer rls.mu.Unlock()

	rls.policies[policy.UserID] = policy
	log.Printf("[数据级权限] 设置用户策略: userID=%s, role=%s, 患者数=%d",
		policy.UserID, policy.Role, len(policy.AllowedPatients))
}

// GetPolicy 获取用户访问策略
func (rls *RowLevelSecurity) GetPolicy(userID string) (*AccessPolicy, bool) {
	rls.mu.RLock()
	defer rls.mu.RUnlock()

	policy, exists := rls.policies[userID]
	return policy, exists
}

// CanAccessRecord 检查用户是否可以访问某条记录
func (rls *RowLevelSecurity) CanAccessRecord(userID string, record *HealthRecord) (bool, string) {
	policy, exists := rls.GetPolicy(userID)
	if !exists {
		return false, "用户策略不存在"
	}

	// 1. 检查时间限制
	if policy.TimeRestriction != nil {
		if !rls.checkTimeRestriction(policy.TimeRestriction) {
			return false, "当前时间不在允许访问的时间范围内"
		}
	}

	// 2. 根据角色检查权限
	switch policy.Role {
	case "elder":
		// 老人只能访问自己的记录
		if record.PatientID != userID {
			return false, "老人只能访问自己的健康记录"
		}
		return true, ""

	case "caregiver":
		// 护工只能访问自己负责的患者记录
		if !rls.isInAllowedList(record.PatientID, policy.AllowedPatients) {
			return false, "护工只能访问负责患者的记录"
		}
		// 护工不能访问高敏感度数据
		if record.Sensitivity == "high" {
			return false, "护工无权访问高敏感度数据"
		}
		return true, ""

	case "doctor":
		// 医生可以访问自己负责的患者或同部门的患者
		if record.DoctorID == userID {
			return true, "" // 自己负责的患者
		}
		if rls.isInAllowedList(record.PatientID, policy.AllowedPatients) {
			return true, "" // 明确授权的患者
		}
		return false, "医生只能访问负责患者的记录"

	case "family":
		// 家属只能访问关联的老人记录
		if !rls.isInAllowedList(record.PatientID, policy.AllowedPatients) {
			return false, "家属只能访问关联老人的记录"
		}
		return true, ""

	default:
		return false, "未知角色"
	}
}

// FilterRecords 过滤用户有权访问的记录
func (rls *RowLevelSecurity) FilterRecords(userID string, records []*HealthRecord) []*HealthRecord {
	filtered := []*HealthRecord{}
	deniedCount := 0

	for _, record := range records {
		canAccess, reason := rls.CanAccessRecord(userID, record)
		if canAccess {
			filtered = append(filtered, record)
		} else {
			deniedCount++
			log.Printf("[数据级权限] 拒绝访问: userID=%s, recordID=%s, 原因=%s",
				userID, record.RecordID, reason)
		}
	}

	log.Printf("[数据级权限] 记录过滤完成: userID=%s, 总数=%d, 允许=%d, 拒绝=%d",
		userID, len(records), len(filtered), deniedCount)

	return filtered
}

// checkTimeRestriction 检查时间限制
func (rls *RowLevelSecurity) checkTimeRestriction(restriction *TimeRestriction) bool {
	now := time.Now()
	currentHour := now.Hour()
	currentDay := int(now.Weekday())

	// 检查小时限制
	if len(restriction.AllowedHours) > 0 {
		hourAllowed := false
		for _, hour := range restriction.AllowedHours {
			if currentHour == hour {
				hourAllowed = true
				break
			}
		}
		if !hourAllowed {
			return false
		}
	}

	// 检查星期限制
	if len(restriction.AllowedDays) > 0 {
		dayAllowed := false
		for _, day := range restriction.AllowedDays {
			if currentDay == day {
				dayAllowed = true
				break
			}
		}
		if !dayAllowed {
			return false
		}
	}

	return true
}

// isInAllowedList 检查ID是否在允许列表中
func (rls *RowLevelSecurity) isInAllowedList(id string, allowedList []string) bool {
	for _, allowedID := range allowedList {
		if id == allowedID {
			return true
		}
	}
	return false
}

// AddAllowedPatient 添加允许访问的患者
func (rls *RowLevelSecurity) AddAllowedPatient(userID, patientID string) error {
	rls.mu.Lock()
	defer rls.mu.Unlock()

	policy, exists := rls.policies[userID]
	if !exists {
		return fmt.Errorf("用户策略不存在: %s", userID)
	}

	// 检查是否已存在
	for _, pid := range policy.AllowedPatients {
		if pid == patientID {
			return nil // 已存在，无需添加
		}
	}

	policy.AllowedPatients = append(policy.AllowedPatients, patientID)
	log.Printf("[数据级权限] 添加允许患者: userID=%s, patientID=%s", userID, patientID)

	return nil
}

// RemoveAllowedPatient 移除允许访问的患者
func (rls *RowLevelSecurity) RemoveAllowedPatient(userID, patientID string) error {
	rls.mu.Lock()
	defer rls.mu.Unlock()

	policy, exists := rls.policies[userID]
	if !exists {
		return fmt.Errorf("用户策略不存在: %s", userID)
	}

	newList := []string{}
	for _, pid := range policy.AllowedPatients {
		if pid != patientID {
			newList = append(newList, pid)
		}
	}

	policy.AllowedPatients = newList
	log.Printf("[数据级权限] 移除允许患者: userID=%s, patientID=%s", userID, patientID)

	return nil
}

// GetAccessiblePatients 获取用户可访问的患者列表
func (rls *RowLevelSecurity) GetAccessiblePatients(userID string) ([]string, error) {
	policy, exists := rls.GetPolicy(userID)
	if !exists {
		return nil, fmt.Errorf("用户策略不存在: %s", userID)
	}

	return policy.AllowedPatients, nil
}

// AuditAccess 审计访问记录
type AccessAudit struct {
	UserID    string
	RecordID  string
	Action    string // "read", "write", "delete"
	Allowed   bool
	Reason    string
	Timestamp time.Time
}

var accessAuditLog []AccessAudit
var auditMu sync.Mutex

// LogAccess 记录访问审计
func (rls *RowLevelSecurity) LogAccess(audit AccessAudit) {
	auditMu.Lock()
	defer auditMu.Unlock()

	audit.Timestamp = time.Now()
	accessAuditLog = append(accessAuditLog, audit)

	// 限制日志大小（保留最近1000条）
	if len(accessAuditLog) > 1000 {
		accessAuditLog = accessAuditLog[len(accessAuditLog)-1000:]
	}

	log.Printf("[访问审计] userID=%s, recordID=%s, action=%s, allowed=%v, reason=%s",
		audit.UserID, audit.RecordID, audit.Action, audit.Allowed, audit.Reason)
}

// GetAuditLog 获取审计日志
func (rls *RowLevelSecurity) GetAuditLog(userID string, limit int) []AccessAudit {
	auditMu.Lock()
	defer auditMu.Unlock()

	filtered := []AccessAudit{}
	for i := len(accessAuditLog) - 1; i >= 0 && len(filtered) < limit; i-- {
		if userID == "" || accessAuditLog[i].UserID == userID {
			filtered = append(filtered, accessAuditLog[i])
		}
	}

	return filtered
}

// GenerateAccessReport 生成访问报告
func (rls *RowLevelSecurity) GenerateAccessReport(userID string, startTime, endTime time.Time) string {
	auditMu.Lock()
	defer auditMu.Unlock()

	totalAccess := 0
	allowedAccess := 0
	deniedAccess := 0
	actionCounts := make(map[string]int)

	for _, audit := range accessAuditLog {
		if audit.UserID == userID &&
			audit.Timestamp.After(startTime) &&
			audit.Timestamp.Before(endTime) {
			totalAccess++
			if audit.Allowed {
				allowedAccess++
			} else {
				deniedAccess++
			}
			actionCounts[audit.Action]++
		}
	}

	report := fmt.Sprintf(`
访问报告
========
用户ID: %s
时间范围: %s - %s
总访问次数: %d
允许访问: %d
拒绝访问: %d
操作统计:
`, userID, startTime.Format("2006-01-02 15:04"), endTime.Format("2006-01-02 15:04"),
		totalAccess, allowedAccess, deniedAccess)

	for action, count := range actionCounts {
		report += fmt.Sprintf("  - %s: %d次\n", action, count)
	}

	return report
}

// EmergencyOverride 紧急覆盖（用于紧急情况）
type EmergencyOverride struct {
	UserID      string
	Reason      string
	ApprovedBy  string
	StartTime   time.Time
	EndTime     time.Time
	IsActive    bool
}

var emergencyOverrides = make(map[string]*EmergencyOverride)
var emergencyMu sync.RWMutex

// GrantEmergencyAccess 授予紧急访问权限
func (rls *RowLevelSecurity) GrantEmergencyAccess(userID, reason, approvedBy string, duration time.Duration) {
	emergencyMu.Lock()
	defer emergencyMu.Unlock()

	override := &EmergencyOverride{
		UserID:     userID,
		Reason:     reason,
		ApprovedBy: approvedBy,
		StartTime:  time.Now(),
		EndTime:    time.Now().Add(duration),
		IsActive:   true,
	}

	emergencyOverrides[userID] = override

	log.Printf("[紧急访问] 授予紧急权限: userID=%s, 原因=%s, 批准人=%s, 有效期=%s",
		userID, reason, approvedBy, duration.String())
}

// RevokeEmergencyAccess 撤销紧急访问权限
func (rls *RowLevelSecurity) RevokeEmergencyAccess(userID string) {
	emergencyMu.Lock()
	defer emergencyMu.Unlock()

	if override, exists := emergencyOverrides[userID]; exists {
		override.IsActive = false
		log.Printf("[紧急访问] 撤销紧急权限: userID=%s", userID)
	}
}

// HasEmergencyAccess 检查是否有紧急访问权限
func (rls *RowLevelSecurity) HasEmergencyAccess(userID string) bool {
	emergencyMu.RLock()
	defer emergencyMu.RUnlock()

	override, exists := emergencyOverrides[userID]
	if !exists {
		return false
	}

	// 检查是否过期
	if time.Now().After(override.EndTime) {
		override.IsActive = false
		return false
	}

	return override.IsActive
}

// BuildSQLFilter 构建SQL过滤条件（用于数据库查询）
// 根据用户权限生成WHERE子句
func (rls *RowLevelSecurity) BuildSQLFilter(userID string, tableName string) (string, error) {
	policy, exists := rls.GetPolicy(userID)
	if !exists {
		return "", fmt.Errorf("用户策略不存在: %s", userID)
	}

	// 检查紧急访问
	if rls.HasEmergencyAccess(userID) {
		log.Printf("[数据级权限] 用户%s拥有紧急访问权限，跳过过滤", userID)
		return "1=1", nil // 允许访问所有记录
	}

	var conditions []string

	switch policy.Role {
	case "elder":
		// 老人只能访问自己的记录
		conditions = append(conditions, fmt.Sprintf("%s.patient_id = '%s'", tableName, userID))

	case "caregiver":
		// 护工只能访问负责的患者
		if len(policy.AllowedPatients) > 0 {
			patientList := "'" + strings.Join(policy.AllowedPatients, "','") + "'"
			conditions = append(conditions, fmt.Sprintf("%s.patient_id IN (%s)", tableName, patientList))
		} else {
			conditions = append(conditions, "1=0") // 没有允许的患者，拒绝所有访问
		}
		// 护工不能访问高敏感度数据
		conditions = append(conditions, fmt.Sprintf("%s.sensitivity != 'high'", tableName))

	case "doctor":
		// 医生可以访问负责的患者
		if len(policy.AllowedPatients) > 0 {
			patientList := "'" + strings.Join(policy.AllowedPatients, "','") + "'"
			conditions = append(conditions, fmt.Sprintf("(%s.patient_id IN (%s) OR %s.doctor_id = '%s')",
				tableName, patientList, tableName, userID))
		} else {
			conditions = append(conditions, fmt.Sprintf("%s.doctor_id = '%s'", tableName, userID))
		}

	case "family":
		// 家属只能访问关联的老人
		if len(policy.AllowedPatients) > 0 {
			patientList := "'" + strings.Join(policy.AllowedPatients, "','") + "'"
			conditions = append(conditions, fmt.Sprintf("%s.patient_id IN (%s)", tableName, patientList))
		} else {
			conditions = append(conditions, "1=0")
		}

	default:
		return "", fmt.Errorf("未知角色: %s", policy.Role)
	}

	if len(conditions) == 0 {
		return "1=0", nil // 默认拒绝访问
	}

	filter := strings.Join(conditions, " AND ")
	log.Printf("[数据级权限] 生成SQL过滤条件: userID=%s, filter=%s", userID, filter)

	return filter, nil
}
