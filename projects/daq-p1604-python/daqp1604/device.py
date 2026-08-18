# -*- coding: utf-8 -*-
"""DAQ-P-1604 压力采集设备 Python 客户端库。

DAQ-P-1604 是一款基于 TCP/IP 的 18 通道压力采集设备：
  - CH1~CH16: 压力值（由硬件 EU 系数决定单位）
  - CH17:     大气压力 (Pa)
  - CH18:     大气温度 (℃)

出厂默认地址 192.168.3.101:9000，自定义二进制协议（非 Modbus）。

协议要点：
  - 所有命令以纯 ASCII 发送，不得附加任何换行符（\r\n 或 \n）
  - 连接后必须首先发送 ``w1601`` 启用"2 字节大端长度前缀"模式
  - 每个响应帧 = 2 字节长度前缀（含自身）+ payload
  - 成功响应 ``A``，失败响应 ``Nxx``
  - 二进制数据帧：5 字节头 + 18 × float32 BE，前 16 路逆序（CH16..CH1）
"""

from __future__ import annotations

import collections
import logging
import socket
import struct
import threading
import time
from typing import Callable, Dict, List, Optional

from .units import (
    DEFAULT_UNIT,
    coefficient_for_unit,
    match_unit_by_coefficient,
)

logger = logging.getLogger("daqp1604")

DEFAULT_HOST = "192.168.3.101"
DEFAULT_PORT = 9000

CONNECT_TIMEOUT = 5.0          # TCP 连接超时（秒）
COMMAND_TIMEOUT = 2.0          # 单条命令响应超时（秒）
READ_TIMEOUT = 0.2             # 采集读超时（秒）——短超时便于 stop 响应
KEEPALIVE_PERIOD = 3.0         # TCP keepalive 探测间隔（秒）

CHANNEL_COUNT = 18             # 16 压力 + 1 大气压 + 1 温度
PRESSURE_CHANNEL_COUNT = 16
FRAME_HEADER_SIZE = 5          # 0x01 + seq(2) + reserved(2)
FRAME_PAYLOAD_SIZE = 5 + 18 * 4  # 77 字节
MAX_FRAME_PAYLOAD = 4096       # 单帧 payload 安全上限

STREAM_ID = "1"                # 流 ID（固定 1）


class DAQP1604Error(Exception):
    """设备操作异常基类。"""


class NotConnectedError(DAQP1604Error):
    """设备未连接。"""


class CommandError(DAQP1604Error):
    """设备返回 Nxx 逻辑错误。"""


class TimeoutError(DAQP1604Error):
    """命令响应超时。"""


def _is_ascii_frame(data: bytes) -> bool:
    """判断 payload 是否为 ASCII 帧（命令响应）而非二进制数据帧。

    检查前 64 字节：全部为可打印 ASCII（0x20-0x7E）或 CR/LF 则判定为 ASCII。
    """
    check = data[:64]
    for b in check:
        if b == 0x0D or b == 0x0A:
            continue
        if b < 0x20 or b > 0x7E:
            return False
    return True


class DAQP1604Device:
    """DAQ-P-1604 设备驱动（线程安全）。

    后台读线程持续解析长度前缀帧：
      - ASCII 帧  → 放入 pendingResponses 队列，匹配 send_command 的 Promise
      - 二进制帧 → 调用数据回调推送 DataPayload

    使用示例::

        dev = DAQP1604Device("192.168.3.101", 9000)
        dev.on_data = lambda payload: print(payload["channels"])
        dev.connect()
        dev.start_acquisition(period_ms=100)
        ...
        dev.stop_acquisition()
        dev.disconnect()
    """

    def __init__(self, host: str = DEFAULT_HOST, port: int = DEFAULT_PORT,
                 device_id: Optional[str] = None) -> None:
        self.host = host
        self.port = port
        self.device_id = device_id or f"{host}:{port}"
        self.unit: str = DEFAULT_UNIT

        # 数据回调：payload = {deviceId, timestamp, channels, channelIndices}
        self.on_data: Optional[Callable[[Dict], None]] = None

        self._sock: Optional[socket.socket] = None
        self._connected = False
        self._acquiring = False
        self._reader_thread: Optional[threading.Thread] = None
        self._stop_event = threading.Event()

        self._recv_buffer = bytearray()
        # pendingResponses FIFO 队列，用于匹配命令响应
        self._pending: collections.deque = collections.deque()
        self._resp_event = threading.Event()
        self._lock = threading.RLock()   # 保护 _recv_buffer / _pending / socket 状态（可重入）
        self._cmd_lock = threading.Lock()  # 串行化命令发送

    # ======== 状态属性 ========

    @property
    def connected(self) -> bool:
        return self._connected

    @property
    def acquiring(self) -> bool:
        return self._acquiring

    # ======== 生命周期 ========

    def connect(self) -> None:
        """建立 TCP 连接并启用长度前缀模式。

        流程：TCP 拨号 → 启用 keepalive → 启动读线程 → 发送 w1601 →
        同步硬件单位（失败仅告警，不阻塞连接）。
        """
        if self._connected:
            return
        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.settimeout(CONNECT_TIMEOUT)
        try:
            sock.connect((self.host, self.port))
        except OSError as exc:
            sock.close()
            raise DAQP1604Error(
                f"connect to {self.host}:{self.port} failed: {exc}"
            ) from exc

        # 启用 TCP keepalive：物理拔网线场景下的兜底检测
        try:
            sock.setsockopt(socket.SOL_SOCKET, socket.SO_KEEPALIVE, 1)
            if hasattr(socket, "TCP_KEEPIDLE"):
                sock.setsockopt(socket.IPPROTO_TCP, socket.TCP_KEEPIDLE, int(KEEPALIVE_PERIOD))
            if hasattr(socket, "TCP_KEEPINTVL"):
                sock.setsockopt(socket.IPPROTO_TCP, socket.TCP_KEEPINTVL, int(KEEPALIVE_PERIOD))
        except OSError as exc:  # keepalive 是优化项，失败不中止
            logger.warning("enable tcp keepalive failed: %s", exc)

        sock.settimeout(READ_TIMEOUT)  # 采集阶段切短超时，便于 stop 快速响应

        with self._lock:
            self._sock = sock
            self._recv_buffer.clear()
            self._pending.clear()
            self._resp_event.clear()
            self._stop_event.clear()
            self._reader_thread = threading.Thread(
                target=self._reader_loop, name="daqp1604-reader", daemon=True
            )
            self._reader_thread.start()
            self._connected = True

        try:
            resp = self.send_command("w1601")
            if not resp.startswith("A"):
                logger.warning("enable length prefix returned: %r", resp)
            # 单位同步失败不阻塞连接（catch 后仅 warn）
            try:
                self.unit = self.read_unit()
            except Exception as exc:  # noqa: BLE001
                logger.warning("unit sync from hardware failed: %s", exc)
        except Exception:
            self.disconnect()
            raise

    def disconnect(self) -> None:
        """断开连接，先停止采集再关 socket。"""
        with self._lock:
            if not self._connected:
                return
            was_acquiring = self._acquiring
            self._acquiring = False
            self._connected = False
            self._stop_event.set()
            sock, self._sock = self._sock, None
            reader = self._reader_thread
            self._reader_thread = None
        # 命令发送在锁外执行，避免锁顺序死锁
        if was_acquiring and sock is not None:
            try:
                self.send_command(f"c 02 {STREAM_ID}")
            except Exception as exc:  # noqa: BLE001 - best effort
                logger.warning("stop stream before disconnect failed: %s", exc)
        if sock is not None:
            try:
                sock.close()
            except OSError:
                pass
        if reader is not None and reader.is_alive():
            reader.join(timeout=2.0)

    # ======== 采集控制 ========

    def start_acquisition(self, period_ms: int = 100) -> None:
        """启动连续采集（数据流模式）。

        内部执行命令链：
          c 00 1 FFFF 1 <period> 7 0   → 配置流参数（7=大端 float, 0=连续）
          c 05 1 0810                  → 压力 + 大气压 + 大气温度
          c 01 1                       → 启动数据流

        任一步返回 Nxx 即抛 CommandError，保证不会处于半启动状态。
        """
        with self._lock:
            if not self._connected or self._sock is None:
                raise NotConnectedError("device not connected")
        period_ms = max(10, int(period_ms))
        for cmd in (
            f"c 00 {STREAM_ID} FFFF 1 {period_ms} 7 0",
            f"c 05 {STREAM_ID} 0810",
            f"c 01 {STREAM_ID}",
        ):
            resp = self.send_command(cmd)
            if not resp.startswith("A"):
                self._stop_acquire_locked()
                raise CommandError(f"start acquisition failed: {cmd!r} -> {resp!r}")
        with self._lock:
            self._acquiring = True

    def stop_acquisition(self) -> None:
        """停止数据流（c 02 1），best effort。"""
        self._stop_acquire_locked()

    def _stop_acquire_locked(self) -> None:
        """停止采集。命令发送在锁外执行，避免锁顺序死锁。"""
        with self._lock:
            if not self._acquiring:
                return
            self._acquiring = False
            sock = self._sock
        if sock is not None:
            try:
                self.send_command(f"c 02 {STREAM_ID}")
            except Exception as exc:  # noqa: BLE001 - 停止命令 best effort
                logger.warning("stop stream command failed: %s", exc)

    # ======== 命令收发 ========

    def send_command(self, command: str, timeout: float = COMMAND_TIMEOUT) -> str:
        """发送纯 ASCII 命令（不带换行符）并等待响应。

        响应以 A 开头表示成功，Nxx 表示逻辑错误（不抛异常，由调用方判断）。
        超时抛 TimeoutError，未连接抛 NotConnectedError。
        """
        with self._cmd_lock:
            with self._lock:
                if not self._connected or self._sock is None:
                    raise NotConnectedError("device not connected")
                self._pending.clear()
                self._resp_event.clear()
                try:
                    self._sock.sendall(command.encode("ascii"))
                except OSError as exc:
                    raise DAQP1604Error(f"send command {command!r} failed: {exc}") from exc

            # 等待响应（读线程会写入 _pending 并 set 事件）
            if not self._resp_event.wait(timeout):
                raise TimeoutError(f"command {command!r} timed out")
            with self._lock:
                if self._pending:
                    return self._pending.popleft()
                raise TimeoutError(f"command {command!r} timed out")

    # ======== 单位读写 ========

    def read_unit(self) -> str:
        """读取硬件 EU 压力转换系数（u01101）并反查单位字符串。"""
        resp = self.send_command("u01101")
        resp = resp.strip()
        if resp.startswith("N"):
            raise CommandError(f"read unit failed: {resp!r}")
        try:
            coeff = float(resp)
        except ValueError as exc:
            raise CommandError(f"unexpected unit response: {resp!r}") from exc
        unit = match_unit_by_coefficient(coeff)
        if not unit:
            raise CommandError(f"unknown unit coefficient: {coeff}")
        self.unit = unit
        return unit

    def set_unit(self, unit: str) -> None:
        """写入 EU 压力转换系数（v01101 <coeff>）切换硬件单位。"""
        coeff = coefficient_for_unit(unit)
        resp = self.send_command(f"v01101 {coeff:.6f}")
        if resp.strip() != "A":
            raise CommandError(f"set unit failed: {resp!r}")
        self.unit = unit

    # ======== 轮询备用 ========

    def poll_all_channels(self) -> List[float]:
        """轮询一次 16 通道压力值（rFFFF0，调试用，不含 CH17/18）。

        返回 CH1..CH16 正序数组。
        """
        resp = self.send_command("rFFFF0", timeout=COMMAND_TIMEOUT)
        resp = resp.strip()
        if resp.startswith("N"):
            raise CommandError(f"poll failed: {resp!r}")
        parts = resp.split()
        values = [0.0] * PRESSURE_CHANNEL_COUNT
        for i, part in enumerate(parts[:PRESSURE_CHANNEL_COUNT]):
            values[PRESSURE_CHANNEL_COUNT - 1 - i] = float(part)  # 逆序 → 正序
        return values

    # ======== 读线程与帧解析 ========

    def _reader_loop(self) -> None:
        """后台读线程：读取数据 → 解析长度前缀帧 → 分发。"""
        while not self._stop_event.is_set():
            sock = self._sock
            if sock is None:
                break
            try:
                chunk = sock.recv(4096)
            except socket.timeout:
                continue
            except OSError:
                break
            if not chunk:
                break
            self._feed(chunk)
        # 连接关闭时清理
        with self._lock:
            self._connected = False
            self._acquiring = False
            self._pending.clear()
            self._resp_event.set()  # 唤醒等待响应的 send_command

    def _feed(self, data: bytes) -> None:
        """把收到的字节追加到缓冲区并按帧切割处理。"""
        with self._lock:
            self._recv_buffer.extend(data)
            while True:
                if len(self._recv_buffer) < 2:
                    return
                frame_len = struct.unpack(">H", bytes(self._recv_buffer[:2]))[0]
                if frame_len < 2:
                    # 异常长度，丢弃首字节尝试重对齐
                    del self._recv_buffer[0]
                    continue
                payload_len = frame_len - 2
                if payload_len > MAX_FRAME_PAYLOAD:
                    del self._recv_buffer[0]
                    continue
                if len(self._recv_buffer) < frame_len:
                    return  # 数据不足，等待下一个 TCP chunk
                payload = bytes(self._recv_buffer[2:frame_len])
                del self._recv_buffer[:frame_len]

                if _is_ascii_frame(payload):
                    # 命令响应
                    self._pending.append(payload.decode("ascii", errors="ignore").strip())
                    self._resp_event.set()
                else:
                    # 二进制数据帧 → 由读线程直接解析并回调（无锁）
                    self._handle_stream_frame(payload)

    def _handle_stream_frame(self, payload: bytes) -> None:
        """解析二进制数据帧并触发数据回调。

        帧结构（长度前缀已剥离）：
          byte0     = 0x01 固定同步标记
          byte1-2   = 帧序号 (uint16 BE)
          byte3-4   = 保留
          byte5-76  = 18 × float32 BE
            index 0..15  → CH16, CH15, ..., CH1（逆序，需反转）
            index 16     → CH17 大气压力
            index 17     → CH18 大气温度
        """
        if len(payload) < FRAME_PAYLOAD_SIZE:
            return
        if payload[0] != 0x01:
            return  # 非流帧，忽略

        seq = struct.unpack(">H", payload[1:3])[0]
        values = list(struct.unpack(">18f", payload[FRAME_HEADER_SIZE:FRAME_PAYLOAD_SIZE]))
        # 前 16 路逆序 → CH1..CH16 正序
        pressures = values[:PRESSURE_CHANNEL_COUNT][::-1]
        channels = pressures + [values[16], values[17]]

        callback = self.on_data
        if callback is not None:
            try:
                callback({
                    "deviceId": self.device_id,
                    "timestamp": time.time(),
                    "seq": seq,
                    "channels": channels,
                    "channelIndices": list(range(CHANNEL_COUNT)),
                })
            except Exception as exc:  # noqa: BLE001 - 回调异常不应杀死读线程
                logger.warning("data callback error: %s", exc)


# ======== 便捷函数 ========

def create_device(host: str = DEFAULT_HOST, port: int = DEFAULT_PORT) -> DAQP1604Device:
    """创建并返回设备实例（便捷入口）。"""
    return DAQP1604Device(host, port)
