@echo off
setlocal
cd /d %~dp0
call scripts\build-app.cmd
if errorlevel 1 exit /b 1
echo.
echo WorkMate application built in dist\WorkMate.exe
echo To build the standard installer, install Inno Setup 6 and run scripts\build-installer.cmd.
pause
