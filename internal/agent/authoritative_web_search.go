package agent

// AuthoritativeWebSearchTool provides a read-only, auditable fetcher for
// public health sources. It deliberately accepts an explicit source URL:
// arbitrary search engines and user-controlled redirects are not trusted
// data sources for a care assistant.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	defaultWebSearchTimeout = 5 * time.Second
	maxWebResponseBytes     = 1 << 20
)

var authoritativeDomains = []string{"who.int", "nhc.gov.cn", "chinacdc.cn", "nmpa.gov.cn"}

// WebSearchRequest is intentionally explicit. URL must point at the source
// article; query is used for local relevance filtering and is never sent to a
// third-party search engine.
type WebSearchRequest struct {
	Query string `json:"query"`
	URL   string `json:"url"`
}

type WebSearchStatus string

const (
	WebSearchOK            WebSearchStatus = "ok"
	WebSearchNotFound      WebSearchStatus = "not_found"
	WebSearchDisabled      WebSearchStatus = "disabled"
	WebSearchTimeout       WebSearchStatus = "timeout"
	WebSearchBlockedQuery  WebSearchStatus = "blocked_query"
	WebSearchInvalidSource WebSearchStatus = "invalid_source"
	WebSearchFetchError    WebSearchStatus = "fetch_error"
)

// WebSearchResult is the stable evidence contract consumed by the Agent/UI.
type WebSearchResult struct {
	Status        WebSearchStatus `json:"status"`
	Title         string          `json:"title,omitempty"`
	Summary       string          `json:"summary,omitempty"`
	URL           string          `json:"url,omitempty"`
	RetrievedAt   time.Time       `json:"retrieved_at"`
	ContentSHA256 string          `json:"content_sha256,omitempty"`
	SourceDomain  string          `json:"source_domain,omitempty"`
	Reason        string          `json:"reason,omitempty"`
}

// PublicSourceEvidence is the only portion of a web-tool observation that may
// be exposed by a browser-facing API. Extracted page text stays request-local.
type PublicSourceEvidence struct {
	Status        WebSearchStatus `json:"status"`
	Title         string          `json:"title,omitempty"`
	URL           string          `json:"url,omitempty"`
	RetrievedAt   time.Time       `json:"retrieved_at"`
	ContentSHA256 string          `json:"content_sha256,omitempty"`
	SourceDomain  string          `json:"source_domain,omitempty"`
	Reason        string          `json:"reason,omitempty"`
}

// PublicSourceEvidenceFromToolOutput converts an actual authoritative-tool
// result into stable public provenance. No other tool output is eligible.
func PublicSourceEvidenceFromToolOutput(toolName, observation string) (PublicSourceEvidence, bool) {
	if toolName != "authoritative_web_search" {
		return PublicSourceEvidence{}, false
	}
	var result WebSearchResult
	if err := json.Unmarshal([]byte(observation), &result); err != nil || result.Status == "" {
		return PublicSourceEvidence{}, false
	}
	if result.URL != "" {
		u, err := url.Parse(result.URL)
		if err != nil || !validSourceURL(u, authoritativeDomains) {
			return PublicSourceEvidence{}, false
		}
	}
	return PublicSourceEvidence{
		Status:        result.Status,
		Title:         result.Title,
		URL:           result.URL,
		RetrievedAt:   result.RetrievedAt,
		ContentSHA256: result.ContentSHA256,
		SourceDomain:  result.SourceDomain,
		Reason:        result.Reason,
	}, true
}

// WebSearchOption configures a client. AllowedDomains is useful for a local
// contract test; production defaults must remain the four authoritative sets.
type WebSearchOption func(*AuthoritativeWebSearchTool)

func WithWebSearchHTTPClient(client *http.Client) WebSearchOption {
	return func(t *AuthoritativeWebSearchTool) {
		if client != nil {
			t.client = client
		}
	}
}

func WithWebSearchAllowedDomains(domains ...string) WebSearchOption {
	return func(t *AuthoritativeWebSearchTool) {
		t.domains = normalizeDomains(domains)
	}
}

func WithWebSearchEnabled(enabled bool) WebSearchOption {
	return func(t *AuthoritativeWebSearchTool) { t.enabled = enabled }
}

type AuthoritativeWebSearchTool struct {
	client  *http.Client
	domains []string
	enabled bool
}

func NewAuthoritativeWebSearchTool(options ...WebSearchOption) *AuthoritativeWebSearchTool {
	t := &AuthoritativeWebSearchTool{
		client:  &http.Client{Timeout: defaultWebSearchTimeout},
		domains: append([]string(nil), authoritativeDomains...),
		enabled: true,
	}
	for _, option := range options {
		option(t)
	}
	return t
}

func (t *AuthoritativeWebSearchTool) Name() string     { return "authoritative_web_search" }
func (t *AuthoritativeWebSearchTool) IsReadOnly() bool { return true }
func (t *AuthoritativeWebSearchTool) Description() string {
	return "只读检索权威公共卫生资料。输入JSON：{\"query\":\"主题关键词\",\"url\":\"https://权威域名/资料链接\"}；仅允许WHO、国家卫健委、国家疾控、国家药监局，禁止姓名/病历/联系方式等个人信息。"
}

func (t *AuthoritativeWebSearchTool) Execute(ctx context.Context, input string) (string, error) {
	var req WebSearchRequest
	if err := json.Unmarshal([]byte(input), &req); err != nil {
		return "", fmt.Errorf("authoritative web search requires JSON input: %w", err)
	}
	result := t.Search(ctx, req)
	data, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (t *AuthoritativeWebSearchTool) Search(ctx context.Context, req WebSearchRequest) WebSearchResult {
	now := time.Now().UTC()
	result := WebSearchResult{Status: WebSearchOK, RetrievedAt: now}
	if !t.enabled {
		result.Status, result.Reason = WebSearchDisabled, "authoritative web search is disabled by policy"
		return result
	}
	if containsPersonalData(req.Query) {
		result.Status, result.Reason = WebSearchBlockedQuery, "query appears to contain personal or medical record identifiers"
		return result
	}
	if strings.TrimSpace(req.Query) == "" {
		result.Status, result.Reason = WebSearchInvalidSource, "query is required"
		return result
	}
	if strings.TrimSpace(req.URL) == "" {
		result.Status, result.Reason = WebSearchInvalidSource, "source URL is required; this tool fetches explicit sources only and does not query search engines"
		return result
	}
	u, err := url.Parse(strings.TrimSpace(req.URL))
	if err != nil || !validSourceURL(u, t.domains) || containsPersonalData(u.RawQuery) || containsPersonalData(u.Fragment) {
		result.Status, result.Reason = WebSearchInvalidSource, "source URL must be HTTPS and belong to an approved authority"
		return result
	}
	result.URL, result.SourceDomain = u.String(), strings.ToLower(u.Hostname())

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		result.Status, result.Reason = WebSearchInvalidSource, err.Error()
		return result
	}
	request.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain;q=0.8")
	request.Header.Set("User-Agent", "ContextKeeper-Research/1.0 (+auditable-read-only)")
	// Clone the client per request so redirect policy cannot be changed by a
	// concurrent caller. The callback runs before following each redirect.
	requestClient := *t.client
	previousRedirect := requestClient.CheckRedirect
	requestClient.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if !validSourceURL(next.URL, t.domains) {
			return http.ErrUseLastResponse
		}
		if previousRedirect != nil {
			return previousRedirect(next, via)
		}
		return nil
	}
	response, err := requestClient.Do(request)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			result.Status = WebSearchTimeout
		} else {
			result.Status = WebSearchFetchError
		}
		result.Reason = "source request failed"
		return result
	}
	defer response.Body.Close()
	// A valid initial URL must not be able to redirect the client to an
	// unapproved host. Validate the effective URL after the transport follows
	// redirects as well.
	if response.Request == nil || !validSourceURL(response.Request.URL, t.domains) {
		result.Status, result.Reason = WebSearchInvalidSource, "source redirected outside approved authorities"
		return result
	}
	if response.StatusCode == http.StatusNotFound {
		result.Status, result.Reason = WebSearchNotFound, "source returned 404"
		return result
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		result.Status, result.Reason = WebSearchFetchError, fmt.Sprintf("source returned HTTP %d", response.StatusCode)
		return result
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxWebResponseBytes+1))
	if err != nil {
		result.Status, result.Reason = WebSearchFetchError, "could not read source response"
		return result
	}
	if len(body) > maxWebResponseBytes {
		result.Status, result.Reason = WebSearchFetchError, "source response exceeds 1 MiB limit"
		return result
	}
	result.ContentSHA256 = sha256Hex(body)
	result.Title, result.Summary = extractHTMLEvidence(string(body))
	if result.Title == "" && result.Summary == "" {
		result.Status, result.Reason = WebSearchNotFound, "source contained no readable text"
		return result
	}
	return result
}

func validSourceURL(u *url.URL, domains []string) bool {
	if u == nil || strings.ToLower(u.Scheme) != "https" || u.User != nil || u.Hostname() == "" {
		return false
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	for _, domain := range domains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

func normalizeDomains(domains []string) []string {
	result := make([]string, 0, len(domains))
	for _, domain := range domains {
		domain = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(domain, ".")))
		if domain != "" {
			result = append(result, domain)
		}
	}
	return result
}

var (
	titlePattern    = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	tagPattern      = regexp.MustCompile(`(?is)<[^>]+>`)
	noisePattern    = regexp.MustCompile(`(?is)<!--.*?-->|<(script|style|noscript)[^>]*>.*?</(script|style|noscript)>`)
	spacePattern    = regexp.MustCompile(`\s+`)
	personalPattern = regexp.MustCompile(`(?i)(?:1[3-9]\d{9}|[\w.+-]+@[\w.-]+\.[A-Za-z]{2,}|\d{17}[\dXx]|(?:身份证|病历|病例|档案|手机号|手机号码|电话|联系方式|住址|地址|床号|用户ID|user[_ -]?id))`)
)

func containsPersonalData(query string) bool { return personalPattern.MatchString(query) }

func extractHTMLEvidence(raw string) (string, string) {
	title := ""
	if match := titlePattern.FindStringSubmatch(raw); len(match) == 2 {
		title = cleanText(match[1])
	}
	text := cleanText(noisePattern.ReplaceAllString(raw, " "))
	text = cleanText(tagPattern.ReplaceAllString(text, " "))
	if len([]rune(text)) > 900 {
		text = string([]rune(text)[:900]) + "…"
	}
	return title, text
}

func cleanText(value string) string {
	return strings.TrimSpace(spacePattern.ReplaceAllString(html.UnescapeString(value), " "))
}
func sha256Hex(value []byte) string { sum := sha256.Sum256(value); return hex.EncodeToString(sum[:]) }
