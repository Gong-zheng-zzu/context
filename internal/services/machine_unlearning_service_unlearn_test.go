package services

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/contextkeeper/service/internal/models"
	"github.com/contextkeeper/service/internal/security"
	"github.com/contextkeeper/service/internal/utils"
)

// unlearningRealTestStore 是完整实现 models.VectorStore 的测试替身，
// 记录批量更新与删除调用，不依赖真实 Qdrant。
type unlearningRealTestStore struct {
	forgetUserID     string
	forgetVectors    []*models.VectorRecord
	retainedVectors  []*models.VectorRecord
	deletedIDs       []string
	batchUpdates     []models.VectorUpdateItem
	countAfterDelete int
	rebuiltCollections []string
}

func (s *unlearningRealTestStore) GenerateEmbedding(string) ([]float32, error) {
	return make([]float32, 4), nil
}
func (s *unlearningRealTestStore) GetEmbeddingDimension() int { return 4 }
func (s *unlearningRealTestStore) StoreMemory(*models.Memory) error {
	return nil
}
func (s *unlearningRealTestStore) StoreMessage(*models.Message) error { return nil }
func (s *unlearningRealTestStore) CountMemories(string) (int, error)  { return 0, nil }
func (s *unlearningRealTestStore) StoreEnhancedMemory(*models.EnhancedMemory) error {
	return nil
}
func (s *unlearningRealTestStore) StoreEnhancedMessage(*models.EnhancedMessage) error {
	return nil
}
func (s *unlearningRealTestStore) SearchByVector(context.Context, []float32, *models.SearchOptions) ([]models.SearchResult, error) {
	results := make([]models.SearchResult, 0, len(s.retainedVectors))
	for _, record := range s.retainedVectors {
		results = append(results, models.SearchResult{
			ID:     record.ID,
			Score:  0.5,
			Fields: map[string]interface{}{"user_id": record.UserID},
		})
	}
	return results, nil
}
func (s *unlearningRealTestStore) SearchByText(context.Context, string, *models.SearchOptions) ([]models.SearchResult, error) {
	return nil, nil
}
func (s *unlearningRealTestStore) SearchByID(context.Context, string, *models.SearchOptions) ([]models.SearchResult, error) {
	return nil, nil
}
func (s *unlearningRealTestStore) SearchByFilter(context.Context, string, *models.SearchOptions) ([]models.SearchResult, error) {
	return nil, nil
}
func (s *unlearningRealTestStore) EnsureCollection(string) error { return nil }
func (s *unlearningRealTestStore) CreateCollection(string, *models.CollectionConfig) error {
	return nil
}
func (s *unlearningRealTestStore) DeleteCollection(string) error { return nil }
func (s *unlearningRealTestStore) CollectionExists(string) (bool, error) {
	return true, nil
}
func (s *unlearningRealTestStore) StoreUserInfo(*models.UserInfo) error { return nil }
func (s *unlearningRealTestStore) GetUserInfo(string) (*models.UserInfo, error) {
	return nil, nil
}
func (s *unlearningRealTestStore) CheckUserExists(string) (bool, error) { return true, nil }
func (s *unlearningRealTestStore) InitUserStorage() error               { return nil }
func (s *unlearningRealTestStore) UpdateVector(context.Context, *models.VectorUpdateRequest) error {
	return nil
}
func (s *unlearningRealTestStore) BatchUpdateVectors(ctx context.Context, req *models.BatchVectorUpdateRequest) error {
	s.batchUpdates = append(s.batchUpdates, req.Updates...)
	return nil
}
func (s *unlearningRealTestStore) GetVectorByID(ctx context.Context, collectionName, vectorID string) (*models.VectorRecord, error) {
	for _, record := range s.retainedVectors {
		if record.ID == vectorID {
			return record, nil
		}
	}
	return nil, fmt.Errorf("vector %s not found", vectorID)
}
func (s *unlearningRealTestStore) GetVectorsByUserID(ctx context.Context, collectionName, userID string) ([]*models.VectorRecord, error) {
	if userID == s.forgetUserID {
		return s.forgetVectors, nil
	}
	return nil, nil
}
func (s *unlearningRealTestStore) DeleteVectors(ctx context.Context, collectionName string, vectorIDs []string) error {
	s.deletedIDs = append(s.deletedIDs, vectorIDs...)
	s.countAfterDelete = 0
	return nil
}
func (s *unlearningRealTestStore) CountVectorsByUserID(ctx context.Context, collectionName, userID string) (int, error) {
	return s.countAfterDelete, nil
}
func (s *unlearningRealTestStore) GetProvider() models.VectorStoreType {
	return models.VectorStoreTypeQdrant
}
func (s *unlearningRealTestStore) DefaultCollection() string { return "nursing_records" }

// RebuildIndex 可选能力：仅 rebuildCapableTestStore 实现，用于验证真实重建路径。
func (s *rebuildCapableTestStore) RebuildIndex(ctx context.Context, collectionName string) error {
	s.rebuiltCollections = append(s.rebuiltCollections, collectionName)
	return nil
}

type rebuildCapableTestStore struct {
	*unlearningRealTestStore
}

func unlearningRealTestConfig() *models.UnlearningConfig {
	return &models.UnlearningConfig{
		Enabled:                     true,
		DefaultEpsilon:              1.0,
		DefaultLearningRate:         0.01,
		DefaultMaxIterations:        5,
		DefaultConvergenceThreshold: 0.001,
		DefaultRetainedSampleRatio:  10,
		MaxPrivacyBudgetPerUser:     5.0,
		AutoRebuildIndex:            false,
		EnableAuditLog:              true,
	}
}

// TestIterativeOrthogonalProjectionMath 验证正交投影的数学正确性：
// 1) g_u⊥ 与公平性梯度 g_f 正交；
// 2) 更新后的向量在 g_u⊥（遗忘方向）上的投影下降；
// 3) 每个留存向量都得到一次批量更新。
func TestIterativeOrthogonalProjectionMath(t *testing.T) {
	store := &unlearningRealTestStore{}
	service := NewMachineUnlearningService(store, security.NewDifferentialPrivacy(1.0, 1e-9), nil)

	gF := []float64{1, 0, 0, 0}
	gU := []float64{0, 0.5, 0.5, 0}
	vectors := []*models.VectorRecord{
		{ID: "r1", Vector: []float32{0, 1, 0, 0}},
		{ID: "r2", Vector: []float32{0, 0, 1, 0}},
	}

	iterations, _, _, err := service.iterativeOrthogonalProjection(
		context.Background(), "nursing_records", vectors, gU, gF,
		0.5, 1, 0.001, 1000, // epsilon=1000 使拉普拉斯噪声可忽略
	)
	if err != nil {
		t.Fatalf("iterativeOrthogonalProjection() error = %v", err)
	}
	if iterations != 1 {
		t.Fatalf("iterations = %d, want 1", iterations)
	}
	if len(store.batchUpdates) != len(vectors) {
		t.Fatalf("batch updates = %d, want %d", len(store.batchUpdates), len(vectors))
	}

	// 投影不变量：g_u⊥ · g_f ≈ 0。
	orthogonal, err := utils.Orthogonalize(gU, gF)
	if err != nil {
		t.Fatalf("Orthogonalize() error = %v", err)
	}
	dot, err := utils.DotProduct(orthogonal, gF)
	if err != nil {
		t.Fatalf("DotProduct() error = %v", err)
	}
	if math.Abs(dot) > 1e-9 {
		t.Fatalf("orthogonalized gradient dot g_f = %v, want ~0", dot)
	}

	// 遗忘方向上的投影必须下降。
	for i, update := range store.batchUpdates {
		vOld := service.float32ToFloat64(vectors[i].Vector)
		vNew := service.float32ToFloat64(update.Vector)
		oldSim, err := utils.CosineSimilarity(vOld, orthogonal)
		if err != nil {
			t.Fatalf("CosineSimilarity(old) error = %v", err)
		}
		newSim, err := utils.CosineSimilarity(vNew, orthogonal)
		if err != nil {
			t.Fatalf("CosineSimilarity(new) error = %v", err)
		}
		if newSim >= oldSim {
			t.Fatalf("vector %s similarity to forget gradient did not decrease: %v -> %v", update.ID, oldSim, newSim)
		}
	}
}

// TestExecuteUnlearningMainFlow 验证删除主流程：计数减少、删除 ID 正确、
// 预算记账、verify 流程通过。
func TestExecuteUnlearningMainFlow(t *testing.T) {
	store := &unlearningRealTestStore{
		forgetUserID: "forget-me",
		forgetVectors: []*models.VectorRecord{
			{ID: "f1", UserID: "forget-me", Vector: []float32{0, 1, 0, 0}},
			{ID: "f2", UserID: "forget-me", Vector: []float32{0, 0, 1, 0}},
		},
		retainedVectors: []*models.VectorRecord{
			{ID: "r1", UserID: "other-user", Vector: []float32{1, 0, 0, 0}},
			{ID: "r2", UserID: "other-user", Vector: []float32{0, 1, 0, 0}},
		},
	}
	service := NewMachineUnlearningService(store, security.NewDifferentialPrivacy(1.0, 1e-9), unlearningRealTestConfig())

	result, err := service.ExecuteUnlearning(context.Background(), &models.UnlearningRequest{
		UserID:  "forget-me",
		Epsilon: 2.0,
	})
	if err != nil {
		t.Fatalf("ExecuteUnlearning() error = %v", err)
	}
	if result.Status != "success" {
		t.Fatalf("status = %s, want success", result.Status)
	}
	if result.RemovedVectorCount != 2 {
		t.Fatalf("removed = %d, want 2", result.RemovedVectorCount)
	}
	if result.UpdatedVectorCount != 2 {
		t.Fatalf("updated = %d, want 2 (retained vectors)", result.UpdatedVectorCount)
	}
	if len(store.deletedIDs) != 2 {
		t.Fatalf("deleted IDs = %v, want exactly the 2 forget vectors", store.deletedIDs)
	}
	for _, want := range []string{"f1", "f2"} {
		found := false
		for _, id := range store.deletedIDs {
			if id == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("deleted IDs %v missing %s", store.deletedIDs, want)
		}
	}
	if result.PrivacyBudgetConsumed != 2.0 {
		t.Fatalf("privacy budget consumed = %v, want 2.0", result.PrivacyBudgetConsumed)
	}
	if result.PrivacyBudgetUsedTotal != 2.0 {
		t.Fatalf("privacy budget total = %v, want 2.0", result.PrivacyBudgetUsedTotal)
	}
	if result.IndexRebuildStatus != "" {
		t.Fatalf("index rebuild status = %q, want empty when rebuild disabled", result.IndexRebuildStatus)
	}

	// verify 流程：删除后目标用户向量计数为 0 → 验证通过。
	verify, err := service.VerifyUnlearning(context.Background(), &models.UnlearningVerifyRequest{
		UserID: "forget-me",
	})
	if err != nil {
		t.Fatalf("VerifyUnlearning() error = %v", err)
	}
	if !verify.IsFullyUnlearned {
		t.Fatalf("IsFullyUnlearned = false, details = %+v", verify.VerificationDetails)
	}
	if verify.RemainingVectorCount != 0 {
		t.Fatalf("remaining vectors = %d, want 0", verify.RemainingVectorCount)
	}
}

// TestPrivacyBudgetEnforcedPerUser 验证隐私预算真实记账与强制校验：
// 累计消耗超过 MaxPrivacyBudgetPerUser 时返回明确错误。
func TestPrivacyBudgetEnforcedPerUser(t *testing.T) {
	store := &unlearningRealTestStore{
		forgetUserID: "user-a",
		forgetVectors: []*models.VectorRecord{
			{ID: "f1", UserID: "user-a", Vector: []float32{0, 1, 0, 0}},
		},
	}
	service := NewMachineUnlearningService(store, nil, unlearningRealTestConfig())

	if _, err := service.ExecuteUnlearning(context.Background(), &models.UnlearningRequest{
		UserID: "user-a", Epsilon: 3.0,
	}); err != nil {
		t.Fatalf("first unlearning should pass budget check, error = %v", err)
	}
	if got := service.totalPrivacyBudget("user-a"); got != 3.0 {
		t.Fatalf("budget after first op = %v, want 3.0", got)
	}

	_, err := service.ExecuteUnlearning(context.Background(), &models.UnlearningRequest{
		UserID: "user-a", Epsilon: 3.0,
	})
	if err == nil {
		t.Fatal("second unlearning exceeding budget should fail")
	}
	if !strings.Contains(err.Error(), "privacy budget exceeded") {
		t.Fatalf("error = %v, want explicit privacy budget exceeded message", err)
	}

	// 其他用户不受 user-a 预算影响。
	other := &unlearningRealTestStore{
		forgetUserID: "user-b",
		forgetVectors: []*models.VectorRecord{
			{ID: "f1", UserID: "user-b", Vector: []float32{0, 1, 0, 0}},
		},
	}
	serviceOther := NewMachineUnlearningService(other, nil, unlearningRealTestConfig())
	if _, err := serviceOther.ExecuteUnlearning(context.Background(), &models.UnlearningRequest{
		UserID: "user-b", Epsilon: 3.0,
	}); err != nil {
		t.Fatalf("other user should not be affected by user-a budget, error = %v", err)
	}
}

// TestIndexRebuildStatusReportsUnsupportedBackend 验证索引重建：
// 环境变量/请求可触发，但后端无重建接口时必须如实标注
// not_supported_by_backend，实现 RebuildIndex 的后端则返回 rebuilt。
func TestIndexRebuildStatusReportsUnsupportedBackend(t *testing.T) {
	newStore := func() *unlearningRealTestStore {
		return &unlearningRealTestStore{
			forgetUserID: "user-a",
			forgetVectors: []*models.VectorRecord{
				{ID: "f1", UserID: "user-a", Vector: []float32{0, 1, 0, 0}},
			},
		}
	}

	// 环境变量开启重建：默认 false 的配置被 env 覆盖。
	t.Setenv("UNLEARNING_AUTO_REBUILD_INDEX", "true")
	store := newStore()
	service := NewMachineUnlearningService(store, nil, unlearningRealTestConfig())
	result, err := service.ExecuteUnlearning(context.Background(), &models.UnlearningRequest{UserID: "user-a"})
	if err != nil {
		t.Fatalf("ExecuteUnlearning() error = %v", err)
	}
	if result.IndexRebuildStatus != "not_supported_by_backend" {
		t.Fatalf("index rebuild status = %q, want not_supported_by_backend", result.IndexRebuildStatus)
	}

	// 请求级开关（env 置空回退到配置 false）同样触发重建尝试。
	t.Setenv("UNLEARNING_AUTO_REBUILD_INDEX", "")
	store = newStore()
	service = NewMachineUnlearningService(store, nil, unlearningRealTestConfig())
	result, err = service.ExecuteUnlearning(context.Background(), &models.UnlearningRequest{
		UserID: "user-a", RebuildIndex: true,
	})
	if err != nil {
		t.Fatalf("ExecuteUnlearning() error = %v", err)
	}
	if result.IndexRebuildStatus != "not_supported_by_backend" {
		t.Fatalf("index rebuild status = %q, want not_supported_by_backend", result.IndexRebuildStatus)
	}

	// 实现了 RebuildIndex 能力的后端执行真实重建。
	capable := &rebuildCapableTestStore{unlearningRealTestStore: newStore()}
	service = NewMachineUnlearningService(capable, nil, unlearningRealTestConfig())
	result, err = service.ExecuteUnlearning(context.Background(), &models.UnlearningRequest{
		UserID: "user-a", RebuildIndex: true,
	})
	if err != nil {
		t.Fatalf("ExecuteUnlearning() error = %v", err)
	}
	if result.IndexRebuildStatus != "rebuilt" {
		t.Fatalf("index rebuild status = %q, want rebuilt", result.IndexRebuildStatus)
	}
	if len(capable.rebuiltCollections) != 1 || capable.rebuiltCollections[0] != "nursing_records" {
		t.Fatalf("rebuilt collections = %v, want [nursing_records]", capable.rebuiltCollections)
	}
}
