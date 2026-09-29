@echo off
setlocal
cd /d "%~dp0.."
where signtool.exe >nul 2>nul || (
  echo [ERROR] signtool.exe not found.
  exit /b 1
)

signtool.exe verify /pa /all /v dist\WorkMate.exe
if errorlevel 1 exit /b 1
for %%F in (dist\WorkMate-Setup-*.exe) do (
  signtool.exe verify /pa /all /v "%%F"
  if errorlevel 1 exit /b 1
)
echo [OK] Signature verification passed.
