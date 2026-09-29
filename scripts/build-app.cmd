@echo off
setlocal enabledelayedexpansion
cd /d "%~dp0.."

where go >nul 2>nul || (
  echo [ERROR] Go 1.23+ not found.
  exit /b 1
)

if not exist dist mkdir dist

where go-winres >nul 2>nul
if errorlevel 1 (
  echo [INFO] go-winres not found. Installing v0.3.3...
  go install github.com/tc-hib/go-winres@v0.3.3
  if errorlevel 1 (
    echo [ERROR] Failed to install go-winres.
    echo        You can still build manually, but icon/manifest/version resources will be missing.
    exit /b 1
  )
  set "PATH=%USERPROFILE%\go\bin;!PATH!"
)

pushd winres
go-winres make --in winres.json --arch amd64 --out ..\rsrc
if errorlevel 1 (
  popd
  exit /b 1
)
popd

set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=0

go vet ./...
if errorlevel 1 exit /b 1

set "ADMIN_PUBLIC_KEY="
set "ADMIN_KEY_FILE=admin-data\license-public.key"
if defined WORKMATE_ADMIN_PUBLIC_KEY_FILE set "ADMIN_KEY_FILE=%WORKMATE_ADMIN_PUBLIC_KEY_FILE%"
if exist "!ADMIN_KEY_FILE!" (
  set /p ADMIN_PUBLIC_KEY=<"!ADMIN_KEY_FILE!"
  echo [INFO] Including admin server public key.
)

go build -trimpath -buildvcs=false -ldflags="-H windowsgui -s -w -X main.onlineLicensePublicKeyB64=!ADMIN_PUBLIC_KEY!" -o dist\WorkMate.exe .
if errorlevel 1 exit /b 1

copy /y assets\commerce.json dist\commerce.json >nul
if errorlevel 1 exit /b 1
copy /y assets\payment_qr.png dist\payment_qr.png >nul
if errorlevel 1 exit /b 1
copy /y WorkMate.ico dist\WorkMate.ico >nul
if errorlevel 1 exit /b 1

echo [OK] dist\WorkMate.exe
