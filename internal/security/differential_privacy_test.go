package security

import (
	"math"
	"testing"
)

// TestDifferentialPrivacy 测试差分隐私保护
func TestDifferentialPrivacy(t *testing.T) {
	t.Log("╔══════════════════════════════════════════════════════════╗")
	t.Log("║  测试[1]: 差分隐私保护                                   ║")
	t.Log("╠══════════════════════════════════════════════════════════╣")
	t.Log("║  算法: 拉普拉斯机制(Laplace Mechanism)                  ║")
	t.Log("║  参数: ε(隐私预算) 控制噪声量，ε越小隐私保护越强        ║")
	t.Log("║  应用: 向量嵌入混淆，防止语义相似度攻击                  ║")
	t.Log("╚══════════════════════════════════════════════════════════╝")

	t.Run("测试拉普拉斯噪声生成", func(t *testing.T) {
		dp := NewDifferentialPrivacy(1.0, 1e-5)
		t.Logf("  参数: ε=1.0, δ=1e-5, μ=0, b=1.0")

		samples := make([]float64, 100)
		for i := 0; i < 100; i++ {
			samples[i] = dp.sampleLaplace(0, 1.0)
		}
		t.Logf("  生成100个拉普拉斯分布样本")

		// 计算均值（应该接近0）
		mean := 0.0
		for _, s := range samples {
			mean += s
		}
		mean /= float64(len(samples))

		t.Logf("拉普拉斯分布样本均值: %.4f (期望: 0.0)", mean)

		if math.Abs(mean) > 0.5 {
			t.Errorf("均值偏差过大: %.4f", mean)
		}
	})

	t.Run("测试向量噪声添加", func(t *testing.T) {
		dp := NewDifferentialPrivacy(1.0, 1e-5)

		originalVector := []float32{0.1, 0.2, 0.3, 0.4, 0.5}
		t.Logf("  原始向量: %v (5维)", originalVector)

		noisyVector := dp.AddLaplaceNoise(originalVector)
		t.Logf("  噪声向量: %v", noisyVector)

		// 检查维度
		if len(noisyVector) != len(originalVector) {
			t.Errorf("向量维度不匹配: 期望 %d, 实际 %d", len(originalVector), len(noisyVector))
		}

		// 检查噪声是否被添加（向量应该不同）
		allSame := true
		for i := range originalVector {
			if noisyVector[i] != originalVector[i] {
				allSame = false
				break
			}
		}

		if allSame {
			t.Error("噪声未被添加，向量完全相同")
		}

		t.Logf("原始向量: %v", originalVector)
		t.Logf("噪声向量: %v", noisyVector)
	})

	t.Run("测试向量混淆", func(t *testing.T) {
		dp := NewDifferentialPrivacy(1.0, 1e-5)

		originalVector := make([]float32, 768) // 模拟768维embedding
		for i := range originalVector {
			originalVector[i] = float32(i%10) / 10.0
		}
		t.Logf("  原始向量: 768维 (模拟BERT/GPT embedding)")

		obfuscatedVector := dp.ObfuscateVector(originalVector)
		t.Logf("  混淆向量: 768维 (添加拉普拉斯噪声+归一化)")

		// 检查维度
		if len(obfuscatedVector) != len(originalVector) {
			t.Errorf("向量维度不匹配: 期望 %d, 实际 %d", len(originalVector), len(obfuscatedVector))
		}

		// 计算相似度
		similarity := dp.CalculateSimilarity(originalVector, obfuscatedVector)
		t.Logf("混淆前后相似度: %.4f", similarity)

		// 相似度应该降低，但不应该为0
		if similarity > 0.95 {
			t.Errorf("相似度过高，混淆效果不足: %.4f", similarity)
		}
		if similarity < 0.3 {
			t.Errorf("相似度过低，可能破坏了向量结构: %.4f", similarity)
		}

		// 检查归一化（范数应该为1）
		norm := float32(0.0)
		for _, v := range obfuscatedVector {
			norm += v * v
		}
		norm = float32(math.Sqrt(float64(norm)))

		t.Logf("混淆向量范数: %.4f (期望: 1.0)", norm)

		if math.Abs(float64(norm)-1.0) > 0.01 {
			t.Errorf("向量未正确归一化: %.4f", norm)
		}
	})

	t.Run("测试不同epsilon值的影响", func(t *testing.T) {
		originalVector := make([]float32, 100)
		for i := range originalVector {
			originalVector[i] = float32(i) / 100.0
		}
		t.Log("  测试不同ε值对隐私保护强度的影响:")
		t.Log("  ε越小 → 噪声越大 → 隐私保护越强 → 相似度越低")
		t.Log("  ε越大 → 噪声越小 → 隐私保护越弱 → 相似度越高")

		epsilons := []float64{0.1, 0.5, 1.0, 2.0, 5.0}
		for _, epsilon := range epsilons {
			dp := NewDifferentialPrivacy(epsilon, 1e-5)
			obfuscatedVector := dp.ObfuscateVector(originalVector)
			similarity := dp.CalculateSimilarity(originalVector, obfuscatedVector)

			t.Logf("epsilon=%.1f, 相似度=%.4f", epsilon, similarity)

			// epsilon越大，隐私保护越弱，相似度应该越高
			if epsilon > 2.0 && similarity < 0.5 {
				t.Errorf("epsilon=%.1f时相似度过低: %.4f", epsilon, similarity)
			}
		}
	})
}

// TestPrivacyBudgetManager 测试隐私预算管理器
func TestPrivacyBudgetManager(t *testing.T) {
	t.Run("测试预算消耗", func(t *testing.T) {
		pbm := NewPrivacyBudgetManager(10.0, 24*3600*1e9) // 10.0总预算，24小时重置

		// 消耗预算
		success := pbm.CheckAndConsumeBudget("user1", 1.0)
		if !success {
			t.Error("预算消耗失败")
		}

		remaining := pbm.GetRemainingBudget()
		if remaining != 9.0 {
			t.Errorf("剩余预算错误: 期望 9.0, 实际 %.2f", remaining)
		}

		t.Logf("剩余全局预算: %.2f", remaining)
	})

	t.Run("测试预算耗尽", func(t *testing.T) {
		pbm := NewPrivacyBudgetManager(5.0, 24*3600*1e9)

		// 消耗所有预算
		pbm.CheckAndConsumeBudget("user1", 3.0)
		pbm.CheckAndConsumeBudget("user2", 2.0)

		// 尝试再次消耗（应该失败）
		success := pbm.CheckAndConsumeBudget("user3", 1.0)
		if success {
			t.Error("预算已耗尽，但仍然允许消耗")
		}

		t.Log("预算耗尽测试通过")
	})

	t.Run("测试用户预算限制", func(t *testing.T) {
		pbm := NewPrivacyBudgetManager(10.0, 24*3600*1e9)

		// 单个用户最多使用10%的预算（1.0）
		pbm.CheckAndConsumeBudget("user1", 0.5)
		success := pbm.CheckAndConsumeBudget("user1", 0.6) // 总共1.1，超过限制

		if success {
			t.Error("用户预算超限，但仍然允许消耗")
		}

		userRemaining := pbm.GetUserRemainingBudget("user1")
		t.Logf("用户剩余预算: %.2f", userRemaining)
	})
}

// TestCalculateSimilarity 测试相似度计算
func TestCalculateSimilarity(t *testing.T) {
	dp := NewDifferentialPrivacy(1.0, 1e-5)

	t.Run("测试相同向量", func(t *testing.T) {
		vec := []float32{1.0, 2.0, 3.0, 4.0, 5.0}
		similarity := dp.CalculateSimilarity(vec, vec)

		if math.Abs(similarity-1.0) > 0.001 {
			t.Errorf("相同向量相似度应为1.0，实际: %.4f", similarity)
		}
	})

	t.Run("测试正交向量", func(t *testing.T) {
		vec1 := []float32{1.0, 0.0, 0.0}
		vec2 := []float32{0.0, 1.0, 0.0}
		similarity := dp.CalculateSimilarity(vec1, vec2)

		if math.Abs(similarity) > 0.001 {
			t.Errorf("正交向量相似度应为0，实际: %.4f", similarity)
		}
	})

	t.Run("测试反向向量", func(t *testing.T) {
		vec1 := []float32{1.0, 2.0, 3.0}
		vec2 := []float32{-1.0, -2.0, -3.0}
		similarity := dp.CalculateSimilarity(vec1, vec2)

		if math.Abs(similarity+1.0) > 0.001 {
			t.Errorf("反向向量相似度应为-1.0，实际: %.4f", similarity)
		}
	})
}

// BenchmarkObfuscateVector 性能测试
func BenchmarkObfuscateVector(b *testing.B) {
	dp := NewDifferentialPrivacy(1.0, 1e-5)
	vector := make([]float32, 768) // 768维向量

	for i := range vector {
		vector[i] = float32(i) / 768.0
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dp.ObfuscateVector(vector)
	}
}
