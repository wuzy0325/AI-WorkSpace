# -*- coding: utf-8 -*-
"""DAQ-P-1604 压力单位与 EU 系数换算。

设备内部以 psi 为基准的 EU 系数（float32 存储）表达压力单位，
数据流中的压力值已经按该系数完成 EU 换算，应用层无需再换算。
"""

PRESSURE_UNIT_COEFFICIENT = {
    "psi": 1.0,
    "Pa": 6894.757,
    "kPa": 6.894757,
    "MPa": 0.006894757,
    "kgf/cm2": 0.070307,
}

DEFAULT_UNIT = "psi"

# 相对容差：设备以 float32 存储系数有精度损失
# （如 Pa 系数 6894.757 在 float32 中为 6894.756836），
# 1e-4 的相对容差可覆盖该损失，同时不会误匹配相邻单位。
UNIT_COEFF_TOLERANCE = 1e-4


def coefficient_for_unit(unit: str) -> float:
    """返回指定压力单位的 EU 系数。"""
    try:
        return PRESSURE_UNIT_COEFFICIENT[unit]
    except KeyError:
        raise ValueError(
            "unsupported unit: %s (supported: %s)"
            % (unit, ", ".join(sorted(PRESSURE_UNIT_COEFFICIENT)))
        ) from None


def match_unit_by_coefficient(coeff: float) -> str:
    """根据 EU 系数反查最接近的压力单位。

    使用相对容差 UNIT_COEFF_TOLERANCE（1e-4），覆盖设备 float32 存储精度损失。
    匹配失败返回 ""。
    """
    best = ""
    best_diff = float("inf")
    for unit, expected in PRESSURE_UNIT_COEFFICIENT.items():
        base = abs(expected)
        if base < 1e-9:
            diff = abs(coeff - expected)
        else:
            diff = abs(coeff - expected) / base
        if diff < UNIT_COEFF_TOLERANCE and diff < best_diff:
            best = unit
            best_diff = diff
    return best


def is_supported_unit(unit: str) -> bool:
    """判断单位是否在硬件支持的列表内。"""
    return unit in PRESSURE_UNIT_COEFFICIENT
