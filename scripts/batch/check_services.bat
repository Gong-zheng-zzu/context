@echo off
chcp 65001 >/dev/null
echo ========================================
echo 忆安智护 - 服务状态检查
echo ========================================
echo.

echo [检查 1] Ollama 服务 (端口 11434)
netstat -ano | findstr ":11434" >/dev/null 2>&1
if %errorlevel% equ 0 (
    echo     ✓ Ollama 服务运行中
    curl -s http://127.0.0.1:11434/api/tags >/dev/null 2>&1
    if %errorlevel% equ 0 (
        echo     ✓ Ollama API 响应正常
    ) else (
        echo     ✗ Ollama API 无响应
    )
) else (
    echo     ✗ Ollama 服务未运行
    echo     启动命令: ollama serve
)

echo.
echo [检查 2] 后端服务 (端口 8088)
netstat -ano | findstr ":8088" >/dev/null 2>&1
if %errorlevel% equ 0 (
    echo     ✓ 后端服务运行中
    curl -s http://localhost:8088/health >/dev/null 2>&1
    if %errorlevel% equ 0 (
        echo     ✓ 后端 API 响应正常
    ) else (
        echo     ✗ 后端 API 无响应
    )
) else (
    echo     ✗ 后端服务未运行
    echo     启动命令: server.exe
)

echo.
echo [检查 3] 前端服务 (端口 8889)
netstat -ano | findstr ":8889" >/dev/null 2>&1
if %errorlevel% equ 0 (
    echo     ✓ 前端服务运行中
) else (
    echo     ✗ 前端服务未运行
    echo     启动命令: python -m http.server 8889 --directory web
)

echo.
echo [检查 4] 配置文件
if exist "config\.env" (
    echo     ✓ 配置文件存在
    findstr "127.0.0.1:11434" config\.env >/dev/null 2>&1
    if %errorlevel% equ 0 (
        echo     ✓ Ollama 地址配置正确 (127.0.0.1)
    ) else (
        echo     ⚠ Ollama 地址可能配置错误
    )
) else (
    echo     ✗ 配置文件不存在
)

echo.
echo ========================================
echo 检查完成
echo ========================================
pause
