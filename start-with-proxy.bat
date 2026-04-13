@echo off
cls
echo 设置代理环境变量...
set HTTP_PROXY=http://127.0.0.1:7897
set HTTPS_PROXY=http://127.0.0.1:7897
echo 代理环境变量设置完成

echo 启动自动签到系统...
go run main.go
pause