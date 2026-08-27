<template>
  <section class="params-card">
    <div class="params-row">
      <!-- 最小值 -->
      <div class="control-group">
        <label>最小:</label>
        <input
          v-model.number="measurementStore.measurementParams.minPressure"
          type="number"
          step="0.1"
          class="compact-input"
          :class="{ invalid: !minValid }"
          :aria-invalid="!minValid"
        >
      </div>

      <!-- 最大值 -->
      <div class="control-group">
        <label>最大:</label>
        <input
          v-model.number="measurementStore.measurementParams.maxPressure"
          type="number"
          step="0.1"
          class="compact-input"
          :class="{ invalid: !maxValid }"
          :aria-invalid="!maxValid"
        >
      </div>

      <!-- 测点数 -->
      <div class="control-group">
        <label>点数:</label>
        <input
          v-model.number="measurementStore.measurementParams.pointCount"
          type="number"
          min="2"
          max="11"
          class="compact-input narrow"
          :class="{ invalid: !pointCountValid }"
          :aria-invalid="!pointCountValid"
        >
      </div>

      <!-- 显示精度 -->
      <div class="control-group">
        <label>显示精度:</label>
        <input
          v-model.number="measurementStore.measurementParams.precision"
          type="number"
          min="0"
          max="6"
          class="compact-input narrow"
        >
      </div>

      <!-- 重复采样次数 -->
      <div class="control-group">
        <label>重复采样:</label>
        <input
          v-model.number="measurementStore.measurementParams.averageCount"
          type="number"
          min="1"
          max="10"
          class="compact-input narrow"
          :class="{ invalid: !averageCountValid }"
          :aria-invalid="!averageCountValid"
        >
      </div>

      <!-- 稳定时间 -->
      <div class="control-group">
        <label>稳定:</label>
        <select
          v-model.number="measurementStore.measurementParams.stableWaitS"
          class="compact-select"
        >
          <option :value="1">
            1s
          </option>
          <option :value="3">
            3s
          </option>
          <option :value="5">
            5s
          </option>
          <option :value="10">
            10s
          </option>
        </select>
      </div>

      <!-- 精度等级 -->
      <div class="control-group precision-level-group">
        <label>精度等级</label>
        <select
          v-model.number="p.precisionLevel"
          class="compact-select wide"
        >
          <option
            v-for="item in precisionLevelOptions"
            :key="item"
            :value="item"
          >
            {{ (item * 100).toFixed(2) }}%
          </option>
        </select>
      </div>

      <div class="divider" />

      <button
        type="button"
        class="generate-btn"
        :disabled="!isParamValid"
        @click="onGenerateClick"
      >
        生成压力表
      </button>

      <!-- 报警配置：合并自原独立 AlarmConfigPanel。
           放在参数行末尾，与生成按钮同行，让用户配置完参数顺手确认报警策略。
           只保留开关 + 摘要（阈值/通道数/单位），完整配置仍走独立卡片逻辑（已删除）。 -->
      <span
        class="alarm-divider"
        aria-hidden="true"
      />
      <div
        class="alarm-inline"
        :class="{ disabled: !measurementStore.alarmConfig.enabled }"
      >
        <label class="inline-check">
          <input
            v-model="measurementStore.alarmConfig.enabled"
            type="checkbox"
          >
          <span>报警</span>
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
          <span>确认</span>
        </label>
        <span class="alarm-summary">
          <span class="summary-item">阈值 <span class="mono">±{{ precisionLevelPercent }}%</span></span>
          <span class="summary-item">通道 <span class="mono">{{ alarmChannelCount }}/16</span></span>
          <span class="summary-item">单位 {{ measureUnit || 'MPa' }}</span>
        </span>
      </div>
    </div>

    <!-- P2-10：参数无效时在参数面板底部就地显示具体原因，
         让用户不用反复试错就能定位是哪个字段出了问题。
         role=status + aria-live=polite：参数错误属于非紧急提示，
         用 status 而非 alert（alert 隐含 assertive，会打断用户当前操作）。 -->
    <div
      v-if="!isParamValid"
      class="params-error"
      role="status"
      aria-live="polite"
    >
      <el-icon><WarningFilled /></el-icon>
      <span>{{ invalidReason }}</span>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { ElMessage } from 'element-plus'
import { WarningFilled } from '@element-plus/icons-vue'
import { useMeasurementStore } from '@/stores/measurement'
import { readSessionUnitConsistency } from '@/api/session'

const measurementStore = useMeasurementStore()

const precisionLevelOptions = [0.0001, 0.0002, 0.0005, 0.001, 0.002]

const p = computed(() => measurementStore.measurementParams)

// 字段级有效性：每个字段单独计算，驱动 input 的 invalid 视觉状态。
// 拆开计算而不是用一个 isParamValid 兜底，是为了让用户看到具体哪个字段错了。
const minValid = computed(() => Number.isFinite(p.value.minPressure))
const maxValid = computed(() =>
  Number.isFinite(p.value.maxPressure) &&
  Number.isFinite(p.value.minPressure) &&
  p.value.maxPressure > p.value.minPressure
)
const pointCountValid = computed(() =>
  p.value.pointCount >= 2 && p.value.pointCount <= 11
)
const averageCountValid = computed(() => p.value.averageCount >= 1)

const isParamValid = computed(() => {
  return (
    minValid.value &&
    maxValid.value &&
    pointCountValid.value &&
    averageCountValid.value
  )
})

// 错误原因文案：按字段优先级返回第一条不满足的条件，
// 与 isParamValid 的短路求值顺序保持一致，避免显示多条错误让用户分心。
const invalidReason = computed(() => {
  if (!minValid.value) return '最小压力未填写或非数字'
  if (!maxValid.value) {
    if (!Number.isFinite(p.value.maxPressure)) return '最大压力未填写或非数字'
    return '最大压力必须大于最小压力'
  }
  if (!pointCountValid.value) return '测点数必须在 2~11 之间'
  if (!averageCountValid.value) return '重复采样次数必须 ≥ 1'
  return ''
})

// 报警摘要字段：合并自原 AlarmConfigPanel
const precisionLevelPercent = computed(() =>
  (measurementStore.measurementParams.precisionLevel * 100).toFixed(2)
)
const alarmChannelCount = computed(() => measurementStore.alarmConfig?.enabledChannels?.length ?? 0)
const measureUnit = computed(() => measurementStore.measureUnit)

async function onGenerateClick() {
  if (!isParamValid.value) {
    // 就地错误提示已在面板内显示，这里只补一个 toast 用于"按钮被点击"的反馈
    ElMessage.warning(invalidReason.value || '请先填写有效的计量参数')
    return
  }
  const unitCheck = await readSessionUnitConsistency().catch(() => null)
  if (unitCheck && !unitCheck.consistent) {
    ElMessage.warning('设备压力单位不一致，建议统一单位后再生成压力表')
  }
  await measurementStore.generatePoints()
}
</script>

<style scoped lang="scss">
.params-card {
  background: #ffffff;
  border-radius: 10px;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.05);
  border: 1px solid $slate-200;
  padding: 10px 12px;
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
  font-family: $font-sans;
  transition: box-shadow 0.2s ease, border-color 0.2s ease;

  &:hover {
    box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.07), 0 2px 4px -2px rgba(0, 0, 0, 0.05);
  }
}

.params-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px 8px;
}

.control-group {
  display: flex;
  align-items: center;
  gap: 4px;

  label {
    font-size: 12px;
    color: $slate-500;
    font-weight: 500;
    letter-spacing: 0.05em;
    white-space: nowrap;
    font-family: $font-sans;
  }
}

.compact-input {
  height: 28px;
  font-size: 12px;
  border: 1px solid $slate-300;
  border-radius: 6px;
  padding: 0 6px;
  width: 48px;
  text-align: center;
  color: $slate-800;
  background: #fff;
  outline: none;
  font-variant-numeric: tabular-nums;
  font-family: $font-mono;
  transition: border-color 0.15s ease, box-shadow 0.15s ease;

  &:focus {
    border-color: $mint;
    box-shadow: 0 0 0 2px rgba(16, 185, 129, 0.15);
  }

  /* P2-10：字段非法时红色边框 + 浅红背景，让用户在视觉上立刻定位错误字段。
     focus 优先级高于 invalid：用户正在编辑的字段以 mint 聚焦反馈为准。 */
  &.invalid:not(:focus) {
    border-color: $red;
    background: rgba(239, 68, 68, 0.04);
  }

  &.narrow {
    width: 36px;
  }

  &.wide {
    width: 72px;
  }
}

.compact-input::-webkit-inner-spin-button,
.compact-input::-webkit-outer-spin-button {
  -webkit-appearance: none;
  margin: 0;
}

.compact-input {
  -moz-appearance: textfield;
}

.compact-select {
  height: 28px;
  font-size: 12px;
  border: 1px solid $slate-300;
  border-radius: 6px;
  padding: 0 20px 0 8px;
  color: $slate-700;
  background: #fff;
  outline: none;
  cursor: pointer;
  min-width: 48px;
  appearance: none;
  font-family: $font-sans;
  background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='12' viewBox='0 0 24 24' fill='none' stroke='%239ca3af' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpath d='m6 9 6 6 6-6'/%3E%3C/svg%3E");
  background-repeat: no-repeat;
  background-position: right 6px center;
  transition: border-color 0.15s ease, box-shadow 0.15s ease;

  &:focus {
    border-color: $mint;
    box-shadow: 0 0 0 2px rgba(16, 185, 129, 0.15);
  }

  &.wide {
    min-width: 72px;
  }
}

.precision-level-group {
  .compact-select,
  .compact-input {
    min-width: 72px;
  }
}

.divider {
  width: 1px;
  height: 18px;
  background: $slate-200;
  margin: 0 2px;
}

.generate-btn {
  height: 28px;
  padding: 0 12px;
  background: linear-gradient(135deg, $mint, $mint-dark);
  color: #fff;
  font-size: 12px;
  font-weight: 600;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.15s ease;
  font-family: $font-sans;
  box-shadow: 0 2px 8px rgba(16, 185, 129, 0.25);

  &:hover:not(:disabled) {
    background: linear-gradient(135deg, $mint-light, $mint);
    box-shadow: 0 4px 12px rgba(16, 185, 129, 0.35);
    transform: translateY(-1px);
  }

  &:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
}

/* P2-10：参数无效时的就地错误提示条，红色 amber 色阶与项目告警色一致 */
.params-error {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 6px;
  padding: 4px 10px;
  background: rgba(239, 68, 68, 0.06);
  border: 1px solid rgba(239, 68, 68, 0.2);
  border-radius: 6px;
  color: $red;
  font-size: 12px;
  font-weight: 500;
  font-family: $font-sans;

  .el-icon {
    font-size: 14px;
    flex-shrink: 0;
  }
}

/* 报警配置内联块：合并自原独立 AlarmConfigPanel，与生成按钮同行 */
.alarm-divider {
  width: 1px;
  height: 18px;
  background: $slate-200;
  margin: 0 2px;
}

.alarm-inline {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;

  &.disabled {
    opacity: 0.5;
  }
}

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

.alarm-summary {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 11px;
  color: $slate-500;
  font-family: $font-sans;
}

.summary-item {
  white-space: nowrap;

  .mono {
    font-family: $font-mono;
    color: $slate-700;
    font-weight: 600;
  }
}

@media (max-width: 900px) {
  .params-row {
    align-items: flex-start;
  }
}
</style>
