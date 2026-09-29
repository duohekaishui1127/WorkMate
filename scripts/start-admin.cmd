@echo off
setlocal
cd /d "%~dp0.."
if not exist dist\WorkMateAdmin.exe (
  call scripts\build-admin.cmd
  if errorlevel 1 exit /b 1
)
dist\WorkMateAdmin.exe -data-dir "%CD%\admin-data" %*
exit /b %errorlevel%
