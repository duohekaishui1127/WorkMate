@echo off
setlocal
cd /d "%~dp0.."
where go >nul 2>nul || (
  echo [ERROR] Go 1.23+ not found.
  exit /b 1
)
if not exist dist mkdir dist
set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=0
go build -trimpath -buildvcs=false -ldflags="-s -w" -o dist\WorkMateAdmin.exe ./cmd/workmate-admin
if errorlevel 1 exit /b 1
echo [OK] dist\WorkMateAdmin.exe
