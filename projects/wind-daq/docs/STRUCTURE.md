# Wind-DAQ 项目结构说明

## 项目概述

Wind-DAQ 是风洞数据采集系统，采用六边形架构，支持多设备数据采集、运动控制、自动校准和曲面扫描等功能。

## 技术栈

- **桌面壳**: Tauri 2 + Rust
- **前端**: Vue 3 + Vite
- **后端**: Rust 服务层
- **通信**: Tauri commands、REST API 或 WebSocket（按功能需要选择）
- **硬件**: DAQ-P1604、DAQ-T1603 数据采集仪

---

## 目录结构

```
wind-daq/
├── apps/
│   └── desktop-tauri/
│       ├── frontend/       # Vue 3 前端
│       └── src-tauri/      # Tauri 桌面壳和薄命令桥
│
├── services/
│   └── api-rs/
│       ├── Cargo.toml
│       └── src/
│           ├── bin/        # 服务入口与依赖装配
│           ├── core/       # 纯业务核心
│           ├── ports/      # trait 契约
│           ├── usecase/    # 用例编排
│           └── adapters/   # 外部系统适配器
│               ├── hardware/
│               ├── db/
│               └── mq/
│
├── contracts/
│   ├── proto/
│   └── openapi/
│
├── deploy/
│   ├── dev/
│   ├── staging/
│   └── prod/
│
├── tests/
│   ├── hil/
│   └── integration/
│
├── config/                 # 默认配置文件或开发样例配置
├── CLAUDE.md
├── AGENTS.md
└── README.md
```

---

## 六边形架构边界

### 1. `services/api-rs/src/core`

纯领域模型与业务规则。禁止依赖硬件 SDK、文件 I/O、网络、串口、数据库、Web 框架、Tauri 类型。

### 2. `services/api-rs/src/ports`

外部依赖的 trait 契约，例如设备、运动控制、配置仓储、事件发布、数据存储。此层不放行为实现。

### 3. `services/api-rs/src/usecase`

应用用例编排层。协调 `core` 和 `ports`，实现设备生命周期、采集、校准、扫描、数据保存等用例。禁止直接依赖具体硬件适配器。

### 4. `services/api-rs/src/adapters`

具体外部系统实现：

- `hardware/`: TCP/串口/CAN/Modbus 等设备通信与协议转换
- `db/`: 数据库或持久化实现
- `mq/`: 事件或消息实现

适配器只做协议转换和 I/O，不写业务规则。

### 5. `apps/desktop-tauri`

桌面应用壳。`frontend/` 负责界面展示和用户交互；`src-tauri/` 负责窗口生命周期、系统能力、薄命令桥。核心业务逻辑仍放 `services/api-rs`。

---

## 数据流

### 采集数据流

```
硬件设备
  ↓
Rust hardware adapter
  ↓
ports trait
  ↓
usecase 聚合与节流
  ↓
Tauri command / WebSocket / API
  ↓
Vue 前端展示
```

### 控制命令流

```
Vue 用户操作
  ↓
Tauri command 或 API client
  ↓
Rust usecase
  ↓
ports trait
  ↓
hardware adapter
  ↓
硬件设备
```

---

## 架构原则

1. **核心层零依赖**: `core` 不依赖任何外部 I/O、框架或硬件实现。
2. **端口抽象**: 业务逻辑通过 `ports` trait 与外部系统交互。
3. **适配器可替换**: 更换硬件只新增或替换 adapter，不改 `core`。
4. **Tauri 保持薄壳**: `src-tauri` 不写采集、校准、运动控制等业务规则。
5. **单向依赖**: shell/API → usecase → core + ports，adapters 实现 ports。
