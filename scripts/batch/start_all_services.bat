@echo off
chcp 65001 >nul
echo ========================================
echo 忆安智护 - 完整服务启动脚本
echo ========================================
echo.

echo [1/4] 检查 Ollama 服务...
curl -s http://127.0.0.1:11434/api/tags >nul 2>&1
if %errorlevel% equ 0 (
    echo     ✓ Ollama 服务运行正常
) else (
    echo     ✗ Ollama 服务未运行
    echo     请先启动 Ollama 服务
    pause
    exit /b 1
)

echo.
echo [2/4] 检查后端服务 (8088端口)...
curl -s http://localhost:8088/health >nul 2>&1
if %errorlevel% equ 0 (
    echo     ✓ 后端服务已运行
) else (
    echo     ℹ 后端服务未运行，正在启动...
    start "忆安智护后端" cmd /k "cd /d %~dp0 && server.exe"
    timeout /t 3 /nobreak >nul
    echo     ✓ 后端服务已启动
)

echo.
echo [3/4] 检查前端服务 (8889端口)...
curl -s http://localhost:8889/ >nul 2>&1
if %errorlevel% equ 0 (
    echo     ✓ 前端服务已运行
) else (
    echo     ℹ 前端服务未运行，正在启动...
    start "忆安智护前端" cmd /k "cd /d "%~dp0" && python -m http.server 8889"
    timeout /t 2 /nobreak >nul
    echo     ✓ 前端服务已启动
)

echo.
echo [4/4] 验证所有服务...
timeout /t 2 /nobreak >nul

curl -s http://localhost:8088/health >nul 2>&1
if %errorlevel% equ 0 (
    echo     ✓ 后端服务 (8088): 正常
) else (
    echo     ✗ 后端服务 (8088): 异常
)

curl -s http://localhost:8889/ >nul 2>&1
if %errorlevel% equ 0 (
    echo     ✓ 前端服务 (8889): 正常
) else (
    echo     ✗ 前端服务 (8889): 异常
)

curl -s http://127.0.0.1:11434/api/tags >nul 2>&1
if %errorlevel% equ 0 (
    echo     ✓ Ollama服务 (11434): 正常
) else (
    echo     ✗ Ollama服务 (11434): 异常
)

echo.
echo ========================================
echo 所有服务启动完成！
echo ========================================
echo.
echo 📱 访问地址：
echo    角色选择页: http://localhost:8889/web/role_selection.html
echo    护工端: http://localhost:8889/web/caregiver_chat.html
echo    医生端: http://localhost:8889/web/doctor_chat.html
echo    家属端: http://localhost:8889/web/family_chat.html
echo    老人端: http://localhost:8889/web/elder_voice_chat.html
echo.
echo 🔧 服务端点：
echo    后端API: http://localhost:8088
echo    健康检查: http://localhost:8088/health
echo    Ollama: http://127.0.0.1:11434
echo.
echo 💡 提示：
echo    - 所有服务已在后台运行
echo    - 关闭此窗口不会停止服务
echo    - 如需停止服务，请使用任务管理器
echo.

REM 自动打开浏览器
start http://localhost:8889/web/role_selection.html

pause
