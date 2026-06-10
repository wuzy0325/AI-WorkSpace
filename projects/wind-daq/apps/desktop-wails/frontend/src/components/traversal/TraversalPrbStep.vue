<script setup lang="ts">
import { computed, ref } from 'vue'
import { isWailsAvailable, wailsApi } from '@api/wails-adapter'
import { useFeedbackStore } from '@stores/feedbackStore'
import { useTraversalStore } from '@stores/traversalStore'
import type {
  CalibrationCsvFileInfo,
  MultiPrbInterpolationMode,
  PrbFileInfo,
  InterpolationAlgorithm
} from '@shared/types/traversal'
import UiButton from '@components/ui/UiButton.vue'
import UiInputNumber from '@components/ui/UiInputNumber.vue'
import UiPanel from '@components/ui/UiPanel.vue'
import UiSelect from '@components/ui/UiSelect.vue'
import UiStatusBadge from '@components/ui/UiStatusBadge.vue'

const prbMode = defineModel<'single' | 'multi'>('prbMode', { required: true })
const interpolationAlgorithm = defineModel<InterpolationAlgorithm>('interpolationAlgorithm', { required: true })
const prbFile = defineModel<PrbFileInfo | null>('prbFile', { required: true })
const multiPrbFiles = defineModel<PrbFileInfo[]>('multiPrbFiles', { required: true })
const multiPrbMachNumbers = defineModel<number[]>('multiPrbMachNumbers', { required: true })
const multiPrbInterpolationMode = defineModel<MultiPrbInterpolationMode>('multiPrbInterpolationMode', { required: true })
const calibrationCsvFile = defineModel<CalibrationCsvFileInfo | null>('calibrationCsvFile', { required: true })

const props = defineProps<{
  t: Record<string, string>
}>()

const traversalStore = useTraversalStore()
const feedbackStore = useFeedbackStore()

const isImportingPrb = ref(false)
const isImportingCsv = ref(false)

const prbValidRangeRows = computed(() => {
  if (!prbFile.value) return []
  return [
    { label: 'Alpha', value: `${prbFile.value.validRange.alphaMin} to ${prbFile.value.validRange.alphaMax} deg` },
    { label: 'Beta', value: `${prbFile.value.validRange.betaMin} to ${prbFile.value.validRange.betaMax} deg` },
    { label: 'Mach', value: `${prbFile.value.validRange.machMin} to ${prbFile.value.validRange.machMax}` }
  ]
})

const interpolationModeOptions = [
  { label: props.t.linearInterpolation || 'Linear', value: 'linear' },
  { label: props.t.nearestInterpolation || 'Nearest', value: 'nearest' },
]

function clonePrbFileInfo(file: PrbFileInfo): PrbFileInfo {
  return { ...file, validRange: { ...file.validRange } }
}

function normalizeMultiPrbMachNumbers(files: PrbFileInfo[], machNumbers: number[] = []): number[] {
  return files.map((file, index) => {
    const value = machNumbers[index] ?? file.machNumber ?? file.validRange.machMin
    return Number.isFinite(value) ? value : 0
  })
}

function setPrbMode(mode: 'single' | 'multi'): void { prbMode.value = mode }

function removeMultiPrbFile(index: number): void {
  multiPrbFiles.value.splice(index, 1)
  multiPrbMachNumbers.value.splice(index, 1)
}

function clearMultiPrbFiles(): void { multiPrbFiles.value = []; multiPrbMachNumbers.value = [] }

async function importPrbFile(): Promise<void> {
  try {
    if (isWailsAvailable()) {
      const filters = [{ displayName: 'PRB files', pattern: '*.prb' }]
      const paths = prbMode.value === 'multi'
        ? await wailsApi.app.pickFiles(props.t.importPrbs || 'Import PRB files', filters)
        : [await wailsApi.app.pickFile(props.t.importPrb || 'Import PRB file', filters)]
      const selectedPaths = paths.filter(Boolean)
      if (selectedPaths.length === 0) return
      isImportingPrb.value = true
      try {
        if (prbMode.value === 'multi') {
          const imported = await traversalStore.importMultiPrbFiles(selectedPaths, undefined, multiPrbInterpolationMode.value)
          if (imported) {
            multiPrbFiles.value = imported.files.map((f) => clonePrbFileInfo(f))
            multiPrbMachNumbers.value = normalizeMultiPrbMachNumbers(imported.files, imported.machNumbers)
            if (imported.warnings.length > 0) feedbackStore.pushToast(imported.warnings.join('\n'), 'warning')
          }
        } else {
          const imported = await traversalStore.importPrbFile(selectedPaths[0]!)
          if (imported) prbFile.value = clonePrbFileInfo(imported)
        }
      } finally { isImportingPrb.value = false }
      return
    }

    const input = document.createElement('input')
    input.type = 'file'; input.accept = '.prb'; input.multiple = prbMode.value === 'multi'
    input.onchange = async (event) => {
      const files = Array.from((event.target as HTMLInputElement).files ?? [])
      if (files.length === 0) return
      isImportingPrb.value = true
      try {
        if (prbMode.value === 'multi') {
          const imported = await traversalStore.importMultiPrbFiles(files.map((f) => (f as any).path ?? f.name), undefined, multiPrbInterpolationMode.value)
          if (imported) { multiPrbFiles.value = imported.files.map((f) => clonePrbFileInfo(f)); multiPrbMachNumbers.value = normalizeMultiPrbMachNumbers(imported.files, imported.machNumbers); if (imported.warnings.length > 0) feedbackStore.pushToast(imported.warnings.join('\n'), 'warning') }
          else feedbackStore.pushToast(props.t.failedImportPrb + ': ' + (traversalStore.error || 'Unknown error'), 'error')
        } else {
          const imported = await traversalStore.importPrbFile((files[0]! as any).path ?? files[0]!.name)
          if (imported) prbFile.value = clonePrbFileInfo(imported)
          else feedbackStore.pushToast(props.t.failedImportPrb + ': ' + (traversalStore.error || 'Unknown error'), 'error')
        }
      } finally { isImportingPrb.value = false }
    }
    input.click()
  } catch (err) {
    feedbackStore.pushToast(props.t.failedImportPrb + ': ' + (err instanceof Error ? err.message : String(err)), 'error')
  }
}

async function importCalibrationCsvFile(): Promise<void> {
  try {
    if (isWailsAvailable()) {
      const filePath = await wailsApi.app.pickFile(props.t.importCsv || 'Import CSV', [{ displayName: 'Calibration CSV', pattern: '*.csv;*.txt' }])
      if (!filePath) return
      isImportingCsv.value = true
      try { const imported = await traversalStore.importCalibrationCsvFile(filePath); if (imported) calibrationCsvFile.value = { ...imported } }
      finally { isImportingCsv.value = false }
      return
    }
    const input = document.createElement('input')
    input.type = 'file'; input.accept = '.csv,.txt'
    input.onchange = async (event) => {
      const files = Array.from((event.target as HTMLInputElement).files ?? [])
      if (files.length === 0) return
      isImportingCsv.value = true
      try {
        const imported = await traversalStore.importCalibrationCsvFile((files[0]! as any).path ?? files[0]!.name)
        if (imported) calibrationCsvFile.value = { ...imported }
        else feedbackStore.pushToast(props.t.failedImportCsv + ': ' + (traversalStore.error || 'Unknown error'), 'error')
      } finally { isImportingCsv.value = false }
    }
    input.click()
  } catch (err) {
    feedbackStore.pushToast(props.t.failedImportCsv + ': ' + (err instanceof Error ? err.message : String(err)), 'error')
  }
}
</script>

<template>
  <div class="step-content">
    <UiPanel class="section-card">
      <div class="mode-row">
        <div><span class="label-section">{{ t.prbMode }}</span><span class="hint-text">{{ t.prbModeHint }}</span></div>
        <div style="display:flex;align-items:center;gap:8px">
          <UiButton size="sm" :type="prbMode === 'single' ? 'primary' : 'default'" secondary @click="setPrbMode('single')">{{ t.singlePrbMode }}</UiButton>
          <UiButton size="sm" :type="prbMode === 'multi' ? 'primary' : 'default'" secondary @click="setPrbMode('multi')">{{ t.multiPrbMode }}</UiButton>
        </div>
      </div>
    </UiPanel>

    <UiPanel class="section-card">
      <div class="mode-row">
        <div><span class="label-section">{{ t.interpolationAlgorithm || 'Interpolation Algorithm' }}</span><span class="hint-text">{{ t.interpolationAlgorithmHint || 'Select interpolation algorithm' }}</span></div>
        <div style="display:flex;align-items:center;gap:8px">
          <UiButton size="sm" :type="interpolationAlgorithm === 'old' ? 'primary' : 'default'" secondary @click="interpolationAlgorithm = 'old'">{{ t.algorithmOld || 'Old' }}</UiButton>
          <UiButton size="sm" :type="interpolationAlgorithm === 'new' ? 'primary' : 'default'" secondary @click="interpolationAlgorithm = 'new'">{{ t.algorithmNew || 'New' }}</UiButton>
        </div>
      </div>
    </UiPanel>

    <UiPanel v-if="interpolationAlgorithm === 'new'" class="section-card">
      <div class="import-head">
        <div><span class="section-title">{{ t.csvImport || 'Import CSV Calibration Data' }}</span><span class="section-hint">{{ t.csvImportHint || 'Import calibration data in CSV format' }}</span></div>
        <UiButton size="sm" variant="primary" :loading="isImportingCsv" @click="importCalibrationCsvFile">{{ isImportingCsv ? (t.importing || 'Importing...') : (t.importCsv || 'Import CSV') }}</UiButton>
      </div>
      <template v-if="calibrationCsvFile">
        <div class="file-row"><div class="file-info"><span class="file-name">{{ calibrationCsvFile.fileName }}</span><span class="file-path">{{ calibrationCsvFile.filePath }}</span></div>        <UiButton size="sm" secondary @click="calibrationCsvFile = null">{{ t.remove }}</UiButton></div>
        <div class="range-grid">
          <div class="range-stat"><span class="range-label">Alpha</span><span class="range-value">{{ calibrationCsvFile.validRange.alphaMin }}..{{ calibrationCsvFile.validRange.alphaMax }} deg</span></div>
          <div class="range-stat"><span class="range-label">Beta</span><span class="range-value">{{ calibrationCsvFile.validRange.betaMin }}..{{ calibrationCsvFile.validRange.betaMax }} deg</span></div>
          <div class="range-stat"><span class="range-label">{{ t.pointCount || 'Points' }}</span><span class="range-value">{{ calibrationCsvFile.pointCount }}</span></div>
        </div>
      </template>
      <div v-else class="empty-state"><span class="empty-text">{{ t.noCsvImported || 'No CSV calibration data imported' }}</span></div>
    </UiPanel>

    <UiPanel v-if="interpolationAlgorithm === 'old'" class="section-card">
      <div class="import-head">
        <div><span class="section-title">{{ prbMode === 'multi' ? t.multiPrbImport : t.prbImport }}</span><span class="section-hint">{{ prbMode === 'multi' ? t.multiPrbImportHint : t.prbImportHint }}</span></div>
        <UiButton size="sm" variant="primary" :loading="isImportingPrb" @click="importPrbFile">{{ isImportingPrb ? t.importing : (prbMode === 'multi' ? t.importPrbs : t.importPrb) }}</UiButton>
      </div>

      <template v-if="prbMode === 'single'">
        <template v-if="prbFile">
          <div class="file-row"><div class="file-info"><span class="file-name">{{ prbFile.fileName }}</span><span class="file-path">{{ prbFile.filePath }}</span></div>        <UiButton size="sm" secondary @click="prbFile = null">{{ t.remove }}</UiButton></div>
          <div class="range-grid"><div v-for="r in prbValidRangeRows" :key="r.label" class="range-stat"><span class="range-label">{{ r.label }}</span><span class="range-value">{{ r.value }}</span></div></div>
        </template>
        <div v-else class="empty-state"><span class="empty-text">{{ t.noPrbImported }}</span></div>
      </template>

      <template v-else>
        <div class="multi-mode-summary">
          <div class="range-stat"><span class="range-label">{{ t.interpolationMode }}</span><UiSelect v-model="multiPrbInterpolationMode" :options="interpolationModeOptions" class="field-full" /></div>
          <div class="range-stat"><span class="range-label">{{ t.multiPrbFilesLabel }}</span><span class="file-count-num">{{ multiPrbFiles.length }}</span></div>
        </div>

        <template v-if="multiPrbFiles.length > 0">
          <div v-for="(file, i) in multiPrbFiles" :key="file.filePath" class="multi-file">
            <div class="file-row"><div class="file-info"><span class="file-name">{{ file.fileName }}</span><span class="file-path">{{ file.filePath }}</span></div>        <UiButton size="sm" secondary @click="removeMultiPrbFile(i)">{{ t.remove }}</UiButton></div>
            <div class="multi-mach-grid">
              <div><span class="compact-label">{{ t.fileMachNumber }}</span><UiInputNumber v-model="multiPrbMachNumbers[i]" :step="0.01" class="field-full" /></div>
              <div class="range-stat"><span class="compact-label-plain">Alpha</span><span class="compact-value">{{ file.validRange.alphaMin }}..{{ file.validRange.alphaMax }} deg</span></div>
              <div class="range-stat"><span class="compact-label-plain">Beta</span><span class="compact-value">{{ file.validRange.betaMin }}..{{ file.validRange.betaMax }} deg</span></div>
              <div class="range-stat"><span class="compact-label-plain">Mach</span><span class="compact-value">{{ file.validRange.machMin }}..{{ file.validRange.machMax }}</span></div>
            </div>
          </div>
          <div class="action-bar">          <UiButton size="sm" secondary @click="clearMultiPrbFiles">{{ t.clearAll }}</UiButton></div>
        </template>
        <div v-else class="empty-state"><span class="empty-text">{{ t.noMultiPrbImported }}</span></div>
      </template>
    </UiPanel>
  </div>
</template>

<style scoped>
.step-content { display:flex; flex-direction:column; gap:var(--space-3) }
.section-card { font-size:var(--text-sm) }
.mode-row { display:flex; align-items:center; justify-content:space-between; gap:var(--space-3) }
.import-head { display:flex; align-items:flex-start; justify-content:space-between; gap:var(--space-3); margin-bottom:var(--space-3) }
.file-row { display:flex; align-items:center; justify-content:space-between; gap:var(--space-2); padding:var(--space-2); border-radius:var(--radius-md); background:var(--bg-panel-strong); margin-bottom:var(--space-2) }
.file-info { min-width:0; flex:1 }
.range-grid { display:grid; grid-template-columns:repeat(3,1fr); gap:var(--space-2) }
.range-stat { padding:var(--space-2); border-radius:var(--radius-md); border:1px solid var(--border-default); background:var(--bg-panel-strong) }
.empty-state { display:flex; align-items:center; justify-content:center; height:100px; border-radius:var(--radius-md); border:1px dashed var(--border-default); background:var(--bg-panel-strong) }
.multi-mode-summary { display:grid; grid-template-columns:1fr 120px; gap:var(--space-2); margin-bottom:var(--space-2) }
.multi-file { padding:var(--space-2); border-radius:var(--radius-md); border:1px solid var(--border-default); background:var(--bg-panel-strong); margin-bottom:var(--space-2) }
.multi-mach-grid { display:grid; grid-template-columns:120px repeat(3,1fr); gap:var(--space-2); margin-top:6px }
.label-section { font-size:var(--text-xs);font-weight:600;text-transform:uppercase;letter-spacing:0.14em;color:var(--text-tertiary) }
.hint-text { font-size:var(--text-xs);margin-top:2px;display:block;color:var(--text-secondary) }
.section-title { font-size:var(--text-sm);font-weight:600 }
.section-hint { font-size:var(--text-xs);margin-top:var(--space-1);display:block;color:var(--text-secondary) }
.file-name { font-size:var(--text-sm);font-weight:600 }
.file-path { font-size:var(--text-xs);color:var(--text-tertiary) }
.range-label { font-size:10px;font-weight:600;text-transform:uppercase;color:var(--text-tertiary) }
.range-value { font-size:var(--text-sm) }
.empty-text { font-size:var(--text-sm);color:var(--text-tertiary) }
.compact-label { font-size:9px;font-weight:600;text-transform:uppercase;letter-spacing:0.14em;color:var(--text-tertiary) }
.compact-label-plain { font-size:9px;font-weight:600;text-transform:uppercase;color:var(--text-tertiary) }
.compact-value { font-size:var(--text-xs) }
.action-bar { display:flex;justify-content:flex-end }
.field-full { width:100%;margin-top:var(--space-1) }
.file-count-num { font-size:var(--text-base);font-weight:700;margin-top:var(--space-1);display:block }
</style>