<script setup lang="ts">
/**
 * 遍历测试顶栏：单行 Header（标题+状态点+控制按钮+配置）。
 *
 * 与 ThreeHoleMain.vue 保持一致：
 *   - 顶栏只保留最高频信息：标题、状态徽章、进度条、已用/剩余时间
 *   - 目标/实际/Ma/V 已足够在侧边栏突出展示，避免顶栏信息过载
 */
import { Pause, Play, Square, Settings } from '@lucide/vue'
import { ref } from 'vue'
import UiButton from '@components/ui/UiButton.vue'
import IconTraversal from '@components/icons/IconTraversal.vue'

defineProps<{
  title: string
  statusText: string
  statusColorToken: string
  automatedRunLabel: string

  hasConfig: boolean
  isStartRequestPending: boolean
  isStarting: boolean
  showRealControls: boolean
  canStart: boolean
  canPause: boolean
  canResume: boolean

  showProgress: boolean
  progressSummary: string
  progressPercent: number
  elapsedText: string
  estimatedRemainingText: string

  labels: {
    configBtn: string
    startRun: string
    travPause: string
    travStop: string
    travResume: string
    elapsed: string
    remaining: string
    progress: string
  }
}>()

const emit = defineEmits<{
  'open-settings': []
  start: []
  pause: []
  resume: []
  stop: []
}>()

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
</script>

<template>
  <div
    data-test="traversal-top-toolbar"
    class="flex items-center justify-between border-b px-4 py-2 flex-shrink-0"
    :style="{ borderColor: 'var(--border-default)', background: 'var(--bg-panel)' }"
  >
    <!-- 左侧：标题 + 状态徽章 + 进度 -->
    <div class="flex items-center gap-4">
      <div class="flex items-center gap-2.5">
        <div
          class="flex h-7 w-7 items-center justify-center rounded-md text-white"
          :style="{ background: 'var(--accent-info)' }"
        >
          <IconTraversal :size="14" />
        </div>
        <div class="leading-tight">
          <h1 class="text-sm font-bold text-[var(--text-primary)]">{{ title }}</h1>
          <p class="text-[11px] text-[var(--text-muted)]">{{ automatedRunLabel }}</p>
        </div>
      </div>

      <!-- 状态徽章 -->
      <span
        class="rounded-full px-2 py-0.5 text-[11px] font-medium"
        :style="{
          backgroundColor: `color-mix(in srgb, var(${statusColorToken}) 15%, transparent)`,
          color: `var(${statusColorToken})`,
        }"
      >{{ statusText }}</span>

      <!-- 进度条 -->
      <div v-if="showProgress" class="flex items-center gap-2 min-w-[160px] w-[220px]">
        <div class="h-1.5 flex-1 overflow-hidden rounded-full bg-[var(--bg-panel-strong)]">
          <div
            class="h-full rounded-full transition-all duration-300"
            :style="{ width: progressPercent + '%', background: 'var(--accent-info)' }"
          ></div>
        </div>
        <span class="text-[11px] font-mono font-bold text-[var(--text-primary)] whitespace-nowrap">{{ progressSummary }} ({{ progressPercent }}%)</span>
      </div>

      <!-- 已用 / 剩余时间 -->
      <div v-if="showProgress" class="flex items-center gap-3 text-[11px]">
        <div class="flex items-center gap-1">
          <span class="text-[var(--text-muted)]">{{ labels.elapsed }}</span>
          <span class="font-mono font-bold text-[var(--text-primary)]">{{ elapsedText }}</span>
        </div>
        <div class="flex items-center gap-1">
          <span class="text-[var(--text-muted)]">{{ labels.remaining }}</span>
          <span class="font-mono font-bold text-[var(--text-primary)]">{{ estimatedRemainingText }}</span>
        </div>
      </div>
    </div>

    <!-- 右侧：控制按钮 + 配置 -->
    <div class="flex items-center gap-2">
      <template v-if="showRealControls">
        <UiButton
          v-if="canStart && !isStartRequestPending"
          ref="startButtonRef"
          variant="primary"
          size="sm"
          :disabled="!hasConfig"
          @click="emit('start')"
        >
          <Play class="h-3.5 w-3.5" />
          <span class="ml-1">{{ labels.startRun }}</span>
        </UiButton>
        <UiButton
          v-else-if="isStartRequestPending || isStarting"
          variant="primary"
          size="sm"
          disabled
        >
          <Play class="h-3.5 w-3.5" />
          <span class="ml-1">{{ labels.startRun }}</span>
        </UiButton>
        <template v-else-if="canPause">
          <UiButton variant="warning" size="sm" @click="emit('pause')">
            <Pause class="h-3.5 w-3.5" />
            <span class="ml-1">{{ labels.travPause }}</span>
          </UiButton>
          <UiButton variant="danger" size="sm" @click="emit('stop')">
            <Square class="h-3.5 w-3.5" />
            <span class="ml-1">{{ labels.travStop }}</span>
          </UiButton>
        </template>
        <template v-else-if="canResume">
          <UiButton variant="primary" size="sm" @click="emit('resume')">
            <Play class="h-3.5 w-3.5" />
            <span class="ml-1">{{ labels.travResume }}</span>
          </UiButton>
          <UiButton variant="danger" size="sm" @click="emit('stop')">
            <Square class="h-3.5 w-3.5" />
            <span class="ml-1">{{ labels.travStop }}</span>
          </UiButton>
        </template>
      </template>

      <UiButton variant="secondary" size="sm" @click="emit('open-settings')">
        <Settings class="h-3.5 w-3.5" />
        <span class="ml-1">{{ labels.configBtn }}</span>
      </UiButton>
    </div>
  </div>
</template>
