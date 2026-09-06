@echo off
chcp 65001 >nul
echo ====================================
echo 养老院系统 - 快速启动
echo ====================================
echo.

echo [步骤 1/3] 启动大数据服务...
cd /d D:\bigdata
call start-all-docker.bat
echo.

echo [步骤 2/3] 等待服务就绪...
timeout /t 10 /nobreak >nul
echo.

echo [步骤 3/3] 检查服务状态...
docker ps --format "table {{.Names}}\t{{.Status}}" | findstr /C:"influxdb" /C:"qdrant"
echo.

echo ====================================
echo 启动完成！
echo ====================================
echo.
echo 请访问: http://localhost:8088/web/role_selection.html
echo.
echo 提示: 如果后端服务未运行，请执行 start-local.bat
echo.
pause

start http://localhost:8088/web/role_selection.html
