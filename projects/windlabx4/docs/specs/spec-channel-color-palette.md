# Spec: DAQ-P-1603 通道颜色区分优化

## Objective（目标）

### 我们在解决什么问题
WindLabX4 设备详情面板的 DAQ-P-1603 设备波形图和数值卡片缺乏颜色区分。

根因：[`channelColors.ts:89-114`](../../apps/desktop-wails/frontend/src/utils/channelColors.ts#L89-L114) 的 `buildChannelColorMap` 对 DAQ-P-1603 走 SensorType 着色，原压力色板仅 5 档蓝色。默认 16 通道全压力（[`DeviceManagementDrawer.vue:265-267`](../../apps/desktop-wails/frontend/src/components/device/DeviceManagementDrawer.vue#L265-L267)），16 / 5 = 循环 3 次以上，CH1/CH6/CH11/CH16 颜色完全相同，且 5 档蓝色明度差异小，导致波形图上挤在一起无法区分，数值卡片的圆点/sparkline/标签徽章也一并变蓝。

### 用户是谁
风洞操作员。DAQ-P-1603 是 16 通道通用 AI 压力采集设备，操作员在采集期间需要同时观察多通道压力趋势，颜色无法区分直接影响他们对通道状态的判断。

### 成功标准（可测试）
1. DAQ-P-1603 16 通道全压力场景下，[`buildChannelColorMap`](../../apps/desktop-wails/frontend/src/utils/channelColors.ts#L89-L114) 返回的 Map 中 16 个颜色值**互不重复**。
2. 压力色板覆盖 16 档冷色族（蓝/青/绿/紫 4 个色相 × 2 明度 × 2 个变体），保证相邻通道色相不同、隔 8 位明度不同。
3. 温度色板精简到 4 档暖色族（橙/红/黄/玫红），温度通道在压力曲线中视觉上极其醒目。
4. 非 DAQ-P-1603 设备完全不受影响：仍走 [`CHANNEL_COLORS`](../../apps/desktop-wails/frontend/src/utils/channelColors.ts#L62-L71) 8 色循环。
5. 现有下游消费者零改动：[`RealtimeChart.vue:144`](../../apps/desktop-wails/frontend/src/components/device/RealtimeChart.vue#L144)、[`DeviceDetailPanel.vue:60-64`](../../apps/desktop-wails/frontend/src/components/main/DeviceDetailPanel.vue#L60-L64)、[`ChartSelector.vue`](../../apps/desktop-wails/frontend/src/components/main/ChartSelector.vue) 仅读 Map，签名不变。
6. 温度色板第一档仍是 `#f97316`（橙色），与原方案一致，保留用户视觉记忆。
7. `npm run typecheck` + `npm run test` + `npm run build` 全绿，新增单元测试行覆盖率 ≥ 95%。
8. **dark 主题（主打场景）** 下 16 档冷色和 4 档暖色全部满足 WCAG 1.4.11 非文本对比度 ≥ 3:1（对深色背景 `#0f172a`）。light 主题（次要场景）下尽量满足 3:1，个别亮色族因色相对亮背景天然低对比度允许例外（见下方对比度实测表）。

## Tech Stack（技术栈）
- TypeScript 5.8（strict mode）
- Vitest 4（单元测试）
- 目标：仅 `projects/WindLabX4`，不动 `daq-t1603` / `daq-p1604` / `shared/`

## Commands（命令）

```powershell
# 进入前端目录
cd projects\WindLabX4\apps\desktop-wails\frontend

# 类型检查
npm run typecheck

# 单元测试（watch 模式开发用）
npx vitest run src/utils/__tests__/channelColors.test.ts

# 全量单元测试
npm run test

# 生产构建
npm run build

# 结构校验（提交前强制）
powershell -File ..\..\..\..\validate-frontend-structure.ps1 -CheckFileSize
```

> **提交前强制顺序**：`npm run typecheck` → `npm run test` → `npm run build` → `validate-frontend-structure.ps1`。

## Project Structure（项目结构）

本次改动仅涉及以下文件，不新增目录：

```
projects/WindLabX4/apps/desktop-wails/frontend/src/
├── utils/
│   ├── channelColors.ts                # 改：PRESSURE_PALETTE 扩到 16 档，TEMPERATURE_PALETTE 精简到 4 档
│   └── __tests__/
│       └── channelColors.test.ts       # 新增：色板长度、唯一性、温度优先橙、非 P-1603 不受影响
```

> 不动 `components/device/RealtimeChart.vue`、`components/main/DeviceDetailPanel.vue`、`components/main/ChartSelector.vue`、`components/main/ChannelCard.vue`——它们仅消费 `buildChannelColorMap` 返回的 Map，签名不变。

## Code Style（代码风格）

遵循工作区规则：**中文注释、类型注解、可扩展设计**。改动草案：

```ts
/**
 * 通道颜色生成工具。
 *
 * 设计目标：
 *   - 保证 RealtimeChart 曲线颜色与 DeviceDetailPanel/ChartSelector 通道卡片颜色一致
 *   - DAQ-P-1603 按 SensorType 分色系：
 *       压力（16 档冷色族：蓝/青/绿/紫 × 亮/深两档）
 *       温度（4 档暖色族：橙/红/黄/玫红）
 *   - 颜色与通道 index 一一对应，与用户在 ChartSelector 中选中的通道集合无关
 *
 * 实现要点：
 *   - 颜色映射按 profile.channels 顺序生成（而非按用户选中的 channelIndices），
 *     这样无论用户选哪些通道，每个通道的颜色都稳定
 *   - 压力色板 16 档恰好覆盖 DAQ-P-1603 最大 16 通道，零循环
 *   - 温度色板 4 档覆盖实际场景（温度通道少，一般 1-2 个）
 */

/**
 * DAQ-P-1603 压力通道色板（16 档冷色族）。
 * 8 个色相 × 2 明度：前 8 档亮色（L≈55%），后 8 档同色相深色版（L≈45%）。
 * 16 通道全压力时零循环，相邻通道色相不同、隔 8 位明度不同。
 * 对比度校准：CH6 原用 #84cc16 因 light 主题对比度 1.89 调为 #65a30d（dark 5.78/light 2.95）；
 *             CH16 原用 #86198f 因 dark 主题对比度 2.17 调为 #c026d3（dark 3.79/light 4.50）。
 */
export const PRESSURE_PALETTE = [
  // 亮色族（CH1-CH8）
  '#3b82f6', // 蓝
  '#06b6d4', // 青
  '#0ea5e9', // 天蓝
  '#14b8a6', // 青绿
  '#10b981', // 翠绿
  '#65a30d', // 黄绿（lime-600，对比度校准）
  '#8b5cf6', // 紫
  '#a855f7', // 紫红
  // 深色族（CH9-CH16，与 CH1-CH8 同色相但明度更低）
  '#2563eb', // 深蓝
  '#0891b2', // 深青
  '#0369a1', // 深天蓝
  '#0f766e', // 深青绿
  '#047857', // 深翠绿
  '#4d7c0f', // 深黄绿
  '#7c3aed', // 深紫
  '#c026d3', // 深紫红（fuchsia-600，对比度校准）
] as const

/**
 * DAQ-P-1603 温度通道色板（4 档暖色族）。
 * 温度通道少（一般 1-2 个，最多 4 个），4 档够用。
 * 第一档为橙色，与原方案一致，保留用户视觉记忆。
 * 对比度校准：第 3 档原用 #eab308 因 light 主题对比度 1.83 调为 #a16207（dark 3.63/light 4.71）。
 */
export const TEMPERATURE_PALETTE = [
  '#f97316', // 橙（保持原第一档，视觉记忆不破坏）
  '#ef4444', // 红
  '#a16207', // 黄（yellow-700，对比度校准）
  '#ec4899', // 玫红
] as const
```

### 对比度实测表（WCAG 1.4.11 非文本对比度 3:1）

背景色：dark `#0f172a`（主打）、light `#f8fafc`（次要）

| 色板位置 | Hex | dark 对比度 | light 对比度 | 备注 |
|---|---|---|---|---|
| 压力 CH1 | `#3b82f6` | 4.85 ✓ | 3.52 ✓ | |
| 压力 CH2 | `#06b6d4` | 7.35 ✓ | 2.32 ✗ | light 例外（色相醒目） |
| 压力 CH3 | `#0ea5e9` | 6.44 ✓ | 2.65 ✗ | light 例外（色相醒目） |
| 压力 CH4 | `#14b8a6` | 7.17 ✓ | 2.38 ✗ | light 例外（色相醒目） |
| 压力 CH5 | `#10b981` | 7.04 ✓ | 2.42 ✗ | light 例外（色相醒目） |
| 压力 CH6 | `#65a30d` | 5.78 ✓ | 2.95 ✗ | light 接近 3:1，lime 色相醒目 |
| 压力 CH7 | `#8b5cf6` | 4.22 ✓ | 4.05 ✓ | |
| 压力 CH8 | `#a855f7` | 4.51 ✓ | 3.78 ✓ | |
| 压力 CH9 | `#2563eb` | 3.45 ✓ | 4.94 ✓ | |
| 压力 CH10 | `#0891b2` | 4.85 ✓ | 3.52 ✓ | |
| 压力 CH11 | `#0369a1` | 3.01 ✓ | 5.67 ✓ | |
| 压力 CH12 | `#0f766e` | 3.26 ✓ | 5.23 ✓ | |
| 压力 CH13 | `#047857` | 3.26 ✓ | 5.24 ✓ | |
| 压力 CH14 | `#4d7c0f` | 3.58 ✓ | 4.77 ✓ | |
| 压力 CH15 | `#7c3aed` | 3.13 ✓ | 5.45 ✓ | |
| 压力 CH16 | `#c026d3` | 3.79 ✓ | 4.50 ✓ | dark 校准（原 #86198f = 2.17） |
| 温度 1 | `#f97316` | 6.37 ✓ | 2.68 ✗ | light 例外（保留视觉记忆） |
| 温度 2 | `#ef4444` | 4.74 ✓ | 3.60 ✓ | |
| 温度 3 | `#a16207` | 3.63 ✓ | 4.71 ✓ | light 校准（原 #eab308 = 1.83） |
| 温度 4 | `#ec4899` | 5.06 ✓ | 3.37 ✓ | |

> **light 主题例外说明**：CH2-CH5 和温度 1（橙）在 light 主题下对比度 2.3-2.7，不足 3:1 但色相醒目（青/绿/橙）实际可见。强行调暗会破坏 dark 主题的亮/深分族设计（16 档区分度下降），权衡后保留。原方案（5 档 blue-300~blue-700）在 light 下对比度更差（blue-300 ≈ 1.5），本方案已优于原方案。

## Testing Strategy（测试策略）

### 框架与位置
- Vitest 4 + @vue/test-utils（项目既有栈）
- 测试文件：`src/utils/__tests__/channelColors.test.ts`（新建）
- 命名约定：与 `src/stores/__tests__/deviceStore.test.ts` 对齐

### 测试覆盖矩阵

| 用例 | 验证点 | 期望 |
|------|--------|------|
| 压力色板长度 | `PRESSURE_PALETTE.length` | === 16 |
| 压力色板唯一性 | `new Set(PRESSURE_PALETTE).size` | === 16（零重复） |
| 温度色板长度 | `TEMPERATURE_PALETTE.length` | === 4 |
| 温度色板首档 | `TEMPERATURE_PALETTE[0]` | === '#f97316'（视觉记忆） |
| 16 通道全压力零循环 | `buildChannelColorMap('DAQ-P-1603', 16 压力通道)` | Map.size === 16 且所有颜色互不重复 |
| 温度优先橙 | 单温度通道 | 返回 '#f97316' |
| 混合场景 | 14 压力 + 2 温度 | 16 个颜色互不重复 |
| 非 P-1603 不受影响 | `buildChannelColorMap('DAQ-P-1604', [...])` | 走 CHANNEL_COLORS 8 色循环 |
| 空通道边界 | `buildChannelColorMap('DAQ-P-1603', [])` | 返回空 Map |
| pickChannelColor 便捷函数 | 单通道查询 | 与 buildChannelColorMap 结果一致 |

### 测试用例格式
按用户偏好采用三段式：
```ts
describe('buildChannelColorMap - DAQ-P-1603 压力色板', () => {
  // 测试前置：构造 16 通道全压力输入
  const channels = Array.from({ length: 16 }, (_, i) => ({
    index: i, sensorType: 'pressure' as const,
  }))

  // 测试步骤：调用 buildChannelColorMap
  const map = buildChannelColorMap('DAQ-P-1603', channels)

  // 期待结果：16 个颜色互不重复
  it('16 通道全压力时零循环，颜色互不重复', () => {
    const colors = Array.from(map.values())
    expect(new Set(colors).size).toBe(16)
  })
})
```

## Boundaries（边界）

### Always（必须做）
- 改 `channelColors.ts` 时保留所有 export 名称（`PRESSURE_PALETTE` / `TEMPERATURE_PALETTE` / `CHANNEL_COLORS` / `ChannelColorInput` / `buildChannelColorMap` / `pickChannelColor`），避免下游 break
- 颜色值用小写 hex（`#3b82f6` 而非 `#3B82F6`），与现有色板风格一致
- 注释说明"为什么"（色相分组、明度分档的原因），不写"做了什么"
- 提交前跑完整验证顺序：typecheck → test → build → validate-frontend-structure

### Ask First（先问再做）
- 想把 `channelColors.ts` 提到 `shared/`——需要单独迁移 spec
- 想让颜色主题感知（light/dark 用不同色板）——会破坏 RealtimeChart 现有 `readThemeColors()` 简化假设
- 想引入 chroma/d3-color 等运行时依赖——违背"硬编码色板"原则
- 想修改 `ChannelColorInput` 接口签名——会破坏 3 处下游消费者

### Never（绝不做）
- 不改 `RealtimeChart.vue` / `DeviceDetailPanel.vue` / `ChartSelector.vue` / `ChannelCard.vue`——它们零改动
- 不动非 DAQ-P-1603 设备的 8 色循环逻辑
- 不在 spec 之外"顺手"重构周边代码（避免 over-engineering）
- 不创建新的文档文件（除本 spec 本身）

## Success Criteria（成功标准摘要）

| # | 验证点 | 验证方式 |
|---|--------|----------|
| 1 | 16 通道全压力零循环 | 单元测试 `new Set(colors).size === 16` |
| 2 | 温度首档保持橙色 | 单元测试 `TEMPERATURE_PALETTE[0] === '#f97316'` |
| 3 | 非 P-1603 不受影响 | 单元测试 DAQ-P-1604 仍走 8 色循环 |
| 4 | 下游零改动 | git diff 显示仅 `channelColors.ts` + 测试文件被修改 |
| 5 | 类型检查通过 | `npm run typecheck` 退出码 0 |
| 6 | 测试全绿 | `npm run test` 退出码 0，覆盖率 ≥ 95% |
| 7 | 构建通过 | `npm run build` 退出码 0 |
| 8 | 人工验证 | 启动 WindLabX4，添加 DAQ-P-1603 设备，16 通道全压力下波形图和数值卡片颜色互不重色 |

## Open Questions（开放问题）

1. **~~深色档在 dark 主题下的对比度~~**（已解决）：实测后 CH16 `#86198f` dark 对比度仅 2.17，不达 WCAG 1.4.11 的 3:1，已调为 `#c026d3`（dark 3.79/light 4.50）。CH6 `#84cc16` 和温度 3 `#eab308` 在 light 主题下分别 1.89/1.83，已调为 `#65a30d`/`#a16207`。spec 标准从 4.5:1（文本）修正为 3:1（非文本，WCAG 1.4.11），因为波形图线条和圆点是图形对象非文本。

2. **温度色板扩展性**：当前 4 档假设温度通道不超过 4 个。如果未来出现 5+ 温度通道（罕见），会循环到第一档（橙色）。是否需要预先扩到 8 档？
   - 倾向：保持 4 档，避免 over-engineering。实际触发再扩。
