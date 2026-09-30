@echo off
setlocal
cd /d "%~dp0.."
set "TARGET=%~1"
if not defined TARGET set "TARGET=amd64"
if not "%TARGET%"=="amd64" if not "%TARGET%"=="arm64" (
  echo Usage: scripts\build-server.cmd [amd64^|arm64]
  exit /b 2
)
where go >nul 2>nul || (echo [ERROR] Go 1.23+ not found. & exit /b 1)
set "GOOS=linux"
set "GOARCH=%TARGET%"
set "CGO_ENABLED=0"
set "OUTPUT=dist\server-linux-%TARGET%"
if not exist "%OUTPUT%" mkdir "%OUTPUT%"
go build -trimpath -buildvcs=false -ldflags="-s -w" -o "%OUTPUT%\workmate-admin" ./cmd/workmate-admin
if errorlevel 1 exit /b 1
copy /y deploy\linux\* "%OUTPUT%\" >nul
if errorlevel 1 exit /b 1
copy /y DEPLOY.md "%OUTPUT%\" >nul
if errorlevel 1 exit /b 1
set "KEY_FILE=admin-data\license-public.key"
if defined WORKMATE_ADMIN_PUBLIC_KEY_FILE set "KEY_FILE=%WORKMATE_ADMIN_PUBLIC_KEY_FILE%"
if not exist "%KEY_FILE%" (
  echo [ERROR] Missing admin public key. Initialize the local backend first.
  exit /b 1
)
copy /y "%KEY_FILE%" "%OUTPUT%\client-license-public.key" >nul
if errorlevel 1 exit /b 1
tar -czf "dist\WorkMate-Server-Linux-%TARGET%.tar.gz" -C dist "server-linux-%TARGET%/workmate-admin" "server-linux-%TARGET%/workmate-admin.service" "server-linux-%TARGET%/admin.env.example" "server-linux-%TARGET%/Caddyfile.example" "server-linux-%TARGET%/install.sh" "server-linux-%TARGET%/backup.sh" "server-linux-%TARGET%/DEPLOY.md" "server-linux-%TARGET%/client-license-public.key"
if errorlevel 1 exit /b 1
echo [OK] dist\WorkMate-Server-Linux-%TARGET%.tar.gz
