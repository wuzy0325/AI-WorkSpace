# DAQ MVP 跨平台验证计划

> 目标：验证 Tauri 2 + Vue 3 + Rust 架构路线能否在 Win11 / Win7 上跑通 mock 采集闭环。

## 项目名

```powershell
powershell -File .\scripts\new-project.ps1 -Name daq-mvp
```

目标目录：`projects/daq-mvp/`

```
projects/daq-mvp/
├── apps/desktop-tauri/
│   ├── frontend/
│   └── src-tauri/
└── services/api-rs/
```

> 建议 MVP 独立，避免污染正式项目。

---

## 范围

### MVP 做

1. Tauri 2 + Vue 3 桌面壳能启动。
2. `services/api-rs` 中有一个 mock DAQ 设备，模拟 1 kHz 采样。
3. 后端 DeviceActor 产生批次数据，并通过 Tauri Channel 推到前端。
4. 前端显示实时计数、最近值、简单 Canvas 折线。
5. 同一套代码能在 Win11 正常构建运行，并能产出 Win7 兼容安装包到 Win7 SP1 上启动验证。

### MVP 不做

- 不接真实厂商 DLL。
- 不做多设备（保留代码结构支持）。
- 不做 ECharts 大屏（只用 Canvas 或文本 + 小折线）。
- 不做 taurpc（先用 Tauri command + Channel）。
- 不做 WebSocket 外部监控。
- 不做 updater。
- 不做完整日志系统（只做 tracing stdout/file 基础日志）。
- 不做 SQLite/数据库（最多写临时 CSV 或完全不落盘）。

---

## 第 1 步：骨架验证

目标：先证明工作区结构、Tauri、Vue、Rust crate 能一起编译。

### 交付

```
projects/daq-mvp/services/api-rs/src/
├── core/sample.rs
├── ports/device_driver.rs
├── usecase/acquisition_manager.rs
├── usecase/device_actor.rs
├── adapters/hardware/mock.rs
└── lib.rs

projects/daq-mvp/apps/desktop-tauri/src-tauri/src/
├── main.rs
├── commands.rs
└── app_state.rs

projects/daq-mvp/apps/desktop-tauri/frontend/src/
├── App.vue
├── api/tauri.ts
└── modules/acquisition/
```

### 验收

```powershell
powershell -File .\scripts\validate-structure.ps1
cargo check --workspace
```

Win11 上能跑：

```powershell
cd projects\daq-mvp\apps\desktop-tauri
cargo tauri dev
```

---

## 第 2 步：Mock 采集闭环

目标：后端模拟设备，不接真实硬件。

### Mock driver 行为

| 属性 | 值 |
|---|---|
| 设备 ID | mock-001 |
| 通道数 | 4 |
| 采样率 | 1000 Hz |
| 批次大小 | 50 samples |
| 批次频率 | 每 50 ms 产生一批 |
| 数据 | 正弦波 + 轻微噪声 |
| 批次标识 | 每批带 `sequence_start` |
| 控制 | 支持 `start` / `stop` |

### 最小数据结构

```rust
pub struct SampleBatch {
    pub device_id: String,
    pub sequence_start: u64,
    pub sample_rate_hz: f64,
    pub channel_count: usize,
    pub sample_count: usize,
    pub values: Vec<f32>,
    pub host_timestamp_ms: i64,
}
```

### 验收

- 点开始采集后，后端每 50 ms 产生一批数据。
- `sequence_start` 连续递增。
- 停止后不再产生数据。
- 反复开始/停止 20 次不崩。

---

## 第 3 步：Tauri IPC 验证

目标：验证 command + Channel，而不是 Event 扛实时数据。

### 最小命令

```
list_devices()
start_acquisition(device_id)
stop_acquisition(device_id)
subscribe_waveform(device_id, channel)
get_runtime_stats()
```

### 数据流

```
Mock DeviceActor
  -> UI 降采样/批次 DTO
  -> Tauri Channel
  -> Vue 前端
```

### 前端只做

- 设备状态：Stopped / Acquiring / Error。
- 总样本数。
- 最新 sequence。
- Channel 收到的批次数。
- 简单 Canvas 折线，最多画最近 1000 个点。
- 丢帧计数（可选）。

### 验收

- Win11 dev 模式下连续采集 30 分钟。
- UI 不明显卡顿。
- 内存没有持续线性增长。
- CPU 在可接受范围内。
- `start` / `stop` / `restart` 正常。

---

## 第 4 步：Win11 发布包验证

目标：先把普通 Windows 构建跑通。

### 命令

```powershell
cd projects\daq-mvp\apps\desktop-tauri
cargo tauri build --target x86_64-pc-windows-msvc
```

### 验收

- Win11 上安装包能生成。
- 安装后能启动。
- 采集闭环能跑 30 分钟。
- 卸载正常。
- 日志能找到，至少包含启动、开始采集、停止采集、错误。

---

## 第 5 步：Win7 兼容构建验证

目标：只验证"能安装、能启动、能跑 mock 采集"，不追求完整功能。

### Win7 构建支持

```powershell
rustup toolchain install nightly
rustup +nightly component add rust-src
```

构建命令：

```powershell
cd projects\daq-mvp\apps\desktop-tauri
cargo +nightly tauri build --target x86_64-win7-windows-msvc -Zbuild-std
```

### Win7 验收机器要求

- Windows 7 SP1 x64
- WebView2 Runtime 109 或离线安装包
- VC++ runtime 已安装或随安装包处理

### Win7 验收

- 安装包能运行。
- WebView 能打开。
- 主界面能显示。
- 开始采集后计数增长。
- Canvas 或文本实时刷新。
- 连续运行 30 分钟。
- 退出后进程完全结束。
- 再次打开仍能运行。

---

## 最小插件清单

### 先用

```toml
tauri = "2"
tokio = { version = "1", features = ["rt-multi-thread", "macros", "time", "sync"] }
serde = { version = "1", features = ["derive"] }
serde_json = "1"
thiserror = "2"
anyhow = "1"
tracing = "0.1"
tracing-subscriber = "0.3"
tokio-util = "0.7"
```

### 暂时不用

taurpc、tokio-tungstenite、rmp-serde、dashmap、tauri-plugin-updater、tauri-plugin-notification、tauri-plugin-fs、tauri-plugin-dialog、真实 libloading DLL。

---

## 最小前端布局

```
┌─────────────────────────────────────────────┐
│ DAQ MVP                                     │
│ [Start] [Stop]                              │
├─────────────────────────────────────────────┤
│ Device: mock-001                            │
│ State: Acquiring                            │
│ Sample rate: 1000 Hz                        │
│ Batches received: 1234                      │
│ Samples received: 61700                     │
│ Last sequence: 61650                        │
│ Last value ch0: 0.123                       │
├─────────────────────────────────────────────┤
│ Canvas waveform, latest 1000 points         │
└─────────────────────────────────────────────┘
```

---

## 验证记录表

| 项目 | Win11 | Win7 |
|---|---|---|
| 安装包能否安装 | | |
| 应用能否启动 | | |
| WebView 是否正常显示 | | |
| Start/Stop 是否正常 | | |
| 采集 30 分钟是否稳定 | | |
| UI 是否明显卡顿 | | |
| 退出后进程是否结束 | | |
| 内存是否持续增长 | | |
| 日志是否可读 | | |
| 主要错误信息 | | |

---

## 推荐时间安排

| 时间 | 内容 |
|---|---|
| 第 0.5 天 | 建项目骨架，跑通 Tauri + Vue + api-rs 编译 |
| 第 0.5 天 | 实现 mock driver、DeviceActor、start/stop |
| 第 0.5 天 | 实现 Tauri Channel 到 Vue，前端显示计数和 Canvas |
| 第 0.5 天 | Win11 build，Win7 build，安装包验证和问题记录 |

---

## 通过标准

1. **Win11**：dev 模式和 release 安装包都能稳定跑 mock 采集 30 分钟。
2. **Win7**：安装包能启动，WebView 能显示，mock 采集能跑 30 分钟。
3. `src-tauri` 没有硬件驱动和采集业务逻辑。
4. 实时数据走 Channel，不走 Event 洪泛。
5. `start`/`stop` 多次操作不会留下后台任务。

这 5 条过了，再进入下一步：接入 `libloading` mock DLL 或一个真实厂商 SDK 的最小读数函数。
