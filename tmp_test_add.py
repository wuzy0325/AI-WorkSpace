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
    add_btn = page.locator('button[title="添加设备"]')
    add_btn.click()
    page.wait_for_timeout(1500)
    page.screenshot(path='/tmp/daq-p1604-step2-add-dialog.png', full_page=True)
    print("Step 2: Clicked add device button")
    
    # 查找对话框中的输入框
    inputs = page.locator('input').all()
    print(f"Found {len(inputs)} inputs after clicking add:")
    for i, inp in enumerate(inputs):
        try:
            placeholder = inp.get_attribute("placeholder") or ""
            inp_type = inp.get_attribute("type") or ""
            value = inp.input_value() or ""
            visible = inp.is_visible()
            print(f"  [{i}] type='{inp_type}', placeholder='{placeholder}', value='{value}', visible={visible}")
        except Exception as e:
            print(f"  [{i}] <error: {e}>")
    
    # 查找对话框中的按钮
    buttons = page.locator('button').all()
    print(f"\nFound {len(buttons)} buttons after clicking add:")
    for i, btn in enumerate(buttons):
        try:
            text = (btn.text_content() or "").strip()[:50]
            title = btn.get_attribute("title") or ""
            visible = btn.is_visible()
            if visible:
                print(f"  [{i}] text='{text}', title='{title}'")
        except:
            pass
    
    # 打印错误日志
    print("\n--- Error Logs ---")
    for log in console_logs:
        if "ERROR" in log or "error" in log.lower() or "PAGE_ERROR" in log:
            print(log)
    
    browser.close()
