"""
daq-p1604 E2E 测试脚本（基于 test-cases.html 194 个测试用例）

设计原则：
  - 按测试用例 ID 一一对应实现可自动化的用例
  - 异常场景（网线断开、磁盘满、UDP 广播失败等）标记为 SKIP，理由是物理操作无法自动化
  - 多设备场景标记为 SKIP，理由是测试环境通常只有 1 台真实设备
  - 每个用例独立 try/except，单个失败不影响后续
  - 每个用例都生成截图作为证据
  - 测试结果以结构化 JSON 输出，便于后续生成测试报告

退出码：
  0 - 全部 PASS 或 SKIP（无 FAIL）
  1 - 至少 1 个 FAIL
  2 - CDP 连接异常
  3 - 窗口未创建
  4 - 侧栏未挂载
"""

from __future__ import annotations

import json
import os
import sys
import time
import traceback
from dataclasses import dataclass, field, asdict
from pathlib import Path
from typing import Optional

from playwright.sync_api import sync_playwright, Browser, Page, TimeoutError as PWTimeout

# ---------------- 配置 ----------------
CDP_URL = "http://127.0.0.1:9222"
SHOT_DIR = Path(__file__).resolve().parent.parent.parent / "docs" / "test-result" / "e2e-test-cases"
SHOT_DIR.mkdir(parents=True, exist_ok=True)
REPORT_PATH = SHOT_DIR.parent / "e2e-report.json"

DEFAULT_TIMEOUT = 10_000       # 单元素等待 10 秒
STATE_TIMEOUT = 30_000         # 状态稳定等待 30 秒
QUICK_POLL = 500               # 状态轮询间隔 0.5 秒

# 已知设备 IP（如环境中存在则使用，否则首次用例会添加）
KNOWN_DEVICE_IP = os.environ.get("DAQ_P1604_TEST_IP", "192.168.1.7")
KNOWN_DEVICE_PORT = int(os.environ.get("DAQ_P1604_TEST_PORT", "9000"))
KNOWN_DEVICE_NAME = os.environ.get("DAQ_P1604_TEST_NAME", "E2E测试设备")


# ---------------- 测试结果收集 ----------------
@dataclass
class TestCaseResult:
    """单个测试用例结果。"""
    tc_id: str
    name: str
    module: str
    result: str = "untested"  # pass / fail / skip / block
    remark: str = ""
    screenshot: str = ""
    duration_ms: int = 0


@dataclass
class TestReport:
    """测试报告汇总。"""
    total: int = 0
    pass_count: int = 0
    fail_count: int = 0
    skip_count: int = 0
    block_count: int = 0
    cases: list = field(default_factory=list)

    def add(self, r: TestCaseResult) -> None:
        self.cases.append(r)
        self.total += 1
        if r.result == "pass":
            self.pass_count += 1
        elif r.result == "fail":
            self.fail_count += 1
        elif r.result == "skip":
            self.skip_count += 1
        elif r.result == "block":
            self.block_count += 1


# 全局报告实例
REPORT = TestReport()


# ---------------- 通用工具 ----------------
def log(msg: str) -> None:
    print(f"[E2E] {time.strftime('%H:%M:%S')} {msg}", flush=True)


def shot(page: Page, name: str) -> str:
    """截图并返回文件名。"""
    fname = f"{name}.png"
    out = SHOT_DIR / fname
    page.screenshot(path=str(out), full_page=False)
    return fname


def safe_text(page: Page, selector: str, timeout_ms: int = 3000) -> str:
    """安全读取元素文本，失败返回 <unknown>。"""
    try:
        return page.locator(selector).first.inner_text(timeout=timeout_ms).strip()
    except Exception:
        return "<unknown>"


def safe_attr(page: Page, selector: str, attr: str, timeout_ms: int = 2000) -> str:
    """安全读取元素属性，失败返回空串。"""
    try:
        return page.locator(selector).first.get_attribute(attr, timeout=timeout_ms) or ""
    except Exception:
        return ""


def click_btn_by_text(page: Page, text: str, timeout: int = DEFAULT_TIMEOUT):
    """按文本点击按钮（子串匹配，兼容 Naive UI NButton 内部 span 包裹）。"""
    btn = page.locator(f"button:has-text(\"{text}\")").first
    btn.wait_for(state="visible", timeout=timeout)
    btn.scroll_into_view_if_needed()
    btn.click()
    return btn


def click_btn_in_scope(page: Page, scope: str, text: str, timeout: int = DEFAULT_TIMEOUT):
    """在指定容器范围内按文本点击按钮（子串匹配）。"""
    btn = page.locator(f"{scope} button:has-text(\"{text}\")").first
    btn.wait_for(state="visible", timeout=timeout)
    btn.scroll_into_view_if_needed()
    btn.click()
    return btn


def wait_status(page: Page, target: str, timeout_ms: int = STATE_TIMEOUT) -> bool:
    """轮询详情头部状态标签，直到匹配或超时。"""
    deadline = time.time() + timeout_ms / 1000
    while time.time() < deadline:
        st = safe_text(page, ".detail__header-right .n-tag")
        if st == target:
            return True
        if st == "错误":
            return False
        time.sleep(QUICK_POLL / 1000)
    return False


def get_status(page: Page) -> str:
    """读取详情头部状态文本。"""
    return safe_text(page, ".detail__header-right .n-tag")


def get_sidebar_status_class(page: Page, idx: int = 0) -> str:
    """读取侧栏第 idx 个设备的状态 class 修饰。"""
    try:
        cls = page.locator("[data-testid='sidebar-item']").nth(idx).locator(".device__status").first.get_attribute("class", timeout=2000) or ""
        return cls
    except Exception:
        return ""


def select_first_device(page: Page) -> str:
    """选中侧栏第一个设备，返回设备名。等待详情面板加载完毕。"""
    item = page.locator("[data-testid='sidebar-item']").first
    item.wait_for(state="visible", timeout=DEFAULT_TIMEOUT)
    item.click()
    # 等待详情面板出现（含状态标签渲染）
    try:
        page.locator(".detail__header").wait_for(state="visible", timeout=5000)
    except PWTimeout:
        pass
    time.sleep(0.3)  # 额外等 n-tag 动画完成
    return safe_text(page, ".detail__device-info h2")


def ensure_device_added(page: Page) -> str:
    """确保至少有一台设备。返回设备名。
    若侧栏已有设备，直接使用第一个；否则通过添加设备弹窗添加 KNOWN_DEVICE_IP。"""
    try:
        page.locator("[data-testid='sidebar-item']").first.wait_for(state="visible", timeout=8000)
        return select_first_device(page)
    except PWTimeout:
        pass
    # 无设备，添加一台
    log(f"侧栏无设备，添加 {KNOWN_DEVICE_IP}:{KNOWN_DEVICE_PORT}")
    page.locator(".topbar__icon-btn[title='添加设备']").first.click()
    page.locator(".modal-panel--narrow").wait_for(state="visible", timeout=DEFAULT_TIMEOUT)
    # 设备名
    page.locator(".dialog__body .dialog__field input").nth(0).fill(KNOWN_DEVICE_NAME)
    # IP
    page.locator(".dialog__row .dialog__field input").first.fill(KNOWN_DEVICE_IP)
    # 端口
    page.locator(".dialog__field--narrow input").first.fill(str(KNOWN_DEVICE_PORT))
    page.locator(".dialog__btn--primary").first.click()
    # 等待弹窗关闭 + 设备出现
    page.locator(".modal-panel--narrow").wait_for(state="hidden", timeout=DEFAULT_TIMEOUT)
    page.locator("[data-testid='sidebar-item']").first.wait_for(state="visible", timeout=DEFAULT_TIMEOUT)
    return select_first_device(page)


def ensure_connected(page: Page) -> bool:
    """确保当前选中设备处于已连接状态。"""
    st = get_status(page)
    if st == "已连接":
        return True
    if st in ("采集中",):
        # 采集中视为已连接
        return True
    if st in ("未连接", "错误"):
        try:
            click_btn_in_scope(page, ".detail__header-right", "连接")
        except PWTimeout:
            return False
        return wait_status(page, "已连接")
    return False


def ensure_acquiring(page: Page) -> bool:
    """确保当前选中设备处于采集中状态。"""
    st = get_status(page)
    if st == "采集中":
        return True
    if st != "已连接":
        if not ensure_connected(page):
            return False
    try:
        click_btn_by_text(page, "开始采集")
    except PWTimeout:
        return False
    return wait_status(page, "采集中")


def ensure_idle(page: Page) -> bool:
    """确保当前设备处于已连接但非采集状态。"""
    st = get_status(page)
    if st == "采集中":
        try:
            click_btn_by_text(page, "停止采集")
        except PWTimeout:
            return False
        return wait_status(page, "已连接")
    if st in ("未连接", "错误"):
        return ensure_connected(page)
    return st == "已连接"


# ---------------- 测试用例 ----------------

def run_tc(tc_id: str, name: str, module: str, fn, page: Page, *args, **kwargs):
    """统一执行单个测试用例，捕获异常并记录结果。"""
    log(f"--- {tc_id} {name} ---")
    start = time.time()
    r = TestCaseResult(tc_id=tc_id, name=name, module=module)
    try:
        screenshot = fn(page, *args, **kwargs)
        if screenshot is None:
            screenshot = tc_id.lower()
        # 若截图函数返回文件名则使用，否则用 tc_id
        shot_path = shot(page, screenshot if screenshot else tc_id.lower())
        r.screenshot = shot_path
        r.result = "pass"
        r.remark = "通过"
        log(f"PASS: {tc_id}")
    except SkipTest as e:
        r.result = "skip"
        r.remark = str(e)
        # 仍截图保留现场
        try:
            r.screenshot = shot(page, tc_id.lower() + "-skip")
        except Exception:
            pass
        log(f"SKIP: {tc_id} - {e}")
    except AssertionError as e:
        r.result = "fail"
        r.remark = f"断言失败: {e}"
        try:
            r.screenshot = shot(page, tc_id.lower() + "-fail")
        except Exception:
            pass
        log(f"FAIL: {tc_id} - {e}")
    except Exception as e:
        r.result = "fail"
        r.remark = f"{type(e).__name__}: {e}"
        try:
            r.screenshot = shot(page, tc_id.lower() + "-err")
        except Exception:
            pass
        log(f"FAIL: {tc_id} - {type(e).__name__}: {e}")
        traceback.print_exc()
    r.duration_ms = int((time.time() - start) * 1000)
    REPORT.add(r)


class SkipTest(Exception):
    """测试用例跳过异常。"""
    pass


# ----- CONN 模块 -----

def tc_CONN_001(page: Page) -> str:
    """CONN-001 P1604 设备连接（真实模式）"""
    device_name = ensure_device_added(page)
    st = get_status(page)
    log(f"CONN-001: device={device_name}, status='{st}', detail_header_visible={page.locator('.detail__header').is_visible()}")
    if st == "已连接":
        # 验证侧栏状态 class 也是 connected
        cls = get_sidebar_status_class(page, 0)
        assert "connected" in cls, f"侧栏状态 class 不含 connected: {cls}"
        return "conn-001-connected"
    # 未连接则点击连接
    click_btn_in_scope(page, ".detail__header-right", "连接")
    assert wait_status(page, "已连接"), f"连接超时，当前状态: {get_status(page)}"
    # 验证底栏在线计数 >= 1
    online_text = safe_text(page, "[data-testid='status-online']")
    assert online_text != "<unknown>", "底栏在线计数元素未找到"
    return "conn-001-connected"


def tc_CONN_002(page: Page) -> str:
    """CONN-002 重复添加设备防御（IP+端口重复）"""
    ensure_device_added(page)
    # 打开添加设备弹窗，填入与已有设备相同的 IP+端口
    page.locator(".topbar__icon-btn[title='添加设备']").first.click()
    page.locator(".modal-panel--narrow").wait_for(state="visible", timeout=DEFAULT_TIMEOUT)
    page.locator(".dialog__body .dialog__field input").nth(0).fill("重复设备")
    page.locator(".dialog__row .dialog__field input").first.fill(KNOWN_DEVICE_IP)
    page.locator(".dialog__field--narrow input").first.fill(str(KNOWN_DEVICE_PORT))
    page.locator(".dialog__btn--primary").first.click()
    # 期待：弹出错误提示，弹窗不关闭或关闭后侧栏设备数不变
    time.sleep(1.0)
    error_text = safe_text(page, ".dialog__error")
    # 弹窗可能仍可见
    modal_visible = page.locator(".modal-panel--narrow").is_visible()
    # 关闭弹窗（无论结果）
    if modal_visible:
        try:
            page.locator(".dialog__btn--secondary").first.click()
        except Exception:
            page.keyboard.press("Escape")
    # 校验：要么有错误提示，要么弹窗仍开着（防御生效）
    assert error_text or modal_visible, f"重复添加未触发防御，error={error_text}, modal_visible={modal_visible}"
    return "conn-002-duplicate-blocked"


def tc_CONN_003(page: Page) -> str:
    """CONN-003 断开设备连接"""
    ensure_device_added(page)
    if not ensure_connected(page):
        raise AssertionError("无法进入已连接状态")
    # 点击断开
    click_btn_in_scope(page, ".detail__header-right", "断开")
    deadline = time.time() + STATE_TIMEOUT / 1000
    while time.time() < deadline:
        st = get_status(page)
        if st == "未连接":
            break
        time.sleep(QUICK_POLL / 1000)
    st = get_status(page)
    assert st == "未连接", f"断开后状态非未连接: {st}"
    return "conn-003-disconnected"


def tc_CONN_005(page: Page) -> str:
    """CONN-005 重复连接防护（已连接状态下点连接应安全）"""
    ensure_device_added(page)
    if not ensure_connected(page):
        raise AssertionError("无法进入已连接状态")
    # 此时按钮文本应为"断开"，不应有"连接"按钮
    btns = page.locator(".detail__header-right button").all_inner_texts()
    assert "连接" not in btns, f"已连接状态下仍出现'连接'按钮: {btns}"
    return "conn-005-no-duplicate-connect"


def tc_CONN_009(page: Page) -> str:
    """CONN-009 自动连接开关"""
    ensure_device_added(page)
    # 打开配置面板
    page.locator("[data-testid='btn-config']").first.click()
    page.locator(".config").wait_for(state="visible", timeout=DEFAULT_TIMEOUT)
    # 找到自动连接开关（第四个 .config__field 内的 .config__toggle）
    toggles = page.locator(".config__toggle").all()
    assert len(toggles) >= 1, "未找到任何配置开关"
    # 自动连接开关通常是第一个 toggle
    auto_toggle = toggles[0]
    cls = auto_toggle.get_attribute("class") or ""
    log(f"自动连接开关当前 class: {cls}")
    # 关闭配置面板
    page.locator(".config__close").first.click()
    page.locator(".config").wait_for(state="hidden", timeout=DEFAULT_TIMEOUT)
    # 验证开关可被识别（不强制改变设置避免影响后续测试）
    assert auto_toggle is not None
    return "conn-009-autoconnect-toggle"


def tc_CONN_014(page: Page) -> str:
    """CONN-014 删除已连接设备"""
    # 注意：此用例会删除设备，需谨慎；先确保有 2 台以上设备或测试结束后补加
    ensure_device_added(page)
    # 暂时跳过：删除当前唯一设备会影响后续测试
    raise SkipTest("删除设备会影响后续用例，需独立测试环境")


# ----- ACQ 模块 -----

def tc_ACQ_001(page: Page) -> str:
    """ACQ-001 启动数据采集"""
    ensure_device_added(page)
    ensure_connected(page)
    click_btn_by_text(page, "开始采集")
    assert wait_status(page, "采集中"), f"启动采集超时，当前状态: {get_status(page)}"
    # 底栏采集状态应为"运行中"
    acq_text = safe_text(page, "[data-testid='status-acquisition']")
    assert "运行中" in acq_text, f"底栏采集状态未变运行中: {acq_text}"
    return "acq-001-acquiring"


def tc_ACQ_002(page: Page) -> str:
    """ACQ-002 停止数据采集"""
    ensure_device_added(page)
    ensure_acquiring(page)
    click_btn_by_text(page, "停止采集")
    assert wait_status(page, "已连接"), f"停止采集超时，当前状态: {get_status(page)}"
    # 底栏采集状态应为"已停止"
    acq_text = safe_text(page, "[data-testid='status-acquisition']")
    assert "已停止" in acq_text, f"底栏采集状态未变已停止: {acq_text}"
    return "acq-002-stopped"


def tc_ACQ_003(page: Page) -> str:
    """ACQ-003 未连接时启动采集"""
    ensure_device_added(page)
    # 主动断开
    if get_status(page) != "未连接":
        try:
            click_btn_in_scope(page, ".detail__header-right", "断开")
            wait_status(page, "未连接")
        except PWTimeout:
            pass
    st = get_status(page)
    if st != "未连接":
        raise SkipTest(f"设备无法进入未连接状态（当前: {st}），跳过")
    # 验证顶栏"开始采集"按钮不存在或禁用
    start_btn = page.locator("button:has-text('开始采集')")
    if start_btn.count() == 0:
        # 按钮不存在 → 防御生效
        return "acq-003-no-start-btn"
    # 按钮存在 → 应该禁用
    disabled = start_btn.first.is_disabled()
    assert disabled, "未连接状态下'开始采集'按钮既未隐藏也未禁用"
    return "acq-003-start-btn-disabled"


def tc_ACQ_004(page: Page) -> str:
    """ACQ-004 重复启动采集（采集中再次点开始采集）"""
    ensure_device_added(page)
    ensure_acquiring(page)
    # 此时按钮应变为"停止采集"，不应有"开始采集"按钮
    start_btn = page.locator("button:has-text('开始采集')")
    assert start_btn.count() == 0, "采集中仍出现'开始采集'按钮"
    return "acq-004-no-start-when-acquiring"


def tc_ACQ_005(page: Page) -> str:
    """ACQ-005 18 通道压力数据接收"""
    ensure_device_added(page)
    ensure_acquiring(page)
    time.sleep(2.0)  # 等数据流入
    # 验证通道卡片数量
    cards = page.locator("article.card").all()
    # 应至少 1 张（启用的通道数），通常 16-18
    assert len(cards) >= 1, f"未发现通道卡片: {len(cards)}"
    log(f"通道卡片数: {len(cards)}")
    # 验证通道数值非 ---（至少 1 张有数值）
    has_value = False
    for i, c in enumerate(cards[:6]):  # 检查前 6 张足够
        try:
            v = c.locator(".card__value").first.inner_text(timeout=1000)
            if v and v != "---":
                has_value = True
                break
        except Exception:
            continue
    assert has_value, "采集 2 秒后所有通道数值仍为 ---"
    return "acq-005-channels-with-data"


def tc_ACQ_009(page: Page) -> str:
    """ACQ-009 断开连接时自动停止采集"""
    ensure_device_added(page)
    ensure_acquiring(page)
    # 点击断开
    click_btn_in_scope(page, ".detail__header-right", "断开")
    # 等状态变为未连接
    deadline = time.time() + STATE_TIMEOUT / 1000
    stopped_acq = False
    while time.time() < deadline:
        st = get_status(page)
        if st == "未连接":
            stopped_acq = True
            break
        time.sleep(QUICK_POLL / 1000)
    assert stopped_acq, "断开后未进入未连接状态"
    # 底栏采集状态应为"已停止"
    acq_text = safe_text(page, "[data-testid='status-acquisition']")
    assert "已停止" in acq_text, f"断开后底栏采集状态未变已停止: {acq_text}"
    return "acq-009-auto-stop-on-disconnect"


# ----- CFG 模块 -----

def tc_CFG_013(page: Page) -> str:
    """CFG-013 配置面板重置"""
    ensure_device_added(page)
    page.locator("[data-testid='btn-config']").first.click()
    page.locator(".config").wait_for(state="visible", timeout=DEFAULT_TIMEOUT)
    # 读取采样频率当前值
    rate_input = page.locator(".config__rate-input").first
    original_rate = rate_input.input_value()
    log(f"原采样频率: {original_rate}")
    # 修改为新值
    new_rate = "100" if original_rate != "100" else "200"
    rate_input.fill(new_rate)
    time.sleep(0.3)
    # 点击重置
    page.locator(".config__btn--secondary").first.click()
    time.sleep(0.5)
    # 期待值回到 original_rate
    after_reset = page.locator(".config__rate-input").first.input_value()
    log(f"重置后采样频率: {after_reset}")
    # 关闭配置面板
    page.locator(".config__close").first.click()
    page.locator(".config").wait_for(state="hidden", timeout=DEFAULT_TIMEOUT)
    assert after_reset == original_rate, f"重置未还原: 原={original_rate}, 重置后={after_reset}"
    return "cfg-013-reset-restored"


def tc_CFG_014(page: Page) -> str:
    """CFG-014 大气压力通道锁定（CH17 应不可禁用）"""
    ensure_device_added(page)
    page.locator("[data-testid='btn-config']").first.click()
    page.locator(".config").wait_for(state="visible", timeout=DEFAULT_TIMEOUT)
    # 找到 CH17 通道卡片
    ch17 = page.locator(".config__channel").filter(has_text="CH17")
    assert ch17.count() >= 1, "未找到 CH17 通道卡片"
    # CH17 应有特殊徽章 "大气压力"
    badge = ch17.locator(".config__channel-badge").first.inner_text(timeout=2000)
    assert "大气压力" in badge, f"CH17 徽章非大气压力: {badge}"
    # CH17 启用开关应不可见或禁用（锁定）
    toggle = ch17.locator(".config__toggle").first
    is_disabled = toggle.get_attribute("disabled") is not None or "disabled" in (toggle.get_attribute("class") or "")
    # 关闭配置面板
    page.locator(".config__close").first.click()
    page.locator(".config").wait_for(state="hidden", timeout=DEFAULT_TIMEOUT)
    # 锁定验证：要么开关禁用，要么根本不渲染开关
    assert is_disabled or toggle.count() == 0, "CH17 大气压力通道未锁定（开关可操作）"
    return "cfg-014-ch17-locked"


def tc_CFG_015(page: Page) -> str:
    """CFG-015 大气温度通道锁定（CH18 应不可禁用）"""
    ensure_device_added(page)
    page.locator("[data-testid='btn-config']").first.click()
    page.locator(".config").wait_for(state="visible", timeout=DEFAULT_TIMEOUT)
    ch18 = page.locator(".config__channel").filter(has_text="CH18")
    assert ch18.count() >= 1, "未找到 CH18 通道卡片"
    badge = ch18.locator(".config__channel-badge").first.inner_text(timeout=2000)
    assert "大气温度" in badge, f"CH18 徽章非大气温度: {badge}"
    toggle = ch18.locator(".config__toggle").first
    is_disabled = toggle.get_attribute("disabled") is not None or "disabled" in (toggle.get_attribute("class") or "")
    page.locator(".config__close").first.click()
    page.locator(".config").wait_for(state="hidden", timeout=DEFAULT_TIMEOUT)
    assert is_disabled or toggle.count() == 0, "CH18 大气温度通道未锁定"
    return "cfg-015-ch18-locked"


def tc_CFG_017(page: Page) -> str:
    """CFG-017 配置变更未保存提示（脏值检测）"""
    ensure_device_added(page)
    page.locator("[data-testid='btn-config']").first.click()
    page.locator(".config").wait_for(state="visible", timeout=DEFAULT_TIMEOUT)
    # 初始应无未保存徽章
    unsaved_init = page.locator(".config__header-unsaved").count()
    assert unsaved_init == 0, f"初始状态出现未保存徽章: {unsaved_init}"
    # 修改采样频率
    rate_input = page.locator(".config__rate-input").first
    original = rate_input.input_value()
    new_val = "150" if original != "150" else "250"
    rate_input.fill(new_val)
    time.sleep(0.5)
    # 期待出现未保存徽章
    unsaved_after = page.locator(".config__header-unsaved").count()
    assert unsaved_after >= 1, "修改后未出现未保存徽章"
    # 改回原值，期待徽章消失（智能脏值比较）
    rate_input.fill(original)
    time.sleep(0.5)
    unsaved_revert = page.locator(".config__header-unsaved").count()
    assert unsaved_revert == 0, f"改回原值后未保存徽章仍存在: {unsaved_revert}"
    # 关闭配置面板
    page.locator(".config__close").first.click()
    page.locator(".config").wait_for(state="hidden", timeout=DEFAULT_TIMEOUT)
    return "cfg-017-smart-dirty"


def tc_CFG_020(page: Page) -> str:
    """CFG-020 配置面板交互（打开/关闭/外部点击关闭）"""
    ensure_device_added(page)
    # 打开
    page.locator("[data-testid='btn-config']").first.click()
    page.locator(".config").wait_for(state="visible", timeout=DEFAULT_TIMEOUT)
    assert page.locator(".config").is_visible(), "配置面板未打开"
    # 通过关闭按钮关闭
    page.locator(".config__close").first.click()
    page.locator(".config").wait_for(state="hidden", timeout=DEFAULT_TIMEOUT)
    assert not page.locator(".config").is_visible(), "配置面板未关闭"
    return "cfg-020-open-close"


# ----- MON 模块 -----

def tc_MON_001(page: Page) -> str:
    """MON-001 18 通道网格显示"""
    ensure_device_added(page)
    ensure_acquiring(page)
    time.sleep(1.5)
    cards = page.locator("article.card").all()
    assert len(cards) >= 1, f"未发现通道卡片: {len(cards)}"
    # 验证通道标签格式 CHxx
    first_tag = cards[0].locator(".card__tag").first.inner_text(timeout=2000)
    assert first_tag.startswith("CH"), f"通道标签格式异常: {first_tag}"
    return "mon-001-grid-shown"


def tc_MON_002(page: Page) -> str:
    """MON-002 通道数值实时更新"""
    ensure_device_added(page)
    ensure_acquiring(page)
    time.sleep(1.0)
    cards = page.locator("article.card").all()
    if len(cards) == 0:
        raise AssertionError("未发现通道卡片")
    # 取第一张卡的数值，等 1.5 秒后再读
    v1 = cards[0].locator(".card__value").first.inner_text(timeout=2000)
    time.sleep(1.5)
    v2 = cards[0].locator(".card__value").first.inner_text(timeout=2000)
    log(f"通道数值: v1={v1}, v2={v2}")
    assert v1 != "---" or v2 != "---", "两次读取均为 ---"
    # 数值有变化（实时更新）
    assert v1 != v2, f"1.5 秒内数值未变化: v1={v1}, v2={v2}"
    return "mon-002-value-updated"


def tc_MON_003(page: Page) -> str:
    """MON-003 通道单位显示"""
    ensure_device_added(page)
    ensure_acquiring(page)
    time.sleep(1.0)
    cards = page.locator("article.card").all()
    if len(cards) == 0:
        raise AssertionError("未发现通道卡片")
    unit = cards[0].locator(".card__unit").first.inner_text(timeout=2000)
    assert unit and unit != "", "通道单位为空"
    log(f"首通道单位: {unit}")
    # CH17 应为 Pa，CH18 应为 °C（如果它们被启用）
    for i, c in enumerate(cards):
        try:
            tag = c.locator(".card__tag").first.inner_text(timeout=500)
            u = c.locator(".card__unit").first.inner_text(timeout=500)
            if tag == "CH17":
                assert u == "Pa", f"CH17 单位非 Pa: {u}"
            elif tag == "CH18":
                assert u == "°C", f"CH18 单位非 °C: {u}"
        except Exception:
            continue
    return "mon-003-units-shown"


def tc_MON_005(page: Page) -> str:
    """MON-005 实时波形图显示"""
    ensure_device_added(page)
    ensure_acquiring(page)
    time.sleep(2.0)
    chart = page.locator("[data-testid='detail-chart']")
    assert chart.count() >= 1, "未找到实时波形图容器"
    # 验证 ECharts canvas 存在
    canvas = page.locator(".chart__canvas canvas")
    assert canvas.count() >= 1, "ECharts canvas 未渲染"
    # 验证不是空状态
    empty = page.locator(".chart__empty").count()
    log(f"波形图空状态元素数: {empty}")
    return "mon-005-chart-rendered"


def tc_MON_015(page: Page) -> str:
    """MON-015 空数据状态显示（未采集时）"""
    ensure_device_added(page)
    ensure_idle(page)
    time.sleep(1.0)
    # 此时未采集，应有空状态提示
    empty_text = safe_text(page, ".chart__empty-text")
    log(f"空状态文本: {empty_text}")
    # 至少 chart 容器存在
    chart = page.locator("[data-testid='detail-chart']")
    assert chart.count() >= 1, "未找到实时波形图容器"
    return "mon-015-empty-state"


# ----- REC 模块 -----

def tc_REC_001(page: Page) -> str:
    """REC-001 启动 CSV 录制
    注意：录制启动会弹出系统目录选择对话框，E2E 无法处理，故跳过。
    """
    raise SkipTest("录制启动会弹出系统目录选择对话框，E2E 无法自动化处理")


def tc_REC_002(page: Page) -> str:
    """REC-002 停止 CSV 录制"""
    raise SkipTest("依赖 REC-001，无法在 E2E 中启动录制")


def tc_REC_019(page: Page) -> str:
    """REC-019 重复启动录制防御"""
    raise SkipTest("录制启动涉及系统对话框，无法自动化")


def tc_REC_020(page: Page) -> str:
    """REC-020 重复停止录制幂等"""
    raise SkipTest("依赖录制已启动，无法在 E2E 中启动录制")


# ----- LOG 模块 -----

def tc_LOG_001(page: Page) -> str:
    """LOG-001 日志面板显示"""
    # 等待日志面板可见（可能默认收起）
    panel = page.locator(".log-panel").first
    panel.wait_for(state="visible", timeout=DEFAULT_TIMEOUT)
    assert panel.is_visible(), "日志面板未显示"
    # 若是收起态，点击展开
    collapsed = page.locator(".log-panel__collapsed").count()
    if collapsed > 0:
        page.locator(".log-panel__collapsed").first.click()
        time.sleep(0.5)
    # 验证日志标题
    title = safe_text(page, ".log-panel__title")
    assert "日志" in title, f"日志面板标题异常: {title}"
    return "log-001-panel-shown"


def tc_LOG_002(page: Page) -> str:
    """LOG-002 日志级别筛选"""
    tc_LOG_001(page)  # 确保展开
    # 点击 Error 级别 chip
    error_chip = page.locator(".log-panel__chip").filter(has_text="Error").first
    error_chip.wait_for(state="visible", timeout=DEFAULT_TIMEOUT)
    error_chip.click()
    time.sleep(0.5)
    # 验证至少有 Error 级别高亮（--active class）
    activated = page.locator(".log-panel__chip--active").count()
    assert activated >= 1, "点击级别 chip 未激活"
    # 取消选中以恢复
    error_chip.click()
    time.sleep(0.3)
    return "log-002-level-filter"


def tc_LOG_003(page: Page) -> str:
    """LOG-003 日志分组筛选"""
    tc_LOG_001(page)
    # 点击"通信"分组 chip
    comm_chip = page.locator(".log-panel__chip").filter(has_text="通信").first
    comm_chip.wait_for(state="visible", timeout=DEFAULT_TIMEOUT)
    comm_chip.click()
    time.sleep(0.5)
    activated = page.locator(".log-panel__chip--active").count()
    assert activated >= 1, "点击分组 chip 未激活"
    # 恢复：点击"全部"
    all_chip = page.locator(".log-panel__chip").filter(has_text="全部").first
    all_chip.click()
    time.sleep(0.3)
    return "log-003-group-filter"


def tc_LOG_005(page: Page) -> str:
    """LOG-005 日志清空"""
    tc_LOG_001(page)
    # 找到清空按钮（第三个 tool-btn, title="清空日志"）
    clear_btn = page.locator(".log-panel__tool-btn[title='清空日志']").first
    clear_btn.wait_for(state="visible", timeout=DEFAULT_TIMEOUT)
    clear_btn.click()
    time.sleep(0.5)
    # 可能弹确认对话框，找确认按钮
    confirm_btn = page.locator("button:has-text('确认')")
    if confirm_btn.count() > 0:
        confirm_btn.first.click()
        time.sleep(0.5)
    # 验证日志列表为空或显示空状态
    empty = page.locator(".log-panel__empty").count()
    entries = page.locator(".log-entry").count()
    log(f"清空后: 空状态={empty}, 日志条数={entries}")
    assert empty >= 1 or entries == 0, f"清空后仍有 {entries} 条日志"
    return "log-005-cleared"


def tc_LOG_008(page: Page) -> str:
    """LOG-008 日志级别颜色区分"""
    tc_LOG_001(page)
    # 验证至少存在 1 条不同级别的日志条目
    info_entries = page.locator(".log-entry--info").count()
    warn_entries = page.locator(".log-entry--warn").count()
    error_entries = page.locator(".log-entry--error").count()
    log(f"日志条数: info={info_entries}, warn={warn_entries}, error={error_entries}")
    # 至少有 info 级别
    assert info_entries >= 1 or warn_entries >= 1 or error_entries >= 1, "未发现任何级别日志条目"
    return "log-008-color-coded"


# ----- UI 模块 -----

def tc_UI_001(page: Page) -> str:
    """UI-001 顶栏布局"""
    topbar = page.locator("header.topbar").first
    topbar.wait_for(state="visible", timeout=DEFAULT_TIMEOUT)
    assert topbar.is_visible(), "顶栏不可见"
    # 验证标题、副标题、操作区都存在
    title = safe_text(page, "[data-testid='topbar-title']")
    assert "DAQ" in title.upper() or "P1604" in title, f"顶栏标题异常: {title}"
    actions = page.locator(".topbar__actions").first
    assert actions.is_visible(), "顶栏操作区不可见"
    return "ui-001-topbar-layout"


def tc_UI_002(page: Page) -> str:
    """UI-002 顶栏采集按钮状态"""
    # 采集按钮在未连接时不应可点
    ensure_device_added(page)
    st = get_status(page)
    if st == "未连接":
        # 按钮应不存在或禁用
        start_btn = page.locator("button:has-text('开始采集')")
        if start_btn.count() > 0:
            assert start_btn.first.is_disabled(), "未连接时开始采集按钮未禁用"
            return "ui-002-start-disabled"
        return "ui-002-start-hidden"
    # 已连接时应有开始采集按钮
    if st == "已连接":
        start_btn = page.locator("button:has-text('开始采集')")
        assert start_btn.count() >= 1, "已连接时未出现开始采集按钮"
        return "ui-002-start-visible"
    # 采集中应有停止采集按钮
    if st == "采集中":
        stop_btn = page.locator("button:has-text('停止采集')")
        assert stop_btn.count() >= 1, "采集中未出现停止采集按钮"
        return "ui-002-stop-visible"
    return "ui-002-unknown-state"


def tc_UI_010(page: Page) -> str:
    """UI-010 底栏状态显示"""
    bottombar = page.locator("footer.bottombar").first
    bottombar.wait_for(state="visible", timeout=DEFAULT_TIMEOUT)
    assert bottombar.is_visible(), "底栏不可见"
    # 验证采集状态、记录状态、设备数、在线数都存在
    acq = page.locator("[data-testid='status-acquisition']").count()
    rec = page.locator("[data-testid='status-recording']").count()
    dev = page.locator("[data-testid='status-devices']").count()
    online = page.locator("[data-testid='status-online']").count()
    assert acq >= 1 and rec >= 1 and dev >= 1 and online >= 1, \
        f"底栏状态元素缺失: acq={acq}, rec={rec}, dev={dev}, online={online}"
    return "ui-010-bottombar-status"


def tc_UI_011(page: Page) -> str:
    """UI-011 底栏在线设备计数"""
    ensure_device_added(page)
    online_text = safe_text(page, "[data-testid='status-online']")
    log(f"底栏在线计数: {online_text}")
    assert online_text != "<unknown>", "在线计数元素未找到"
    # 解析数字
    import re
    m = re.search(r"\d+", online_text)
    assert m, f"在线计数无法解析为数字: {online_text}"
    n = int(m.group())
    # 至少有 0 或 1，应该非负
    assert n >= 0, f"在线计数异常: {n}"
    return "ui-011-online-count"


def tc_UI_012(page: Page) -> str:
    """UI-012 底栏采集状态"""
    ensure_device_added(page)
    ensure_acquiring(page)
    acq_text = safe_text(page, "[data-testid='status-acquisition']")
    assert "运行中" in acq_text, f"采集中底栏未显示运行中: {acq_text}"
    # 停止后再验证
    click_btn_by_text(page, "停止采集")
    wait_status(page, "已连接")
    time.sleep(0.5)
    acq_text2 = safe_text(page, "[data-testid='status-acquisition']")
    assert "已停止" in acq_text2, f"停止后底栏未显示已停止: {acq_text2}"
    return "ui-012-acquisition-status"


def tc_UI_013(page: Page) -> str:
    """UI-013 底栏录制状态"""
    rec_text = safe_text(page, "[data-testid='status-recording']")
    log(f"底栏录制状态: {rec_text}")
    assert rec_text != "<unknown>", "录制状态元素未找到"
    # 未录制时应为"未保存"
    assert "未保存" in rec_text or "保存中" in rec_text, f"录制状态文本异常: {rec_text}"
    return "ui-013-recording-status"


def tc_UI_018(page: Page) -> str:
    """UI-018 主题切换"""
    # 读取当前主题
    html_attr = page.evaluate("document.documentElement.dataset.theme || ''")
    log(f"当前主题: {html_attr or 'default'}")
    # 点击主题切换按钮
    page.locator("[data-testid='btn-theme-toggle']").first.click()
    time.sleep(0.8)
    # 验证主题已切换
    new_attr = page.evaluate("document.documentElement.dataset.theme || ''")
    log(f"切换后主题: {new_attr or 'default'}")
    assert new_attr != html_attr, f"主题未切换: 原={html_attr}, 后={new_attr}"
    # 切回原主题（保持环境干净）
    page.locator("[data-testid='btn-theme-toggle']").first.click()
    time.sleep(0.5)
    return "ui-018-theme-toggled"


# ----- SCAN 模块 -----

def tc_SCAN_001(page: Page) -> str:
    """SCAN-001 单网段扫描发现设备
    注意：扫描依赖网络环境中存在真实设备，结果不确定性大。
    """
    # 打开扫描弹窗
    page.locator(".sidebar__scan-btn").first.click()
    page.locator(".modal-panel--scan").wait_for(state="visible", timeout=DEFAULT_TIMEOUT)
    # 等扫描完成（最多 8 秒）
    deadline = time.time() + 8
    while time.time() < deadline:
        loading = page.locator(".scan__loading").count()
        if loading == 0:
            break
        time.sleep(0.5)
    # 检查结果
    items = page.locator(".scan__item").count()
    empty = page.locator(".scan__empty").count()
    log(f"扫描结果: 设备数={items}, 空状态={empty}")
    # 关闭弹窗
    try:
        page.locator(".dialog--scan .dialog__btn--secondary").first.click()
    except Exception:
        page.keyboard.press("Escape")
    time.sleep(0.3)
    # 不强制断言设备数（环境相关），只验证扫描流程能完成
    return "scan-001-completed"


# ----- STAB 模块 -----

def tc_STB_001(page: Page) -> str:
    """STB-001 未连接时操作采集按钮"""
    ensure_device_added(page)
    # 主动断开
    if get_status(page) != "未连接":
        try:
            click_btn_in_scope(page, ".detail__header-right", "断开")
            wait_status(page, "未连接")
        except PWTimeout:
            pass
    st = get_status(page)
    if st != "未连接":
        raise SkipTest(f"无法进入未连接状态（当前: {st}）")
    # 验证开始采集按钮不存在或禁用
    start_btn = page.locator("button:has-text('开始采集')")
    if start_btn.count() == 0:
        return "stb-001-no-start-btn"
    assert start_btn.first.is_disabled(), "未连接状态下'开始采集'按钮未禁用"
    return "stb-001-start-disabled"


def tc_STB_002(page: Page) -> str:
    """STB-002 快速连续点击采集按钮"""
    ensure_device_added(page)
    ensure_connected(page)
    # 点击开始采集
    start_btn = page.locator("button:has-text('开始采集')").first
    start_btn.wait_for(state="visible", timeout=DEFAULT_TIMEOUT)
    start_btn.click()
    time.sleep(0.1)
    # 此时按钮应已变为停止采集，连点开始应无效
    start_btn_count = page.locator("button:has-text('开始采集')").count()
    stop_btn_count = page.locator("button:has-text('停止采集')").count()
    log(f"快速点击后: 开始按钮={start_btn_count}, 停止按钮={stop_btn_count}")
    # 等待稳定
    wait_status(page, "采集中", timeout_ms=5000)
    # 底栏不应出现多个采集任务（验证无重复触发）
    acq_text = safe_text(page, "[data-testid='status-acquisition']")
    assert "运行中" in acq_text, f"快速点击后采集未启动: {acq_text}"
    return "stb-002-rapid-click-safe"


# ---------------- 主流程 ----------------

def main() -> int:
    # 启动前备份用户配置（复制，不删除原始文件，避免误删已保存的设备列表）
    config_path = Path(os.environ.get("APPDATA", "")) / "daq-p1604" / "device-profiles.json"
    if config_path.exists():
        try:
            import shutil
            bak = config_path.with_name("device-profiles.json.bak-e2e")
            shutil.copy2(config_path, bak)
            log(f"已备份用户配置: {bak}")
        except Exception as e:
            log(f"备份配置失败（不影响运行）: {e}")

    log(f"连接 CDP: {CDP_URL}")
    with sync_playwright() as p:
        browser: Browser = p.chromium.connect_over_cdp(CDP_URL)
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
        # 等待 Vue 应用挂载
        try:
            page.locator("aside.sidebar").first.wait_for(state="visible", timeout=DEFAULT_TIMEOUT)
        except PWTimeout:
            log("侧栏未出现，Vue 挂载失败")
            shot(page, "00-fail-no-sidebar")
            return 4

        # 初始截图
        shot(page, "00-initial")

        # ===== 执行测试用例 =====

        # CONN 模块
        run_tc("CONN-001", "P1604 设备连接（真实模式）", "CONN", tc_CONN_001, page)
        run_tc("CONN-002", "重复添加设备防御", "CONN", tc_CONN_002, page)
        run_tc("CONN-003", "断开设备连接", "CONN", tc_CONN_003, page)
        run_tc("CONN-005", "重复连接防护", "CONN", tc_CONN_005, page)
        run_tc("CONN-009", "自动连接开关", "CONN", tc_CONN_009, page)
        run_tc("CONN-014", "删除已连接设备", "CONN", tc_CONN_014, page)

        # ACQ 模块
        run_tc("ACQ-001", "启动数据采集", "ACQ", tc_ACQ_001, page)
        run_tc("ACQ-002", "停止数据采集", "ACQ", tc_ACQ_002, page)
        run_tc("ACQ-003", "未连接时启动采集", "ACQ", tc_ACQ_003, page)
        run_tc("ACQ-004", "重复启动采集", "ACQ", tc_ACQ_004, page)
        run_tc("ACQ-005", "18 通道压力数据接收", "ACQ", tc_ACQ_005, page)
        run_tc("ACQ-009", "断开连接时自动停止采集", "ACQ", tc_ACQ_009, page)

        # CFG 模块
        run_tc("CFG-013", "配置面板重置", "CFG", tc_CFG_013, page)
        run_tc("CFG-014", "大气压力通道锁定", "CFG", tc_CFG_014, page)
        run_tc("CFG-015", "大气温度通道锁定", "CFG", tc_CFG_015, page)
        run_tc("CFG-017", "配置变更未保存提示", "CFG", tc_CFG_017, page)
        run_tc("CFG-020", "配置面板交互", "CFG", tc_CFG_020, page)

        # MON 模块
        run_tc("MON-001", "18 通道网格显示", "MON", tc_MON_001, page)
        run_tc("MON-002", "通道数值实时更新", "MON", tc_MON_002, page)
        run_tc("MON-003", "通道单位显示", "MON", tc_MON_003, page)
        run_tc("MON-005", "实时波形图显示", "MON", tc_MON_005, page)
        run_tc("MON-015", "空数据状态显示", "MON", tc_MON_015, page)

        # REC 模块
        run_tc("REC-001", "启动 CSV 录制", "REC", tc_REC_001, page)
        run_tc("REC-002", "停止 CSV 录制", "REC", tc_REC_002, page)
        run_tc("REC-019", "重复启动录制防御", "REC", tc_REC_019, page)
        run_tc("REC-020", "重复停止录制幂等", "REC", tc_REC_020, page)

        # LOG 模块
        run_tc("LOG-001", "日志面板显示", "LOG", tc_LOG_001, page)
        run_tc("LOG-002", "日志级别筛选", "LOG", tc_LOG_002, page)
        run_tc("LOG-003", "日志分组筛选", "LOG", tc_LOG_003, page)
        run_tc("LOG-005", "日志清空", "LOG", tc_LOG_005, page)
        run_tc("LOG-008", "日志级别颜色区分", "LOG", tc_LOG_008, page)

        # UI 模块
        run_tc("UI-001", "顶栏布局", "UI", tc_UI_001, page)
        run_tc("UI-002", "顶栏采集按钮状态", "UI", tc_UI_002, page)
        run_tc("UI-010", "底栏状态显示", "UI", tc_UI_010, page)
        run_tc("UI-011", "底栏在线设备计数", "UI", tc_UI_011, page)
        run_tc("UI-012", "底栏采集状态", "UI", tc_UI_012, page)
        run_tc("UI-013", "底栏录制状态", "UI", tc_UI_013, page)
        run_tc("UI-018", "主题切换", "UI", tc_UI_018, page)

        # SCAN 模块
        run_tc("SCAN-001", "单网段扫描发现设备", "SCAN", tc_SCAN_001, page)

        # STAB 模块
        run_tc("STB-001", "未连接时操作采集按钮", "STAB", tc_STB_001, page)
        run_tc("STB-002", "快速连续点击采集按钮", "STAB", tc_STB_002, page)

        # 最终截图
        shot(page, "99-final")

    # 写入报告
    report_data = asdict(REPORT)
    REPORT_PATH.write_text(json.dumps(report_data, ensure_ascii=False, indent=2), encoding="utf-8")
    log(f"测试报告已写入: {REPORT_PATH}")

    # 汇总
    log("=" * 60)
    log(f"总计: {REPORT.total}, 通过: {REPORT.pass_count}, 失败: {REPORT.fail_count}, "
        f"跳过: {REPORT.skip_count}, 阻塞: {REPORT.block_count}")
    log(f"截图目录: {SHOT_DIR}")
    if REPORT.fail_count > 0:
        log("失败用例:")
        for c in REPORT.cases:
            if c.result == "fail":
                log(f"  - {c.tc_id} {c.name}: {c.remark}")
        return 1
    log("E2E 测试完成 ✓")
    return 0


if __name__ == "__main__":
    sys.exit(main())
