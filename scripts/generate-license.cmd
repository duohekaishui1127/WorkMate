@echo off
setlocal
cd /d "%~dp0.."
if "%~1"=="" goto usage
if "%~2"=="" goto usage
go run .\tools\licensegen -device "%~1" -id "%~2" -customer "%~3"
exit /b %errorlevel%
:usage
echo Usage: scripts\generate-license.cmd FULL_DEVICE_HASH LICENSE_ID [CUSTOMER]
exit /b 2
