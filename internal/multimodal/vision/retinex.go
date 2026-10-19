package vision

import (
	"fmt"
	"image"
	"math"
	"time"
)

// ============================================================================
// Retinex / 多尺度 Retinex（MSR）低照度增强
// ============================================================================
//
// 策划书原文依据：
//	"利用Retinex空间增强和多尺度Retinex（MSR）算法提升低照度养老环境下
//	 的敏感区域可见性"
//
// 算法
// ----
// Retinex 理论把图像 I 分解为「照度 L」×「反射率 R」，目标是剥离照度、
// 保留反射率（物体的固有属性）。单尺度 Retinex（SSR）用高斯核估计照度：
//
//	R(x,y) = log( I(x,y) + ε ) − log( G_σ * I(x,y) + ε )
//
// 多尺度 Retinex（MSR）在多个尺度上求平均，兼顾细节与整体动态范围：
//
//	MSR(x,y) = (1/N) · Σ_i R_i(x,y)          σ_i 通常取 15 / 80 / 250
//
// MSRCR（带色彩恢复）额外保持色比，避免直接对各通道做 Retinex 产生的色偏：
//
//	MSRCR_C(x,y) = L_out(x,y) · ( C(x,y) / max(L(x,y), ε) )
//
// 边界声明
// --------
//   - ε = 1.0 用于避免 log(0)；它同时决定了极暗区域的增强上限，
//     取值偏大时会压暗部细节，取值偏小时暗部噪声会被放大。
//   - 输出统一做 min-max 线性拉伸到 [0,255]。这一步是**必要的**，
//     但意味着输出不再保持绝对亮度——若下游要做光度测量，不能直接用。
//   - σ=250 这类大尺度在超大图上仍较慢；实现已复用包内的降采样加速
//     高斯模糊（blurSeparable），但不是 GPU 实现。

// DefaultMSRScales 默认多尺度：15 / 80 / 250，与经典 MSR 取值一致。
var DefaultMSRScales = []float64{15, 80, 250}

// retinexEpsilon log 运算的保护项，避免 log(0)。
const retinexEpsilon = 1.0

// EnhanceResult 增强结果，携带处理前后的对照指标。
type EnhanceResult struct {
	// Image 增强后的图像。
	Image *image.RGBA `json:"-"`
	// Before / After 为增强前后的对比度与信息熵，用于量化"是否真的变清晰"。
	Before  Metrics       `json:"before"`
	After   Metrics       `json:"after"`
	Elapsed time.Duration `json:"elapsed"`
}

// ContrastGain 对比度提升倍数。
//
// 返回 0 表示处理前对比度本就为 0（纯色图），此时倍数无定义。
func (r *EnhanceResult) ContrastGain() float64 {
	if r.Before.Contrast <= 0 {
		return 0
	}
	return r.After.Contrast / r.Before.Contrast
}

// EntropyGain 信息熵提升量（bit）。
func (r *EnhanceResult) EntropyGain() float64 {
	return r.After.Entropy - r.Before.Entropy
}

// SSR 单尺度 Retinex，等价于只用一个尺度的 MSR。
func SSR(img image.Image, sigma float64) (*image.RGBA, error) {
	return MSR(img, []float64{sigma})
}

// MSR 多尺度 Retinex，对 R/G/B 三通道**各自独立**做多尺度分解。
//
// sigmas 为空或全为非正值时回退到 DefaultMSRScales。
// 非正的单个尺度会被跳过（而不是报错），因为调用方可能只想用其中几个尺度。
func MSR(img image.Image, sigmas []float64) (*image.RGBA, error) {
	if img == nil {
		return nil, ErrNoImage
	}
	rgba := toRGBA(img)
	bounds := rgba.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("multimodal/vision: empty image bounds %v", bounds)
	}

	scales := validScales(sigmas)
	if len(scales) == 0 {
		scales = DefaultMSRScales
	}

	channels := imageToFloatChannels(rgba)
	out := image.NewRGBA(bounds)

	for c := 0; c < 3; c++ {
		enhanced := retinexChannel(channels[c], w, h, scales)
		stretchToByteRange(enhanced)
		for i, value := range enhanced {
			out.Pix[i*4+c] = clampUint8(value)
		}
	}
	// Alpha 置为不透明。
	for i := 0; i+3 < len(out.Pix); i += 4 {
		out.Pix[i+3] = 255
	}
	return out, nil
}

// MSRCR 带色彩恢复的多尺度 Retinex。
//
// 只对亮度平面做多尺度分解（三通道各自做会产生色偏），再按原色比恢复颜色。
// 相比 MSR，它在低照度彩色图像上更不容易出现偏色。
func MSRCR(img image.Image, sigmas []float64) (*image.RGBA, error) {
	if img == nil {
		return nil, ErrNoImage
	}
	rgba := toRGBA(img)
	bounds := rgba.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("multimodal/vision: empty image bounds %v", bounds)
	}

	scales := validScales(sigmas)
	if len(scales) == 0 {
		scales = DefaultMSRScales
	}

	luma := LumaPlane(rgba)
	channels := imageToFloatChannels(rgba)

	enhanced := retinexChannel(luma, w, h, scales)
	stretchToByteRange(enhanced)

	out := image.NewRGBA(bounds)
	for i, newLuma := range enhanced {
		denominator := luma[i]
		if denominator < retinexEpsilon {
			denominator = retinexEpsilon
		}
		for c := 0; c < 3; c++ {
			out.Pix[i*4+c] = clampUint8(newLuma * channels[c][i] / denominator)
		}
		out.Pix[i*4+3] = 255
	}
	return out, nil
}

// Enhance 低照度增强统一入口，同时给出处理前后的对照指标。
//
// 之所以把指标一起返回：策划书声称"提升敏感区域可见性"，这个主张需要
// 可量化的对照（对比度、信息熵），否则无法验证，也无法在比赛中出示证据。
func Enhance(img image.Image, sigmas []float64) (*EnhanceResult, error) {
	if img == nil {
		return nil, ErrNoImage
	}
	start := time.Now()

	enhanced, err := MSRCR(img, sigmas)
	if err != nil {
		return nil, err
	}
	return &EnhanceResult{
		Image:   enhanced,
		Before:  MeasureMetrics(img),
		After:   MeasureMetrics(enhanced),
		Elapsed: time.Since(start),
	}, nil
}

// retinexChannel 对单个通道做多尺度 Retinex，返回未经归一化的响应。
func retinexChannel(channel []float64, w, h int, scales []float64) []float64 {
	response := make([]float64, len(channel))

	for _, sigma := range scales {
		illuminance := blurSeparable(channel, w, h, sigma)
		for i := range response {
			response[i] += math.Log(channel[i]+retinexEpsilon) -
				math.Log(illuminance[i]+retinexEpsilon)
		}
	}

	scale := 1.0 / float64(len(scales))
	for i := range response {
		response[i] *= scale
	}
	return response
}

// stretchToByteRange 把数值线性拉伸到 [0,255]。
//
// 全等值（纯色）时直接置为 0：此时分母为 0，且"拉伸"没有意义。
// 这个分支必须显式处理，否则会产生 NaN 并污染整个输出图像。
func stretchToByteRange(values []float64) {
	if len(values) == 0 {
		return
	}
	min, max := values[0], values[0]
	for _, v := range values {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	span := max - min
	if span <= 0 || math.IsNaN(span) {
		for i := range values {
			values[i] = 0
		}
		return
	}
	for i, v := range values {
		values[i] = (v - min) / span * 255.0
	}
}

// validScales 过滤出正的高斯尺度。
func validScales(sigmas []float64) []float64 {
	out := make([]float64, 0, len(sigmas))
	for _, sigma := range sigmas {
		if sigma > 0 && !math.IsNaN(sigma) && !math.IsInf(sigma, 0) {
			out = append(out, sigma)
		}
	}
	return out
}
