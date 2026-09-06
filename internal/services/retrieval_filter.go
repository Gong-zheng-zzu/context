package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
)

const (
	// RetrievalRuleFilterEnabledEnv controls whether the loaded rules are applied.
	RetrievalRuleFilterEnabledEnv = "RETRIEVAL_RULE_FILTER_ENABLED"
	// RetrievalRuleFilterRulesetEnv specifies the versioned JSON ruleset to load.
	RetrievalRuleFilterRulesetEnv = "RETRIEVAL_RULE_FILTER_RULESET"

	RemovalReasonCrossUser       = "cross_user"
	RemovalReasonCrossSession    = "cross_session"
	RemovalReasonEmptyDocID      = "empty_doc_id"
	RemovalReasonLowScore        = "low_score"
	RemovalReasonInvalidMetadata = "invalid_metadata"
)

// RetrievalFilterRuleset is a versioned, declarative retrieval policy.
// InvalidMetadataKeys are metadata keys whose true value rejects a candidate.
type RetrievalFilterRuleset struct {
	Version             string   `json:"version"`
	MinScore            float64  `json:"min_score"`
	RequireUserMatch    bool     `json:"require_user_match"`
	RequireSessionMatch bool     `json:"require_session_match"`
	InvalidMetadataKeys []string `json:"invalid_metadata_keys"`
}

// RetrievalFilterConfig combines an enabled switch with its immutable ruleset identity.
type RetrievalFilterConfig struct {
	Enabled       bool                   `json:"enabled"`
	Ruleset       RetrievalFilterRuleset `json:"ruleset"`
	RulesetSHA256 string                 `json:"ruleset_sha256"`
}

// RetrievalFilterScope is the requester identity against which candidates are checked.
type RetrievalFilterScope struct {
	UserID    string `json:"user_id"`
	SessionID string `json:"session_id"`
}

// RetrievalFilterCandidate is the source-neutral record consumed by the filter.
// Callers map native vector, graph, or timeline results into this value before filtering.
type RetrievalFilterCandidate struct {
	Index     int                    `json:"-"`
	DocID     string                 `json:"doc_id"`
	UserID    string                 `json:"user_id"`
	SessionID string                 `json:"session_id"`
	Score     float64                `json:"score"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// RetrievalFilterAudit records the exact policy identity and candidate counts for one run.
type RetrievalFilterAudit struct {
	Enabled        bool           `json:"enabled"`
	RulesetVersion string         `json:"ruleset_version"`
	RulesetSHA256  string         `json:"ruleset_sha256"`
	Before         int            `json:"before"`
	After          int            `json:"after"`
	RemovalReasons map[string]int `json:"removal_reasons"`
}

// LoadRetrievalFilterConfigFromEnv loads the JSON policy referenced by the retrieval filter
// environment variables. A missing enabled variable means the filter is disabled.
func LoadRetrievalFilterConfigFromEnv() (RetrievalFilterConfig, error) {
	enabled, err := envBool(RetrievalRuleFilterEnabledEnv)
	if err != nil {
		return RetrievalFilterConfig{}, err
	}

	rulesetPath := strings.TrimSpace(os.Getenv(RetrievalRuleFilterRulesetEnv))
	if rulesetPath == "" {
		return RetrievalFilterConfig{}, fmt.Errorf("%s is required", RetrievalRuleFilterRulesetEnv)
	}

	contents, err := os.ReadFile(rulesetPath)
	if err != nil {
		return RetrievalFilterConfig{}, fmt.Errorf("read retrieval filter ruleset: %w", err)
	}

	var ruleset RetrievalFilterRuleset
	if err := json.Unmarshal(contents, &ruleset); err != nil {
		return RetrievalFilterConfig{}, fmt.Errorf("parse retrieval filter ruleset: %w", err)
	}
	if err := validateRetrievalFilterRuleset(ruleset); err != nil {
		return RetrievalFilterConfig{}, err
	}

	digest := sha256.Sum256(contents)
	return RetrievalFilterConfig{
		Enabled:       enabled,
		Ruleset:       ruleset,
		RulesetSHA256: hex.EncodeToString(digest[:]),
	}, nil
}

// FilterRetrievalCandidates filters source-neutral candidates without mutating their metadata.
// A disabled configuration preserves all candidates while still producing audit counts.
func FilterRetrievalCandidates(config RetrievalFilterConfig, scope RetrievalFilterScope, candidates []RetrievalFilterCandidate) ([]RetrievalFilterCandidate, RetrievalFilterAudit) {
	audit := RetrievalFilterAudit{
		Enabled:        config.Enabled,
		RulesetVersion: config.Ruleset.Version,
		RulesetSHA256:  config.RulesetSHA256,
		Before:         len(candidates),
		RemovalReasons: make(map[string]int),
	}
	if !config.Enabled {
		filtered := append([]RetrievalFilterCandidate(nil), candidates...)
		audit.After = len(filtered)
		return filtered, audit
	}

	filtered := make([]RetrievalFilterCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if reason := retrievalRemovalReason(config.Ruleset, scope, candidate); reason != "" {
			audit.RemovalReasons[reason]++
			continue
		}
		filtered = append(filtered, candidate)
	}
	audit.After = len(filtered)
	return filtered, audit
}

func envBool(name string) (bool, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return false, nil
	}
	enabled, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("parse %s: %w", name, err)
	}
	return enabled, nil
}

func validateRetrievalFilterRuleset(ruleset RetrievalFilterRuleset) error {
	if strings.TrimSpace(ruleset.Version) == "" {
		return fmt.Errorf("retrieval filter ruleset version is required")
	}
	if math.IsNaN(ruleset.MinScore) || math.IsInf(ruleset.MinScore, 0) || ruleset.MinScore < 0 || ruleset.MinScore > 1 {
		return fmt.Errorf("retrieval filter min_score must be finite and within [0, 1]")
	}
	for _, key := range ruleset.InvalidMetadataKeys {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("retrieval filter invalid_metadata_keys must not contain empty keys")
		}
	}
	return nil
}

func retrievalRemovalReason(ruleset RetrievalFilterRuleset, scope RetrievalFilterScope, candidate RetrievalFilterCandidate) string {
	if ruleset.RequireUserMatch && strings.TrimSpace(scope.UserID) != "" && candidate.UserID != scope.UserID {
		return RemovalReasonCrossUser
	}
	if ruleset.RequireSessionMatch && strings.TrimSpace(scope.SessionID) != "" && candidate.SessionID != scope.SessionID {
		return RemovalReasonCrossSession
	}
	if strings.TrimSpace(candidate.DocID) == "" {
		return RemovalReasonEmptyDocID
	}
	if math.IsNaN(candidate.Score) || candidate.Score < ruleset.MinScore {
		return RemovalReasonLowScore
	}
	for _, key := range ruleset.InvalidMetadataKeys {
		if isInvalidMetadataValue(candidate.Metadata[key]) {
			return RemovalReasonInvalidMetadata
		}
	}
	return ""
}

func isInvalidMetadataValue(value interface{}) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		return err == nil && parsed
	default:
		return false
	}
}
