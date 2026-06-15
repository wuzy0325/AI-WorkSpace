from playwright.sync_api import sync_playwright
import os

output_dir = r"c:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace\projects\wind-daq\screenshots"
os.makedirs(output_dir, exist_ok=True)

# 导航项的 aria-label 列表（根据 i18nStore.ts 中的中文文本）
nav_items = [
    ("dashboard", "仪表盘"),
    ("motion", "运动控制"),
    ("calibration", "探针校准"),
    ("traversal", "遍历测试"),
    ("log", "运行日志"),
]

with sync_playwright() as p:
    browser = p.chromium.launch(headless=True)
    page = browser.new_page(viewport={"width": 1440, "height": 900})

    # 先打开首页
    page.goto("http://127.0.0.1:15173/")
    page.wait_for_load_state("networkidle")
    page.wait_for_timeout(2000)

    for page_id, label in nav_items:
        if page_id != "dashboard":
            # 通过 aria-label 点击导航按钮
            page.locator(f"[aria-label='{label}']").click()
            # 遍历测试页面需要更长时间恢复状态
            if page_id == "traversal":
                page.wait_for_timeout(4000)
            else:
                page.wait_for_timeout(2000)

        screenshot_path = os.path.join(output_dir, f"{page_id}.png")
        page.screenshot(path=screenshot_path, full_page=True)
        print(f"截图完成: {label} -> {screenshot_path}")

    browser.close()
