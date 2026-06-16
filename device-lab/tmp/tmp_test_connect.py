from playwright.sync_api import sync_playwright
import time

with sync_playwright() as p:
    browser = p.chromium.launch(headless=True)
    page = browser.new_page(viewport={"width": 1600, "height": 900})
    
    console_logs = []
    page.on("console", lambda msg: console_logs.append(f"[{msg.type}] {msg.text}"))
    page.on("pageerror", lambda err: console_logs.append(f"[PAGE_ERROR] {err}"))
    
    page.goto('http://localhost:15175')
    page.wait_for_load_state('networkidle')
    page.wait_for_timeout(3000)
    
    # 点击"添加设备"按钮
    page.locator('button[title="添加设备"]').click()
    page.wait_for_timeout(1000)
    
    # 填写设备信息
    inputs = page.locator('input:visible').all()
    print(f"Found {len(inputs)} visible inputs")
    
    # 输入名称
    name_input = inputs[0]
    name_input.clear()
    name_input.fill('P1604 压力采集器')
    
    # 输入地址
    addr_input = inputs[1]
    addr_input.clear()
    addr_input.fill('192.168.1.7')
    
    # 输入端口
    port_input = inputs[2]
    port_input.clear()
    port_input.fill('9000')
    
    page.wait_for_timeout(500)
    page.screenshot(path='/tmp/daq-p1604-step3-filled.png', full_page=True)
    print("Step 3: Filled device info")
    
    # 点击"添加"按钮
    add_confirm_btn = page.locator('button:has-text("添加")')
    add_confirm_btn.click()
    page.wait_for_timeout(2000)
    
    page.screenshot(path='/tmp/daq-p1604-step4-added.png', full_page=True)
    print("Step 4: Clicked add button")
    
    # 查看当前状态
    buttons = page.locator('button:visible').all()
    print(f"\nVisible buttons after adding:")
    for i, btn in enumerate(buttons):
        try:
            text = (btn.text_content() or "").strip()[:50]
            title = btn.get_attribute("title") or ""
            print(f"  [{i}] text='{text}', title='{title}'")
        except:
            pass
    
    # 查看是否有设备出现在侧边栏
    sidebar_items = page.locator('.sidebar__item, .device-item, [class*="device"], [class*="sidebar"]').all()
    print(f"\nSidebar/device elements: {len(sidebar_items)}")
    
    # 打印最近的错误
    print("\n--- Recent Error Logs ---")
    for log in console_logs[-10:]:
        if "ERROR" in log or "error" in log.lower() or "PAGE_ERROR" in log:
            print(log)
    
    # 打印所有日志（最近20条）
    print("\n--- Recent Console Logs ---")
    for log in console_logs[-20:]:
        print(log)
    
    browser.close()
