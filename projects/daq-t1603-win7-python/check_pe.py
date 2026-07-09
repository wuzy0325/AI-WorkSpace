# -*- coding: utf-8 -*-
"""检查 PE 文件的 SubsystemVersion,判定 Win7 兼容性。

SubsystemVersion 是 PE Optional Header 中的字段,Windows 加载器用它判定
最低系统版本。值 <= 6.01 表示 Win7 SP1 可运行,6.02 需要 Win8+,6.03 需要 Win8.1+。

Python 3.8 编译的 exe 默认 SubsystemVersion = 6.01(Win7 兼容)
Python 3.9+ 默认 6.02(Win8+),3.10+ 默认 6.02
"""
import struct
import sys


def check_pe_subsystem(path: str) -> None:
    with open(path, "rb") as f:
        data = f.read()

    # DOS header: PE 头偏移在 0x3C
    pe_offset = struct.unpack_from("<I", data, 0x3C)[0]
    # 校验 PE 标记
    if data[pe_offset:pe_offset + 4] != b"PE\x00\x00":
        print("ERROR: 不是有效的 PE 文件")
        return

    # Optional Header 起始位置 = PE 标记(4) + COFF header(20)
    opt_header = pe_offset + 4 + 20
    magic = struct.unpack_from("<H", data, opt_header)[0]
    is_pe32_plus = (magic == 0x20B)

    print(f"PE Magic: 0x{magic:03X} ({'PE32+ 64-bit' if is_pe32_plus else 'PE32 32-bit'})")

    # SubsystemVersion 偏移
    # PE32:  Optional Header + 40 (ImageBase 是 4 字节)
    # PE32+: Optional Header + 48 (ImageBase 是 8 字节)
    sv_offset = opt_header + 48 if is_pe32_plus else opt_header + 40
    osv_offset = opt_header + 40 if is_pe32_plus else opt_header + 32

    major = struct.unpack_from("<H", data, sv_offset)[0]
    minor = struct.unpack_from("<H", data, sv_offset + 2)[0]
    os_major = struct.unpack_from("<H", data, osv_offset)[0]
    os_minor = struct.unpack_from("<H", data, osv_offset + 2)[0]

    print(f"PE SubsystemVersion: {major}.{minor:02d}")
    print(f"PE OSVersion:        {os_major}.{os_minor:02d}")

    # 判定
    if major < 6 or (major == 6 and minor <= 1):
        print("结论: WIN7_COMPATIBLE - 可在 Win7 SP1+ 运行")
    elif major == 6 and minor == 2:
        print("结论: WIN8_REQUIRED - 需要 Win8+")
    else:
        print("结论: WIN81_REQUIRED - 需要 Win8.1+")


if __name__ == "__main__":
    exe = sys.argv[1] if len(sys.argv) > 1 else \
        r"c:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace\projects\daq-t1603-win7-python\dist\DAQ-T-1603-Win7.exe"
    check_pe_subsystem(exe)
