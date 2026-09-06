package timeline

import (
	"strings"
	"testing"
)

func TestTimelineSearchTermsKeepsOriginalChineseQueryAndUsefulTerms(t *testing.T) {
	terms := timelineSearchTerms("查询 Redis 缓存故障", []string{"Redis", "缓存", "", "redis", "故障"})
	want := []string{"查询 Redis 缓存故障", "Redis", "缓存", "故障"}
	if len(terms) != len(want) {
		t.Fatalf("terms = %v, want %v", terms, want)
	}
	for i := range want {
		if terms[i] != want[i] {
			t.Fatalf("terms[%d] = %q, want %q", i, terms[i], want[i])
		}
	}
}

func TestBuildRetrievalQueryKeepsChineseSearchScoped(t *testing.T) {
	query := &TimelineQuery{
		UserID:      "user-1",
		SessionID:   "session-1",
		WorkspaceID: "workspace-1",
		SearchText:  "查询 Redis 缓存故障",
		Keywords:    []string{"Redis", "缓存", "故障"},
		Limit:       10,
	}

	sqlQuery, args := (&TimescaleDBEngine{}).buildRetrievalQuery(query)
	for _, scope := range []string{"user_id = $1", "workspace_id = $2", "session_id = $3"} {
		if !strings.Contains(sqlQuery, scope) {
			t.Fatalf("query is missing scope %q:\n%s", scope, sqlQuery)
		}
	}
	if strings.Contains(sqlQuery, "chinese_zh") {
		t.Fatalf("query must not require an optional Chinese text-search configuration:\n%s", sqlQuery)
	}
	if !strings.Contains(sqlQuery, "ILIKE ANY") {
		t.Fatalf("query must literally match the original query and extracted terms:\n%s", sqlQuery)
	}
	if got, ok := args[3].(string); !ok || got != "查询 Redis 缓存故障 Redis 缓存 故障" {
		t.Fatalf("full-text argument = %#v, want original query plus extracted terms", args[3])
	}
}

func TestBuildEventCountQueryUsesTheSameScopedFilters(t *testing.T) {
	query := &TimelineQuery{
		UserID:      "user-1",
		SessionID:   "session-1",
		WorkspaceID: "workspace-1",
		SearchText:  "Redis 缓存故障",
		Keywords:    []string{"Redis", "缓存"},
		Limit:       10,
	}

	countSQL, args := (&TimescaleDBEngine{}).buildEventCountQuery(query)
	if strings.Contains(countSQL, "ORDER BY") || strings.Contains(countSQL, "LIMIT") || strings.Contains(countSQL, "OFFSET") {
		t.Fatalf("count query must not include pagination or ordering:\n%s", countSQL)
	}
	for _, scope := range []string{"user_id = $1", "workspace_id = $2", "session_id = $3", "ILIKE ANY"} {
		if !strings.Contains(countSQL, scope) {
			t.Fatalf("count query is missing filter %q:\n%s", scope, countSQL)
		}
	}
	if len(args) != 7 {
		t.Fatalf("count arguments = %d, want 7 without pagination", len(args))
	}
}
