"""wind-daq 前端烟雾测试 —— echarts 按需注册回归测试

验证目标（依据 perf-bundle-fix-verification.md 第 4 节 checklist）：

1. 首页 (MainDashboardView) 能正常渲染，无 JS 报错
2. 设备面板出现时，RealtimeChart 异步加载并初始化（验证 LineChart 注册）
3. 切到 traversal 页面，4 个可视化 tab 能正常渲染（即使没数据也要无 import 报错）
4. 切到 calibration 页面，能正常渲染
5. 切到 log 页面，能正常渲染

测试方法：通过 console error / page error 捕获任何 echarts 注册错误
（如 "Series xxx is not registered"），失败立即抛出。

前提：
  - 后端跑在 :8080（默认）
  - 前端 preview 跑在 :15174（npx vite preview --port 15174）
"""
import os
import sys
import time
from playwright.sync_api import sync_playwright, Page

# 截图目录：Windows 上 /tmp 在 git-bash 里有 alias，但 Python 进程拿不到，
# 用 tempfile 取真实路径，并确保目录存在
import tempfile
SCREENSHOT_DIR = os.path.join(tempfile.gettempdir(), "winddaq-smoke")
os.makedirs(SCREENSHOT_DIR, exist_ok=True)


def collect_errors(page: Page) -> tuple[list, list]:
    """注册所有 error 捕获通道，返回 (errors, loaded_chunks)"""
    errors = []
    chunks = []
    page.on("pageerror", lambda exc: errors.append(("pageerror", str(exc))))

    def on_console(msg):
        if msg.type == "error":
            text = msg.text
            if "404" in text or "ERR_CONNECTION" in text or "Failed to load resource" in text:
                return
            errors.append(("console.error", text))

    def on_response(resp):
        url_path = resp.url
        # 只关心 .js chunk
        if ".js" in url_path and resp.status < 400:
            # 提取文件名
            import re
            m = re.search(r'/assets/([^/?]+\.js)', url_path)
            if m:
                chunks.append(m.group(1))

    page.on("console", on_console)
    page.on("response", on_response)
    return errors, chunks


def assert_no_echarts_errors(errors: list, scenario: str):
    """专门检查 echarts 注册相关错误"""
    echarts_errs = [
        (typ, msg) for typ, msg in errors
        if any(kw in msg.lower() for kw in [
            "echarts", "is not registered", "is not a constructor",
            "componenttype", "seriestype"
        ])
    ]
    if echarts_errs:
        print(f"  ❌ {scenario}: ECHARTS 错误 {len(echarts_errs)} 条")
        for typ, msg in echarts_errs[:5]:
            print(f"      [{typ}] {msg[:200]}")
        return False
    print(f"  ✅ {scenario}: 无 echarts 错误")
    return True


def smoke_test():
    url = "http://localhost:15174"
    results = []

    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        ctx = browser.new_context()
        page = ctx.new_page()
        errors, chunks = collect_errors(page)

        # ============ 1. 首页加载 ============
        print("\n=== Scenario 1: 首页加载 (MainDashboardView) ===")
        # 不用 networkidle —— 设备 SSE 长连接会让 networkidle 永远不到
        page.goto(url, wait_until="domcontentloaded")
        page.wait_for_timeout(3500)
        results.append(("首页加载", assert_no_echarts_errors(errors, "首页加载")))

        # ============ 2. 设备面板 (RealtimeChart) ============
        print("\n=== Scenario 2: 设备面板（RealtimeChart 异步加载）===")
        prev_count = len(errors)
        try:
            # 找设备 sim-1 卡片或加号；多种策略尝试
            # 默认 dashboard 应该已经显示设备列表，sim-1 是默认设备
            page.wait_for_timeout(1500)
            # 尝试点击 sim-1 设备卡片（DeviceSidebar 或 DeviceOverviewPanel）
            sim_card = page.locator("text=/Simulator|sim-1|模拟/").first
            if sim_card.count() > 0:
                sim_card.click(timeout=3000)
                page.wait_for_timeout(2500)
                print("  ℹ️ 已点击设备 sim-1")
            else:
                print("  ℹ️ 未找到设备卡片，跳过此场景")
            new = errors[prev_count:]
            results.append(("设备面板", assert_no_echarts_errors(new, "设备面板")))
        except Exception as e:
            print(f"  ⚠️ 交互失败: {e}")
            results.append(("设备面板", False))

        # ============ 3. 切到 Traversal 页 ============
        print("\n=== Scenario 3: Traversal 页 + 4 个可视化 tab ===")
        prev_count = len(errors)

        # 拦截 traversal status 接口，返回带 dataPoints 的假数据，
        # 这样 4 个可视化组件会真实挂载，echarts 注册才能被验证。
        def mock_traversal_status(route):
            fake_status = {
                "state": "completed",
                "config": None,
                "completedPoints": 9,
                "totalPoints": 9,
                "currentPointIndex": 8,
                "dataPoints": [
                    {
                        "pointId": i,
                        "coordinates": {"alpha": -10 + (i % 3) * 10, "beta": -10 + (i // 3) * 10},
                        "rawPressure": {"p1": 100 + i, "p2": 200, "p3": 100 - i,
                                        "p4": 105 + i, "p5": 95 - i, "pAtm": 101325, "tAtm": 20},
                        "interpolationResult": {
                            "isValid": True,
                            "alpha": -10 + (i % 3) * 10,
                            "beta": -10 + (i // 3) * 10,
                            "machNumber": 0.3,
                            "velocity": 100,
                            "vx": 50, "vy": 10, "vz": 80,
                            "totalPressure": 101800, "staticPressure": 101000,
                            "totalTemperature": 295, "staticTemperature": 293,
                            "cas": 100, "sat": 20,
                            "warning": ""
                        },
                        "sampleCount": 100,
                        "timestamp": 1700000000000 + i * 1000,
                        "dwellTimeElapsed": 1000
                    }
                    for i in range(9)
                ],
                "isPaused": False,
                "validationWarnings": [],
            }
            route.fulfill(status=200, content_type="application/json", body=__import__("json").dumps(fake_status))

        page.route("**/api/traversal/status", mock_traversal_status)

        try:
            traversal_nav = page.locator('[aria-label="遍历测试"], [aria-label="Traversal"]').first
            if traversal_nav.count() == 0:
                traversal_nav = page.locator('[title="遍历测试"]').first
            traversal_nav.click(timeout=5000)
            page.wait_for_timeout(3000)
            print("  ℹ️ 已进入 traversal 页")

            # Traversal 默认是 'preview' workspace tab；需要先切到 'visualization'
            vis_workspace_btn = page.locator('button[title="流场可视化"], button:has-text("流场可视化")').first
            if vis_workspace_btn.count() > 0:
                vis_workspace_btn.click(timeout=3000)
                page.wait_for_timeout(2000)
                print("  ℹ️ 已切到 workspace tab：流场可视化")
            else:
                print("  ⚠️ 找不到 '流场可视化' workspace tab")

            # 4 个内层 tab 依次点击；i18n 实际文本：热力图/剖面图/矢量场/压力雷达
            canvas_results = {}
            for tab_text in ["热力图", "剖面图", "矢量场", "压力雷达"]:
                btns_loc = page.get_by_role("button", name=tab_text, exact=True)
                btn_count = btns_loc.count()
                if btn_count == 0:
                    print(f"  ⚠️ 按钮 '{tab_text}' 不存在")
                    canvas_results[tab_text] = 0
                    continue
                btn = btns_loc.last
                try:
                    btn.scroll_into_view_if_needed(timeout=2000)
                    btn.click(timeout=3000, force=True)
                    page.wait_for_timeout(2000)
                    canvases = page.locator("canvas").count()
                    canvas_results[tab_text] = canvases
                    print(f"    - 切换到 tab {tab_text}: canvas 数 = {canvases}")
                    page.screenshot(path=os.path.join(SCREENSHOT_DIR, f"traversal-{tab_text}.png"), full_page=False)
                except Exception as e:
                    print(f"    - tab {tab_text} 点击失败: {e}")

            any_canvas = any(c > 0 for c in canvas_results.values())
            if not any_canvas:
                print("  ⚠️ 4 个 tab 都没出现 canvas — mock 数据可能未生效")
            new = errors[prev_count:]
            chart_ok = assert_no_echarts_errors(new, "Traversal+4tab")
            results.append(("Traversal+4tab(canvas=%s)" % any_canvas, chart_ok and any_canvas))
        except Exception as e:
            print(f"  ⚠️ traversal 交互失败: {e}")
            results.append(("Traversal+4tab", False))

        page.unroute("**/api/traversal/status")

        # ============ 4. 切到 Calibration ============
        print("\n=== Scenario 4: Calibration 页 ===")
        prev_count = len(errors)
        try:
            cal = page.locator('[aria-label="探针校准"], [aria-label*="Calibration"], [title="探针校准"]').first
            cal.click(timeout=5000)
            page.wait_for_timeout(2000)
            results.append(("Calibration", assert_no_echarts_errors(errors[prev_count:], "Calibration")))
        except Exception as e:
            print(f"  ⚠️ {e}")
            results.append(("Calibration", False))

        # ============ 5. 切到 Log ============
        print("\n=== Scenario 5: Log 页 ===")
        prev_count = len(errors)
        try:
            log_btn = page.locator('[aria-label="运行日志"], [aria-label*="Log"], [title="运行日志"]').first
            log_btn.click(timeout=5000)
            page.wait_for_timeout(2000)
            results.append(("Log", assert_no_echarts_errors(errors[prev_count:], "Log")))
        except Exception as e:
            print(f"  ⚠️ {e}")
            results.append(("Log", False))

        # ============ 收尾 ============
        page.screenshot(path=os.path.join(SCREENSHOT_DIR, "final.png"), full_page=True)
        browser.close()

    # 汇总
    print("\n" + "=" * 60)
    print("汇总")
    print("=" * 60)
    passed = sum(1 for _, ok in results if ok)
    for name, ok in results:
        print(f"  {'✅' if ok else '❌'} {name}")
    print(f"\n通过 {passed}/{len(results)}")

    # Chunk 加载报告（验证 lazy chunk 真的按需下载）
    print("\n=== Lazy chunk 加载分析 ===")
    unique_chunks = sorted(set(chunks))
    print(f"  共加载 {len(unique_chunks)} 个 .js chunk")
    interesting = ["RealtimeChart", "TraversalView", "CalibrationView", "LogViewer", "install"]
    for kw in interesting:
        matched = [c for c in unique_chunks if kw in c]
        if matched:
            print(f"  ✅ {kw}: {matched[0]}（已 lazy 加载）")
        else:
            print(f"  ⚠️  {kw}: 未加载（场景未触发）")

    return passed == len(results)


if __name__ == "__main__":
    ok = smoke_test()
    sys.exit(0 if ok else 1)
