package services

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/contextkeeper/service/internal/config"
	"github.com/contextkeeper/service/internal/engines/multi_dimensional_retrieval/knowledge"
	"github.com/contextkeeper/service/internal/engines/multi_dimensional_retrieval/timeline"
	"github.com/contextkeeper/service/internal/models"
	"github.com/contextkeeper/service/internal/store"
	"github.com/contextkeeper/service/internal/utils"
)

var ErrSessionOwnerMismatch = errors.New("authenticated user does not own session")

type sessionReplica interface {
	CountSessionReplicaRecords(ctx context.Context, userID, sessionID string) (int64, error)
	DeleteSessionReplicaRecords(ctx context.Context, userID, sessionID string) (int64, error)
}

// sessionVectorStore is intentionally narrower than models.VectorStore. It
// keeps the cascade service independent from the Qdrant implementation while
// still allowing the configured collection to be audited and cleaned.
type sessionVectorStore interface {
	GetVectorsByUserID(ctx context.Context, collectionName, userID string) ([]*models.VectorRecord, error)
	DeleteVectors(ctx context.Context, collectionName string, vectorIDs []string) error
}

// sessionScopedVectorStore is implemented by Qdrant. Keeping it optional
// preserves support for other providers while ensuring the production Qdrant
// cascade never derives a session count from a user-wide listing.
type sessionScopedVectorStore interface {
	CountVectorsByUserAndSessionID(ctx context.Context, collectionName, userID, sessionID string) (int, error)
	DeleteVectorsByUserAndSessionID(ctx context.Context, collectionName, userID, sessionID string) (int, error)
}

type vectorSessionReplica struct {
	store      sessionVectorStore
	collection string
}

func (r *vectorSessionReplica) CountSessionReplicaRecords(ctx context.Context, userID, sessionID string) (int64, error) {
	if scoped, ok := r.store.(sessionScopedVectorStore); ok {
		count, err := scoped.CountVectorsByUserAndSessionID(ctx, r.collection, userID, sessionID)
		if err != nil {
			return 0, fmt.Errorf("count Qdrant vectors for session: %w", err)
		}
		return int64(count), nil
	}
	records, err := r.store.GetVectorsByUserID(ctx, r.collection, userID)
	if err != nil {
		return 0, fmt.Errorf("list Qdrant vectors: %w", err)
	}
	return int64(len(sessionVectorIDs(records, userID, sessionID))), nil
}

func (r *vectorSessionReplica) DeleteSessionReplicaRecords(ctx context.Context, userID, sessionID string) (int64, error) {
	if scoped, ok := r.store.(sessionScopedVectorStore); ok {
		deleted, err := scoped.DeleteVectorsByUserAndSessionID(ctx, r.collection, userID, sessionID)
		if err != nil {
			return 0, fmt.Errorf("delete Qdrant vectors for session: %w", err)
		}
		return int64(deleted), nil
	}
	records, err := r.store.GetVectorsByUserID(ctx, r.collection, userID)
	if err != nil {
		return 0, fmt.Errorf("list Qdrant vectors: %w", err)
	}
	ids := sessionVectorIDs(records, userID, sessionID)
	if len(ids) == 0 {
		return 0, nil
	}
	if err := r.store.DeleteVectors(ctx, r.collection, ids); err != nil {
		return 0, fmt.Errorf("delete Qdrant vectors: %w", err)
	}
	return int64(len(ids)), nil
}

func sessionVectorIDs(records []*models.VectorRecord, userID, sessionID string) []string {
	ids := make([]string, 0, len(records))
	for _, record := range records {
		if record == nil || record.ID == "" || record.Metadata == nil {
			continue
		}
		storedUserID := record.UserID
		if storedUserID == "" {
			storedUserID, _ = record.Metadata["user_id"].(string)
		}
		if storedUserID != userID {
			continue
		}
		storedSessionID, _ := record.Metadata["session_id"].(string)
		if storedSessionID == sessionID {
			ids = append(ids, record.ID)
		}
	}
	return ids
}

type sessionReplicaBinding struct {
	name       string
	replica    sessionReplica
	skipReason string
}

// SessionDeletionStoreResult is an auditable outcome for one storage location.
type SessionDeletionStoreResult struct {
	Store   string `json:"store"`
	Status  string `json:"status"`
	Before  int64  `json:"before"`
	Deleted int64  `json:"deleted"`
	After   int64  `json:"after"`
	Error   string `json:"error,omitempty"`
}

// SessionDeletionResult is returned for successful, dry-run, and failed cascades.
type SessionDeletionResult struct {
	SessionID      string    `json:"session_id"`
	UserID         string    `json:"user_id"`
	DryRun         bool      `json:"dry_run"`
	VerificationID string    `json:"verification_id"`
	TraceID        string    `json:"trace_id"`
	VerifiedAt     time.Time `json:"verified_at"`
	ScopeSHA256    string    `json:"scope_sha256"`
	EvidenceSHA256 string    `json:"evidence_sha256,omitempty"`
	// Complete is true only after every configured durable replica and the
	// local session cache/file have been verified as deleted.
	Complete bool                         `json:"complete"`
	Stores   []SessionDeletionStoreResult `json:"stores"`
}

// SessionDeletionService removes one owner-scoped session from all configured replicas.
type SessionDeletionService struct {
	sessionStore *store.SessionStore
	replicas     []sessionReplicaBinding
}

func NewSessionDeletionService(sessionStore *store.SessionStore, replicas ...sessionReplicaBinding) *SessionDeletionService {
	return &SessionDeletionService{sessionStore: sessionStore, replicas: replicas}
}

// NewSessionDeletionServiceWithVectorStore adds an owner- and session-scoped
// Qdrant replica. The collection is required so no default or legacy
// collection can silently be used during a destructive cascade.
func NewSessionDeletionServiceWithVectorStore(sessionStore *store.SessionStore, vectorStore sessionVectorStore, collection string, replicas ...sessionReplicaBinding) (*SessionDeletionService, error) {
	if vectorStore == nil {
		return nil, fmt.Errorf("Qdrant vector store is required for a complete session cascade")
	}
	if collection == "" {
		return nil, fmt.Errorf("VECTOR_DB_COLLECTION is required for a complete session cascade")
	}
	bindings := make([]sessionReplicaBinding, 0, len(replicas)+1)
	bindings = append(bindings, sessionReplicaBinding{
		name:    "qdrant",
		replica: &vectorSessionReplica{store: vectorStore, collection: collection},
	})
	bindings = append(bindings, replicas...)
	return NewSessionDeletionService(sessionStore, bindings...), nil
}

// NewProductionSessionDeletionService builds the configured replica clients. The caller
// must invoke the returned cleanup function after the request finishes.
func NewProductionSessionDeletionService(sessionStore *store.SessionStore) (*SessionDeletionService, func(), error) {
	databaseConfig, err := config.LoadDatabaseConfig()
	if err != nil {
		return nil, func() {}, fmt.Errorf("load replica configuration: %w", err)
	}

	bindings := make([]sessionReplicaBinding, 0, 2)
	cleanup := make([]func(), 0, 2)
	closeAll := func() {
		for i := len(cleanup) - 1; i >= 0; i-- {
			cleanup[i]()
		}
	}

	if databaseConfig.TimescaleDB.Enabled {
		engine, err := timeline.NewTimescaleDBEngine(&timeline.TimescaleDBConfig{
			Host:        databaseConfig.TimescaleDB.Host,
			Port:        databaseConfig.TimescaleDB.Port,
			Database:    databaseConfig.TimescaleDB.Database,
			Username:    databaseConfig.TimescaleDB.Username,
			Password:    databaseConfig.TimescaleDB.Password,
			SSLMode:     databaseConfig.TimescaleDB.SSLMode,
			MaxConns:    databaseConfig.TimescaleDB.MaxConns,
			MaxIdleTime: databaseConfig.TimescaleDB.MaxIdleTime,
		})
		if err != nil {
			closeAll()
			return nil, func() {}, fmt.Errorf("connect TimescaleDB replica: %w", err)
		}
		bindings = append(bindings, sessionReplicaBinding{name: "timescaledb", replica: engine})
		cleanup = append(cleanup, func() { _ = engine.Close() })
	} else {
		bindings = append(bindings, sessionReplicaBinding{name: "timescaledb", skipReason: "not enabled"})
	}

	if databaseConfig.Neo4j.Enabled {
		engine, err := knowledge.NewNeo4jEngine(&knowledge.Neo4jConfig{
			URI:                     databaseConfig.Neo4j.URI,
			Username:                databaseConfig.Neo4j.Username,
			Password:                databaseConfig.Neo4j.Password,
			Database:                databaseConfig.Neo4j.Database,
			MaxConnectionPoolSize:   databaseConfig.Neo4j.MaxConnectionPoolSize,
			ConnectionTimeout:       databaseConfig.Neo4j.ConnectionTimeout,
			MaxTransactionRetryTime: databaseConfig.Neo4j.MaxTransactionRetryTime,
		})
		if err != nil {
			closeAll()
			return nil, func() {}, fmt.Errorf("connect Neo4j replica: %w", err)
		}
		bindings = append(bindings, sessionReplicaBinding{name: "neo4j", replica: engine})
		cleanup = append(cleanup, func() { _ = engine.Close(context.Background()) })
	} else {
		bindings = append(bindings, sessionReplicaBinding{name: "neo4j", skipReason: "not enabled"})
	}

	return NewSessionDeletionService(sessionStore, bindings...), closeAll, nil
}

// NewProductionSessionDeletionServiceWithVectorStore creates the configured
// TimescaleDB/Neo4j replicas and prepends Qdrant for an all-store cascade.
// The caller must pass cfg.VectorDBCollection from the active runtime config.
func NewProductionSessionDeletionServiceWithVectorStore(sessionStore *store.SessionStore, vectorStore sessionVectorStore, collection string) (*SessionDeletionService, func(), error) {
	service, cleanup, err := NewProductionSessionDeletionService(sessionStore)
	if err != nil {
		return nil, cleanup, err
	}
	completeService, err := NewSessionDeletionServiceWithVectorStore(sessionStore, vectorStore, collection, service.replicas...)
	if err != nil {
		cleanup()
		return nil, func() {}, err
	}
	return completeService, cleanup, nil
}

// Delete removes durable replicas before local state. A replica failure leaves the session
// file and cache untouched, allowing a subsequent request to retry the incomplete cascade.
func (s *SessionDeletionService) Delete(ctx context.Context, userID, sessionID string, dryRun bool) (*SessionDeletionResult, error) {
	verifiedAt := time.Now().UTC()
	traceID := utils.GetTraceIDFromContext(ctx)
	scopeHash := fmt.Sprintf("%x", sha256.Sum256([]byte(userID+"\x1f"+sessionID)))
	verificationHash := sha256.Sum256([]byte(scopeHash + "\x1f" + traceID + "\x1f" + verifiedAt.Format(time.RFC3339Nano)))
	result := &SessionDeletionResult{
		SessionID: sessionID, UserID: userID, DryRun: dryRun,
		VerificationID: fmt.Sprintf("del-%x", verificationHash[:8]), TraceID: traceID,
		VerifiedAt: verifiedAt, ScopeSHA256: scopeHash,
	}
	if s.sessionStore == nil {
		return result, fmt.Errorf("session store is not configured")
	}

	owner, err := s.sessionStore.GetSessionOwner(sessionID)
	if err != nil && !errors.Is(err, store.ErrSessionNotFound) {
		return result, err
	}
	if err == nil && owner != userID {
		return result, fmt.Errorf("%w: session=%s", ErrSessionOwnerMismatch, sessionID)
	}

	for _, binding := range s.replicas {
		storeResult := SessionDeletionStoreResult{Store: binding.name}
		if binding.replica == nil {
			storeResult.Status = "skipped"
			storeResult.Error = binding.skipReason
			result.Stores = append(result.Stores, storeResult)
			continue
		}

		before, err := binding.replica.CountSessionReplicaRecords(ctx, userID, sessionID)
		storeResult.Before = before
		if err != nil {
			storeResult.Status = "failed"
			storeResult.Error = err.Error()
			result.Stores = append(result.Stores, storeResult)
			s.audit(result)
			return result, fmt.Errorf("count %s replica: %w", binding.name, err)
		}
		if dryRun {
			storeResult.Status = "dry_run"
			storeResult.After = before
			result.Stores = append(result.Stores, storeResult)
			continue
		}

		deleted, err := binding.replica.DeleteSessionReplicaRecords(ctx, userID, sessionID)
		storeResult.Deleted = deleted
		if err != nil {
			storeResult.Status = "failed"
			storeResult.Error = err.Error()
			result.Stores = append(result.Stores, storeResult)
			s.audit(result)
			return result, fmt.Errorf("delete %s replica: %w", binding.name, err)
		}
		after, err := binding.replica.CountSessionReplicaRecords(ctx, userID, sessionID)
		storeResult.After = after
		if err != nil || after != 0 {
			storeResult.Status = "failed"
			if err != nil {
				storeResult.Error = err.Error()
			} else {
				storeResult.Error = fmt.Sprintf("verification found %d remaining records", after)
			}
			result.Stores = append(result.Stores, storeResult)
			s.audit(result)
			if err != nil {
				return result, fmt.Errorf("verify %s replica: %w", binding.name, err)
			}
			return result, fmt.Errorf("verify %s replica: %s", binding.name, storeResult.Error)
		}
		storeResult.Status = "deleted"
		result.Stores = append(result.Stores, storeResult)
	}

	localResult := SessionDeletionStoreResult{Store: "session_file_cache"}
	before, err := s.sessionStore.SessionArtifactCount(sessionID)
	// A missing local artifact must not abort the cascade. The local session file
	// lives inside the application container while the durable replicas (vector,
	// timeline, graph) live in their own stores, so recreating the container
	// routinely leaves the local lane absent next to replicas that still hold the
	// session's records. Propagating store.ErrSessionNotFound made the handler
	// answer 404, and callers read 404 as "already clean" — so the replicas were
	// never cleaned and a newly seeded corpus silently shared the session with the
	// previous one, which invalidates every retrieval metric measured afterwards.
	// Treating "absent" as the verified-empty state keeps the not-found answer
	// reserved for a session that is genuinely gone everywhere.
	if err != nil && !errors.Is(err, store.ErrSessionNotFound) {
		localResult.Status = "failed"
		localResult.Error = err.Error()
		result.Stores = append(result.Stores, localResult)
		s.audit(result)
		return result, fmt.Errorf("count session file/cache: %w", err)
	}
	if err != nil {
		before = 0
	}
	localResult.Before = before
	if dryRun {
		localResult.Status = "dry_run"
		localResult.After = before
		result.Stores = append(result.Stores, localResult)
		s.audit(result)
		return result, nil
	}
	if before == 0 {
		// The local cache can legitimately be absent while durable stores still
		// contain an orphaned replica. Its verified empty state completes this
		// lane without turning a successful replica cleanup into a false 404.
		localResult.Status = "deleted"
		localResult.After = 0
		result.Stores = append(result.Stores, localResult)
		result.Complete = true
		s.audit(result)
		return result, nil
	}

	deleted, err := s.sessionStore.DeleteSession(sessionID)
	localResult.Deleted = deleted
	if err != nil {
		localResult.Status = "failed"
		localResult.Error = err.Error()
		result.Stores = append(result.Stores, localResult)
		s.audit(result)
		return result, fmt.Errorf("delete session file/cache: %w", err)
	}
	after, err := s.sessionStore.SessionArtifactCountAfterDeletion(sessionID)
	localResult.After = after
	if err != nil || after != 0 {
		localResult.Status = "failed"
		if err != nil {
			localResult.Error = err.Error()
		} else {
			localResult.Error = fmt.Sprintf("verification found %d remaining artifacts", after)
		}
		result.Stores = append(result.Stores, localResult)
		s.audit(result)
		if err != nil {
			return result, fmt.Errorf("verify session file/cache: %w", err)
		}
		return result, fmt.Errorf("verify session file/cache: %s", localResult.Error)
	}
	localResult.Status = "deleted"
	result.Stores = append(result.Stores, localResult)
	result.Complete = true
	s.audit(result)
	return result, nil
}

func (s *SessionDeletionService) audit(result *SessionDeletionResult) {
	result.EvidenceSHA256 = ""
	canonical, err := json.Marshal(result)
	if err != nil {
		log.Printf("[session-delete] evidence marshal failed: %v", err)
		return
	}
	result.EvidenceSHA256 = fmt.Sprintf("%x", sha256.Sum256(canonical))
	payload, err := json.Marshal(result)
	if err != nil {
		log.Printf("[session-delete] audit marshal failed: %v", err)
		return
	}
	log.Printf("[session-delete] audit=%s", payload)
}
