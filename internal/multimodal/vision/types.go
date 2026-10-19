// Package vision 提供多模态前端「图像物理剥离」能力：
//
//   - Retinex 低照度增强（SSR / MSR），提升养老场景敏感区域可见性；
//   - 敏感区域检测：人脸（pigo Pico 级联真实模型检测 + 启发式降级）、
//     床位牌 / 病历卡 / 二维码（启发式检测，非模型检测）；
//   - 像素级去身份化：高斯模糊 / 马赛克 / 语义掩码替换。
//
// 诚实性声明（重要）：
//   - DetectFaces 在级联可用时使用 pigo 的 Pico 级联分类器，属于**真实模型检测**；
//     级联不可用或显式要求降级时，退化为**肤色 + 梯度密度启发式**，此时
//     检测结果方法字段会标记为 MethodHeuristic，绝不冒充模型输出。
//   - 床位牌 / 病历卡 / 二维码检测全部是基于区域特征的**启发式实现**，
//     不是训练好的检测模型，检测方法字段固定标记为 MethodHeuristic。
package vision

import (
	"errors"
	"image"
	"time"
)

// DetectionMethod 描述一个检测结果究竟来自真实模型还是启发式规则。
// 该字段用于审计：调用方必须能够区分「模型检测」与「启发式/降级」。
type DetectionMethod string

const (
	// MethodPigoCascade 表示使用 pigo Pico 级联分类器（真实模型检测）。
	MethodPigoCascade DetectionMethod = "pigo_cascade"
	// MethodHeuristic 表示启发式/降级检测（非模型检测）。
	MethodHeuristic DetectionMethod = "heuristic"
)

// RegionKind 敏感区域类别。
type RegionKind string

const (
	// KindFace 人脸区域。
	KindFace RegionKind = "face"
	// KindBedPlate 床位牌区域。
	KindBedPlate RegionKind = "bed_plate"
	// KindMedicalCard 病历卡区域。
	KindMedicalCard RegionKind = "medical_card"
	// KindQRCode 二维码区域。
	KindQRCode RegionKind = "qr_code"
	// KindTextDense 通用高对比度文本密集区域。
	KindTextDense RegionKind = "text_dense"
)

// Region 表示图像中的一个矩形区域（左上角闭、右下角开）。
type Region struct {
	Kind RegionKind `json:"kind"`
	// X, Y 为左上角坐标；W, H 为宽高，单位像素。
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
	// Score 为检测置信度/启发式评分，取值 0~1，仅用于排序与审计，不代表概率校准。
	Score float64 `json:"score"`
	// Method 标明该区域的检测来源（模型 / 启发式）。
	Method DetectionMethod `json:"method"`
	// Note 为人可读的说明（例如降级原因）。
	Note string `json:"note,omitempty"`
}

// Bounds 返回该区域的像素边界。
func (r Region) Bounds() image.Rectangle {
	return image.Rect(r.X, r.Y, r.X+r.W, r.Y+r.H)
}

// Area 返回区域像素数。
func (r Region) Area() int { return r.W * r.H }

// Clip 将区域裁剪到给定图像范围内，返回是否仍然有效（面积大于 0）。
func (r Region) Clip(b image.Rectangle) (Region, bool) {
	x0, y0 := r.X, r.Y
	x1, y1 := r.X+r.W, r.Y+r.H
	if x0 < b.Min.X {
		x0 = b.Min.X
	}
	if y0 < b.Min.Y {
		y0 = b.Min.Y
	}
	if x1 > b.Max.X {
		x1 = b.Max.X
	}
	if y1 > b.Max.Y {
		y1 = b.Max.Y
	}
	if x1 <= x0 || y1 <= y0 {
		return Region{}, false
	}
	r.X, r.Y, r.W, r.H = x0, y0, x1-x0, y1-y0
	return r, true
}

// DetectResult 是一次区域检测的完整结果，便于接口化与审计。
type DetectResult struct {
	Regions []Region `json:"regions"`
	// Methods 汇总本次检测中实际使用过的检测来源。
	Methods []DetectionMethod `json:"methods"`
	// Degraded 为 true 表示至少有一类检测走了降级/启发式路径。
	Degraded bool `json:"degraded"`
	// Notes 记录降级原因等可审计信息。
	Notes []string `json:"notes,omitempty"`
	// Elapsed 为本次检测总耗时（纳秒，调用方可自行换算毫秒）。
	Elapsed time.Duration `json:"elapsed"`
}

// Strategy 去身份化策略。
type Strategy string

const (
	// StrategyGaussianBlur 高斯模糊（可分离卷积实现）。
	StrategyGaussianBlur Strategy = "gaussian_blur"
	// StrategyMosaic 马赛克。
	StrategyMosaic Strategy = "mosaic"
	// StrategySemanticMask 语义掩码替换。
	StrategySemanticMask Strategy = "semantic_mask"
)

// GaussianBlurParams 高斯模糊参数。
type GaussianBlurParams struct {
	// Sigma 为高斯核标准差（像素），<=0 时取默认值 6。
	Sigma float64 `json:"sigma"`
}

// MosaicParams 马赛克参数。
type MosaicParams struct {
	// BlockSize 为马赛克块边长（像素），<=0 时取默认值 12。
	BlockSize int `json:"block_size"`
}

// SemanticMaskParams 语义掩码参数。
type SemanticMaskParams struct {
	// FillMode 为 "mean"（用区域均值色填充）或 "keep_shape"（保留弱化轮廓）。
	FillMode string `json:"fill_mode"`
	// Intensity 为掩码强度 0~1，1 表示完全替换。
	Intensity float64 `json:"intensity"`
}

// Policy 描述「区域类别 → 去身份化策略」的映射。
type Policy struct {
	// Default 为未匹配到具体类别时使用的策略；为空则表示跳过该区域。
	Default Strategy `json:"default"`
	// ByKind 为按区域类别指定的策略，优先级高于 Default。
	ByKind map[RegionKind]Strategy `json:"by_kind,omitempty"`
	// GaussianBlur / Mosaic / SemanticMask 为各策略的参数。
	GaussianBlur GaussianBlurParams `json:"gaussian_blur"`
	Mosaic       MosaicParams       `json:"mosaic"`
	SemanticMask SemanticMaskParams `json:"semantic_mask"`
}

// DefaultPolicy 返回一个保守的默认策略：
// 人脸用高斯模糊、二维码/牌面用马赛克、其余用语义掩码。
func DefaultPolicy() Policy {
	return Policy{
		Default: StrategySemanticMask,
		ByKind: map[RegionKind]Strategy{
			KindFace:        StrategyGaussianBlur,
			KindQRCode:      StrategyMosaic,
			KindBedPlate:    StrategyMosaic,
			KindMedicalCard: StrategyMosaic,
		},
		GaussianBlur: GaussianBlurParams{Sigma: 8},
		Mosaic:       MosaicParams{BlockSize: 14},
		SemanticMask: SemanticMaskParams{FillMode: "mean", Intensity: 1.0},
	}
}

// RegionResult 记录单个区域的处理结果，用于审计。
type RegionResult struct {
	Region   Region        `json:"region"`
	Strategy Strategy      `json:"strategy"`
	Elapsed  time.Duration `json:"elapsed"`
	Changed  bool          `json:"changed"`
}

// DeIdentifyResult 去身份化结果。
type DeIdentifyResult struct {
	Image   image.Image    `json:"-"`
	Applied []RegionResult `json:"applied"`
	// Skipped 记录因策略为空而跳过的区域。
	Skipped []Region `json:"skipped,omitempty"`
	Elapsed time.Duration
}

// ErrNoImage 表示输入图像为空。
var ErrNoImage = errors.New("multimodal/vision: nil image")
