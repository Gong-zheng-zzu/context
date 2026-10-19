package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/contextkeeper/service/internal/multimodal/audio"
	"github.com/contextkeeper/service/internal/multimodal/vision"
	"github.com/gin-gonic/gin"
)

const (
	// The image handler deliberately has a small in-memory ceiling. It never
	// calls ParseMultipartForm, which may spill an upload to a temp directory.
	maxMultimodalImageBytes  int64 = 5 << 20
	maxMultimodalImagePixels       = 8_000_000
	maxPCMBytes              int64 = 512 << 10
	maxPCMDurationSeconds          = 5
	minPCMSampleRate               = 8_000
	maxPCMSampleRate               = 48_000
)

var supportedMultimodalImageTypes = map[string]struct{}{
	"image/jpeg": {},
	"image/png":  {},
}

// PCMPrivacyAnalysisRequest accepts one in-memory, mono, little-endian PCM16
// signal. The API intentionally does not accept file paths, WAV/MP3 uploads,
// speaker IDs, or arbitrary codec declarations.
type PCMPrivacyAnalysisRequest struct {
	PCMBase64  string `json:"pcm_base64" binding:"required"`
	SampleRate int    `json:"sample_rate" binding:"required"`
	Encoding   string `json:"encoding" binding:"required"`
}

// RegisterCompetitionMultimodalRoutes mounts privacy processing only below
// the authenticated competition API group. The handlers also verify identity
// so moving a route cannot accidentally make processing public.
func (h *Handler) RegisterCompetitionMultimodalRoutes(group *gin.RouterGroup) {
	multimodal := group.Group("/v1/multimodal")
	multimodal.POST("/image/deidentify", h.HandleImageDeIdentification)
	multimodal.POST("/audio/privacy-analysis", h.HandlePCMPrivacyAnalysis)
}

// HandleImageDeIdentification processes a supplied JPEG or PNG in memory and
// returns only the de-identified derivative. The route must be registered in
// the JWT-protected API group; this method also checks the injected identity so
// it remains safe when embedded in a different router.
func (h *Handler) HandleImageDeIdentification(c *gin.Context) {
	const stage = "image_deidentification"
	if _, ok := getAuthenticatedUserID(c); !ok {
		return
	}

	started := time.Now()
	payload, declaredType, err := readMultimodalImageUpload(c)
	if err != nil {
		writeMultimodalError(c, stage, http.StatusBadRequest, err)
		return
	}
	sniffedType := http.DetectContentType(payload)
	if _, ok := supportedMultimodalImageTypes[sniffedType]; !ok {
		writeMultimodalError(c, stage, http.StatusUnsupportedMediaType, fmt.Errorf("unsupported image content type %q", sniffedType))
		return
	}
	if declaredType != "" && declaredType != sniffedType {
		writeMultimodalError(c, stage, http.StatusUnsupportedMediaType, fmt.Errorf("declared image content type does not match decoded content"))
		return
	}

	config, _, err := image.DecodeConfig(bytes.NewReader(payload))
	if err != nil {
		writeMultimodalError(c, stage, http.StatusBadRequest, fmt.Errorf("invalid image: %w", err))
		return
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > 4096 || config.Height > 4096 || config.Width*config.Height > maxMultimodalImagePixels {
		writeMultimodalError(c, stage, http.StatusBadRequest, fmt.Errorf("image dimensions exceed the in-memory processing limit"))
		return
	}

	img, _, err := image.Decode(bytes.NewReader(payload))
	if err != nil {
		writeMultimodalError(c, stage, http.StatusBadRequest, fmt.Errorf("decode image: %w", err))
		return
	}
	result, detection, err := vision.DeIdentifyImage(img, vision.DefaultPolicy())
	if err != nil {
		writeMultimodalError(c, stage, http.StatusInternalServerError, fmt.Errorf("de-identification failed: %w", err))
		return
	}

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, result.Image); err != nil {
		writeMultimodalError(c, stage, http.StatusInternalServerError, fmt.Errorf("encode de-identified image: %w", err))
		return
	}
	if int64(encoded.Len()) > maxMultimodalImageBytes {
		writeMultimodalError(c, stage, http.StatusRequestEntityTooLarge, fmt.Errorf("de-identified image exceeds response limit"))
		return
	}

	regions := make([]gin.H, 0, len(detection.Regions))
	for _, region := range detection.Regions {
		regions = append(regions, gin.H{
			"kind":   region.Kind,
			"x":      region.X,
			"y":      region.Y,
			"width":  region.W,
			"height": region.H,
			"score":  region.Score,
			"method": region.Method,
		})
	}

	inputHash := sha256.Sum256(payload)
	c.JSON(http.StatusOK, gin.H{
		"success":             true,
		"trace_id":            competitionTraceID(c),
		"stage":               stage,
		"status":              "ok",
		"execution_mode":      "memory_only",
		"input_sha256":        fmt.Sprintf("%x", inputHash[:]),
		"input_content_type":  sniffedType,
		"input_size_bytes":    len(payload),
		"deidentified_image":  "data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes()),
		"output_content_type": "image/png",
		"regions":             regions,
		"detection_methods":   detection.Methods,
		"heuristic_degraded":  detection.Degraded,
		"detector_notes":      detection.Notes,
		"applied_regions":     len(result.Applied),
		"skipped_regions":     len(result.Skipped),
		"latency_ms":          durationMilliseconds(time.Since(started)),
		"original_returned":   false,
		"persisted":           false,
	})
}

// HandlePCMPrivacyAnalysis runs the existing signal-level privacy transform
// against a bounded PCM16 request. It returns metrics only: neither supplied
// PCM nor transformed PCM is retained or returned by this endpoint.
func (h *Handler) HandlePCMPrivacyAnalysis(c *gin.Context) {
	const stage = "pcm_privacy_analysis"
	if _, ok := getAuthenticatedUserID(c); !ok {
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxPCMBytes*2)
	var req PCMPrivacyAnalysisRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeMultimodalError(c, stage, http.StatusBadRequest, fmt.Errorf("invalid PCM request: %w", err))
		return
	}
	if strings.ToLower(strings.TrimSpace(req.Encoding)) != "pcm_s16le" {
		writeMultimodalError(c, stage, http.StatusUnsupportedMediaType, fmt.Errorf("encoding must be pcm_s16le"))
		return
	}
	if req.SampleRate < minPCMSampleRate || req.SampleRate > maxPCMSampleRate {
		writeMultimodalError(c, stage, http.StatusBadRequest, fmt.Errorf("sample_rate must be between %d and %d Hz", minPCMSampleRate, maxPCMSampleRate))
		return
	}

	raw, err := base64.StdEncoding.DecodeString(req.PCMBase64)
	if err != nil {
		writeMultimodalError(c, stage, http.StatusBadRequest, fmt.Errorf("pcm_base64 is not valid base64"))
		return
	}
	if len(raw) == 0 || len(raw)%2 != 0 || int64(len(raw)) > maxPCMBytes {
		writeMultimodalError(c, stage, http.StatusRequestEntityTooLarge, fmt.Errorf("PCM must contain an even number of at most %d bytes", maxPCMBytes))
		return
	}
	sampleCount := len(raw) / 2
	if sampleCount > req.SampleRate*maxPCMDurationSeconds {
		writeMultimodalError(c, stage, http.StatusRequestEntityTooLarge, fmt.Errorf("PCM duration must not exceed %d seconds", maxPCMDurationSeconds))
		return
	}

	pcm := make([]float64, sampleCount)
	for index := range pcm {
		pcm[index] = float64(int16(binary.LittleEndian.Uint16(raw[index*2:]))) / 32768.0
	}

	started := time.Now()
	before := audio.ExtractFeatures(pcm, req.SampleRate, 12)
	transformed, err := audio.Anonymize(pcm, req.SampleRate, audio.DefaultAnonymizeConfig())
	if err != nil {
		writeMultimodalError(c, stage, http.StatusInternalServerError, fmt.Errorf("PCM privacy transform failed: %w", err))
		return
	}
	after := audio.ExtractFeatures(transformed, req.SampleRate, 12)
	inputHash := sha256.Sum256(raw)

	c.JSON(http.StatusOK, gin.H{
		"success":        true,
		"trace_id":       competitionTraceID(c),
		"stage":          stage,
		"status":         "ok",
		"execution_mode": "memory_only",
		"input_sha256":   fmt.Sprintf("%x", inputHash[:]),
		"sample_rate_hz": req.SampleRate,
		"sample_count":   sampleCount,
		"duration_ms":    float64(sampleCount) * 1000 / float64(req.SampleRate),
		"transform": gin.H{
			"algorithm":           "f0_normalization_spectral_envelope",
			"applied":             true,
			"output_retained":     false,
			"output_returned":     false,
			"output_sample_count": len(transformed),
			"target_f0_hz":        audio.DefaultAnonymizeConfig().TargetF0,
			"formant_ratio":       audio.DefaultAnonymizeConfig().FormantRatio,
			"envelope_flattening": audio.DefaultAnonymizeConfig().EnvelopeFlattening,
		},
		"feature_summary": gin.H{
			"before": pcmFeatureSummary(before),
			"after":  pcmFeatureSummary(after),
		},
		"latency_ms":        durationMilliseconds(time.Since(started)),
		"persisted":         false,
		"raw_pcm_returned":  false,
		"raw_pcm_persisted": false,
	})
}

func readMultimodalImageUpload(c *gin.Context) ([]byte, string, error) {
	contentType := c.GetHeader("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.EqualFold(mediaType, "multipart/form-data") {
		return nil, "", fmt.Errorf("Content-Type must be multipart/form-data")
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxMultimodalImageBytes+4096)
	reader, err := c.Request.MultipartReader()
	if err != nil {
		return nil, "", fmt.Errorf("invalid multipart request")
	}

	var payload []byte
	var declaredType string
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "", fmt.Errorf("read multipart upload: %w", err)
		}
		if part.FormName() != "image" || part.FileName() == "" {
			_, _ = io.Copy(io.Discard, io.LimitReader(part, 4096))
			continue
		}
		if payload != nil {
			return nil, "", fmt.Errorf("exactly one image upload is required")
		}
		payload, err = io.ReadAll(io.LimitReader(part, maxMultimodalImageBytes+1))
		if err != nil {
			return nil, "", fmt.Errorf("read image upload: %w", err)
		}
		if int64(len(payload)) > maxMultimodalImageBytes {
			return nil, "", fmt.Errorf("image exceeds %d byte limit", maxMultimodalImageBytes)
		}
		declaredType = strings.ToLower(strings.TrimSpace(part.Header.Get("Content-Type")))
	}
	if len(payload) == 0 {
		return nil, "", fmt.Errorf("multipart field image is required")
	}
	if declaredType != "" {
		declaredType, _, err = mime.ParseMediaType(declaredType)
		if err != nil {
			return nil, "", fmt.Errorf("invalid image content type")
		}
		if _, ok := supportedMultimodalImageTypes[declaredType]; !ok {
			return nil, "", fmt.Errorf("unsupported declared image content type %q", declaredType)
		}
	}
	return payload, declaredType, nil
}

func pcmFeatureSummary(features audio.VoiceFeatureVector) gin.H {
	return gin.H{
		"estimated_f0_hz":        features.F0,
		"spectral_centroid_hz":   features.Centroid,
		"detected_formant_count": len(features.Formants),
	}
}

func durationMilliseconds(duration time.Duration) float64 {
	return float64(duration.Microseconds()) / 1000
}

func writeMultimodalError(c *gin.Context, stage string, status int, err error) {
	writeCompetitionError(c, status, stage, err.Error())
}
