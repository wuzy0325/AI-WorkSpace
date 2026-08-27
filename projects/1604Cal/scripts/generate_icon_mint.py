#!/usr/bin/env python3
"""
Cal1604 Mint 主题图标生成器
生成匹配 DESIGN.md 的 Mint 色系应用图标
"""
import os, math
from PIL import Image, ImageDraw

MINT = (16, 185, 129)
MINT_DARK = (4, 120, 87)
WHITE = (255, 255, 255)
SIZE = 1024

def draw_rounded_rect(draw, xy, radius, fill):
    x1, y1, x2, y2 = xy
    draw.rounded_rectangle(xy, radius=radius, fill=fill)

def lerp_color(c1, c2, t):
    return tuple(int(a + (b - a) * t) for a, b in zip(c1, c2))

def radial_gradient(size, cx, cy, c1, c2):
    img = Image.new('RGBA', (size, size), (0,0,0,0))
    draw = ImageDraw.Draw(img)
    max_r = int(math.sqrt(2) * size / 2)
    for r in range(max_r, 0, -4):
        t = r / max_r
        col = lerp_color(c1, c2, t)
        draw.ellipse([cx - r, cy - r, cx + r, cy + r], fill=(*col, 255))
    return img

def draw_icon(size=1024):
    img = Image.new('RGBA', (size, size), (0,0,0,0))
    draw = ImageDraw.Draw(img)

    # --- 圆角背景渐变 ---
    corner = size // 5
    pad = size // 48

    bg = radial_gradient(size, size//2, size//2, MINT, MINT_DARK)
    # 裁剪为圆角矩形
    bg_cropped = Image.new('RGBA', (size, size), (0,0,0,0))
    bg_cropped_draw = ImageDraw.Draw(bg_cropped)
    bg_cropped_draw.rounded_rectangle([pad, pad, size-pad, size-pad], radius=corner, fill=(0,0,0,255))
    bg.putalpha(bg_cropped.getchannel('A'))
    img.paste(bg, (0,0), bg)

    # --- 高光折射 (左上角柔和光晕) ---
    highlight = Image.new('RGBA', (size, size), (0,0,0,0))
    h_draw = ImageDraw.Draw(highlight)
    for r in range(int(size*0.3), 0, -4):
        alpha = int(18 * (1 - r / (size*0.3)))
        h_draw.ellipse([size*0.12 - r, size*0.12 - r, size*0.12 + r, size*0.12 + r],
                       fill=(255,255,255,alpha))
    # 旋转高光
    highlight = highlight.rotate(-25, center=(size//2, size//2), resample=Image.Resampling.BICUBIC)
    mask = Image.new('L', (size, size), 0)
    ImageDraw.Draw(mask).rounded_rectangle([pad, pad, size-pad, size-pad], radius=corner, fill=255)
    img.paste(highlight, (0,0), mask)

    # --- 压力表符号 ---
    cx, cy = size//2, int(size*0.58)
    gauge_r = int(size*0.22)

    # 弧形 (半圆, 约 220 度, 从 -160 到 160 度)
    arc_bbox = [cx - gauge_r, cy - gauge_r, cx + gauge_r, cy + gauge_r]
    draw.arc(arc_bbox, -160, 160, fill=(*WHITE, 90), width=max(6, size//60))

    # 主刻度 (顶部居中 = 12 点方向)
    for angle in range(-50, 51, 25):
        rad = math.radians(angle - 90)
        outer = gauge_r - 4
        inner = gauge_r - gauge_r * 0.38
        x1 = cx + math.cos(rad) * inner
        y1 = cy + math.sin(rad) * inner
        x2 = cx + math.cos(rad) * outer
        y2 = cy + math.sin(rad) * outer
        w = max(4, size//120) if angle == 0 else max(3, size//160)
        draw.line([(x1, y1), (x2, y2)], fill=(*WHITE, 200 if angle == 0 else 120), width=w)

    # 内弧 (细装饰)
    inner_r = gauge_r - gauge_r * 0.28
    draw.arc([cx - inner_r, cy - inner_r, cx + inner_r, cy + inner_r],
             -140, 140, fill=(*WHITE, 30), width=2)

    # 指针 (从中心指向右上, 约 -30 度)
    needle_len = int(gauge_r * 0.9)
    needle_angle = math.radians(-35)
    tip_x = cx + math.cos(needle_angle) * needle_len
    tip_y = cy + math.sin(needle_angle) * needle_len

    # 指针尾部 (从中心向后延伸, 较短)
    tail_angle = math.radians(145)
    tail_len = int(gauge_r * 0.3)
    tail_x = cx + math.cos(tail_angle) * tail_len
    tail_y = cy + math.sin(tail_angle) * tail_len

    # 主指针线
    draw.line([(tail_x, tail_y), (tip_x, tip_y)], fill=WHITE, width=max(8, size//60))
    # 指针尖端收窄
    draw.line([(cx, cy), (tip_x, tip_y)], fill=WHITE, width=max(6, size//80))

    # 中心圆点
    dot_r = max(10, size//50)
    draw.ellipse([cx - dot_r, cy - dot_r, cx + dot_r, cy + dot_r], fill=WHITE)
    inner_dot_r = max(5, size//100)
    draw.ellipse([cx - inner_dot_r, cy - inner_dot_r, cx + inner_dot_r, cy + inner_dot_r], fill=MINT)

    return img

def make_dib_from_pil(img):
    """Convert PIL RGBA image to Windows ICO DIB (bottom-up BGRA + AND mask)"""
    import struct
    w, h = img.size
    raw = img.tobytes()
    and_stride = ((w + 31) // 32) * 4

    xor_rows = []
    and_rows = []

    for y in range(h):
        row_start = y * w * 4
        xor_row = bytearray(w * 4)
        and_row = bytearray(and_stride)
        for x in range(w):
            i = row_start + x * 4
            r, g, b, a = raw[i], raw[i+1], raw[i+2], raw[i+3]
            xor_row[x*4]   = b
            xor_row[x*4+1] = g
            xor_row[x*4+2] = r
            xor_row[x*4+3] = a
            if a < 128:
                and_row[x // 8] |= (0x80 >> (x % 8))
        xor_rows.append(xor_row)
        and_rows.append(and_row)

    # ICO BMP: bottom-up (reverse rows)
    xor_data = b''.join(reversed(xor_rows))
    and_data = b''.join(reversed(and_rows))

    hdr = struct.pack('<IiiHHIIiiII',
        40, w, h * 2, 1, 32, 0, len(xor_data) + len(and_data), 0, 0, 0, 0)

    return hdr + xor_data + and_data

def create_ico(ico_path):
    import struct
    sizes = [16, 24, 32, 48, 64, 128, 256]
    base_img = draw_icon(1024).convert('RGBA')
    entries = []
    for s in sizes:
        img = base_img.resize((s, s), Image.Resampling.LANCZOS)
        dib_data = make_dib_from_pil(img)
        entries.append((s, dib_data))

    count = len(entries)
    header = struct.pack('<HHH', 0, 1, count)
    dir_size = 16 * count
    offset = 6 + dir_size
    with open(ico_path, 'wb') as f:
        f.write(header)
        for s, data in entries:
            w = 0 if s >= 256 else s
            h = 0 if s >= 256 else s
            dir_entry = struct.pack('<BBBBHHII', w, h, 0, 0, 1, 32, len(data), offset)
            f.write(dir_entry)
            offset += len(data)
        for _, data in entries:
            f.write(data)
    print(f"ICO: {ico_path} ({count} sizes, BMP format)")

def save_pngs(build_dir, img):
    sizes = {1024: 'appicon.png', 512: 'appicon_512.png', 256: 'appicon_256.png'}
    for s, name in sizes.items():
        p = os.path.join(build_dir, name)
        if s != 1024:
            img.resize((s, s), Image.Resampling.LANCZOS).save(p, 'PNG')
        else:
            img.save(p, 'PNG')
        print(f"PNG: {p}")

def main():
    root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    build_dir = os.path.join(root, 'build')
    os.makedirs(build_dir, exist_ok=True)
    os.makedirs(os.path.join(build_dir, 'windows'), exist_ok=True)

    print("生成 Mint 主题图标...")
    img = draw_icon(1024)
    save_pngs(build_dir, img)
    create_ico(os.path.join(build_dir, 'windows', 'icon.ico'))
    print("完成!")

if __name__ == '__main__':
    main()
