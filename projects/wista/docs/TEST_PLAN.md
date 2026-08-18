# wista 测试计划与用例文档

> **版本**: 0.1.0  
> **日期**: 2026-06-02  
> **范围**: Go 后端 (六边形架构) + Vue 3 前端 (Wails v2)  
> **测试框架**: Go testing + Vitest (前端)  

---

## 1. 测试策略概述

### 1.1 测试分层

```
┌─────────────────────────────────────────────────────────────┐
│  E2E 测试 (端到端)                                          │
│  - 完整用户流程: 添加设备 → 连接 → 配置 → 采集 → 保存 → 断开  │
└─────────────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────────────┐
│  集成测试 (Integration)                                      │
│  - 多组件协作: usecase + adapter + config + recording       │
│  - 数据流验证: 采集 → relay → Events → 前端渲染              │
└─────────────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────────────┐
│  单元测试 (Unit)                                             │
│  - Go: core, usecase, ports, adapters (各层独立测试)         │
│  - TS: stores, composables, bridge, utils                   │
└─────────────────────────────────────────────────────────────┘
```

### 1.2 测试目标

| 维度 | 目标 |
|------|------|
| **功能正确性** | 设备连接/断开、采集启停、数据流推送、配置应用、录制保存 |
| **状态一致性** | 前端状态与后端真实状态同步，避免"假连接"、"假采集" |
| **异常处理** | 网络断开、设备离线、重复操作、非法参数、资源泄漏 |
| **并发安全** | 多设备同时采集、goroutine 泄漏、channel 关闭安全 |
| **UI 交互** | 按钮状态、模态框、空状态、加载状态、错误提示 |

---

## 2. Go 后端测试用例

### 2.1 Core 层测试 (`core/`)

#### TC-CORE-01: DeviceStatus 枚举转换
```go
func TestDeviceStatus_String(t *testing.T)
```
- **输入**: `StatusDisconnected`, `StatusConnected`, `StatusAcquiring`, `StatusError`, `DeviceStatus(99)`
- **期望输出**: `"Disconnected"`, `"Connected"`, `"Acquiring"`, `"Error"`, `"Unknown"`
- **优先级**: P1

#### TC-CORE-02: TemperatureSnapshot 序列化
```go
func TestTemperatureSnapshot_JSON(t *testing.T)
```
- **输入**: `TemperatureSnapshot{DeviceID:"dev1", Timestamp:1234567890, Values:[]float64{1.0,2.0}, Unit:"°C"}`
- **期望输出**: JSON 字段名匹配 `json` tag（deviceId, timestamp, values, unit）
- **优先级**: P2

---

### 2.2 Usecase 层测试 (`usecase/`)

#### TC-UC-01: 获取空设备列表
```go
func TestDeviceUsecase_GetProfiles_Empty(t *testing.T)
```
- **前置条件**: 配置文件不存在或为空
- **操作**: `uc.GetProfiles()`
- **期望**: 返回空切片 `[]`, 不 panic
- **优先级**: P1 ✅ 已实现

#### TC-UC-02: 添加并查询设备
```go
func TestDeviceUsecase_UpsertAndGet(t *testing.T)
```
- **操作**: `UpsertProfile(p)` → `GetProfiles()`
- **期望**: 列表长度=1, 第一个元素 ID 匹配
- **优先级**: P1 ✅ 已实现

#### TC-UC-03: 连接不存在的设备
```go
func TestDeviceUsecase_ConnectMissingProfile(t *testing.T)
```
- **操作**: `Connect("nonexistent")`
- **期望**: 返回 error, 不 panic
- **优先级**: P1 ✅ 已实现

#### TC-UC-04: 连接已存在的设备
```go
func TestDeviceUsecase_ConnectExistingProfile(t *testing.T)
```
- **前置条件**: 已保存 profile
- **操作**: `Connect("dev1")`
- **期望**: 无 error
- **优先级**: P1 ✅ 已实现

#### TC-UC-05: 启动/停止采集完整流程
```go
func TestDeviceUsecase_StartStopAcquisition(t *testing.T)
```
- **前置条件**: 设备已连接
- **操作**: `StartAcquisition("dev1")` → 读取 channel → `StopAcquisition("dev1")`
- **期望**: channel 能收到 snapshot, snapshot.DeviceID 正确, 停止后 channel 关闭
- **优先级**: P1 ✅ 已实现

#### TC-UC-06: 重复启动采集应报错
```go
func TestDeviceUsecase_DoubleAcquisition(t *testing.T)
```
- **前置条件**: 设备已连接且正在采集
- **操作**: 第二次调用 `StartAcquisition("dev1")`
- **期望**: 返回 error（"already acquiring"）
- **优先级**: P1

#### TC-UC-07: 未连接设备启动采集应报错
```go
func TestDeviceUsecase_StartAcquisition_NotConnected(t *testing.T)
```
- **操作**: 对未连接设备调用 `StartAcquisition("dev1")`
- **期望**: 返回 error（"not connected"）
- **优先级**: P1

#### TC-UC-08: 应用配置到已连接设备
```go
func TestDeviceUsecase_ApplyConfig(t *testing.T)
```
- **前置条件**: 设备已连接
- **操作**: `ApplyConfig("dev1", cfg)`
- **期望**: 无 error
- **优先级**: P1 ✅ 已实现

#### TC-UC-09: 应用配置到未连接设备应报错
```go
func TestDeviceUsecase_ApplyConfig_NotConnected(t *testing.T)
```
- **操作**: 对未连接设备调用 `ApplyConfig`
- **期望**: 返回 error
- **优先级**: P1

#### TC-UC-10: 扫描设备（无 scanner）
```go
func TestDeviceUsecase_ScanDevices_NilScanner(t *testing.T)
```
- **前置条件**: scanner 未设置
- **操作**: `ScanDevices()`
- **期望**: 返回空切片, 无 error
- **优先级**: P2 ✅ 已实现

#### TC-UC-11: 扫描设备（有 scanner）
```go
func TestDeviceUsecase_ScanDevices_WithScanner(t *testing.T)
```
- **前置条件**: 已设置模拟 scanner
- **操作**: `ScanDevices()`
- **期望**: 返回预期结果列表
- **优先级**: P2 ✅ 已实现

#### TC-UC-12: 录制生命周期
```go
func TestRecordingUsecase(t *testing.T)
```
- **操作**: `Start()` → `Write(snapshot)` → `Stop()` → `Status()`
- **期望**: 状态从 Idle → Active → Idle, snapshotCount=1
- **优先级**: P1 ✅ 已实现

#### TC-UC-13: 录制时写入多条数据
```go
func TestRecordingUsecase_MultipleWrites(t *testing.T)
```
- **操作**: `Start()` → 写入 10 条 → `Stop()`
- **期望**: `snapshotCount=10`
- **优先级**: P1

#### TC-UC-14: 未启动录制时写入应忽略
```go
func TestRecordingUsecase_WriteWithoutStart(t *testing.T)
```
- **操作**: 直接 `Write(snapshot)` 不调用 `Start()`
- **期望**: 无 error, snapshotCount 不变
- **优先级**: P2

---

### 2.3 Adapter 层测试 (`adapters/`)

#### TC-ADP-01: T1603Adapter 连接/断开
```go
func TestT1603Adapter_ConnectDisconnect(t *testing.T)
```
- **操作**: `Connect(profile)` → `Status(id)` → `Disconnect(id)` → `Status(id)`
- **期望**: 连接后状态 Connected, 断开后状态 Disconnected
- **优先级**: P1

#### TC-ADP-02: T1603Adapter 重复连接应报错
```go
func TestT1603Adapter_DoubleConnect(t *testing.T)
```
- **操作**: 对同一设备调用两次 `Connect`
- **期望**: 第二次返回 error（"already connected"）
- **优先级**: P1

#### TC-ADP-03: T1603Adapter 采集数据流
```go
func TestT1603Adapter_AcquisitionStream(t *testing.T)
```
- **操作**: `Connect` → `StartAcquisition` → 读取 channel → `StopAcquisition`
- **期望**: channel 收到 TemperatureSnapshot, Values 长度为 16
- **优先级**: P1

#### TC-ADP-04: T1603Adapter 停止采集后资源清理
```go
func TestT1603Adapter_StopAcquisitionCleanup(t *testing.T)
```
- **操作**: `StartAcquisition` → `StopAcquisition` → 检查内部 maps
- **期望**: channels, stopChs, sinks 中对应设备条目已删除
- **优先级**: P1

#### TC-ADP-05: T1603Adapter 断开时自动停止采集
```go
func TestT1603Adapter_DisconnectStopsAcquisition(t *testing.T)
```
- **操作**: `Connect` → `StartAcquisition` → `Disconnect`
- **期望**: 采集自动停止, 无 goroutine 泄漏
- **优先级**: P1

#### TC-ADP-07: JSONConfigStore 保存/加载
```go
func TestJSONConfigStore_SaveLoad(t *testing.T)
```
- **操作**: `SaveProfile(p)` → `LoadProfiles()`
- **期望**: 加载结果与保存数据一致
- **优先级**: P1 ✅ 已实现

#### TC-ADP-08: JSONConfigStore 删除
```go
func TestJSONConfigStore_Delete(t *testing.T)
```
- **操作**: `SaveProfile(p)` → `DeleteProfile(id)` → `LoadProfiles()`
- **期望**: 加载结果为空
- **优先级**: P1 ✅ 已实现

#### TC-ADP-09: CSVRecorder 文件生成
```go
func TestCSVRecorder_FileOutput(t *testing.T)
```
- **操作**: `Start(dir, prefix)` → `Write(snapshot)` → `Stop()`
- **期望**: 目录下生成 CSV 文件, 内容包含表头和数据行
- **优先级**: P1 ✅ 已实现

#### TC-ADP-10: CSVRecorder 多条数据追加
```go
func TestCSVRecorder_Append(t *testing.T)
```
- **操作**: `Start` → 写入 5 条 → `Stop` → `Start`（同一文件）→ 写入 3 条 → `Stop`
- **期望**: 文件包含 8 条数据
- **优先级**: P2

---

### 2.4 Backend (App) 层测试 (`backend/`)

#### TC-APP-01: 获取空设备列表
```go
func TestGetProfiles_Empty(t *testing.T)
```
- **期望**: 返回空切片
- **优先级**: P1 ✅ 已实现

#### TC-APP-02: 添加并查询设备
```go
func TestUpsertAndGetProfiles(t *testing.T)
```
- **期望**: 添加后能查询到
- **优先级**: P1 ✅ 已实现

#### TC-APP-03: 删除设备
```go
func TestDeleteProfile(t *testing.T)
```
- **操作**: `UpsertProfile` → `DeleteProfile` → `GetProfiles`
- **期望**: 删除后列表为空
- **优先级**: P1 ✅ 已实现

#### TC-APP-04: 连接/断开设备
```go
func TestConnectDisconnect(t *testing.T)
```
- **期望**: 连接后状态 Connected, 断开后状态 Disconnected
- **优先级**: P1 ✅ 已实现

#### TC-APP-05: 连接不存在的设备
```go
func TestConnectNonexistent(t *testing.T)
```
- **期望**: 返回 error
- **优先级**: P1 ✅ 已实现

#### TC-APP-06: 应用配置到未连接设备
```go
func TestApplyConfigNotConnected(t *testing.T)
```
- **期望**: 返回 error
- **优先级**: P1 ✅ 已实现

#### TC-APP-07: 应用配置到已连接设备
```go
func TestApplyConfig(t *testing.T)
```
- **期望**: 无 error
- **优先级**: P1 ✅ 已实现

#### TC-APP-08: 扫描设备
```go
func TestScanDevices(t *testing.T)
```
- **期望**: 返回扫描到的设备列表
- **优先级**: P2 ✅ 已实现

#### TC-APP-09: 录制生命周期
```go
func TestRecordingLifecycle(t *testing.T)
```
- **期望**: Idle → Active → Idle 状态转换正确
- **优先级**: P1 ✅ 已实现

#### TC-APP-10: 采集完整流程
```go
func TestAcquisitionFlow(t *testing.T)
```
- **操作**: Connect → StartAcquisition → 检查状态 → StopAcquisition
- **期望**: 状态从 Connected → Acquiring → Connected
- **优先级**: P1 ✅ 已实现

#### TC-APP-11: 重复启动采集应报错
```go
func TestDoubleAcquisition(t *testing.T)
```
- **期望**: 第二次返回 error
- **优先级**: P1 ✅ 已实现

#### TC-APP-12: 停止未采集的设备
```go
func TestStopAcquisitionIdle(t *testing.T)
```
- **期望**: 无 error（幂等）
- **优先级**: P2 ✅ 已实现

#### TC-APP-13: relayStream 正确推送 Events
```go
func TestRelayStream_EmitsEvents(t *testing.T)
```
- **前置条件**: 使用模拟 runtime.EventsEmit
- **操作**: StartAcquisition
- **期望**: 收到 `daq:payload` 事件, 数据格式正确
- **优先级**: P1

#### TC-APP-14: relayStream 在 StopAcquisition 后停止
```go
func TestRelayStream_StopsOnStop(t *testing.T)
```
- **操作**: StartAcquisition → 收到数据 → StopAcquisition
- **期望**: Stop 后不再收到事件
- **优先级**: P1

#### TC-APP-15: relayStream 在 channel 关闭后推送状态事件
```go
func TestRelayStream_StatusEventOnClose(t *testing.T)
```
- **操作**: StartAcquisition → 设备断开（channel 关闭）
- **期望**: 收到 `daq:device-status` 事件, status="Connected"
- **优先级**: P1

#### TC-APP-16: GetStatusString 返回正确字符串
```go
func TestGetStatusString(t *testing.T)
```
- **输入**: 已连接设备、未连接设备
- **期望**: "Connected" / "Disconnected"
- **优先级**: P2

#### TC-APP-17: SyncDeviceStatus 推送事件
```go
func TestSyncDeviceStatus(t *testing.T)
```
- **操作**: `SyncDeviceStatus("dev1")`
- **期望**: 收到 `daq:device-status` 事件
- **优先级**: P2

---

## 3. 前端测试用例 (Vue 3 + TypeScript)

### 3.1 测试配置

- **框架**: Vitest + jsdom + @vue/test-utils
- **命令**: `npm test` (vitest run)
- **文件匹配**: `src/**/*.test.ts`

### 3.2 Bridge 层测试 (`src/bridge/`)

#### TC-FRONT-BRIDGE-01: deviceBridge 类型导出完整
```ts
// src/bridge/deviceBridge.test.ts
import { describe, it, expect } from 'vitest'
import * as bridge from './deviceBridge'

describe('deviceBridge', () => {
  it('should export all required functions', () => {
    expect(typeof bridge.getProfiles).toBe('function')
    expect(typeof bridge.connect).toBe('function')
    expect(typeof bridge.startAcquisition).toBe('function')
    expect(typeof bridge.onPayload).toBe('function')
    expect(typeof bridge.onDeviceStatus).toBe('function')
    expect(typeof bridge.getStatusString).toBe('function')
  })
})
```
- **优先级**: P2

#### TC-FRONT-BRIDGE-02: TC_RANGES 数据正确
```ts
it('should have correct K-type range', () => {
  expect(bridge.TC_RANGES.K).toEqual({ min: -200, max: 1372 })
})
```
- **优先级**: P2

---

### 3.3 Store 层测试 (`src/stores/`)

#### TC-FRONT-STORE-01: deviceStore 初始化状态
```ts
// src/stores/deviceStore.test.ts
import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useDeviceStore } from './deviceStore'

describe('deviceStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('should have correct initial state', () => {
    const store = useDeviceStore()
    expect(store.profiles).toEqual([])
    expect(store.selectedId).toBeNull()
    expect(store.statusMap).toEqual({})
    expect(store.historyMap).toEqual({})
  })
})
```
- **优先级**: P1

#### TC-FRONT-STORE-02: pushSnapshot 添加数据
```ts
it('should push snapshot to history', () => {
  const store = useDeviceStore()
  store.pushSnapshot({ deviceId: 'dev1', timestamp: 1, values: [1, 2], unit: '°C' })
  expect(store.snapshotMap['dev1']).toBeDefined()
  expect(store.historyFor('dev1')).toHaveLength(1)
})
```
- **优先级**: P1

#### TC-FRONT-STORE-03: pushSnapshot 历史限制
```ts
it('should limit history to MAX_HISTORY', () => {
  const store = useDeviceStore()
  for (let i = 0; i < 250; i++) {
    store.pushSnapshot({ deviceId: 'dev1', timestamp: i, values: [i], unit: '°C' })
  }
  expect(store.historyFor('dev1')).toHaveLength(200)
})
```
- **优先级**: P1

#### TC-FRONT-STORE-04: handleDeviceStatusEvent 更新状态
```ts
it('should update status from device status event', () => {
  const store = useDeviceStore()
  store.handleDeviceStatusEvent({ deviceId: 'dev1', status: 'Connected' })
  expect(store.statusFor('dev1')).toBe('Connected')
})
```
- **优先级**: P1

#### TC-FRONT-STORE-05: toggleChartSelection
```ts
it('should toggle chart selection', () => {
  const store = useDeviceStore()
  store.toggleChartSelection('dev1', 0)
  expect(store.isChartSelected('dev1', 0)).toBe(true)
  store.toggleChartSelection('dev1', 0)
  expect(store.isChartSelected('dev1', 0)).toBe(false)
})
```
- **优先级**: P2

#### TC-FRONT-STORE-06: initializeDefaultChartSelection
```ts
it('should select first 4 enabled channels by default', () => {
  const store = useDeviceStore()
  store.profiles = [{
    id: 'dev1', name: 'Test', address: 'x', port: 1, samplingRate: 5,
    channels: Array.from({ length: 16 }, (_, i) => ({
      index: i, name: `CH${i + 1}`, enabled: i < 8, unit: '°C', color: '#000', precision: 2
    })),
    t1603Config: { thermocoupleTypes: 'K'.repeat(16), channelMask: 'FFFF', samplingRate: 10, averageCount: 4, showTimestamp: false, showSequence: false },
    createdAt: 1
  }]
  store.initializeDefaultChartSelection('dev1')
  expect(store.isChartSelected('dev1', 0)).toBe(true)
  expect(store.isChartSelected('dev1', 3)).toBe(true)
  expect(store.isChartSelected('dev1', 4)).toBe(false)
})
```
- **优先级**: P2

#### TC-FRONT-STORE-07: selectDevice 初始化默认选择
```ts
it('should initialize chart selection when selecting device', () => {
  const store = useDeviceStore()
  store.profiles = [{
    id: 'dev1', name: 'Test', address: 'x', port: 1, samplingRate: 5,
    channels: Array.from({ length: 16 }, (_, i) => ({
      index: i, name: `CH${i + 1}`, enabled: true, unit: '°C', color: '#000', precision: 2
    })),
    t1603Config: { thermocoupleTypes: 'K'.repeat(16), channelMask: 'FFFF', samplingRate: 10, averageCount: 4, showTimestamp: false, showSequence: false },
    createdAt: 1
  }]
  store.selectDevice('dev1')
  expect(store.selectedId).toBe('dev1')
  // 默认选中前4个通道
  expect(store.isChartSelected('dev1', 0)).toBe(true)
})
```
- **优先级**: P2

#### TC-FRONT-STORE-08: recordingStore 初始状态
```ts
// src/stores/recordingStore.test.ts
it('should have correct initial state', () => {
  const store = useRecordingStore()
  expect(store.isRecording).toBe(false)
  expect(store.snapshotCount).toBe(0)
})
```
- **优先级**: P2

---

### 3.4 组件测试 (`src/components/`)

#### TC-FRONT-COMP-01: ChannelCard 渲染
```ts
// src/components/device/ChannelCard.test.ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ChannelCard from './ChannelCard.vue'

describe('ChannelCard', () => {
  it('should render channel index and name', () => {
    const wrapper = mount(ChannelCard, {
      props: { index: 0, value: 25.5, unit: '°C', color: '#3b82f6', name: '通道1', chartSelected: false, active: false }
    })
    expect(wrapper.text()).toContain('CH01')
    expect(wrapper.text()).toContain('通道1')
    expect(wrapper.text()).toContain('25.50')
  })

  it('should display --- for invalid value', () => {
    const wrapper = mount(ChannelCard, {
      props: { index: 0, value: NaN, unit: '°C', color: '#3b82f6', name: '通道1', chartSelected: false, active: false }
    })
    expect(wrapper.text()).toContain('---')
  })
})
```
- **优先级**: P2

#### TC-FRONT-COMP-02: ChannelCard 切换波形图
```ts
it('should emit toggleChart on button click', async () => {
  const wrapper = mount(ChannelCard, {
    props: { index: 0, value: 25, unit: '°C', color: '#3b82f6', name: '通道1', chartSelected: false, active: false }
  })
  await wrapper.find('.card__toggle').trigger('click')
  expect(wrapper.emitted('toggleChart')).toBeTruthy()
})
```
- **优先级**: P2

#### TC-FRONT-COMP-03: RealtimeChart 空状态
```ts
// src/components/device/RealtimeChart.test.ts
it('should show empty state when no data', () => {
  // 需要 mock deviceStore
})
```
- **优先级**: P2

#### TC-FRONT-COMP-04: MonitorView 空状态
```ts
// src/views/MonitorView.test.ts
it('should show empty state when no device selected', () => {
  const wrapper = mount(MonitorView)
  expect(wrapper.find('.detail__empty').exists()).toBe(true)
})
```
- **优先级**: P2

---

## 4. 测试执行计划

### 4.1 执行命令

```bash
# Go 后端全部测试
cd projects/wista/apps/desktop-wails
go test ./... -v

# Go 后端带覆盖率
go test ./... -cover -coverprofile=coverage.out
go tool cover -html=coverage.out

# 前端测试
cd projects/wista/apps/desktop-wails/frontend
npm test

# 前端带覆盖率
npm test -- --coverage

# 前端类型检查
npm run typecheck
```

### 4.2 优先级定义

| 优先级 | 含义 | 执行时机 |
|--------|------|----------|
| P0 | 阻塞发布 | 每次提交前必跑 |
| P1 | 核心功能 | CI/CD 自动跑 |
| P2 | 一般功能 | 发布前手动跑 |
| P3 | 边缘场景 | 按需执行 |

### 4.3 测试矩阵

| 测试类型 | 文件位置 | 数量 | 状态 |
|----------|----------|------|------|
| Go Unit - usecase | `usecase/*_test.go` | 14 | 10 ✅ + 4 待实现 |
| Go Unit - backend | `backend/*_test.go` | 17 | 12 ✅ + 5 待实现 |
| Go Unit - adapters | `adapters/**/*_test.go` | 10 | 部分 ✅ |
| Go Integration | `tests/integration/*_test.go` | 4 | 1 ✅ + 3 待实现 |
| TS Unit - stores | `src/stores/*.test.ts` | 8 | 待创建 |
| TS Unit - bridge | `src/bridge/*.test.ts` | 2 | 待创建 |
| TS Unit - components | `src/components/**/*.test.ts` | 4 | 待创建 |

---

## 5. 待实现测试清单

### 5.1 Go 后端（高优先级）

- [ ] `usecase/device_usecase_test.go` - TC-UC-06, TC-UC-07, TC-UC-09, TC-UC-13, TC-UC-14
- [ ] `backend/app_test.go` - TC-APP-13, TC-APP-14, TC-APP-15, TC-APP-16, TC-APP-17
- [ ] `adapters/hardware/t1603_adapter_test.go` - TC-ADP-01 ~ TC-ADP-05
- [ ] `tests/integration/` - TC-INT-02, TC-INT-03, TC-INT-04

### 5.2 前端（高优先级）

- [ ] `src/stores/deviceStore.test.ts` - TC-FRONT-STORE-01 ~ 07
- [ ] `src/stores/recordingStore.test.ts` - TC-FRONT-STORE-08
- [ ] `src/bridge/deviceBridge.test.ts` - TC-FRONT-BRIDGE-01, 02
- [ ] `src/components/device/ChannelCard.test.ts` - TC-FRONT-COMP-01, 02
- [ ] `src/components/device/RealtimeChart.test.ts` - TC-FRONT-COMP-03
- [ ] `src/views/MonitorView.test.ts` - TC-FRONT-COMP-04

---

## 6. 附录：Mock 辅助工具

### 6.1 Go Fake 端口实现

```go
// 参考 usecase/device_usecase_test.go 中的 fakeDevicePort/fakeConfigPort
// 已提供完整实现，可直接复用
```

### 6.2 前端 Mock Wails Runtime

```ts
// vitest.setup.ts (需创建)
import { vi } from 'vitest'

vi.mock('../../wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn(),
  EventsOff: vi.fn(),
}))

vi.mock('../../wailsjs/go/backend/App', () => ({
  GetProfiles: vi.fn(() => Promise.resolve([])),
  Connect: vi.fn(() => Promise.resolve()),
  StartAcquisition: vi.fn(() => Promise.resolve()),
  // ... 其他方法
}))
```
