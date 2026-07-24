<script setup lang="ts">
import { watch, onMounted } from 'vue'
import MotionControlPanel from '@components/motion/MotionControlPanel.vue'
import UiButton from '@components/ui/UiButton.vue'
import UiTooltip from '@components/ui/UiTooltip.vue'
import { useI18nStore } from '@stores/i18nStore'

const i18n = useI18nStore()

// 关闭当前独立窗口。
// 在 Electron 桌面端：window.close() 会触发 main.cjs 中 motionWindow 的 'closed' 事件，
// 主进程随之 kill motion-only 子进程并清理 motionBackendProcess 引用。
// 在浏览器开发态：window.close() 关闭当前标签页（或被浏览器拦截，无害）。
async function closeWindow(): Promise<void> {
  window.close()
}

// 根据当前语言同步独立窗口标题
// Electron 下不支持 Window.SetTitle，通过 document.title 设置窗口标题
function syncWindowTitle(): void {
  document.title = i18n.t.motion_windowTitle
}

onMounted(syncWindowTitle)
watch(() => i18n.locale, syncWindowTitle)
</script>

<template>
  <div data-test="motion-shell" class="flex h-full min-h-0 flex-col overflow-hidden font-sans" style="background:var(--bg-canvas);color:var(--text-primary)">
    <header class="motion-shell-header flex shrink-0 items-center justify-between px-5 py-3">
      <div class="flex items-center gap-3 min-w-0">
        <h1 class="motion-shell-title">{{ i18n.t.motion_shellTitle }}</h1>
        <span class="motion-shell-subtitle">{{ i18n.t.motion_shellSubtitle }}</span>
        <UiTooltip :content="i18n.t.motion_shellHelpTooltip" position="right">
          <span class="help-icon" :aria-label="i18n.t.motion_helpAriaLabel">?</span>
        </UiTooltip>
      </div>
      <div class="flex items-center gap-2">
        <!-- 关闭窗口按钮：退出独立进程 -->
        <UiButton variant="ghost" size="sm" @click="closeWindow" :title="i18n.t.motion_closeWindow">
          <template #icon>
            <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <line x1="18" y1="6" x2="6" y2="18"/>
              <line x1="6" y1="6" x2="18" y2="18"/>
            </svg>
          </template>
          {{ i18n.t.motion_closeWindow }}
        </UiButton>
      </div>
    </header>
    <main class="flex-1 min-h-0 p-4 overflow-auto" style="background:var(--bg-canvas)">
      <MotionControlPanel />
    </main>
  </div>
</template>

<style scoped>
.motion-shell-header {
  background: var(--bg-panel);
  border-bottom: 1px solid var(--border-default);
}
.motion-shell-title {
  font-size: 15px;
  font-weight: 700;
  letter-spacing: -0.01em;
  color: var(--text-primary);
  line-height: 1.2;
}
.motion-shell-subtitle {
  font-size: 11px;
  font-weight: 500;
  color: var(--text-muted);
  border-left: 1px solid var(--border-default);
  padding-left: 12px;
  line-height: 1.2;
}
.help-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  border-radius: 50%;
  background: var(--bg-panel-strong);
  border: 1px solid var(--border-default);
  color: var(--text-muted);
  font-size: 10px;
  font-weight: 700;
  cursor: help;
  transition: color 0.15s, border-color 0.15s;
}
.help-icon:hover {
  color: var(--accent-primary);
  border-color: var(--accent-primary);
}
</style>
