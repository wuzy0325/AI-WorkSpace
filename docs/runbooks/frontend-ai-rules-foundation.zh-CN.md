# AI 前端开发规则 · 基础架构

> 本文是 `frontend-ai-rules.zh-CN.md` 拆分后的**基础架构**部分（§1-§10）。
> 其他部分：
> - 基础架构（§1-§10）：[frontend-ai-rules-foundation.zh-CN.md](frontend-ai-rules-foundation.zh-CN.md)
> - 状态与 UI 组件（§11-§17）：[frontend-ai-rules-state.zh-CN.md](frontend-ai-rules-state.zh-CN.md)
> - 编码质量（§18-§27）：[frontend-ai-rules-quality.zh-CN.md](frontend-ai-rules-quality.zh-CN.md)
> - 样式/集成/验证（§28-§35）：[frontend-ai-rules-deploy.zh-CN.md](frontend-ai-rules-deploy.zh-CN.md)
> 完整目录与章节级精读协议见 [frontend-ai-rules.zh-CN.md](frontend-ai-rules.zh-CN.md)。

## 1. 适用范围

当任务涉及以下内容时，AI 必须加载并遵守本文：

- `apps/desktop-wails/frontend/` 下的 Vue 3 代码
- `shared/frontend/` 下的共享前端代码
- UI 布局、样式、控件、图表、表单、交互状态
- 前端 store、API client、Wails binding 调用封装
- 项目级 `DESIGN.md` 指定的视觉或布局规则

若本文与项目级 `DESIGN.md` 冲突，优先遵守项目级 `DESIGN.md`。

## 2. AI 执行流程

前端任务开始前，AI 必须先确认：

1. 当前项目是否存在 `AGENTS.md`、`CLAUDE.md`、`DESIGN.md`。
2. 是否已有同类组件、store、composable、API client、样式 token。
3. 修改属于通用 UI、界面模式、业务功能、页面组装、主题样式还是 API 调用。
4. 是否需要后端能力或 API 合约支持。
5. 是否会把业务规则、设备控制或算法错误地下沉到前端。

若后端能力不存在，AI 不得在前端伪造完整业务能力。可以保留可见入口，但必须明确显示 `disabled`、离线、未实现或能力缺口状态，并在说明中指出后端缺口。

## 3. 前端职责边界

前端只负责：

- 展示数据
- 用户交互
- 页面状态
- 表单输入状态
- 组件组合
- 图表与可视化
- 调用后端 API 或 Wails binding
- 将后端错误转化为用户可理解的界面反馈

前端禁止：

- 直接访问硬件
- 实现校准算法
- 实现采集控制核心逻辑
- 实现设备状态机
- 直接访问串口、TCP、UDP
- 直接读写本地文件系统
- 把 Go 后端业务规则复制到 Vue 组件中

判断规则：如果把 Vue UI 替换成 CLI、Web、移动端或其他前端后，这段逻辑仍然必须存在，它不属于前端，应放到 Go 后端。

## 4. 代码放置规则

优先遵守项目既有目录。新代码按以下规则放置：

- 通用基础控件：项目内 `components/ui/`（共享控件待 `shared/frontend/ui/` 建立后迁移）
- 复用界面模式：项目内 `components/patterns/`（共享模式待 `shared/frontend/patterns/` 建立后迁移）
- 业务功能组件：`features/<domain>/` 或项目内 `components/<domain>/`
- 页面级组装：`pages/`、`views/` 或项目内既有页面目录
- 主题、颜色、间距、圆角、动效、z-index：`design/`、`styles/tokens/` 或既有 token 目录
- API 调用封装：`api/` 或 feature-local service
- 跨组件业务状态：`stores/`
- 局部 UI 状态：组件内部

禁止：

- 通用 UI 组件依赖业务 store
- 通用 UI 组件出现设备、采集、校准、遍历等业务词
- 页面组件堆入大量业务流程
- 为了少传 props 把所有状态塞进全局 store
- 随手新增一套与既有结构冲突的目录体系

## 5. UI 设计规则

本工作空间默认是工业工具型桌面应用，设计优先级为：

1. 清晰
2. 稳定
3. 高信息密度
4. 状态可见
5. 操作低摩擦
6. 装饰克制

默认禁止生成：

- 营销式 hero 页面
- 大面积渐变背景
- 漂浮装饰卡片堆叠
- 无意义装饰图形
- 过大的展示标题
- 低密度宣传页布局
- 卡片套卡片

除非用户明确要求品牌页、宣传页、演示页或非工具型界面。项目级 `DESIGN.md` 明确要求的视觉效果不受此限制。

## 6. 控件使用规则

优先使用项目现有控件库和基础组件。AI 不得因为方便而混用多套控件体系。

使用控件时遵守：

- 布尔值：checkbox、switch、toggle
- 单选：radio、select、segmented control
- 多选：checkbox group、multi-select
- 数值：input number、stepper、slider
- 模式切换：tabs、segmented control
- 工具动作：图标按钮，并提供 tooltip 或 aria-label
- 危险操作：清晰文案、禁用条件、确认或撤销机制

禁止：

- 用一组普通文本块伪装控件
- 所有按钮都使用主按钮样式
- 图标按钮没有可理解标签
- 同一页面混用不同视觉风格的按钮、输入框和弹窗

## 7. 布局和响应式规则

桌面应用以桌面优先。除项目级规则另有说明，AI 不应擅自改成移动优先布局。

所有前端改动必须保证：

- 窗口缩小时不重叠
- 长文本不溢出
- 表格和面板可滚动
- 关键操作不被遮挡
- 弹窗在目标最小窗口内可操作
- 动态内容不会造成明显布局跳动

禁止：

- 文本覆盖按钮
- 工具栏被挤出不可见
- 固定宽度导致页面横向爆炸
- loading、hover 或错误文案改变控件尺寸

## 8. 可访问性规则

至少满足：

- 按钮有明确 label
- 图标按钮有 tooltip 或 aria-label
- focus 状态可见
- 错误不只靠颜色表达
- 禁用状态原因清楚
- 文本对比度足够
- 键盘可操作关键流程

### 8.1 焦点陷阱与 ARIA live regions

弹窗、抽屉、模态对话框必须实现焦点陷阱：

- 打开时焦点移至弹窗内第一个可交互元素
- Tab / Shift+Tab 在弹窗内循环，不聚焦到背景遮罩
- 关闭时焦点返回触发元素（last focused element before open）
- ESC 键关闭弹窗（除非显式声明不可关闭，如确认删除二次确认）
- `aria-modal="true"` 标记模态、`role="dialog"` 标记对话框

Toast 通知、错误提示、状态变化必须通过 ARIA live regions 主动播报：

- Toast 容器加 `aria-live="polite"`（非紧急）或 `aria-live="assertive"`（错误/告警）
- `aria-atomic="true"` 确保整个区域被读出
- 错误状态变化必须用 `role="alert"` 或 `aria-live="assertive"`
- 状态变化（如"录制中" → "已停止"）必须有可见文本，不仅靠图标颜色

`prefers-reduced-motion` 适配：

- 检测 `@media (prefers-reduced-motion: reduce)` 时禁用动画
- 至少禁用 `transition` / `animation` 中的 `transform` 与 `opacity` 之外的位移
- 工业工具型应用默认动效已克制，但仍需尊重用户系统偏好

## 9. 模块边界与依赖方向

前端依赖方向（与 hexagonal 后端对齐）：

```
views/  →  components/<domain>/  →  composables/  →  stores/  →  api/  →  bindings/
                                  ↓
                            components/ui/  (叶子，零业务依赖)
                            shared/types/    (叶子，零运行时依赖)
```

强制约束：

- `components/ui/` 是叶子模块：零业务 store 导入、零业务术语、零业务类型
- `shared/types/` 仅类型定义，零运行时代码
- `api/` 或 `bridge/` 二选一，不共存
- `views/` 是组合层，业务逻辑放 `components/<domain>/` 或 `composables/`

domain 之间禁止横向依赖：

- `components/motion/` 禁止 import `components/calibration/`
- `components/device/` 禁止 import `components/motion/`
- 跨 domain 共享必须通过 `composables/` 或 `stores/`

循环引用禁止：

- 文件 A import B，B 禁止再 import A
- 循环引用是设计错误，必须重构（提取共享层、用事件总线、改单向依赖）
- 类型循环用 `import type` + 接口隔离打破

barrel file (`index.ts`) 规则：

- `index.ts` 仅用于 re-export，禁止含逻辑代码
- 最多 1 层 barrel（`stores/index.ts` 可以，`stores/motion/index.ts` + `stores/index.ts` 两层禁止）
- barrel 必须用 `export type` 区分类型与值，避免循环
- 不强制要求每个目录都有 `index.ts`，按需创建

default export 规则：

- Vue 组件用 default export（Vue SFC 规范要求）
- `.ts` 模块优先 named export，default export 仅用于"单文件单主要导出"场景
- 一个 `.ts` 文件最多 1 个 default export
- 工具函数、常量、类型必须 named export

依赖方向自检：

- 改 `components/ui/` 时检查零业务依赖（已有规则）
- 改 `stores/` 时检查零 Wails runtime 直接导入（已有规则）
- 改 `api/` 时检查零 store 导入（API 层不应反向依赖 store）
- 改 `composables/` 时检查零组件导入（composable 不能依赖组件）

## 10. 文件与 import 组织规范

`<script setup>` 内 import 顺序（自上而下，空行分隔）：

1. Vue 内置（`vue`、`vue-router`、`pinia`）
2. 第三方库（`@vueuse/core`、`dayjs`、`chart.js`）
3. 工作空间共享（`@shared/types/*`、`@shared/utils/*`）
4. 项目内 alias（`@stores/*`、`@components/*`、`@composables/*`、`@api/*`）
5. 相对路径（`./`、`../`）
6. 类型 import 必须用 `import type` 单独分组

```typescript
// 1. Vue 内置
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'

// 2. 第三方
import { useDebounceFn } from '@vueuse/core'
import dayjs from 'dayjs'

// 3. 工作空间共享
import type { MotionProfile } from '@shared/types/motion'
import { formatDuration } from '@shared/utils/format'

// 4. 项目内 alias
import { useMotionStore } from '@stores/motionStore'
import UiButton from '@components/ui/UiButton.vue'
import { useAxisLimits } from '@composables/useAxisLimits'

// 5. 相对路径
import ChildComponent from './ChildComponent.vue'
import { localHelper } from './utils'

// 6. 类型 import（单独分组）
import type { AxisConfig } from './types'
```

import 风格：

- 必须用 alias（`@stores/*`），禁止 `../../../stores/xxx`（除相对路径段落）
- 单行 import 超过 100 字符必须换行
- 一个 import 语句最多导入 10 个命名导出，超出拆分
- 禁止 side-effect import（`import './style.css'`）放在文件中部，必须放顶部

文件组织：

- 单文件单一主题：一个 `.ts` 文件 export 一个主要概念（一个 store、一个 composable、一组相关工具函数）
- 文件名与主要 export 名对齐：`useMotionHistory.ts` 导出 `useMotionHistory`
- 文件顶部 1-3 行注释说明意图（仅复杂或非显然文件需要）

```typescript
// useMotionHistory.ts
// 维护轴运动历史的环形缓冲区，提供 HSL 颜色映射用于状态栏可视化
// 配套组件：MotionControlPanel.vue 第 234-278 行的历史采样
import { ref, computed, type Ref } from 'vue'
```

export 规则：

- 工具函数 / 常量 / 类型用 named export
- Vue 组件用 default export
- 一个文件最多 1 个 default export
- 禁止 `export *`（造成 tree-shaking 困难和命名冲突）
