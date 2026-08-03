# -*- coding: utf-8 -*-
"""
DAQ-T-1603 Win7 版冒烟测试脚本

测试内容(不依赖真实设备):
  1. UI 能否启动(QApplication + MainWindow 实例化)
  2. 协议解析逻辑正确性(模拟 64 字节二进制帧)
  3. 端到端连接测试(模拟 TCP server + 真实 T1603Device)

运行方式:
  python smoke_test.py

前置条件:
  - pip install PyQt5
"""

from __future__ import annotations

import os
import struct
import sys
import threading
import time
from typing import List, Optional

# 把当前目录加入 path,确保能 import main
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

# ======== 测试 1:协议解析逻辑 ========
def test_frame_parsing() -> None:
    """验证 16 × float32 LE + 反转逻辑。

    构造一个已知温度序列,打包成 64 字节,解析后应与预期一致。
    """
    print("[Test 1] 协议解析逻辑...")
    # 模拟 16 通道温度:CH0=25.00, CH1=25.50, ... CH15=32.50
    expected = [25.00 + i * 0.50 for i in range(16)]
    # 设备发送顺序是 CH15→CH0,所以打包前要反转
    device_order = list(reversed(expected))
    frame = struct.pack("<16f", *device_order)
    assert len(frame) == 64, f"帧长度应为 64,实际 {len(frame)}"

    # 模拟 T1603Device.read_frame 的解析逻辑
    temps = list(struct.unpack("<16f", frame))
    temps.reverse()
    for i, (got, want) in enumerate(zip(temps, expected)):
        assert abs(got - want) < 0.001, f"CH{i:02d} 解析错误: got={got}, want={want}"
    print(f"  PASS: 16 通道解析正确,示例值 {[round(t, 2) for t in temps[:4]]}...")


# ======== 测试 2:模拟 TCP server ========
class MockT1603Server:
    """模拟 DAQ-T-1603 设备的 TCP server。

    行为:
      - 接受连接后,响应所有 SCPI 命令(返回 "A\\n" 作为 ACK)
      - 收到 @f0 启动采集后,持续发送 64 字节二进制帧
      - 收到 @f1 停止采集
    """

    def __init__(self, host: str = "127.0.0.1", port: int = 19000) -> None:
        self.host = host
        self.port = port
        self._sock: Optional[socket.socket] = None
        self._client: Optional[socket.socket] = None
        self._acquiring = False
        self._thread: Optional[threading.Thread] = None
        self._stop = threading.Event()

    def start(self) -> None:
        import socket
        self._sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self._sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        self._sock.bind((self.host, self.port))
        self._sock.listen(1)
        self._thread = threading.Thread(target=self._serve, daemon=True)
        self._thread.start()

    def stop(self) -> None:
        self._stop.set()
        self._acquiring = False
        if self._client:
            try:
                self._client.close()
            except OSError:
                pass
        if self._sock:
            try:
                self._sock.close()
            except OSError:
                pass

    def _serve(self) -> None:
        import socket
        try:
            self._sock.settimeout(1.0)
            while not self._stop.is_set():
                try:
                    client, _ = self._sock.accept()
                except socket.timeout:
                    continue
                self._client = client
                self._handle_client(client)
        except OSError:
            pass

    def _handle_client(self, client: socket.socket) -> None:
        import socket
        client.settimeout(0.2)
        buf = bytearray()
        while not self._stop.is_set():
            # 读取命令
            try:
                data = client.recv(1024)
                if not data:
                    break
                buf.extend(data)
                # 按命令边界处理(命令以换行或无换行的文本结尾)
                while b"\n" in buf or b"@f0" in buf or b"@f1" in buf or b"@fe" in buf or b"@f3" in buf:
                    line_end = -1
                    for sep in [b"\n", b"@"]:
                        idx = buf.find(sep, 1) if sep == b"@" else buf.find(sep)
                        if idx > 0 and (line_end < 0 or idx < line_end):
                            line_end = idx
                    if line_end < 0:
                        line_end = len(buf)
                    cmd = bytes(buf[:line_end]).decode(errors="ignore").strip()
                    del buf[:line_end]
                    if not cmd:
                        continue
                    self._handle_cmd(client, cmd)
            except socket.timeout:
                # 没有命令时,如果正在采集就发数据帧
                if self._acquiring:
                    self._send_frame(client)
                continue
            except OSError:
                break

    def _handle_cmd(self, client: socket.socket, cmd: str) -> None:
        if cmd.startswith("@fe"):
            # 配置命令,回 ACK
            client.sendall(b"A\n")
        elif cmd.startswith("@f3"):
            # 热电偶配置,回 ACK
            client.sendall(b"A\n")
        elif cmd.startswith("@f0"):
            # 启动采集
            self._acquiring = True
        elif cmd.startswith("@f1"):
            # 停止采集
            self._acquiring = False
        # 其他命令忽略

    def _send_frame(self, client: socket.socket) -> None:
        """发送一帧 64 字节二进制数据(16 × float32 LE,CH15→CH0 顺序)。"""
        temps_ch0_to_ch15 = [25.00 + (i % 16) * 0.50 for i in range(16)]
        device_order = list(reversed(temps_ch0_to_ch15))
        frame = struct.pack("<16f", *device_order)
        try:
            client.sendall(frame)
        except OSError:
            self._acquiring = False


# ======== 测试 3:端到端连接 ========
def test_end_to_end() -> None:
    """验证 T1603Device 能连接模拟 server 并读出数据。"""
    print("[Test 3] 端到端连接(模拟设备)...")
    import socket
    from main import T1603Device, CHANNEL_COUNT

    server = MockT1603Server(port=19001)
    server.start()
    time.sleep(0.3)  # 等 server 起来

    try:
        device = T1603Device(host="127.0.0.1", port=19001)
        device.connect()
        assert device.connected, "连接失败"
        print("  PASS: 连接成功")

        # 设置热电偶配置(应不报错)
        device.set_thermocouple_types("K" * CHANNEL_COUNT)
        print("  PASS: 热电偶配置应用成功")

        # 启动采集,读 3 帧
        device.start_acquisition()
        assert device.acquiring, "启动采集失败"
        frames_read = 0
        deadline = time.time() + 5.0
        while frames_read < 3 and time.time() < deadline:
            temps = device.read_frame()
            if temps is not None:
                frames_read += 1
                print(f"  PASS: 收到第 {frames_read} 帧,前 4 通道 = {[round(t, 2) for t in temps[:4]]}")
        assert frames_read >= 1, f"5 秒内未收到任何数据帧,实际收到 {frames_read} 帧"

        device.stop_acquisition()
        assert not device.acquiring, "停止采集失败"
        print("  PASS: 停止采集成功")

        device.disconnect()
        assert not device.connected, "断开连接失败"
        print("  PASS: 断开连接成功")
    finally:
        server.stop()


# ======== 测试 4:UI 能否实例化 ========
def test_ui_instantiation() -> None:
    """验证 MainWindow 能否实例化(不显示窗口)。

    需要 PyQt5,若未安装则跳过。
    """
    print("[Test 4] UI 实例化...")
    try:
        from PyQt5.QtWidgets import QApplication
        os.environ["QT_QPA_PLATFORM"] = "offscreen"  # 无显示环境也能跑
        app = QApplication.instance() or QApplication(sys.argv)
        from main import MainWindow
        window = MainWindow()
        assert window.windowTitle().startswith("DAQ-T-1603"), "窗口标题错误"
        assert len(window.channel_labels) == 16, f"应有 16 个通道标签,实际 {len(window.channel_labels)}"
        assert len(window.tc_combos) == 16, f"应有 16 个热电偶下拉框,实际 {len(window.tc_combos)}"
        print("  PASS: UI 实例化成功(16 通道标签 + 16 热电偶下拉框)")
        # 测试数据更新槽
        window._on_data_received([25.0 + i for i in range(16)])
        assert window.channel_labels[0].text() == "25.00", f"CH01 应显示 25.00,实际 {window.channel_labels[0].text()}"
        print("  PASS: 数据更新槽正常工作")
    except ImportError as e:
        print(f"  SKIP: PyQt5 未安装({e}),跳过 UI 测试")
        return
    finally:
        os.environ.pop("QT_QPA_PLATFORM", None)


# ======== 主入口 ========
def main() -> int:
    print("=" * 60)
    print("DAQ-T-1603 Win7 版冒烟测试")
    print("=" * 60)
    try:
        test_frame_parsing()
        test_end_to_end()
        test_ui_instantiation()
        print("=" * 60)
        print("ALL TESTS PASSED")
        print("=" * 60)
        return 0
    except AssertionError as e:
        print(f"\n[FAIL] {e}")
        return 1
    except Exception as e:
        print(f"\n[ERROR] {e}")
        import traceback
        traceback.print_exc()
        return 2


if __name__ == "__main__":
    sys.exit(main())
