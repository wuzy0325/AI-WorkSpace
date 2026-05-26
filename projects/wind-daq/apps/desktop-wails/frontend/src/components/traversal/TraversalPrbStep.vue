<script setup lang="ts">
import { computed, ref } from 'vue'
import { isWailsAvailable, wailsApi } from '@api/wails-adapter'
import UiButton from '@components/ui/UiButton.vue'
import { useFeedbackStore } from '@stores/feedbackStore'
import { useTraversalStore } from '@stores/traversalStore'
import type {
  CalibrationCsvFileInfo,
  MultiPrbInterpolationMode,
  PrbFileInfo,
  InterpolationAlgorithm
} from '@shared/types/traversal'

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

const subCardClass = 'rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel-strong)] p-3'

const prbValidRangeRows = computed(() => {
  if (!prbFile.value) return []
  return [
    { label: 'Alpha', value: `${prbFile.value.validRange.alphaMin} to ${prbFile.value.validRange.alphaMax} deg` },
    { label: 'Beta', value: `${prbFile.value.validRange.betaMin} to ${prbFile.value.validRange.betaMax} deg` },
    { label: 'Mach', value: `${prbFile.value.validRange.machMin} to ${prbFile.value.validRange.machMax}` }
  ]
})

function clonePrbFileInfo(file: PrbFileInfo): PrbFileInfo {
  return { ...file, validRange: { ...file.validRange } }
}

function normalizeMultiPrbMachNumbers(files: PrbFileInfo[], machNumbers: number[] = []): number[] {
  return files.map((file, index) => {
    const value = machNumbers[index] ?? file.machNumber ?? file.validRange.machMin
    return Number.isFinite(value) ? value : 0
  })
}

function setPrbMode(mode: 'single' | 'multi'): void {
  prbMode.value = mode
}

function removeMultiPrbFile(index: number): void {
  multiPrbFiles.value.splice(index, 1)
  multiPrbMachNumbers.value.splice(index, 1)
}

function clearMultiPrbFiles(): void {
  multiPrbFiles.value = []
  multiPrbMachNumbers.value = []
}

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
            multiPrbFiles.value = imported.files.map((file) => clonePrbFileInfo(file))
            multiPrbMachNumbers.value = normalizeMultiPrbMachNumbers(imported.files, imported.machNumbers)
            if (imported.warnings.length > 0) feedbackStore.pushToast(imported.warnings.join('\n'), 'warning')
          }
        } else {
          const imported = await traversalStore.importPrbFile(selectedPaths[0]!)
          if (imported) prbFile.value = clonePrbFileInfo(imported)
        }
      } finally {
        isImportingPrb.value = false
      }
      return
    }

    const input = document.createElement('input')
    input.type = 'file'
    input.accept = '.prb'
    input.multiple = prbMode.value === 'multi'
    input.onchange = async (event) => {
      const files = Array.from((event.target as HTMLInputElement).files ?? [])
      if (files.length === 0) return
      isImportingPrb.value = true
      try {
        if (prbMode.value === 'multi') {
          const imported = await traversalStore.importMultiPrbFiles(
            files.map((file) => (file as File & { path?: string }).path ?? file.name), undefined, multiPrbInterpolationMode.value
          )
          if (imported) {
            multiPrbFiles.value = imported.files.map((file) => clonePrbFileInfo(file))
            multiPrbMachNumbers.value = normalizeMultiPrbMachNumbers(imported.files, imported.machNumbers)
            if (imported.warnings.length > 0) {
              feedbackStore.pushToast(imported.warnings.join('\n'), 'warning')
            }
          } else {
            feedbackStore.pushToast(props.t.failedImportPrb + ': ' + (traversalStore.error || 'Unknown error'), 'error')
          }
        } else {
          const imported = await traversalStore.importPrbFile((files[0]! as File & { path?: string }).path ?? files[0]!.name)
          if (imported) {
            prbFile.value = clonePrbFileInfo(imported)
          } else {
            feedbackStore.pushToast(props.t.failedImportPrb + ': ' + (traversalStore.error || 'Unknown error'), 'error')
          }
        }
      } finally {
        isImportingPrb.value = false
      }
    }
    input.click()
  } catch (err) {
    feedbackStore.pushToast(props.t.failedImportPrb + ': ' + (err instanceof Error ? err.message : String(err)), 'error')
  }
}

async function importCalibrationCsvFile(): Promise<void> {
  try {
    if (isWailsAvailable()) {
      const filePath = await wailsApi.app.pickFile(props.t.importCsv || 'Import CSV', [
        { displayName: 'Calibration CSV', pattern: '*.csv;*.txt' }
      ])
      if (!filePath) return
      isImportingCsv.value = true
      try {
        const imported = await traversalStore.importCalibrationCsvFile(filePath)
        if (imported) calibrationCsvFile.value = { ...imported }
      } finally {
        isImportingCsv.value = false
      }
      return
    }

    const input = document.createElement('input')
    input.type = 'file'
    input.accept = '.csv,.txt'
    input.onchange = async (event) => {
      const files = Array.from((event.target as HTMLInputElement).files ?? [])
      if (files.length === 0) return
      isImportingCsv.value = true
      try {
        const imported = await traversalStore.importCalibrationCsvFile((files[0]! as File & { path?: string }).path ?? files[0]!.name)
        if (imported) {
          calibrationCsvFile.value = { ...imported }
        } else {
          feedbackStore.pushToast(props.t.failedImportCsv + ': ' + (traversalStore.error || 'Unknown error'), 'error')
        }
      } finally {
        isImportingCsv.value = false
      }
    }
    input.click()
  } catch (err) {
    feedbackStore.pushToast(props.t.failedImportCsv + ': ' + (err instanceof Error ? err.message : String(err)), 'error')
  }
}
</script>

<template>
  <div class="space-y-4">
    <section class="ui-panel-surface p-4 p-3">
      <div class="flex items-center justify-between gap-3">
        <div>
          <div class="text-[11px] font-semibold uppercase tracking-[0.14em] text-[color:var(--text-muted)]">{{ t.prbMode }}</div>
          <p class="mt-1 text-xs text-[color:var(--text-secondary)]">{{ t.prbModeHint }}</p>
        </div>
        <div class="inline-flex rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel-strong)] p-1">
          <button
            data-test="traversal-prb-mode-single" type="button"
            class="rounded-[var(--radius-sm)] px-3 py-1.5 text-xs font-semibold transition-colors"
            :class="prbMode === 'single' ? 'bg-[color:var(--accent-primary)] text-white' : 'text-[color:var(--text-secondary)]'"
            :aria-pressed="prbMode === 'single'"
            @click="setPrbMode('single')"
          >{{ t.singlePrbMode }}</button>
          <button
            data-test="traversal-prb-mode-multi" type="button"
            class="rounded-[var(--radius-sm)] px-3 py-1.5 text-xs font-semibold transition-colors"
            :class="prbMode === 'multi' ? 'bg-[color:var(--accent-primary)] text-white' : 'text-[color:var(--text-secondary)]'"
            :aria-pressed="prbMode === 'multi'"
            @click="setPrbMode('multi')"
          >{{ t.multiPrbMode }}</button>
        </div>
      </div>
    </section>

    <section class="ui-panel-surface p-4 p-3">
      <div class="flex items-center justify-between gap-3">
        <div>
          <div class="text-[11px] font-semibold uppercase tracking-[0.14em] text-[color:var(--text-muted)]">{{ t.interpolationAlgorithm || 'Interpolation Algorithm' }}</div>
          <p class="mt-1 text-xs text-[color:var(--text-secondary)]">{{ t.interpolationAlgorithmHint || 'Select interpolation algorithm' }}</p>
        </div>
        <div class="inline-flex rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel-strong)] p-1">
          <button
            data-test="traversal-algorithm-old" type="button"
            class="rounded-[var(--radius-sm)] px-3 py-1.5 text-xs font-semibold transition-colors"
            :class="interpolationAlgorithm === 'old' ? 'bg-[color:var(--accent-primary)] text-white' : 'text-[color:var(--text-secondary)]'"
            :aria-pressed="interpolationAlgorithm === 'old'"
            @click="interpolationAlgorithm = 'old'"
          >{{ t.algorithmOld || 'Old' }}</button>
          <button
            data-test="traversal-algorithm-new" type="button"
            class="rounded-[var(--radius-sm)] px-3 py-1.5 text-xs font-semibold transition-colors"
            :class="interpolationAlgorithm === 'new' ? 'bg-[color:var(--accent-primary)] text-white' : 'text-[color:var(--text-secondary)]'"
            :aria-pressed="interpolationAlgorithm === 'new'"
            @click="interpolationAlgorithm = 'new'"
          >{{ t.algorithmNew || 'New' }}</button>
        </div>
      </div>
    </section>

    <!-- CSV calibration data import (new algorithm) -->
    <section v-if="interpolationAlgorithm === 'new'" class="ui-panel-surface p-4">
      <div class="flex items-start justify-between gap-4 mb-4">
        <div>
          <h3 class="text-sm font-semibold text-[color:var(--text-primary)]">{{ t.csvImport || 'Import CSV Calibration Data' }}</h3>
          <p class="mt-1 text-xs text-[color:var(--text-secondary)]">{{ t.csvImportHint || 'Import calibration data in CSV format' }}</p>
        </div>
        <UiButton size="sm" variant="primary" :disabled="isImportingCsv" @click="importCalibrationCsvFile">{{ isImportingCsv ? (t.importing || 'Importing...') : (t.importCsv || 'Import CSV') }}</UiButton>
      </div>
      <template v-if="calibrationCsvFile">
        <div class="flex items-center justify-between p-3 bg-[color:var(--bg-panel-strong)] rounded-[var(--radius-sm)]">
          <div class="min-w-0">
            <div class="truncate text-sm font-semibold text-[color:var(--text-primary)]">{{ calibrationCsvFile.fileName }}</div>
            <div class="truncate text-xs text-[color:var(--text-muted)]">{{ calibrationCsvFile.filePath }}</div>
          </div>
          <UiButton size="sm" variant="danger" @click="calibrationCsvFile = null">{{ t.remove }}</UiButton>
        </div>
        <div class="grid gap-3 md:grid-cols-4 mt-3">
          <div :class="subCardClass">
            <div class="text-[11px] font-semibold uppercase tracking-[0.14em] text-[color:var(--text-muted)]">Alpha</div>
            <div class="mt-2 text-sm font-medium text-[color:var(--text-primary)]">{{ calibrationCsvFile.validRange.alphaMin }}..{{ calibrationCsvFile.validRange.alphaMax }} deg</div>
          </div>
          <div :class="subCardClass">
            <div class="text-[11px] font-semibold uppercase tracking-[0.14em] text-[color:var(--text-muted)]">Beta</div>
            <div class="mt-2 text-sm font-medium text-[color:var(--text-primary)]">{{ calibrationCsvFile.validRange.betaMin }}..{{ calibrationCsvFile.validRange.betaMax }} deg</div>
          </div>
          <div :class="subCardClass">
            <div class="text-[11px] font-semibold uppercase tracking-[0.14em] text-[color:var(--text-muted)]">{{ t.pointCount || 'Points' }}</div>
            <div class="mt-2 text-sm font-medium text-[color:var(--text-primary)]">{{ calibrationCsvFile.pointCount }}</div>
          </div>
        </div>
      </template>
      <div v-else class="flex h-32 items-center justify-center rounded-[var(--radius-sm)] border border-dashed border-[color:var(--border-default)] bg-[color:var(--bg-panel-strong)] text-sm text-[color:var(--text-muted)]">
        {{ t.noCsvImported || 'No CSV calibration data imported' }}
      </div>
    </section>

    <!-- PRB import (old algorithm) -->
    <section v-if="interpolationAlgorithm === 'old'" class="ui-panel-surface p-4">
      <div class="flex items-start justify-between gap-4 mb-4">
        <div>
          <h3 class="text-sm font-semibold text-[color:var(--text-primary)]">{{ prbMode === 'multi' ? t.multiPrbImport : t.prbImport }}</h3>
          <p class="mt-1 text-xs text-[color:var(--text-secondary)]">{{ prbMode === 'multi' ? t.multiPrbImportHint : t.prbImportHint }}</p>
        </div>
        <UiButton size="sm" variant="primary" :disabled="isImportingPrb" @click="importPrbFile">{{ isImportingPrb ? t.importing : (prbMode === 'multi' ? t.importPrbs : t.importPrb) }}</UiButton>
      </div>

      <template v-if="prbMode === 'single'">
        <template v-if="prbFile">
          <div class="flex items-center justify-between p-3 bg-[color:var(--bg-panel-strong)] rounded-[var(--radius-sm)]">
            <div class="min-w-0">
              <div class="truncate text-sm font-semibold text-[color:var(--text-primary)]">{{ prbFile.fileName }}</div>
              <div class="truncate text-xs text-[color:var(--text-muted)]">{{ prbFile.filePath }}</div>
            </div>
            <UiButton size="sm" variant="danger" @click="prbFile = null">{{ t.remove }}</UiButton>
          </div>
          <div data-test="traversal-prb-valid-range" class="grid gap-3 md:grid-cols-3">
            <div v-for="range in prbValidRangeRows" :key="range.label" :class="subCardClass">
              <div class="text-[11px] font-semibold uppercase tracking-[0.14em] text-[color:var(--text-muted)]">{{ range.label }}</div>
              <div class="mt-2 text-sm font-medium text-[color:var(--text-primary)]">{{ range.value }}</div>
            </div>
          </div>
        </template>
        <div v-else class="flex h-32 items-center justify-center rounded-[var(--radius-sm)] border border-dashed border-[color:var(--border-default)] bg-[color:var(--bg-panel-strong)] text-sm text-[color:var(--text-muted)]">
          {{ t.noPrbImported }}
        </div>
      </template>

      <template v-else>
        <div class="grid gap-2 md:grid-cols-[minmax(0,1fr)_120px] mb-3">
          <div class="rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel-strong)] p-2.5">
            <div class="text-[11px] font-semibold uppercase tracking-[0.14em] text-[color:var(--text-muted)]">{{ t.interpolationMode }}</div>
            <select data-test="traversal-multi-prb-mode-select" v-model="multiPrbInterpolationMode"
              class="mt-1.5 w-full rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel)] px-2 py-1.5 text-sm text-[color:var(--text-primary)]">
              <option value="linear">{{ t.linearInterpolation }}</option>
              <option value="nearest">{{ t.nearestInterpolation }}</option>
            </select>
          </div>
          <div class="rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel-strong)] p-2.5">
            <div class="text-[11px] font-semibold uppercase tracking-[0.14em] text-[color:var(--text-muted)]">{{ t.multiPrbFilesLabel }}</div>
            <div class="mt-1.5 text-sm font-semibold text-[color:var(--text-primary)]">{{ multiPrbFiles.length }}</div>
          </div>
        </div>

        <template v-if="multiPrbFiles.length > 0">
          <div class="space-y-2.5">
            <div v-for="(file, index) in multiPrbFiles" :key="file.filePath" data-test="traversal-multi-prb-file"
              class="rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel-strong)] p-2.5">
              <div class="flex items-start justify-between gap-2.5">
                <div class="min-w-0">
                  <div class="truncate text-sm font-semibold text-[color:var(--text-primary)]">{{ file.fileName }}</div>
                  <div class="truncate text-[11px] leading-4 text-[color:var(--text-muted)]">{{ file.filePath }}</div>
                </div>
                <UiButton size="sm" variant="danger" @click="removeMultiPrbFile(index)">{{ t.remove }}</UiButton>
              </div>
              <div class="mt-2.5 grid gap-2 md:grid-cols-[120px_repeat(3,minmax(0,1fr))] items-stretch">
                <div>
                  <label class="block text-[9px] font-semibold uppercase tracking-[0.14em] text-[color:var(--text-muted)]">{{ t.fileMachNumber }}</label>
                  <input v-model.number="multiPrbMachNumbers[index]" type="number" step="0.01"
                    class="mt-1 w-full rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel)] px-2 py-1.5 text-sm text-[color:var(--text-primary)]" />
                </div>
                <div class="rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel)] px-2 py-1.5">
                  <div class="text-[9px] font-semibold uppercase tracking-[0.14em] text-[color:var(--text-muted)]">Alpha</div>
                  <div class="mt-1 whitespace-nowrap text-[11px] font-medium leading-4 text-[color:var(--text-primary)]">{{ file.validRange.alphaMin }}..{{ file.validRange.alphaMax }} deg</div>
                </div>
                <div class="rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel)] px-2 py-1.5">
                  <div class="text-[9px] font-semibold uppercase tracking-[0.14em] text-[color:var(--text-muted)]">Beta</div>
                  <div class="mt-1 whitespace-nowrap text-[11px] font-medium leading-4 text-[color:var(--text-primary)]">{{ file.validRange.betaMin }}..{{ file.validRange.betaMax }} deg</div>
                </div>
                <div class="rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel)] px-2 py-1.5">
                  <div class="text-[9px] font-semibold uppercase tracking-[0.14em] text-[color:var(--text-muted)]">Mach</div>
                  <div class="mt-1 whitespace-nowrap text-[11px] font-medium leading-4 text-[color:var(--text-primary)]">{{ file.validRange.machMin }}..{{ file.validRange.machMax }}</div>
                </div>
              </div>
            </div>
          </div>
          <div class="flex justify-end">
            <UiButton size="sm" variant="secondary" @click="clearMultiPrbFiles">{{ t.clearAll }}</UiButton>
          </div>
        </template>
        <div v-else class="flex h-32 items-center justify-center rounded-[var(--radius-sm)] border border-dashed border-[color:var(--border-default)] bg-[color:var(--bg-panel-strong)] text-sm text-[color:var(--text-muted)]">
          {{ t.noMultiPrbImported }}
        </div>
      </template>
    </section>
  </div>
</template>

