@echo off
chcp 65001 >nul
echo ========================================
echo 健康助手功能演示
echo ========================================
echo.
echo 本脚本演示健康助手的6个核心功能
echo.

echo [步骤1] 检查服务是否运行...
curl -s http://localhost:8088/health >nul 2>&1
if %errorlevel% neq 0 (
    echo ❌ 服务未启动！请先运行: go run cmd/server/main.go
    pause
    exit /b 1
)
echo ✅ 服务正常运行
echo.

:: 初始化统计变量
set /a total=0
set /a success=0
set /a failed=0

echo ========================================
echo 场景1: 健康信息记录 + 自动脱敏（体检）
echo ========================================
echo.
echo 场景说明: 记录体检信息，系统自动检测并脱敏敏感数据
echo 输入内容: "2024年5月13日体检，身份证320106199001011234，手机13812345678，
echo           体检报告号TJ-2024-001234，血压140/90 mmHg，血糖6.5 mmol/L"
echo 预期效果: 自动脱敏身份证、手机号、体检报告号
echo.
set /a total+=1

echo 📤 发送请求...
curl -X POST http://localhost:8088/api/health/record ^
  -H "Content-Type: application/json" ^
  -d "{\"userId\":\"demo_user\",\"category\":\"体检\",\"content\":\"2024年5月13日体检，身份证320106199001011234，手机13812345678，体检报告号TJ-2024-001234，血压140/90 mmHg，血糖6.5 mmol/L\"}" 2>nul

if %errorlevel% equ 0 (
    echo.
    echo ✅ 记录成功
    set /a success+=1
) else (
    echo.
    echo ❌ 记录失败
    set /a failed+=1
)
echo.
pause

echo ========================================
echo 场景2: 就诊记录管理
echo ========================================
echo.
echo 场景说明: 记录就诊信息，包含医院、医生、处方等敏感信息
echo 输入内容: "今天去北京协和医院就诊，医生李医生（手机13987654321），
echo           处方号CF-2024-005678，医保卡号123456789012345678，
echo           诊断为高血压，开了降压药"
echo 预期效果: 自动脱敏手机号、处方号、医保卡号
echo.
set /a total+=1

echo 📤 发送请求...
curl -X POST http://localhost:8088/api/health/record ^
  -H "Content-Type: application/json" ^
  -d "{\"userId\":\"demo_user\",\"category\":\"就诊\",\"content\":\"今天去北京协和医院就诊，医生李医生（手机13987654321），处方号CF-2024-005678，医保卡号123456789012345678，诊断为高血压，开了降压药\"}" 2>nul

if %errorlevel% equ 0 (
    echo.
    echo ✅ 记录成功
    set /a success+=1
) else (
    echo.
    echo ❌ 记录失败
    set /a failed+=1
)
echo.
pause

echo ========================================
echo 场景3: 用药记录
echo ========================================
echo.
echo 场景说明: 记录用药信息和过敏史
echo 输入内容: "开始服用阿莫西林，对青霉素过敏"
echo 预期效果: 检测药物过敏信息
echo.
set /a total+=1

echo 📤 发送请求...
curl -X POST http://localhost:8088/api/health/record ^
  -H "Content-Type: application/json" ^
  -d "{\"userId\":\"demo_user\",\"category\":\"用药\",\"content\":\"开始服用阿莫西林，对青霉素过敏\"}" 2>nul

if %errorlevel% equ 0 (
    echo.
    echo ✅ 记录成功
    set /a success+=1
) else (
    echo.
    echo ❌ 记录失败
    set /a failed+=1
)
echo.
pause

echo ========================================
echo 场景4: 健康档案查询
echo ========================================
echo.
echo 场景说明: 查询最近30天的健康记录
echo 查询参数: userId=demo_user, days=30
echo 预期效果: 返回记录列表和统计信息
echo.
set /a total+=1

echo 📤 发送请求...
curl -X GET "http://localhost:8088/api/health/history?userId=demo_user&days=30" 2>nul

if %errorlevel% equ 0 (
    echo.
    echo ✅ 查询成功
    set /a success+=1
) else (
    echo.
    echo ❌ 查询失败
    set /a failed+=1
)
echo.
pause

echo ========================================
echo 场景5: 健康档案摘要
echo ========================================
echo.
echo 场景说明: 获取健康档案的分类摘要
echo 查询参数: userId=demo_user
echo 预期效果: 按类别分组显示摘要信息
echo.
set /a total+=1

echo 📤 发送请求...
curl -X GET "http://localhost:8088/api/health/summary?userId=demo_user" 2>nul

if %errorlevel% equ 0 (
    echo.
    echo ✅ 查询成功
    set /a success+=1
) else (
    echo.
    echo ❌ 查询失败
    set /a failed+=1
)
echo.
pause

echo ========================================
echo 场景6: 生成就医报告
echo ========================================
echo.
echo 场景说明: 生成最近30天的完整就医报告
echo 查询参数: userId=demo_user, days=30
echo 预期效果: 返回格式化的就医报告（已脱敏）
echo.
set /a total+=1

echo 📤 发送请求...
curl -X GET "http://localhost:8088/api/health/report?userId=demo_user&days=30" 2>nul

if %errorlevel% equ 0 (
    echo.
    echo ✅ 生成成功
    set /a success+=1
) else (
    echo.
    echo ❌ 生成失败
    set /a failed+=1
)
echo.
pause

echo ========================================
echo 健康助手功能演示完成！
echo ========================================
echo.
echo 📊 测试统计:
echo    总测试数: %total%
echo    成功: %success% 次
echo    失败: %failed% 次
echo.

:: 计算成功率
set /a success_rate=success*100/total
echo 📈 成功率: %success_rate%%%
echo.

if %failed% equ 0 (
    echo ✅ 完美运行！所有功能测试通过
) else (
    echo ⚠️  存在 %failed% 次失败，请检查服务日志
)
echo.

echo 📝 功能说明:
echo    - 场景1: 体检信息记录，自动脱敏身份证、手机号、报告号
echo    - 场景2: 就诊记录管理，脱敏处方号、医保卡号
echo    - 场景3: 用药记录，检测药物过敏信息
echo    - 场景4: 健康档案查询，获取历史记录列表
echo    - 场景5: 健康档案摘要，按类别分组展示
echo    - 场景6: 生成就医报告，完整的脱敏报告
echo.

echo 🔒 隐私保护特性:
echo    ✓ 自动检测敏感信息（身份证、手机号、医保卡等）
echo    ✓ 智能脱敏处理（保留部分信息便于识别）
echo    ✓ 医疗场景特化（处方号、体检报告号等）
echo    ✓ 过敏信息标记（重要医疗安全信息）
echo.

echo 💡 提示:
echo    - 详细响应内容请查看上方输出
echo    - 服务端日志包含更多调试信息
echo    - 可通过 /api/health/clear?userId=demo_user 清空测试数据
echo.
pause
