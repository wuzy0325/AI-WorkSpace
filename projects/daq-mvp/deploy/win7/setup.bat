@echo off
setlocal enabledelayedexpansion
title DAQ MVP Setup

echo ============================================
echo   DAQ MVP v0.1.0 - Setup & Launch
echo ============================================
echo.

set WEBVIEW2_OK=0

REM Check if WebView2 is already installed (Evergreen)
reg query "HKLM\SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" /v pv >nul 2>&1
if !errorlevel! equ 0 set WEBVIEW2_OK=1
reg query "HKLM\SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" /v pv >nul 2>&1
if !errorlevel! equ 0 set WEBVIEW2_OK=1

if !WEBVIEW2_OK! equ 1 (
    echo [1/2] WebView2 Runtime: INSTALLED
) else (
    echo [1/2] Installing WebView2 Runtime...
    "%~dp0MicrosoftEdgeWebview2Setup.exe" /silent /install
    if !errorlevel! equ 0 (
        echo [1/2] WebView2 Runtime: OK
    ) else (
        echo [ERROR] WebView2 install failed. Try running as Administrator.
        echo Download manually: https://go.microsoft.com/fwlink/p/?LinkId=2124703
        pause
        exit /b 1
    )
)

echo [2/2] Starting DAQ MVP...
start "" "%~dp0daq-mvp.exe"
exit
