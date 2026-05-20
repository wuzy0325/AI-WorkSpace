# DAQ 数据采集与展示系统设计评审

> 日期：2026-05-12
> 范围：评审 `Rust + Tauri 2 + Vue 3 + tokio + oldwin` 技术路线下的 DAQ 桌面系统方案。
> 结论：总体技术方向可行，但原方案需要调整架构边界、Windows 7 兼容策略、Tauri IPC 数据通道、依赖版本与测试顺序。

## 1. 总体结论

推荐继续采用 `Rust + Tauri 2 + Vue 3 + tokio`：

- Rust 适合承载本地设备接入、采集调度、数据处理、长时间运行和文件落盘。
- Tauri 2 适合提供轻量桌面壳、系统能力、安装包和前端 WebView。
- Vue 3 适合本工作区既定的桌面 UI 技术栈。
- tokio 适合异步任务调度，但阻塞型厂商 SDK 调用必须隔离，不能直接阻塞 tokio worker 线程。

原方案中需要修正的关键点：

1. 硬件驱动、采集 Actor、设备管理不能放在 `apps/desktop-tauri/src-tauri`。
2. Windows 7 兼容不是普通构建目标，应作为单独兼容 SKU 处理。
3. Tauri Event 不应作为高吞吐实时波形主通道，实时批量数据应优先走 Channel。
4. `rmp-serde` 不会自动改变 Tauri IPC 的 JSON 序列化机制。
5. `oldwin`、`taurpc`、`tokio-tungstenite`、`cargo-fuzz` 等依赖描述需要更新。
6. 测试计划应把 Win7 可行性验证前置到 P0，而不是等到打包阶段。

## 2. 架构边界修正

本工作区的架构总纲以 `CLAUDE.md`、`docs/architecture/module-design.md` 和 `docs/decisions/ADR-001-workspace-layout.md` 为准：

- `projects/<name>/apps/desktop-tauri/src-tauri/`：Tauri 启动、窗口、插件、命令桥接，零业务逻辑。
- `projects/<name>/apps/desktop-tauri/frontend/`：Vue 3 UI，负责展示和交互，零硬件访问、零采集算法。
- `projects/<name>/services/api-rs/src/core/`：纯业务规则，零硬件、零文件 I/O、零框架依赖。
- `projects/<name>/services/api-rs/src/usecase/`：采集、设备、导出等应用用例编排。
- `projects/<name>/services/api-rs/src/ports/`：外部依赖 trait 定义，零实现。
- `projects/<name>/services/api-rs/src/adapters/hardware/`：硬件驱动和厂商 SDK 封装，零业务逻辑。
- `shared/device-sdk/`：跨项目复用的设备协议、帧解析、底层通信 primitives。

原方案中建议放在 `src-tauri/src/driver`、`src-tauri/src/device`、`src-tauri/src/actor.rs` 的代码，应整体移到 `services/api-rs`。

## 3. 推荐目录结构

```text
projects/daq/
├── apps/
│   └── desktop-tauri/
│       ├── frontend/
│       │   └── src/
│       │       ├── modules/
│       │       │   ├── device/
│       │       │   ├── acquisition/
│       │       │   ├── history/
│       │       │   └── settings/
│       │       ├── shared/
│       │       ├── api/
│       │       └── App.vue
│       └── src-tauri/
│           └── src/
│               ├── main.rs
│               ├── commands.rs
│               └── app_state.rs
├── services/
│   └── api-rs/
│       └── src/
│           ├── bin/
│           ├── core/
│           │   ├── acquisition/
│           │   ├── measurement/
│           │   └── sample.rs
│           ├── usecase/
│           │   ├── acquisition_manager.rs
│           │   ├── device_actor.rs
│           │   ├── supervisor.rs
│           │   └── export_task.rs
│           ├── ports/
│           │   ├── device_driver.rs
│           │   ├── sample_store.rs
│           │   ├── realtime_sink.rs
│           │   └── config_repo.rs
│           ├── adapters/
│           │   ├── hardware/
│           │   │   ├── mock.rs
│           │   │   └── vendor_a.rs
│           │   ├── storage/
│           │   ├── telemetry/
│           │   └── config/
│           ├── config.rs
│           └── error.rs
├── contracts/
├── tests/
│   ├── integration/
│   └── hil/
└── deploy/
```

Tauri 命令只调用后端 facade：

```rust
#[tauri::command]
async fn start_acquisition(
    state: tauri::State<'_, DesktopState>,
    device_id: String,
) -> Result<(), CommandError> {
    state.acquisition.start(device_id).await.map_err(Into::into)
}
```

`src-tauri` 不直接加载 DLL、不持有采集循环、不实现数据校准、不直接操作硬件。

## 4. 后端并发模型建议

推荐采用 `Supervisor + DeviceActor + bounded queue` 模型。

```text
AcquisitionSupervisor
  ├─ DeviceActor(dev-01)
  ├─ DeviceActor(dev-02)
  ├─ DeviceActor(dev-03)
  ├─ StorageWriter
  ├─ UiStreamFanout
  └─ HealthMonitor
```

每个 `DeviceActor` 负责一个物理设备：

- 持有设备连接状态。
- 接收控制命令：启动、停止、重连、设置参数。
- 采集样本批次。
- 上报状态、错误和健康指标。
- 通过取消令牌退出，避免悬挂任务。

建议状态机：

```text
Disconnected
  -> Connecting
  -> Ready
  -> Acquiring
  -> Faulted
  -> Reconnecting
  -> Stopped
```

关键改进点：

1. `broadcast::Sender` 不等于背压队列。慢消费者会 lag/drop，不能用它保证不丢原始采集数据。
2. 原始采集链路和 UI 展示链路必须拆开。
3. 阻塞型 DLL、USB、串口调用不能直接跑在 tokio worker 线程。
4. 所有硬件 I/O 必须有 timeout、错误码映射、重试策略和明确状态迁移。
5. 配置修改必须进入设备 Actor 命令队列，不允许多个地方直接修改 driver。

推荐数据流：

```text
DeviceActor
  ├─ raw sample batch -> bounded lossless queue -> StorageWriter
  └─ ui sample frame  -> lossy/latest queue      -> Tauri Channel -> Vue
```

原始数据链路：

- 目标是不丢点。
- 队列满时应进入告警、降采样、暂停采集或落盘缓冲策略。
- 必须记录序列号缺口。

UI 数据链路：

- 允许丢帧。
- 只保留最新批次。
- 前端只展示窗口内数据，例如最近 5 秒或最近 5000 个 UI 点。

## 5. 数据模型建议

原方案的 `DataPacket { values: Vec<f64> }` 适合原型，不适合高频多通道采集。

建议后端内部使用批次结构：

```rust
pub struct SampleBatch {
    pub device_id: DeviceId,
    pub sequence_start: u64,
    pub sample_rate_hz: f64,
    pub device_timestamp_ns: Option<i64>,
    pub host_timestamp_ns: i64,
    pub channels: Vec<ChannelId>,
    pub values: Vec<f32>,
    pub sample_count: usize,
    pub flags: BatchFlags,
}
```

字段含义：

- `sequence_start`：用于检测丢点和乱序。
- `sample_rate_hz`：记录批次对应采样率。
- `device_timestamp_ns`：设备端时间戳，可为空。
- `host_timestamp_ns`：主机接收或归档时间。
- `values`：建议使用 row-major 布局，长度为 `sample_count * channel_count`。
- `flags`：标记过载、溢出、重连首包、降采样、插值等状态。

前端 UI 可使用更轻量的 DTO：

```ts
export interface WaveformFrame {
  deviceId: string
  sequenceStart: number
  sampleRateHz: number
  channelIds: string[]
  values: Float32Array
  sampleCount: number
  hostTimestampNs: number
}
```

## 6. Tauri IPC 通信策略

Tauri v2 推荐按数据形态选择通信方式：

| 场景 | 推荐方式 | 数据量 | 说明 |
|---|---|---:|---|
| 开始采集、停止采集、设置参数 | command / invoke | 小 | 请求-响应 |
| 设备状态、报警、生命周期事件 | Event | 小 | 低频广播 |
| 实时波形 | Channel | 中到大 | 批量、流式、避免事件风暴 |
| 历史数据导出 | 文件 / Channel 分块 / ArrayBuffer | 大 | 不要一次性 JSON 返回上万点 |
| 外部 Web 监控面板 | 可选 WebSocket | 中 | 仅绑定 `127.0.0.1`，必须有鉴权 |

注意事项：

- Tauri command 默认要求参数和返回值可 JSON 序列化。
- Tauri Event 适合少量消息，不适合作为高吞吐低延迟数据通道。
- Channel 更适合持续流式传输。
- `rmp-serde` 可用于磁盘缓存、自定义协议、外部 WebSocket 或设备帧归档，但不会自动让 Tauri IPC 改用 MessagePack。

建议事件命名：

```text
daq:device-status
daq:acquisition-state
daq:alarm
daq:storage-warning
```

实时波形建议走 Channel：

```rust
use tauri::ipc::Channel;

#[tauri::command]
async fn subscribe_waveform(
    state: tauri::State<'_, DesktopState>,
    device_id: String,
    on_frame: Channel<WaveformFrameDto>,
) -> Result<(), CommandError> {
    state.acquisition.subscribe_waveform(device_id, on_frame).await?;
    Ok(())
}
```

## 7. 硬件驱动封装

硬件驱动 trait 应定义在 `services/api-rs/src/ports/device_driver.rs`：

```rust
#[async_trait::async_trait]
pub trait DeviceDriver: Send {
    fn device_id(&self) -> &DeviceId;
    fn device_type(&self) -> &str;
    fn capabilities(&self) -> DeviceCapabilities;

    async fn connect(&mut self) -> Result<(), DeviceError>;
    async fn configure(&mut self, config: DeviceConfig) -> Result<(), DeviceError>;
    async fn read_batch(&mut self) -> Result<SampleBatch, DeviceError>;
    async fn stop(&mut self) -> Result<(), DeviceError>;
    async fn disconnect(&mut self) -> Result<(), DeviceError>;
}
```

厂商 DLL 封装放在 `services/api-rs/src/adapters/hardware/vendor_a.rs`。

封装原则：

- `unsafe` 只允许出现在 adapter 内部。
- DLL 函数指针生命周期必须绑定到 `libloading::Library` 生命周期。
- 厂商错误码必须转换成项目内 `DeviceError`。
- 字符串编码、结构体对齐、调用约定、线程安全要求必须写清楚。
- 如果厂商 SDK 非线程安全，每个设备必须串行化访问，或使用 dedicated thread。
- 如果 DLL 调用会阻塞，使用 dedicated thread 或 `spawn_blocking`，并设置并发上限。

## 8. Windows 7 兼容策略

Windows 7 应视为单独兼容版本，而不是普通构建开关。

原因：

- Microsoft Edge/WebView2 Runtime `109` 是最后一个支持 Windows 7、Windows 8 和 Windows 8.1 的版本。
- Rust `x86_64-win7-windows-msvc` 是 Tier 3 target，不提供预编译标准库，需要 nightly `build-std` 或自建标准库。
- 现场 Win7 机器常见 TLS、证书、补丁、WebView2 安装状态不一致，不能假设在线安装总是成功。

建议发布两个目标：

| 目标 | 支持系统 | 构建目标 | 说明 |
|---|---|---|---|
| 主版本 | Windows 10/11 | `x86_64-pc-windows-msvc` | 默认交付 |
| 兼容版 | Windows 7 SP1 | `x86_64-win7-windows-msvc` | 单独测试、单独发布 |

Win7 构建建议：

```powershell
rustup toolchain install nightly
rustup +nightly component add rust-src
cargo +nightly tauri build --target x86_64-win7-windows-msvc -Zbuild-std
```

`oldwin` 建议配置：

```toml
[target.'cfg(target_family = "windows")'.dependencies]
oldwin-targets = { version = "0.1.1", default-features = false, features = [
  "win7",
  "yy-thunks",
  "vc-ltl5",
] }

[target.'cfg(target_family = "windows")'.build-dependencies]
oldwin = { version = "0.1.3", default-features = false, features = [
  "win7",
  "yy-thunks",
  "vc-ltl5",
] }
```

`build.rs`：

```rust
fn main() {
    #[cfg(target_family = "windows")]
    {
        oldwin::inject();
    }
}
```

安装器建议：

- 优先 NSIS。
- Win7 离线现场优先使用 `offlineInstaller` 或 `fixedRuntime`。
- 如果使用 fixed runtime，版本不得高于 Win7 可运行的 WebView2 109 边界。
- 前端构建目标需要兼容 Chromium 109，不要使用只在新 Chromium 中可用的 API。

## 9. 依赖建议

依赖版本应以实际 `Cargo.lock` 为准，并在项目落地时再次确认。

| 依赖 | 建议 | 说明 |
|---|---|---|
| `tauri` | `2.x` | 保留主版本，实际版本由 `Cargo.lock` 固定。 |
| `tokio` | `1.x` | 采集调度、任务、通道、定时器。 |
| `tauri-plugin-shell` | `2.x` | 仅在确实需要外部命令时启用。 |
| `tauri-plugin-dialog` | `2.x` | 导入配置、导出文件。 |
| `tauri-plugin-fs` | `2.x` | 谨慎授权，前端不直接做业务文件读写。 |
| `tauri-plugin-notification` | `2.x` | 告警通知，Win7 需单独实测。 |
| `tauri-plugin-updater` | `2.x` | 远程更新，必须配置签名验证。 |
| `libloading` | `0.8` 或 `0.9` | 以 MSRV 和 Win7 构建结果为准。 |
| `tracing` | `0.1` | 结构化日志。 |
| `tracing-subscriber` | `0.3` | 日志订阅、过滤。 |
| `tracing-appender` | `0.2` | 文件轮转。 |
| `thiserror` | `2.x` | 库内错误类型。 |
| `anyhow` | `1.x` | 应用边界聚合错误。 |
| `serde` | `1.x` | DTO 和配置序列化。 |
| `tokio-util` | `0.7` | `CancellationToken`。 |
| `dashmap` | 可选 | 不要默认引入；先确认是否真需要并发 map。 |
| `rmp-serde` | 可选 | 用于磁盘缓存、自定义协议或外部 WebSocket，不用于普通 Tauri IPC。 |
| `tokio-tungstenite` | 可选 | 只有外部 Web 监控需要时再引入。 |
| `taurpc` | 暂不作为 P0 必选 | 版本和 Tauri 2 配套情况需单独验证。 |
| `cargo-fuzz` | 不放入 dev-dependencies | 作为 cargo 子命令安装，通常依赖 nightly。 |

## 10. 前端设计建议

技术栈直接定为 Vue 3，不再保留 React 备选，避免团队和代码生成路径分叉。

推荐界面结构：

```text
顶部：会话状态 / 开始停止 / 记录 / 导出 / 报警
左侧：设备树 / 通道启用 / 健康状态
中间：实时波形 / 多通道叠加 / 游标
右侧：当前设备参数 / 量程 / 校准状态
底部：事件日志 / 队列深度 / 丢帧 / 丢点指标
```

实时波形策略：

- 中低频趋势图可用 ECharts。
- 高频波形优先 Canvas 或 WebGL 自绘。
- 前端只维护 UI ring buffer，不保存全量原始数据。
- 使用 `requestAnimationFrame` 控制渲染节奏。
- 后端负责降采样和批次合并，避免 WebView 被原始采样率压垮。
- 设备状态以后端主动推送为主，前端定时 reconcile 为辅。

前端模块建议沿用工作区规则：

```text
frontend/src/modules/device/
frontend/src/modules/acquisition/
frontend/src/modules/history/
frontend/src/modules/settings/
frontend/src/shared/
frontend/src/api/
```

## 11. 日志与可观测性

日志使用 `tracing`，并按设备、任务、采集会话建立 span。

建议关键字段：

- `device_id`
- `session_id`
- `sequence_start`
- `sample_count`
- `sample_rate_hz`
- `queue_depth`
- `dropped_ui_frames`
- `sequence_gap_count`
- `driver_error_code`
- `reconnect_count`
- `latency_ms`

建议指标：

- 原始采集序列号缺口。
- UI 丢帧数。
- 存储队列深度。
- 实时推送队列深度。
- 每设备最新样本年龄。
- P50、P95、P99、P99.9 延迟。
- 进程 RSS/private bytes。
- 线程数、句柄数。
- 磁盘写入延迟。
- 重连次数。

日志文件建议写入应用数据目录，例如：

```text
%APPDATA%/<app-name>/logs/
```

按天轮转，保留最近 30 天。原始字节流只在 DEBUG 或故障采样模式启用，避免长稳运行时日志爆量。

## 12. 测试策略

### 12.1 P0 可行性测试

P0 必须先证明：

- Tauri 2 + Vue 3 + Rust 后端 crate 能跑通。
- mock 设备能以目标频率产生数据。
- Channel 能把 UI 降采样批次推到前端。
- Windows 10/11 普通构建通过。
- Windows 7 兼容构建能编译。
- Win7 SP1 虚拟机能安装、启动、显示基础 UI。
- WebView2 安装策略可用。

### 12.2 单元测试

覆盖：

- `core` 中采样规则、单位转换、触发规则。
- 协议帧解析。
- ring buffer。
- 错误码映射。
- 状态机迁移。

### 12.3 集成测试

覆盖：

- mock driver 正常采集。
- mock driver 随机延迟。
- mock driver 返回错误。
- 设备断连和恢复。
- 同一设备并发命令冲突。
- 存储队列满。
- UI 消费慢。

### 12.4 HIL 测试

放在：

```text
projects/<name>/tests/hil/
```

覆盖真实设备：

- 插拔。
- 上电/断电。
- 高采样率。
- 多设备并发。
- 厂商 DLL 返回异常码。
- 采集卡缓冲区溢出。

### 12.5 长稳测试

建议：

- Windows 10/11：72 小时。
- Windows 7 SP1：72 小时，作为兼容版发布门禁。
- 每小时采集指标快照。
- 失败时保留最近日志、配置、指标和最小复现条件。

### 12.6 模糊测试

`cargo-fuzz` 只用于纯解析器：

- 厂商协议帧解析。
- 配置导入。
- 二进制缓存格式。
- 历史数据导入格式。

不要 fuzz 整个 async Actor 或真实硬件路径。

## 13. 构建与发布

开发构建：

```powershell
cargo tauri dev
```

Windows 10/11 发布构建：

```powershell
cargo tauri build --target x86_64-pc-windows-msvc
```

Windows 7 兼容构建：

```powershell
cargo +nightly tauri build --target x86_64-win7-windows-msvc -Zbuild-std
```

发布建议：

- NSIS 作为 Windows 安装器。
- 安装包和主程序都做代码签名。
- 更新器使用 `tauri-plugin-updater`，并配置签名校验。
- Win7 兼容版单独版本号或单独渠道发布。
- 发布产物附带 Changelog、安装说明和已验证系统矩阵。

## 14. 推荐开发路线

| 阶段 | 目标 | 关键交付 |
|---|---|---|
| P0 可行性验证 | 证明路线能跑 | Tauri + Vue + `api-rs` + mock 1 kHz + Channel 推送 + Win7 烟测 |
| P1 架构骨架 | 建好六边形边界 | `core/ports/usecase/adapters`、mock driver、supervisor、typed error |
| P2 实时采集闭环 | 多设备稳定采集 | bounded queues、storage writer、UI decimation、状态机 |
| P3 硬件接入 | 接入一个真实厂商 SDK | `libloading` 安全封装、错误码映射、断连恢复 |
| P4 长稳与性能 | 验证长期运行 | 24h/72h soak、延迟、内存、队列、丢点指标 |
| P5 打包发布 | 可交付安装包 | NSIS、WebView2 策略、签名、更新器、部署文档 |

## 15. 参考链接

- Tauri Windows installer：<https://v2.tauri.app/distribute/windows-installer/>
- Tauri command / IPC：<https://v2.tauri.app/zh-cn/develop/calling-rust/>
- Tauri calling frontend / Channel：<https://v2.tauri.app/fr/develop/calling-frontend/>
- Tauri updater：<https://v2.tauri.app/fr/plugin/updater/>
- Rust Win7 target：<https://doc.rust-lang.org/nightly/rustc/platform-support/win7-windows-msvc.html>
- Microsoft WebView2 Windows 7 support end：<https://blogs.windows.com/msedgedev/2022/12/09/microsoft-edge-and-webview2-ending-support-for-windows-7-and-windows-8-8-1/>
- oldwin crate：<https://docs.rs/crate/oldwin/latest>
- cargo-fuzz setup：<https://rust-fuzz.github.io/book/cargo-fuzz/setup.html>

