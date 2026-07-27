<script setup lang="ts">
/**
 * 扫描结果列表（多选形态）。
 *
 * 特性：
 * - 每行 checkbox 支持批量选择
 * - 内联命名输入框，占位为按 MAC/IP 生成的默认名
 * - 已添加设备（host key 命中）行置灰、checkbox 禁用、显示"已添加"标签
 * - 通过 v-model 向父组件暴露当前选中项（含用户覆盖的名字）
 */
import { computed, ref, watch } from 'vue'
import type { ScanResult } from '@bridge/deviceBridge'
import { Wifi, Monitor, Loader2, CheckCircle2 } from '@lucide/vue'
import { hostKey, makeDefaultName } from '@stores/deviceStoreHelpers'
import { useI18nStore } from '@stores/i18nStore'

const i18n = useI18nStore()

/** 单条选择项：id 与后端 ScanResult.id 对齐，name 为用户改名后或默认名 */
export interface ScanSelectionItem {
  id: string
  name: string
  address: string
  port: number
  macAddress: string
  serialNumber: string
}

const props = defineProps<{
  results: ScanResult[]
  scanning: boolean
  /** 已添加设备的 host key 集合（从 deviceStore.existingDeviceKeys 传入） */
  existingKeys: Set<string>
  /** v-model 双向绑定：当前选中项列表 */
  modelValue: ScanSelectionItem[]
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: ScanSelectionItem[]): void
}>()

/**
 * 本地缓存：记录用户曾对某个扫描 id 输入过的自定义名字。
 * 用途：用户勾选 → 改名 → 取消勾选 → 再勾选时，恢复之前输入的名字，
 * 而不是重置为默认名。缓存生命周期跟随组件实例，弹窗关闭即销毁。
 */
const overriddenNames = ref<Map<string, string>>(new Map())

/**
 * 记录用户已手动"取消勾选"过的扫描 id。
 *
 * 目的：默认对所有未添加设备预勾选，让用户点一次即可批量添加；
 * 但一旦用户主动取消某台，就要"尊重"用户意图——即使扫描结果刷新，
 * 也不能再自动帮它勾回来。
 * 缓存生命周期跟随组件实例（弹窗关闭即销毁）。
 */
const manuallyUnchecked = ref<Set<string>>(new Set())

/**
 * 自动预勾选：每次扫描结果变化，把所有"未添加 & 未被用户主动取消"的行加入选中列表。
 *
 * 触发时机：
 * - 扫描完成后 results 从空变有
 * - 重新扫描导致 results 更新
 * - 侧边栏正在扫描过程中新设备增量出现（如果后端将来支持流式）
 */
watch(
  () => props.results,
  (results) => {
    if (!results || results.length === 0) return
    const currentIds = new Set(props.modelValue.map((item) => item.id))
    const next: ScanSelectionItem[] = [...props.modelValue]
    let changed = false
    for (const r of results) {
      if (isAlreadyAdded(r)) continue
      if (currentIds.has(r.id)) continue
      if (manuallyUnchecked.value.has(r.id)) continue
      const cachedName = overriddenNames.value.get(r.id)
      const resolvedName = cachedName ?? makeDefaultName({
        macAddress: r.macAddress,
        address: r.address,
        port: r.port,
      })
      next.push({
        id: r.id,
        name: resolvedName,
        address: r.address,
        port: r.port,
        macAddress: r.macAddress ?? '',
        serialNumber: r.serialNumber ?? '',
      })
      changed = true
    }
    if (changed) emit('update:modelValue', next)
  },
  { immediate: true, deep: false },
)

/**
 * 判断某扫描结果是否已存在（host:addr:port 命中现有 profile）。
 * 使用 host key 匹配，因为 profile 未存 MAC，只能按 IP+port 查重。
 */
function isAlreadyAdded(r: ScanResult): boolean {
  return props.existingKeys.has(hostKey(r.address, r.port))
}

/** 是否已勾选：以选中列表中 id 命中为准 */
function isChecked(r: ScanResult): boolean {
  return props.modelValue.some((item) => item.id === r.id)
}

/** 计算该行显示的名字：优先用选中列表中已存的名字，否则给默认名 */
function nameFor(r: ScanResult): string {
  const existing = props.modelValue.find((item) => item.id === r.id)
  if (existing) return existing.name
  return makeDefaultName({ macAddress: r.macAddress, address: r.address, port: r.port })
}

/** 切换某行的勾选状态 */
function toggle(r: ScanResult): void {
  if (isAlreadyAdded(r)) return
  const next = [...props.modelValue]
  const idx = next.findIndex((item) => item.id === r.id)
  if (idx >= 0) {
    next.splice(idx, 1)
    // 记录用户主动取消勾选，防止 results 刷新时被 watcher 自动勾回来
    manuallyUnchecked.value.add(r.id)
  } else {
    // 用户主动重新勾选：解除"取消状态"，之后 results 刷新可再自动预勾
    manuallyUnchecked.value.delete(r.id)
    // 优先复用用户之前对该 id 输入过的自定义名（如果有）；否则用默认名
    const cachedName = overriddenNames.value.get(r.id)
    const resolvedName = cachedName ?? makeDefaultName({
      macAddress: r.macAddress,
      address: r.address,
      port: r.port,
    })
    next.push({
      id: r.id,
      name: resolvedName,
      address: r.address,
      port: r.port,
      macAddress: r.macAddress ?? '',
      serialNumber: r.serialNumber ?? '',
    })
  }
  emit('update:modelValue', next)
}

/** 用户内联改名：只有已勾选的行才允许改名（未勾选行显示只读的默认名占位） */
function updateName(r: ScanResult, newName: string): void {
  // 同步写入本地缓存，供之后重新勾选时恢复
  overriddenNames.value.set(r.id, newName)
  const next = props.modelValue.map((item) =>
    item.id === r.id ? { ...item, name: newName } : item,
  )
  emit('update:modelValue', next)
}

/** 可勾选行数（用于父组件判断是否禁用"添加所选"按钮） */
const selectableCount = computed(() =>
  props.results.filter((r) => !isAlreadyAdded(r)).length,
)

/** 已勾选行数（用于全选按钮的 tri-state 判断） */
const checkedCount = computed(() =>
  props.modelValue.filter((item) =>
    props.results.some((r) => r.id === item.id),
  ).length,
)

/** 全选状态：'none' | 'partial' | 'all' */
const selectAllState = computed<'none' | 'partial' | 'all'>(() => {
  const total = selectableCount.value
  if (total === 0) return 'none'
  if (checkedCount.value === 0) return 'none'
  if (checkedCount.value >= total) return 'all'
  return 'partial'
})

/**
 * 全选/全取消：
 * - 当前为 all → 清空所有可选项的勾选，并把 id 记入"手动取消"集合
 * - 否则（none / partial）→ 选中全部可选项，清理"手动取消"集合
 */
function toggleAll(): void {
  if (selectAllState.value === 'all') {
    // 全取消：从选中列表中移除本次扫描结果内的所有条目；
    // 同时把可选 id 记入 manuallyUnchecked，避免 watcher 立即再勾回来
    const scanIds = new Set(props.results.map((r) => r.id))
    const next = props.modelValue.filter((item) => !scanIds.has(item.id))
    for (const r of props.results) {
      if (!isAlreadyAdded(r)) manuallyUnchecked.value.add(r.id)
    }
    emit('update:modelValue', next)
    return
  }
  // 全选：把所有可选行加入选中列表；同时清理"手动取消"集合，恢复默认勾选行为
  const existingIds = new Set(props.modelValue.map((item) => item.id))
  const next: ScanSelectionItem[] = [...props.modelValue]
  for (const r of props.results) {
    if (isAlreadyAdded(r)) continue
    manuallyUnchecked.value.delete(r.id)
    if (existingIds.has(r.id)) continue
    const cachedName = overriddenNames.value.get(r.id)
    const resolvedName = cachedName ?? makeDefaultName({
      macAddress: r.macAddress,
      address: r.address,
      port: r.port,
    })
    next.push({
      id: r.id,
      name: resolvedName,
      address: r.address,
      port: r.port,
      macAddress: r.macAddress ?? '',
      serialNumber: r.serialNumber ?? '',
    })
  }
  emit('update:modelValue', next)
}

defineExpose({ selectableCount, checkedCount, toggleAll })
</script>

<template>
  <div class="scan">
    <div v-if="scanning" class="scan__loading">
      <Loader2 class="scan__spinner" />
      <span>{{ i18n.t('scan.scanning') }}</span>
    </div>

    <div v-else-if="results.length === 0" class="scan__empty">
      <Monitor class="scan__empty-icon" />
      <p>{{ i18n.t('scan.noDevices') }}</p>
      <p class="scan__empty-hint">{{ i18n.t('scan.noDevicesHint') }}</p>
    </div>

    <template v-else>
      <!-- 表头行：全选按钮 + 汇总信息 -->
      <div class="scan__toolbar">
        <button
          class="scan__selectall"
          :disabled="selectableCount === 0"
          :title="selectAllState === 'all' ? i18n.t('scan.unselectAllTitle') : i18n.t('scan.selectAllTitle')"
          @click="toggleAll"
        >
          <span
            class="scan__selectall-box"
            :class="{
              'scan__selectall-box--partial': selectAllState === 'partial',
              'scan__selectall-box--all': selectAllState === 'all',
            }"
          >
            <CheckCircle2 v-if="selectAllState === 'all'" class="scan__selectall-icon" />
            <span v-else-if="selectAllState === 'partial'" class="scan__selectall-dash" />
          </span>
          {{ selectAllState === 'all' ? i18n.t('scan.unselectAll') : i18n.t('scan.selectAll') }}
        </button>
        <span class="scan__toolbar-info">
          {{ i18n.t('scan.selectionSummary', { checked: checkedCount, selectable: selectableCount, total: results.length }) }}
        </span>
      </div>

      <ul class="scan__list">
        <li
          v-for="r in results"
          :key="r.id"
          class="scan__item"
          :class="{ 'scan__item--disabled': isAlreadyAdded(r) }"
        >
          <label class="scan__item-check">
            <input
              type="checkbox"
              :checked="isChecked(r)"
              :disabled="isAlreadyAdded(r)"
              @change="toggle(r)"
            />
          </label>

          <div class="scan__item-icon">
            <Wifi class="scan__item-icon-svg" />
          </div>

          <div class="scan__item-info">
            <input
              v-if="!isAlreadyAdded(r)"
              class="scan__item-name-input"
              type="text"
              :value="nameFor(r)"
              :placeholder="nameFor(r)"
              :disabled="!isChecked(r)"
              :title="isChecked(r) ? i18n.t('scan.editNameChecked') : i18n.t('scan.editNameUnchecked')"
              @input="(ev) => updateName(r, (ev.target as HTMLInputElement).value)"
            />
            <span v-else class="scan__item-name scan__item-name--muted">
              {{ nameFor(r) }}
            </span>
            <span class="scan__item-addr mono">{{ r.address }}:{{ r.port }}</span>
            <span v-if="r.serialNumber" class="scan__item-sn mono">{{ r.serialNumber }}</span>
          </div>

          <div class="scan__item-meta">
            <span v-if="isAlreadyAdded(r)" class="scan__item-badge">
              <CheckCircle2 class="scan__item-badge-icon" />
              {{ i18n.t('scan.added') }}
            </span>
            <span v-if="r.macAddress" class="scan__item-mac mono">{{ r.macAddress }}</span>
            <span v-if="r.firmwareVersion" class="scan__item-fw mono">FW {{ r.firmwareVersion }}</span>
          </div>
        </li>
      </ul>
    </template>
  </div>
</template>

<style scoped>
.scan {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  /* 撑满父容器（dialog__body），让 .scan__list 能吃到所有剩余高度 */
  flex: 1;
  min-height: 0;
}

.scan__loading {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.6rem;
  padding: 2.5rem 1rem;
  color: var(--text-muted);
  font-size: var(--font-size-sm);
  font-weight: 600;
}

.scan__spinner {
  width: 20px;
  height: 20px;
  animation: spin 1s linear infinite;
}

.scan__empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.4rem;
  padding: 2rem 1rem;
  color: var(--text-muted);
}

.scan__empty-icon {
  width: 32px;
  height: 32px;
  opacity: 0.4;
  margin-bottom: 0.3rem;
}

.scan__empty p {
  font-size: var(--font-size-sm);
  font-weight: 600;
}

.scan__empty-hint {
  font-size: var(--font-size-xs) !important;
  font-weight: 400 !important;
  color: var(--text-muted);
  text-align: center;
}

.scan__list {
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  /* 撑满弹窗内可用高度：由 dialog__body flex 分配 */
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding-right: 0.25rem;
}

/* 表头工具栏：全选按钮 + 已选/可选汇总信息 */
.scan__toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  padding: 0.4rem 0.5rem;
  background: var(--bg-panel-strong);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  flex-shrink: 0;
}

.scan__selectall {
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.35rem 0.6rem;
  background: transparent;
  border: 1px solid var(--border-default);
  border-radius: var(--radius-sm);
  font-size: var(--font-size-xs);
  font-weight: 700;
  color: var(--text-secondary);
  cursor: pointer;
  transition: all var(--motion-fast) var(--easing-standard);
}

.scan__selectall:hover:not(:disabled) {
  border-color: var(--accent-border);
  color: var(--accent);
  background: var(--accent-soft);
}

.scan__selectall:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

/* 三态视觉：空 / 部分 / 全选 */
.scan__selectall-box {
  width: 14px;
  height: 14px;
  border: 1.5px solid var(--border-default);
  border-radius: 3px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: transparent;
  transition: all var(--motion-fast) var(--easing-standard);
}

.scan__selectall-box--partial,
.scan__selectall-box--all {
  border-color: var(--accent);
  background: var(--accent);
}

.scan__selectall-icon {
  width: 10px;
  height: 10px;
  color: #ffffff;
}

.scan__selectall-dash {
  width: 8px;
  height: 2px;
  background: #ffffff;
  border-radius: 1px;
}

.scan__toolbar-info {
  font-size: var(--font-size-xs);
  color: var(--text-muted);
  font-weight: 600;
}

.scan__item {
  display: flex;
  align-items: center;
  gap: 0.7rem;
  padding: 0.65rem 0.75rem;
  border-radius: var(--radius-md);
  background: var(--btn-bg);
  border: 1px solid var(--border-default);
  transition: all var(--motion-fast) var(--easing-standard);
}

.scan__item:hover:not(.scan__item--disabled) {
  border-color: var(--accent-border);
  background: var(--accent-soft);
}

.scan__item--disabled {
  opacity: 0.55;
  cursor: not-allowed;
}

.scan__item--disabled:hover {
  border-color: var(--border-default);
  background: var(--btn-bg);
}

.scan__item-check {
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.scan__item-check input[type='checkbox'] {
  width: 16px;
  height: 16px;
  cursor: pointer;
  accent-color: var(--accent);
}

.scan__item-check input[type='checkbox']:disabled {
  cursor: not-allowed;
}

.scan__item-icon {
  width: 32px;
  height: 32px;
  border-radius: var(--radius-md);
  background: linear-gradient(135deg, var(--accent) 0%, var(--accent-active) 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.scan__item-icon-svg {
  width: 16px;
  height: 16px;
  color: #ffffff;
}

.scan__item-info {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
  flex: 1;
  min-width: 0;
}

.scan__item-name-input {
  font-size: var(--font-size-sm);
  font-weight: 700;
  color: var(--text-primary);
  background: transparent;
  border: 1px solid transparent;
  border-radius: var(--radius-sm);
  padding: 0.15rem 0.35rem;
  margin-left: -0.35rem;
  outline: none;
  transition: all var(--motion-fast) var(--easing-standard);
  width: 100%;
}

.scan__item-name-input:focus {
  background: var(--bg-panel);
  border-color: var(--accent-border);
}

.scan__item-name-input:disabled {
  color: var(--text-muted);
  cursor: not-allowed;
}

.scan__item-name {
  font-size: var(--font-size-sm);
  font-weight: 700;
  color: var(--text-primary);
}

.scan__item-name--muted {
  color: var(--text-muted);
}

.scan__item-addr {
  font-size: var(--font-size-xs);
  color: var(--text-muted);
  font-weight: 500;
}

.scan__item-sn {
  font-size: 0.6rem;
  color: var(--text-muted);
}

.scan__item-meta {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 0.15rem;
  flex-shrink: 0;
}

.scan__item-badge {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.15rem 0.45rem;
  background: var(--accent-muted);
  color: var(--accent);
  font-size: 0.6rem;
  font-weight: 700;
  border-radius: var(--radius-sm);
  border: 1px solid var(--accent-border);
}

.scan__item-badge-icon {
  width: 10px;
  height: 10px;
}

.scan__item-mac {
  font-size: 0.55rem;
  color: var(--text-muted);
}

.scan__item-fw {
  font-size: 0.55rem;
  color: var(--text-muted);
}

.scan__count {
  font-size: var(--font-size-xs);
  color: var(--text-muted);
  font-weight: 600;
  text-align: center;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}
</style>
