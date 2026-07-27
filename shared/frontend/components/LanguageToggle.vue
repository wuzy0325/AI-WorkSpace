<script setup lang="ts">
import { Globe } from '@lucide/vue'

type Locale = 'zh' | 'en'

const props = defineProps<{
  locale: Locale
  toggleLabel: string
  switchToZhLabel: string
  switchToEnLabel: string
}>()

const emit = defineEmits<{
  change: [locale: Locale]
}>()
</script>

<template>
  <div class="lang-toggle" role="group" :aria-label="props.toggleLabel">
    <button
      class="lang-btn"
      :class="{ 'lang-btn--active': props.locale === 'zh' }"
      :aria-label="props.switchToZhLabel"
      :aria-pressed="props.locale === 'zh'"
      :title="props.toggleLabel"
      data-testid="btn-locale-zh"
      @click="emit('change', 'zh')"
    >
      <Globe class="lang-btn__icon" />
      <span>中</span>
    </button>
    <button
      class="lang-btn"
      :class="{ 'lang-btn--active': props.locale === 'en' }"
      :aria-label="props.switchToEnLabel"
      :aria-pressed="props.locale === 'en'"
      :title="props.toggleLabel"
      data-testid="btn-locale-en"
      @click="emit('change', 'en')"
    >
      <Globe class="lang-btn__icon" />
      <span>EN</span>
    </button>
  </div>
</template>

<style scoped>
.lang-toggle {
  display: flex;
  align-items: center;
  gap: 0.2rem;
  padding: 0.15rem;
  background: var(--btn-bg);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  height: 36px;
}

.lang-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.25rem;
  height: 26px;
  min-width: 38px;
  padding: 0 0.45rem;
  border-radius: var(--radius-sm);
  font-size: 0.65rem;
  font-weight: 700;
  color: var(--text-secondary);
  background: transparent;
  border: none;
  cursor: pointer;
  transition: all var(--motion-fast) var(--easing-standard);
}

.lang-btn:hover {
  color: var(--text-primary);
  background: var(--btn-bg-hover);
}

.lang-btn--active,
.lang-btn--active:hover {
  color: var(--accent);
  background: var(--accent-muted);
}

.lang-btn__icon {
  width: 12px;
  height: 12px;
}

@media (max-width: 767px) {
  .lang-toggle {
    height: 32px;
  }

  .lang-btn {
    height: 22px;
    min-width: 32px;
    padding: 0 0.35rem;
  }
}
</style>
