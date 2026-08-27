# 代码组织结构整改方案

> **版本**: 1.1 | **日期**: 2026-04-20 | **基于**: 代码组织 Review + 实际代码核实

---

## 一、当前问题清单

### 🔴 严重问题

| 问题 | 现状 | 影响 |
|------|------|------|
| **1. tcp_connection_driver.go 文件过大** | 单文件 984 行，包含 5 个设备驱动 + 底层 TCP 连接逻辑；811A/820/860 三个驱动代码高度重复 | 违反单一职责，难以维护和测试；重复代码增加修改风险 |
| **2. calibration/service.go 文件过大** | 单文件 1018 行，校准业务全流程编排集中在一个 Service 中 | 全项目最大文件，职责过重，难以理解和修改 |
| **3. apiClient.ts 职责混杂** | 517 行，类型定义(118行) + API 调用函数 + 初始化逻辑混在一起 | 违反前后端契约优先原则 |
| **4. calibrationStore 职责过重** | 691 行，包含状态管理、API 调用、业务逻辑、UI 控制 | 违反 Store 单一职责原则 |
| **5. CalibrationView.vue 行数超标** | 1661 行（模板 559 + 脚本 328 + 样式 770），主要问题是模板和样式膨胀 | 违反 Vue SFC < 300 行规范 |
| **6. MeasurementView.vue 行数超标** | 1998 行，前端最大文件，问题同上 | 同上，且方案此前完全遗漏 |
| **7. DeviceManagementPanel.vue 行数超标** | 1233 行，组件目录下超大文件 | 组件职责不清，难以维护 |
| **8. RealtimeDataPanel.vue 行数超标** | 874 行，组件目录下超大文件 | 同上 |

### 🟡 中等问题

| 问题 | 现状 | 影响 |
|------|------|------|
| **9. 缺少 composables 层** | 前端业务逻辑直接写在 Store 或 View 中 | 逻辑无法复用，违反规范 |
| **10. 类型定义分散** | Go 类型在 `domain/`、`application/`、`api/dto/` 中散落；TS 类型在 `apiClient.ts` 中 | 前后端契约不清晰 |
| **11. calibration_handler.go 偏大** | 463 行，单个 Handler 文件包含过多逻辑 | Handler 应按职责拆分 |
| **12. test 目录位置不当** | `test/debug_api.go` 等非标准测试文件在根目录下 | 与 `cmd/`、`internal/` 结构不协调 |

### 🟢 轻微问题

| 问题 | 现状 | 影响 |
|------|------|------|
| **13. 根目录存在临时/构建产物** | `nul`、`server.exe`(8.87MB)、`cal1604.exe`(11.21MB)、`cal1604-server-debug.exe`、4 个日志文件 | 污染根目录 |
| **14. 根目录存在 npm-cache** | `npm-cache/` 852MB，`e2e/node_modules/` + `e2e/npm-cache/` | 磁盘浪费，应 .gitignore 排除 |
| **15. 前端组件分类不清晰** | `components/` 下 calibration、measurement、common 混杂 | 组件职责不清晰 |
| **16. 缺少 .ai-rules.md** | 规范文档在 `AGENTS.md` 中，但未独立成 AI 规则文件 | 新 AI 会话无法快速加载规范 |

---

## 二、整改方案

### 方案 1：拆分 tcp_connection_driver.go

**当前问题：** 984 行，5 种设备驱动全部内联，811A/820/860 代码高度重复。

**整改后结构：**

```
internal/infrastructure/driver/
├── factory.go                    # 工厂方法，创建设备驱动（已有，70行）
├── tcp_base.go                   # TCP 基础连接驱动（原 tcpConnectionDriver）
├── wtn1604_driver.go             # WTN1604 计量设备驱动
├── const_base_driver.go          # ConST 系列公共基类（提取 811A/820/860 重复逻辑）
├── const811a_driver.go           # ConST 811A 打压设备驱动（嵌入 const_base_driver）
├── const820_driver.go            # ConST 820 打压设备驱动（嵌入 const_base_driver）
├── const860_driver.go            # ConST 860 打压设备驱动（嵌入 const_base_driver）
├── spc4000_driver.go             # SPC4000 打压设备驱动
├── helpers.go                    # 辅助函数（单位转换、通道位图、SCPI 解析）
└── *_test.go                     # 对应测试文件
```

**拆分原则：**
- `tcp_base.go` 仅包含通用 TCP 连接逻辑（Connect/Disconnect/sendCommand 等）
- **新增 `const_base_driver.go`**：提取 811A/820/860 三个驱动的公共逻辑，消除重复代码
- 各设备驱动文件仅包含该设备特有的协议命令
- 每个驱动文件控制在 200-300 行（含 struct 定义和方法签名）

---

### 方案 2：拆分 calibration/service.go

**当前问题：** 1018 行，全项目最大文件，校准业务全流程编排集中在一个 Service 中。

**整改后结构：**

```
internal/application/calibration/
├── service.go                    # 校准流程编排（会话生命周期管理，< 300 行）
├── collector.go                  # 数据采集逻辑（collectData、fitData 等）
├── pressure.go                   # 压力点管理（generatePoints、pressurize 等）
└── service_test.go               # 测试
```

**拆分原则：**
- `service.go` 保留核心编排逻辑（会话启动/暂停/停止、设备协调）
- 按业务职责拆出 `collector.go`（数据采集）和 `pressure.go`（压力点管理）
- 各文件通过 struct 方法组织，共享同一个 Service struct 的不同方法组

---

### 方案 3：重构 apiClient.ts

**当前问题：** 517 行，类型定义(118行) + API 调用 + 初始化逻辑混在一起。

**整改后结构：**

```
web/src/
├── types/                        # 新增：类型定义目录
│   ├── api.ts                    # API 响应类型（ApiResponse、HealthResponse 等）
│   ├── device.ts                 # 设备相关类型（DeviceDTO、SetDevicesRequest 等）
│   ├── calibration.ts            # 校准相关类型（SessionState、PressurePointDTO 等）
│   └── multipress.ts             # 多设备打压类型
├── api/                          # 重命名：services → api
│   ├── client.ts                 # HTTP 客户端基础（initDesktopApiBase、requestJSON）
│   ├── device.ts                 # 设备相关 API
│   ├── calibration.ts            # 校准相关 API
│   └── multipress.ts             # 多设备打压 API
```

**注意：** composables 层不在此方案中预先创建，而是在拆分 Store 和 View 时自然产生。

---

### 方案 4：拆分 calibrationStore

**当前问题：** 691 行，包含状态管理、API 调用、业务逻辑、UI 控制。

**整改后结构：**

```
web/src/
├── stores/
│   └── calibration/
│       ├── index.ts              # useCalibrationStore — 会话状态 + 步骤控制（< 200 行）
│       ├── pressurePoints.ts     # usePressurePointStore — 压力点管理（< 150 行）
│       └── deviceControl.ts      # useDeviceControlStore — 设备连接/控制（< 150 行）
├── composables/
│   ├── useCalibrationFlow.ts     # 校准流程编排（步骤切换、状态映射、SSE 事件处理）
│   └── useCalibrationConfig.ts   # 校准参数管理逻辑
```

**拆分原则：**
- **按业务领域拆成多个独立 Store**，而非按技术层（state/actions）拆同一个 Store
- Pinia 的 `defineStore` 本身就是 state+getters+actions 的组合，强行拆成 state.ts + actions.ts 增加理解成本
- Store 间通过 import 互相引用（Pinia 原生支持）
- 业务编排逻辑下沉到 composables

---

### 方案 5：拆分 CalibrationView.vue

**当前问题：** 1661 行（模板 559 + 脚本 328 + 样式 770），主要问题是模板和样式膨胀。

**整改后结构：**

```
web/src/views/
└── CalibrationView.vue           # 仅包含布局组装（< 100 行）
web/src/components/calibration/
├── CalibrationSidebar.vue        # 侧边栏（设备连接、单位检查、启动条件）
├── CalibrationParams.vue         # 参数配置区
├── CalibrationControl.vue        # 控制面板（开始、暂停、停止按钮）
├── CalibrationDataView.vue       # 数据展示区（压力点列表、采集数据）
└── CalibrationDialogs.vue        # 对话框集合（确认、拟合结果等）
```

**拆分原则：**
- View 仅负责布局组装，不包含业务逻辑
- **样式随组件走**：每个子组件携带自己的 scoped SCSS，从 View 中剥离 770 行样式
- 每个业务组件 < 300 行（含模板+脚本+样式）
- 业务逻辑通过 composables 注入

---

### 方案 6：拆分 MeasurementView.vue + 大组件

**当前问题：** MeasurementView.vue 1998 行、DeviceManagementPanel.vue 1233 行、RealtimeDataPanel.vue 874 行。

**整改策略：**
- 与方案 5 同理，按布局区域拆出子组件
- MeasurementView.vue → 布局壳 + MeasurementSidebar + MeasurementDataView + MeasurementControl
- DeviceManagementPanel.vue → DeviceList + DeviceDetail + DeviceActions
- RealtimeDataPanel.vue → DataChart + DataStats + DataControls

**每个子组件 < 300 行（含模板+脚本+样式）。**

---

### 方案 7：优化 internal 目录结构

**当前问题：** 部分目录命名不够清晰，大 Handler 文件需要拆分。

**整改后结构：**

```
internal/
├── api/
│   ├── http/
│   │   ├── router.go
│   │   ├── handlers/
│   │   │   ├── device_handler.go
│   │   │   ├── calibration_handler.go
│   │   │   ├── multipress_handler.go
│   │   │   ├── session_handler.go
│   │   │   ├── config_handler.go
│   │   │   ├── events_handler.go
│   │   │   ├── report_handler.go
│   │   │   └── health_handler.go
│   │   ├── error_mapper.go
│   │   └── response_writer.go
│   └── dto/
│       ├── device_dto.go         # 新增：从 handler 中提取设备相关 DTO
│       ├── calibration_dto.go    # 新增：从 handler 中提取校准相关 DTO
│       └── response.go
├── application/                  # 保持原名（重命名收益不足以抵消导入路径变更代价）
│   ├── calibration/
│   │   ├── service.go            # 已拆分（方案 2）
│   │   ├── collector.go
│   │   └── pressure.go
│   ├── deviceconnect/
│   │   └── service.go
│   └── multipress/
│       └── service.go
├── domain/
│   ├── device.go
│   └── session_state.go
├── device/                       # 保持原位（interfaces.go 仅 61 行，移动收益低）
│   ├── interfaces.go
│   └── manager/
│       ├── device_manager.go
│       └── persistent_device_manager.go
├── infrastructure/
│   └── driver/                   # 已拆分（方案 1）
├── workflow/
│   ├── session_machine.go
│   ├── stability_service.go
│   └── alarm_service.go
├── events/
│   └── bus.go
├── config/
│   └── app_config.go
├── errors/
│   └── codes.go
└── report/
    ├── report_service.go
    └── template_selector.go
```

**关键决策：**
- **`application/` 不重命名为 `app/`**：仅省 7 个字符，但需全项目导入路径变更 + Git 历史断裂，收益不足以抵消代价
- **`device/interfaces.go` 不移动到 `domain/ports.go`**：仅 61 行，当前位置合理
- **`device/manager/` 不重命名为 `infrastructure/store/`**：同理，重命名收益低
- **Handler 文件移入 `handlers/` 子目录**：当前已在 `api/http/` 下，移入子目录可配合 DTO 拆分

---

### 方案 8：清理根目录

**当前问题：** 根目录存在临时文件、构建产物、npm 缓存、日志文件。

**整改措施：**

```
# 删除
nul                              # Windows 误创建文件
server-debug.err.log             # 调试日志
server-debug.log                 # 调试日志
server-err.log                   # 错误日志
server-out.log                   # 输出日志

# 移动
e2e-test.mjs                     # → e2e/tests/legacy.mjs

# .gitignore 新增条目（构建产物和缓存应排除，不手动归档）
*.exe
npm-cache/
e2e/npm-cache/
e2e/node_modules/
web/node_modules/
web/dist/
build/
*.log

# 新增文件
.ai-rules.md                     # AI 规则文件（从 AGENTS.md 提取核心规则）
```

**关键决策：** `.exe` 构建产物通过 `.gitignore` 排除，而非手动移到 `build/windows/`——构建产物本就不应入库。

---

### 方案 9：前端组件分类重构

**当前问题：** 组件分类不清晰，common、calibration、measurement 混杂。

**整改后结构：**

```
web/src/components/
├── common/                      # 保持原名（重命名为 ui/ 是纯审美调整，收益低）
│   ├── DeviceStatusBadge.vue
│   ├── StatCard.vue
│   └── Sidebar.vue
├── calibration/                 # 标定相关（含方案 5 拆出的子组件）
│   ├── CalibrationControlPanel.vue
│   ├── CalibrationDataTable.vue
│   ├── CalibrationSidebar.vue   # 从 View 拆出
│   ├── CalibrationParams.vue    # 从 View 拆出
│   ├── CalibrationControl.vue   # 从 View 拆出
│   ├── CalibrationDataView.vue  # 从 View 拆出
│   ├── CalibrationDialogs.vue   # 从 View 拆出
│   ├── ChannelMatrix.vue
│   ├── Device1604Panel.vue
│   ├── PressDevicePanel.vue
│   ├── PressurePointList.vue
│   └── ProgressIndicator.vue
├── measurement/                 # 计量相关（含方案 6 拆出的子组件）
│   ├── DevicePanel.vue
│   ├── MeasureDeviceCard.vue
│   └── PressureDeviceCard.vue
└── device/                      # 设备管理（从根 components/ 移入）
    ├── DeviceManagementPanel.vue
    ├── DeviceSelectionPanel.vue
    └── RealtimeDataPanel.vue
```

**关键决策：** `common/` 不重命名为 `ui/`——这是纯审美调整，所有导入路径都要改，收益极低。

---

## 三、整改优先级与实施路线

| 阶段 | 优先级 | 整改项 | 预计影响 |
|------|--------|--------|----------|
| **第 1 阶段** | P0 | 拆分 tcp_connection_driver.go（含 ConST 公共基类提取） | 消除重复代码，提高驱动可维护性 |
| **第 1 阶段** | P0 | 拆分 calibration/service.go | 全项目最大文件，降低修改风险 |
| **第 1 阶段** | P0 | 重构 apiClient.ts（types/ + api/ 分层） | 前后端契约清晰化 |
| **第 2 阶段** | P1 | 拆分 calibrationStore（按领域拆多 Store + composables） | Store 职责单一化 |
| **第 2 阶段** | P1 | 拆分 CalibrationView.vue（含样式分配策略） | 页面组件轻量化 |
| **第 2 阶段** | P1 | 拆分 MeasurementView.vue + 大组件 | 前端最大文件，此前遗漏 |
| **第 3 阶段** | P2 | 优化 internal 目录结构（Handler 拆分 + DTO 提取） | Handler 职责清晰 |
| **第 3 阶段** | P2 | 清理根目录 + .gitignore | 项目结构整洁 |
| **第 3 阶段** | P2 | 前端组件分类重构 | 组件职责清晰 |

---

## 四、整改前后对比

### 整改前

```
Cal1604/
├── internal/
│   ├── application/calibration/service.go        # 1018 行
│   ├── infrastructure/driver/tcp_connection_driver.go  # 984 行
│   ├── api/http/calibration_handler.go           # 463 行
│   └── device/interfaces.go                      # 61 行（位置合理）
├── web/src/
│   ├── services/apiClient.ts                     # 517 行（类型 + API + 初始化混杂）
│   ├── stores/calibration/index.ts               # 691 行
│   ├── views/CalibrationView.vue                 # 1661 行
│   ├── views/MeasurementView.vue                 # 1998 行
│   ├── components/DeviceManagementPanel.vue      # 1233 行
│   └── components/RealtimeDataPanel.vue          # 874 行
└── [根目录]                                      # 临时文件 + npm-cache 852MB
```

### 整改后

```
Cal1604/
├── internal/
│   ├── application/calibration/
│   │   ├── service.go                            # < 300 行
│   │   ├── collector.go                          # < 300 行
│   │   └── pressure.go                           # < 300 行
│   ├── infrastructure/driver/
│   │   ├── tcp_base.go                           # < 200 行
│   │   ├── const_base_driver.go                  # < 150 行（消除重复）
│   │   ├── wtn1604_driver.go                     # < 250 行
│   │   └── const811a_driver.go                   # < 100 行
│   └── api/http/handlers/                        # Handler 按文件拆分
├── web/src/
│   ├── types/                                    # 类型定义集中
│   ├── api/                                      # API 调用分层
│   ├── composables/                              # 逻辑复用层（自然产生）
│   ├── stores/calibration/
│   │   ├── index.ts                              # < 200 行
│   │   ├── pressurePoints.ts                     # < 150 行
│   │   └── deviceControl.ts                      # < 150 行
│   ├── views/CalibrationView.vue                 # < 100 行
│   └── views/MeasurementView.vue                 # < 100 行
└── .gitignore                                    # 排除构建产物和缓存
```

---

## 五、实施建议

1. **分阶段实施**：按 P0 → P1 → P2 顺序逐步整改，避免一次性大改动引入风险。
2. **保持功能可用**：每个阶段完成后运行 `npm run typecheck`、`npm run lint`、`go test ./...` 确保功能正常。
3. **保留 Git 历史**：使用 `git mv` 移动文件，保留文件修改历史。
4. **更新导入路径**：整改后全局搜索更新导入路径，确保无遗漏。
5. **避免纯重命名**：目录/文件重命名仅在确实带来显著可读性提升时执行，否则保持原名以减少导入路径变更和 Git 历史断裂。
6. **composables 自然产生**：不预先创建 composables 目录，而是在拆分 Store 和 View 时将提取出的逻辑放入 composables，确保每个 composable 都有实际使用方。
