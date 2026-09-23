package services

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// AuditChain is an append-only JSONL integrity log. It deliberately provides
// tamper evidence, not a claim of legal or blockchain immutability.
type AuditChain struct {
	path string
	mu   sync.Mutex
}

type AuditChainEntry struct {
	Version      int             `json:"version"`
	RecordedAt   time.Time       `json:"recorded_at"`
	PreviousHash string          `json:"previous_hash"`
	Payload      json.RawMessage `json:"payload"`
	EventHash    string          `json:"event_hash"`
}

func NewAuditChain(path string) (*AuditChain, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("audit chain path is required")
	}
	return &AuditChain{path: path}, nil
}

func (c *AuditChain) Append(payload any) (string, error) {
	if c == nil {
		return "", fmt.Errorf("audit chain is not configured")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal audit payload: %w", err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	previous, err := readAuditChainTail(c.path)
	if err != nil {
		return "", err
	}
	entry := AuditChainEntry{
		Version:      1,
		RecordedAt:   time.Now().UTC(),
		PreviousHash: previous,
		Payload:      encoded,
	}
	entry.EventHash = auditChainHash(entry.PreviousHash, entry.Payload)
	line, err := json.Marshal(entry)
	if err != nil {
		return "", fmt.Errorf("marshal audit entry: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o750); err != nil {
		return "", fmt.Errorf("create audit directory: %w", err)
	}
	file, err := os.OpenFile(c.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return "", fmt.Errorf("open audit chain: %w", err)
	}
	defer file.Close()
	if _, err := file.Write(append(line, '\n')); err != nil {
		return "", fmt.Errorf("append audit entry: %w", err)
	}
	if err := file.Sync(); err != nil {
		return "", fmt.Errorf("sync audit entry: %w", err)
	}
	return entry.EventHash, nil
}

// Verify checks ordering and hashes. It is intended for operational evidence
// verification and will detect modification, deletion in the middle, or a
// malformed JSONL entry.
func (c *AuditChain) Verify() error {
	if c == nil {
		return fmt.Errorf("audit chain is not configured")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	file, err := os.Open(c.path)
	if err != nil {
		return fmt.Errorf("open audit chain: %w", err)
	}
	defer file.Close()

	previous := ""
	scanner := bufio.NewScanner(file)
	line := 0
	for scanner.Scan() {
		line++
		var entry AuditChainEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			return fmt.Errorf("decode audit entry %d: %w", line, err)
		}
		if entry.Version != 1 || entry.PreviousHash != previous {
			return fmt.Errorf("audit chain discontinuity at entry %d", line)
		}
		if entry.EventHash != auditChainHash(entry.PreviousHash, entry.Payload) {
			return fmt.Errorf("audit chain hash mismatch at entry %d", line)
		}
		previous = entry.EventHash
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read audit chain: %w", err)
	}
	return nil
}

func readAuditChainTail(path string) (string, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("open audit chain: %w", err)
	}
	defer file.Close()
	var tail string
	scanner := bufio.NewScanner(file)
	line := 0
	for scanner.Scan() {
		line++
		var entry AuditChainEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			return "", fmt.Errorf("decode audit entry %d: %w", line, err)
		}
		tail = entry.EventHash
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read audit chain: %w", err)
	}
	return tail, nil
}

func auditChainHash(previous string, payload json.RawMessage) string {
	digest := sha256.Sum256(append(append([]byte(previous), '\n'), payload...))
	return hex.EncodeToString(digest[:])
}
