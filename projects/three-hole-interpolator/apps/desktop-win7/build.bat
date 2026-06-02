@echo off
chcp 65001 >nul 2>&1
setlocal enabledelayedexpansion

echo ========================================
echo   三孔探针插值计算 (Win7版) 构建脚本
echo ========================================
echo.

set "ROOT_DIR=%~dp0"
set "FRONTEND_DIR=%ROOT_DIR%frontend"
set "OUTPUT_DIR=%ROOT_DIR%build\bin"
set "APP_NAME=三孔探针插值计算_Win7版"

echo [1/4] 安装前端依赖...
cd /d "%FRONTEND_DIR%"
call npm install --no-audit --no-fund
if errorlevel 1 (
    echo 前端依赖安装失败！
    pause
    exit /b 1
)
echo.

echo [2/4] 构建前端...
call npm run build
if errorlevel 1 (
    echo 前端构建失败！
    pause
    exit /b 1
)
echo.

echo [3/4] 整理Go模块依赖...
cd /d "%ROOT_DIR%"
go mod tidy
if errorlevel 1 (
    echo Go模块整理失败！
    pause
    exit /b 1
)
echo.

echo [4/4] 编译Go程序...
if not exist "%OUTPUT_DIR%" mkdir "%OUTPUT_DIR%"
go build -buildvcs=false -ldflags="-s -w" -o "%OUTPUT_DIR%\三孔探针插值计算.exe" .
if errorlevel 1 (
    echo Go编译失败！
    pause
    exit /b 1
)
echo.

echo ========================================
echo   构建成功！
echo   输出目录: %OUTPUT_DIR%
echo ========================================
echo.

set "RELEASE_DIR=%ROOT_DIR%release\%APP_NAME%_v1.0.0"
if exist "%RELEASE_DIR%" rmdir /s /q "%RELEASE_DIR%"
mkdir "%RELEASE_DIR%"
mkdir "%RELEASE_DIR%\docs"
mkdir "%RELEASE_DIR%\sample"

copy "%OUTPUT_DIR%\三孔探针插值计算.exe" "%RELEASE_DIR%\" >nul

if exist "%ROOT_DIR%..\..\..\docs\用户说明书.html" (
    copy "%ROOT_DIR%..\..\..\docs\用户说明书.html" "%RELEASE_DIR%\docs\" >nul
) else if exist "%ROOT_DIR%..\..\..\..\docs\用户说明书.html" (
    copy "%ROOT_DIR%..\..\..\..\docs\用户说明书.html" "%RELEASE_DIR%\docs\" >nul
)

if exist "%ROOT_DIR%..\..\..\shared\algorithms\go\threehole\interpolation\testdata\0.8.prb" (
    copy "%ROOT_DIR%..\..\..\shared\algorithms\go\threehole\interpolation\testdata\0.8.prb" "%RELEASE_DIR%\sample\" >nul
)
if exist "%ROOT_DIR%..\..\..\shared\algorithms\go\threehole\interpolation\testdata\customer_data.txt" (
    copy "%ROOT_DIR%..\..\..\shared\algorithms\go\threehole\interpolation\testdata\customer_data.txt" "%RELEASE_DIR%\sample\" >nul
)

echo [说明] 已生成发布目录: %RELEASE_DIR%
echo.
echo 使用方式:
echo   1. 将 release 目录打包发送给客户
echo   2. 客户双击运行 三孔探针插值计算.exe
echo   3. 系统自动打开浏览器，在浏览器中操作
echo   4. 关闭命令行窗口即可停止服务
echo.

pause
