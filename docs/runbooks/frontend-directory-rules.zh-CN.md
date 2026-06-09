# 前端目录结构规则

> 定义工作空间内所有 Vue 3 前端项目的目录结构标准。已有项目仍可在其当前结构下增删，
> 新建项目必须遵守本标准。小项目允许省略可选目录。

## 顶层目录总览

```
frontend/src/
├── main.ts                     # 应用入口
├── App.vue                     # 根组件
├── env.d.ts                    # 环境类型声明
│
├── pages/                      # 页面级路由组件 ← 或 views/
├── views/                      # ← 两者取一，不并存
│
├── components/
│   ├── ui/                     # 基础 UI 控件（Ui*）
│   ├── layout/                 # 布局组件（shell、topbar、sidebar、footer）
│   ├── feedback/               # 反馈类组件（toast、confirm、notification）
│   ├── icons/                  # 图标组件
│   └── <domain>/               # 业务领域组件（device、calibration、traversal…）
│
├── stores/                     # Pinia 状态管理
├── api/                        # API 调用封装层（HTTP / Wails binding）
├── styles/
│   ├── tokens/                 # 设计 token（color、spacing、typography、radius、motion、layout）
│   └── themes/                 # 主题（dark.css、light.css）
│
├── router/                     # 路由配置
├── composables/                # 跨组件可复用逻辑
├── types/                      # 全局 TypeScript 类型
│
├── shared/                     # 跨领域共享（工具函数、类型、composable）
├── bridge/                     # Wails binding 封装层（替代 api/，二选一）
├── core/                       # 框架无关逻辑容器（容器、常量、服务注册）
└── spikes/                     # 实验性代码（仅开发态）
```

## 各目录详细职责

### `pages/` (或 `views/`)
页面级组件，一个文件对应一条路由。只做组件组装，不承载领域逻辑。

允许：组合 feature 组件、调用 store、调用 api。

禁止：直接访问硬件、实现算法、写文件。

### `components/ui/`
基础 UI 控件，命名必须为 `Ui*` 前缀。不依赖业务 store、不出现业务领域词。

包含：`UiButton`、`UiInput`、`UiSelect`、`UiPanel`、`UiToggle`、`UiStatusBadge`、`UiFormField`、`UiDialog`、`UiToolbar`、`UiEmptyState`、`UiLoadingState`、`UiErrorState`。

### `components/layout/`
应用框架组件，只负责布局结构。

包含：`AppShell`、`MainTopBar`、`AppRailNav`、`MainBottomBar`、`DeviceSidebar`。

### `components/<domain>/`
按业务领域分组的 feature 组件，例如 `device/`、`calibration/`、`traversal/`、`motion/`、`storage/`。

允许：调用 store、api、composable。禁止：包含通用 UI 控件逻辑。

### `stores/`
Pinia store。每个文件一个 store，职责单一。

允许：跨页面共享的响应式状态、调用 api 或 bridge。

禁止：直接调用 Wails 生成代码、直接访问硬件。

### `api/`
HTTP API 或 Wails binding 封装层。前端调用后端或设备的唯一入口（除 bridge/ 替代方案外）。

### `styles/tokens/`
CSS 自定义属性定义。每个 token 文件对应一个维度。

包含：`color.css`、`spacing.css`、`typography.css`、`radius.css`、`motion.css`、`layout.css`。

### `styles/themes/`
主题覆盖。`dark.css` 和 `light.css` 为深色/浅色主题提供变量覆盖。

### `composables/`
可复用的 Vue composable。如果是特定领域内使用，放在 `components/<domain>/composables/`。

### `router/`
路由配置文件。

### `shared/`
跨领域共享的纯工具函数、类型定义、composable。非 UI 控件层。

## 规则

### 位置规则

| 代码类型 | 必须放在 |
|---|---|
| 基础 UI 控件（UiButton、UiInput…） | `components/ui/` |
| 布局结构组件 | `components/layout/` |
| 业务领域组件 | `components/<domain>/` |
| 页面组装 | `pages/` 或 `views/` |
| 设计 token | `styles/tokens/` |
| 主题 | `styles/themes/` |
| Pinia store | `stores/` |
| API 封装 | `api/` 或 `bridge/` |
| 路由 | `router/` |
| 全局类型 | `types/` |
| 应用入口 | `main.ts` |
| 根组件 | `App.vue` |

### 禁止

- 通用 UI 组件放在业务领域目录下
- 业务组件放在 `components/ui/` 下
- `pages/` 和 `views/` 并存
- `api/` 和 `bridge/` 并存
- `main.ts` 中出现业务逻辑
- `components/ui/` 中出现业务领域命名
- 在 `stores/` 中直接调用 Wails 生成代码

### 大小项目适应

- 小项目（1-3 个页面）：可省略 `router/`、`composables/`、`types/`、`shared/`、`spikes/`
- 小项目可以没有 `components/layout/`，layout 组件直接放在 `components/` 下
- 极简项目只保留 `pages/`、`stores/`、`api/`、`styles/tokens/` 即可
- `styles/tokens/` 即使只有一个文件也必须存在

### 与已有项目的关系

| 已有项目 | 当前结构 | 要求 |
|---|---|---|
| wind-daq | `views/` + `components/<domain>/` + `components/ui/` | 符合标准，维持 |
| motion-controller | `views/` + `components/<domain>/` + `components/ui/` | 符合标准，维持 |
| daq-t1603 | `views/` + `components/<domain>/` + `bridge/` | 符合标准，建议补 `components/ui/` |
| five-hole-interpolator | 极简（无 pages/ 等） | 无需改动 |

## 验证

运行 `powershell -File scripts/validate-frontend-structure.ps1` 检查当前项目结构。
