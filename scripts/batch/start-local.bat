@echo off
chcp 65001 >nul
echo ========================================
echo Context-Keeper 本地演示环境启动
echo ========================================
echo.

echo [1/4] 检查 Ollama 是否运行...
curl -s http://localhost:11434/api/tags >nul 2>&1
if %errorlevel% neq 0 (
    echo ❌ Ollama 未运行，请先启动 Ollama
    echo    下载地址: https://ollama.ai/download
    pause
    exit /b 1
)
echo ✅ Ollama 运行正常

echo.
echo [2/4] 检查嵌入模型是否已安装...
ollama list | findstr "nomic-embed-text" >nul 2>&1
if %errorlevel% neq 0 (
    echo ⚠️  nomic-embed-text 模型未安装，正在安装...
    ollama pull nomic-embed-text
    if %errorlevel% neq 0 (
        echo ❌ 模型安装失败
        pause
        exit /b 1
    )
)
echo ✅ 嵌入模型已就绪

echo.
echo [3/4] 启动 Docker 容器...
docker-compose down
docker-compose up -d

echo.
echo [4/4] 等待服务启动...
timeout /t 10 /nobreak >nul

echo.
echo ========================================
echo 🎉 启动完成！
echo ========================================
echo.
echo 📊 服务地址:
echo    - Context-Keeper API: http://localhost:8088
echo    - 演示页面: http://localhost:8000/chat-demo-fixed.html
echo    - Qdrant 管理界面: http://localhost:6333/dashboard
echo    - 健康检查: http://localhost:8088/health
echo.
echo 📝 查看日志:
echo    docker-compose logs -f
echo.
echo 🛑 停止服务:
echo    docker-compose down
echo.
pause
