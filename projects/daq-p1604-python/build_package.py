# -*- coding: utf-8 -*-
"""DAQ-P-1604 Python 客户交付包打包脚本。

将 lib + demo + 测试 + 文档打成 zip 交付包，排除 __pycache__ 等无关文件。
用法: python build_package.py
产物: dist/daq-p1604-python-<version>.zip
"""

from __future__ import annotations

import zipfile
from pathlib import Path

ROOT = Path(__file__).resolve().parent

EXCLUDE_SUFFIXES = (".pyc", ".pyo")
EXCLUDE_DIRS = {"__pycache__", "build", "dist", ".venv", "venv", "env", ".git", ".idea", ".vscode"}

INCLUDE_FILES = {
    "README.md",
    "pyproject.toml",
}

INCLUDE_DIRS = {
    "daqp1604",
    "tests",
    "demo.py",
}


def _collect(root: Path) -> list:
    files = []
    for rel in INCLUDE_FILES:
        p = root / rel
        if p.is_file():
            files.append(p)
    for rel in INCLUDE_DIRS:
        p = root / rel
        if p.is_file():
            files.append(p)
            continue
        if p.is_dir():
            for f in sorted(p.rglob("*")):
                if not f.is_file():
                    continue
                if any(part in EXCLUDE_DIRS for part in f.parts):
                    continue
                if f.suffix in EXCLUDE_SUFFIXES:
                    continue
                files.append(f)
    return files


def main() -> None:
    dist = ROOT / "dist"
    dist.mkdir(exist_ok=True)
    version = "1.0.0"
    zip_name = dist / f"daq-p1604-python-{version}.zip"
    files = _collect(ROOT)
    with zipfile.ZipFile(zip_name, "w", zipfile.ZIP_DEFLATED) as zf:
        for f in files:
            arcname = f.relative_to(ROOT)
            zf.write(f, arcname.as_posix())
            print(f"  + {arcname.as_posix()}")
    print(f"\n完成: {zip_name} ({len(files)} 个文件)")


if __name__ == "__main__":
    main()
