package security

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const ASDFPipelineVersion = "asdf-normalization-v2"

// ASDFSpanMap maps one changed byte range before and after a normalization
// step. Both coordinate systems are retained so an auditor can translate a
// detector span in either direction without returning the sensitive text.
type ASDFSpanMap struct {
	OriginalStart   int  `json:"original_start"`
	OriginalEnd     int  `json:"original_end"`
	NormalizedStart int  `json:"normalized_start"`
	NormalizedEnd   int  `json:"normalized_end"`
	Bidirectional   bool `json:"bidirectional"`
}

// ASDFNormalizationStep is metadata-only evidence for one applied detector.
type ASDFNormalizationStep struct {
	Sequence     int           `json:"sequence"`
	AttackType   string        `json:"attack_type"`
	Confidence   float64       `json:"confidence"`
	InputSHA256  string        `json:"input_sha256"`
	OutputSHA256 string        `json:"output_sha256"`
	Changed      bool          `json:"changed"`
	SpanMap      []ASDFSpanMap `json:"span_map"`
}

// ASDFAuditResult preserves the legacy ASDF result and adds reproducible,
// non-plaintext normalization evidence.
type ASDFAuditResult struct {
	NormalizedText       string                  `json:"-"`
	IsAdversarial        bool                    `json:"is_adversarial"`
	AttackTypes          []string                `json:"attack_types"`
	Confidence           float64                 `json:"confidence"`
	PipelineVersion      string                  `json:"pipeline_version"`
	OriginalSHA256       string                  `json:"original_sha256"`
	NormalizedSHA256     string                  `json:"normalized_sha256"`
	NormalizationSteps   []ASDFNormalizationStep `json:"normalization_steps"`
	RedetectionPerformed bool                    `json:"redetection_performed"`
	ResidualAttackTypes  []string                `json:"residual_attack_types"`
}

// AdversarialSampleDefenseFramework 对抗样本防御框架
// Adversarial Sample Defense Framework (ASDF)
type AdversarialSampleDefenseFramework struct {
	// 对抗样本检测器
	detectors []ASDFDetector
}

// ASDFDetector 对抗样本检测器接口（ASDF框架专用）
type ASDFDetector interface {
	// Detect 检测是否为对抗样本
	Detect(text string) (isAdversarial bool, confidence float64, attackType string)

	// Normalize 对抗样本归一化（还原为标准形式）
	Normalize(text string) string
}

// NewAdversarialSampleDefenseFramework 创建对抗样本防御框架
func NewAdversarialSampleDefenseFramework() *AdversarialSampleDefenseFramework {
	framework := &AdversarialSampleDefenseFramework{
		detectors: []ASDFDetector{},
	}

	// 注册各类对抗样本检测器
	framework.detectors = append(framework.detectors, &SpaceSeparationDetector{})
	framework.detectors = append(framework.detectors, &SpecialCharDetector{})
	framework.detectors = append(framework.detectors, &HomophoneDetector{})
	framework.detectors = append(framework.detectors, &ChineseNumberDetector{})
	framework.detectors = append(framework.detectors, &Base64Detector{})

	return framework
}

// DefendAndNormalize 防御并归一化对抗样本
func (f *AdversarialSampleDefenseFramework) DefendAndNormalize(text string) (
	normalizedText string,
	isAdversarial bool,
	attackTypes []string,
	confidence float64,
) {
	result := f.DefendAndNormalizeWithAudit(text)
	return result.NormalizedText, result.IsAdversarial, result.AttackTypes, result.Confidence
}

// DefendAndNormalizeWithAudit executes the same legacy normalization path and
// records hashes and byte-range mappings for each applied transformation.
func (f *AdversarialSampleDefenseFramework) DefendAndNormalizeWithAudit(text string) ASDFAuditResult {
	normalizedText := text
	attackTypes := []string{}
	isAdversarial := false
	confidence := 0.0
	steps := make([]ASDFNormalizationStep, 0)
	totalConfidence := 0.0
	detectionCount := 0

	// 依次通过各个检测器
	for _, detector := range f.detectors {
		isAdv, conf, attackType := detector.Detect(normalizedText)
		if isAdv {
			isAdversarial = true
			attackTypes = append(attackTypes, attackType)
			totalConfidence += conf
			detectionCount++

			before := normalizedText
			normalizedText = detector.Normalize(before)
			steps = append(steps, ASDFNormalizationStep{
				Sequence:     len(steps) + 1,
				AttackType:   attackType,
				Confidence:   conf,
				InputSHA256:  asdfTextHash(before),
				OutputSHA256: asdfTextHash(normalizedText),
				Changed:      before != normalizedText,
				SpanMap:      asdfChangedSpanMap(before, normalizedText),
			})
		}
	}

	// 计算平均置信度
	if detectionCount > 0 {
		confidence = totalConfidence / float64(detectionCount)
	}

	residual := make([]string, 0)
	for _, detector := range f.detectors {
		if detected, _, attackType := detector.Detect(normalizedText); detected {
			residual = append(residual, attackType)
		}
	}
	return ASDFAuditResult{
		NormalizedText:       normalizedText,
		IsAdversarial:        isAdversarial,
		AttackTypes:          attackTypes,
		Confidence:           confidence,
		PipelineVersion:      ASDFPipelineVersion,
		OriginalSHA256:       asdfTextHash(text),
		NormalizedSHA256:     asdfTextHash(normalizedText),
		NormalizationSteps:   steps,
		RedetectionPerformed: true,
		ResidualAttackTypes:  residual,
	}
}

func asdfTextHash(text string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(text)))
}

func asdfChangedSpanMap(before, after string) []ASDFSpanMap {
	if before == after {
		return []ASDFSpanMap{}
	}
	prefix := 0
	for prefix < len(before) && prefix < len(after) && before[prefix] == after[prefix] {
		prefix++
	}
	beforeSuffix, afterSuffix := len(before), len(after)
	for beforeSuffix > prefix && afterSuffix > prefix && before[beforeSuffix-1] == after[afterSuffix-1] {
		beforeSuffix--
		afterSuffix--
	}
	return []ASDFSpanMap{{
		OriginalStart: prefix, OriginalEnd: beforeSuffix,
		NormalizedStart: prefix, NormalizedEnd: afterSuffix,
		Bidirectional: true,
	}}
}

// ============================================
// 具体对抗样本检测器实现
// ============================================

// SpaceSeparationDetector 空格分隔检测器
type SpaceSeparationDetector struct{}

const minimumObfuscatedIdentifierDigits = 10

// maximumSingleIdentifierDigits 是「单个」受支持标识符可能达到的最大位数。
//
// 依据：本项目支持的最长标识符是中国银行卡（16~19 位），其后依次是身份证
// 18 位、手机号 11 位。分隔符合一化的用途是把「某一个」被拆散的标识符还原成
// 连续数字，所以候选里的数字位数不可能超过该上限。
//
// 一旦超过上限，候选必然是「多个不同标识符被分隔符连在一起」——这在真实
// 第三方文本里非常常见（例如 `电话/身份证/银行卡` 用 `/` 串成一行）。此时
// 按分隔符合并会把它们糊成一整串数字，令所有带边界约束的检测器同时失配，
// 原本能被检出的值反而被"洗掉"。这类候选必须原样保留，也不应被声明为混淆样本。
//
// 该缺口由第三方语料评测暴露（内部语料每条只有一个被混淆的值，永远测不出来）。
const maximumSingleIdentifierDigits = 19

// plausibleObfuscationCandidate 判定一个「数字 + 分隔符」候选是否可能对应单个标识符。
func plausibleObfuscationCandidate(digits int) bool {
	return digits >= minimumObfuscatedIdentifierDigits && digits <= maximumSingleIdentifierDigits
}

var (
	spaceSeparatedDigitPattern   = regexp.MustCompile(`[0-9]+(?:[\t ]+[0-9]+)+`)
	specialSeparatedDigitPattern = regexp.MustCompile(`[0-9]+(?:[-_./\\|]+[0-9]+)+`)
)

func (d *SpaceSeparationDetector) Detect(text string) (bool, float64, string) {
	// Count separators and digits inside one contiguous candidate. Global
	// counts combine unrelated dates, measurements and prose spaces, causing
	// ordinary nursing records to be classified as attacks.
	for _, candidate := range spaceSeparatedDigitPattern.FindAllString(text, -1) {
		digitCount, separatorCount := digitAndSeparatorCounts(candidate)
		if !plausibleObfuscationCandidate(digitCount) {
			continue
		}
		confidence := float64(separatorCount) / float64(digitCount)
		if confidence > 0.8 {
			confidence = 0.8
		}
		if confidence < 0.5 {
			confidence = 0.5
		}
		return true, confidence, "space_separation"
	}

	return false, 0.0, ""
}

func (d *SpaceSeparationDetector) Normalize(text string) string {
	return spaceSeparatedDigitPattern.ReplaceAllStringFunc(text, func(candidate string) string {
		if digitCount, _ := digitAndSeparatorCounts(candidate); !plausibleObfuscationCandidate(digitCount) {
			return candidate
		}
		return strings.Map(func(r rune) rune {
			if unicode.IsSpace(r) {
				return -1
			}
			return r
		}, candidate)
	})
}

// SpecialCharDetector 特殊字符混淆检测器
type SpecialCharDetector struct{}

func (d *SpecialCharDetector) Detect(text string) (bool, float64, string) {
	for _, candidate := range specialSeparatedDigitPattern.FindAllString(text, -1) {
		if digitCount, _ := digitAndSeparatorCounts(candidate); plausibleObfuscationCandidate(digitCount) {
			return true, 0.6, "special_char_obfuscation"
		}
	}

	return false, 0.0, ""
}

func (d *SpecialCharDetector) Normalize(text string) string {
	return specialSeparatedDigitPattern.ReplaceAllStringFunc(text, func(candidate string) string {
		digitCount, _ := digitAndSeparatorCounts(candidate)
		if !plausibleObfuscationCandidate(digitCount) {
			return candidate
		}
		return strings.Map(func(r rune) rune {
			if unicode.IsDigit(r) {
				return r
			}
			return -1
		}, candidate)
	})
}

func digitAndSeparatorCounts(candidate string) (digits, separators int) {
	for _, r := range candidate {
		if unicode.IsDigit(r) {
			digits++
		} else {
			separators++
		}
	}
	return digits, separators
}

// homophoneConfidence 表示一条同音字映射的置信分层。
//
// 分层的目的是让「扩展覆盖面」与「不误伤正常文本」同时成立：
//   - 高置信字（幺/壹/洞 等）几乎不用于正常叙述，连续 3 个即可判定为混淆；
//   - 低置信字（要/医/留 等）在护理文本中是普通用词，必须要求更长的连续
//     序列（见 minHomophoneRun）才会被当作混淆并参与归一化。
type homophoneConfidence int

const (
	homophoneLow homophoneConfidence = iota
	homophoneHigh
)

// minHomophoneRun 返回该置信层触发「判定 + 归一化」所需的最短连续长度。
func minHomophoneRun(level homophoneConfidence) int {
	if level == homophoneHigh {
		return 3
	}
	return 4
}

// HomophoneDetector 同音字检测器
type HomophoneDetector struct {
	homophoneMap map[rune]rune
	runeLevel    map[rune]homophoneConfidence
}

func (d *HomophoneDetector) Detect(text string) (bool, float64, string) {
	d.ensureHomophoneMaps()

	// A single Chinese numeral is ordinary care language ("一片药",
	// "两次测量"). Require a contiguous run of numeral-like characters --
	// longer when the run contains a low-confidence character -- before
	// treating it as an obfuscated identifier.
	if longestQualifiedHomophoneRun([]rune(text), d.runeLevel) >= 3 {
		confidence := 0.75
		return true, confidence, "homophone_substitution"
	}

	return false, 0.0, ""
}

// Normalize 只重写「满足分层阈值」的连续同音字片段，其余正文原样保留。
//
// 这一点与历史上「全文逐字符替换」的做法不同：当一段文本里既有混淆序列
// （例："幺幺零幺零幺"）又有普通用词（例："一片药"）时，旧实现会把后者也
// 改写成 "1片药"。按 run 限定后，只有达到阈值的片段会被改写。
func (d *HomophoneDetector) Normalize(text string) string {
	d.ensureHomophoneMaps()

	runes := []rune(text)
	result := make([]rune, 0, len(runes))

	for index := 0; index < len(runes); {
		_, mapped := d.runeLevel[runes[index]]
		if !mapped {
			result = append(result, runes[index])
			index++
			continue
		}

		end := index
		for end < len(runes) {
			if _, ok := d.runeLevel[runes[end]]; !ok {
				break
			}
			end++
		}

		if homophoneRunThreshold(runes[index:end], d.runeLevel) <= end-index {
			for _, ch := range runes[index:end] {
				result = append(result, d.homophoneMap[ch])
			}
		} else {
			result = append(result, runes[index:end]...)
		}
		index = end
	}

	return string(result)
}

func (d *HomophoneDetector) ensureHomophoneMaps() {
	if d.homophoneMap != nil && d.runeLevel != nil {
		return
	}
	d.initHomophoneMap()
}

// homophoneRunThreshold 返回该 run 触发归一化所需的最短长度：
// 只要其中包含任一个低置信字符，就要求更长的连续序列。
func homophoneRunThreshold(run []rune, levels map[rune]homophoneConfidence) int {
	threshold := minHomophoneRun(homophoneHigh)
	for _, ch := range run {
		if levels[ch] == homophoneLow {
			return minHomophoneRun(homophoneLow)
		}
	}
	return threshold
}

// longestQualifiedHomophoneRun 返回满足分层阈值的最长连续同音字片段长度。
func longestQualifiedHomophoneRun(runes []rune, levels map[rune]homophoneConfidence) int {
	longest := 0
	for index := 0; index < len(runes); {
		if _, ok := levels[runes[index]]; !ok {
			index++
			continue
		}
		end := index
		for end < len(runes) {
			if _, ok := levels[runes[end]]; !ok {
				break
			}
			end++
		}
		if length := end - index; length >= homophoneRunThreshold(runes[index:end], levels) && length > longest {
			longest = length
		}
		index = end
	}
	return longest
}

// initHomophoneMap 初始化同音字映射。
//
// 高置信层沿用历史数字同音字；低置信层为本次扩展，覆盖在护理文本中常见、
// 但一旦成串出现即强烈指向号码混淆的同音字。低置信层要求更长的连续序列
// （见 minHomophoneRun），因此单独出现时不会改写正文。
func (d *HomophoneDetector) initHomophoneMap() {
	d.homophoneMap = map[rune]rune{
		// 高置信：几乎不用于正常叙述的数字同音字
		'幺': '1', '壹': '1', '一': '1',
		'贰': '2', '二': '2', '两': '2',
		'叁': '3', '三': '3',
		'肆': '4', '四': '4',
		'伍': '5', '五': '5',
		'陆': '6', '六': '6',
		'柒': '7', '七': '7',
		'捌': '8', '八': '8',
		'玖': '9', '九': '9',
		'零': '0', '〇': '0', '洞': '0',

		// 低置信：日常用语中的常见同音字，仅在成串出现时参与归一化
		'要': '1', '医': '1', '依': '1',
		'儿': '2', '鸥': '2',
		'散': '3', '伞': '3',
		'死': '4', '撕': '4', '寺': '4',
		'舞': '5', '捂': '5',
		'留': '6', '溜': '6',
		'妻': '7', '凄': '7',
		'霸': '8', '发': '8',
		'久': '9', '救': '9', '酒': '9',
		'灵': '0',
	}

	d.runeLevel = make(map[rune]homophoneConfidence, len(d.homophoneMap))
	highConfidence := map[rune]struct{}{
		'幺': {}, '壹': {}, '一': {},
		'贰': {}, '二': {}, '两': {},
		'叁': {}, '三': {},
		'肆': {}, '四': {},
		'伍': {}, '五': {},
		'陆': {}, '六': {},
		'柒': {}, '七': {},
		'捌': {}, '八': {},
		'玖': {}, '九': {},
		'零': {}, '〇': {}, '洞': {},
	}
	for ch := range d.homophoneMap {
		if _, high := highConfidence[ch]; high {
			d.runeLevel[ch] = homophoneHigh
			continue
		}
		d.runeLevel[ch] = homophoneLow
	}
}

// ChineseNumberDetector 中文数字检测器
type ChineseNumberDetector struct{}

func (d *ChineseNumberDetector) Detect(text string) (bool, float64, string) {
	// 检测是否包含中文数字
	chineseNumbers := []rune{'零', '一', '二', '三', '四', '五', '六', '七', '八', '九', '十'}
	chineseMap := make(map[rune]rune, len(chineseNumbers))
	for _, ch := range chineseNumbers {
		// 十 is a sequence marker, but mapping it to itself still lets it
		// participate in suspicious runs without changing normalization.
		chineseMap[ch] = ch
	}
	if longestMappedRun(text, chineseMap) >= 3 {
		confidence := 0.7
		return true, confidence, "chinese_number"
	}

	return false, 0.0, ""
}

// longestMappedRun returns the longest contiguous run of characters in the
// supplied map. Contiguity is intentional: isolated numerals in prose should
// remain normal text, while a phone/ID-like sequence is still detected.
func longestMappedRun(text string, mapped map[rune]rune) int {
	longest, current := 0, 0
	for _, ch := range text {
		if _, ok := mapped[ch]; ok {
			current++
			if current > longest {
				longest = current
			}
			continue
		}
		current = 0
	}
	return longest
}

func (d *ChineseNumberDetector) Normalize(text string) string {
	// 将中文数字转换为阿拉伯数字
	chineseToArabic := map[rune]rune{
		'零': '0', '一': '1', '二': '2', '三': '3', '四': '4',
		'五': '5', '六': '6', '七': '7', '八': '8', '九': '9',
	}

	result := []rune(text)
	for i, ch := range result {
		if digit, exists := chineseToArabic[ch]; exists {
			result[i] = digit
		}
	}
	return string(result)
}

// Base64Detector Base64编码检测器
type Base64Detector struct{}

// base64TokenPattern 提取候选 Base64 token。
//
// 下限取 15 个 Base64 正文字符（`=` 填充不计入）：11~12 位数字编码后正好是
// 「15 个正文字符 + 1 个 `=`」，例如 11 位手机号 13812345678 -> MTM4MTIzNDU2Nzg=。
// 此前下限为 16，导致这类形态根本进不了候选，从而漏检（已由评测数据集
// test_data/test_data_100.json 的 bypass_attack 样本 `编码信息：MTM4MTIzNDU2Nzg=` 暴露）。
//
// 边界说明：更短的数值（如 10 位，编码为 14 正文字符 + `==`）仍不在覆盖范围内——
// 继续下调会扩大候选面，需另行评估误报，故本次不做。
// 候选仍要经过 validBase64Token（长度对齐/可解码/UTF-8/无控制字符）与
// isSensitiveDecodedPayload（数字密度或敏感关键词）两级精筛才判定命中。
var base64TokenPattern = regexp.MustCompile(`[A-Za-z0-9+/]{15,}={0,2}`)
var sensitiveDecodedPattern = regexp.MustCompile(`(?:\d[\s-]*){6,}`)

func validBase64Token(token string) ([]byte, bool) {
	if len(token)%4 != 0 {
		return nil, false
	}
	decoded, err := base64.StdEncoding.DecodeString(token)
	if err != nil || len(decoded) < 4 || !utf8.Valid(decoded) {
		return nil, false
	}
	for _, r := range string(decoded) {
		if unicode.IsControl(r) && !unicode.IsSpace(r) {
			return nil, false
		}
	}
	return decoded, true
}

func isSensitiveDecodedPayload(decoded []byte) bool {
	text := string(decoded)
	if sensitiveDecodedPattern.MatchString(text) {
		return true
	}
	for _, keyword := range []string{"身份证", "手机号", "电话", "病历号", "银行卡", "密码", "token", "api_key"} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

func (d *Base64Detector) Detect(text string) (bool, float64, string) {
	for _, token := range base64TokenPattern.FindAllString(text, -1) {
		if decoded, ok := validBase64Token(token); ok && isSensitiveDecodedPayload(decoded) {
			return true, 0.85, "base64_encoding"
		}
	}
	return false, 0.0, ""
}

func (d *Base64Detector) Normalize(text string) string {
	return base64TokenPattern.ReplaceAllStringFunc(text, func(token string) string {
		decoded, ok := validBase64Token(token)
		if !ok || !isSensitiveDecodedPayload(decoded) {
			return token
		}
		return string(decoded)
	})
}

// GetFrameworkDescription 获取框架描述（用于论文和答辩）
func (f *AdversarialSampleDefenseFramework) GetFrameworkDescription() string {
	return `
对抗样本防御框架（ASDF - Adversarial Sample Defense Framework）

核心思想：
将绕过攻击视为"对抗样本"，借鉴AI安全领域的对抗学习思想，构建防御框架。

理论基础：
在AI安全领域，对抗样本是指通过微小扰动使模型误判的输入。
在敏感信息检测中，绕过攻击（空格、同音字等）本质上也是对抗样本。

框架架构：
┌─────────────────────────────────────┐
│         输入文本                     │
└─────────────────────────────────────┘
              ↓
┌─────────────────────────────────────┐
│    对抗样本检测层                    │
│  ├─ 空格分隔检测器                   │
│  ├─ 特殊字符混淆检测器               │
│  ├─ 同音字替换检测器                 │
│  ├─ 中文数字检测器                   │
│  └─ Base64编码检测器                 │
└─────────────────────────────────────┘
              ↓
┌─────────────────────────────────────┐
│    对抗样本归一化层                  │
│  （还原为标准形式）                  │
└─────────────────────────────────────┘
              ↓
┌─────────────────────────────────────┐
│    标准检测层                        │
│  （正则/字典/LLM）                   │
└─────────────────────────────────────┘

防御策略：
1. 检测：识别对抗样本特征
2. 归一化：将对抗样本还原为标准形式
3. 重检测：对归一化后的文本进行标准检测

支持的对抗样本类型：
1. 空格分隔：1 1 0 1 0 1 → 110101
2. 特殊字符：138-1234-5678 → 13812345678
3. 同音字：幺幺零幺零幺 → 110101
4. 中文数字：一一零一零一 → 110101
5. Base64编码：MTEwMTAx → 110101（解码）

设计特点：
1. 方法借鉴：将对抗样本归一化思想用于敏感信息绕过检测，不主张未经检索验证的首次性
2. 框架化：可扩展的检测器架构，易于添加新的对抗样本类型
3. 归一化：不只是检测，还能还原，提升后续检测准确率
4. 多层防御：对抗样本防御 + 标准检测，双重保障

评测边界：
- 代码覆盖空格、特殊字符、同音字、中文数字和Base64等归一化路径
- 防御率、误伤率和提升幅度只引用带数据集哈希与配置指纹的正式评测结果
- 历史运行不能代替当前代码复测

研究价值：
- 研究方向：AI安全与隐私保护的交叉工程验证
- 论文或专利价值需要新颖性检索、对比实验和独立评审，代码不作发表级别承诺
`
}
