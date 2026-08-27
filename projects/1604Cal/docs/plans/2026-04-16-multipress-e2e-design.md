# 多设备打压控制 E2E 测试设计

> 文档日期：2026-04-16  
> 目标：为 Cal1604 系统的多设备打压控制（multipress）模块建立基于 Playwright 的浏览器端到端测试，使用真实 ConST 811A 打压设备验证核心命令链路。

---

## 1. 背景与目标

### 1.1 现状

- 前端：Vue3 + Element Plus + Pinia，单元测试使用 Vitest + jsdom。
- 后端：Go HTTP API，内置多设备打压控制服务（`internal/application/multipress`）。
- 设备：支持 ConST 811A/820（SCPI over TCP），当前驱动已实现完整命令集。
- **缺失**：没有浏览器 E2E 测试，无法自动验证用户从页面点击到设备响应的完整链路。

### 1.2 目标

1. 建立独立的 Playwright E2E 测试工程。
2. 覆盖 multipress 模块全部 8 个核心命令：注册、注销、设置压力、停止、排空、设置单位、全部停止、读取压力/稳定状态。
3. 使用真实 ConST 811A 设备（192.168.3.131:8000）运行测试，确保 TCP/SCPI 链路真实可用。
4. 测试流程可一键执行，并在失败时提供截图/Trace 回放。

---

## 2. 测试架构

```
Cal1604/
├── web/                    # Vue3 前端（Vite 构建）
├── internal/               # Go 后端
├── cmd/server/             # 独立 HTTP 服务器入口
└── e2e/                    # 【新增】Playwright E2E 测试
    ├── package.json
    ├── playwright.config.ts
    ├── fixtures/
    │   └── device-setup.ts # 设备预置 API 辅助
    ├── tests/
    │   └── multipress.spec.ts
    └── global-setup.ts     # 启动 Go 后端 + 预置设备
```

### 2.1 运行时拓扑

```
Playwright (Chromium)
    │
    ▼
web/dist (Vite 构建产物，通过 file:// 或 http-server 加载)
    │
    ▼ (API 请求)
Go HTTP Server (localhost:8080)
    │
    ▼ (TCP SCPI)
ConST 811A (192.168.3.131:8000)
```

---

## 3. 环境依赖

### 3.1 硬件

- ConST 811A 打压设备 1 台
- IP：`192.168.3.131`
- 端口：`8000`
- 网络：测试机与设备 TCP 可达

### 3.2 软件

- Node.js >= 18
- Go >= 1.21
- Playwright >= 1.42

---

## 4. 测试前置条件

E2E 测试执行前，需完成以下步骤：

1. **构建前端**：`cd web && npm run build`
2. **启动 Go 后端**：`go run cmd/server/main.go`（监听 `:8080`）
3. **预置设备配置**：通过 API 创建 811A 打压设备
   ```json
   POST /api/v1/devices
   {
     "id": "811a-test-01",
     "name": "ConST 811A 测试设备",
     "type": "pressure",
     "model": "811A",
     "host": "192.168.3.131",
     "port": 8000,
     "status": "disconnected"
   }
   ```
4. **连接设备**（可选，multipress register 会自行连接）：`POST /api/v1/devices/connect { "id": "811a-test-01" }`

---

## 5. 测试用例设计

### 5.1 用例 1：设备注册与注销（TC01）

| 步骤 | 操作 | 期望结果 |
|------|------|----------|
| 1 | 打开 `/multi-pressure` 页面 | 页面显示"可用打压设备"，包含"ConST 811A 测试设备" |
| 2 | 点击"注册"按钮 | 设备卡片移动到"已注册设备"区域，状态显示"空闲" |
| 3 | 点击"注销"按钮 | 设备卡片回到"可用打压设备"区域 |

### 5.2 用例 2：设置目标压力与停止（TC02）

| 步骤 | 操作 | 期望结果 |
|------|------|----------|
| 1 | 注册设备 | 状态为"空闲" |
| 2 | 输入目标压力 `0.5`，单位 `MPa`，点击"开始打压" | 状态变为"打压中"，当前压力值开始更新 |
| 3 | 等待 5~10 秒 | 当前压力值接近 0.5 MPa，稳定状态可能变为"已稳定" |
| 4 | 点击"停止" | 状态恢复为"空闲" |

### 5.3 用例 3：排空压力（TC03）

| 步骤 | 操作 | 期望结果 |
|------|------|----------|
| 1 | 注册设备并设置目标压力 0.3 MPa | 状态为"打压中" |
| 2 | 点击"排空" | 状态变为"排空中" |
| 3 | 等待 5~10 秒 | 当前压力值下降至接近 0 |
| 4 | 点击"停止" | 状态恢复为"空闲" |

### 5.4 用例 4：切换单位（TC04）

| 步骤 | 操作 | 期望结果 |
|------|------|----------|
| 1 | 注册设备 | 状态为"空闲" |
| 2 | 将单位从 `kPa` 切换为 `MPa` | 单位显示变为 `MPa` |
| 3 | 设置目标压力 `0.1` 并打压 | 打压正常执行，压力读数单位为 MPa |
| 4 | 停止并切换回 `kPa` | 单位显示恢复 `kPa` |

### 5.5 用例 5：全部停止（TC05）

| 步骤 | 操作 | 期望结果 |
|------|------|----------|
| 1 | 注册同一台设备（或如有多台则注册多台） | 状态为"空闲" |
| 2 | 设置目标压力开始打压 | 状态为"打压中" |
| 3 | 点击页面顶部"全部停止"按钮 | 所有已注册设备状态恢复为"空闲" |

### 5.6 用例 6：读取压力与稳定状态（TC06）

| 步骤 | 操作 | 期望结果 |
|------|------|----------|
| 1 | 注册设备 | 状态为"空闲" |
| 2 | 设置目标压力 0.2 MPa 开始打压 | 状态为"打压中" |
| 3 | 通过 API 直接读取压力和稳定状态 | `GET /api/v1/multipress/pressure?deviceId=811a-test-01` 返回数值；`GET /api/v1/multipress/stability?deviceId=811a-test-01` 返回布尔值 |
| 4 | 停止打压 | 状态恢复"空闲" |

---

## 6. 断言策略

1. **UI 断言**：
   - 状态文本（空闲 / 打压中 / 排空中）
   - CSS 类名（`.pressurizing`、`.exhausting`、`.idle`）
   - 压力数值显示（`pressure-value` 元素内容不为 `--`）
   - 稳定指示器（`已稳定` / `稳定中...`）

2. **网络断言**（Playwright `page.waitForResponse`）：
   - `POST /api/v1/multipress/register` → 200
   - `POST /api/v1/multipress/set-pressure` → 200
   - `POST /api/v1/multipress/stop` → 200

3. **SSE 断言**（可选）：
   - 监听 `events/stream`，验证收到 `multipress.pressure.changed` 或 `multipress.device.status` 事件。

---

## 7. 失败诊断

- Playwright 配置 `trace: 'on-first-retry'`，失败时自动生成 trace.zip。
- 截图配置 `screenshot: 'only-on-failure'`。
- 每个用例结束后调用 `multipress/stop-all` 并 `unregister` 设备，确保设备回到安全状态。

---

## 8. 安全与清理

- **测试前**：确认设备物理状态安全（无高压、管路通畅）。
- **测试中**：目标压力设置不超过 0.5 MPa（低压安全范围）。
- **测试后**：无论成功失败，均执行 `stop-all` + `unregister`，并可选调用 `exhaust` 排空。

---

## 9. 执行命令

```bash
# 1. 构建前端
cd web && npm run build

# 2. 启动 Go 后端（另一个终端）
go run cmd/server/main.go

# 3. 运行 E2E 测试
cd e2e && npx playwright test

# 4. 查看报告
npx playwright show-report
```

---

## 10. 风险与后续

| 风险 | 缓解措施 |
|------|----------|
| 真实设备离线导致测试失败 | 测试前 ping 设备 IP；失败时给出明确错误提示 |
| 设备响应慢导致断言超时 | Playwright 使用 `expect(...).toBeVisible({ timeout: 15000 })` |
| 多个测试并发访问同一设备 | 串行执行（`fullyParallel: false`） |
| 未来需要 CI 运行 | 预留 Mock 驱动切换接口（环境变量 `E2E_USE_MOCK_DRIVER`） |

---

## 11. 决策记录

- **使用 Playwright**：Vue3 生态首选，Trace 和自动等待能力最强。
- **独立 `e2e/` 目录**：不污染 `web/package.json`，与单元测试职责分离。
- **真实设备优先**：当前目标是验证端到端真实链路，Mock 方案作为后续扩展。
- **通过 `cmd/server/main.go` 启动后端**：复用现有独立 HTTP 服务器入口，无需修改 Wails 桌面代码。
