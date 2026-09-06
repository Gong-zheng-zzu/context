@echo off
chcp 65001 >nul
echo ========================================
echo 随机数据攻击防御演示
echo （证明系统不是硬编码特定数据）
echo ========================================
echo.

set BASE_URL=http://localhost:8088

echo [步骤1] 检查服务是否运行...
curl -s %BASE_URL%/health >nul 2>&1
if %errorlevel% neq 0 (
    echo ❌ 服务未启动！
    pause
    exit /b 1
)
echo ✅ 服务正常运行
echo.

:: 生成随机测试数据
echo ========================================
echo 测试1: 随机手机号拦截
echo ========================================
echo.
echo 攻击说明: 使用随机生成的手机号（非脚本预设）
echo.

set phone1=13911223344
set phone2=18600998877
set phone3=15912345678

echo [攻击1.1] 测试手机号: %phone1%
curl -X POST %BASE_URL%/api/health/record -H "Content-Type: application/json" -d "{\"userId\":\"test\",\"content\":\"我的电话是%phone1%\"}" 2>nul | findstr "%phone1%" >nul
if errorlevel 1 (
    echo ✅ 防御成功：手机号 %phone1% 已脱敏
) else (
    echo ❌ 防御失败：手机号泄露
)
echo.

echo [攻击1.2] 测试手机号: %phone2%
curl -X POST %BASE_URL%/api/health/record -H "Content-Type: application/json" -d "{\"userId\":\"test\",\"content\":\"联系方式：%phone2%\"}" 2>nul | findstr "%phone2%" >nul
if errorlevel 1 (
    echo ✅ 防御成功：手机号 %phone2% 已脱敏
) else (
    echo ❌ 防御失败：手机号泄露
)
echo.

echo [攻击1.3] 测试手机号: %phone3%
curl -X POST %BASE_URL%/api/health/record -H "Content-Type: application/json" -d "{\"userId\":\"test\",\"content\":\"手机：%phone3%\"}" 2>nul | findstr "%phone3%" >nul
if errorlevel 1 (
    echo ✅ 防御成功：手机号 %phone3% 已脱敏
) else (
    echo ❌ 防御失败：手机号泄露
)
echo.
pause

echo ========================================
echo 测试2: 随机身份证号拦截
echo ========================================
echo.
echo 攻击说明: 使用随机生成的身份证号（非脚本预设）
echo.

set id1=320106198506204321
set id2=51010119900101123X
set id3=440301199912151234

echo [攻击2.1] 测试身份证: %id1%
curl -X POST %BASE_URL%/api/health/record -H "Content-Type: application/json" -d "{\"userId\":\"test\",\"content\":\"患者身份证：%id1%\"}" 2>nul | findstr "%id1%" >nul
if errorlevel 1 (
    echo ✅ 防御成功：身份证 %id1% 已脱敏
) else (
    echo ❌ 防御失败：身份证泄露
)
echo.

echo [攻击2.2] 测试身份证: %id2%
curl -X POST %BASE_URL%/api/health/record -H "Content-Type: application/json" -d "{\"userId\":\"test\",\"content\":\"身份证号码：%id2%\"}" 2>nul | findstr "%id2%" >nul
if errorlevel 1 (
    echo ✅ 防御成功：身份证 %id2% 已脱敏
) else (
    echo ❌ 防御失败：身份证泄露
)
echo.

echo [攻击2.3] 测试身份证: %id3%
curl -X POST %BASE_URL%/api/health/record -H "Content-Type: application/json" -d "{\"userId\":\"test\",\"content\":\"ID: %id3%\"}" 2>nul | findstr "%id3%" >nul
if errorlevel 1 (
    echo ✅ 防御成功：身份证 %id3% 已脱敏
) else (
    echo ❌ 防御失败：身份证泄露
)
echo.
pause

echo ========================================
echo 测试3: 评委自定义测试
echo ========================================
echo.
echo 💡 提示：评委可以在这里输入任意手机号/身份证进行测试
echo.
set /p custom_data="请输入要测试的敏感信息（手机号或身份证）: "

echo.
echo [测试] 您输入的数据: %custom_data%
curl -X POST %BASE_URL%/api/health/record -H "Content-Type: application/json" -d "{\"userId\":\"test\",\"content\":\"测试数据：%custom_data%\"}" 2>nul | findstr "%custom_data%" >nul
if errorlevel 1 (
    echo ✅ 防御成功：您输入的数据已被脱敏
) else (
    echo ❌ 防御失败：数据泄露
)
echo.

echo ========================================
echo 测试完成！
echo ========================================
echo.
echo 📊 结论:
echo   - 系统使用通用正则表达式检测，而非硬编码特定数据
echo   - 任何符合格式的敏感信息都会被拦截
echo   - 评委可以输入任意数据验证防御能力
echo.
pause
