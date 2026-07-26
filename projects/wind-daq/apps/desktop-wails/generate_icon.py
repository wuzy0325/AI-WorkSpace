"""生成 wind-daq 专属应用图标 appicon.ico

设计：
  - 圆角方形背景，蓝绿渐变（风洞主题）
  - 中央 "WD" 字样（Wind DAQ 缩写），白色
  - 多尺寸 ICO（16/32/48/64/128/256），适配 Windows 任务栏/标题栏/资源管理器

执行：python generate_icon.py
输出：appicon.ico（与 daq-t1603/appicon.ico 同目录结构）
"""
from PIL import Image, ImageDraw, ImageFont
import os

def make_icon(size: int) -> Image.Image:
    """生成指定尺寸的图标"""
    img = Image.new('RGBA', (size, size), (0, 0, 0, 0))
    draw = ImageDraw.Draw(img)

    # 圆角矩形背景：蓝绿色渐变（风洞主题）
    # 主色 #0ea5e9（sky-500）到 #10b981（emerald-500）渐变
    for y in range(size):
        ratio = y / size
        r = int(14 + (16 - 14) * ratio)      # 14 -> 16
        g = int(165 + (185 - 165) * ratio)    # 165 -> 185
        b = int(233 + (129 - 233) * ratio)    # 233 -> 129
        for x in range(size):
            # 圆角判断：四角透明
            radius = size // 6
            if (x < radius and y < radius and (radius - x) ** 2 + (radius - y) ** 2 > radius ** 2) or \
               (x > size - radius and y < radius and (x - (size - radius - 1)) ** 2 + (radius - y) ** 2 > radius ** 2) or \
               (x < radius and y > size - radius and (radius - x) ** 2 + (y - (size - radius - 1)) ** 2 > radius ** 2) or \
               (x > size - radius and y > size - radius and (x - (size - radius - 1)) ** 2 + (y - (size - radius - 1)) ** 2 > radius ** 2):
                continue
            img.putpixel((x, y), (r, g, b, 255))

    # 中央 "WD" 字样
    font_size = int(size * 0.5)
    font = None
    # Windows 系统字体
    for font_path in [
        'C:\\Windows\\Fonts\\arialbd.ttf',   # Arial Bold
        'C:\\Windows\\Fonts\\arial.ttf',      # Arial
        'C:\\Windows\\Fonts\\segoeui.ttf',    # Segoe UI
    ]:
        if os.path.exists(font_path):
            font = ImageFont.truetype(font_path, font_size)
            break
    if font is None:
        font = ImageFont.load_default()

    text = 'WD'
    # 测量文字尺寸（Pillow < 10 用 textsize，>= 10 用 textbbox）
    try:
        bbox = draw.textbbox((0, 0), text, font=font)
        text_w = bbox[2] - bbox[0]
        text_h = bbox[3] - bbox[1]
        text_x = (size - text_w) // 2 - bbox[0]
        text_y = (size - text_h) // 2 - bbox[1]
    except AttributeError:
        text_w, text_h = draw.textsize(text, font=font)
        text_x = (size - text_w) // 2
        text_y = (size - text_h) // 2

    # 文字阴影（轻微深色，增加对比度）
    draw.text((text_x + 1, text_y + 1), text, font=font, fill=(0, 0, 0, 80))
    # 主文字（白色）
    draw.text((text_x, text_y), text, font=font, fill=(255, 255, 255, 255))

    return img


def main():
    # 生成 256x256 主图，PIL 保存 ICO 时自动缩放到各尺寸
    # （ICO 格式不支持 append_images，需用 sizes 参数让 PIL 从主图缩放）
    main_img = make_icon(256)

    out_path = os.path.join(os.path.dirname(__file__), 'appicon.ico')
    sizes = [(16, 16), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256)]
    main_img.save(out_path, format='ICO', sizes=sizes)

    size_bytes = os.path.getsize(out_path)
    print(f'Generated: {out_path}')
    print(f'Size: {size_bytes} bytes ({len(sizes)} sizes: {[s[0] for s in sizes]})')


if __name__ == '__main__':
    main()
