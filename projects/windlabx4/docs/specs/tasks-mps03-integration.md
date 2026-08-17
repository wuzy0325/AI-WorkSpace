# Tasks: MPS-03 集成到 WindLabX4

> **配套 spec**：[spec-mps03-integration.md](./spec-mps03-integration.md)
> **配套 plan**：[plan-mps03-integration.md](./plan-mps03-integration.md)
> **用途**：执行期 checklist，逐任务打勾。详细 AC/Verify/Files 见 plan。

---

## 执行规则

1. **严格按 Phase 顺序执行**，同 Phase 内可按"依赖"列并行
2. **每完成一个任务**：勾选 `[x]` + 运行该任务的 Verify 命令
3. **每完成一个 Checkpoint**：运行 Checkpoint 全部验证命令 + grep 检查 + 人工 review
4. **失败处理**：Verify 失败立即修复，不跳过下一个任务
5. **命名红线**：MPS-03 代码中禁止出现 `DAQP1603`/`P1603` 字样
6. **依赖方向**：`shared/device-sdk/go/*` 禁止 `import "windlabx4/services/api-go/internal/*"`

---

## Phase 1: shared SDK 协议层

- [ ] **T1** MPS03CommandClient 命令收发客户端
  - 依赖：无
  - 文件：`shared/device-sdk/go/protocol/mps03_frame.go`（新增）、`mps03_frame_test.go`（新增）
  - Verify：`cd shared/device-sdk/go && go test ./protocol/... -run MPS03 -v`
  - 关键点：7 条响应识别规则 + 半包/粘包 + 超时 + ioMu 串行化

- [ ] **T2** ParseCSVLine + TMODE/TCHO 转换（与 T1 并行）
  - 依赖：无
  - 文件：同 T1（追加函数和用例）
  - Verify：`cd shared/device-sdk/go && go test ./protocol/... -run "ParseCSVLine|Tmode|Tcho" -v`
  - 关键点：HEAD=0/1 解析 + 错误帧返回 error（禁止填 0）+ 十六进制/字母双向转换

### Checkpoint C1: Phase 1 完成
- [ ] `cd shared/device-sdk/go && go build ./... && go test ./protocol/... && go vet ./...`
- [ ] `grep -r "WindLabX4" shared/device-sdk/go/protocol/` 无命中
- [ ] 人工 review

---

## Phase 2: shared SDK 驱动层

- [ ] **T3** shared SDK core/ports 扩展
  - 依赖：无（建议先于 T4 完成）
  - 文件：`shared/device-sdk/go/daq/core/types.go`（改）、`daq/ports/device.go`（改）
  - Verify：`cd shared/device-sdk/go && go build ./... && go vet ./...`
  - 关键点：`DeviceMPS03 Type = "MPS03"` + `MPS03HardwareConfig`（9 字段）+ `Profile.MPS03Config` + `MPS03Configurable` 接口

- [ ] **T4** MPS03Device 驱动主体
  - 依赖：T1, T2, T3
  - 文件：`shared/device-sdk/go/daq/hardware/mps03.go`（新增）、`mps03_test.go`（新增）
  - Verify：`cd shared/device-sdk/go && go test ./daq/hardware/... -run MPS03 -v`
  - 关键点：Connect（9 个 #GET）+ StartAcquisition（#SET BIN 0 + #START）+ readLoop + 错误帧丢弃计数 + 采集期间禁止配置 + CHA 契约（仅启用通道）

- [ ] **T5** 重连状态机
  - 依赖：T4
  - 文件：同 T4（追加状态机代码和用例）
  - Verify：`cd shared/device-sdk/go && go test ./daq/hardware/... -run MPS03Reconnect -v`
  - 关键点：指数退避 1/2/4/8/16s × 5 次 + 恢复 9 个 #GET + 条件 #START + manualDisconnect 禁止重连 + 旧连接清理 1s 超时

- [ ] **T6** MPS-03 模拟器（与 T4/T5 并行）
  - 依赖：T1, T2
  - 文件：`shared/device-sdk/go/testing/sim/mps03_sim.go`（新增）、`mps03_sim_test.go`（新增）
  - Verify：`cd shared/device-sdk/go && go test ./testing/sim/... -run MPS03 -v`
  - 关键点：9 个 #GET 响应 + #START 按 DELAY 推送 + #STOP 停止 + CHA 掩码生效

### Checkpoint C2: Phase 2 完成
- [ ] `cd shared/device-sdk/go && go build ./... && go test ./... && go vet ./...`
- [ ] `grep -r "WindLabX4" shared/device-sdk/go/` 无命中
- [ ] `grep -r "DAQP1603" shared/device-sdk/go/daq/hardware/mps03*.go shared/device-sdk/go/protocol/mps03*.go` 无命中
- [ ] 人工 review

---

## Phase 3: WindLabX4 项目层

- [ ] **T7** WindLabX4 core/ports 镜像
  - 依赖：T3
  - 文件：`projects/windlabx4/services/api-go/internal/core/device/types.go`（改）、`internal/ports/device.go`（改）
  - Verify：`cd projects/windlabx4/services/api-go && go build ./... && go vet ./...`
  - 关键点：WindLabX4 自有 `DeviceMPS03` + `MPS03HardwareConfig` + `Profile.MPS03Config` + `ports.MPS03Configurable`（用 WindLabX4 类型）

- [ ] **T8** MPS03Adapter thin wrapper
  - 依赖：T4, T7
  - 文件：`projects/windlabx4/services/api-go/internal/adapters/hardware/mps03_adapter.go`（新增）、`mps03_adapter_test.go`（新增可选）
  - Verify：`cd projects/windlabx4/services/api-go && go test ./internal/adapters/hardware/... -run MPS03 -v`
  - 关键点：编译期断言 + Profile/Status/DataPayload 双向翻译 + sink 包装 + onError 委托

- [ ] **T9** default_profiles + bootstrap 注册
  - 依赖：T7, T8
  - 文件：`projects/windlabx4/services/api-go/internal/adapters/config/default_profiles.go`（改）、`internal/bootstrap/bootstrap.go`（改）
  - Verify：`cd projects/windlabx4/services/api-go && go build ./... && go vet ./...`
  - 关键点：默认 IP 192.168.1.9/端口 9000/16 通道 + 默认 MPS03HardwareConfig（AVG=8/DELAY=1000/CHA="FFFF"/TMODE=17/TTYPE=2/TCHO="E"）+ NormalizeProfile + deviceFactory.Create 新增 case

- [ ] **T10** HTTP API 端点
  - 依赖：T8, T9
  - 文件：`projects/windlabx4/services/api-go/api/server.go`（改）
  - Verify：`cd projects/windlabx4/services/api-go && go test ./api/... -run MPS03 -v`
  - 关键点：GET/PUT `/api/device/{id}/mps03-config` + 409/400/502/503/504 状态码 + `/api/storage/start` 收集 MPS03 channels

- [ ] **T11** CSV sink 17 列表头分支（与 T10 并行）
  - 依赖：T7
  - 文件：`projects/windlabx4/services/api-go/internal/adapters/storage/csv_sink.go`（改）
  - Verify：`cd projects/windlabx4/services/api-go && go test ./internal/adapters/storage/... -run MPS03 -v`
  - 关键点：`case device.DeviceMPS03:` + 17 列固定表头（Timestamp + 16 ASCII 列名）+ 禁用通道填空字符串 + UTF-8 BOM + `\r\n`

- [ ] **T12** API 集成测试
  - 依赖：T10, T11
  - 文件：`projects/windlabx4/services/api-go/api/server_test.go`（改）
  - Verify：`cd projects/windlabx4/services/api-go && go test ./api/... -run MPS03 -v`
  - 关键点：GET/PUT 全流程 + 409/400 状态码 + CSV 17 列 + CHA != FFFF 时 DataPayload 仅含启用通道

### Checkpoint C3: Phase 3 完成
- [ ] `cd projects/windlabx4/services/api-go && go build ./... && go test ./... && go vet ./...`
- [ ] `grep -r "DAQP1603" projects/windlabx4/services/api-go/internal/adapters/hardware/mps03*.go` 无命中
- [ ] 人工 review

---

## Phase 4: 前端

- [ ] **T13** types.ts + deviceApi.ts
  - 依赖：T10
  - 文件：`projects/windlabx4/apps/desktop-wails/frontend/src/api/types.ts`（改）、`src/api/deviceApi.ts`（改）
  - Verify：`cd projects/windlabx4/apps/desktop-wails/frontend && npm run typecheck && npm run build`
  - 关键点：`DeviceType` 联合新增 `'MPS03'` + `MPS03HardwareConfig` 接口（9 字段）+ `getMps03Config`/`applyMps03Config` + 错误状态码处理

- [ ] **T14** Mps03ConfigPanel.vue
  - 依赖：T13
  - 文件：`projects/windlabx4/apps/desktop-wails/frontend/src/components/device/Mps03ConfigPanel.vue`（新增）
  - Verify：`npm run typecheck && npm run build`
  - 关键点：9 项配置 UI + 采集时 disabled + DELAY 实时算 Hz 只读 + AVG/TTYPE 下拉 + TCHO 单选 + CHA 十六进制 + 设计 token

- [ ] **T15** DeviceManagementDrawer.vue MPS03 分支
  - 依赖：T13, T14
  - 文件：`projects/windlabx4/apps/desktop-wails/frontend/src/components/device/DeviceManagementDrawer.vue`（改）
  - Verify：`npm run typecheck && npm run build`
  - 关键点：类型下拉含 MPS-03 + 默认值 + 嵌入 Mps03ConfigPanel + 通用 samplingRate 隐藏 + 设备卡片显示 Hz 只读 + 校零排除 T_ext/T_int

- [ ] **T16** deviceStore.ts MPS03 操作（与 T14/T15 并行）
  - 依赖：T13
  - 文件：`projects/windlabx4/apps/desktop-wails/frontend/src/stores/deviceStore.ts`（改）
  - Verify：`npm run typecheck && npm run build`
  - 关键点：`loadMps03Config`/`saveMps03Config` action + 保存后更新本地 profile + 校零通道过滤（排除 14/15）

### Checkpoint C4: Phase 4 完成
- [ ] `cd projects/windlabx4/apps/desktop-wails/frontend && npm run typecheck && npm run build`
- [ ] 模拟器手动 UI 验证：添加 → 连接 → 配置 → 采集 → 录制全流程
- [ ] 人工 review

---

## Phase 5: Wails binding + 验证

- [ ] **T17** Wails binding 新增 + 同步
  - 依赖：T10
  - 文件：`projects/windlabx4/apps/desktop-wails/backend/app.go`（改）+ 自动生成 TS binding
  - Verify：`cd projects/windlabx4/apps/desktop-wails && wails3 generate bindings -silent` + `go build ./... && go vet ./...` + `npm run typecheck && npm run build`
  - 关键点：`DeviceGetMps03Config`/`DeviceApplyMps03Config` + `MPS03ConfigResult` 结构体 + 委托 DeviceManager

- [ ] **T18** 端到端验证
  - 依赖：T1-T17 全部完成
  - 文件：无（验证任务）
  - Verify：见下方完整验证清单
  - 关键点：模拟器全流程 + 真实硬件 CHA 契约验证 + 双模块测试全绿 + 结构验证全绿

### Checkpoint C5: Phase 5 完成（项目完成）
- [ ] `cd shared/device-sdk/go && go test ./... && go vet ./...`
- [ ] `cd projects/windlabx4/services/api-go && go test ./... && go vet ./...`
- [ ] `cd projects/windlabx4/apps/desktop-wails/frontend && npm run typecheck && npm run build`
- [ ] `.\validate-structure.ps1`
- [ ] `.\validate-frontend-structure.ps1 -CheckFileSize`
- [ ] spec 16 项成功标准全部满足
- [ ] 真实硬件验证完成（CHA 契约确认，更新 TODO-HW-VERIFY）
- [ ] Ready for release

---

## T18 端到端验证清单（展开）

### 模拟器验证
- [ ] 添加 MPS-03 设备 → 默认 IP/端口正确
- [ ] 连接 → 9 个 #GET 完成 → UI 只读显示配置
- [ ] 修改 DELAY → #SAVE → 重连后配置持久化
- [ ] 启动采集 → 波形图显示启用通道 → 采样率符合 DELAY
- [ ] 停止采集 → 数据流立即停止
- [ ] 录制 CSV → 17 列表头正确 → 禁用通道列空字符串
- [ ] CHA != FFFF → DataPayload 仅含启用通道 → CSV 禁用列空字符串
- [ ] 采集期间调用 ApplyMps03Config → 返回 409
- [ ] 拔网线 → 重连状态机启动 → 5 次内重连成功 → 恢复采集
- [ ] 重连 5 次失败 → Error 态 → 不再自动重连
- [ ] 注入错误帧 → 不进 sink → 连续 50 帧触发 Error

### 真实硬件验证（TODO-HW-VERIFY）
- [ ] CHA != FFFF 时 CSV 字段顺序确认
- [ ] HEAD=1 + CHA != FFFF 时序号字段是否仍存在
- [ ] TMODE 十六进制 SET / 十进制 GET 转换正确
- [ ] TCHO 字母 GET / 数字 SET 转换正确
- [ ] 错误帧阈值 50 是否合理
- [ ] 重连退避 5 次是否足够

### 回归验证
- [ ] DAQ-P-1603 设备功能不受影响
- [ ] DAQ-P-1604 设备功能不受影响
- [ ] 其他设备 CSV 录制表头不变
- [ ] 现有测试全部通过

---

## 任务统计

| Phase | 任务数 | 可并行组 | 关键路径 |
|---|---|---|---|
| Phase 1 | 2 | T1 ∥ T2 | T1 或 T2 |
| Phase 2 | 4 | T6 ∥ (T4→T5) | T3 → T4 → T5 |
| Phase 3 | 6 | T11 ∥ T10 | T7 → T8 → T9 → T10 → T12 |
| Phase 4 | 4 | T14 ∥ T15 ∥ T16 | T13 → T14/T15/T16 |
| Phase 5 | 2 | T17 ∥ T18 准备 | T17 → T18 |
| **合计** | **18** | — | T1→T3→T4→T5→T7→T8→T9→T10→T12→T13→T14→T15→T17→T18 |

---

## 风险追踪（来自 plan）

| 风险 ID | 任务 | 状态 | 触发时处理 |
|---|---|---|---|
| R1 CHA 字段顺序 | T18 | 待验证 | 真实硬件不符则更新 spec + 模拟器 |
| R2 响应识别漏判 | T1 | 待测试 | mock TCP 注入异常响应补充用例 |
| R3 重连泄漏 | T5 | 待测试 | goroutine leak 检测工具辅助 |
| R4 采集期误配置 | T4/T10 | 待测试 | 双重拦截验证 |
| R5 依赖方向 | C1/C2 | 待验证 | grep + validate-structure 兜底 |
| R6 命名混淆 | C2/C3 | 待验证 | grep 验证 |
| R7 binding 同步 | T17 | 待执行 | typecheck 兜底 |
| R8 CSV 冲突 | T11 | 待测试 | 独立分支不影响其他设备 |
| R9 模拟器偏差 | T18 | 待验证 | 真实硬件校准 |
| R10 Hz 抖动 | T14 | 已接受 | round(1000/delay) |
| R11 阈值 50 | T18 | 待实测 | 常量可调 |
| R12 弱网超时 | T14 | 待实测 | 10s 超时 + UI 提示 |
