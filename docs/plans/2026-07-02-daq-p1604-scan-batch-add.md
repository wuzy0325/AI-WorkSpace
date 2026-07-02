# Spec: daq-p1604 扫描批量添加设备重设计

**状态**：待评审 → 待实现
**创建时间**：2026-07-02
**范围**：`projects/daq-p1604/apps/desktop-wails/frontend/*`（前端），后端零改动

---

## Objective

daq-p1604 当前"扫描 → 逐条 + → 跳到手动添加窗手打名字"的流程存在两个明确痛点：

1. 扫到多台设备时一次只能添加一台，弹窗关闭，需重复扫描 N 次
2. 已添加过的设备仍会出现在扫描结果中且可再次添加，产生重复 profile（当前 `id = p1604_<timestamp>` 无查重）

重设计后：扫描弹窗支持"勾多台 + 一键添加"，已添加设备自动置灰不可重加；每台生成不同的默认设备名，新增设备默认启用自动连接。降低多设备装机场景的操作成本。

## User

daq-p1604 操作员，典型场景：首次装机 / 换设备，一次扫到 2–8 台需批量入库。

## Tech Stack

- 前端：Vue 3.5 + TypeScript + Vite + Pinia（不引入 Naive UI 依赖，遵循项目现有零 UI 库风格）
- 桌面壳：Wails v3 alpha.95
- 后端：Go 1.25（**本 spec 后端零改动**）

## Commands

```powershell
# 目录：projects/daq-p1604/apps/desktop-wails
# 前端类型检查 + 构建
cd frontend
npm run typecheck
npm run build

# 后端构建 + 测试（sanity check，本 spec 不改后端）
cd ..
go build ./...
go vet ./...
go test ./...

# 结构校验（工作区级）
powershell -File .\scripts\validate-structure.ps1
powershell -File .\scripts\validate-frontend-structure.ps1 -ProjectDir "projects/daq-p1604/apps/desktop-wails/frontend/src"
```

**不需要**：`wails3 generate bindings`（无 Go 方法签名变化；且 daq-p1604 上跑此命令会误删 TS binding，见 project_memory §1）。

## Project Structure

改动仅集中在以下文件：

```
projects/daq-p1604/apps/desktop-wails/frontend/src/
├── components/
│   ├── device/
│   │   └── ScanResultList.vue        # 多选 checkbox + 内联改名 + 已添加置灰
│   └── layout/
│       └── AppShell.vue              # 扫描弹窗结构、批量添加逻辑、移除手动名字确认框跳转
└── stores/
    └── deviceStore.ts                # addProfile 支持批量 + 按 MAC/IP:Port 去重 + 默认名生成 + autoConnect 参数
```

## Code Style

沿用项目现有风格（TS strict + `<script setup>` + `defineProps/defineEmits` + CSS BEM 命名）。示例：

```ts
// deviceStore.ts —— 批量添加，返回逐条成功/跳过详情
interface AddProfileInput {
  address: string
  port: number
  macAddress: string
  serialNumber: string
  defaultAutoConnect: boolean
}

interface AddProfileResult {
  added: PressureProfile[]        // 本次新加入的
  skipped: Array<{                // 因去重跳过的
    address: string
    port: number
    macAddress: string
    reason: 'duplicate-mac' | 'duplicate-address'
  }>
}

/** 批量添加扫描到的设备；按 MAC 优先、IP:Port 次之去重；名字冲突自动追加 (2)/(3)/... */
async function addScannedProfiles(
  inputs: AddProfileInput[],
): Promise<AddProfileResult> {
  // ...
}
```

**默认名生成规则**：

```ts
// 生成设备默认名：优先用 MAC 末 6 位，回退用 IP 末段+端口
function makeDefaultName(input: AddProfileInput): string {
  const mac = input.macAddress?.replace(/[:\-]/g, '').toUpperCase()
  if (mac && mac.length >= 6) {
    return `DAQ-P-1604-${mac.slice(-6)}`
  }
  const ipTail = input.address.split('.').pop() ?? 'unknown'
  return `DAQ-P-1604-${ipTail}-${input.port}`
}

// 冲突自动追加 (2)/(3)/...
function dedupeName(base: string, existingNames: Set<string>): string {
  if (!existingNames.has(base)) return base
  let n = 2
  while (existingNames.has(`${base} (${n})`)) n += 1
  return `${base} (${n})`
}
```

**Profile ID 生成规则改为可复现**：

```ts
// 有 MAC 用 MAC；无 MAC 用 host-port，保证同一台真机重复扫描命中同一 profile
function makeProfileId(input: AddProfileInput): string {
  const mac = input.macAddress?.replace(/[:\-]/g, '').toLowerCase()
  if (mac) return `p1604-${mac}`
  return `p1604-${input.address.replace(/\./g, '-')}-${input.port}`
}
```

## Testing Strategy

本项目前端**无 Vitest / 无 Playwright**（package.json 仅 `typecheck / build / preview`），验证以**类型检查 + 构建 + 桌面应用手工验收**为主。

**手工验收剧本**：
1. 首次装机场景：清空 profile.json → 打开应用 → 侧边栏扫描 → 勾 2 台 → 点"添加所选 (2)"→ 弹窗关闭 → 侧边栏出现 2 条，名字不同、`autoConnect=true`。
2. 重复添加保护：不关闭应用再次扫描 → 弹窗中这 2 台显示 `已添加`、checkbox `disabled`、不能勾选。
3. 内联改名：扫描时改一台名字为 `工位A` → 添加后侧边栏显示 `工位A`。
4. 默认自动连接关闭：顶部开关切到关闭状态 → 添加另一台 → 新条目 `autoConnect=false`。
5. 重启应用 → `autoConnect=true` 的设备自动连接（复用现有 `autoConnectAll`）。
6. MAC 缺失回退：模拟扫描器（simulated_scanner）跑一遍，验证无 MAC 时按 IP+端口去重与命名。

## Boundaries

**Always（本任务必须遵守）**：
- 只改上述 3 个文件（`ScanResultList.vue` / `AppShell.vue` / `deviceStore.ts`）
- 生产代码带中文注释（用户偏好硬约束）
- 改完跑 `npm run typecheck` + `npm run build` + `go build ./...` + `go vet ./...` 全绿
- 遵守六边形约束：前端只调现有 binding，不新增 Go 方法

**Ask first（发现要动时先停下问）**：
- 若发现必须修改 `core.PressureProfile` 结构（当前判断不需要）
- 若发现必须修改后端 `ScanResult` / `p1604_scanner.go`
- 若发现必须新增 npm 依赖

**Never（红线）**：
- 不动 `backend/*.go`、`adapters/**`、`usecase/**`、`ports/**`、`core/*.go`
- 不跑 `wails3 generate bindings`（会误删 daq-p1604 的 TS binding）
- 不引入 Naive UI 依赖
- 不做设备分组/预设保存/入口形态变更（out of scope）
- 不主动创建额外文档 / README

## Success Criteria（可测）

| # | 条件 | 验证方式 |
|---|------|----------|
| S1 | 扫到 N 台新设备 → 勾 N 台 → 点"添加所选 (N)" → profiles 列表新增 N 条 | 手工：侧边栏计数 + profile.json 内容 |
| S2 | 添加后弹窗**关闭**（用户已选 Q1=B）| 手工 |
| S3 | 新条目名字互不相同，格式为 `DAQ-P-1604-<MAC末6>` 或 `DAQ-P-1604-<IP末段>-<port>` | 手工 + profile.json |
| S4 | 名字冲突自动加 `(2)/(3)/...` 后缀 | 手工（人为造两台 MAC 末6 相同） |
| S5 | 再次扫描 → 已添加设备 `checkbox disabled` + 显示"已添加" | 手工 |
| S6 | 相同 MAC / IP:Port 无法被批量添加（跳过并提示） | 手工：先手动添加一台再扫描 |
| S7 | 顶部"默认启用自动连接"开关初值 = 开启 | 手工 |
| S8 | 开关开启时新条目 `autoConnect=true`，关闭时 `false` | 手工 + profile.json |
| S9 | 重启应用后 `autoConnect=true` 的设备被 `autoConnectAll` 自动连接 | 手工 |
| S10 | 扫描弹窗内每行 checkbox 旁允许内联改名，改后名字生效 | 手工 |
| S11 | `npm run typecheck` + `npm run build` + `go build ./...` + `go vet ./...` 全部通过 | 命令 |

## Open Questions

暂无（Q1/Q2/Q3/Q4 已在意图确认阶段拍板：Q1=B / Q2=A / Q3=A / Q4=A）。

---

## Interaction Design（弹窗形态确认）

```
┌─ 扫描设备 ─────────────────────────── × ┐
│                                        │
│ [√] 勾选后添加的设备默认启用自动连接   │
│                                        │
│ ┌──────────────────────────────────┐   │
│ │ [√] DAQ-P-1604-3A5F1B  192.168…  │   │
│ │     [名称: DAQ-P-1604-3A5F1B  ]  │   │
│ ├──────────────────────────────────┤   │
│ │ [√] DAQ-P-1604-9C82AA  192.168…  │   │
│ │     [名称: DAQ-P-1604-9C82AA  ]  │   │
│ ├──────────────────────────────────┤   │
│ │ [ ] (已添加) 192.168.1.10:7000   │   │
│ │     灰色显示，不可勾选           │   │
│ └──────────────────────────────────┘   │
│                                        │
│               [取消]  [重新扫描]        │
│                     [添加所选 (2)]      │
└────────────────────────────────────────┘
```

Q1=B 决策：添加成功后**关闭弹窗**（不保持打开）。用户如需继续，重开扫描即可，已添加设备会自动置灰。

## Out of Scope（明确不做）

- ~~扫描入口位置调整~~ 仍在侧边栏搜索图标
- ~~设备分组 / 分类 / 预设保存~~
- ~~抽屉化 / 独立设备管理页~~
- ~~后端结构改动~~
- ~~新 Wails binding 方法~~
- ~~逐设备的自动连接独立开关~~（统一走顶部批量默认；单独调整走设备配置弹窗）
