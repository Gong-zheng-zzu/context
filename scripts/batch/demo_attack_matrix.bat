@echo off
chcp 65001 >nul
echo ========================================
echo 多层敏感信息检测 - 攻防对抗矩阵演示
echo ========================================
echo.
echo 本演示展示10种绕过攻击 vs 3层防御体系
echo.

set BASE_URL=http://localhost:8088
set SUCCESS_COUNT=0
set TOTAL_COUNT=10

echo ========================================
echo 第一层防御：正则表达式检测
echo ========================================
echo.

echo [攻击1] 标准格式身份证
curl -X POST %BASE_URL%/api/health/record -H "Content-Type: application/json" -d "{\"userId\":\"test\",\"sessionId\":\"attack_test\",\"content\":\"患者身份证：110101199001011234\"}" 2>nul | findstr "110101199001011234" >nul
if errorlevel 1 (
    echo ✅ 防御成功：身份证已脱敏
    set /a SUCCESS_COUNT+=1
) else (
    echo ❌ 防御失败：身份证泄露
)
echo.

echo [攻击2] 标准格式手机号
curl -X POST %BASE_URL%/api/health/record -H "Content-Type: application/json" -d "{\"userId\":\"test\",\"sessionId\":\"attack_test\",\"content\":\"联系电话：13812345678\"}" 2>nul | findstr "13812345678" >nul
if errorlevel 1 (
    echo ✅ 防御成功：手机号已脱敏
    set /a SUCCESS_COUNT+=1
) else (
    echo ❌ 防御失败：手机号泄露
)
echo.

echo ========================================
echo 第二层防御：字典树+AC自动机
echo ========================================
echo.

echo [攻击3] 空格分隔绕过
curl -X POST %BASE_URL%/api/health/record -H "Content-Type: application/json" -d "{\"userId\":\"test\",\"sessionId\":\"attack_test\",\"content\":\"身份证号：1 1 0 1 0 1 1 9 9 0 0 1 0 1 1 2 3 4\"}" 2>nul | findstr "110101" >nul
if errorlevel 1 (
    echo ✅ 防御成功：空格绕过被识别
    set /a SUCCESS_COUNT+=1
) else (
    echo ❌ 防御失败：空格绕过成功
)
echo.

echo [攻击4] 特殊字符混淆
curl -X POST %BASE_URL%/api/health/record -H "Content-Type: application/json" -d "{\"userId\":\"test\",\"sessionId\":\"attack_test\",\"content\":\"手机：138-1234-5678\"}" 2>nul | findstr "138-1234-5678" >nul
if errorlevel 1 (
    echo ✅ 防御成功：特殊字符混淆被识别
    set /a SUCCESS_COUNT+=1
) else (
    echo ❌ 防御失败：特殊字符绕过成功
)
echo.

echo [攻击5] 中文数字混合
curl -X POST %BASE_URL%/api/health/record -H "Content-Type: application/json" -d "{\"userId\":\"test\",\"sessionId\":\"attack_test\",\"content\":\"身份证：一一零一零一一九九零零一零一一二三四\"}" 2>nul | findstr "一一零" >nul
if errorlevel 1 (
    echo ✅ 防御成功：中文数字被识别
    set /a SUCCESS_COUNT+=1
) else (
    echo ❌ 防御失败：中文数字绕过成功
)
echo.

echo ========================================
echo 第三层防御：LLM语义检测
echo ========================================
echo.

echo [攻击6] 语义描述绕过
curl -X POST %BASE_URL%/api/health/record -H "Content-Type: application/json" -d "{\"userId\":\"test\",\"sessionId\":\"attack_test\",\"content\":\"患者的证件号码是一一零一零一开头的那个十八位数字\"}" 2>nul | findstr "一一零一零一" >nul
if errorlevel 1 (
    echo ✅ 防御成功：语义描述被识别
    set /a SUCCESS_COUNT+=1
) else (
    echo ❌ 防御失败：语义绕过成功
)
echo.

echo [攻击7] 同音字替换
curl -X POST %BASE_URL%/api/health/record -H "Content-Type: application/json" -d "{\"userId\":\"test\",\"sessionId\":\"attack_test\",\"content\":\"身份证：幺幺零幺零幺幺玖玖零零幺零幺幺贰叁肆\"}" 2>nul | findstr "幺幺零" >nul
if errorlevel 1 (
    echo ✅ 防御成功：同音字被识别
    set /a SUCCESS_COUNT+=1
) else (
    echo ❌ 防御失败：同音字绕过成功
)
echo.

echo [攻击8] 拆字攻击
curl -X POST %BASE_URL%/api/health/record -H "Content-Type: application/json" -d "{\"userId\":\"test\",\"sessionId\":\"attack_test\",\"content\":\"证件号前六位是110101，后面是19900101加1234\"}" 2>nul | findstr "110101" >nul
if errorlevel 1 (
    echo ✅ 防御成功：拆字攻击被识别
    set /a SUCCESS_COUNT+=1
) else (
    echo ❌ 防御失败：拆字绕过成功
)
echo.

echo [攻击9] Base64编码
curl -X POST %BASE_URL%/api/health/record -H "Content-Type: application/json" -d "{\"userId\":\"test\",\"sessionId\":\"attack_test\",\"content\":\"身份证编码：MTEwMTAxMTk5MDAxMDExMjM0\"}" 2>nul | findstr "MTEwMTAx" >nul
if errorlevel 1 (
    echo ✅ 防御成功：Base64编码被识别
    set /a SUCCESS_COUNT+=1
) else (
    echo ❌ 防御失败：Base64绕过成功
)
echo.

echo [攻击10] 图片OCR绕过（模拟）
echo 说明：攻击者将敏感信息嵌入图片，尝试绕过文本检测
curl -X POST %BASE_URL%/api/health/record -H "Content-Type: application/json" -d "{\"userId\":\"test\",\"sessionId\":\"attack_test\",\"content\":\"[图片：身份证照片.jpg]\"}" 2>nul | findstr "图片" >nul
if errorlevel 1 (
    echo ✅ 防御成功：图片内容被拦截
    set /a SUCCESS_COUNT+=1
) else (
    echo ⚠️  当前版本：图片检测待实现
)
echo.

echo ========================================
echo 攻防对抗结果统计
echo ========================================
echo.
echo 总攻击次数：%TOTAL_COUNT%
echo 防御成功：%SUCCESS_COUNT%
echo 防御成功率：%SUCCESS_COUNT%0%%
echo.
if %SUCCESS_COUNT% GEQ 9 (
    echo 🏆 防御等级：优秀 - 多层检测体系有效
) else if %SUCCESS_COUNT% GEQ 7 (
    echo ⭐ 防御等级：良好 - 大部分攻击被拦截
) else (
    echo ⚠️  防御等级：需改进 - 存在绕过风险
)
echo.
echo ========================================
echo 技术亮点总结
echo ========================================
echo.
echo 1. 三层防御体系：正则 + 字典树 + LLM语义
echo 2. 覆盖10种常见绕过攻击场景
echo 3. 防御成功率目标：100%%
echo 4. 本地化部署，数据不出本地
echo 5. 实时检测，响应时间 ^<100ms
echo.
pause
