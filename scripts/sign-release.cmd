@echo off
setlocal enabledelayedexpansion
cd /d "%~dp0.."

where signtool.exe >nul 2>nul || (
  echo [ERROR] signtool.exe not found. Install Windows SDK.
  exit /b 1
)
if not exist "dist\WorkMate.exe" (
  echo [ERROR] dist\WorkMate.exe not found. Run scripts\build-app.cmd first.
  exit /b 1
)
if "%WORKMATE_TIMESTAMP_URL%"=="" (
  echo [ERROR] WORKMATE_TIMESTAMP_URL is not set.
  echo Use the RFC3161 timestamp URL provided by your certificate provider.
  exit /b 1
)

if not "%WORKMATE_CERT_PFX%"=="" goto sign_app_pfx
if not "%WORKMATE_CERT_THUMBPRINT%"=="" goto sign_app_store

echo [ERROR] Set either WORKMATE_CERT_PFX or WORKMATE_CERT_THUMBPRINT.
exit /b 1

:sign_app_pfx
call :signone_pfx "dist\WorkMate.exe"
if errorlevel 1 exit /b 1
goto build_setup

:sign_app_store
call :signone_store "dist\WorkMate.exe"
if errorlevel 1 exit /b 1
goto build_setup

:build_setup
call scripts\build-installer.cmd
if errorlevel 1 exit /b 1

for %%F in (dist\WorkMate-Setup-*.exe) do (
  if not "%WORKMATE_CERT_PFX%"=="" (
    call :signone_pfx "%%F"
  ) else (
    call :signone_store "%%F"
  )
  if errorlevel 1 exit /b 1
)
call scripts\verify-signature.cmd
exit /b %errorlevel%

:signone_pfx
signtool.exe sign /fd SHA256 /f "%WORKMATE_CERT_PFX%" /p "%WORKMATE_CERT_PASSWORD%" /tr "%WORKMATE_TIMESTAMP_URL%" /td SHA256 /v %1
exit /b %errorlevel%

:signone_store
signtool.exe sign /fd SHA256 /sha1 "%WORKMATE_CERT_THUMBPRINT%" /tr "%WORKMATE_TIMESTAMP_URL%" /td SHA256 /v %1
exit /b %errorlevel%
