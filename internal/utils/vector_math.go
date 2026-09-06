package utils

import (
	"fmt"
	"math"
)

// DotProduct 计算两个向量的点积
func DotProduct(a, b []float64) (float64, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("向量维度不匹配: %d vs %d", len(a), len(b))
	}

	var sum float64
	for i := 0; i < len(a); i++ {
		sum += a[i] * b[i]
	}

	return sum, nil
}

// L2Norm 计算向量的L2范数（欧几里得范数）
func L2Norm(v []float64) float64 {
	var sum float64
	for _, val := range v {
		sum += val * val
	}
	return math.Sqrt(sum)
}

// Normalize 归一化向量到单位长度
func Normalize(v []float64) []float64 {
	norm := L2Norm(v)
	if norm == 0 {
		return v
	}

	result := make([]float64, len(v))
	for i := 0; i < len(v); i++ {
		result[i] = v[i] / norm
	}

	return result
}

// Add 向量加法
func Add(a, b []float64) ([]float64, error) {
	if len(a) != len(b) {
		return nil, fmt.Errorf("向量维度不匹配: %d vs %d", len(a), len(b))
	}

	result := make([]float64, len(a))
	for i := 0; i < len(a); i++ {
		result[i] = a[i] + b[i]
	}

	return result, nil
}

// Subtract 向量减法
func Subtract(a, b []float64) ([]float64, error) {
	if len(a) != len(b) {
		return nil, fmt.Errorf("向量维度不匹配: %d vs %d", len(a), len(b))
	}

	result := make([]float64, len(a))
	for i := 0; i < len(a); i++ {
		result[i] = a[i] - b[i]
	}

	return result, nil
}

// Scale 向量标量乘法
func Scale(v []float64, scalar float64) []float64 {
	result := make([]float64, len(v))
	for i := 0; i < len(v); i++ {
		result[i] = v[i] * scalar
	}
	return result
}

// ProjectOnto 将向量v投影到向量onto上
// projection = (v·onto / ||onto||²) * onto
func ProjectOnto(v, onto []float64) ([]float64, error) {
	if len(v) != len(onto) {
		return nil, fmt.Errorf("向量维度不匹配: %d vs %d", len(v), len(onto))
	}

	// 计算点积 v·onto
	dotProd, err := DotProduct(v, onto)
	if err != nil {
		return nil, err
	}

	// 计算 ||onto||²
	normSquared := 0.0
	for _, val := range onto {
		normSquared += val * val
	}

	if normSquared == 0 {
		return make([]float64, len(v)), nil
	}

	// projection = (v·onto / ||onto||²) * onto
	scalar := dotProd / normSquared
	return Scale(onto, scalar), nil
}

// Orthogonalize 将target向量正交化到reference向量
// 实现策划书第2.3.2节的梯度正交投影公式: g_u⊥ = g_u - (g_u·g_f / ||g_f||²) * g_f
func Orthogonalize(target, reference []float64) ([]float64, error) {
	if len(target) != len(reference) {
		return nil, fmt.Errorf("向量维度不匹配: %d vs %d", len(target), len(reference))
	}

	// 计算target在reference上的投影
	projection, err := ProjectOnto(target, reference)
	if err != nil {
		return nil, err
	}

	// 正交分量 = target - projection
	orthogonal, err := Subtract(target, projection)
	if err != nil {
		return nil, err
	}

	return orthogonal, nil
}

// CosineSimilarity 计算两个向量的余弦相似度
func CosineSimilarity(a, b []float64) (float64, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("向量维度不匹配: %d vs %d", len(a), len(b))
	}

	dotProd, err := DotProduct(a, b)
	if err != nil {
		return 0, err
	}

	normA := L2Norm(a)
	normB := L2Norm(b)

	if normA == 0 || normB == 0 {
		return 0, nil
	}

	return dotProd / (normA * normB), nil
}

// EuclideanDistance 计算两个向量的欧几里得距离
func EuclideanDistance(a, b []float64) (float64, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("向量维度不匹配: %d vs %d", len(a), len(b))
	}

	var sum float64
	for i := 0; i < len(a); i++ {
		diff := a[i] - b[i]
		sum += diff * diff
	}

	return math.Sqrt(sum), nil
}

// Mean 计算多个向量的平均向量
func Mean(vectors [][]float64) ([]float64, error) {
	if len(vectors) == 0 {
		return nil, fmt.Errorf("向量列表为空")
	}

	dim := len(vectors[0])
	result := make([]float64, dim)

	for _, vec := range vectors {
		if len(vec) != dim {
			return nil, fmt.Errorf("向量维度不一致")
		}
		for i := 0; i < dim; i++ {
			result[i] += vec[i]
		}
	}

	count := float64(len(vectors))
	for i := 0; i < dim; i++ {
		result[i] /= count
	}

	return result, nil
}

// Clone 克隆向量
func Clone(v []float64) []float64 {
	result := make([]float64, len(v))
	copy(result, v)
	return result
}

// IsZeroVector 判断是否为零向量
func IsZeroVector(v []float64, epsilon float64) bool {
	if epsilon == 0 {
		epsilon = 1e-10
	}

	for _, val := range v {
		if math.Abs(val) > epsilon {
			return false
		}
	}
	return true
}

// ManhattanDistance 计算曼哈顿距离（L1距离）
func ManhattanDistance(a, b []float64) (float64, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("向量维度不匹配: %d vs %d", len(a), len(b))
	}

	var sum float64
	for i := 0; i < len(a); i++ {
		sum += math.Abs(a[i] - b[i])
	}

	return sum, nil
}

// ElementwiseMultiply 逐元素乘法
func ElementwiseMultiply(a, b []float64) ([]float64, error) {
	if len(a) != len(b) {
		return nil, fmt.Errorf("向量维度不匹配: %d vs %d", len(a), len(b))
	}

	result := make([]float64, len(a))
	for i := 0; i < len(a); i++ {
		result[i] = a[i] * b[i]
	}

	return result, nil
}

// L1Norm 计算L1范数（曼哈顿范数）
func L1Norm(v []float64) float64 {
	var sum float64
	for _, val := range v {
		sum += math.Abs(val)
	}
	return sum
}

// Max 找出向量中的最大值
func Max(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}

	max := v[0]
	for _, val := range v {
		if val > max {
			max = val
		}
	}
	return max
}

// Min 找出向量中的最小值
func Min(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}

	min := v[0]
	for _, val := range v {
		if val < min {
			min = val
		}
	}
	return min
}
