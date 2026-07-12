<script setup lang="ts">
/**
 * 遍历测试顶栏：单行 Header（标题+状态点+控制按钮+配置）。
 *
 * 与 ThreeHoleMain.vue 保持一致：
 *   - 顶栏只保留最高频信息：标题、状态点、进度条、已用/剩余时间
 *   - 目标/实际/Ma/V 已足够在侧边栏突出展示，避免顶栏信息过载
 *
 * 视觉风格（2026-07）：
 *   - 极简信息栏，去掉卡片化和彩色装饰
 *   - 标题图标改为浅色描边，状态用圆点+文字，用分隔线划分区域
 *   - 整体更轻盈、更现代，降低视觉重量
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
    class="flex items-center justify-between border-b px-5 py-2.5 flex-shrink-0"
    :style="{ borderColor: 'var(--border-default)', background: 'var(--bg-panel)' }"
  >
    <!-- 左侧：标题 + 状态指示 + 进度信息 -->
    <div class="flex items-center gap-4">
      <!-- 标题块：图标改为浅色描边，降低视觉冲击 -->
      <div class="flex items-center gap-2.5">
        <div
          class="flex h-7 w-7 items-center justify-center rounded-md"
          :style="{
            background: 'var(--bg-panel-strong)',
            color: 'var(--accent-primary)',
            border: '1px solid var(--border-default)',
          }"
        >
          <IconTraversal :size="14" />
        </div>
        <div class="leading-tight">
          <h1 class="text-sm font-bold text-[var(--text-primary)]">{{ title }}</h1>
          <p class="text-[11px] text-[var(--text-muted)]">{{ automatedRunLabel }}</p>
        </div>
      </div>

      <!-- 分隔线 -->
      <div class="h-4 w-px" :style="{ background: 'var(--border-default)' }"></div>

      <!-- 状态指示：圆点 + 文字，取代徽章 -->
      <div class="flex items-center gap-1.5">
        <span
          class="h-2 w-2 rounded-full"
          :style="{ background: `var(${statusColorToken})` }"
        ></span>
        <span class="text-xs font-medium" :style="{ color: `var(${statusColorToken})` }">{{ statusText }}</span>
      </div>

      <!-- 进度条 + 进度摘要 -->
      <template v-if="showProgress">
        <div class="h-4 w-px" :style="{ background: 'var(--border-default)' }"></div>

        <div class="flex items-center gap-2 min-w-[140px]">
          <div class="h-1.5 flex-1 overflow-hidden rounded-full bg-[var(--bg-panel-strong)]">
            <div
              class="h-full rounded-full transition-all duration-300"
              :style="{ width: progressPercent + '%', background: 'var(--accent-primary)' }"
            ></div>
          </div>
          <span class="text-[11px] font-mono font-bold text-[var(--text-primary)] whitespace-nowrap">{{ progressSummary }}</span>
        </div>

        <!-- 已用 / 剩余时间 -->
        <div class="flex items-center gap-3 text-[11px]">
          <div class="flex items-center gap-1">
            <span class="text-[var(--text-muted)]">{{ labels.elapsed }}</span>
            <span class="font-mono font-bold text-[var(--text-primary)]">{{ elapsedText }}</span>
          </div>
          <div class="flex items-center gap-1">
            <span class="text-[var(--text-muted)]">{{ labels.remaining }}</span>
            <span class="font-mono font-bold text-[var(--text-primary)]">{{ estimatedRemainingText }}</span>
          </div>
        </div>
      </template>
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