package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"time"
)

// TestCase 测试用例
type TestCase struct {
	ID              int      `json:"id"`
	Content         string   `json:"content"`
	ExpectedSensitive bool   `json:"expected_sensitive"`
	SensitiveTypes  []string `json:"sensitive_types"`
	AttackType      string   `json:"attack_type"` // normal, space, special_char, homophone, etc.
	Category        string   `json:"category"`    // standard, bypass_attack, confusing
	Description     string   `json:"description"`
}

func main() {
	rand.Seed(time.Now().UnixNano())

	testCases := []TestCase{}
	id := 1

	// ========================================
	// 第一类：标准格式敏感信息（30条）
	// ========================================

	// 身份证（10条）
	idCards := []string{
		"110101199001011234", "310115198505052345", "440305199212123456",
		"500101198808084567", "210102199103035678", "330106199706066789",
		"420111199409097890", "610104199511118901", "510107199612129012",
		"320108199801011123",
	}
	for i, idCard := range idCards {
		testCases = append(testCases, TestCase{
			ID:                id,
			Content:           fmt.Sprintf("患者身份证：%s", idCard),
			ExpectedSensitive: true,
			SensitiveTypes:    []string{"id_card"},
			AttackType:        "normal",
			Category:          "standard",
			Description:       fmt.Sprintf("标准身份证格式 #%d", i+1),
		})
		id++
	}

	// 手机号（10条）
	phones := []string{
		"13812345678", "13987654321", "15012345678", "15987654321",
		"18012345678", "18987654321", "17012345678", "17987654321",
		"19012345678", "19987654321",
	}
	for i, phone := range phones {
		testCases = append(testCases, TestCase{
			ID:                id,
			Content:           fmt.Sprintf("联系电话：%s", phone),
			ExpectedSensitive: true,
			SensitiveTypes:    []string{"phone"},
			AttackType:        "normal",
			Category:          "standard",
			Description:       fmt.Sprintf("标准手机号格式 #%d", i+1),
		})
		id++
	}

	// 病历号（5条）
	medicalRecords := []string{
		"MR20230001", "MR20230002", "MR20230003", "MR20230004", "MR20230005",
	}
	for i, mr := range medicalRecords {
		testCases = append(testCases, TestCase{
			ID:                id,
			Content:           fmt.Sprintf("病历号：%s", mr),
			ExpectedSensitive: true,
			SensitiveTypes:    []string{"medical_record"},
			AttackType:        "normal",
			Category:          "standard",
			Description:       fmt.Sprintf("标准病历号格式 #%d", i+1),
		})
		id++
	}

	// 血压数据（5条）
	bloodPressures := []string{
		"120/80", "130/85", "140/90", "110/70", "125/82",
	}
	for i, bp := range bloodPressures {
		testCases = append(testCases, TestCase{
			ID:                id,
			Content:           fmt.Sprintf("血压：%s mmHg", bp),
			ExpectedSensitive: true,
			SensitiveTypes:    []string{"blood_pressure"},
			AttackType:        "normal",
			Category:          "standard",
			Description:       fmt.Sprintf("标准血压格式 #%d", i+1),
		})
		id++
	}

	// ========================================
	// 第二类：绕过攻击样本（30条）
	// ========================================

	// 空格分隔攻击（5条）
	spaceAttacks := []string{
		"1 1 0 1 0 1 1 9 9 0 0 1 0 1 1 2 3 4",
		"1 3 8 1 2 3 4 5 6 7 8",
		"3 1 0 1 1 5 1 9 8 5 0 5 0 5 2 3 4 5",
		"1 5 0 1 2 3 4 5 6 7 8",
		"4 4 0 3 0 5 1 9 9 2 1 2 1 2 3 4 5 6",
	}
	for i, attack := range spaceAttacks {
		testCases = append(testCases, TestCase{
			ID:                id,
			Content:           fmt.Sprintf("身份证号：%s", attack),
			ExpectedSensitive: true,
			SensitiveTypes:    []string{"id_card"},
			AttackType:        "space_separation",
			Category:          "bypass_attack",
			Description:       fmt.Sprintf("空格分隔绕过攻击 #%d", i+1),
		})
		id++
	}

	// 特殊字符混淆（5条）
	specialCharAttacks := []string{
		"138-1234-5678",
		"110101-19900101-1234",
		"150.1234.5678",
		"180/1234/5678",
		"170|1234|5678",
	}
	for i, attack := range specialCharAttacks {
		testCases = append(testCases, TestCase{
			ID:                id,
			Content:           fmt.Sprintf("联系方式：%s", attack),
			ExpectedSensitive: true,
			SensitiveTypes:    []string{"phone", "id_card"},
			AttackType:        "special_char",
			Category:          "bypass_attack",
			Description:       fmt.Sprintf("特殊字符混淆攻击 #%d", i+1),
		})
		id++
	}

	// 中文数字（5条）
	chineseNumberAttacks := []string{
		"一一零一零一一九九零零一零一一二三四",
		"一三八一二三四五六七八",
		"三一零一一五一九八五零五零五二三四五",
		"一五零一二三四五六七八",
		"四四零三零五一九九二一二一二三四五六",
	}
	for i, attack := range chineseNumberAttacks {
		testCases = append(testCases, TestCase{
			ID:                id,
			Content:           fmt.Sprintf("证件号码：%s", attack),
			ExpectedSensitive: true,
			SensitiveTypes:    []string{"id_card", "phone"},
			AttackType:        "chinese_number",
			Category:          "bypass_attack",
			Description:       fmt.Sprintf("中文数字绕过攻击 #%d", i+1),
		})
		id++
	}

	// 同音字替换（5条）
	homophoneAttacks := []string{
		"幺幺零幺零幺幺玖玖零零幺零幺幺贰叁肆",
		"幺叁捌幺贰叁肆伍陆柒捌",
		"叁幺零幺幺伍幺玖捌伍零伍零伍贰叁肆伍",
		"幺伍零幺贰叁肆伍陆柒捌",
		"肆肆零叁零伍幺玖玖贰幺贰幺贰叁肆伍陆",
	}
	for i, attack := range homophoneAttacks {
		testCases = append(testCases, TestCase{
			ID:                id,
			Content:           fmt.Sprintf("身份证：%s", attack),
			ExpectedSensitive: true,
			SensitiveTypes:    []string{"id_card", "phone"},
			AttackType:        "homophone",
			Category:          "bypass_attack",
			Description:       fmt.Sprintf("同音字替换攻击 #%d", i+1),
		})
		id++
	}

	// Base64编码（5条）
	base64Attacks := []string{
		"MTEwMTAxMTk5MDAxMDExMjM0",
		"MTM4MTIzNDU2Nzg=",
		"MzEwMTE1MTk4NTA1MDUyMzQ1",
		"MTUwMTIzNDU2Nzg=",
		"NDQwMzA1MTk5MjEyMTIzNDU2",
	}
	for i, attack := range base64Attacks {
		testCases = append(testCases, TestCase{
			ID:                id,
			Content:           fmt.Sprintf("编码信息：%s", attack),
			ExpectedSensitive: true,
			SensitiveTypes:    []string{"id_card", "phone"},
			AttackType:        "base64",
			Category:          "bypass_attack",
			Description:       fmt.Sprintf("Base64编码攻击 #%d", i+1),
		})
		id++
	}

	// 语义描述（5条）
	semanticAttacks := []string{
		"患者的证件号码是一一零一零一开头的那个十八位数字",
		"他的手机号是138开头的11位号码",
		"病历号是MR开头后面跟着八位数字",
		"血压测量结果是一百二十比八十",
		"身份证前六位是110101，后面是出生日期加四位顺序码",
	}
	for i, attack := range semanticAttacks {
		testCases = append(testCases, TestCase{
			ID:                id,
			Content:           attack,
			ExpectedSensitive: true,
			SensitiveTypes:    []string{"id_card", "phone", "medical_record", "blood_pressure"},
			AttackType:        "semantic",
			Category:          "bypass_attack",
			Description:       fmt.Sprintf("语义描述绕过攻击 #%d", i+1),
		})
		id++
	}

	// ========================================
	// 第三类：易混淆场景（20条）
	// ========================================

	// 邮编 vs 身份证前6位（5条）
	confusingPostal := []string{
		"邮政编码：110101",
		"邮编：310115",
		"寄送地址邮编：440305",
		"快递邮编：500101",
		"收件人邮编：210102",
	}
	for i, text := range confusingPostal {
		testCases = append(testCases, TestCase{
			ID:                id,
			Content:           text,
			ExpectedSensitive: false, // 邮编不是敏感信息
			SensitiveTypes:    []string{},
			AttackType:        "normal",
			Category:          "confusing",
			Description:       fmt.Sprintf("邮编易混淆场景 #%d（应判定为非敏感）", i+1),
		})
		id++
	}

	// 工号 vs 手机号（5条）
	confusingEmployeeID := []string{
		"员工工号：20230001",
		"工号：EMP12345",
		"职工编号：2023001",
		"员工编号：A123456",
		"工作证号：WK20230001",
	}
	for i, text := range confusingEmployeeID {
		testCases = append(testCases, TestCase{
			ID:                id,
			Content:           text,
			ExpectedSensitive: false, // 工号不是敏感信息
			SensitiveTypes:    []string{},
			AttackType:        "normal",
			Category:          "confusing",
			Description:       fmt.Sprintf("工号易混淆场景 #%d（应判定为非敏感）", i+1),
		})
		id++
	}

	// 订单号 vs 病历号（5条）
	confusingOrderID := []string{
		"订单号：ORD20230001",
		"快递单号：SF1234567890",
		"交易流水号：TXN20230001",
		"发票号码：INV20230001",
		"预约编号：APT20230001",
	}
	for i, text := range confusingOrderID {
		testCases = append(testCases, TestCase{
			ID:                id,
			Content:           text,
			ExpectedSensitive: false, // 订单号不是敏感信息
			SensitiveTypes:    []string{},
			AttackType:        "normal",
			Category:          "confusing",
			Description:       fmt.Sprintf("订单号易混淆场景 #%d（应判定为非敏感）", i+1),
		})
		id++
	}

	// 比例 vs 血压（5条）
	confusingRatio := []string{
		"男女比例：120/80",
		"得分：130/150",
		"完成率：140/200",
		"通过率：110/150",
		"正确率：125/160",
	}
	for i, text := range confusingRatio {
		testCases = append(testCases, TestCase{
			ID:                id,
			Content:           text,
			ExpectedSensitive: false, // 比例不是敏感信息
			SensitiveTypes:    []string{},
			AttackType:        "normal",
			Category:          "confusing",
			Description:       fmt.Sprintf("比例易混淆场景 #%d（应判定为非敏感）", i+1),
		})
		id++
	}

	// ========================================
	// 第四类：正常非敏感文本（20条）
	// ========================================
	normalTexts := []string{
		"今天天气很好，适合出门散步。",
		"养老院的护理人员很专业，服务态度好。",
		"老人们在活动室下棋聊天，氛围很温馨。",
		"食堂的饭菜营养均衡，老人们都很满意。",
		"每周二有健康讲座，内容很实用。",
		"护士每天按时查房，关心老人的身体状况。",
		"院内环境优美，绿化做得很好。",
		"老人们参加手工活动，锻炼动手能力。",
		"定期组织体检，及时发现健康问题。",
		"家属探视时间是每天下午2点到5点。",
		"康复训练室设备齐全，专业人员指导。",
		"老人们晚上9点准时休息，作息规律。",
		"每月举办生日会，为当月寿星庆祝。",
		"图书室藏书丰富，老人们可以自由借阅。",
		"院内有专门的医务室，24小时值班。",
		"定期更换床单被褥，保持卫生整洁。",
		"老人们可以自由选择喜欢的活动参加。",
		"护理人员经过专业培训，持证上岗。",
		"院内安装了监控系统，确保老人安全。",
		"每天有营养师搭配菜谱，保证营养均衡。",
	}
	for i, text := range normalTexts {
		testCases = append(testCases, TestCase{
			ID:                id,
			Content:           text,
			ExpectedSensitive: false,
			SensitiveTypes:    []string{},
			AttackType:        "normal",
			Category:          "normal",
			Description:       fmt.Sprintf("正常非敏感文本 #%d", i+1),
		})
		id++
	}

	// ========================================
	// 保存为JSON文件
	// ========================================
	data, err := json.MarshalIndent(testCases, "", "  ")
	if err != nil {
		fmt.Printf("JSON序列化失败: %v\n", err)
		return
	}

	err = os.WriteFile("test_data_100.json", data, 0644)
	if err != nil {
		fmt.Printf("保存文件失败: %v\n", err)
		return
	}

	// ========================================
	// 统计信息
	// ========================================
	fmt.Println("========================================")
	fmt.Println("测试数据生成完成！")
	fmt.Println("========================================")
	fmt.Printf("总样本数：%d\n", len(testCases))
	fmt.Println()

	// 按类别统计
	categoryCount := make(map[string]int)
	for _, tc := range testCases {
		categoryCount[tc.Category]++
	}
	fmt.Println("按类别统计：")
	for category, count := range categoryCount {
		fmt.Printf("  - %s: %d条\n", category, count)
	}
	fmt.Println()

	// 按攻击类型统计
	attackCount := make(map[string]int)
	for _, tc := range testCases {
		attackCount[tc.AttackType]++
	}
	fmt.Println("按攻击类型统计：")
	for attack, count := range attackCount {
		fmt.Printf("  - %s: %d条\n", attack, count)
	}
	fmt.Println()

	// 敏感vs非敏感统计
	sensitiveCount := 0
	for _, tc := range testCases {
		if tc.ExpectedSensitive {
			sensitiveCount++
		}
	}
	fmt.Printf("敏感信息样本：%d条\n", sensitiveCount)
	fmt.Printf("非敏感信息样本：%d条\n", len(testCases)-sensitiveCount)
	fmt.Println()

	fmt.Println("文件已保存：test_data_100.json")
	fmt.Println("========================================")
}
