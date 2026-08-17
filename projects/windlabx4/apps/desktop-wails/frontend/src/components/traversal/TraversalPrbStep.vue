<script setup lang="ts">
import { computed } from 'vue'
import type {
  CalibrationCsvFileInfo,
  MultiPrbInterpolationMode,
  PrbFileInfo,
  InterpolationAlgorithm,
  SevenHolePrbDraft,
  TraversalProbeType
} from '@shared/types/traversal'
import FiveHolePrbConfig from './FiveHolePrbConfig.vue'
import SevenHolePrbConfig from './SevenHolePrbConfig.vue'
import { useTraversalStore } from '@stores/traversalStore'
import type { TraversalPrbOperations } from './traversalPrbOperations'

/**
 * 遍历 PRB 步骤公共壳（spec-seven-hole-traversal §6.3）：
 * 仅按探针类型选择子组件，不持有任何五孔/七孔算法状态；
 * 两个子组件只接收各自类型的 model，不读取对方字段。
 */

const probeType = defineModel<TraversalProbeType>('probeType', { required: true })

// 五孔子组件 models（原样透传，壳不消费）
const prbMode = defineModel<'single' | 'multi'>('prbMode', { required: true })
const interpolationAlgorithm = defineModel<InterpolationAlgorithm>('interpolationAlgorithm', { required: true })
const prbFile = defineModel<PrbFileInfo | null>('prbFile', { required: true })
const multiPrbFiles = defineModel<PrbFileInfo[]>('multiPrbFiles', { required: true })
const multiPrbMachNumbers = defineModel<number[]>('multiPrbMachNumbers', { required: true })
const multiPrbInterpolationMode = defineModel<MultiPrbInterpolationMode>('multiPrbInterpolationMode', { required: true })
const calibrationCsvFile = defineModel<CalibrationCsvFileInfo | null>('calibrationCsvFile', { required: true })

// 七孔子组件 model（编辑态 Draft，壳不消费）
const sevenHolePrbDraft = defineModel<SevenHolePrbDraft>('sevenHolePrbDraft', { required: true })

const props = defineProps<{
  t: Record<string, string>
  operations?: TraversalPrbOperations
}>()

const traversalStore = useTraversalStore()
const defaultOperations: TraversalPrbOperations = {
  getError: () => traversalStore.error,
  importPrbFile: traversalStore.importPrbFile,
  importMultiPrbFiles: traversalStore.importMultiPrbFiles,
  importCalibrationCsvFile: traversalStore.importCalibrationCsvFile,
  importSevenHolePrbFiles: traversalStore.importSevenHolePrbFiles,
  importSevenHoleCalibrationCsvFiles: traversalStore.importSevenHoleCalibrationCsvFiles,
  clearInterpolator: (_probeType) => {
    traversalStore.clearInterpolator()
    return true
  },
}
const operations = computed(() => props.operations ?? defaultOperations)
const isSevenHole = computed(() => probeType.value === 'seven-hole')
</script>

<template>
  <SevenHolePrbConfig v-if="isSevenHole" v-model="sevenHolePrbDraft" :t="props.t" :operations="operations" />
  <FiveHolePrbConfig
    v-else
    v-model:prb-mode="prbMode"
    v-model:interpolation-algorithm="interpolationAlgorithm"
    v-model:prb-file="prbFile"
    v-model:multi-prb-files="multiPrbFiles"
    v-model:multi-prb-mach-numbers="multiPrbMachNumbers"
    v-model:multi-prb-interpolation-mode="multiPrbInterpolationMode"
    v-model:calibration-csv-file="calibrationCsvFile"
    :t="props.t"
    :operations="operations"
  />
</template>
