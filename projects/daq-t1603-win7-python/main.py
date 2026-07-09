# -*- coding: utf-8 -*-
"""
DAQ-T-1603 温度采集 - Windows 7 兼容版(极简)

设计目标:
  - Win7 SP1 + Python 3.8 + PyQt5.15 即可运行
  - 单文件可读,约 400 行覆盖完整业务
  - UI 极简:IP/端口 + 连接/断开 + 开始/停止采集 + 16 通道数据 + 16 通道热电偶配置
  - 采集线程独立,不阻塞 UI;socket 短超时,停止响应迅速

协议要点(来自 shared/device-sdk/go/protocol/daq_t1603_frame.go):
  - TCP 连接后发 SCPI 文本命令配置设备
  - @fe BIN 1   强制二进制帧模式(64 字节 = 16 × float32 LE)
  - @fe TIME 0  关闭硬件时间戳前缀(避免 72 字节帧)
  - @fe HEAD 0  关闭序号前缀
  - @f0 FFFF 2  启动连续采集(掩码 FFFF = 全部 16 通道)
  - @f1         停止采集
  - @f3 0<TC>0  设置 16 通道热电偶类型(<TC> 为 16 字符,如 KKKKKKKKKKKKKKKK)
  - 二进制帧 CH15→CH0 排列,需反转为 CH0→CH15
"""

from __future__ import annotations

import socket
import struct
import sys
import threading
import time
from typing import List, Optional

from PyQt5.QtCore import Qt, QThread, pyqtSignal
from PyQt5.QtGui import QFont
from PyQt5.QtWidgets import (
    QApplication, QComboBox, QGridLayout, QGroupBox, QHBoxLayout,
    QLabel, QLineEdit, QMessageBox, QPushButton, QVBoxLayout, QWidget,
)

# ======== 协议常量 ========
DEFAULT_HOST = "192.168.3.101"   # 与项目本地 device-sdk 保持一致
DEFAULT_PORT = 9000
FRAME_SIZE = 64                   # 16 × float32 LE = 64 字节
CONNECT_TIMEOUT = 5.0             # TCP 连接超时(秒)
CMD_TIMEOUT = 1.0                 # 单条 SCPI 命令超时(秒)
READ_TIMEOUT = 0.2                # 采集读超时(秒)——短超时便于 stop 响应
ACK_DRAIN_DELAY = 0.2             # 启动采集后等待 ACK 的时间(秒)
CHANNEL_COUNT = 16                # DAQ-T-1603 固定 16 通道

# 支持的热电偶类型(与设备 @e3 命令返回的字符集一致)
# K/J/T/E/N 是常用基础型;R/S/B 是高温贵金属型
THERMOCOUPLE_TYPES = ["K", "J", "T", "E", "N", "R", "S", "B"]


# ======== 设备通信层 ========
class T1603Device:
    """DAQ-T-1603 设备通信封装。

    职责:TCP 连接管理、SCPI 命令收发、二进制帧解析。
    线程安全:所有公共方法内置锁,可在 UI 线程和采集线程交叉调用。
    设计取舍:不依赖任何第三方库,纯 socket + struct,确保 Win7 零依赖。
    """

    def __init__(self, host: str = DEFAULT_HOST, port: int = DEFAULT_PORT) -> None:
        self.host = host
        self.port = port
        self._sock: Optional[socket.socket] = None
        self._connected = False
        self._acquiring = False
        self._lock = threading.Lock()
        self._stop_event = threading.Event()

    @property
    def connected(self) -> bool:
        return self._connected

    @property
    def acquiring(self) -> bool:
        return self._acquiring

    def connect(self) -> None:
        """建立 TCP 连接并强制二进制模式。

        异常会向上抛,由 UI 层捕获展示。
        三条 @fe 命令必须全部成功,否则帧解析会错位。
        """
        if self._connected:
            return
        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.settimeout(CONNECT_TIMEOUT)
        sock.connect((self.host, self.port))
        # 保持 keepalive,避免长连接被中间设备断开
        sock.setsockopt(socket.SOL_SOCKET, socket.SO_KEEPALIVE, 1)
        # 强制二进制 + 关闭时间戳 + 关闭序号前缀
        # 三条命令任一失败都会导致帧错位,直接抛出
        self._send_cmd(sock, "@fe BIN 1")
        self._send_cmd(sock, "@fe TIME 0")
        self._send_cmd(sock, "@fe HEAD 0")
        sock.settimeout(READ_TIMEOUT)  # 采集阶段切短超时
        self._sock = sock
        self._connected = True

    def disconnect(self) -> None:
        """断开连接,先停止采集再关 socket。"""
        with self._lock:
            if self._acquiring:
                self._stop_acquire_locked()
            if self._sock is not None:
                try:
                    self._sock.close()
                except OSError:
                    pass
                self._sock = None
            self._connected = False

    def set_thermocouple_types(self, types: str) -> None:
        """设置 16 通道热电偶类型。

        参数 types 必须为 16 字符字符串,每个字符代表一个通道的类型字母(如 K/J/T)。
        命令格式 @f3 0<TC>0:前后各一个 0 是设备协议要求的占位符。
        """
        if not self._connected or self._sock is None:
            raise RuntimeError("设备未连接")
        if len(types) != CHANNEL_COUNT:
            raise ValueError(f"热电偶类型必须 {CHANNEL_COUNT} 字符,实际 {len(types)}")
        if self._acquiring:
            raise RuntimeError("采集中不能修改热电偶配置")
        # 持锁保护 socket 写入,避免与采集线程争用
        with self._lock:
            self._send_cmd(self._sock, f"@f3 0{types}0")

    def start_acquisition(self, mask: str = "FFFF") -> None:
        """启动连续采集。

        mask 为 4 位十六进制通道掩码,FFFF 表示全部 16 通道。
        流程:
          1. 先发 @f1 停止命令,清空设备缓冲区残留数据帧
          2. 排空 TCP 接收缓冲区
          3. 发 @f0 <mask> 2 启动连续采集
          4. 等待 200ms 让设备发完 ACK,排空 ACK 残留
             (ACK 若不被消费,会被当作首帧字节,破坏帧对齐)
        """
        with self._lock:
            if not self._connected or self._sock is None:
                raise RuntimeError("设备未连接")
            if self._acquiring:
                return
            # 步骤 1:停止命令清空设备缓冲区
            try:
                self._sock.sendall(b"@f1")
                time.sleep(0.05)
                self._drain_recv(0.1)
            except OSError:
                pass  # 设备可能未在采集,忽略错误
            # 步骤 3:启动采集(不读响应,因为数据帧会立即到来)
            self._sock.sendall(f"@f0 {mask} 2".encode())
            # 步骤 4:等待并消费 ACK
            time.sleep(ACK_DRAIN_DELAY)
            self._drain_recv(0.1)
            self._acquiring = True
            self._stop_event.clear()

    def stop_acquisition(self) -> None:
        """停止采集。"""
        with self._lock:
            self._stop_acquire_locked()

    def _stop_acquire_locked(self) -> None:
        """停止采集(已持锁版本)。"""
        if not self._acquiring:
            return
        self._stop_event.set()
        if self._sock is not None:
            try:
                self._sock.sendall(b"@f1")
            except OSError:
                pass  # 连接已断开时这是预期行为
        self._acquiring = False

    def read_frame(self) -> Optional[List[float]]:
        """读取一帧数据,返回 16 通道温度值(°C)。

        返回 None 表示暂无完整帧(超时)或已停止。
        线程在 _stop_event 被设置时能快速退出(最多 READ_TIMEOUT 秒)。
        """
        if not self._acquiring or self._sock is None:
            return None
        buf = bytearray()
        # 按定长 64 字节读取,直到收满或被停止
        while len(buf) < FRAME_SIZE and not self._stop_event.is_set():
            try:
                chunk = self._sock.recv(FRAME_SIZE - len(buf))
                if not chunk:
                    # 对端关闭连接
                    return None
                buf.extend(chunk)
            except socket.timeout:
                # 短超时让 stop 能及时响应
                continue
            except OSError:
                return None
        if len(buf) < FRAME_SIZE:
            return None
        # 16 × float32 LE 解包;CH15→CH0 反转为 CH0→CH15
        temps = list(struct.unpack("<16f", bytes(buf)))
        temps.reverse()
        return temps

    def _send_cmd(self, sock: socket.socket, cmd: str) -> str:
        """发送 SCPI 命令并读取一行响应(以 \\n 结尾)。

        命令阶段用 1s 超时,确保设备响应慢也能等到。
        读响应按字节读,遇到 \\n 即结束,兼容 \r\n。
        """
        sock.settimeout(CMD_TIMEOUT)
        sock.sendall(cmd.encode())
        buf = bytearray()
        # 单字节读,直到遇到换行或超时
        while len(buf) < 1024:
            try:
                byte = sock.recv(1)
                if not byte:
                    break
                if byte == b"\n":
                    break
                buf.extend(byte)
            except socket.timeout:
                break  # 超时返回已读到的部分
        return buf.decode(errors="ignore").strip()

    def _drain_recv(self, timeout: float) -> None:
        """排空接收缓冲区,总耗时不超过 timeout 秒。

        关键设计:用单调时钟限制总时长,而不是依赖单次 recv 超时。
        原因:设备在采集状态下会持续高速发数据帧,单次 recv 永远能在
        timeout 内收到数据,导致 while True 循环永不退出 → start_acquisition
        持锁卡死,UI 表现为"开始采集后卡住"。

        解决:deadline 到了就停,无论是否还有数据。剩余数据留给 read_frame
        的帧解析逻辑去消费(它会按 64 字节定长切帧)。
        """
        if self._sock is None:
            return
        deadline = time.monotonic() + timeout
        # 单次 recv 超时设短一点(50ms),让循环能频繁检查 deadline
        self._sock.settimeout(0.05)
        try:
            while time.monotonic() < deadline:
                try:
                    chunk = self._sock.recv(4096)
                    if not chunk:
                        break  # 对端关闭
                except socket.timeout:
                    break  # 50ms 内无数据,认为缓冲已空
                except OSError:
                    break
        finally:
            self._sock.settimeout(READ_TIMEOUT)


# ======== 采集线程 ========
class AcquisitionWorker(QThread):
    """采集线程,通过信号向主线程推送数据。

    特性:
      - 按 display_hz 控制 UI 刷新频率(设备约 100Hz,默认 10Hz 更新 UI)
      - 用时间戳间隔控制,不依赖设备帧率,适应不同设备的实际速率
      - 信号通过 Qt 事件循环投递到主线程,UI 更新线程安全
    """

    # 使用 object 而非 list 类型,避免 PyInstaller 打包时信号类型解析问题
    data_received = pyqtSignal(object)
    error_occurred = pyqtSignal(str)
    finished_acquisition = pyqtSignal()

    def __init__(self, device: T1603Device, display_hz: int = 10,
                 parent: Optional[QWidget] = None) -> None:
        super().__init__(parent)
        self._device = device
        self._display_hz = display_hz

    def set_display_hz(self, hz: int) -> None:
        """运行中动态调整显示频率(线程安全)。"""
        self._display_hz = max(1, hz)

    def run(self) -> None:
        """线程主循环:持续读帧,按频率控制 UI 更新节奏。

        频率控制策略:
          - 用 time.monotonic() 计算最小间隔,不依赖假设的设备帧率
          - 例如 10Hz → 最小间隔 100ms,帧率高于 100ms 的帧会被丢弃
          - 帧率低于设定值的场景下,每一帧都会显示(实际频率可能低于设定值)
        """
        consecutive_none = 0
        # 上一次发射信号的时间戳(单调时钟,不受系统时间调整影响)
        last_emit = 0.0
        while self._device.acquiring and not self.isInterruptionRequested():
            try:
                temps = self._device.read_frame()
                if temps is None:
                    consecutive_none += 1
                    if consecutive_none > 50:
                        self.error_occurred.emit("连续 10 秒无数据,连接可能已断开")
                        break
                    continue
                consecutive_none = 0
                # 按频率控制 UI 更新节奏
                now = time.monotonic()
                min_interval = 1.0 / self._display_hz
                if now - last_emit >= min_interval:
                    last_emit = now
                    self.data_received.emit(temps)
            except Exception as e:  # noqa: BLE001 - 采集线程不应因任何异常崩溃
                self.error_occurred.emit(f"采集错误: {e}")
                break
        self.finished_acquisition.emit()


# ======== 主窗口 ========
class MainWindow(QWidget):
    """主窗口:极简 UI。

    布局自上而下:
      1. 顶部连接区:IP + 端口 + 连接/断开
      2. 中部采集控制区:开始/停止采集 + 状态显示
      3. 通道数据区:4×4 网格显示 16 通道实时温度
      4. 热电偶配置区:4×4 网格 16 个下拉框 + 应用按钮
    """

    def __init__(self) -> None:
        super().__init__()
        self._device = T1603Device()
        self._worker: Optional[AcquisitionWorker] = None
        self._frame_count = 0
        self._init_ui()
        self._connect_signals()
        self._update_button_states()

    def _init_ui(self) -> None:
        """构建 UI 控件与布局。"""
        self.setWindowTitle("DAQ-T-1603 温度采集 (Win7 版)")
        self.resize(900, 700)

        root = QVBoxLayout(self)

        # ---- 1. 连接区 ----
        conn_group = QGroupBox("设备连接")
        conn_layout = QHBoxLayout(conn_group)
        conn_layout.addWidget(QLabel("IP:"))
        self.ip_edit = QLineEdit(DEFAULT_HOST)
        self.ip_edit.setFixedWidth(160)
        conn_layout.addWidget(self.ip_edit)
        conn_layout.addWidget(QLabel("端口:"))
        self.port_edit = QLineEdit(str(DEFAULT_PORT))
        self.port_edit.setFixedWidth(60)
        conn_layout.addWidget(self.port_edit)
        self.connect_btn = QPushButton("连接")
        self.disconnect_btn = QPushButton("断开")
        conn_layout.addWidget(self.connect_btn)
        conn_layout.addWidget(self.disconnect_btn)
        conn_layout.addStretch()
        self.status_label = QLabel("状态: 未连接")
        self.status_label.setStyleSheet("color: #888;")
        conn_layout.addWidget(self.status_label)
        root.addWidget(conn_group)

        # ---- 2. 采集控制区 ----
        acq_group = QGroupBox("采集控制")
        acq_layout = QHBoxLayout(acq_group)
        self.start_btn = QPushButton("开始采集")
        self.stop_btn = QPushButton("停止采集")
        # 刷新频率设置:设备约 100Hz,默认 10Hz 更新 UI
        acq_layout.addWidget(QLabel("刷新率:"))
        self.hz_combo = QComboBox()
        for hz in [1, 2, 5, 10, 20, 50, 100]:
            self.hz_combo.addItem(f"{hz} Hz", hz)
        self.hz_combo.setCurrentIndex(3)  # 默认 10Hz
        self.hz_combo.setToolTip("UI 显示刷新频率(设备采集频率约 100Hz,此值仅控制显示刷新节奏)")
        acq_layout.addWidget(self.hz_combo)
        self.frame_count_label = QLabel("已收帧数: 0")
        self.frame_count_label.setStyleSheet("color: #666; font-size: 12px;")
        acq_layout.addWidget(self.start_btn)
        acq_layout.addWidget(self.stop_btn)
        acq_layout.addWidget(self.frame_count_label)
        acq_layout.addStretch()
        root.addWidget(acq_group)

        # ---- 3. 通道数据区 ----
        data_group = QGroupBox("通道数据 (°C)")
        data_layout = QGridLayout(data_group)
        data_layout.setSpacing(8)
        self.channel_labels: List[QLabel] = []
        # 4×4 网格布局,CH01..CH16
        for i in range(CHANNEL_COUNT):
            row = i // 4
            col = i % 4
            cell = QVBoxLayout()
            name_label = QLabel(f"CH{i + 1:02d}")
            name_label.setAlignment(Qt.AlignCenter)
            name_label.setStyleSheet("color: #666; font-size: 12px;")
            value_label = QLabel("----")
            value_label.setAlignment(Qt.AlignCenter)
            # 大字号突出显示核心数据,符合用户"关键数据大字号"偏好
            value_font = QFont()
            value_font.setPointSize(16)
            value_font.setBold(True)
            value_label.setFont(value_font)
            cell.addWidget(name_label)
            cell.addWidget(value_label)
            wrapper = QWidget()
            wrapper.setLayout(cell)
            data_layout.addWidget(wrapper, row, col)
            self.channel_labels.append(value_label)
        root.addWidget(data_group, stretch=1)

        # ---- 4. 热电偶配置区 ----
        tc_group = QGroupBox("热电偶类型配置")
        tc_layout = QGridLayout(tc_group)
        tc_layout.setSpacing(6)

        # 全部通道一站式设置(首行)
        tc_layout.addWidget(QLabel("全部设为:"), 0, 0)
        self.all_tc_combo = QComboBox()
        for tc_type in THERMOCOUPLE_TYPES:
            self.all_tc_combo.addItem(tc_type)
        self.all_tc_combo.setCurrentIndex(0)  # 默认 K 型
        self.all_tc_combo.setToolTip("选择后自动应用到全部 16 通道")
        tc_layout.addWidget(self.all_tc_combo, 0, 1)
        # 占位,使全部设为不拉太长
        placeholder = QLabel("(选择后自动应用到全部通道)")
        placeholder.setStyleSheet("color: #999; font-size: 11px;")
        tc_layout.addWidget(placeholder, 0, 2, 1, 2)

        # 16 通道独立设置
        self.tc_combos: List[QComboBox] = []
        for i in range(CHANNEL_COUNT):
            row = i // 4 + 1  # 从第 1 行开始(第 0 行是全部设为)
            col = i % 4
            cell = QHBoxLayout()
            cell.addWidget(QLabel(f"CH{i + 1:02d}:"))
            combo = QComboBox()
            for tc_type in THERMOCOUPLE_TYPES:
                combo.addItem(tc_type)
            combo.setCurrentIndex(0)  # 默认 K 型
            cell.addWidget(combo)
            cell.addStretch()
            wrapper = QWidget()
            wrapper.setLayout(cell)
            tc_layout.addWidget(wrapper, row, col)
            self.tc_combos.append(combo)
        # 应用按钮独占一行
        apply_row = QHBoxLayout()
        apply_row.addStretch()
        self.apply_tc_btn = QPushButton("应用热电偶配置")
        apply_row.addWidget(self.apply_tc_btn)
        apply_wrapper = QWidget()
        apply_wrapper.setLayout(apply_row)
        tc_layout.addWidget(apply_wrapper, 5, 0, 1, 4)
        root.addWidget(tc_group)

    def _connect_signals(self) -> None:
        """连接信号槽。"""
        self.connect_btn.clicked.connect(self._on_connect)
        self.disconnect_btn.clicked.connect(self._on_disconnect)
        self.start_btn.clicked.connect(self._on_start)
        self.stop_btn.clicked.connect(self._on_stop)
        self.apply_tc_btn.clicked.connect(self._on_apply_tc)
        # 全部设为→同步到所有通道
        self.all_tc_combo.currentTextChanged.connect(self._on_all_tc_changed)

    def _update_button_states(self) -> None:
        """根据设备状态更新按钮启用/禁用。"""
        connected = self._device.connected
        acquiring = self._device.acquiring
        self.connect_btn.setEnabled(not connected)
        self.disconnect_btn.setEnabled(connected and not acquiring)
        self.start_btn.setEnabled(connected and not acquiring)
        self.stop_btn.setEnabled(connected and acquiring)
        self.apply_tc_btn.setEnabled(connected and not acquiring)
        self.hz_combo.setEnabled(not acquiring)  # 采集中禁止修改频率
        self.ip_edit.setEnabled(not connected)
        self.port_edit.setEnabled(not connected)

        # 状态文本与颜色
        if not connected:
            self.status_label.setText("状态: 未连接")
            self.status_label.setStyleSheet("color: #888;")
        elif acquiring:
            self.status_label.setText("状态: 采集中")
            self.status_label.setStyleSheet("color: #2196F3; font-weight: bold;")
        else:
            self.status_label.setText("状态: 已连接")
            self.status_label.setStyleSheet("color: #4CAF50; font-weight: bold;")

    # ---- 事件处理 ----
    def _on_connect(self) -> None:
        """连接设备。"""
        host = self.ip_edit.text().strip()
        port_text = self.port_edit.text().strip()
        if not host:
            QMessageBox.warning(self, "输入错误", "请输入设备 IP")
            return
        try:
            port = int(port_text)
        except ValueError:
            QMessageBox.warning(self, "输入错误", "端口必须是数字")
            return
        if not (1 <= port <= 65535):
            QMessageBox.warning(self, "输入错误", "端口范围 1-65535")
            return

        self._device.host = host
        self._device.port = port
        try:
            self._device.connect()
        except Exception as e:  # noqa: BLE001 - 连接错误需展示给用户
            QMessageBox.critical(self, "连接失败", str(e))
            return
        self._update_button_states()

    def _on_disconnect(self) -> None:
        """断开设备。"""
        self._device.disconnect()
        self._clear_channel_display()
        self._update_button_states()

    def _on_start(self) -> None:
        """开始采集。"""
        try:
            self._device.start_acquisition()
        except Exception as e:  # noqa: BLE001 - 启动错误需展示给用户
            QMessageBox.critical(self, "启动采集失败", str(e))
            return
        # 读取当前设定的刷新频率
        display_hz = self.hz_combo.currentData()
        # 启动采集线程
        self._worker = AcquisitionWorker(self._device, display_hz=display_hz, parent=self)
        self._worker.data_received.connect(self._on_data_received)
        self._worker.error_occurred.connect(self._on_acq_error)
        self._worker.finished_acquisition.connect(self._on_acq_finished)
        self._worker.start()
        self._update_button_states()

    def _on_stop(self) -> None:
        """停止采集。"""
        self._device.stop_acquisition()
        # 等待线程退出(最多 1 秒)
        if self._worker is not None and self._worker.isRunning():
            self._worker.wait(1000)
            if self._worker.isRunning():
                self._worker.requestInterruption()
                self._worker.wait(1000)
        self._worker = None
        self._frame_count = 0
        self._update_button_states()

    def _on_apply_tc(self) -> None:
        """应用热电偶配置到设备。"""
        types = "".join(combo.currentText() for combo in self.tc_combos)
        try:
            self._device.set_thermocouple_types(types)
        except Exception as e:  # noqa: BLE001 - 配置错误需展示给用户
            QMessageBox.critical(self, "配置失败", str(e))
            return
        QMessageBox.information(self, "成功", f"已应用热电偶配置: {types}")

    def _on_all_tc_changed(self, tc_type: str) -> None:
        """全部设为切换处理:自动同步到所有通道。"""
        # 找到当前选中的索引
        idx = self.all_tc_combo.currentIndex()
        # 应用到所有通道
        for combo in self.tc_combos:
            combo.setCurrentIndex(idx)

    def _on_data_received(self, temps: list) -> None:
        """更新通道数据显示(主线程槽,线程安全)。"""
        self._frame_count += 1
        self.frame_count_label.setText(f"已收帧数: {self._frame_count}")
        for i, temp in enumerate(temps):
            if i >= CHANNEL_COUNT:
                break
            label = self.channel_labels[i]
            # NaN/Inf 显示为 ----
            if isinstance(temp, float) and (temp != temp or abs(temp) > 5000):
                label.setText("----")
                label.setStyleSheet("color: #F44336;")
            else:
                label.setText(f"{temp:.2f}")
                # 通道温度正常用深色,异常超范围用红色
                if -200 <= temp <= 1350:
                    label.setStyleSheet("color: #212121;")
                else:
                    label.setStyleSheet("color: #FF9800;")

    def _on_acq_error(self, msg: str) -> None:
        """采集线程错误处理。"""
        QMessageBox.critical(self, "采集错误", msg)
        # 错误后清理状态
        self._device.stop_acquisition()
        self._update_button_states()

    def _on_acq_finished(self) -> None:
        """采集线程退出后的清理。"""
        self._update_button_states()

    def _clear_channel_display(self) -> None:
        """清空通道显示和帧计数。"""
        for label in self.channel_labels:
            label.setText("----")
            label.setStyleSheet("color: #212121;")
        self._frame_count = 0
        self.frame_count_label.setText("已收帧数: 0")

    def closeEvent(self, event) -> None:  # noqa: N802 - Qt 命名约定
        """窗口关闭时清理资源。"""
        if self._worker is not None and self._worker.isRunning():
            self._device.stop_acquisition()
            self._worker.requestInterruption()
            self._worker.wait(2000)
        self._device.disconnect()
        super().closeEvent(event)


# ======== 入口 ========
def main() -> int:
    app = QApplication(sys.argv)
    app.setApplicationName("DAQ-T-1603 Win7")
    window = MainWindow()
    window.show()
    return app.exec_()


if __name__ == "__main__":
    sys.exit(main())
