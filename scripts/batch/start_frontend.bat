@echo off
chcp 65001 >nul
echo ========================================
echo 忆安智护 - 前端服务启动脚本
echo ========================================
echo.

echo [1/2] 检查 Python 是否安装...
python --version >nul 2>&1
if %errorlevel% neq 0 (
    echo     ✗ Python 未安装
    echo     请先安装 Python 3.x
    pause
    exit /b 1
)
echo     ✓ Python 已安装

echo.
echo [2/2] 启动前端静态文件服务器（端口 8889）...
cd /d "%~dp0"
start "忆安智护前端" cmd /k "cd /d "%~dp0" && python -m http.server 8889"
timeout /t 2 /nobreak >nul

echo.
echo ========================================
echo 前端服务启动完成！
echo ========================================
echo.
echo 前端页面: http://localhost:8889/web/caregiver_chat.html
echo 角色选择: http://localhost:8889/web/role_selection.html
echo.
echo 提示: 请确保后端服务已在 8088 端口运行
echo.
pause
