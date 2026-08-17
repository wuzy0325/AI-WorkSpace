<script setup lang="ts">
import { ref } from 'vue'

const props = withDefaults(
  defineProps<{
    content?: string
    position?: 'top' | 'bottom' | 'left' | 'right'
  }>(),
  { position: 'top' },
)

const visible = ref(false)
let timer: ReturnType<typeof setTimeout> | null = null

function show() {
  if (timer) clearTimeout(timer)
  timer = setTimeout(() => { visible.value = true }, 200)
}

function hide() {
  if (timer) clearTimeout(timer)
  timer = setTimeout(() => { visible.value = false }, 100)
}
</script>

<template>
  <span class="ui-tooltip-wrapper" @mouseenter="show" @mouseleave="hide">
    <slot />
    <Transition name="tooltip">
      <span
        v-if="visible && content"
        class="ui-tooltip"
        :class="`ui-tooltip--${position}`"
        role="tooltip"
      >
        {{ content }}
      </span>
    </Transition>
  </span>
</template>

<style scoped>
.ui-tooltip-wrapper {
  position: relative;
  display: inline-flex;
  align-items: center;
}

.ui-tooltip {
  position: absolute;
  z-index: 100;
  padding: 0.375rem 0.75rem;
  border-radius: 0.5rem;
  background: var(--bg-panel-strong);
  border: 1px solid var(--border-default);
  color: var(--text-secondary);
  font-size: var(--font-size-2xs);
  font-weight: 600;
  white-space: nowrap;
  pointer-events: none;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
}

.ui-tooltip--top {
  bottom: calc(100% + 6px);
  left: 50%;
  transform: translateX(-50%);
}

.ui-tooltip--bottom {
  top: calc(100% + 6px);
  left: 50%;
  transform: translateX(-50%);
}

.ui-tooltip--left {
  right: calc(100% + 6px);
  top: 50%;
  transform: translateY(-50%);
}

.ui-tooltip--right {
  left: calc(100% + 6px);
  top: 50%;
  transform: translateY(-50%);
}

.tooltip-enter-active,
.tooltip-leave-active {
  transition: opacity 0.15s ease, transform 0.15s ease;
}

.tooltip-enter-from,
.tooltip-leave-to {
  opacity: 0;
  transform: translateX(-50%) translateY(4px);
}
</style>
