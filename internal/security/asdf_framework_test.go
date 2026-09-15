package security

import (
	"encoding/base64"
	"testing"
)

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
