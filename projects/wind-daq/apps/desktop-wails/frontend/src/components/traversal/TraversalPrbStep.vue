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
import {
  NButton,
  NCard,
  NInputNumber,
  NSelect,
  NTag,
  NText,
} from 'naive-ui'

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
    <NCard size="small" :bordered="true" class="section-card">
      <div class="mode-row">
        <div><NText depth="3" style="font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:0.14em">{{ t.prbMode }}</NText><NText depth="2" style="font-size:11px;margin-top:2px;display:block">{{ t.prbModeHint }}</NText></div>
        <NSpace size="small">
          <NButton size="tiny" :type="prbMode === 'single' ? 'primary' : 'default'" secondary @click="setPrbMode('single')">{{ t.singlePrbMode }}</NButton>
          <NButton size="tiny" :type="prbMode === 'multi' ? 'primary' : 'default'" secondary @click="setPrbMode('multi')">{{ t.multiPrbMode }}</NButton>
        </NSpace>
      </div>
    </NCard>

    <NCard size="small" :bordered="true" class="section-card">
      <div class="mode-row">
        <div><NText depth="3" style="font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:0.14em">{{ t.interpolationAlgorithm || 'Interpolation Algorithm' }}</NText><NText depth="2" style="font-size:11px;margin-top:2px;display:block">{{ t.interpolationAlgorithmHint || 'Select interpolation algorithm' }}</NText></div>
        <NSpace size="small">
          <NButton size="tiny" :type="interpolationAlgorithm === 'old' ? 'primary' : 'default'" secondary @click="interpolationAlgorithm = 'old'">{{ t.algorithmOld || 'Old' }}</NButton>
          <NButton size="tiny" :type="interpolationAlgorithm === 'new' ? 'primary' : 'default'" secondary @click="interpolationAlgorithm = 'new'">{{ t.algorithmNew || 'New' }}</NButton>
        </NSpace>
      </div>
    </NCard>

    <NCard v-if="interpolationAlgorithm === 'new'" size="small" :bordered="true" class="section-card">
      <div class="import-head">
        <div><NText depth="1" style="font-size:12px;font-weight:600">{{ t.csvImport || 'Import CSV Calibration Data' }}</NText><NText depth="2" style="font-size:11px;margin-top:1px;display:block">{{ t.csvImportHint || 'Import calibration data in CSV format' }}</NText></div>
        <NButton size="tiny" type="primary" :loading="isImportingCsv" @click="importCalibrationCsvFile">{{ isImportingCsv ? (t.importing || 'Importing...') : (t.importCsv || 'Import CSV') }}</NButton>
      </div>
      <template v-if="calibrationCsvFile">
        <div class="file-row"><div class="file-info"><NText depth="1" style="font-size:12px;font-weight:600;truncate">{{ calibrationCsvFile.fileName }}</NText><NText depth="3" style="font-size:11px;truncate">{{ calibrationCsvFile.filePath }}</NText></div><NButton size="tiny" secondary @click="calibrationCsvFile = null">{{ t.remove }}</NButton></div>
        <div class="range-grid">
          <div class="range-stat"><NText depth="3" style="font-size:10px;font-weight:600;text-transform:uppercase">Alpha</NText><NText depth="1" style="font-size:12px">{{ calibrationCsvFile.validRange.alphaMin }}..{{ calibrationCsvFile.validRange.alphaMax }} deg</NText></div>
          <div class="range-stat"><NText depth="3" style="font-size:10px;font-weight:600;text-transform:uppercase">Beta</NText><NText depth="1" style="font-size:12px">{{ calibrationCsvFile.validRange.betaMin }}..{{ calibrationCsvFile.validRange.betaMax }} deg</NText></div>
          <div class="range-stat"><NText depth="3" style="font-size:10px;font-weight:600;text-transform:uppercase">{{ t.pointCount || 'Points' }}</NText><NText depth="1" style="font-size:12px">{{ calibrationCsvFile.pointCount }}</NText></div>
        </div>
      </template>
      <div v-else class="empty-state"><NText depth="3" style="font-size:12px">{{ t.noCsvImported || 'No CSV calibration data imported' }}</NText></div>
    </NCard>

    <NCard v-if="interpolationAlgorithm === 'old'" size="small" :bordered="true" class="section-card">
      <div class="import-head">
        <div><NText depth="1" style="font-size:12px;font-weight:600">{{ prbMode === 'multi' ? t.multiPrbImport : t.prbImport }}</NText><NText depth="2" style="font-size:11px;margin-top:1px;display:block">{{ prbMode === 'multi' ? t.multiPrbImportHint : t.prbImportHint }}</NText></div>
        <NButton size="tiny" type="primary" :loading="isImportingPrb" @click="importPrbFile">{{ isImportingPrb ? t.importing : (prbMode === 'multi' ? t.importPrbs : t.importPrb) }}</NButton>
      </div>

      <template v-if="prbMode === 'single'">
        <template v-if="prbFile">
          <div class="file-row"><div class="file-info"><NText depth="1" style="font-size:12px;font-weight:600;truncate">{{ prbFile.fileName }}</NText><NText depth="3" style="font-size:11px;truncate">{{ prbFile.filePath }}</NText></div><NButton size="tiny" secondary @click="prbFile = null">{{ t.remove }}</NButton></div>
          <div class="range-grid"><div v-for="r in prbValidRangeRows" :key="r.label" class="range-stat"><NText depth="3" style="font-size:10px;font-weight:600;text-transform:uppercase">{{ r.label }}</NText><NText depth="1" style="font-size:12px">{{ r.value }}</NText></div></div>
        </template>
        <div v-else class="empty-state"><NText depth="3" style="font-size:12px">{{ t.noPrbImported }}</NText></div>
      </template>

      <template v-else>
        <div class="multi-mode-summary">
          <div class="range-stat"><NText depth="3" style="font-size:10px;font-weight:600;text-transform:uppercase">{{ t.interpolationMode }}</NText><NSelect v-model:value="multiPrbInterpolationMode" :options="interpolationModeOptions" size="tiny" style="width:100%;margin-top:4px" /></div>
          <div class="range-stat"><NText depth="3" style="font-size:10px;font-weight:600;text-transform:uppercase">{{ t.multiPrbFilesLabel }}</NText><NText depth="1" style="font-size:16px;font-weight:700;margin-top:4px;display:block">{{ multiPrbFiles.length }}</NText></div>
        </div>

        <template v-if="multiPrbFiles.length > 0">
          <div v-for="(file, i) in multiPrbFiles" :key="file.filePath" class="multi-file">
            <div class="file-row"><div class="file-info"><NText depth="1" style="font-size:12px;font-weight:600;truncate">{{ file.fileName }}</NText><NText depth="3" style="font-size:11px;truncate">{{ file.filePath }}</NText></div><NButton size="tiny" secondary @click="removeMultiPrbFile(i)">{{ t.remove }}</NButton></div>
            <div class="multi-mach-grid">
              <div><NText depth="3" style="font-size:9px;font-weight:600;text-transform:uppercase;letter-spacing:0.14em">{{ t.fileMachNumber }}</NText><NInputNumber v-model:value="multiPrbMachNumbers[i]" :step="0.01" size="tiny" style="width:100%;margin-top:4px" /></div>
              <div class="range-stat"><NText depth="3" style="font-size:9px;font-weight:600;text-transform:uppercase">Alpha</NText><NText depth="1" style="font-size:11px">{{ file.validRange.alphaMin }}..{{ file.validRange.alphaMax }} deg</NText></div>
              <div class="range-stat"><NText depth="3" style="font-size:9px;font-weight:600;text-transform:uppercase">Beta</NText><NText depth="1" style="font-size:11px">{{ file.validRange.betaMin }}..{{ file.validRange.betaMax }} deg</NText></div>
              <div class="range-stat"><NText depth="3" style="font-size:9px;font-weight:600;text-transform:uppercase">Mach</NText><NText depth="1" style="font-size:11px">{{ file.validRange.machMin }}..{{ file.validRange.machMax }}</NText></div>
            </div>
          </div>
          <div style="display:flex;justify-content:flex-end"><NButton size="tiny" secondary @click="clearMultiPrbFiles">{{ t.clearAll }}</NButton></div>
        </template>
        <div v-else class="empty-state"><NText depth="3" style="font-size:12px">{{ t.noMultiPrbImported }}</NText></div>
      </template>
    </NCard>
  </div>
</template>

<style scoped>
.step-content { display:flex; flex-direction:column; gap:12px; }
.section-card { font-size:12px; }
.mode-row { display:flex; align-items:center; justify-content:space-between; gap:12px; }
.import-head { display:flex; align-items:flex-start; justify-content:space-between; gap:12px; margin-bottom:12px; }
.file-row { display:flex; align-items:center; justify-content:space-between; gap:8px; padding:8px; border-radius:4px; background:var(--bg-panel-strong); margin-bottom:8px; }
.file-info { min-width:0; flex:1; }
.range-grid { display:grid; grid-template-columns:repeat(3,1fr); gap:8px; }
.range-stat { padding:8px; border-radius:4px; border:1px solid var(--border-default); background:var(--bg-panel-strong); }
.empty-state { display:flex; align-items:center; justify-content:center; height:100px; border-radius:4px; border:1px dashed var(--border-default); background:var(--bg-panel-strong); }
.multi-mode-summary { display:grid; grid-template-columns:1fr 120px; gap:8px; margin-bottom:8px; }
.multi-file { padding:8px; border-radius:4px; border:1px solid var(--border-default); background:var(--bg-panel-strong); margin-bottom:8px; }
.multi-mach-grid { display:grid; grid-template-columns:120px repeat(3,1fr); gap:8px; margin-top:6px; }
</style>