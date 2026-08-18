# -*- coding: utf-8 -*-
"""DAQ-P-1604 模拟设备（TCP 服务器）。

按真实协议行为实现，用于在没有硬件时验证客户端库：
  - 2 字节大端长度前缀帧（含自身）
  - w1601 → A；c 00 / c 05 / c 01 → A；c 02 → A
  - c 01 后持续发送二进制数据帧（CH16..CH1 逆序 + 大气压 + 温度）
  - u01101 → EU 系数；rFFFF0 → 16 通道 ASCII 逆序
"""

from __future__ import annotations

import re
import socket
import struct
import threading
import time
from typing import Optional


class MockP1604Server:
    """模拟 DAQ-P-1604 设备。"""

    def __init__(self, host: str = "127.0.0.1", port: int = 0,
                 coeff: float = 6.894757, period_ms: int = 100) -> None:
        self.coeff = coeff
        self.period_ms = period_ms
        self._srv = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self._srv.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        self._srv.bind((host, port))
        self._srv.listen(1)
        self.host, self.port = self._srv.getsockname()

        self._conn: Optional[socket.socket] = None
        self._streaming = False
        self._seq = 0
        self._stop = threading.Event()
        self._thread = threading.Thread(target=self._serve, daemon=True)
        self._stream_thread: Optional[threading.Thread] = None

        # 记录收到的命令，便于断言
        self.commands: list = []
        self.frame_count = 0

    # ---- 启动 / 停止 ----
    def start(self) -> None:
        self._thread.start()

    def stop(self) -> None:
        self._stop.set()
        if self._conn is not None:
            try:
                self._conn.close()
            except OSError:
                pass
        try:
            self._srv.close()
        except OSError:
            pass

    # ---- 服务循环 ----
    def _serve(self) -> None:
        self._srv.settimeout(0.2)
        while not self._stop.is_set():
            try:
                conn, _ = self._srv.accept()
            except socket.timeout:
                continue
            except OSError:
                break
            self._conn = conn
            conn.settimeout(0.2)
            self._handle_conn(conn)
            break

    def _handle_conn(self, conn: socket.socket) -> None:
        buf = bytearray()
        while not self._stop.is_set():
            try:
                chunk = conn.recv(4096)
            except socket.timeout:
                continue
            except OSError:
                break
            if not chunk:
                break
            buf.extend(chunk)
            # 命令以纯 ASCII 发送，无长度前缀、无换行；按已知命令前缀做增量匹配
            while True:
                cmd = self._match_command(bytes(buf))
                if cmd is None:
                    break
                del buf[:len(cmd)]
                self.commands.append(cmd)
                self._handle_command(conn, cmd)

    # 已知命令模式（按优先级顺序匹配）
    _COMMAND_PATTERNS = [
        "w1601",
        "c 00 1 FFFF 1 100 7 0",
        "c 05 1 0810",
        "c 01 1",
        "c 02 1",
        "u01101",
        "rFFFF0",
    ]

    def _match_command(self, data: bytes) -> Optional[str]:
        """从缓冲区开头匹配一个完整命令；不完整返回 None。"""
        text = data.decode("ascii", errors="ignore")
        for pat in self._COMMAND_PATTERNS:
            if text.startswith(pat):
                return pat
        # c 00 支持任意周期参数：c 00 1 FFFF 1 <per> 7 0
        m = re.match(r"c 00 1 FFFF 1 (\d+) 7 0", text)
        if m:
            return m.group(0)
        # v01101 写入系数
        m = re.match(r"v01101 [0-9.]+", text)
        if m:
            return m.group(0)
        return None

    def _handle_command(self, conn: socket.socket, cmd: str) -> None:
        if cmd == "w1601":
            self._send_ack(conn, "A")
        elif cmd.startswith("c 00 1 FFFF 1"):
            self._send_ack(conn, "A")
        elif cmd == "c 05 1 0810":
            self._send_ack(conn, "A")
        elif cmd == "c 01 1":
            self._send_ack(conn, "A")
            self._streaming = True
            self._seq = 0
            # 单独线程持续推帧，避免与命令处理互斥
            self._stream_thread = threading.Thread(
                target=self._stream_loop, args=(conn,), daemon=True
            )
            self._stream_thread.start()
        elif cmd == "c 02 1":
            self._streaming = False
            self._send_ack(conn, "A")
        elif cmd == "u01101":
            self._send_ack(conn, f"{self.coeff:.6f}")
        elif cmd.startswith("v01101 "):
            self.coeff = float(cmd.split()[1])
            self._send_ack(conn, "A")
        elif cmd == "rFFFF0":
            # 16 通道 ASCII，逆序 CH16..CH1
            vals = " ".join(f"{i * 10.0:.3f}" for i in range(16, 0, -1))
            self._send_ack(conn, vals)
        else:
            self._send_ack(conn, "N01")

    # ---- 帧发送 ----
    def _send_ack(self, conn: socket.socket, text: str) -> None:
        payload = text.encode("ascii")
        frame = struct.pack(">H", len(payload) + 2) + payload
        try:
            conn.sendall(frame)
        except OSError:
            pass

    def _stream_loop(self, conn: socket.socket) -> None:
        while self._streaming and not self._stop.is_set():
            self._send_stream_frame(conn)
            time.sleep(self.period_ms / 1000.0)

    def _send_stream_frame(self, conn: socket.socket) -> None:
        # 帧负载 77 字节：5 字节头 + 18 × float32 BE
        header = b"\x01" + struct.pack(">H", self._seq) + b"\x00\x00"
        # 压力通道按 CH16..CH1 逆序生成（i=16..1）
        pressures = [float(i) * 100.0 for i in range(16, 0, -1)]
        atm_pressure = 101325.0
        atm_temp = 25.5
        payload = header + b"".join(
            struct.pack(">f", v) for v in (pressures + [atm_pressure, atm_temp])
        )
        frame = struct.pack(">H", len(payload) + 2) + payload
        try:
            conn.sendall(frame)
        except OSError:
            return
        self._seq = (self._seq + 1) & 0xFFFF
        self.frame_count += 1


def main() -> None:
    """独立运行模拟设备（调试用）。"""
    import sys

    port = int(sys.argv[1]) if len(sys.argv) > 1 else 9000
    server = MockP1604Server(port=port)
    server.start()
    print(f"Mock DAQ-P-1604 listening on 127.0.0.1:{port}")
    try:
        while True:
            time.sleep(1)
    except KeyboardInterrupt:
        server.stop()


if __name__ == "__main__":
    main()
