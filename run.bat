@echo off

:: 设置控制台编码为UTF-8
chcp 65001

:: 设置代理
set HTTP_PROXY=http://127.0.0.1:7897
set HTTPS_PROXY=http://127.0.0.1:7897

echo 启动自动签到系统...

:: 无限循环运行
:loop
echo [%date% %time%] 启动自动签到系统...
bot.exe
echo [%date% %time%] 系统退出，等待5秒后重新启动...
timeout /t 5 /nobreak >nul
goto loop