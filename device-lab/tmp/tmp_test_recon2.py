from playwright.sync_api import sync_playwright
import time

with sync_playwright() as p:
    browser = p.chromium.launch(headless=True)
    page = browser.new_page(viewport={"width": 1600, "height": 900})
    
    # 收集控制台日志和错误
    console_logs = []
    page.on("console", lambda msg: console_logs.append(f"[{msg.type}] {msg.text}"))
    page.on("pageerror", lambda err: console_logs.append(f"[PAGE_ERROR] {err}"))
    
    # 访问前端 DevServer
    page.goto('http://localhost:15175')
    page.wait_for_load_state('networkidle')
    page.wait_for_timeout(3000)
    
    # 截图查看初始状态
    page.screenshot(path='/tmp/daq-p1604-step1.png', full_page=True)
    print("Step 1: Initial state")
    
    # 打印页面内容摘要
    html = page.content()
    print(f"Page HTML length: {len(html)}")
    
    # 查找所有按钮
    buttons = page.locator('button').all()
    print(f"\nFound {len(buttons)} buttons:")
    for i, btn in enumerate(buttons[:15]):
        try:
            text = (btn.text_content() or "").strip()[:50]
            title = btn.get_attribute("title") or ""
            data_testid = btn.get_attribute("data-testid") or ""
            print(f"  [{i}] text='{text}', title='{title}', testid='{data_testid}'")
        except:
            print(f"  [{i}] <error reading button>")

    # 查找输入框
    inputs = page.locator('input').all()
    print(f"\nFound {len(inputs)} inputs:")
    for i, inp in enumerate(inputs[:10]):
        try:
            placeholder = inp.get_attribute("placeholder") or ""
            inp_type = inp.get_attribute("type") or ""
            print(f"  [{i}] type='{inp_type}', placeholder='{placeholder}'")
        except:
            print(f"  [{i}] <error reading input>")
    
    # 打印控制台日志
    print("\n--- Console Logs ---")
    for log in console_logs:
        print(log)
    
    browser.close()
