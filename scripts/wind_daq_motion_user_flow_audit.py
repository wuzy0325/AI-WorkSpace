from __future__ import annotations

import json
import re
import sys
import time
from dataclasses import dataclass, field
from urllib.parse import urlparse

from playwright.sync_api import Page, Route, TimeoutError, sync_playwright


APP_URL = "http://127.0.0.1:9245/#/motion"
LOCAL_API = "http://127.0.0.1:8900"
REPORT_JSON = "wind-daq-motion-user-flow-audit.json"

reconfigure_stdout = getattr(sys.stdout, "reconfigure", None)
if reconfigure_stdout:
    reconfigure_stdout(encoding="utf-8", errors="backslashreplace")


@dataclass
class Audit:
    actions: list[dict[str, str]] = field(default_factory=list)
    console: list[str] = field(default_factory=list)
    page_errors: list[str] = field(default_factory=list)
    request_failures: list[str] = field(default_factory=list)
    snapshots: list[dict[str, object]] = field(default_factory=list)


def log(audit: Audit, target: str, status: str, note: str = "") -> None:
    print(f"{status}: {target}", flush=True)
    audit.actions.append({"target": target, "status": status, "note": note})


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


def api_snapshot(page: Page, audit: Audit, label: str) -> None:
    try:
        resp = page.request.get(f"{LOCAL_API}/api/motion/status", timeout=3000)
        audit.snapshots.append({"label": label, "status": resp.status, "body": resp.json()})
    except Exception as exc:  # noqa: BLE001
        audit.snapshots.append({"label": label, "status": "failed", "body": str(exc)})


def reload_motion(page: Page, audit: Audit, label: str) -> None:
    page.goto(APP_URL, wait_until="domcontentloaded", timeout=30000)
    try:
        page.wait_for_load_state("networkidle", timeout=5000)
    except TimeoutError:
        pass
    page.wait_for_timeout(1200)
    log(audit, label, "reloaded")


def click_role(page: Page, audit: Audit, name: str, exact: bool = True) -> bool:
    before = len(audit.console) + len(audit.page_errors) + len(audit.request_failures)
    try:
        loc = page.get_by_role("button", name=name, exact=exact)
        if loc.count() == 0:
            log(audit, name, "missing")
            return False
        if loc.first.is_disabled(timeout=800):
            log(audit, name, "disabled")
            return False
        loc.first.click(timeout=3000)
        page.wait_for_timeout(900)
        after = len(audit.console) + len(audit.page_errors) + len(audit.request_failures)
        log(audit, name, "ok" if after == before else "issue")
        return True
    except Exception as exc:  # noqa: BLE001
        log(audit, name, "failed", str(exc)[:800])
        return False


def click_text_button(page: Page, audit: Audit, pattern: str, index: int = 0) -> bool:
    regex = re.compile(pattern, re.I)
    matches = []
    for i in range(page.locator("button").count()):
        btn = page.locator("button").nth(i)
        try:
            if not btn.is_visible(timeout=300):
                continue
            text = (btn.inner_text(timeout=500) or "") + " " + (btn.get_attribute("title") or "")
            if regex.search(text):
                matches.append((btn, re.sub(r"\s+", " ", text).strip()))
        except Exception:
            continue
    if len(matches) <= index:
        log(audit, pattern, "missing")
        return False
    btn, label = matches[index]
    before = len(audit.console) + len(audit.page_errors) + len(audit.request_failures)
    try:
        if btn.is_disabled(timeout=500):
            log(audit, label, "disabled")
            return False
        btn.click(timeout=3000)
        page.wait_for_timeout(900)
        after = len(audit.console) + len(audit.page_errors) + len(audit.request_failures)
        log(audit, label, "ok" if after == before else "issue")
        return True
    except Exception as exc:  # noqa: BLE001
        log(audit, label, "failed", str(exc)[:800])
        return False


def run() -> Audit:
    audit = Audit()
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        page = browser.new_page(viewport={"width": 1440, "height": 950})
        page.route("**/api/motion/**", proxy_motion)
        page.on("console", lambda msg: audit.console.append(f"{msg.type}: {msg.text}") if msg.type == "error" else None)
        page.on("pageerror", lambda exc: audit.page_errors.append(str(exc)))
        page.on("requestfailed", lambda req: audit.request_failures.append(f"{req.method} {req.url} {req.failure}"))

        reload_motion(page, audit, "motion page")
        api_snapshot(page, audit, "initial")

        click_text_button(page, audit, "Simulated Controller 1|模拟控制器 1")
        api_snapshot(page, audit, "after select simulated")

        click_role(page, audit, "断开")
        api_snapshot(page, audit, "after disconnect")
        reload_motion(page, audit, "motion page after disconnect")
        click_text_button(page, audit, "Simulated Controller 1|模拟控制器 1")
        click_role(page, audit, "连接")
        api_snapshot(page, audit, "after reconnect")
        reload_motion(page, audit, "motion page after reconnect")
        click_text_button(page, audit, "Simulated Controller 1|模拟控制器 1")

        click_text_button(page, audit, "置零|Set Zero")
        api_snapshot(page, audit, "after set zero")
        click_text_button(page, audit, r"\+")
        api_snapshot(page, audit, "after plus step")
        click_text_button(page, audit, "移动|Move")
        api_snapshot(page, audit, "after move")
        click_role(page, audit, "全部停止")
        api_snapshot(page, audit, "after stop all")
        click_role(page, audit, "急停")
        api_snapshot(page, audit, "after e-stop")

        click_role(page, audit, "配置")
        page.wait_for_timeout(800)
        click_text_button(page, audit, "B140 控制器|B140")
        click_text_button(page, audit, "新建")
        click_role(page, audit, "取消")

        browser.close()
    return audit


if __name__ == "__main__":
    result = run()
    payload = {
        "started": time.strftime("%Y-%m-%d %H:%M:%S"),
        "url": APP_URL,
        "localApi": LOCAL_API,
        "actions": result.actions,
        "snapshots": result.snapshots,
        "console": result.console,
        "page_errors": result.page_errors,
        "request_failures": result.request_failures,
    }
    with open(REPORT_JSON, "w", encoding="utf-8") as f:
        json.dump(payload, f, ensure_ascii=False, indent=2)
    print(json.dumps({"report": REPORT_JSON, "actions": len(result.actions), "errors": len(result.console) + len(result.page_errors)}, ensure_ascii=False), flush=True)
