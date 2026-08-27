#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Cal1604 桌面端图标生成器。

输出以下资源：
- build/appicon.svg
- build/appicon.png
- build/appicon_1024.png
- build/appicon_512.png
- build/appicon_256.png
- build/windows/icon.ico

说明：
- Wails 构建默认直接读取 build/windows/icon.ico。
- 只有在显式传入 --refresh-syso 时，才额外刷新根目录
  resource_windows_amd64.syso，供纯 go build 链路使用。
"""

from __future__ import annotations

import argparse
import math
import shutil
import subprocess
from pathlib import Path

from PIL import Image, ImageDraw

CANVAS_SIZE = 1024

BACKGROUND_TOP = (54, 58, 67)
BACKGROUND_BOTTOM = (19, 21, 26)
BACKGROUND_EDGE = (11, 13, 16, 255)
HIGHLIGHT = (255, 255, 255, 22)
WHITE = (245, 246, 248, 255)
WHITE_SOFT = (255, 255, 255, 92)
WHITE_DIM = (255, 255, 255, 40)
AMBER = (255, 198, 56, 255)
AMBER_BRIGHT = (255, 220, 112, 255)
SURFACE_SHADOW = (0, 0, 0, 32)


def lerp_color(start: tuple[int, ...], end: tuple[int, ...], ratio: float) -> tuple[int, ...]:
    """在线性插值中混合颜色。"""
    return tuple(int(a + (b - a) * ratio) for a, b in zip(start, end))


def polar_point(cx: float, cy: float, radius: float, degrees: float) -> tuple[float, float]:
    """将极坐标角度转换为画布坐标。"""
    radians = math.radians(degrees)
    return cx + math.cos(radians) * radius, cy + math.sin(radians) * radius


def build_tick_segments(
    cx: float,
    cy: float,
    radius: float,
    inner_radius: float,
    angles: list[float],
) -> list[tuple[tuple[float, float], tuple[float, float], int]]:
    """生成刻度线的起止点，供 PIL 和 SVG 共用。"""
    segments: list[tuple[tuple[float, float], tuple[float, float], int]] = []
    middle_angle = angles[len(angles) // 2]
    for angle in angles:
        outer = polar_point(cx, cy, radius, angle)
        inner = polar_point(cx, cy, inner_radius, angle)
        stroke = 20 if angle == middle_angle else 14
        segments.append((inner, outer, stroke))
    return segments


def draw_background(size: int) -> Image.Image:
    """绘制深色圆角底板和柔和高光。"""
    image = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    mask = Image.new("L", (size, size), 0)
    mask_draw = ImageDraw.Draw(mask)

    padding = int(size * 0.045)
    radius = int(size * 0.21)
    bounds = [padding, padding, size - padding, size - padding]
    mask_draw.rounded_rectangle(bounds, radius=radius, fill=255)

    gradient = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    gradient_draw = ImageDraw.Draw(gradient)
    for y in range(size):
        ratio = y / (size - 1)
        color = lerp_color(BACKGROUND_TOP, BACKGROUND_BOTTOM, ratio)
        gradient_draw.line([(0, y), (size, y)], fill=(*color, 255))

    glow_layer = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    glow_draw = ImageDraw.Draw(glow_layer)
    glow_center = (size * 0.5, size * 0.28)
    max_radius = int(size * 0.48)
    for step in range(max_radius, 0, -8):
        alpha = int(22 * (step / max_radius))
        glow_draw.ellipse(
            [
                glow_center[0] - step,
                glow_center[1] - step * 0.82,
                glow_center[0] + step,
                glow_center[1] + step * 0.82,
            ],
            fill=(73, 80, 94, alpha),
        )

    highlight_layer = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    highlight_draw = ImageDraw.Draw(highlight_layer)
    highlight_draw.ellipse(
        [
            int(size * -0.02),
            int(size * -0.08),
            int(size * 0.52),
            int(size * 0.30),
        ],
        fill=HIGHLIGHT,
    )

    border_layer = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    border_draw = ImageDraw.Draw(border_layer)
    border_draw.rounded_rectangle(bounds, radius=radius, outline=BACKGROUND_EDGE, width=8)

    combined = Image.alpha_composite(gradient, glow_layer)
    combined = Image.alpha_composite(combined, highlight_layer)
    combined.putalpha(mask)
    image.alpha_composite(combined)
    image.alpha_composite(border_layer)
    return image


def draw_symbol(size: int) -> Image.Image:
    """绘制仪表盘、指针与校准勾。"""
    image = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    draw = ImageDraw.Draw(image)

    center_x = size * 0.5
    center_y = size * 0.59
    radius = size * 0.23
    arc_width = max(30, size // 22)
    bbox = [center_x - radius, center_y - radius, center_x + radius, center_y + radius]

    # 先铺一层弱阴影，避免高对比白色主体显得悬浮。
    shadow_bbox = [value + (8 if index % 2 == 0 else 14) for index, value in enumerate(bbox)]
    draw.arc(shadow_bbox, start=204, end=336, fill=SURFACE_SHADOW, width=arc_width)
    draw.arc(bbox, start=204, end=336, fill=WHITE, width=arc_width)

    inner_radius = radius * 0.78
    inner_bbox = [
        center_x - inner_radius,
        center_y - inner_radius,
        center_x + inner_radius,
        center_y + inner_radius,
    ]
    draw.arc(inner_bbox, start=214, end=326, fill=WHITE_DIM, width=max(6, size // 180))

    tick_segments = build_tick_segments(
        center_x,
        center_y,
        radius - arc_width * 0.08,
        radius - arc_width * 0.72,
        [214, 242, 270, 298, 326],
    )
    for start_point, end_point, stroke in tick_segments:
        draw.line([start_point, end_point], fill=WHITE_SOFT, width=stroke)

    needle_angle = 322
    tip = polar_point(center_x, center_y, radius * 0.76, needle_angle)
    tail = polar_point(center_x, center_y, radius * 0.20, needle_angle + 180)
    draw.line([tail, tip], fill=WHITE, width=max(18, size // 58))
    draw.line([(center_x, center_y), tip], fill=WHITE, width=max(12, size // 78))

    hub_outer = max(28, size // 24)
    hub_inner = max(12, size // 52)
    draw.ellipse(
        [center_x - hub_outer, center_y - hub_outer, center_x + hub_outer, center_y + hub_outer],
        fill=AMBER,
    )
    draw.ellipse(
        [center_x - hub_inner, center_y - hub_inner, center_x + hub_inner, center_y + hub_inner],
        fill=BACKGROUND_BOTTOM + (255,),
    )

    badge_center_x = size * 0.74
    badge_center_y = size * 0.23
    badge_radius = size * 0.08
    badge_box = [
        badge_center_x - badge_radius,
        badge_center_y - badge_radius,
        badge_center_x + badge_radius,
        badge_center_y + badge_radius,
    ]
    draw.arc(
        badge_box,
        start=210,
        end=510,
        fill=(*AMBER_BRIGHT[:3], 200),
        width=max(14, size // 70),
    )

    check_width = max(18, size // 58)
    check_points = [
        (badge_center_x - badge_radius * 0.42, badge_center_y + badge_radius * 0.08),
        (badge_center_x - badge_radius * 0.08, badge_center_y + badge_radius * 0.42),
        (badge_center_x + badge_radius * 0.48, badge_center_y - badge_radius * 0.26),
    ]
    draw.line(check_points[:2], fill=AMBER, width=check_width)
    draw.line(check_points[1:], fill=AMBER, width=check_width)

    return image


def draw_icon(size: int = CANVAS_SIZE) -> Image.Image:
    """合成最终图标。"""
    background = draw_background(size)
    symbol = draw_symbol(size)
    return Image.alpha_composite(background, symbol)


def svg_arc_path(cx: float, cy: float, radius: float, start_angle: float, end_angle: float) -> str:
    """生成 SVG 圆弧路径。"""
    start = polar_point(cx, cy, radius, start_angle)
    end = polar_point(cx, cy, radius, end_angle)
    large_arc = 1 if abs(end_angle - start_angle) > 180 else 0
    return (
        f"M {start[0]:.2f} {start[1]:.2f} "
        f"A {radius:.2f} {radius:.2f} 0 {large_arc} 1 {end[0]:.2f} {end[1]:.2f}"
    )


def generate_svg(size: int = CANVAS_SIZE) -> str:
    """输出可维护的 SVG 源文件。"""
    padding = size * 0.045
    radius = size * 0.21
    center_x = size * 0.5
    center_y = size * 0.59
    gauge_radius = size * 0.23
    arc_width = max(30, size // 22)
    inner_radius = gauge_radius * 0.78
    hub_outer = max(28, size // 24)
    hub_inner = max(12, size // 52)
    badge_center_x = size * 0.74
    badge_center_y = size * 0.23
    badge_radius = size * 0.08

    arc_path = svg_arc_path(center_x, center_y, gauge_radius, 204, 336)
    inner_arc_path = svg_arc_path(center_x, center_y, inner_radius, 214, 326)
    badge_arc_path = svg_arc_path(badge_center_x, badge_center_y, badge_radius, 210, 510)

    tick_lines = []
    for start_point, end_point, stroke in build_tick_segments(
        center_x,
        center_y,
        gauge_radius - arc_width * 0.08,
        gauge_radius - arc_width * 0.72,
        [214, 242, 270, 298, 326],
    ):
        tick_lines.append(
            f'<line x1="{start_point[0]:.2f}" y1="{start_point[1]:.2f}" '
            f'x2="{end_point[0]:.2f}" y2="{end_point[1]:.2f}" '
            f'stroke="#ffffff" stroke-opacity="0.36" stroke-width="{stroke}" '
            'stroke-linecap="round" />'
        )

    needle_tip = polar_point(center_x, center_y, gauge_radius * 0.76, 322)
    needle_tail = polar_point(center_x, center_y, gauge_radius * 0.20, 142)
    check_points = [
        (badge_center_x - badge_radius * 0.42, badge_center_y + badge_radius * 0.08),
        (badge_center_x - badge_radius * 0.08, badge_center_y + badge_radius * 0.42),
        (badge_center_x + badge_radius * 0.48, badge_center_y - badge_radius * 0.26),
    ]

    return f"""<?xml version="1.0" encoding="UTF-8"?>
<svg width="{size}" height="{size}" viewBox="0 0 {size} {size}" xmlns="http://www.w3.org/2000/svg">
  <defs>
    <linearGradient id="bgGradient" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0%" stop-color="#363a43"/>
      <stop offset="100%" stop-color="#13151a"/>
    </linearGradient>
    <radialGradient id="glowGradient" cx="50%" cy="28%" r="48%">
      <stop offset="0%" stop-color="#49505e" stop-opacity="0.38"/>
      <stop offset="100%" stop-color="#49505e" stop-opacity="0"/>
    </radialGradient>
    <linearGradient id="amberGradient" x1="0" y1="0" x2="1" y2="1">
      <stop offset="0%" stop-color="#ffdc70"/>
      <stop offset="100%" stop-color="#ffc638"/>
    </linearGradient>
    <clipPath id="tileClip">
      <rect x="{padding:.2f}" y="{padding:.2f}" width="{size - padding * 2:.2f}" height="{size - padding * 2:.2f}" rx="{radius:.2f}" ry="{radius:.2f}" />
    </clipPath>
  </defs>

  <g clip-path="url(#tileClip)">
    <rect width="{size}" height="{size}" fill="url(#bgGradient)" />
    <ellipse cx="{size * 0.5:.2f}" cy="{size * 0.28:.2f}" rx="{size * 0.48:.2f}" ry="{size * 0.39:.2f}" fill="url(#glowGradient)" />
    <ellipse cx="{size * 0.25:.2f}" cy="{size * 0.11:.2f}" rx="{size * 0.27:.2f}" ry="{size * 0.19:.2f}" fill="#ffffff" fill-opacity="0.08" />
  </g>

  <rect x="{padding:.2f}" y="{padding:.2f}" width="{size - padding * 2:.2f}" height="{size - padding * 2:.2f}" rx="{radius:.2f}" ry="{radius:.2f}" fill="none" stroke="#0b0d10" stroke-width="8" />

  <path d="{arc_path}" fill="none" stroke="#f5f6f8" stroke-width="{arc_width}" stroke-linecap="round" />
  <path d="{inner_arc_path}" fill="none" stroke="#ffffff" stroke-opacity="0.16" stroke-width="{max(6, size // 180)}" stroke-linecap="round" />
  {" ".join(tick_lines)}

  <line x1="{needle_tail[0]:.2f}" y1="{needle_tail[1]:.2f}" x2="{needle_tip[0]:.2f}" y2="{needle_tip[1]:.2f}"
        stroke="#f5f6f8" stroke-width="{max(18, size // 58)}" stroke-linecap="round" />
  <line x1="{center_x:.2f}" y1="{center_y:.2f}" x2="{needle_tip[0]:.2f}" y2="{needle_tip[1]:.2f}"
        stroke="#f5f6f8" stroke-width="{max(12, size // 78)}" stroke-linecap="round" />

  <circle cx="{center_x:.2f}" cy="{center_y:.2f}" r="{hub_outer}" fill="url(#amberGradient)" />
  <circle cx="{center_x:.2f}" cy="{center_y:.2f}" r="{hub_inner}" fill="#13151a" />

  <path d="{badge_arc_path}" fill="none" stroke="url(#amberGradient)" stroke-width="{max(14, size // 70)}" stroke-linecap="round" />
  <path d="M {check_points[0][0]:.2f} {check_points[0][1]:.2f} L {check_points[1][0]:.2f} {check_points[1][1]:.2f} L {check_points[2][0]:.2f} {check_points[2][1]:.2f}"
        fill="none" stroke="url(#amberGradient)" stroke-width="{max(18, size // 58)}" stroke-linecap="round" stroke-linejoin="round" />
</svg>
"""


def save_png_assets(build_dir: Path, icon: Image.Image) -> None:
    """输出 PNG 资源。"""
    targets = {
        "appicon.png": CANVAS_SIZE,
        "appicon_1024.png": CANVAS_SIZE,
        "appicon_512.png": 512,
        "appicon_256.png": 256,
    }
    for filename, size in targets.items():
        target = build_dir / filename
        output = icon if size == CANVAS_SIZE else icon.resize((size, size), Image.Resampling.LANCZOS)
        output.save(target, "PNG")
        print(f"PNG: {target}")


def save_svg_asset(build_dir: Path) -> None:
    """输出 SVG 源文件。"""
    target = build_dir / "appicon.svg"
    target.write_text(generate_svg(), encoding="utf-8")
    print(f"SVG: {target}")


def save_ico_asset(windows_dir: Path, icon: Image.Image) -> None:
    """输出 Windows ICO。"""
    target = windows_dir / "icon.ico"
    icon.save(
        target,
        format="ICO",
        sizes=[(256, 256), (128, 128), (64, 64), (48, 48), (32, 32), (24, 24), (16, 16)],
    )
    print(f"ICO: {target}")


def refresh_windows_resource(root_dir: Path) -> bool:
    """刷新 syso，供纯 go build 链路使用。"""
    goversioninfo = shutil.which("goversioninfo") or shutil.which("goversioninfo.exe")
    if not goversioninfo:
        print("未找到 goversioninfo，跳过 syso 刷新。")
        return False

    resourceinfo_path = root_dir / "resourceinfo.json"
    syso_path = root_dir / "resource_windows_amd64.syso"
    subprocess.run(
        [goversioninfo, "-64", "-o", str(syso_path), str(resourceinfo_path)],
        cwd=root_dir,
        check=True,
    )
    print(f"SYSO: {syso_path}")
    return True


def parse_args() -> argparse.Namespace:
    """解析命令行参数。"""
    parser = argparse.ArgumentParser(description="生成 Cal1604 图标资源。")
    parser.add_argument(
        "--refresh-syso",
        action="store_true",
        help="额外刷新根目录 resource_windows_amd64.syso，供纯 go build 链路使用。",
    )
    return parser.parse_args()


def main() -> None:
    """生成图标资源。"""
    args = parse_args()
    root_dir = Path(__file__).resolve().parents[1]
    build_dir = root_dir / "build"
    windows_dir = build_dir / "windows"
    build_dir.mkdir(parents=True, exist_ok=True)
    windows_dir.mkdir(parents=True, exist_ok=True)

    print("生成 Cal1604 新版桌面图标...")
    icon = draw_icon()
    save_svg_asset(build_dir)
    save_png_assets(build_dir, icon)
    save_ico_asset(windows_dir, icon)

    if args.refresh_syso:
        refresh_windows_resource(root_dir)
    else:
        print("已跳过 syso 刷新；Wails 构建默认会直接读取 build/windows/icon.ico。")

    print("图标资源已完成刷新。")


if __name__ == "__main__":
    main()
