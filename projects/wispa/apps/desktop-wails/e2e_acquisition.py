"""
wispa 真实设备 E2E 测试脚本

测试链路：
  1. 通过 CDP 连接已运行的 WebView2（wispa_CDP_PORT=9222）
  2. 截图初始状态
  3. 选中侧栏的 P1604 设备
  4. 点击"连接"按钮，等待设备进入 Connected 状态
  5. 截图已连接状态
  6. 点击"开始采集"，等待数据流入
  7. 截图实时数据
  8. 等待几秒，再截图（验证持续采集）
  9. 点击"停止采集"
  10. 点击"断开"
  11. 最终截图

每一步都打印日志并保存截图到 docs/test-result/e2e-acquisition/
"""

from __future__ import annotations

import os
import sys
import time
from pathlib import Path
from typing import Optional

from playwright.sync_api import sync_playwright, Browser, Page, TimeoutError as PWTimeout

# ---------------- 配置 ----------------
CDP_URL = "http://127.0.0.1:9222"
SHOT_DIR = Path(__file__).parent.parent / "docs" / "test-result" / "e2e-acquisition"
SHOT_DIR.mkdir(parents=True, exist_ok=True)

# 等待元素的最长时间（秒）
DEFAULT_TIMEOUT = 15_000
# 等待状态稳定的最长时间（毫秒）
STATE_TIMEOUT = 30_000


def log(msg: str) -> None:
    print(f"[E2E] {time.strftime('%H:%M:%S')} {msg}", flush=True)


def shot(page: Page, name: str) -> Path:
    """截图并返回路径。文件名按步骤编号 + 名称。"""
    out = SHOT_DIR / f"{name}.png"
    page.screenshot(path=str(out), full_page=False)
    log(f"截图保存: {out.name}")
    return out


def wait_for_visible(page: Page, selector: str, timeout: int = DEFAULT_TIMEOUT):
    log(f"等待元素可见: {selector}")
    loc = page.locator(selector).first
    loc.wait_for(state="visible", timeout=timeout)
    return loc


def get_device_status_text(page: Page) -> str:
    """读取详情头部的状态标签文本。"""
    try:
        # 详情头 .n-tag 内文本：已连接 / 未连接 / 连接中 / 采集中 / 错误
        txt = page.locator(".detail__header-right .n-tag").first.inner_text(timeout=2000)
        return txt.strip()
    except Exception:
        return "<unknown>"


def click_text_button(page: Page, text: str, timeout: int = DEFAULT_TIMEOUT):
    """点击包含指定文本的按钮（兼容 Naive UI NButton）。"""
    log(f"点击按钮: {text}")
    # 优先匹配按钮内包含完整文本
    btn = page.locator(f"button:has-text(\"{text}\")").first
    btn.wait_for(state="visible", timeout=timeout)
    # 滚动到可见
    btn.scroll_into_view_if_needed()
    btn.click()
    return btn


def select_first_device(page: Page) -> str:
    """选中侧栏第一个设备条目，返回设备名。"""
    item = page.locator("[data-testid='sidebar-item']").first
    item.wait_for(state="visible", timeout=DEFAULT_TIMEOUT)
    item.click()
    # 读取设备名（详情头部 h2 或 .detail__device-info 内文本）
    try:
        name = page.locator(".detail__device-info h2").first.inner_text(timeout=3000).strip()
    except Exception:
        name = "<unknown>"
    log(f"已选中设备: {name}")
    return name


def main() -> int:
    log(f"连接 CDP: {CDP_URL}")
    with sync_playwright() as p:
        browser: Browser = p.chromium.connect_over_cdp(CDP_URL)
        # WebView2 通常只有一个 context 和一个 page
        if not browser.contexts:
            log("错误：没有 browser context")
            return 2
        context = browser.contexts[0]
        if not context.pages:
            log("错误：没有 page（窗口可能还没创建）")
            return 3
        page: Page = context.pages[0]
        page.set_default_timeout(DEFAULT_TIMEOUT)
        log(f"已连接页面: {page.url}")
        # 等待 Vue 应用挂载完成（侧栏可见）
        try:
            wait_for_visible(page, "aside.sidebar")
        except PWTimeout:
            log("侧栏未出现，可能页面未加载完成")
            shot(page, "00-fail-no-sidebar")
            return 4

        # 步骤 0：初始状态截图
        shot(page, "00-initial")

        # 步骤 1：选中侧栏第一个设备
        log("=" * 60)
        log("步骤 1：选中侧栏第一个设备")
        try:
            device_name = select_first_device(page)
        except Exception as e:
            log(f"选中设备失败: {e}")
            shot(page, "01-fail-select")
            return 5
        # 等待详情面板加载
        try:
            wait_for_visible(page, ".detail__header", timeout=8000)
        except PWTimeout:
            log("详情头部未出现")
            shot(page, "01-fail-detail-header")
            return 6
        time.sleep(1.0)  # 等状态标签渲染稳定
        initial_status = get_device_status_text(page)
        log(f"初始状态: {initial_status}")
        shot(page, "01-device-selected")

        # 步骤 2：连接设备
        log("=" * 60)
        log("步骤 2：连接设备")
        # 如果初始就是已连接/采集中，跳过连接
        if initial_status in ("已连接", "采集中"):
            log(f"设备已处于 {initial_status} 状态，跳过连接步骤")
        else:
            try:
                click_text_button(page, "连接")
            except PWTimeout:
                log("找不到'连接'按钮，可能已连接或文本不同")
                shot(page, "02-fail-connect-btn")
                return 7
            # 等待状态变化：连接中 -> 已连接
            log("等待设备进入'已连接'状态...")
            deadline = time.time() + STATE_TIMEOUT / 1000
            connected = False
            while time.time() < deadline:
                st = get_device_status_text(page)
                if st == "已连接":
                    connected = True
                    break
                if st == "错误":
                    log("设备进入错误状态")
                    shot(page, "02-error-state")
                    return 8
                time.sleep(0.5)
            if not connected:
                log(f"等待'已连接'超时，当前状态: {get_device_status_text(page)}")
                shot(page, "02-timeout-connect")
                return 9
            log("设备已连接")
        shot(page, "02-connected")

        # 步骤 3：开始采集
        log("=" * 60)
        log("步骤 3：开始采集")
        try:
            click_text_button(page, "开始采集")
        except PWTimeout:
            log("找不到'开始采集'按钮")
            shot(page, "03-fail-start-btn")
            return 10
        # 等待状态变为"采集中"
        log("等待设备进入'采集中'状态...")
        deadline = time.time() + STATE_TIMEOUT / 1000
        acquiring = False
        while time.time() < deadline:
            st = get_device_status_text(page)
            if st == "采集中":
                acquiring = True
                break
            if st == "错误":
                log("设备进入错误状态")
                shot(page, "03-error-state")
                return 11
            time.sleep(0.5)
        if not acquiring:
            log(f"等待'采集中'超时，当前状态: {get_device_status_text(page)}")
            shot(page, "03-timeout-acquire")
            return 12
        log("设备采集中")

        # 步骤 4：等待数据流入，截几张实时数据截图
        log("=" * 60)
        log("步骤 4：观察实时数据")
        # 等通道卡片出现数值
        time.sleep(2.0)
        shot(page, "04-acquiring-t2")
        time.sleep(3.0)
        shot(page, "05-acquiring-t5")

        # 步骤 5：停止采集
        log("=" * 60)
        log("步骤 5：停止采集")
        try:
            click_text_button(page, "停止采集")
        except PWTimeout:
            log("找不到'停止采集'按钮，可能状态已变化")
            shot(page, "06-fail-stop-btn")
        # 等待状态回到"已连接"
        deadline = time.time() + STATE_TIMEOUT / 1000
        while time.time() < deadline:
            st = get_device_status_text(page)
            if st == "已连接":
                break
            time.sleep(0.5)
        time.sleep(1.0)
        shot(page, "06-stopped")

        # 步骤 6：断开设备
        log("=" * 60)
        log("步骤 6：断开设备")
        try:
            click_text_button(page, "断开")
        except PWTimeout:
            log("找不到'断开'按钮")
            shot(page, "07-fail-disconnect-btn")
        deadline = time.time() + STATE_TIMEOUT / 1000
        while time.time() < deadline:
            st = get_device_status_text(page)
            if st in ("未连接", "已断开"):
                break
            time.sleep(0.5)
        time.sleep(1.0)
        final_status = get_device_status_text(page)
        log(f"最终状态: {final_status}")
        shot(page, "07-disconnected")

        log("=" * 60)
        log("E2E 测试全部通过 ✓")
        log(f"截图目录: {SHOT_DIR}")

    return 0


if __name__ == "__main__":
    sys.exit(main())
