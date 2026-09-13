package engines

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/contextkeeper/service/internal/models"
)

type recordingTimelineStore struct {
	delay time.Duration
	mu    sync.Mutex
	req   *models.TimelineSearchRequest
}

func (s *recordingTimelineStore) SearchByQuery(_ context.Context, req *models.TimelineSearchRequest) ([]*models.TimelineEvent, error) {
	time.Sleep(s.delay)
	s.mu.Lock()
	copy := *req
	s.req = &copy
	s.mu.Unlock()
	return []*models.TimelineEvent{{ID: "timeline-1", Title: "timeline"}}, nil
}

func (s *recordingTimelineStore) SearchByID(context.Context, string) (*models.TimelineEvent, error) {
	return nil, nil
}

type recordingKnowledgeStore struct {
	delay time.Duration
	mu    sync.Mutex
	req   KnowledgeSearchRequest
}

func (s *recordingKnowledgeStore) SearchByQuery(_ context.Context, req KnowledgeSearchRequest) ([]*models.KnowledgeNode, error) {
	time.Sleep(s.delay)
	s.mu.Lock()
	s.req = req
	s.mu.Unlock()
	return []*models.KnowledgeNode{{ID: "knowledge-1", Name: "knowledge"}}, nil
}

type recordingVectorStore struct {
	delay  time.Duration
	mu     sync.Mutex
	query  string
	limit  int
	userID string
}

func (s *recordingVectorStore) SearchByQuery(ctx context.Context, query string, limit int) ([]*models.VectorMatch, error) {
	time.Sleep(s.delay)
	s.mu.Lock()
	s.query, s.limit = query, limit
	s.userID, _ = ctx.Value("user_id").(string)
	s.mu.Unlock()
	return []*models.VectorMatch{{ID: "vector-1", Content: "vector", Score: 0.9}}, nil
}

func TestParallelRetrieveReportsWallClockDurationAndPreservesScope(t *testing.T) {
	const delay = 60 * time.Millisecond
	timeline := &recordingTimelineStore{delay: delay}
	knowledge := &recordingKnowledgeStore{delay: delay}
	vector := &recordingVectorStore{delay: delay}
	retriever := NewMultiDimensionalRetriever(timeline, knowledge, vector)

	results, err := retriever.ParallelRetrieve(context.Background(), &models.MultiDimensionalQuery{
		TimelineQueries:  []string{"timeline query"},
		KnowledgeQueries: []string{"knowledge query"},
		VectorQueries:    []string{"vector query"},
		KeyConcepts:      []string{"blood pressure"},
		UserID:           "user-1",
		SessionID:        "session-1",
		WorkspaceID:      "workspace-1",
	})
	if err != nil {
		t.Fatalf("ParallelRetrieve() error = %v", err)
	}

	if results.TimelineCount != 1 || results.KnowledgeCount != 1 || results.VectorCount != 1 {
		t.Fatalf("source counts = timeline:%d knowledge:%d vector:%d, want 1 each", results.TimelineCount, results.KnowledgeCount, results.VectorCount)
	}
	if got, want := results.SourceStatuses, map[string]string{"timeline": "success", "knowledge": "success", "vector": "success"}; !mapsEqual(got, want) {
		t.Fatalf("source statuses = %#v, want %#v", got, want)
	}
	if results.RetrievalTime < int64(delay.Milliseconds()) || results.RetrievalTime >= int64((delay*2).Milliseconds()) {
		t.Fatalf("RetrievalTime = %dms, want concurrent wall-clock duration between %dms and %dms", results.RetrievalTime, delay.Milliseconds(), (delay * 2).Milliseconds())
	}
	if results.TimelineLatencyMs < int64(delay.Milliseconds()) || results.KnowledgeLatencyMs < int64(delay.Milliseconds()) || results.VectorLatencyMs < int64(delay.Milliseconds()) {
		t.Fatalf("per-source latency = timeline:%d knowledge:%d vector:%d, want each >= %dms", results.TimelineLatencyMs, results.KnowledgeLatencyMs, results.VectorLatencyMs, delay.Milliseconds())
	}

	timeline.mu.Lock()
	timelineRequest := timeline.req
	timeline.mu.Unlock()
	if timelineRequest == nil || timelineRequest.UserID != "user-1" || timelineRequest.SessionID != "session-1" || timelineRequest.WorkspaceID != "workspace-1" {
		t.Fatalf("timeline scope = %#v, want user/session/workspace propagation", timelineRequest)
	}
	knowledge.mu.Lock()
	knowledgeRequest := knowledge.req
	knowledge.mu.Unlock()
	if knowledgeRequest.UserID != "user-1" || knowledgeRequest.SessionID != "session-1" || knowledgeRequest.WorkspaceID != "workspace-1" {
		t.Fatalf("knowledge scope = %#v, want user/session/workspace propagation", knowledgeRequest)
	}
	vector.mu.Lock()
	vectorQuery, vectorLimit, vectorUserID := vector.query, vector.limit, vector.userID
	vector.mu.Unlock()
	if vectorQuery != "vector query" || vectorLimit != retriever.config.VectorMaxResults {
		t.Fatalf("vector call = query:%q limit:%d, want configured query and limit", vectorQuery, vectorLimit)
	}
	if vectorUserID != "user-1" {
		t.Fatalf("vector user_id = %q, want query tenant propagated to context", vectorUserID)
	}
}

func TestParallelRetrievePreservesAuthenticatedContextTenant(t *testing.T) {
	vector := &recordingVectorStore{}
	retriever := NewMultiDimensionalRetriever(nil, nil, vector)
	ctx := context.WithValue(context.Background(), "user_id", "authenticated-user")

	_, err := retriever.ParallelRetrieve(ctx, &models.MultiDimensionalQuery{
		VectorQueries: []string{"vector query"},
		UserID:        "query-user-should-not-override",
	})
	if err != nil {
		t.Fatalf("ParallelRetrieve() error = %v", err)
	}

	vector.mu.Lock()
	got := vector.userID
	vector.mu.Unlock()
	if got != "authenticated-user" {
		t.Fatalf("vector user_id = %q, want authenticated context tenant", got)
	}
}

func mapsEqual(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}
