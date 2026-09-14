package security

import "testing"

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
