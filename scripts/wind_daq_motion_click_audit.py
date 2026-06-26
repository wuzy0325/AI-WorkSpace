from __future__ import annotations

import json
import os
import re
import sys
import time
from dataclasses import dataclass, field
from typing import Any

from playwright.sync_api import Error, Page, TimeoutError, sync_playwright


APP_URL = os.environ.get("WIND_DAQ_E2E_URL", "http://127.0.0.1:9245/#/motion")
REPORT_JSON = os.environ.get("WIND_DAQ_MOTION_E2E_JSON", "wind-daq-motion-click-audit.json")

reconfigure_stdout = getattr(sys.stdout, "reconfigure", None)
if reconfigure_stdout:
    reconfigure_stdout(encoding="utf-8", errors="backslashreplace")


@dataclass
class ActionResult:
    phase: str
    target: str
    status: str
    note: str = ""


@dataclass
class AuditState:
    actions: list[ActionResult] = field(default_factory=list)
    console: list[str] = field(default_factory=list)
    page_errors: list[str] = field(default_factory=list)
    request_failures: list[str] = field(default_factory=list)
    visible_buttons: dict[str, list[str]] = field(default_factory=dict)
    visible_inputs: dict[str, list[str]] = field(default_factory=dict)


def clean(text: str) -> str:
    return re.sub(r"\s+", " ", text or "").strip()


def labels(page: Page, selector: str) -> list[str]:
    out: list[str] = []
    count = page.locator(selector).count()
    for idx in range(count):
        try:
            item = page.locator(selector).nth(idx).evaluate(
                """
                (el) => {
                  const rect = el.getBoundingClientRect();
                  const style = window.getComputedStyle(el);
                  if (rect.width <= 0 || rect.height <= 0 || style.display === 'none' || style.visibility === 'hidden') return '';
                  return [el.getAttribute('aria-label'), el.getAttribute('title'), el.innerText, el.textContent]
                    .filter(Boolean).join(' ').replace(/\s+/g, ' ').trim();
                }
                """
            )
        except Error:
            continue
        if clean(str(item)):
            out.append(clean(str(item)))
    return out


def capture(state: AuditState, page: Page, phase: str) -> None:
    state.visible_buttons[phase] = labels(page, "button")
    state.visible_inputs[phase] = labels(page, "input, select, textarea")


def record(state: AuditState, phase: str, target: str, status: str, note: str = "") -> None:
    print(f"[{phase}] {status}: {target}", flush=True)
    state.actions.append(ActionResult(phase, target, status, note))


def click_by_text(page: Page, state: AuditState, phase: str, pattern: str, skip_disabled: bool = True) -> bool:
    rx = re.compile(pattern, re.I)
    count = page.locator("button").count()
    for idx in range(count):
        button = page.locator("button").nth(idx)
        try:
            desc = button.evaluate(
                """
                (el) => {
                  const rect = el.getBoundingClientRect();
                  const style = window.getComputedStyle(el);
                  return {
                    label: [el.getAttribute('aria-label'), el.getAttribute('title'), el.innerText, el.textContent]
                      .filter(Boolean).join(' ').replace(/\s+/g, ' ').trim(),
                    visible: rect.width > 0 && rect.height > 0 && style.display !== 'none' && style.visibility !== 'hidden',
                    disabled: Boolean(el.disabled) || el.getAttribute('aria-disabled') === 'true'
                  };
                }
                """
            )
        except Error as exc:
            record(state, phase, f"button[{idx}]", "failed", str(exc))
            continue
        label = clean(str(desc.get("label") or f"button[{idx}]"))
        if not desc.get("visible") or not rx.search(label):
            continue
        if skip_disabled and desc.get("disabled"):
            record(state, phase, label, "disabled")
            return False
        before = len(state.console) + len(state.page_errors) + len(state.request_failures)
        try:
            button.click(timeout=2000)
            page.wait_for_timeout(600)
            after = len(state.console) + len(state.page_errors) + len(state.request_failures)
            record(state, phase, label, "ok" if after == before else "issue")
            return True
        except Exception as exc:  # noqa: BLE001
            record(state, phase, label, "failed", str(exc))
            return False
    record(state, phase, pattern, "missing")
    return False


def set_select_value(page: Page, state: AuditState, phase: str, value: str) -> bool:
    count = page.locator("select").count()
    for idx in range(count):
        select = page.locator("select").nth(idx)
        try:
            options = select.evaluate("(el) => Array.from(el.options || []).map((o) => o.value)")
            if value not in options:
                continue
            select.select_option(value)
            page.wait_for_timeout(350)
            record(state, phase, f"select[{idx}]={value}", "ok")
            return True
        except Exception as exc:  # noqa: BLE001
            record(state, phase, f"select[{idx}]={value}", "failed", str(exc))
            return False
    record(state, phase, f"select option {value}", "missing")
    return False


def click_checkbox(page: Page, state: AuditState, phase: str, index: int) -> bool:
    box = page.locator("input[type='checkbox']").nth(index)
    try:
        if box.count() == 0:
            record(state, phase, f"checkbox[{index}]", "missing")
            return False
        box.click(timeout=1500)
        page.wait_for_timeout(250)
        record(state, phase, f"checkbox[{index}]", "ok")
        return True
    except Exception as exc:  # noqa: BLE001
        record(state, phase, f"checkbox[{index}]", "failed", str(exc))
        return False


def run() -> AuditState:
    state = AuditState()
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        page = browser.new_page(viewport={"width": 1440, "height": 950})
        page.on("console", lambda msg: state.console.append(f"{msg.type}: {msg.text}") if msg.type == "error" else None)
        page.on("pageerror", lambda exc: state.page_errors.append(str(exc)))
        page.on("requestfailed", lambda req: state.request_failures.append(f"{req.method} {req.url} {req.failure}"))

        page.goto(APP_URL, wait_until="domcontentloaded", timeout=30000)
        try:
            page.wait_for_load_state("networkidle", timeout=8000)
        except TimeoutError:
            record(state, "motion-load", "networkidle", "timeout", "motion page has polling or failed requests")
        page.wait_for_timeout(1200)
        capture(state, page, "motion-initial")

        click_by_text(page, state, "motion-control", "配置|Config")
        page.wait_for_timeout(1000)
        capture(state, page, "motion-config-open")

        click_checkbox(page, state, "motion-config", 0)
        set_select_value(page, state, "motion-config", "B140-MC")
        set_select_value(page, state, "motion-config", "ROTARY")
        click_checkbox(page, state, "motion-config", 1)
        set_select_value(page, state, "motion-config", "encoder")
        click_checkbox(page, state, "motion-config", 2)
        capture(state, page, "motion-config-electric-encoder")
        click_by_text(page, state, "motion-config", "新建|New")
        capture(state, page, "motion-config-new")
        click_by_text(page, state, "motion-config", "取消|Cancel|关闭")

        capture(state, page, "motion-after-config")
        click_by_text(page, state, "motion-control", "连接|Connect")
        click_by_text(page, state, "motion-control", "断开|Disconnect")
        click_by_text(page, state, "motion-control", "停止全部|Stop All|Stop")
        click_by_text(page, state, "motion-control", "急停|紧急停止|E-Stop|Emergency")
        click_by_text(page, state, "motion-control", "关闭窗口")

        browser.close()
    return state


if __name__ == "__main__":
    payload_state = run()
    payload: dict[str, Any] = {
        "started": time.strftime("%Y-%m-%d %H:%M:%S"),
        "url": APP_URL,
        "actions": [a.__dict__ for a in payload_state.actions],
        "console": payload_state.console,
        "page_errors": payload_state.page_errors,
        "request_failures": payload_state.request_failures,
        "visible_buttons": payload_state.visible_buttons,
        "visible_inputs": payload_state.visible_inputs,
    }
    with open(REPORT_JSON, "w", encoding="utf-8") as f:
        json.dump(payload, f, ensure_ascii=False, indent=2)
    print(json.dumps({"report": REPORT_JSON, "actions": len(payload_state.actions), "errors": len(payload_state.console) + len(payload_state.page_errors)}, ensure_ascii=False), flush=True)
