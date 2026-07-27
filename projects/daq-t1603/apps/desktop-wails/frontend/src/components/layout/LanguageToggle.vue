<script setup lang="ts">
import { Globe } from '@lucide/vue'
import { useI18nStore } from '@stores/i18nStore'

/**
 * 顶栏语言切换按钮组。
 *
 * 设计：
 * - 双按钮形态（中 / EN），active 态高亮当前语言，比下拉菜单少一次点击
 * - 复用全局 .topbar__icon-btn 视觉语言，与主题切换、配置等按钮风格一致
 * - 切换通过 i18nStore.setLocale 持久化到 localStorage，下次启动自动恢复
 */
const i18n = useI18nStore()
</script>

<template>
  <div class="lang-toggle" role="group" :aria-label="i18n.t('topbar.toggleLanguage')">
    <button
      class="lang-btn"
      :class="{ 'lang-btn--active': i18n.locale === 'zh' }"
      :aria-label="i18n.t('topbar.switchToZh')"
      :aria-pressed="i18n.locale === 'zh'"
      :title="i18n.t('topbar.toggleLanguage')"
      data-testid="btn-locale-zh"
      @click="i18n.setLocale('zh')"
    >
      <Globe class="lang-btn__icon" />
      <span>中</span>
    </button>
    <button
      class="lang-btn"
      :class="{ 'lang-btn--active': i18n.locale === 'en' }"
      :aria-label="i18n.t('topbar.switchToEn')"
      :aria-pressed="i18n.locale === 'en'"
      :title="i18n.t('topbar.toggleLanguage')"
      data-testid="btn-locale-en"
      @click="i18n.setLocale('en')"
    >
      <Globe class="lang-btn__icon" />
      <span>EN</span>
    </button>
  </div>
</template>

<style scoped>
/* 容器：与单个 icon-btn 等高，内部两个按钮紧凑排列 */
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

/* 单个语言按钮：默认透明背景，激活时高亮 */
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

/* 激活态：用 accent 色高亮，与 nav active 视觉语言一致 */
.lang-btn--active {
  color: var(--accent);
  background: var(--accent-muted);
}

.lang-btn--active:hover {
  color: var(--accent);
  background: var(--accent-muted);
}

.lang-btn__icon {
  width: 12px;
  height: 12px;
}

/* 窄屏：与 icon-btn 一起缩小 */
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
