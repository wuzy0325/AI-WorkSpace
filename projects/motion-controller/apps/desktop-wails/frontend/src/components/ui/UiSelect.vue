<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

/** 选择器选项类型 */
export type UiSelectOption<T extends string | number = string> = {
  value: T
  label: string
  disabled?: boolean
}

const props = withDefaults(
  defineProps<{
    modelValue: string | number
    options: UiSelectOption<string | number>[]
    disabled?: boolean
    placeholder?: string
    widthClass?: string
    compact?: boolean
  }>(),
  {
    disabled: false,
    placeholder: '请选择...',
    widthClass: 'w-full',
    compact: false
  }
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: string | number): void
}>()

const open = ref(false)
const buttonRef = ref<HTMLButtonElement | null>(null)
const menuRef = ref<HTMLDivElement | null>(null)
const menuStyle = ref<Record<string, string>>({})

/** 当前选中的选项 */
const selected = computed<UiSelectOption<string | number> | null>(() =>
  props.options.find((o) => o.value === props.modelValue) ?? null
)

function close(): void {
  open.value = false
}

function toggle(): void {
  if (props.disabled) return
  open.value = !open.value
}

function selectOption(opt: UiSelectOption<string | number>): void {
  if (opt.disabled) return
  emit('update:modelValue', opt.value)
  close()
  nextTick(() => buttonRef.value?.focus())
}

function onKeydown(e: KeyboardEvent): void {
  if (props.disabled) return
  if (e.key === 'Escape') {
    e.preventDefault()
    close()
    return
  }
  if (e.key === 'Enter' || e.key === ' ') {
    if (e.target === buttonRef.value) {
      e.preventDefault()
      toggle()
    }
  }
}

/** 点击外部关闭下拉菜单 */
function onDocPointerDown(e: PointerEvent): void {
  const target = e.target as Node | null
  if (!target) return
  if (buttonRef.value?.contains(target)) return
  if (menuRef.value?.contains(target)) return
  close()
}

/** 更新下拉菜单位置 */
function updateMenuPosition(): void {
  const btn = buttonRef.value
  if (!btn) return
  const rect = btn.getBoundingClientRect()
  menuStyle.value = {
    position: 'fixed',
    top: `${rect.bottom + 4}px`,
    left: `${rect.left}px`,
    width: `${rect.width}px`,
    'z-index': '50'
  }
}

watch(open, async (v) => {
  if (!v) {
    menuStyle.value = {}
    return
  }
  await nextTick()
  updateMenuPosition()
  const menu = menuRef.value
  const selectedBtn = menu?.querySelector<HTMLButtonElement>('[data-selected="true"]')
  selectedBtn?.focus()
})

// [FIX] 事件监听器在 onMounted 中注册，避免模块初始化时注册导致内存泄漏
onMounted(() => {
  document.addEventListener('pointerdown', onDocPointerDown, { capture: true })
})

onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', onDocPointerDown, { capture: true } as EventListenerOptions)
})
</script>

<template>
  <div :class="['relative', widthClass]" @keydown="onKeydown">
    <button
      ref="buttonRef"
      type="button"
      :disabled="disabled"
      class="ui-select"
      :class="props.compact ? 'ui-select--compact' : ''"
      @click="toggle"
    >
      <span class="min-w-0 flex-1 truncate">
        <span v-if="selected" class="text-[color:var(--text-primary)]">{{ selected.label }}</span>
        <span v-else class="text-[color:var(--text-muted)]">{{ placeholder }}</span>
      </span>
      <svg class="ui-select__icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        <path d="M6 9l6 6 6-6" />
      </svg>
    </button>

    <Teleport to="body">
      <div v-if="open" ref="menuRef" class="ui-select__menu" role="listbox" :style="menuStyle">
        <div class="max-h-60 overflow-auto p-1">
          <button
            v-for="opt in options"
            :key="String(opt.value)"
            type="button"
            class="ui-select__option"
            :disabled="!!opt.disabled"
            :data-selected="opt.value === modelValue"
            :class="opt.value === modelValue ? 'ui-select__option--selected' : ''"
            @click="selectOption(opt)"
          >
            <span class="truncate">{{ opt.label }}</span>
            <span v-if="opt.value === modelValue" class="text-[color:var(--accent-success)]">✓</span>
          </button>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.ui-select {
  display: flex;
  width: 100%;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
  min-height: 38px;
  padding: 0 var(--space-3);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  background: color-mix(in srgb, var(--bg-panel) 88%, transparent);
  color: var(--text-primary);
  font-size: var(--font-size-sm);
  text-align: left;
  cursor: pointer;
  transition:
    border-color var(--motion-fast) var(--easing-standard),
    background-color var(--motion-fast) var(--easing-standard),
    box-shadow var(--motion-fast) var(--easing-standard);
}

.ui-select:hover:not(:disabled) {
  border-color: var(--border-strong);
  background: color-mix(in srgb, var(--bg-panel-strong) 82%, transparent);
}

.ui-select:focus-visible {
  outline: none;
  border-color: var(--accent-success);
  box-shadow: 0 0 0 1px var(--focus-ring), 0 0 0 3px var(--focus-ring-soft);
}

.ui-select:disabled {
  cursor: not-allowed;
  opacity: 0.6;
}

.ui-select__icon {
  width: 1rem;
  height: 1rem;
  flex: none;
  color: var(--text-muted);
  transition: color var(--motion-fast) var(--easing-standard);
}

.ui-select:hover:not(:disabled) .ui-select__icon {
  color: var(--text-secondary);
}

/* 下拉菜单 */
.ui-select__menu {
  overflow: hidden;
  border: 1px solid var(--border-default);
  border-radius: var(--radius-lg);
  background: color-mix(in srgb, var(--bg-panel-strong) 96%, transparent);
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.3);
  outline: none;
  backdrop-filter: blur(var(--blur-dropdown));
}

.ui-select__option {
  display: flex;
  width: 100%;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-md);
  color: var(--text-primary);
  font-size: var(--font-size-sm);
  cursor: pointer;
  transition:
    background-color var(--motion-fast) var(--easing-standard),
    color var(--motion-fast) var(--easing-standard);
}

.ui-select__option:hover:not(:disabled),
.ui-select__option:focus-visible {
  background: color-mix(in srgb, var(--bg-panel) 72%, var(--accent-success));
  outline: none;
}

.ui-select__option:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.ui-select__option--selected {
  background: color-mix(in srgb, var(--accent-success) 18%, transparent);
  color: var(--text-primary);
}

/* 紧凑模式 */
.ui-select--compact {
  min-height: 28px;
  padding: 0 var(--space-2);
  border-radius: var(--radius-sm);
  font-size: 11px;
}

.ui-select--compact .ui-select__icon {
  width: 0.875rem;
  height: 0.875rem;
}
</style>
