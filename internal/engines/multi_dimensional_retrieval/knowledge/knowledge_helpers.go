package knowledge

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// KnowledgeNodeIDs 知识节点ID集合
type KnowledgeNodeIDs struct {
	EntityIDs   []string `json:"entity_ids"`
	EventIDs    []string `json:"event_ids"`
	SolutionIDs []string `json:"solution_ids"`
}

// GenerateEntityUUID 生成Entity的确定性UUID
// 基于 name + type + workspace 生成，确保同一实体在同一工作空间内唯一
func GenerateEntityUUID(name, entityType, workspace string) string {
	content := fmt.Sprintf("entity:%s:%s:%s", strings.ToLower(name), entityType, workspace)
	return generateDeterministicUUID(content)
}

// GenerateEventUUID 生成Event的确定性UUID
// 基于 name + type + workspace 生成
func GenerateEventUUID(name, eventType, workspace string) string {
	content := fmt.Sprintf("event:%s:%s:%s", strings.ToLower(name), eventType, workspace)
	return generateDeterministicUUID(content)
}

// GenerateSolutionUUID 生成Solution的确定性UUID
// 基于 name + type + workspace 生成
func GenerateSolutionUUID(name, solutionType, workspace string) string {
	content := fmt.Sprintf("solution:%s:%s:%s", strings.ToLower(name), solutionType, workspace)
	return generateDeterministicUUID(content)
}

// GenerateFeatureUUID 生成Feature的确定性UUID
func GenerateFeatureUUID(name, workspace string) string {
	content := fmt.Sprintf("feature:%s:%s", strings.ToLower(name), workspace)
	return generateDeterministicUUID(content)
}

// GenerateRelationUUID 生成Relation的确定性UUID
func GenerateRelationUUID(sourceID, targetID, relationType string) string {
	content := fmt.Sprintf("relation:%s:%s:%s", sourceID, targetID, relationType)
	return generateDeterministicUUID(content)
}

// generateDeterministicUUID 基于内容生成确定性UUID（v5风格）
func generateDeterministicUUID(content string) string {
	hash := sha256.Sum256([]byte(content))
	// 取前16字节作为UUID
	uuidBytes := hash[:16]
	// 设置版本号（version 5）和变体
	uuidBytes[6] = (uuidBytes[6] & 0x0f) | 0x50 // version 5
	uuidBytes[8] = (uuidBytes[8] & 0x3f) | 0x80 // variant
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		uuidBytes[0:4], uuidBytes[4:6], uuidBytes[6:8], uuidBytes[8:10], uuidBytes[10:16])
}

// GenerateRandomUUID 生成随机UUID（用于新建节点）
func GenerateRandomUUID() string {
	return uuid.New().String()
}

// ExtractKnowledgeNodeIDs 从LLM分析结果中提取知识节点IDs
// analysisData 应该包含 entities, events, solutions 数组
func ExtractKnowledgeNodeIDs(analysisData map[string]interface{}, workspace string) *KnowledgeNodeIDs {
	result := &KnowledgeNodeIDs{
		EntityIDs:   []string{},
		EventIDs:    []string{},
		SolutionIDs: []string{},
	}

	// 提取Entities
	if entities, ok := analysisData["entities"].([]interface{}); ok {
		for _, e := range entities {
			if entity, ok := e.(map[string]interface{}); ok {
				name := getStringValue(entity, "name")
				entityType := getStringValue(entity, "type")
				if name != "" {
					if entityType == "" {
						entityType = EntityTypeConcept // 默认类型
					}
					entityID := GenerateEntityUUID(name, entityType, workspace)
					result.EntityIDs = append(result.EntityIDs, entityID)
				}
			}
		}
	}

	// 提取Events
	if events, ok := analysisData["events"].([]interface{}); ok {
		for _, e := range events {
			if event, ok := e.(map[string]interface{}); ok {
				name := getStringValue(event, "name")
				eventType := getStringValue(event, "type")
				if name != "" {
					if eventType == "" {
						eventType = EventTypeIssue // 默认类型
					}
					eventID := GenerateEventUUID(name, eventType, workspace)
					result.EventIDs = append(result.EventIDs, eventID)
				}
			}
		}
	}

	// 提取Solutions
	if solutions, ok := analysisData["solutions"].([]interface{}); ok {
		for _, s := range solutions {
			if solution, ok := s.(map[string]interface{}); ok {
				name := getStringValue(solution, "name")
				solutionType := getStringValue(solution, "type")
				if name != "" {
					if solutionType == "" {
						solutionType = SolutionTypeMethod // 默认类型
					}
					solutionID := GenerateSolutionUUID(name, solutionType, workspace)
					result.SolutionIDs = append(result.SolutionIDs, solutionID)
				}
			}
		}
	}

	return result
}

// BuildEntitiesFromAnalysis 从分析结果构建Entity对象列表
func BuildEntitiesFromAnalysis(analysisData map[string]interface{}, workspace string) []*Entity {
	var entities []*Entity
	now := time.Now()

	if entitiesData, ok := analysisData["entities"].([]interface{}); ok {
		for _, e := range entitiesData {
			if entityData, ok := e.(map[string]interface{}); ok {
				name := getStringValue(entityData, "name")
				entityType := getStringValue(entityData, "type")
				description := getStringValue(entityData, "description")

				if name == "" {
					continue
				}
				if entityType == "" {
					entityType = EntityTypeConcept
				}

				entity := &Entity{
					ID:          GenerateEntityUUID(name, entityType, workspace),
					Name:        name,
					Type:        entityType,
					Description: description,
					Workspace:   workspace,
					MemoryIDs:   []string{},
					CreatedAt:   now,
					UpdatedAt:   now,
				}
				entities = append(entities, entity)
			}
		}
	}
	return entities
}

// BuildEventsFromAnalysis 从分析结果构建Event对象列表
func BuildEventsFromAnalysis(analysisData map[string]interface{}, workspace string) []*Event {
	var events []*Event
	now := time.Now()

	if eventsData, ok := analysisData["events"].([]interface{}); ok {
		for _, e := range eventsData {
			if eventData, ok := e.(map[string]interface{}); ok {
				name := getStringValue(eventData, "name")
				eventType := getStringValue(eventData, "type")
				description := getStringValue(eventData, "description")

				if name == "" {
					continue
				}
				if eventType == "" {
					eventType = EventTypeIssue
				}

				event := &Event{
					ID:          GenerateEventUUID(name, eventType, workspace),
					Name:        name,
					Type:        eventType,
					Description: description,
					Workspace:   workspace,
					MemoryIDs:   []string{},
					CreatedAt:   now,
					UpdatedAt:   now,
				}
				events = append(events, event)
			}
		}
	}
	return events
}

// BuildSolutionsFromAnalysis 从分析结果构建Solution对象列表
func BuildSolutionsFromAnalysis(analysisData map[string]interface{}, workspace string) []*Solution {
	var solutions []*Solution
	now := time.Now()

	if solutionsData, ok := analysisData["solutions"].([]interface{}); ok {
		for _, s := range solutionsData {
			if solutionData, ok := s.(map[string]interface{}); ok {
				name := getStringValue(solutionData, "name")
				solutionType := getStringValue(solutionData, "type")
				description := getStringValue(solutionData, "description")

				if name == "" {
					continue
				}
				if solutionType == "" {
					solutionType = SolutionTypeMethod
				}

				solution := &Solution{
					ID:          GenerateSolutionUUID(name, solutionType, workspace),
					Name:        name,
					Type:        solutionType,
					Description: description,
					Workspace:   workspace,
					MemoryIDs:   []string{},
					CreatedAt:   now,
					UpdatedAt:   now,
				}
				solutions = append(solutions, solution)
			}
		}
	}
	return solutions
}

// BuildRelationsFromAnalysis 从分析结果构建Relation对象列表
func BuildRelationsFromAnalysis(analysisData map[string]interface{}, entityMap map[string]string) []*Relation {
	var relations []*Relation
	now := time.Now()

	if relationsData, ok := analysisData["relations"].([]interface{}); ok {
		for _, r := range relationsData {
			if relationData, ok := r.(map[string]interface{}); ok {
				sourceName := getStringValue(relationData, "source")
				targetName := getStringValue(relationData, "target")
				relationType := getStringValue(relationData, "type")
				weight := getFloatValue(relationData, "weight")

				if sourceName == "" || targetName == "" {
					continue
				}
				if relationType == "" {
					relationType = RelationRelatesTo
				}
				if weight == 0 {
					weight = 0.5 // 默认权重
				}

				// 查找源和目标的UUID
				sourceID, sourceOK := entityMap[sourceName]
				targetID, targetOK := entityMap[targetName]

				if !sourceOK || !targetOK {
					continue // 跳过未找到的实体
				}

				relation := &Relation{
					SourceID:  sourceID,
					TargetID:  targetID,
					Type:      relationType,
					Weight:    weight,
					CreatedAt: now,
				}
				relations = append(relations, relation)
			}
		}
	}
	return relations
}

// getStringValue 安全获取字符串值
func getStringValue(data map[string]interface{}, key string) string {
	if val, ok := data[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// getFloatValue 安全获取浮点值
func getFloatValue(data map[string]interface{}, key string) float64 {
	if val, ok := data[key]; ok {
		switch v := val.(type) {
		case float64:
			return v
		case float32:
			return float64(v)
		case int:
			return float64(v)
		case int64:
			return float64(v)
		}
	}
	return 0
}

// MergeKnowledgeNodeIDs 合并两个KnowledgeNodeIDs（去重）
func MergeKnowledgeNodeIDs(a, b *KnowledgeNodeIDs) *KnowledgeNodeIDs {
	result := &KnowledgeNodeIDs{
		EntityIDs:   mergeStringSlices(a.EntityIDs, b.EntityIDs),
		EventIDs:    mergeStringSlices(a.EventIDs, b.EventIDs),
		SolutionIDs: mergeStringSlices(a.SolutionIDs, b.SolutionIDs),
	}
	return result
}

// mergeStringSlices 合并字符串切片并去重
func mergeStringSlices(a, b []string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, s := range a {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	for _, s := range b {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

// ValidateUUID 验证UUID格式
func ValidateUUID(id string) bool {
	if len(id) != 36 {
		return false
	}
	// 简单验证格式：8-4-4-4-12
	parts := strings.Split(id, "-")
	if len(parts) != 5 {
		return false
	}
	expectedLengths := []int{8, 4, 4, 4, 12}
	for i, part := range parts {
		if len(part) != expectedLengths[i] {
			return false
		}
		// 验证是否为有效的十六进制
		if _, err := hex.DecodeString(part); err != nil {
			return false
		}
	}
	return true
}

