@echo off
echo ========================================
echo 忆安智护 - 后端服务重启脚本
echo ========================================
echo.

echo [1/3] 停止现有服务...
taskkill /F /IM server.exe 2>/dev/null
if %errorlevel% equ 0 (
    echo     ✓ 已停止旧服务
    timeout /t 2 /nobreak >/dev/null
) else (
    echo     ℹ 没有运行中的服务
)

echo.
echo [2/3] 检查 Ollama 服务状态...
curl -s http://127.0.0.1:11434/api/tags >/dev/null 2>&1
if %errorlevel% equ 0 (
    echo     ✓ Ollama 服务运行正常
) else (
    echo     ✗ Ollama 服务未运行
    echo     请先启动 Ollama 服务
    echo     命令: ollama serve
    pause
    exit /b 1
)

echo.
echo [3/3] 启动后端服务...
start "忆安智护后端" cmd /k "cd /d %~dp0 && server.exe"
timeout /t 2 /nobreak >/dev/null

echo.
echo ========================================
echo 服务启动完成！
echo ========================================
echo.
echo 后端服务: http://localhost:8088
echo 前端页面: http://localhost:8889
echo.
echo 按任意键关闭此窗口...
pause >/dev/null
