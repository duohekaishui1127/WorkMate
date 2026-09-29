@echo off
setlocal
cd /d "%~dp0.."

if not exist "dist\WorkMate.exe" (
  echo [ERROR] dist\WorkMate.exe does not exist.
  echo Run scripts\build-app.cmd first. If you use code signing, sign WorkMate.exe before building the installer.
  exit /b 1
)

set "ISCC="
for %%P in (
  "%ProgramFiles%\Inno Setup 7\ISCC.exe"
  "%ProgramFiles(x86)%\Inno Setup 7\ISCC.exe"
  "%ProgramFiles%\Inno Setup 6\ISCC.exe"
  "%ProgramFiles(x86)%\Inno Setup 6\ISCC.exe"
) do (
  if exist %%P if not defined ISCC set "ISCC=%%~P"
)

if not defined ISCC (
  echo [ERROR] Inno Setup 7/6 not found.
  echo Install Inno Setup, then run this script again.
  exit /b 1
)

"%ISCC%" installer\WorkMate.iss
if errorlevel 1 exit /b 1

echo [OK] Standard installer created in dist\
