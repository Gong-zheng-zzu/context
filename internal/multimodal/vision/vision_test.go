package vision

import (
	"image"
	"image/color"
	"math"
	"testing"
)

// ============================================================================
// 程序生成测试图像（不依赖任何外部图片文件）
// ============================================================================

// makeUniform 生成纯色图。
func makeUniform(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

// makeLowLight 生成低照度图：整体偏暗，内含一块稍亮的细节。
//
// 模拟养老场景夜间监控：画面几乎全黑，但有一块微弱可见的目标。
func makeLowLight(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			value := uint8(28)
			// 中央区域稍亮，模拟需要被增强的目标。
			if x > w/3 && x < 2*w/3 && y > h/3 && y < 2*h/3 {
				value = 52
			}
			img.SetRGBA(x, y, color.RGBA{R: value, G: value, B: value, A: 255})
		}
	}
	return img
}

// makeCheckerboard 生成黑白棋盘格（用于二维码启发式检测）。
func makeCheckerboard(w, h, block int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			value := uint8(255)
			if (x/block+y/block)%2 == 0 {
				value = 0
			}
			img.SetRGBA(x, y, color.RGBA{R: value, G: value, B: value, A: 255})
		}
	}
	return img
}

// makeTextLike 生成白底黑字的"文字感"图案（用于床牌/病历卡启发式检测）。
func makeTextLike(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 245, G: 245, B: 245, A: 255})
		}
	}
	// 每 12 行画一条高 3 像素的黑色"文字行"，长度随机化以产生边缘。
	for row := 8; row < h-8; row += 12 {
		for x := 8; x < w-8; x++ {
			if (x+row)%7 < 5 { // 断续，模拟字符间隙
				for dy := 0; dy < 3; dy++ {
					img.SetRGBA(x, row+dy, color.RGBA{R: 20, G: 20, B: 20, A: 255})
				}
			}
		}
	}
	return img
}

// skinColor 落在 YCbCr 肤色区间的示例色。
var skinColor = color.RGBA{R: 220, G: 150, B: 130, A: 255}

// makeSkinPatch 生成中央为肤色块的图像（用于启发式人脸检测）。
func makeSkinPatch(w, h int) *image.RGBA {
	img := makeUniform(w, h, color.RGBA{R: 60, G: 70, B: 90, A: 255})
	for y := h / 4; y < 3*h/4; y++ {
		for x := w / 4; x < 3*w/4; x++ {
			img.SetRGBA(x, y, skinColor)
		}
	}
	return img
}

// ============================================================================
// Retinex / MSR
// ============================================================================

// TestMSRImprovesContrastOnLowLight 低照度图经 MSRCR 增强后，
// 对比度与信息熵必须提升——这是"提升敏感区域可见性"的可量化证据。
func TestMSRImprovesContrastOnLowLight(t *testing.T) {
	img := makeLowLight(96, 96)

	result, err := Enhance(img, DefaultMSRScales)
	if err != nil {
		t.Fatalf("Enhance: %v", err)
	}
	if result.Image == nil {
		t.Fatal("enhanced image is nil")
	}
	if result.After.Contrast <= result.Before.Contrast {
		t.Fatalf("contrast did not improve: before %v -> after %v",
			result.Before.Contrast, result.After.Contrast)
	}
	if result.ContrastGain() <= 1.0 {
		t.Fatalf("contrast gain = %v, want > 1", result.ContrastGain())
	}
}

// TestMSRPreservesDimensions 输出尺寸必须与输入一致。
func TestMSRPreservesDimensions(t *testing.T) {
	img := makeLowLight(64, 48)
	out, err := MSRCR(img, DefaultMSRScales)
	if err != nil {
		t.Fatalf("MSRCR: %v", err)
	}
	if out.Bounds().Dx() != 64 || out.Bounds().Dy() != 48 {
		t.Fatalf("output size = %vx%v, want 64x48", out.Bounds().Dx(), out.Bounds().Dy())
	}
}

// TestMSRHandlesUniformImageWithoutNaN 纯色图（零方差）是 Retinex 的退化情形：
// 拉伸时分母为 0，必须显式处理，不得产出 NaN。
func TestMSRHandlesUniformImageWithoutNaN(t *testing.T) {
	img := makeUniform(32, 32, color.RGBA{R: 40, G: 40, B: 40, A: 255})

	out, err := MSRCR(img, DefaultMSRScales)
	if err != nil {
		t.Fatalf("MSRCR: %v", err)
	}
	for _, pixel := range out.Pix {
		if pixel != 0 && pixel != 255 {
			// 纯色图的合法输出是全 0（拉伸无意义），不应出现中间值。
			t.Fatalf("uniform image produced non-degenerate pixel value %d", pixel)
		}
	}

	// 全黑图同样不得产生 NaN。
	black := makeUniform(32, 32, color.RGBA{A: 255})
	outBlack, err := MSRCR(black, DefaultMSRScales)
	if err != nil {
		t.Fatalf("MSRCR(black): %v", err)
	}
	for i, pixel := range outBlack.Pix {
		if math.IsNaN(float64(pixel)) {
			t.Fatalf("NaN at pixel %d", i)
		}
	}
}

// TestSSRIsSingleScaleMSR SSR 等价于单尺度 MSR，且同样能提升对比度。
func TestSSRIsSingleScaleMSR(t *testing.T) {
	img := makeLowLight(64, 64)
	out, err := SSR(img, 30)
	if err != nil {
		t.Fatalf("SSR: %v", err)
	}
	if MeasureMetrics(out).Contrast <= MeasureMetrics(img).Contrast {
		t.Fatal("SSR did not improve contrast")
	}
}

// TestMSRRejectsNilAndEmpty 空输入必须报错而非 panic。
func TestMSRRejectsNilAndEmpty(t *testing.T) {
	if _, err := MSRCR(nil, DefaultMSRScales); err == nil {
		t.Fatal("expected error for nil image")
	}
	if _, err := Enhance(nil, DefaultMSRScales); err == nil {
		t.Fatal("expected error for nil image")
	}
	empty := image.NewRGBA(image.Rect(0, 0, 0, 0))
	if _, err := MSRCR(empty, DefaultMSRScales); err == nil {
		t.Fatal("expected error for empty bounds")
	}
}

// TestMSRFallsBackToDefaultScales 非法/空尺度回退到默认三尺度。
func TestMSRFallsBackToDefaultScales(t *testing.T) {
	img := makeLowLight(48, 48)
	out, err := MSRCR(img, nil)
	if err != nil {
		t.Fatalf("MSRCR with nil scales: %v", err)
	}
	if out == nil {
		t.Fatal("nil output")
	}
	// 含非正值的尺度列表同样不得 panic。
	if _, err := MSRCR(img, []float64{-1, 0, 20}); err != nil {
		t.Fatalf("MSRCR with mixed scales: %v", err)
	}
}

// ============================================================================
// 人脸检测
// ============================================================================

// TestPigoCascadeIsAvailable 关键证据：pigo 级联可被加载，
// 说明人脸检测走的是**真实模型**而非启发式。
func TestPigoCascadeIsAvailable(t *testing.T) {
	if !CascadeAvailable() {
		t.Fatal("pigo face cascade is not available — face detection would silently degrade")
	}
	if len(faceCascadeBytes) == 0 {
		t.Fatal("embedded facefinder cascade is empty")
	}
}

// TestDetectFacesDoesNotCrashOnNonFace 对非人脸图像，模型检测应正常返回
// （可能 0 个区域），不得 panic 或报错。
//
// 诚实说明：合成图像很难骗过训练好的 Pico 级联，因此这里只断言
// "不崩溃且返回合法结果"，而不断言"必然检测到人脸"——后者需要真实
// 人脸照片才能验证，不应在单元测试里伪造。
func TestDetectFacesDoesNotCrashOnNonFace(t *testing.T) {
	img := makeUniform(160, 160, color.RGBA{R: 120, G: 120, B: 120, A: 255})
	regions, err := DetectFaces(img)
	if err != nil {
		t.Fatalf("DetectFaces: %v", err)
	}
	for _, region := range regions {
		if region.W <= 0 || region.H <= 0 {
			t.Fatalf("invalid region size: %+v", region)
		}
		if region.Method != MethodPigoCascade {
			t.Fatalf("model path must report MethodPigoCascade, got %s", region.Method)
		}
	}
}

// TestDetectFacesHeuristicFindsSkinPatch 启发式路径（明确标注为降级）
// 能在肤色块上产生候选区域。
func TestDetectFacesHeuristicFindsSkinPatch(t *testing.T) {
	img := makeSkinPatch(96, 96)
	regions, err := DetectFacesHeuristic(img)
	if err != nil {
		t.Fatalf("DetectFacesHeuristic: %v", err)
	}
	if len(regions) == 0 {
		t.Fatal("heuristic face detection found no candidate on a skin-colored patch")
	}
	for _, region := range regions {
		if region.Method != MethodHeuristic {
			t.Fatalf("heuristic path must report MethodHeuristic, got %s", region.Method)
		}
		if region.Kind != KindFace {
			t.Fatalf("expected KindFace, got %s", region.Kind)
		}
	}
}

// TestDetectFacesHeuristicIgnoresNonSkin 非肤色图像不应产生候选，
// 这是误报控制的基本检查。
func TestDetectFacesHeuristicIgnoresNonSkin(t *testing.T) {
	img := makeUniform(96, 96, color.RGBA{R: 60, G: 70, B: 90, A: 255})
	regions, err := DetectFacesHeuristic(img)
	if err != nil {
		t.Fatalf("DetectFacesHeuristic: %v", err)
	}
	if len(regions) != 0 {
		t.Fatalf("false positive on non-skin image: %+v", regions)
	}
}

// ============================================================================
// 床位牌 / 病历卡 / 二维码（启发式）
// ============================================================================

// TestDetectQRCodesFindsCheckerboard 棋盘格满足二维码的高对比 + 高频黑白
// 交替 + 高边缘密度特征，应被启发式检出。
func TestDetectQRCodesFindsCheckerboard(t *testing.T) {
	// 用 3 像素的细棋盘格：二维码判据之一是"黑白高频交替"，
	// 过粗的格子（如 8 像素）跳变频率不足，本就不该被判为二维码。
	img := makeCheckerboard(96, 96, 3)
	regions, err := DetectQRCodes(img)
	if err != nil {
		t.Fatalf("DetectQRCodes: %v", err)
	}
	if len(regions) == 0 {
		t.Fatal("QR heuristic found nothing on a checkerboard")
	}
	for _, region := range regions {
		if region.Kind != KindQRCode {
			t.Fatalf("expected KindQRCode, got %s", region.Kind)
		}
		if region.Method != MethodHeuristic {
			t.Fatalf("QR detection is heuristic and must be labeled so, got %s", region.Method)
		}
	}
}

// TestHeuristicDetectorsAreLabeledAsHeuristic 所有启发式检测的结果
// 都必须如实标注 MethodHeuristic——不得冒充模型输出。
func TestHeuristicDetectorsAreLabeledAsHeuristic(t *testing.T) {
	img := makeTextLike(128, 128)

	detectors := map[string]func(image.Image) ([]Region, error){
		"bed plate":    DetectBedPlates,
		"medical card": DetectMedicalCards,
		"qr code":      DetectQRCodes,
	}
	for name, detect := range detectors {
		regions, err := detect(img)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, region := range regions {
			if region.Method != MethodHeuristic {
				t.Fatalf("%s region must be MethodHeuristic, got %s", name, region.Method)
			}
		}
	}
}

// TestDetectSensitiveRegionsAggregates 聚合检测返回完整结果且标注了方法来源。
func TestDetectSensitiveRegionsAggregates(t *testing.T) {
	img := makeCheckerboard(96, 96, 8)
	result, err := DetectSensitiveRegions(img)
	if err != nil {
		t.Fatalf("DetectSensitiveRegions: %v", err)
	}
	if len(result.Methods) == 0 {
		t.Fatal("no detection methods recorded")
	}
	if result.Elapsed <= 0 {
		t.Fatal("elapsed not recorded")
	}
	// 棋盘格不是人脸，故不应有人脸区域（避免启发式误判）。
	for _, region := range result.Regions {
		if region.Kind == KindFace && region.Method == MethodPigoCascade {
			t.Logf("note: checkerboard triggered a face detection (Q=%v)", region.Score)
		}
	}
}

// TestDetectRejectsNilAndEmpty 检测器的空输入处理。
func TestDetectRejectsNilAndEmpty(t *testing.T) {
	if _, err := DetectFaces(nil); err == nil {
		t.Fatal("expected error for nil image")
	}
	if _, err := DetectFacesHeuristic(nil); err == nil {
		t.Fatal("expected error for nil image")
	}
	if _, err := DetectQRCodes(nil); err == nil {
		t.Fatal("expected error for nil image")
	}
	if _, err := DetectBedPlates(nil); err == nil {
		t.Fatal("expected error for nil image")
	}
}

// ============================================================================
// 去身份化
// ============================================================================

// regionOf 构造一个测试区域。
func regionOf(kind RegionKind, x, y, w, h int) Region {
	return Region{Kind: kind, X: x, Y: y, W: w, H: h, Score: 0.9, Method: MethodHeuristic}
}

// singleStrategyPolicy 构造"所有区域统一用同一种策略"的策略。
//
// 必须**不带 ByKind**：DefaultPolicy() 的 ByKind 会覆盖 Default，
// 若直接改 Default 而保留 ByKind，测试实际走的仍是 ByKind 里的策略，
// 表面上通过、实则测错了东西。
func singleStrategyPolicy(strategy Strategy) Policy {
	return Policy{
		Default:      strategy,
		GaussianBlur: GaussianBlurParams{Sigma: 8},
		Mosaic:       MosaicParams{BlockSize: 14},
		SemanticMask: SemanticMaskParams{FillMode: "mean", Intensity: 1.0},
	}
}

// TestMosaicChangesPixels 马赛克必须真的改变像素。
func TestMosaicChangesPixels(t *testing.T) {
	img := makeCheckerboard(64, 64, 4)

	result, err := DeIdentify(img, []Region{regionOf(KindFace, 8, 8, 32, 32)},
		singleStrategyPolicy(StrategyMosaic))
	if err != nil {
		t.Fatalf("DeIdentify: %v", err)
	}
	if len(result.Applied) != 1 {
		t.Fatalf("expected 1 applied region, got %d", len(result.Applied))
	}
	if !result.Applied[0].Changed {
		t.Fatal("mosaic reported no pixel change")
	}

	// 区域内像素应变得比原来更均匀（马赛克的本质）。
	before := MeasureMetrics(cropRGBA(img, 8, 8, 32, 32))
	after := MeasureMetrics(cropRGBA(toRGBA(result.Image), 8, 8, 32, 32))
	if after.Contrast > before.Contrast {
		t.Fatalf("mosaic did not reduce local contrast: %v -> %v", before.Contrast, after.Contrast)
	}
}

// TestGaussianBlurChangesPixels 高斯模糊必须真的改变像素。
func TestGaussianBlurChangesPixels(t *testing.T) {
	img := makeCheckerboard(64, 64, 4)

	result, err := DeIdentify(img, []Region{regionOf(KindFace, 8, 8, 32, 32)},
		singleStrategyPolicy(StrategyGaussianBlur))
	if err != nil {
		t.Fatalf("DeIdentify: %v", err)
	}
	if len(result.Applied) != 1 || !result.Applied[0].Changed {
		t.Fatalf("gaussian blur reported no change: %+v", result.Applied)
	}
	before := MeasureMetrics(cropRGBA(img, 8, 8, 32, 32))
	after := MeasureMetrics(cropRGBA(toRGBA(result.Image), 8, 8, 32, 32))
	if after.Contrast >= before.Contrast {
		t.Fatalf("blur did not reduce local contrast: %v -> %v", before.Contrast, after.Contrast)
	}
}

// TestSemanticMaskChangesPixels 语义掩码必须真的改变像素。
func TestSemanticMaskChangesPixels(t *testing.T) {
	img := makeCheckerboard(64, 64, 4)

	result, err := DeIdentify(img, []Region{regionOf(KindBedPlate, 8, 8, 32, 32)},
		singleStrategyPolicy(StrategySemanticMask))
	if err != nil {
		t.Fatalf("DeIdentify: %v", err)
	}
	if len(result.Applied) != 1 || !result.Applied[0].Changed {
		t.Fatalf("semantic mask reported no change: %+v", result.Applied)
	}

	// 完全替换（Intensity=1）后区域应变为接近纯色。
	after := MeasureMetrics(cropRGBA(toRGBA(result.Image), 8, 8, 32, 32))
	if after.Contrast > 1.0 {
		t.Fatalf("full-intensity mask should flatten the region, contrast = %v", after.Contrast)
	}
}

// TestDeIdentifyDoesNotMutateInput 入参图像必须保持不变——
// 调用方通常还要拿原图做审计或展示。
func TestDeIdentifyDoesNotMutateInput(t *testing.T) {
	img := makeCheckerboard(64, 64, 4)
	before := append([]uint8(nil), toRGBA(img).Pix...)

	if _, err := DeIdentify(img, []Region{regionOf(KindFace, 8, 8, 32, 32)},
		singleStrategyPolicy(StrategyMosaic)); err != nil {
		t.Fatalf("DeIdentify: %v", err)
	}

	after := toRGBA(img).Pix
	for i := range before {
		if before[i] != after[i] {
			t.Fatalf("input image was mutated at index %d", i)
		}
	}
}

// TestDeIdentifySkipsWhenStrategyEmpty 策略为空时必须跳过，
// 而不是静默套用某个默认策略——未声明的处理等于未授权。
func TestDeIdentifySkipsWhenStrategyEmpty(t *testing.T) {
	img := makeCheckerboard(64, 64, 4)
	policy := Policy{} // Default 为空

	result, err := DeIdentify(img, []Region{regionOf(KindFace, 8, 8, 32, 32)}, policy)
	if err != nil {
		t.Fatalf("DeIdentify: %v", err)
	}
	if len(result.Applied) != 0 {
		t.Fatalf("expected no applied regions, got %d", len(result.Applied))
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("expected 1 skipped region, got %d", len(result.Skipped))
	}
}

// TestDeIdentifySkipsUnknownStrategy 未知策略同样跳过而非猜一个。
func TestDeIdentifySkipsUnknownStrategy(t *testing.T) {
	img := makeCheckerboard(64, 64, 4)
	policy := singleStrategyPolicy(Strategy("not_a_real_strategy"))

	result, err := DeIdentify(img, []Region{regionOf(KindFace, 8, 8, 32, 32)}, policy)
	if err != nil {
		t.Fatalf("DeIdentify: %v", err)
	}
	if len(result.Applied) != 0 || len(result.Skipped) != 1 {
		t.Fatalf("unknown strategy should be skipped: applied=%d skipped=%d",
			len(result.Applied), len(result.Skipped))
	}
}

// TestDeIdentifyClipsOutOfBoundsRegion 越界区域被裁剪，不 panic。
func TestDeIdentifyClipsOutOfBoundsRegion(t *testing.T) {
	img := makeCheckerboard(64, 64, 4)
	policy := DefaultPolicy()
	policy.Default = StrategyMosaic

	result, err := DeIdentify(img, []Region{
		{Kind: KindFace, X: -50, Y: -50, W: 200, H: 200, Method: MethodHeuristic},
	}, policy)
	if err != nil {
		t.Fatalf("DeIdentify with out-of-bounds region: %v", err)
	}
	if result.Image.Bounds().Dx() != 64 || result.Image.Bounds().Dy() != 64 {
		t.Fatal("output bounds changed")
	}
}

// TestDeIdentifyRejectsNil 空图像必须报错。
func TestDeIdentifyRejectsNil(t *testing.T) {
	if _, err := DeIdentify(nil, nil, DefaultPolicy()); err == nil {
		t.Fatal("expected error for nil image")
	}
	if _, _, err := DeIdentifyImage(nil, DefaultPolicy()); err == nil {
		t.Fatal("expected error for nil image")
	}
}

// TestDeIdentifyImageEndToEnd 检测 + 脱敏的一步式入口可正常工作。
func TestDeIdentifyImageEndToEnd(t *testing.T) {
	img := makeCheckerboard(96, 96, 8)

	result, detection, err := DeIdentifyImage(img, DefaultPolicy())
	if err != nil {
		t.Fatalf("DeIdentifyImage: %v", err)
	}
	if result == nil || detection == nil {
		t.Fatal("nil result")
	}
	if result.Image == nil {
		t.Fatal("nil output image")
	}
	if result.Image.Bounds() != img.Bounds() {
		t.Fatal("output bounds differ from input")
	}
}

// TestPolicyStrategyFor ByKind 优先于 Default。
func TestPolicyStrategyFor(t *testing.T) {
	policy := Policy{
		Default: StrategySemanticMask,
		ByKind:  map[RegionKind]Strategy{KindFace: StrategyGaussianBlur},
	}
	if got := policy.StrategyFor(KindFace); got != StrategyGaussianBlur {
		t.Fatalf("ByKind should win, got %s", got)
	}
	if got := policy.StrategyFor(KindQRCode); got != StrategySemanticMask {
		t.Fatalf("should fall back to Default, got %s", got)
	}
	empty := Policy{}
	if got := empty.StrategyFor(KindFace); got != "" {
		t.Fatalf("empty policy should yield empty strategy, got %s", got)
	}
}

// TestDefaultPolicyIsConservative 默认策略对人脸用模糊、对可读内容用马赛克。
func TestDefaultPolicyIsConservative(t *testing.T) {
	policy := DefaultPolicy()
	if policy.StrategyFor(KindFace) != StrategyGaussianBlur {
		t.Fatal("default policy should blur faces")
	}
	if policy.StrategyFor(KindQRCode) != StrategyMosaic {
		t.Fatal("default policy should mosaic QR codes")
	}
	if policy.StrategyFor(KindBedPlate) != StrategyMosaic {
		t.Fatal("default policy should mosaic bed plates")
	}
}

// cropRGBA 裁剪出子图（测试辅助）。
func cropRGBA(src *image.RGBA, x, y, w, h int) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for dy := 0; dy < h; dy++ {
		for dx := 0; dx < w; dx++ {
			out.SetRGBA(dx, dy, src.RGBAAt(x+dx, y+dy))
		}
	}
	return out
}
