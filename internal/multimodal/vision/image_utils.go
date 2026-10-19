package vision

import (
	"image"
	"image/color"
	"math"
)

// toRGBA 将任意 image.Image 转为 *image.RGBA（原点归零）。
func toRGBA(img image.Image) *image.RGBA {
	if img == nil {
		return nil
	}
	if r, ok := img.(*image.RGBA); ok && r.Bounds().Min.X == 0 && r.Bounds().Min.Y == 0 {
		return r
	}
	b := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			dst.Set(x, y, color.RGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)))
		}
	}
	return dst
}

// cloneRGBA 深拷贝 RGBA 图像。
func cloneRGBA(src *image.RGBA) *image.RGBA {
	if src == nil {
		return nil
	}
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		copy(dst.Pix[y*dst.Stride:(y*dst.Stride)+b.Dx()*4],
			src.Pix[(b.Min.Y+y-src.Rect.Min.Y)*src.Stride+(b.Min.X-src.Rect.Min.X)*4:])
	}
	return dst
}

// imageToFloatChannels 提取 R/G/B 三通道 float64 数据（范围 0~255）。
func imageToFloatChannels(img image.Image) [3][]float64 {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	var ch [3][]float64
	for c := 0; c < 3; c++ {
		ch[c] = make([]float64, w*h)
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, bb, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			i := y*w + x
			ch[0][i] = float64(r >> 8)
			ch[1][i] = float64(g >> 8)
			ch[2][i] = float64(bb >> 8)
		}
	}
	return ch
}

// writeChannelsToRGBA 将 3 个 float64 通道（0~255，越界截断）写入 RGBA 图像。
func writeChannelsToRGBA(dst *image.RGBA, ch []float64, w, h int) {
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			dst.Set(x, y, color.RGBA{
				R: clampUint8(ch[i]),
				G: clampUint8(ch[w*h+i]),
				B: clampUint8(ch[2*w*h+i]),
				A: 255,
			})
		}
	}
}

func clampUint8(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v + 0.5)
}

// LumaPlane 返回亮度平面（0~255），系数采用 Rec.601。
func LumaPlane(img image.Image) []float64 {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	out := make([]float64, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, bb, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			out[y*w+x] = 0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(bb>>8)
		}
	}
	return out
}

// Metrics 图像量化指标，用于增强前后的可验证对照。
type Metrics struct {
	// Contrast 为亮度标准差（0~255 量纲）。
	Contrast float64 `json:"contrast"`
	// Entropy 为亮度直方图（256 bin）的香农熵，单位 bit。
	Entropy float64 `json:"entropy"`
	// MeanLuma 为平均亮度。
	MeanLuma float64 `json:"mean_luma"`
}

// MeasureMetrics 计算图像的对比度与信息熵。
func MeasureMetrics(img image.Image) Metrics {
	luma := LumaPlane(img)
	n := len(luma)
	if n == 0 {
		return Metrics{}
	}
	var mean float64
	for _, v := range luma {
		mean += v
	}
	mean /= float64(n)

	var variance float64
	hist := make([]float64, 256)
	for _, v := range luma {
		d := v - mean
		variance += d * d
		hist[clampUint8(v)]++
	}
	variance /= float64(n)

	var entropy float64
	for _, c := range hist {
		if c <= 0 {
			continue
		}
		p := c / float64(n)
		entropy -= p * math.Log2(p)
	}
	return Metrics{
		Contrast: math.Sqrt(variance),
		Entropy:  entropy,
		MeanLuma: mean,
	}
}

// percentile 返回给定分位数（0~1）对应的值，输入会被排序副本处理。
func percentile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if q <= 0 {
		return sorted[0]
	}
	if q >= 1 {
		return sorted[len(sorted)-1]
	}
	pos := q * float64(len(sorted)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return sorted[lo]
	}
	frac := pos - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}
