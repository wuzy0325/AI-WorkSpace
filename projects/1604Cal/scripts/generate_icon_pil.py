#!/usr/bin/env python3
"""
Cal1604 应用程序图标生成器 (PIL版本)
使用 Pillow 直接绘制图标，不依赖 Cairo
"""

import os
import math
from PIL import Image, ImageDraw, ImageFont

def draw_gradient_background(draw, size, color1, color2):
    """绘制径向渐变背景"""
    cx, cy = size // 2, size // 2
    max_radius = int(math.sqrt(2) * size / 2)
    
    for r in range(max_radius, 0, -2):
        ratio = r / max_radius
        # 简化的渐变 - 使用同心圆
        r1, g1, b1 = color1
        r2, g2, b2 = color2
        col = (
            int(r1 + (r2 - r1) * (1 - ratio)),
            int(g1 + (g2 - g1) * (1 - ratio)),
            int(b2 + (b2 - g2) * (1 - ratio))
        )
        draw.ellipse([cx - r, cy - r, cx + r, cy + r], fill=col)

def interpolate_color(color1, color2, ratio):
    """颜色插值"""
    return tuple(int(c1 + (c2 - c1) * ratio) for c1, c2 in zip(color1, color2))

def hex_to_rgb(hex_color):
    """十六进制颜色转RGB"""
    hex_color = hex_color.lstrip('#')
    return tuple(int(hex_color[i:i+2], 16) for i in (0, 2, 4))

def draw_rounded_rect(draw, xy, radius, fill, outline=None, width=1):
    """绘制圆角矩形"""
    x1, y1, x2, y2 = xy
    # 主体矩形
    draw.rectangle([x1 + radius, y1, x2 - radius, y2], fill=fill)
    draw.rectangle([x1, y1 + radius, x2, y2 - radius], fill=fill)
    # 四个圆角
    draw.pieslice([x1, y1, x1 + radius * 2, y1 + radius * 2], 180, 270, fill=fill)
    draw.pieslice([x2 - radius * 2, y1, x2, y1 + radius * 2], 270, 360, fill=fill)
    draw.pieslice([x1, y2 - radius * 2, x1 + radius * 2, y2], 90, 180, fill=fill)
    draw.pieslice([x2 - radius * 2, y2 - radius * 2, x2, y2], 0, 90, fill=fill)
    
    if outline:
        # 绘制边框
        draw.arc([x1, y1, x1 + radius * 2, y1 + radius * 2], 180, 270, fill=outline, width=width)
        draw.arc([x2 - radius * 2, y1, x2, y1 + radius * 2], 270, 360, fill=outline, width=width)
        draw.arc([x1, y2 - radius * 2, x1 + radius * 2, y2], 90, 180, fill=outline, width=width)
        draw.arc([x2 - radius * 2, y2 - radius * 2, x2, y2], 0, 90, fill=outline, width=width)
        draw.line([x1 + radius, y1, x2 - radius, y1], fill=outline, width=width)
        draw.line([x1 + radius, y2, x2 - radius, y2], fill=outline, width=width)
        draw.line([x1, y1 + radius, x1, y2 - radius], fill=outline, width=width)
        draw.line([x2, y1 + radius, x2, y2 - radius], fill=outline, width=width)

def draw_icon(size=1024):
    """绘制图标"""
    # 配色方案
    bg_primary = hex_to_rgb("#1e1e1e")      # 主背景
    bg_secondary = hex_to_rgb("#252526")    # 次级背景
    bg_tertiary = hex_to_rgb("#2d2d2d")     # 三级背景
    accent_gold = hex_to_rgb("#ffd700")     # 明金
    accent_gold_light = hex_to_rgb("#ffed4a")  # 浅金
    accent_gold_dark = hex_to_rgb("#d4a800")   # 深金
    text_color = hex_to_rgb("#d4d4d4")      # 主要文字
    border_color = hex_to_rgb("#333333")    # 边框
    border_strong = hex_to_rgb("#444444")   # 强边框
    
    # 创建图像 (带透明通道)
    img = Image.new('RGBA', (size, size), (0, 0, 0, 0))
    draw = ImageDraw.Draw(img)
    
    # 圆角半径
    corner_radius = size // 6
    padding = size // 32
    
    # 绘制背景圆角矩形 (径向渐变效果)
    for i in range(size // 2, 0, -1):
        ratio = i / (size // 2)
        # 从中心向外渐变
        r = int(bg_tertiary[0] + (bg_primary[0] - bg_tertiary[0]) * ratio)
        g = int(bg_tertiary[1] + (bg_primary[1] - bg_tertiary[1]) * ratio)
        b = int(bg_tertiary[2] + (bg_primary[2] - bg_tertiary[2]) * ratio)
        
        # 绘制圆角渐变
        offset = corner_radius - int(corner_radius * ratio)
        draw.rounded_rectangle(
            [padding + offset, padding + offset, size - padding - offset, size - padding - offset],
            radius=corner_radius - offset,
            fill=(r, g, b, 255)
        )
    
    center = size // 2
    gauge_center_y = int(size * 0.45)  # 压力表中心偏上
    gauge_radius = int(size * 0.28)
    
    # 绘制装饰圆环
    draw.ellipse([center - gauge_radius - 60, gauge_center_y - gauge_radius - 60,
                  center + gauge_radius + 60, gauge_center_y + gauge_radius + 60],
                 outline=border_color, width=2)
    draw.ellipse([center - gauge_radius - 50, gauge_center_y - gauge_radius - 50,
                  center + gauge_radius + 50, gauge_center_y + gauge_radius + 50],
                 outline=border_strong, width=1)
    
    # 绘制压力表外圈 (金色)
    draw.ellipse([center - gauge_radius, gauge_center_y - gauge_radius,
                  center + gauge_radius, gauge_center_y + gauge_radius],
                 outline=accent_gold, width=max(4, size // 128))
    
    # 绘制压力表刻度盘背景
    inner_radius = gauge_radius - 20
    draw.ellipse([center - inner_radius, gauge_center_y - inner_radius,
                  center + inner_radius, gauge_center_y + inner_radius],
                 fill=bg_secondary, outline=border_color, width=2)
    
    # 绘制刻度线 - 主刻度 (12, 3, 6, 9 点方向)
    main_ticks = [(0, -1), (1, 0), (0, 1), (-1, 0)]  # 上右下左
    tick_length = 30
    tick_start = inner_radius - 5
    
    for dx, dy in main_ticks:
        x1 = center + dx * tick_start
        y1 = gauge_center_y + dy * tick_start
        x2 = center + dx * (tick_start - tick_length)
        y2 = gauge_center_y + dy * (tick_start - tick_length)
        draw.line([(x1, y1), (x2, y2)], fill=accent_gold, width=max(3, size // 171))
    
    # 绘制刻度线 - 次刻度 (45度方向)
    angle_45 = math.pi / 4
    for i in range(4):
        angle = angle_45 + i * math.pi / 2
        dx = math.cos(angle)
        dy = math.sin(angle)
        x1 = center + dx * tick_start
        y1 = gauge_center_y + dy * tick_start
        x2 = center + dx * (tick_start - tick_length * 0.7)
        y2 = gauge_center_y + dy * (tick_start - tick_length * 0.7)
        draw.line([(x1, y1), (x2, y2)], fill=(0x55, 0x55, 0x55), width=2)
    
    # 绘制中心点
    center_radius = max(16, size // 64)
    draw.ellipse([center - center_radius, gauge_center_y - center_radius,
                  center + center_radius, gauge_center_y + center_radius],
                 fill=accent_gold)
    draw.ellipse([center - center_radius // 2, gauge_center_y - center_radius // 2,
                  center + center_radius // 2, gauge_center_y + center_radius // 2],
                 fill=bg_primary)
    
    # 绘制指针 (指向12点方向)
    needle_length = inner_radius - 40
    needle_width = max(10, size // 102)
    
    # 指针主体
    points = [
        (center, gauge_center_y - needle_length),  # 尖端
        (center - needle_width, gauge_center_y + 10),  # 左下
        (center + needle_width, gauge_center_y + 10),  # 右下
    ]
    draw.polygon(points, fill=accent_gold)
    
    # 校准对勾标记 (右上角)
    check_x, check_y = center + gauge_radius + 40, gauge_center_y - gauge_radius + 40
    draw.line([(check_x, check_y + 20), (check_x + 15, check_y + 35)], fill=accent_gold, width=6)
    draw.line([(check_x + 15, check_y + 35), (check_x + 45, check_y + 5)], fill=accent_gold, width=6)
    
    # 绘制文字
    try:
        # 尝试使用系统字体
        font_large = ImageFont.truetype("arial.ttf", size // 8)
        font_small = ImageFont.truetype("arial.ttf", size // 20)
    except:
        # 使用默认字体
        font_large = ImageFont.load_default()
        font_small = ImageFont.load_default()
    
    # 1604 文字
    text_1604 = "1604"
    bbox = draw.textbbox((0, 0), text_1604, font=font_large)
    text_width = bbox[2] - bbox[0]
    text_y = int(size * 0.82)
    draw.text((center - text_width // 2, text_y), text_1604, font=font_large, fill=accent_gold)
    
    # CALIBRATION 文字
    text_cal = "CAL"
    bbox_cal = draw.textbbox((0, 0), text_cal, font=font_small)
    text_cal_width = bbox_cal[2] - bbox_cal[0]
    text_cal_y = int(size * 0.62)
    draw.text((center - text_cal_width // 2, text_cal_y), text_cal, font=font_small, fill=(0x88, 0x88, 0x88))
    
    # 装饰性角落圆点
    dot_radius = 8
    dot_offset = corner_radius
    for dx, dy in [(1, 1), (-1, 1), (1, -1), (-1, -1)]:
        dot_x = center + dx * (center - dot_offset)
        dot_y = center + dy * (center - dot_offset)
        draw.ellipse([dot_x - dot_radius, dot_y - dot_radius,
                      dot_x + dot_radius, dot_y + dot_radius],
                     fill=border_color)
    
    return img

def create_ico(images, ico_path):
    """创建 Windows ICO 文件"""
    sizes = [16, 24, 32, 48, 64, 128, 256]
    ico_images = []
    
    for size in sizes:
        # 找到最接近的图像并调整大小
        img = images[-1].copy()  # 使用最大图像
        img = img.resize((size, size), Image.Resampling.LANCZOS)
        ico_images.append(img)
    
    # 保存 ICO (第一张图作为主图，其余附加)
    ico_images[0].save(ico_path, format='ICO', sizes=[(s, s) for s in sizes], append_images=ico_images[1:])
    print(f"ICO 图标已保存: {ico_path}")

def main():
    """主函数"""
    # 项目根目录
    project_root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    build_dir = os.path.join(project_root, 'build')
    
    # 确保 build 目录存在
    os.makedirs(build_dir, exist_ok=True)
    os.makedirs(os.path.join(build_dir, 'windows'), exist_ok=True)
    
    print("=" * 50)
    print("Cal1604 图标生成器 (PIL版本)")
    print("=" * 50)
    
    # 生成不同尺寸的图标
    sizes = [256, 512, 1024]
    images = []
    
    for size in sizes:
        print(f"\n生成 {size}x{size} 图标...")
        img = draw_icon(size)
        images.append(img)
        
        # 保存 PNG
        if size == 1024:
            png_path = os.path.join(build_dir, 'appicon.png')
            img.save(png_path, 'PNG')
            print(f"PNG 图标已保存: {png_path}")
        
        png_path = os.path.join(build_dir, f'appicon_{size}.png')
        img.save(png_path, 'PNG')
        print(f"PNG ({size}x{size}) 已保存: {png_path}")
    
    # 创建 ICO
    print("\n创建 Windows ICO 图标...")
    ico_path = os.path.join(build_dir, 'windows', 'icon.ico')
    create_ico(images, ico_path)
    
    print("\n" + "=" * 50)
    print("图标生成完成!")
    print("=" * 50)
    print(f"\n生成的文件:")
    print(f"  - PNG: {os.path.join(build_dir, 'appicon.png')}")
    print(f"  - ICO: {ico_path}")
    
    print("\n图标设计特点:")
    print("  - 深色渐变背景 (#1e1e1e ~ #2d2d2d)")
    print("  - 金色压力表 (#ffd700) 体现精确校准")
    print("  - 指针指向12点方向，象征精准")
    print("  - 金色对勾标记表示校准完成")
    print("  - 1604 型号标识突出产品特征")
    print("  - 整体风格专业、现代、工业感")

if __name__ == '__main__':
    main()
