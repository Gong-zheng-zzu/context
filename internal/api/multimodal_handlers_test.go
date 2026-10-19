package api

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"math"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"

	"github.com/contextkeeper/service/internal/middleware"
	"github.com/gin-gonic/gin"
)

func multimodalRouter(authenticated bool) *gin.Engine {
	router := gin.New()
	if authenticated {
		router.Use(func(c *gin.Context) {
			c.Set(CtxKeyUserID, "caregiver_001")
			c.Set("traceId", "multimodal-trace-001")
			c.Next()
		})
	}
	handler := &Handler{}
	router.POST("/image", handler.HandleImageDeIdentification)
	router.POST("/audio", handler.HandlePCMPrivacyAnalysis)
	return router
}

func TestImageDeIdentificationRequiresAuthenticatedIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := multimodalRouter(false)
	request := newImageRequest(t, "image/png", testPNG(t))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestCompetitionMultimodalRouteRegistrationUsesVersionedPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	(&Handler{}).RegisterCompetitionMultimodalRoutes(router.Group("/api"))
	paths := make(map[string]bool)
	for _, route := range router.Routes() {
		paths[route.Method+" "+route.Path] = true
	}
	if !paths["POST /api/v1/multimodal/image/deidentify"] || !paths["POST /api/v1/multimodal/audio/privacy-analysis"] {
		t.Fatalf("competition multimodal routes = %#v", paths)
	}
}

func TestCompetitionMultimodalRoutesRequireJWTAtProductionBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("JWT_SECRET", "multimodal-route-test-secret")
	router := gin.New()
	protected := router.Group("/api")
	protected.Use(middleware.JWTAuth())
	(&Handler{}).RegisterCompetitionMultimodalRoutes(protected)

	unauthenticated := httptest.NewRequest(http.MethodPost, "/api/v1/multimodal/audio/privacy-analysis", nil)
	denied := httptest.NewRecorder()
	router.ServeHTTP(denied, unauthenticated)
	if denied.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d body=%s", denied.Code, denied.Body.String())
	}

	token, err := middleware.GenerateToken("caregiver_001", "competition")
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	authenticated := httptest.NewRequest(http.MethodPost, "/api/v1/multimodal/audio/privacy-analysis", nil)
	authenticated.Header.Set("Authorization", "Bearer "+token)
	allowed := httptest.NewRecorder()
	router.ServeHTTP(allowed, authenticated)
	if allowed.Code != http.StatusBadRequest {
		t.Fatalf("authenticated status=%d body=%s", allowed.Code, allowed.Body.String())
	}
}

func TestImageDeIdentificationProcessesOnlyInMemoryDerivative(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := multimodalRouter(true)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, newImageRequest(t, "image/png", testPNG(t)))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["execution_mode"] != "memory_only" || payload["original_returned"] != false || payload["persisted"] != false {
		t.Fatalf("unsafe execution evidence: %#v", payload)
	}
	imageData, ok := payload["deidentified_image"].(string)
	if !ok || len(imageData) < 64 || imageData[:22] != "data:image/png;base64," {
		t.Fatalf("missing derivative image: %#v", payload["deidentified_image"])
	}
	if _, leaksOriginal := payload["original"]; leaksOriginal {
		t.Fatalf("response must not contain the original image: %s", response.Body.String())
	}
	if payload["trace_id"] != "multimodal-trace-001" {
		t.Fatalf("missing trace evidence: %#v", payload)
	}
}

func TestImageDeIdentificationRejectsMismatchedContentType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := multimodalRouter(true)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, newImageRequest(t, "image/jpeg", testPNG(t)))
	if response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestPCMPrivacyAnalysisReturnsMetricsWithoutPCMAudio(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := multimodalRouter(true)
	payload := map[string]interface{}{
		"encoding":    "pcm_s16le",
		"sample_rate": 16000,
		"pcm_base64":  base64.StdEncoding.EncodeToString(testPCM16LE(16000, 300)),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/audio", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var result map[string]interface{}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["execution_mode"] != "memory_only" || result["raw_pcm_returned"] != false || result["raw_pcm_persisted"] != false {
		t.Fatalf("unsafe audio response: %#v", result)
	}
	transform, ok := result["transform"].(map[string]interface{})
	if !ok || transform["output_retained"] != false || transform["output_returned"] != false {
		t.Fatalf("missing no-retention evidence: %#v", result["transform"])
	}
	if bytes.Contains(response.Body.Bytes(), []byte(payload["pcm_base64"].(string))) {
		t.Fatalf("response must not echo raw PCM")
	}
}

func TestPCMPrivacyAnalysisRejectsCodecAndDurationOutsideLimits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := multimodalRouter(true)
	for _, test := range []struct {
		name     string
		encoding string
		rate     int
		pcm      []byte
	}{
		{name: "unsupported codec", encoding: "wav", rate: 16000, pcm: testPCM16LE(16000, 100)},
		{name: "duration", encoding: "pcm_s16le", rate: 8000, pcm: testPCM16LE(8000, maxPCMDurationSeconds*1000+1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			body, err := json.Marshal(map[string]interface{}{
				"encoding": test.encoding, "sample_rate": test.rate,
				"pcm_base64": base64.StdEncoding.EncodeToString(test.pcm),
			})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/audio", bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusUnsupportedMediaType && response.Code != http.StatusRequestEntityTooLarge {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func newImageRequest(t *testing.T, contentType string, imageBytes []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreatePart(textprotoMIMEHeader("image", "sample.png", contentType))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(imageBytes); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/image", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func textprotoMIMEHeader(field, filename, contentType string) textproto.MIMEHeader {
	return textproto.MIMEHeader{
		"Content-Disposition": {`form-data; name="` + field + `"; filename="` + filename + `"`},
		"Content-Type":        {contentType},
	}
}

func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 7), G: uint8(y * 7), B: 100, A: 255})
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func testPCM16LE(sampleRate, durationMilliseconds int) []byte {
	samples := sampleRate * durationMilliseconds / 1000
	payload := make([]byte, samples*2)
	for index := 0; index < samples; index++ {
		value := int16(math.Sin(2*math.Pi*220*float64(index)/float64(sampleRate)) * 12000)
		binary.LittleEndian.PutUint16(payload[index*2:], uint16(value))
	}
	return payload
}
