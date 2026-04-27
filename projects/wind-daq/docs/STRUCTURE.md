# Wind-DAQ 项目结构说明

## 项目概述

Wind-DAQ 是风洞数据采集系统，采用六边形架构（Hexagonal Architecture），支持多设备数据采集、运动控制、自动校准和曲面扫描等功能。

## 技术栈

- **后端**: Go + Gin Web 框架
- **前端**: Vue 3 + Wails (桌面应用)
- **通信**: REST API + WebSocket 实时推送
- **硬件**: DAQ-P1604、DAQ-T1603 数据采集仪

---

## 目录结构

```
wind-daq/
├── apps/                    # 应用程序
│   └── desktop-wails/       # Wails 桌面应用
│       ├── backend/        # Go 后端绑定
│       └── frontend/       # Vue 3 前端
│
├── contracts/               # 契约/接口定义
│   ├── proto/              # Protobuf 定义
│   └── openapi/           # OpenAPI 规范
│
├── deploy/                 # 部署配置
│   ├── dev/               # 开发环境
│   ├── staging/          # 测试环境
│   └── prod/             # 生产环境
│
├── services/                # 服务层
│   └── api-go/            # Go API 服务
│       ├── api/           # HTTP REST API + 路由
│       ├── cmd/           # 命令行入口
│       │   └── server/    # 主程序入口
│       └── internal/      # 内部包(六边形架构)
│           ├── core/      # 业务核心
│           ├── ports/     # 接口定义
│           ├── usecase/   # 用例层
│           ├── adapters/  # 适配器层
│           └── pkg/       # 共享工具(可选)
│
├── tests/                   # 测试代码
│   ├── hil/              # 硬件在环测试
│   └── integration/      # 集成测试
│
├── config/                  # 默认配置文件(运行时生成)
├── data/                    # 运行数据目录(运行时生成)
├── CLAUDE.md              # AI 辅助规则
├── AGENTS.md             # Agent 规则
└── README.md             # 项目说明
```

---

## 六边形架构详解

### 1. 核心层 (core/)

**业务核心领域模型，不依赖任何外部框架**

```
core/
├── device/
│   └── types.go          # 设备类型定义
│
├── motion/
│   └── types.go          # 运动控制类型定义
│
├── calibration/
│   ├── types.go         # 校准类型定义
│   └── formulas.go     # 校准计算公式
│
├── traversal/
│   └── types.go         # 曲面扫描类型定义
│
└── interpolation/
    ├── prb_interpolator.go        # 探针插值
    └── multi_prb_interpolator.go  # 多探针插值
```

**职责**:
- 定义设备、运动控制器、校准、扫描的数据结构
- 纯业务逻辑，无 I/O 操作
- 无框架依赖

---

### 2. 端口层 (ports/)

**接口抽象层，定义业务与外部交互的契约**

```
ports/
├── device.go            # 设备硬件接口
├── motion.go            # 运动控制器接口
├── data_sink.go         # 数据接收回调接口
├── config_repo.go       # 配置仓库接口
├── scan.go              # 设备扫描接口
└── event_bus.go        # 事件总线接口
```

**职责**:
- 定义 `Device` 接口：连接、采集、数据回调
- 定义 `MotionController` 接口：运动控制
- 定义 `DataSink`：数据接收
- 定义 `ConfigRepo`：配置持久化
- 定义 `DeviceScanner`：设备发现

**特点**:
- 仅包含接口定义，无实现
- 依赖 core 层类型
- 供 adapters 层实现

---

### 3. 用例层 (usecase/)

**应用业务逻辑，编排核心功能和端口**

```
usecase/
├── device_manager.go        # 设备生命周期管理
├── acquisition.go          # 多设备数据聚合与推送
├── motion_manager.go      # 运动控制器管理
├── calibration.go         # 自动校准服务
├── traversal.go           # 曲面扫描服务
├── storage.go             # CSV 数据存储
└── scan.go               # 设备扫描服务
```

**职责**:
- `DeviceManager`: 设备配置管理、连接控制、采集控制
- `AcquisitionHub`: 聚合多设备数据，按频率推送到前端
- `MotionManager`: 运动控制器配置、运动指令
- `CalibrationService`: 自动校准工作流（五孔/三孔/总压/总温探针）
- `TraversalService`: 曲面扫描工作流
- `StorageService`: 将采集数据写入 CSV 文件

**特点**:
- 依赖 ports 层接口
- 不直接调用硬件，通过端口抽象
- 包含业务规则验证

---

### 4. 适配器层 (adapters/)

**外部系统适配器，实现 ports 层接口**

```
adapters/
├── hardware/               # 硬件驱动适配器
│   ├── base_device.go     # 设备驱动基类
│   ├── factory.go        # 驱动工厂
│   ├── simulated.go     # 模拟设备驱动
│   ├── simulated_motion.go  # 模拟运动控制器驱动
│   ├── daq_p1604.go     # DAQ-P1604 驱动
│   ├── daq_t1603.go     # DAQ-T1603 驱动
│   ├── command_protocol.go  # 命令协议
│   └── docs/            # 驱动文档
│
├── config/               # 配置持久化适配器
│   ├── manager.go        # JSON 配置管理器
│   ├── device.go        # 设备配置存储
│   ├── acquisition.go   # 采集配置存储
│   └── storage.go       # 存储配置
│
├── ws/                   # WebSocket 适配器
│   ├── hub.go           # 连接管理+广播
│   ├── client.go       # 客户端连接
│   └── channels.go     # 频道常量
│
├── scan/                 # 设备扫描适配器
│   ├── scanner.go       # 扫描器接口
│   ├── scan_service.go # 扫描服务
│   ├── udp_scanner.go # UDP 扫描
│   ├── daq_p1604_scanner.go  # P1604 扫描
│   └── daq_t1603_scanner.go   # T1603 扫描
│
├── storage/              # 存储适配器
│   └── service.go       # 存储服务
│
├── report/              # 报告生成适配器
│   └── service.go       # 报告服务
│
├── mq/                  # 消息队列适配器(预留)
│   └── .gitkeep
│
└── db/                  # 数据库适配器(预留)
    └── .gitkeep
```

**职责**:
- **hardware**: 实现具体硬件驱动（模拟、P1604、T1603）
- **config**: JSON 文件配置读写
- **ws**: WebSocket 实时通信
- **scan**: 网络设备发现
- **storage**: CSV 文件写入
- **report**: 报告生成（Markdown/PDF）
- **mq**: 消息队列适配器（预留）
- **db**: 数据库适配器（预留）

**特点**:
- 实现 ports 层定义的接口
- 处理外部系统通信（TCP/UDP/文件）
- 可替换：不改变业务逻辑更换硬件

---

### 5. API 层 (api/)

**HTTP REST API 和 HTTP 处理**（位于 `services/api-go/api/`）

```
api/
├── server.go             # HTTP 服务器
├── middleware.go         # 中间件(Recovery/Logger/CORS)
└── handler/            # HTTP 请求处理器
    ├── app.go          # 应用级 API
    ├── device.go       # 设备 API
    ├── daq.go         # DAQ 采集 API
    ├── motion.go      # 运动控制 API
    ├── calibration.go  # 校准 API
    ├── traversal.go   # 曲面扫描 API
    ├── storage.go    # 存储 API
    ├── scan.go       # 扫描 API
    └── report.go     # 报告 API
```

**职责**:
- 暴露 REST API 给前端
- HTTP 请求参数校验
- 调用 useCase 层服务
- WebSocket 升级处理
- 响应格式化

**注意**: `api/` 与 `internal/` 同级，因为 API 层需要导入 `internal/` 各包，放在外部避免循环引用。

---

### 6. 共享工具层 (internal/pkg/)

**纯工具函数，无业务含义**

```
internal/pkg/
├── converter/            # 类型转换/单位转换
├── validator/            # 通用校验函数
└── math/                # 数学工具（滤波、统计等）
```

注意：此目录遵循"有实际共享需求时才创建"原则，避免过早抽象。`pkg/` 内的代码应是无状态的纯函数。

---

### 7. 入口层 (cmd/)

**应用程序入口**（位于 `services/api-go/cmd/`）

```
cmd/server/main.go        # 主程序入口
```

**职责**:
- 加载配置
- 初始化各层组件（依赖注入）
- 启动 HTTP 服务器
- 处理优雅关闭

---

## 数据流

### 采集数据流

```
硬件设备 (DAQ-P1604/T1603)
    ↓ 数据推送
 Device 驱动 (adapters/hardware/)
    ↓ 调用 DataSink 回调
 AcquisitionHub (usecase/)
    ↓ 聚合,按频率推送
 WebSocket Hub (adapters/ws/)
    ↓ 广播
 前端 WebSocket 客户端
```

### 控制命令流

```
前端 HTTP 请求
    ↓
 API Handler (api/handler/)
    ↓
 UseCase 层 (usecase/)
    ↓ 调用端口接口
 DeviceManager / MotionManager
    ↓
 端口接口 (ports/)
    ↓
 硬件驱动 (adapters/hardware/)
    ↓ TCP/Serial 通信
 硬件设备
```

---

## REST API 端点

### 应用

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/app/version` | 获取应用版本 |

### 设备管理

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/device/profiles` | 获取设备配置列表 |
| PUT | `/api/device/profiles` | 添加/更新设备配置 |
| DELETE | `/api/device/profiles/:id` | 删除设备配置 |
| GET | `/api/device/instances` | 获取活跃设备实例 |
| GET | `/api/device/status` | 获取所有设备状态 |
| GET | `/api/device/capabilities` | 获取支持的能力列表 |
| PUT | `/api/device/:id/unit` | 设置通道单位 |
| GET | `/api/device/:id/daqT1603Config` | 获取 DAQ-T1603 配置 |
| PUT | `/api/device/:id/daqT1603Config` | 应用 DAQ-T1603 配置 |
| POST | `/api/device/:id/connect` | 连接设备 |
| POST | `/api/device/:id/disconnect` | 断开设备 |
| POST | `/api/device/:id/startAcquisition` | 开始采集 |
| POST | `/api/device/:id/stopAcquisition` | 停止采集 |

### 设备扫描

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/scan/devices` | 扫描网络设备 |

### DAQ 采集

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/daq/startAcquisition` | 开始所有设备采集 |
| POST | `/api/daq/stopAcquisition` | 停止所有设备采集 |
| PUT | `/api/daq/publishRate` | 设置推送频率 |
| GET | `/api/daq/publishRate` | 获取推送频率 |

### 运动控制

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/motion/profiles` | 获取控制器配置 |
| PUT | `/api/motion/profiles` | 添加/更新配置 |
| DELETE | `/api/motion/profiles/:id` | 删除配置 |
| GET | `/api/motion/status` | 获取所有控制器状态 |
| POST | `/api/motion/:id/connect` | 连接控制器 |
| POST | `/api/motion/:id/disconnect` | 断开控制器 |
| POST | `/api/motion/:id/moveTo` | 绝对运动 |
| POST | `/api/motion/:id/moveBy` | 相对运动 |
| POST | `/api/motion/:id/jog` | 寸动 |
| POST | `/api/motion/:id/home` | 回零 |
| POST | `/api/motion/:id/stop` | 停止 |
| POST | `/api/motion/:id/emergencyStop` | 急停 |
| POST | `/api/motion/:id/definePosition` | 定义当前位置 |

### 校准

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/calibration/start` | 开始校准 |
| POST | `/api/calibration/pause` | 暂停校准 |
| POST | `/api/calibration/resume` | 恢复校准 |
| POST | `/api/calibration/stop` | 停止校准 |
| GET | `/api/calibration/status` | 获取校准状态 |
| PUT | `/api/calibration/config` | 保存校准配置 |
| GET | `/api/calibration/config` | 获取校准配置 |

### 曲面扫描

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/traversal/start` | 开始扫描 |
| POST | `/api/traversal/pause` | 暂停扫描 |
| POST | `/api/traversal/resume` | 恢复扫描 |
| POST | `/api/traversal/stop` | 停止扫描 |
| GET | `/api/traversal/progress` | 获取扫描进度 |

### 数据存储

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/storage/settings` | 获取存储设置 |
| PUT | `/api/storage/settings` | 更新存储设置 |
| GET | `/api/storage/status` | 获取存储状态 |
| POST | `/api/storage/startRecording` | 开始录制 |
| POST | `/api/storage/stopRecording` | 停止录制 |
| POST | `/api/storage/pickDirectory` | 获取数据存储目录 |

### 报告

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/report/calibration` | 生成校准报告 |
| POST | `/api/report/traversal` | 生成扫描报告 |
| POST | `/api/report/generate` | 生成通用报告 |

---

## WebSocket 频道

| 频道 | 说明 |
|------|------|
| `daq:data-snapshot` | DAQ 采集数据快照 |
| `device:status-updated` | 设备状态更新 |
| `motion:status-updated` | 运动控制器状态更新 |
| `calibration:progress` | 校准进度更新 |
| `calibration:complete` | 校准完成事件 |
| `calibration:realtime` | 校准实时数据 |
| `traversal:onProgress` | 曲面扫描进度 |
| `traversal:onComplete` | 曲面扫描完成 |
| `traversal:onError` | 曲面扫描错误 |

---

## 配置存储

配置文件保存在 `./config` 目录（可通过 `CONFIG_DIR` 环境变量修改），首次启动时自动创建：

```
config/
├── device-profiles.json    # 设备配置
├── motion-profiles.json  # 运动控制器配置
├── acquisition.json      # 采集配置
└── storage.json          # 存储配置
```

数据保存在 `./data` 目录（可通过 `DATA_DIR` 环境变量修改），首次录制时自动创建：

```
data/
└── data/
    ├── device1_20240101_120000.csv
    ├── device2_20240101_120000.csv
    └── ...
```

---

## 启动方式

```bash
# 默认配置(端口 8080)
go run ./services/api-go/cmd/server/main.go

# 自定义端口
PORT=9000 go run ./services/api-go/cmd/server/main.go

# 开发模式(详细日志)
DEV=true go run ./services/api-go/cmd/server/main.go

# 自定义配置目录
CONFIG_DIR=/path/to/config DATA_DIR=/path/to/data go run ./services/api-go/cmd/server/main.go
```

---

## 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| PORT | 8080 | HTTP 服务器端口 |
| DEV | false | 开发模式(详细日志) |
| CONFIG_DIR | ./config | 配置存储目录 |
| DATA_DIR | ./data | 数据存储目录 |

---

## 支持的硬件设备

### DAQ-P-1604
- 恒奥多通道数据采集仪
- 16 通道模拟输入
- TCP/IP 网络通信

### DAQ-T-1603
- 恒奥热电偶采集仪
- 支持 K/B/E/J/T/S/N/R/C 型热电偶
- TCP/IP 网络通信

---

## 架构原则

1. **核心层零依赖**: core 层不依赖任何外部包
2. **端口抽象**: 业务逻辑通过接口与外部交互
3. **可替换适配器**: 更换硬件只需实现对应端口接口
4. **依赖注入**: 各层通过依赖注入解耦
5. **单向依赖**: 依赖方向从外到内：API → UseCase → Ports → Core