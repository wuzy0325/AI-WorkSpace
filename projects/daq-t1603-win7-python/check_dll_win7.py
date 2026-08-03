# -*- coding: utf-8 -*-
"""检查 Qt5Core.dll 是否依赖了 Win7 不存在的 API。

主要嫌疑:
  - GetSystemTimePreciseAsFileTime (Win8+)
  - GetTickCount64 (Vista+,但 Win7 有)
  - SetThreadDescription (Win10+)
  - RegGetValueW (Win7 有)

如果发现 Win8+ API,需要换更老的 Qt 版本。
"""
import os
import re
import subprocess
import sys
from pathlib import Path


def check_dll(dll_path: str) -> None:
    """检查单个 DLL 的导入表。"""
    print(f"\n=== {Path(dll_path).name} ===")
    if not os.path.exists(dll_path):
        print(f"  文件不存在: {dll_path}")
        return

    # 用 dumpbin /imports (VS 自带) 或 objdump (MinGW)
    # 优先 dumpbin,失败回退到 Python pefile
    try:
        result = subprocess.run(
            ["dumpbin", "/imports", dll_path],
            capture_output=True, text=True, timeout=30
        )
        if result.returncode == 0:
            output = result.stdout
        else:
            raise FileNotFoundError("dumpbin failed")
    except (FileNotFoundError, subprocess.TimeoutExpired):
        # 回退到 pefile
        try:
            import pefile
        except ImportError:
            print("  需要 pefile: pip install pefile")
            return
        pe = pefile.PE(dll_path)
        output = ""
        for entry in pe.DIRECTORY_ENTRY_IMPORT:
            output += f"  {entry.dll.decode()}\n"
            for imp in entry.imports:
                if imp.name:
                    output += f"    {imp.name.decode()}\n"
        pe.close()

    # 检查 Win7 不存在的 API
    suspicious_apis = [
        "GetSystemTimePreciseAsFileTime",  # Win8+
        "SetThreadDescription",            # Win10 1607+
        "GetThreadDescription",            # Win10 1607+
        "RegGetValueW",  # Win7 有,但有些老 Win7 没装 SP1
        "SetProcessMitigationPolicy",      # Win8+
        "GetTempPath2W",                   # Win10+
    ]

    found = []
    for api in suspicious_apis:
        if re.search(rf"\b{api}\b", output):
            found.append(api)

    if found:
        print(f"  [WARNING] 发现 Win7 不支持的 API: {found}")
    else:
        print("  [OK] 未发现 Win7 不兼容的 API")


def main() -> None:
    venv = os.path.join(os.environ.get("LOCALAPPDATA", ""), "daq-t1603-venv")
    qt_dir = os.path.join(venv, "Lib", "site-packages", "PyQt5", "Qt5", "bin")

    if not os.path.exists(qt_dir):
        print(f"Qt bin 目录不存在: {qt_dir}")
        print("尝试找 Qt5Core.dll...")
        # 退而求其次,只检查 QtCore 模块附带的 dll
        pyqt5_dir = os.path.join(venv, "Lib", "site-packages", "PyQt5")
        for root, dirs, files in os.walk(pyqt5_dir):
            for f in files:
                if f.lower() == "qt5core.dll":
                    check_dll(os.path.join(root, f))
        return

    # 检查关键 Qt DLL
    for dll in ["Qt5Core.dll", "Qt5Gui.dll", "Qt5Widgets.dll", "Qt5Network.dll"]:
        check_dll(os.path.join(qt_dir, dll))


if __name__ == "__main__":
    main()
