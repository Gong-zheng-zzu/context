@echo off
chcp 65001 >nul
echo ========================================
echo 养老院护理助手 - 多角色系统演示
echo ========================================
echo.

set API_BASE=http://localhost:8080/api

echo [步骤1] 检查服务器是否运行...
curl -s %API_BASE%/health >nul 2>&1
if errorlevel 1 (
    echo ❌ 服务器未运行，请先启动服务器
    echo 运行命令: server.exe
    pause
    exit /b 1
)
echo ✅ 服务器运行正常
echo.

echo [步骤2] 护工登录并记录生命体征...
echo.
curl -X POST %API_BASE%/role/login ^
  -H "Content-Type: application/json" ^
  -d "{\"user_id\":\"caregiver_001\",\"password\":\"password123\",\"role\":\"caregiver\"}" ^
  > temp_caregiver_token.json

for /f "tokens=2 delims=:," %%a in ('type temp_caregiver_token.json ^| findstr "token"') do set CAREGIVER_TOKEN=%%a
set CAREGIVER_TOKEN=%CAREGIVER_TOKEN:"=%
set CAREGIVER_TOKEN=%CAREGIVER_TOKEN: =%

echo 护工登录成功，Token: %CAREGIVER_TOKEN:~0,20%...
echo.

echo 护工为张奶奶测量血压...
curl -X POST %API_BASE%/health/vital-signs ^
  -H "Authorization: Bearer %CAREGIVER_TOKEN%" ^
  -H "Content-Type: application/json" ^
  -d "{\"resident_id\":\"elder_001\",\"type\":\"blood_pressure\",\"value\":145,\"value2\":92,\"unit\":\"mmHg\",\"recorded_by\":\"caregiver_001\"}"
echo.
echo.

echo 护工为李爷爷测量体温...
curl -X POST %API_BASE%/health/vital-signs ^
  -H "Authorization: Bearer %CAREGIVER_TOKEN%" ^
  -H "Content-Type: application/json" ^
  -d "{\"resident_id\":\"elder_002\",\"type\":\"temperature\",\"value\":37.8,\"unit\":\"°C\",\"recorded_by\":\"caregiver_001\"}"
echo.
echo.

echo 护工记录日常护理...
curl -X POST %API_BASE%/health/record ^
  -H "Authorization: Bearer %CAREGIVER_TOKEN%" ^
  -H "Content-Type: application/json" ^
  -d "{\"user_id\":\"elder_001\",\"session_id\":\"session_001\",\"content\":\"早餐进食良好，精神状态佳\",\"category\":\"日常护理\"}"
echo.
echo.

timeout /t 2 >nul

echo [步骤3] 医生登录并查看告警...
echo.
curl -X POST %API_BASE%/role/login ^
  -H "Content-Type: application/json" ^
  -d "{\"user_id\":\"doctor_001\",\"password\":\"password123\",\"role\":\"doctor\"}" ^
  > temp_doctor_token.json

for /f "tokens=2 delims=:," %%a in ('type temp_doctor_token.json ^| findstr "token"') do set DOCTOR_TOKEN=%%a
set DOCTOR_TOKEN=%DOCTOR_TOKEN:"=%
set DOCTOR_TOKEN=%DOCTOR_TOKEN: =%

echo 医生登录成功，Token: %DOCTOR_TOKEN:~0,20%...
echo.

echo 医生查看仪表盘...
curl -X GET %API_BASE%/dashboard ^
  -H "Authorization: Bearer %DOCTOR_TOKEN%" ^
  > temp_doctor_dashboard.json
echo.

echo 医生仪表盘数据:
type temp_doctor_dashboard.json
echo.
echo.

echo 医生查看告警信息...
curl -X GET %API_BASE%/alerts ^
  -H "Authorization: Bearer %DOCTOR_TOKEN%" ^
  > temp_doctor_alerts.json
echo.

echo 告警信息:
type temp_doctor_alerts.json
echo.
echo.

echo 医生查看诊疗建议...
curl -X GET %API_BASE%/recommendations ^
  -H "Authorization: Bearer %DOCTOR_TOKEN%" ^
  > temp_doctor_recommendations.json
echo.

echo 诊疗建议:
type temp_doctor_recommendations.json
echo.
echo.

timeout /t 2 >nul

echo [步骤4] 家属登录并查看亲人状况...
echo.
curl -X POST %API_BASE%/role/login ^
  -H "Content-Type: application/json" ^
  -d "{\"user_id\":\"family_001\",\"password\":\"password123\",\"role\":\"family\"}" ^
  > temp_family_token.json

for /f "tokens=2 delims=:," %%a in ('type temp_family_token.json ^| findstr "token"') do set FAMILY_TOKEN=%%a
set FAMILY_TOKEN=%FAMILY_TOKEN:"=%
set FAMILY_TOKEN=%FAMILY_TOKEN: =%

echo 家属登录成功，Token: %FAMILY_TOKEN:~0,20%...
echo.

echo 家属查看亲人状况...
curl -X GET %API_BASE%/dashboard ^
  -H "Authorization: Bearer %FAMILY_TOKEN%" ^
  > temp_family_dashboard.json
echo.

echo 家属仪表盘数据:
type temp_family_dashboard.json
echo.
echo.

echo 家属给亲人留言...
curl -X POST %API_BASE%/family-message ^
  -H "Authorization: Bearer %FAMILY_TOKEN%" ^
  -H "Content-Type: application/json" ^
  -d "{\"elder_id\":\"elder_001\",\"message\":\"妈妈，周末我会来看您\"}"
echo.
echo.

timeout /t 2 >nul

echo [步骤5] 老人登录并呼叫护工...
echo.
curl -X POST %API_BASE%/role/login ^
  -H "Content-Type: application/json" ^
  -d "{\"user_id\":\"elder_001\",\"password\":\"password123\",\"role\":\"elder\"}" ^
  > temp_elder_token.json

for /f "tokens=2 delims=:," %%a in ('type temp_elder_token.json ^| findstr "token"') do set ELDER_TOKEN=%%a
set ELDER_TOKEN=%ELDER_TOKEN:"=%
set ELDER_TOKEN=%ELDER_TOKEN: =%

echo 老人登录成功，Token: %ELDER_TOKEN:~0,20%...
echo.

echo 老人查看自己的信息...
curl -X GET %API_BASE%/dashboard ^
  -H "Authorization: Bearer %ELDER_TOKEN%" ^
  > temp_elder_dashboard.json
echo.

echo 老人仪表盘数据:
type temp_elder_dashboard.json
echo.
echo.

echo 老人呼叫护工...
curl -X POST %API_BASE%/call-caregiver ^
  -H "Authorization: Bearer %ELDER_TOKEN%" ^
  -H "Content-Type: application/json" ^
  -d "{\"elder_id\":\"elder_001\",\"reason\":\"需要帮助\"}"
echo.
echo.

echo [步骤6] 查看生命体征历史数据...
echo.
echo 查询张奶奶的血压历史（最近7天）...
curl -X GET "%API_BASE%/health/vital-signs/history?resident_id=elder_001&type=blood_pressure&days=7" ^
  -H "Authorization: Bearer %DOCTOR_TOKEN%" ^
  > temp_vital_history.json
echo.

echo 血压历史数据:
type temp_vital_history.json
echo.
echo.

echo [步骤7] 查询生命体征统计（带差分隐私保护）...
echo.
curl -X GET "%API_BASE%/health/vital-signs/stats?resident_id=elder_001&type=blood_pressure&days=30" ^
  -H "Authorization: Bearer %DOCTOR_TOKEN%" ^
  > temp_vital_stats.json
echo.

echo 统计数据（已应用差分隐私保护）:
type temp_vital_stats.json
echo.
echo.

echo ========================================
echo 演示完成！
echo ========================================
echo.
echo 现在可以打开浏览器访问各个端口:
echo.
echo 护工端: file:///%CD%\web\caregiver_portal.html
echo 医生端: file:///%CD%\web\doctor_portal.html
echo 家属端: file:///%CD%\web\family_portal.html
echo 老人端: file:///%CD%\web\elder_portal.html
echo.
echo 登录信息:
echo 护工: caregiver_001 / password123
echo 医生: doctor_001 / password123
echo 家属: family_001 / password123
echo 老人: elder_001 / password123
echo.

echo 清理临时文件...
del temp_*.json 2>nul

pause
