@echo off
chcp 936 >nul
title Create Shortcut

echo ========================================
echo  Create Desktop Shortcut
echo ========================================
echo.

set "APP_DIR=%~dp0"
set "APP_DIR=%APP_DIR:~0,-1%"
set "SHORTCUT=%USERPROFILE%\Desktop\Three-Hole-Probe.lnk"

powershell -Command "$ws=New-Object -ComObject WScript.Shell;$sc=$ws.CreateShortcut('%SHORTCUT%');$sc.TargetPath='%APP_DIR%\three-hole-interpolator.exe';$sc.WorkingDirectory='%APP_DIR%';$sc.Description='Three-Hole Probe Interpolation System v1.0.0';$sc.Save();Write-Host 'Shortcut created on desktop!'"

echo.
echo Shortcut path: %SHORTCUT%
echo.
echo You can move the program folder to any location,
echo then run this script again to update the shortcut.
echo.
pause
