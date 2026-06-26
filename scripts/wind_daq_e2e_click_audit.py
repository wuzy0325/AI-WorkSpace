from __future__ import annotations

import json
import os
import re
import sys
import time
from dataclasses import dataclass, field
from typing import Any

from playwright.sync_api import Error, Page, TimeoutError, sync_playwright


APP_URL = os.environ.get("WIND_DAQ_E2E_URL", "http://127.0.0.1:9245")
REPORT_JSON = os.environ.get("WIND_DAQ_E2E_JSON", "wind-daq-e2e-click-audit.json")
MAX_PHASE_CLICKS = int(os.environ.get("WIND_DAQ_E2E_MAX_PHASE_CLICKS", "24"))

reconfigure_stdout = getattr(sys.stdout, "reconfigure", None)
if reconfigure_stdout:
    reconfigure_stdout(encoding="utf-8", errors="backslashreplace")


@dataclass
class ClickResult:
    phase: str
    label: str
    selector_hint: str
    status: str
    note: str = ""


@dataclass
class AuditState:
    clicks: list[ClickResult] = field(default_factory=list)
    console: list[str] = field(default_factory=list)
    page_errors: list[str] = field(default_factory=list)
    request_failures: list[str] = field(default_factory=list)
    disabled_buttons: list[dict[str, str]] = field(default_factory=list)
    visible_buttons_by_phase: dict[str, list[str]] = field(default_factory=dict)


def clean(text: str) -> str:
    return re.sub(r"\s+", " ", text or "").strip()


def describe_button(page: Page, index: int) -> dict[str, Any]:
    return page.locator("button").nth(index).evaluate(
        r"""
        (el) => {
          const rect = el.getBoundingClientRect();
          const style = window.getComputedStyle(el);
          const text = (el.innerText || el.textContent || '').replace(/\s+/g, ' ').trim();
          return {
            text,
            aria: el.getAttribute('aria-label') || '',
            title: el.getAttribute('title') || '',
            disabled: Boolean(el.disabled) || el.getAttribute('aria-disabled') === 'true',
            visible: rect.width > 0 && rect.height > 0 && style.visibility !== 'hidden' && style.display !== 'none',
            className: el.className || '',
          };
        }
        """
    )


def button_label(desc: dict[str, Any], fallback: str) -> str:
    return clean(desc.get("aria") or desc.get("title") or desc.get("text") or fallback)


def visible_buttons(page: Page) -> list[dict[str, Any]]:
    count = page.locator("button").count()
    out: list[dict[str, Any]] = []
    for i in range(count):
        try:
            desc = describe_button(page, i)
        except Error:
            continue
        if desc.get("visible"):
            desc["index"] = i
            desc["label"] = button_label(desc, f"button[{i}]")
            out.append(desc)
    return out


def capture_phase(state: AuditState, page: Page, phase: str) -> None:
    buttons = visible_buttons(page)
    state.visible_buttons_by_phase[phase] = [b["label"] for b in buttons]
    for b in buttons:
        if b.get("disabled"):
            state.disabled_buttons.append({"phase": phase, "label": b["label"]})


def click_button(page: Page, state: AuditState, phase: str, label_pattern: str, exact: bool = False) -> bool:
    pattern = re.compile(f"^{re.escape(label_pattern)}$" if exact else label_pattern, re.I)
    buttons = visible_buttons(page)
    for b in buttons:
        if b.get("disabled"):
            continue
        label = b["label"]
        if not pattern.search(label):
            continue
        before_errors = len(state.console) + len(state.page_errors)
        try:
            page.locator("button").nth(int(b["index"])).click(timeout=3000)
            page.wait_for_timeout(450)
            settle_overlays(page)
            after_errors = len(state.console) + len(state.page_errors)
            status = "ok" if after_errors == before_errors else "issue"
            state.clicks.append(ClickResult(phase, label, f"button[{b['index']}]", status))
            return True
        except Exception as exc:  # noqa: BLE001 - audit records UI failures without aborting the run.
            state.clicks.append(ClickResult(phase, label, f"button[{b['index']}]", "failed", str(exc)))
            return False
    state.clicks.append(ClickResult(phase, label_pattern, "button text/aria", "missing"))
    return False


def click_all_current_buttons(page: Page, state: AuditState, phase: str, max_clicks: int = MAX_PHASE_CLICKS) -> None:
    seen: set[str] = set()
    nav_like = re.compile("仪表盘|Dashboard|探针|校准|Calibration|遍历|Traversal|日志|Logs|Log|运动|Motion|设置|展开导航|收起导航", re.I)
    for _ in range(max_clicks):
        buttons = [b for b in visible_buttons(page) if not b.get("disabled")]
        target = None
        for b in buttons:
            label = b["label"]
            key = f"{phase}:{label}"
            if key in seen:
                continue
            if label in {"取消", "关闭", "完成", "知道了"} or nav_like.search(label):
                continue
            target = b
            break
        if not target:
            break
        label = target["label"]
        print(f"[{phase}] click {label}", flush=True)
        seen.add(f"{phase}:{label}")
        before_errors = len(state.console) + len(state.page_errors)
        try:
            page.locator("button").nth(int(target["index"])).click(timeout=1500)
            page.wait_for_timeout(500)
            settle_overlays(page)
            after_errors = len(state.console) + len(state.page_errors)
            state.clicks.append(
                ClickResult(phase, label, f"button[{target['index']}]", "ok" if after_errors == before_errors else "issue")
            )
        except Exception as exc:  # noqa: BLE001
            state.clicks.append(ClickResult(phase, label, f"button[{target['index']}]", "failed", str(exc)))
        page.wait_for_timeout(150)


def settle_overlays(page: Page) -> None:
    # Prefer cancel/close actions after destructive or modal-producing clicks so the audit stays non-destructive.
    for text in ["取消", "关闭", "完成", "知道了"]:
        try:
            candidate = page.get_by_role("button", name=re.compile(f"^{re.escape(text)}$"))
            if candidate.count() and candidate.first.is_visible(timeout=300):
                candidate.first.click(timeout=1000)
                page.wait_for_timeout(250)
        except Exception:
            pass


def navigate(page: Page, state: AuditState, label_pattern: str, phase: str) -> None:
    print(f"[{phase}] navigate {label_pattern}", flush=True)
    click_button(page, state, phase, label_pattern)
    try:
        page.wait_for_load_state("networkidle", timeout=8000)
    except TimeoutError:
        state.clicks.append(ClickResult(phase, "networkidle", "page", "timeout", "导航后仍有长连接或轮询请求，继续巡检"))
    page.wait_for_timeout(700)


def run() -> AuditState:
    state = AuditState()
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        page = browser.new_page(viewport={"width": 1440, "height": 950})
        page.on("console", lambda msg: state.console.append(f"{msg.type}: {msg.text}") if msg.type in {"error"} else None)
        page.on("pageerror", lambda exc: state.page_errors.append(str(exc)))
        page.on("requestfailed", lambda req: state.request_failures.append(f"{req.method} {req.url} {req.failure}"))

        page.goto(APP_URL, wait_until="domcontentloaded", timeout=30000)
        try:
            page.wait_for_load_state("networkidle", timeout=10000)
        except TimeoutError:
            pass
        page.wait_for_timeout(1500)

        capture_phase(state, page, "dashboard-initial")
        click_button(page, state, "dashboard-nav", "展开导航")
        capture_phase(state, page, "dashboard-expanded")

        click_all_current_buttons(page, state, "dashboard")
        capture_phase(state, page, "dashboard-after-clicks")

        navigate(page, state, "探针|校准|Probe|Calibration", "nav-calibration")
        capture_phase(state, page, "calibration")
        click_all_current_buttons(page, state, "calibration")

        navigate(page, state, "遍历|Traversal", "nav-traversal")
        capture_phase(state, page, "traversal")
        click_all_current_buttons(page, state, "traversal")

        navigate(page, state, "Logs|日志|Log", "nav-log")
        capture_phase(state, page, "log")
        click_all_current_buttons(page, state, "log")

        click_button(page, state, "settings", "设置")
        capture_phase(state, page, "settings")
        click_all_current_buttons(page, state, "settings")

        browser.close()
    return state


if __name__ == "__main__":
    started = time.strftime("%Y-%m-%d %H:%M:%S")
    result = run()
    payload = {
        "started": started,
        "url": APP_URL,
        "clicks": [r.__dict__ for r in result.clicks],
        "console": result.console,
        "page_errors": result.page_errors,
        "request_failures": result.request_failures,
        "disabled_buttons": result.disabled_buttons,
        "visible_buttons_by_phase": result.visible_buttons_by_phase,
    }
    with open(REPORT_JSON, "w", encoding="utf-8") as f:
        json.dump(payload, f, ensure_ascii=False, indent=2)
    print(json.dumps({"report": REPORT_JSON, "clicks": len(result.clicks), "console_errors": len(result.console), "page_errors": len(result.page_errors)}, ensure_ascii=False))
