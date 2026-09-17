package services

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/contextkeeper/service/internal/models"
	"github.com/contextkeeper/service/internal/security"
	"github.com/contextkeeper/service/internal/utils"
)

// MachineUnlearningService 机器遗忘服务
// 实现策划书第2.3.2节的梯度正交投影算法
type MachineUnlearningService struct {
	vectorStore models.VectorStore
	dpService   *security.DifferentialPrivacy
	config      *models.UnlearningConfig

	// 隐私预算真实记账：按用户累计每次遗忘操作消耗的 epsilon（进程内存态，
	// 重启后清零）。并发安全；MaxPrivacyBudgetPerUser > 0 时强制校验。
	budgetMu          sync.Mutex
	privacyBudgetUsed map[string]float64
}

// NewMachineUnlearningService 创建机器遗忘服务实例
func NewMachineUnlearningService(
	vectorStore models.VectorStore,
	dpService *security.DifferentialPrivacy,
	config *models.UnlearningConfig,
) *MachineUnlearningService {
	if config == nil {
		config = getDefaultUnlearningConfig()
	}
	return &MachineUnlearningService{
		vectorStore:       vectorStore,
		dpService:         dpService,
		config:            config,
		privacyBudgetUsed: make(map[string]float64),
	}
}

// ExecuteUnlearning 执行机器遗忘
// 实现策划书公式: g_u⊥ = g_u - (g_u·g_f / ||g_f||²) * g_f
func (s *MachineUnlearningService) ExecuteUnlearning(ctx context.Context, req *models.UnlearningRequest) (*models.UnlearningResult, error) {
	startTime := time.Now()

	// 参数校验与默认值
	if err := s.validateAndFillDefaults(req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	collections := req.CollectionNames
	if len(collections) == 0 {
		collections = s.getDefaultCollections()
	}
	if len(collections) == 0 {
		return nil, fmt.Errorf("no configured vector collection for unlearning")
	}

	// 隐私预算在执行前记账并强制校验：即使操作中途失败，已注入的噪声
	// 也无法撤回，因此预算消耗必须预先计入。
	if err := s.chargePrivacyBudget(req.UserID, req.Epsilon); err != nil {
		return nil, err
	}

	result := &models.UnlearningResult{
		UserID:              req.UserID,
		Status:              "success",
		CollectionsAffected: collections,
		Timestamp:           startTime,
	}

	totalUpdated := 0
	totalRemoved := 0

	// 遍历每个集合执行遗忘
	for _, collectionName := range collections {
		updated, removed, err := s.unlearnFromCollection(ctx, req, collectionName)
		if err != nil {
			result.Status = "failed"
			result.ErrorMessage = fmt.Sprintf("collection %s failed: %v", collectionName, err)
			result.Duration = time.Since(startTime)
			return result, fmt.Errorf("unlearning failed for collection %s: %w", collectionName, err)
		}
		totalUpdated += updated
		totalRemoved += removed
	}

	result.UpdatedVectorCount = totalUpdated
	result.RemovedVectorCount = totalRemoved
	result.Duration = time.Since(startTime)
	result.PrivacyBudgetConsumed = req.Epsilon
	result.PrivacyBudgetUsedTotal = s.totalPrivacyBudget(req.UserID)
	result.IndexRebuildStatus = s.rebuildIndexes(ctx, collections, req.RebuildIndex)

	return result, nil
}

// chargePrivacyBudget 对用户级隐私预算做真实记账：先校验再累加。
// MaxPrivacyBudgetPerUser <= 0 表示配置未设置预算上限，此时只记账不强制
// （保持与旧配置的兼容），但消耗仍会如实累计。
func (s *MachineUnlearningService) chargePrivacyBudget(userID string, epsilon float64) error {
	s.budgetMu.Lock()
	defer s.budgetMu.Unlock()
	if s.privacyBudgetUsed == nil {
		s.privacyBudgetUsed = make(map[string]float64)
	}
	maxBudget := s.config.MaxPrivacyBudgetPerUser
	if maxBudget > 0 {
		used := s.privacyBudgetUsed[userID]
		if used+epsilon > maxBudget {
			return fmt.Errorf(
				"privacy budget exceeded for user %s: used %.4f + requested %.4f > max %.4f",
				userID, used, epsilon, maxBudget,
			)
		}
	}
	s.privacyBudgetUsed[userID] += epsilon
	return nil
}

// totalPrivacyBudget 返回指定用户累计消耗的隐私预算。
func (s *MachineUnlearningService) totalPrivacyBudget(userID string) float64 {
	s.budgetMu.Lock()
	defer s.budgetMu.Unlock()
	return s.privacyBudgetUsed[userID]
}

// autoRebuildIndexEnabled 计算索引重建开关：
// 环境变量 UNLEARNING_AUTO_REBUILD_INDEX（"true"/"1" 等）优先，
// 未设置时回退到配置项 AutoRebuildIndex。两者都未启用时为 false（默认行为不变）。
func (s *MachineUnlearningService) autoRebuildIndexEnabled() bool {
	if raw, ok := os.LookupEnv("UNLEARNING_AUTO_REBUILD_INDEX"); ok {
		if value, err := strconv.ParseBool(strings.TrimSpace(raw)); err == nil {
			return value
		}
	}
	return s.config.AutoRebuildIndex
}

// rebuildIndexes 在删除完成后尝试触发向量存储的索引重建/优化。
// 当前 Qdrant 后端（pkg/vectorstore/qdrant_store.go）没有暴露索引重建或
// 优化接口，其他后端也未实现可选的 RebuildIndex 能力，因此默认如实返回
// not_supported_by_backend，不谎报重建成功。只有当后端显式实现了
// RebuildIndex(ctx, collection) 接口时才执行真实重建。
func (s *MachineUnlearningService) rebuildIndexes(ctx context.Context, collections []string, rebuildRequested bool) string {
	if !rebuildRequested && !s.autoRebuildIndexEnabled() {
		return ""
	}

	type rebuildable interface {
		RebuildIndex(ctx context.Context, collectionName string) error
	}
	store, ok := s.vectorStore.(rebuildable)
	if !ok {
		return "not_supported_by_backend"
	}

	for _, collectionName := range collections {
		if err := store.RebuildIndex(ctx, collectionName); err != nil {
			return fmt.Sprintf("failed: %v", err)
		}
	}
	return "rebuilt"
}

// unlearnFromCollection 从单个集合执行遗忘
func (s *MachineUnlearningService) unlearnFromCollection(
	ctx context.Context,
	req *models.UnlearningRequest,
	collectionName string,
) (updated, removed int, err error) {
	// 1. 获取待遗忘用户的所有向量
	forgetVectors, err := s.vectorStore.GetVectorsByUserID(ctx, collectionName, req.UserID)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get forget vectors: %w", err)
	}

	if len(forgetVectors) == 0 {
		return 0, 0, nil // 没有向量需要遗忘
	}

	// 2. 采样留存数据向量（10倍样本量）
	retainedVectors, err := s.sampleRetainedVectors(ctx, collectionName, req.UserID, len(forgetVectors)*req.RetainedSampleRatio)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to sample retained vectors: %w", err)
	}

	if len(retainedVectors) == 0 {
		// 如果没有留存数据，直接删除待遗忘向量
		vectorIDs := make([]string, len(forgetVectors))
		for i, v := range forgetVectors {
			vectorIDs[i] = v.ID
		}
		if err := s.vectorStore.DeleteVectors(ctx, collectionName, vectorIDs); err != nil {
			return 0, 0, fmt.Errorf("failed to delete vectors: %w", err)
		}
		return 0, len(forgetVectors), nil
	}

	// 3. 计算遗忘梯度 g_u（待遗忘向量的平均）
	g_u, err := s.computeAverageGradient(forgetVectors)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to compute forget gradient: %w", err)
	}

	// 4. 计算公平性梯度 g_f（留存向量的平均）
	g_f, err := s.computeAverageGradient(retainedVectors)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to compute fairness gradient: %w", err)
	}

	// 5. 迭代更新留存向量
	iterations, fairnessLoss, converged, err := s.iterativeOrthogonalProjection(
		ctx,
		collectionName,
		retainedVectors,
		g_u,
		g_f,
		req.LearningRate,
		req.MaxIterations,
		req.ConvergenceThreshold,
		req.Epsilon,
	)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to apply orthogonal projection: %w", err)
	}

	// 6. 删除待遗忘用户的向量
	forgetIDs := make([]string, len(forgetVectors))
	for i, v := range forgetVectors {
		forgetIDs[i] = v.ID
	}
	if err := s.vectorStore.DeleteVectors(ctx, collectionName, forgetIDs); err != nil {
		return 0, 0, fmt.Errorf("failed to delete forget vectors: %w", err)
	}

	// 索引重建由 ExecuteUnlearning 在全部集合删除完成后统一触发，
	// 并把真实能力状态写入结果（见 rebuildIndexes）。

	updated = len(retainedVectors)
	removed = len(forgetVectors)

	// 记录收敛信息到result（需要传递回去）
	_ = iterations
	_ = fairnessLoss
	_ = converged

	return updated, removed, nil
}

// iterativeOrthogonalProjection 迭代正交投影更新
func (s *MachineUnlearningService) iterativeOrthogonalProjection(
	ctx context.Context,
	collectionName string,
	vectors []*models.VectorRecord,
	g_u, g_f []float64,
	learningRate float64,
	maxIterations int,
	convergenceThreshold float64,
	epsilon float64,
) (iterations int, finalLoss float64, converged bool, err error) {
	// 转换g_u和g_f为float64切片供后续计算
	prevLoss := 1.0

	for iter := 0; iter < maxIterations; iter++ {
		iterations = iter + 1

		// a. 正交投影: g_u⊥ = g_u - (g_u·g_f / ||g_f||²) * g_f
		g_u_orthogonal, err := utils.Orthogonalize(g_u, g_f)
		if err != nil {
			return iterations, finalLoss, false, fmt.Errorf("orthogonalization failed: %w", err)
		}

		// b. 批量更新向量
		updates := make([]models.VectorUpdateItem, 0, len(vectors))
		for _, record := range vectors {
			// 转换向量为float64
			v_old := s.float32ToFloat64(record.Vector)

			// 梯度更新: v_new = v_old - η * g_u⊥
			scaled_gradient := utils.Scale(g_u_orthogonal, learningRate)
			v_new, err := utils.Subtract(v_old, scaled_gradient)
			if err != nil {
				continue // 跳过错误向量
			}

			// c. 添加拉普拉斯噪声（差分隐私）
			v_noisy := s.addLaplaceNoise(v_new, epsilon)

			// d. L2归一化
			v_normalized := utils.Normalize(v_noisy)

			// 转换回float32
			v_final := s.float64ToFloat32(v_normalized)

			updates = append(updates, models.VectorUpdateItem{
				ID:     record.ID,
				Vector: v_final,
			})
		}

		// 批量更新数据库
		batchReq := &models.BatchVectorUpdateRequest{
			CollectionName: collectionName,
			Updates:        updates,
		}
		if err := s.vectorStore.BatchUpdateVectors(ctx, batchReq); err != nil {
			return iterations, finalLoss, false, fmt.Errorf("batch update failed: %w", err)
		}

		// e. 计算公平性损失（平均相似度）
		currentLoss := s.calculateFairnessLoss(updates, g_f)

		// f. 检查收敛
		if iter > 0 && abs(prevLoss-currentLoss) < convergenceThreshold {
			finalLoss = currentLoss
			converged = true
			break
		}

		prevLoss = currentLoss
		finalLoss = currentLoss
	}

	return iterations, finalLoss, converged, nil
}

// computeAverageGradient 计算平均梯度（向量平均）
func (s *MachineUnlearningService) computeAverageGradient(vectors []*models.VectorRecord) ([]float64, error) {
	if len(vectors) == 0 {
		return nil, fmt.Errorf("empty vector list")
	}

	// 转换为float64向量列表
	float64Vectors := make([][]float64, len(vectors))
	for i, record := range vectors {
		float64Vectors[i] = s.float32ToFloat64(record.Vector)
	}

	// 计算平均
	return utils.Mean(float64Vectors)
}

// calculateFairnessLoss 计算公平性损失（与留存梯度的相似度）
func (s *MachineUnlearningService) calculateFairnessLoss(updates []models.VectorUpdateItem, g_f []float64) float64 {
	if len(updates) == 0 {
		return 0.0
	}

	totalSimilarity := 0.0
	for _, update := range updates {
		v := s.float32ToFloat64(update.Vector)
		similarity, err := utils.CosineSimilarity(v, g_f)
		if err == nil {
			totalSimilarity += similarity
		}
	}

	return totalSimilarity / float64(len(updates))
}

// sampleRetainedVectors 采样留存数据向量
func (s *MachineUnlearningService) sampleRetainedVectors(
	ctx context.Context,
	collectionName string,
	excludeUserID string,
	sampleSize int,
) ([]*models.VectorRecord, error) {
	// 使用SearchByFilter获取非目标用户的向量
	// 这里简化实现，实际应根据具体向量数据库API调整
	options := &models.SearchOptions{
		Limit: sampleSize,
		ExtraFilters: map[string]interface{}{
			"user_id_ne": excludeUserID, // 不等于目标用户
		},
	}

	// 使用随机向量进行搜索以获取采样
	randomVector := s.generateRandomVector(s.getVectorDimension())
	var results []models.SearchResult
	var err error
	if store, ok := s.vectorStore.(interface {
		SearchByVectorInCollection(context.Context, string, []float32, *models.SearchOptions) ([]models.SearchResult, error)
	}); ok {
		results, err = store.SearchByVectorInCollection(ctx, collectionName, randomVector, options)
	} else {
		results, err = s.vectorStore.SearchByVector(ctx, randomVector, options)
	}
	if err != nil {
		return nil, err
	}

	// 转换为VectorRecord
	records := make([]*models.VectorRecord, 0, len(results))
	for _, result := range results {
		// 需要通过ID获取完整向量
		record, err := s.vectorStore.GetVectorByID(ctx, collectionName, result.ID)
		if err != nil {
			return nil, fmt.Errorf("get retained vector %s: %w", result.ID, err)
		}
		if record.UserID == excludeUserID {
			continue
		}
		records = append(records, record)
	}

	return records, nil
}

// VerifyUnlearning 验证遗忘效果
func (s *MachineUnlearningService) VerifyUnlearning(ctx context.Context, req *models.UnlearningVerifyRequest) (*models.UnlearningVerifyResult, error) {
	if req == nil || req.UserID == "" {
		return nil, fmt.Errorf("user_id is required")
	}
	result := &models.UnlearningVerifyResult{
		UserID:           req.UserID,
		IsFullyUnlearned: true,
		Timestamp:        time.Now(),
	}

	collections := req.CollectionNames
	if len(collections) == 0 {
		collections = s.getDefaultCollections()
	}
	if len(collections) == 0 {
		return nil, fmt.Errorf("no configured vector collection for verification")
	}

	maxSimilarity := 0.0
	totalSimilarity := 0.0
	totalVectors := 0

	for _, collectionName := range collections {
		detail, err := s.verifyCollection(ctx, collectionName, req)
		if err != nil {
			return nil, fmt.Errorf("verify collection %s: %w", collectionName, err)
		}

		result.VerificationDetails = append(result.VerificationDetails, detail)
		result.RemainingVectorCount += detail.RemainingVectors

		if detail.MaxSimilarity > maxSimilarity {
			maxSimilarity = detail.MaxSimilarity
		}
		totalSimilarity += detail.MaxSimilarity
		totalVectors++

		if !detail.Passed {
			result.IsFullyUnlearned = false
		}
	}

	result.MaxSimilarity = maxSimilarity
	if totalVectors > 0 {
		result.AverageSimilarity = totalSimilarity / float64(totalVectors)
	}
	result.CollectionsChecked = collections

	return result, nil
}

// verifyCollection 验证单个集合的遗忘效果
func (s *MachineUnlearningService) verifyCollection(
	ctx context.Context,
	collectionName string,
	req *models.UnlearningVerifyRequest,
) (*models.CollectionVerifyDetail, error) {
	detail := &models.CollectionVerifyDetail{
		CollectionName: collectionName,
		Passed:         true,
	}

	// 1. 检查剩余向量数量
	count, err := s.vectorStore.CountVectorsByUserID(ctx, collectionName, req.UserID)
	if err != nil {
		return nil, err
	}
	detail.RemainingVectors = count

	// 如果还有剩余向量，验证失败
	if count > 0 {
		detail.Passed = false
		return detail, nil
	}

	// 2. 如果提供了查询文本，检查相似度
	if req.QueryText != "" {
		options := &models.SearchOptions{
			Limit: 10,
		}
		var results []models.SearchResult
		var err error
		if store, ok := s.vectorStore.(interface {
			SearchByTextInCollection(context.Context, string, string, *models.SearchOptions) ([]models.SearchResult, error)
		}); ok {
			results, err = store.SearchByTextInCollection(ctx, collectionName, req.QueryText, options)
		} else {
			results, err = s.vectorStore.SearchByText(ctx, req.QueryText, options)
		}
		if err != nil {
			return nil, err
		}

		// 检查是否有高相似度结果（可能是遗忘不完全）
		threshold := req.SimilarityThreshold
		if threshold == 0 {
			threshold = 0.1
		}

		for _, result := range results {
			// Results for retained users are legitimate. A probe only fails when
			// a result is still attributable to the forgotten user.
			resultUserID, _ := result.Fields["user_id"].(string)
			if resultUserID != req.UserID {
				continue
			}
			if result.Score > detail.MaxSimilarity {
				detail.MaxSimilarity = result.Score
			}
			if result.Score >= threshold {
				detail.Passed = false
			}
		}
	}

	return detail, nil
}

// Helper functions

func (s *MachineUnlearningService) validateAndFillDefaults(req *models.UnlearningRequest) error {
	if req == nil {
		return fmt.Errorf("request is required")
	}
	if req.UserID == "" {
		return fmt.Errorf("user_id is required")
	}

	if req.Epsilon <= 0 {
		req.Epsilon = s.config.DefaultEpsilon
	}
	if req.LearningRate <= 0 {
		req.LearningRate = s.config.DefaultLearningRate
	}
	if req.MaxIterations <= 0 {
		req.MaxIterations = s.config.DefaultMaxIterations
	}
	if req.ConvergenceThreshold <= 0 {
		req.ConvergenceThreshold = s.config.DefaultConvergenceThreshold
	}
	if req.RetainedSampleRatio <= 0 {
		req.RetainedSampleRatio = s.config.DefaultRetainedSampleRatio
	}

	return nil
}

func (s *MachineUnlearningService) getDefaultCollections() []string {
	if store, ok := s.vectorStore.(interface{ DefaultCollection() string }); ok {
		if collectionName := store.DefaultCollection(); collectionName != "" {
			return []string{collectionName}
		}
	}
	return nil
}

func (s *MachineUnlearningService) getVectorDimension() int {
	return s.vectorStore.GetEmbeddingDimension()
}

func (s *MachineUnlearningService) generateRandomVector(dim int) []float32 {
	vec := make([]float32, dim)
	for i := 0; i < dim; i++ {
		vec[i] = rand.Float32()
	}
	return vec
}

func (s *MachineUnlearningService) float32ToFloat64(vec []float32) []float64 {
	result := make([]float64, len(vec))
	for i, v := range vec {
		result[i] = float64(v)
	}
	return result
}

func (s *MachineUnlearningService) float64ToFloat32(vec []float64) []float32 {
	result := make([]float32, len(vec))
	for i, v := range vec {
		result[i] = float32(v)
	}
	return result
}

func (s *MachineUnlearningService) addLaplaceNoise(vec []float64, epsilon float64) []float64 {
	result := make([]float64, len(vec))
	for i, v := range vec {
		// 拉普拉斯噪声: Lap(0, Δf/ε), Δf=1为敏感度
		noise := s.dpService.GenerateLaplaceNoise(1.0 / epsilon)
		result[i] = v + noise
	}
	return result
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func getDefaultUnlearningConfig() *models.UnlearningConfig {
	return &models.UnlearningConfig{
		Enabled:                     true,
		DefaultEpsilon:              1.0,
		DefaultLearningRate:         0.001,
		DefaultMaxIterations:        50,
		DefaultConvergenceThreshold: 0.001,
		DefaultRetainedSampleRatio:  10,
		MaxPrivacyBudgetPerUser:     5.0,
		AutoRebuildIndex:            false,
		EnableAuditLog:              true,
	}
}
