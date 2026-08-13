# -*- coding: utf-8 -*-
"""DAQ-P-1604 交互式控制台 Demo。

菜单驱动的控制台程序，覆盖完整生命周期：
  1. 连接
  2. 开始采集（实时显示 18 通道数据）
  3. 停止采集
  4. 断开
  5. 退出

用法::

    python demo.py [host] [port]

默认 host=192.168.3.101 port=9000。
"""

from __future__ import annotations

import sys
import time
from typing import List, Optional

from daqp1604 import DAQP1604Device, DAQP1604Error, scan_devices

CHANNEL_NAMES = [f"CH{i:02d}" for i in range(1, 17)] + ["大气压", "大气温"]


class DemoApp:
    """控制台 Demo 主逻辑。"""

    def __init__(self, host: str, port: int) -> None:
        self.host = host
        self.port = port
        self.device = DAQP1604Device(host, port)
        self.last_channels: Optional[List[float]] = None
        self.frame_count = 0

    # ---- 数据回调（在后台读线程中被调用）----
    def _on_data(self, payload: dict) -> None:
        self.last_channels = payload["channels"]
        self.frame_count += 1

    # ---- 各操作 ----
    def connect(self) -> None:
        print(f"连接 {self.host}:{self.port} ...")
        self.device.on_data = self._on_data
        try:
            self.device.connect()
        except DAQP1604Error as exc:
            print(f"  连接失败: {exc}")
            return
        print(f"  已连接，硬件单位: {self.device.unit}")

    def disconnect(self) -> None:
        if not self.device.connected:
            print("  当前未连接")
            return
        self.stop_acquisition()
        self.device.disconnect()
        print("  已断开")

    def start_acquisition(self) -> None:
        if not self.device.connected:
            print("  请先连接设备")
            return
        if self.device.acquiring:
            print("  采集中，无需重复启动")
            return
        period_ms = 100
        try:
            self.device.start_acquisition(period_ms=period_ms)
        except DAQP1604Error as exc:
            print(f"  启动失败: {exc}")
            return
        print(f"  采集已启动 (周期 {period_ms}ms)，按任意键停止...")

        # 阻塞显示实时数据，按回车返回菜单
        self.frame_count = 0
        self.last_channels = None
        input_hint_printed = False
        start = time.time()
        while self.device.acquiring:
            if self.last_channels is not None:
                elapsed = time.time() - start
                print(f"[#{self.frame_count}  t={elapsed:6.2f}s]  " + self._format_channels(self.last_channels))
                self.last_channels = None
                input_hint_printed = False
            # 检查是否有回车（非阻塞读）
            if self._key_pressed():
                break
            if not input_hint_printed and time.time() - start > 0.2:
                input_hint_printed = True
            time.sleep(0.02)

        self.stop_acquisition()

    def stop_acquisition(self) -> None:
        if not self.device.acquiring:
            return
        try:
            self.device.stop_acquisition()
        except DAQP1604Error as exc:
            print(f"  停止失败: {exc}")
            return
        print("  采集已停止")

    def scan(self) -> None:
        print("扫描局域网设备 (3s) ...")
        try:
            devices = scan_devices(timeout=3.0)
        except Exception as exc:  # noqa: BLE001
            print(f"  扫描失败: {exc}")
            return
        if not devices:
            print("  未发现设备")
            return
        for d in devices:
            print(f"  {d.ip}:{d.port}  序列号={d.serial}  固件={d.firmware}")

    # ---- 工具 ----
    @staticmethod
    def _format_channels(channels: List[float]) -> str:
        parts = []
        for i, (name, val) in enumerate(zip(CHANNEL_NAMES, channels)):
            if i < 16:
                parts.append(f"{name}={val:.3f}")
            else:
                parts.append(f"{name}={val:.2f}")
        return "  ".join(parts)

    def _key_pressed(self) -> bool:
        import msvcrt  # Windows 平台专用
        if msvcrt.kbhit():
            msvcrt.getch()
            return True
        return False

    def run(self) -> None:
        """菜单主循环。"""
        while True:
            status = ("采集中" if self.device.acquiring
                      else "已连接" if self.device.connected
                      else "未连接")
            print()
            print(f"===== DAQ-P-1604 Demo @ {self.host}:{self.port} [状态: {status}] =====")
            print("  1. 连接")
            print("  2. 开始采集")
            print("  3. 停止采集")
            print("  4. 断开")
            print("  5. 扫描设备")
            print("  0. 退出")
            choice = input("请选择: ").strip()

            if choice == "1":
                self.connect()
            elif choice == "2":
                self.start_acquisition()
            elif choice == "3":
                self.stop_acquisition()
            elif choice == "4":
                self.disconnect()
            elif choice == "5":
                self.scan()
            elif choice == "0":
                self.disconnect()
                print("再见")
                return
            else:
                print("无效选择")


def main() -> None:
    host = sys.argv[1] if len(sys.argv) > 1 else "192.168.3.101"
    port = int(sys.argv[2]) if len(sys.argv) > 2 else 9000
    DemoApp(host, port).run()


if __name__ == "__main__":
    main()
