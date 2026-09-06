# ASDF框架阈值设置指南

## 问题：阈值设置的权衡

### 阈值过高的问题（修改前）
- ❌ 漏报率高：很多绕过手段无法检测
- ❌ 安全性差：攻击者容易绕过
- ✅ 误报率低：不会误伤正常文本

### 阈值过低的问题（如果设为1）
- ✅ 漏报率低：几乎所有绕过都能检测
- ✅ 安全性好：难以绕过
- ❌ 误报率高：正常文本可能被误判

---

## 各检测器的合理阈值

### 1. 空格检测器 (SpaceSeparationDetector)

#### 场景分析
```
正常文本：
- "我今天买了 8 个苹果" → 数字1个，空格2个
- "价格是 100 元" → 数字3个，空格2个
- "电话：138 1234 5678" → 数字11个，空格2个 ⚠️

攻击文本：
- "1 3 8 1 2 3 4 5 6 7 8" → 数字11个，空格10个 ✓
- "身份证 1 1 0 1 0 1 1 9 9 0 0 1 0 1 1 2 3 4" → 数字18个，空格17个 ✓
```

#### 推荐阈值
```go
// 方案1：保守（推荐）
if digitCount >= 10 && spaceCount >= 2 {
    // 检测：10+个数字且有2+个空格
    // 优点：平衡误报和漏报
    // 缺点：可能漏掉只有1个空格的情况
}

// 方案2：激进（当前）
if digitCount >= 8 && spaceCount >= 1 {
    // 检测：8+个数字且有1+个空格
    // 优点：检测更全面
    // 缺点：可能误报"电话：138 1234 5678"这种正常格式
}

// 方案3：智能（最佳）
if digitCount >= 10 && spaceCount >= 2 {
    return true, confidence, "space_separation"
} else if digitCount >= 8 && spaceCount >= 1 {
    // 额外检查：空格是否在数字之间
    if hasSpaceBetweenDigits(text) {
        return true, 0.5, "space_separation"
    }
}
```

**建议：方案1（保守）**
- 阈值：`digitCount >= 10 && spaceCount >= 2`
- 理由：电话号码11位，身份证18位，都会被检测到

---

### 2. 特殊字符检测器 (SpecialCharDetector)

#### 场景分析
```
正常文本：
- "价格：100元" → 数字3个，特殊字符1个（:）
- "版本号：v1.2.3" → 数字3个，特殊字符2个（:和.）
- "电话：138-1234-5678" → 数字11个，特殊字符3个 ⚠️

攻击文本：
- "138-1234-5678" → 数字11个，特殊字符2个 ✓
- "110101-1990-0101-1234" → 数字18个，特殊字符3个 ✓
```

#### 推荐阈值
```go
// 方案1：保守
if digitCount >= 10 && specialCount >= 2 {
    // 检测：10+个数字且有2+个特殊字符
    // 优点：减少误报
    // 缺点：漏掉"138-12345678"（只有1个-）
}

// 方案2：激进（当前）
if digitCount >= 8 && specialCount >= 1 {
    // 检测：8+个数字且有1+个特殊字符
    // 优点：检测更全面
    // 缺点：可能误报"版本号：v1.2.3"
}

// 方案3：智能（最佳）
if digitCount >= 10 && specialCount >= 1 {
    // 检测：10+个数字且有1+个特殊字符
    // 额外检查：特殊字符是否在数字之间
    if hasSpecialCharBetweenDigits(text) {
        return true, 0.6, "special_char_obfuscation"
    }
}
```

**建议：方案3（智能）**
- 阈值：`digitCount >= 10 && specialCount >= 1`
- 理由：平衡检测率和误报率

---

### 3. 中文数字检测器 (ChineseNumberDetector)

#### 场景分析
```
正常文本：
- "我有三个苹果" → 中文数字1个
- "第一名" → 中文数字1个
- "二零二四年" → 中文数字4个
- "电话一三八一二三四五六七八" → 中文数字11个 ⚠️

攻击文本：
- "6101二52零0503153524" → 中文数字2个 ✓
- "一三八一二三四五六七八" → 中文数字11个 ✓
```

#### 推荐阈值
```go
// 方案1：保守
if chineseCount >= 5 {
    // 检测：5+个中文数字
    // 优点：不误报"二零二四年"
    // 缺点：漏掉"6101二52零0503153524"（只有2个）
}

// 方案2：激进（当前）
if chineseCount >= 1 {
    // 检测：1+个中文数字
    // 优点：检测最全面
    // 缺点：误报率极高（"我有三个苹果"也会触发）
}

// 方案3：智能（最佳）
if chineseCount >= 3 {
    // 检测：3+个中文数字
    // 优点：平衡误报和漏报
    // 缺点：仍会漏掉"6101二52零0503153524"
}

// 方案4：上下文感知（推荐）
if chineseCount >= 1 && hasDigitsNearby(text) {
    // 检测：有中文数字且附近有阿拉伯数字
    // 优点：精准检测混合攻击
    // 缺点：实现复杂
}
```

**建议：方案4（上下文感知）**
- 阈值：`chineseCount >= 1 && hasDigitsNearby(text)`
- 理由：专门检测"混合攻击"（阿拉伯数字+中文数字）

---

### 4. 同音字检测器 (HomophoneDetector)

#### 场景分析
```
正常文本：
- "一二三四五" → 同音字5个
- "零零后" → 同音字2个
- "二手车" → 同音字1个

攻击文本：
- "6101二52零0503153524" → 同音字2个 ✓
- "壹叁捌壹贰叁肆伍陆柒捌" → 同音字11个 ✓
```

#### 推荐阈值
```go
// 方案1：保守
if homophoneCount >= 5 {
    // 检测：5+个同音字
    // 优点：不误报"零零后"
    // 缺点：漏掉"6101二52零0503153524"
}

// 方案2：激进（当前）
if homophoneCount >= 1 {
    // 检测：1+个同音字
    // 优点：检测最全面
    // 缺点：误报"二手车"
}

// 方案3：智能（最佳）
if homophoneCount >= 2 && hasDigitsNearby(text) {
    // 检测：2+个同音字且附近有阿拉伯数字
    // 优点：精准检测混合攻击
    // 缺点：实现复杂
}
```

**建议：方案3（智能）**
- 阈值：`homophoneCount >= 2 && hasDigitsNearby(text)`
- 理由：专门检测"混合攻击"

---

## 最终推荐配置

### 配置1：保守模式（低误报）
```go
// 空格检测
if digitCount >= 10 && spaceCount >= 2 {
    return true, confidence, "space_separation"
}

// 特殊字符检测
if digitCount >= 10 && specialCount >= 2 {
    return true, 0.6, "special_char_obfuscation"
}

// 中文数字检测
if chineseCount >= 5 {
    return true, 0.7, "chinese_number"
}

// 同音字检测
if homophoneCount >= 5 {
    return true, 0.75, "homophone_substitution"
}
```

**适用场景**：
- 用户输入质量高
- 不希望误报影响用户体验
- 可以接受少量漏报

---

### 配置2：平衡模式（推荐）⭐
```go
// 空格检测
if digitCount >= 10 && spaceCount >= 2 {
    return true, confidence, "space_separation"
}

// 特殊字符检测
if digitCount >= 10 && specialCount >= 1 {
    return true, 0.6, "special_char_obfuscation"
}

// 中文数字检测（上下文感知）
if chineseCount >= 3 {
    return true, 0.7, "chinese_number"
} else if chineseCount >= 1 && hasDigitsNearby(text) {
    return true, 0.6, "chinese_number_mixed"
}

// 同音字检测（上下文感知）
if homophoneCount >= 3 {
    return true, 0.75, "homophone_substitution"
} else if homophoneCount >= 1 && hasDigitsNearby(text) {
    return true, 0.65, "homophone_mixed"
}
```

**适用场景**：
- 大多数生产环境
- 平衡安全性和用户体验
- 推荐使用

---

### 配置3：激进模式（高安全）
```go
// 空格检测
if digitCount >= 8 && spaceCount >= 1 {
    return true, confidence, "space_separation"
}

// 特殊字符检测
if digitCount >= 8 && specialCount >= 1 {
    return true, 0.6, "special_char_obfuscation"
}

// 中文数字检测
if chineseCount >= 1 {
    return true, 0.7, "chinese_number"
}

// 同音字检测
if homophoneCount >= 1 {
    return true, 0.75, "homophone_substitution"
}
```

**适用场景**：
- 高安全要求环境（金融、医疗）
- 可以接受较高误报率
- 有人工审核机制

---

## 实现"上下文感知"检测

### hasDigitsNearby 函数实现
```go
// 检查中文数字附近是否有阿拉伯数字
func hasDigitsNearby(text string, chinesePos int, window int) bool {
    runes := []rune(text)
    start := max(0, chinesePos-window)
    end := min(len(runes), chinesePos+window)
    
    for i := start; i < end; i++ {
        if unicode.IsDigit(runes[i]) {
            return true
        }
    }
    return false
}

// 使用示例
func (d *ChineseNumberDetector) Detect(text string) (bool, float64, string) {
    runes := []rune(text)
    chineseCount := 0
    hasMixedAttack := false
    
    for i, ch := range runes {
        if isChineseNumber(ch) {
            chineseCount++
            // 检查前后5个字符内是否有阿拉伯数字
            if hasDigitsNearby(text, i, 5) {
                hasMixedAttack = true
            }
        }
    }
    
    // 纯中文数字攻击
    if chineseCount >= 5 {
        return true, 0.8, "chinese_number"
    }
    
    // 混合攻击（中文+阿拉伯数字）
    if chineseCount >= 1 && hasMixedAttack {
        return true, 0.7, "chinese_number_mixed"
    }
    
    return false, 0.0, ""
}
```

---

## 测试用例覆盖

### 应该检测到的（True Positive）
```
✓ "138-1234-5678"           → 特殊字符检测
✓ "138 1234 5678"           → 空格检测
✓ "一三八一二三四五六七八"   → 中文数字检测
✓ "6101二52零0503153524"    → 混合攻击检测
✓ "1 1 0 1 0 1 1 9 9 0..."  → 空格检测
```

### 不应该检测到的（True Negative）
```
✓ "我有三个苹果"            → 中文数字太少
✓ "二零二四年"              → 中文数字太少
✓ "版本号：v1.2.3"          → 数字太少
✓ "价格：100元"             → 数字太少
✓ "第一名"                  → 中文数字太少
```

### 边界情况
```
? "电话：138 1234 5678"     → 可能误报（取决于阈值）
? "零零后"                  → 可能误报（取决于阈值）
? "二手车"                  → 不应该检测
```

---

## 我的最终建议

**使用"平衡模式"配置**，并实现"上下文感知"检测：

1. **空格检测**：`digitCount >= 10 && spaceCount >= 2`
2. **特殊字符检测**：`digitCount >= 10 && specialCount >= 1`
3. **中文数字检测**：
   - 纯中文：`chineseCount >= 5`
   - 混合攻击：`chineseCount >= 1 && hasDigitsNearby`
4. **同音字检测**：
   - 纯同音字：`homophoneCount >= 5`
   - 混合攻击：`homophoneCount >= 1 && hasDigitsNearby`

这样可以：
- ✅ 检测到你的测试用例（`182-9134 24 99` 和 `6101二52零0503153524`）
- ✅ 不误报正常文本（"我有三个苹果"、"二零二四年"）
- ✅ 平衡安全性和用户体验
