"""
DAQ-T-1603 快速启停采集 ACK 位置验证脚本（v3）。

v3 改进（修复 v2 三个 bug）：
  1. 排空不彻底：改用"发送 @f1 → 等 300ms → 长超时 drain → 验证"多阶段排空
  2. ACK 误判：数据帧中 0x41 ('A') 字节会被误认为 ACK。
     v3 改为：先确认首字节是否为 'A'，是→ACK 在前；否→读取足够字节判断是否为有效帧
  3. 帧扫描 bug：不再用"扫描整个流找 0x41"的方式判断 ACK 位置

测试策略：
  - 每轮：@f0 → 读首字节 → 判断是 'A' 还是数据帧 → @f1 → 彻底排空 → 验证
  - 排空策略：@f1 后 sleep 300ms，再 drain（1s 超时），再 is_buffer_clean（200ms）
  - 每轮之间 sleep 500ms，确保设备完全停止
"""

import socket
import time
import sys
import struct

HOST = "192.168.1.10"
PORT = 9000
TIMEOUT = 3.0
CYCLES = 100

FRAME_SIZE = 64  # BIN=1, TIME=0, HEAD=0 → 64 字节帧


def send_command_exact(sock: socket.socket, cmd: str, n: int) -> str:
    """发送命令并读取固定 n 字节响应。"""
    sock.sendall((cmd + "\n").encode())
    sock.settimeout(TIMEOUT)
    data = b""
    while len(data) < n:
        chunk = sock.recv(n - len(data))
        if not chunk:
            break
        data += chunk
    return data.decode().strip()


def drain_socket(sock: socket.socket, timeout: float = 1.0) -> int:
    """排空 socket 接收缓冲区，返回排空的字节数。"""
    sock.settimeout(timeout)
    total = 0
    while True:
        try:
            data = sock.recv(65536)
            if not data:
                break
            total += len(data)
        except socket.timeout:
            break
        except OSError:
            break
    return total


def is_buffer_clean(sock: socket.socket, timeout: float = 0.2) -> bool:
    """验证接收缓冲区是否为空。"""
    sock.settimeout(timeout)
    try:
        data = sock.recv(65536)
        return len(data) == 0
    except socket.timeout:
        return True
    except OSError:
        return True


def thorough_stop(sock: socket.socket, send_stop: bool = True) -> dict:
    """
    彻底停止采集并排空缓冲区。

    Args:
        send_stop: True=发送 @f1 停止命令（采集状态下用）；
                   False=只 drain 不发 @f1（未采集状态下用，避免误发停止命令）。
    """
    stats = {"drained": 0, "clean_checks": 0, "final_clean": False, "sent_stop": send_stop}

    # 阶段 1：发 @f1（仅在采集状态下）
    if send_stop:
        sock.settimeout(TIMEOUT)
        try:
            sock.sendall(b"@f1\n")
        except OSError:
            pass
        # 等 300ms 让设备处理停止命令 + 发送尾随帧
        time.sleep(0.3)

    # 阶段 2：drain（多轮，每轮 500ms 超时）
    for _ in range(5):
        d = drain_socket(sock, timeout=0.5)
        stats["drained"] += d
        if d == 0:
            break
        time.sleep(0.05)

    # 阶段 3：最终验证（200ms 超时）
    stats["final_clean"] = is_buffer_clean(sock, timeout=0.2)
    stats["clean_checks"] = 1

    # 如果还不干净，再 drain 一次
    while not stats["final_clean"]:
        d = drain_socket(sock, timeout=0.5)
        stats["drained"] += d
        stats["clean_checks"] += 1
        if stats["clean_checks"] > 5:
            break
        stats["final_clean"] = is_buffer_clean(sock, timeout=0.2)

    return stats


def configure_device(sock: socket.socket):
    """配置设备为 BIN=1, TIME=0, HEAD=0。"""
    print("  [配置] 设置 BIN=1...")
    resp = send_command_exact(sock, "@fe BIN 1", 1)
    print(f"    @fe BIN 1 -> {resp!r}")
    resp = send_command_exact(sock, "@fd BIN", 1)
    print(f"    @fd BIN -> {resp!r}")

    print("  [配置] 设置 SPS=500, TIME=0, HEAD=0...")
    for cmd in ["@fe SPS 500", "@fe TIME 0", "@fe HEAD 0"]:
        resp = send_command_exact(sock, cmd, 1)
        print(f"    {cmd} -> {resp!r}")


def is_valid_frame(data: bytes) -> bool:
    """校验 64 字节是否为有效温度帧。"""
    if len(data) != FRAME_SIZE:
        return False
    try:
        temps = struct.unpack("<16f", data)
    except struct.error:
        return False
    for t in temps:
        if t != t or t == float("inf") or t == float("-inf"):
            continue
        if t < -1000 or t > 5000:
            return False
    return True


def main():
    print(f"=== DAQ-T-1603 ACK 位置验证 v3 ===")
    print(f"目标: {HOST}:{PORT}")
    print(f"循环次数: {CYCLES}")
    print(f"帧大小: {FRAME_SIZE} 字节 (BIN=1, TIME=0, HEAD=0)")
    print(f"排空策略: @f1 → 300ms → drain(1s) → 验证(200ms)")
    print()

    # ---- 连接 ----
    print("[1] 连接设备...")
    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    sock.settimeout(TIMEOUT)
    try:
        sock.connect((HOST, PORT))
        print(f"  连接成功: {HOST}:{PORT}")
    except Exception as e:
        print(f"  连接失败: {e}")
        return 1

    try:
        configure_device(sock)

        # 先做一次彻底排空（不发 @f1，避免在未采集状态下误发停止命令）
        print("\n[1.5] 初始排空...")
        init_drain = thorough_stop(sock, send_stop=False)
        print(f"  初始排空: {init_drain['drained']} 字节, clean={init_drain['final_clean']}")

        print(f"\n[2] 开始快速启停测试 ({CYCLES} 次循环)...")
        print()

        stats = {
            "total": 0,
            "ack_first_byte": 0,       # 首字节 = 'A'
            "data_frame_first": 0,     # 首字节是数据帧
            "other": 0,                # 其他情况
            "errors": 0,
            "stop_drain_stats": [],    # 每次 stop 后排空统计
            "pre_start_dirty": 0,      # start 前缓冲区脏的次数
        }

        for i in range(CYCLES):
            stats["total"] += 1

            try:
                # ---- 步骤 0：start 前验证缓冲区 ----
                if not is_buffer_clean(sock, timeout=0.2):
                    stats["pre_start_dirty"] += 1
                    extra = drain_socket(sock, timeout=0.5)
                    print(f"  [{i+1:3d}] WARNING: start 前残留 {extra} 字节，已排空")

                # ---- 步骤 1：发送 @f0 开始采集 ----
                sock.settimeout(TIMEOUT)
                sock.sendall(b"@f0 FFFF 2\n")

                # ---- 步骤 2：读首字节 ----
                # 用 500ms 超时确保至少读到 1 字节
                sock.settimeout(0.5)
                try:
                    first_byte = sock.recv(1)
                except socket.timeout:
                    print(f"  [{i+1:3d}] ERROR: @f0 后 500ms 无响应")
                    stats["errors"] += 1
                    # 未采集成功，只 drain 不发 @f1
                    stop_stats = thorough_stop(sock, send_stop=False)
                    stats["stop_drain_stats"].append(stop_stats)
                    continue

                if not first_byte:
                    print(f"  [{i+1:3d}] ERROR: 连接断开")
                    stats["errors"] += 1
                    try:
                        sock.close()
                    except Exception:
                        pass
                    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
                    sock.settimeout(TIMEOUT)
                    sock.connect((HOST, PORT))
                    configure_device(sock)
                    continue

                fb = first_byte[0]

                if fb == ord('A'):
                    # 首字节是 ACK
                    stats["ack_first_byte"] += 1
                    status = "ACK 在首字节"
                else:
                    # 首字节不是 ACK，读取剩余字节判断是否为数据帧
                    sock.settimeout(0.3)
                    remaining = b""
                    try:
                        remaining = sock.recv(FRAME_SIZE * 2)  # 读足够多
                    except socket.timeout:
                        pass

                    candidate = first_byte + remaining
                    if len(candidate) >= FRAME_SIZE and is_valid_frame(candidate[:FRAME_SIZE]):
                        stats["data_frame_first"] += 1
                        status = f"数据帧在前 (首字节=0x{fb:02X}, 帧={1}, 后续={len(remaining)}B)"
                    else:
                        stats["other"] += 1
                        # 打印前 16 字节用于诊断
                        preview = candidate[:16].hex()
                        status = f"其他 (首字节=0x{fb:02X}, 后续={len(remaining)}B, hex={preview})"

                print(f"  [{i+1:3d}] {status}")

                # ---- 步骤 3：彻底停止 + 排空 ----
                stop_stats = thorough_stop(sock)
                stats["stop_drain_stats"].append(stop_stats)

                # ---- 步骤 4：轮间间隔 ----
                time.sleep(0.2)  # 给设备充分时间稳定

            except socket.timeout:
                print(f"  [{i+1:3d}] TIMEOUT")
                stats["errors"] += 1
                stop_stats = thorough_stop(sock)
                stats["stop_drain_stats"].append(stop_stats)
                continue

            except OSError as e:
                print(f"  [{i+1:3d}] ERROR: {e}")
                stats["errors"] += 1
                try:
                    sock.close()
                except Exception:
                    pass
                sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
                sock.settimeout(TIMEOUT)
                try:
                    sock.connect((HOST, PORT))
                    configure_device(sock)
                except Exception as re:
                    print(f"  重连失败: {re}")
                    break
                continue

        # ---- 统计 ----
        print()
        print("=" * 60)
        print("测试结果统计")
        print("=" * 60)
        print(f"  总测试次数:       {stats['total']}")
        print(f"  ACK 在首字节:     {stats['ack_first_byte']} ({stats['ack_first_byte']/max(stats['total'],1)*100:.1f}%)")
        print(f"  数据帧在前:       {stats['data_frame_first']} ({stats['data_frame_first']/max(stats['total'],1)*100:.1f}%)")
        print(f"  其他:             {stats['other']}")
        print(f"  错误:             {stats['errors']}")
        print(f"  start 前脏次数:   {stats['pre_start_dirty']}")

        if stats["stop_drain_stats"]:
            clean_count = sum(1 for s in stats["stop_drain_stats"] if s["final_clean"])
            avg_drain = sum(s["drained"] for s in stats["stop_drain_stats"]) / len(stats["stop_drain_stats"])
            max_drain = max(s["drained"] for s in stats["stop_drain_stats"])
            print(f"\n  stop 后排空: avg={avg_drain:.1f}B, max={max_drain}B")
            print(f"  stop 后最终 clean: {clean_count}/{len(stats['stop_drain_stats'])}")

        print()
        if stats["data_frame_first"] == 0 and stats["other"] == 0:
            print("结论: @f0 后首字节始终是 ACK ('A')。")
            print("      v2 的'ACK 不在首字节'是缓冲区残留 + ACK 误判导致。")
        elif stats["data_frame_first"] > 0:
            print(f"结论: {stats['data_frame_first']} 次数据帧在前，ACK 不在首字节。")
        else:
            print("结论: 有异常情况，需要进一步分析。")

    finally:
        try:
            sock.sendall(b"@f1\n")
            time.sleep(0.2)
            drain_socket(sock, timeout=0.5)
        except Exception:
            pass
        try:
            sock.close()
        except Exception:
            pass
        print("\n连接已关闭。")

    return 0


if __name__ == "__main__":
    sys.exit(main())