@echo off

:loop
 echo 启动自动签到系统...
 set HTTP_PROXY=http://127.0.0.1:7897
 set HTTPS_PROXY=http://127.0.0.1:7897
 .\bot.exe
 echo 系统退出，3秒后重新启动...
 timeout /t 3 /nobreak >nul
 goto loop
