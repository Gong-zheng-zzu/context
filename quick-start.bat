@echo off
REM Context-Keeper 快速启动脚本 (Windows版)
REM 用途：一键启动所有服务并检查健康状态

echo ==========================================
echo    Context-Keeper 快速启动脚本
echo ==========================================
echo.

REM 检查Docker是否安装
echo [检查] Docker环境...
docker --version >nul 2>&1
if %errorlevel% neq 0 (
    echo [错误] Docker未安装，请先安装Docker Desktop
    echo 下载地址: https://www.docker.com/products/docker-desktop/
    pause
    exit /b 1
)

docker info >nul 2>&1
if %errorlevel% neq 0 (
    echo [错误] Docker服务未启动，请启动Docker Desktop
    pause
    exit /b 1
)

echo [成功] Docker环境正常
echo.

REM 检查config/.env文件
echo [检查] 配置文件...
if not exist config\.env (
    if exist .env.example (
        echo [警告] 未找到config\.env文件，正在从.env.example复制...
        if not exist config mkdir config
        copy .env.example config\.env
        echo [成功] 已创建config\.env文件
        echo.
        echo 重要：请编辑 config\.env 文件，修改以下配置：
        echo   - JWT_SECRET (必须修改)
        echo   - NEO4J_PASSWORD
        echo   - TIMESCALEDB_PASSWORD
        echo   - VECTOR_DB_API_KEY (如使用阿里云)
        echo   - EMBEDDING_API_KEY (如使用阿里云)
        echo.
        pause
        notepad config\.env
    ) else (
        echo [错误] 未找到.env.example文件
        pause
        exit /b 1
    )
)

echo [成功] 配置文件存在
echo.

REM 启动服务
echo [启动] Docker容器...
docker compose up -d

echo.
echo [等待] 服务启动中（30秒）...
timeout /t 30 /nobreak >nul

REM 检查服务状态
echo.
echo [检查] 服务健康状态...
echo.

docker compose ps | findstr "context-keeper" | findstr "Up" >nul
if %errorlevel% equ 0 (
    echo [成功] Context-Keeper 后端服务 [Running]
) else (
    echo [失败] Context-Keeper 后端服务 [Failed]
    echo 查看日志: docker compose logs context-keeper
)

docker compose ps | findstr "neo4j" | findstr "Up" >nul
if %errorlevel% equ 0 (
    echo [成功] Neo4j 图数据库 [Running]
) else (
    echo [失败] Neo4j 图数据库 [Failed]
)

docker compose ps | findstr "timescaledb" | findstr "Up" >nul
if %errorlevel% equ 0 (
    echo [成功] TimescaleDB 时序数据库 [Running]
) else (
    echo [失败] TimescaleDB 时序数据库 [Failed]
)

docker compose ps | findstr "qdrant" | findstr "Up" >nul
if %errorlevel% equ 0 (
    echo [成功] Qdrant 向量数据库 [Running]
) else (
    echo [警告] Qdrant 向量数据库 [Not Running]
)

REM 检查Ollama
echo.
echo [检查] Ollama本地LLM...
curl -s http://localhost:11434/api/tags >nul 2>&1
if %errorlevel% equ 0 (
    echo [成功] Ollama服务 [Running]

    curl -s http://localhost:11434/api/tags | findstr "qwen2.5" >nul
    if %errorlevel% equ 0 (
        echo [成功] Qwen2.5 模型已下载
    ) else (
        echo [警告] Qwen2.5 模型未下载
        echo   运行以下命令下载: ollama pull qwen2.5:3b
    )
) else (
    echo [警告] Ollama服务未运行 (可选)
    echo   如需本地LLM功能，请安装Ollama: https://ollama.com
)

echo.
echo ==========================================
echo    服务访问地址
echo ==========================================
echo.
echo Web界面:          http://localhost:8088
echo 默认账号:         doctor_wang
echo 默认密码:         (查看config\.env中的DEMO_AUTH_PASSWORD)
echo.
echo Neo4j Browser:    http://localhost:7474
echo Qdrant Dashboard: http://localhost:6333/dashboard
echo Ollama API:       http://localhost:11434
echo.
echo ==========================================
echo.

REM 询问是否启动可视化服务
set /p viz_choice="是否启动可视化服务？(用于生成血压图表) [y/n]: "
if /i "%viz_choice%"=="y" (
    echo.
    echo [启动] 可视化服务...

    python --version >nul 2>&1
    if %errorlevel% neq 0 (
        echo [错误] Python未安装
    ) else (
        cd tools

        REM 检查依赖
        python -c "import flask" 2>nul
        if %errorlevel% neq 0 (
            echo [安装] Python依赖...
            pip install flask flask-cors matplotlib
        )

        echo [成功] 启动可视化服务 (后台运行)
        start /b python visualization_service.py > visualization.log 2>&1

        timeout /t 2 /nobreak >nul

        curl -s http://localhost:5001/health >nul 2>&1
        if %errorlevel% equ 0 (
            echo [成功] 可视化服务已启动 http://localhost:5001
        ) else (
            echo [失败] 可视化服务启动失败，查看 tools\visualization.log
        )

        cd ..
    )
)

echo.
echo ==========================================
echo    快速测试
echo ==========================================
echo.
echo 运行以下命令插入测试数据:
echo   cd tools
echo   go run insert_test_data.go
echo.
echo 查看服务日志:
echo   docker compose logs -f context-keeper
echo.
echo 停止所有服务:
echo   docker compose down
echo.
echo [成功] 启动完成！请访问 http://localhost:8088
echo.
pause
