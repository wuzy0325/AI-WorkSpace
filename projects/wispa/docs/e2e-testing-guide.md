# E2E 测试指南：Wails v3 + WebView2 + Playwright CDP

> **适用范围**：所有使用 Wails v3 + WebView2 渲染的桌面应用（daq-p1604 / daq-t1603 / wind-daq / motion-controller 等）。本文以 `daq-p1604` 为具体示例，方法论与代码模板可直接复用。
>
> **目标读者**：AI Agent（首选）与人类测试人员。文档假设读者已熟悉 Go / Vue 3 / Wails 基本概念。

---

## 0. 为什么需要这份指南

Wails v3 桌面应用的渲染层是 WebView2（Edge Chromium 内核），不是普通的浏览器页面。直接用 Playwright `launch()` 启动一个 Chromium 实例无法访问 Wails 注入的 `window.go` / `window.runtime` 等 binding，也无法触达真实设备的硬件链路。

**结论**：必须让 Playwright **连接到正在运行的 WebView2 实例**，而不是新开浏览器。这通过 Chrome DevTools Protocol (CDP) 实现。

---

## 1. 方案选型对比

| 方案 | 可行性 | 优点 | 缺点 | 结论 |
|---|---|---|---|---|
| A. Playwright `launch()` 起新 Chromium | 不可行 | 简单 | 无法访问 Wails binding、无设备链路 | 放弃 |
| B. Playwright `connect_over_cdp` 连 WebView2 | **可行** | 真实链路、可截图、可断言 | 需向 WebView2 注入 CDP 端口 | **采用** |
| C. Windows UI Automation (UIA) | 可行 | 不依赖 CDP | 选择器脆弱、无截图、AI 不友好 | 不采用 |
| D. 手工测试 | 可行 | 零成本 | 不可复现、不可自动化 | 仅作补充 |

**采用方案 B**：Python + Playwright `sync_playwright` + `connect_over_cdp`。

---

## 2. 关键改造：向 WebView2 注入 CDP 调试端口

### 2.1 为什么不能直接用环境变量

go-webview2 v1.0.22 在 `webviewloader/env_create.go` 中**显式清空** `WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS` 环境变量。所以通过环境变量传 `--remote-debugging-port` 是无效的。

### 2.2 正确做法：Wails v3 的 `WindowsOptions.AdditionalBrowserArgs`

Wails v3 在 `application.WindowsOptions` 下暴露了 `AdditionalBrowserArgs []string` 字段，可直接传入 Chromium 启动参数。

> **字段位置陷阱**：`AdditionalBrowserArgs` 在 `WindowsOptions` 下，**不是** `Options` 顶层。`go doc application.Options` 看不到这个字段，必须 `appOpts.Windows.AdditionalBrowserArgs`。

### 2.3 改造代码（以 `apps/desktop-wails/main.go` 为例）

```go
// E2E 测试专用：通过环境变量 DAQ_P1604_CDP_PORT 注入 WebView2 CDP 调试端口。
// 设为空时（生产路径）不影响任何行为；设为端口号时（如 9222），
// Playwright 可通过 connect_over_cdp("http://localhost:9222") 连接 WebView2
// 进行真实 E2E 测试。
appOpts := application.Options{
    Name:        "DAQ-P-1604",
    // ... 其他字段
}
if cdpPort := os.Getenv("DAQ_P1604_CDP_PORT"); cdpPort != "" {
    slog.Info("CDP debugging enabled (E2E test mode)", "port", cdpPort)
    // 字段挂在 WindowsOptions 下（非 Options 顶层）：
    // 参见 wails/v3/pkg/application/application_options.go 中 WindowsOptions.AdditionalBrowserArgs
    appOpts.Windows.AdditionalBrowserArgs = []string{"--remote-debugging-port=" + cdpPort}
}
wailsApp := application.New(appOpts)
```

### 2.4 环境变量命名约定

每个项目使用独立的环境变量名，避免端口冲突：

| 项目 | 环境变量 | 推荐端口 |
|---|---|---|
| daq-p1604 | `DAQ_P1604_CDP_PORT` | 9222 |
| daq-t1603 | `DAQ_T1603_CDP_PORT` | 9223 |
| wind-daq | `WIND_DAQ_CDP_PORT` | 9224 |
| motion-controller | `MOTION_CDP_PORT` | 9225 |

### 2.5 生产安全性

- 环境变量为空时，`AdditionalBrowserArgs` 不设置，**零影响**生产路径。
- 发布构建**不设**该环境变量即可。
- 代码中通过 `slog.Info` 显式记录 CDP 启用，便于审计。

---

## 3. 环境准备

### 3.1 Python + Playwright

```powershell
# Python 3.10+ 已安装的前提下
pip install playwright
# connect_over_cdp 不需要本地 chromium，无需 playwright install
```

验证：

```powershell
python -c "from playwright.sync_api import sync_playwright; print('OK')"
```

### 3.2 设备与网络

- 真实设备已上电、IP 可达（如 daq-p1604 设备 `192.168.1.7:9000`）。
- 应用内的设备配置已存在（设备发现 / autoConnect 由前端 store 管理）。
- 主机防火墙允许 `127.0.0.1:9222` 本地回环（默认允许）。

---

## 4. 启动被测应用

### 4.1 编译

```powershell
cd projects\daq-p1604\apps\desktop-wails
$env:GOWORK="off"   # daq-p1604 在 go.work 中，按需设置
go build -o daq-p1604.exe .
```

### 4.2 以 CDP 模式启动

```powershell
# 先杀掉可能残留的旧进程
Get-Process daq-p1604 -ErrorAction SilentlyContinue | Stop-Process -Force

# 设置 CDP 端口并启动
$env:DAQ_P1604_CDP_PORT="9222"
.\daq-p1604.exe
```

> **注意**：`daq-p1604.exe` 是阻塞进程，需在独立终端启动，或用 `Start-Process` 后台启动。AI Agent 建议用 `RunCommand` 的 `blocking: false` + `command_type: "long_running_process"`。

### 4.3 验证 CDP 端口可达

```powershell
curl http://127.0.0.1:9222/json/version
```

预期返回（关键字段）：

```json
{
  "Browser": "Edge/150.x.x.x",
  "webSocketDebuggerUrl": "ws://127.0.0.1:9222/devtools/browser/..."
}
```

再查页面列表：

```powershell
curl http://127.0.0.1:9222/json/list
```

预期返回 1 个 page，title 为应用窗口标题（如 "DAQ-P-1604 压力采集"）。

---

## 5. 编写测试脚本

### 5.1 骨架

```python
from playwright.sync_api import sync_playwright, Browser, Page, TimeoutError as PWTimeout

CDP_URL = "http://127.0.0.1:9222"

with sync_playwright() as p:
    browser: Browser = p.chromium.connect_over_cdp(CDP_URL)
    # WebView2 通常只有一个 context 和一个 page
    context = browser.contexts[0]
    page: Page = context.pages[0]
    page.set_default_timeout(15_000)  # 15 秒
    # ... 测试逻辑
```

### 5.2 Selector 经验（Naive UI + 本项目约定）

| 元素 | Selector | 说明 |
|---|---|---|
| 侧栏设备条目 | `[data-testid='sidebar-item']` | 项目约定，最稳定 |
| 详情头部 | `.detail__header` | 设备详情面板容器 |
| 状态标签 | `.detail__header-right .n-tag` | Naive UI NTag，文本如 "已连接" |
| 设备名 | `.detail__device-info h2` | 详情区 H2 标题 |
| 按文本找按钮 | `button:has-text("连接")` | 兼容 NButton 内部 span 包裹 |
| 侧栏容器 | `aside.sidebar` | 等待 Vue 挂载完成 |

> **选择器优先级**：`data-testid` > 语义化 class > 文本匹配 > DOM 结构。新写组件时务必加 `data-testid`。

### 5.3 状态等待模式（轮询而非 sleep）

```python
def wait_for_status(page, target_status, timeout_ms=30_000):
    """轮询状态标签，直到匹配或超时。"""
    deadline = time.time() + timeout_ms / 1000
    while time.time() < deadline:
        st = get_device_status_text(page)
        if st == target_status:
            return True
        if st == "错误":
            raise RuntimeError(f"设备进入错误状态")
        time.sleep(0.5)
    raise PWTimeout(f"等待状态 '{target_status}' 超时")
```

**为什么轮询**：设备连接 / 采集启动是异步过程，状态标签文本会从 "连接中" → "已连接" 变化。固定 `sleep` 不可靠（慢设备会失败，快设备浪费时间）。

### 5.4 截图与日志规范

```python
SHOT_DIR = Path(__file__).parent.parent / "docs" / "test-result" / "e2e-acquisition"
SHOT_DIR.mkdir(parents=True, exist_ok=True)

def shot(page, name):
    out = SHOT_DIR / f"{name}.png"
    page.screenshot(path=str(out), full_page=False)
    print(f"[E2E] 截图保存: {out.name}", flush=True)
    return out

def log(msg):
    print(f"[E2E] {time.strftime('%H:%M:%S')} {msg}", flush=True)
```

**命名约定**：`{序号}-{步骤}-{变体}.png`，如 `02-connected.png`、`04-acquiring-t2.png`。序号保证排序，变体便于多帧对比。

> **路径陷阱**：`Path(__file__).parent.parent` 从 `apps/desktop-wails/` 上溯两级是 `apps/`，所以截图落在 `apps/docs/`。要落到 `projects/daq-p1604/docs/`，需 `parent.parent.parent` 或显式指定绝对路径。**建议用绝对路径或相对仓库根**。

### 5.5 完整测试链路（daq-p1604 实例）

| 步骤 | 操作 | 期待状态 | 截图 |
|---|---|---|---|
| 0 | 初始 | 侧栏可见 | `00-initial.png` |
| 1 | 选中侧栏第一个设备 | 详情面板加载 | `01-device-selected.png` |
| 2 | 点击"连接"（若未连接） | 已连接 | `02-connected.png` |
| 3 | 点击"开始采集" | 采集中 | — |
| 4 | 等 2s | 数据流入 | `04-acquiring-t2.png` |
| 5 | 等 3s | 持续采集 | `05-acquiring-t5.png` |
| 6 | 点击"停止采集" | 已连接 | `06-stopped.png` |
| 7 | 点击"断开" | 未连接 | `07-disconnected.png` |

完整脚本参见 [e2e_acquisition.py](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/daq-p1604/apps/desktop-wails/e2e_acquisition.py)。

---

## 6. 运行测试

```powershell
cd projects\daq-p1604\apps\desktop-wails
python e2e_acquisition.py
```

**成功输出示例**：

```
[E2E] 15:38:13 连接 CDP: http://127.0.0.1:9222
[E2E] 15:38:14 已连接页面: http://wails.localhost/
[E2E] 15:38:14 已选中设备: DAQ-P-1604-374D87
[E2E] 15:38:15 初始状态: 已连接
[E2E] 15:38:15 设备已处于 已连接 状态，跳过连接步骤
[E2E] 15:38:16 设备采集中
[E2E] 15:38:18 截图保存: 04-acquiring-t2.png
[E2E] 15:38:21 截图保存: 05-acquiring-t5.png
[E2E] 15:38:23 截图保存: 06-stopped.png
[E2E] 15:38:24 最终状态: 未连接
[E2E] 15:38:24 E2E 测试全部通过 ✓
```

退出码约定：

| 退出码 | 含义 |
|---|---|
| 0 | 全部通过 |
| 2 | 无 browser context（CDP 连接异常） |
| 3 | 无 page（窗口未创建） |
| 4 | 侧栏未出现（Vue 挂载失败） |
| 5+ | 各步骤具体失败 |

---

## 7. 截图后处理（可选）

如需把截图嵌入报告或聊天展示（避免 PNG 过大），可缩成 JPEG + base64。脚本见 [resize_shots.ps1](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/daq-p1604/apps/desktop-wails/resize_shots.ps1)。

```powershell
.\resize_shots.ps1
# 输出: 800x450 JPEG (质量 70) + .b64 文件
```

> 仅在需要嵌入聊天 / PureShowWidget 时使用。归档测试结果保留原始 PNG 即可。

---

## 8. 踩坑记录

| 问题 | 根因 | 解决 |
|---|---|---|
| CDP 端口连不上 | go-webview2 清空 `WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS` 环境变量 | 用 `WindowsOptions.AdditionalBrowserArgs` 而非环境变量 |
| `appOpts.AdditionalBrowserArgs undefined` | 字段在 `WindowsOptions` 下，不在 `Options` 顶层 | `appOpts.Windows.AdditionalBrowserArgs` |
| `browser.contexts` 为空 | CDP 连接了但 WebView2 还没初始化完 | 等 1-2 秒后重试，或检查 `/json/list` 是否有 page |
| `context.pages` 为空 | 窗口未创建（main.go 中 `Window.NewWithOptions` 还没执行） | 确认应用主窗口已显示 |
| 状态标签读不到 | `.n-tag` 还没渲染 | `time.sleep(1.0)` 后再读，或用 `wait_for(state="visible")` |
| 找不到"连接"按钮 | 设备已 autoConnect，按钮文本变成"断开" | 先读状态，已连接则跳过连接步骤 |
| 截图路径不对 | `Path(__file__).parent.parent` 上溯层级算错 | 用绝对路径或显式 `Path(__file__).resolve().parents[N]` |

---

## 9. 验证标准（Definition of Done）

E2E 测试通过需同时满足：

- [ ] 脚本退出码为 0
- [ ] 日志出现 "E2E 测试全部通过 ✓"
- [ ] 所有步骤截图已生成（按 [第 5.5 节](#55-完整测试链路daq-p1604-实例)表格）
- [ ] 状态流转符合预期：初始 → 已连接 → 采集中 → 已连接 → 未连接
- [ ] 采集期间截图可见实时数据（通道卡片有数值变化）
- [ ] 无未捕获异常（脚本无 traceback）

---

## 10. 复用到其他项目的步骤

1. **复制 `e2e_acquisition.py`** 到目标项目 `apps/desktop-wails/`。
2. **改环境变量名**：`DAQ_P1604_CDP_PORT` → `XXX_CDP_PORT`（见 [第 2.4 节](#24-环境变量命名约定)）。
3. **改 main.go**：加 `appOpts.Windows.AdditionalBrowserArgs` 注入逻辑（见 [第 2.3 节](#23-改造代码以-appsdesktop-wailsmango-为例)）。
4. **改 Selector**：根据目标项目前端组件调整（`data-testid` 优先）。
5. **改状态文本**：如目标项目状态文案不同（如 "Connected" 而非 "已连接"），同步修改。
6. **改截图目录**：`SHOT_DIR` 指向目标项目 `docs/test-result/`。
7. **跑通验证**：按 [第 4 节](#4-启动被测应用) 启动 → [第 6 节](#6-运行测试) 运行。

---

## 11. AI Agent 执行清单

AI 拿到本指南后，执行 E2E 测试的标准流程：

1. `Read` 本指南 + 目标项目 `main.go` + 现有 `e2e_*.py`（如有）。
2. 确认 `main.go` 已加 CDP 注入逻辑；若未加，按 [第 2.3 节](#23-改造代码以-appsdesktop-wailsmango-为例) 修改并 `go build`。
3. 杀掉旧进程，以 `XXX_CDP_PORT=9222` 启动应用（`blocking: false`）。
4. `curl /json/version` 验证 CDP 可达。
5. 编写 / 修改测试脚本。
6. `python e2e_*.py` 运行，观察日志与退出码。
7. 失败时按 [第 8 节](#8-踩坑记录) 排查；不要盲目重试。
8. 通过后按 [第 9 节](#9-验证标准definition-of-done) 自检。
9. 汇报结果：状态流转、截图路径、退出码、关键日志。
10. 清理：告知用户 CDP 端口仍在运行，下次正常启动（不设环境变量）即恢复生产态。

---

**维护者**：本文档基于 2026-07-23 daq-p1604 真实设备 E2E 测试经验整理。Wails v3 版本以 `v3.0.0-alpha2.106` 为准；后续 Wails 升级若改变 `AdditionalBrowserArgs` 字段位置，需同步更新 [第 2.2 节](#22-正确做法wails-v3-的-windowsoptionsadditionalbrowserargs)。
