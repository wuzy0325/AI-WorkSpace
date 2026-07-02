# AI 前端开发规则

> 本文档用于约束 AI agent 在本工作空间内进行前端开发。目标是让 AI 能判断代码放置、职责边界、UI 质量与验证要求，而不是只产出“能跑起来”的界面。

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

## 7. 状态完整性规则

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

错误反馈必须说明原因和下一步。禁止只显示“失败”。

### 7.1 共享 Store 边界规则

跨页面共享的 store（例如设备、运动控制器、采集、校准、遍历、存储）必须把“业务资产状态”和“加载错误状态”分开维护。

必须遵守：

- 共享 store 的刷新函数只能在后端明确成功返回时替换业务资产列表。
- 临时异常、网络失败、Wails binding 失败、状态查询失败不得清空已有业务资产。
- `[]` 只能表示后端明确返回空列表；异常不能被转换成空列表。
- 失败分支必须保留上次成功状态，并通过 `loading` / `error` / toast / banner 等可见状态暴露原因。
- 删除、清空、重置等破坏性状态变化必须来自显式用户动作或明确后端成功响应，不能由“刷新失败”隐式触发。
- 多个模块初始化时，彼此独立的加载任务应优先使用 `Promise.allSettled` 或等价容错流程，避免设备列表刷新失败拖垮校准、遍历、存储等其他配置加载。
- 共享 store 的失败语义必须有单测覆盖，至少验证“失败保留旧状态”和“成功清除错误状态”。

禁止：

- 在 `catch` 中把共享业务列表直接置为 `[]`、默认空对象或模拟数据，除非这是用户确认的重置动作。
- 让功能页面通过调用共享刷新函数间接影响其他页面的核心列表状态。
- 用一个全局 `loading` 或 `error` 混淆多个独立资源的加载状态。

## 8. 表单规则

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

## 9. 图表和实时数据规则

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

## 10. 布局和响应式规则

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

## 11. 样式和 token 规则

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

## 12. 可访问性规则

至少满足：

- 按钮有明确 label
- 图标按钮有 tooltip 或 aria-label
- focus 状态可见
- 错误不只靠颜色表达
- 禁用状态原因清楚
- 文本对比度足够
- 键盘可操作关键流程

## 13. 验证要求

前端改动后至少运行目标项目的：

- `npm run typecheck`
- `npm run build`

有测试时运行：

- `npm run test`

涉及布局、视觉、响应式时，还需要人工或浏览器截图验证：

- 目标桌面窗口
- 项目规定的最小窗口
- 空状态
- 错误状态
- loading 状态
- 长文本状态

若命令无法运行，AI 必须在最终说明中报告原因和未验证风险。

### 13.1 Wails 绑定同步（强制，零容忍）

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
