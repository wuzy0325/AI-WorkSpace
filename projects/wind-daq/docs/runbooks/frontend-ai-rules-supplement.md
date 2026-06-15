# Wind-DAQ 前端规则增补

> 本文档是对 `docs/runbooks/frontend-ai-rules.zh-CN.md` 的补充和细化，专门针对 Wind-DAQ 项目实践中发现的问题。

## 1. 按钮使用边界（补充第 6 章）

### 1.1 UiButton 适用场景

`UiButton`（包装 `NButton`）适用于：
- 工具栏操作按钮（连接、断开、开始、停止等）
- 表单提交按钮
- 弹窗确认/取消按钮
- 简单的图标按钮

### 1.2 UiButton 不适用场景

`UiButton` **不适用于**：
- 侧边栏设备列表项（需要复杂的内部 flex 布局，如名称+状态+地址的多行结构）
- 控制器列表项（需要状态圆点、名称、连接地址的多列布局）
- 任何需要 `NButton` 内部 `.n-button__content` 作为 flex 容器且需要自定义布局的列表项

**原因**：`UiButton` 包装了 `NButton`，其内部 `.n-button__content` 使用 `display: inline-flex`，会压缩和干扰复杂的内部布局。强行使用会导致文本截断、布局错乱。

### 1.3 列表项的正确做法

对于侧边栏列表、卡片列表等场景，优先使用以下方式：

1. **原生 `<div>` 或 `<button>` 元素 + 自定义样式**
   - 使用项目 design token（`--color-bg-panel`、 `--space-3`、 `--radius-lg` 等）
   - 自行实现激活态、悬停态、错误态样式
   - 内部布局完全自由，不受 `NButton` 限制

2. **创建专门的列表项组件**
   - 如果该列表项模式在多个领域复用，创建 `UiListItem` 或 `UiSidebarItem`
   - 通用列表项放在 `components/ui/`
   - 带特定布局模式的放在 `components/patterns/`

3. **禁止行为**
   - 禁止为绕过 `UiButton` 限制而大量使用 `:deep()` 覆盖 `NButton` 内部样式
   - 禁止在列表项内部使用 `UiButton` 后再用 `:deep()` 强制修改 `display: flex`

### 1.4 直接 Naive UI 使用条件

以下情况允许直接使用 `NButton`、`NInput` 等 Naive UI 原生组件：
- 无 `Ui*` 包装器可用
- 组件足够复杂，包装会增加负担（如 `NDataTable`、`NModal`、`NSteps`）
- 需要 `NButton` 的 `type` 属性（如 `type="primary"`、`type="error"`）且 `UiButton` 的 `variant` 无法满足时
- 文件是 spike 或迁移-only 实验

## 2. 截图验证规则细化（补充第 13 章）

### 2.1 必须截图验证的改动

涉及以下修改时，必须启动应用并进行截图验证：

| 改动类型 | 验证内容 | 最小验证场景 |
|---|---|---|
| 列表项布局 | 设备列表、控制器列表 | 正常状态、长文本、空状态 |
| 按钮样式调整 | 工具栏按钮、卡片按钮 | 默认态、悬停态、禁用态 |
| 侧边栏/面板布局 | 设备侧边栏、配置面板 | 正常宽度、最小窗口宽度 |
| 状态文本显示 | 连接状态、运行状态 | 各状态枚举值 |
| 表单布局 | 输入框、标签对齐 | 错误状态、长标签 |
| 弹窗/抽屉 | 确认弹窗、配置抽屉 | 最小窗口下是否完整显示 |

### 2.2 只需 build 验证的改动

以下改动通过 `npm run typecheck` 和 `npm run build` 即可：

- 纯逻辑修改（API 调用、数据处理、状态管理）
- 颜色 token 值调整（不涉及布局）
- 文案修改（短文本，不影响布局）
- 图标替换（同尺寸）
- 动画时长调整

### 2.3 截图验证流程

```powershell
# 1. 启动前端开发服务器
cd apps\desktop-wails\frontend
npm run dev

# 2. 在另一个终端运行截图脚本
cd projects\wind-daq
python screenshot_test.py

# 3. 检查 screenshots/ 目录下的输出
# - dashboard.png: 设备列表、顶部栏
# - motion.png: 控制器列表
# - calibration.png: 校准页面
# - traversal.png: 遍历页面
# - log.png: 日志页面
```

### 2.4 验证检查清单

截图后必须检查：

- [ ] 文本无截断（特别是中文标签、状态文本）
- [ ] 按钮文字完整显示，无溢出
- [ ] 列表项内部元素对齐正确
- [ ] 状态颜色与文本对应正确
- [ ] 窗口最小化时无元素重叠
- [ ] 空状态提示完整显示
- [ ] 错误状态提示完整显示

## 3. 控件封装与实际使用冲突时的处理原则

### 3.1 冲突识别

当出现以下情况时，说明 `Ui*` 封装与实际需求存在冲突：
- 需要大量 `:deep()` 覆盖才能满足布局需求
- 需要覆盖组件内部 3 个以上 CSS 类
- 需要使用 `!important` 强制修改组件默认行为
- 组件内部布局限制导致文本截断或元素错位

### 3.2 处理优先级

1. **优先使用原生元素**：如果 `Ui*` 组件的限制导致布局问题，优先使用原生 HTML 元素 + 自定义样式
2. **创建专用组件**：如果该模式在多处复用，创建新的 `Ui*` 原语（如 `UiListItem`）
3. **扩展现有组件**：如果现有 `Ui*` 组件只需少量扩展即可满足，添加 props 或 slots
4. **最后手段**：在单处使用且无法扩展时，允许直接使用 Naive UI 原生组件，但需在代码注释中说明原因

### 3.3 禁止做法

- 禁止为绕过封装限制而写大量 hack CSS
- 禁止在业务组件中复制 `Ui*` 组件的源码进行修改
- 禁止明知有布局问题仍坚持使用不合适的 `Ui*` 组件

## 4. 国际化文本规则

### 4.1 状态文本映射

设备状态、连接状态等枚举值必须正确映射到中文：

```typescript
// 正确做法：在组件内部或 store 中映射
const statusText = computed(() => {
  const map: Record<string, string> = {
    'Connected': '已连接',
    'Disconnected': '未连接',
    'Connecting': '连接中...',
    'Error': '错误',
  }
  return map[props.status] || props.status
})
```

### 4.2 错误提示文本

API 错误必须转换为用户友好的中文提示：

```typescript
// 正确做法：在 API 层统一处理
function formatApiError(error: unknown): string {
  const msg = error instanceof Error ? error.message : String(error)
  if (msg.includes('Failed to fetch')) {
    return '网络连接失败，请检查后端服务是否已启动'
  }
  return msg
}
```

## 5. 与上级规则的关系

- 本文档与 `docs/runbooks/frontend-ai-rules.zh-CN.md` 冲突时，以本文档为准（项目级优先）
- 本文档与 `components/ui/README.md` 冲突时，以 `components/ui/README.md` 为准（组件级优先）
- 所有规则最终服从项目级 `DESIGN.md` 的视觉和布局要求
