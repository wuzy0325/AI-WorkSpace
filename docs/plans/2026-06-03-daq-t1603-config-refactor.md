# Spec: DAQ-T-1603 配置项精简与通道独立热电偶类型

## Objective

对 DAQ-T-1603 配置界面进行精简与增强：

1. **移除序号（ShowSequence）** — UI 删除，不让用户设置
2. **移除平均次数（AverageCount）** — UI 删除，不让用户设置
3. **通道独立热电偶类型** — 每个通道可单独选择热电偶类型（K/J/T/E/N/R/S/B）

涉及两个项目：`daq-t1603`（独立桌面应用）和 `wind-daq`（综合采集系统）。

## ASSUMPTIONS

1. Go 后端结构体字段保留（AverageCount、ShowSequence）以确保与共享 SDK 和 JSON 配置文件兼容；仅在 UI 层移除
2. 热电偶类型从"单一类型应用于所有通道"改为"UI 逐通道选择，保存时聚合为 16 字符字符串"
3. 共享 SDK（`shared/device-sdk/go/`）的 `DaqT1603HardwareConfig` 不修改（已使用 16 字符字符串）
4. 两个项目的 UI 修改同步进行

## Tech Stack

- **Frontend**: Vue 3 + TypeScript + Pinia
- **Backend**: Go (Wails v2)
- **Shared SDK**: Go module `shared.local/device-sdk/go`

## Commands

```powershell
# daq-t1603 standalone
cd projects/daq-t1603/apps/desktop-wails
go vet ./...
go test ./...
cd frontend
npm run typecheck
cd ../../../..

# wind-daq
cd projects/wind-daq/services/api-go
go vet ./...
go test ./...
cd ../../apps/desktop-wails/frontend
npm run typecheck
cd ../../../../../..
```

## Project Structure

### daq-t1603 standalone — 修改的文件

| 文件 | 修改内容 |
|------|----------|
| `frontend/src/components/device/DaqT1603Config.vue` | 移除平均次数/序号 UI，通道行增加热电偶类型下拉框 |
| `frontend/src/stores/deviceStore.ts` | `defaultT1603Config()` 更新默认值 |
| `frontend/src/bridge/deviceBridge.ts` | `T1603Config` 接口增加 `thermocoupleTypes`（16 字符）替代 `thermocoupleType`，ChannelConfig 增加 `thermocoupleType` |
| `backend/app.go` | 无变更（Go 结构体保留字段） |
| `adapters/hardware/t1603_adapter.go` | `mapT1603SharedConfig` 使用 `ThermocoupleTypes` 替代 `strings.Repeat` |
| `core/types.go` | `T1603Config.ThermocoupleType string` → `ThermocoupleTypes string`（或别名保持） |
| `frontend/wailsjs/go/models.ts` | 自动生成，需 `wails generate module` 后更新 |

### wind-daq — 修改的文件

| 文件 | 修改内容 |
|------|----------|
| `apps/desktop-wails/frontend/src/components/device/DaqT1603Config.vue` | 移除平均次数/序号 UI，单类型选择改为每通道独立下拉框 |
| `apps/desktop-wails/frontend/src/components/device/DeviceManagementDrawer.vue` | DAQ-T-1603 通道列表增加热电偶类型列；`createBlankProfile` 使用默认类型填充 |
| `apps/desktop-wails/frontend/src/api/types.ts` | 无变更（`DaqT1603HardwareConfig.thermocoupleTypes` 已为 16 字符字符串） |
| `services/api-go/internal/adapters/hardware/t1603_adapter.go` | 无变更（已使用 `ThermocoupleTypes` 1:1 映射） |
| `services/api-go/internal/core/device/types.go` | 无变更（字段保留） |

### 共享 SDK — 不修改

`shared/device-sdk/go/` 保持原样。

## Code Style

### daq-t1603 `DaqT1603Config.vue` — 通道行模板（示意）

```vue
<div class="config__channel-row">
  <span class="config__channel-index">CH{{ pad(i, 2) }}</span>
  <input v-model="channelNames[i - 1]" placeholder="名称" />
  <select v-model="channelTcTypes[i - 1]">
    <option v-for="t in thermocoupleOptions" :key="t" :value="t">{{ t }} 型</option>
  </select>
  <button @click="toggleChannel(i - 1)"><Zap /></button>
</div>
```

### wind-daq `DeviceManagementDrawer.vue` — DAQ-T-1603 通道表

```vue
<tr v-for="c in draft.channels" :key="c.index">
  <td class="font-mono">{{ pad(c.index + 1, 2) }}</td>
  <td><input v-model="c.name" :disabled="isReadOnly" /></td>
  <td>
    <select v-model="c.thermocoupleType" :disabled="isReadOnly">
      <option v-for="t in tcTypes" :key="t" :value="t">{{ t }}</option>
    </select>
  </td>
</tr>
```

## Testing Strategy

- `go test ./...` 在 daq-t1603 和 wind-daq 两个项目中通过
- `npm run typecheck` 在前端项目中通过
- 手动确认：序号和平均次数不再出现在 UI 中
- 手动确认：每个通道可独立选择热电偶类型

## Boundaries

- **Always:** 运行 `go vet ./...`、`npm run typecheck`，不修改共享 SDK 类型
- **Ask first:** 修改 Go 结构体字段名/类型（因 Wails 绑定需要重新生成）
- **Never:** 直接编辑 `wailsjs/go/models.ts`（应通过 `wails generate module` 重新生成）

## Success Criteria

1. ✅ daq-t1603 配置界面不再显示"序号"开关
2. ✅ daq-t1603 配置界面不再显示"平均次数"下拉框
3. ✅ daq-t1603 每个通道行右侧有独立的热电偶类型下拉框
4. ✅ wind-daq 设备编辑器中 DAQ-T-1603 通道表增加热电偶类型列
5. ✅ wind-daq 配置界面不再显示"平均次数"和"序号"
6. ✅ 保存配置后，硬件接收到的 `@f3` 命令包含正确的各通道热电偶类型
7. ✅ `go vet ./...` 和 `npm run typecheck` 通过

## Resolved Decisions

| 问题 | 决定 |
|------|------|
| daq-t1603 `ThermocoupleType` 字段名 | 改为 `ThermocoupleTypes string`，需 `wails generate module` 重新生成绑定 |
| wind-daq 通道列表列位置 | 热电偶类型列放在通道名称之后
