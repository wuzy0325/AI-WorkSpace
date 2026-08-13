# -*- coding: utf-8 -*-
"""DAQ-P-1604 客户端库端到端测试（基于模拟设备）。

运行：python tests/test_mock_server.py
覆盖：连接握手、单位同步、采集启停、数据帧解析（逆序校验）、轮询、断开。
"""

from __future__ import annotations

import sys
import time
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from daqp1604 import DAQP1604Device, CommandError  # noqa: E402

from mock_server import MockP1604Server  # noqa: E402


def _collect(dev: DAQP1604Device, duration: float) -> list:
    frames = []

    def on_data(payload):
        frames.append(payload)

    dev.on_data = on_data
    end = time.monotonic() + duration
    while time.monotonic() < end:
        time.sleep(0.02)
    return frames


def test_full_flow() -> None:
    server = MockP1604Server(coeff=6.894757, period_ms=50)  # kPa
    server.start()
    try:
        dev = DAQP1604Device("127.0.0.1", server.port)

        # 1. 连接握手
        dev.connect()
        assert dev.connected, "connect() 后应已连接"
        assert "w1601" in server.commands, "连接后应发送 w1601"
        assert dev.unit == "kPa", f"单位应同步为 kPa, got {dev.unit!r}"

        # 2. 启动采集
        dev.start_acquisition(period_ms=50)
        assert dev.acquiring, "start_acquisition 后应处于采集中"
        assert "c 00 1 FFFF 1 50 7 0" in server.commands
        assert "c 05 1 0810" in server.commands
        assert "c 01 1" in server.commands

        # 3. 采集数据（0.3s）
        frames = _collect(dev, 0.3)
        assert frames, "0.3s 内应收到数据帧"

        # 4. 帧解析校验：CH16..CH1 逆序 → 正序
        first = frames[0]["channels"]
        assert len(first) == 18, f"应 18 通道, got {len(first)}"
        # 模拟设备发送 CH16=1600.0, ..., CH1=100.0
        assert abs(first[0] - 100.0) < 0.01, f"CH1 应为 100, got {first[0]}"
        assert abs(first[15] - 1600.0) < 0.01, f"CH16 应为 1600, got {first[15]}"
        assert abs(first[16] - 101325.0) < 0.01, f"CH17 大气压应为 101325, got {first[16]}"
        assert abs(first[17] - 25.5) < 0.01, f"CH18 温度应为 25.5, got {first[17]}"
        assert frames[0]["channelIndices"] == list(range(18))

        # 5. 停止采集
        dev.stop_acquisition()
        assert not dev.acquiring, "stop_acquisition 后应停止采集"
        assert "c 02 1" in server.commands

        # 6. 轮询模式
        polled = dev.poll_all_channels()
        assert len(polled) == 16, f"轮询应 16 通道, got {len(polled)}"
        assert abs(polled[0] - 10.0) < 0.01, f"轮询 CH1 应为 10, got {polled[0]}"
        assert abs(polled[15] - 160.0) < 0.01, f"轮询 CH16 应为 160, got {polled[15]}"

        # 7. 单位读写
        dev.set_unit("MPa")
        assert dev.unit == "MPa"
        assert any(c.startswith("v01101 0.006895") for c in server.commands)

        # 8. 断开
        dev.disconnect()
        assert not dev.connected, "disconnect() 后应断开"
    finally:
        server.stop()

    print("PASS: full_flow")


def test_connect_refused() -> None:
    dev = DAQP1604Device("127.0.0.1", 1)  # 无服务端口
    try:
        dev.connect()
        assert False, "连接被拒绝应抛异常"
    except Exception:
        pass
    assert not dev.connected
    print("PASS: connect_refused")


def test_command_error() -> None:
    server = MockP1604Server()
    server.start()
    try:
        dev = DAQP1604Device("127.0.0.1", server.port)
        dev.connect()
        # 命令超时在未连接时抛 NotConnectedError
        dev.disconnect()
        try:
            dev.send_command("anything")
            assert False, "未连接时应抛 NotConnectedError"
        except Exception as exc:
            assert type(exc).__name__ == "NotConnectedError"
    finally:
        server.stop()
    print("PASS: command_error")


def main() -> None:
    test_full_flow()
    test_connect_refused()
    test_command_error()
    print("\n全部测试通过")


if __name__ == "__main__":
    main()
