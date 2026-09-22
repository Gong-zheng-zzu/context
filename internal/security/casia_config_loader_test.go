package security

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func casiaKeywordsPath() string {
	return filepath.Join("..", "..", "config", "casia_keywords.yaml")
}

// TestCASIAExternalKeywordsMatchBuiltinDefaults 逐项核对 config/casia_keywords.yaml
// 与内置 initContextKeywords 完全一致（向后兼容硬要求）。
func TestCASIAExternalKeywordsMatchBuiltinDefaults(t *testing.T) {
	loaded, err := LoadCASIAConfigFromFile(casiaKeywordsPath())
	if err != nil {
		t.Fatalf("LoadCASIAConfigFromFile error = %v", err)
	}
	builtin := NewContextAwareSensitiveInfoAlgorithm().GetConfiguration()

	if !reflect.DeepEqual(loaded.KeywordWeights, builtin.KeywordWeights) {
		t.Fatalf("external keyword weights differ from builtin defaults:\nexternal=%v\nbuiltin=%v",
			loaded.KeywordWeights, builtin.KeywordWeights)
	}
	if loaded.ContextWindow != builtin.ContextWindow {
		t.Fatalf("context window = %d, want %d", loaded.ContextWindow, builtin.ContextWindow)
	}
	if loaded.DecisionThreshold != builtin.DecisionThreshold {
		t.Fatalf("decision threshold = %v, want %v", loaded.DecisionThreshold, builtin.DecisionThreshold)
	}
	if loaded.Source != CASIAKeywordSourceFile {
		t.Fatalf("source = %q, want %q", loaded.Source, CASIAKeywordSourceFile)
	}
	if !strings.HasSuffix(loaded.Version, "-"+CASIAKeywordSourceFile) {
		t.Fatalf("version %q should reflect file origin", loaded.Version)
	}
}

// TestCASIABuiltinKeywordWeightsTable 表驱动断言内置默认权重的关键条目，
// 防止 initContextKeywords 被无意修改。
func TestCASIABuiltinKeywordWeightsTable(t *testing.T) {
	builtin := NewContextAwareSensitiveInfoAlgorithm().GetConfiguration().KeywordWeights
	cases := []struct {
		sensitiveType string
		keyword       string
		weight        float64
	}{
		{"id_card", "身份证", 1.0},
		{"id_card", "证件号", 0.9},
		{"id_card", "邮编", -0.8},
		{"phone", "手机", 1.0},
		{"phone", "联系方式", 0.8},
		{"phone", "工号", -1.2},
		{"id_card", "订单号", -1.2},
		{"id_card", "编号", -1.2},
		{"medical_record", "病历号", 1.0},
		{"medical_record", "快递单号", -1.2},
		{"blood_pressure", "收缩压", 0.9},
		{"blood_pressure", "分数", -0.7},
		{"bank_card", "银行卡", 1.0},
		{"bank_card", "订单号", -1.2},
		{"ip_address", "IP地址", 1.0},
		{"ip_address", "版本号", -1.2},
		{"passport", "护照号", 1.0},
		{"passport", "工号", -1.2},
		{"email", "邮箱", 1.0},
		{"email", "示例", -0.8},
		{"credit_card", "信用卡", 1.0},
		{"credit_card", "订单号", -1.2},
		// 真实世界易混淆负样本引入的新负向词。
		{"id_card", "发票号", -1.2},
		{"id_card", "合同编号", -1.2},
		{"id_card", "住院号", -1.2},
		{"id_card", "处方号", -1.2},
		{"id_card", "交易号", -1.2},
		{"id_card", "会员号", -1.2},
		{"id_card", "设备序列号", -1.2},
		{"id_card", "单号", -1.2},
		{"id_card", "流水号", -1.2},
		{"id_card", "序列号", -1.2},
		{"phone", "学号", -1.2},
		{"phone", "会员号", -1.2},
		{"phone", "验证码", -1.2},
		{"phone", "员工编号", -1.2},
		{"bank_card", "交易号", -1.2},
		{"bank_card", "发票号", -1.2},
		{"bank_card", "合同编号", -1.2},
		{"bank_card", "设备序列号", -1.2},
		{"bank_card", "单号", -1.2},
		{"bank_card", "序列号", -1.2},
		{"credit_card", "交易号", -1.2},
		{"credit_card", "发票号", -1.2},
		{"credit_card", "设备序列号", -1.2},
		{"credit_card", "单号", -1.2},
		{"credit_card", "序列号", -1.2},
	}
	for _, tc := range cases {
		got, ok := builtin[tc.sensitiveType][tc.keyword]
		if !ok {
			t.Fatalf("builtin missing keyword %q in %q", tc.keyword, tc.sensitiveType)
		}
		if got != tc.weight {
			t.Fatalf("builtin weight %s/%s = %v, want %v", tc.sensitiveType, tc.keyword, got, tc.weight)
		}
	}
}

// TestCASIAExternalConfigOverridesWeights 验证外部配置可覆盖权重并进入证据链。
func TestCASIAExternalConfigOverridesWeights(t *testing.T) {
	path := filepath.Join(t.TempDir(), "casia_custom.yaml")
	content := `version: casia-custom-v2
context_window_bytes: 24
decision_threshold: 0.55
keyword_weights:
  medical_record:
    病历: 5.0
    订单号: -0.7
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	config, err := LoadCASIAConfigFromFile(path)
	if err != nil {
		t.Fatalf("LoadCASIAConfigFromFile error = %v", err)
	}
	if config.Version != "casia-custom-v2-file" {
		t.Fatalf("version = %q, want %q", config.Version, "casia-custom-v2-file")
	}
	if config.ContextWindow != 24 || config.DecisionThreshold != 0.55 {
		t.Fatalf("config = %+v", config)
	}

	algo := NewContextAwareSensitiveInfoAlgorithmWithConfig(config)
	evidence := algo.AnalyzeContext("病历记录：张三", 0, len("病历记录：张三"), "medical_record", 0)
	if evidence.RawWeight != 5.0 {
		t.Fatalf("raw weight = %v, want 5.0", evidence.RawWeight)
	}
	if evidence.ConfigurationVersion != "casia-custom-v2-file" {
		t.Fatalf("evidence configuration version = %q", evidence.ConfigurationVersion)
	}
}

// TestCASIAConfigMissingFileFallsBack 验证文件缺失时回退到内置默认值。
func TestCASIAConfigMissingFileFallsBack(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.yaml")
	if _, err := LoadCASIAConfigFromFile(missing); err == nil {
		t.Fatal("expected error for missing file")
	}

	config := LoadCASIAConfigFromFileOrDefault(missing)
	if config.Source != CASIAKeywordSourceBuiltin {
		t.Fatalf("fallback source = %q, want %q", config.Source, CASIAKeywordSourceBuiltin)
	}
	if config.Version != CASIAConfigVersion {
		t.Fatalf("fallback version = %q, want %q", config.Version, CASIAConfigVersion)
	}
	if len(config.KeywordWeights) == 0 {
		t.Fatal("fallback config should include builtin keyword weights")
	}

	// 回退算法的行为必须与内置默认算法完全一致。
	fallbackAlgo := NewContextAwareSensitiveInfoAlgorithmFromFile(missing)
	defaultAlgo := NewContextAwareSensitiveInfoAlgorithm()
	text := "邮编110101。患者身份证110101199001011234。"
	got := fallbackAlgo.AnalyzeContext(text, 0, len(text), "id_card", 64)
	want := defaultAlgo.AnalyzeContext(text, 0, len(text), "id_card", 64)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fallback behavior differs:\ngot=%+v\nwant=%+v", got, want)
	}
}

// TestCASIAConfigInvalidFileFallsBack 验证非法配置（空表/畸形 YAML/非有限权重）回退。
func TestCASIAConfigInvalidFileFallsBack(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]string{
		"empty_weights.yaml": "keyword_weights: {}\n",
		"malformed.yaml":     "keyword_weights: [unterminated\n",
		"nonfinite.yaml":     "keyword_weights:\n  medical_record:\n    病历: .nan\n",
	}
	for name, content := range cases {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", name, err)
		}
		if _, err := LoadCASIAConfigFromFile(path); err == nil {
			t.Fatalf("%s: expected load error", name)
		}
		config := LoadCASIAConfigFromFileOrDefault(path)
		if config.Source != CASIAKeywordSourceBuiltin || len(config.KeywordWeights) == 0 {
			t.Fatalf("%s: fallback not applied: source=%q", name, config.Source)
		}
	}
}
