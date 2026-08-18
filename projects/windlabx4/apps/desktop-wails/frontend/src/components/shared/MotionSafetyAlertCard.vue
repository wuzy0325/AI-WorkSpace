<script setup lang="ts">
/**
 * 运动安全故障现场告警卡片（共享组件）
 *
 * 设计目标：在校准运行时界面与遍历实时监控界面复用同一卡片，结构化展示
 * 后端 Status.MotionSafetyFailure 现场快照（控制器 / 轴 / 目标 / 实际 / 偏差 / 点号），
 * 避免操作员从 lastError 字符串中人工解析。
 *
 * 配色策略：
 *   - 急停类 verdict（critical_deviation / limit_triggered / status_unavailable）→ 红色高亮
 *     强调需立即处置，且后端已下发急停指令
 *   - 普通停止类 verdict（deviation / overshoot / no_progress）→ 橙色提示
 *     表示运动被安全停止但未达急停级别
 *
 * 数据来源：与 MotionSafetyPanel.vue 同源（@shared/types/traversal 中的 MotionSafetyFailure），
 * 通过 props.failure 注入；当 failure 为 null / undefined 时卡片不渲染。
 *
 * i18n 策略：复用 traversal 模块的 travMotionSafety* 系列键，校准与遍历共用同一份文案，
 * 不引入 calibMotionSafety* 重复键集合（与 CalibrationErrorCode = TraversalErrorCode 同构策略一致）。
 */
import { computed } from 'vue'
import { AlertTriangle } from '@lucide/vue'
import type { MotionSafetyFailure, MotionSafetyVerdict } from '@shared/types/traversal'
import { getMotionSafetyVerdictLabel, isMotionSafetyEmergency } from '@shared/types/traversal'

const props = defineProps<{
  /** 运动安全故障现场快照；为 null/undefined 时不渲染卡片 */
  failure: MotionSafetyFailure | null | undefined
  /** i18n 字典：调用方传入完整 t 对象，内部读取 travMotionSafety* 系列键 */
  t: Record<string, string>
}>()

/**
 * verdict → 本地化文案映射（响应式）。
 *
 * 以 computed 暴露给模板，使语言切换后 verdict 标签即时刷新。
 * 实际的 verdict 查表与缺省回退由共享函数 getMotionSafetyVerdictLabel 统一维护，
 * 避免与 TraversalLiveMonitor 各持一份 switch 实现导致行为分叉。
 */
const verdictLabels = computed<Partial<Record<MotionSafetyVerdict, string>>>(() => ({
  ok: props.t.travMotionSafetyVerdictOk,
  arrived: props.t.travMotionSafetyVerdictArrived,
  deviation: props.t.travMotionSafetyVerdictDeviation,
  critical_deviation: props.t.travMotionSafetyVerdictCriticalDeviation,
  limit_triggered: props.t.travMotionSafetyVerdictLimitTriggered,
  no_progress: props.t.travMotionSafetyVerdictNoProgress,
  overshoot: props.t.travMotionSafetyVerdictOvershoot,
  status_unavailable: props.t.travMotionSafetyVerdictStatusUnavailable
}))

/** 偏差格式化：actual - target，3 位小数，与目标/实际保持一致精度 */
function formatDeviation(f: MotionSafetyFailure): string {
  return (f.actual - f.target).toFixed(3)
}
</script>

<template>
  <!--
    运动安全故障现场卡片：仅在 failure 存在时渲染。
    急停类用红色高亮强调严重性，普通停止类用橙色提示。
    现场信息（控制器/轴/目标/实际/偏差/点号）直接展示，避免操作员从 lastError 字符串解析。
  -->
  <div
    v-if="failure"
    class="motion-safety-alert"
    :class="isMotionSafetyEmergency(failure.verdict) ? 'is-emergency' : 'is-warning'"
  >
    <div class="alert-header">
      <AlertTriangle
        class="alert-icon"
        :style="{ color: isMotionSafetyEmergency(failure.verdict) ? 'var(--accent-danger)' : 'var(--state-warning)' }"
      />
      <span
        class="alert-title"
        :style="{ color: isMotionSafetyEmergency(failure.verdict) ? 'var(--accent-danger)' : 'var(--state-warning)' }"
      >
        {{ isMotionSafetyEmergency(failure.verdict) ? t.travMotionSafetyAlertEmergency : t.travMotionSafetyAlert }}
        · {{ getMotionSafetyVerdictLabel(failure.verdict, verdictLabels) }}
      </span>
    </div>
    <div class="alert-grid">
      <div class="alert-row">
        <span class="row-label">{{ t.travMotionSafetyController }}</span>
        <span class="row-value truncate" :title="failure.controllerId">
          {{ failure.controllerName || failure.controllerId || '--' }}
        </span>
      </div>
      <div class="alert-row">
        <span class="row-label">{{ t.travMotionSafetyAxis }}</span>
        <span class="row-value">{{ failure.axis || '--' }}</span>
      </div>
      <div class="alert-row">
        <span class="row-label">{{ t.travMotionSafetyTarget }}</span>
        <span class="row-value">{{ failure.target.toFixed(3) }}</span>
      </div>
      <div class="alert-row">
        <span class="row-label">{{ t.travMotionSafetyActual }}</span>
        <span class="row-value">{{ failure.actual.toFixed(3) }}</span>
      </div>
      <div class="alert-row">
        <span class="row-label">{{ t.travMotionSafetyDeviation }}</span>
        <span
          class="row-value row-deviation"
          :style="{ color: isMotionSafetyEmergency(failure.verdict) ? 'var(--accent-danger)' : 'var(--state-warning)' }"
        >
          {{ formatDeviation(failure) }}
        </span>
      </div>
      <div class="alert-row">
        <span class="row-label">{{ t.travMotionSafetyPointIndex }}</span>
        <span class="row-value">{{ failure.pointIndex }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* 卡片容器：圆角 + 1px 边框 + 半透明背景。
   is-emergency 红色（accent-danger），is-warning 橙色（state-warning）。
   color-mix 让背景与文字配色同源，避免视觉割裂。 */
.motion-safety-alert {
  border-radius: var(--radius-md, 6px);
  padding: 8px 10px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.motion-safety-alert.is-emergency {
  background: color-mix(in srgb, var(--accent-danger) 14%, transparent);
  border: 1px solid var(--accent-danger);
}

.motion-safety-alert.is-warning {
  background: color-mix(in srgb, var(--state-warning) 12%, transparent);
  border: 1px solid var(--state-warning);
}

/* 标题行：图标 + 标签 + verdict，紧凑无换行 */
.alert-header {
  display: flex;
  align-items: center;
  gap: 6px;
}

.alert-icon {
  width: 12px;
  height: 12px;
  flex-shrink: 0;
}

.alert-title {
  font-size: 10px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

/* 现场字段网格：2 列布局，标签左对齐、数值右对齐。
   gap-y-0.5 让多行信息紧凑可读。 */
.alert-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 2px 8px;
}

.alert-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 4px;
  font-size: 10px;
}

.row-label {
  color: var(--text-muted);
  white-space: nowrap;
}

.row-value {
  color: var(--text-secondary);
  max-width: 100px;
}

/* 偏差字段：在应急态加重显示，让操作员第一眼看到关键偏离量 */
.row-deviation {
  font-weight: 500;
}
</style>
