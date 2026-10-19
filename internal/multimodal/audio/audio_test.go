package audio

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

const testSampleRate = 16000

// ============================================================================
// 基频（F0）
// ============================================================================

// TestEstimateF0OnSyntheticTones 对已知基频的合成元音，自相关估计
// 必须准确（误差 < 5%）。这是 F0 估计器数值正确性的核心证据。
func TestEstimateF0OnSyntheticTones(t *testing.T) {
	for _, want := range []float64{100, 150, 200, 250, 300} {
		pcm := SynthesizeVowel(testSampleRate, 500, want, []float64{500, 1500, 2500})
		if len(pcm) == 0 {
			t.Fatalf("synthesis failed for f0=%v", want)
		}
		got := EstimateF0(pcm, testSampleRate)
		error := math.Abs(got-want) / want
		if error > 0.05 {
			t.Errorf("F0 = %v, want %v (relative error %.1f%%)", got, want, error*100)
		}
	}
}

// TestEstimateF0RejectsDegenerateInput 退化输入返回 0，不得 panic 或返回 NaN。
func TestEstimateF0RejectsDegenerateInput(t *testing.T) {
	if got := EstimateF0(nil, testSampleRate); got != 0 {
		t.Errorf("EstimateF0(nil) = %v, want 0", got)
	}
	if got := EstimateF0([]float64{1, 2, 3}, 0); got != 0 {
		t.Errorf("EstimateF0 with sampleRate=0 = %v, want 0", got)
	}
	// 静音（全零）无基频可言。
	silence := make([]float64, 8000)
	if got := EstimateF0(silence, testSampleRate); got != 0 && !math.IsNaN(got) {
		t.Errorf("EstimateF0(silence) = %v, want 0", got)
	}
}

// ============================================================================
// 谱质心
// ============================================================================

// TestSpectralCentroidIncreasesWithFrequency 谱质心随信号频率单调上升——
// 这是"谱质心确实反映音色明暗"的直接验证。
func TestSpectralCentroidIncreasesWithFrequency(t *testing.T) {
	low := SynthesizeVowel(testSampleRate, 400, 120, []float64{300, 900, 1600})
	high := SynthesizeVowel(testSampleRate, 400, 120, []float64{900, 2400, 3600})

	centroidLow := SpectralCentroid(MagnitudeSpectrum(low), testSampleRate)
	centroidHigh := SpectralCentroid(MagnitudeSpectrum(high), testSampleRate)

	if centroidHigh <= centroidLow {
		t.Fatalf("centroid did not rise with frequency: low=%v high=%v", centroidLow, centroidHigh)
	}
}

// TestSpectralCentroidHandlesSilence 静音帧的谱质心定义为 0（无能量则无重心）。
func TestSpectralCentroidHandlesSilence(t *testing.T) {
	silence := make([]float64, 1024)
	if got := SpectralCentroid(MagnitudeSpectrum(silence), testSampleRate); got != 0 {
		t.Fatalf("centroid of silence = %v, want 0", got)
	}
	if got := SpectralCentroid(nil, testSampleRate); got != 0 {
		t.Fatalf("centroid of nil spectrum = %v, want 0", got)
	}
}

// ============================================================================
// 共振峰
// ============================================================================

// TestEstimateFormantsFindsPlausibleValues LPC 谱峰法应在合成元音上
// 给出数量合理、落在语音频带内的共振峰。
//
// 诚实说明：合成信号是简化的源-滤波器模型，不是真实语音，
// 因此这里只断言"落在合理区间"，不断言精确匹配到合成时设定的值。
func TestEstimateFormantsFindsPlausibleValues(t *testing.T) {
	pcm := SynthesizeVowel(testSampleRate, 500, 150, []float64{500, 1500, 2500})
	formants := EstimateFormants(pcm, testSampleRate, 3)

	if len(formants) == 0 {
		t.Fatal("no formants estimated")
	}
	for i, f := range formants {
		// 语音共振峰大致落在 90Hz ~ 4kHz。
		if f < 90 || f > 4000 {
			t.Errorf("formant[%d] = %v Hz, outside plausible speech range", i, f)
		}
	}
	// 必须按频率升序返回。
	for i := 1; i < len(formants); i++ {
		if formants[i] <= formants[i-1] {
			t.Errorf("formants not ascending: %v", formants)
		}
	}
}

// TestEstimateFormantsRejectsDegenerateInput 退化输入返回 nil。
func TestEstimateFormantsRejectsDegenerateInput(t *testing.T) {
	if got := EstimateFormants(nil, testSampleRate, 3); got != nil {
		t.Errorf("EstimateFormants(nil) = %v, want nil", got)
	}
	if got := EstimateFormants(make([]float64, 10), 0, 3); got != nil {
		t.Errorf("EstimateFormants with sampleRate=0 = %v, want nil", got)
	}
}

// ============================================================================
// 分帧与窗函数
// ============================================================================

// TestFrameSignalGeometry 25ms 窗 / 10ms hop 在 16kHz 下应为 400/160 采样点。
func TestFrameSignalGeometry(t *testing.T) {
	duration := 1.0
	pcm := make([]float64, int(duration*testSampleRate))
	frames := FrameSignal(pcm, testSampleRate, DefaultFrameMs, DefaultHopMs)

	if len(frames) == 0 {
		t.Fatal("no frames produced")
	}
	if len(frames[0]) != 400 {
		t.Fatalf("frame size = %d, want 400", len(frames[0]))
	}
	// 帧数 = 1 + (N - frameSize) / hopSize
	expected := 1 + (len(pcm)-400)/160
	if len(frames) != expected {
		t.Fatalf("frame count = %d, want %d", len(frames), expected)
	}
}

// TestFrameSignalRejectsDegenerateInput 退化输入返回 nil。
func TestFrameSignalRejectsDegenerateInput(t *testing.T) {
	if got := FrameSignal(nil, testSampleRate, 25, 10); got != nil {
		t.Error("expected nil for empty PCM")
	}
	if got := FrameSignal(make([]float64, 1000), 0, 25, 10); got != nil {
		t.Error("expected nil for zero sample rate")
	}
}

// TestHammingWindow 汉明窗的端点值与对称性。
func TestHammingWindow(t *testing.T) {
	window := HammingWindow(100)
	if len(window) != 100 {
		t.Fatalf("length = %d, want 100", len(window))
	}
	// 汉明窗：w(0) = 0.08，w(n-1) 同理（对称）。
	if math.Abs(window[0]-0.08) > 1e-9 {
		t.Errorf("window[0] = %v, want 0.08", window[0])
	}
	for i := range window {
		if window[i] < 0 || window[i] > 1 {
			t.Fatalf("window[%d] = %v out of [0,1]", i, window[i])
		}
		// 浮点比较需用容差：cos(2π−x) 与 cos(x) 的浮点结果可能末位不同。
		if math.Abs(window[i]-window[len(window)-1-i]) > 1e-12 {
			t.Fatalf("window not symmetric at %d", i)
		}
	}
	if got := HammingWindow(0); got != nil {
		t.Error("expected nil for n=0")
	}
}

// ============================================================================
// Mel 能量
// ============================================================================

// TestMelEnergies 不同音色的语音应产生不同的 Mel 能量向量，
// 且维度正确——这是可链接性度量能区分说话人的前提。
func TestMelEnergies(t *testing.T) {
	a := SynthesizeVowel(testSampleRate, 400, 120, []float64{400, 1200, 2200})
	b := SynthesizeVowel(testSampleRate, 400, 240, []float64{800, 2000, 3200})

	energiesA := MelEnergies(a, testSampleRate, 20)
	energiesB := MelEnergies(b, testSampleRate, 20)

	if len(energiesA) != 20 || len(energiesB) != 20 {
		t.Fatalf("mel band count = %d / %d, want 20", len(energiesA), len(energiesB))
	}
	// 两者不应完全相同。
	identical := true
	for i := range energiesA {
		if math.Abs(energiesA[i]-energiesB[i]) > 1e-6 {
			identical = false
			break
		}
	}
	if identical {
		t.Fatal("different voices produced identical mel energies")
	}
}

// ============================================================================
// 编解码
// ============================================================================

// TestWAVRoundTrip 写盘再读回，长度、采样率一致，样值在 16-bit 量化误差内。
func TestWAVRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "roundtrip.wav")

	original := SynthesizeVowel(testSampleRate, 300, 180, []float64{500, 1500, 2500})
	if err := SaveWAV(path, original, testSampleRate); err != nil {
		t.Fatalf("SaveWAV: %v", err)
	}

	decoded, rate, err := LoadAudio(path)
	if err != nil {
		t.Fatalf("LoadAudio: %v", err)
	}
	if rate != testSampleRate {
		t.Fatalf("sample rate = %d, want %d", rate, testSampleRate)
	}
	if len(decoded) != len(original) {
		t.Fatalf("length = %d, want %d", len(decoded), len(original))
	}

	// 16-bit 量化误差上限约为 1/32768，取 1e-4 为宽松上界。
	maxError := 0.0
	for i := range original {
		if diff := math.Abs(original[i] - decoded[i]); diff > maxError {
			maxError = diff
		}
	}
	if maxError > 1e-4 {
		t.Fatalf("quantization error too large: %v", maxError)
	}
}

// TestLoadAudioRejectsUnsupportedFormat 不支持的扩展名与缺失文件必须报错。
func TestLoadAudioRejectsUnsupportedFormat(t *testing.T) {
	if _, _, err := LoadAudio(""); err == nil {
		t.Fatal("expected error for empty path")
	}
	if _, _, err := LoadAudio("missing.wav"); err == nil {
		t.Fatal("expected error for missing file")
	}

	dir := t.TempDir()
	unknown := filepath.Join(dir, "clip.flac")
	if err := os.WriteFile(unknown, []byte("not-audio"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, _, err := LoadAudio(unknown); err == nil {
		t.Fatal("expected error for unsupported format")
	}
}

// TestSaveWAVValidation 保存参数的合法性校验。
func TestSaveWAVValidation(t *testing.T) {
	dir := t.TempDir()
	if err := SaveWAV(filepath.Join(dir, "a.wav"), nil, testSampleRate); err == nil {
		t.Fatal("expected error for empty PCM")
	}
	if err := SaveWAV(filepath.Join(dir, "b.wav"), []float64{0.1}, 0); err == nil {
		t.Fatal("expected error for zero sample rate")
	}
}

// ============================================================================
// 声纹剥离
// ============================================================================

// TestPitchShift 重采样搬移基频：比例 > 1 时基频升高、时长缩短。
func TestPitchShift(t *testing.T) {
	original := SynthesizeVowel(testSampleRate, 500, 200, []float64{500, 1500, 2500})

	shifted := PitchShift(original, 1.25)
	if len(shifted) >= len(original) {
		t.Fatalf("pitch shift up should shorten the signal: %d -> %d", len(original), len(shifted))
	}
	f0After := EstimateF0(shifted, testSampleRate)
	// 200Hz × 1.25 = 250Hz，允许 5% 误差。
	if math.Abs(f0After-250)/250 > 0.05 {
		t.Fatalf("F0 after 1.25x shift = %v, want ~250", f0After)
	}
}

// TestAnonymizeProducesValidSignal 匿名化输出必须长度相近、无 NaN、有能量。
//
// 这里刻意使用**不启用基频归一化**的配置：频域处理（共振峰搬移 +
// 包络展平）不应改变信号长度，因此长度稳定是可断言的性质。
// 启用 TargetF0 时会走重采样，长度必然变化，不属于本测试的断言范围。
func TestAnonymizeProducesValidSignal(t *testing.T) {
	original := SynthesizeVowel(testSampleRate, 500, 180, []float64{500, 1500, 2500})
	config := AnonymizeConfig{FormantRatio: 1.15, EnvelopeFlattening: 0.7}

	processed, err := Anonymize(original, testSampleRate, config)
	if err != nil {
		t.Fatalf("Anonymize: %v", err)
	}
	if len(processed) == 0 {
		t.Fatal("empty output")
	}
	for i, sample := range processed {
		if math.IsNaN(sample) || math.IsInf(sample, 0) {
			t.Fatalf("invalid sample at %d: %v", i, sample)
		}
	}
	// 长度不应大幅变化。
	drift := math.Abs(float64(len(processed)-len(original))) / float64(len(original))
	if drift > 0.05 {
		t.Fatalf("length drift %.1f%% exceeds 5%%", drift*100)
	}
	// 输出应有能量（不是被静音了）。
	energy := 0.0
	for _, sample := range processed {
		energy += sample * sample
	}
	if energy <= 0 {
		t.Fatal("output has no energy")
	}
}

// TestAnonymizeIdentityIsNoOp 全默认（无任何变换）时输出应与输入一致。
func TestAnonymizeIdentityIsNoOp(t *testing.T) {
	original := SynthesizeVowel(testSampleRate, 400, 180, []float64{500, 1500, 2500})

	processed, err := Anonymize(original, testSampleRate, AnonymizeConfig{
		PitchRatio: 1.0, FormantRatio: 1.0, EnvelopeFlattening: 0,
	})
	if err != nil {
		t.Fatalf("Anonymize: %v", err)
	}
	if len(processed) != len(original) {
		t.Fatalf("length changed: %d -> %d", len(original), len(processed))
	}
	for i := range original {
		if math.Abs(original[i]-processed[i]) > 1e-9 {
			t.Fatalf("identity config altered sample %d: %v -> %v", i, original[i], processed[i])
		}
	}
}

// TestAnonymizeNormalizesF0ToTarget 基频归一化的核心验证：
// 不同说话人的原始 F0 差异很大，匿名化后应全部趋近同一目标值。
//
// 这是"基频线索被抹除"的直接证据，也是可链接性得以下降的原因——
// 单纯按比例搬移无法做到这一点（相对差异会原样保留）。
func TestAnonymizeNormalizesF0ToTarget(t *testing.T) {
	const target = 130.0
	config := AnonymizeConfig{TargetF0: target}

	for _, originalF0 := range []float64{110, 180, 250} {
		pcm := SynthesizeVowel(testSampleRate, 600, originalF0, []float64{500, 1500, 2500})
		processed, err := Anonymize(pcm, testSampleRate, config)
		if err != nil {
			t.Fatalf("Anonymize (f0=%v): %v", originalF0, err)
		}
		got := EstimateF0(processed, testSampleRate)
		if got == 0 {
			t.Fatalf("F0 estimation failed after anonymization (original %v)", originalF0)
		}
		// 归一化后应接近目标值（允许 10% 误差，覆盖估计与插值误差）。
		if math.Abs(got-target)/target > 0.10 {
			t.Errorf("original F0 %v -> anonymized %v, want ~%v", originalF0, got, target)
		}
	}
}

// TestAnonymizeRejectsInvalidInput 非法输入必须报错。
func TestAnonymizeRejectsInvalidInput(t *testing.T) {
	if _, err := Anonymize(nil, testSampleRate, DefaultAnonymizeConfig()); err == nil {
		t.Fatal("expected error for empty PCM")
	}
	if _, err := Anonymize([]float64{0.1, 0.2}, 0, DefaultAnonymizeConfig()); err == nil {
		t.Fatal("expected error for zero sample rate")
	}
}

// ============================================================================
// 可链接性度量（策划书"对角线优势指标矩阵的相对变化"）
// ============================================================================

// speakerCorpus 构造两位"说话人"、每人两段语音的合成语料。
func speakerCorpus() []SpeakerSegment {
	return []SpeakerSegment{
		// 说话人 A：低沉，共振峰偏低。
		{SpeakerID: "A", SampleRate: testSampleRate,
			PCM: SynthesizeVowel(testSampleRate, 600, 110, []float64{450, 1200, 2300})},
		{SpeakerID: "A", SampleRate: testSampleRate,
			PCM: SynthesizeVowel(testSampleRate, 600, 115, []float64{470, 1250, 2350})},
		// 说话人 B：高亢，共振峰偏高。
		{SpeakerID: "B", SampleRate: testSampleRate,
			PCM: SynthesizeVowel(testSampleRate, 600, 230, []float64{800, 2000, 3100})},
		{SpeakerID: "B", SampleRate: testSampleRate,
			PCM: SynthesizeVowel(testSampleRate, 600, 225, []float64{820, 2050, 3150})},
	}
}

// TestMeasureLinkabilitySeparatesSpeakers 匿名化前，同一说话人的片段
// 应明显比跨说话人更相似——这是"身份可分性"存在的证据。
func TestMeasureLinkabilitySeparatesSpeakers(t *testing.T) {
	result, err := MeasureLinkability(speakerCorpus())
	if err != nil {
		t.Fatalf("MeasureLinkability: %v", err)
	}
	if result.SampleCount != 4 {
		t.Fatalf("sample count = %d, want 4", result.SampleCount)
	}
	if len(result.Matrix) != 4 || len(result.Matrix[0]) != 4 {
		t.Fatal("matrix is not 4x4")
	}
	// 对角线（自身）应为 1。
	for i := range result.Matrix {
		if math.Abs(result.Matrix[i][i]-1.0) > 1e-9 {
			t.Fatalf("matrix[%d][%d] = %v, want 1", i, i, result.Matrix[i][i])
		}
	}
	// 矩阵对称。
	for i := range result.Matrix {
		for j := range result.Matrix {
			if math.Abs(result.Matrix[i][j]-result.Matrix[j][i]) > 1e-12 {
				t.Fatalf("matrix not symmetric at (%d,%d)", i, j)
			}
		}
	}
	if result.SameSpeakerMean <= result.CrossSpeakerMean {
		t.Fatalf("same-speaker similarity (%v) should exceed cross-speaker (%v) before anonymization",
			result.SameSpeakerMean, result.CrossSpeakerMean)
	}
}

// TestAnonymizationReducesDiagonalDominance 核心验证：声纹剥离后，
// 说话人可链接性（对角线优势度）必须下降。
//
// 这是策划书"剔除与发音器官和个人声纹相关的特征"这一主张的量化证据。
func TestAnonymizationReducesDiagonalDominance(t *testing.T) {
	before, after, err := CompareAnonymization(speakerCorpus(), DefaultAnonymizeConfig())
	if err != nil {
		t.Fatalf("CompareAnonymization: %v", err)
	}

	if after.DiagonalDominance >= before.DiagonalDominance {
		t.Fatalf("anonymization did not reduce linkability: before %v -> after %v",
			before.DiagonalDominance, after.DiagonalDominance)
	}
	if reduction := DominanceReduction(before, after); reduction <= 0 {
		t.Fatalf("dominance reduction = %v, want > 0", reduction)
	}
}

// TestDominanceReductionEdgeCases 度量边界：nil 输入与零优势度不得 panic。
func TestDominanceReductionEdgeCases(t *testing.T) {
	if got := DominanceReduction(nil, nil); got != 0 {
		t.Errorf("DominanceReduction(nil,nil) = %v, want 0", got)
	}
	zero := &LinkabilityResult{DiagonalDominance: 0}
	if got := DominanceReduction(zero, &LinkabilityResult{DiagonalDominance: 2}); got != 0 {
		t.Errorf("DominanceReduction with zero baseline = %v, want 0", got)
	}
}

// TestMeasureLinkabilityRejectsInvalidInput 输入校验。
func TestMeasureLinkabilityRejectsInvalidInput(t *testing.T) {
	if _, err := MeasureLinkability(nil); err == nil {
		t.Fatal("expected error for empty segments")
	}
	if _, err := MeasureLinkability([]SpeakerSegment{{SpeakerID: "A", PCM: []float64{0.1}, SampleRate: testSampleRate}}); err == nil {
		t.Fatal("expected error for single segment")
	}
	bad := []SpeakerSegment{
		{SpeakerID: "A", SampleRate: testSampleRate, PCM: SynthesizeVowel(testSampleRate, 100, 150, []float64{500})},
		{SpeakerID: "B", SampleRate: 0, PCM: []float64{0.1, 0.2}},
	}
	if _, err := MeasureLinkability(bad); err == nil {
		t.Fatal("expected error for invalid sample rate")
	}
}

// TestCosineSimilarity 余弦相似度的基本性质。
func TestCosineSimilarity(t *testing.T) {
	// 自身相似度为 1。
	if got := CosineSimilarity([]float64{1, 2, 3}, []float64{1, 2, 3}); math.Abs(got-1) > 1e-12 {
		t.Errorf("self similarity = %v, want 1", got)
	}
	// 正交为 0。
	if got := CosineSimilarity([]float64{1, 0}, []float64{0, 1}); math.Abs(got) > 1e-12 {
		t.Errorf("orthogonal similarity = %v, want 0", got)
	}
	// 零向量为 0（方向未定义）。
	if got := CosineSimilarity([]float64{0, 0}, []float64{1, 1}); got != 0 {
		t.Errorf("zero vector similarity = %v, want 0", got)
	}
	// 长度不一致为 0。
	if got := CosineSimilarity([]float64{1, 2}, []float64{1, 2, 3}); got != 0 {
		t.Errorf("mismatched length = %v, want 0", got)
	}
}

// ============================================================================
// 合成器
// ============================================================================

// TestSynthesizeVowelProducesPeriodicSignal 合成信号应为峰值归一化的
// 有限值序列，且能驱动 F0 估计。
func TestSynthesizeVowelProducesPeriodicSignal(t *testing.T) {
	pcm := SynthesizeVowel(testSampleRate, 200, 200, []float64{500, 1500, 2500})
	if len(pcm) != testSampleRate*200/1000 {
		t.Fatalf("length = %d, want %d", len(pcm), testSampleRate*200/1000)
	}
	peak := 0.0
	for _, sample := range pcm {
		if math.IsNaN(sample) || math.IsInf(sample, 0) {
			t.Fatal("invalid sample in synthesized signal")
		}
		if absolute := math.Abs(sample); absolute > peak {
			peak = absolute
		}
	}
	// 峰值归一化到 0.9。
	if math.Abs(peak-0.9) > 1e-6 {
		t.Fatalf("peak = %v, want 0.9", peak)
	}
	if got := SynthesizeVowel(0, 200, 200, []float64{500}); got != nil {
		t.Error("expected nil for zero sample rate")
	}
}
