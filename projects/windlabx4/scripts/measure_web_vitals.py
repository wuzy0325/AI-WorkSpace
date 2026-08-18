"""抓取 WindLabX4 前端的 Web Vitals 数据。
默认指向 http://localhost:15173（vite dev）；
传 --preview 改用 http://localhost:15174（vite preview，生产构建）。
"""
import sys
import time
from playwright.sync_api import sync_playwright


def main():
    url = "http://localhost:15174" if "--preview" in sys.argv else "http://localhost:15173"
    print(f"Target: {url}")
    logs = []
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        ctx = browser.new_context()
        page = ctx.new_page()

        # 捕获所有控制台输出
        page.on("console", lambda msg: logs.append((msg.type, msg.text)))

        page.goto(url, wait_until="networkidle")
        # web-vitals 的 LCP/CLS 需要交互或页面停留才上报；触发一次点击/移动激活 INP
        page.wait_for_timeout(2000)
        try:
            page.mouse.move(400, 300)
            page.mouse.click(400, 300)
        except Exception:
            pass
        page.wait_for_timeout(2500)

        # 触发 hidden 让 web-vitals flush LCP/CLS（这是 web-vitals 上报机制）
        page.evaluate("document.dispatchEvent(new Event('visibilitychange'))")
        page.evaluate("Object.defineProperty(document, 'visibilityState', {value: 'hidden', configurable: true})")
        page.evaluate("document.dispatchEvent(new Event('visibilitychange'))")
        page.wait_for_timeout(1000)

        samples = page.evaluate("window.__WEB_VITALS__ || []")

        browser.close()

    print("\n=== Console logs related to web-vitals ===")
    for typ, text in logs:
        if "web-vitals" in text or "vitals" in text.lower():
            print(f"  [{typ}] {text}")

    print("\n=== __WEB_VITALS__ buffer ===")
    if not samples:
        print("  (empty — metrics may not have fired yet)")
    else:
        for s in samples:
            name = s.get("name", "?")
            value = s.get("value", 0)
            rating = s.get("rating", "?")
            display = f"{value:.3f}" if name == "CLS" else f"{int(value)}ms"
            print(f"  {name:5} = {display:>8}  ({rating})")

    print("\n=== Other console messages (first 20) ===")
    for typ, text in logs[:20]:
        if "web-vitals" not in text:
            print(f"  [{typ}] {text[:120]}")


if __name__ == "__main__":
    main()
