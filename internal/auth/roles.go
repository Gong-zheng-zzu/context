package auth

import (
	"fmt"
)

// Role 角色类型
type Role string

// 定义4种角色
const (
	RoleCaregiver Role = "caregiver" // 护工
	RoleDoctor    Role = "doctor"    // 医生
	RoleFamily    Role = "family"    // 家属
	RoleElder     Role = "elder"     // 老人
)

// Permission 权限定义
type Permission struct {
	CanViewAllElders      bool // 查看所有老人
	CanEditHealthRecords  bool // 编辑健康记录
	CanPrescribe          bool // 开处方
	CanViewSensitiveData  bool // 查看敏感数据
	CanReceiveAlerts      bool // 接收告警
	CanCallCaregiver      bool // 呼叫护工
	CanViewOwnFamily      bool // 查看自己的家人
	CanManageElders       bool // 管理老人信息
}

// RolePermissions 角色权限映射
var RolePermissions = map[Role]Permission{
	RoleCaregiver: {
		CanViewAllElders:      false, // 只能看负责的老人
		CanEditHealthRecords:  true,  // 可以记录护理信息
		CanPrescribe:          false, // 不能开处方
		CanViewSensitiveData:  false, // 不能看敏感数据（如诊断）
		CanReceiveAlerts:      true,  // 接收护理提醒
		CanCallCaregiver:      false,
		CanViewOwnFamily:      false,
		CanManageElders:       false,
	},
	RoleDoctor: {
		CanViewAllElders:      true,  // 可以看所有老人
		CanEditHealthRecords:  true,  // 可以编辑健康记录
		CanPrescribe:          true,  // 可以开处方
		CanViewSensitiveData:  true,  // 可以看敏感数据
		CanReceiveAlerts:      true,  // 接收医疗告警
		CanCallCaregiver:      false,
		CanViewOwnFamily:      false,
		CanManageElders:       true, // 可以管理老人信息
	},
	RoleFamily: {
		CanViewAllElders:      false, // 只能看自己的亲属
		CanEditHealthRecords:  false, // 不能编辑健康记录
		CanPrescribe:          false, // 不能开处方
		CanViewSensitiveData:  false, // 不能看敏感数据（脱敏后的）
		CanReceiveAlerts:      true,  // 接收异常通知
		CanCallCaregiver:      false,
		CanViewOwnFamily:      true, // 只能看自己的家人
		CanManageElders:       false,
	},
	RoleElder: {
		CanViewAllElders:      false, // 只能看自己
		CanEditHealthRecords:  false, // 不能编辑
		CanPrescribe:          false, // 不能开处方
		CanViewSensitiveData:  false, // 不能看敏感数据
		CanReceiveAlerts:      true,  // 接收用药提醒等
		CanCallCaregiver:      true,  // 可以呼叫护工
		CanViewOwnFamily:      false,
		CanManageElders:       false,
	},
}

// GetPermission 获取角色权限
func GetPermission(role Role) (Permission, error) {
	perm, exists := RolePermissions[role]
	if !exists {
		return Permission{}, fmt.Errorf("未知角色: %s", role)
	}
	return perm, nil
}

// IsValidRole 验证角色是否有效
func IsValidRole(role string) bool {
	r := Role(role)
	_, exists := RolePermissions[r]
	return exists
}

// GetRoleName 获取角色中文名称
func GetRoleName(role Role) string {
	names := map[Role]string{
		RoleCaregiver: "护工",
		RoleDoctor:    "医生",
		RoleFamily:    "家属",
		RoleElder:     "老人",
	}
	return names[role]
}

// ElderRelation 老人与用户的关系
type ElderRelation struct {
	ElderID  string // 老人ID
	UserID   string // 用户ID（护工、家属等）
	Role     Role   // 用户角色
	Relation string // 关系描述（如"负责护工"、"子女"、"配偶"）
}

// 内存存储（生产环境应使用数据库）
var (
	elderRelations = make(map[string][]ElderRelation) // key: elderID, value: 关联的用户列表
)

// AddElderRelation 添加老人关系
func AddElderRelation(elderID, userID string, role Role, relation string) {
	elderRelations[elderID] = append(elderRelations[elderID], ElderRelation{
		ElderID:  elderID,
		UserID:   userID,
		Role:     role,
		Relation: relation,
	})
}

// GetElderRelations 获取老人的所有关联用户
func GetElderRelations(elderID string) []ElderRelation {
	return elderRelations[elderID]
}

// GetUserElders 获取用户负责/关联的所有老人
func GetUserElders(userID string, role Role) []string {
	var elders []string
	for elderID, relations := range elderRelations {
		for _, rel := range relations {
			if rel.UserID == userID && rel.Role == role {
				elders = append(elders, elderID)
				break
			}
		}
	}
	return elders
}

// CanAccessElder 检查用户是否有权限访问某个老人的数据
func CanAccessElder(userID string, role Role, elderID string) bool {
	perm, err := GetPermission(role)
	if err != nil {
		return false
	}

	// 医生可以访问所有老人
	if perm.CanViewAllElders {
		return true
	}

	// 老人只能访问自己
	if role == RoleElder {
		return userID == elderID
	}

	// 护工和家属只能访问关联的老人
	relations := elderRelations[elderID]
	for _, rel := range relations {
		if rel.UserID == userID && rel.Role == role {
			return true
		}
	}

	return false
}

// InitDemoRelations 初始化演示数据（用于测试）
func InitDemoRelations() {
	// 清空现有数据
	elderRelations = make(map[string][]ElderRelation)

	// 老人1: 张奶奶 (elder_001)
	AddElderRelation("elder_001", "caregiver_001", RoleCaregiver, "负责护工")
	AddElderRelation("elder_001", "doctor_001", RoleDoctor, "主治医生")
	AddElderRelation("elder_001", "family_001", RoleFamily, "女儿")

	// 老人2: 李爷爷 (elder_002)
	AddElderRelation("elder_002", "caregiver_001", RoleCaregiver, "负责护工")
	AddElderRelation("elder_002", "doctor_001", RoleDoctor, "主治医生")
	AddElderRelation("elder_002", "family_002", RoleFamily, "儿子")

	// 老人3: 王奶奶 (elder_003)
	AddElderRelation("elder_003", "caregiver_002", RoleCaregiver, "负责护工")
	AddElderRelation("elder_003", "doctor_001", RoleDoctor, "主治医生")
	AddElderRelation("elder_003", "family_003", RoleFamily, "女儿")
}
