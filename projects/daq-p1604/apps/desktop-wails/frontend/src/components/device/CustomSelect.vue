<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { ChevronDown } from '@lucide/vue'

export interface SelectOption {
  value: string | number
  label: string
}

const props = withDefaults(defineProps<{
  modelValue: string | number
  options: SelectOption[]
  disabled?: boolean
  placeholder?: string
}>(), {
  disabled: false,
  placeholder: '',
})

const emit = defineEmits<{
  (e: 'update:modelValue', value: string | number): void
}>()

const isOpen = ref(false)
const triggerRef = ref<HTMLElement | null>(null)
const dropdownRef = ref<HTMLElement | null>(null)
const dropdownStyle = ref<Record<string, string>>({})

// 当前选中项的标签
const selectedLabel = computed(() => {
  const opt = props.options.find(o => o.value === props.modelValue)
  return opt ? opt.label : props.placeholder
})

/** 根据触发器位置计算下拉菜单的 fixed 定位 */
function updateDropdownPosition() {
  if (!isOpen.value || !triggerRef.value) {
    dropdownStyle.value = {}
    return
  }
  const rect = triggerRef.value.getBoundingClientRect()
  dropdownStyle.value = {
    position: 'fixed',
    left: `${rect.left}px`,
    top: `${rect.bottom + 4}px`,
    minWidth: `${rect.width}px`,
    zIndex: '9999',
  }
}

function toggle() {
  if (props.disabled) return
  isOpen.value = !isOpen.value
  if (isOpen.value) {
    nextTick(updateDropdownPosition)
  }
}

function selectOption(opt: SelectOption) {
  emit('update:modelValue', opt.value)
  isOpen.value = false
}

// 点击外部关闭下拉
function onClickOutside(e: MouseEvent) {
  if (!isOpen.value) return
  const target = e.target as Node
  if (
    triggerRef.value?.contains(target) ||
    dropdownRef.value?.contains(target)
  ) {
    return
  }
  isOpen.value = false
}

// 打开时监听滚动和窗口变化，更新下拉位置
function onScroll() {
  if (isOpen.value) updateDropdownPosition()
}

function onResize() {
  if (isOpen.value) updateDropdownPosition()
}

watch(isOpen, (val) => {
  if (val) {
    window.addEventListener('scroll', onScroll, true)
    window.addEventListener('resize', onResize)
  } else {
    window.removeEventListener('scroll', onScroll, true)
    window.removeEventListener('resize', onResize)
  }
})

onMounted(() => {
  document.addEventListener('mousedown', onClickOutside, true)
})

onBeforeUnmount(() => {
  document.removeEventListener('mousedown', onClickOutside, true)
  window.removeEventListener('scroll', onScroll, true)
  window.removeEventListener('resize', onResize)
})
</script>

<template>
  <div class="cselect" :class="{ 'cselect--disabled': disabled }">
    <!-- 触发器 -->
    <button
      ref="triggerRef"
      type="button"
      class="cselect__trigger"
      :disabled="disabled"
      @click="toggle"
    >
      <span class="cselect__value">{{ selectedLabel }}</span>
      <ChevronDown class="cselect__arrow" :class="{ 'cselect__arrow--open': isOpen }" />
    </button>

    <!-- 下拉选项列表：使用 Teleport 到 body，fixed 定位不受父容器 overflow:hidden 影响 -->
    <Teleport to="body">
      <div
        v-if="isOpen"
        ref="dropdownRef"
        class="cselect__dropdown"
        :style="dropdownStyle"
      >
        <div
          v-for="opt in options"
          :key="opt.value"
          class="cselect__option"
          :class="{ 'cselect__option--selected': opt.value === modelValue }"
          @click="selectOption(opt)"
        >
          {{ opt.label }}
        </div>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.cselect {
  position: relative;
  min-width: 7rem;
}

.cselect--disabled {
  opacity: 0.5;
  pointer-events: none;
}

.cselect__trigger {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  padding: 0.45rem 1.75rem 0.45rem 0.75rem;
  font-size: var(--font-size-sm);
  font-weight: 600;
  background: var(--bg-input);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  color: var(--text-primary);
  cursor: pointer;
  transition: border-color var(--motion-fast) var(--easing-standard),
              box-shadow var(--motion-fast) var(--easing-standard);
  text-align: left;
}

.cselect__trigger:hover {
  border-color: var(--border-hover);
}

.cselect__trigger:focus {
  outline: none;
  border-color: var(--accent);
  box-shadow: 0 0 0 2px var(--accent-muted);
}

.cselect__value {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.cselect__arrow {
  width: 14px;
  height: 14px;
  color: var(--text-muted);
  flex-shrink: 0;
  transition: transform var(--motion-fast) var(--easing-standard);
}

.cselect__arrow--open {
  transform: rotate(180deg);
}

.cselect__dropdown {
  background: var(--bg-panel);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-lg);
  max-height: 240px;
  overflow-y: auto;
  padding: 0.25rem;
}

.cselect__option {
  padding: 0.4rem 0.75rem;
  font-size: var(--font-size-sm);
  font-weight: 600;
  color: var(--text-primary);
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: background var(--motion-fast) var(--easing-standard);
}

.cselect__option:hover {
  background: var(--btn-bg-hover);
}

.cselect__option--selected {
  color: var(--accent);
  background: var(--accent-muted);
}
</style>
