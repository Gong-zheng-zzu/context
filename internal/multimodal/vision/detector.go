package vision

import (
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"math"
	"time"

	pigo "github.com/esimov/pigo/core"
)

// faceCascadeBytes 内嵌的 Pico 人脸检测级联（来自 esimov/pigo）。
//
// 使用 go:embed 而非运行时从磁盘加载，是为了让检测结果不依赖部署环境的
// 文件路径——这对"接口化与审计"很关键：同样一份二进制在任何机器上
// 都应给出同样的检测能力声明。
//
//go:embed cascades/facefinder
var faceCascadeBytes []byte

// ============================================================================
// 人脸检测：pigo Pico 级联（真实模型）
// ============================================================================

// faceClassifier 惰性解包并缓存的分类器。
//
// 解包级联有一定开销，且分类器本身是只读的，因此全局缓存一份即可。
// 并发安全：pigo 的 RunCascade 不修改分类器内部状态。
var faceClassifier *pigo.Pigo

// faceClassifierErr 记录首次解包的失败原因，避免每次调用都重试解包。
var faceClassifierErr error

// cascadeReady 标记分类器是否已尝试初始化。
var cascadeReady bool

// loadFaceClassifier 加载（并缓存）人脸级联分类器。
func loadFaceClassifier() (*pigo.Pigo, error) {
	if cascadeReady {
		return faceClassifier, faceClassifierErr
	}
	cascadeReady = true

	if len(faceCascadeBytes) == 0 {
		faceClassifierErr = fmt.Errorf("内嵌的 facefinder 级联为空")
		return nil, faceClassifierErr
	}
	classifier, err := pigo.NewPigo().Unpack(faceCascadeBytes)
	if err != nil {
		faceClassifierErr = fmt.Errorf("解包人脸级联失败: %w", err)
		return nil, faceClassifierErr
	}
	faceClassifier = classifier
	return classifier, nil
}

// CascadeAvailable 报告真实模型（pigo 级联）是否可用。
//
// 暴露该方法是为了让上层能**主动声明**本次检测用的是模型还是启发式，
// 而不是在结果里含糊带过。
func CascadeAvailable() bool {
	classifier, err := loadFaceClassifier()
	return classifier != nil && err == nil
}

// DetectFaces 使用 pigo Pico 级联检测人脸。
//
// 级联不可用时返回错误，由调用方决定是否走 DetectFacesHeuristic。
// 这样设计是为了让"降级"成为一个**显式决策**，而不是静默发生——
// 检测结果里的 Method 字段必须能如实反映来源。
func DetectFaces(img image.Image) ([]Region, error) {
	if img == nil {
		return nil, ErrNoImage
	}
	classifier, err := loadFaceClassifier()
	if err != nil {
		return nil, err
	}

	src := pigo.ImgToNRGBA(img)
	bounds := src.Bounds()
	cols, rows := bounds.Dx(), bounds.Dy()
	if cols <= 0 || rows <= 0 {
		return nil, fmt.Errorf("multimodal/vision: empty image bounds %v", bounds)
	}

	pixels := pigo.RgbToGrayscale(src)
	params := pigo.CascadeParams{
		MinSize:     20,
		MaxSize:     minInt(rows, cols),
		ShiftFactor: 0.1,
		ScaleFactor: 1.1,
		ImageParams: pigo.ImageParams{
			Pixels: pixels,
			Rows:   rows,
			Cols:   cols,
			Dim:    cols,
		},
	}

	detections := classifier.RunCascade(params, 0.0)
	detections = classifier.ClusterDetections(detections, 0.2)

	regions := make([]Region, 0, len(detections))
	for _, detection := range detections {
		size := detection.Scale
		region := Region{
			Kind:   KindFace,
			X:      detection.Col - size/2,
			Y:      detection.Row - size/2,
			W:      size,
			H:      size,
			Score:  clamp01(float64(detection.Q)),
			Method: MethodPigoCascade,
		}
		clipped, ok := region.Clip(bounds)
		if !ok {
			continue
		}
		regions = append(regions, clipped)
	}
	return regions, nil
}

// DetectFacesHeuristic 肤色 + 密度启发式的人脸候选检测（**降级路径**）。
//
// 判定依据：YCbCr 空间中肤色像素呈聚集分布。做法是把图像切成网格，
// 统计每块的肤色像素占比，再把相邻的高占比块合并成矩形候选。
//
// 诚实说明：这是**启发式**，不是模型检测。它会把裸露的手臂、木质家具、
// 暖色调墙面也判为候选，因此仅用于"级联不可用时的兜底"，
// 结果 Method 字段固定为 MethodHeuristic，绝不冒充模型输出。
func DetectFacesHeuristic(img image.Image) ([]Region, error) {
	if img == nil {
		return nil, ErrNoImage
	}
	rgba := toRGBA(img)
	bounds := rgba.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("multimodal/vision: empty image bounds %v", bounds)
	}

	const blockSize = 16
	blocksX := (w + blockSize - 1) / blockSize
	blocksY := (h + blockSize - 1) / blockSize

	density := make([]float64, blocksX*blocksY)
	for by := 0; by < blocksY; by++ {
		for bx := 0; bx < blocksX; bx++ {
			skin, total := 0, 0
			for y := by * blockSize; y < minInt((by+1)*blockSize, h); y++ {
				for x := bx * blockSize; x < minInt((bx+1)*blockSize, w); x++ {
					if isSkinTone(rgba.RGBAAt(x, y)) {
						skin++
					}
					total++
				}
			}
			if total > 0 {
				density[by*blocksX+bx] = float64(skin) / float64(total)
			}
		}
	}

	// 阈值 0.5：一块里过半像素是肤色才认为可能是人脸区域。
	const skinThreshold = 0.5
	regions := make([]Region, 0, 8)
	for _, group := range groupAdjacentBlocks(density, blocksX, blocksY, skinThreshold) {
		x0, y0 := group.minX*blockSize, group.minY*blockSize
		x1, y1 := minInt((group.maxX+1)*blockSize, w), minInt((group.maxY+1)*blockSize, h)
		region := Region{
			Kind: KindFace,
			X:    x0, Y: y0, W: x1 - x0, H: y1 - y0,
			Score:  group.score,
			Method: MethodHeuristic,
			Note:   "肤色密度启发式（降级路径，非模型检测）",
		}
		if clipped, ok := region.Clip(bounds); ok {
			regions = append(regions, clipped)
		}
	}
	return regions, nil
}

// isSkinTone 判断像素是否落在常见肤色的 YCbCr 区间。
//
// 该区间是文献中常用的经验范围，对暖色调物体有误报——这也是
// 本函数只用于降级路径的原因之一。
func isSkinTone(c color.RGBA) bool {
	rf, gf, bf := float64(c.R), float64(c.G), float64(c.B)
	y := 0.299*rf + 0.587*gf + 0.114*bf
	cb := 128 - 0.168736*rf - 0.331264*gf + 0.5*bf
	cr := 128 + 0.5*rf - 0.418688*gf - 0.081312*bf
	return y > 50 && cb >= 77 && cb <= 127 && cr >= 133 && cr <= 173
}

// ============================================================================
// 床位牌 / 病历卡 / 二维码：启发式区域检测
// ============================================================================
//
// 诚实声明：以下三类**全部是启发式实现**，不是训练好的检测模型。
// 之所以不引入模型：本机 cgo 不可用，无法编译 ONNX/OpenCV 类依赖；
// 与其冒充模型检测，不如实现规则检测并如实标注 Method=MethodHeuristic。

// DetectBedPlates 床位牌检测（启发式）。
//
// 依据：床位牌通常是白底、含少量大号文字的矩形区域，表现为
// "中等亮度 + 高局部对比度 + 边缘密度集中在边界"。
func DetectBedPlates(img image.Image) ([]Region, error) {
	return detectStructuredRegions(img, KindBedPlate, structuredCriteria{
		MinMeanLuma:    120,
		MinContrast:    45,
		MinEdgeDensity: 0.08,
		MaxEdgeDensity: 0.60,
		Note:           "床位牌启发式：亮底 + 高对比 + 中等边缘密度（非模型检测）",
	})
}

// DetectMedicalCards 病历卡检测（启发式）。
//
// 依据：病历卡文字密度高于床位牌，整体偏白底、边缘密集。
func DetectMedicalCards(img image.Image) ([]Region, error) {
	return detectStructuredRegions(img, KindMedicalCard, structuredCriteria{
		MinMeanLuma:    110,
		MinContrast:    40,
		MinEdgeDensity: 0.14,
		MaxEdgeDensity: 0.85,
		Note:           "病历卡启发式：亮底 + 高对比 + 高边缘密度（非模型检测）",
	})
}

// DetectQRCodes 二维码检测（启发式）。
//
// 依据：二维码是严格黑白二值的方形容错图案，其判据与文字区域不同——
// 表现为"极高对比度 + 二值化后黑白交替频繁 + 边缘密度很高且分布均匀"。
func DetectQRCodes(img image.Image) ([]Region, error) {
	if img == nil {
		return nil, ErrNoImage
	}
	rgba := toRGBA(img)
	bounds := rgba.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("multimodal/vision: empty image bounds %v", bounds)
	}

	const blockSize = 24
	blocksX := (w + blockSize - 1) / blockSize
	blocksY := (h + blockSize - 1) / blockSize

	regions := make([]Region, 0, 4)
	for by := 0; by < blocksY; by++ {
		for bx := 0; bx < blocksX; bx++ {
			x0, y0 := bx*blockSize, by*blockSize
			x1, y1 := minInt(x0+blockSize, w), minInt(y0+blockSize, h)
			contrast := blockContrast(rgba, x0, y0, x1, y1)
			alternation := blockBinaryAlternation(rgba, x0, y0, x1, y1)
			edge := blockEdgeDensity(rgba, x0, y0, x1, y1)

			// 二维码：对比度极高、黑白交替频繁、边缘密集。
			if contrast < 90 || alternation < 0.20 || edge < 0.25 {
				continue
			}
			score := clamp01((contrast/255.0)*0.4 + clamp01(alternation)*0.35 + clamp01(edge)*0.25)
			region := Region{
				Kind: KindQRCode,
				X:    x0, Y: y0, W: x1 - x0, H: y1 - y0,
				Score:  score,
				Method: MethodHeuristic,
				Note:   "二维码启发式：极高对比 + 黑白交替 + 高边缘密度（非模型检测）",
			}
			if clipped, ok := region.Clip(bounds); ok {
				regions = append(regions, clipped)
			}
		}
	}
	return regions, nil
}

// structuredCriteria 结构化区域（牌面/卡片）的判定阈值。
type structuredCriteria struct {
	MinMeanLuma    float64
	MinContrast    float64
	MinEdgeDensity float64
	MaxEdgeDensity float64
	Note           string
}

// detectStructuredRegions 按亮度/对比度/边缘密度三类特征扫描区域。
func detectStructuredRegions(img image.Image, kind RegionKind, criteria structuredCriteria) ([]Region, error) {
	if img == nil {
		return nil, ErrNoImage
	}
	rgba := toRGBA(img)
	bounds := rgba.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("multimodal/vision: empty image bounds %v", bounds)
	}

	const blockSize = 32
	blocksX := (w + blockSize - 1) / blockSize
	blocksY := (h + blockSize - 1) / blockSize

	regions := make([]Region, 0, 8)
	for by := 0; by < blocksY; by++ {
		for bx := 0; bx < blocksX; bx++ {
			x0, y0 := bx*blockSize, by*blockSize
			x1, y1 := minInt(x0+blockSize, w), minInt(y0+blockSize, h)

			meanLuma := blockMeanLuma(rgba, x0, y0, x1, y1)
			contrast := blockContrast(rgba, x0, y0, x1, y1)
			edge := blockEdgeDensity(rgba, x0, y0, x1, y1)

			if meanLuma < criteria.MinMeanLuma ||
				contrast < criteria.MinContrast ||
				edge < criteria.MinEdgeDensity ||
				edge > criteria.MaxEdgeDensity {
				continue
			}
			score := clamp01(0.35*(contrast/255.0) + 0.45*clamp01(edge) + 0.20*(meanLuma/255.0))
			region := Region{
				Kind: kind,
				X:    x0, Y: y0, W: x1 - x0, H: y1 - y0,
				Score:  score,
				Method: MethodHeuristic,
				Note:   criteria.Note,
			}
			if clipped, ok := region.Clip(bounds); ok {
				regions = append(regions, clipped)
			}
		}
	}
	return regions, nil
}

// DetectSensitiveRegions 聚合所有敏感区域检测。
//
// 人脸优先走 pigo 真实模型；级联不可用时**显式降级**并在结果中标注，
// 同时把降级原因写入 Notes 供审计。
func DetectSensitiveRegions(img image.Image) (*DetectResult, error) {
	if img == nil {
		return nil, ErrNoImage
	}
	start := time.Now()
	result := &DetectResult{}

	var notes []string
	var methods []DetectionMethod

	// --- 人脸 ---
	faces, faceErr := DetectFaces(img)
	if faceErr != nil {
		// 显式降级，并如实记录原因。
		heuristic, heuristicErr := DetectFacesHeuristic(img)
		if heuristicErr == nil {
			faces = heuristic
			notes = append(notes, "人脸检测降级为肤色启发式："+faceErr.Error())
			result.Degraded = true
			methods = append(methods, MethodHeuristic)
		} else {
			notes = append(notes, "人脸检测不可用："+faceErr.Error())
			result.Degraded = true
		}
	} else {
		methods = append(methods, MethodPigoCascade)
	}
	result.Regions = append(result.Regions, faces...)

	// --- 牌面 / 卡片 / 二维码（均为启发式）---
	heuristicDetectors := []func(image.Image) ([]Region, error){
		DetectBedPlates, DetectMedicalCards, DetectQRCodes,
	}
	for _, detect := range heuristicDetectors {
		regions, err := detect(img)
		if err != nil {
			notes = append(notes, "启发式检测失败："+err.Error())
			continue
		}
		result.Regions = append(result.Regions, regions...)
	}
	methods = append(methods, MethodHeuristic)

	result.Methods = dedupeMethods(methods)
	result.Notes = notes
	result.Elapsed = time.Since(start)
	return result, nil
}

// ============================================================================
// 区域统计辅助
// ============================================================================

// blockGroup 一组相邻的高分块。
type blockGroup struct {
	minX, minY, maxX, maxY int
	score                  float64
}

// groupAdjacentBlocks 把超过阈值的相邻块合并为连通区域（4 邻域）。
func groupAdjacentBlocks(density []float64, blocksX, blocksY int, threshold float64) []blockGroup {
	visited := make([]bool, len(density))
	var groups []blockGroup

	for by := 0; by < blocksY; by++ {
		for bx := 0; bx < blocksX; bx++ {
			index := by*blocksX + bx
			if visited[index] || density[index] < threshold {
				continue
			}

			// 广度优先扩张。
			group := blockGroup{minX: bx, minY: by, maxX: bx, maxY: by}
			queue := []int{index}
			visited[index] = true
			scoreSum, count := 0.0, 0

			for len(queue) > 0 {
				current := queue[0]
				queue = queue[1:]
				cx, cy := current%blocksX, current/blocksX
				scoreSum += density[current]
				count++

				group.minX = minInt(group.minX, cx)
				group.maxX = maxInt(group.maxX, cx)
				group.minY = minInt(group.minY, cy)
				group.maxY = maxInt(group.maxY, cy)

				for _, offset := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
					nx, ny := cx+offset[0], cy+offset[1]
					if nx < 0 || ny < 0 || nx >= blocksX || ny >= blocksY {
						continue
					}
					neighbor := ny*blocksX + nx
					if visited[neighbor] || density[neighbor] < threshold {
						continue
					}
					visited[neighbor] = true
					queue = append(queue, neighbor)
				}
			}
			if count > 0 {
				group.score = clamp01(scoreSum / float64(count))
			}
			groups = append(groups, group)
		}
	}
	return groups
}

// blockMeanLuma 计算块内平均亮度。
func blockMeanLuma(img *image.RGBA, x0, y0, x1, y1 int) float64 {
	sum, count := 0.0, 0
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			r, g, b, _ := img.RGBAAt(x, y).RGBA()
			sum += 0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(b>>8)
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

// blockContrast 计算块内亮度标准差。
func blockContrast(img *image.RGBA, x0, y0, x1, y1 int) float64 {
	values := make([]float64, 0, (x1-x0)*(y1-y0))
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			r, g, b, _ := img.RGBAAt(x, y).RGBA()
			values = append(values, 0.299*float64(r>>8)+0.587*float64(g>>8)+0.114*float64(b>>8))
		}
	}
	if len(values) == 0 {
		return 0
	}
	mean := 0.0
	for _, v := range values {
		mean += v
	}
	mean /= float64(len(values))
	variance := 0.0
	for _, v := range values {
		variance += (v - mean) * (v - mean)
	}
	return math.Sqrt(variance / float64(len(values)))
}

// blockEdgeDensity 计算块内边缘像素占比（Sobel 梯度幅值超阈值即为边缘）。
func blockEdgeDensity(img *image.RGBA, x0, y0, x1, y1 int) float64 {
	if x1-x0 < 3 || y1-y0 < 3 {
		return 0
	}
	edge, total := 0, 0
	for y := y0 + 1; y < y1-1; y++ {
		for x := x0 + 1; x < x1-1; x++ {
			gx := luma(img, x+1, y) - luma(img, x-1, y)
			gy := luma(img, x, y+1) - luma(img, x, y-1)
			if math.Abs(gx)+math.Abs(gy) > 60 {
				edge++
			}
			total++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(edge) / float64(total)
}

// blockBinaryAlternation 计算块内"二值化后黑白交替"的频率。
//
// 这是区分二维码与普通文字区域的关键特征：二维码在任意扫描方向上
// 都会频繁地在黑白之间跳变，且跳变间隔近似等距。
func blockBinaryAlternation(img *image.RGBA, x0, y0, x1, y1 int) float64 {
	width, height := x1-x0, y1-y0
	if width < 4 || height < 4 {
		return 0
	}
	// 以块内均值作为二值化阈值。
	threshold := blockMeanLuma(img, x0, y0, x1, y1)

	switches, comparisons := 0, 0
	for y := y0; y < y1; y++ {
		previous := -1
		for x := x0; x < x1; x++ {
			current := 0
			if luma(img, x, y) >= threshold {
				current = 1
			}
			if previous >= 0 {
				comparisons++
				if current != previous {
					switches++
				}
			}
			previous = current
		}
	}
	if comparisons == 0 {
		return 0
	}
	return float64(switches) / float64(comparisons)
}

// luma 取单像素亮度（0~255）。
func luma(img *image.RGBA, x, y int) float64 {
	r, g, b, _ := img.RGBAAt(x, y).RGBA()
	return 0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(b>>8)
}

// dedupeMethods 去除重复并稳定排序（模型在前，启发式在后）。
func dedupeMethods(methods []DetectionMethod) []DetectionMethod {
	seen := make(map[DetectionMethod]struct{}, len(methods))
	out := make([]DetectionMethod, 0, len(methods))
	for _, method := range methods {
		if _, ok := seen[method]; ok {
			continue
		}
		seen[method] = struct{}{}
		out = append(out, method)
	}
	return out
}

// clamp01 把数值夹取到 [0,1]。
func clamp01(value float64) float64 {
	if math.IsNaN(value) {
		return 0
	}
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
