package vision

import (
	"image"
	"image/color"
	"time"
)

// ============================================================================
// 像素级去身份化：统一入口
// ============================================================================
//
// 三种策略与策划书表述的对应关系：
//   - StrategyGaussianBlur —— 人脸等需要"看得出有人但认不出是谁"的区域
//   - StrategyMosaic       —— 床位牌、病历卡、二维码等含可读文字/可解码内容的区域
//   - StrategySemanticMask —— 其余敏感区域，用语义掩码整体替换
//
// 设计要点
// --------
//  1. **逐区域记录审计信息**：每个区域的处理策略、耗时、是否真的改动了像素
//     都会记录。其中 `Changed` 很重要——若某区域策略生效但像素没变
//     （例如区域尺寸为 0），应当被如实记录为未改动，而不是默认成功。
//  2. **策略为空即跳过**，并记入 Skipped，而不是回退到某个默认策略。
//     隐私场景下"静默替用户选一个策略"是危险的：未声明的处理等于未授权。
//  3. 区域一律先 `Clip` 到图像边界，避免越界写入导致 panic。

// StrategyFor 返回指定区域类别应使用的策略。
//
// 优先级：ByKind > Default。Default 为空时返回空策略（表示不处理）。
func (p Policy) StrategyFor(kind RegionKind) Strategy {
	if strategy, ok := p.ByKind[kind]; ok && strategy != "" {
		return strategy
	}
	return p.Default
}

// DeIdentify 对给定区域执行去身份化处理。
//
// 返回的结果图像是**新的**图像，入参 img 不会被修改。
func DeIdentify(img image.Image, regions []Region, policy Policy) (*DeIdentifyResult, error) {
	if img == nil {
		return nil, ErrNoImage
	}
	start := time.Now()

	source := toRGBA(img)
	canvas := image.NewRGBA(source.Bounds())
	copy(canvas.Pix, source.Pix)

	result := &DeIdentifyResult{}

	for _, region := range regions {
		strategy := policy.StrategyFor(region.Kind)
		if strategy == "" {
			result.Skipped = append(result.Skipped, region)
			continue
		}

		regionStart := time.Now()
		changed := false
		handled := true

		switch strategy {
		case StrategyGaussianBlur:
			changed = applyGaussianBlur(canvas, region, policy.GaussianBlur)
		case StrategyMosaic:
			changed = applyMosaic(canvas, region, policy.Mosaic)
		case StrategySemanticMask:
			changed = applySemanticMask(canvas, region, policy.SemanticMask)
		default:
			// 未知策略：不猜测，直接跳过并计入 Skipped。
			handled = false
		}

		if !handled {
			result.Skipped = append(result.Skipped, region)
			continue
		}
		result.Applied = append(result.Applied, RegionResult{
			Region:   region,
			Strategy: strategy,
			Elapsed:  time.Since(regionStart),
			Changed:  changed,
		})
	}

	result.Image = canvas
	result.Elapsed = time.Since(start)
	return result, nil
}

// DeIdentifyImage 检测敏感区域并一步完成去身份化。
//
// 这是面向调用方的便捷入口：先 DetectSensitiveRegions 再 DeIdentify。
// 单独提供它的原因是这两步几乎总是成对出现，让调用方自己拼容易
// 漏掉"检测失败时该怎么办"的处理。
func DeIdentifyImage(img image.Image, policy Policy) (*DeIdentifyResult, *DetectResult, error) {
	if img == nil {
		return nil, nil, ErrNoImage
	}
	detection, err := DetectSensitiveRegions(img)
	if err != nil {
		return nil, nil, err
	}
	result, err := DeIdentify(img, detection.Regions, policy)
	if err != nil {
		return nil, detection, err
	}
	return result, detection, nil
}

// ============================================================================
// 策略实现
// ============================================================================

// defaultBlurSigma 高斯模糊默认标准差（与 DefaultPolicy 保持一致）。
const defaultBlurSigma = 8.0

// defaultMosaicBlock 马赛克默认块边长。
const defaultMosaicBlock = 14

// applyGaussianBlur 对区域做高斯模糊。
//
// 复用 GaussianBlurRegion（它已处理区域边界的卷积伪影：向四周扩展
// margin 后模糊子图，再写回原图）。
func applyGaussianBlur(canvas *image.RGBA, region Region, params GaussianBlurParams) bool {
	sigma := params.Sigma
	if sigma <= 0 {
		sigma = defaultBlurSigma
	}
	blurred, ok := GaussianBlurRegion(canvas, region, sigma)
	if !ok || blurred == nil {
		return false
	}

	blurredRGBA, isRGBA := blurred.(*image.RGBA)
	if !isRGBA {
		blurredRGBA = toRGBA(blurred)
	}
	// 尺寸不一致时不能盲目拷贝，否则会越界或留下未处理区域。
	if len(blurredRGBA.Pix) != len(canvas.Pix) ||
		!blurredRGBA.Bounds().Eq(canvas.Bounds()) {
		return false
	}

	before := append([]uint8(nil), canvas.Pix...)
	copy(canvas.Pix, blurredRGBA.Pix)
	return !samePixels(before, canvas.Pix)
}

// applyMosaic 对区域做马赛克：每个块用块内均值色填充。
func applyMosaic(canvas *image.RGBA, region Region, params MosaicParams) bool {
	clipped, ok := region.Clip(canvas.Bounds())
	if !ok {
		return false
	}
	block := params.BlockSize
	if block <= 0 {
		block = defaultMosaicBlock
	}

	changed := false
	for by := clipped.Y; by < clipped.Y+clipped.H; by += block {
		for bx := clipped.X; bx < clipped.X+clipped.W; bx += block {
			x1 := minInt(bx+block, clipped.X+clipped.W)
			y1 := minInt(by+block, clipped.Y+clipped.H)
			r, g, b := averageColor(canvas, bx, by, x1, y1)
			fill := color.RGBA{R: r, G: g, B: b, A: 255}
			for y := by; y < y1; y++ {
				for x := bx; x < x1; x++ {
					if canvas.RGBAAt(x, y) != fill {
						canvas.SetRGBA(x, y, fill)
						changed = true
					}
				}
			}
		}
	}
	return changed
}

// applySemanticMask 语义掩码替换。
//
// FillMode:
//   - "mean"（默认）：用区域均值色按 Intensity 比例混合替换，完全替换时区域变为纯色块。
//   - "keep_shape"：用较大的块做粗粒度马赛克，保留大致轮廓但消除细节。
//
// Intensity ∈ [0,1]，1 表示完全替换。超出范围会被夹取。
func applySemanticMask(canvas *image.RGBA, region Region, params SemanticMaskParams) bool {
	clipped, ok := region.Clip(canvas.Bounds())
	if !ok {
		return false
	}

	if params.FillMode == "keep_shape" {
		// 粗粒度马赛克：块边长取区域短边的 1/4，至少 4 像素。
		shortEdge := minInt(clipped.W, clipped.H)
		block := shortEdge / 4
		if block < 4 {
			block = 4
		}
		return applyMosaic(canvas, region, MosaicParams{BlockSize: block})
	}

	intensity := params.Intensity
	if intensity <= 0 {
		intensity = 1.0
	}
	if intensity > 1 {
		intensity = 1.0
	}

	r, g, b := averageColor(canvas, clipped.X, clipped.Y,
		clipped.X+clipped.W, clipped.Y+clipped.H)

	changed := false
	for y := clipped.Y; y < clipped.Y+clipped.H; y++ {
		for x := clipped.X; x < clipped.X+clipped.W; x++ {
			src := canvas.RGBAAt(x, y)
			target := color.RGBA{
				R: mixUint8(src.R, r, intensity),
				G: mixUint8(src.G, g, intensity),
				B: mixUint8(src.B, b, intensity),
				A: 255,
			}
			if src != target {
				canvas.SetRGBA(x, y, target)
				changed = true
			}
		}
	}
	return changed
}

// ============================================================================
// 辅助
// ============================================================================

// averageColor 计算矩形区域的平均颜色。
func averageColor(img *image.RGBA, x0, y0, x1, y1 int) (uint8, uint8, uint8) {
	sumR, sumG, sumB, count := 0, 0, 0, 0
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			c := img.RGBAAt(x, y)
			sumR += int(c.R)
			sumG += int(c.G)
			sumB += int(c.B)
			count++
		}
	}
	if count == 0 {
		return 0, 0, 0
	}
	return uint8(sumR / count), uint8(sumG / count), uint8(sumB / count)
}

// mixUint8 按 intensity 把 src 向 target 混合。
func mixUint8(src, target uint8, intensity float64) uint8 {
	value := float64(target)*intensity + float64(src)*(1-intensity)
	if value < 0 {
		value = 0
	}
	if value > 255 {
		value = 255
	}
	return uint8(value + 0.5)
}

// samePixels 逐字节比较两段像素数据。
//
// 只在需要判断"是否真的改动了"时使用；O(n) 但只在模糊策略下调用一次，
// 相比让调用方无从判断"到底改没改"，这点开销是值得的。
func samePixels(a, b []uint8) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
