package agent

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type webRoundTripper func(*http.Request) (*http.Response, error)

func (f webRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

func testTool(roundTrip webRoundTripper) *AuthoritativeWebSearchTool {
	return NewAuthoritativeWebSearchTool(
		WithWebSearchAllowedDomains("who.int"),
		WithWebSearchHTTPClient(&http.Client{Transport: roundTrip, Timeout: time.Second}),
	)
}

func TestAuthoritativeWebSearchEvidenceContract(t *testing.T) {
	tool := testTool(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "https://who.int/guidance" {
			t.Fatalf("unexpected URL: %s", req.URL)
		}
		resp := response(http.StatusOK, `<html><head><title>压疮预防指南</title></head><body><script>secret</script><p>定期翻身并观察皮肤完整性。</p></body></html>`)
		resp.Request = req
		return resp, nil
	})
	result := tool.Search(context.Background(), WebSearchRequest{Query: "pressure ulcer prevention", URL: "https://who.int/guidance"})
	if result.Status != WebSearchOK || result.Title != "压疮预防指南" || !strings.Contains(result.Summary, "定期翻身") {
		t.Fatalf("unexpected evidence result: %#v", result)
	}
	if result.ContentSHA256 == "" || result.SourceDomain != "who.int" || result.RetrievedAt.IsZero() {
		t.Fatalf("missing audit fields: %#v", result)
	}
	if strings.Contains(result.Summary, "secret") {
		t.Fatal("script content must not appear in summary")
	}
}

func TestAuthoritativeWebSearchRejectsUntrustedSourceAndPII(t *testing.T) {
	tool := testTool(func(*http.Request) (*http.Response, error) { t.Fatal("transport must not be called"); return nil, nil })
	cases := []struct {
		name   string
		req    WebSearchRequest
		status WebSearchStatus
	}{
		{"non authority", WebSearchRequest{Query: "guidance", URL: "https://example.com/a"}, WebSearchInvalidSource},
		{"non tls", WebSearchRequest{Query: "guidance", URL: "http://who.int/a"}, WebSearchInvalidSource},
		{"phone", WebSearchRequest{Query: "请搜索 13800138000 的护理档案", URL: "https://who.int/a"}, WebSearchBlockedQuery},
		{"record label", WebSearchRequest{Query: "查询某人的病历号", URL: "https://who.int/a"}, WebSearchBlockedQuery},
		{"missing source URL", WebSearchRequest{Query: "pressure ulcer prevention"}, WebSearchInvalidSource},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tool.Search(context.Background(), tc.req).Status; got != tc.status {
				t.Fatalf("status=%s want %s", got, tc.status)
			}
		})
	}
}

func TestAuthoritativeWebSearchMissingURLDoesNotCallSearchEngine(t *testing.T) {
	called := false
	tool := testTool(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, errors.New("must not call transport")
	})
	result := tool.Search(context.Background(), WebSearchRequest{Query: "跌倒预防指南"})
	if result.Status != WebSearchInvalidSource {
		t.Fatalf("status=%s", result.Status)
	}
	if !strings.Contains(result.Reason, "explicit sources only") {
		t.Fatalf("reason=%q", result.Reason)
	}
	if called {
		t.Fatal("missing URL must not be sent to a search engine or HTTP transport")
	}
}

func TestAuthoritativeWebSearchStatusesAndSizeLimit(t *testing.T) {
	tool := testTool(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/missing":
			resp := response(http.StatusNotFound, "")
			resp.Request = req
			return resp, nil
		case "/large":
			resp := response(http.StatusOK, strings.Repeat("x", maxWebResponseBytes+1))
			resp.Request = req
			return resp, nil
		default:
			return nil, context.DeadlineExceeded
		}
	})
	if got := tool.Search(context.Background(), WebSearchRequest{Query: "guidance", URL: "https://who.int/missing"}).Status; got != WebSearchNotFound {
		t.Fatalf("404 status=%s", got)
	}
	if got := tool.Search(context.Background(), WebSearchRequest{Query: "guidance", URL: "https://who.int/large"}).Status; got != WebSearchFetchError {
		t.Fatalf("large status=%s", got)
	}
	if got := tool.Search(context.Background(), WebSearchRequest{Query: "guidance", URL: "https://who.int/slow"}).Status; got != WebSearchTimeout {
		t.Fatalf("timeout status=%s", got)
	}
	disabled := NewAuthoritativeWebSearchTool(WithWebSearchEnabled(false))
	if got := disabled.Search(context.Background(), WebSearchRequest{Query: "guidance", URL: "https://who.int/a"}).Status; got != WebSearchDisabled {
		t.Fatalf("disabled status=%s", got)
	}
}

func TestAuthoritativeWebSearchExecuteJSON(t *testing.T) {
	tool := testTool(func(req *http.Request) (*http.Response, error) {
		resp := response(http.StatusOK, `<title>WHO</title><p>Guidance</p>`)
		resp.Request = req
		return resp, nil
	})
	out, err := tool.Execute(context.Background(), `{"query":"guidance","url":"https://who.int/a"}`)
	if err != nil || !strings.Contains(out, `"status":"ok"`) || !strings.Contains(out, `"content_sha256"`) {
		t.Fatalf("output=%s err=%v", out, err)
	}
	if _, err := tool.Execute(context.Background(), "guidance"); err == nil {
		t.Fatal("non-JSON input must fail")
	}
}

func TestAuthoritativeWebSearchTransportError(t *testing.T) {
	tool := testTool(func(*http.Request) (*http.Response, error) { return nil, errors.New("connection refused") })
	if got := tool.Search(context.Background(), WebSearchRequest{Query: "guidance", URL: "https://who.int/a"}).Status; got != WebSearchFetchError {
		t.Fatalf("status=%s", got)
	}
}

func TestAuthoritativeWebSearchRejectsRedirectedHost(t *testing.T) {
	tool := testTool(func(req *http.Request) (*http.Response, error) {
		resp := response(http.StatusOK, `<title>untrusted</title><p>data</p>`)
		redirected := req.Clone(req.Context())
		redirected.URL = req.URL
		redirected.URL.Host = "example.com"
		resp.Request = redirected
		return resp, nil
	})
	if got := tool.Search(context.Background(), WebSearchRequest{Query: "guidance", URL: "https://who.int/a"}).Status; got != WebSearchInvalidSource {
		t.Fatalf("redirect status=%s", got)
	}
}
