@echo off
rem 双击或在 cmd 中运行：scripts\dev-windows.cmd [参数...]
rem 参数直接转给 dev-windows.ps1，例如：scripts\dev-windows.cmd -Clean -Cloud
setlocal
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0dev-windows.ps1" %*
set EXITCODE=%ERRORLEVEL%
if not "%EXITCODE%"=="0" (
  echo.
  echo 启动失败（退出码 %EXITCODE%）。按任意键关闭窗口。
  pause >nul
)
exit /b %EXITCODE%
