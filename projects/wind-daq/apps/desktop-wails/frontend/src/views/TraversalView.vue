<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import TraversalMain from '@components/traversal/TraversalMain.vue'
import TraversalSettings from '@components/traversal/TraversalSettings.vue'
import { useTraversalStore } from '@stores/traversalStore'

withDefaults(
  defineProps<{
    embedded?: boolean
  }>(),
  {
    embedded: false
  }
)

const emit = defineEmits<{ (event: 'back'): void }>()

const traversalStore = useTraversalStore()
const isRecovering = ref(true)
const showTraversalSettings = ref(false)
let isRecoveryActive = true

onMounted(async () => {
  await traversalStore.recoverRendererState()
  if (isRecoveryActive) {
    isRecovering.value = false
  }
})

onBeforeUnmount(() => {
  isRecoveryActive = false
  traversalStore.cancelRecovery()
})

function backFromTraversal(): void {
  window.location.hash = ''
  emit('back')
}

async function onConfigSaved(): Promise<void> {
  showTraversalSettings.value = false
  // 閲嶆柊鍔犺浇閰嶇疆浠ョ‘淇濅富鐣岄潰鏄剧ず鏈€鏂伴厤缃?
  await traversalStore.loadConfig()
}
</script>

<template>
  <div data-test="traversal-shell" class="bg-[color:var(--bg-canvas)] text-[color:var(--text-primary)] font-sans flex flex-col overflow-hidden" :class="embedded ? 'h-full min-h-0' : 'h-screen'">
    <TraversalMain :recovering="isRecovering" @open-settings="showTraversalSettings = true" @back="backFromTraversal" />
    <TraversalSettings
      v-if="showTraversalSettings"
      @close="showTraversalSettings = false"
      @saved="onConfigSaved"
    />
  </div>
</template>

