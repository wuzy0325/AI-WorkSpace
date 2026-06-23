<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import TraversalMain from '@components/traversal/TraversalMain.vue'
import TraversalSettings from '@components/traversal/TraversalSettings.vue'
import UiLoadingState from '@components/ui/UiLoadingState.vue'
import UiErrorState from '@components/ui/UiErrorState.vue'
import UiButton from '@components/ui/UiButton.vue'
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
const recoveryError = ref('')
const showTraversalSettings = ref(false)
let isRecoveryActive = true

onMounted(async () => {
  try {
    await traversalStore.recoverRendererState()
  } catch (err) {
    recoveryError.value = err instanceof Error ? err.message : '恢复状态失败'
  } finally {
    if (isRecoveryActive) {
      isRecovering.value = false
    }
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

async function retryRecovery(): Promise<void> {
  recoveryError.value = ''
  isRecovering.value = true
  try {
    await traversalStore.recoverRendererState()
  } catch (err) {
    recoveryError.value = err instanceof Error ? err.message : '重试恢复失败'
  } finally {
    isRecovering.value = false
  }
}

async function onConfigSaved(): Promise<void> {
  showTraversalSettings.value = false
  await traversalStore.loadConfig()
}
</script>

<template>
  <div data-test="traversal-shell" class="flex h-full min-h-0 flex-col overflow-hidden font-sans" style="background:var(--bg-canvas);color:var(--text-primary)">
    <UiLoadingState v-if="isRecovering" loading text="正在恢复移位测试状态..." />
    <UiErrorState v-else-if="recoveryError" title="恢复失败" :message="recoveryError">
      <template #action>
        <UiButton variant="secondary" size="sm" @click="retryRecovery">重试</UiButton>
      </template>
    </UiErrorState>
    <template v-else>
      <TraversalMain :recovering="false" @open-settings="showTraversalSettings = true" @back="backFromTraversal" />
      <TraversalSettings :show="showTraversalSettings" @close="showTraversalSettings = false" @saved="onConfigSaved" />
    </template>
  </div>
</template>

