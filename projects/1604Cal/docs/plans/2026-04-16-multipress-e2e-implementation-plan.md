# Multipress E2E 测试实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 建立基于 Playwright 的独立 E2E 测试工程，覆盖多设备打压控制模块在真实 ConST 811A 设备上的全部核心命令。

**Architecture:** 在 `Cal1604/e2e/` 目录下新建 Playwright 项目，通过 `http-server` 或 `vite preview` 提供前端构建产物，Go 后端使用 `cmd/server/main.go` 启动。测试用 `global-setup.ts` 预置设备配置，用例覆盖注册/注销/打压/停止/排空/设单位/全停/读状态。

**Tech Stack:** Playwright (Node.js), TypeScript, Go HTTP server, ConST 811A (SCPI over TCP)

---

## 前置检查

- 确保 `192.168.3.131:8000` 可达（已释放占用端口）。
- 确保 Node.js >= 18、Go >= 1.21 已安装。

---

## Task 1: 初始化 Playwright E2E 工程

**Files:**
- Create: `e2e/package.json`
- Create: `e2e/playwright.config.ts`
- Create: `e2e/.gitignore`

**Step 1: 创建 e2e 目录并写入 package.json**

```bash
mkdir -p e2e/tests e2e/fixtures
```

写入 `e2e/package.json`：

```json
{
  "name": "cal1604-e2e",
  "version": "1.0.0",
  "private": true,
  "scripts": {
    "test": "playwright test",
    "test:ui": "playwright test --ui",
    "report": "playwright show-report"
  },
  "devDependencies": {
    "@playwright/test": "^1.42.0",
    "http-server": "^14.1.1"
  }
}
```

**Step 2: 安装依赖**

```bash
cd e2e && npm install
npx playwright install chromium
```

Expected: `node_modules` 和 `package-lock.json` 生成，Chromium 下载完成。

**Step 3: 写入 playwright.config.ts**

```typescript
import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: './tests',
  fullyParallel: false, // 串行，避免同时操作同一台真实设备
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: 1,
  reporter: 'html',
  use: {
    baseURL: 'http://localhost:4173',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    video: 'on-first-retry',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
})
```

**Step 4: 写入 .gitignore**

```
node_modules/
test-results/
playwright-report/
playwright/.cache/
```

**Step 5: Commit**

```bash
git add e2e/package.json e2e/package-lock.json e2e/playwright.config.ts e2e/.gitignore
git commit -m "chore: init playwright e2e project for multipress"
```

---

## Task 2: 编写全局设备预置辅助脚本

**Files:**
- Create: `e2e/fixtures/device-setup.ts`

**Step 1: 写入设备预置 API 辅助函数**

```typescript
// e2e/fixtures/device-setup.ts
export const API_BASE = 'http://localhost:8080/api/v1'

export const DEVICE_ID = '811a-test-01'

export async function setupDevice() {
  // 1. 创建设备配置
  const resp = await fetch(`${API_BASE}/devices`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      id: DEVICE_ID,
      name: 'ConST 811A 测试设备',
      type: 'pressure',
      model: '811A',
      host: '192.168.3.131',
      port: 8000,
      status: 'disconnected',
    }),
  })
  if (!resp.ok && resp.status !== 409) {
    throw new Error(`setupDevice failed: ${resp.status}`)
  }

  // 2. 连接设备（设备管理模块层面）
  await fetch(`${API_BASE}/devices/connect`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ id: DEVICE_ID }),
  })
}

export async function teardownDevice() {
  // 先 multipress 层面停止并注销
  await fetch(`${API_BASE}/multipress/stop-all`, { method: 'POST' }).catch(() => {})
  await fetch(`${API_BASE}/multipress/unregister`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ deviceId: DEVICE_ID }),
  }).catch(() => {})

  // 再断开设备管理连接
  await fetch(`${API_BASE}/devices/disconnect`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ id: DEVICE_ID }),
  }).catch(() => {})
}
```

**Step 2: Commit**

```bash
git add e2e/fixtures/device-setup.ts
git commit -m "test(e2e): add device setup/teardown helpers"
```

---

## Task 3: 编写第一个 E2E 测试用例（注册与注销）

**Files:**
- Create: `e2e/tests/multipress.spec.ts`

**Step 1: 编写测试文件骨架**

```typescript
import { test, expect } from '@playwright/test'
import { setupDevice, teardownDevice, DEVICE_ID } from '../fixtures/device-setup'

test.beforeAll(async () => {
  await setupDevice()
})

test.afterAll(async () => {
  await teardownDevice()
})

test.describe('多设备打压控制', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/#/multi-pressure')
    await page.waitForSelector('.multi-press-page', { timeout: 10000 })
  })

  test('TC01: 注册与注销设备', async ({ page }) => {
    // 1. 可用设备列表中应包含测试设备
    const availableCard = page.locator('.available-card').filter({ hasText: 'ConST 811A 测试设备' })
    await expect(availableCard).toBeVisible({ timeout: 10000 })

    // 2. 点击注册
    await availableCard.locator('button:has-text("注册")').click()

    // 3. 等待设备进入已注册区域
    const registeredCard = page.locator('.pressure-card').filter({ hasText: 'ConST 811A 测试设备' })
    await expect(registeredCard).toBeVisible({ timeout: 15000 })

    // 4. 验证状态为"空闲"
    await expect(registeredCard.locator('.status-label')).toHaveText('空闲')

    // 5. 点击注销
    await registeredCard.locator('button:has-text("注销")').click()

    // 6. 设备回到可用区域
    await expect(availableCard).toBeVisible({ timeout: 10000 })
  })
})
```

**Step 2: 构建前端并启动服务**

终端 1（Go 后端）：
```bash
go run cmd/server/main.go
```

终端 2（前端静态服务）：
```bash
cd web && npm run build
cd web/dist && npx http-server -p 4173 --cors
```

**Step 3: 运行测试**

```bash
cd e2e && npx playwright test tests/multipress.spec.ts --project=chromium
```

Expected: 测试通过，Chromium 窗口自动操作页面。

**Step 4: Commit**

```bash
git add e2e/tests/multipress.spec.ts
git commit -m "test(e2e): add multipress register/unregister test"
```

---

## Task 4: 扩展测试用例（打压、停止、排空、设单位）

**Files:**
- Modify: `e2e/tests/multipress.spec.ts`

**Step 1: 在 spec 文件中追加用例**

在 `test.describe` 内添加：

```typescript
  test('TC02: 设置目标压力并停止', async ({ page }) => {
    // 注册
    const availableCard = page.locator('.available-card').filter({ hasText: 'ConST 811A 测试设备' })
    await availableCard.locator('button:has-text("注册")').click()
    const registeredCard = page.locator('.pressure-card').filter({ hasText: 'ConST 811A 测试设备' })
    await expect(registeredCard).toBeVisible({ timeout: 15000 })

    // 输入 0.5 MPa
    await registeredCard.locator('.target-input input').fill('0.5')
    await registeredCard.locator('.unit-select .el-input__inner').click()
    await page.locator('.el-select-dropdown__item:has-text("MPa")').click()

    // 开始打压
    await registeredCard.locator('button:has-text("开始打压")').click()

    // 断言状态变为"打压中"
    await expect(registeredCard.locator('.status-label')).toHaveText('打压中', { timeout: 10000 })
    await expect(registeredCard).toHaveClass(/pressurizing/)

    // 等待压力值更新（非 --）
    await expect(registeredCard.locator('.pressure-value')).not.toHaveText('--', { timeout: 15000 })

    // 停止
    await registeredCard.locator('button:has-text("停止")').click()
    await expect(registeredCard.locator('.status-label')).toHaveText('空闲', { timeout: 10000 })

    // 注销
    await registeredCard.locator('button:has-text("注销")').click()
    await expect(availableCard).toBeVisible({ timeout: 10000 })
  })

  test('TC03: 排空压力', async ({ page }) => {
    const availableCard = page.locator('.available-card').filter({ hasText: 'ConST 811A 测试设备' })
    await availableCard.locator('button:has-text("注册")').click()
    const registeredCard = page.locator('.pressure-card').filter({ hasText: 'ConST 811A 测试设备' })
    await expect(registeredCard).toBeVisible({ timeout: 15000 })

    // 先打压到 0.3 MPa
    await registeredCard.locator('.target-input input').fill('0.3')
    await registeredCard.locator('.unit-select .el-input__inner').click()
    await page.locator('.el-select-dropdown__item:has-text("MPa")').click()
    await registeredCard.locator('button:has-text("开始打压")').click()
    await expect(registeredCard.locator('.status-label')).toHaveText('打压中', { timeout: 10000 })

    // 排空
    await registeredCard.locator('button:has-text("排空")').click()
    await expect(registeredCard.locator('.status-label')).toHaveText('排空中', { timeout: 10000 })
    await expect(registeredCard).toHaveClass(/exhausting/)

    // 等待 10 秒后停止
    await page.waitForTimeout(10000)
    await registeredCard.locator('button:has-text("停止")').click()
    await expect(registeredCard.locator('.status-label')).toHaveText('空闲', { timeout: 10000 })

    // 注销
    await registeredCard.locator('button:has-text("注销")').click()
    await expect(availableCard).toBeVisible({ timeout: 10000 })
  })

  test('TC04: 切换压力单位', async ({ page }) => {
    const availableCard = page.locator('.available-card').filter({ hasText: 'ConST 811A 测试设备' })
    await availableCard.locator('button:has-text("注册")').click()
    const registeredCard = page.locator('.pressure-card').filter({ hasText: 'ConST 811A 测试设备' })
    await expect(registeredCard).toBeVisible({ timeout: 15000 })

    // 切换单位为 MPa
    await registeredCard.locator('.unit-select .el-input__inner').click()
    await page.locator('.el-select-dropdown__item:has-text("MPa")').click()
    await expect(registeredCard.locator('.pressure-unit')).toHaveText('MPa')

    // 打压 0.1 MPa 验证可用
    await registeredCard.locator('.target-input input').fill('0.1')
    await registeredCard.locator('button:has-text("开始打压")').click()
    await expect(registeredCard.locator('.status-label')).toHaveText('打压中', { timeout: 10000 })

    // 停止并切回 kPa
    await registeredCard.locator('button:has-text("停止")').click()
    await expect(registeredCard.locator('.status-label')).toHaveText('空闲', { timeout: 10000 })
    await registeredCard.locator('.unit-select .el-input__inner').click()
    await page.locator('.el-select-dropdown__item:has-text("kPa")').click()
    await expect(registeredCard.locator('.pressure-unit')).toHaveText('kPa')

    // 注销
    await registeredCard.locator('button:has-text("注销")').click()
    await expect(availableCard).toBeVisible({ timeout: 10000 })
  })
```

**Step 2: 运行全部测试**

```bash
cd e2e && npx playwright test tests/multipress.spec.ts --project=chromium
```

Expected: 4 个用例全部通过。

**Step 3: Commit**

```bash
git add e2e/tests/multipress.spec.ts
git commit -m "test(e2e): add pressurize, exhaust and unit switch tests"
```

---

## Task 5: 添加全部停止与 API 读取状态用例

**Files:**
- Modify: `e2e/tests/multipress.spec.ts`
- Modify: `e2e/fixtures/device-setup.ts`（如需要 API 辅助）

**Step 1: 追加 TC05 和 TC06**

```typescript
  test('TC05: 全部停止', async ({ page }) => {
    const availableCard = page.locator('.available-card').filter({ hasText: 'ConST 811A 测试设备' })
    await availableCard.locator('button:has-text("注册")').click()
    const registeredCard = page.locator('.pressure-card').filter({ hasText: 'ConST 811A 测试设备' })
    await expect(registeredCard).toBeVisible({ timeout: 15000 })

    // 开始打压
    await registeredCard.locator('.target-input input').fill('0.2')
    await registeredCard.locator('.unit-select .el-input__inner').click()
    await page.locator('.el-select-dropdown__item:has-text("MPa")').click()
    await registeredCard.locator('button:has-text("开始打压")').click()
    await expect(registeredCard.locator('.status-label')).toHaveText('打压中', { timeout: 10000 })

    // 点击全部停止
    await page.locator('button:has-text("全部停止")').click()
    await expect(registeredCard.locator('.status-label')).toHaveText('空闲', { timeout: 10000 })

    // 注销
    await registeredCard.locator('button:has-text("注销")').click()
    await expect(availableCard).toBeVisible({ timeout: 10000 })
  })

  test('TC06: 通过 API 读取压力与稳定状态', async ({ page, request }) => {
    const availableCard = page.locator('.available-card').filter({ hasText: 'ConST 811A 测试设备' })
    await availableCard.locator('button:has-text("注册")').click()
    const registeredCard = page.locator('.pressure-card').filter({ hasText: 'ConST 811A 测试设备' })
    await expect(registeredCard).toBeVisible({ timeout: 15000 })

    // 开始打压
    await registeredCard.locator('.target-input input').fill('0.2')
    await registeredCard.locator('.unit-select .el-input__inner').click()
    await page.locator('.el-select-dropdown__item:has-text("MPa")').click()
    await registeredCard.locator('button:has-text("开始打压")').click()
    await expect(registeredCard.locator('.status-label')).toHaveText('打压中', { timeout: 10000 })

    // 等待 5 秒后调用 API
    await page.waitForTimeout(5000)

    const pressureResp = await request.get(`${API_BASE}/multipress/pressure?deviceId=${DEVICE_ID}`)
    expect(pressureResp.status()).toBe(200)
    const pressureBody = await pressureResp.json()
    expect(typeof pressureBody.data.pressure).toBe('number')

    const stabilityResp = await request.get(`${API_BASE}/multipress/stability?deviceId=${DEVICE_ID}`)
    expect(stabilityResp.status()).toBe(200)
    const stabilityBody = await stabilityResp.json()
    expect(typeof stabilityBody.data.stable).toBe('boolean')

    // 停止并注销
    await registeredCard.locator('button:has-text("停止")').click()
    await expect(registeredCard.locator('.status-label')).toHaveText('空闲', { timeout: 10000 })
    await registeredCard.locator('button:has-text("注销")').click()
    await expect(availableCard).toBeVisible({ timeout: 10000 })
  })
```

**Step 2: 运行全部 6 个用例**

```bash
cd e2e && npx playwright test tests/multipress.spec.ts --project=chromium
```

Expected: 6 个用例全部通过。

**Step 3: Commit**

```bash
git add e2e/tests/multipress.spec.ts
git commit -m "test(e2e): add stop-all and api read state tests"
```

---

## Task 6: 添加 npm 脚本与运行说明

**Files:**
- Modify: `e2e/package.json`
- Modify: `web/package.json`（可选，添加 `preview` 脚本）
- Modify: `docs/plans/2026-04-16-multipress-e2e-design.md`（补充快速运行命令）

**Step 1: 在 e2e/package.json 中添加脚本**

```json
  "scripts": {
    "test": "playwright test",
    "test:ui": "playwright test --ui",
    "report": "playwright show-report",
    "prebuild": "cd ../web && npm run build",
    "pretest": "cd ../web && npm run build"
  }
```

**Step 2: 在 web/package.json 中添加 preview 脚本**

```json
  "preview": "vite preview --port 4173"
```

**Step 3: 验证一键运行流程**

终端 1：
```bash
go run cmd/server/main.go
```

终端 2：
```bash
cd web && npm run build && npm run preview
```

终端 3：
```bash
cd e2e && npm test
```

Expected: 6 个用例全部通过，生成 `playwright-report/`。

**Step 4: Commit**

```bash
git add e2e/package.json web/package.json
git commit -m "chore: add e2e run scripts and preview support"
```

---

## Task 7: 运行质量门禁并收尾

**Step 1: 运行前端 lint/typecheck**

```bash
cd web && npm run typecheck && npm run lint
```

Expected: 无错误。

**Step 2: 运行 Go 测试**

```bash
go test ./...
```

Expected: 全部通过。

**Step 3: 最终 Commit（如需要）**

```bash
git add docs/plans/2026-04-16-multipress-e2e-design.md
git commit -m "docs: add multipress e2e test design document"
```

---

## 附录：常见问题排查

| 现象 | 排查方法 |
|------|----------|
| `192.168.3.131:8000` 连接超时 | `telnet 192.168.3.131 8000` 确认端口可达；检查防火墙 |
| 页面元素找不到 | 确认前端已构建；确认 `baseURL` 正确；查看 Playwright trace |
| 打压后压力不变化 | 检查设备气源/液压源是否接通；检查管路是否泄漏 |
| 多个用例互相影响 | 确认 `fullyParallel: false` 和 `workers: 1` 已设置 |
