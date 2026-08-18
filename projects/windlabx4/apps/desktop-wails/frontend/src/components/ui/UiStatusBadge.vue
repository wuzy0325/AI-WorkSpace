<script setup lang="ts">
import { computed } from 'vue'
import { NTag } from 'naive-ui'

const props = withDefaults(
  defineProps<{
    status?: 'idle' | 'connected' | 'acquiring' | 'error' | 'warning'
    pulse?: boolean
  }>(),
  { status: 'idle', pulse: false },
)

const tagType = computed(() => {
  if (props.status === 'error') return 'error'
  if (props.status === 'warning') return 'warning'
  if (props.status === 'connected' || props.status === 'acquiring') return 'success'
  return 'default'
})

const showPulse = computed(() => props.pulse || props.status === 'acquiring')
</script>

<template>
  <NTag :type="tagType" size="small" :bordered="false" round>
    <template #avatar>
      <span class="status-dot" :class="{ 'status-dot--pulse': showPulse }" />
    </template>
    <slot />
  </NTag>
</template>

<style scoped>
.status-dot {
  display: inline-block;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: currentColor;
}

.status-dot--pulse {
  animation: pulse 1.5s ease-in-out infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 1; transform: scale(1); }
  50% { opacity: 0.4; transform: scale(0.7); }
}
</style>
