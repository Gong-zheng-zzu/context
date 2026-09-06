# 忆安智护 - 核心算法单元测试演示
# 右键 -> 使用 PowerShell 运行

[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$ErrorActionPreference = "SilentlyContinue"

Write-Host ""
Write-Host "  ============================================================" -ForegroundColor Cyan
Write-Host "   忆安智护 - 核心算法单元测试演示" -ForegroundColor Cyan
Write-Host "   ASDF | ConfidenceScorer | DifferentialPrivacy | MultiLayer" -ForegroundColor Cyan
Write-Host "  ============================================================" -ForegroundColor Cyan
Write-Host ""

Set-Location "d:\context\context-keeper-main"

$tests = @(
    @{ Name = "[1] 敏感信息检测器 (18种PII)"; Pattern = "^TestDetector_"; Color = "White" },
    @{ Name = "[2] ASDF对抗样本防御 (零宽字符/同形字)"; Pattern = "^TestAdversarialDetector"; Color = "Yellow" },
    @{ Name = "[3] AI响应置信度评分器"; Pattern = "^TestConfidenceScorer"; Color = "Green" },
    @{ Name = "[4] 差分隐私保护 (拉普拉斯噪声)"; Pattern = "^TestDifferentialPrivacy|^TestPrivacyBudget"; Color = "Green" },
    @{ Name = "[5] 多层融合检测器"; Pattern = "^TestMultiLayerDetector|^TestLayerPerformance"; Color = "Cyan" },
    @{ Name = "[6] Prompt注入检测器"; Pattern = "^TestPromptInjectionDetector"; Color = "Red" },
    @{ Name = "[7] 词典匹配器"; Pattern = "^TestDictionaryMatcher"; Color = "Magenta" },
    @{ Name = "[8] 上下文规则引擎"; Pattern = "^TestContextRuleEngine"; Color = "Magenta" },
    @{ Name = "[9] 输出过滤器"; Pattern = "^TestOutputFilter"; Color = "DarkYellow" },
    @{ Name = "[10] 模型DoS防护"; Pattern = "^TestModelDosProtection|^TestEstimateTokens"; Color = "Red" },
    @{ Name = "[11] AI安全集成测试"; Pattern = "^TestAISecurity"; Color = "Cyan" }
)

$totalPass = 0
$totalFail = 0

foreach ($test in $tests) {
    Write-Host ""
    Write-Host "  ----------------------------------------------------------" -ForegroundColor DarkGray
    Write-Host "   $($test.Name)" -ForegroundColor $test.Color
    Write-Host "  ----------------------------------------------------------" -ForegroundColor DarkGray

    $output = go test ./internal/security/ -run $test.Pattern -v -count=1 2>&1
    $outputStr = $output -join "`n"

    $passCount = ([regex]::Matches($outputStr, "--- PASS")).Count
    $failCount = ([regex]::Matches($outputStr, "--- FAIL")).Count
    $totalPass += $passCount
    $totalFail += $failCount

    # 显示关键测试数据行
    foreach ($line in $output) {
        $lineStr = "$line"
        # 显示标题框 (Unicode box drawing)
        if ($lineStr -match "box-drawing|crosshatch|input:|expected:|actual:|detected:|reason:|score:|confidence:|PASS.*check|FAIL|safe|blocked|attack|steps|flow|data:|normalized:|triggers:|suppressors:|summary:|types:|warning:|encryption|decryption|store|retrieve|delete|laplace|noise|vector|epsilon|budget|remaining|consumed|similarity|orthogonal|reverse|same|zero.width|homoglyph|structural|statistical|semantic|override|role.play|bypass|exfiltration|sql|xss|normal|base64|url.encoded|unicode|reversed|redact|phone|password|credit.card|aws|token|bearer|apikey|idcard|scoring|level|contradiction|sensitive|leakage|short|quality|reference|data.point|filter|validate|blacklist|path|ip.address|dos|limit|rate|token.limit|concurrent|user|request|performance|integration|scenario|module|step|verify|stat") {
            Write-Host "   $lineStr" -ForegroundColor Gray
        }
    }

    if ($failCount -eq 0 -and $passCount -gt 0) {
        Write-Host "   全部通过 ($passCount 项)" -ForegroundColor Green
    } elseif ($failCount -gt 0) {
        Write-Host "   通过: $passCount  失败: $failCount" -ForegroundColor Yellow
    } else {
        Write-Host "   无匹配测试或已跳过" -ForegroundColor DarkGray
    }
}

Write-Host ""
Write-Host "  ============================================================" -ForegroundColor Cyan
Write-Host "   测试汇总: 通过 $totalPass 项  失败 $totalFail 项" -ForegroundColor $(if($totalFail -eq 0){"Green"}else{"Yellow"})
Write-Host "  ============================================================" -ForegroundColor Cyan
Write-Host ""
Read-Host "按回车退出"
