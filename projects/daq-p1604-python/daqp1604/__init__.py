# -*- coding: utf-8 -*-
"""DAQ-P-1604 压力采集设备 Python 客户端库。

特性：
  - TCP 连接 / 断开（自动启用长度前缀模式）
  - 数据流采集启停（18 通道：16 压力 + 大气压 + 温度）
  - 单位读写（psi / Pa / kPa / MPa / kgf/cm2）
  - 轮询备用模式（rFFFF0）
  - UDP 设备发现

零第三方依赖，纯标准库实现（Python 3.8+）。

快速开始::

    from daqp1604 import DAQP1604Device

    dev = DAQP1604Device("192.168.3.101", 9000)
    dev.on_data = lambda p: print(p["channels"])
    dev.connect()
    dev.start_acquisition(period_ms=100)
    ...
    dev.stop_acquisition()
    dev.disconnect()
"""

from .device import (
    DAQP1604Device,
    DAQP1604Error,
    NotConnectedError,
    CommandError,
    TimeoutError,
    create_device,
)
from .discovery import DiscoveredDevice, scan_devices
from .units import (
    PRESSURE_UNIT_COEFFICIENT,
    DEFAULT_UNIT,
    coefficient_for_unit,
    match_unit_by_coefficient,
    is_supported_unit,
)

__version__ = "1.0.0"
__all__ = [
    "DAQP1604Device",
    "DAQP1604Error",
    "NotConnectedError",
    "CommandError",
    "TimeoutError",
    "create_device",
    "DiscoveredDevice",
    "scan_devices",
    "PRESSURE_UNIT_COEFFICIENT",
    "DEFAULT_UNIT",
    "coefficient_for_unit",
    "match_unit_by_coefficient",
    "is_supported_unit",
    "__version__",
]
