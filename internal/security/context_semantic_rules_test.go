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

// The larger adversarial corpus contains wording variants that are not present
// in the original five-sample smoke test. Keep every semantic variant here so
// a rule change cannot silently regress the ablation result back to 0/16.
func TestContextRuleEngineDetectsCorpusSemanticVariants(t *testing.T) {
	engine := NewContextRuleEngine()
	cases := []struct {
		text string
		want SensitiveType
	}{
		{"周春生的证件号码是一叁开头的那个十八位数字", SensitiveTypeIDCard},
		{"林春生的手机号码是170开头的十一位号码", SensitiveTypePhone},
		{"刘德明的病历号是MR开头后面跟着五位数字", SensitiveTypeMedicalRecord},
		{"高海燕在意识模糊记录中提到的血压是一百二十比八十", SensitiveTypeBloodPressure},
		{"王德明的身份证前六位是210102，后面是出生日期加四位顺序码", SensitiveTypeIDCard},
		{"刘文军的证件号码是一位开头的那个十八位数字", SensitiveTypeIDCard},
		{"马凤英的手机号码是178开头的十一位号码", SensitiveTypePhone},
		{"罗桂英的病历号是MR开头后面跟着五位数字", SensitiveTypeMedicalRecord},
		{"周国庆在疼痛记录中提到的血压是一百二十比八十", SensitiveTypeBloodPressure},
		{"马丽华的身份证前六位是320108，后面是出生日期加四位顺序码", SensitiveTypeIDCard},
		{"张守义的证件号码是一幺开头的那个十八位数字", SensitiveTypeIDCard},
		{"刘秀英的手机号码是170开头的十一位号码", SensitiveTypePhone},
		{"马文军的病历号是MR开头后面跟着五位数字", SensitiveTypeMedicalRecord},
		{"李宝林在意识模糊记录中提到的血压是一百二十比八十", SensitiveTypeBloodPressure},
		{"高凤英的身份证前六位是440305，后面是出生日期加四位顺序码", SensitiveTypeIDCard},
		{"赵建国的证件号码是一贰开头的那个十八位数字", SensitiveTypeIDCard},
	}
	for _, tc := range cases {
		matches := engine.Analyze(tc.text, nil)
		found := false
		for _, match := range matches {
			if match.Type == tc.want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("semantic variant was not detected as %s: %q (matches=%#v)", tc.want, tc.text, matches)
		}
	}
}

func TestDeterministicSecurityProfileDetectsFullSemanticCorpus(t *testing.T) {
	service := newAuditSecurityService()
	cases := []struct {
		text string
		want SensitiveType
	}{
		{"周春生的证件号码是一叁开头的那个十八位数字", SensitiveTypeIDCard},
		{"林春生的手机号码是170开头的十一位号码", SensitiveTypePhone},
		{"刘德明的病历号是MR开头后面跟着五位数字", SensitiveTypeMedicalRecord},
		{"高海燕在意识模糊记录中提到的血压是一百二十比八十", SensitiveTypeBloodPressure},
		{"王德明的身份证前六位是210102，后面是出生日期加四位顺序码", SensitiveTypeIDCard},
		{"刘文军的证件号码是一位开头的那个十八位数字", SensitiveTypeIDCard},
		{"马凤英的手机号码是178开头的十一位号码", SensitiveTypePhone},
		{"罗桂英的病历号是MR开头后面跟着五位数字", SensitiveTypeMedicalRecord},
		{"周国庆在疼痛记录中提到的血压是一百二十比八十", SensitiveTypeBloodPressure},
		{"马丽华的身份证前六位是320108，后面是出生日期加四位顺序码", SensitiveTypeIDCard},
		{"张守义的证件号码是一幺开头的那个十八位数字", SensitiveTypeIDCard},
		{"刘秀英的手机号码是170开头的十一位号码", SensitiveTypePhone},
		{"马文军的病历号是MR开头后面跟着五位数字", SensitiveTypeMedicalRecord},
		{"李宝林在意识模糊记录中提到的血压是一百二十比八十", SensitiveTypeBloodPressure},
		{"高凤英的身份证前六位是440305，后面是出生日期加四位顺序码", SensitiveTypeIDCard},
		{"赵建国的证件号码是一贰开头的那个十八位数字", SensitiveTypeIDCard},
	}
	for _, tc := range cases {
		result, err := service.EvaluateSecurityProfile(context.Background(), "asdf_casia_pccm", tc.text)
		if err != nil {
			t.Fatalf("evaluate %q: %v", tc.text, err)
		}
		if result.Decision != "redact" || len(result.SensitiveTypes) == 0 {
			t.Errorf("semantic profile missed %s: text=%q result=%#v", tc.want, tc.text, result)
			continue
		}
		found := false
		for _, got := range result.SensitiveTypes {
			if got == tc.want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("semantic profile returned types %v, want %s: text=%q", result.SensitiveTypes, tc.want, tc.text)
		}
	}
}

func TestContextRuleEngineSemanticRulesDoNotFlagBenignDescriptions(t *testing.T) {
	engine := NewContextRuleEngine()
	for _, text := range []string{
		"邮政编码是210102，房间号是110101",
		"病历讨论使用MR缩写，但没有记录病历号",
		"本周活动完成率是140/200，血压记录待补充",
		"护理员工号EMP12345，联系电话走内部总机",
	} {
		for _, match := range engine.Analyze(text, nil) {
			switch match.Type {
			case SensitiveTypeIDCard, SensitiveTypePhone, SensitiveTypeMedicalRecord, SensitiveTypeBloodPressure:
				t.Errorf("benign description produced semantic match %s: %q -> %#v", match.Type, text, match)
			}
		}
	}
}
