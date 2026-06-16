from playwright.sync_api import sync_playwright

with sync_playwright() as p:
    browser = p.chromium.launch(headless=True)
    page = browser.new_page(viewport={"width": 1600, "height": 900})
    
    # 访问 Wails dev server
    page.goto('http://localhost:34115')
    page.wait_for_load_state('networkidle')
    page.wait_for_timeout(3000)
    
    # 截图查看初始状态
    page.screenshot(path='/tmp/daq-p1604-initial.png', full_page=True)
    print("Initial screenshot saved")
    
    browser.close()
