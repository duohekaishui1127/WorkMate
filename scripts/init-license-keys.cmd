@echo off
setlocal
cd /d "%~dp0.."
echo Generating NEW WorkMate commercial license signing keys...
go run .\tools\licensegen -generate
if errorlevel 1 exit /b %errorlevel%
echo.
echo IMPORTANT: developer-secrets\license-private.key is your commercial master key.
echo Back it up offline. NEVER ship it with WorkMate or upload it to a public repository.
pause
