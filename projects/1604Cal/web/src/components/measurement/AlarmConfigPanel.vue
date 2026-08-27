<template>
  <!--
    报警配置独立卡片（P0-4）。
    设计动机：原报警 checkbox 挤在控制条底部，与通道选择混在一起，
    缺乏层级感；报警阈值/单位没有可见入口。这里把"开关 + 阈值摘要 + 通道统计"
    整合为一张独立卡片，让使用者一眼看到当前报警策略。
    样式与 CalibrationParams / MeasurementControl 的 control-card 同源：
    自给自足卡片 + 顶部 1px mint 强调线，遵循 DESIGN.md「禁止卡片嵌套」。
  -->
  <section class="alarm-card">
    <div class="alarm-row">
      <!-- 标题区：图标 + "报警配置" 文案 -->
      <div class="title-group">
        <el-icon class="title-icon">
          <WarningFilled />
        </el-icon>
        <span class="title-text">报警配置</span>
      </div>

      <!-- 开关区：三个独立 toggle，与原控制条 inline-check 同风格 -->
      <div class="toggle-group">
        <label class="inline-check">
          <input
            v-model="measurementStore.alarmConfig.enabled"
            type="checkbox"
          >
          <span>启用</span>
        </label>
        <label class="inline-check">
          <input
            v-model="measurementStore.alarmConfig.soundEnabled"
            type="checkbox"
            :disabled="!measurementStore.alarmConfig.enabled"
          >
          <span>声音</span>
        </label>
        <label class="inline-check">
          <input
            v-model="measurementStore.alarmConfig.confirmOnAlarm"
            type="checkbox"
            :disabled="!measurementStore.alarmConfig.enabled"
          >
          <span>报警确认</span>
        </label>
      </div>

      <!-- 摘要区：阈值来源 + 报警通道统计 + 单位。
           precisionLevel 来自 measurementParams（与 MeasurementParamsPanel 同源），
           这里只读展示，编辑入口仍在参数面板，避免双写。 -->
      <div
        class="summary-group"
        :class="{ disabled: !measurementStore.alarmConfig.enabled }"
      >
        <div class="summary-item">
          <span class="summary-label">阈值</span>
          <span class="summary-value mono">±{{ precisionLevelPercent }}%</span>
          <span class="summary-hint">× 量程</span>
        </div>
        <span class="summary-divider" />
        <div class="summary-item">
          <span class="summary-label">报警通道</span>
          <span class="summary-value mono">{{ alarmChannelCount }}/16</span>
        </div>
        <span class="summary-divider" />
        <div class="summary-item">
          <span class="summary-label">单位</span>
          <span class="summary-value">{{ measureUnit || 'MPa' }}</span>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { WarningFilled } from '@element-plus/icons-vue'
import { useMeasurementStore } from '@/stores/measurement'

const measurementStore = useMeasurementStore()

// 精度等级百分比展示：0.0002 → "0.02%"
const precisionLevelPercent = computed(() =>
  (measurementStore.measurementParams.precisionLevel * 100).toFixed(2)
)

// 报警通道数：alarmConfig.enabledChannels 与 measurementStore.channels 在
// MeasurementView 的 watch 中保持同步（保存时 enabledChannels ← channels）。
// 这里直接取 enabledChannels 长度，反映"实际参与报警判定的通道数"。
// 防御：加载异常或旧配置缺失该字段时避免崩溃。
const alarmChannelCount = computed(() => measurementStore.alarmConfig?.enabledChannels?.length ?? 0)

const measureUnit = computed(() => measurementStore.measureUnit)
</script>

<style scoped lang="scss">
// 与 CalibrationParams / MeasurementControl 一致：自给自足卡片 + mint 顶部强调线。
// 不依赖外层 card-block 包裹，避免卡片嵌套（DESIGN.md 规定）。
.alarm-card {
  position: relative;
  background: #ffffff;
  border-radius: 10px;
  border: 1px solid $slate-200;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.05);
  padding: 8px 12px;
  font-family: $font-sans;
  overflow: hidden;
  flex-shrink: 0;
  transition: box-shadow 0.2s ease, border-color 0.2s ease;

  &::before {
    content: '';
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    height: 1px;
    background: $amber;
  }

  &:hover {
    box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.07), 0 2px 4px -2px rgba(0, 0, 0, 0.05);
  }
}

.alarm-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px 12px;
}

.title-group {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
}

.title-icon {
  font-size: 12px;
  color: $amber;
}

.title-text {
  font-size: 12px;
  font-weight: 600;
  color: $slate-700;
  letter-spacing: 0.02em;
}

.toggle-group {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

/* 与 MeasurementControl.inline-check 同风格，保证视觉一致 */
.inline-check {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: $slate-600;
  font-size: 12px;
  cursor: pointer;
  white-space: nowrap;
  font-family: $font-sans;

  input[type="checkbox"] {
    width: 13px;
    height: 13px;
    accent-color: $mint;
    border: 1px solid $slate-300;
    border-radius: 3px;
    cursor: pointer;

    &:disabled {
      cursor: not-allowed;
      opacity: 0.5;
    }
  }

  &:hover span {
    color: $slate-800;
  }
}

.summary-group {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-left: auto;
  padding-left: 12px;
  border-left: 1px solid $slate-100;
  flex-wrap: wrap;

  &.disabled {
    opacity: 0.5;
  }
}

.summary-item {
  display: flex;
  align-items: baseline;
  gap: 3px;
  white-space: nowrap;
}

.summary-label {
  font-size: 10px;
  color: $slate-400;
  font-weight: 500;
  letter-spacing: 0.05em;
  text-transform: uppercase;
}

.summary-value {
  font-size: 12px;
  font-weight: 600;
  color: $slate-700;
  font-family: $font-sans;

  &.mono {
    font-family: $font-mono;
    color: $slate-800;
  }
}

.summary-hint {
  font-size: 10px;
  color: $slate-400;
}

.summary-divider {
  width: 1px;
  height: 12px;
  background: $slate-200;
}

@media (max-width: 900px) {
  .summary-group {
    margin-left: 0;
    padding-left: 0;
    border-left: none;
    width: 100%;
    border-top: 1px solid $slate-100;
    padding-top: 6px;
  }
}
</style>
