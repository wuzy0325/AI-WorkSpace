# -*- coding: utf-8 -*-
"""
生成DAQ-T-1603应用程序的高清ICO图标
根据用户提供的参考图片，使用矢量绘制方式生成多分辨率ICO文件
"""

from PIL import Image, ImageDraw
import math
from pathlib import Path

# 定义颜色常量
COLOR_BACKGROUND = (0, 0, 0, 255)       # 黑色背景
COLOR_ORANGE_BORDER = (255, 127, 39, 255)  # 橙色边框 (#FF7F27)
COLOR_ORANGE_M = (255, 165, 80, 255)    # 橙色M形图案（稍浅）
COLOR_WHITE_DOT = (255, 255, 255, 255)  # 白色圆点


def create_icon(size: int) -> Image.Image:
    """
    创建指定尺寸的单张图标
    
    参数:
        size: 图标尺寸（宽高相同）
    
    返回:
        PIL Image对象（RGBA模式）
    """
    # 创建透明背景的图像
    img = Image.new('RGBA', (size, size), (0, 0, 0, 0))
    draw = ImageDraw.Draw(img)
    
    # 计算比例因子
    scale = size / 256.0
    
    # 外边距
    margin = int(8 * scale)
    
    # 绘制黑色背景圆角矩形
    bg_radius = int(48 * scale)
    draw.rounded_rectangle(
        [margin, margin, size - margin, size - margin],
        radius=bg_radius,
        fill=COLOR_BACKGROUND
    )
    
    # 绘制橙色边框（外框）
    border_width = int(20 * scale)
    border_radius = int(48 * scale)
    draw.rounded_rectangle(
        [margin, margin, size - margin, size - margin],
        radius=border_radius,
        outline=COLOR_ORANGE_BORDER,
        width=border_width
    )
    
    # 计算内部区域（用于绘制M形和圆点）
    inner_margin = margin + border_width + int(10 * scale)
    inner_left = inner_margin
    inner_top = inner_margin
    inner_right = size - inner_margin
    inner_bottom = size - inner_margin
    inner_width = inner_right - inner_left
    inner_height = inner_bottom - inner_top
    
    # M形图案的参数
    center_x = (inner_left + inner_right) // 2
    center_y = (inner_top + inner_bottom) // 2
    
    # M形的三个顶点坐标（相对于中心）
    m_width = inner_width * 0.7
    m_height = inner_height * 0.5
    m_top_y = inner_top + inner_height * 0.25
    m_bottom_y = inner_top + inner_height * 0.75
    
    # M形的三个顶点
    left_x = center_x - m_width // 2
    right_x = center_x + m_width // 2
    
    # M形线条宽度
    m_line_width = int(28 * scale)
    
    # 绘制M形的三条线段
    # 左上到中间下
    draw.line(
        [(left_x, m_top_y), (center_x, m_bottom_y)],
        fill=COLOR_ORANGE_M,
        width=m_line_width
    )
    # 中间下到右上
    draw.line(
        [(center_x, m_bottom_y), (right_x, m_top_y)],
        fill=COLOR_ORANGE_M,
        width=m_line_width
    )
    
    # 绘制M形端点的圆形（让连接处更圆滑）
    cap_radius = m_line_width // 2
    
    # 左端圆
    draw.ellipse(
        [left_x - cap_radius, m_top_y - cap_radius,
         left_x + cap_radius, m_top_y + cap_radius],
        fill=COLOR_ORANGE_M
    )
    # 右端圆
    draw.ellipse(
        [right_x - cap_radius, m_top_y - cap_radius,
         right_x + cap_radius, m_top_y + cap_radius],
        fill=COLOR_ORANGE_M
    )
    # 底端圆
    draw.ellipse(
        [center_x - cap_radius, m_bottom_y - cap_radius,
         center_x + cap_radius, m_bottom_y + cap_radius],
        fill=COLOR_ORANGE_M
    )
    
    # 绘制三个白色圆点
    dot_radius = int(18 * scale)
    
    # 左上圆点
    draw.ellipse(
        [left_x - dot_radius, m_top_y - dot_radius,
         left_x + dot_radius, m_top_y + dot_radius],
        fill=COLOR_WHITE_DOT
    )
    
    # 右上圆点
    draw.ellipse(
        [right_x - dot_radius, m_top_y - dot_radius,
         right_x + dot_radius, m_top_y + dot_radius],
        fill=COLOR_WHITE_DOT
    )
    
    # 下方圆点
    draw.ellipse(
        [center_x - dot_radius, m_bottom_y - dot_radius,
         center_x + dot_radius, m_bottom_y + dot_radius],
        fill=COLOR_WHITE_DOT
    )
    
    return img


def generate_ico(output_path: str) -> None:
    """
    生成包含多种分辨率的标准ICO文件
    
    参数:
        output_path: 输出ICO文件的完整路径
    """
    # Windows ICO标准分辨率（从大到小排列）
    sizes = [256, 128, 64, 48, 32, 16]
    
    images = []
    for size in sizes:
        img = create_icon(size)
        images.append(img)
        print(f"已生成 {size}x{size} 分辨率图标")
    
    # 保存为ICO文件（第一张图作为主图，其余作为备选分辨率）
    images[0].save(
        output_path,
        format='ICO',
        sizes=[(img.width, img.height) for img in images],
        append_images=images[1:]
    )
    
    print(f"\nICO文件已保存至: {output_path}")
    print(f"包含分辨率: {', '.join([f'{s}x{s}' for s in sizes])}")


if __name__ == "__main__":
    output_file = Path(__file__).resolve().parent / "projects" / "wista" / "apps" / "desktop-wails" / "build" / "appicon.ico"
    generate_ico(str(output_file))
