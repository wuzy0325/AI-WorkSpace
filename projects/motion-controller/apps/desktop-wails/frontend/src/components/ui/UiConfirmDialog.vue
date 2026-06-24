<script setup lang="ts">
import { useFeedbackStore } from '@stores/feedbackStore'

const feedbackStore = useFeedbackStore()
</script>

<template>
  <Teleport to="body">
    <Transition
      enter-active-class="transition ease-out duration-200"
      enter-from-class="opacity-0"
      enter-to-class="opacity-100"
      leave-active-class="transition ease-in duration-150"
      leave-from-class="opacity-100"
      leave-to-class="opacity-0"
    >
      <div v-if="feedbackStore.confirmState.open" class="fixed inset-0 z-[300] flex items-center justify-center bg-black/60 backdrop-blur-sm px-4">
        <Transition
          enter-active-class="transition ease-out duration-300"
          enter-from-class="opacity-0 scale-95"
          enter-to-class="opacity-100 scale-100"
          leave-active-class="transition ease-in duration-200"
          leave-from-class="opacity-100 scale-100"
          leave-to-class="opacity-0 scale-95"
        >
          <div v-if="feedbackStore.confirmState.open" class="w-full max-w-md rounded-xl border border-[color:var(--border-default)] bg-[color:var(--bg-panel)] p-5 shadow-2xl">
            <h3 class="text-sm font-bold text-[color:var(--text-primary)]">{{ feedbackStore.confirmState.title }}</h3>
            <p class="mt-3 whitespace-pre-line text-xs text-[color:var(--text-secondary)] leading-relaxed">{{ feedbackStore.confirmState.message }}</p>
            <div class="mt-5 flex justify-end gap-2">
              <button
                class="rounded-lg border border-[color:var(--border-default)] bg-[color:var(--bg-panel-strong)] px-4 py-2 text-xs font-semibold text-[color:var(--text-secondary)] transition-colors hover:bg-[color:var(--bg-panel)] hover:text-[color:var(--text-primary)]"
                @click="feedbackStore.resolveConfirm(false)"
              >
                {{ feedbackStore.confirmState.cancelText }}
              </button>
              <button
                :class="[
                  'rounded-lg border px-4 py-2 text-xs font-semibold transition-colors',
                  feedbackStore.confirmState.variant === 'danger'
                    ? 'border-[color:var(--accent-danger)]/40 bg-[color:var(--accent-danger)]/20 text-[color:var(--accent-danger)] hover:bg-[color:var(--accent-danger)] hover:text-white'
                    : 'border-[color:var(--accent-success)]/40 bg-[color:var(--accent-success)]/20 text-[color:var(--accent-success)] hover:bg-[color:var(--accent-success)] hover:text-white'
                ]"
                @click="feedbackStore.resolveConfirm(true)"
              >
                {{ feedbackStore.confirmState.confirmText }}
              </button>
            </div>
          </div>
        </Transition>
      </div>
    </Transition>
  </Teleport>
</template>
