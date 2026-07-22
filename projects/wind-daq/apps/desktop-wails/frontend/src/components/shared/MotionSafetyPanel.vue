<script setup lang="ts">
/**
 * 运动安全配置面板（共享组件）
 *
 * 设计目标：在遍历硬件配置步骤与校准配置界面中复用同一面板，暴露 4 个全局运动安全
 * 阈值与按轴覆盖能力，让操作员根据设备精度（编码器/无编码器）与机构行程独立调整
 * 到位容差、严重偏离阈值、无进展超时、进展阈值。
 *
 * 迁移历史：原位于 `components/traversal/MotionSafetyPanel.vue`，校准模块运动安全
 * 移植（spec-calibration-motion-safety）时提升为共享组件，校准与遍历均通过
 * `@components/shared/MotionSafetyPanel.vue` 引用。
 *
 * 数据模型约束：
 *   - 后端 MotionSafetyConfig 字段全部为指针（*float64 / *int），零值表示"未配置"，
 *     下游 Resolve() 时合并默认值。前端用 number | undefined 表达同语义。
 *   - 输入框留空 → 字段从配置中移除（undefined）→ 后端使用默认值或全局值（轴覆盖场景）。
 *   - 输入框有值 → 字段写入配置 → 后端使用用户指定值。
 *
 * 校验规则（Task 18 契约对齐：blocking 与 advisory 分离，后端 validateMotionSafetyConfig 始终权威）：
 *   参考：
 *   - traversal_helpers.go 的 EvaluateMotionSafety（轴态判定）
 *   - core/traversal/types.go 的 (*MotionSafetyConfig).Resolve（默认值合并）
 *   - traversal_config.go 的 validateMotionSafetyConfig / validateMotionSafetyFields（字段范围与跨字段校验）
 *
 *   blockingErrors（与后端拒绝规则一一对齐；非空时 isValid=false，父组件阻止保存）：
 *   - 浮点字段必须有限（非 NaN/Inf），且 > 0（progressEpsilon 同样要求 > 0）
 *   - noProgressTimeoutMs 必须 >= 200（后端 2*motionCompletePollMsForValidation=200）
 *   - criticalDeviationLimit 必须严格大于 arrivalTolerance（两者都配置时）
 *   - axisOverrides 键必须是 motionAxes 中已绑定的轴名（后端拒绝未绑定轴）
 *   - 按轴覆盖项内字段同样适用上述范围校验
 *   - 每个绑定轴 Resolve(axis) 后的合并值必须满足 criticalDeviationLimit > arrivalTolerance
 *     （防止"全局+轴覆盖"组合倒置；嵌套 axisOverrides 由类型层面阻止，运行时不重复校验）
 *
 *   advisoryWarnings（后端不拒绝、仅 UX 提示；不使 isValid=false）：
 *   - progressEpsilon 应小于 arrivalTolerance（否则微动卡死会被误判为"有进展"）
 *   - noProgressTimeoutMs 应显著小于 waitForMotionComplete 的 120s 兜底超时
 *     （否则看门狗永远不先于兜底超时触发，形同虚设）
 *
 * 产品决策（2026-07-22）：运动安全 UI 在遍历测试与所有探针校准配置中隐藏，
 * 统一使用后端 DEFAULT_MOTION_SAFETY 默认值，避免操作员误调引发急停/卡死。
 *   - 模板根 v-if="false" 使面板不渲染；调用处的 v-model:motion-safety 仍可正常双向绑定，
 *     已保存配置中的 motionSafety 字段在加载时仍会回填并在保存时透传（向后兼容）。
 *   - 父组件通过 ref 读取的 isValid 因面板未挂载而为 null，保存前校验自动跳过。
 *   - 如需恢复 UI，移除下方 <UiPanel> 上的 v-if="false" 即可。
 */
import { computed } from 'vue'
import type { MotionSafetyConfig, MotionSafetyFieldKey } from '@shared/types/traversal'
import { DEFAULT_MOTION_SAFETY } from '@shared/types/traversal'
import UiPanel from '@components/ui/UiPanel.vue'
import UiInputNumber from '@components/ui/UiInputNumber.vue'

/**
 * 运动轴最小引用——只关心 UI 展示与 axisOverrides key 所需的两个字段。
 *
 * 设计动机：避免共享组件耦合到 traversal.TraversalMotionAxisConfig 或
 * calibration.MotionAxisConfig 任一具体类型；通过结构化类型兼容让两侧均可传入。
 *   - traversal.TraversalMotionAxisConfig 满足此接口（name/axis 为字面量类型，是 string 子类型）
 *   - calibration.MotionAxisConfig 满足此接口（name/axis 为 string）
 */
export interface MotionSafetyPanelAxis {
  /** 轴的逻辑名称（如 "α"、"β"、"X"），仅用于表格行展示 */
  name: string
  /** 物理轴标识（如 "X"/"Y"/"Z"/"U"），用作 axisOverrides 的 key */
  axis: string
}

const props = defineProps<{
  t: Record<string, string>
  /** 当前已绑定的运动轴列表（来自遍历 Hardware 步骤或校准 motionAxes 配置），用于按轴覆盖表格的行 */
  motionAxes: MotionSafetyPanelAxis[]
}>()

// defineModel 双向绑定：父组件通过 v-model:motion-safety 传入/接收配置
// 初始 undefined 时面板内部所有输入框留空，等价于"使用后端默认值"
const motionSafety = defineModel<MotionSafetyConfig | undefined>('motionSafety', { default: undefined })

// ---- 全局字段访问器 ----
// 模板中以 :value + @update:value 形式绑定，避免 v-model 对 ref 属性路径写入失败。
// 留空（null）→ 字段移除（undefined）→ 后端使用默认值
function getGlobalField(key: MotionSafetyFieldKey): number | null {
  const v = motionSafety.value?.[key]
  return typeof v === 'number' ? v : null
}
function setGlobalField(key: MotionSafetyFieldKey, v: number | null): void {
  const next: MotionSafetyConfig = { ...(motionSafety.value ?? {}) }
  if (v === null || !Number.isFinite(v)) {
    delete next[key]
  } else {
    ;(next[key] as unknown as number) = v
  }
  motionSafety.value = next
}

// 按轴覆盖字段访问器：留空 → 从覆盖项中删除该字段（继承全局值）
function getAxisField(axis: string, key: MotionSafetyFieldKey): number | null {
  const v = motionSafety.value?.axisOverrides?.[axis]?.[key]
  return typeof v === 'number' ? v : null
}
function setAxisField(axis: string, key: MotionSafetyFieldKey, v: number | null): void {
  const root: MotionSafetyConfig = { ...(motionSafety.value ?? {}) }
  const overrides = { ...(root.axisOverrides ?? {}) }
  const current = { ...(overrides[axis] ?? {}) }
  if (v === null || !Number.isFinite(v)) {
    delete current[key]
  } else {
    ;(current[key] as unknown as number) = v
  }
  // 覆盖项全空则删除整条，避免持久化空对象
  const hasAny = (['arrivalTolerance', 'criticalDeviationLimit', 'noProgressTimeoutMs', 'progressEpsilon'] as const)
    .some((k) => current[k] !== undefined)
  if (hasAny) {
    overrides[axis] = current
  } else {
    delete overrides[axis]
  }
  // 整个 axisOverrides 全空则删除字段，让序列化更干净
  if (Object.keys(overrides).length > 0) {
    root.axisOverrides = overrides
  } else {
    delete root.axisOverrides
  }
  motionSafety.value = root
}

// 全局 4 个字段的快捷访问封装，模板中通过 :value + @update:value 使用
const arrivalTolerance = computed(() => getGlobalField('arrivalTolerance'))
const criticalDeviationLimit = computed(() => getGlobalField('criticalDeviationLimit'))
const noProgressTimeoutMs = computed(() => getGlobalField('noProgressTimeoutMs'))
const progressEpsilon = computed(() => getGlobalField('progressEpsilon'))

function setArrivalTolerance(v: number | null) { setGlobalField('arrivalTolerance', v) }
function setCriticalDeviationLimit(v: number | null) { setGlobalField('criticalDeviationLimit', v) }
function setNoProgressTimeoutMs(v: number | null) { setGlobalField('noProgressTimeoutMs', v) }
function setProgressEpsilon(v: number | null) { setGlobalField('progressEpsilon', v) }

// 按轴覆盖：每行返回一个轴的 4 个字段访问器
// axisFieldAccessors 在 motionAxes 变化时自动重算，保证表格行与绑定轴一致
const axisFieldAccessors = computed(() =>
  props.motionAxes.map((ax) => ({
    name: ax.name,
    axis: ax.axis,
    arrivalTolerance: getAxisField(ax.axis, 'arrivalTolerance'),
    criticalDeviationLimit: getAxisField(ax.axis, 'criticalDeviationLimit'),
    noProgressTimeoutMs: getAxisField(ax.axis, 'noProgressTimeoutMs'),
    progressEpsilon: getAxisField(ax.axis, 'progressEpsilon'),
  })),
)

// ---- placeholder 生成 ----
// 全局字段留空时显示"默认 <值>"，让操作员一眼看到后端兜底值；
// 按轴覆盖留空时显示"继承 <全局当前值 或 默认值>"，让操作员看到该轴实际生效的阈值。
// 与后端 Resolve() 合并优先级（默认 < 全局 < 轴覆盖）保持语义一致。
function defaultTpl(value: number): string {
  return props.t.travMotionSafetyDefaultTpl.replace('{value}', String(value))
}
function inheritTpl(globalValue: number | null, defaultValue: number): string {
  // 全局已配置 → 继承全局；全局未配置 → 继承默认值
  return props.t.travMotionSafetyInheritTpl.replace('{value}', String(globalValue ?? defaultValue))
}

// 全局 4 个字段 placeholder（始终显示后端默认值）
const arrivalDefaultPh = computed(() => defaultTpl(DEFAULT_MOTION_SAFETY.arrivalTolerance))
const criticalDefaultPh = computed(() => defaultTpl(DEFAULT_MOTION_SAFETY.criticalDeviationLimit))
const timeoutDefaultPh = computed(() => defaultTpl(DEFAULT_MOTION_SAFETY.noProgressTimeoutMs))
const epsilonDefaultPh = computed(() => defaultTpl(DEFAULT_MOTION_SAFETY.progressEpsilon))

// 按轴覆盖 4 个字段 placeholder：依赖全局当前值，全局变化时自动重算
const arrivalInheritPh = computed(() => inheritTpl(arrivalTolerance.value, DEFAULT_MOTION_SAFETY.arrivalTolerance))
const criticalInheritPh = computed(() => inheritTpl(criticalDeviationLimit.value, DEFAULT_MOTION_SAFETY.criticalDeviationLimit))
const timeoutInheritPh = computed(() => inheritTpl(noProgressTimeoutMs.value, DEFAULT_MOTION_SAFETY.noProgressTimeoutMs))
const epsilonInheritPh = computed(() => inheritTpl(progressEpsilon.value, DEFAULT_MOTION_SAFETY.progressEpsilon))

// ---- 实时校验 ----
// Task 18 契约对齐：校验分为 blockingErrors（与后端 validateMotionSafetyConfig 拒绝规则一一对齐）
// 与 advisoryWarnings（后端不拒绝、仅 UX 提示）。isValid 仅由 blockingErrors 决定。
//
// blocking 规则（与后端 traversal_config.go validateMotionSafetyConfig / validateMotionSafetyFields 对齐）：
//   B1. 浮点字段必须有限（非 NaN、非 Inf）
//   B2. arrivalTolerance > 0
//   B3. criticalDeviationLimit > 0
//   B4. criticalDeviationLimit > arrivalTolerance（两者都配置时）
//   B5. noProgressTimeoutMs >= 200（2 * motionCompletePollMsForValidation）
//   B6. progressEpsilon > 0
//   B7. axisOverrides 键必须是 motionAxes 中已绑定的轴名
//   B9. 每个绑定轴 Resolve(axis) 后 criticalDeviationLimit > arrivalTolerance
//   （B8 嵌套 axisOverrides 由 MotionSafetyAxisOverride 类型在编译期阻止，运行时不重复校验）
//
// advisory 规则（后端不拒绝，仅 UX 提示）：
//   A1. progressEpsilon >= arrivalTolerance（微动卡死会被误判为有进展）
//   A2. noProgressTimeoutMs >= 120000（看门狗永远不先于兜底超时触发）
//   A3. axisOverrides 包含未绑定轴（用户可能遗留旧配置；后端会拒绝，前端提前提示）
const MIN_NO_PROGRESS_TIMEOUT_MS = 200 // 后端 2 * motionCompletePollMsForValidation
const FALLBACK_TIMEOUT_MS = 120000      // 后端 waitForMotionComplete 兜底超时

/**
 * 解析指定轴的有效合并配置（与后端 MotionSafetyConfig.Resolve(axis) 语义一致）。
 * 合并优先级：默认值 < 全局配置 < 按轴覆盖。
 * 返回的 4 个字段一定有值（默认兜底），供跨字段合并校验使用。
 */
function resolveAxis(axis: string): Required<
  Pick<MotionSafetyConfig, 'arrivalTolerance' | 'criticalDeviationLimit' | 'noProgressTimeoutMs' | 'progressEpsilon'>
> {
  const root = motionSafety.value
  const ov = root?.axisOverrides?.[axis]
  return {
    arrivalTolerance: ov?.arrivalTolerance ?? root?.arrivalTolerance ?? DEFAULT_MOTION_SAFETY.arrivalTolerance,
    criticalDeviationLimit: ov?.criticalDeviationLimit ?? root?.criticalDeviationLimit ?? DEFAULT_MOTION_SAFETY.criticalDeviationLimit,
    noProgressTimeoutMs: ov?.noProgressTimeoutMs ?? root?.noProgressTimeoutMs ?? DEFAULT_MOTION_SAFETY.noProgressTimeoutMs,
    progressEpsilon: ov?.progressEpsilon ?? root?.progressEpsilon ?? DEFAULT_MOTION_SAFETY.progressEpsilon,
  }
}

/** 按轴覆盖字段读取辅助（仅限 4 个数值字段，不含 axisOverrides） */
function getAxisFieldTyped(axis: string, key: MotionSafetyFieldKey): number | null {
  const v = motionSafety.value?.axisOverrides?.[axis]?.[key]
  return typeof v === 'number' ? v : null
}

/** 未绑定轴的覆盖键——用户可能从旧 profile 加载到已不存在的轴配置 */
const unboundOverrideKeys = computed<string[]>(() => {
  const root = motionSafety.value
  if (!root?.axisOverrides) return []
  const bound = new Set(props.motionAxes.map((a) => a.axis))
  return Object.keys(root.axisOverrides).filter((k) => !bound.has(k))
})

/**
 * 阻断错误：与后端 validateMotionSafetyConfig 拒绝规则一一对齐。
 * 空数组表示配置在后端会被接受；非空时父组件应阻止保存。
 */
const blockingErrors = computed<string[]>(() => {
  const errors: string[] = []
  const a = arrivalTolerance.value
  const c = criticalDeviationLimit.value
  const n = noProgressTimeoutMs.value
  const e = progressEpsilon.value

  // B1+B2: arrivalTolerance 必须有限且 > 0
  if (a !== null) {
    if (!Number.isFinite(a)) errors.push(props.t.travMotionSafetyErrArrivalFinite)
    else if (a <= 0) errors.push(props.t.travMotionSafetyErrArrivalPositive)
  }
  // B1+B3: criticalDeviationLimit 必须有限且 > 0
  if (c !== null) {
    if (!Number.isFinite(c)) errors.push(props.t.travMotionSafetyErrCriticalFinite)
    else if (c <= 0) errors.push(props.t.travMotionSafetyErrCriticalPositive)
  }
  // B5: noProgressTimeoutMs >= 200（后端仅此一条；< 200 已涵盖 <= 0，不重复校验 > 0）
  if (n !== null && n < MIN_NO_PROGRESS_TIMEOUT_MS) {
    errors.push(props.t.travMotionSafetyErrTimeoutMin200)
  }
  // B1+B6: progressEpsilon 必须有限且 > 0
  if (e !== null) {
    if (!Number.isFinite(e)) errors.push(props.t.travMotionSafetyErrEpsilonFinite)
    else if (e <= 0) errors.push(props.t.travMotionSafetyErrEpsilonPositive)
  }

  // B4: 全局 criticalDeviationLimit 必须严格大于 arrivalTolerance（两者都配置时）
  if (a !== null && c !== null && Number.isFinite(a) && Number.isFinite(c) && c <= a) {
    errors.push(props.t.travMotionSafetyErrCriticalGreaterThanArrival)
  }

  // B7: axisOverrides 键必须是 motionAxes 中已绑定的轴名（后端拒绝未绑定轴）
  //     用户可能从旧 profile 加载到已不存在的轴配置，前端阻断并提示
  for (const key of unboundOverrideKeys.value) {
    errors.push(props.t.travMotionSafetyErrUnboundAxis.replace('{axis}', key))
  }

  // 按轴覆盖字段范围校验（B1-B6 同全局规则，仅对覆盖项内已设置字段）
  for (const ax of props.motionAxes) {
    const oa = getAxisFieldTyped(ax.axis, 'arrivalTolerance')
    const oc = getAxisFieldTyped(ax.axis, 'criticalDeviationLimit')
    const on = getAxisFieldTyped(ax.axis, 'noProgressTimeoutMs')
    const oe = getAxisFieldTyped(ax.axis, 'progressEpsilon')
    if (oa !== null) {
      if (!Number.isFinite(oa)) errors.push(`[${ax.axis}] ${props.t.travMotionSafetyErrArrivalFinite}`)
      else if (oa <= 0) errors.push(`[${ax.axis}] ${props.t.travMotionSafetyErrArrivalPositive}`)
    }
    if (oc !== null) {
      if (!Number.isFinite(oc)) errors.push(`[${ax.axis}] ${props.t.travMotionSafetyErrCriticalFinite}`)
      else if (oc <= 0) errors.push(`[${ax.axis}] ${props.t.travMotionSafetyErrCriticalPositive}`)
    }
    if (on !== null && on < MIN_NO_PROGRESS_TIMEOUT_MS) {
      errors.push(`[${ax.axis}] ${props.t.travMotionSafetyErrTimeoutMin200}`)
    }
    if (oe !== null) {
      if (!Number.isFinite(oe)) errors.push(`[${ax.axis}] ${props.t.travMotionSafetyErrEpsilonFinite}`)
      else if (oe <= 0) errors.push(`[${ax.axis}] ${props.t.travMotionSafetyErrEpsilonPositive}`)
    }
  }

  // B9: 每个绑定轴 Resolve(axis) 后的合并值校验 critical > arrival
  //      防止"全局 arrival + 轴覆盖 critical"组合倒置绕过单对象校验
  for (const ax of props.motionAxes) {
    const resolved = resolveAxis(ax.axis)
    if (resolved.criticalDeviationLimit <= resolved.arrivalTolerance) {
      errors.push(`[${ax.axis}] ${props.t.travMotionSafetyErrCriticalGreaterThanArrival}`)
    }
  }

  return errors
})

/**
 * 建议修正：后端不拒绝、仅 UX 提示的规则。
 * 非空时不阻止保存，但运行时可能行为异常（如看门狗失效、微动卡死误判）。
 */
const advisoryWarnings = computed<string[]>(() => {
  const warnings: string[] = []
  const a = arrivalTolerance.value
  const c = criticalDeviationLimit.value
  const n = noProgressTimeoutMs.value
  const e = progressEpsilon.value

  // A1: progressEpsilon >= arrivalTolerance（微动卡死会被误判为有进展）
  //     仅在两者都为有限数时校验，避免与 B1/B4 阻断错误重复
  if (a !== null && e !== null && Number.isFinite(a) && Number.isFinite(e) && e >= a) {
    warnings.push(props.t.travMotionSafetyErrEpsilonLessThanArrival)
  }

  // A2: noProgressTimeoutMs >= 120000（看门狗永远不先于兜底超时触发）
  if (n !== null && n >= FALLBACK_TIMEOUT_MS) {
    warnings.push(props.t.travMotionSafetyErrTimeoutLessThan120s)
  }

  return warnings
})

// 是否通过校验——父组件可据此阻止保存。
// 仅 blockingErrors 决定；advisoryWarnings 不影响 isValid（契约要求 advisory 不使 isValid=false）。
const isValid = computed(() => blockingErrors.value.length === 0)
// 暴露 blockingErrors / advisoryWarnings 供父组件读取具体错误信息（如 toast 提示首个阻断错误）
defineExpose({ isValid, blockingErrors, advisoryWarnings })
</script>

<template>
  <!-- v-if="false"：按产品决策隐藏运动安全 UI，统一使用后端默认值。
       恢复方式：移除本 v-if 即可重新渲染面板与按轴覆盖表格。 -->
  <UiPanel v-if="false" class="motion-safety-card">
    <template #header>
      <div class="panel-header">
        <span class="panel-title">{{ t.travMotionSafety }}</span>
        <span class="panel-hint">{{ t.travMotionSafetyHint }}</span>
      </div>
    </template>

    <!-- 全局阈值：4 个数值输入框，紧凑水平排列。
         标签列 120px，控件 1fr，单位列 50px，与全局配置规范 §35 对齐。
         使用 :value + @update:value 而非 v-model，因为 v-model 写入 computed 属性路径
         会替换 ref 而非触发 setter。
         placeholder 显示后端默认值，让操作员一眼看到留空的生效值。 -->
    <div class="global-section">
      <div class="field-row">
        <label class="field-label">
          <span class="label-text">{{ t.travArrivalTolerance }}</span>
          <span class="hint-badge" :title="t.travArrivalToleranceHint">?</span>
        </label>
        <UiInputNumber
          :model-value="arrivalTolerance"
          :min="0"
          :step="0.001"
          :placeholder="arrivalDefaultPh"
          class="field-input"
          @update:model-value="setArrivalTolerance"
        />
        <span class="field-unit">{{ t.travUnitMmOrDeg }}</span>
      </div>
      <div class="field-row">
        <label class="field-label">
          <span class="label-text">{{ t.travCriticalDeviationLimit }}</span>
          <span class="hint-badge" :title="t.travCriticalDeviationLimitHint">?</span>
        </label>
        <UiInputNumber
          :model-value="criticalDeviationLimit"
          :min="0"
          :step="0.1"
          :placeholder="criticalDefaultPh"
          class="field-input"
          @update:model-value="setCriticalDeviationLimit"
        />
        <span class="field-unit">{{ t.travUnitMmOrDeg }}</span>
      </div>
      <div class="field-row">
        <label class="field-label">
          <span class="label-text">{{ t.travNoProgressTimeout }}</span>
          <span class="hint-badge" :title="t.travNoProgressTimeoutHint">?</span>
        </label>
        <UiInputNumber
          :model-value="noProgressTimeoutMs"
          :min="200"
          :step="100"
          :placeholder="timeoutDefaultPh"
          class="field-input"
          @update:model-value="setNoProgressTimeoutMs"
        />
        <span class="field-unit">{{ t.travUnitMs }}</span>
      </div>
      <div class="field-row">
        <label class="field-label">
          <span class="label-text">{{ t.travProgressEpsilon }}</span>
          <span class="hint-badge" :title="t.travProgressEpsilonHint">?</span>
        </label>
        <UiInputNumber
          :model-value="progressEpsilon"
          :min="0"
          :step="0.0001"
          :placeholder="epsilonDefaultPh"
          class="field-input"
          @update:model-value="setProgressEpsilon"
        />
        <span class="field-unit">{{ t.travUnitMmOrDeg }}</span>
      </div>
    </div>

    <!-- 按轴覆盖表格：每行一个绑定轴，4 个阈值可独立覆盖，留空继承全局。
         placeholder 显示"继承 <当前全局值 或 默认值>"，让操作员看到该轴实际生效阈值。
         使用 table-like grid 布局保证列对齐；行高与全局字段一致保持视觉密度。 -->
    <div v-if="axisFieldAccessors.length > 0" class="axis-overrides">
      <div class="axis-header">
        <span class="axis-col-axis">{{ t.physicalAxis }}</span>
        <span class="axis-col-field">
          <span class="label-text">{{ t.travArrivalToleranceShort }}</span>
          <span class="hint-badge" :title="t.travArrivalToleranceHint">?</span>
        </span>
        <span class="axis-col-field">
          <span class="label-text">{{ t.travCriticalDeviationLimitShort }}</span>
          <span class="hint-badge" :title="t.travCriticalDeviationLimitHint">?</span>
        </span>
        <span class="axis-col-field">
          <span class="label-text">{{ t.travNoProgressTimeoutShort }}</span>
          <span class="hint-badge" :title="t.travNoProgressTimeoutHint">?</span>
        </span>
        <span class="axis-col-field">
          <span class="label-text">{{ t.travProgressEpsilonShort }}</span>
          <span class="hint-badge" :title="t.travProgressEpsilonHint">?</span>
        </span>
      </div>
      <div v-for="ax in axisFieldAccessors" :key="ax.name" class="axis-row">
        <span class="axis-col-axis">{{ ax.axis }}</span>
        <UiInputNumber
          :model-value="ax.arrivalTolerance"
          :min="0"
          :step="0.001"
          :placeholder="arrivalInheritPh"
          class="axis-input"
          @update:model-value="(v: number | null) => setAxisField(ax.axis, 'arrivalTolerance', v)"
        />
        <UiInputNumber
          :model-value="ax.criticalDeviationLimit"
          :min="0"
          :step="0.1"
          :placeholder="criticalInheritPh"
          class="axis-input"
          @update:model-value="(v: number | null) => setAxisField(ax.axis, 'criticalDeviationLimit', v)"
        />
        <UiInputNumber
          :model-value="ax.noProgressTimeoutMs"
          :min="200"
          :step="100"
          :placeholder="timeoutInheritPh"
          class="axis-input"
          @update:model-value="(v: number | null) => setAxisField(ax.axis, 'noProgressTimeoutMs', v)"
        />
        <UiInputNumber
          :model-value="ax.progressEpsilon"
          :min="0"
          :step="0.0001"
          :placeholder="epsilonInheritPh"
          class="axis-input"
          @update:model-value="(v: number | null) => setAxisField(ax.axis, 'progressEpsilon', v)"
        />
      </div>
    </div>

    <!-- 阻断错误提示：与后端 validateMotionSafetyConfig 拒绝规则对齐，红色高亮。
         父组件通过 isValid 阻止保存；advisoryWarnings 不会出现在此区块。 -->
    <div v-if="blockingErrors.length > 0" class="validation-errors">
      <div class="error-section-title">{{ t.travMotionSafetyBlocking }}</div>
      <div v-for="(err, i) in blockingErrors" :key="i" class="error-item">⚠ {{ err }}</div>
    </div>
    <!-- 建议修正提示：后端不拒绝、仅 UX 提示，黄色警示。
         不阻止保存，但运行时可能行为异常（如看门狗失效、微动卡死误判）。 -->
    <div v-if="advisoryWarnings.length > 0" class="advisory-warnings">
      <div class="advisory-section-title">{{ t.travMotionSafetyAdvisory }}</div>
      <div v-for="(warn, i) in advisoryWarnings" :key="i" class="advisory-item">ℹ {{ warn }}</div>
    </div>
  </UiPanel>
</template>

<style scoped>
/* 运动安全面板：紧凑密度，与 Hardware 步骤其他卡片视觉对齐 */
.motion-safety-card {
  font-size: var(--text-sm)
}
.motion-safety-card :deep(.n-card__content) {
  padding: 4px 8px
}
.motion-safety-card :deep(.n-card-header) {
  padding: 4px 8px
}

.panel-header {
  display: flex;
  align-items: baseline;
  gap: var(--space-2);
  flex: 1;
  min-width: 0
}
.panel-title {
  font-size: var(--text-sm);
  font-weight: 600;
  color: var(--text-primary);
  white-space: nowrap
}
.panel-hint {
  font-size: var(--text-xs);
  color: var(--text-tertiary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap
}

/* 全局阈值区：4 行，标签列 120px + 控件列 1fr + 单位列 ~50px */
.global-section {
  display: flex;
  flex-direction: column;
  gap: 6px
}
.field-row {
  display: grid;
  grid-template-columns: 120px 1fr 50px;
  align-items: center;
  gap: 6px
}
.field-label {
  /* flex 布局：文字 ellipsis + ? 号 badge 固定不收缩 */
  display: flex;
  align-items: center;
  gap: 4px;
  min-width: 0;
  font-size: var(--text-xs);
  color: var(--text-secondary)
}
.field-label .label-text {
  /* 文字部分允许截断，badge 始终可见 */
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap
}
/* ? 号提示徽标：小圆圈样式，hover 显示 :title 说明 */
.hint-badge {
  flex-shrink: 0;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 14px;
  height: 14px;
  border-radius: 50%;
  border: 1px solid var(--border-default);
  color: var(--text-muted);
  font-size: 10px;
  font-weight: 600;
  line-height: 1;
  cursor: help;
  user-select: none
}
.hint-badge:hover {
  border-color: var(--accent-info);
  color: var(--accent-info)
}
.field-input {
  width: 100%
}
.field-unit {
  font-size: var(--text-xs);
  color: var(--text-tertiary);
  text-align: left
}

/* 按轴覆盖表格：5 列（轴名 + 4 个阈值），列宽自适应 */
.axis-overrides {
  margin-top: 8px;
  border-top: 1px dashed var(--border-default);
  padding-top: 6px
}
.axis-header,
.axis-row {
  display: grid;
  grid-template-columns: 50px repeat(4, 1fr);
  align-items: center;
  gap: 6px;
  padding: 2px 0
}
.axis-header {
  border-bottom: 1px solid var(--border-default);
  padding-bottom: 3px
}
.axis-row:hover {
  background: var(--bg-panel-strong);
  border-radius: var(--radius-md)
}
.axis-col-axis {
  font-size: var(--text-xs);
  font-weight: 600;
  color: var(--text-primary)
}
.axis-header .axis-col-field {
  /* 表头列：文字 + ? 号 badge 水平居中排列 */
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 3px;
  font-size: var(--text-xs);
  color: var(--text-muted)
}
.axis-header .axis-col-field .label-text {
  /* Short 标签较短，允许截断兜底 */
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap
}
.axis-input {
  width: 100%
}

/* 阻断错误提示：红色高亮（与后端拒绝规则对齐，阻止保存） */
.validation-errors {
  margin-top: 6px;
  padding: 4px 6px;
  border-radius: var(--radius-md);
  background: rgba(239, 68, 68, 0.08);
  border: 1px solid rgba(239, 68, 68, 0.3)
}
.error-section-title {
  font-size: var(--text-xs);
  font-weight: 600;
  color: var(--color-error, #ef4444);
  margin-bottom: 2px
}
.error-item {
  font-size: var(--text-xs);
  color: var(--color-error, #ef4444);
  line-height: 1.4
}

/* 建议修正提示：黄色警示（不阻止保存，但运行时可能行为异常） */
.advisory-warnings {
  margin-top: 4px;
  padding: 4px 6px;
  border-radius: var(--radius-md);
  background: rgba(245, 158, 11, 0.08);
  border: 1px solid rgba(245, 158, 11, 0.3)
}
.advisory-section-title {
  font-size: var(--text-xs);
  font-weight: 600;
  color: #d97706;
  margin-bottom: 2px
}
.advisory-item {
  font-size: var(--text-xs);
  color: #b45309;
  line-height: 1.4
}
</style>
