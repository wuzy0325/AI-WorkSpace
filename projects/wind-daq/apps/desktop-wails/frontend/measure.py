from playwright.sync_api import sync_playwright
import os

path = os.path.abspath('projects/wind-daq/apps/desktop-wails/frontend/test_layout.html')
url = f'file:///{path.replace(os.sep, "/")}'

with sync_playwright() as p:
    browser = p.chromium.launch(headless=True)
    page = browser.new_page(viewport={'width': 736, 'height': 800})
    page.goto(url)
    page.wait_for_load_state('networkidle')

    # 测量条件行
    rows = page.locator('.condition-row').all()
    print(f"条件行数量: {len(rows)}")
    for i, row in enumerate(rows):
        main = row.locator('.condition-row__main')
        inp = row.locator('.condition-row__input')
        num = row.locator('.n-input-number')
        unit = row.locator('.input-unit')
        rb = row.bounding_box()
        mb = main.bounding_box()
        ib = inp.bounding_box()
        nb = num.bounding_box()
        ub = unit.bounding_box()
        print(f"\n--- 条件行 {i} ---")
        print(f"  row: x={rb['x']:.0f} w={rb['width']:.0f}")
        print(f"  main: x={mb['x']:.0f} w={mb['width']:.0f} (右边缘={mb['x']+mb['width']:.0f})")
        print(f"  input区: x={ib['x']:.0f} w={ib['width']:.0f}")
        print(f"  输入框: x={nb['x']:.0f} w={nb['width']:.0f}")
        print(f"  单位: x={ub['x']:.0f} w={ub['width']:.0f}")

    # 测量 slider 行
    print("\n=== 波形图 slider 行 ===")
    slider = page.locator('.refresh-slider .n-slider')
    sbox = slider.bounding_box()
    print(f"  slider rail: x={sbox['x']:.0f} y={sbox['y']:.0f} w={sbox['width']:.0f} h={sbox['height']:.0f}")
    slider_area = page.locator('.refresh-slider')
    sabox = slider_area.bounding_box()
    print(f"  slider区: x={sabox['x']:.0f} y={sabox['y']:.0f} w={sabox['width']:.0f} h={sabox['height']:.0f}")

    browser.close()
