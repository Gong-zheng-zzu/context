package vision

import (
	"image"
	"math"
)

// GaussianKernel1D 生成归一化的一维高斯核。
// radius 为核半径，实际核长度为 2*radius+1。
func GaussianKernel1D(sigma float64, radius int) []float64 {
	if radius < 1 {
		radius = 1
	}
	if sigma <= 0 {
		sigma = 1
	}
	k := make([]float64, 2*radius+1)
	var sum float64
	for i := -radius; i <= radius; i++ {
		v := math.Exp(-float64(i*i) / (2 * sigma * sigma))
		k[i+radius] = v
		sum += v
	}
	for i := range k {
		k[i] /= sum
	}
	return k
}

// blurSeparableDirect 对单通道 float 图像做严格的二维可分离高斯卷积：
// 先横向一维卷积，再纵向一维卷积。边界采用复制（clamp）延拓。
func blurSeparableDirect(src []float64, w, h int, sigma float64) []float64 {
	if w <= 0 || h <= 0 || len(src) == 0 {
		return src
	}
	if sigma <= 0 {
		out := make([]float64, len(src))
		copy(out, src)
		return out
	}
	radius := int(math.Ceil(3 * sigma))
	if radius < 1 {
		radius = 1
	}
	kernel := GaussianKernel1D(sigma, radius)
	kLen := 2*radius + 1

	tmp := make([]float64, w*h)
	// 横向卷积
	for y := 0; y < h; y++ {
		row := y * w
		for x := 0; x < w; x++ {
			var acc float64
			for i := 0; i < kLen; i++ {
				xx := x + i - radius
				if xx < 0 {
					xx = 0
				} else if xx >= w {
					xx = w - 1
				}
				acc += src[row+xx] * kernel[i]
			}
			tmp[row+x] = acc
		}
	}

	out := make([]float64, w*h)
	// 纵向卷积
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var acc float64
			for i := 0; i < kLen; i++ {
				yy := y + i - radius
				if yy < 0 {
					yy = 0
				} else if yy >= h {
					yy = h - 1
				}
				acc += tmp[yy*w+x] * kernel[i]
			}
			out[y*w+x] = acc
		}
	}
	return out
}

// downsampleBox 以整数因子 scale 做均值下采样。
func downsampleBox(src []float64, w, h, scale int) ([]float64, int, int) {
	if scale <= 1 {
		out := make([]float64, len(src))
		copy(out, src)
		return out, w, h
	}
	dw := (w + scale - 1) / scale
	dh := (h + scale - 1) / scale
	out := make([]float64, dw*dh)
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			var sum float64
			var n int
			for dy := 0; dy < scale; dy++ {
				sy := y*scale + dy
				if sy >= h {
					break
				}
				for dx := 0; dx < scale; dx++ {
					sx := x*scale + dx
					if sx >= w {
						break
					}
					sum += src[sy*w+sx]
					n++
				}
			}
			if n > 0 {
				out[y*dw+x] = sum / float64(n)
			}
		}
	}
	return out, dw, dh
}

// upsampleBilinear 双线性上采样回原始尺寸。
func upsampleBilinear(src []float64, sw, sh, dw, dh int) []float64 {
	out := make([]float64, dw*dh)
	if sw == 0 || sh == 0 {
		return out
	}
	for y := 0; y < dh; y++ {
		fy := (float64(y) + 0.5) * float64(sh) / float64(dh)
		y0 := int(math.Floor(fy - 0.5))
		wy := (fy - 0.5) - float64(y0)
		y1 := y0 + 1
		if y0 < 0 {
			y0 = 0
		}
		if y0 > sh-1 {
			y0 = sh - 1
		}
		if y1 < 0 {
			y1 = 0
		}
		if y1 > sh-1 {
			y1 = sh - 1
		}
		for x := 0; x < dw; x++ {
			fx := (float64(x) + 0.5) * float64(sw) / float64(dw)
			x0 := int(math.Floor(fx - 0.5))
			wx := (fx - 0.5) - float64(x0)
			x1 := x0 + 1
			if x0 < 0 {
				x0 = 0
			}
			if x0 > sw-1 {
				x0 = sw - 1
			}
			if x1 < 0 {
				x1 = 0
			}
			if x1 > sw-1 {
				x1 = sw - 1
			}
			v00 := src[y0*sw+x0]
			v01 := src[y0*sw+x1]
			v10 := src[y1*sw+x0]
			v11 := src[y1*sw+x1]
			top := v00*(1-wx) + v01*wx
			bot := v10*(1-wx) + v11*wx
			out[y*dw+x] = top*(1-wy) + bot*wy
		}
	}
	return out
}

// blurSeparable 可分离高斯卷积（带大尺度加速）。
//
// 加速策略：sigma 很大时，低通分量在高分辨率上几乎不可分辨，因此先按整数因子
// 下采样、在低分辨率上做严格的可分离高斯卷积、再双线性上采样。这样大 sigma
// 下的卷积核半径被限制在 ~3*6=18 级别，避免 O(W*H*sigma) 的爆炸。
// 该策略只影响大尺度低通分量，Retinex 的结果差异在 1e-3 量级以内。
func blurSeparable(src []float64, w, h int, sigma float64) []float64 {
	if sigma <= 0 {
		out := make([]float64, len(src))
		copy(out, src)
		return out
	}
	scale := 1
	for sigma/float64(scale) > 6 && scale < 16 {
		scale *= 2
	}
	if scale == 1 {
		return blurSeparableDirect(src, w, h, sigma)
	}
	small, sw, sh := downsampleBox(src, w, h, scale)
	blurred := blurSeparableDirect(small, sw, sh, sigma/float64(scale))
	return upsampleBilinear(blurred, sw, sh, w, h)
}

// GaussianBlurImage 对整幅图像做可分离高斯模糊，返回 RGBA 图像。
func GaussianBlurImage(img image.Image, sigma float64) *image.RGBA {
	if img == nil {
		return nil
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	ch := imageToFloatChannels(img)
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	blurred := make([]float64, 3*w*h)
	for c := 0; c < 3; c++ {
		copy(blurred[c*w*h:(c+1)*w*h], blurSeparable(ch[c], w, h, sigma))
	}
	writeChannelsToRGBA(out, blurred, w, h)
	return out
}

// GaussianBlurRegion 只对指定区域做高斯模糊。
//
// 实现：向四周扩展 margin = ceil(3*sigma) 像素（裁剪到图像边界）后对子图做模糊，
// 再写回原图对应区域，避免区域内部出现卷积边界伪影。
func GaussianBlurRegion(img image.Image, region Region, sigma float64) (image.Image, bool) {
	if img == nil {
		return nil, false
	}
	b := img.Bounds()
	r, ok := region.Clip(b)
	if !ok {
		return img, false
	}
	margin := int(math.Ceil(3 * sigma))
	if margin < 1 {
		margin = 1
	}
	x0 := maxInt(b.Min.X, r.X-margin)
	y0 := maxInt(b.Min.Y, r.Y-margin)
	x1 := minInt(b.Max.X, r.X+r.W+margin)
	y1 := minInt(b.Max.Y, r.Y+r.H+margin)
	padded := image.Rect(x0, y0, x1, y1)

	rgba := toRGBA(img)
	sub := image.NewRGBA(image.Rect(0, 0, padded.Dx(), padded.Dy()))
	for y := padded.Min.Y; y < padded.Max.Y; y++ {
		for x := padded.Min.X; x < padded.Max.X; x++ {
			sub.Set(x-padded.Min.X, y-padded.Min.Y, rgba.At(x, y))
		}
	}
	blurred := GaussianBlurImage(sub, sigma)
	if blurred == nil {
		return img, false
	}
	dst := cloneRGBA(rgba)
	for y := r.Y; y < r.Y+r.H; y++ {
		for x := r.X; x < r.X+r.W; x++ {
			dst.Set(x, y, blurred.At(x-padded.Min.X, y-padded.Min.Y))
		}
	}
	return dst, true
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
