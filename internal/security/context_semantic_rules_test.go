package security

import (
	"context"
	"testing"
)

func TestContextRuleEngineDetectsSensitiveShapeDescriptions(t *testing.T) {
	engine := NewContextRuleEngine()
	cases := []struct {
		name string
		text string
		want SensitiveType
	}{
		{"id length", "患者的证件号码是一一零一零一开头的那个十八位数字", SensitiveTypeIDCard},
		{"phone length", "他的手机号是138开头的11位号码", SensitiveTypePhone},
		{"medical length", "病历号是MR开头后面跟着八位数字", SensitiveTypeMedicalRecord},
		{"blood pressure words", "血压测量结果是一百二十比八十", SensitiveTypeBloodPressure},
		{"id components", "身份证前六位是110101，后面是出生日期加四位顺序码", SensitiveTypeIDCard},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			matches := engine.Analyze(tc.text, nil)
			for _, match := range matches {
				if match.Type == tc.want {
					if match.Confidence <= 0.5 || match.Start >= match.End {
						t.Fatalf("invalid semantic evidence: %#v", match)
					}
					return
				}
			}
			t.Fatalf("no %s match in %q: %#v", tc.want, tc.text, matches)
		})
	}
}

func TestContextRuleEngineDoesNotTreatPostalCodeAsSemanticIdentity(t *testing.T) {
	engine := NewContextRuleEngine()
	matches := engine.Analyze("邮政编码10101，房间号110101", nil)
	for _, match := range matches {
		if match.Type == SensitiveTypeIDCard {
			t.Fatalf("postal/room numbers should not create semantic identity evidence: %#v", match)
		}
	}
}

func TestDeterministicSecurityProfileDetectsSemanticDescriptionsWithoutLLM(t *testing.T) {
	service := newAuditSecurityService()
	for _, text := range []string{
		"患者的证件号码是一一零一零一开头的那个十八位数字",
		"他的手机号是138开头的11位号码",
		"病历号是MR开头后面跟着八位数字",
		"血压测量结果是一百二十比八十",
		"身份证前六位是110101，后面是出生日期加四位顺序码",
	} {
		result, err := service.EvaluateSecurityProfile(context.Background(), "asdf_casia_pccm", text)
		if err != nil {
			t.Fatalf("evaluate %q: %v", text, err)
		}
		if result.Decision != "redact" || len(result.SensitiveTypes) == 0 {
			t.Fatalf("semantic description was not detected: text=%q result=%#v", text, result)
		}
	}
}
