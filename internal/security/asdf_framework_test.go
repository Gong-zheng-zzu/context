package security

import (
	"encoding/base64"
	"testing"
)

func TestASDFSeparatedDigitDetectorsIgnoreNormalNursingMeasurements(t *testing.T) {
	testCases := []string{
		"2026-09-16 08:30，血压 120/80 mmHg，SpO2 98%",
		"晨间血糖 6.8 mmol/L，晚间血糖 7.2 mmol/L",
		"护理员记录 20260916，老人步行 1200 米",
	}
	for _, input := range testCases {
		for name, detector := range map[string]ASDFDetector{
			"space":   &SpaceSeparationDetector{},
			"special": &SpecialCharDetector{},
		} {
			if detected, _, _ := detector.Detect(input); detected {
				t.Fatalf("%s detector classified normal nursing text as adversarial: %q", name, input)
			}
			if got := detector.Normalize(input); got != input {
				t.Fatalf("%s detector changed normal nursing text: got %q, want %q", name, got, input)
			}
		}
	}
}

func TestASDFSeparatedDigitDetectorsNormalizeOnlySuspiciousSpan(t *testing.T) {
	testCases := []struct {
		name       string
		detector   ASDFDetector
		input      string
		want       string
		attackType string
	}{
		{
			name:       "space separated identifier",
			detector:   &SpaceSeparationDetector{},
			input:      "护理备注 保留此处空格；身份证 1 1 0 1 0 1 1 9 9 0 0 1 0 1 1 2 3 4，待核验",
			want:       "护理备注 保留此处空格；身份证 110101199001011234，待核验",
			attackType: "space_separation",
		},
		{
			name:       "punctuated phone",
			detector:   &SpecialCharDetector{},
			input:      "记录日期 2026-09-16；联系电话 138-1234-5678，血压 120/80",
			want:       "记录日期 2026-09-16；联系电话 13812345678，血压 120/80",
			attackType: "special_char_obfuscation",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			detected, _, attackType := testCase.detector.Detect(testCase.input)
			if !detected || attackType != testCase.attackType {
				t.Fatalf("Detect() = (%v, %q), want (true, %q)", detected, attackType, testCase.attackType)
			}
			if got := testCase.detector.Normalize(testCase.input); got != testCase.want {
				t.Fatalf("Normalize() = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestASDFChineseNumeralDetectorsRequireSuspiciousSequence(t *testing.T) {
	chinese := &ChineseNumberDetector{}
	for _, input := range []string{"老人今天吃一片药", "两次测量血压", "护理记录第三天复查"} {
		if detected, _, _ := chinese.Detect(input); detected {
			t.Fatalf("ChineseNumberDetector(%q) detected ordinary prose", input)
		}
	}
	for _, input := range []string{"身份证一一零一零一一九九零零零一零一一二三四", "手机号一三八一二三四五六七八"} {
		if detected, _, _ := chinese.Detect(input); !detected {
			t.Fatalf("ChineseNumberDetector(%q) did not detect a numeral sequence", input)
		}
	}
}

func TestASDFHomophoneDetectorRequiresSuspiciousSequence(t *testing.T) {
	detector := &HomophoneDetector{}
	if detected, _, _ := detector.Detect("老人吃了一片药"); detected {
		t.Fatal("HomophoneDetector flagged an isolated numeral word")
	}
	if detected, _, _ := detector.Detect("联系方式幺幺零幺零幺"); !detected {
		t.Fatal("HomophoneDetector missed a contiguous obfuscated sequence")
	}
}

func TestASDFBase64DetectorDecodesEmbeddedToken(t *testing.T) {
	detector := &Base64Detector{}
	payload := base64.StdEncoding.EncodeToString([]byte("身份证110101199001011234"))
	input := "护理备注：" + payload + "，请脱敏"
	if detected, _, attackType := detector.Detect(input); !detected || attackType != "base64_encoding" {
		t.Fatalf("Detect(%q) = detected=%v type=%q, want base64_encoding", input, detected, attackType)
	}
	if got, want := detector.Normalize(input), "护理备注：身份证110101199001011234，请脱敏"; got != want {
		t.Fatalf("Normalize(%q) = %q, want %q", input, got, want)
	}
}

func TestASDFBase64DetectorRejectsInvalidOrShortTokens(t *testing.T) {
	detector := &Base64Detector{}
	for _, input := range []string{"普通护理记录ABCDEF12345678", "无效编码：not-base64-token", "短值：QUJD"} {
		if detected, _, _ := detector.Detect(input); detected {
			t.Fatalf("Detect(%q) unexpectedly classified text as Base64", input)
		}
		if got := detector.Normalize(input); got != input {
			t.Fatalf("Normalize(%q) changed invalid input to %q", input, got)
		}
	}
}

func TestASDFBase64DetectorDoesNotFlagOrdinaryEncodedText(t *testing.T) {
	detector := &Base64Detector{}
	input := base64.StdEncoding.EncodeToString([]byte("ordinary documentation text"))
	if detected, _, _ := detector.Detect(input); detected {
		t.Fatalf("ordinary Base64 text was classified as sensitive obfuscation: %q", input)
	}
	if got := detector.Normalize(input); got != input {
		t.Fatalf("ordinary Base64 text was normalized unexpectedly: %q", got)
	}
}
