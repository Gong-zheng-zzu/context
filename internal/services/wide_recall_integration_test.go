package services

import (
	"context"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/contextkeeper/service/internal/models"
)

// TestWideRecallIntegration 宽召回集成测试
func TestWideRecallIntegration(t *testing.T) {
	// === 创建模拟存储 ===
	mockTimelineStore := &MockTimelineStore{}
	mockKnowledgeStore := &MockKnowledgeStore{}
	mockVectorStore := &MockVectorStore{}
	mockLLMService := &MockLLMService{}

	// === 创建宽召回服务 ===
	wideRecallConfig := &WideRecallConfig{
		LLMTimeout:           30,
		TimelineTimeout:      5,
		KnowledgeTimeout:     5,
		VectorTimeout:        5,
		TimelineMaxResults:   20,
		KnowledgeMaxResults:  15,
		VectorMaxResults:     25,
		MinSimilarityScore:   0.6,
		MinRelevanceScore:    0.5,
		ConfidenceThreshold:  0.7,
		UpdateThreshold:      0.4,
		PersistenceThreshold: 0.7,
		MaxRetries:           2,
		RetryInterval:        1,
	}

	wideRecallService := NewWideRecallService(
		mockTimelineStore,
		mockKnowledgeStore,
		mockVectorStore,
		mockLLMService,
		wideRecallConfig,
	)

	// === 创建宽召回上下文管理器 ===
	contextConfig := &WideRecallContextConfig{
		MemoryThreshold:      0.4,
		PersistenceThreshold: 0.7,
		MaxCacheSize:         100,
		CacheExpiry:          30 * time.Minute,
		CleanupInterval:      5 * time.Minute,
		MaxConcurrency:       5,
	}

	contextManager := NewWideRecallContextManager(wideRecallService, contextConfig)
	defer contextManager.Stop()

	// === 测试用例1: 首次上下文创建 ===
	t.Run("首次上下文创建", func(t *testing.T) {
		req := &models.ContextUpdateRequest{
			UserID:      "test_user_001",
			SessionID:   "test_session_001",
			WorkspaceID: "/test/workspace",
			UserQuery:   "如何实现一个高性能的缓存系统？",
		}

		// 执行上下文更新
		resp, err := contextManager.UpdateContextWithWideRecall(req)
		if err != nil {
			t.Fatalf("上下文创建失败: %v", err)
		}

		// 验证响应
		if !resp.Success {
			t.Errorf("期望成功，但得到失败: %s", resp.UpdateSummary)
		}

		if resp.UpdatedContext == nil {
			t.Error("期望得到更新的上下文，但为nil")
		}

		if resp.UpdatedContext.SessionID != req.SessionID {
			t.Errorf("期望会话ID %s，但得到 %s", req.SessionID, resp.UpdatedContext.SessionID)
		}

		t.Logf("✅ 首次上下文创建成功，置信度: %.2f", resp.ConfidenceLevel)
	})

	// === 测试用例2: 上下文更新 ===
	t.Run("上下文更新", func(t *testing.T) {
		req := &models.ContextUpdateRequest{
			UserID:      "test_user_001",
			SessionID:   "test_session_001", // 使用相同的会话ID
			WorkspaceID: "/test/workspace",
			UserQuery:   "缓存系统的数据一致性如何保证？",
		}

		// 执行上下文更新
		resp, err := contextManager.UpdateContextWithWideRecall(req)
		if err != nil {
			t.Fatalf("上下文更新失败: %v", err)
		}

		// 验证响应
		if !resp.Success {
			t.Errorf("期望成功，但得到失败: %s", resp.UpdateSummary)
		}

		t.Logf("✅ 上下文更新成功，置信度: %.2f", resp.ConfidenceLevel)
	})

	// === 测试用例3: 不同会话的上下文隔离 ===
	t.Run("上下文隔离", func(t *testing.T) {
		req := &models.ContextUpdateRequest{
			UserID:      "test_user_001",
			SessionID:   "test_session_002", // 不同的会话ID
			WorkspaceID: "/test/workspace",
			UserQuery:   "如何设计微服务架构？",
		}

		// 执行上下文创建
		resp, err := contextManager.UpdateContextWithWideRecall(req)
		if err != nil {
			t.Fatalf("新会话上下文创建失败: %v", err)
		}

		// 验证这是一个新的上下文
		if !resp.Success {
			t.Errorf("期望成功，但得到失败: %s", resp.UpdateSummary)
		}

		t.Logf("✅ 新会话上下文创建成功，置信度: %.2f", resp.ConfidenceLevel)
	})
}

// MockTimelineStore 模拟时间线存储
type MockTimelineStore struct{}

func (m *MockTimelineStore) SearchEvents(ctx context.Context, req *TimelineSearchRequest) ([]*TimelineSearchResult, error) {
	return []*TimelineSearchResult{
		{
			EventID:         "event_001",
			EventType:       "code_change",
			Title:           "实现缓存接口",
			Content:         "添加了Redis缓存接口的实现",
			Timestamp:       time.Now().Add(-2 * time.Hour),
			Source:          "git",
			ImportanceScore: 0.8,
			RelevanceScore:  0.9,
			Tags:            []string{"cache", "redis"},
			Metadata:        map[string]interface{}{"commit_id": "abc123"},
		},
	}, nil
}

// MockKnowledgeStore 模拟知识图谱存储
type MockKnowledgeStore struct{}

func (m *MockKnowledgeStore) SearchConcepts(ctx context.Context, req *KnowledgeSearchRequest) ([]*KnowledgeSearchResult, error) {
	return []*KnowledgeSearchResult{
		{
			ConceptID:       "concept_001",
			ConceptName:     "缓存系统",
			ConceptType:     "技术概念",
			Description:     "用于提高数据访问速度的存储系统",
			RelevanceScore:  0.9,
			ConfidenceScore: 0.8,
			Source:          "技术文档",
			LastUpdated:     time.Now().Add(-1 * time.Hour),
			Properties:      map[string]interface{}{"category": "backend"},
			RelatedConcepts: []RelatedConcept{
				{
					ConceptName:    "Redis",
					RelationType:   "实现方式",
					RelationWeight: 0.9,
				},
			},
		},
	}, nil
}

// MockVectorStore 模拟向量存储
type MockVectorStore struct{}

func (m *MockVectorStore) SearchSimilar(ctx context.Context, req *VectorSearchRequest) ([]*VectorSearchResult, error) {
	return []*VectorSearchResult{
		{
			DocumentID:     "doc_001",
			Content:        "缓存系统设计需要考虑数据一致性、性能和可扩展性",
			ContentType:    "text",
			Source:         "技术文档",
			Similarity:     0.85,
			RelevanceScore: 0.8,
			Timestamp:      time.Now().Add(-1 * time.Hour),
			Tags:           []string{"cache", "design"},
			Metadata:       map[string]interface{}{"doc_type": "technical"},
			MatchedSegments: []MatchedSegment{
				{
					SegmentText: "缓存系统设计",
					StartPos:    0,
					EndPos:      6,
					Similarity:  0.9,
				},
			},
		},
	}, nil
}

// MockLLMService 模拟LLM服务
type MockLLMService struct{}

// NewMockLLMService returns the deterministic test implementation used by
// context-manager tests. It never performs network calls.
func NewMockLLMService() *MockLLMService { return &MockLLMService{} }

func (m *MockLLMService) GenerateResponse(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	// 添加调试信息
	promptPreview := req.Prompt
	if len(promptPreview) > 100 {
		promptPreview = promptPreview[:100] + "..."
	}
	log.Printf("🔍 [Mock LLM] 收到请求，Format: %s, Prompt前100字符: %s", req.Format, promptPreview)

	// 根据请求内容返回不同的模拟响应
	if req.Format == "json" {
		// 检查是否是上下文合成请求（优先匹配，因为它也包含"分析"字样）
		if strings.Contains(req.Prompt, "上下文评估与合成") || strings.Contains(req.Prompt, "上下文合成") || strings.Contains(req.Prompt, "context_synthesis") {
			log.Printf("🎯 [Mock LLM] 匹配到上下文合成请求，返回合成响应")
			return &GenerateResponse{
				Content: `{
					"evaluation_result": {
						"should_update": true,
						"update_confidence": 0.8,
						"evaluation_reason": "检索到相关技术信息，建议更新上下文"
					},
					"synthesized_context": {
						"session_id": "test_session",
						"user_id": "test_user",
						"workspace_id": "/test/workspace",
						"current_topic": {
							"main_topic": "缓存系统设计",
							"topic_category": "technical",
							"user_intent": {
								"intent_type": "query",
								"intent_description": "技术咨询",
								"priority": "medium"
							},
							"primary_pain_point": "需要了解缓存系统设计",
							"expected_outcome": "获得设计指导",
							"key_concepts": [{"concept_name": "缓存", "concept_type": "technical"}],
							"confidence_level": 0.8
						}
					},
					"synthesis_metadata": {
						"information_sources": {
							"timeline_contribution": 0.3,
							"knowledge_contribution": 0.4,
							"vector_contribution": 0.3
						},
						"quality_assessment": {
							"overall_quality": 0.8,
							"information_conflicts": [],
							"information_gaps": []
						}
					}
				}`,
				Usage: Usage{
					PromptTokens:     200,
					CompletionTokens: 100,
					TotalTokens:      300,
				},
			}, nil
		}

		// 检查是否是意图分析请求
		if strings.Contains(req.Prompt, "意图分析") || strings.Contains(req.Prompt, "intent_analysis") {
			log.Printf("🎯 [Mock LLM] 匹配到意图分析请求，返回意图分析响应")
			return &GenerateResponse{
				Content: `{
					"intent_analysis": {
						"core_intent": "技术咨询",
						"intent_type": "query",
						"intent_category": "技术实现",
						"key_concepts": ["缓存", "系统设计", "性能优化"],
						"time_scope": "immediate",
						"urgency_level": "medium",
						"expected_outcome": "获得缓存系统设计指导"
					},
					"key_extraction": {
						"project_keywords": ["缓存", "系统"],
						"technical_keywords": ["性能", "设计", "实现"],
						"domain_keywords": ["后端", "架构"]
					},
					"retrieval_strategy": {
						"timeline_queries": [{"query_text": "缓存 实现", "time_range": "24h", "event_types": ["code_change"], "priority": 3}],
						"knowledge_queries": [{"query_text": "缓存系统", "concept_types": ["技术概念"], "relation_types": ["实现"], "priority": 3}],
						"vector_queries": [{"query_text": "缓存设计", "similarity_threshold": 0.7, "priority": 3}]
					}
				}`,
				Usage: Usage{
					PromptTokens:     100,
					CompletionTokens: 50,
					TotalTokens:      150,
				},
			}, nil
		}
	}

	return &GenerateResponse{
		Content: "基于检索到的信息，缓存系统设计需要考虑数据一致性、性能和可扩展性等关键因素。",
		Usage: Usage{
			PromptTokens:     30,
			CompletionTokens: 20,
			TotalTokens:      50,
		},
	}, nil
}

func (m *MockLLMService) AnalyzeUserIntent(userQuery string) (*models.IntentAnalysisResult, error) {
	return &models.IntentAnalysisResult{
		CoreIntentText:       "技术咨询",
		DomainContextText:    "后端开发",
		ScenarioText:         "用户询问技术实现方案",
		IntentCount:          1,
		MultiIntentBreakdown: []string{"技术咨询"},
	}, nil
}

func (m *MockLLMService) SynthesizeAndEvaluateContext(
	userQuery string,
	currentContext *models.UnifiedContextModel,
	retrievalResults *models.ParallelRetrievalResult,
	intentAnalysis *models.IntentAnalysisResult,
) (*models.ContextSynthesisResult, error) {
	if currentContext == nil {
		currentContext = &models.UnifiedContextModel{
			SessionID: "mock-session", UserID: "mock-user", WorkspaceID: "mock-workspace",
			CurrentTopic: &models.TopicContext{MainTopic: userQuery, ConfidenceLevel: 0.8},
		}
	}
	return &models.ContextSynthesisResult{
		ShouldUpdate:     true,
		UpdateConfidence: 0.8,
		EvaluationReason: "检索到相关技术信息，需要更新上下文",
		UpdatedContext:   currentContext,
	}, nil
}
