@echo off
setlocal
cd /d "%~dp0.."
call scripts\build-app.cmd
if errorlevel 1 exit /b 1
call scripts\build-installer.cmd
exit /b %errorlevel%
