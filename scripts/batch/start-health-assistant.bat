@echo off
chcp 65001 >nul
echo ==========================================
echo 健康助手 - Docker 启动脚本
echo ==========================================
echo.

REM 检查.env文件
if not exist "config\.env" (
    echo ⚠️  未找到 config\.env 文件
    echo 正在创建默认配置...

    if not exist "config" mkdir config

    (
        echo # LLM配置
        echo LLM_DRIVEN_ENABLED=true
        echo LLM_PROVIDER=ollama_local
        echo LLM_MODEL=qwen2.5:3b
        echo # DEEPSEEK_API_KEY=your_api_key_here
        echo.
        echo # 如果使用DeepSeek云端API，取消注释下面的配置
        echo # LLM_PROVIDER=deepseek
        echo # LLM_MODEL=deepseek-chat
        echo # DEEPSEEK_API_KEY=your_api_key_here
        echo.
        echo # 服务器配置
        echo RUN_MODE=http
        echo HTTP_SERVER_PORT=8088
        echo HOST_PORT=8088
        echo.
        echo # 日志配置
        echo LOG_LEVEL=info
        echo CONTEXT_KEEPER_LOG_TO_STDOUT=true
        echo.
        echo # 时区
        echo TZ=Asia/Shanghai
    ) > config\.env

    echo ✅ 已创建 config\.env 文件
    echo ⚠️  请编辑 config\.env 文件，设置你的 DEEPSEEK_API_KEY
    echo.
    pause
)

REM 检查Docker是否运行
docker info >nul 2>&1
if errorlevel 1 (
    echo ❌ Docker未运行，请先启动Docker Desktop
    pause
    exit /b 1
)

echo.
echo 🚀 启动健康助手服务...
echo.

REM 构建并启动
docker-compose up -d --build

REM 等待Ollama服务启动
echo.
echo ⏳ 等待Ollama服务启动...
timeout /t 10 /nobreak >nul

REM 拉取Ollama模型
echo.
echo 📥 拉取Ollama模型 (qwen2.5:3b)...
docker exec context-keeper-ollama ollama pull qwen2.5:3b

REM 等待服务启动
echo.
echo ⏳ 等待健康助手服务启动...
timeout /t 5 /nobreak >nul

REM 检查服务状态
docker-compose ps | findstr "Up" >nul
if not errorlevel 1 (
    echo.
    echo ==========================================
    echo ✅ 健康助手服务已启动！
    echo ==========================================
    echo.
    echo 📝 访问地址：
    echo    前端页面: file:///%CD%\web\user_health_assistant.html
    echo    或者: http://localhost:8088/user_health_assistant.html
    echo.
    echo 🔧 API端点：
    echo    健康检查: http://localhost:8088/health
    echo    聊天API: http://localhost:8088/api/chat
    echo.
    echo 📊 查看日志：
    echo    docker-compose logs -f context-keeper
    echo.
    echo 🛑 停止服务：
    echo    docker-compose down
    echo.
) else (
    echo.
    echo ❌ 服务启动失败，请查看日志：
    echo    docker-compose logs context-keeper
    echo.
)

pause
