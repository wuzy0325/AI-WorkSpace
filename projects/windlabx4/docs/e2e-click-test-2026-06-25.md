# Wind-DAQ E2E 实际点击测试记录

测试时间：2026-06-25 23:13

## 测试工具与方式

- 测试工具：Python Playwright 真实 Chromium 点击巡检脚本。
- 启动方式：复用当前机器上已运行的 `http://127.0.0.1:9245` 前端与 `:8080` 后端服务。
- 测试范围：按当前配置打开 Wind-DAQ，依次巡检仪表盘、探针校准、遍历测试、运行日志、设置入口等页面的可见按钮。
- 原始记录：仓库根目录 `wind-daq-e2e-click-audit.json`。
- 运动控制专项原始记录：仓库根目录 `wind-daq-motion-click-audit.json`、`wind-daq-motion-control-click-audit.json`、`wind-daq-motion-control-exact-click-audit.json`。
- 运动控制真实 local API 用户流记录：仓库根目录 `wind-daq-motion-user-flow-audit.json`。
- 剩余控件专项记录：仓库根目录 `wind-daq-remaining-controls-audit.json`。
- 临时脚本：仓库根目录 `scripts/wind_daq_e2e_click_audit.py`。

## 点击覆盖概览

- 总点击次数：57。
- 控制台错误：16 条。
- 页面错误：1 条。
- 请求失败：18 条。
- 导航等待超时：3 次，均与长连接/SSE/轮询请求有关，测试脚本继续执行。

## 已点击的主要功能按钮

### 仪表盘

- 视图切换：`图表`、`表格`、`混合`、`总览`。
- 采集控制：`开始采集`、`停止采集`。
- 记录控制：`开始记录`。
- 设备侧栏/管理：`管理`、`sim-1 已连接`、`全部归零`、抽屉关闭 `✕`。

### 探针校准

- 顶栏采集控制：`开始采集`、`停止采集`。
- 顶栏记录控制：`开始记录`。

### 遍历测试

- 顶栏采集控制：`开始采集`、`停止采集`。
- 顶栏记录控制：`开始记录`。
- 功能入口：`配置`、`点位预览`、`流场可视化`、`热力图`、`剖面图`、`矢量场`、`压力雷达`、关闭按钮 `close`。

### 运行日志

- 顶栏采集控制：`开始采集`、`停止采集`。
- 顶栏记录控制：`开始记录`。
- 日志控制：`Pause`、`Resume`、`Clear`、`Copy all`。
- 日志过滤：`ALL`、`DEBUG`、`INFO`、`WARN`、`ERROR`。

### 设置入口

- 点击了侧栏 `设置` 入口。
- 本轮实际页面仍停留在日志页按钮组，未观察到设置弹窗内的设置项按钮被稳定枚举到。

### 运动控制器专项补测

- 直达页面：`http://127.0.0.1:9245/#/motion`。
- 已点击：`配置`、控制器卡片 `模拟控制器 1`、精确按钮 `连接`、`急停`。
- 已观察但因状态禁用：`断开`、`全部停止`。
- 配置弹窗已进入，看到并操作到通信设置中的控制器类型，`SIMULATED-MC` 可切到 `B140-MC`。
- 配置弹窗内 `删除`、`新建`、`取消`、`保存/创建控制器` 可见；为避免改配置，本轮未点击 `删除`、`保存/创建控制器`。
- 轴配置区域可见内容包括：`步距角 °`、`细分数`、`导程 mm/减速比`、`最大速度/最大转速`、`方向反转`、`位置来源`；但 Naive UI 自定义 Select/Checkbox 没有被通用原生控件脚本稳定点到，需后续用组件级 selector 专项覆盖。

### 运动控制器 local API 真实链路补测

- 启动 `build/bin/wind-daq.exe` 后，`127.0.0.1:8900/api/motion/status` 在崩溃前返回 200。
- local API 返回 3 个已连接控制器：`Simulated Controller 1`、`B140`、`MC4`。
- 为避免误动真实硬件，带运动状态改变的按钮只对 `Simulated Controller 1` 执行。
- 已通过浏览器用户流点击并代理到 `8900`：选择 `Simulated Controller 1`、`断开`、`置零`、`+`、`移动`、`全部停止`、`急停`、`配置`、`新建控制器`、`取消`。
- 后端直测确认：`POST /api/motion/disconnect` 对 `sim-motion-1` 返回 200，随后 `/api/motion/status` 中模拟控制器消失，只剩 `B140`/`MC4`，说明后端断开成功。

### 阻塞修复后复测

- 修复后新后端在 `:18080` 验证 `GET /api/motion/status` 返回 200，说明 API-Go 启动路径已注册 `/api/motion/*`。
- 修复后桌面主程序启动 9 秒后仍存活，`127.0.0.1:8900/api/motion/status` 返回 200，说明启动即崩溃阻塞已解除。
- 修复后关键运动控制用户流跑通：`Simulated Controller 1` 选择、`断开`、刷新后 `连接`、`置零`、`+`、`移动`、`全部停止`、`急停` 均为 `ok`。
- 复测结束后 `8900` 与 `9245` 均仍可访问。

### 剩余控件专项补测

- 设备管理抽屉：`管理`、`扫描`、`新建设备`、`基本信息`、`取消`、关闭 `✕` 已点击；`通道设置` 被 toast 遮挡导致点击失败；`全部启用`、`全部禁用`、`重置` 未出现在当前编辑状态；批量按钮在未选中设备状态下未出现。
- 设置弹窗：`设置` 入口已点击；`采集`、`存储`、`取消` 已点击；`浅色`、`深色`、`中文`、`English` 定位到不可见节点，未成功点击；`选择` 类按钮被 modal mask 遮挡。
- 运动控制配置：`配置`、`自动连接`、`直线轴`、`旋转轴`、`启用`、`方向反转`、`寄存器`、`新建控制器`、`取消` 已点击；`模拟控制器`、`B140 控制器` 定位到不可见原生 option，未按用户方式完成；`编码器` 未出现在当前可用状态。

## 发现的问题

### P1：前端仍请求 `127.0.0.1:8900`，当前 Vite+Go 后端配置下相关功能失败

- 现象：多次出现 `net::ERR_CONNECTION_REFUSED`。
- 失败请求包括：
- `GET http://127.0.0.1:8900/api/storage/status`
- `POST http://127.0.0.1:8900/api/storage/start`
- `GET http://127.0.0.1:8900/api/traversal/config`
- `GET http://127.0.0.1:8900/api/traversal/status`
- `GET http://127.0.0.1:8900/api/traversal/loadCheckpoint`
- 用户影响：`开始记录`、遍历配置/状态加载等功能在当前 Vite dev + `:8080` 后端配置下不可用或报错。
- 相关线索：`projects/wind-daq/apps/desktop-wails/frontend/src/api/traversalApi.ts` 默认 API base 为 `http://127.0.0.1:8900`，与 README 中 Vite proxy 到 `:8080` 的当前开发配置不一致。

### P1：页面出现 `404 page not found` 页面错误

- 现象：巡检过程中捕获页面错误 `404 page not found`。
- 可能关联：部分请求被错误拼接或走到了不支持的路径。
- 用户影响：功能按钮点击后可能触发无效接口，造成 UI 无法完成操作。

### P1：Wails 主应用启动后崩溃，导致 `8900` local API 中途消失

- 复现方式：运行 `build/bin/wind-daq.exe`，应用初始化成功并打印 `Wind-DAQ local API listening on http://127.0.0.1:8900`。
- 崩溃日志：`panic error: reflect: call of reflect.Value.IsNil on string Value`。
- 崩溃栈：`encoding/json` → `github.com/wailsapp/wails/v3/pkg/application.(*CustomEvent).ToJSON` → `DispatchWailsEvent` → `dispatchWindowEvent`。
- 用户影响：桌面主进程会退出，`127.0.0.1:8900` 随之消失；运动控制真实链路测试无法持续完成。
- 备注：崩溃前 local API 确认可返回运动状态，且自动连接了 `Simulated Controller 1`、`B140`、`MC4`。
- 修复状态：已通过取消 Wails window JS event 分发规避该 v3 alpha 序列化崩溃；桌面进程复测保持存活。

### P2：设备管理抽屉遮罩打开后，下层按钮仍被巡检识别但无法点击

- 现象：点击 `管理` 后，脚本继续尝试点击 `sim-1 已连接`、`全部归零`，Playwright 报告抽屉遮罩 `drawer-mask` / `drawer-toolbar` 拦截了指针事件。
- 用户影响：这不一定是用户可见 bug，但说明当前自动化枚举到的下层按钮没有被隐藏出可交互树；E2E 测试和辅助技术可能误判可点击区域。
- 记录：`sim-1 已连接` 与 `全部归零` 两次点击状态为 `failed`。

### P2：遍历页 `配置`、`点位预览`、日志页 `Resume` 触发错误状态

- 现象：点击结果标记为 `issue`，对应点击后控制台错误数量增加。
- 关联错误：同轮出现 `127.0.0.1:8900` 请求失败。
- 用户影响：遍历配置/预览和日志暂停恢复相关操作需要进一步人工确认 UI 状态是否符合预期。

### P2：运动控制页 `连接` 和 `急停` 点击后触发 404 错误

- 现象：在 `#/motion` 页面精确点击 `连接`、`急停` 后，点击状态标记为 `issue`。
- 捕获错误：控制台多次出现 `Failed to load resource: the server responded with a status of 404 (Not Found)`，页面错误为 `404 page not found`。
- 用户影响：当前 Vite 浏览器环境下，运动控制器连接/急停动作无法证明成功；`断开`、`全部停止` 因控制器未连接保持禁用。
- 相关线索：运动控制 API 在浏览器非 Wails 环境下走 `/api/motion/*`，当前 `:8080` 后端或 Vite proxy 对这些路径返回 404。
- 修复状态：API-Go 启动路径已注册 MotionService，新启动后端 `/api/motion/status` 返回 200；旧 `:8080` 进程仍被外部 `ApplicationWebServer.exe` 占用，验证使用 `:18080`。

### P2：浏览器版运动页非 standalone 模式不会自动刷新状态

- 现象：点击 `断开` 后，local API 已确认 `sim-motion-1` 从 `/api/motion/status` 消失，但浏览器页面刷新/重新进入后仍显示 `Simulated Controller 1 已连接` 的一段时间内状态。
- 影响：真实用户可能看到运动控制器连接状态与后端实际状态不一致，后续 `连接` 按钮被误判为 disabled。
- 相关线索：`MotionControlPanel` 在 `onMounted` 只 `refreshStatus()` 一次，非 Wails、非 motion standalone 浏览器模式下 `onStatusUpdated` 不会启动轮询。
- 修复状态：本轮未改该 UI 刷新机制；复测中通过刷新页面确认状态并继续点击，后续仍建议修复。

### P2：运动控制器配置弹窗遮罩影响后续点击

- 现象：配置弹窗打开后，如果未通过弹窗内部 `取消/关闭` 正确退出，外层 `关闭窗口`、控制器卡片等按钮被 `config-overlay` 拦截。
- 用户影响：真实用户一般可通过弹窗按钮关闭；但 E2E 需要明确使用弹窗内部按钮，不能用页面外按钮继续操作。

### P2：设备管理弹窗内 toast 会遮挡通道设置按钮

- 现象：点击 `新建设备` 后再点 `通道设置`，Playwright 报告 `toast-host` 拦截指针事件。
- 用户影响：真实用户可能在 toast 显示期间无法立即切换到通道设置，需要等待 toast 消失或调整 toast 位置/层级。

### P2：设置弹窗中部分按钮定位到不可见节点或被 modal mask 遮挡

- 现象：`浅色`、`深色`、`中文`、`English` 定位到不可见节点；`选择` 类按钮被 `n-modal-mask` 拦截。
- 用户影响：设置页自动化和可能的键盘/辅助技术访问不稳定；需要增加更明确的可访问名称或测试定位标识。

### P2：运动控制配置中下拉选项需要组件级交互

- 现象：`模拟控制器`、`B140 控制器` 被定位到隐藏 `<option>`，不是用户实际点击的可见 Naive UI 下拉面板；`编码器` 在当前状态未出现。
- 用户影响：真实用户可能能操作，但 E2E 需要按 Naive UI 的弹出层选项测试；建议为控制器类型、轴类型、位置来源补 `data-test` 或可访问 label。

### P3：`networkidle` 在多个页面导航后超时

- 现象：进入探针校准、遍历测试、运行日志后，等待 `networkidle` 超时。
- 可能原因：SSE、轮询或长连接持续存在。
- 用户影响：对真实用户不一定有直接影响，但会让 E2E 测试稳定性下降；建议正式 Playwright 用元素级断言替代 `networkidle` 作为主要等待条件。

## 未完全覆盖/需补测

- 设备管理抽屉内部已补测，但批量按钮需要先选择设备；本轮未执行真实批量删除以避免破坏配置。
- 设置弹窗内部已补测，但主题/语言按钮定位到不可见节点，需代码层补可测 selector 或可访问名称后再复测。
- 运动控制器配置中的 Naive UI 自定义复选框和下拉项已补测一部分；控制器类型/编码器来源仍需按弹出层组件级 selector 复测。
- Wails 桌面独立窗口功能 `运动控制` 在 Vite 浏览器环境下只能通过 `#/motion` 补测页面逻辑，仍需在 Wails desktop 环境中补测独立窗口启动、关闭和主进程 API 联动。

## 工具建议

- 短期：继续使用当前 Python Playwright 脚本做冒烟式真实点击巡检，不引入项目依赖。
- 长期：建议在 `projects/wind-daq/apps/desktop-wails/frontend` 引入官方 `@playwright/test`，增加 `playwright.config.ts`、trace/video/screenshot 留档，并把 `VITE_API_BASE` 与启动命令固化到 webServer 配置。
- 不建议优先选择 Cypress：它对 Electron/Wails 桌面窗口、多窗口、下载/本地文件选择等场景扩展成本更高。
- 可备选：WebdriverIO 适合桌面/Electron 自动化，但本项目 Vue/Vite 页面级 E2E 优先 Playwright 更轻。

## 建议修复顺序

1. 统一 Web dev 环境 API base，避免 `storage`、`traversal` 等模块默认请求 `127.0.0.1:8900`。
2. 修复 Wails 主应用启动后的 `CustomEvent.ToJSON` 崩溃，否则桌面真实 E2E 会被主进程退出中断。
3. 给浏览器/Vite 模式补齐 `/api/motion/*` 路由或显式走 `8900`，避免运动控制页点击 404。
4. 修复运动页非 standalone 浏览器模式下的状态刷新机制，避免断开后 UI 仍显示已连接。
5. 给设备管理抽屉、设置弹窗、运动配置弹窗补专项 Playwright 用例，使用明确 selector，而不是纯按钮枚举。
6. 把 E2E 等待条件从 `networkidle` 改为页面关键元素可见/按钮可用。
