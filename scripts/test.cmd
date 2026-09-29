@echo off
setlocal
cd /d "%~dp0.."
where go >nul 2>nul || (
  echo [ERROR] Go 1.23+ not found.
  exit /b 1
)
set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=0
go vet ./...
if errorlevel 1 exit /b 1
go test -v ./...
exit /b %errorlevel%
