# -*- coding: utf-8 -*-
"""
端到端真机验证:模拟 UI 的完整流程(连接→配置→采集→停止→断开)。

不启动 GUI,但完全复用 main.py 的 T1603Device + AcquisitionWorker 逻辑,
验证修复后的代码在真机上不再卡住。
"""

from __future__ import annotations

import os
import sys
import time
from typing import List

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

# 用 offscreen 模式跑 Qt,不弹窗
os.environ["QT_QPA_PLATFORM"] = "offscreen"

from PyQt5.QtCore import QCoreApplication, QEventLoop
from PyQt5.QtWidgets import QApplication

from main import T1603Device, AcquisitionWorker, CHANNEL_COUNT

HOST = "192.168.1.10"
PORT = 9000


def main() -> int:
    app = QApplication.instance() or QApplication(sys.argv)

    print(f"=== 端到端真机验证 {HOST}:{PORT} ===", flush=True)

    device = T1603Device(host=HOST, port=PORT)

    # 1. 连接
    print("[1/6] 连接设备...", flush=True)
    t0 = time.time()
    device.connect()
    print(f"  连接成功,{(time.time()-t0)*1000:.0f}ms, connected={device.connected}", flush=True)

    # 2. 配置热电偶(设备返回 KEEJEEEEEEEEEEEE,我们用相同配置)
    print("[2/6] 应用热电偶配置 KEEJEEEEEEEEEEEE...", flush=True)
    device.set_thermocouple_types("KEEJEEEEEEEEEEEE")
    print("  配置成功", flush=True)

    # 3. 启动采集 + 启动 worker 线程
    print("[3/6] 启动采集...", flush=True)
    t0 = time.time()
    device.start_acquisition()
    print(f"  start_acquisition 返回,{(time.time()-t0)*1000:.0f}ms, acquiring={device.acquiring}", flush=True)

    received_frames: List[list] = []
    errors: List[str] = []
    worker = AcquisitionWorker(device)
    worker.data_received.connect(lambda temps: received_frames.append(temps))
    worker.error_occurred.connect(lambda msg: errors.append(msg))
    worker.finished_acquisition.connect(lambda: print("  worker 退出", flush=True))

    print("[4/6] 启动采集线程,采集 3 秒...", flush=True)
    worker.start()

    # 用事件循环跑 3 秒,让信号能投递
    loop = QEventLoop()
    timer = app  # 借用 app 做单次定时器
    from PyQt5.QtCore import QTimer
    QTimer.singleShot(3000, loop.quit)
    loop.exec_()

    print(f"  3 秒内收到 {len(received_frames)} 帧,错误 {len(errors)} 个", flush=True)
    if received_frames:
        first = received_frames[0]
        print(f"  首帧前 4 通道: {[round(t, 2) for t in first[:4]]}", flush=True)
        # 找出非零通道
        nonzero = [(i, round(t, 2)) for i, t in enumerate(first) if abs(t) > 0.01]
        print(f"  非零通道: {nonzero}", flush=True)
    if errors:
        print(f"  错误: {errors}", flush=True)

    # 5. 停止采集
    print("[5/6] 停止采集...", flush=True)
    t0 = time.time()
    device.stop_acquisition()
    worker.wait(2000)
    print(f"  停止完成,{(time.time()-t0)*1000:.0f}ms, acquiring={device.acquiring}", flush=True)

    # 6. 断开
    print("[6/6] 断开连接...", flush=True)
    device.disconnect()
    print(f"  disconnected={device.disconnected if hasattr(device, 'disconnected') else (not device.connected)}", flush=True)

    # 判定
    print("\n=== 判定 ===", flush=True)
    ok = True
    if not device.connected and len(received_frames) > 0 and not errors:
        print("PASS: 连接/配置/采集/停止/断开 全流程正常,未卡死", flush=True)
    else:
        print(f"FAIL: connected={device.connected} frames={len(received_frames)} errors={len(errors)}", flush=True)
        ok = False

    return 0 if ok else 1


if __name__ == "__main__":
    sys.exit(main())
