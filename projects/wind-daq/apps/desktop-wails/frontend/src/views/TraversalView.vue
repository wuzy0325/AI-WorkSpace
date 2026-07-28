<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import TraversalMain from '@components/traversal/TraversalMain.vue'
import TraversalSettings from '@components/traversal/TraversalSettings.vue'
import DualTraversalMain from '@components/traversal/dual/DualTraversalMain.vue'
import DualTraversalSettings from '@components/traversal/dual/DualTraversalSettings.vue'
import UiLoadingState from '@components/ui/UiLoadingState.vue'
import UiErrorState from '@components/ui/UiErrorState.vue'
import UiButton from '@components/ui/UiButton.vue'
import { useTraversalStore } from '@stores/traversalStore'
import { useDualTraversalStore } from '@stores/dualTraversalStore'
import { useTraversalModeStore } from '@stores/traversalModeStore'
import type { ProbeId } from '@shared/types/traversal'

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
const dualTraversalStore = useDualTraversalStore()
const traversalModeStore = useTraversalModeStore()

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

// 模式状态由全局 traversalModeStore 管理（入口在 MainDashboardView 侧边栏子菜单）。
// TraversalView 仅消费 mode 决定渲染分支，不再持有顶部模式开关。
const mode = computed(() => traversalModeStore.mode)

// ---------------------------------------------------------------------------
// Dual 模式配置对话框（每 probe 独立入口）
// ---------------------------------------------------------------------------
const showDualSettings = ref(false)
const dualSettingsProbeId = ref<ProbeId>('probe1')

function onDualOpenSettings(probeId: ProbeId): void {
  dualSettingsProbeId.value = probeId
  showDualSettings.value = true
}

function onDualSettingsClose(): void {
  showDualSettings.value = false
}

async function onDualSettingsSaved(): Promise<void> {
  showDualSettings.value = false
  // 重新加载该 probe 的配置确保 store 同步
  await dualTraversalStore.loadConfig(dualSettingsProbeId.value)
}

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
  // 离开遍历视图时清理 dual 模式订阅/timer（spec FR1）
  void traversalModeStore.cleanupOnLeave()
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
      <!-- 主区域：根据模式渲染（模式选择入口在侧边栏「遍历测试」子菜单） -->
      <div class="flex flex-1 min-h-0 overflow-hidden">
        <TraversalMain
          v-if="mode === 'single'"
          :recovering="false"
          @open-settings="openSettings"
          @back="backFromTraversal"
        />
        <DualTraversalMain
          v-else
          @open-settings="onDualOpenSettings"
          @back="backFromTraversal"
        />
      </div>

      <!-- single 模式配置对话框 -->
      <TraversalSettings
        v-if="mode === 'single'"
        :show="showTraversalSettings"
        :initial-step="traversalInitialStep"
        @close="showTraversalSettings = false"
        @saved="onConfigSaved"
      />

      <!-- dual 模式配置对话框（每 probe 独立入口） -->
      <DualTraversalSettings
        v-else
        :show="showDualSettings"
        :probe-id="dualSettingsProbeId"
        @close="onDualSettingsClose"
        @saved="onDualSettingsSaved"
      />
    </template>
  </div>
</template>

<style scoped>
/* 模式开关已迁移到侧边栏「遍历测试」子菜单（MainDashboardView），本视图不再需要顶部样式 */
</style>
