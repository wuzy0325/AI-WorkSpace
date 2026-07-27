<script setup lang="ts">
// SevenHoleSlotRow.vue 是 7 孔探针工作区的校准文件槽位行组件。
// 由 SevenHoleWorkspace.vue 复用 7 次（1 个内区 + 6 个外区扇区）。
//
// 抽出原因：SevenHoleWorkspace.vue 单文件超 1400 行，槽位行模板结构在 7 个槽位间
// 完全重复（标签 + 文件信息 + 选择/移除按钮）。抽出后父组件减少约 150 行模板，
// 且槽位的视觉/交互逻辑独立维护，便于未来调整单槽位 UX。
//
// 注意：本组件不持有状态，所有数据通过 props 下传、操作通过 emit 上抛。
import type { SevenHolePrbFileInfo } from '../adapters/seven-hole'

defineProps<{
  /** 槽位 sector 编号：7=内区，1..6=外区扇区 n */
  sector: number
  /** 当前槽位的文件信息，null 表示未选文件 */
  file: SevenHolePrbFileInfo | null
  /** 槽位标签展示名（如 "7.prb" 或 "小角度区 CSV"） */
  displayName: string
  /** 是否为校准 CSV 数据源（影响样式和文案） */
  isCsv: boolean
  /** 是否正在批量导入中（禁用所有按钮） */
  isImporting: boolean
}>()

const emit = defineEmits<{
  (e: 'pick'): void
  (e: 'remove'): void
}>()
</script>

<template>
  <div class="slot-row" :class="{ 'slot-row--inner': sector === 7 }">
    <div class="slot-label">
      <span class="slot-tag" :class="{ 'slot-tag--inner': sector === 7 }">
        {{ sector === 7 ? '内区' : `外区 ${sector}` }}
      </span>
      <span class="slot-name">{{ displayName }}</span>
    </div>
    <template v-if="file">
      <div class="file-info" :title="file.filePath">
        <span class="file-name">{{ file.fileName }}</span>
        <span v-if="file.pointCount" class="file-meta">{{ file.pointCount }} pts</span>
      </div>
      <button
        class="btn btn-icon danger"
        :disabled="isImporting"
        @click="emit('remove')"
        title="移除"
      >
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <polyline points="3 6 5 6 21 6"/>
          <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
        </svg>
      </button>
    </template>
    <button
      v-else
      class="btn btn-secondary btn-pick"
      :disabled="isImporting"
      @click="emit('pick')"
    >
      <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
        <polyline points="14 2 14 8 20 8"/>
      </svg>
      选择文件
    </button>
  </div>
</template>

<style src="../styles/seven-hole-slot-row.css"></style>
