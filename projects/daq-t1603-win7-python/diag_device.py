# -*- coding: utf-8 -*-
"""
DAQ-T-1603 真机诊断脚本

定位"开始采集后卡住"的根因:
  1. 连接 + 配置命令是否正常返回
  2. 启动采集命令 @f0 后设备是否真的开始发数据
  3. 数据帧格式是否符合预期(64 字节二进制)
  4. read_frame 的收帧循环是否在等什么

直接打印每一步的耗时和原始字节,不依赖 PyQt,便于诊断。
"""

from __future__ import annotations

import socket
import struct
import sys
import time
from typing import List, Optional

HOST = "192.168.1.10"
PORT = 9000
CONNECT_TIMEOUT = 5.0
CMD_TIMEOUT = 2.0
READ_TIMEOUT = 0.5
FRAME_SIZE = 64


def log(msg: str) -> None:
    print(f"[{time.strftime('%H:%M:%S')}] {msg}", flush=True)


def send_cmd(sock: socket.socket, cmd: str) -> str:
    """发送命令并读一行响应,打印耗时和返回值。"""
    t0 = time.time()
    sock.settimeout(CMD_TIMEOUT)
    sock.sendall(cmd.encode())
    buf = bytearray()
    while len(buf) < 1024:
        try:
            byte = sock.recv(1)
            if not byte:
                break
            if byte == b"\n":
                break
            buf.extend(byte)
        except socket.timeout:
            break
    resp = buf.decode(errors="ignore").strip()
    log(f"  -> 发 {cmd!r:20s} 收 {resp!r:30s}  耗时 {(time.time()-t0)*1000:.0f}ms")
    return resp


def main() -> int:
    log(f"=== 连接 {HOST}:{PORT} ===")
    t0 = time.time()
    try:
        sock = socket.create_connection((HOST, PORT), timeout=CONNECT_TIMEOUT)
    except Exception as e:
        log(f"连接失败: {e}")
        return 1
    log(f"连接成功,耗时 {(time.time()-t0)*1000:.0f}ms")
    sock.setsockopt(socket.SOL_SOCKET, socket.SO_KEEPALIVE, 1)

    # ---- 步骤 1:配置命令 ----
    log("=== 步骤 1:发配置命令 ===")
    send_cmd(sock, "@fe BIN 1")
    send_cmd(sock, "@fe TIME 0")
    send_cmd(sock, "@fe HEAD 0")

    # 顺便查一下设备当前配置
    log("=== 步骤 1b:查询设备配置 ===")
    sock.settimeout(CMD_TIMEOUT)
    sock.sendall(b"@e3")
    time.sleep(0.1)
    try:
        resp = sock.recv(64)
        log(f"  @e3 返回 {len(resp)} 字节: {resp!r}")
    except socket.timeout:
        log("  @e3 超时无响应")

    # ---- 步骤 2:启动采集 ----
    log("=== 步骤 2:启动采集 ===")
    # 先发停止命令清空缓冲
    try:
        sock.sendall(b"@f1")
        time.sleep(0.05)
        # 排空接收缓冲
        sock.settimeout(0.1)
        try:
            while True:
                chunk = sock.recv(4096)
                if not chunk:
                    break
        except socket.timeout:
            pass
    except OSError as e:
        log(f"  发 @f1 失败(预期): {e}")

    # 发启动命令
    t0 = time.time()
    sock.sendall(b"@f0 FFFF 2")
    log(f"  已发 @f0 FFFF 2,耗时 {(time.time()-t0)*1000:.0f}ms")

    # ---- 步骤 3:等 200ms 后排空 ACK ----
    log("=== 步骤 3:排空 ACK ===")
    time.sleep(0.2)
    sock.settimeout(0.3)
    ack_drained = bytearray()
    try:
        while True:
            chunk = sock.recv(4096)
            if not chunk:
                break
            ack_drained.extend(chunk)
            if len(ack_drained) > 1000:
                break
    except socket.timeout:
        pass
    log(f"  ACK 阶段排空 {len(ack_drained)} 字节")
    if ack_drained:
        log(f"  ACK 内容(前 64 字节): {bytes(ack_drained[:64])!r}")

    # ---- 步骤 4:开始读帧 ----
    log("=== 步骤 4:读数据帧(最多 5 秒) ===")
    sock.settimeout(READ_TIMEOUT)
    buf = bytearray()
    frames_read = 0
    start = time.time()
    last_data_at = time.time()
    raw_dumps: List[bytes] = []

    while time.time() - start < 5.0 and frames_read < 5:
        try:
            chunk = sock.recv(1024)
            if not chunk:
                log("  对端关闭连接")
                break
            buf.extend(chunk)
            last_data_at = time.time()
            # 尝试从 buf 中切出完整帧
            while len(buf) >= FRAME_SIZE:
                frame = bytes(buf[:FRAME_SIZE])
                del buf[:FRAME_SIZE]
                frames_read += 1
                if frames_read <= 3:
                    raw_dumps.append(frame)
                # 解析
                try:
                    temps = list(struct.unpack("<16f", frame))
                    temps.reverse()
                    log(f"  帧 {frames_read}: {len(frame)}B, 前 4 通道 = {[round(t, 2) for t in temps[:4]]}")
                except Exception as e:
                    log(f"  帧 {frames_read}: 解析失败 {e}, 原始 = {frame[:16]!r}")
        except socket.timeout:
            elapsed = time.time() - last_data_at
            if elapsed > 2.0:
                log(f"  已 {elapsed:.1f}s 无数据,可能设备没在发")
            continue
        except OSError as e:
            log(f"  读错误: {e}")
            break

    log(f"=== 结果:共收到 {frames_read} 帧,buf 剩余 {len(buf)} 字节 ===")
    if raw_dumps:
        log("原始帧 dump:")
        for i, frame in enumerate(raw_dumps):
            log(f"  帧 {i+1}: {frame.hex()}")

    # ---- 步骤 5:停止 + 断开 ----
    log("=== 步骤 5:停止采集 ===")
    try:
        sock.sendall(b"@f1")
        log("  已发 @f1")
    except OSError as e:
        log(f"  发 @f1 失败: {e}")
    time.sleep(0.1)
    sock.close()
    log("=== 完成 ===")
    return 0


if __name__ == "__main__":
    sys.exit(main())
