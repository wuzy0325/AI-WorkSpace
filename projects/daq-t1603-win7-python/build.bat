@echo off
REM ============================================================
REM DAQ-T-1603 Win7 版 PyInstaller 打包脚本
REM
REM 前置条件:
REM   - Python 3.8.x (Win7 兼容)
REM   - 已执行 pip install -r requirements.txt
REM
REM 产物:
REM   dist\DAQ-T-1603-Win7\DAQ-T-1603-Win7.exe (单目录,可直接复制到 Win7 运行)
REM ============================================================

setlocal
cd /d %~dp0

echo [1/3] 清理旧产物...
if exist build rmdir /s /q build
if exist dist rmdir /s /q dist
if exist DAQ-T-1603-Win7.spec del /q DAQ-T-1603-Win7.spec

echo [2/3] 调用 PyInstaller 打包...
REM --windowed: 无控制台窗口
REM --name: 输出 exe 名称
REM --onefile: 单文件(首次启动稍慢,但部署最简单)
REM --collect-submodules PyQt5: 确保 Qt 所有子模块都被打包
pyinstaller ^
    --windowed ^
    --name "DAQ-T-1603-Win7" ^
    --onefile ^
    --collect-submodules PyQt5 ^
    main.py

if errorlevel 1 (
    echo.
    echo [错误] 打包失败,请检查上方日志
    pause
    exit /b 1
)

echo.
echo [3/3] 打包完成!
echo 产物路径: %~dp0dist\DAQ-T-1603-Win7.exe
echo 直接复制该 exe 到 Win7 机器即可运行(无需安装 Python)
pause
