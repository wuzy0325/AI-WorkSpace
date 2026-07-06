# AI 前端开发规则 · 状态与 UI 组件

> 本文是 `frontend-ai-rules.zh-CN.md` 拆分后的**状态与 UI 组件**部分（§11-§17）。
> 其他部分：
> - 基础架构（§1-§10）：[frontend-ai-rules-foundation.zh-CN.md](frontend-ai-rules-foundation.zh-CN.md)
> - 状态与 UI 组件（§11-§17）：[frontend-ai-rules-state.zh-CN.md](frontend-ai-rules-state.zh-CN.md)
> - 编码质量（§18-§27）：[frontend-ai-rules-quality.zh-CN.md](frontend-ai-rules-quality.zh-CN.md)
> - 样式/集成/验证（§28-§35）：[frontend-ai-rules-deploy.zh-CN.md](frontend-ai-rules-deploy.zh-CN.md)
> 完整目录与章节级精读协议见 [frontend-ai-rules.zh-CN.md](frontend-ai-rules.zh-CN.md)。

## 11. 状态完整性规则

重要界面必须覆盖以下状态中的适用项：

- loading
- empty
- error
- disabled
- selected
- dirty
- saving
- saved
- offline
- reconnecting
- permission denied

设备、采集、记录、移动、校准类操作必须显示当前状态，不能只靠按钮文本暗示。

错误反馈必须说明原因和下一步。禁止只显示"失败"。

### 11.1 共享 Store 边界规则

跨页面共享的 store（例如设备、运动控制器、采集、校准、遍历、存储）必须把"业务资产状态"和"加载错误状态"分开维护。

必须遵守：

- 共享 store 的刷新函数只能在后端明确成功返回时替换业务资产列表。
- 临时异常、网络失败、Wails binding 失败、状态查询失败不得清空已有业务资产。
- `[]` 只能表示后端明确返回空列表；异常不能被转换成空列表。
- 失败分支必须保留上次成功状态，并通过 `loading` / `error` / toast / banner 等可见状态暴露原因。
- 删除、清空、重置等破坏性状态变化必须来自显式用户动作或明确后端成功响应，不能由"刷新失败"隐式触发。
- 多个模块初始化时，彼此独立的加载任务应优先使用 `Promise.allSettled` 或等价容错流程，避免设备列表刷新失败拖垮校准、遍历、存储等其他配置加载。
- 共享 store 的失败语义必须有单测覆盖，至少验证"失败保留旧状态"和"成功清除错误状态"。

禁止：

- 在 `catch` 中把共享业务列表直接置为 `[]`、默认空对象或模拟数据，除非这是用户确认的重置动作。
- 让功能页面通过调用共享刷新函数间接影响其他页面的核心列表状态。
- 用一个全局 `loading` 或 `error` 混淆多个独立资源的加载状态。

## 12. 响应式数据规范

`ref` vs `reactive` 选择规则：

- 原始类型（`string` / `number` / `boolean`）必须用 `ref`
- 对象且不需要整体替换（`state = newState`）用 `reactive`
- 对象需要整体替换必须用 `ref<T>({})` + `.value = newState`
- 数组用 `ref<T[]>([])`，禁止用 `reactive<T[]>([])`（替换时会有响应性陷阱）

禁止解构 `reactive` 失去响应性：

- `reactive` 对象解构后失去响应性，必须用 `toRefs` 或 `toRef`
- 函数返回 `reactive` 对象时，调用方解构必须用 `storeToRefs`（Pinia）或 `toRefs`

大对象与性能：

- 大于 1000 元素的数组用 `shallowRef`，避免深度响应式开销
- 树形结构（深度 > 3）用 `shallowRef` + 手动触发更新
- 频繁更新的对象（如 1000Hz 数据流）用 `shallowRef` + `triggerRef`

`computed` 规则：

- `computed` 必须无副作用，禁止在内部修改其他响应式状态
- `computed` 必须可缓存（依赖响应式数据），禁止依赖非响应式外部变量
- 复杂计算（>5 行）抽到独立函数，computed 仅做派生
- `computed` 写入（`set`）必须谨慎，优先用 `watch` 替代

`watch` 规则：

- `watch` 必须指定 `immediate` / `deep` 选项，不依赖默认值
- `watch` 副作用必须用 `onCleanup` 清理上一次（如防抖、异步取消）
- 监听 `reactive` 对象属性必须用 getter 函数（`() => state.foo`），禁止 `watch(state, ...)` 全对象深度监听
- `watchEffect` 仅用于依赖自动收集的简单场景，复杂依赖用 `watch`

模板内禁止复杂计算：

- 模板内禁止 `{{ }}` 表达式调用方法（`{{ formatValue(item.value) }}`），必须用 `computed`
- 模板内禁止内联 `Math.round(x * 100) / 100` 这种计算，必须抽 `computed` 或 `formatter` 函数
- `v-if` 条件禁止超过 3 个逻辑运算符（`a && b && c && d`），必须抽 `computed`

## 13. Provide/Inject 与依赖注入

类型化强制要求：

- 必须用 `InjectionKey<T>` 定义 injection key，禁止 `inject('xxx')` 字符串无类型
- key 定义放独立文件 `src/keys.ts` 或 `components/<domain>/keys.ts`
- `provide` 与 `inject` 必须引用同一个 key 常量，禁止重复字面量

```typescript
// 正确写法
import type { InjectionKey } from 'vue'
import { provide, inject } from 'vue'

interface TooltipApi {
  show: (text: string, e: MouseEvent) => void
  hide: () => void
}
export const TooltipKey: InjectionKey<TooltipApi> = Symbol('tooltip')

// 上层
provide(TooltipKey, { show: showTooltip, hide: hideTooltip })

// 下层
const tooltip = inject(TooltipKey)
if (!tooltip) throw new Error('TooltipKey not provided')
```

默认值与可选性：

- `inject` 必须给默认值（`inject(Key, defaultValue)`）或显式 `undefined`
- 必须处理 `undefined` 情况（throw 或 fallback），禁止直接调用可能 undefined 的方法
- 可选依赖在文档中标注（注释 `// optional: 仅在 X 场景下 provide`）

禁止：

- `inject` 字符串 key（`inject('showTooltip')`）
- `inject` 后不类型守卫直接使用
- 跨层级 `provide`（祖父 → 孙子跳过父级），必须层层传递或用 Pinia store

依赖方向：

- 通用 UI 组件禁止 `provide` 业务对象
- 业务组件 `provide` 仅给自己子组件用，禁止跨域 provide
- 跨域共享必须用 Pinia store，不用 provide/inject

## 14. Pinia store 间依赖

依赖方向规则：

- store 之间禁止循环依赖（A 用 B 的 action，B 不能再用 A 的 action）
- 业务 store（如 `motionStore`）可依赖基础设施 store（如 `feedbackStore`），反向禁止
- 跨业务域 store 依赖必须通过组合而非直接调用（在 usecase 层组合）

state 修改规则：

- 禁止直接修改其他 store 的 state（`otherStore.foo = bar`），必须走其 actions
- 禁止在 `getter` 中调用其他 store 的 action（getter 必须无副作用）
- 跨 store 同步状态必须用 actions 显式调用，禁止隐式 watch 联动

store 风格：

- 全工作空间统一用 setup 风格：`defineStore('name', () => { ... })`
- 禁止 options 风格：`defineStore('name', { state, actions, getters })`
- ref/computed/function 在 setup 内定义并 return，外部通过 store 实例访问

store 文件组织：

- 一个 store 一个文件，文件名 `xxxStore.ts`
- store 内禁止导入其他 store 的内部 ref / computed（必须用 `useOtherStore()` 实例化）
- 跨 store 共享类型放 `shared/types/`，禁止 store 间互相 import 类型造成循环

actions 错误处理：

- actions 内 try/catch 后必须 rethrow 或转为 feedbackStore 错误反馈
- 禁止 actions 吞掉错误（仅 console.error 不反馈用户）
- actions 返回值必须明确（`Promise<boolean>` 表示成功失败，或 `Promise<T | null>`）

## 15. 状态机与复杂状态管理

多布尔组合禁令：

- 同一对象有 ≥3 个布尔字段表示状态时，必须重构为状态机
- 反例：`{ isLoading, isLoaded, isError, isSuccess }` 互相排斥却用 4 个布尔
- 正例：`{ status: 'idle' | 'loading' | 'success' | 'error' }` 单一字段

显式状态机：

- 复杂状态用状态机模式（如设备连接：idle → connecting → connected / error → disconnected）
- 状态转换必须走 action，禁止外部直接改 status 字段
- 非法转换必须 throw 或 warning（如 connected → connecting 不允许）

```typescript
// 显式状态机示例
type ConnectionState = 'idle' | 'connecting' | 'connected' | 'error' | 'disconnecting'

const VALID_TRANSITIONS: Record<ConnectionState, ConnectionState[]> = {
  idle: ['connecting'],
  connecting: ['connected', 'error', 'idle'],
  connected: ['disconnecting', 'error'],
  error: ['idle'],
  disconnecting: ['idle'],
}

function transition(from: ConnectionState, to: ConnectionState): ConnectionState {
  if (!VALID_TRANSITIONS[from].includes(to)) {
    throw new Error(`Invalid transition: ${from} -> ${to}`)
  }
  return to
}
```

状态来源单一：

- 同一数据不要存两个地方（如 device 状态既在 store 又在组件 ref）
- 派生数据用 computed，不用 watch 同步
- 服务端数据缓存归 store 管，组件不持有副本

状态归一化：

- 列表数据用 Map / Record keyed by id，而非数组（查找 O(1)）
- 关联数据用 id 引用，而非嵌套对象（避免数据冗余）
- 派生数据不存储，computed 实时计算

复杂表单状态：

- 表单状态用 `reactive` 集中管理，禁止散落多个 ref
- dirty 检测用 `JSON.stringify(original) !== JSON.stringify(current)`
- 提交前必须校验 + sanitize（移除空字段、规范格式）
- 取消编辑必须能恢复原值，禁止"取消后字段空了"

撤销 / 重做：

- 复杂编辑场景（如 Profile 编辑）支持 undo/redo
- 用 history stack（每次提交存 snapshot）
- 限制 history 深度（如 50 步），避免内存膨胀

## 16. 表单规则

表单必须满足：

- 字段标签清晰
- 单位明确
- 默认值合理
- 错误提示指向具体字段
- 提交中禁止重复提交
- 保存成功或失败有反馈

禁止：

- 隐藏单位
- 前端伪造后端校验规则
- 把业务限制硬编码在组件里
- 表单提交失败后丢失用户输入

## 17. 图表和实时数据规则

实时数据界面必须：

- 显示连接状态
- 显示采集状态
- 显示数据更新时间或 freshness
- 处理空数据
- 处理断连
- 处理异常值
- 避免高频刷新导致 UI 卡顿

原始数据在前端仅做展示和可视化，禁止在前端做滤波、重采样、插值、协议解码等信号处理。此类逻辑必须由 Go 后端处理。

大数据列表或曲线必须考虑：

- 节流
- 采样
- 虚拟滚动
- 固定尺寸容器
- 避免布局抖动
