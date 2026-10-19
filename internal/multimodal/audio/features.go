// Package audio 实现语音信号的声学特征提取与声纹剥离。
//
// 策划书原文依据：
//	"在音频侧剔除谱质心、基频、共振峰等与发音器官和个人声纹相关的特征"
//
// 为什么是这三个特征
// ------------------
//   - **基频 F0**：由声带长度与张力决定，是说话人身份与性别的直接线索。
//   - **共振峰 Formant**：由声道形状（口腔、鼻腔、咽喉几何）决定。
//     它承载了大部分"音色"，是声纹识别最主要的依据。
//   - **谱质心 Spectral Centroid**：描述频谱能量重心，反映整体音色明暗。
//
// 三者分别对应「声源—声道—整体音色」三个层次，缺一都会留下可识别的身份线索。
//
// 边界声明
// --------
//   - 本包是**信号处理层**的实现，不是说话人识别/声纹识别系统。
//     它做的是削弱上述特征的可分性，不提供"这是谁"的判定。
//   - 由于本机 cgo 不可用，无法引入需要编译的音频库；解码使用
//     纯 Go 的 `go-audio/wav`（WAV）与 `hajimehoshi/go-mp3`（MP3）。
//     MP3 支持仅覆盖 go-mp3 能解码的 MPEG-1/2 Layer III 格式。
package audio

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-audio/audio"
	"github.com/go-audio/wav"
	"github.com/hajimehoshi/go-mp3"
	"github.com/madelynnblue/go-dsp/fft"
)

// ============================================================================
// 解码 / 编码
// ============================================================================

// LoadAudio 加载音频文件，返回**单声道** float64 PCM（归一化到 [-1,1]）与采样率。
//
// 支持 WAV 与 MP3，按扩展名分派。多声道会被平均为单声道——
// 声纹相关特征都在单声道上定义，混音会破坏特征的可比性。
func LoadAudio(path string) ([]float64, int, error) {
	if path == "" {
		return nil, 0, fmt.Errorf("audio: empty path")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, fmt.Errorf("audio: open %q: %w", path, err)
	}
	defer file.Close()

	switch strings.ToLower(filepath.Ext(path)) {
	case ".wav", ".wave":
		return decodeWAV(file)
	case ".mp3":
		return decodeMP3(file)
	default:
		return nil, 0, fmt.Errorf("audio: unsupported format %q (only .wav and .mp3)", filepath.Ext(path))
	}
}

// decodeWAV 解码 WAV 为单声道 PCM。
func decodeWAV(reader io.ReadSeeker) ([]float64, int, error) {
	decoder := wav.NewDecoder(reader)
	if !decoder.IsValidFile() {
		return nil, 0, fmt.Errorf("audio: invalid WAV file")
	}
	buffer, err := decoder.FullPCMBuffer()
	if err != nil {
		return nil, 0, fmt.Errorf("audio: decode WAV: %w", err)
	}
	sampleRate := int(decoder.SampleRate)

	// IntBuffer.Format 是字段（*audio.Format），不是方法。
	channels := 1
	if buffer.Format != nil && buffer.Format.NumChannels > 0 {
		channels = buffer.Format.NumChannels
	}
	frames := len(buffer.Data) / channels
	pcm := make([]float64, frames)

	// IntBuffer 的样本是定点整数，需按其位深归一化。
	bitDepth := int(decoder.BitDepth)
	scale := 1.0 / float64(int(1)<<uint(bitDepth-1))
	for i := 0; i < frames; i++ {
		sum := 0.0
		for c := 0; c < channels; c++ {
			sum += float64(buffer.Data[i*channels+c]) * scale
		}
		pcm[i] = sum / float64(channels)
	}
	return pcm, sampleRate, nil
}

// decodeMP3 解码 MP3 为单声道 PCM。
//
// go-mp3 输出 16-bit 小端、双声道交错的原始字节流。
func decodeMP3(reader io.ReadSeeker) ([]float64, int, error) {
	decoder, err := mp3.NewDecoder(reader)
	if err != nil {
		return nil, 0, fmt.Errorf("audio: decode MP3: %w", err)
	}
	sampleRate := decoder.SampleRate()
	if sampleRate <= 0 {
		return nil, 0, fmt.Errorf("audio: invalid MP3 sample rate %d", sampleRate)
	}

	raw, err := io.ReadAll(decoder)
	if err != nil {
		return nil, 0, fmt.Errorf("audio: read MP3: %w", err)
	}
	const channels = 2 // go-mp3 固定输出立体声
	sampleCount := len(raw) / 2
	frames := sampleCount / channels
	pcm := make([]float64, frames)

	const scale = 1.0 / 32768.0
	for i := 0; i < frames; i++ {
		left := int16(binary.LittleEndian.Uint16(raw[i*4:]))
		right := int16(binary.LittleEndian.Uint16(raw[i*4+2:]))
		pcm[i] = (float64(left) + float64(right)) * 0.5 * scale
	}
	return pcm, sampleRate, nil
}

// SaveWAV 把单声道 float64 PCM 写成 16-bit 单声道 WAV。
//
// 用于把匿名化后的音频落盘，便于人工试听与二次处理。
func SaveWAV(path string, pcm []float64, sampleRate int) error {
	if len(pcm) == 0 {
		return fmt.Errorf("audio: empty PCM")
	}
	if sampleRate <= 0 {
		return fmt.Errorf("audio: invalid sample rate %d", sampleRate)
	}
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("audio: create %q: %w", path, err)
	}
	defer file.Close()

	encoder := wav.NewEncoder(file, sampleRate, 16, 1, 1)
	buffer := &audio.IntBuffer{
		Format: &audio.Format{SampleRate: sampleRate, NumChannels: 1},
		Data:   make([]int, len(pcm)),
	}
	for i, sample := range pcm {
		clamped := sample
		if clamped > 1 {
			clamped = 1
		}
		if clamped < -1 {
			clamped = -1
		}
		buffer.Data[i] = int(clamped * 32767)
	}
	if err := encoder.Write(buffer); err != nil {
		return fmt.Errorf("audio: encode WAV: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return fmt.Errorf("audio: close WAV encoder: %w", err)
	}
	return nil
}

// ============================================================================
// 分帧与频谱
// ============================================================================

// DefaultFrameMs 默认帧长（毫秒）。
//
// 25ms 是语音处理的标准取值：足够长以容纳至少一个基频周期
// （最低基频约 70Hz，周期约 14ms），又足够短使声道近似平稳。
const (
	DefaultFrameMs = 25.0
	DefaultHopMs   = 10.0
)

// FrameSignal 把 PCM 切成重叠帧并加汉明窗。
//
// 帧长与帧移按毫秒给出，内部换算为采样点，使调用方不必关心采样率。
func FrameSignal(pcm []float64, sampleRate int, frameMs, hopMs float64) [][]float64 {
	if len(pcm) == 0 || sampleRate <= 0 {
		return nil
	}
	frameSize := int(frameMs * float64(sampleRate) / 1000.0)
	hopSize := int(hopMs * float64(sampleRate) / 1000.0)
	if frameSize <= 0 {
		return nil
	}
	if hopSize <= 0 {
		hopSize = frameSize / 2
	}

	window := HammingWindow(frameSize)
	var frames [][]float64
	for start := 0; start+frameSize <= len(pcm); start += hopSize {
		frame := make([]float64, frameSize)
		copy(frame, pcm[start:start+frameSize])
		for i := range frame {
			frame[i] *= window[i]
		}
		frames = append(frames, frame)
	}
	return frames
}

// HammingWindow 生成长度为 n 的汉明窗。
//
// 加窗是为了抑制分帧造成的频谱泄漏；不加窗时帧边界的突变会在频谱上
// 产生大量虚假高频分量，直接污染谱质心与共振峰估计。
func HammingWindow(n int) []float64 {
	if n <= 0 {
		return nil
	}
	window := make([]float64, n)
	if n == 1 {
		window[0] = 1
		return window
	}
	for i := range window {
		window[i] = 0.54 - 0.46*math.Cos(2*math.Pi*float64(i)/float64(n-1))
	}
	return window
}

// MagnitudeSpectrum 计算帧的幅度谱（只返回正频率部分，长度 n/2+1）。
//
// 输入长度会被补齐到不小于原长的 2 的幂，以适配 radix-2 FFT。
func MagnitudeSpectrum(frame []float64) []float64 {
	if len(frame) == 0 {
		return nil
	}
	size := nextPowerOfTwo(len(frame))
	padded := make([]float64, size)
	copy(padded, frame)

	spectrum := fft.FFTReal(padded)
	magnitude := make([]float64, size/2+1)
	for i := range magnitude {
		magnitude[i] = math.Hypot(real(spectrum[i]), imag(spectrum[i]))
	}
	return magnitude
}

// SpectralCentroid 计算幅度谱的谱质心（Hz）。
//
//	centroid = Σ(f_i · m_i) / Σ(m_i)
//
// 语义：频谱能量的"重心"所在频率，反映音色明暗。说话人身份线索之一。
// 全零谱（静音帧）返回 0。
func SpectralCentroid(magnitude []float64, sampleRate int) float64 {
	if len(magnitude) == 0 || sampleRate <= 0 {
		return 0
	}
	binWidth := float64(sampleRate) / 2.0 / float64(len(magnitude)-1)

	weightedSum, total := 0.0, 0.0
	for i, m := range magnitude {
		weightedSum += float64(i) * binWidth * m
		total += m
	}
	if total <= 0 {
		return 0
	}
	return weightedSum / total
}

// ============================================================================
// 基频（F0）
// ============================================================================

// F0 搜索范围（Hz）：覆盖成人男低音到女高音/儿童的常见基频。
const (
	MinF0 = 70.0
	MaxF0 = 400.0
)

// EstimateF0 用归一化自相关估计一段语音的基频。
//
// 归一化（除以两段能量之积的平方根）而非使用原始自相关，是因为原始
// 自相关随滞后增大而系统性下降（重叠样本变少），会把估计偏向最小滞后，
// 即把所有人判成高音。
func EstimateF0(pcm []float64, sampleRate int) float64 {
	if len(pcm) < 2 || sampleRate <= 0 {
		return 0
	}
	minLag := int(float64(sampleRate) / MaxF0)
	maxLag := int(float64(sampleRate) / MinF0)
	if minLag < 1 {
		minLag = 1
	}
	if maxLag >= len(pcm) {
		maxLag = len(pcm) - 1
	}
	if minLag > maxLag {
		return 0
	}

	// 先算出全部候选滞后上的归一化自相关。
	normalized := make([]float64, maxLag+1)
	globalMax, bestLag := 0.0, 0
	for lag := minLag; lag <= maxLag; lag++ {
		correlation, energyA, energyB := 0.0, 0.0, 0.0
		count := len(pcm) - lag
		for i := 0; i < count; i++ {
			a, b := pcm[i], pcm[i+lag]
			correlation += a * b
			energyA += a * a
			energyB += b * b
		}
		if energyA <= 0 || energyB <= 0 {
			continue
		}
		normalized[lag] = correlation / math.Sqrt(energyA*energyB)
		if normalized[lag] > globalMax {
			globalMax = normalized[lag]
			bestLag = lag
		}
	}
	if bestLag == 0 || globalMax <= 0 {
		return 0
	}

	// 倍频（octave）修正。
	//
	// 周期性信号在 lag、2·lag、3·lag… 处都有强自相关峰。若直接取全局
	// 最大值，常会得到"低八度"的错误结果（实测：250Hz 被判成 125Hz、
	// 300Hz 被判成 100Hz）。标准做法是从**最短滞后**开始，取第一个
	// 接近全局峰值的局部极大——即语音真正的基音周期。
	const peakTolerance = 0.85
	for lag := minLag; lag <= maxLag; lag++ {
		if normalized[lag] < globalMax*peakTolerance {
			continue
		}
		// 要求是局部极大（或 plateau 的起点）。
		if lag > minLag && normalized[lag] < normalized[lag-1] {
			continue
		}
		if lag < maxLag && normalized[lag] < normalized[lag+1] {
			continue
		}
		return float64(sampleRate) / float64(lag)
	}
	// 兜底：没有满足条件的局部极大时，退回全局最大值对应的滞后。
	return float64(sampleRate) / float64(bestLag)
}

// ============================================================================
// 共振峰（Formant）
// ============================================================================

// EstimateFormants 用 LPC 谱峰法估计共振峰频率（Hz）。
//
// 流程：自相关 → Levinson-Durbin 解 LPC 系数 → 求 LPC 谱 1/|A(e^{jw})|
// → 取局部极大值作为共振峰。
//
// 之所以用 LPC 谱峰而不是直接找 FFT 谱峰：语音的 FFT 谱上，谐波（由基频
// 决定）与共振峰（由声道决定）混在一起，直接取峰得到的是谐波而非共振峰。
// LPC 谱建模的正是声道传递函数的包络，其峰值才是共振峰。
//
// count 为期望返回的共振峰个数（通常 3~4，对应 F1/F2/F3）。
// 返回值按频率升序，个数可能少于 count（谱上峰不够多时）。
func EstimateFormants(frame []float64, sampleRate int, count int) []float64 {
	if len(frame) < 8 || sampleRate <= 0 {
		return nil
	}
	if count <= 0 {
		count = 3
	}

	// 经验阶数：每 kHz 约 1 个极点，另加 2 个补偿整体谱倾斜。
	order := sampleRate/1000 + 2
	if order < 8 {
		order = 8
	}
	if order > len(frame)-1 {
		order = len(frame) - 1
	}
	if order < 2 {
		return nil
	}

	coefficients := lpcCoefficients(frame, order)
	spectrum := lpcSpectrum(coefficients, 512)

	// 共振峰频率上限取 Nyquist 的 90%，避免取到边缘伪峰。
	maxBin := int(float64(len(spectrum)) * 0.9)
	var formants []float64
	for i := 1; i < maxBin-1; i++ {
		if spectrum[i] > spectrum[i-1] && spectrum[i] >= spectrum[i+1] {
			frequency := float64(i) / float64(len(spectrum)-1) * float64(sampleRate) / 2.0
			// 低于 90Hz 的峰通常是基频/声源泄漏，不是声道共振。
			if frequency < 90 {
				continue
			}
			formants = append(formants, frequency)
			if len(formants) >= count {
				break
			}
		}
	}
	return formants
}

// lpcCoefficients 用 Levinson-Durbin 递推求解线性预测系数。
//
// 返回 a[0..order]，其中 a[0] = 1，预测误差为 1 - Σ a[k]·z^{-k}。
func lpcCoefficients(frame []float64, order int) []float64 {
	// 自相关 r[0..order]。
	r := make([]float64, order+1)
	for k := 0; k <= order; k++ {
		sum := 0.0
		for i := k; i < len(frame); i++ {
			sum += frame[i] * frame[i-k]
		}
		r[k] = sum
	}
	if r[0] <= 0 {
		return nil
	}

	a := make([]float64, order+1)
	a[0] = 1
	error := r[0]

	for i := 1; i <= order; i++ {
		accumulator := r[i]
		for j := 1; j < i; j++ {
			accumulator += a[j] * r[i-j]
		}
		reflection := -accumulator / error
		a[i] = reflection

		// 对称更新，这是 Levinson 递推的标准形式。
		for j := 1; j <= i/2; j++ {
			previous := a[j]
			a[j] = previous + reflection*a[i-j]
			if j != i-j {
				a[i-j] = a[i-j] + reflection*previous
			}
		}
		error *= 1 - reflection*reflection
		if error <= 1e-12 {
			// 数值退化：停止递推，保留已得系数。
			break
		}
	}
	return a
}

// lpcSpectrum 计算 LPC 谱 1/|A(e^{jw})|，在 [0, π] 上采样 bins 点。
func lpcSpectrum(a []float64, bins int) []float64 {
	if len(a) == 0 || bins <= 0 {
		return nil
	}
	spectrum := make([]float64, bins)
	for b := 0; b < bins; b++ {
		w := math.Pi * float64(b) / float64(bins-1)
		re, im := 0.0, 0.0
		for k := 1; k < len(a); k++ {
			re += a[k] * math.Cos(w*float64(k))
			im -= a[k] * math.Sin(w*float64(k))
		}
		magnitude := math.Sqrt((1-re)*(1-re) + im*im)
		if magnitude < 1e-10 {
			magnitude = 1e-10
		}
		spectrum[b] = 1 / magnitude
	}
	return spectrum
}

// ============================================================================
// Mel 能量（用于说话人可链接性度量）
// ============================================================================

// MelEnergies 计算 Mel 滤波器组对数能量。
//
// 这是 MFCC 的前半部分（未做 DCT）。之所以不继续做 DCT：
// 本包用它来计算两段语音的相似度，Mel 能量向量已足够表达音色分布，
// 而 DCT 去相关主要服务于后续的高斯建模，对余弦相似度排序影响很小。
// 少一步变换，也就少一处可调参数与实现偏差的来源。
func MelEnergies(frame []float64, sampleRate, filterCount int) []float64 {
	if len(frame) == 0 || sampleRate <= 0 || filterCount <= 0 {
		return nil
	}
	magnitude := MagnitudeSpectrum(frame)
	if len(magnitude) < 2 {
		return nil
	}

	nyquist := float64(sampleRate) / 2.0
	bins := len(magnitude)

	// Mel 刻度上的等距边界点。
	edges := make([]float64, filterCount+2)
	melMax := hertzToMel(nyquist)
	for i := range edges {
		mel := melMax * float64(i) / float64(filterCount+1)
		edges[i] = melToHertz(mel)
	}

	energies := make([]float64, filterCount)
	for f := 0; f < filterCount; f++ {
		left := frequencyToBin(edges[f], nyquist, bins)
		center := frequencyToBin(edges[f+1], nyquist, bins)
		right := frequencyToBin(edges[f+2], nyquist, bins)

		if left >= bins {
			continue
		}
		if right > bins {
			right = bins
		}
		energy := 0.0
		for i := left; i < center && i < bins; i++ {
			if center == left {
				continue
			}
			energy += magnitude[i] * float64(i-left) / float64(center-left)
		}
		for i := center; i < right && i < bins; i++ {
			if right == center {
				continue
			}
			energy += magnitude[i] * float64(right-i) / float64(right-center)
		}
		// 对数压缩，并设地板避免 log(0)。
		energies[f] = math.Log(energy + 1e-10)
	}
	return energies
}

// VoiceFeatureVector 汇总一段语音的声纹相关特征。
type VoiceFeatureVector struct {
	F0          float64   `json:"f0"`
	Centroid    float64   `json:"centroid"`
	Formants    []float64 `json:"formants"`
	MelEnergies []float64 `json:"mel_energies"`
}

// ExtractFeatures 提取一段语音的声纹相关特征。
func ExtractFeatures(pcm []float64, sampleRate int, melBands int) VoiceFeatureVector {
	if melBands <= 0 {
		melBands = 20
	}
	features := VoiceFeatureVector{
		F0:       EstimateF0(pcm, sampleRate),
		Formants: EstimateFormants(pcm, sampleRate, 3),
	}
	if magnitude := MagnitudeSpectrum(pcm); len(magnitude) > 0 {
		features.Centroid = SpectralCentroid(magnitude, sampleRate)
	}
	features.MelEnergies = MelEnergies(pcm, sampleRate, melBands)
	return features
}

// ============================================================================
// 辅助
// ============================================================================

// hertzToMel / melToHertz 频率与 Mel 刻度的互换。
func hertzToMel(hertz float64) float64 {
	if hertz < 0 {
		hertz = 0
	}
	return 2595.0 * math.Log10(1.0+hertz/700.0)
}

func melToHertz(mel float64) float64 {
	return 700.0 * (math.Pow(10.0, mel/2595.0) - 1.0)
}

// frequencyToBin 把频率映射到幅度谱的 bin 下标。
func frequencyToBin(frequency, nyquist float64, bins int) int {
	if nyquist <= 0 || bins <= 0 {
		return 0
	}
	bin := int(frequency / nyquist * float64(bins-1))
	if bin < 0 {
		return 0
	}
	if bin >= bins {
		return bins
	}
	return bin
}

// nextPowerOfTwo 返回不小于 n 的最小 2 的幂。
func nextPowerOfTwo(n int) int {
	if n <= 1 {
		return 1
	}
	result := 1
	for result < n {
		result <<= 1
	}
	return result
}
