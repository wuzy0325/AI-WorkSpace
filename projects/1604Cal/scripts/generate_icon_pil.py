#!/usr/bin/env python3
"""
Cal1604 应用程序图标生成器 (PIL版本)
使用 Pillow 直接绘制图标，不依赖 Cairo

小尺寸清晰度设计原则（2026-08 模糊问题修复）：
- 所有元素尺寸按画布比例计算，杜绝固定像素值
- ICO 每档尺寸独立绘制（4x 超采样后 LANCZOS 缩小），而非从 1024 直接缩放
- ≤48px 使用简化设计（粗表圈 + 粗指针），≥64px 才绘制刻度、文字与对勾
"""

import os
import math
from PIL import Image, ImageDraw, ImageFont

# ---------- 配色方案 ----------
BG_PRIMARY = (0x1e, 0x1e, 0x1e)      # 背景深色（边缘）
BG_TERTIARY = (0x2d, 0x2d, 0x2d)     # 背景亮色（中心）
GOLD = (0xff, 0xd7, 0x00)            # 明金（主图形）
GOLD_DARK = (0xd4, 0xa8, 0x00)       # 深金（阴影/次要）


def draw_background(size: int) -> Image.Image:
    """绘制深色径向渐变圆角背景（由外向内渐亮，提升主体对比）。"""
    img = Image.new('RGBA', (size, size), (0, 0, 0, 0))
    draw = ImageDraw.Draw(img)
    corner_radius = size // 5
    padding = size // 40
    steps = max(32, size // 4)
    for i in range(steps, 0, -1):
        ratio = i / steps  # 1=外圈 → 0=中心
        r = int(BG_TERTIARY[0] + (BG_PRIMARY[0] - BG_TERTIARY[0]) * ratio)
        g = int(BG_TERTIARY[1] + (BG_PRIMARY[1] - BG_TERTIARY[1]) * ratio)
        b = int(BG_TERTIARY[2] + (BG_PRIMARY[2] - BG_TERTIARY[2]) * ratio)
        offset = int(corner_radius * ratio * 0.6)
        draw.rounded_rectangle(
            [padding + offset, padding + offset,
             size - padding - offset, size - padding - offset],
            radius=corner_radius - offset,
            fill=(r, g, b, 255),
        )
    return img


def _load_bold_font(pixel: int):
    """加载粗体系统字体，失败时回退默认字体。"""
    for name in ("arialbd.ttf", "segoeuib.ttf", "arial.ttf"):
        try:
            return ImageFont.truetype(name, pixel)
        except OSError:
            continue
    return ImageFont.load_default()


def draw_gauge(size: int, simplified: bool) -> Image.Image:
    """在透明画布上绘制金色压力表主体。

    simplified=True 时仅保留粗表圈 + 粗指针 + 轴心，
    供 ≤48px 的小尺寸 ICO 使用，保证缩到桌面图标大小时依然锐利。
    """
    img = Image.new('RGBA', (size, size), (0, 0, 0, 0))
    draw = ImageDraw.Draw(img)

    cx = size / 2
    cy = size * 0.46
    radius = size * 0.30

    # 表圈：宽度按比例取粗（约半径的 1/8），小尺寸也能撑起 2~3px 实线
    ring_w = max(2, round(radius / 8))
    bbox = [cx - radius, cy - radius, cx + radius, cy + radius]
    # 先铺一层深金弱阴影，再叠明金主体，增强立体感
    shadow_grow = ring_w * 0.5
    draw.ellipse([bbox[0] - shadow_grow, bbox[1] - shadow_grow,
                  bbox[2] + shadow_grow, bbox[3] + shadow_grow],
                 outline=(*GOLD_DARK, 140), width=ring_w)
    draw.ellipse(bbox, outline=(*GOLD, 255), width=ring_w)

    if not simplified:
        # 主刻度（12/3/6/9 点）：长度与宽度均按比例
        tick_len = radius * 0.22
        tick_w = max(2, round(radius / 14))
        tick_start = radius - ring_w * 1.6
        for angle_deg in (0, 90, 180, 270):
            rad = math.radians(angle_deg - 90)
            dx, dy = math.cos(rad), math.sin(rad)
            x1 = cx + dx * tick_start
            y1 = cy + dy * tick_start
            x2 = cx + dx * (tick_start - tick_len)
            y2 = cy + dy * (tick_start - tick_len)
            draw.line([(x1, y1), (x2, y2)], fill=(*GOLD, 230), width=tick_w)

    # 指针：指向 12 点方向的粗楔形（底部宽约半径 1/5）
    needle_base_w = radius * 0.20
    needle_len = radius * (0.62 if simplified else 0.58)
    needle = [
        (cx, cy - needle_len),
        (cx - needle_base_w / 2, cy + radius * 0.14),
        (cx + needle_base_w / 2, cy + radius * 0.14),
    ]
    draw.polygon(needle, fill=(*GOLD, 255))

    # 轴心：金色圆 + 深色内点
    hub_r = radius * (0.16 if simplified else 0.12)
    draw.ellipse([cx - hub_r, cy - hub_r, cx + hub_r, cy + hub_r], fill=(*GOLD, 255))
    inner_r = hub_r * 0.45
    draw.ellipse([cx - inner_r, cy - inner_r, cx + inner_r, cy + inner_r],
                 fill=(*BG_PRIMARY, 255))

    return img


def draw_icon(size: int = 1024, simplified: bool = False) -> Image.Image:
    """绘制完整图标：背景 + 压力表 + （大尺寸）刻度附加元素。"""
    img = draw_background(size)
    gauge = draw_gauge(size, simplified)

    if not simplified and size >= 256:
        # 大尺寸附加元素：右上角校准对勾 + 底部 1604 型号文字
        overlay = Image.new('RGBA', (size, size), (0, 0, 0, 0))
        odraw = ImageDraw.Draw(overlay)

        check_cx, check_cy = size * 0.76, size * 0.20
        check_r = size * 0.075
        check_w = max(3, round(check_r / 2.6))
        odraw.line(
            [(check_cx - check_r * 0.5, check_cy + check_r * 0.05),
             (check_cx - check_r * 0.1, check_cy + check_r * 0.45)],
            fill=(*GOLD, 255), width=check_w)
        odraw.line(
            [(check_cx - check_r * 0.1, check_cy + check_r * 0.45),
             (check_cx + check_r * 0.55, check_cy - check_r * 0.30)],
            fill=(*GOLD, 255), width=check_w)

        font = _load_bold_font(round(size * 0.115))
        text = "1604"
        bbox = odraw.textbbox((0, 0), text, font=font)
        text_w = bbox[2] - bbox[0]
        odraw.text((size / 2 - text_w / 2 - bbox[0], size * 0.80), text,
                   font=font, fill=(*GOLD, 255))

        img = Image.alpha_composite(img, overlay)

    img = Image.alpha_composite(img, gauge)
    return img


def build_ico_entry(target: int) -> Image.Image:
    """单档 ICO 尺寸：4x 超采样绘制后 LANCZOS 缩小，边缘平滑且细节锐利。"""
    simplified = target <= 48
    canvas = draw_icon(target * 4, simplified=simplified)
    return canvas.resize((target, target), Image.Resampling.LANCZOS)


def create_ico(ico_path: str) -> None:
    """创建 Windows ICO：≤48px 简化设计，≥64px 完整设计。

    注意：PIL 保存 ICO 时会跳过大于基础图像的尺寸，
    因此必须用最大图（256px）作为基础图，其余尺寸经 append_images 提供。
    """
    sizes = [16, 24, 32, 48, 64, 128, 256]
    entries = {s: build_ico_entry(s) for s in sizes}
    base = entries[256]
    base.save(
        ico_path,
        format='ICO',
        sizes=[(s, s) for s in sizes],
        append_images=[entries[s] for s in sizes if s != 256],
    )
    print(f"ICO 图标已保存: {ico_path}（7 档尺寸独立绘制，≤48px 简化设计）")


def main():
    """主函数"""
    project_root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    build_dir = os.path.join(project_root, 'build')

    os.makedirs(build_dir, exist_ok=True)
    os.makedirs(os.path.join(build_dir, 'windows'), exist_ok=True)

    print("=" * 50)
    print("Cal1604 图标生成器 (PIL版本，小尺寸优化)")
    print("=" * 50)

    # 大尺寸 PNG 资源：1024 原生绘制，512/256 由 1024 高质量缩放
    img_1024 = draw_icon(1024, simplified=False)
    for size, img in (
        (1024, img_1024),
        (512, img_1024.resize((512, 512), Image.Resampling.LANCZOS)),
        (256, img_1024.resize((256, 256), Image.Resampling.LANCZOS)),
    ):
        png_path = os.path.join(build_dir, f'appicon_{size}.png')
        img.save(png_path, 'PNG')
        print(f"PNG ({size}x{size}) 已保存: {png_path}")
        if size == 1024:
            appicon_path = os.path.join(build_dir, 'appicon.png')
            img.save(appicon_path, 'PNG')
            print(f"PNG 图标已保存: {appicon_path}")

    print("\n创建 Windows ICO 图标...")
    ico_path = os.path.join(build_dir, 'windows', 'icon.ico')
    create_ico(ico_path)

    print("\n" + "=" * 50)
    print("图标生成完成!")
    print("=" * 50)
    print("\n小尺寸清晰度策略:")
    print("  - 全部元素按画布比例绘制，无固定像素值")
    print("  - 每档 ICO 尺寸 4x 超采样独立绘制")
    print("  - ≤48px：粗表圈 + 粗指针简化设计")
    print("  - ≥64px：增加刻度、1604 文字与校准对勾")


if __name__ == '__main__':
    main()
