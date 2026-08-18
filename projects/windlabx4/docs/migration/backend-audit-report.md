# WindLabX4 Go 后端功能审查报告与实施计划

> **状态（2026-06）：** 本文档作为**后端功能清单**仍然有效——记录哪些后端能力已从 Cursor DAQ 迁移到 WindLabX4。UI / 视觉对等已不再是设计目标（见 `../../DESIGN.md`），但后端业务逻辑覆盖与本文档结论无关，可继续作为功能差异追踪表使用。

> 审查日期：2026-05-22
> 审查范围：`projects/WindLabX4/services/api-go` 全部后端模块
> 参考标准：`C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\Ai Agent\Cursor DAQ\src\main`

---

## 一、审查概述

本次审查对 WindLabX4 项目的 Go 后端进行了逐模块的全面检查，从功能完整性、API 等效性、前端集成、代码质量和测试覆盖五个维度与参考项目（Cursor DAQ）进行了对比分析。

### 整体完成度

| 模块 | 完成度 | 优先级 |
|------|--------|--------|
| 数据采集中心 (AcquisitionHub) | 90% | - |
| 设备管理 (DeviceManager) | 85% | P1 |
| 数据存储 (Storage) | 75% | P2 |
| 设备驱动适配层 (Hardware Adapters) | 70% | P2 |
| 遍历测试 (Traversal) | 55% | P1 |
| 运动控制 (Motion) | 60% | P1 |
| 校准模块 (Calibration) | 40% | P0 |
| 报告生成 (Report) | 25% | P1 |
| 插值算法 (Interpolation) | 100% | Shared |
| 运行时基础设施 (Runtime) | 15% | P2 |

**整体完成度约 55%。** 核心框架和基础功能已就位，但校准算法、插值计算和 PDF 报告等专业领域功能严重缺失。

---

## 二、模块详细审查结果

### 2.1 设备管理模块 (DeviceManager) — 完成度: 85%

| 功能 | 参考项目 | Go 实现 | 状态 |
|------|---------|---------|------|
| 设备 Profile CRUD | `deviceStore.ts` + `JsonProfileStore.ts` | `adapters/config/file_profile_store.go` | ✅ 完成 |
| 设备连接/断开 | `DeviceManager.ts` connect/disconnect | `usecase/device_manager.go` | ✅ 完成 |
| 采集启动/停止 | startAcquisitionFor/stopAcquisitionFor | StartAcquisition/StopAcquisition | ✅ 完成 |
| 设备扫描 | `DeviceScanService.ts` + `DaqP1604Scanner.ts` + `DaqT1603Scanner.ts` | `adapters/scan/network_scanner.go` | ⚠️ 部分 — 仅有 UDP 广播扫描，缺少专用扫描器 |
| 熔断器 (Circuit Breaker) | `allowConnection()` + `recordSuccess/Failure` | 无 | ❌ 缺失 |
| 设备实例状态管理 | `instances` Map + `getAllStatuses()` | `GetStatus(id)` 单设备查询 | ⚠️ 部分 — 缺少批量状态查询 |
| DAQ-T-1603 配置 | getDaqT1603Config / applyDaqT1603Config | GetDaqT1603Config / ApplyDaqT1603Config | ✅ 完成 |
| 单位设置 | setUnit() | SetUnit | ✅ 完成 |
| Profile 删除 | deleteProfile() | DeleteProfile | ✅ 完成 |
| Capabilities 接口 | `GET /api/devices/capabilities` | 无 | ❌ 缺失 |

**关键差异:**
- 参考项目有**熔断器机制**（连接失败后冷却期），Go 版本完全缺失
- 参考项目有 `getInstances()` 和 `getAllStatuses()` 批量接口，Go 版本仅有单设备查询
- 参考项目有 `capabilities` 接口（WTN PXI 功能开关），Go 版本缺失

---

### 2.2 数据采集中心 (AcquisitionHub) — 完成度: 90%

| 功能 | 参考项目 | Go 实现 | 状态 |
|------|---------|---------|------|
| 数据接收与分发 | `onData()` + `dataSink` | `OnData()` | ✅ 完成 |
| 发布频率控制 | `startPublishing(hz)` / `getPublishRate()` | SetPublishRate / PublishRate | ✅ 完成 |
| SSE 流式推送 | WebSocket `broadcastEvent` | `handleDaqStream` (SSE) | ✅ 完成 (改用 SSE) |
| 最新数据查询 | `getLatestData()` | GetLatestData | ✅ 完成 |
| 订阅机制 | WebSocket subscribe/unsubscribe | Subscribe (channel-based) | ✅ 完成 |
| 历史数据 | 无 | GetRecentData | ✅ 增强 |

**关键差异:**
- 参考项目使用 **WebSocket** 推送，Go 版本使用 **SSE (Server-Sent Events)**，前端已适配
- Go 版本增加了历史数据缓冲功能，是增强

---

### 2.3 设备驱动适配层 — 完成度: 70%

| 驱动 | 参考项目 | Go 实现 | 状态 |
|------|---------|---------|------|
| SimulatedDevice | `SimulatedDevice.ts` | `simulated.go` | ✅ 完成 |
| DAQ-P-1604 | `DAQP1604Device.ts` | `daq_p1604.go` | ✅ 完成 |
| DAQ-T-1603 | `DAQT1603Device.ts` | `daq_t1603.go` | ✅ 完成 |
| DSA3217 | `DSA3217Device.ts` | 无 | ❌ 缺失 |
| DAQ-P-1064Pre | `DAQP1064PreDevice.ts` | 无 | ❌ 缺失 |
| WTN PXI | `WTNPXIDevice.ts` | 无 | ❌ 缺失 |
| B140 运动控制器 | `B140MotionController.ts` | 无 | ❌ 不迁移 (当前版本范围外) |
| WTN MC4A 运动控制器 | `WTNMC4AMotionController.ts` + FFI | 无 | ❌ 不迁移 (当前版本范围外) |
| SimulatedMotionController | `SimulatedMotionController.ts` | `simulated_motion.go` | ✅ 完成 |

**关键差异:**
- 参考项目支持 6 种硬件驱动，Go 版本仅实现了 3 种 (Simulated + DAQ-P-1604 + DAQ-T-1603)
- DSA3217、DAQ-P-1064Pre、WTN PXI 需要后续迁移

---

### 2.4 校准模块 (Calibration) — 完成度: 40%

| 功能 | 参考项目 | Go 实现 | 状态 |
|------|---------|---------|------|
| 校准任务控制 (start/pause/resume/stop) | `CalibrationService.ts` | `usecase/calibration.go` | ✅ 完成 |
| 手动采集点 | `acquireCurrentPoint()` / `reacquirePoint()` | CollectCurrentPoint | ⚠️ 部分 — 缺少 reacquirePoint |
| 五孔探针校准 | `FiveHoleCalibration.ts` | 无 | ❌ 缺失 |
| 三孔探针校准 | `ThreeHoleCalibration.ts` | 无 | ❌ 缺失 |
| 总压校准 | `TotalPressureCalibration.ts` | 无 | ❌ 缺失 |
| 总温校准 | `TotalTemperatureCalibration.ts` | 无 | ❌ 缺失 |
| 自动校准 | `AutomaticCalibration.ts` | 无 | ❌ 缺失 |
| 校准公式计算 | `formulas.ts` | 无 | ❌ 缺失 |
| 球罐闸门控制 | `sphereTankGate.ts` | 无 | ❌ 缺失 |
| 探针通道读取 | `readProbeChannels.ts` | 无 | ❌ 缺失 |
| 数据点防护 | `dataPointGuards.ts` | 无 | ❌ 缺失 |
| 校准采集协调器 | `CalibrationAcquisitionCoordinator.ts` | 无 | ❌ 缺失 |
| 导出载荷 | `getExportPayload()` | 无 | ❌ 缺失 |
| 总温状态 | `getTotalTemperatureState()` | 无 | ❌ 缺失 |

**这是差距最大的模块。** 参考项目有完整的校准类型体系（五孔、三孔、总压、总温），每种类型都有独立的算法实现和测试。Go 版本仅有通用的任务控制框架，缺少所有具体的校准算法。

---

### 2.5 遍历测试模块 (Traversal) — 完成度: 55%

| 功能 | 参考项目 | Go 实现 | 状态 |
|------|---------|---------|------|
| 遍历任务控制 | `traversalTestService.ts` | `usecase/traversal.go` | ✅ 完成 |
| 网格路径生成 | `TraversalInterpolationManager.ts` | `core/traversal/path.go` GenerateGridPath | ✅ 完成 |
| 线性插值路径 | `TraversalInterpolationManager.ts` | InterpolateLinearPath | ✅ 完成 |
| 运动等待与数据采集 | 手动 + 自动 | RunCurrentPoint (含运动等待) | ✅ 完成 |
| 实时更新节流 | `RealtimeUpdateThrottler.ts` | 无 | ❌ 缺失 |
| 优化实时插值器 | `OptimizedRealtimeInterpolator.ts` | 无 | ❌ 缺失 |
| 插值缓存 | `InterpolationCache.ts` | 无 | ❌ 缺失 |
| 扫描阀适配器 | `ScanValveAdapter.ts` | 无 | ❌ 缺失 |
| 遍历 CSV 写入 | `TraversalCsvWriter.ts` | 通用 CSV sink 代替 | ⚠️ 部分 |
| 遍历配置持久化 | `traversalStore.ts` | 无 | ❌ 缺失 |
| 遍历错误体系 | `TraversalErrors.ts` | 简单 error 字符串 | ⚠️ 简化 |

---

### 2.6 运动控制模块 (Motion) — 完成度: 60%

| 功能 | 参考项目 | Go 实现 | 状态 |
|------|---------|---------|------|
| 连接/断开 | `MotionControllerManager.ts` | `usecase/motion.go` | ✅ 完成 |
| MoveTo/MoveBy/Jog | 完整实现 | 完整实现 | ✅ 完成 |
| Home/Stop/EmergencyStop | 完整实现 | 完整实现 | ✅ 完成 |
| 多控制器管理 | `getProfiles()` + 多实例 | 单控制器 | ❌ 缺失 |
| DefinePosition | `definePosition()` | 无 | ❌ 缺失 |
| 运动任务执行器 | `MotionTaskExecutor.ts` | 无 | ❌ 缺失 |
| Profile 管理 | `motionStore.ts` | localStorage (前端) | ⚠️ 前端本地实现 |
| 运动转换 | `motionConversion.ts` | 无 | ❌ 缺失 |

**关键差异:**
- Go 版本仅支持**单个运动控制器**，参考项目支持多控制器实例管理
- 缺少 `definePosition` 功能（设置轴当前位置而不移动）
- 缺少运动任务队列执行器

---

### 2.7 数据存储模块 (Storage) — 完成度: 75%

| 功能 | 参考项目 | Go 实现 | 状态 |
|------|---------|---------|------|
| CSV 录制 | `DataStorageService.ts` + `DeviceFileWriter.ts` | `adapters/storage/csv_sink.go` | ✅ 完成 |
| 写入队列 | `WriteQueue.ts` | 无 (直接写入) | ⚠️ 简化 |
| 存储设置 | `storageStore.ts` + getSettings/updateSettings | 无 | ❌ 缺失 |
| 三孔 CSV 格式 | 专用格式 | 通用 CSV 格式 | ⚠️ 简化 |
| 设备文件写入器 | `DeviceFileWriter.ts` (按设备分文件) | 单文件 | ⚠️ 简化 |

---

### 2.8 报告生成模块 (Report) — 完成度: 25%

| 功能 | 参考项目 | Go 实现 | 状态 |
|------|---------|---------|------|
| PDF 报告生成 | `ReportGenerator.ts` (jsPDF) | 无 | ❌ 缺失 |
| 五孔校准报告 | `generateFiveHoleCalibrationReport()` | 无 | ❌ 缺失 |
| 三孔校准报告 | `generateThreeHoleCalibrationReport()` | 无 | ❌ 缺失 |
| 总压校准报告 | `generateTotalPressureCalibrationReport()` | 无 | ❌ 缺失 |
| 总温校准报告 | `generateTotalTempCalibrationReport()` | 无 | ❌ 缺失 |
| CSV 报告 | 无 | `csv_writer.go` | ✅ 仅 CSV |

**关键差异:**
- 参考项目使用 jsPDF 生成专业 PDF 报告（含表格、标题页、参数页），Go 版本仅有简单 CSV 导出
- 缺少所有校准类型的专用报告模板

---

### 2.9 插值算法模块 (Interpolation) — 完成度: 100%

| 功能 | 参考项目 | Go 实现 | 状态 |
|------|---------|---------|------|
| 插值器接口 | `IInterpolator.ts` | 无 | ❌ 缺失 |
| PRB 插值器 | `PrbInterpolator.ts` | 无 | ❌ 缺失 |
| 多 PRB 插值器 | `MultiPrbInterpolator.ts` | 无 | ❌ 缺失 |
| 五孔新插值器 | `FiveHoleNewInterpolator.ts` | 无 | ❌ 缺失 |

---

### 2.10 运行时基础设施 — 完成度: 15%

| 功能 | 参考项目 | Go 实现 | 状态 |
|------|---------|---------|------|
| 资源锁服务 | `ResourceLockService.ts` (TTL + 自动过期) | 无 | ❌ 缺失 |
| 系统事件总线 | `SystemEventBus.ts` | noopPublisher (空实现) | ❌ 缺失 |
| 健康监控 | `HealthMonitor.ts` | 无 | ❌ 缺失 |
| 应用运行时 | `AppRuntime.ts` | `bootstrap.go` | ⚠️ 简化 |

---

## 三、前端 API 集成差异

### 3.1 路径不匹配问题

| 前端调用 | Go 后端端点 | 问题 |
|---------|-----------|------|
| `POST /api/devices/:id/set-unit` | `PUT /api/device/:id/unit` | 路径和方法不匹配 |
| `POST /api/devices/:id/t1603-config` | `GET /api/device/:id/daqT1603Config` | 路径不匹配 |
| `POST /api/devices/scan` | `GET /api/device/scan` | 方法不匹配 |
| `POST /api/devices/:id/connect` | `POST /api/device/:id/connect` | 路径前缀不同 (devices vs device) |

### 3.2 前端空实现

| 功能 | 前端文件 | 状态 |
|------|---------|------|
| `definePosition` | `motionApi.ts` | 返回 `true` 空实现 |
| `pauseCalibration` | `calibrationApi.ts` | 返回 `{ success: true }` 空实现 |
| `resumeCalibration` | `calibrationApi.ts` | 返回 `{ success: true }` 空实现 |
| `stopCalibration` | `calibrationApi.ts` | 返回 `{ success: true }` 空实现 |
| `saveData` | `calibrationApi.ts` | 返回空实现 |
| `exportReport` | `calibrationApi.ts` | 返回空实现 |

---

## 四、代码质量评估

### 4.1 优点

1. **架构清晰** — 严格遵循六边形架构，core/ports/usecase/adapters 分层明确
2. **并发安全** — 所有共享状态使用 `sync.RWMutex` 保护
3. **错误处理** — 使用 `fmt.Errorf` 包装错误，提供上下文信息
4. **接口设计** — ports 层接口定义清晰，便于测试和替换
5. **类型安全** — 使用 Go 类型系统定义核心类型（Type, Connection, State 等）

### 4.2 不足

1. **缺少 Context 传递** — 所有方法未接受 `context.Context`，无法支持超时和取消
2. **日志不统一** — 部分使用 `slog`，部分无日志
3. **缺少结构化错误** — 使用字符串错误而非自定义错误类型
4. **API 路由硬编码** — 路由注册在单个函数中，缺少路由分组和中间件支持
5. **缺少 CORS 中间件** — 参考项目有 CORS 处理，Go 版本缺失

---

## 五、测试覆盖分析

Go 后端有 **21 个测试文件**，覆盖情况如下：

| 模块 | 测试文件 | 覆盖评估 |
|------|---------|---------|
| API Server | `server_test.go`, `apiserver_test.go` | ✅ 良好 |
| DeviceManager | `device_manager_test.go` | ✅ 良好 |
| AcquisitionHub | `acquisition_test.go` | ✅ 良好 |
| Calibration | `calibration_test.go`, `types_test.go` | ⚠️ 仅框架测试 |
| Traversal | `traversal_test.go`, `path_test.go` | ✅ 良好 |
| Motion | `motion_test.go`, `simulated_motion_test.go` | ✅ 良好 |
| Storage | `storage_test.go`, `csv_sink_test.go` | ✅ 良好 |
| Report | `report_test.go`, `csv_writer_test.go` | ✅ 良好 |
| Config | `file_profile_store_test.go` | ✅ 良好 |
| Scan | `network_scanner_test.go`, `simulated_scanner_test.go` | ✅ 良好 |
| Bootstrap | `bootstrap_test.go` | ✅ 良好 |

**缺失的测试:**
- DAQ-P-1604 和 DAQ-T-1603 硬件驱动无测试
- 校准算法（五孔/三孔/总压/总温）无测试
- 插值算法测试已迁移到 `shared/algorithms/go/fivehole/interpolation`
- SSE 流式推送集成测试
- 并发压力测试

---

## 六、实施计划

### 6.1 实施原则

1. **优先修复集成问题** — API 路径不匹配会导致运行时错误，必须首先修复
2. **先框架后算法** — 校准模块先建立类型体系和接口，再逐个实现算法
3. **保持架构一致性** — 所有新增代码遵循六边形架构
4. **测试驱动** — 关键算法实现前先编写测试用例
5. **渐进式推进** — 按优先级从高到低依次执行，每步验证构建和测试

### 6.2 P0 — 必须立即完成（核心功能缺失）

| 任务编号 | 任务名称 | 涉及文件 | 具体内容 | 完成标准 | 状态 |
|----------|----------|----------|----------|----------|------|
| B0-1 | 修复前端 API 路径不匹配 | `api/server.go`, `frontend/src/api/deviceApi.ts` | 统一 API 路径：`/api/device/` → `/api/devices/`，对齐前端调用 | 前端所有 API 调用与后端匹配 | ⬜ |
| B0-2 | 校准类型体系 | `core/calibration/types.go` | 新增 CalibrationType 枚举、各类型 Config/Result 结构体 | 类型定义覆盖五孔/三孔/总压/总温 | ⬜ |
| B0-3 | 校准算法接口 | `ports/calibration.go` | 定义 CalibrationAlgorithm 接口（Calculate/Validate/AcquireData） | 接口可被不同校准类型实现 | ⬜ |
| B0-4 | 五孔校准算法 | `core/calibration/five_hole.go` | 移植 `FiveHoleCalibration.ts` 的核心算法 | 五孔校准计算结果与参考项目一致 | ⬜ |
| B0-5 | 三孔校准算法 | `core/calibration/three_hole.go` | 移植 `ThreeHoleCalibration.ts` 的核心算法 | 三孔校准计算结果与参考项目一致 | ⬜ |
| B0-6 | 总压校准算法 | `core/calibration/total_pressure.go` | 移植 `TotalPressureCalibration.ts` 的核心算法 | 总压校准计算结果与参考项目一致 | ⬜ |
| B0-7 | 总温校准算法 | `core/calibration/total_temperature.go` | 移植 `TotalTemperatureCalibration.ts` 的核心算法 | 总温校准计算结果与参考项目一致 | ⬜ |
| B0-8 | 插值器接口 | `shared/algorithms/go/fivehole/interpolation/types.go` | 定义 Interpolator 接口 | 接口可被不同插值算法实现 | ✅ |
| B0-9 | PRB 插值器 | `shared/algorithms/go/fivehole/interpolation/prb_interpolator.go` | 移植 `PrbInterpolator.ts` | 插值结果与参考项目一致 | ✅ |
| B0-10 | 多 PRB 插值器 | `shared/algorithms/go/fivehole/interpolation/multi_prb_interpolator.go` | 移植 `MultiPrbInterpolator.ts` | 插值结果与参考项目一致 | ✅ |
| B0-11 | 五孔插值器 | `shared/algorithms/go/fivehole/interpolation/five_hole_new_interpolator.go` | 移植 `FiveHoleNewInterpolator.ts` | 插值结果与参考项目一致 | ✅ |

### 6.3 P1 — 高优先级（功能完整性）

| 任务编号 | 任务名称 | 涉及文件 | 具体内容 | 完成标准 | 状态 |
|----------|----------|----------|----------|----------|------|
| B1-1 | 熔断器机制 | `core/device/circuit_breaker.go` | 实现 CircuitBreaker（失败计数、冷却期、半开状态） | 连接失败后自动冷却，恢复后允许重试 | ⬜ |
| B1-2 | 多运动控制器支持 | `usecase/motion.go`, `ports/motion.go` | MotionManager 管理多控制器实例，支持按 ID 操作 | 可同时管理多个运动控制器 | ⬜ |
| B1-3 | DefinePosition 功能 | `ports/motion.go`, `simulated_motion.go` | 添加 DefinePosition 接口方法和实现 | 可设置轴当前位置而不移动 | ⬜ |
| B1-4 | Context 传递 | 全部 usecase 和 adapter 文件 | 所有方法添加 context.Context 参数 | 支持请求超时和优雅关闭 | ⬜ |
| B1-5 | 资源锁服务 | `core/runtime/lock.go` | 移植 ResourceLockService（TTL + 自动过期） | 防止并发操作冲突 | ⬜ |
| B1-6 | 系统事件总线 | `core/runtime/eventbus.go` | 实现 EventBus（发布/订阅/广播） | 组件间解耦通信 | ⬜ |
| B1-7 | 校准 reacquirePoint | `usecase/calibration.go` | 添加 ReacquirePoint 方法 | 可重新采集指定校准点 | ⬜ |
| B1-8 | 校准导出功能 | `usecase/calibration.go` | 添加 GetExportPayload 方法 | 可导出校准数据 | ⬜ |

### 6.4 P2 — 中优先级（健壮性）

| 任务编号 | 任务名称 | 涉及文件 | 具体内容 | 完成标准 | 状态 |
|----------|----------|----------|----------|----------|------|
| B2-1 | 批量设备状态查询 | `usecase/device_manager.go`, `api/server.go` | 添加 GetAllStatuses 方法 | 一次查询所有设备状态 | ⬜ |
| B2-2 | CORS 中间件 | `api/server.go` | 添加 CORS 处理中间件 | 支持跨域请求 | ⬜ |
| B2-3 | 结构化错误类型 | `core/errors/` | 定义 DomainError/DeviceError/CalibrationError 等 | 错误可按类型匹配和处理 | ⬜ |
| B2-4 | 校准公式库 | `core/calibration/formulas.go` | 移植 `formulas.ts` 的数学公式 | 公式计算结果与参考项目一致 | ⬜ |
| B2-5 | 球罐闸门控制 | `core/calibration/sphere_tank_gate.go` | 移植 `sphereTankGate.ts` | 球罐闸门状态管理 | ⬜ |
| B2-6 | 数据点防护 | `core/calibration/data_point_guards.go` | 移植 `dataPointGuards.ts` | 校准数据有效性检查 | ⬜ |
| B2-7 | 遍历配置持久化 | `adapters/config/traversal_store.go` | 实现遍历配置文件存储 | 遍历配置可保存和加载 | ⬜ |
| B2-8 | 健康监控 | `core/runtime/health.go` | 移植 `HealthMonitor.ts` | 设备和系统状态监控 | ⬜ |

### 6.5 P3 — 低优先级（增强）

| 任务编号 | 任务名称 | 涉及文件 | 具体内容 | 完成标准 | 状态 |
|----------|----------|----------|----------|----------|------|
| B3-1 | DSA3217 驱动 | `adapters/hardware/dsa3217.go` | 移植 `DSA3217Device.ts` | DSA3217 设备可连接和采集 | ⬜ |
| B3-2 | DAQ-P-1064Pre 驱动 | `adapters/hardware/daq_p1064pre.go` | 移植 `DAQP1064PreDevice.ts` | DAQ-P-1064Pre 设备可连接和采集 | ⬜ |
| B3-3 | WTN PXI 驱动 | `adapters/hardware/wtn_pxi.go` | 移植 `WTNPXIDevice.ts` | WTN PXI 设备可连接和采集 | ⬜ |
| B3-4 | 存储设置管理 | `core/storage/settings.go` | 实现存储配置持久化 | 存储设置可保存和加载 | ⬜ |
| B3-5 | 遍历实时更新节流 | `core/traversal/throttler.go` | 移植 `RealtimeUpdateThrottler.ts` | 高频数据场景优化 | ⬜ |
| B3-6 | PDF 报告生成 | `adapters/report/pdf_writer.go` | 使用 Go PDF 库生成专业报告 | 校准报告可导出为 PDF | ⬜ |

---

## 七、进度跟踪

> 以下进度随实施实时更新

### P0 进度

| 任务 | 状态 | 完成时间 | 备注 |
|------|------|---------|------|
| B0-1: 修复前端 API 路径不匹配 | ⬜ | - | - |
| B0-2: 校准类型体系 | ⬜ | - | - |
| B0-3: 校准算法接口 | ⬜ | - | - |
| B0-4: 五孔校准算法 | ⬜ | - | - |
| B0-5: 三孔校准算法 | ⬜ | - | - |
| B0-6: 总压校准算法 | ⬜ | - | - |
| B0-7: 总温校准算法 | ⬜ | - | - |
| B0-8: 插值器接口 | ⬜ | - | - |
| B0-9: PRB 插值器 | ⬜ | - | - |
| B0-10: 多 PRB 插值器 | ⬜ | - | - |
| B0-11: 五孔插值器 | ✅ | - | 已迁移到 `shared/algorithms/go/fivehole` |

### P1 进度

| 任务 | 状态 | 完成时间 | 备注 |
|------|------|---------|------|
| B1-1: 熔断器机制 | ⬜ | - | - |
| B1-2: 多运动控制器支持 | ⬜ | - | - |
| B1-3: DefinePosition 功能 | ⬜ | - | - |
| B1-4: Context 传递 | ⬜ | - | - |
| B1-5: 资源锁服务 | ⬜ | - | - |
| B1-6: 系统事件总线 | ⬜ | - | - |
| B1-7: 校准 reacquirePoint | ⬜ | - | - |
| B1-8: 校准导出功能 | ⬜ | - | - |

### P2 进度

| 任务 | 状态 | 完成时间 | 备注 |
|------|------|---------|------|
| B2-1 ~ B2-8 | ⬜ | - | - |

### P3 进度

| 任务 | 状态 | 完成时间 | 备注 |
|------|------|---------|------|
| B3-1 ~ B3-6 | ⬜ | - | - |

---

## 八、验证清单

每个任务完成后需验证：

- [ ] `go build ./...` 编译通过
- [ ] `go test ./...` 测试通过
- [ ] 新增代码有对应测试
- [ ] API 端点与前端调用匹配
- [ ] 六边形架构约束未被违反（core 无外部依赖，ports 无实现）
- [ ] 并发安全（共享状态有 mutex 保护）
