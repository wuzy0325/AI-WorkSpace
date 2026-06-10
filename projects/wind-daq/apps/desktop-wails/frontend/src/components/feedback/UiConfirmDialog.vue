<script setup lang="ts">
import { useFeedbackStore } from '@stores/feedbackStore'
import UiButton from '@components/ui/UiButton.vue'

const feedbackStore = useFeedbackStore()
</script>

<template>
  <div v-if="feedbackStore.confirmState.open" class="confirm-overlay">
    <div class="confirm-dialog">
      <h3 class="confirm-dialog__title">{{ feedbackStore.confirmState.title }}</h3>
      <p class="confirm-dialog__message">{{ feedbackStore.confirmState.message }}</p>
      <div class="confirm-dialog__actions">
        <UiButton
          quaternary
          size="md"
          @click="feedbackStore.resolveConfirm(false)"
        >
          {{ feedbackStore.confirmState.cancelText }}
        </UiButton>
        <UiButton
          variant="danger"
          size="md"
          @click="feedbackStore.resolveConfirm(true)"
        >
          {{ feedbackStore.confirmState.confirmText }}
        </UiButton>
      </div>
    </div>
  </div>
</template>

<style scoped>
.confirm-overlay {
  position: fixed;
  inset: 0;
  z-index: 110;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.6);
  padding: 1rem;
}

.confirm-dialog {
  width: 100%;
  max-width: 420px;
  border-radius: 0.75rem;
  border: 1px solid var(--border-default);
  background: var(--bg-panel-strong);
  padding: 1.25rem;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.4);
}

.confirm-dialog__title {
  margin: 0;
  font-size: 1rem;
  font-weight: 600;
  color: var(--text-primary);
}

.confirm-dialog__message {
  margin: 0.75rem 0 0;
  white-space: pre-line;
  font-size: 0.875rem;
  color: var(--text-secondary);
  line-height: 1.5;
}

.confirm-dialog__actions {
  margin-top: 1.25rem;
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
}

.confirm-dialog__btn {
  padding: 0.375rem 0.75rem;
  border-radius: 0.375rem;
  font-size: 0.8125rem;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.2s ease;
  border: 1px solid transparent;
}

.confirm-dialog__btn--cancel {
  background: rgba(255, 255, 255, 0.06);
  color: var(--text-secondary);
  border-color: rgba(255, 255, 255, 0.1);
}

.confirm-dialog__btn--cancel:hover {
  background: rgba(255, 255, 255, 0.1);
}

.confirm-dialog__btn--confirm {
  background: rgba(244, 63, 94, 0.12);
  color: var(--accent-danger);
  border-color: rgba(244, 63, 94, 0.25);
}

.confirm-dialog__btn--confirm:hover {
  background: rgba(244, 63, 94, 0.2);
}

:root[data-theme='light'] .confirm-dialog__btn--cancel {
  background: rgba(0, 0, 0, 0.05);
  color: var(--text-secondary);
  border-color: rgba(0, 0, 0, 0.1);
}

:root[data-theme='light'] .confirm-dialog__btn--cancel:hover {
  background: rgba(0, 0, 0, 0.08);
}
</style>
