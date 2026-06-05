三孔探针插值计算系统 v1.0.0
=============================

系统概述
--------
三孔探针插值计算系统是一款专业的风洞试验数据处理工具，
用于根据三孔探针的测量数据计算气流参数（攻角、马赫数、总压、静压）。

系统要求
--------
* 操作系统：Windows 10 / Windows 11（64位）
* 运行环境：Microsoft Edge WebView2 Runtime
  （Windows 10/11 通常已预装，如未安装请访问以下链接下载：
   https://developer.microsoft.com/zh-cn/microsoft-edge/webview2/）

文件说明
--------
* three-hole-interpolator.exe  - 主程序（双击运行）
* docs/用户说明书.html          - 用户说明书（浏览器打开）
* sample/customer_data.txt      - 示例测量数据文件
* sample/0.8.prb                - 示例校准文件
* 安装快捷方式.bat              - 一键创建桌面快捷方式

快速开始
--------
1. 双击 three-hole-interpolator.exe 启动程序
2. 点击"加载 PRB 文件"选择校准文件（sample/0.8.prb）
3. 输入或导入测量数据
4. 点击"执行插值计算"查看结果

更多详细操作说明请参阅 docs/用户说明书.html
