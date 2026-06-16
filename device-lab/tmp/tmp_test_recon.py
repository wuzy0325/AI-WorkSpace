from playwright.sync_api import sync_playwright
import time

with sync_playwright() as p:
    browser = p.chromium.launch(headless=True)
    page = browser.new_page(viewport={"width": 1600, "height": 900})
    
    # 收集控制台日志
    console_logs = []
    page.on("console", lambda msg: console_logs.append(f"[{msg.type}] {msg.text}"))
    
    # 访问前端 DevServer
    page.goto('http://localhost:15175')
    page.wait_for_load_state('networkidle')
    page.wait_for_timeout(3000)
    
    # 截图查看初始状态
    page.screenshot(path='/tmp/daq-p1604-step1-initial.png', full_page=True)
    print("Step 1: Initial state screenshot saved")
    
    # 查找"添加设备"按钮
    add_btn = page.locator('button:has-text("添加"), button[title="添加设备"]').first
    if add_btn.count() > 0:
        add_btn.click()
        page.wait_for_timeout(1000)
        page.screenshot(path='/tmp/daq-p1604-step2-add-dialog.png', full_page=True)
        print("Step 2: Add device dialog screenshot saved")
    else:
        # 尝试查找所有按钮
        buttons = page.locator('button').all()
        print(f"Found {len(buttons)} buttons")
        for i, btn in enumerate(buttons[:10]):
            text = btn.text_content() or ""
            title = btn.get_attribute("title") or ""
            print(f"  Button {i}: text='{text.strip()}', title='{title}'")
    
    # 打印控制台日志
    print("\n--- Console Logs ---")
    for log in console_logs[-20:]:
        print(log)
    
    browser.close()
