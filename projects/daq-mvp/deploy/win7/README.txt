DAQ MVP v0.1.0 - Windows 7 部署说明
=========================================

系统要求
--------
- Windows 7 SP1 x64 或更高版本（Win8/8.1/10/11 均支持）
- 内存：512 MB 以上
- 磁盘：50 MB 可用空间

文件说明
--------
daq-mvp.exe                      - 主程序（单文件 EXE）
MicrosoftEdgeWebview2Setup.exe   - WebView2 Runtime 安装程序（v109，最后支持 Win7 的版本）
setup.bat                        - 一键安装并启动（推荐首次使用）
run.bat                          - 直接启动（跳过 WebView2 检查）

安装步骤（首次使用）
--------------------
1. 右键 setup.bat → "以管理员身份运行"
2. 脚本会自动检测并安装 WebView2 Runtime
3. 安装完成后自动启动 DAQ MVP

如果 WebView2 安装失败：
- 手动运行 MicrosoftEdgeWebview2Setup.exe
- 或从微软官网下载：https://go.microsoft.com/fwlink/p/?LinkId=2124703

启动步骤（已安装 WebView2 后）
------------------------------
- 双击 run.bat
- 或直接双击 daq-mvp.exe

功能说明
--------
- Start 按钮：启动模拟数据采集（4 通道正弦波，1kHz 采样率）
- Stop 按钮：停止采集
- 波形区域：实时显示 4 通道波形（Canvas 渲染，60fps）
- 指标面板：批量数、采样数、速率、各通道最新值

注意事项
--------
- 本版本为 MVP（最小可行产品），使用模拟设备进行演示
- 模拟信号：4 通道正弦波（10/17/23/31 Hz），含 ±1% 噪声
- 下次启动无需再次安装 WebView2
- WebView2 Runtime 是 Edge 浏览器的底层组件，不影响系统稳定性

技术支持
--------
版本：0.1.0
构建时间：2026-05-15
