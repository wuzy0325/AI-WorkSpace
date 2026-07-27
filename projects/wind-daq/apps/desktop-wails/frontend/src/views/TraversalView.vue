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
/**
 * TraversalSettings 打开时定位的步骤索引(0=通道, 1=PRB, 2=布点, 3=摘要)。
 * 默认 0;由 "PRB 未加载" 状态条点击 navigate-to-prb 触发时设为 1,
 * 让用户直接看到 PRB 配置面板而非通道配置,降低排障路径成本。
 */
const traversalInitialStep = ref(0)
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
  // 关闭后重置 initialStep,避免下次普通打开仍跳到 PRB 步骤
  traversalInitialStep.value = 0
  await traversalStore.loadConfig()
}

/**
 * 打开 TraversalSettings 对话框。
 * @param step 可选,定位到的步骤索引(0=通道, 1=PRB, 2=布点, 3=摘要);
 *             缺省时为 0。"PRB 未加载" 状态条点击时传 1 直达 PRB 步骤。
 *             类型与 TraversalMain 的 emit openSettings: [step?: number] 对齐。
 */
function openSettings(step?: number): void {
  traversalInitialStep.value = step ?? 0
  showTraversalSettings.value = true
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
      <TraversalMain :recovering="false" @open-settings="openSettings" @back="backFromTraversal" />
      <TraversalSettings :show="showTraversalSettings" :initial-step="traversalInitialStep" @close="showTraversalSettings = false" @saved="onConfigSaved" />
    </template>
  </div>
</template>

