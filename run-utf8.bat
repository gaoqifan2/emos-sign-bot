@echo off

REM 设置控制台编码为UTF-8
chcp 65001 >nul

REM 启动自动签到系统
echo 启动自动签到系统...
set HTTP_PROXY=http://127.0.0.1:7897
set HTTPS_PROXY=http://127.0.0.1:7897

:loop
REM 运行系统
.ot.exe

REM 如果程序退出，等待3秒后重启
echo 系统退出，3秒后重新启动...
timeout /t 3 /nobreak >nul
goto loop
