package audio

import (
	"fmt"
	"math"
	"math/cmplx"
	"sort"

	"github.com/madelynnblue/go-dsp/fft"
)

// ============================================================================
// 声纹剥离
// ============================================================================
//
// 目标：削弱语音中的说话人身份线索，同时尽量保留语义可懂度。
//
// 三种手段及其作用层次
// --------------------
//  1. **谱包络展平**（主要手段）：把频谱除以 LPC 声道包络。
//     LPC 包络正是声道形状的估计——它的峰就是共振峰。削弱它等于
//     直接抹掉"这个人声道长什么样"，这是声纹识别最主要的依据。
//  2. **共振峰搬移**：沿频率轴缩放频谱，把共振峰搬到别处。
//  3. **基频搬移**：时域重采样，改变声带振动频率线索。
//
// 已知局限（如实说明）
// --------------------
// 手段 2 通过频率轴重采样（warp）实现，**会连带改变基频**——因为谐波
// 间隔（=F0）同样位于频率轴上。严格的"只搬共振峰、不动基频"需要
// PSOLA/TD-PSOLA 一类的基音同步时域方法，本包**未实现**。
// 因此本包把三种手段独立暴露，由调用方按场景取舍，
// 并建议以"可链接性指标"实测效果，而不是假定某个参数组合一定更优。

// AnonymizeConfig 匿名化参数。
type AnonymizeConfig struct {
	// TargetF0 目标基频（Hz）。> 0 时启用**基频归一化**：
	// 先估计当前语音的 F0，再把它搬移到该目标值。
	//
	// 为什么必须做归一化而不是按比例搬移——这是本实现的关键设计点：
	// 若对所有人施加同一个比例（如 ×1.1），说话人之间的**相对差异
	// 完全保留**（110Hz→121Hz、230Hz→253Hz，依然是 1:2），
	// 可链接性丝毫不会下降。只有把所有人的 F0 拉到同一个目标值，
	// 声带线索才真正被抹除。
	//
	// 代价：听感会明显改变，是"可懂度与自然度"换"隐私"的取舍。
	TargetF0 float64 `json:"target_f0"`
	// PitchRatio 基频搬移比例，1.0 表示不变。
	// 与 TargetF0 互斥：两者都设置时以 TargetF0 为准。
	PitchRatio float64 `json:"pitch_ratio"`
	// FormantRatio 共振峰搬移比例，1.0 表示不变。
	FormantRatio float64 `json:"formant_ratio"`
	// EnvelopeFlattening 谱包络展平强度 ∈ [0,1]。
	// 0 表示不动，1 表示完全白化（频谱变得平坦，可懂度显著下降）。
	EnvelopeFlattening float64 `json:"envelope_flattening"`
}

// DefaultAnonymizeConfig 返回一组偏保守的默认参数。
//
// 取值理由：完全白化（=1.0）虽然能最大程度抹除声道特征，但会让语音
// 听起来像耳语噪声，语义可懂度显著下降。0.7 是在"削弱身份线索"与
// "还能听懂"之间的折中——具体取舍应由场景决定，这里不假装存在最优解。
func DefaultAnonymizeConfig() AnonymizeConfig {
	return AnonymizeConfig{
		TargetF0:           130.0,
		PitchRatio:         1.0,
		FormantRatio:       1.15,
		EnvelopeFlattening: 0.7,
	}
}

// Anonymize 对整段语音执行声纹剥离，返回处理后的单声道 PCM。
func Anonymize(pcm []float64, sampleRate int, config AnonymizeConfig) ([]float64, error) {
	if len(pcm) == 0 {
		return nil, fmt.Errorf("audio: empty PCM")
	}
	if sampleRate <= 0 {
		return nil, fmt.Errorf("audio: invalid sample rate %d", sampleRate)
	}
	if config.PitchRatio <= 0 {
		config.PitchRatio = 1.0
	}
	if config.FormantRatio <= 0 {
		config.FormantRatio = 1.0
	}
	if config.EnvelopeFlattening < 0 {
		config.EnvelopeFlattening = 0
	}
	if config.EnvelopeFlattening > 1 {
		config.EnvelopeFlattening = 1
	}

	processed := pcm
	if config.TargetF0 > 0 {
		// 基频归一化：把 F0 拉到统一目标值，抹除声带线索。
		currentF0 := EstimateF0(processed, sampleRate)
		if currentF0 > 0 {
			ratio := config.TargetF0 / currentF0
			// 限制搬移幅度，避免极端拉伸把语音变得不可懂。
			if ratio < 0.5 {
				ratio = 0.5
			}
			if ratio > 2.0 {
				ratio = 2.0
			}
			processed = PitchShift(processed, ratio)
		}
	} else if config.PitchRatio != 1.0 {
		processed = PitchShift(processed, config.PitchRatio)
	}
	if config.FormantRatio == 1.0 && config.EnvelopeFlattening == 0 {
		return processed, nil
	}
	return processSpectralEnvelope(processed, sampleRate, config)
}

// PitchShift 通过线性插值重采样搬移基频。
//
// ratio > 1 提高音高。副作用：共振峰也会被同比例搬移——
// 这是重采样方法的固有性质，不是 bug。
func PitchShift(pcm []float64, ratio float64) []float64 {
	if len(pcm) == 0 || ratio <= 0 {
		return pcm
	}
	if ratio == 1.0 {
		return append([]float64(nil), pcm...)
	}
	outputLength := int(float64(len(pcm)) / ratio)
	if outputLength <= 0 {
		return nil
	}
	out := make([]float64, outputLength)
	for i := 0; i < outputLength; i++ {
		source := float64(i) * ratio
		lower := int(source)
		if lower >= len(pcm)-1 {
			out[i] = pcm[len(pcm)-1]
			continue
		}
		fraction := source - float64(lower)
		out[i] = pcm[lower]*(1-fraction) + pcm[lower+1]*fraction
	}
	return out
}

// processSpectralEnvelope 在频域做共振峰搬移与包络展平，再重建时域信号。
func processSpectralEnvelope(pcm []float64, sampleRate int, config AnonymizeConfig) ([]float64, error) {
	frameSize := int(DefaultFrameMs * float64(sampleRate) / 1000.0)
	hopSize := int(DefaultHopMs * float64(sampleRate) / 1000.0)
	if frameSize <= 0 {
		return nil, fmt.Errorf("audio: invalid frame size for sample rate %d", sampleRate)
	}
	if hopSize <= 0 {
		hopSize = frameSize / 2
	}
	if len(pcm) < frameSize {
		// 短于一个帧的音频无法做 STFT，原样返回并说明原因。
		return append([]float64(nil), pcm...), nil
	}

	window := HammingWindow(frameSize)
	order := sampleRate/1000 + 2
	if order < 8 {
		order = 8
	}
	if order > frameSize-1 {
		order = frameSize - 1
	}

	// 逐帧处理，输出用 overlap-add 重建。
	outputLength := frameSize + ((len(pcm)-frameSize)/hopSize)*hopSize
	out := make([]float64, outputLength)
	normalization := make([]float64, outputLength)

	for start := 0; start+frameSize <= len(pcm); start += hopSize {
		frame := make([]float64, frameSize)
		for i := 0; i < frameSize; i++ {
			frame[i] = pcm[start+i] * window[i]
		}
		spectrum := fft.FFTReal(frame)

		if config.EnvelopeFlattening > 0 {
			flattenSpectrum(spectrum, frame, order, config.EnvelopeFlattening)
		}
		if config.FormantRatio != 1.0 {
			spectrum = warpSpectrum(spectrum, config.FormantRatio)
		}

		reconstructed := fft.IFFT(spectrum)
		for i := 0; i < frameSize; i++ {
			out[start+i] += real(reconstructed[i])
			normalization[start+i] += window[i]
		}
	}

	// 除以窗函数累加和，抵消 overlap-add 的幅度调制。
	for i := range out {
		if normalization[i] > 1e-8 {
			out[i] /= normalization[i]
		}
	}
	return out, nil
}

// flattenSpectrum 把幅度谱按 LPC 包络展平，保持相位不变。
//
// 数学上：mag' = mag / (envelope_normalized)^degree。
// 包络先按其均值归一化，避免除法把整体幅度放大到离谱的量级。
//
// 保持相位是必要的：相位承载时间结构，随意改动会让重建信号听起来
// 像金属噪声，可懂度严重受损。
func flattenSpectrum(spectrum []complex128, frame []float64, order int, degree float64) {
	if len(spectrum) == 0 || degree <= 0 {
		return
	}
	bins := len(spectrum)/2 + 1
	coefficients := lpcCoefficients(frame, order)
	if len(coefficients) == 0 {
		return
	}
	envelope := lpcSpectrum(coefficients, bins)

	// 归一化包络（除以均值）。
	mean := 0.0
	for _, value := range envelope {
		mean += value
	}
	if len(envelope) > 0 {
		mean /= float64(len(envelope))
	}
	if mean <= 0 {
		return
	}
	for i := range envelope {
		envelope[i] /= mean
		if envelope[i] < 1e-6 {
			envelope[i] = 1e-6
		}
	}

	for i := 0; i < bins; i++ {
		magnitude := cmplx.Abs(spectrum[i])
		phase := cmplx.Phase(spectrum[i])
		flattened := magnitude / math.Pow(envelope[i], degree)
		spectrum[i] = cmplx.Rect(flattened, phase)
		// 保持共轭对称，使逆变换结果为实信号。
		if i > 0 && i < bins-1 && len(spectrum) > i {
			mirror := len(spectrum) - i
			if mirror < len(spectrum) {
				spectrum[mirror] = cmplx.Conj(spectrum[i])
			}
		}
	}
}

// warpSpectrum 沿频率轴重采样频谱，实现共振峰搬移。
//
// ratio > 1 把共振峰向高频搬移。
// 副作用（如实说明）：谐波间隔同样被缩放，因此**基频也会被改变**。
func warpSpectrum(spectrum []complex128, ratio float64) []complex128 {
	if len(spectrum) == 0 || ratio <= 0 || ratio == 1.0 {
		return spectrum
	}
	size := len(spectrum)
	bins := size/2 + 1
	out := make([]complex128, size)

	for i := 0; i < bins; i++ {
		source := float64(i) / ratio
		if source >= float64(bins-1) {
			continue
		}
		lower := int(source)
		fraction := source - float64(lower)
		if lower+1 >= bins {
			continue
		}
		a, b := spectrum[lower], spectrum[lower+1]
		out[i] = complex(
			real(a)*(1-fraction)+real(b)*fraction,
			imag(a)*(1-fraction)+imag(b)*fraction,
		)
	}
	// 重建共轭对称部分。
	for i := 1; i < bins-1; i++ {
		mirror := size - i
		if mirror >= 0 && mirror < size {
			out[mirror] = cmplx.Conj(out[i])
		}
	}
	return out
}

// ============================================================================
// 说话人可链接性度量
// ============================================================================
//
// 策划书提及的"对角线优势指标矩阵的相对变化"在此实现。
//
// 语义：把 N 段语音两两求相似度，得到 N×N 矩阵。若来自同一说话人的
// 片段彼此明显更相似，矩阵会呈现"同类聚块"的结构，对角线（同类）
// 相似度显著高于非对角（跨说话人）——这就是"对角线优势"。
//
// 该指标衡量的是**身份可分性**：值越接近 1 表示同/跨说话人无法区分
// （匿名化效果好），值越大表示越容易被按说话人聚类（匿名化不足）。

// SpeakerSegment 一段带说话人标签的语音。
type SpeakerSegment struct {
	// SpeakerID 说话人标识。
	SpeakerID string
	// PCM 单声道采样。
	PCM []float64
	// SampleRate 采样率。
	SampleRate int
	// Label 可选的可读标签，便于报告展示。
	Label string
}

// LinkabilityResult 可链接性度量结果。
type LinkabilityResult struct {
	// Matrix 为 N×N 的余弦相似度矩阵。
	Matrix [][]float64 `json:"matrix"`
	// DiagonalDominance 为对角线优势度：同类平均相似度 ÷ 跨类平均相似度。
	DiagonalDominance float64 `json:"diagonal_dominance"`
	// SameSpeakerMean 为同类平均相似度。
	SameSpeakerMean float64 `json:"same_speaker_mean"`
	// CrossSpeakerMean 为跨类平均相似度。
	CrossSpeakerMean float64 `json:"cross_speaker_mean"`
	// SampleCount 为参与度量的片段数。
	SampleCount int `json:"sample_count"`
}

// MeasureLinkability 计算一组语音片段的说话人可链接性。
//
// 特征用 Mel 能量向量——它刻画音色分布，是身份可分性的主要载体。
// 相似度用余弦相似度，对不同片段的能量差异不敏感。
func MeasureLinkability(segments []SpeakerSegment) (*LinkabilityResult, error) {
	if len(segments) < 2 {
		return nil, fmt.Errorf("audio: need at least 2 segments, got %d", len(segments))
	}

	features := make([][]float64, len(segments))
	for i, segment := range segments {
		if len(segment.PCM) == 0 {
			return nil, fmt.Errorf("audio: segment %d (%s) has empty PCM", i, segment.SpeakerID)
		}
		if segment.SampleRate <= 0 {
			return nil, fmt.Errorf("audio: segment %d has invalid sample rate", i)
		}
		features[i] = MelEnergies(segment.PCM, segment.SampleRate, 20)
		if len(features[i]) == 0 {
			return nil, fmt.Errorf("audio: failed to extract features for segment %d", i)
		}
	}

	// 不同片段的 Mel 向量长度可能因 FFT 补零不同而存在差异，
	// 统一截断到最短长度后再比较。
	length := len(features[0])
	for _, f := range features {
		if len(f) < length {
			length = len(f)
		}
	}

	size := len(segments)
	matrix := make([][]float64, size)
	for i := range matrix {
		matrix[i] = make([]float64, size)
	}

	sameSum, sameCount := 0.0, 0
	crossSum, crossCount := 0.0, 0
	for i := 0; i < size; i++ {
		for j := i; j < size; j++ {
			similarity := CosineSimilarity(features[i][:length], features[j][:length])
			matrix[i][j] = similarity
			matrix[j][i] = similarity
			if i == j {
				continue
			}
			if segments[i].SpeakerID == segments[j].SpeakerID {
				sameSum += similarity
				sameCount++
			} else {
				crossSum += similarity
				crossCount++
			}
		}
	}

	result := &LinkabilityResult{
		Matrix:      matrix,
		SampleCount: size,
	}
	if sameCount > 0 {
		result.SameSpeakerMean = sameSum / float64(sameCount)
	}
	if crossCount > 0 {
		result.CrossSpeakerMean = crossSum / float64(crossCount)
	}
	// 跨类均值为 0（特征完全正交）时优势度无定义，返回 1 表示"无区分性"。
	if result.CrossSpeakerMean > 1e-12 {
		result.DiagonalDominance = result.SameSpeakerMean / result.CrossSpeakerMean
	} else {
		result.DiagonalDominance = 1.0
	}
	return result, nil
}

// CosineSimilarity 计算两个等长向量的余弦相似度。
//
// 零向量返回 0（方向未定义，不能返回 1，否则会凭空制造"完全相同"的假象）。
func CosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	dot, normA, normB := 0.0, 0.0, 0.0
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA <= 0 || normB <= 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// CompareAnonymization 对比匿名化前后的可链接性，量化身份可分性的下降。
//
// 返回 (匿名化前, 匿名化后)。调用方据此判断剥离是否有效——
// 这是"效果好不好"的判据，而不是凭参数感觉。
func CompareAnonymization(segments []SpeakerSegment, config AnonymizeConfig) (
	before *LinkabilityResult, after *LinkabilityResult, err error) {

	before, err = MeasureLinkability(segments)
	if err != nil {
		return nil, nil, err
	}

	anonymized := make([]SpeakerSegment, len(segments))
	for i, segment := range segments {
		processed, err := Anonymize(segment.PCM, segment.SampleRate, config)
		if err != nil {
			return nil, nil, fmt.Errorf("segment %d: %w", i, err)
		}
		anonymized[i] = segment
		anonymized[i].PCM = processed
	}

	after, err = MeasureLinkability(anonymized)
	if err != nil {
		return nil, nil, err
	}
	return before, after, nil
}

// DominanceReduction 计算对角线优势度的下降比例 ∈ (-∞, 1]。
//
// 1 表示完全消除可分性，0 表示没有变化，负值表示反而更易区分。
func DominanceReduction(before, after *LinkabilityResult) float64 {
	if before == nil || after == nil || before.DiagonalDominance <= 0 {
		return 0
	}
	return 1 - after.DiagonalDominance/before.DiagonalDominance
}

// ============================================================================
// 合成测试语音（供单元测试与演示使用）
// ============================================================================

// SynthesizeVowel 合成一段"元音"样的周期性语音。
//
// 生成方式：以 f0 为基频生成谐波，其幅度由一组共振峰包络整形。
// 这足以驱动 F0 估计、共振峰估计与可链接性度量的单元测试，
// 且**不依赖任何外部音频文件**。
//
// 诚实说明：这是简化的源-滤波器模型，不是真实语音。它能验证特征提取
// 的数值正确性，但不能替代真实语料上的效果评估。
func SynthesizeVowel(sampleRate int, durationMs int, f0 float64, formants []float64) []float64 {
	if sampleRate <= 0 || durationMs <= 0 || f0 <= 0 {
		return nil
	}
	count := sampleRate * durationMs / 1000
	out := make([]float64, count)

	for i := 0; i < count; i++ {
		t := float64(i) / float64(sampleRate)
		sum := 0.0
		// 前 20 个谐波已覆盖到 8kHz 以下的主要能量。
		for harmonic := 1; harmonic <= 20; harmonic++ {
			frequency := f0 * float64(harmonic)
			if frequency >= float64(sampleRate)/2 {
				break
			}
			// 共振峰包络：各共振峰以谐振器响应叠加。
			amplitude := 1.0 / float64(harmonic)
			for _, formant := range formants {
				if formant <= 0 {
					continue
				}
				// 简化的谐振响应：越接近共振峰增益越大。
				bandwidth := 80.0
				delta := (frequency - formant) / bandwidth
				amplitude += 1.0 / (1.0 + delta*delta) / float64(harmonic)
			}
			sum += amplitude * math.Sin(2*math.Pi*frequency*t)
		}
		out[i] = sum
	}
	normalizePeak(out, 0.9)
	return out
}

// normalizePeak 按峰值归一化到指定幅值。
func normalizePeak(pcm []float64, peak float64) {
	if len(pcm) == 0 {
		return
	}
	max := 0.0
	for _, sample := range pcm {
		if absolute := math.Abs(sample); absolute > max {
			max = absolute
		}
	}
	if max <= 0 {
		return
	}
	scale := peak / max
	for i := range pcm {
		pcm[i] *= scale
	}
}

// sortedCopy 返回排序后的副本（供报告展示使用）。
func sortedCopy(values []float64) []float64 {
	out := append([]float64(nil), values...)
	sort.Float64s(out)
	return out
}
