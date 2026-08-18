from __future__ import annotations

import json
import re
import sys
import time
from dataclasses import dataclass, field
from urllib.parse import urlparse

from playwright.sync_api import Page, Route, TimeoutError, sync_playwright


APP_URL = "http://127.0.0.1:9245"
LOCAL_API = "http://127.0.0.1:8900"
REPORT_JSON = "WindLabX4-remaining-controls-audit.json"

reconfigure_stdout = getattr(sys.stdout, "reconfigure", None)
if reconfigure_stdout:
    reconfigure_stdout(encoding="utf-8", errors="backslashreplace")


@dataclass
class Audit:
    actions: list[dict[str, str]] = field(default_factory=list)
    snapshots: dict[str, list[str]] = field(default_factory=dict)
    console: list[str] = field(default_factory=list)
    page_errors: list[str] = field(default_factory=list)
    request_failures: list[str] = field(default_factory=list)


def log(audit: Audit, phase: str, target: str, status: str, note: str = "") -> None:
    print(f"[{phase}] {status}: {target}", flush=True)
    audit.actions.append({"phase": phase, "target": target, "status": status, "note": note})


def proxy_motion(route: Route) -> None:
    req = route.request
    parsed = urlparse(req.url)
    target = f"{LOCAL_API}{parsed.path}"
    if parsed.query:
        target += f"?{parsed.query}"
    try:
        response = route.fetch(url=target, method=req.method, headers=req.headers, post_data=req.post_data)
        route.fulfill(response=response)
    except Exception as exc:  # noqa: BLE001
        route.fulfill(status=502, body=f"motion proxy failed: {exc}")


def safe_label(text: str) -> str:
    return re.sub(r"\s+", " ", text or "").strip()


def snapshot_buttons(page: Page, audit: Audit, phase: str) -> None:
    labels: list[str] = []
    for i in range(page.locator("button").count()):
        button = page.locator("button").nth(i)
        try:
            if not button.is_visible(timeout=200):
                continue
            label = safe_label((button.get_attribute("aria-label") or "") + " " + (button.get_attribute("title") or "") + " " + button.inner_text(timeout=500))
            if label:
                labels.append(label)
        except Exception:
            continue
    audit.snapshots[phase] = labels


def click_button(page: Page, audit: Audit, phase: str, pattern: str, *, exact: bool = False, index: int = 0) -> bool:
    rx = re.compile(f"^{re.escape(pattern)}$" if exact else pattern, re.I)
    matches = []
    for i in range(page.locator("button").count()):
        button = page.locator("button").nth(i)
        try:
            if not button.is_visible(timeout=200):
                continue
            label = safe_label((button.get_attribute("aria-label") or "") + " " + (button.get_attribute("title") or "") + " " + button.inner_text(timeout=500))
            if rx.search(label):
                matches.append((button, label))
        except Exception:
            continue
    if len(matches) <= index:
        log(audit, phase, pattern, "missing")
        return False
    button, label = matches[index]
    before = len(audit.console) + len(audit.page_errors) + len(audit.request_failures)
    try:
        if button.is_disabled(timeout=300):
            log(audit, phase, label, "disabled")
            return False
        button.click(timeout=3000)
        page.wait_for_timeout(700)
        after = len(audit.console) + len(audit.page_errors) + len(audit.request_failures)
        log(audit, phase, label, "ok" if after == before else "issue")
        return True
    except Exception as exc:  # noqa: BLE001
        log(audit, phase, label, "failed", str(exc)[:800])
        return False


def click_text(page: Page, audit: Audit, phase: str, text: str) -> bool:
    try:
        target = page.get_by_text(text, exact=True).first
        if target.count() == 0:
            log(audit, phase, text, "missing")
            return False
        target.click(timeout=3000)
        page.wait_for_timeout(500)
        log(audit, phase, text, "ok")
        return True
    except Exception as exc:  # noqa: BLE001
        log(audit, phase, text, "failed", str(exc)[:800])
        return False


def click_checkbox_by_label(page: Page, audit: Audit, phase: str, label: str) -> bool:
    try:
        loc = page.locator("label", has_text=label).first
        if loc.count() == 0:
            log(audit, phase, label, "missing")
            return False
        loc.click(timeout=3000)
        page.wait_for_timeout(400)
        log(audit, phase, label, "ok")
        return True
    except Exception as exc:  # noqa: BLE001
        log(audit, phase, label, "failed", str(exc)[:800])
        return False


def open_dashboard(page: Page) -> None:
    page.goto(APP_URL, wait_until="domcontentloaded", timeout=30000)
    try:
        page.wait_for_load_state("networkidle", timeout=5000)
    except TimeoutError:
        pass
    page.wait_for_timeout(1300)


def audit_device_drawer(page: Page, audit: Audit) -> None:
    open_dashboard(page)
    click_button(page, audit, "device", "管理")
    page.wait_for_timeout(1200)
    snapshot_buttons(page, audit, "device-drawer-open")
    click_button(page, audit, "device", "扫描")
    click_button(page, audit, "device", "新建|添加")
    snapshot_buttons(page, audit, "device-editor-open")
    click_button(page, audit, "device", "基本|Basic")
    click_button(page, audit, "device", "通道|Channels")
    click_button(page, audit, "device", "全部启用")
    click_button(page, audit, "device", "全部禁用")
    click_button(page, audit, "device", "重置")
    click_button(page, audit, "device", "取消|关闭")
    click_button(page, audit, "device", "批量连接")
    click_button(page, audit, "device", "批量断开")
    click_button(page, audit, "device", "批量删除")
    click_button(page, audit, "device", "清除")
    click_button(page, audit, "device", "✕|关闭")


def audit_settings(page: Page, audit: Audit) -> None:
    open_dashboard(page)
    click_button(page, audit, "settings", "设置")
    page.wait_for_timeout(1000)
    snapshot_buttons(page, audit, "settings-open")
    for text in ["界面", "数据", "采集", "存储", "浅色", "深色", "中文", "English"]:
        click_text(page, audit, "settings", text)
    click_button(page, audit, "settings", "重试")
    click_button(page, audit, "settings", "选择")
    click_button(page, audit, "settings", "重置")
    click_button(page, audit, "settings", "取消|关闭")


def audit_motion_config(page: Page, audit: Audit) -> None:
    page.goto(f"{APP_URL}/#/motion", wait_until="domcontentloaded", timeout=30000)
    page.wait_for_timeout(1500)
    click_button(page, audit, "motion-config", "配置")
    page.wait_for_timeout(900)
    snapshot_buttons(page, audit, "motion-config-open")
    click_checkbox_by_label(page, audit, "motion-config", "自动连接")
    click_text(page, audit, "motion-config", "模拟控制器")
    click_text(page, audit, "motion-config", "B140 控制器")
    click_text(page, audit, "motion-config", "直线轴")
    click_text(page, audit, "motion-config", "旋转轴")
    click_checkbox_by_label(page, audit, "motion-config", "启用")
    click_checkbox_by_label(page, audit, "motion-config", "方向反转")
    click_text(page, audit, "motion-config", "寄存器")
    click_text(page, audit, "motion-config", "编码器")
    click_checkbox_by_label(page, audit, "motion-config", "启用")
    click_button(page, audit, "motion-config", "新建")
    click_button(page, audit, "motion-config", "取消")


def run() -> Audit:
    audit = Audit()
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        page = browser.new_page(viewport={"width": 1440, "height": 950})
        page.route("**/api/motion/**", proxy_motion)
        page.on("console", lambda msg: audit.console.append(f"{msg.type}: {msg.text}") if msg.type == "error" else None)
        page.on("pageerror", lambda exc: audit.page_errors.append(str(exc)))
        page.on("requestfailed", lambda req: audit.request_failures.append(f"{req.method} {req.url} {req.failure}"))

        audit_device_drawer(page, audit)
        audit_settings(page, audit)
        audit_motion_config(page, audit)

        browser.close()
    return audit


if __name__ == "__main__":
    result = run()
    payload = {
        "started": time.strftime("%Y-%m-%d %H:%M:%S"),
        "actions": result.actions,
        "snapshots": result.snapshots,
        "console": result.console,
        "page_errors": result.page_errors,
        "request_failures": result.request_failures,
    }
    with open(REPORT_JSON, "w", encoding="utf-8") as f:
        json.dump(payload, f, ensure_ascii=False, indent=2)
    print(json.dumps({"report": REPORT_JSON, "actions": len(result.actions), "errors": len(result.console) + len(result.page_errors)}, ensure_ascii=False), flush=True)
