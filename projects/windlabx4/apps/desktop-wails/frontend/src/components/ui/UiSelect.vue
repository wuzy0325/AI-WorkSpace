<script setup lang="ts">
import { computed, h } from 'vue'
import { NSelect } from 'naive-ui'
import type { SelectOption } from 'naive-ui'

const props = withDefaults(
  defineProps<{
    modelValue?: string
    options?: Array<{ value: string; label: string; title?: string }>
    placeholder?: string
    size?: 'sm' | 'md' | 'lg'
    disabled?: boolean
    ariaLabel?: string
    dataTest?: string
    /**
     * value 在 options 中找不到匹配项时的回退行为
     *
     * - false：不显示回退内容，直接展示 placeholder（用于"原设备已删除"场景，避免显示原始 UUID）
     * - 默认 true：显示 value 字符串本身（naive-ui NSelect 默认行为）
     */
    fallback?: boolean
  }>(),
  { modelValue: '', options: () => [], placeholder: '', size: 'sm', disabled: false, ariaLabel: '', dataTest: '', fallback: true },
)

const naiveSize = computed(() => {
  if (props.size === 'sm') return 'small'
  if (props.size === 'lg') return 'large'
  return 'medium'
})

// 当前选中的文本：用于触发器 hover 提示，解决选项/占位符被截断后无法识别的问题。
// 选项可带 title（如 "K（0~1200 ℃）"），优先级高于 label。
const selectedLabel = computed(() => {
  if (!props.modelValue) return props.placeholder || ''
  const found = props.options.find(o => o.value === props.modelValue)
  return found?.title ?? found?.label ?? (props.fallback ? props.modelValue : props.placeholder || '')
})

// 下拉选项渲染：给每个选项加原生 title，鼠标悬停即可看到完整名称
function renderLabel(option: SelectOption) {
  // 项目内 options 的 label 均为字符串；非字符串时回退到 value，避免调用签名不确定的渲染函数
  const text: string = typeof option.label === 'string' ? option.label : String(option.value ?? '')
  const title: string = typeof (option as { title?: unknown }).title === 'string'
    ? (option as { title?: string }).title as string
    : text
  return h('span', { title }, text)
}

// 菜单使用自定义类，便于通过全局样式限制最大宽度（菜单通过 portal 渲染到 body，无法用 scoped 样式）
const menuProps = computed(() => ({ class: 'ui-select-menu' }))

const emit = defineEmits<{ (e: 'update:modelValue', v: string): void }>()
</script>

<template>
  <!-- 用包装层承载 title，使整个选择框 hover 时都能提示当前值/占位符 -->
  <div class="ui-select" :title="selectedLabel">
    <NSelect
      :value="modelValue || null"
      :options="options"
      :placeholder="placeholder"
      :size="naiveSize"
      :disabled="disabled"
      :aria-label="ariaLabel || undefined"
      :data-test="dataTest || undefined"
      :fallback="fallback"
      :consistent-menu-width="false"
      :menu-props="menuProps"
      :render-label="renderLabel"
      @update:value="emit('update:modelValue', $event ?? '')"
    />
  </div>
</template>

<style scoped>
/* 包装层仅用于承载 title 提示；不显式设置 width，避免与消费方传入的宽度类
   （如 sel-w150 / sel-w80）发生特异性冲突。NSelect 根元素 .n-select 自身
   已带 width:100%（naive-ui 默认），会自动填满本包装层。 */
.ui-select {
  display: block;
}
</style>

<style>
/* ui-select 的下拉菜单通过 portal 挂载到 body，必须使用全局样式；
   限制最大宽度避免设备名等长选项在窄触发器下被截断，同时防止超长选项撑爆视口。 */
.ui-select-menu {
  max-width: min(90vw, 480px) !important;
}
</style>
