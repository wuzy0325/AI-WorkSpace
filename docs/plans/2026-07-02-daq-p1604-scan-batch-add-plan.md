# Implementation Plan: daq-p1604 扫描批量添加设备重设计

**关联 Spec**：[2026-07-02-daq-p1604-scan-batch-add.md](./2026-07-02-daq-p1604-scan-batch-add.md)
**创建时间**：2026-07-02
**范围**：前端 3 文件，后端零改动

---

## Overview

改造 daq-p1604 扫描弹窗，从"逐条点 + 跳到手动添加窗手打名字"改为"多选 + 内联改名 + 批量入库"；已添加设备置灰不可重加；每台自动生成互不相同的默认名；顶部一个批量默认自动连接开关。**范围严格限于前端 3 文件**：`stores/deviceStore.ts`、`components/device/ScanResultList.vue`、`components/layout/AppShell.vue`。

## Architecture Decisions

1. **去重键 = `host:<address>:<port>`**
   `core.PressureProfile` 未存 MAC，无法从已有 profile 反推 MAC 参与查重。真实场景中同一台设备通常复用固定 IP，`host:addr:port` 已够用；MAC 仅用于**生成默认名与 ID**。若后续需要 MAC 级查重需改 core（当前 out of scope）。

2. **Profile ID 从 `p1604_<timestamp>` 改为可复现规则**
   - 有 MAC：`p1604-<mac 小写去冒号>`
   - 无 MAC：`p1604-<addr 去点>-<port>`
   同一台真机重扫命中同一 ID，`json_config.go` 现有"按 ID 命中则替换"逻辑天然幂等。

3. **保留 `addProfile(name, addr, port)` 不动**
   `openAddDevice`（手动添加）仍在使用。新增 `addScannedProfiles(inputs)` 为批量扫描专用路径，两条路径并存。

4. **删除 `addFromScanResult` 跳转手动添加窗的桥接**
   扫描添加不再打开"手动添加"对话框，直接由批量 API 落库。

5. **critical：不跑 `wails3 generate bindings`**
   本次无 Go 方法签名变化，且 project_memory §1 记录该命令在 daq-p1604 上会误删 TS binding。

## Task List

### Phase 1: Foundation（Store 层，无 UI 依赖）

- [x] **Task 1**: 见下 §Task 1 —— deviceStore 加批量 API、默认名/ID 工具、existingDeviceKeys getter

### Checkpoint: Foundation
- [ ] `npm run typecheck` 全绿
- [ ] `existingDeviceKeys` 在 devtools 中可见且随 profiles 变化而更新
- [ ] `addScannedProfiles` 单函数纯 TS，未破坏原 `addProfile` 签名

### Phase 2: Interaction Layer（ScanResultList 组件契约）

- [x] **Task 2**: 见下 §Task 2 —— ScanResultList 改为多选 + 内联改名 + 已添加置灰

### Checkpoint: Interaction Layer
- [ ] `npm run build` 全绿
- [ ] 组件 emit/props 契约与 Task 3 消费方一致
- [ ] 组件在无选中状态下依然可渲染（selection=[] 场景）

### Phase 3: Composition Layer（AppShell 装配）

- [x] **Task 3**: 见下 §Task 3 —— AppShell 弹窗装配 + 批量提交 + 顶部自动连接开关

### Checkpoint: Complete
- [x] **Task 4**: 见下 §Task 4 —— 全项目冒烟
- [ ] 所有 Spec Success Criteria S1–S11 可手工验收（S11 由命令验证）
- [ ] 交给用户桌面手工验收 S1–S10

---

## Task 1: deviceStore.ts — 批量 API + 命名/ID 工具

**Description**: 在 `deviceStore.ts` 加入批量扫描添加所需的全部纯 TS 逻辑。不动 UI，先把底层 API 与去重/命名规则打好。

**Acceptance criteria**:
- [ ] 新增导出接口 `AddScannedInput`（含 address/port/macAddress?/serialNumber?/name?/defaultAutoConnect）
- [ ] 新增导出接口 `AddScannedResult`（含 `added: PressureProfile[]`、`skipped: SkippedItem[]`）
- [ ] 新增 store method `addScannedProfiles(inputs, opts)`：返回 `AddScannedResult`
- [ ] 新增内部工具 `makeProfileId / makeDefaultName / dedupeName / hostKey`
- [ ] 新增 store getter `existingDeviceKeys: ComputedRef<Set<string>>`，格式 `host:<addr>:<port>`（小写、trim）
- [ ] 原 `addProfile(name, address, port)` 签名与行为不变

**Verification**:
- [ ] `cd projects/daq-p1604/apps/desktop-wails/frontend && npm run typecheck` 全绿
- [ ] 手工代码走查：`makeProfileId` 对 MAC `00:1A:2B:3A:5F:1B` → `p1604-001a2b3a5f1b`；对无 MAC + IP `192.168.3.101` port `9000` → `p1604-192-168-3-101-9000`
- [ ] `makeDefaultName` 对同上 MAC → `DAQ-P-1604-3A5F1B`；无 MAC → `DAQ-P-1604-101-9000`
- [ ] `dedupeName('DAQ-P-1604-3A5F1B', new Set(['DAQ-P-1604-3A5F1B']))` → `DAQ-P-1604-3A5F1B (2)`

**Dependencies**: None

**Files likely touched**:
- `projects/daq-p1604/apps/desktop-wails/frontend/src/stores/deviceStore.ts`

**Estimated scope**: S（1 文件）

---

## Task 2: ScanResultList.vue — 多选 + 内联改名 + 已添加置灰

**Description**: 把 `ScanResultList` 从"逐条 + 按钮"改为"checkbox 多选 + 内联命名 + 已添加置灰"。组件通过 `v-model` 向父组件暴露当前选中项与内联名字。

**Acceptance criteria**:
- [ ] Props 新增：`existingKeys: Set<string>`（父组件传入）、`modelValue: ScanSelectionItem[]`
- [ ] Emits：`update:modelValue`（不再 emit `add`）
- [ ] 每行结构：`<input type="checkbox">` + 原信息 + `<input type="text">`（内联命名，占位符为默认名）
- [ ] 若行的 `host:<addr>:<port>` 在 `existingKeys` 中 → 行加 `.scan__item--disabled`、checkbox `disabled`、行末显示"已添加"标签、命名 input `disabled`
- [ ] 命名 input 的初始值为默认名（通过 `makeDefaultName` 计算或从 props 传入），用户改动后触发 `update:modelValue`
- [ ] 移除 `+` 按钮及 `emit('add')`
- [ ] 保留 loading / empty 两个已有状态，样式沿用现有 CSS 变量

**Verification**:
- [ ] `cd projects/daq-p1604/apps/desktop-wails/frontend && npm run build` 全绿
- [ ] 手工代码走查：`selection` 结构 `{ id: string; name: string; address: string; port: number; macAddress: string; serialNumber: string }`
- [ ] Vue devtools 中勾/取消勾会立即修改父组件绑定的数组

**Dependencies**: Task 1（复用 `makeDefaultName` 与 `hostKey` 逻辑；不硬依赖但共享概念）

**Files likely touched**:
- `projects/daq-p1604/apps/desktop-wails/frontend/src/components/device/ScanResultList.vue`

**Estimated scope**: S（1 文件）

---

## Task 3: AppShell.vue — 弹窗装配 + 批量提交 + 顶部开关

**Description**: 装配扫描弹窗。顶部一个批量默认自动连接开关（默认开启）；中部承载 ScanResultList 并绑定 v-model；底部三按钮 `取消 / 重新扫描 / 添加所选 (N)`；点添加所选调用 `deviceStore.addScannedProfiles`，成功后关闭弹窗（Q1=B）；同时删除 `addFromScanResult` 跳转手动添加窗的逻辑。

**Acceptance criteria**:
- [ ] 引入 `deviceStore.existingDeviceKeys` 传给 ScanResultList
- [ ] 新增 `scanSelection = ref<ScanSelectionItem[]>([])`，v-model 到 ScanResultList
- [ ] 新增 `defaultAutoConnectOnAdd = ref(true)`（Q2=A：默认开启）
- [ ] 扫描弹窗顶部渲染一个 `<label>` + 复选框，标题"添加的设备默认启用自动连接"
- [ ] 底部按钮组：`取消` `重新扫描`（原有）+ 新增 `添加所选 (N)`；N = 选中数
- [ ] `添加所选` 逻辑：
  - 若 N=0 禁用按钮
  - 若 N>0 → `await deviceStore.addScannedProfiles(selection, { defaultAutoConnect })`
  - 成功后 `showScanDialog = false`；`scanSelection = []`；若 `skipped.length > 0` → `logStore.warn(...)`
  - 失败 → `logStore.error(...)`，弹窗保持打开
- [ ] 删除 `addFromScanResult` 函数（扫描不再触发 `openAddDevice`）
- [ ] 保留 `openAddDevice` / `confirmAddDevice`（顶栏"+"手动添加仍在用）

**Verification**:
- [ ] `cd projects/daq-p1604/apps/desktop-wails/frontend && npm run typecheck && npm run build` 全绿
- [ ] 手工验收剧本 1（多选批量添加）通过
- [ ] 手工验收剧本 2（重复扫描置灰）通过
- [ ] 手工验收剧本 3（内联改名生效）通过
- [ ] 手工验收剧本 4（关自动连接开关后新增条目 `autoConnect=false`）通过
- [ ] Q1=B 验证：`添加所选` 成功后弹窗**关闭**

**Dependencies**: Task 1、Task 2

**Files likely touched**:
- `projects/daq-p1604/apps/desktop-wails/frontend/src/components/layout/AppShell.vue`

**Estimated scope**: S（1 文件）

---

## Task 4: 全项目冒烟 + 结构校验

**Description**: 所有代码改动完成后跑齐编译期检查，确保没有引入类型错误、构建错误或结构违规。

**Acceptance criteria**:
- [ ] `cd projects/daq-p1604/apps/desktop-wails/frontend && npm run typecheck` 通过
- [ ] `cd projects/daq-p1604/apps/desktop-wails/frontend && npm run build` 通过
- [ ] `cd projects/daq-p1604/apps/desktop-wails && go build ./...` 通过（sanity check，实际未改后端）
- [ ] `cd projects/daq-p1604/apps/desktop-wails && go vet ./...` 通过
- [ ] `powershell -File scripts/validate-frontend-structure.ps1 -ProjectDir "projects/daq-p1604/apps/desktop-wails/frontend/src"` 通过

**Verification**: 命令输出

**Dependencies**: Task 1、Task 2、Task 3

**Files likely touched**: 无（仅命令）

**Estimated scope**: XS

---

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| checkbox 与 profile 变化脱同步 | Med | 用 `computed(existingDeviceKeys)` 让其响应式跟随 `profiles.value` |
| 内联改名冲突（两台默认名相同） | Med | 落库前对整批 + 现有 profile 名字集合走 `dedupeName` |
| 批量添加中途单条 IPC 失败 | Med | `Promise.allSettled` 逐条统计；已成功继续保留；失败写 logStore.error |
| ID 规则从 timestamp 改为 mac/host → 已有 profile 迁移问题 | Low | 只影响新添加；旧 profile 按原 ID 继续存在；查重按 host 命中，仍能挡住重复 |
| 用户在扫描进行中关闭弹窗 | Low | Cancel 只关弹窗；扫描 IPC 完成后仅写 scanResults，无副作用 |
| logStore 引用可能未注入 | Low | store 内直接 `useLogStore()` 已在文件其他位置存在，可复用 |
| Wails binding 误删 | High | 本次严禁跑 `wails3 generate bindings`；无 Go 签名变化，binding 无需重生成 |

## Open Questions

无。Spec 阶段已拍板 Q1=B / Q2=A / Q3=A / Q4=A。

## Parallelization

- **不可并行**：Task 1 → Task 2 → Task 3 严格顺序（Task 3 依赖 Task 1 的 API 和 Task 2 的组件契约）
- **可并行**：无（3 个文件都在依赖链上，且总量小，串行即可）
