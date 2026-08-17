# AI 前端开发规则 · 样式/集成/验证

> 本文是 `frontend-ai-rules.zh-CN.md` 拆分后的**样式/集成/验证**部分（§28-§35）。
> 其他部分：
> - 基础架构（§1-§10）：[frontend-ai-rules-foundation.zh-CN.md](frontend-ai-rules-foundation.zh-CN.md)
> - 状态与 UI 组件（§11-§17）：[frontend-ai-rules-state.zh-CN.md](frontend-ai-rules-state.zh-CN.md)
> - 编码质量（§18-§27）：[frontend-ai-rules-quality.zh-CN.md](frontend-ai-rules-quality.zh-CN.md)
> - 样式/集成/验证（§28-§35）：[frontend-ai-rules-deploy.zh-CN.md](frontend-ai-rules-deploy.zh-CN.md)
> 完整目录与章节级精读协议见 [frontend-ai-rules.zh-CN.md](frontend-ai-rules.zh-CN.md)。

## 28. 样式和 token 规则

必须优先使用设计 token：

- color
- spacing
- typography
- radius
- shadow
- z-index
- motion

命名使用语义名，例如：

- `--color-bg-panel`
- `--color-text-primary`
- `--space-3`
- `--radius-md`
- `--z-dialog`

禁止：

- `blue1`
- `gray2`
- `gap13`
- 随手写 magic number
- 单个组件私自定义一套视觉体系
- 为局部视觉效果绕过主题变量

### 28.1 文件体量与逻辑分离量化红线

> 本节是硬性阈值，配合 `scripts/validate-frontend-structure.ps1 -CheckFileSize` 自动检查。AI 在新增或修改 `.vue` 文件时必须自检不超过下表阈值，超出必须在同一 PR 内完成拆分。

| 维度 | 警告阈值 | 错误阈值 | 触发后必须做的事 |
|---|---|---|---|
| 单个 `.vue` 文件总行数 | > 500 行 | > 800 行 | 拆分为父子组件或抽取 composable |
| 单个 `.vue` 文件 `<script setup>` 段行数 | > 200 行 | > 300 行 | 把本地函数抽到 `composables/` 或 `components/<domain>/composables/` |
| 单个 `.vue` 文件 `<style scoped>` 段行数 | > 200 行 | > 300 行 | 把通用样式抽到 `styles/utilities/`，或拆出 patterns 组件 |
| 单个 `.vue` 文件 `<script setup>` 内本地函数数 | > 10 个 | > 15 个 | 抽取 composable，组件 setup 仅保留事件绑定和生命周期钩子 |
| 单个 Pinia store 文件行数 | > 400 行 | > 600 行 | 拆分为多个职责单一的 store，或把纯计算抽到 `shared/` |
| 单个 i18n / 字典文件行数 | > 400 行 | > 600 行 | 拆分为 `locales/zh.ts` + `locales/en.ts` 等独立 locale 文件 |

豁免规则：

- `i18nStore.ts` 等字典文件在拆分为独立 locale 文件后，主 store 文件不计字典字面量行数。
- 自动生成的 Wails binding 文件（`frontend/bindings/**`）不受此限制。
- 第三方依赖文件不受此限制。

逻辑分离强制要求：

- 项目内 Pinia store 数量 ≥ 4 个，或业务领域组件（`components/<domain>/*.vue`）数量 ≥ 6 个时，必须建立 `src/composables/` 目录并开始抽取可复用逻辑。
- 单个 `.vue` 文件 `<script setup>` 内本地函数（非 import、非 store 调用、非生命周期钩子）超过 10 个时，必须把至少一半抽到 composable。
- composable 命名必须以 `use` 开头（如 `useAxisLimits`、`useMotionHistory`、`useConfigFormValidation`），文件放在 `src/composables/` 或 `src/components/<domain>/composables/`。
- composable 必须是纯逻辑：不渲染模板、不持有 DOM 引用以外的全局状态，可在多个组件复用。

反面案例（motion-controller，2026-07-06 审查发现）：

- `components/motion/MotionControlPanel.vue` 1818 行（script 325 + template 393 + style 1098），单文件承担列表 / 轴卡片 / 急停 / 键盘监听 / 历史采样 / 限位校验 / 主题色映射 7 重职责。
- `components/motion/MotionControllerConfig.vue` 994 行，setup 内联 30+ 函数未抽 composable。
- `stores/i18nStore.ts` 696 行，字典字面量内联未拆 locale 文件。

### 28.2 硬编码颜色与 CSS 字面量禁令

`<style scoped>` 块内**禁止**出现以下字面量，必须改用 token：

- 十六进制颜色：`#fff`、`#dc2626`、`#1e293b` 等
- `rgba()` / `rgb()` / `hsl()` / `hsla()` 函数调用
- `color-mix()` 中带 hex / rgba fallback 的写法
- `blur(Npx)` 中固定数值（应使用 `--blur-*` token）

允许的例外（必须在代码注释中说明原因）：

- 内联 `style` 属性引用 CSS 变量：`style="--axis-hue: var(--axis-x)"`（这是变量传递，不是颜色字面量）
- `opacity: 0.5` 等不涉及颜色的透明度数值
- 媒体查询断点 `@media (min-width: 768px)`（响应式断点另由 §7 布局规则管理）

跨组件重复的 CSS class 必须抽到 `styles/utilities/` 下单独文件：

- 同一个 class 名在 2 个及以上 `.vue` 文件的 `<style scoped>` 中重复定义时，必须抽到 `styles/utilities/<name>.css` 并在各组件中通过 `@import` 或全局引入复用。
- 典型案例：轴主题色类 `.axis-x-theme / .axis-y-theme / .axis-z-theme / .axis-u-theme` 不应在每个组件重复定义，必须集中到 `styles/utilities/axis-theme.css`。

新增视觉变量必须先定义 token：

- 任何新增的阴影、模糊、遮罩、过渡时长等视觉变量，必须先在 `styles/tokens/` 对应文件中定义，再在组件中使用。
- 不允许在组件 scoped 块内"私自定义一套视觉体系"（与 §28 既有禁令一致，本节明确为可检测的量化规则）。

反面案例（motion-controller，2026-07-06 审查发现 30+ 处违规）：

- `MotionControlPanel.vue` 急停按钮硬编码 `#dc2626 / #b91c1c`，应改用 `var(--accent-danger)`。
- `UiButton.vue` 按钮白字硬编码 `color: #ffffff`，应新增 `--color-brand-foreground` token。
- `MotionControllerConfig.vue` 遮罩层硬编码 `rgba(0,0,0,0.5)`，应新增 `--scrim-backdrop` token。
- `ToastOverlay.vue` 12 处 `var(--accent-info, #38bdf8)` 写法，fallback 色与 token 重复，应删除 fallback。
- `styles/glass.css` 全文 30 行硬编码 rgba，应抽 `--shadow-panel` / `--blur-panel` token。

## 29. 资源管理

图标管理：

- 所有图标统一从 `components/icons/` 引用，禁止散落 SVG 内联
- 优先用图标库（`@lucide/vue`），自定义图标抽组件
- 图标命名 `IconXxx.vue`（如 `IconConnect.vue`、`IconStop.vue`）
- 禁止在 `<template>` 内联大段 SVG path（>3 行 path 抽组件）

图片管理：

- 图片资源放 `assets/images/`，用 `import` 引入而非 URL 字面量
- 禁止 `<img src="/assets/xxx.png">` 这种 public 路径访问（除字体等静态资源）
- 图片必须指定 `width` / `height` 防布局抖动（见 §24）
- 大图片用 `loading="lazy"`

字体管理：

- 字体文件放 `assets/fonts/`
- `@font-face` 定义集中在 `styles/typography.css`
- 字体加载用 `font-display: swap`，禁止阻塞渲染
- 中文字体必须 subset 子集化，禁止全量加载

静态资源版本控制：

- `define` 注入版本号（`__APP_VERSION__`），便于排查
- 资源 URL 加版本查询参数（`?v=1.2.3`），避免缓存问题
- 在 `vite.config.ts` 中配置 `define: { __APP_VERSION__: JSON.stringify(version) }`

## 30. 构建配置

Vite 配置：

- `build.target` 至少 `es2020`（工业工具型应用不考虑旧浏览器）
- `build.sourcemap` 生产环境关闭（或用 hidden sourcemap 上传到错误监控）
- `build.chunkSizeWarningLimit` 设置合理阈值（默认 500KB 偏小，工业应用可设 1000KB）
- `build.rollupOptions.output.manualChunks` 拆分 vendor（vue、wails、chart 库独立 chunk）

依赖外部化：

- Wails runtime 不打包进 bundle，外部化 `@wailsapp/runtime`
- 大依赖（chart.js、monaco-editor）按需加载或外部化
- `optimizeDeps.exclude` 排除 Wails binding

`define` 注入：

- `__APP_VERSION__`：版本号
- `__BUILD_TIME__`：构建时间
- `__DEV__`：开发环境标记（替代 `process.env.NODE_ENV`）

环境变量：

- 用 `import.meta.env.VITE_XXX`，禁止 `process.env`（Vite 标准）
- 敏感变量（API key 等）不通过 Vite 注入，运行时从后端获取
- `.env.development` / `.env.production` 分环境配置

HMR 配置：

- 开发模式 `server.hmr` 配置正确，避免端口冲突
- Wails 项目 HMR 端口与 Go 后端不冲突
- 大文件改动（如 i18n 字典）触发 full reload 而非 HMR

构建产物检查：

- 构建后必须报告 bundle 大小（`vite-plugin-bundle-analyzer` 或 `--report`）
- 主 chunk 不超过 500KB（gzip 后），超出必须拆分
- 单文件依赖不超过 200KB

## 31. 面向对象与领域模型

前端偏函数式，但领域模型 / 协议适配 / 复杂状态仍需 OOP。

何时用 class vs interface vs type：

| 场景 | 选择 | 示例 |
|---|---|---|
| 领域模型有行为 | `class` | `class DeviceProfile { connect() {...} disconnect() {...} }` |
| 数据契约（无行为） | `interface` | `interface DeviceResponse { id: string; status: number }` |
| 联合类型 / 字面量 | `type` | `type DeviceStatus = 'idle' \| 'connecting' \| 'connected'` |
| 函数式工具 | `function` | `formatDuration(ms: number): string` |
| 可复用逻辑（Vue） | composable | `useMotionHistory()` |

封装规则：

- 内部状态用 `private` 字段，外部通过方法访问
- 必要的属性暴露用 `get` / `set`，而非直接 public 字段
- 不可变值用 `readonly` 字段或 getter only
- 防御性拷贝：返回内部数组 / 对象时返回副本，避免外部修改

继承规则：

- 继承层级 ≤ 2（祖父 → 父 → 子，禁止更深）
- 优先组合而非继承（`Has-A` 优于 `Is-A`）
- 子类不能削弱父类契约（里式替换）
- 抽象类用于模板方法模式，子类实现具体步骤

SOLID 在前端的落地：

- **S**（单一职责）：组件 / composable / store / class 各司其职，禁止一个组件做列表+表单+图表
- **O**（开闭原则）：通过 props 扩展，而非修改组件源码加 if/else 分支
- **L**（里式替换）：子组件不能要求比父组件更严格的 props，不能拒绝父组件能处理的 events
- **I**（接口隔离）：Props 拆分到最小可用，禁止"胖 Props"（一个组件 props 超过 10 个时拆分）
- **D**（依赖倒置）：组件依赖 composable 接口（约定返回值），不依赖具体实现；store 依赖 port 接口而非具体 adapter

领域模型有行为（数据 + 行为绑定）：

```typescript
// 不好：数据和操作分离，函数散落各处
interface Device { id: string; status: DeviceStatus }
function connectDevice(d: Device) { ... }
function disconnectDevice(d: Device) { ... }
function getDeviceStatus(d: Device) { ... }

// 好：数据 + 行为绑定
class Device {
  constructor(
    readonly id: string,
    private status: DeviceStatus = 'idle',
  ) {}

  async connect(): Promise<void> { ... }
  async disconnect(): Promise<void> { ... }
  getStatus(): DeviceStatus { return this.status }
}
```

不可变值对象：

- 配置 / Profile 等数据用 `readonly` + `Object.freeze` 或 `as const`
- 修改必须返回新对象（`{ ...profile, name: 'new' }`），禁止原地改
- 用 zod / valibot 的 `.readonly()` 强制不可变

枚举与字面量类型：

- 状态值用 enum 或联合字面量，禁止裸数字 / 裸字符串
- enum 必须显式值（`enum Status { Idle = 'idle' }`），不依赖自增数字
- 联合字面量优于 enum（tree-shaking 友好、零运行时）

```typescript
// 推荐
type DeviceStatus = 'idle' | 'connecting' | 'connected' | 'error'

// 也可
enum DeviceStatus {
  Idle = 'idle',
  Connecting = 'connecting',
  Connected = 'connected',
  Error = 'error',
}

// 禁止
const STATUS_IDLE = 0
const STATUS_CONNECTING = 1
```

## 32. Wails 前后端桥接规范

错误映射：

- Go error 必须在前端映射为可读消息，禁止直接显示 `err.message`
- 错误分类：连接错误 / 超时错误 / 业务错误 / 权限错误，每类有对应 UI 反馈
- 集中映射在 `api/wails-adapter.ts` 或 `api/error-mapper.ts`，禁止散落各组件

```typescript
// 集中错误映射示例
function mapWailsError(err: unknown, context: string): UserError {
  const msg = err instanceof Error ? err.message : String(err)
  if (msg.includes('TCP timeout')) return { type: 'network', message: i18n.t.networkTimeout }
  if (msg.includes('N05')) return { type: 'device', message: i18n.t.deviceRejected }
  return { type: 'unknown', message: i18n.t.unknownError }
}
```

loading 状态保护：

- 后端调用必须配 loading 状态（`isLoading.value = true` 在 try，`finally` 置 false）
- loading 期间禁止重复触发（按钮 disabled 或防抖）
- 长 loading（>2s）必须显示进度提示

超时处理：

- 所有 Wails 调用必须有超时（默认 10s，长操作 30s）
- 用 `Promise.race` 或 AbortController 实现
- 超时后必须取消操作并反馈用户，禁止悬挂 Promise

批量调用优化：

- 多个独立后端调用用 `Promise.allSettled` 并行，禁止串行 await
- 部分失败不阻塞其他成功项（`allSettled` 而非 `all`）
- 失败项单独反馈，不整体失败

事件订阅规范：

- `Events.On('xxx', cb)` 必须保存返回的取消函数
- 取消函数在 `onBeforeUnmount` 调用（见 §26）
- 高频事件（>10Hz）必须节流后再更新 UI（见 §24 性能规则）

数据序列化：

- Go struct 字段名必须 JSON tag（`field:"name"`），前端访问用 tag 名
- 大数据传输用 `Uint8Array` 或 `ArrayBuffer`，禁止 base64 字符串
- 时间戳统一用 `number`（Unix 毫秒）或 `string`（RFC3339），禁止 `Date` 跨边界

### 32.1 Wails 绑定同步（强制，零容忍）

**修改任何被 Wails binding 暴露给前端的方法签名后，必须立即运行 `wails3 generate bindings` 重新生成 `frontend/bindings/`。** 这是 Go 与 JS 之间的运行时桥，不会自动跟随 Go 代码变化。

触发场景包括但不限于：

- 改方法参数数量、类型、顺序
- 改方法返回值类型
- 改结构体字段（被 binding 暴露的 Go struct）
- 新增/删除/重命名被 binding 暴露的方法

操作步骤：

1. 改 `apps/desktop-wails/backend/app.go` 或被 binding 引用的 Go 代码后
2. 在 `apps/desktop-wails` 目录运行：
   ```powershell
   wails3 generate bindings -silent
   ```
   （默认生成 `.js`；如需 TypeScript 加 `-ts`）
3. 检查 `frontend/bindings/.../app.js` 中对应方法的签名已更新
4. 重新运行 `npm run typecheck` 和 `npm run build`

**禁止**：把 typecheck/build/test 全绿当成 binding 已同步的证明。原因：

- `wails-adapter.ts` 用 `@ts-expect-error` 动态 `import('...bindings/.../app.js')`，TypeScript 看不到运行时签名不匹配
- `vite build` 不校验运行时
- `vitest` 走 HTTP mock，不经过 Wails binding

Wails binding 错位的典型症状：运行时报 `expects N arguments, got M`、参数被当字符串传入 Go 后反序列化失败、undefined 被当成合法参数导致 Go 端逻辑错乱。

经验教训：2026-06-30 改 `StorageStartRecording` 从 `(outputDir, filePrefix)` 改为 `(config StorageRecordingConfig)` 时漏了重新生成 binding，导致用户在桌面应用点"开始记录"时撞到 `expects 1 arguments, got 2` 错误。typecheck、build、40 个测试全过，但都没触到这层缝隙。

## 33. 测试规范

测试文件命名与位置：

- 单元测试：`*.test.ts` 或 `*.spec.ts`，与被测文件同目录（co-located）
- 集成测试：`tests/integration/` 目录
- E2E 测试：`tests/e2e/` 目录（用 Playwright）
- 测试文件名与被测文件同名：`motionStore.ts` → `motionStore.test.ts`

测试覆盖率门禁：

- 工具函数（`shared/`、`utils/`）覆盖率 ≥ 90%
- store actions 覆盖率 ≥ 80%
- composable 覆盖率 ≥ 70%
- 组件渲染逻辑覆盖率 ≥ 60%
- 整体覆盖率 ≥ 70%（具体阈值按项目阶段调整）

测试内容要求：

- store 必须测：state 初始值、关键 actions 成功/失败路径、getters 派生正确性
- composable 必须测：返回值响应式、cleanup 行为、边界条件
- 纯函数必须测：正常输入、边界输入（0/空/null/极大值）、异常输入
- 组件测试用 `@vue/test-utils`，测用户交互而非内部实现

mock 规则：

- API 调用必须 mock（`vi.mock('@/api/xxx')`）
- Wails binding 必须 mock（`vi.mock('@/api/wails-adapter')`）
- 时间相关必须用 `vi.useFakeTimers()`，禁止 `setTimeout` 真实等待
- DOM 必须用 `jsdom` 或 `happy-dom`，禁止依赖真实浏览器

禁止：

- 测试依赖执行顺序（`test.beforeAll` 设置被其他 test 依赖）
- 测试间共享可变状态
- `expect(true).toBe(true)` 这种无意义断言凑覆盖率
- 跳过失败测试（`test.skip`）而不修

## 34. 验证要求

前端改动后至少运行目标项目的：

- `npm run typecheck`
- `npm run build`

有测试时运行：

- `npm run test`

结构合规验证（修改 `.vue` / `.ts` / 样式文件后必须运行）：

- `powershell -File scripts/validate-frontend-structure.ps1 -ProjectDir "projects/<name>/apps/desktop-wails/frontend/src" -CheckFileSize`
  - 检查 §28.1 文件体量阈值（单文件行数 / script setup 行数 / scoped 行数 / 本地函数数 / store 行数 / i18n 行数）
  - 检查 §28.2 硬编码颜色（scoped 块内 hex / rgba / hsl / color-mix fallback / 固定 blur 值）
  - 检查跨组件重复 CSS class
  - 检查 composable 缺失（store ≥ 4 或业务组件 ≥ 6 时未建 `composables/` 目录）
- `powershell -File scripts/check-naive-imports.ps1 -ProjectDir "projects/windlabx4/apps/desktop-wails/frontend/src"` — windlabx4 项目专用，防止直接 naive-ui 导入

涉及布局、视觉、响应式时，还需要人工或浏览器截图验证：

- 目标桌面窗口
- 项目规定的最小窗口
- 空状态
- 错误状态
- loading 状态
- 长文本状态

若命令无法运行，AI 必须在最终说明中报告原因和未验证风险。

## 35. AI 友好的代码组织

可搜索性（grep 友好）：

- 命名唯一且语义化，避免通用词：`data` / `info` / `manager` / `temp` / `obj` / `item`
- 同义概念全项目统一一个词：连接用 `connect` 就别混用 `link` / `attach` / `bind`
- 缩写限制：仅通用缩写允许（`API` / `URL` / `ID` / `CSV` / `JSON`），业务缩写必须注释（如 `PR` = Position Register，注释一次）
- 函数名包含对象 + 动作：`connectDevice` 优于 `connect`，`loadMotionProfiles` 优于 `load`

文件路径可推断：

- 从符号名能推断文件位置（约定优于配置）：
  - `useXxx` → `composables/useXxx.ts` 或 `components/<domain>/composables/useXxx.ts`
  - `xxxStore` → `stores/xxxStore.ts`
  - `XxxComponent` → `components/<domain>/XxxComponent.vue`
  - `XxxButton` → `components/ui/XxxButton.vue`
  - `formatXxx` / `parseXxx` → `utils/format.ts` 或 `utils/parse.ts`
- 改名必须同步改文件名，避免符号名与文件名不一致

结构化标签（便于 AI 检索）：

- `TODO` / `FIXME` / `NOTE` / `HACK` / `WARN` 必须按 §20 格式
- 必须带 owner，AI 可定位责任人
- 标签后紧跟简短描述（≤80 字符），详细说明另起一行

单文件单一主题：

- 一个文件 export 一个主要概念
- 复杂文件顶部 1-3 行说明意图、配套组件、关键约束（见 §10）
- AI 阅读时能从文件名 + 顶部注释快速判断是否相关

变更追溯：

- public API / store action 标注 owner（JSDoc `@author` 或 `@owner`）
- deprecated 流程：先标 `@deprecated` 注释 + 替代方案，至少一个版本周期后才删除
- breaking change 必须在 CHANGELOG 显式记录，加 `BREAKING:` 前缀
- 实验 API 加 `@experimental` 标签

避免隐式约定：

- 避免看上下文才能理解的代码（如魔术字符串 `'connected'` 散落各处，必须抽常量或类型）
- 显式优于隐式：函数返回类型必须显式标注，不依赖推断
- 配置项有默认值，禁止"未配置就崩"
- 避免副作用：函数名暗示纯查询（`getXxx` / `loadXxx`）就不应改状态

AI 阅读友好格式：

- 长函数用注释分块（`// ---- 校验 ----` / `// ---- 执行 ----` / `// ---- 清理 ----`）
- 复杂条件用命名变量替代内联表达式（`if (isDeviceReady && hasPermission)` 优于 `if (device.status === 'idle' && user.role === 'admin')`）
- 数据流方向清晰：state → computed → action → state，避免双向耦合
- 关键决策点注释解释"为什么"（见 §20）

避免反 AI 模式：

- 禁止动态生成函数名 / 变量名（`obj['fn' + i]()` 无法静态分析）
- 禁止过度使用高阶函数嵌套（`compose(f, map(g), filter(h))` 难以追踪）
- 禁止隐式全局（依赖未声明的全局变量）
- 禁止语义化弱的命名（`const x = ...; doSomething(x)` 看不出 x 是啥）

约定文件位置（便于 AI 一次定位）：

| 类型 | 位置 | 示例 |
|---|---|---|
| 业务领域类型 | `shared/types/<domain>.ts` | `shared/types/motion.ts` |
| 项目内类型 | `src/types/` 或 `components/<domain>/types.ts` | `src/types/wails.d.ts` |
| 跨项目工具 | `shared/utils/` | `shared/utils/format.ts` |
| 项目内工具 | `src/utils/` | `src/utils/clipboard.ts` |
| 常量 | `src/constants/` 或文件顶部 | `src/constants/axis-defaults.ts` |
| 配置 | `src/config/` | `src/config/theme.ts` |
