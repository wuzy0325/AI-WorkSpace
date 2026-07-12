<script setup lang="ts">
/**
 * Start-test confirmation dialog: precondition checks + summary + actions.
 * Phase C: switched to UiDialog primitive. Focus trap is handled by NModal
 * internally, so we no longer need the manual trap-tab plumbing.
 */
import { CheckCircle, Play, XCircle } from '@lucide/vue'
import UiButton from '@components/ui/UiButton.vue'
import UiDialog from '@components/ui/UiDialog.vue'
import { computed } from 'vue'
import {
  getTraversalLayoutPointCount,
  type PreconditionCheckResult,
  type TraversalTestConfig
} from '@shared/types/traversal'
import { useI18nStore } from '@stores/i18nStore'

defineProps<{
  show: boolean
  currentConfig: TraversalTestConfig | null
  isCheckingPreconditions: boolean
  preconditionResult: PreconditionCheckResult | null
  labels: {
    title: string
    message: string
    checking: string
    points: string
    output: string
    dismiss: string
    start: string
  }
}>()

const i18n = useI18nStore()
const t = computed(() => i18n.t)

// 后端 checks[].message 为硬编码英文，此处按精确文案映射到 i18n key，
// 使前置条件检查项在中文/英文界面下跟随语言；未命中（如动态 restore 错误）回退原 message。
const MESSAGE_I18N: Record<string, string> = {
  'Load PRB or calibration CSV before running interpolation': 'checkPRBMessage',
  'Motion manager is available': 'checkMotionMessage',
  'DAQ acquisition hub is available': 'checkDAQMessage',
  'All required channel labels are mapped': 'checkChannelMapOk',
  'Patm channel label is required for pressure normalization': 'checkChannelMapFailPatm',
  'Tatm channel label is required for atmospheric calculation': 'checkChannelMapFailTatm',
}

function checkMessage(check: { message?: string; name: string }): string {
  if (check.message) {
    const key = MESSAGE_I18N[check.message]
    const translated = key ? (t.value as Record<string, string>)[key] : undefined
    return translated ?? check.message
  }
  return check.name
}

defineEmits<{
  confirm: []
  cancel: []
}>()
</script>

<template>
  <UiDialog
    :show="show"
    :title="labels.title"
    width="480px"
    :closable="true"
    @update:show="(v) => !v && $emit('cancel')"
  >
    <div class="space-y-3">
      <p class="text-xs text-[var(--text-muted)]">{{ labels.message }}</p>

      <!-- 前置条件检查结果 -->
      <div v-if="isCheckingPreconditions" class="flex items-center gap-2 text-xs text-[var(--text-muted)]">
        <div
          class="h-4 w-4 animate-spin rounded-full border-2"
          :style="{ borderColor: 'var(--accent-info)', borderTopColor: 'transparent' }"
        ></div>
        {{ labels.checking }}
      </div>
      <div v-else-if="preconditionResult" class="space-y-1.5">
        <div
          v-for="check in preconditionResult.checks"
          :key="check.name"
          class="flex items-center gap-2 rounded-md px-2.5 py-1.5 text-xs"
          :style="{
            background: check.passed
              ? 'color-mix(in srgb, var(--state-success) 10%, transparent)'
              : 'color-mix(in srgb, var(--state-error) 10%, transparent)',
          }"
        >
          <CheckCircle v-if="check.passed" class="h-3.5 w-3.5 shrink-0 text-[var(--state-success)]" />
          <XCircle v-else class="h-3.5 w-3.5 shrink-0 text-[var(--state-error)]" />
          <span :style="{ color: check.passed ? 'var(--text-secondary)' : 'var(--state-error)' }" :class="check.passed ? '' : 'font-medium'">
            {{ checkMessage(check) }}
          </span>
        </div>
      </div>

      <!-- 测试摘要 -->
      <div
        v-if="currentConfig"
        class="rounded-md p-2.5"
        :style="{ background: 'var(--bg-panel-strong)' }"
      >
        <div class="grid grid-cols-2 gap-2 text-xs">
          <div class="flex items-baseline gap-1.5">
            <span class="text-[var(--text-muted)]">{{ labels.points }}</span>
            <span class="font-mono font-semibold tabular-nums text-[var(--accent-info)]">{{ currentConfig.layout ? getTraversalLayoutPointCount(currentConfig.layout) : '--' }}</span>
          </div>
          <div class="flex items-baseline gap-1.5 min-w-0">
            <span class="text-[var(--text-muted)] shrink-0">{{ labels.output }}</span>
            <span class="truncate text-[var(--text-secondary)]">{{ currentConfig.savePath || '--' }}</span>
          </div>
        </div>
      </div>
    </div>

    <template #footer>
      <div class="flex items-center justify-end gap-2">
        <UiButton quaternary size="sm" @click="$emit('cancel')">
          {{ labels.dismiss }}
        </UiButton>
        <UiButton
          :variant="preconditionResult?.allPassed ? 'primary' : 'danger'"
          size="sm"
          :disabled="!preconditionResult || isCheckingPreconditions || !preconditionResult.allPassed"
          @click="$emit('confirm')"
        >
          <template #icon>
            <Play class="h-3.5 w-3.5 fill-current" />
          </template>
          {{ labels.start }}
        </UiButton>
      </div>
    </template>
  </UiDialog>
</template>