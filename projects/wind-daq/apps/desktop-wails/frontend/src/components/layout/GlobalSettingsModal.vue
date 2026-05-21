<script setup lang="ts">
import { ref } from 'vue'
import { useThemeStore } from '@stores/themeStore'
import { useI18nStore } from '@stores/i18nStore'

defineProps<{ open: boolean }>()
const emit = defineEmits<{ (e: 'update:open', v: boolean): void }>()

const themeStore = useThemeStore()
const i18n = useI18nStore()
const activeTab = ref<'appearance' | 'language' | 'about'>('appearance')
</script>

<template>
  <Teleport to="body">
    <div v-if="open" class="modal-overlay" @click.self="emit('update:open', false)">
      <div class="modal">
        <div class="modal__head">
          <h2>设置</h2>
          <button class="modal__close" @click="emit('update:open', false)">✕</button>
        </div>

        <div class="modal__tabs">
          <button
            v-for="tab in (['appearance', 'language', 'about'] as const)"
            :key="tab"
            :class="{ active: activeTab === tab }"
            @click="activeTab = tab"
          >
            {{ tab === 'appearance' ? '外观' : tab === 'language' ? '语言' : '关于' }}
          </button>
        </div>

        <div class="modal__body">
          <div v-if="activeTab === 'appearance'" class="modal__section">
            <h3>主题</h3>
            <div class="modal__row">
              <button
                class="theme-btn"
                :class="{ active: themeStore.theme === 'dark' }"
                @click="themeStore.setTheme('dark')"
              >
                <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>
                深色
              </button>
              <button
                class="theme-btn"
                :class="{ active: themeStore.theme === 'light' }"
                @click="themeStore.setTheme('light')"
              >
                <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="5"/><path d="M12 1v2M12 21v2M4.22 4.22l1.42 1.42M18.36 18.36l1.42 1.42M1 12h2M21 12h2M4.22 19.78l1.42-1.42M18.36 5.64l1.42-1.42"/></svg>
                浅色
              </button>
            </div>
          </div>

          <div v-else-if="activeTab === 'language'" class="modal__section">
            <h3>界面语言</h3>
            <div class="modal__row">
              <button class="lang-btn" :class="{ active: i18n.locale === 'zh' }" @click="i18n.setLocale('zh')">中文</button>
              <button class="lang-btn" :class="{ active: i18n.locale === 'en' }" @click="i18n.setLocale('en')">English</button>
            </div>
          </div>

          <div v-else class="modal__section">
            <h3>Wind-DAQ</h3>
            <p>版本 0.1.0</p>
            <p class="modal__muted">数据采集与运动控制系统</p>
            <p class="modal__muted">基于 Go + Vue 3 + Wails 重构</p>
          </div>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.modal-overlay {
  position: fixed;
  inset: 0;
  z-index: 200;
  background: rgba(0, 0, 0, 0.6);
  backdrop-filter: blur(4px);
  display: flex;
  align-items: center;
  justify-content: center;
}

.modal {
  width: 420px;
  max-width: 90vw;
  border-radius: 1rem;
  background: var(--bg-panel);
  border: 1px solid var(--border-default);
  box-shadow: 0 25px 50px rgba(0, 0, 0, 0.4);
}

.modal__head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: var(--space-5) var(--space-6);
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}

.modal__head h2 {
  margin: 0;
  font-size: 1.1rem;
}

.modal__close {
  width: 32px;
  height: 32px;
  display: grid;
  place-items: center;
  border-radius: 0.5rem;
  color: var(--text-muted);
  background: rgba(255, 255, 255, 0.05);
}

.modal__tabs {
  display: flex;
  gap: 0;
  padding: 0 var(--space-6);
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}

.modal__tabs button {
  padding: 0.6rem 1rem;
  font-size: 0.8rem;
  font-weight: 700;
  color: var(--text-muted);
  border-bottom: 2px solid transparent;
  transition: all 0.2s ease;
}

.modal__tabs button.active {
  color: var(--accent-success);
  border-bottom-color: var(--accent-success);
}

.modal__body {
  padding: var(--space-5) var(--space-6);
}

.modal__section h3 {
  margin: 0 0 var(--space-3);
  font-size: 0.8rem;
  font-weight: 700;
  color: var(--text-muted);
  letter-spacing: 0.06em;
  text-transform: uppercase;
}

.modal__section p {
  margin: 0.25rem 0;
  font-size: 0.85rem;
}

.modal__muted {
  color: var(--text-muted);
  font-size: 0.8rem;
}

.modal__row {
  display: flex;
  gap: 0.75rem;
}

.theme-btn,
.lang-btn {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.5rem 1rem;
  border-radius: 0.5rem;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(30, 41, 59, 0.4);
  color: var(--text-secondary);
  font-size: 0.8rem;
  font-weight: 700;
  transition: all 0.2s ease;
}

.theme-btn.active,
.lang-btn.active {
  border-color: rgba(34, 197, 94, 0.4);
  background: rgba(34, 197, 94, 0.1);
  color: #22c55e;
}
</style>
