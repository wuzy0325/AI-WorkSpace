@echo off
chcp 65001 >nul
setlocal enabledelayedexpansion

echo ========================================
echo   三孔探针插值计算 (Win7版) 构建
echo ========================================
echo.

set "ROOT_DIR=%~dp0"

echo [1/5] 准备共享模块（排除测试文件）...
set "THREEHOLE_SRC=%ROOT_DIR%..\..\shared\algorithms\go\threehole"
set "THREEHOLE_DST=%ROOT_DIR%build\threehole_no_tests"
if exist "%THREEHOLE_DST%" rmdir /s /q "%THREEHOLE_DST%"
mkdir "%THREEHOLE_DST%\interpolation" >nul 2>&1
xcopy "%THREEHOLE_SRC%\interpolation\*.go" "%THREEHOLE_DST%\interpolation\" /Y >nul 2>&1
if exist "%THREEHOLE_DST%\interpolation\*_test.go" del "%THREEHOLE_DST%\interpolation\*_test.go" /q >nul 2>&1
echo module ai-workspace/shared/algorithms/go/threehole > "%THREEHOLE_DST%\go.mod"
echo. >> "%THREEHOLE_DST%\go.mod"
echo go 1.20 >> "%THREEHOLE_DST%\go.mod"

echo [2/5] 复制 Walk 库到本地（打 Win7 兼容补丁）...
set "WALK_SRC=%ROOT_DIR%build\patched_walk"
if not exist "%WALK_SRC%" (
    echo [ERROR] patched_walk 目录不存在！
    echo   请确认 %WALK_SRC% 存在
    pause
    exit /b 1
)

echo [3/5] 切换 go.mod 为 Win7 兼容模式...
cd /d "%ROOT_DIR%"
copy go.mod go.mod.dev.bak >nul 2>&1
(
    echo module three-hole-interpolator/apps/desktop-win7-walk
    echo.
    echo go 1.20
    echo.
    echo require (
    echo 	ai-workspace/shared/algorithms/go/threehole v0.0.0
    echo 	github.com/lxn/walk v0.0.0-20210112085537-c389da54e794
    echo )
    echo.
    echo require (
    echo 	github.com/lxn/win v0.0.0-20210218163916-a377121e959e // indirect
    echo 	golang.org/x/sys v0.0.0-20201018230417-eeed37f84f13 // indirect
    echo 	gopkg.in/Knetic/govaluate.v3 v3.0.0 // indirect
    echo )
    echo.
    echo replace (
    echo     ai-workspace/shared/algorithms/go/threehole =^> ./build/threehole_no_tests
    echo     github.com/lxn/walk =^> ./build/patched_walk
    echo )
) > go.mod.tmp
move /y go.mod.tmp go.mod >nul

echo [4/5] 使用 Go 1.20 编译（Win7 兼容）...
where go1.20.14 >nul 2>&1
if errorlevel 1 (
    echo [ERROR] 需要 Go 1.20.14，请执行:
    echo   go install golang.org/dl/go1.20.14@latest
    echo   go1.20.14 download
    if exist go.mod.dev.bak copy /y go.mod.dev.bak go.mod >nul
    pause
    exit /b 1
)

go1.20.14 clean -cache >nul 2>&1
del go.sum >nul 2>&1
go1.20.14 mod tidy >nul
if errorlevel 1 (
    echo [ERROR] 模块整理失败
    if exist go.mod.dev.bak copy /y go.mod.dev.bak go.mod >nul
    pause
    exit /b 1
)

if not exist "%ROOT_DIR%build\bin" mkdir "%ROOT_DIR%build\bin"
go1.20.14 build -buildvcs=false -ldflags="-s -w -H windowsgui" -o "%ROOT_DIR%build\bin\三孔探针插值计算_Win7版.exe" .
if errorlevel 1 (
    echo [ERROR] 编译失败
    if exist go.mod.dev.bak copy /y go.mod.dev.bak go.mod >nul
    pause
    exit /b 1
)

echo [5/5] 复制外置 manifest 文件...
copy "%ROOT_DIR%app.manifest" "%ROOT_DIR%build\bin\三孔探针插值计算_Win7版.exe.manifest" /Y >nul

rem 恢复 go.mod
if exist go.mod.dev.bak copy /y go.mod.dev.bak go.mod >nul
del go.mod.dev.bak >nul 2>&1
del go.mod.tmp >nul 2>&1

echo.
echo ========================================
echo   构建成功！
echo   输出:
echo     %ROOT_DIR%build\bin\三孔探针插值计算_Win7版.exe
echo     %ROOT_DIR%build\bin\三孔探针插值计算_Win7版.exe.manifest
echo.
echo   使用说明:
echo   将 build\bin 目录下的两个文件一起给客户
echo   双击 exe 即可运行
echo ========================================
echo.
pause
