# AI 前端开发规则 · 编码质量

> 本文是 `frontend-ai-rules.zh-CN.md` 拆分后的**编码质量**部分（§18-§27）。
> 其他部分：
> - 基础架构（§1-§10）：[frontend-ai-rules-foundation.zh-CN.md](frontend-ai-rules-foundation.zh-CN.md)
> - 状态与 UI 组件（§11-§17）：[frontend-ai-rules-state.zh-CN.md](frontend-ai-rules-state.zh-CN.md)
> - 编码质量（§18-§27）：[frontend-ai-rules-quality.zh-CN.md](frontend-ai-rules-quality.zh-CN.md)
> - 样式/集成/验证（§28-§35）：[frontend-ai-rules-deploy.zh-CN.md](frontend-ai-rules-deploy.zh-CN.md)
> 完整目录与章节级精读协议见 [frontend-ai-rules.zh-CN.md](frontend-ai-rules.zh-CN.md)。

## 18. TypeScript 严格性规范

`tsconfig.json` 必须开启严格模式：

- `"strict": true`
- `"noImplicitAny": true`
- `"strictNullChecks": true`
- `"noUnusedLocals": true`
- `"noUnusedParameters": true`
- `"noImplicitReturns": true`
- `"noFallthroughCasesInSwitch": true`
- `"forceConsistentCasingInFileNames": true`

类型注解强制要求：

- 函数参数与返回值必须显式标注类型（除了一行的 arrow function 可推断）
- `ref<T>()` 必须显式泛型，禁止依赖初值推断（`ref('')` 推断为 `Ref<string>` 是反模式）
- `reactive<T>()` 必须显式 interface 类型
- Props 与 Emits 必须用 `defineProps<T>()` / `defineEmits<T>()` 泛型写法，禁止运行时对象声明
- `computed<T>()` 复杂计算必须显式泛型

禁止的写法：

- `: any` — 必须用 `unknown` + 类型守卫，或定义具体类型
- `as any` — 必须用 `as unknown as T` 显式标注不安全断言并加注释说明原因
- `as` 任意断言（除 `as const`、`as unknown as T`）— 必须用类型守卫或类型谓词
- `@ts-ignore` — 完全禁用，必须用 `@ts-expect-error` 并附注释说明预期错误
- `@ts-expect-error` 滥用 — 必须说明为何不能修复类型，且仅用于第三方类型缺失
- `// eslint-disable-next-line` 滥用 — 必须说明为何不能修复

类型定义位置：

- 跨文件复用类型放 `shared/types/` 或 `src/types/`
- 单组件内部类型放 `<script setup>` 顶部或 `components/<domain>/types.ts`
- 禁止在 `index.ts` 中重新导出类型造成循环依赖

第三方类型缺失处理：

- 优先创建 `src/types/<package>.d.ts` 声明模块
- 不要在业务代码用 `as any` 绕过，必须集中到 `.d.ts` 文件
- Wails binding 缺类型时，参考 §32.1，禁止在 `wails-adapter.ts` 之外用 `@ts-expect-error`

## 19. 命名规范

文件命名：

| 类型 | 规则 | 示例 |
|---|---|---|
| `.vue` 组件 | PascalCase | `MotionControlPanel.vue`、`UiButton.vue` |
| `.ts` 模块 | kebab-case | `motion-api.ts`、`http-client.ts` |
| `.ts` 类型/接口 | PascalCase | `Motion.ts`、`Calibration.ts` |
| composable | camelCase，`use` 开头 | `useAxisLimits.ts`、`useMotionHistory.ts` |
| store | camelCase，`use...Store` | `motionStore.ts`、`feedbackStore.ts` |
| constants | kebab-case 或 UPPER_SNAKE | `axis-defaults.ts` 或 `AXIS_DEFAULTS.ts` |
| 测试 | `*.test.ts` / `*.spec.ts` | `motionStore.test.ts` |
| 类型声明 | `*.d.ts` | `env.d.ts`、`wails.d.ts` |

变量命名：

- 布尔值前缀：`is` / `has` / `can` / `should` / `will`（如 `isRecording`、`hasError`、`canConnect`）
- 事件 handler 前缀：`on` 或 `handle`（如 `onConnectClick`、`handleSave`），全项目统一一种
- 异步函数后缀：`Async` 或显式 `Promise<T>` 返回类型（如 `loadProfilesAsync`）
- 常量：`UPPER_SNAKE_CASE`（如 `MAX_HISTORY = 50`）
- 私有：前缀 `_`（如 `_internalState`），或用 TypeScript `private` 关键字
- ref 解包后变量不加 `Ref` 后缀（`const isOpen = ref(false)`，禁止 `isOpenRef`）

Props 与 Emits 命名：

- Props 用 camelCase 声明，模板用 kebab-case 传入（`defineProps<{ showConfig: boolean }>()` ↔ `<Comp :show-config="true">`）
- Emits 用 kebab-case 事件名（`defineEmits<{ 'close': []; 'saved': [id: string] }>()`）
- 双向绑定事件名用 `update:modelValue`，禁止自定义 `change` 事件

CSS class 命名：

- BEM 风格：`block__element--modifier`（如 `axis-card__header--active`）
- 工具类用 Tailwind 或 utility class，不与 BEM 混用
- 状态类前缀 `is-` / `has-`（如 `is-loading`、`has-error`）

## 20. 注释规范与代码自文档化

注释原则：**注释解释"为什么"，代码解释"是什么"**。

必须注释的场景：

- 业务规则来源（如 `// 客户要求 CH17 锁定 Pa，禁止用户修改（见需求文档 §3.2）`）
- workaround 与第三方 bug（如 `// wails v3 alpha.95 不会自动重生成 syso，必须手动跑（见 ADR-004）`）
- 魔法数字含义（如 `// 0x0C10 = 时间戳位 + 通道使能位，协议手册 P12`）
- 非显然的性能优化（如 `// 用 shallowRef 避免 1000Hz 数据流的深度响应式开销`）
- 临时禁用的代码（必须说明复启用条件，如 `// FIXME: 等 wails v3 正式版恢复此特性`）
- 公共 API / 工具函数的 JSDoc（参数、返回值、异常、示例）

禁止的注释：

- 复述代码的注释（如 `// 设置标题` 紧跟 `title.value = x`）
- 过时的注释（代码改了注释没改，比没注释更糟）
- 死代码注释（保留已删除代码"以备后用"，应依赖版本控制）
- 区域分隔注释（如 `// ============` 大段分隔，应拆函数或拆文件）

`<script setup>` 内部组织顺序（自上而下，便于 AI 与人类定位）：

1. imports
2. props / emits / defineModel
3. composable 调用（`useXxx()`）
4. store 实例化
5. 本地 ref / reactive state
6. computed
7. watch / watchEffect
8. 普通函数（按使用频率从高到低，或按数据流方向）
9. 生命周期钩子（onMounted / onBeforeUnmount）

标签格式（统一全工作空间）：

| 标签 | 含义 | 格式 | 示例 |
|---|---|---|---|
| `TODO` | 待办 | `// TODO(owner): desc` | `// TODO(wuzhy): 改为虚拟滚动` |
| `FIXME` | 待修缺陷 | `// FIXME(owner): desc` | `// FIXME(wuzhy): 多设备时时间戳错乱` |
| `NOTE` | 重要提示 | `// NOTE: desc` | `// NOTE: 设备时间戳固件 bug 见 §9` |
| `HACK` | 临时绕过 | `// HACK: desc` | `// HACK: wails v3 限制，alpha.96 后改` |
| `WARN` | 警告 | `// WARN: desc` | `// WARN: 此处不能加 \r\n，设备会拒绝` |

标签必须大写、紧跟 `//`，`owner` 是 SVN/Git 用户名或 team 名（如 `frontend`、`backend`）。无 owner 的 `// TODO` 禁止——避免无人认领。

JSDoc 示例：

```typescript
/**
 * 启动设备采集
 * @param deviceId - 设备 ID（来自 profile.devices[].id）
 * @param options - 采集参数，rate 单位 Hz
 * @returns 启动成功返回 true，设备已断连返回 false
 * @throws {DeviceError} 设备不支持此 rate 时抛出
 * @example
 * await startAcquisition('p1604-01', { rate: 1000 })
 */
async function startAcquisition(deviceId: string, options: AcqOptions): Promise<boolean> { ... }
```

## 21. 函数与复杂度量化规则

函数量化阈值（硬性，配合 `validate-frontend-structure.ps1 -CheckFileSize` 自动检测）：

| 维度 | 警告阈值 | 错误阈值 | 触发后必须做的事 |
|---|---|---|---|
| 函数总行数 | > 50 行 | > 80 行 | 拆分为子函数，每个子函数单一职责 |
| 函数参数数 | > 4 个 | > 6 个 | 用 options object 收纳相关参数 |
| 嵌套层级（if/for/while/switch） | > 3 层 | > 4 层 | 用 early return / 提取子函数 |
| 圈复杂度（分支数） | > 10 | > 15 | 拆函数或用查表法替代 switch |
| 回调嵌套（callback hell） | > 2 层 | > 3 层 | 改 async/await 或拆函数 |

函数单一职责：

- 函数名应能完整描述其行为（`loadAndRenderAndSaveProfile` 必须拆三个函数）
- 函数名动词 + 名词：`loadProfiles`、`connectDevice`、`startRecording`
- 布尔判断函数用 `is/has/can/should` 前缀：`isConnected`、`hasError`
- 禁止函数副作用超出其名（`getUserData` 不应触发 toast）

参数设计：

- 同类参数聚合成 object：`connect(host, port, timeout, retry)` → `connect({ host, port, timeout, retry })`
- 可选参数用 options object，不用多个 overload
- 回调函数放最后位置，或放 options 内
- 禁止 flag 参数（`render(data, true)` 看不懂 true 干嘛，拆两个函数）

控制流简化：

- 优先 early return / guard clause（先处理异常情况，主逻辑不嵌套）
- 用 `Array.find/filter/map` 替代 for + push
- 用 optional chaining 替代多层 if null（`a?.b?.c` 优于 `a && a.b && a.b.c`）
- 用 nullish coalescing 替代 `|| 0`（`a ?? 0` 优于 `a || 0`，因为 0 是合法值）

## 22. 错误处理规范

Promise 链规则：

- `async/await` 优于 `.then().catch()` 链
- 顶层 await（`onMounted` 内）必须用 `try/catch` 包裹
- `.then()` 链必须配 `.catch()`，禁止悬挂 Promise
- `.catch()` 块禁止空 body，至少 `console.error` 或转发到 `feedbackStore`

错误信息必须含上下文：

- 禁止 `throw new Error('失败')` 这种无信息错误
- 必须含：操作名 + 目标对象 ID + 原因（如 `connectDevice ${deviceId} failed: TCP timeout after 3000ms`）
- 用户可见错误必须经过 i18n 翻译，禁止直接 `toast.error(err.message)` 显示原始英文

错误分类与处理：

| 错误类型 | 处理方式 |
|---|---|
| 网络错误 / TCP 超时 | toast 提示 + 状态栏显示 + 自动重试（按业务规则） |
| 设备断连 | 更新 `deviceState.error` + toast + 自动重连（按 profile 配置） |
| 业务校验失败 | 表单字段级错误提示（不弹 toast） |
| 用户取消操作 | 静默处理，不报错 |
| Go 后端返回 error | 必须解析 error 字符串映射到前端可读消息 |

禁止：

- `catch (e: any) { console.error(e) }` 后吞掉错误不反馈用户
- `catch (e) {}` 空 catch 块
- 把 `unknown` 类型错误直接 `e.message` 访问（必须先 `e instanceof Error` 类型守卫）
- 在 `finally` 中做副作用（如 `loading = false`），应用 `try/catch/finally` 完整流程

错误边界（Vue 3）：

- 关键路由级组件用 `errorCaptured` 钩子捕获子组件异常
- 捕获后必须显示 fallback UI（错误占位 + 重试按钮），不能白屏

## 23. 输入校验与防御性编程

用户输入校验：

- 表单提交前必须校验（必填、格式、范围、长度）
- 校验失败必须字段级反馈，不弹 toast
- 数值输入必须校验范围（如 rate ∈ [1, 1000]）和类型
- 字符串输入必须校验长度上限（防 DoS）和字符集（如设备 ID 禁止中文）
- 文件路径输入必须校验合法性（禁止 `..` 跨目录、禁止特殊字符）

API 响应 schema 校验：

- 后端返回数据必须校验关键字段存在且类型正确
- 用 zod / valibot 或手写 type guard
- 校验失败必须显式处理（throw 或 fallback），不能 silent fail
- 第三方 API（如设备 SCPI 响应）必须解析后校验

```typescript
// 用 type guard 校验
function isDeviceResponse(v: unknown): v is DeviceResponse {
  return typeof v === 'object' && v !== null &&
    typeof (v as any).id === 'string' &&
    typeof (v as any).status === 'number'
}

const res = await fetchDevice()
if (!isDeviceResponse(res)) {
  throw new Error(`Invalid device response: ${JSON.stringify(res)}`)
}
```

null / undefined 边界：

- 优先用 `unknown` 替代 `any` 接收外部数据
- 必须显式处理 `null` / `undefined`，禁止 `!` 非空断言（除单元测试）
- 数组访问 `[0]` 必须检查 `length > 0`
- Map / Set 访问 `.get(key)` 必须检查 `has(key)` 或处理 undefined
- 可选链 `?.` 优于手动 if null 判断

浮点数与 NaN：

- 浮点比较禁止 `===`，用 `Math.abs(a - b) < epsilon`
- 计算结果必须检查 `Number.isNaN` / `Number.isFinite`
- 货币 / 计量精度场景用 decimal.js 或整数运算
- 显示用 `toFixed(n)` 必须先校验输入是合法数字

防御性边界检查：

- 数组下标越界必须显式处理
- 字符串 slice / substring 必须校验起始位置
- 正则 `match` 结果可能为 null，必须守卫
- `JSON.parse` 必须 try/catch
- `parseInt` / `parseFloat` 失败返回 NaN，必须检查

外部资源访问：

- 文件路径必须 normalize 后校验不越界（不超出允许的根目录）
- URL 必须校验 protocol（禁止 `file://` 等危险协议）
- 用户输入的 SQL / 命令必须转义（虽然前端通常不直接拼 SQL，但 Wails 调用可能传到后端拼接）
- 跨域资源必须显式 CORS 配置

## 24. 性能规范

`v-for` 规则：

- 必须 `:key` 绑定唯一稳定 ID，禁止用 `index` 作为 key
- key 必须在列表生命周期内不变（如 `item.id`，禁止 `Date.now()` 或随机生成）
- 列表项有子组件时，key 必须传到子组件（`<Item :key="item.id" :id="item.id">`）

`v-if` / `v-for` 互斥：

- 禁止同一元素同时用 `v-if` 和 `v-for`
- 必须用 `<template v-for>` 包裹 + 内部 `v-if`，或用 `computed` 过滤

大列表与表格：

- > 100 行的列表必须虚拟滚动（`vue-virtual-scroller` 或自研）
- > 50 列的表格必须横向虚拟滚动或固定列
- 列表项渲染时间 > 16ms 必须用 `v-memo` 缓存

`v-once` / `v-memo`：

- 静态内容（图标、标签、不变文本）用 `v-once`
- 列表项依赖部分属性变化时用 `v-memo="[item.id, item.selected]"`

`computed` 缓存：

- 同一派生计算在多处使用时必须 `computed`，禁止多处重复调用同一函数
- `computed` 依赖超过 5 个 ref 时考虑拆分

事件防抖与节流：

- 输入框 `@input` 必须防抖（200-300ms）
- 滚动 `@scroll` / 窗口 resize 必须节流（100ms）
- 拖拽 `@mousemove` 必须节流（16ms = 60fps）
- 高频数据流（设备 1000Hz 采样）必须用 `requestAnimationFrame` 批处理

Key 帧动画与过渡：

- 优先用 CSS `transform` / `opacity`，避免触发 layout（`width` / `height` / `top` / `left`）
- 长动画必须用 `will-change` 提示浏览器
- 动画结束后必须移除 `will-change`

图片与资源：

- 图片必须指定 `width` / `height` 属性或 CSS 尺寸，避免布局抖动
- 大图片用 `loading="lazy"`
- SVG 优先内联，复杂 SVG 抽组件

## 25. i18n 规范

key 命名规则：

- 嵌套点分命名：`motion.controller.title`、`calibration.five-hole.settings`
- 禁止扁平 key（`motionControllerTitle`），必须分组
- key 全小写 + 连字符（`five-hole`），禁止驼峰
- key 必须语义化，禁止 `key1` / `text_a` 这种无意义命名

禁止硬编码中文/英文字面量：

- `<template>` 内禁止直接写中文字符串，必须 `{{ i18n.t.xxx }}`
- `<script>` 内 `toast.error('连接失败')` 必须改 `toast.error(i18n.t.connectFailed)`
- 唯一例外：`console.error` / `console.warn` 调试日志可不翻译

locale 文件结构：

- 主 store 文件不内联字典字面量（见 §28.1 阈值）
- 拆分到 `src/locales/zh.ts` + `src/locales/en.ts`，主 store 仅做 locale 切换
- locale 文件必须类型对齐（用 `as const` 或共享 interface）
- 新增 key 必须同时更新两个 locale 文件，CI 校验 key 数量一致

缺失 key fallback：

- 必须有 fallback 机制：`i18n.t.missing || 'fallback text'` 或 store 内部兜底
- 开发模式必须 warning 提示缺失 key（`console.warn`）
- 生产模式必须回退到 key 本身或默认语言，禁止空白显示

变量插值：

- 复杂插值用函数：`i18n.t.deviceCount({ n: 3 })` → `"3 台设备"`
- 禁止字符串拼接：`i18n.t.deviceCount + count + i18n.t.units`

日期 / 数字格式化：

- 日期必须用 `Intl.DateTimeFormat` 或 dayjs，禁止手写 `YYYY-MM-DD`
- 数字必须用 `Intl.NumberFormat`，避免不同 locale 的小数分隔符差异

## 26. 生命周期与资源清理

强制清理规则：

- `onMounted` 中注册的副作用，必须在 `onBeforeUnmount` 清理：
  - `addEventListener` → `removeEventListener`
  - `setInterval` → `clearInterval`
  - `setTimeout`（>1s）→ `clearTimeout`（短定时器可豁免）
  - `requestAnimationFrame` → `cancelAnimationFrame`
  - `ResizeObserver` / `MutationObserver` / `IntersectionObserver` → `.disconnect()`
  - `EventSource` / `WebSocket` → `.close()`
  - 订阅（Pinia `$subscribe`、Wails `Events.On`、自定义 emitter）→ 取消订阅函数
- 禁止在 `setup` 顶层（生命周期外）创建未清理的副作用
- 禁止在 `watch` / `computed` 中创建副作用（`watch` 用 `onCleanup` 参数清理上一次副作用）

异常场景清理：

- 组件可能在 `onMounted` 后立即被销毁（条件渲染），所有注册代码必须保证可安全取消
- 异步操作完成后组件已卸载时，禁止继续更新 state（用 `onScopeDispose` 或 ref 标记）
- 弹窗/抽屉关闭时必须取消进行中的请求（用 AbortController）

反面案例：

- `MotionControlPanel.vue` 第 290-305 行 `onMounted` 注册了键盘监听和定时器，`onBeforeUnmount` 必须配对清理（已检查通过）
- 任何用 `Events.On('xxx', cb)` 订阅 Wails 事件，必须保存返回的取消函数并在 `onBeforeUnmount` 调用

## 27. 并发与竞态处理

竞态场景识别：

- 快速点击同一按钮（连接 / 启动 / 保存）
- 异步操作未完成时组件卸载
- 多个异步操作依赖同一资源
- 定时器与异步操作交错
- 事件订阅在异步操作期间触发

防护策略：

**按钮防抖**（防止快速重复点击）：

```typescript
// 不好：用户连点 5 次会发 5 个请求
async function connect() { ... }

// 好：loading 期间禁用按钮
const isConnecting = ref(false)
async function connect() {
  if (isConnecting.value) return
  isConnecting.value = true
  try { ... } finally { isConnecting.value = false }
}
```

**AbortController 取消**（异步操作可取消）：

```typescript
let abortController: AbortController | null = null

async function loadProfiles() {
  abortController?.abort()  // 取消上一次
  abortController = new AbortController()
  try {
    const res = await fetch('/api/profiles', { signal: abortController.signal })
    ...
  } catch (e) {
    if (e instanceof DOMException && e.name === 'AbortError') return  // 静默
    throw e
  }
}

onBeforeUnmount(() => abortController?.abort())
```

**Generation token**（防止旧响应覆盖新响应）：

```typescript
let generation = 0
async function search(keyword: string) {
  const myGen = ++generation
  const res = await api.search(keyword)
  if (myGen !== generation) return  // 我已被新请求取代，丢弃结果
  results.value = res
}
```

**stale closure 防护**：

- `setInterval` / `setTimeout` 回调闭包捕获旧值，必须用 ref 或 ref-like 机制
- 事件监听器闭包同上
- 解决：用 ref 存可变状态，闭包内 `state.value` 访问最新值

**Promise.allSettled 优于 all**：

- 多个独立异步操作必须用 `Promise.allSettled`，部分失败不阻塞
- 失败项单独处理，不整体 throw
- 禁止 `Promise.all` 用于"全部成功才算成功"以外的场景

**并发限制**：

- 同时发起的请求必须限制并发数（如设备扫描 256 个 IP，不能并发 256 个请求）
- 用 `p-limit` 或自研 semaphore
- 默认并发上限 8（浏览器 / Node 通用值）

禁止：

- 异步操作完成后组件已卸载仍更新 state（用 `onScopeDispose` 或 `isMounted` ref）
- 多个 watcher 互相触发形成无限循环（必须用 generation 或 flag 跳过）
- 在 `watch` 内同步触发自身依赖的更新（必须异步或加 guard）
