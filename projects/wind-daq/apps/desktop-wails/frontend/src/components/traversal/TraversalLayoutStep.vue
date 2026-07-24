<script setup lang="ts">
import { computed, ref } from 'vue'
import type { StepSegment, TraversalMotionAxisConfig, TraversalPattern, TraversalPrimaryAxis, TraversalPoint } from '@shared/types/traversal'
import { filterTraversalAxisOptions, findDuplicateMotionAxisBindings, findOccupiedMotionAxisDirections, findTraversalAxisKindIssues, getTraversalDisplayedAxes, motionAxisBindingKey } from '@shared/types/traversal'
import { useTraversalSegmentValidation, getSegmentError } from '@composables/useTraversalValidation'
// 自定义点位文件导入：前端纯解析，不依赖后端文件 IO
import { parsePointsFileWithWarnings, type ParsedPoint } from '@shared/pointsFileParser'
import { useFeedbackStore } from '@stores/feedbackStore'
import { useI18nStore } from '@stores/i18nStore'
import { useMotionStore } from '@stores/motionStore'
import UiButton from '@components/ui/UiButton.vue'
import UiDialog from '@components/ui/UiDialog.vue'
import UiPanel from '@components/ui/UiPanel.vue'
import UiInput from '@components/ui/UiInput.vue'
import UiInputNumber from '@components/ui/UiInputNumber.vue'
import UiCheckbox from '@components/ui/UiCheckbox.vue'
import UiSelect from '@components/ui/UiSelect.vue'
// 自定义布点表格：虚拟滚动 + 多选批量删除/清空，替换原 .pt-list 解决上百点画面崩溃
import CustomPointsTable from '@components/traversal/CustomPointsTable.vue'

const testName = defineModel<string>('testName', { required: true })
const dwellTimeMs = defineModel<number>('dwellTimeMs', { required: true })
const samplesPerPoint = defineModel<number>('samplesPerPoint', { required: true })
const pattern = defineModel<TraversalPattern>('pattern', { required: true })
const lineConfig = defineModel<{
  startX: number; endX: number
  xStepSegments: StepSegment[]
}>('lineConfig', { required: true })
const rectangleConfig = defineModel<{
  xMin: number; xMax: number; xStepSegments: StepSegment[]
  yMin: number; yMax: number; yStepSegments: StepSegment[]
}>('rectangleConfig', { required: true })
const sectorConfig = defineModel<{
  centerX: number; centerY: number; radiusMin: number; radiusMax: number
  radialStepSegments: StepSegment[]; angleStart: number; angleEnd: number
  angularStepSegments: StepSegment[]
}>('sectorConfig', { required: true })
const customPoints = defineModel<TraversalPoint[]>('customPoints', { required: true })
const customPointInput = defineModel<{ x: number; y: number; z: number; u: number }>('customPointInput', { required: true })
const snakeOrder = defineModel<boolean>('snakeOrder', { required: true })
// 走线主轴：仅 rectangle 布局消费（line 单行无主轴概念，sector 固定先走半径）。
// 用 default: 'x' 而非 required，避免未来其他父组件漏传时 dev 告警 + radio 无选中项。
const primaryAxis = defineModel<TraversalPrimaryAxis>('primaryAxis', { default: 'x' })
// 运动轴绑定（X/Y 方向 → 控制器 + 物理轴）：原属 Hardware 步骤，迁至布点步骤，
// 使扇形布点的轴类型约束（X=直线轴/平移台、Y=旋转轴/旋转台）与布点模式选择同屏可见。
const motionAxes = defineModel<TraversalMotionAxisConfig[]>('motionAxes', { required: true })

const props = defineProps<{
  estimatedPointCount: number
  t: Record<string, string>
}>()

const motionStore = useMotionStore()
const i18n = useI18nStore()

// 扇形布点轴类型约束：选项过滤（axisOptionsFor）+ 视觉错误（axisKindIssueTargets）
// + 扇形面板告警（sectorAxisInvalid），与 TraversalSettings.vue isStepValid 的阻断
// 共用 shared/types/traversal.ts 同一真相源。其余布局不限制轴类型。
const allAxisNames = ['X', 'Y', 'Z', 'U'] as const
const showSectorAxisHint = computed(() => pattern.value === 'sector')
const axisKindIssues = computed(() => findTraversalAxisKindIssues(motionAxes.value, motionStore.profiles, pattern.value))
const axisKindIssueTargets = computed(() => new Set(axisKindIssues.value.map(i => i.name)))
const sectorAxisInvalid = computed(() => axisKindIssues.value.length > 0)

/** 单个方向行的物理轴选项：扇形布局按所选控制器 profile 的轴类型过滤；
 *  未选控制器或非扇形布局时返回全部 4 根轴（原 Hardware 步骤行为）。
 *  被其他显示行占用的轴在 label 上追加占用后缀（如"X 轴（已被 Y 方向占用）"）作事前预告；
 *  选项保持可选不 disabled——交换两行绑定必须先选到对方占用的轴，禁用会锁死交换路径。 */
function axisOptionsFor(ax: TraversalMotionAxisConfig) {
  const profile = motionStore.profiles.find(p => p.id === ax.controllerId)
  const allowed = filterTraversalAxisOptions(profile, pattern.value, ax.name)
  const names = allowed ?? allAxisNames
  const occupied = occupiedAxesByRow.value.get(ax.name)
  return names.map(a => {
    const base = props.t.travAxisSuffix.replace('{axis}', a)
    const occupiers = occupied?.get(a)
    if (!occupiers || occupiers.length === 0) return { label: base, value: a }
    // {direction} 填占用方向名（X/Y/Z/U，多方向用 / 连接）；括号随语言切换全角/半角，
    // 与 travAxisSuffix 的中英文 label 风格保持一致
    const suffix = props.t.travAxisOccupiedBy.replace('{direction}', occupiers.join('/'))
    const label = i18n.locale === 'zh' ? `${base}（${suffix}）` : `${base} (${suffix})`
    return { label, value: a }
  })
}

// 走线主轴选项：与 TraversalHardwareStep.vue 的 radio 模式对齐（v-model + options 数组 + active 高亮）
const primaryAxisOptions = computed(() => [
  { value: 'x' as const, label: props.t.travPrimaryAxisX || 'X first' },
  { value: 'y' as const, label: props.t.travPrimaryAxisY || 'Y first' }
])

// 运动轴显示过滤：按"后端实际消费的轴"显示，避免冗余输入。
// 过滤规则（line → X，rectangle/sector → X/Y，custom → 4 轴）与运行屏共用
// shared/types/traversal.ts 的 getTraversalDisplayedAxes 单一真相源。
const displayedMotionAxes = computed(() => getTraversalDisplayedAxes(pattern.value, motionAxes.value))

// 物理轴占用预告：每个显示行 → 同控制器上"被其他显示行占用的物理轴"映射。
// 与下方重复绑定检测互补——重复检测是选完后的事后拦截（红框 + 横幅 + 阻断），
// 本映射供 axisOptionsFor 在下拉选项 label 上做事前预告（占用后缀），
// 判定规则（仅 displayedMotionAxes、仅同控制器、排除当前行）由 shared 侧
// findOccupiedMotionAxisDirections 单一实现，避免两份判定逻辑漂移。
const occupiedAxesByRow = computed(() => {
  const map = new Map<string, Map<string, Array<'X' | 'Y' | 'Z' | 'U'>>>()
  for (const ax of displayedMotionAxes.value) {
    map.set(ax.name, findOccupiedMotionAxisDirections(displayedMotionAxes.value, ax))
  }
  return map
})

// 同控制器同一物理轴被多个方向绑定的冲突检测。
// 设计动机：X→ctrlA.X + Z→ctrlA.X 时，两个方向都会向同一根物理轴下发 MoveTo，
// 后到目标覆盖先到目标，另一个方向静默丢失——必须 UI 阻断，避免静默 bug。
// 仅对 displayedMotionAxes（用户实际看到的行）做高亮，未显示的行不参与判定。
const duplicateAxisBindings = computed(() => findDuplicateMotionAxisBindings(displayedMotionAxes.value))
const hasDuplicateAxisBinding = computed(() => duplicateAxisBindings.value.size > 0)
function isAxisBindingDuplicated(ax: TraversalMotionAxisConfig): boolean {
  if (!ax.controllerId || !ax.axis) return false
  return duplicateAxisBindings.value.has(motionAxisBindingKey(ax))
}

// 逻辑目标方向 → i18n 标签键：与 i18nStore 的 travTargetXDirection/Y/Z/U 对齐。
// 用函数映射代替三元/switch，便于后续扩展第五轴时仅改映射表。
const directionLabelKey: Record<'X' | 'Y' | 'Z' | 'U', keyof typeof props.t> = {
  X: 'travTargetXDirection',
  Y: 'travTargetYDirection',
  Z: 'travTargetZDirection',
  U: 'travTargetUDirection'
}
function getDirectionLabel(name: 'X' | 'Y' | 'Z' | 'U'): string {
  // 缺失 i18n 键时回退到 "{name}方向"，避免显示空字符串让用户以为 UI 没渲染
  return props.t[directionLabelKey[name]] as string || `${name}方向`
}

// 遍历顺序适用性：与后端 path.go:399-455 的消费矩阵严格对齐。
// - 蛇形 snakeOrder：仅 rectangle（行列反转）+ sector（半径环反转角度）有意义；
//   line 单行无"行"概念，custom 用户已自定序，勾选无效果。
// - 走线主轴 primaryAxis：仅 rectangle 消费（line 单行无主轴，sector 固定先走半径）。
// 抽成 computed 避免模板里重复 pattern 判断。
const supportsSnakeOrder = computed(() => pattern.value === 'rectangle' || pattern.value === 'sector')
const supportsPrimaryAxis = computed(() => pattern.value === 'rectangle')

// computedRectangleRange/computedSectorRange 改为纯 computed（无副作用）：
// 旧实现在此处写 rectangleConfig.value.xMin/xMax/yMin/yMax 是 Vue 反模式——
// computed 是惰性求值，副作用只在模板访问时触发；当 TraversalLayoutStep 被
// v-if="currentStep === 0" 销毁（用户切到 step 1）时，副作用停止，xMin/xMax 停在旧值，
// 与 segments 不同步。现在 xMin/xMax 由 TraversalSettings.vue 的 currentLayout 实时派生，
// 此处仅用于模板显示（line 240 的 "X: ... Y: ..." 文本）。
const computedRectangleRange = computed(() => {
  const xs = rectangleConfig.value.xStepSegments
  const ys = rectangleConfig.value.yStepSegments
  const xMin = xs.length > 0 ? Math.min(...xs.map(s => s.start)) : 0
  const xMax = xs.length > 0 ? Math.max(...xs.map(s => s.end)) : 0
  const yMin = ys.length > 0 ? Math.min(...ys.map(s => s.start)) : 0
  const yMax = ys.length > 0 ? Math.max(...ys.map(s => s.end)) : 0
  return { xMin, xMax, yMin, yMax }
})

const rectangleRangeError = computed(() => {
  if (computedRectangleRange.value.xMax <= computedRectangleRange.value.xMin) {
    return `X: ${props.t.maxGreaterThanMin || 'Max must be greater than min'}`
  }
  if (computedRectangleRange.value.yMax <= computedRectangleRange.value.yMin) {
    return `Y: ${props.t.maxGreaterThanMin || 'Max must be greater than min'}`
  }
  return ''
})

// sector 同理：移除写 sectorConfig.value 的副作用。
// 注意：sector 模板有 radiusMin/radiusMax/angleStart/angleEnd 的输入框（v-model），
// 旧副作用会覆盖用户手动输入的值——这也是 bug。现在用户输入值得以保留。
const computedSectorRange = computed(() => {
  const rs = sectorConfig.value.radialStepSegments
  const as = sectorConfig.value.angularStepSegments
  const radiusMin = rs.length > 0 ? Math.min(...rs.map(s => s.start)) : 0
  const radiusMax = rs.length > 0 ? Math.max(...rs.map(s => s.end)) : 0
  const angleStart = as.length > 0 ? Math.min(...as.map(s => s.start)) : 0
  const angleEnd = as.length > 0 ? Math.max(...as.map(s => s.end)) : 0
  return { radiusMin, radiusMax, angleStart, angleEnd }
})

const tRef = computed(() => props.t)

// 根据模式类型获取对应的显示标签
const getPatternLabel = (p: TraversalPattern): string => {
  switch (p) {
    case 'line': return tRef.value.patternLine
    case 'rectangle': return tRef.value.patternRectangle
    case 'sector': return tRef.value.patternSector
    default: return tRef.value.patternCustom
  }
}

const { errors: rxSegErrs, countError: rxSegCntErr } = useTraversalSegmentValidation(computed(() => rectangleConfig.value.xStepSegments), computed(() => rectangleConfig.value.xMin), computed(() => rectangleConfig.value.xMax), tRef)
const { errors: rySegErrs } = useTraversalSegmentValidation(computed(() => rectangleConfig.value.yStepSegments), computed(() => rectangleConfig.value.yMin), computed(() => rectangleConfig.value.yMax), tRef)
const { errors: srSegErrs } = useTraversalSegmentValidation(computed(() => sectorConfig.value.radialStepSegments), computed(() => sectorConfig.value.radiusMin), computed(() => sectorConfig.value.radiusMax), tRef)
const { errors: saSegErrs } = useTraversalSegmentValidation(computed(() => sectorConfig.value.angularStepSegments), computed(() => sectorConfig.value.angleStart), computed(() => sectorConfig.value.angleEnd), tRef)
const { errors: lxSegErrs } = useTraversalSegmentValidation(computed(() => lineConfig.value.xStepSegments), computed(() => lineConfig.value.startX), computed(() => lineConfig.value.endX), tRef)
// line 模式已简化为仅 X 轴布点，不再有 Y 步进分段校验（lySegErrs 已移除）

function addSegment() { lineConfig.value.xStepSegments.push({ start: -30, end: 30, step: 5 }) }
function removeSegment(i: number) { lineConfig.value.xStepSegments.splice(i, 1) }
function addRectangleXSegment() { const s = rectangleConfig.value.xStepSegments; if (s.length === 0) s.push({ start: 0, end: 10, step: 5 }); else { const l = s[s.length - 1]; s.push({ start: l.end, end: l.end + 10, step: 5 }) } }
function removeRectangleXSegment(i: number) { rectangleConfig.value.xStepSegments.splice(i, 1) }
function addRectangleYSegment() { const s = rectangleConfig.value.yStepSegments; if (s.length === 0) s.push({ start: 0, end: 10, step: 5 }); else { const l = s[s.length - 1]; s.push({ start: l.end, end: l.end + 10, step: 5 }) } }
function removeRectangleYSegment(i: number) { rectangleConfig.value.yStepSegments.splice(i, 1) }
function addSectorRadialSegment() { const s = sectorConfig.value.radialStepSegments; if (s.length === 0) s.push({ start: 100, end: 200, step: 50 }); else { const l = s[s.length - 1]; s.push({ start: l.end, end: l.end + 100, step: 50 }) } }
function removeSectorRadialSegment(i: number) { sectorConfig.value.radialStepSegments.splice(i, 1) }
function addSectorAngularSegment() { const s = sectorConfig.value.angularStepSegments; if (s.length === 0) s.push({ start: 0, end: 10, step: 5 }); else { const l = s[s.length - 1]; s.push({ start: l.end, end: l.end + 10, step: 5 }) } }
function removeSectorAngularSegment(i: number) { sectorConfig.value.angularStepSegments.splice(i, 1) }
function addCustomPoint() {
  customPoints.value.push({
    x: customPointInput.value.x,
    y: customPointInput.value.y,
    z: customPointInput.value.z,
    u: customPointInput.value.u
  })
  customPointInput.value = { x: 0, y: 0, z: 0, u: 0 }
}
// removeCustomPoint 已迁入 CustomPointsTable.vue（支持批量删除/清空全部）

// 自定义点位文件导入流程：
// 1. 用户点击"导入点位"按钮 → 触发隐藏的 file input 点击
// 2. FileReader 读取文本 → parsePointsFile 解析为 ParsedPoint[]
// 3. 解析结果为空 → 通过 feedbackStore 推 warning toast，不替换
// 4. 已有 customPoints → 弹二次确认对话框（避免误覆盖）
// 5. 确认或无既有数据 → 替换 customPoints，重置 input value 允许重复选同一文件
const feedbackStore = useFeedbackStore()
const fileInputRef = ref<HTMLInputElement | null>(null)
const pendingImport = ref<ParsedPoint[] | null>(null)
const showImportConfirm = ref(false)

function triggerImportFile() {
  fileInputRef.value?.click()
}

function handleFileChange(event: Event) {
  const target = event.target as HTMLInputElement
  const file = target.files?.[0]
  if (!file) return
  const reader = new FileReader()
  reader.onload = () => {
    const text = String(reader.result ?? '')
    // 使用带范围 clamp + warnings 的解析器：
    // CSV 导入路径绕过 UI 输入框 min/max 约束（外部工具导出的值可能非法），
    // 由 parser 出口统一 clamp 到 [100,60000]/[1,1000] 并收集 warnings，
    // 通过 toast 反馈给用户，避免静默修正导致用户误以为原始值已生效。
    const { points: parsed, warnings } = parsePointsFileWithWarnings(text)
    if (warnings.length > 0) {
      // 仅展示首条 warning，避免上百点全部超界时 toast 列表爆炸
      // 完整列表可后续接入日志面板（当前无此渠道，先简化）
      feedbackStore.pushToast(warnings[0], 'warning')
    }
    if (parsed.length === 0) {
      // 无有效点位：用项目统一 toast 提示，避免原生 alert 阻塞主线程且风格割裂
      feedbackStore.pushToast(props.t.importParseEmptyHint, 'warning')
    } else if (customPoints.value.length > 0) {
      // 已有数据 → 二次确认
      pendingImport.value = parsed
      showImportConfirm.value = true
    } else {
      // 无既有数据 → 直接替换
      customPoints.value = parsed.map(toTraversalPoint)
    }
    // 重置 input value 让用户能重复选同一文件
    target.value = ''
  }
  reader.onerror = () => {
    // 读取失败：反馈具体原因（如权限不足/文件被锁），避免 UI 静默无反应
    const reason = reader.error?.message ?? String(reader.error ?? '')
    feedbackStore.pushToast(props.t.importReadFailed.replace('{reason}', reason), 'error')
    target.value = ''
  }
  reader.readAsText(file)
}

/**
 * ParsedPoint → TraversalPoint 单一映射函数
 *
 * 抽出此函数消除 handleFileChange / confirmImportReplace 两处重复的字段透传逻辑，
 * 后续若新增 per-point 字段（如压力阈值）只需改这一处，避免遗漏。
 * per-point 字段 undefined 表示"用全局默认"，与 TraversalPoint 类型对齐。
 */
function toTraversalPoint(p: ParsedPoint): TraversalPoint {
  return {
    x: p.x,
    y: p.y,
    z: p.z,
    u: p.u,
    dwellMs: p.dwellMs,
    samples: p.samples,
    test: p.test,
  }
}

function confirmImportReplace() {
  if (pendingImport.value) {
    customPoints.value = pendingImport.value.map(toTraversalPoint)
  }
  pendingImport.value = null
  showImportConfirm.value = false
}

function cancelImportReplace() {
  pendingImport.value = null
  showImportConfirm.value = false
}

// 导入文件格式说明弹窗：默认隐藏，点击"格式说明"按钮触发
// 用独立 ref 而非走 feedbackStore.confirm，因为这是说明性弹窗（无确认/取消二元选择）
const showFormatHelp = ref(false)
function openFormatHelp() {
  showFormatHelp.value = true
}
function closeFormatHelp() {
  showFormatHelp.value = false
}
</script>

<template>
  <div class="step-content">
    <UiPanel class="section-card">
      <div class="layout-basics">
        <div><span class="label-helper">{{ t.testNameLabel }}</span><UiInput v-model="testName" :placeholder="t.testNameLabel" /></div>
        <div><span class="label-helper">{{ t.dwellMsLabel }}</span><UiInputNumber v-model="dwellTimeMs" :min="100" :max="60000" class="w-full" /></div>
        <div><span class="label-helper">{{ t.samplesLabel }}</span><UiInputNumber v-model="samplesPerPoint" :min="1" :max="1000" class="w-full" /></div>
      </div>
    </UiPanel>

    <div class="flex gap-2">
      <UiButton v-for="p in (['line', 'rectangle', 'sector', 'custom'] as const)" :key="p" size="sm" :type="pattern === p ? 'primary' : 'default'" secondary @click="pattern = p">{{ getPatternLabel(p) }}</UiButton>
    </div>

    <!-- 遍历顺序面板：仅对支持蛇形/主轴的布点显示对应控件，避免无效勾选 -->
    <UiPanel v-if="supportsSnakeOrder || supportsPrimaryAxis" class="section-card">
      <!-- 蛇形扫描顺序：偶数行正向，奇数行反向，减少回程时间（仅 rectangle/sector） -->
      <label v-if="supportsSnakeOrder" class="option-label">
        <UiCheckbox :checked="snakeOrder" size="small" @update:checked="snakeOrder = $event" />
        <span>{{ t.travSnakeOrder || 'Snake scan order' }}</span>
      </label>
      <!-- 走线主轴：仅 rectangle 布局提供，控制物理走线方向 -->
      <div v-if="supportsPrimaryAxis" class="primary-axis-row">
        <span class="primary-axis-label">{{ t.travPrimaryAxis || 'Primary axis' }}</span>
        <div class="radio-group primary-axis-options">
          <label
            v-for="opt in primaryAxisOptions"
            :key="opt.value"
            class="radio-label"
            :class="{ active: primaryAxis === opt.value }"
          >
            <input v-model="primaryAxis" type="radio" :value="opt.value" />
            <span>{{ opt.label }}</span>
          </label>
        </div>
      </div>
    </UiPanel>
    <!-- line/custom 布点不支持遍历顺序：用灰字提示替代控件，避免界面"空着"让用户以为没加载完 -->
    <div v-else class="traversal-order-hint">
      {{ t.travOrderNotApplicable || '当前布点不支持遍历顺序设置' }}
    </div>

    <UiPanel v-if="pattern === 'line'" class="section-card">
      <div class="seg-grid">
        <div class="seg-side">
          <span class="section-title">{{ t.pointLayout }}</span>
          <!-- line 模式仅沿 X 轴布点，Y 恒为 0，不再提供 startY/endY 输入 -->
          <div class="seg-pts">
            <div><span class="label-tiny">{{ t.startX }}</span><UiInputNumber v-model="lineConfig.startX" class="w-full" /></div>
            <div><span class="label-tiny">{{ t.endX }}</span><UiInputNumber v-model="lineConfig.endX" class="w-full" /></div>
          </div>
        </div>
        <div class="seg-list">
          <div class="seg-header"><span class="seg-header-label">{{ t.xSegments }}</span><UiButton size="sm" secondary @click="addSegment">{{ t.addSegment }}</UiButton></div>
          <div class="seg-labels"><span class="col-label">{{ t.start }}</span><span class="col-label">{{ t.end }}</span><span class="col-label">{{ t.step }}</span><div class="w-40px"></div></div>
          <div v-for="(s, i) in lineConfig.xStepSegments" :key="i" class="seg-row">
            <UiInputNumber v-model="s.start" class="flex-1" />
            <UiInputNumber v-model="s.end" class="flex-1" />
            <UiInputNumber v-model="s.step" class="flex-1" />
            <UiButton size="sm" secondary :disabled="lineConfig.xStepSegments.length === 1" @click="removeSegment(i)">{{ t.del }}</UiButton>
          </div>
        </div>
      </div>
    </UiPanel>

    <UiPanel v-else-if="pattern === 'rectangle'" class="section-card">
      <span class="section-title-block">{{ t.pointLayout }} (X: {{ computedRectangleRange.xMin }}..{{ computedRectangleRange.xMax }}, Y: {{ computedRectangleRange.yMin }}..{{ computedRectangleRange.yMax }})</span>
      <span v-if="rectangleRangeError" class="rectangle-range-error">{{ rectangleRangeError }}</span>
      <div class="seg-col-list">
        <div class="seg-list"><div class="seg-header"><span class="seg-header-label">X {{ t.xSegments }}</span><UiButton size="sm" secondary @click="addRectangleXSegment">{{ t.addSegment }}</UiButton></div>
          <div class="seg-labels"><span class="col-label">{{ t.start }}</span><span class="col-label">{{ t.end }}</span><span class="col-label">{{ t.step }}</span><div class="w-40px"></div></div>
          <div v-for="(s, i) in rectangleConfig.xStepSegments" :key="i" class="seg-row"><UiInputNumber v-model="s.start" class="flex-1" /><UiInputNumber v-model="s.end" class="flex-1" /><UiInputNumber v-model="s.step" class="flex-1" /><UiButton size="sm" secondary :disabled="rectangleConfig.xStepSegments.length === 1" @click="removeRectangleXSegment(i)">{{ t.del }}</UiButton></div>
        </div>
        <div class="seg-list"><div class="seg-header"><span class="seg-header-label">Y {{ t.ySegments }}</span><UiButton size="sm" secondary @click="addRectangleYSegment">{{ t.addSegment }}</UiButton></div>
          <div class="seg-labels"><span class="col-label">{{ t.start }}</span><span class="col-label">{{ t.end }}</span><span class="col-label">{{ t.step }}</span><div class="w-40px"></div></div>
          <div v-for="(s, i) in rectangleConfig.yStepSegments" :key="i" class="seg-row"><UiInputNumber v-model="s.start" class="flex-1" /><UiInputNumber v-model="s.end" class="flex-1" /><UiInputNumber v-model="s.step" class="flex-1" /><UiButton size="sm" secondary :disabled="rectangleConfig.yStepSegments.length === 1" @click="removeRectangleYSegment(i)">{{ t.del }}</UiButton></div>
        </div>
      </div>
    </UiPanel>

    <UiPanel v-else-if="pattern === 'sector'" class="section-card">
      <div class="seg-grid">
        <div class="seg-side">
          <span class="section-title">{{ t.pointLayout }}</span>
          <!-- 半径范围:圆心不输入,默认第一个测点=当前位置=原点(0,0)。 -->
          <div class="sector-group">
            <span class="sector-group-title">{{ t.sectorRadiusRange }}</span>
            <div class="seg-pts">
              <div><span class="label-tiny">{{ t.radiusMin }}</span><UiInputNumber v-model="sectorConfig.radiusMin" :min="0" class="w-full" /></div>
              <div><span class="label-tiny">{{ t.radiusMax }}</span><UiInputNumber v-model="sectorConfig.radiusMax" :min="0" class="w-full" /></div>
            </div>
          </div>
          <!-- 角度范围:0°=+X 方向,逆时针为正。 -->
          <div class="sector-group">
            <span class="sector-group-title">{{ t.sectorAngleRange }}</span>
            <div class="seg-pts">
              <div><span class="label-tiny">{{ t.angleStart }}</span><UiInputNumber v-model="sectorConfig.angleStart" :step="5" class="w-full" /></div>
              <div><span class="label-tiny">{{ t.angleEnd }}</span><UiInputNumber v-model="sectorConfig.angleEnd" :step="5" class="w-full" /></div>
            </div>
          </div>
        </div>
        <div class="seg-col-list">
          <div class="seg-list"><div class="seg-header"><span class="seg-header-label">{{ t.radiusSegments }}</span><UiButton size="sm" secondary @click="addSectorRadialSegment">{{ t.addSegment }}</UiButton></div>
            <div class="seg-labels"><span class="col-label">{{ t.start }}</span><span class="col-label">{{ t.end }}</span><span class="col-label">{{ t.step }}</span><div class="w-40px"></div></div>
            <div v-for="(s, i) in sectorConfig.radialStepSegments" :key="i" class="seg-row">
              <div class="seg-cell"><UiInputNumber v-model="s.start" class="flex-1" /><span v-if="getSegmentError(srSegErrs, i, 'start')" class="seg-err">{{ getSegmentError(srSegErrs, i, 'start') }}</span></div>
              <div class="seg-cell"><UiInputNumber v-model="s.end" class="flex-1" /><span v-if="getSegmentError(srSegErrs, i, 'end')" class="seg-err">{{ getSegmentError(srSegErrs, i, 'end') }}</span></div>
              <div class="seg-cell"><UiInputNumber v-model="s.step" class="flex-1" /><span v-if="getSegmentError(srSegErrs, i, 'step')" class="seg-err">{{ getSegmentError(srSegErrs, i, 'step') }}</span></div>
              <UiButton size="sm" secondary :disabled="sectorConfig.radialStepSegments.length === 1" :title="sectorConfig.radialStepSegments.length === 1 ? (t.atLeastOneSegment || 'At least one segment required') : ''" @click="removeSectorRadialSegment(i)">{{ t.del }}</UiButton>
            </div>
          </div>
          <div class="seg-list"><div class="seg-header"><span class="seg-header-label">{{ t.angleSegments }}</span><UiButton size="sm" secondary @click="addSectorAngularSegment">{{ t.addSegment }}</UiButton></div>
            <div class="seg-labels"><span class="col-label">{{ t.start }}</span><span class="col-label">{{ t.end }}</span><span class="col-label">{{ t.step }}</span><div class="w-40px"></div></div>
            <div v-for="(s, i) in sectorConfig.angularStepSegments" :key="i" class="seg-row">
              <div class="seg-cell"><UiInputNumber v-model="s.start" class="flex-1" /><span v-if="getSegmentError(saSegErrs, i, 'start')" class="seg-err">{{ getSegmentError(saSegErrs, i, 'start') }}</span></div>
              <div class="seg-cell"><UiInputNumber v-model="s.end" class="flex-1" /><span v-if="getSegmentError(saSegErrs, i, 'end')" class="seg-err">{{ getSegmentError(saSegErrs, i, 'end') }}</span></div>
              <div class="seg-cell"><UiInputNumber v-model="s.step" class="flex-1" /><span v-if="getSegmentError(saSegErrs, i, 'step')" class="seg-err">{{ getSegmentError(saSegErrs, i, 'step') }}</span></div>
              <UiButton size="sm" secondary :disabled="sectorConfig.angularStepSegments.length === 1" :title="sectorConfig.angularStepSegments.length === 1 ? (t.atLeastOneSegment || 'At least one segment required') : ''" @click="removeSectorAngularSegment(i)">{{ t.del }}</UiButton>
            </div>
          </div>
        </div>
        <div class="sector-start-hint" role="note">
          <span class="sector-start-hint-icon" aria-hidden="true">ⓘ</span>
          <span>{{ t.sectorStartHint }}</span>
        </div>
        <!-- 轴绑定不合规告警：扇形要求 X=平移台（直线轴）、Y=旋转台（旋转轴），
             引导用户在下方运动轴配置区修正；实际阻断在 TraversalSettings.vue isStepValid -->
        <div v-if="sectorAxisInvalid" class="sector-axis-invalid-hint" role="alert">
          <span class="sector-axis-invalid-hint-icon" aria-hidden="true">⚠</span>
          <span>{{ t.travSectorAxisInvalidHint }}</span>
        </div>
      </div>
    </UiPanel>

    <UiPanel v-else class="section-card">
      <span class="section-title-block">{{ t.pointLayout }}</span>
      <!-- 自定义布点录入区：分两组——左侧 4 轴录入 + 添加点（数据写入动作），
           右侧导入点位 + 格式说明（文件操作动作）。组间用分隔符拉开视觉间距，
           避免按钮全挤一行让用户误点。4 轴 label 与 input 同行（inline-flex）
           压缩高度，原 label 上 input 下占两行高度浪费空间。 -->
      <div class="custom-input-row">
        <div class="custom-input-group">
          <label class="axis-input">
            <span class="axis-input-label">X</span>
            <UiInputNumber v-model="customPointInput.x" class="w-80px" />
          </label>
          <label class="axis-input">
            <span class="axis-input-label">Y</span>
            <UiInputNumber v-model="customPointInput.y" class="w-80px" />
          </label>
          <label class="axis-input">
            <span class="axis-input-label">Z</span>
            <UiInputNumber v-model="customPointInput.z" class="w-80px" />
          </label>
          <label class="axis-input">
            <span class="axis-input-label">U</span>
            <UiInputNumber v-model="customPointInput.u" class="w-80px" />
          </label>
          <UiButton size="sm" variant="primary" @click="addCustomPoint">{{ t.addPoint }}</UiButton>
        </div>
        <div class="custom-input-divider" aria-hidden="true"></div>
        <div class="custom-input-group">
          <!-- 文件导入：仅 custom 模式可见，支持 TXT/CSV，复用 pointsFileParser -->
          <UiButton size="sm" variant="secondary" @click="triggerImportFile">{{ t.importButton }}</UiButton>
          <UiButton size="sm" variant="ghost" @click="openFormatHelp">{{ t.customPointsFormatHelp }}</UiButton>
          <input
            ref="fileInputRef"
            type="file"
            accept=".txt,.csv,text/plain,text/csv"
            style="display: none"
            @change="handleFileChange"
          />
        </div>
      </div>
      <CustomPointsTable v-model="customPoints" />
    </UiPanel>

    <!-- 运动轴配置：X/Y/Z/U 方向 → 控制器 + 物理轴绑定。自 Hardware 步骤迁至本步，
         使扇形轴类型约束（X=直线轴/平移台，Y=旋转轴/旋转台）与布点选择同屏可见 -->
    <UiPanel class="section-card">
      <div class="hw-head">
        <span class="hdr-w80">{{ t.travTarget }}</span>
        <span class="hdr-name">{{ t.motionControllerLabel }}</span>
        <span class="hdr-w80">{{ t.physicalAxis }}</span>
      </div>
      <div v-for="ax in displayedMotionAxes" :key="ax.name" class="hw-row">
        <span class="axis-name">{{ getDirectionLabel(ax.name) }}</span>
        <UiSelect v-model="ax.controllerId" :options="motionStore.profiles.map(c => ({ label: c.name, value: c.id }))" :placeholder="t.selectController" class="sel-flex" />
        <UiSelect
          v-model="ax.axis"
          :options="axisOptionsFor(ax)"
          class="sel-w80"
          :class="{ 'sel-axis-error': axisKindIssueTargets.has(ax.name) || isAxisBindingDuplicated(ax) }"
          :title="isAxisBindingDuplicated(ax) ? t.travAxisBindingDuplicate : (axisKindIssueTargets.has(ax.name) ? t.travAxisKindMismatch : undefined)"
        />
      </div>
      <!-- 同控制器同物理轴冲突提示：X→ctrlA.X + Z→ctrlA.X 时后到目标覆盖先到目标，
           另一方向静默丢失。必须 UI 阻断，避免运行时静默 bug -->
      <div v-if="hasDuplicateAxisBinding" class="axis-binding-duplicate-hint" role="alert">
        <span class="axis-binding-duplicate-hint-icon" aria-hidden="true">⚠</span>
        <span>{{ t.travAxisBindingDuplicate }}</span>
      </div>
      <!-- 扇形布点轴类型约束说明：仅扇形布局显示 -->
      <div v-if="showSectorAxisHint" class="axis-kind-hint" role="note">
        <span class="axis-kind-hint-icon" aria-hidden="true">ⓘ</span>
        <span>{{ t.travSectorAxisKindHint }}</span>
      </div>
    </UiPanel>

    <!-- 导入二次确认：用户已存在 customPoints 时弹此对话框，避免误覆盖 -->
    <UiDialog
      :show="showImportConfirm"
      :title="t.importConfirmTitle"
      width="420px"
      :closable="true"
      @update:show="(v) => !v && cancelImportReplace()"
    >
      <p class="import-confirm-body">{{ t.importConfirmBody }}</p>
      <template #footer>
        <div class="import-confirm-actions">
          <UiButton quaternary size="sm" @click="cancelImportReplace">{{ t.cancel }}</UiButton>
          <UiButton variant="primary" size="sm" @click="confirmImportReplace">{{ t.importConfirmTitle }}</UiButton>
        </div>
      </template>
    </UiDialog>

    <!-- 导入文件格式说明：展示 TXT/CSV 格式范例，让用户知道如何准备文件。
         body 文本含 \n 换行，用 white-space: pre-line 渲染保留换行结构 -->
    <UiDialog
      :show="showFormatHelp"
      :title="t.customPointsFormatHelpTitle"
      width="560px"
      :closable="true"
      @update:show="(v) => !v && closeFormatHelp()"
    >
      <pre class="format-help-body">{{ t.customPointsFormatHelpBody }}</pre>
      <template #footer>
        <div class="import-confirm-actions">
          <UiButton variant="primary" size="sm" @click="closeFormatHelp">{{ t.close }}</UiButton>
        </div>
      </template>
    </UiDialog>
  </div>
</template>

<style scoped>
.step-content { display:flex; flex-direction:column; gap:8px; }
.section-card { font-size:12px; }

/* 紧凑化 UiPanel 内边距：默认 var(--space-3) var(--space-4) 偏大，
   覆盖为 6px 10px 让基础信息/蛇形扫描/点位布局卡片更紧凑、与硬件步骤视觉对齐 */
.section-card :deep(.n-card__content) { padding: 6px 10px }
.section-card :deep(.n-card-header) { padding: 6px 10px }

/* 基础信息 3 列网格：minmax(0, 1fr) 防止标签撑开列宽导致换行变纵向文字 */
.layout-basics { display:grid; grid-template-columns:repeat(3, minmax(0, 1fr)); gap:8px; }
/* 字段容器：min-width:0 防止内容撑开父列；flex-column 与三孔画面 .field 结构对齐 */
.layout-basics > div { display:flex; flex-direction:column; gap:2px; min-width:0; }
.seg-grid { display:grid; grid-template-columns:180px minmax(0, 1fr); gap:8px; }
.seg-side { padding:6px; border-radius:4px; border:1px solid var(--border-default); background:var(--bg-panel-strong); }
/* 点位 2 列网格：minmax(0, 1fr) 防止 startX/startY 等短标签撑开导致换行 */
.seg-pts { display:grid; grid-template-columns:repeat(2, minmax(0, 1fr)); gap:4px; margin-top:4px; }
.seg-pts > div { display:flex; flex-direction:column; gap:2px; min-width:0; }
.seg-list { padding:6px; border-radius:4px; border:1px solid var(--border-default); background:var(--bg-panel-strong); }
.seg-col-list { display:flex; flex-direction:column; gap:6px; }
.seg-header { display:flex; align-items:center; justify-content:space-between; margin-bottom:4px; }
.seg-labels { display:flex; gap:6px; padding:0 2px 4px; }
.seg-row { display:flex; gap:6px; align-items:flex-start; margin-bottom:4px; }
.seg-cell { display:flex; flex-direction:column; flex:1; min-width:0; }
.seg-err { font-size:var(--font-size-micro); color:var(--accent-error, #ef4444); margin-top:2px; line-height:1.2; }
.rectangle-range-error { display:block; margin:-2px 0 6px; font-size:var(--font-size-2xs); color:var(--accent-error, #ef4444); }

/* 字段标签：white-space:nowrap 防止换行变纵向文字；字号统一用 token 替代 9px/10px 硬编码
   - var(--font-size-micro) 10px：紧凑字段标签（label-helper/label-tiny/col-label）
   - var(--font-size-2xs) 11px：区块标题（section-title/sector-group-title 等） */
.label-helper { font-size: var(--font-size-micro); color: var(--text-muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.label-tiny { font-size: var(--font-size-micro); color: var(--text-muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.section-title { font-size: var(--font-size-2xs); font-weight: 500; color: var(--text-muted); white-space: nowrap; }
.sector-group { margin-top:6px; }
.sector-group-title { font-size: var(--font-size-2xs); font-weight: 500; display:block; margin-bottom:2px; color: var(--text-secondary, var(--text-primary)); white-space: nowrap; }
.sector-start-hint {
  grid-column: 1 / -1;
  display: flex;
  align-items: flex-start;
  gap: var(--density-field-gap);
  padding-top: var(--density-group-title-gap);
  border-top: 1px solid var(--border-default);
  font-size: var(--font-size-2xs);
  color: var(--text-muted);
  line-height: 1.5;
}
.sector-start-hint-icon { flex: 0 0 auto; color: var(--text-secondary, var(--text-primary)); }
/* 扇形轴绑定不合规告警：与 .sector-start-hint 同版式，换用警示色突出 */
.sector-axis-invalid-hint {
  grid-column: 1 / -1;
  display: flex;
  align-items: flex-start;
  gap: var(--density-field-gap);
  padding-top: var(--density-group-title-gap);
  font-size: var(--font-size-2xs);
  color: var(--color-warning);
  line-height: 1.5;
}
.sector-axis-invalid-hint-icon { flex: 0 0 auto; color: var(--color-warning); }

/* 运动轴配置区（自 TraversalHardwareStep.vue 迁入）：表头/行与通道表同款紧凑版式 */
.hw-head {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding-bottom: 3px;
  border-bottom: 1px solid var(--border-default)
}
.hw-row {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: 2px 0
}
.hw-row:hover { background: var(--bg-panel-strong); border-radius: var(--radius-md) }
.hdr-name { font-size: var(--font-size-micro); flex: 1; color: var(--text-muted) }
.hdr-w80 { font-size: var(--font-size-micro); width: 80px; color: var(--text-muted) }
.axis-name { font-size: var(--font-size-xs); font-weight: 600; width: 80px; color: var(--text-primary) }
.sel-w80 { width: 80px }
.sel-flex { flex: 1 }

/* 扇形轴类型不匹配错误：选择器红框 + hover title 说明（同 Hardware 步骤通道号重复视觉），
   实际阻断在 TraversalSettings.vue 的 isStepValid 中实施 */
.sel-axis-error :deep(.n-base-selection) {
  border-color: var(--state-error) !important;
  box-shadow: 0 0 0 2px var(--chart-band-danger);
}

/* 扇形布点轴类型约束说明：与 .sector-start-hint 视觉一致 */
.axis-kind-hint {
  display: flex;
  align-items: flex-start;
  gap: var(--density-field-gap);
  padding-top: var(--density-group-title-gap);
  font-size: var(--font-size-2xs);
  line-height: 1.5;
  color: var(--text-muted);
}
.axis-kind-hint-icon { flex: 0 0 auto; color: var(--text-secondary); }

/* 同控制器同物理轴冲突提示：用 warning 色（橙）区分于扇形轴类型不匹配的 error（红），
   让用户一眼分辨"配置冲突可改"与"轴类型错"。阻断在 TraversalSettings.vue isStepValid */
.axis-binding-duplicate-hint {
  display: flex;
  align-items: flex-start;
  gap: var(--density-field-gap);
  padding-top: var(--density-group-title-gap);
  font-size: var(--font-size-2xs);
  line-height: 1.5;
  color: var(--state-warning);
}
.axis-binding-duplicate-hint-icon { flex: 0 0 auto; }
.seg-header-label { font-size: var(--font-size-2xs); text-transform: uppercase; color: var(--text-muted); white-space: nowrap; }
.col-label { font-size: var(--font-size-micro); flex: 1; color: var(--text-muted); white-space: nowrap; }
.flex-1 { flex: 1; min-width: 0 }
.w-full { width: 100% }
.w-40px { width: 40px }
.w-80px { width: 80px }
.section-title-block { font-size: var(--font-size-2xs); font-weight: 500; display: block; margin-bottom: 6px; color: var(--text-muted); }
.option-label { display:flex; align-items:center; gap:6px; font-size:var(--text-sm); color:var(--text-primary); cursor:pointer }

/* line/custom 布点遍历顺序占位提示：用 --text-muted 灰字 + 内边距与 UiPanel 视觉对齐，
   让用户明确"此处无可用配置"而非界面未加载 */
.traversal-order-hint {
  font-size: var(--text-sm);
  color: var(--text-muted);
  padding: 6px 10px;
  line-height: 1.4;
}

/* 走线主轴选择行：与蛇形扫描同面板，水平排列标签与单选按钮 */
.primary-axis-row {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  margin-top: 6px;
  padding-top: 6px;
  border-top: 1px dashed var(--border-default)
}
.primary-axis-label { font-size: var(--text-sm); color: var(--text-primary); white-space: nowrap; }
.primary-axis-options { display: flex; gap: var(--space-2); margin-top: 0 }

/* 复用项目既有 radio 视觉规范（与 TraversalHardwareStep.vue 的 .radio-label 一致）：
   带 padding/border/hover/active 高亮，避免同项目同名 class 视觉割裂 */
.radio-label {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px;
  font-size: var(--text-sm);
  color: var(--text-primary);
  cursor: pointer;
  border-radius: var(--radius-md);
  border: 1px solid var(--border-default);
  min-height: 28px;
  white-space: nowrap;
}
.radio-label input[type="radio"] { margin: 0 }
.radio-label:hover { background: var(--bg-panel-strong) }
.radio-label.active {
  border-color: var(--color-primary);
  background: var(--color-primary-light, rgba(59, 130, 246, 0.1));
  color: var(--color-primary)
}

/* 导入二次确认弹窗：复用 TraversalStartConfirm 视觉规范 */
.import-confirm-body {
  font-size: var(--text-sm);
  color: var(--text-muted);
  line-height: 1.5;
  margin: 0;
}
.import-confirm-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: var(--space-2);
}

/* 自定义布点录入区：两组（录入 / 文件）水平排列，组间用细分隔符拉开间距 */
.custom-input-row {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}
.custom-input-group {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
/* 4 轴输入：label 与 input 同行（inline-flex），压缩高度，
   原	label 上 input 下占两行高度浪费空间 */
.axis-input {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
.axis-input-label {
  font-size: var(--font-size-micro);
  color: var(--text-muted);
  white-space: nowrap;
  min-width: 10px;
}
/* 组间分隔符：1px 宽竖线，高度与输入框对齐，比单纯加 gap 更明确"分组"语义 */
.custom-input-divider {
  width: 1px;
  height: 24px;
  background: var(--border-default);
  flex-shrink: 0;
}

/* 格式说明弹窗正文：等宽字体 + 保留换行 + 自动横向滚动，
   让 CSV/TSV 示例对齐易读 */
.format-help-body {
  font-family: var(--font-family-mono, 'Consolas', 'Monaco', monospace);
  font-size: var(--font-size-xs);
  color: var(--text-primary);
  line-height: 1.6;
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  background: var(--bg-panel-strong);
  padding: 10px 12px;
  border-radius: var(--radius-md);
  border: 1px solid var(--border-default);
  max-height: 50vh;
  overflow-y: auto;
}
</style>
