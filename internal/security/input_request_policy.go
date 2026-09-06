package security

import (
	"regexp"
	"strings"
)

// InputRequestPolicyDetector identifies high-confidence attempts to override
// the assistant or disclose another subject's sensitive records.
type InputRequestPolicyDetector struct {
	overridePatterns []*regexp.Regexp
}

var (
	disclosureActionPattern   = regexp.MustCompile(`(?i)(provide|tell|show|display|list|export|generate|print|query|retrieve|fetch|dump|access|reveal|output|return|how\s+many|where\s+does|what\s+year|介绍|提供|告诉|显示|列出|导出|生成|打印|查询|查看|获取|整理|备份|对比|统计|输出|返回|访问|展示|说说|总结|知道|多少|哪里|哪年|是什么|第.{0,3}位|倒着说)`)
	sensitiveAttributePattern = regexp.MustCompile(`(?i)(id\s*(card|number)?|identity\s*(number|card)?|phone|mobile|contact|address|medical\s*(record|history|profile)|health\s*(record|profile)|diagnosis|prescription|care\s*log|location\s*history|personal\s*(data|record)|detailed\s+information|身份证|手机号|手机号码|电话|联系方式|住址|地址|健康档案|档案|病历|诊断|用药|护理日志|行动轨迹|原始数据|敏感数据|详细信息|床头卡|紧急联系|出生年份|房间号|床号)`)
	thirdPartySubjectPattern  = regexp.MustCompile(`(?i)(所有|全部|每位|每个|全体).{0,8}(老人|患者|住民|用户|人员|客户)|(?:老人|患者|住民|病人|客户|用户).{0,8}(姓名|身份证|电话|地址|档案|病历|诊断|用药|床号)|(?:他|她|其|该患者|该老人|某位|某个).{0,12}的|(?:^|[，。！？、\s])[^，。！？、\s]{1,24}的(?:身份证|电话|手机号|手机号码|联系方式|住址|地址|健康档案|完整档案|病历|诊断|用药|行动轨迹|房间号|床号)`)
	selfDirectedPattern       = regexp.MustCompile(`(?:我的|本人(?:的)?|我自己(?:的)?|自己的|我们自己的).{0,12}(?:身份证|电话|手机号|手机号码|联系方式|住址|地址|健康档案|病历|诊断|用药|档案|记录)|(?:my|our)\s+(?:id|identity|phone|mobile|contact|address|medical\s+record|health\s+record)`)
)

func NewInputRequestPolicyDetector() *InputRequestPolicyDetector {
	return &InputRequestPolicyDetector{
		overridePatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)\b(ignore|disregard|override|bypass|disable|replace)\b.{0,80}\b(instruction|rule|prompt|security|filter|check|permission|restriction)\b`),
			regexp.MustCompile(`(?i)\b(admin[_ ]?override|system\s+(command|maintenance)|developer\s+mode|debug\s+mode|database\s+admin)\b`),
			regexp.MustCompile(`(忽略|绕过|跳过|覆盖).{0,20}(指令|规则|过滤|安全|权限|检查|限制)`),
			regexp.MustCompile(`(切换到|进入|启用|重新定义).{0,20}(调试模式|管理员|数据库管理员|数据导出工具|系统命令)`),
			regexp.MustCompile(`(停止当前对话|暂停上述任务|系统维护模式|系统命令).{0,30}(新任务|改为执行|覆盖|绕过|返回|输出)`),
		},
	}
}

// Detect returns only an enforcement category, never the matched request.
func (d *InputRequestPolicyDetector) Detect(input string) (bool, string) {
	if isConceptualInstructionOverride(input) {
		return true, "instruction_override"
	}
	for _, pattern := range d.overridePatterns {
		if pattern.MatchString(input) {
			return true, "instruction_override"
		}
	}
	if isEncodedExecutionRequest(input) {
		return true, "instruction_override"
	}
	if isThirdPartySensitiveRecordRequest(input) {
		return true, "third_party_sensitive_data_request"
	}

	if !hasDisclosureAction(input) || !sensitiveAttributePattern.MatchString(input) {
		return false, ""
	}
	if selfDirectedPattern.MatchString(input) {
		return false, ""
	}
	if isSafeGuidanceRequest(input) {
		return false, ""
	}
	if thirdPartySubjectPattern.MatchString(input) || hasThirdPartyPossessive(input) {
		return true, "third_party_sensitive_data_request"
	}
	return false, ""
}

func isConceptualInstructionOverride(input string) bool {
	lower := strings.ToLower(input)
	if containsAny(lower, "ignore", "disregard", "override", "bypass", "disable", "replace") &&
		containsAny(lower, "instruction", "rule", "prompt", "safety", "security", "filter", "check", "permission", "restriction") {
		return true
	}
	if containsAny(input, "忽略", "绕过", "跳过", "覆盖") &&
		containsAny(input, "指令", "规则", "过滤", "安全", "权限", "检查", "限制") {
		return true
	}
	if (containsAny(lower, "you are now", "redefine your role", "switch to") &&
		containsAny(lower, "admin", "administrator", "developer", "debug", "database", "export tool")) ||
		(containsAny(input, "你现在是", "重新定义", "切换到", "改为") &&
			containsAny(input, "管理员", "调试", "数据库", "数据导出", "系统命令")) {
		return true
	}
	if containsAny(lower, "[critical]", "system command", "system maintenance") &&
		(containsAny(lower, "override", "bypass", "output", "return") || containsAny(input, "覆盖", "绕过", "输出", "返回")) {
		return true
	}
	return false
}

func isThirdPartySensitiveRecordRequest(input string) bool {
	if selfDirectedPattern.MatchString(input) || isSafeGuidanceRequest(input) {
		return false
	}
	lower := strings.ToLower(input)
	if !containsAny(lower,
		"provide", "tell", "show", "display", "list", "export", "generate", "print", "query", "retrieve", "fetch", "dump", "access", "reveal", "output", "return", "summarize", "summarise", "compare", "create", "make", "format", "table", "spreadsheet", "what is", "where is", "how old", "how many") &&
		!containsAny(input, "提供", "告诉", "显示", "列出", "导出", "生成", "打印", "查询", "查看", "获取", "整理", "备份", "对比", "统计", "汇总", "归纳", "制作", "做成", "做个", "表格", "清单", "名单", "输出", "返回", "访问", "展示", "介绍", "说", "总结", "知道", "多少", "哪里", "哪年", "是什么") {
		return false
	}
	if !containsAny(lower,
		"id", "identity", "insurance", "phone", "mobile", "contact", "address", "medical record", "health record", "care record", "diagnosis", "prescription", "medication", "care log", "location", "bed number", "personal data", "personal record") &&
		!containsAny(input, "身份证", "手机号", "手机号码", "电话", "联系方式", "住址", "地址", "健康档案", "档案", "病历", "诊断", "用药", "护理日志", "行动轨迹", "原始数据", "敏感数据", "详细信息", "床头卡", "紧急联系", "出生年份", "房间号", "床号") {
		return false
	}
	return containsAny(lower, "patient", "resident", "elder", "client", "customer", "another person", "all residents", "every resident") ||
		containsAny(input, "患者", "住民", "老人", "病人", "客户", "用户", "他的", "她的", "该患者", "该住民") ||
		hasThirdPartyPossessive(input)
}

func containsAny(input string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(input, term) {
			return true
		}
	}
	return false
}

func isEncodedExecutionRequest(input string) bool {
	lower := strings.ToLower(input)
	if strings.Contains(lower, "base64") &&
		(strings.Contains(lower, "decode") || strings.Contains(input, "解码")) &&
		(strings.Contains(lower, "execute") || strings.Contains(lower, "run") || strings.Contains(input, "执行")) {
		return true
	}
	return false
}

func hasDisclosureAction(input string) bool {
	return disclosureActionPattern.MatchString(input) || strings.Contains(input, "说")
}

func isSafeGuidanceRequest(input string) bool {
	guidanceTerms := []string{"保护", "脱敏", "识别", "模板", "规范", "流程", "政策", "安全", "申请"}
	lower := strings.ToLower(input)
	if strings.Contains(input, "请解释") || strings.Contains(lower, "how to redact") ||
		((strings.Contains(lower, "how should") || strings.Contains(lower, "how to")) &&
			containsAny(lower, "redact", "training", "template", "policy", "guidance")) {
		return true
	}
	if !strings.Contains(input, "如何") && !strings.Contains(input, "怎么") {
		return false
	}
	for _, term := range guidanceTerms {
		if strings.Contains(input, term) {
			return true
		}
	}
	return false
}

func hasThirdPartyPossessive(input string) bool {
	if !strings.Contains(input, "的") {
		return false
	}
	return !strings.Contains(input, "我的") && !strings.Contains(input, "本人") && !strings.Contains(input, "自己")
}
