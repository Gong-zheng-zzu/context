@echo off
REM Context-Keeper Docker 快速启动脚本 (Windows)
REM 用于快速启动和管理 Docker 服务

setlocal enabledelayedexpansion

REM 切换到脚本所在目录
cd /d "%~dp0"

REM 显示帮助信息
if "%1"=="" goto :show_help
if "%1"=="help" goto :show_help
if "%1"=="--help" goto :show_help
if "%1"=="-h" goto :show_help

REM 检查 Docker
where docker >nul 2>&1
if errorlevel 1 (
    echo [错误] 未安装 Docker
    exit /b 1
)

REM 执行命令
if "%1"=="build" goto :build
if "%1"=="up" goto :up
if "%1"=="start" goto :up
if "%1"=="down" goto :down
if "%1"=="stop" goto :down
if "%1"=="restart" goto :restart
if "%1"=="logs" goto :logs
if "%1"=="status" goto :status
if "%1"=="ps" goto :status
if "%1"=="clean" goto :clean
if "%1"=="shell" goto :shell
if "%1"=="exec" goto :shell

echo [错误] 未知命令: %1
echo.
goto :show_help

:show_help
echo Context-Keeper Docker 管理脚本
echo.
echo 用法: docker.bat [命令] [选项]
echo.
echo 命令:
echo   build       构建 Docker 镜像
echo   up          启动所有服务
echo   down        停止所有服务
echo   restart     重启所有服务
echo   logs        查看服务日志
echo   status      查看服务状态
echo   clean       清理所有容器和数据卷
echo   shell       进入容器 shell
echo   help        显示此帮助信息
echo.
echo 示例:
echo   docker.bat up           启动服务
echo   docker.bat logs         查看日志
echo   docker.bat status       查看状态
echo   docker.bat shell        进入容器
goto :eof

:build
echo [构建] 构建 Docker 镜像...
docker compose build --no-cache
if errorlevel 1 (
    echo [错误] 构建失败
    exit /b 1
)
echo [成功] 镜像构建完成
goto :eof

:up
echo [启动] 检查 Ollama 服务...
curl -s http://localhost:11434/api/tags >nul 2>&1
if errorlevel 1 (
    echo [警告] Ollama 服务未运行，请先启动 Ollama
    echo.
    echo 继续启动? (Y/N)
    set /p confirm=
    if /i not "!confirm!"=="Y" exit /b 1
)

echo [启动] 启动 Context-Keeper 服务...
docker compose up -d
if errorlevel 1 (
    echo [错误] 启动失败
    exit /b 1
)

echo.
echo [成功] 服务启动成功
echo.
echo 服务访问地址:
echo   主服务: http://localhost:8088
echo   健康检查: http://localhost:8088/health
echo   Qdrant: http://localhost:6333
echo.
echo 查看日志: docker.bat logs
goto :eof

:down
echo [停止] 停止服务...
docker compose down
echo [成功] 服务已停止
goto :eof

:restart
echo [重启] 重启服务...
docker compose restart
echo [成功] 服务已重启
goto :eof

:logs
set SERVICE=%2
if "%SERVICE%"=="" set SERVICE=context-keeper
echo [日志] 查看 %SERVICE% 日志 (Ctrl+C 退出)...
docker compose logs -f %SERVICE%
goto :eof

:status
echo [状态] 服务状态:
docker compose ps
echo.
echo [资源] 资源使用:
docker stats --no-stream --format "table {{.Container}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.NetIO}}"
goto :eof

:clean
echo [警告] 此操作将删除所有容器、镜像和数据卷
set /p confirm=确认继续? (yes/no):
if not "%confirm%"=="yes" (
    echo [取消] 已取消清理
    goto :eof
)
echo [清理] 清理中...
docker compose down -v --rmi all
echo [成功] 清理完成
goto :eof

:shell
set SERVICE=%2
if "%SERVICE%"=="" set SERVICE=context-keeper
echo [Shell] 进入 %SERVICE% 容器...
docker compose exec %SERVICE% /bin/bash
goto :eof
