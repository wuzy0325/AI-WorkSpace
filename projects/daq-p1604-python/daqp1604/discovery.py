# -*- coding: utf-8 -*-
"""DAQ-P-1604 设备发现（UDP 广播）。

协议：向局域网广播地址:7000 发送 ``psi9000``，在 7001 端口监听响应。
响应 CSV 格式：``<IP>,<MAC>,,<序列号>,<固件版本>,,<端口>,<子网掩码>,<网关>``
"""

from __future__ import annotations

import ipaddress
import socket
import time
from typing import List, Optional

DISCOVERY_CMD = b"psi9000"
SEND_PORT = 7000
LISTEN_PORT = 7001
DEFAULT_SCAN_TIMEOUT = 3.0


class DiscoveredDevice:
    """发现到的设备信息。"""

    __slots__ = ("ip", "mac", "serial", "firmware", "port", "netmask", "gateway")

    def __init__(self, ip: str, mac: str, serial: str, firmware: str,
                 port: int, netmask: str, gateway: str) -> None:
        self.ip = ip
        self.mac = mac
        self.serial = serial
        self.firmware = firmware
        self.port = port
        self.netmask = netmask
        self.gateway = gateway

    def __repr__(self) -> str:
        return (f"DiscoveredDevice(ip={self.ip!r}, port={self.port}, "
                f"serial={self.serial!r}, firmware={self.firmware!r})")


def _broadcast_addresses() -> List[str]:
    """枚举所有非内部 IPv4 网卡的广播地址（address | ~netmask）。"""
    addrs = []
    try:
        hostname = socket.gethostname()
        # 通过 UDP connect 获取本机各接口地址，再按 /24 推导广播地址
        for info in socket.getaddrinfo(hostname, None, socket.AF_INET):
            ip = info[4][0]
            if ip.startswith("127."):
                continue
            addrs.append(_directed_broadcast(ip))
    except OSError:
        pass
    return [a for a in addrs if a]


def _directed_broadcast(ip: str, prefix: int = 24) -> str:
    """计算 IPv4 地址的定向广播地址（假设 /24 网段，与局域网部署一致）。"""
    net = ipaddress.ip_network(f"{ip}/{prefix}", strict=False)
    return str(net.broadcast_address)


def scan_devices(timeout: float = DEFAULT_SCAN_TIMEOUT) -> List[DiscoveredDevice]:
    """在局域网内广播 ``psi9000`` 并监听设备响应。

    :param timeout: 总扫描等待时间（秒）
    :return: 发现的设备列表（可能为空）
    """
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    sock.setsockopt(socket.SOL_SOCKET, socket.SO_BROADCAST, 1)
    try:
        sock.bind(("", LISTEN_PORT))
    except OSError:
        # 7001 被占用时退回随机端口
        sock.bind(("", 0))
    sock.settimeout(0.2)

    devices = []
    deadline = time.monotonic() + timeout
    try:
        for bcast in _broadcast_addresses() or ["255.255.255.255"]:
            try:
                sock.sendto(DISCOVERY_CMD, (bcast, SEND_PORT))
            except OSError:
                continue

        while time.monotonic() < deadline:
            try:
                sock.settimeout(deadline - time.monotonic())
                data, _ = sock.recvfrom(1024)
            except socket.timeout:
                break
            except OSError:
                break
            parts = data.decode("ascii", errors="ignore").split(",")
            if len(parts) < 10:
                continue
            try:
                port = int(parts[7].strip())
            except (ValueError, IndexError):
                port = 0
            devices.append(DiscoveredDevice(
                ip=parts[0].strip(),
                mac=parts[1].strip(),
                serial=parts[3].strip(),
                firmware=parts[4].strip(),
                port=port,
                netmask=parts[8].strip(),
                gateway=parts[9].strip(),
            ))
    finally:
        sock.close()
    return devices
