<script setup lang="ts">
/**
 * Top toolbar for the traversal page: title + status + progress summary +
 * control button group (config / simulation / start / pause / resume / stop / sim-run).
 *
 * The button visibility logic lives here so the parent stays focused on state.
 * Phase B: tokenised colors.
 */
import { ref } from 'vue'
import { AlertTriangle, Clock, FlaskConical, Pause, Play, Settings, Square } from '@lucide/vue'
import UiButton from '@components/ui/UiButton.vue'
import IconTraversal from '@components/icons/IconTraversal.vue'

defineProps<{
  /** Display title and status text are localised by the parent. */
  title: string
  statusText: string
  statusDotClass: string
  automatedRunLabel: string

  /** Progress summary visible only when running/paused/completed. */
  showProgress: boolean
  progressSummary: string
  progressPercent: number
  estimatedRemainingText: string

  /** Validation warnings badge. */
  validationWarnings: readonly string[] | undefined

  /** Toolbar mode flags. */
  hasConfig: boolean
  isStartRequestPending: boolean
  isStarting: boolean
  isSimulationMode: boolean
  /** Real (non-simulation) controls visible? */
  showRealControls: boolean
  canStart: boolean
  canPause: boolean
  canResume: boolean

  /** Simulation progress string (e.g. "12 / 81"). */
  simulationProgress: string

  labels: {
    configBtn: string
    startRun: string
    travPause: string
    travStop: string
    travResume: string
    travSimRun: string
    travSimProgressTemplate: string  // contains "{progress}"
  }
}>()

const emit = defineEmits<{
  'open-settings': []
  start: []
  pause: []
  resume: []
  stop: []
  'run-simulation': []
  'cancel-simulation': []
}>()

/**
 * Start-button ref + imperative `focusStart()` exposed for the parent's
 * focus-restore flow after the start-confirm dialog closes. Encapsulating
 * the DOM access here keeps the parent free of `$el` reach-ins; we read
 * the public instance's $el lazily via getCurrentInstance — works in all
 * Vue 3.x versions because we go through the component proxy explicitly.
 */
const startButtonRef = ref<InstanceType<typeof UiButton> | null>(null)

function focusStart(): boolean {
  const inst = startButtonRef.value as unknown as { $el?: HTMLElement } | null
  const el = inst?.$el
  if (el && typeof el.focus === 'function') {
    el.focus()
    return true
  }
  return false
}

defineExpose({ focusStart })

function formatSimProgress(template: string, progress: string): string {
  return template.replace('{progress}', progress)
}
</script>

<template>
  <div
    data-test="traversal-top-toolbar"
    class="flex shrink-0 items-center justify-between border-b px-4 py-2"
    :style="{ borderColor: 'var(--border-default)', background: 'var(--bg-panel)' }"
  >
    <!-- 左侧：标题区 -->
    <div class="flex items-center gap-2.5">
      <div
        class="flex h-7 w-7 items-center justify-center rounded-md text-white"
        :style="{ background: 'var(--accent-info)' }"
      >
        <IconTraversal :size="14" />
      </div>
      <div class="leading-tight">
        <h1 class="text-sm font-semibold text-[var(--text-primary)]">{{ title }}</h1>
        <div class="flex items-center gap-1.5 mt-0.5">
          <span class="flex h-1.5 w-1.5 rounded-full" :class="statusDotClass"></span>
          <p class="text-[11px] text-[var(--text-muted)]">{{ statusText }} · {{ automatedRunLabel }}</p>
        </div>
      </div>
    </div>

    <!-- 右侧：控制区 -->
    <div class="flex items-center gap-2">
      <!-- 进度摘要 -->
      <div
        v-if="showProgress"
        class="flex items-center gap-2 rounded-md border px-2.5 py-1"
        :style="{ borderColor: 'var(--border-default)', background: 'var(--bg-panel-strong)' }"
      >
        <span class="font-mono text-xs font-semibold text-[var(--accent-info)]">{{ progressSummary }}</span>
        <span class="text-[11px] text-[var(--text-muted)]">({{ progressPercent }}%)</span>
        <div
          class="h-1 w-14 overflow-hidden rounded-full"
          :style="{ background: 'color-mix(in srgb, var(--border-default) 70%, transparent)' }"
        >
          <div
            class="h-full rounded-full transition-all duration-300"
            :style="{ width: `${progressPercent}%`, background: 'var(--accent-info)' }"
          ></div>
        </div>
        <template v-if="estimatedRemainingText !== '--'">
          <div class="h-3 w-px" :style="{ background: 'var(--border-default)' }"></div>
          <Clock class="h-3 w-3 text-[var(--text-muted)]" />
          <span class="font-mono text-[11px] text-[var(--text-muted)]">{{ estimatedRemainingText }}</span>
        </template>
      </div>

      <!-- 验证警告徽标 -->
      <div
        v-if="validationWarnings?.length"
        class="flex items-center gap-1 rounded-md border px-2 py-1"
        :style="{
          borderColor: 'color-mix(in srgb, var(--state-warning) 35%, transparent)',
          background: 'color-mix(in srgb, var(--state-warning) 10%, transparent)',
        }"
        :title="validationWarnings.join('\n')"
      >
        <AlertTriangle class="h-3 w-3 text-[var(--state-warning)]" />
        <span class="text-[11px] font-semibold text-[var(--state-warning)]">{{ validationWarnings.length }}</span>
      </div>

      <!-- 配置按钮 -->
      <UiButton
        variant="secondary" size="sm"
        @click="emit('open-settings')"
      >
        <template #icon>
          <Settings class="h-3.5 w-3.5" />
        </template>
        {{ labels.configBtn }}
      </UiButton>

      <div class="h-4 w-px" :style="{ background: 'var(--border-default)' }"></div>

      <div class="flex items-center gap-1.5">
        <!-- 模拟模式控制 -->
        <template v-if="isSimulationMode">
          <div
            class="flex items-center gap-1.5 rounded-md border px-2 py-1"
            :style="{
              borderColor: 'color-mix(in srgb, var(--accent-info) 35%, transparent)',
              background: 'color-mix(in srgb, var(--accent-info) 10%, transparent)',
            }"
          >
            <FlaskConical class="h-3 w-3 text-[var(--accent-info)]" />
            <span class="text-[11px] font-semibold text-[var(--accent-info)]">{{ formatSimProgress(labels.travSimProgressTemplate, simulationProgress) }}</span>
          </div>
          <UiButton variant="danger" size="sm" @click="emit('cancel-simulation')">
            <template #icon>
              <Square class="h-3.5 w-3.5 fill-current" />
            </template>
            {{ labels.travStop }}
          </UiButton>
        </template>

        <!-- 实际控制 -->
        <template v-else-if="showRealControls">
          <UiButton
            v-if="canStart && !isStartRequestPending"
            ref="startButtonRef"
            variant="primary" size="sm"
            :disabled="!hasConfig"
            @click="emit('start')"
          >
            <template #icon>
              <Play class="h-3.5 w-3.5 fill-current" />
            </template>
            {{ labels.startRun }}
          </UiButton>
          <UiButton
            v-else-if="isStartRequestPending || isStarting"
            variant="primary" size="sm"
            disabled
          >
            <template #icon>
              <Play class="h-3.5 w-3.5 fill-current" />
            </template>
            {{ labels.startRun }}
          </UiButton>
          <template v-else-if="canPause">
            <UiButton variant="warning" size="sm" @click="emit('pause')">
              <template #icon>
                <Pause class="h-3.5 w-3.5 fill-current" />
              </template>
              {{ labels.travPause }}
            </UiButton>
            <UiButton variant="danger" size="sm" @click="emit('stop')">
              <template #icon>
                <Square class="h-3.5 w-3.5 fill-current" />
              </template>
              {{ labels.travStop }}
            </UiButton>
          </template>
          <template v-else-if="canResume">
            <UiButton variant="primary" size="sm" @click="emit('resume')">
              <template #icon>
                <Play class="h-3.5 w-3.5 fill-current" />
              </template>
              {{ labels.travResume }}
            </UiButton>
            <UiButton variant="danger" size="sm" @click="emit('stop')">
              <template #icon>
                <Square class="h-3.5 w-3.5 fill-current" />
              </template>
              {{ labels.travStop }}
            </UiButton>
          </template>

          <!-- 模拟运行按钮 -->
          <template v-if="canStart && !isStartRequestPending">
            <div class="h-4 w-px" :style="{ background: 'var(--border-default)' }"></div>
            <UiButton
              quaternary size="sm"
              @click="emit('run-simulation')"
            >
              <template #icon>
                <FlaskConical class="h-3.5 w-3.5" />
              </template>
              {{ labels.travSimRun }}
            </UiButton>
          </template>
        </template>
      </div>
    </div>
  </div>
</template>
