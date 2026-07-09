# -*- coding: utf-8 -*-
"""
复现 main.py 在真机上的卡住问题。

直接用 main.py 的 T1603Device,连接真机启动采集,看卡在哪。
"""

from __future__ import annotations

import sys
import time
import os

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from main import T1603Device

HOST = "192.168.1.10"
PORT = 9000


def main() -> int:
    print(f"=== 用 main.py 的 T1603Device 连接 {HOST}:{PORT} ===", flush=True)

    device = T1603Device(host=HOST, port=PORT)

    # 连接
    t0 = time.time()
    try:
        device.connect()
    except Exception as e:
        print(f"连接失败: {e}", flush=True)
        return 1
    print(f"连接成功,耗时 {(time.time()-t0)*1000:.0f}ms", flush=True)

    # 启动采集
    print("=== 启动采集 ===", flush=True)
    t0 = time.time()
    try:
        device.start_acquisition()
    except Exception as e:
        print(f"启动采集失败: {e}", flush=True)
        return 1
    print(f"start_acquisition 返回,耗时 {(time.time()-t0)*1000:.0f}ms", flush=True)

    # 读帧 - 这里是卡住的地方
    print("=== 开始 read_frame 循环 ===", flush=True)
    frames_read = 0
    start = time.time()
    last_log = time.time()
    while time.time() - start < 8.0 and frames_read < 10:
        t_frame = time.time()
        temps = device.read_frame()
        elapsed = (time.time() - t_frame) * 1000
        if temps is None:
            none_count = getattr(main, "_nones", 0) + 1
            main._nones = none_count  # type: ignore[attr-defined]
            if time.time() - last_log > 1.0:
                print(f"  [{time.time()-start:.1f}s] 已读 {frames_read} 帧, None 次数 {none_count}", flush=True)
                last_log = time.time()
            continue
        frames_read += 1
        print(f"  帧 {frames_read}: read_frame 耗时 {elapsed:.0f}ms, 前 4 通道 = {[round(t, 2) for t in temps[:4]]}", flush=True)

    print(f"=== 结果:8 秒内读到 {frames_read} 帧 ===", flush=True)

    # 停止
    print("=== 停止采集 ===", flush=True)
    t0 = time.time()
    device.stop_acquisition()
    print(f"stop_acquisition 耗时 {(time.time()-t0)*1000:.0f}ms", flush=True)

    device.disconnect()
    print("=== 完成 ===", flush=True)
    return 0


if __name__ == "__main__":
    sys.exit(main())
