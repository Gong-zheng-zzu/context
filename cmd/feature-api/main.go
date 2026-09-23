// Command feature-api 提供一个轻量 HTTP 服务，把新增功能暴露给网页测试。
//
// 运行方式：
//
//	go run ./cmd/feature-api
//	浏览器打开 http://localhost:8089/web/feature-playground.html
//
// 为什么不直接加到主服务 cmd/server：主服务依赖 onnxruntime_go，
// 在 CGO 关闭的环境无法编译启动。本服务只依赖各功能包本身
// （均不依赖 CGO），可在任意环境直接运行用于验证。
package main

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/contextkeeper/service/internal/memory/tiered"
	"github.com/contextkeeper/service/internal/multimodal/audio"
	"github.com/contextkeeper/service/internal/multimodal/vision"
	"github.com/contextkeeper/service/internal/security"
	"github.com/contextkeeper/service/internal/storage/asdffile"
)

// ============================================================================
// 冷热分层：进程内维护一个管理器，供网页登记记忆并观察分层过程。
// ============================================================================

type tieredState struct {
	mu       sync.Mutex
	manager  *tiered.Manager
	archiver *asdffile.ArchiveStore
	dir      string
	contents map[string][]byte
	counter  int
}

var state tieredState

// initTiered 初始化冷热分层管理器（限时压缩到亚秒级，便于网页观察）。
func initTiered() error {
	dir, err := os.MkdirTemp("", "feature-api-archive")
	if err != nil {
		return err
	}
	archiveDir := filepath.Join(dir, "cold-archive")
	archiver, err := asdffile.NewArchiveStore(archiveDir)
	if err != nil {
		return err
	}
	policy := tiered.TierPolicy{
		HotMaxAge:                400 * time.Millisecond,
		WarmMaxAge:               400 * time.Millisecond,
		ColdMaxAge:               time.Hour,
		FrozenMaxAge:             time.Hour,
		HotBudgetBytes:           1 << 20,
		WarmBudgetBytes:          1 << 20,
		MinAccessCountToKeepWarm: 100,
	}
	state.archiver = archiver
	state.dir = archiveDir
	state.contents = map[string][]byte{}
	state.manager = tiered.NewManager(policy, archiver, &inlineProvider{})
	return nil
}

// inlineProvider 从 state 取回记忆内容用于归档。
type inlineProvider struct{}

func (p *inlineProvider) Payload(id string) ([]byte, error) {
	state.mu.Lock()
	defer state.mu.Unlock()
	data, ok := state.contents[id]
	if !ok {
		return nil, http.ErrNoLocation
	}
	return data, nil
}

// ============================================================================
// 主函数
// ============================================================================

func main() {
	if err := initTiered(); err != nil {
		log.Fatalf("初始化归档存储失败: %v", err)
	}

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()

	// 静态页面：网页通过 http://localhost:8089/web/xxx.html 访问。
	router.Static("/web", "./web")
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy", "service": "feature-api"})
	})

	api := router.Group("/api")
	{
		api.POST("/security/filter", handleFilter)
		api.POST("/vision/deidentify", handleVision)
		api.POST("/audio/anonymize", handleAudio)
		api.POST("/tiered/memory", handleTieredAdd)
		api.GET("/tiered/state", handleTieredState)
		api.POST("/tiered/cycle", handleTieredCycle)
	}

	log.Println("特性验证服务已启动: http://localhost:8089")
	log.Println("测试页面: http://localhost:8089/web/feature-playground.html")
	if err := router.Run("0.0.0.0:8089"); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}

// ============================================================================
// 1) 九层安全过滤
// ============================================================================

func handleFilter(c *gin.Context) {
	var request struct {
		Text string `json:"text"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体必须是 {\"text\": \"...\"}"})
		return
	}

	result := security.NewNineLayerFilter().Filter(request.Text)

	layers := make([]gin.H, 0, len(result.Layers))
	for _, layer := range result.Layers {
		layers = append(layers, gin.H{
			"id":      fmtLayerID(layer.LayerID),
			"name":    layer.Name,
			"matches": layer.Matches,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"original":     result.Original,
		"filtered":     result.Filtered,
		"totalMatches": result.TotalMatches,
		"blocked":      result.Blocked,
		"elapsed":      result.TotalElapsed.String(),
		"layers":       layers,
	})
}

// fmtLayerID 把层号统一成字符串，避免前端处理数字类型差异。
func fmtLayerID(id interface{}) string {
	switch value := id.(type) {
	case int:
		return itoa(value)
	case int8:
		return itoa(int(value))
	case int16:
		return itoa(int(value))
	case int32:
		return itoa(int(value))
	case int64:
		return itoa(int(value))
	case uint:
		return itoa(int(value))
	case uint8:
		return itoa(int(value))
	case uint16:
		return itoa(int(value))
	case uint32:
		return itoa(int(value))
	case uint64:
		return itoa(int(value))
	case string:
		return value
	default:
		return "?"
	}
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	var buf [24]byte
	pos := len(buf)
	for value > 0 {
		pos--
		buf[pos] = byte('0' + value%10)
		value /= 10
	}
	if negative {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// ============================================================================
// 2) 图像敏感区域检测与脱敏
// ============================================================================

func handleVision(c *gin.Context) {
	img := makeDemoImage(320, 240)

	result, detect, err := vision.DeIdentifyImage(img, vision.DefaultPolicy())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	regions := make([]gin.H, 0, len(detect.Regions))
	for _, region := range detect.Regions {
		regions = append(regions, gin.H{
			"kind":   string(region.Kind),
			"x":      region.X,
			"y":      region.Y,
			"w":      region.W,
			"h":      region.H,
			"score":  region.Score,
			"method": string(region.Method),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"original":     encodePNG(img),
		"deidentified": encodePNG(result.Image),
		"regions":      regions,
		"methods":      detect.Methods,
		"degraded":     detect.Degraded,
		"notes":        detect.Notes,
		"applied":      len(result.Applied),
		"elapsed":      detect.Elapsed.String(),
	})
}

// encodePNG 把图片编码为 data URL，便于网页直接显示。
func encodePNG(img image.Image) string {
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buffer.Bytes())
}

// makeDemoImage 生成演示图：肤色块（模拟人脸）+ 棋盘格（模拟二维码）。
func makeDemoImage(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{120, 120, 120, 255})
		}
	}
	x0, y0 := w/8, h/4
	for y := y0; y < y0+h/3; y++ {
		for x := x0; x < x0+w/4; x++ {
			img.Set(x, y, color.RGBA{220, 170, 140, 255})
		}
	}
	qx, qy := w/2, h/3
	for y := qy; y < qy+h/4; y++ {
		for x := qx; x < qx+w/4; x++ {
			if ((x-qx)/3+(y-qy)/3)%2 == 0 {
				img.Set(x, y, color.RGBA{0, 0, 0, 255})
			} else {
				img.Set(x, y, color.RGBA{255, 255, 255, 255})
			}
		}
	}
	return img
}

// ============================================================================
// 3) 声纹剥离
// ============================================================================

func handleAudio(c *gin.Context) {
	const sampleRate = 16000
	speakerA := audio.SynthesizeVowel(sampleRate, 700, 100, []float64{600, 1100, 2400})
	speakerB := audio.SynthesizeVowel(sampleRate, 700, 300, []float64{900, 2200, 3100})

	segments := []audio.SpeakerSegment{
		{SpeakerID: "A", Label: "说话人A F0≈100Hz", PCM: speakerA, SampleRate: sampleRate},
		{SpeakerID: "A", Label: "说话人A 第二段", PCM: speakerA, SampleRate: sampleRate},
		{SpeakerID: "B", Label: "说话人B F0≈300Hz", PCM: speakerB, SampleRate: sampleRate},
		{SpeakerID: "B", Label: "说话人B 第二段", PCM: speakerB, SampleRate: sampleRate},
	}

	before, after, err := audio.CompareAnonymization(segments, audio.DefaultAnonymizeConfig())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"before": gin.H{
			"sameSpeakerMean":   before.SameSpeakerMean,
			"crossSpeakerMean":  before.CrossSpeakerMean,
			"diagonalDominance": before.DiagonalDominance,
		},
		"after": gin.H{
			"sameSpeakerMean":   after.SameSpeakerMean,
			"crossSpeakerMean":  after.CrossSpeakerMean,
			"diagonalDominance": after.DiagonalDominance,
		},
		"reductionPercent": audio.DominanceReduction(before, after) * 100,
	})
}

// ============================================================================
// 4) 冷热分层 + ASDF 归档
// ============================================================================

func handleTieredAdd(c *gin.Context) {
	var request struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || request.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请提供 {\"content\": \"记忆内容\"}"})
		return
	}

	state.mu.Lock()
	state.counter++
	id := fmtMemoryID(state.counter)
	state.contents[id] = []byte(request.Content)
	err := state.manager.Add(&tiered.MemoryItem{
		ID:             id,
		SizeBytes:      int64(len(request.Content)),
		CreatedAt:      time.Now(),
		LastAccessedAt: time.Now(),
		AccessCount:    1,
	})
	state.mu.Unlock()

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "message": "已登记到热层"})
}

func handleTieredCycle(c *gin.Context) {
	// Manager owns its item mutex. Do not hold state.mu while applying a
	// transition: cold-tier archival calls inlineProvider.Payload, which needs
	// state.mu to read the payload. Holding both locks here would deadlock.
	state.mu.Lock()
	manager := state.manager
	state.mu.Unlock()
	if manager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "tiered manager is not initialized"})
		return
	}
	results := manager.Apply(manager.Evaluate())

	transitions := make([]gin.H, 0, len(results))
	for _, result := range results {
		if result.Applied {
			transitions = append(transitions, gin.H{
				"id":     result.Transition.ItemID,
				"from":   result.Transition.From.String(),
				"to":     result.Transition.To.String(),
				"reason": result.Transition.Reason,
			})
		} else {
			transitions = append(transitions, gin.H{
				"id":    result.Transition.ItemID,
				"error": result.Error,
			})
		}
	}
	c.JSON(http.StatusOK, gin.H{"transitions": transitions, "count": len(transitions)})
}

func handleTieredState(c *gin.Context) {
	state.mu.Lock()
	defer state.mu.Unlock()

	items := make([]gin.H, 0, state.counter)
	for i := 1; i <= state.counter; i++ {
		id := fmtMemoryID(i)
		item, ok := state.manager.Item(id)
		if !ok {
			continue
		}
		entry := gin.H{
			"id":          id,
			"tier":        item.Tier.String(),
			"accessCount": item.AccessCount,
			"sizeBytes":   item.SizeBytes,
			"idle":        time.Since(item.LastAccessedAt).String(),
			"content":     string(state.contents[id]),
		}
		// 冷层条目额外读出归档内容，证明数据真的落盘了。
		if item.Tier == tiered.TierCold {
			if archived, err := state.archiver.Restore(id); err == nil {
				entry["archived"] = string(archived)
				entry["archivedOK"] = string(archived) == string(state.contents[id])
			} else {
				entry["archived"] = ""
				entry["archivedOK"] = false
			}
		}
		items = append(items, entry)
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "archiveDir": state.dir})
}

func fmtMemoryID(n int) string {
	return "记忆-" + itoa(n)
}
