package models

import (
	"encoding/json"
	"testing"
)

// ==================== Memory与知识节点关联测试 ====================

// TestMemory_EntityIDs 测试Memory.EntityIDs字段
func TestMemory_EntityIDs(t *testing.T) {
	tests := []struct {
		name         string
		memory       *Memory
		wantCount    int
		wantContains string
	}{
		{
			name: "正常情况：包含3个EntityID（UUID）",
			memory: &Memory{
				ID:        "mem_001",
				EntityIDs: []string{"uuid-entity-001", "uuid-entity-002", "uuid-entity-003"},
			},
			wantCount:    3,
			wantContains: "uuid-entity-001",
		},
		{
			name: "空列表",
			memory: &Memory{
				ID:        "mem_002",
				EntityIDs: []string{},
			},
			wantCount: 0,
		},
		{
			name: "nil值",
			memory: &Memory{
				ID:        "mem_003",
				EntityIDs: nil,
			},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.memory.EntityIDs) != tt.wantCount {
				t.Errorf("EntityIDs count = %d, want %d", len(tt.memory.EntityIDs), tt.wantCount)
			}
			if tt.wantContains != "" {
				found := false
				for _, id := range tt.memory.EntityIDs {
					if id == tt.wantContains {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("EntityIDs should contain %s", tt.wantContains)
				}
			}
		})
	}
}

// TestMemory_EventIDs 测试Memory.EventIDs字段
func TestMemory_EventIDs(t *testing.T) {
	memory := &Memory{
		ID:       "mem_001",
		EventIDs: []string{"evt-uuid-001", "evt-uuid-002"},
	}

	if len(memory.EventIDs) != 2 {
		t.Errorf("EventIDs count = %d, want 2", len(memory.EventIDs))
	}

	// 检查是否包含特定值
	found := false
	for _, id := range memory.EventIDs {
		if id == "evt-uuid-001" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("EventIDs should contain evt-uuid-001")
	}
}

// TestMemory_SolutionIDs 测试Memory.SolutionIDs字段
func TestMemory_SolutionIDs(t *testing.T) {
	memory := &Memory{
		ID:          "mem_001",
		SolutionIDs: []string{"sol-uuid-001", "sol-uuid-002", "sol-uuid-003"},
	}

	if len(memory.SolutionIDs) != 3 {
		t.Errorf("SolutionIDs count = %d, want 3", len(memory.SolutionIDs))
	}
}

// TestMemory_JSONSerialization 测试Memory JSON序列化与反序列化
func TestMemory_JSONSerialization(t *testing.T) {
	memory := &Memory{
		ID:          "mem_001",
		Content:     "订单服务依赖MySQL",
		EntityIDs:   []string{"ent-uuid-001", "ent-uuid-002"},
		EventIDs:    []string{"evt-uuid-001"},
		SolutionIDs: []string{"sol-uuid-001"},
	}

	// 序列化
	jsonData, err := json.Marshal(memory)
	if err != nil {
		t.Fatalf("JSON Marshal error: %v", err)
	}

	// 反序列化
	var decoded Memory
	err = json.Unmarshal(jsonData, &decoded)
	if err != nil {
		t.Fatalf("JSON Unmarshal error: %v", err)
	}

	// 验证
	if decoded.ID != memory.ID {
		t.Errorf("ID = %s, want %s", decoded.ID, memory.ID)
	}
	if decoded.Content != memory.Content {
		t.Errorf("Content = %s, want %s", decoded.Content, memory.Content)
	}
	if len(decoded.EntityIDs) != len(memory.EntityIDs) {
		t.Errorf("EntityIDs count = %d, want %d", len(decoded.EntityIDs), len(memory.EntityIDs))
	}
	if len(decoded.EventIDs) != len(memory.EventIDs) {
		t.Errorf("EventIDs count = %d, want %d", len(decoded.EventIDs), len(memory.EventIDs))
	}
	if len(decoded.SolutionIDs) != len(memory.SolutionIDs) {
		t.Errorf("SolutionIDs count = %d, want %d", len(decoded.SolutionIDs), len(memory.SolutionIDs))
	}
}

// TestMemory_AllKnowledgeIDsEmpty 测试所有知识节点ID都为空的情况
func TestMemory_AllKnowledgeIDsEmpty(t *testing.T) {
	memory := &Memory{
		ID:      "mem_001",
		Content: "普通内容",
	}

	// 验证默认值
	if memory.EntityIDs != nil && len(memory.EntityIDs) > 0 {
		t.Errorf("EntityIDs should be nil or empty")
	}
	if memory.EventIDs != nil && len(memory.EventIDs) > 0 {
		t.Errorf("EventIDs should be nil or empty")
	}
	if memory.SolutionIDs != nil && len(memory.SolutionIDs) > 0 {
		t.Errorf("SolutionIDs should be nil or empty")
	}
}

