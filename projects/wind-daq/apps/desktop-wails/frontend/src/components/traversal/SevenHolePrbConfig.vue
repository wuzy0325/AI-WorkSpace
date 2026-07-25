<script setup lang="ts">
import { computed, ref } from 'vue'
import { useFileImport } from '@composables/useFileImport'
import { useFeedbackStore } from '@stores/feedbackStore'
import { useTraversalStore } from '@stores/traversalStore'
import type { SevenHolePrbDraft, SevenHolePrbFileInfo } from '@shared/types/traversal'
import { assignSevenHoleCsvFilesByName, assignSevenHoleFilesByName, detectSevenHoleBatchFormat } from '@shared/types/traversal'
import UiButton from '@components/ui/UiButton.vue'
import UiPanel from '@components/ui/UiPanel.vue'

/**
 * 七孔 PRB 配置子组件（spec-seven-hole-traversal §6.3）：
 * 承载 1 个内区槽位（7.prb）+ 6 个扇区槽位（1.prb~6.prb，固定孔号顺序），
 * 7 份齐备后自动调用 store 导入动作并展示逐文件 pointCount 与 validRange；
 * 只接收七孔判别配置的编辑态 Draft，不读取五孔任何字段。
 */

const draft = defineModel<SevenHolePrbDraft>({ required: true })

const props = defineProps<{
  t: Record<string, string>
}>()

const traversalStore = useTraversalStore()
const feedbackStore = useFeedbackStore()

const fileImport = useFileImport({
  onError: (message) => feedbackStore.pushToast(props.t.travErrImportSevenHolePrb + ': ' + message, 'error')
})
const isBackendImporting = ref(false)
const isImporting = computed(() => fileImport.isImporting.value || isBackendImporting.value)

const isComplete = computed(
  () => draft.value.innerFile !== null && draft.value.outerFiles.every((f) => f !== null)
)

const isCsvSource = computed(() => draft.value.source === 'calibration-csv')

const validRangeText = ref<string | null>(null)

/** 数据源切换（PRB 文件集 / 校准 CSV）：清空旧格式槽位，避免混用两种解析入口。 */
function setSource(source: 'prb' | 'calibration-csv'): void {
  if (isImporting.value || source === draft.value.source) return
  draft.value = {
    source,
    innerFile: null,
    outerFiles: [null, null, null, null, null, null]
  }
  validRangeText.value = null
}

function fileNameOf(path: string): string {
  return path.split(/[\\/]/).pop() ?? path
}

/**
 * 批量导入：一次多选全部文件（.prb 与 .csv 均可见），按所选文件自动识别格式：
 * 全 .prb → PRB 编号分配（7.prb→内区，1~6.prb→扇区）；
 * 全 .csv → 校准 CSV 命名分配（小角度区→内区，大角度N区→扇区 N），并同步数据源切换。
 * 混选两种格式或无法命名的文件给出提示，对应槽位由用户手动补选。
 */
async function batchImport(): Promise<void> {
  const paths = await fileImport.importFiles({
    title: props.t.sevenHolePrbBatchImport,
    filters: [{ displayName: 'PRB / Calibration CSV files', pattern: '*.prb;*.csv' }],
    multiple: true
  })
  if (paths.length === 0) return
  const format = detectSevenHoleBatchFormat(paths)
  if (format === 'mixed' || format === 'empty') {
    feedbackStore.pushToast(props.t.sevenHolePrbMixedFormat, 'warning')
    return
  }
  // 此处 format 必为 'prb' | 'calibration-csv'
  const sourceChanged = format !== draft.value.source
  const assignment = format === 'calibration-csv'
    ? assignSevenHoleCsvFilesByName(paths)
    : assignSevenHoleFilesByName(paths)
  const outer = sourceChanged
    ? [null, null, null, null, null, null] as SevenHolePrbDraft['outerFiles']
    : [...draft.value.outerFiles]
  for (const [sector, info] of assignment.outerFiles) {
    outer[sector - 1] = info
  }
  const nextDraft: SevenHolePrbDraft = {
    source: format,
    innerFile: assignment.innerFile ?? (sourceChanged ? null : draft.value.innerFile),
    outerFiles: outer
  }
  draft.value = nextDraft
  if (sourceChanged) validRangeText.value = null
  if (assignment.unmatched.length > 0) {
    feedbackStore.pushToast(
      props.t.sevenHolePrbUnmatched.replace('{files}', assignment.unmatched.map((p) => fileNameOf(p)).join(', ')),
      'warning'
    )
  }
  await importIfComplete(nextDraft)
}

/** 按数据源选择文件过滤器与错误文案 */
function fileFilters(): { displayName: string; pattern: string }[] {
  return isCsvSource.value
    ? [{ displayName: 'Calibration CSV files', pattern: '*.csv' }]
    : [{ displayName: 'PRB files', pattern: '*.prb' }]
}

function importErrorKey(): string {
  return isCsvSource.value ? props.t.travErrImportSevenHoleCsv : props.t.travErrImportSevenHolePrb
}

/** 槽位选文件：仅记录路径与占位信息；齐备后自动触发后端导入 */
async function pickInner(): Promise<void> {
  const path = await fileImport.importSingleFile({
    title: props.t.sevenHolePrbInnerFile,
    filters: fileFilters()
  })
  if (!path) return
  const nextDraft: SevenHolePrbDraft = {
    ...draft.value,
    innerFile: { filePath: path, fileName: fileNameOf(path), sector: 7 }
  }
  draft.value = nextDraft
  await importIfComplete(nextDraft)
}

async function pickOuter(sector: number): Promise<void> {
  const path = await fileImport.importSingleFile({
    title: props.t.sevenHolePrbOuterFile.replace('{n}', String(sector)),
    filters: fileFilters()
  })
  if (!path) return
  const outer = [...draft.value.outerFiles]
  outer[sector - 1] = { filePath: path, fileName: fileNameOf(path), sector }
  const nextDraft: SevenHolePrbDraft = { ...draft.value, outerFiles: outer }
  draft.value = nextDraft
  await importIfComplete(nextDraft)
}

function removeInner(): void {
  draft.value = { ...draft.value, innerFile: null }
  validRangeText.value = null
}

function removeOuter(sector: number): void {
  const outer = [...draft.value.outerFiles]
  outer[sector - 1] = null
  draft.value = { ...draft.value, outerFiles: outer }
}

/** 7 份齐备后按数据源自动导入；失败保留槽位由用户修正 */
async function importIfComplete(candidate: SevenHolePrbDraft = draft.value): Promise<void> {
  if (candidate.innerFile === null || candidate.outerFiles.some((f) => f === null)) return
  const inner = candidate.innerFile
  const outer = candidate.outerFiles as SevenHolePrbFileInfo[]
  isBackendImporting.value = true
  let imported: Awaited<ReturnType<typeof traversalStore.importSevenHolePrbFiles>>
  try {
    imported = candidate.source === 'calibration-csv'
      ? await traversalStore.importSevenHoleCalibrationCsvFiles(inner.filePath, outer.map((f) => f.filePath))
      : await traversalStore.importSevenHolePrbFiles(inner.filePath, outer.map((f) => f.filePath))
  } finally {
    isBackendImporting.value = false
  }
  if (!imported) {
    feedbackStore.pushToast(
      importErrorKey() + ': ' + (traversalStore.error || props.t.unknownError),
      'error'
    )
    return
  }
  // 用服务端返回的逐文件信息（pointCount/loadedAt）回填槽位
  const innerRet = imported.files.find((f) => f.sector === 7)
  const nextInner: SevenHolePrbFileInfo = { ...inner, ...(innerRet ?? {}) }
  const nextOuter = outer.map((slot, i) => {
    const ret = imported.files.find((f) => f.sector === i + 1)
    return ret ? { ...slot, ...ret } : slot
  })
  draft.value = { ...candidate, innerFile: nextInner, outerFiles: nextOuter }
  const vr = imported.validRange
  validRangeText.value = `Alpha ${vr.alphaMin}..${vr.alphaMax} deg / Beta ${vr.betaMin}..${vr.betaMax} deg`
}
</script>

<template>
  <div class="step-content">
    <UiPanel class="section-card">
      <div class="mode-row">
        <div><span class="label-section">{{ t.sevenHolePrbSource }}</span><span class="hint-text">{{ isCsvSource ? t.sevenHolePrbSourceCsvHint : t.sevenHolePrbSourcePrbHint }}</span></div>
        <div style="display:flex;align-items:center;gap:8px">
          <UiButton size="sm" :type="!isCsvSource ? 'primary' : 'default'" secondary :disabled="isImporting" @click="setSource('prb')">{{ t.sevenHolePrbSourcePrb }}</UiButton>
          <UiButton size="sm" :type="isCsvSource ? 'primary' : 'default'" secondary :disabled="isImporting" @click="setSource('calibration-csv')">{{ t.sevenHolePrbSourceCsv }}</UiButton>
        </div>
      </div>
    </UiPanel>

    <UiPanel class="section-card">
      <div class="import-head">
        <div>
          <span class="section-title">{{ t.sevenHolePrbTitle }}</span>
          <span class="section-hint">{{ t.sevenHolePrbBatchHint }}</span>
        </div>
        <div class="head-actions">
          <UiButton size="sm" variant="primary" :loading="isImporting" :disabled="isImporting" @click="batchImport">
            {{ isImporting ? t.importing : t.sevenHolePrbBatchImport }}
          </UiButton>
        </div>
      </div>

      <!-- 内区槽位（7.prb，小角度区 169 点） -->
      <div class="file-row file-row--inner">
        <div class="slot-label">{{ t.sevenHolePrbInnerFile }}</div>
        <template v-if="draft.innerFile">
          <div class="file-info">
            <span class="file-name">{{ draft.innerFile.fileName }}</span>
            <span class="file-path">{{ draft.innerFile.filePath }}</span>
            <span v-if="draft.innerFile.pointCount" class="file-meta">{{ draft.innerFile.pointCount }} pts</span>
          </div>
          <UiButton size="sm" secondary :disabled="isImporting" @click="removeInner">{{ t.remove }}</UiButton>
        </template>
        <UiButton v-else size="sm" variant="primary" :disabled="isImporting" @click="pickInner">{{ t.importPrb }}</UiButton>
      </div>

      <!-- 扇区槽位（1.prb~6.prb，固定孔号顺序，点数动态 = thetaCount×13） -->
      <div v-for="sector in [1, 2, 3, 4, 5, 6]" :key="sector" class="file-row">
        <div class="slot-label">{{ t.sevenHolePrbOuterFile.replace('{n}', String(sector)) }}</div>
        <template v-if="draft.outerFiles[sector - 1]">
          <div class="file-info">
            <span class="file-name">{{ draft.outerFiles[sector - 1]!.fileName }}</span>
            <span class="file-path">{{ draft.outerFiles[sector - 1]!.filePath }}</span>
            <span v-if="draft.outerFiles[sector - 1]!.pointCount" class="file-meta">{{ draft.outerFiles[sector - 1]!.pointCount }} pts</span>
          </div>
          <UiButton size="sm" secondary :disabled="isImporting" @click="removeOuter(sector)">{{ t.remove }}</UiButton>
        </template>
        <UiButton v-else size="sm" variant="primary" :disabled="isImporting" @click="pickOuter(sector)">{{ t.importPrb }}</UiButton>
      </div>

      <div v-if="validRangeText" class="range-bar">
        <span class="range-label">validRange</span>
        <span class="range-value">{{ validRangeText }}</span>
      </div>
      <div v-else-if="!isComplete" class="empty-state">
        <span class="empty-text">{{ t.sevenHolePrbIncomplete }}</span>
      </div>
    </UiPanel>
  </div>
</template>

<style scoped>
.step-content { display:flex; flex-direction:column; gap:var(--space-2) }
.section-card { font-size:var(--text-sm) }
.mode-row { display:flex; align-items:center; justify-content:space-between; gap:var(--space-2) }
.label-section { font-size:var(--text-xs);font-weight:600;text-transform:uppercase;letter-spacing:0.14em;color:var(--text-tertiary) }
.hint-text { font-size:var(--text-xs);margin-top:2px;display:block;color:var(--text-secondary) }
.import-head { display:flex; align-items:flex-start; justify-content:space-between; gap:var(--space-2); margin-bottom:var(--space-2) }
.head-actions { display:flex; align-items:center; gap:var(--space-2) }
.section-title { font-size:var(--text-sm);font-weight:600 }
.section-hint { font-size:var(--text-xs);margin-top:var(--space-1);display:block;color:var(--text-secondary) }
.importing-text { font-size:var(--text-xs);color:var(--text-tertiary) }
.file-row { display:flex; align-items:center; justify-content:space-between; gap:var(--space-2); padding:var(--space-2); border-radius:var(--radius-md); background:var(--bg-panel-strong); margin-bottom:var(--space-2) }
.file-row--inner { border:1px solid var(--border-strong, var(--border-default)) }
.slot-label { font-size:var(--text-xs);font-weight:600;color:var(--text-secondary); min-width:180px }
.file-info { min-width:0; flex:1; display:flex; flex-direction:column }
.file-name { font-size:var(--text-sm);font-weight:600 }
.file-path { font-size:var(--text-xs);color:var(--text-tertiary); overflow:hidden; text-overflow:ellipsis; white-space:nowrap }
.file-meta { font-size:var(--text-xs);color:var(--text-secondary) }
.range-bar { display:flex; align-items:center; gap:var(--space-2); padding:var(--space-2); border-radius:var(--radius-md); border:1px solid var(--border-default); background:var(--bg-panel-strong) }
.range-label { font-size:10px;font-weight:600;text-transform:uppercase;color:var(--text-tertiary) }
.range-value { font-size:var(--text-sm) }
.empty-state { display:flex; align-items:center; justify-content:center; height:64px; border-radius:var(--radius-md); border:1px dashed var(--border-default); background:var(--bg-panel-strong) }
.empty-text { font-size:var(--text-sm);color:var(--text-tertiary) }
</style>
