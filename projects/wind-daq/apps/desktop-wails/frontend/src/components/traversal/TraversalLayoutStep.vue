<script setup lang="ts">
import { computed, ref } from 'vue'
import type { StepSegment, TraversalPattern, TraversalPrimaryAxis } from '@shared/types/traversal'
import { useTraversalSegmentValidation, getSegmentError } from '@composables/useTraversalValidation'
// 自定义点位文件导入：前端纯解析，不依赖后端文件 IO
import { parsePointsFile, type ParsedPoint } from '@shared/pointsFileParser'
import { useFeedbackStore } from '@stores/feedbackStore'
import UiButton from '@components/ui/UiButton.vue'
import UiDialog from '@components/ui/UiDialog.vue'
import UiPanel from '@components/ui/UiPanel.vue'
import UiInput from '@components/ui/UiInput.vue'
import UiInputNumber from '@components/ui/UiInputNumber.vue'
import UiCheckbox from '@components/ui/UiCheckbox.vue'

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
const customPoints = defineModel<Array<{ x: number; y: number; z: number; u: number }>>('customPoints', { required: true })
const customPointInput = defineModel<{ x: number; y: number; z: number; u: number }>('customPointInput', { required: true })
const snakeOrder = defineModel<boolean>('snakeOrder', { required: true })
// 走线主轴：仅 line / rectangle 布局消费，扇形不显示此选项。
// 用 default: 'x' 而非 required，避免未来其他父组件漏传时 dev 告警 + radio 无选中项。
const primaryAxis = defineModel<TraversalPrimaryAxis>('primaryAxis', { default: 'x' })

const props = defineProps<{
  estimatedPointCount: number
  t: Record<string, string>
}>()

// 走线主轴选项：与 TraversalHardwareStep.vue 的 radio 模式对齐（v-model + options 数组 + active 高亮）
const primaryAxisOptions = computed(() => [
  { value: 'x' as const, label: props.t.travPrimaryAxisX || 'X first' },
  { value: 'y' as const, label: props.t.travPrimaryAxisY || 'Y first' }
])

// 仅 line / rectangle 布局消费走线主轴；扇形/自定义不显示此选项。
// 抽成 computed 避免模板里 `pattern === 'line' || pattern === 'rectangle'` 重复。
const supportsPrimaryAxis = computed(() => pattern.value === 'line' || pattern.value === 'rectangle')

const computedRectangleRange = computed(() => {
  const xs = rectangleConfig.value.xStepSegments
  const ys = rectangleConfig.value.yStepSegments
  const xMin = xs.length > 0 ? Math.min(...xs.map(s => s.start)) : 0
  const xMax = xs.length > 0 ? Math.max(...xs.map(s => s.end)) : 0
  const yMin = ys.length > 0 ? Math.min(...ys.map(s => s.start)) : 0
  const yMax = ys.length > 0 ? Math.max(...ys.map(s => s.end)) : 0
  rectangleConfig.value.xMin = xMin; rectangleConfig.value.xMax = xMax
  rectangleConfig.value.yMin = yMin; rectangleConfig.value.yMax = yMax
  return { xMin, xMax, yMin, yMax }
})

const computedSectorRange = computed(() => {
  const rs = sectorConfig.value.radialStepSegments
  const as = sectorConfig.value.angularStepSegments
  const radiusMin = rs.length > 0 ? Math.min(...rs.map(s => s.start)) : 0
  const radiusMax = rs.length > 0 ? Math.max(...rs.map(s => s.end)) : 0
  const angleStart = as.length > 0 ? Math.min(...as.map(s => s.start)) : 0
  const angleEnd = as.length > 0 ? Math.max(...as.map(s => s.end)) : 0
  sectorConfig.value.radiusMin = radiusMin; sectorConfig.value.radiusMax = radiusMax
  sectorConfig.value.angleStart = angleStart; sectorConfig.value.angleEnd = angleEnd
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
function removeCustomPoint(i: number) { customPoints.value.splice(i, 1) }

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
    const parsed = parsePointsFile(text)
    if (parsed.length === 0) {
      // 无有效点位：用项目统一 toast 提示，避免原生 alert 阻塞主线程且风格割裂
      feedbackStore.pushToast(props.t.importParseEmptyHint, 'warning')
    } else if (customPoints.value.length > 0) {
      // 已有数据 → 二次确认
      pendingImport.value = parsed
      showImportConfirm.value = true
    } else {
      // 无既有数据 → 直接替换
      customPoints.value = parsed.map((p) => ({ x: p.x, y: p.y, z: p.z, u: p.u }))
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

function confirmImportReplace() {
  if (pendingImport.value) {
    customPoints.value = pendingImport.value.map((p) => ({ x: p.x, y: p.y, z: p.z, u: p.u }))
  }
  pendingImport.value = null
  showImportConfirm.value = false
}

function cancelImportReplace() {
  pendingImport.value = null
  showImportConfirm.value = false
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

    <!-- 蛇形扫描顺序：偶数行正向，奇数行反向，减少回程时间 -->
    <UiPanel class="section-card">
      <label class="option-label">
        <UiCheckbox :checked="snakeOrder" size="small" @update:checked="snakeOrder = $event" />
        <span>{{ t.travSnakeOrder || 'Snake scan order' }}</span>
      </label>
      <!-- 走线主轴：仅 line / rectangle 布局提供，控制物理走线方向 -->
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
          <p class="sector-start-hint">ⓘ {{ t.sectorStartHint }}</p>
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
      </div>
    </UiPanel>

    <UiPanel v-else class="section-card">
      <span class="section-title-block">{{ t.pointLayout }}</span>
      <NSpace size="small" align="flex-end">
        <div><span class="label-tiny">X</span><UiInputNumber v-model="customPointInput.x" class="w-80px" /></div>
        <div><span class="label-tiny">Y</span><UiInputNumber v-model="customPointInput.y" class="w-80px" /></div>
        <div><span class="label-tiny">Z</span><UiInputNumber v-model="customPointInput.z" class="w-80px" /></div>
        <div><span class="label-tiny">U</span><UiInputNumber v-model="customPointInput.u" class="w-80px" /></div>
        <UiButton size="sm" variant="primary" @click="addCustomPoint">{{ t.addPoint }}</UiButton>
        <!-- 文件导入：仅 custom 模式可见，支持 TXT/CSV，复用 pointsFileParser -->
        <UiButton size="sm" secondary @click="triggerImportFile">{{ t.importButton }}</UiButton>
        <input
          ref="fileInputRef"
          type="file"
          accept=".txt,.csv,text/plain,text/csv"
          style="display: none"
          @change="handleFileChange"
        />
      </NSpace>
      <div v-if="customPoints.length > 0" class="pt-list">
        <div v-for="(pt, i) in customPoints" :key="i" class="pt-row">
          <span class="point-label">{{ t.point }} {{ i + 1 }}: {{ pt.x }}, {{ pt.y }}, {{ pt.z }}, {{ pt.u }}</span>
          <UiButton size="sm" secondary @click="removeCustomPoint(i)">{{ t.remove }}</UiButton>
        </div>
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
.pt-list { display:flex; flex-direction:column; gap:4px; margin-top:6px; }
.pt-row { display:flex; align-items:center; justify-content:space-between; padding:4px 6px; border-radius:4px; border:1px solid var(--border-default); background:var(--bg-panel-strong); }

/* 字段标签：white-space:nowrap 防止换行变纵向文字；字号统一用 token 替代 9px/10px 硬编码
   - var(--font-size-micro) 10px：紧凑字段标签（label-helper/label-tiny/col-label）
   - var(--font-size-2xs) 11px：区块标题（section-title/sector-group-title 等） */
.label-helper { font-size: var(--font-size-micro); color: var(--text-muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.label-tiny { font-size: var(--font-size-micro); color: var(--text-muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.section-title { font-size: var(--font-size-2xs); font-weight: 500; color: var(--text-muted); white-space: nowrap; }
.sector-group { margin-top:6px; }
.sector-group-title { font-size: var(--font-size-2xs); font-weight: 500; display:block; margin-bottom:2px; color: var(--text-secondary, var(--text-primary)); white-space: nowrap; }
.sector-start-hint { font-size: var(--font-size-2xs); color: var(--text-muted); margin: 8px 0 0; line-height: 1.4; }
.seg-header-label { font-size: var(--font-size-2xs); text-transform: uppercase; color: var(--text-muted); white-space: nowrap; }
.col-label { font-size: var(--font-size-micro); flex: 1; color: var(--text-muted); white-space: nowrap; }
.flex-1 { flex: 1; min-width: 0 }
.w-full { width: 100% }
.w-40px { width: 40px }
.w-80px { width: 80px }
.section-title-block { font-size: var(--font-size-2xs); font-weight: 500; display: block; margin-bottom: 6px; color: var(--text-muted) }
.point-label { font-size: var(--font-size-xs); color: var(--text-primary); white-space: nowrap; }
.option-label { display:flex; align-items:center; gap:6px; font-size:var(--text-sm); color:var(--text-primary); cursor:pointer }

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
</style>